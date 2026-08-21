package app.reyna.data

import android.content.Context
import android.content.SharedPreferences
import android.util.Log
import app.reyna.attribution.Attribution
import app.reyna.attribution.ExportParser
import app.reyna.attribution.NotificationReader
import app.reyna.capture.ReconcileScanner
import app.reyna.capture.WhatsAppPaths
import app.reyna.net.ReynaApi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.withContext
import java.io.File
import java.io.Reader
import java.util.concurrent.TimeUnit

/**
 * The single place capture, attribution, storage and the backend meet.
 *
 * Everything the UI can do goes through here, and every capture path lands
 * here, so there is exactly one implementation of "what happens when a file
 * appears" rather than one per entry point.
 */
class Repo private constructor(private val context: Context) {

    private val db = ReynaDb.get(context)
    private val dao = db.dao()
    private val prefs: SharedPreferences =
        context.getSharedPreferences("reyna", Context.MODE_PRIVATE)

    // ── Settings ──

    var backendUrl: String
        get() = prefs.getString(KEY_BACKEND, DEFAULT_BACKEND) ?: DEFAULT_BACKEND
        set(v) = prefs.edit().putString(KEY_BACKEND, v).apply()

    var deviceToken: String
        get() = prefs.getString(KEY_TOKEN, "") ?: ""
        set(v) = prefs.edit().putString(KEY_TOKEN, v).apply()

    var capturing: Boolean
        get() = prefs.getBoolean(KEY_CAPTURING, true)
        set(v) = prefs.edit().putBoolean(KEY_CAPTURING, v).apply()

    /** Off by default: capture is meant to be silent. */
    var dailyDigest: Boolean
        get() = prefs.getBoolean(KEY_DIGEST, false)
        set(v) = prefs.edit().putBoolean(KEY_DIGEST, v).apply()

    var onboarded: Boolean
        get() = prefs.getBoolean(KEY_ONBOARDED, false)
        set(v) = prefs.edit().putBoolean(KEY_ONBOARDED, v).apply()

    private fun api() = ReynaApi(backendUrl, deviceToken)

    // ── Reads for the UI ──

    fun observeFiles(): Flow<List<FileEntity>> = dao.observeFiles()
    fun observeFileCount(): Flow<Int> = dao.observeFileCount()
    fun observeMessages(): Flow<List<MessageEntity>> = dao.observeMessages()

    suspend fun watchedChats(): List<String> = withContext(Dispatchers.IO) { dao.knownChats() }

    suspend fun unattributed(): List<FileEntity> = withContext(Dispatchers.IO) {
        dao.unattributed(Attribution.MIN_NAMED)
    }

    // ── Capture ──

    /**
     * Records one file and attributes it.
     *
     * Deduplicated by content hash rather than path, because the observer and
     * the reconcile scan both see the same file, and the same document
     * forwarded twice is one document. Returns null when it was already known.
     */
    suspend fun onFileFound(found: ReconcileScanner.Found): FileEntity? = withContext(Dispatchers.IO) {
        if (dao.countByHash(found.sha256) > 0) return@withContext null

        val entity = FileEntity(
            path = found.file.absolutePath,
            name = found.file.name,
            sha256 = found.sha256,
            sizeBytes = found.sizeBytes,
            mtime = found.mtimeMillis,
            // Until attribution runs, the only time we know is when it landed.
            postedAt = found.mtimeMillis,
            isImage = found.file.extension.lowercase() in IMAGE_EXT,
            isSent = found.isSent,
        )
        val id = dao.insertFile(entity)
        if (id <= 0) return@withContext null

        attributeFile(id)
        dao.file(id)
    }

    /**
     * Takes a file the user handed to Reyna directly, rather than one found by
     * watching the folder.
     *
     * Copied into Reyna's own storage rather than referenced in place. The
     * picker hands back a content URI whose read grant does not survive a
     * reboot, so keeping the URI would give a file that opens today and fails
     * next week. Attribution is self_sent at full confidence: the user handed
     * it over themselves, which is the one thing we can be certain of.
     *
     * Returns the outcome rather than a bare boolean so the caller can write a
     * receipt into the conversation naming the file and linking to it. A
     * transient toast is the wrong acknowledgement in a chat: it is gone before
     * the user can act on it, and it leaves nothing to tap.
     */
    suspend fun importFile(uri: android.net.Uri): Imported = withContext(Dispatchers.IO) {
        val name = displayName(uri) ?: return@withContext Imported.Unreadable
        val dir = File(context.filesDir, "added").apply { mkdirs() }
        val dest = File(dir, name)

        runCatching {
            context.contentResolver.openInputStream(uri)?.use { input ->
                dest.outputStream().use { input.copyTo(it) }
            } ?: return@withContext Imported.Unreadable
        }.getOrElse { return@withContext Imported.Unreadable }

        val hash = runCatching { ReconcileScanner.sha256(dest) }.getOrElse {
            dest.delete()
            return@withContext Imported.Unreadable
        }
        dao.fileByHash(hash)?.let { existing ->
            // Already held. Drop the copy rather than leaving a duplicate on
            // disk that nothing points at, and point at the one we kept.
            dest.delete()
            return@withContext Imported.Duplicate(existing.id, existing.name)
        }

        val now = System.currentTimeMillis()
        val id = dao.insertFile(
            FileEntity(
                path = dest.absolutePath,
                name = name,
                sha256 = hash,
                sizeBytes = dest.length(),
                mtime = now,
                postedAt = now,
                isImage = dest.extension.lowercase() in IMAGE_EXT,
                isSent = true,
                senderName = "You",
                confidence = 1.0,
                method = Attribution.Method.SELF_SENT,
            )
        )
        if (id <= 0) {
            dest.delete()
            return@withContext Imported.Unreadable
        }
        syncPending()
        Imported.Added(id, name)
    }

    private fun displayName(uri: android.net.Uri): String? {
        context.contentResolver.query(uri, null, null, null, null)?.use { c ->
            val i = c.getColumnIndex(android.provider.OpenableColumns.DISPLAY_NAME)
            if (i >= 0 && c.moveToFirst()) return c.getString(i)
        }
        return uri.lastPathSegment?.substringAfterLast('/')
    }

    /** Runs a full scan and records anything new. Returns how many were added. */
    suspend fun reconcile(): Int = withContext(Dispatchers.IO) {
        if (!WhatsAppPaths.anyVisible()) return@withContext 0
        // Fetched once rather than queried per file. The scan runs every
        // fifteen minutes over a folder that mostly does not change, so the
        // difference between one query and two thousand is the difference
        // between a job nobody notices and one that shows up in battery stats.
        val known = dao.knownPathKeys().toHashSet()
        val scanner = ReconcileScanner { path, mtime -> "$path|$mtime" in known }
        var added = 0
        for (found in scanner.scan()) {
            if (onFileFound(found) != null) added++
        }
        Log.i(TAG, "reconcile added $added")
        added
    }

    // ── Attribution ──

    /** Stores a notification, then re-attributes anything it might explain. */
    suspend fun onNotification(obs: NotificationReader.Observed) = withContext(Dispatchers.IO) {
        dao.insertEvent(
            EventEntity(
                chatKey = obs.shortcutId ?: obs.chatName,
                chatName = obs.chatName,
                senderKey = obs.senderKey,
                senderDisplay = obs.senderName,
                postedAt = obs.postedAtMillis,
                text = obs.text,
                attachmentName = "",
                hasAttachment = true,
                source = if (obs.source == NotificationReader.Observed.Source.MESSAGING_STYLE)
                    "notification" else "notification_fallback",
            )
        )
        // The join runs in both directions: a file downloaded hours before this
        // notification arrived is still explained by it.
        reattributeWeak()

        // The server keeps its own copy so it can re-join files uploaded from
        // anywhere. Best effort: attribution already happened locally, and a
        // failed send is retried on the next sync rather than losing the event.
        if (deviceToken.isNotBlank()) {
            api().sendEvents(
                obs.chatName,
                listOf(
                    ReynaApi.DeviceEvent(
                        chatKey = obs.shortcutId ?: obs.chatName,
                        chatName = obs.chatName,
                        senderName = obs.senderName,
                        postedAtSeconds = obs.postedAtMillis / 1000,
                        text = obs.text,
                        attachmentName = "",
                        source = "notification",
                    )
                ),
            )
        }
    }

    /**
     * Imports a chat export.
     *
     * Parsed here, on the device, and only the attachment rows are kept. The
     * export contains every message the group ever sent and Reyna needs about
     * two percent of it; parsing locally and discarding the rest is what makes
     * the promise checkable, and it is why an import works in airplane mode.
     *
     * Returns how many messages were read and how many file records were kept,
     * so the UI can show the receipt before anything is saved.
     */
    suspend fun importExport(reader: Reader): ImportResult = withContext(Dispatchers.IO) {
        val parsed = ExportParser.parse(reader)
        val attachments = parsed.attachments()
        var stored = 0
        for (m in attachments) {
            val id = dao.insertEvent(
                EventEntity(
                    chatKey = "export",
                    chatName = "",
                    senderKey = "",
                    senderDisplay = m.sender,
                    postedAt = m.postedAtMillis,
                    text = m.text,
                    attachmentName = m.attachmentName,
                    hasAttachment = true,
                    source = "export",
                )
            )
            if (id > 0) stored++
        }
        val improved = reattributeWeak()

        // Only the attachment rows travel. The raw chat text is parsed here and
        // discarded here, which is what makes the airplane-mode claim true.
        if (deviceToken.isNotBlank() && attachments.isNotEmpty()) {
            api().sendExport(
                chatName = null,
                events = attachments.map { m ->
                    ReynaApi.DeviceEvent(
                        chatKey = "export",
                        chatName = "",
                        senderName = m.sender,
                        postedAtSeconds = m.postedAtMillis / 1000,
                        text = "",
                        attachmentName = m.attachmentName,
                        source = "export",
                    )
                },
                dateOrderProven = parsed.dateOrder.proven,
            )
        }

        ImportResult(
            messagesRead = parsed.messages.size,
            filesFound = attachments.size,
            eventsStored = stored,
            filesAttributed = improved,
            dateOrder = parsed.dateOrder.toString(),
        )
    }

    data class ImportResult(
        val messagesRead: Int,
        val filesFound: Int,
        val eventsStored: Int,
        val filesAttributed: Int,
        val dateOrder: String,
    )

    /**
     * Re-runs the join over files we cannot yet name.
     *
     * Only the weak ones: a file already attributed at full confidence has
     * nothing to gain, and re-deciding it risks replacing a fact with a guess.
     */
    suspend fun reattributeWeak(): Int = withContext(Dispatchers.IO) {
        var improved = 0
        for (f in dao.unattributed(Attribution.MIN_NAMED)) {
            if (attributeFile(f.id)) improved++
        }
        improved
    }

    /**
     * Attributes one file against everything we know.
     *
     * Returns true when it improved. Events are pulled from a window around the
     * file rather than the whole table, since a message a year away cannot
     * explain it and scanning everything would grow linearly forever.
     */
    private suspend fun attributeFile(fileId: Long): Boolean {
        val f = dao.file(fileId) ?: return false
        val window = TimeUnit.DAYS.toMillis(3)
        val events = dao.eventsBetween(f.mtime - window, f.mtime + window)
            .ifEmpty { dao.recentEvents(200) }

        val result = Attribution.attribute(
            file = Attribution.CapturedFile(
                id = f.id, diskName = f.name, mtimeMillis = f.mtime, isSent = f.isSent,
            ),
            events = events.map {
                Attribution.Event(
                    id = it.id, chatKey = it.chatKey, chatName = it.chatName,
                    senderKey = it.senderKey, senderDisplay = it.senderDisplay,
                    postedAtMillis = it.postedAt, text = it.text,
                    attachmentName = it.attachmentName, hasAttachment = it.hasAttachment,
                    source = it.source,
                )
            },
        )

        if (result.confidence <= f.confidence) return false

        val winner = result.best?.eventId?.let { id -> events.firstOrNull { it.id == id } }
        dao.deactivateLinks(f.id)
        for (link in result.candidates) {
            dao.insertLink(
                LinkEntity(
                    fileId = link.fileId, eventId = link.eventId,
                    method = link.method, confidence = link.confidence,
                    isActive = link === result.best,
                    linkedAt = System.currentTimeMillis(),
                )
            )
        }
        dao.setAttribution(
            id = f.id,
            sender = if (result.method == Attribution.Method.SELF_SENT) "You" else winner?.senderDisplay,
            chat = winner?.chatName?.ifBlank { null },
            confidence = result.confidence,
            method = result.method,
            // The event's time is the real message time; the file's mtime is
            // only when it reached the disk.
            postedAt = winner?.postedAt ?: f.postedAt,
        )
        return true
    }

    /**
     * The user telling us who shared a file.
     *
     * Full confidence, because they know and we do not. This is the repair path
     * behind every "who shared this?" affordance.
     */
    suspend fun setSenderManually(fileId: Long, sender: String) = withContext(Dispatchers.IO) {
        val f = dao.file(fileId) ?: return@withContext
        dao.setAttribution(fileId, sender, f.chatName, 1.0, Attribution.Method.USER, f.postedAt)
        if (deviceToken.isNotBlank() && f.remoteId > 0) {
            api().setSender(f.remoteId, sender)
        }
    }

    /**
     * The Google OAuth URL, or null when Drive is not configured on the server.
     *
     * The app cannot complete OAuth itself: the client secret lives on the
     * server, and shipping it in an APK would hand it to every user. So the
     * server mints the URL and the callback lands back there.
     */
    suspend fun driveConnectUrl(): String? = withContext(Dispatchers.IO) {
        if (deviceToken.isBlank()) return@withContext null
        api().driveConnectUrl().getOrNull()
    }

    suspend fun driveConnected(): Boolean = withContext(Dispatchers.IO) {
        if (deviceToken.isBlank()) false else api().driveConnected()
    }

    // ── Backend ──

    /** Uploads anything not yet accepted. Safe to call repeatedly. */
    suspend fun syncPending(limit: Int = 20): Int = withContext(Dispatchers.IO) {
        if (deviceToken.isBlank()) return@withContext 0
        val api = api()
        var sent = 0
        for (f in dao.pendingUpload(limit)) {
            val file = File(f.path)
            // WhatsApp reclaims media, so a row can outlive its file. Marked
            // uploaded to stop retrying something that will never succeed.
            if (!file.isFile) {
                dao.markUploaded(f.id, 0, null)
                continue
            }
            val result = api.upload(
                file = file,
                fileName = f.name,
                mimeType = mimeOf(f.name, f.isImage),
                chatName = f.chatName,
                senderName = if (f.confidence >= Attribution.MIN_NAMED) f.senderName else null,
                postedAtSeconds = f.postedAt / 1000,
                confidence = f.confidence,
                method = f.method,
            )
            result.onSuccess {
                dao.markUploaded(f.id, it.remoteId, it.folder)
                sent++
            }
            if (result.isFailure) break // Server is down; try again later.
        }
        sent
    }

    /**
     * Asks a question and records both turns.
     *
     * [onCall] hands the in-flight HTTP call up so it can be cancelled. The
     * user's own message is written before the request goes out, so a question
     * is never lost if the answer fails or is stopped.
     */
    suspend fun ask(
        question: String,
        onCall: (okhttp3.Call) -> Unit = {},
    ): ReynaApi.Answer? = withContext(Dispatchers.IO) {
        dao.insertMessage(MessageEntity(text = question, fromUser = true, at = System.currentTimeMillis()))
        val answer = api().ask(question, onCall).getOrNull()
        val reply = answer?.reply
            ?: "I could not reach the backend. Your files are still safe on this phone."

        // Match the cited filenames back to local rows so the answer can show
        // real chips. Matched by name because the backend numbers files by its
        // own ids, which the phone does not share.
        val local = dao.allFiles()
        val citedIds = answer?.files.orEmpty().mapNotNull { cited ->
            local.firstOrNull { it.name.equals(cited.name, ignoreCase = true) }?.id
        }

        dao.insertMessage(
            MessageEntity(
                text = reply,
                fromUser = false,
                at = System.currentTimeMillis(),
                fileIds = citedIds.joinToString(","),
            )
        )
        answer
    }

    /** Wipes the conversation. Files and attribution are untouched. */
    suspend fun clearConversation() = withContext(Dispatchers.IO) {
        dao.clearMessages()
    }

    /**
     * Records a message Reyna wrote about something it did, not an answer.
     *
     * [fileIds] attaches chips, so a receipt for a file just added is openable
     * from the conversation instead of sending the user off to the Files tab to
     * find what they were already holding.
     */
    suspend fun say(text: String, fileIds: List<Long> = emptyList()) = withContext(Dispatchers.IO) {
        dao.insertMessage(
            MessageEntity(
                text = text,
                fromUser = false,
                at = System.currentTimeMillis(),
                fileIds = fileIds.joinToString(","),
            )
        )
    }

    /**
     * Opens the conversation, once.
     *
     * Guarded on the conversation being empty. It is called on every app start
     * and again when onboarding finishes, and without the guard each launch
     * appended another copy until the screen was nothing but greetings.
     */
    suspend fun seedGreeting() = withContext(Dispatchers.IO) {
        if (dao.messageCount() > 0) return@withContext
        val files = dao.allFiles().size
        val chats = dao.knownChats().size
        val text = if (files == 0) {
            "I am watching for files now. Share something in a chat, or import a chat to catch up on what is already here."
        } else {
            "I have $files files from $chats chats. Ask me for something you half remember."
        }
        dao.insertMessage(MessageEntity(text = text, fromUser = false, at = System.currentTimeMillis()))
    }

    suspend fun deleteEverything() = withContext(Dispatchers.IO) {
        dao.clearLinks(); dao.clearEvents(); dao.clearFiles(); dao.clearMessages()
    }

    /**
     * The type to tell the server a file is.
     *
     * Derived from the extension. Everything that was not an image used to be
     * sent as application/pdf, so a .docx or .pptx, which is most of what
     * circulates in a class group, arrived mislabelled and was read as the
     * wrong kind of document.
     */
    private fun mimeOf(name: String, isImage: Boolean): String {
        if (isImage) {
            val ext = name.substringAfterLast('.', "").lowercase()
            return android.webkit.MimeTypeMap.getSingleton()
                .getMimeTypeFromExtension(ext) ?: "image/jpeg"
        }
        val ext = name.substringAfterLast('.', "").lowercase()
        return android.webkit.MimeTypeMap.getSingleton()
            .getMimeTypeFromExtension(ext) ?: "application/octet-stream"
    }

    /**
     * What happened to a file handed to Reyna directly.
     *
     * A duplicate carries the id of the copy already held, so the user is shown
     * the file they meant rather than told no.
     */
    sealed interface Imported {
        data class Added(val fileId: Long, val name: String) : Imported
        data class Duplicate(val fileId: Long, val name: String) : Imported
        data object Unreadable : Imported
    }

    companion object {
        private const val TAG = "ReynaRepo"
        private const val KEY_BACKEND = "backend_url"
        private const val KEY_TOKEN = "device_token"
        private const val KEY_CAPTURING = "capturing"
        private const val KEY_DIGEST = "daily_digest"
        private const val KEY_ONBOARDED = "onboarded"

        /** 10.0.2.2 is the host machine as seen from an emulator. */
        private const val DEFAULT_BACKEND = "http://10.0.2.2:8080"

        private val IMAGE_EXT = setOf("jpg", "jpeg", "png", "webp")

        @Volatile private var instance: Repo? = null

        fun get(context: Context): Repo = instance ?: synchronized(this) {
            instance ?: Repo(context.applicationContext).also { instance = it }
        }
    }
}
