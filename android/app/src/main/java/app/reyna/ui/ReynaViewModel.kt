package app.reyna.ui

import android.app.Application
import android.content.Intent
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import app.reyna.attribution.Attribution
import app.reyna.capture.CaptureService
import app.reyna.capture.ReconcileWorker
import app.reyna.data.FileEntity
import app.reyna.data.Repo
import app.reyna.permissions.Permissions
import app.reyna.search.SearchableFile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

/**
 * All app state, in one place.
 *
 * Every button in the UI resolves to a method here, and every method does
 * something real: there are no handlers that only log. Where an action cannot
 * complete (no backend configured, permission missing), it says so in the
 * conversation rather than failing silently.
 */
class ReynaViewModel(app: Application) : AndroidViewModel(app) {

    private val repo = Repo.get(app)

    private val _files = MutableStateFlow<List<FileEntity>>(emptyList())
    val files: StateFlow<List<FileEntity>> = _files.asStateFlow()

    private val _messages = MutableStateFlow<List<ChatMessage>>(emptyList())
    val messages: StateFlow<List<ChatMessage>> = _messages.asStateFlow()

    private val _permissions = MutableStateFlow(Permissions.all(app))
    val permissions: StateFlow<List<Permissions.State>> = _permissions.asStateFlow()

    private val _scanning = MutableStateFlow(false)
    val scanning: StateFlow<Boolean> = _scanning.asStateFlow()

    private val _onboardingStep = MutableStateFlow(OnboardingStep.Welcome)
    val onboardingStep: StateFlow<OnboardingStep> = _onboardingStep.asStateFlow()

    private val _needsOnboarding = MutableStateFlow(!repo.onboarded)
    val needsOnboarding: StateFlow<Boolean> = _needsOnboarding.asStateFlow()

    private val _toast = MutableStateFlow<String?>(null)
    val toast: StateFlow<String?> = _toast.asStateFlow()

    /** True while an answer is in flight, so the composer can offer Stop. */
    private val _sending = MutableStateFlow(false)
    val sending: StateFlow<Boolean> = _sending.asStateFlow()

    // The in-flight request, held so it can actually be torn down. Cancelling
    // the coroutine alone would leave the socket open and the server working.
    private var askJob: kotlinx.coroutines.Job? = null
    private var askCall: okhttp3.Call? = null

    val capturing: Boolean get() = repo.capturing
    val dailyDigest: Boolean get() = repo.dailyDigest
    var backendUrl: String
        get() = repo.backendUrl
        set(v) { repo.backendUrl = v }
    var deviceToken: String
        get() = repo.deviceToken
        set(v) { repo.deviceToken = v }

    init {
        viewModelScope.launch {
            repo.observeFiles().collect { _files.value = it }
        }
        viewModelScope.launch {
            // Combined rather than collected alone. Chips are rebuilt from the
            // file rows, so a message stream that only re-emits when messages
            // change would render every chip empty on a cold start, when the
            // conversation loads before the library does, and leave them empty
            // until the next message arrived.
            kotlinx.coroutines.flow.combine(
                repo.observeMessages(),
                repo.observeFiles(),
            ) { rows, files -> rows to files }.collect { (rows, files) ->
                _messages.value = rows.map { m ->
                    ChatMessage(
                        text = plainText(m.text),
                        fromUser = m.fromUser,
                        time = timeOf(m.at),
                        // Chips are rebuilt from the local rows rather than
                        // stored with the message, so an answer always shows
                        // the current attribution rather than what was true
                        // when it was written.
                        files = m.fileIds
                            .split(",")
                            .mapNotNull { it.trim().toLongOrNull() }
                            .mapNotNull { id -> files.firstOrNull { it.id == id } }
                            .map { f ->
                                FoundFile(
                                    id = f.id,
                                    fileName = f.name,
                                    senderName = f.senderName,
                                    chatName = f.chatName,
                                    whenText = relativeTime(f.postedAt),
                                    confidence = f.confidence,
                                    isImage = f.isImage,
                                )
                            },
                    )
                }
            }
        }
        viewModelScope.launch {
            // An empty conversation is a dead end, so Reyna opens by saying
            // what it has and inviting a question.
            if (repo.onboarded) repo.seedGreeting()
        }
    }

    // ── Lifecycle ──

    /**
     * Re-reads permission state.
     *
     * Every permission is granted on a Settings screen outside the app, so the
     * only reliable moment to check is when we come back.
     */
    fun refreshPermissions() {
        _permissions.value = Permissions.all(getApplication())
    }

    /** Starts capture if it is allowed to run. Idempotent. */
    fun startCaptureIfPossible() {
        val app = getApplication<Application>()
        if (!repo.capturing || !repo.onboarded) return
        if (!Permissions.hasStorage(app)) return
        CaptureService.start(app)
        ReconcileWorker.schedule(app)
    }

    // ── Onboarding ──

    fun onboardingNext() {
        val next = OnboardingStep.entries.getOrNull(_onboardingStep.value.ordinal + 1)
        if (next == null) {
            finishOnboarding()
            return
        }
        _onboardingStep.value = next
        // Scanning starts as soon as storage is granted, so the results screen
        // has something to show by the time the user reaches it.
        if (next == OnboardingStep.FirstScan) runFirstScan()
    }

    fun onboardingBack() {
        val prev = OnboardingStep.entries.getOrNull(_onboardingStep.value.ordinal - 1) ?: return
        _onboardingStep.value = prev
    }

    fun onboardingSkip() = onboardingNext()

    private fun finishOnboarding() {
        repo.onboarded = true
        _needsOnboarding.value = false
        startCaptureIfPossible()
        viewModelScope.launch { repo.seedGreeting() }
    }

    private fun runFirstScan() {
        viewModelScope.launch {
            _scanning.value = true
            runCatching { repo.reconcile() }
            _scanning.value = false
        }
    }

    // ── Chat ──

    fun ask(question: String) {
        // One question at a time. A second send while the first is in flight
        // would interleave two answers into the same conversation.
        if (_sending.value) return

        askJob = viewModelScope.launch {
            _sending.value = true
            try {
                if (repo.deviceToken.isBlank()) {
                    // Still records the turn and answers honestly, rather than
                    // dropping the question on the floor.
                    repo.ask(question)
                    _toast.value = "Set a device token in Settings to reach the backend"
                    return@launch
                }
                repo.ask(question) { call -> askCall = call }
            } finally {
                askCall = null
                _sending.value = false
            }
        }
    }

    /**
     * Stops an answer in progress.
     *
     * Cancels the HTTP call as well as the coroutine, so the socket is torn
     * down rather than left running while its result is quietly discarded. The
     * user's question stays in the conversation, because they did ask it, and
     * Reyna says plainly that it stopped rather than leaving a turn dangling.
     */
    fun stopAnswering() {
        if (!_sending.value) return
        askCall?.cancel()
        askJob?.cancel()
        askCall = null
        _sending.value = false
        viewModelScope.launch { repo.say("Stopped.") }
    }

    /** Wipes the conversation. Files and attribution are untouched. */
    fun clearChat() {
        viewModelScope.launch {
            stopAnswering()
            repo.clearConversation()
            repo.seedGreeting()
        }
    }

    /**
     * Takes a file the user handed to Reyna directly.
     *
     * Copied into Reyna's own storage first: the picker gives a content URI
     * that is only readable for as long as the grant lasts, so keeping the URI
     * would mean a file that opens today and fails next week.
     */
    /**
     * Hands Reyna a file directly.
     *
     * The outcome is written into the conversation, not just flashed as a
     * toast. This is a chat: the user's mental model is that what they hand
     * over shows up in the thread, and the receipt carries a chip so the file
     * is openable from the spot where they added it.
     */
    fun addFile(uri: android.net.Uri) {
        viewModelScope.launch {
            when (val result = repo.importFile(uri)) {
                is Repo.Imported.Added ->
                    repo.say("Got ${result.name}. Filed under you.", listOf(result.fileId))

                is Repo.Imported.Duplicate ->
                    repo.say("I already have ${result.name}.", listOf(result.fileId))

                Repo.Imported.Unreadable ->
                    repo.say("I could not read that file. Nothing was added.")
            }
        }
    }

    // ── Files ──

    /**
     * The library, as search sees it.
     *
     * Carries confidence rather than a rendered sender line, so a file
     * attributed too weakly to name is still searchable by chat and filename
     * but never presented with a sender.
     */
    fun searchableFiles(): List<SearchableFile> = _files.value.map { f ->
        SearchableFile(
            id = f.id,
            fileName = f.name,
            senderName = f.senderName,
            chatName = f.chatName,
            whenText = relativeTime(f.postedAt),
            confidence = f.confidence,
            isImage = f.isImage,
        )
    }

    /** Opens a captured file in whatever app can handle it. */
    fun openFile(id: Long) {
        viewModelScope.launch {
            val f = _files.value.firstOrNull { it.id == id } ?: return@launch
            val file = java.io.File(f.path)
            if (!file.isFile) {
                _toast.value = "That file is no longer on this phone"
                return@launch
            }
            val app = getApplication<Application>()

            // getUriForFile throws when the file sits outside every root
            // declared in file_paths.xml, and it throws rather than returning
            // null, so leaving it outside the guard turned a file Reyna could
            // not serve into a crash. Opening a file must never take the app
            // down; the worst case is telling the user it cannot be opened.
            runCatching {
                val uri = androidx.core.content.FileProvider.getUriForFile(
                    app, "${app.packageName}.files", file,
                )
                val intent = Intent(Intent.ACTION_VIEW).apply {
                    setDataAndType(uri, mimeOf(f.name, f.isImage))
                    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
                }
                app.startActivity(intent)
            }.onFailure { _toast.value = "No app on this phone can open that file" }
        }
    }

    /**
     * The type to hand another app when opening a file.
     *
     * Derived from the extension rather than assumed. Everything that was not
     * an image used to be opened as application/pdf, so a .docx or .pptx, which
     * is most of what circulates in a class group, either failed to open or
     * opened in the wrong reader.
     */
    private fun mimeOf(name: String, isImage: Boolean): String {
        if (isImage) return "image/*"
        val ext = name.substringAfterLast('.', "").lowercase()
        return android.webkit.MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext)
            ?: "application/octet-stream"
    }

    /** The repair path behind every "who shared this?" affordance. */
    fun setSender(fileId: Long, sender: String) {
        viewModelScope.launch {
            repo.setSenderManually(fileId, sender)
            _toast.value = "Thanks. Reyna will remember."
        }
    }

    /** Names Reyna already knows, to offer as one-tap answers. */
    suspend fun knownSenders(): List<String> = withContext(Dispatchers.Default) {
        _files.value.mapNotNull { it.senderName }
            .filter { it.isNotBlank() && it != "You" }
            .distinct()
            .sorted()
    }

    // ── Import ──

    /** Parses an export on-device and keeps only the attachment rows. */
    fun importExport(uri: android.net.Uri, onDone: (Repo.ImportResult?) -> Unit) {
        viewModelScope.launch {
            val app = getApplication<Application>()
            val result = runCatching {
                app.contentResolver.openInputStream(uri)!!.reader().use { repo.importExport(it) }
            }.getOrNull()
            if (result == null) _toast.value = "Could not read that export"
            onDone(result)
        }
    }

    // ── Tracking ──

    fun trackingState(): TrackingState {
        val all = _files.value
        val named = all.count { it.confidence >= Attribution.MIN_NAMED }
        val chatOnly = all.count { it.confidence in 0.30..<Attribution.MIN_NAMED }
        val unknown = all.count { it.confidence < 0.30 }
        val byChat = all.filter { !it.chatName.isNullOrBlank() }.groupBy { it.chatName!! }

        return TrackingState(
            total = all.size,
            named = named,
            chatOnly = chatOnly,
            unknown = unknown,
            chats = byChat.map { (name, files) ->
                WatchedChat(name, files.size, relativeTime(files.maxOf { it.postedAt }))
            }.sortedByDescending { it.fileCount },
            permissions = _permissions.value.map {
                PermissionState(it.title, it.granted, it.consequence)
            },
            onDeviceLabel = formatBytes(all.sumOf { it.sizeBytes }),
            inDriveLabel = formatBytes(all.filter { it.uploaded }.sumOf { it.sizeBytes }),
        )
    }

    val watchedChatCount: Int
        get() = _files.value.mapNotNull { it.chatName }.filter { it.isNotBlank() }.distinct().size

    // ── Settings ──

    fun setCapturing(on: Boolean) {
        repo.capturing = on
        val app = getApplication<Application>()
        if (on) startCaptureIfPossible() else {
            CaptureService.stop(app)
            ReconcileWorker.cancel(app)
        }
        _toast.value = if (on) "Watching for new files" else "Capture paused"
    }

    fun setDailyDigest(on: Boolean) {
        repo.dailyDigest = on
    }

    fun rescanNow() {
        viewModelScope.launch {
            _scanning.value = true
            val added = runCatching { repo.reconcile() }.getOrDefault(0)
            repo.reattributeWeak()
            _scanning.value = false
            _toast.value = if (added == 0) "No new files" else "Found $added new"
        }
    }

    fun syncNow() {
        viewModelScope.launch {
            if (repo.deviceToken.isBlank()) {
                _toast.value = "Set a device token first"
                return@launch
            }
            val sent = repo.syncPending(100)
            _toast.value = if (sent == 0) "Nothing to send" else "Sent $sent to the backend"
        }
    }

    fun deleteEverything() {
        viewModelScope.launch {
            repo.deleteEverything()
            _toast.value = "Deleted. Your files and Drive are untouched."
        }
    }

    /**
     * Opens Google's consent screen in a browser.
     *
     * Not inert and not a stub: the server mints the URL because it holds the
     * client secret, and the callback lands back on the server which stores the
     * tokens. If Drive is not configured server-side, say so rather than
     * opening a page that will fail.
     */
    fun connectDrive() {
        viewModelScope.launch {
            if (repo.deviceToken.isBlank()) {
                _toast.value = "Set a device token first"
                return@launch
            }
            val url = repo.driveConnectUrl()
            if (url == null) {
                _toast.value = "Drive is not configured on the server"
                return@launch
            }
            val app = getApplication<Application>()
            val intent = Intent(Intent.ACTION_VIEW, android.net.Uri.parse(url))
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            runCatching { app.startActivity(intent) }
                .onFailure { _toast.value = "No browser on this phone" }
        }
    }

    fun clearToast() { _toast.value = null }

    // ── Formatting ──

    private fun timeOf(millis: Long): String {
        val cal = java.util.Calendar.getInstance().apply { timeInMillis = millis }
        return "%d:%02d".format(
            cal.get(java.util.Calendar.HOUR_OF_DAY),
            cal.get(java.util.Calendar.MINUTE),
        )
    }

    /**
     * Strips the markdown the backend writes into its replies.
     *
     * The bubble renders plain text, so **bold** arrived on screen as literal
     * asterisks around every filename. Stripping here rather than changing the
     * backend keeps the same reply usable by the web dashboard, which does
     * render markdown.
     */
    private fun plainText(s: String): String = s.lines().joinToString("\n") { line ->
        line
            .replace("**", "")
            .replace("\u2014", "\u00b7")
            // Markdown bullets, which the model writes as "*   " or "- ",
            // become a real bullet rather than a stray asterisk.
            .replaceFirst(Regex("^\\s*[*-]\\s+"), "\u2022 ")
            .trimEnd()
    }.trim()

    /** How long ago something was shared, phrased the way every screen phrases it. */
    fun relativeTime(millis: Long): String {
        val delta = System.currentTimeMillis() - millis
        val days = TimeUnit.MILLISECONDS.toDays(delta)
        val hours = TimeUnit.MILLISECONDS.toHours(delta)
        val mins = TimeUnit.MILLISECONDS.toMinutes(delta)
        return when {
            mins < 1 -> "just now"
            mins < 60 -> "$mins min ago"
            hours < 24 -> "$hours hours ago"
            days == 1L -> "yesterday"
            days < 30 -> "$days days ago"
            else -> {
                val cal = java.util.Calendar.getInstance().apply { timeInMillis = millis }
                "%d %s".format(
                    cal.get(java.util.Calendar.DAY_OF_MONTH),
                    java.text.DateFormatSymbols().shortMonths[cal.get(java.util.Calendar.MONTH)],
                )
            }
        }
    }

    private fun formatBytes(bytes: Long): String = when {
        bytes <= 0 -> "nothing yet"
        bytes < 1024L * 1024 -> "${bytes / 1024} KB"
        bytes < 1024L * 1024 * 1024 -> "%.0f MB".format(bytes / 1024.0 / 1024)
        else -> "%.1f GB".format(bytes / 1024.0 / 1024 / 1024)
    }
}
