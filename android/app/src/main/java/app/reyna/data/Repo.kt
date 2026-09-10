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
import androidx.sqlite.db.SimpleSQLiteQuery
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
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
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val previewDownloadMutex = Mutex()

    init {
        scope.launch {
            cleanBogusData()
        }
    }

    // ── Settings ──

    var backendUrl: String
        get() {
            val saved = prefs.getString(KEY_BACKEND, null)
            if (saved.isNullOrBlank() || (saved == "http://10.0.2.2:8080" && DEFAULT_BACKEND != "http://10.0.2.2:8080")) {
                return DEFAULT_BACKEND
            }
            return saved
        }
        set(v) = prefs.edit().putString(KEY_BACKEND, v).apply()

    /**
     * The shared secret for this backend.
     *
     * Seeded from the build when the user has not set one, so a build made for
     * a known server works on first launch instead of opening on a settings
     * screen. Anything typed in Settings wins from then on.
     */
    var deviceToken: String
        get() {
            val saved = prefs.getString(KEY_TOKEN, null)
            if (saved.isNullOrBlank()) {
                return app.reyna.BuildConfig.DEVICE_TOKEN
            }
            return saved
        }
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

    /** Uses the SQLite inverted index to avoid scanning every OCR body per keystroke. */
    suspend fun searchContentIds(match: String): Set<Long> = withContext(Dispatchers.IO) {
        if (match.isBlank()) return@withContext emptySet()
        dao.searchFileIds(
            SimpleSQLiteQuery(
                "SELECT docid FROM files_fts WHERE files_fts MATCH ? LIMIT 500",
                arrayOf(match),
            )
        ).toSet()
    }

    /**
     * Resolves preview bytes locally. Uploaded files remain previewable after
     * WhatsApp removes its copy by restoring them into Reyna's cache.
     */
    suspend fun previewFile(localPath: String, remoteId: Long, fileName: String): File? =
        withContext(Dispatchers.IO) {
            File(localPath).takeIf { it.isFile && it.length() > 0L }?.let { return@withContext it }
            if (remoteId <= 0L || deviceToken.isBlank()) return@withContext null

            previewDownloadMutex.withLock {
                val ext = fileName.substringAfterLast('.', "").lowercase()
                    .takeIf { it.matches(Regex("[a-z0-9]{1,8}")) }
                val cacheDir = File(context.cacheDir, "previews").apply { mkdirs() }
                val cached = File(cacheDir, remoteId.toString() + if (ext == null) "" else ".$ext")
                if (cached.isFile && cached.length() > 0L) return@withLock cached

                val partial = File(cacheDir, "${cached.name}.part")
                partial.delete()
                api().downloadFile(remoteId, partial).getOrElse {
                    partial.delete()
                    return@withLock null
                }
                if (!partial.renameTo(cached)) {
                    partial.delete()
                    return@withLock null
                }
                cached
            }
        }

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
        if (entity.isImage || entity.name.endsWith(".pdf", ignoreCase = true)) {
            val text = app.reyna.ocr.OnDeviceExtractor.extract(context, found.file, entity.isImage) ?: ""
            dao.updateExtractedText(id, text)
        }
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
    suspend fun reconcile(onProgress: (Int) -> Unit = {}): Int = withContext(Dispatchers.IO) {
        cleanBogusData()
        if (!WhatsAppPaths.anyVisible()) return@withContext 0
        // Fetched once rather than queried per file. The scan runs every
        // fifteen minutes over a folder that mostly does not change, so the
        // difference between one query and two thousand is the difference
        // between a job nobody notices and one that shows up in battery stats.
        val known = dao.knownPathKeys().toHashSet()
        val scanner = ReconcileScanner { path, mtime -> "$path|$mtime" in known }
        var added = 0
        for (found in scanner.scan(onProgress)) {
            if (onFileFound(found) != null) added++
        }
        Log.i(TAG, "reconcile added $added")
        runPendingOcr(5)
        added
    }

    /** Runs on-device text extraction (ML Kit OCR / PdfRenderer) for files needing it. */
    suspend fun runPendingOcr(limit: Int = 10): Int = withContext(Dispatchers.IO) {
        val pending = dao.pendingOcr(limit)
        var done = 0
        for (f in pending) {
            val file = File(f.path)
            if (!file.exists()) {
                dao.updateExtractedText(f.id, "")
                continue
            }
            val text = app.reyna.ocr.OnDeviceExtractor.extract(context, file, f.isImage) ?: ""
            dao.updateExtractedText(f.id, text)
            done++
        }
        if (done > 0) {
            Log.i(TAG, "runPendingOcr processed $done files")
        }
        done
    }

    // ── Attribution ──

    /** Stores a notification, then re-attributes anything it might explain. */
    suspend fun onNotification(obs: NotificationReader.Observed) = withContext(Dispatchers.IO) {
        if (NotificationReader.isSummary(obs.chatName, obs.text)) return@withContext

        val (attName, hasAtt) = NotificationReader.detectAttachment(obs.text)

        dao.insertEvent(
            EventEntity(
                chatKey = obs.shortcutId ?: obs.chatName,
                chatName = obs.chatName,
                senderKey = obs.senderKey,
                senderDisplay = obs.senderName,
                postedAt = obs.postedAtMillis,
                text = obs.text,
                attachmentName = attName,
                hasAttachment = hasAtt,
                source = if (obs.source == NotificationReader.Observed.Source.MESSAGING_STYLE)
                    "notification" else "notification_fallback",
            )
        )

        // When an attachment notification arrives, scan and upload right away
        // so the new file is indexed and on the backend without waiting 15 mins.
        if (hasAtt) {
            reconcile()
            runPendingOcr(5)
            syncPending(10)
        }

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
                        attachmentName = attName,
                        hasAttachment = hasAtt,
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
     * Cleans bogus "WhatsApp" events and resets bogus WhatsApp attributions so
     * files can honestly re-attribute to their real senders or honest degradation.
     */
    suspend fun cleanBogusData(): Boolean = withContext(Dispatchers.IO) {
        val deleted = dao.deleteBogusEvents()
        val reset = dao.resetBogusAttributions()
        if (deleted > 0 || reset > 0) {
            Log.i(TAG, "cleaned bogus events: $deleted, reset files: $reset")
            reattributeWeak()
            true
        } else {
            false
        }
    }

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
        val newSender = if (result.method == Attribution.Method.SELF_SENT) "You" else winner?.senderDisplay
        dao.setAttribution(
            id = f.id,
            sender = newSender,
            chat = winner?.chatName?.ifBlank { null },
            confidence = result.confidence,
            method = result.method,
            // The event's time is the real message time; the file's mtime is
            // only when it reached the disk.
            postedAt = winner?.postedAt ?: f.postedAt,
        )
        if (deviceToken.isNotBlank() && f.remoteId > 0 && !newSender.isNullOrBlank() && result.confidence >= Attribution.MIN_NAMED) {
            runCatching { api().setSender(f.remoteId, newSender) }
        }
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
    /**
     * Sends captured files the server has not seen yet, in the order given.
     *
     * Two kinds of failure, two opposite responses. A file the server *refused*
     * is skipped and retired, because a 4xx is a verdict on that file and
     * sending it again changes nothing. A file that failed to *reach* the
     * server ends the pass, because the network is down and every file behind
     * it will fail the same way.
     *
     * The distinction matters more than it looks. The queue is oldest first, so
     * a single permanently refused file sits at the head and is retried first
     * on every pass, and everything behind it is never sent at all. That is how
     * eighty two files ended up stuck, and why a document plainly visible in
     * Search was invisible to chat: chat searches the server, and the server
     * had never received it.
     */
    suspend fun syncPending(limit: Int = 60): Int = uploadAll(readableOnly(dao.pendingUpload(limit * 8)).take(limit))

    /**
     * Drops everything the server could not read anyway, and retires it so it
     * stops being offered.
     *
     * Done here rather than in the SQL because the queue is defined by an
     * extension, and putting that list in a Room query would freeze it into
     * the schema.
     */
    private suspend fun readableOnly(pending: List<FileEntity>): List<FileEntity> {
        val keep = ArrayList<FileEntity>(pending.size)
        for (f in pending) {
            if (isReadable(f.name)) keep.add(f) else retire(f.id)
        }
        return keep
    }

    /**
     * Sends the pending files whose names look like the question, before
     * anything else.
     *
     * Chat can only answer from what the server holds, and the queue is drained
     * oldest first, so the file someone is asking about right now is usually
     * the last one to be sent. This puts it first. It is a few seconds spent to
     * turn "I could not find that" into an answer, which is the difference
     * between the app being wrong and the app being slow.
     *
     * Matching is on the filename only. The phone has no extracted text, so it
     * cannot judge the contents; it can only notice that a file nobody has read
     * yet is called something like what was asked.
     */
    suspend fun syncMatchingFirst(question: String, limit: Int = 5): Int {
        val tokens = queryTokens(question)
        val pending = readableOnly(dao.pendingUpload(4000))
        if (tokens.isEmpty()) {
            // Questions like "what was the last doc" or "what arrived" don't name a file;
            // sync the most recent pending files so the server has recent context.
            return uploadAll(pending.take(limit))
        }
        val ranked = pending
            .map { f ->
                val nameScore = score(f.name, tokens)
                val senderScore = f.senderName?.let { score(it, tokens) * 3 } ?: 0
                val chatScore = f.chatName?.let { score(it, tokens) * 2 } ?: 0
                val textScore = f.extractedText?.let { score(it, tokens) * 4 } ?: 0
                f to (nameScore + senderScore + chatScore + textScore)
            }
            .filter { it.second > 0 }
            .sortedByDescending { it.second }
            .take(limit)
            .map { it.first }
        val sent = uploadAll(ranked)
        // If question didn't match any filename/sender/chat, also ensure the top recent pending files are synced
        if (sent == 0 && pending.isNotEmpty()) {
            return uploadAll(pending.take(2))
        }
        return sent
    }

    private suspend fun uploadAll(pending: List<FileEntity>): Int = withContext(Dispatchers.IO) {
        if (deviceToken.isBlank()) return@withContext 0
        val api = api()
        var sent = 0
        for (f in pending) {
            val file = File(f.path)
            // WhatsApp reclaims media, so a row can outlive its file. Retired
            // to stop retrying something that will never succeed.
            if (!file.isFile) {
                retire(f.id)
                continue
            }
            // Refused here rather than by the server. Pushing fifty megabytes
            // up a phone connection only to be told no costs the user data and
            // minutes, every pass, forever.
            if (file.length() > MAX_UPLOAD_BYTES) {
                Log.w(TAG, "skipping ${f.name}: ${file.length()} bytes is over the server limit")
                retire(f.id)
                continue
            }
            val extracted = f.extractedText ?: if (f.isImage || f.name.endsWith(".pdf", ignoreCase = true)) {
                app.reyna.ocr.OnDeviceExtractor.extract(context, file, f.isImage)?.also {
                    dao.updateExtractedText(f.id, it)
                }
            } else null

            val result = api.upload(
                file = file,
                fileName = f.name,
                mimeType = mimeOf(f.name, f.isImage),
                chatName = f.chatName,
                senderName = if (f.confidence >= Attribution.MIN_NAMED && !f.senderName.isNullOrBlank()) f.senderName else null,
                postedAtSeconds = f.postedAt / 1000,
                confidence = f.confidence,
                method = f.method,
                extractedText = extracted,
            )
            result.onSuccess {
                dao.markUploaded(f.id, it.remoteId, it.folder)
                sent++
            }
            val failure = result.exceptionOrNull() ?: continue
            if (failure is ReynaApi.Rejected) {
                // The server looked at this file and said no. Retire it and
                // carry on, so it stops blocking everything behind it.
                Log.w(TAG, "server refused ${f.name}: ${failure.message}")
                retire(f.id)
                continue
            }
            // Could not reach the server at all. Nothing else will get through
            // either, so stop here and let the next pass try again.
            Log.w(TAG, "upload stopped at ${f.name}: ${failure.message}")
            break
        }
        sent
    }

    /**
     * Stops trying to send a file, without pretending it reached the server.
     *
     * Marks with remoteId -1 so it is retired from future queues without
     * colliding with un-uploaded files (remoteId = 0).
     */
    private suspend fun retire(id: Long) = dao.markUploaded(id, -1, null)

    /** Resets files that were retired before image uploading was supported. */
    suspend fun resetRetiredFiles(): Int = withContext(Dispatchers.IO) {
        dao.resetRetiredFiles()
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
        /** Files the user picked from a previous "which one did you mean". */
        fileIds: List<Long> = emptyList(),
        /**
         * Whether to write the question into the conversation.
         *
         * False when the question is already there: answering a choice, and
         * asking again after discarding a reply, are both second attempts at a
         * turn the user only took once.
         */
        recordQuestion: Boolean = true,
        /** Called with what Reyna is doing, as it changes. */
        onStage: (String) -> Unit = {},
        onCall: (okhttp3.Call) -> Unit = {},
    ): ReynaApi.Answer? = withContext(Dispatchers.IO) {
        // Collect recent conversation turns before inserting current query
        val recent = dao.recentMessages(6).reversed()
            .filter { it.text.isNotBlank() }
            // The question being asked is not context for itself.
            //
            // It is already in the conversation when a choice is answered or a
            // reply is discarded and asked again, so without this the server
            // sees a history ending in the identical question and resolves the
            // new one as a follow-up to it.
            .dropLastWhile { !recordQuestion && it.fromUser && it.text == question }
        val history = recent.map { msg ->
            ReynaApi.ChatContext(
                role = if (msg.fromUser) "user" else "assistant",
                text = msg.text,
                fileNames = decodeCitations(msg.citations).map { it.fileName },
            )
        }

        if (recordQuestion) {
            dao.insertMessage(MessageEntity(text = question, fromUser = true, at = System.currentTimeMillis()))
        }

        // Reconcile quickly so newly arrived WhatsApp files are indexed before matching
        runCatching { reconcile() }

        // Send the few files this question is about, and nothing else.
        //
        // This used to run syncPending() as well, which walks up to four
        // thousand queued files and uploads every one of them before the
        // question was even sent. That is what "Looking through your files"
        // was actually doing for minutes at a time, and why the count of
        // documents waiting for Drive visibly climbed while the user watched a
        // spinner: they were watching an upload, not a search. The backlog is
        // real work and still gets done, but it is not the user's question and
        // must not be in front of it.
        onStage("Sending the files you asked about")
        runCatching { syncMatchingFirst(question) }

        val answer = api().ask(question, history, fileIds, onStage, onCall).getOrNull()
        var reply = answer?.reply
            ?: "I could not reach the server, so nothing has been searched. Your files are safe on this phone. Try again in a moment."

        // Match the cited filenames back to local rows so the answer can show
        // real chips. Matched by name because the backend numbers files by its
        // own ids, which the phone does not share.
        val local = dao.allFiles()
        val attachedFiles = answer?.files.orEmpty().takeIf { answer?.intent == "fetch" }.orEmpty()
        var citedIds = attachedFiles.mapNotNull { cited ->
            local.firstOrNull { it.name.equals(cited.name, ignoreCase = true) }?.id
        }
        var citations = answer?.citations.orEmpty()

        // When the server cannot be reached, say that, and nothing more.
        //
        // Two earlier versions of this went wrong the same way. The first
        // matched the question against local filenames and wrote "I found A,
        // B, C on your phone" as though it were an answer. The second kept the
        // list but labelled it honestly, which was still a list of filenames
        // stapled to an error, still ordered by a score the user cannot see,
        // and still able to print the same name twice when the phone holds two
        // rows for one document. Neither told anyone anything they could act
        // on. An unreachable server is one fact and gets one sentence.

        // A false "I have never seen that" is the worst answer this app can
        // give, because it is indistinguishable from the file being lost. If
        // the server found nothing and the phone is still holding files it has
        // not sent, say so instead of letting the user conclude it is gone.
        val askingWhich = answer?.status == ReynaApi.STATUS_NEEDS_CHOICE
        // The backlog note exists to stop a false "I have never seen that",
        // which can only be false if something was actually being looked for.
        // Stapled to "hi" it turned a greeting into a report about eight
        // hundred unsent files.
        if (citations.isEmpty() && citedIds.isEmpty() && answer != null &&
            !askingWhich && !SmallTalk.matches(question)
        ) {
            val queued = dao.pendingUpload(4000).count { isReadable(it.name) }
            if (queued > 0) {
                reply += if (queued == 1) {
                    " One file on this phone has not been sent for reading yet, so it was not searched."
                } else {
                    " $queued files on this phone have not been sent for reading yet, so they were not searched."
                }
            }
        }

        dao.insertMessage(
            MessageEntity(
                text = reply,
                fromUser = false,
                at = System.currentTimeMillis(),
                fileIds = citedIds.joinToString(","),
                citations = encodeEvidence(citations, attachedFiles),
                // A request that never landed is Reyna's own state, not an
                // answer, so it gets the same treatment as running out of
                // allowance rather than sitting in the conversation looking
                // like a reply that went wrong.
                notice = answer?.notice ?: ReynaApi.NOTICE_UNREACHABLE,
            )
        )
        answer
    }

    /**
     * The words in a question worth matching a filename against.
     *
     * The stop list is deliberately about *asking*, not about English. "the"
     * and "is" are dropped because they are everywhere; "find", "show" and
     * "received" are dropped because they describe the request rather than the
     * thing requested, and left in they match half the library.
     */
    private fun queryTokens(question: String): List<String> {
        val stopWords = setOf(
            "find", "any", "the", "for", "with", "from", "that", "this", "file", "files",
            "pdf", "pdfs", "doc", "docs", "notes", "note", "can", "you", "me", "show",
            "tell", "what", "where", "which", "is", "are", "have", "please", "received", "get",
            "did", "share", "shared", "send", "sent", "about", "who", "when", "how", "give",
        )
        return question.lowercase()
            .split(Regex("[^a-zA-Z0-9]+"))
            .filter { it.isNotBlank() && it !in stopWords && (it.length > 1 || it[0].isDigit()) }
    }

    /**
     * How well a filename answers a question.
     *
     * A number is worth more than a word. Filenames in a library like this one
     * repeat their vocabulary constantly and differ only in the index, so
     * "module" separates almost nothing and "4" separates almost everything.
     */
    private fun score(name: String, tokens: List<String>): Int = Words.score(name, tokens)

    private fun decodeCitations(json: String): List<ReynaApi.Citation> {
        if (json.isBlank()) return emptyList()
        return runCatching {
            val arr = org.json.JSONArray(json)
            (0 until arr.length()).mapNotNull { i ->
                val o = arr.optJSONObject(i) ?: return@mapNotNull null
                val quote = o.optString("quote")
                if (quote.isBlank()) return@mapNotNull null
                ReynaApi.Citation(
                    fileId = o.optLong("file_id", 0),
                    fileName = o.optString("file_name"),
                    sender = o.optString("sender").ifBlank { null },
                    sharedAt = o.optString("shared_at").ifBlank { null },
                    folder = o.optString("folder").ifBlank { null },
                    quote = quote,
                    context = o.optString("context").ifBlank { quote },
                    confidence = o.optDouble("confidence", 0.0),
                    page = o.optInt("page", 1).coerceAtLeast(1),
                )
            }
        }.getOrDefault(emptyList())
    }

    /**
     * Citations, stored as JSON on the message.
     *
     * Hand rolled rather than pulling in a serialisation library for one type.
     * The app has no JSON dependency and this is not a reason to acquire one.
     */
    private fun encodeEvidence(
        cs: List<ReynaApi.Citation>,
        files: List<ReynaApi.CitedFile> = emptyList(),
    ): String {
        if (cs.isEmpty() && files.isEmpty()) return ""
        val arr = org.json.JSONArray()
        for (c in cs) {
            arr.put(
                org.json.JSONObject()
                    .put("file_id", c.fileId)
                    .put("file_name", c.fileName)
                    .put("sender", c.sender ?: "")
                    .put("shared_at", c.sharedAt ?: "")
                    .put("folder", c.folder ?: "")
                    .put("quote", c.quote)
                    .put("context", c.context)
                    .put("confidence", c.confidence)
                    .put("page", c.page)
            )
        }
        for (file in files) {
            arr.put(
                org.json.JSONObject()
                    .put("kind", "file")
                    .put("file_id", file.id)
                    .put("file_name", file.name)
                    .put("folder", file.folder ?: "")
                    .put("sender", file.sender ?: "")
                    .put("shared_at", file.sharedAt ?: "")
                    .put("confidence", file.confidence)
            )
        }
        return arr.toString()
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
    /**
     * The question an answer was given to, and the point to rewind to.
     *
     * Returns null when there is nothing to re-ask, which is the case for the
     * very first turn and for anything Reyna said unprompted.
     */
    suspend fun questionBehind(answerAt: Long): String? = withContext(Dispatchers.IO) {
        dao.lastQuestionBefore(answerAt)?.text?.takeIf { it.isNotBlank() }
    }

    /**
     * Removes an answer and everything after it.
     *
     * The question stays. Asking again means answering the same question a
     * second time, not asking a new one, and leaving the old reply behind
     * would put a turn the user rejected into the history the next request
     * carries.
     */
    suspend fun forgetFrom(at: Long) = withContext(Dispatchers.IO) {
        dao.deleteMessagesFrom(at)
    }

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


    suspend fun deleteEverything() = withContext(Dispatchers.IO) {
        dao.clearLinks(); dao.clearEvents(); dao.clearFiles(); dao.clearMessages()
    }

    /**
     * Puts the install back to how it arrived.
     *
     * Everything deleteEverything clears, plus the onboarding flag, so the app
     * opens on the welcome screen again. Deliberately keeps the server address
     * and the device token: those are how this phone reaches its backend, and
     * losing them turns a reset into a setup session.
     */
    suspend fun resetToFirstRun() = withContext(Dispatchers.IO) {
        deleteEverything()
        onboarded = false
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


    /** What the server says has reached Drive. Null when it cannot be asked. */
    suspend fun driveState(): ReynaApi.DriveState? = withContext(Dispatchers.IO) {
        if (deviceToken.isBlank()) return@withContext null
        api().driveState().getOrNull()
    }

    /**
     * What came back when asking for a Drive consent URL.
     *
     * Three outcomes, not two. Collapsing them into a null meant an unreachable
     * backend was reported as "Drive is not configured on the server", which
     * sends the user to check a server setting when the real problem is that
     * their phone cannot see the server at all.
     */
    sealed interface DriveConnect {
        data class Url(val value: String) : DriveConnect
        data object NotConfigured : DriveConnect
        data object Unreachable : DriveConnect
    }

    suspend fun driveConnect(): DriveConnect = withContext(Dispatchers.IO) {
        if (deviceToken.isBlank()) return@withContext DriveConnect.Unreachable
        api().driveConnectUrl().fold(
            onSuccess = { url ->
                if (url.isNullOrBlank()) DriveConnect.NotConfigured else DriveConnect.Url(url)
            },
            onFailure = { DriveConnect.Unreachable },
        )
    }

    /** Forgets the Drive connection server-side. Returns false if unreachable. */
    suspend fun driveDisconnect(): Boolean = withContext(Dispatchers.IO) {
        if (deviceToken.isBlank()) return@withContext false
        api().driveDisconnect().isSuccess
    }

    /** Files everything staged into Drive now. Returns how many moved. */
    suspend fun drivePush(): Int? = withContext(Dispatchers.IO) {
        if (deviceToken.isBlank()) return@withContext null
        api().drivePush().getOrNull()
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

        /**
         * The server refuses a body over this, in handleDeviceUpload.
         * Kept in step by hand: the two are far apart and there is no
         * shared place to put it.
         */
        private const val MAX_UPLOAD_BYTES = 50L * 1024 * 1024
        private const val KEY_BACKEND = "backend_url"
        private const val KEY_TOKEN = "device_token"
        private const val KEY_CAPTURING = "capturing"
        private const val KEY_DIGEST = "daily_digest"
        private const val KEY_ONBOARDED = "onboarded"

        /**
         * Where to look for the backend before anyone has said otherwise.
         *
         * Baked at build time from android/local.properties, so a build made
         * for a particular machine arrives already pointed at it. Falls back to
         * the emulator's alias for the host when nothing was configured.
         */
        private val DEFAULT_BACKEND: String = app.reyna.BuildConfig.BACKEND_URL

        private val IMAGE_EXT = setOf("jpg", "jpeg", "png", "webp")

        /**
         * The file types Reyna can actually read.
         *
         * Anything that yields text without OCR. Photographs, video and audio
         * are deliberately absent: they cost a model call each, return nothing
         * a question can be answered from, and there are thousands of them on
         * a normal phone. Sending them was the reason a five thousand file
         * library never finished uploading and the one PDF somebody asked
         * about sat behind eight hundred holiday snaps.
         */
        private val READABLE_EXT = setOf(
            "pdf",
            "doc", "docx", "odt", "rtf",
            "ppt", "pptx", "odp",
            "xls", "xlsx", "ods", "csv",
            "txt", "md", "log", "json", "xml", "html", "htm", "epub",
        ) + IMAGE_EXT

        fun isReadable(name: String): Boolean =
            name.substringAfterLast('.', "").lowercase() in READABLE_EXT

        @Volatile private var instance: Repo? = null

        fun get(context: Context): Repo = instance ?: synchronized(this) {
            instance ?: Repo(context.applicationContext).also { instance = it }
        }
    }
}
