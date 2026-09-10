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
import app.reyna.net.ReynaApi
import app.reyna.permissions.Permissions
import app.reyna.search.SearchableFile
import app.reyna.search.FileSearch
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

/**
 * Where the Drive connection has got to.
 *
 * Modelled as a state rather than a boolean because the outcome has to stay on
 * the screen. Onboarding is exactly where a snackbar is missed: the user is
 * coming back from a browser, looking at the button they pressed, and a button
 * that has quietly gone back to saying "Connect Drive" is indistinguishable
 * from one that never did anything. It has to say what happened and let them
 * try again.
 */
sealed interface DriveConnectState {
    data object Idle : DriveConnectState
    data object Connecting : DriveConnectState
    data class Connected(val email: String) : DriveConnectState
    data class Failed(val reason: String) : DriveConnectState
}

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

    /**
     * What is waiting to reach Drive.
     *
     * Polled rather than inferred. The phone finishing an upload says nothing
     * about whether the file was filed, and an app that quietly implies your
     * documents are backed up when they are not is worse than one that says
     * nothing at all.
     */
    private val _driveState = MutableStateFlow<app.reyna.net.ReynaApi.DriveState?>(null)
    val driveState: StateFlow<app.reyna.net.ReynaApi.DriveState?> = _driveState.asStateFlow()

    private val _pushing = MutableStateFlow(false)
    val pushing: StateFlow<Boolean> = _pushing.asStateFlow()

    /**
     * True from tapping Connect until the browser has come back and the server
     * has been asked whether it worked.
     *
     * Connecting Drive leaves the app entirely, which is exactly when a user
     * needs to be told something is in progress. Without this the row sat
     * unchanged through the whole round trip and looked like the tap missed.
     */
    private val _driveConnect = MutableStateFlow<DriveConnectState>(DriveConnectState.Idle)
    val driveConnect: StateFlow<DriveConnectState> = _driveConnect.asStateFlow()

    /** The one bit of the above that the settings sheet cares about. */
    val connectingDrive: StateFlow<Boolean> = _driveConnect
        .map { it is DriveConnectState.Connecting }
        .stateIn(viewModelScope, SharingStarted.Eagerly, false)

    /** True while an answer is in flight, so the composer can offer Stop. */
    private val _sending = MutableStateFlow(false)
    val sending: StateFlow<Boolean> = _sending.asStateFlow()

    /**
     * What Reyna is doing right now, in words, while it does it.
     *
     * The bubble used to read "Looking through your files" for the whole wait,
     * which could be most of a minute and said nothing about whether anything
     * was happening. Most of that time is one specific document being read,
     * and saying which one turns a hang into progress.
     */
    private val _stage = MutableStateFlow("")
    val stage: StateFlow<String> = _stage.asStateFlow()

    /**
     * The documents Reyna is asking the user to choose between, if any.
     *
     * Held in memory rather than on the message, because adding a column to
     * the message table would trigger Room's destructive migration and wipe
     * the phone's index of every captured file. A choice that is lost when the
     * app is killed simply means asking again; the index is not replaceable.
     */
    private val _pendingChoice = MutableStateFlow<PendingChoice?>(null)
    val pendingChoice: StateFlow<PendingChoice?> = _pendingChoice.asStateFlow()

    /** A question waiting on the user to say which document they meant. */
    data class PendingChoice(
        val question: String,
        val prompt: String,
        val candidates: List<ReynaApi.Candidate>,
    )

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
                        sources = decodeSources(m.citations),
                        text = plainText(m.text),
                        fromUser = m.fromUser,
                        time = timeOf(m.at),
                        at = m.at,
                        notice = m.notice,
                        // Chips are rebuilt from the local rows rather than
                        // stored with the message, so an answer always shows
                        // the current attribution rather than what was true
                        // when it was written.
                        files = (m.fileIds
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
                                    remoteId = f.remoteId,
                                )
                            } + decodeFetchedFiles(m.citations, files))
                            .distinctBy { it.fileName.lowercase() },
                    )
                }
            }
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

    /** Re-reads Drive state. Cheap, and called whenever the user returns. */
    fun refreshDriveState() {
        viewModelScope.launch { _driveState.value = repo.driveState() }
    }

    /** Files everything waiting into Drive, then re-reads the state. */
    fun pushToDrive() {
        if (_pushing.value) return
        viewModelScope.launch {
            _pushing.value = true
            val moved = repo.drivePush()
            _pushing.value = false
            _toast.value = when {
                moved == null -> "Could not reach the backend"
                moved == 0 -> "Nothing waiting to file"
                moved == 1 -> "Filed 1 document into your Drive"
                else -> "Filed $moved documents into your Drive"
            }
            _driveState.value = repo.driveState()
        }
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
        if (next == OnboardingStep.Drive) refreshDriveConnect()
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
    }

    /** Files seen so far by the first scan, so the screen can show it climbing. */
    private val _scanProgress = MutableStateFlow(0)
    val scanProgress: StateFlow<Int> = _scanProgress.asStateFlow()

    private fun runFirstScan() {
        viewModelScope.launch {
            _scanning.value = true
            _scanProgress.value = 0
            // Failures are swallowed on purpose. A scan that cannot read the
            // folder is a permission problem the user has already been shown,
            // and hanging the onboarding on it would trap them on this screen.
            runCatching { repo.reconcile { _scanProgress.value = it } }
            _scanning.value = false
        }
    }

    // ── Chat ──

    fun ask(question: String) {
        // One question at a time. A second send while the first is in flight
        // would interleave two answers into the same conversation.
        if (_sending.value) return
        _pendingChoice.value = null
        run(question, emptyList(), recordQuestion = true)
    }

    /**
     * Answers the outstanding question against the document the user picked.
     *
     * The question is not written into the conversation again: it was already
     * asked, and the choice is part of answering it rather than a new turn.
     */
    fun chooseCandidate(fileIds: List<Long>) {
        val pending = _pendingChoice.value ?: return
        if (_sending.value || fileIds.isEmpty()) return
        _pendingChoice.value = null
        run(pending.question, fileIds, recordQuestion = false)
    }

    /**
     * Puts an answer on the clipboard.
     *
     * The text as it was read, not the citations. Somebody copying a reply is
     * taking it somewhere else, and a block of quoted source material pasted
     * behind it is never what they meant.
     */
    fun copyAnswer(text: String) {
        if (text.isBlank()) return
        val clip = getApplication<Application>()
            .getSystemService(android.content.Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
        clip.setPrimaryClip(android.content.ClipData.newPlainText("Reyna", text))
        // Android 13 and up shows its own copy confirmation, so saying it
        // again would put two notices on screen for one action.
        if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.TIRAMISU) {
            _toast.value = "Copied"
        }
    }

    /**
     * Answers the same question again, discarding the reply that was given.
     *
     * The old answer is deleted rather than left above the new one. A reply
     * the user rejected is not part of the conversation, and leaving it in
     * would feed it back as context to the very request meant to replace it.
     */
    fun retry(answerAt: Long) {
        if (_sending.value) return
        viewModelScope.launch {
            val question = repo.questionBehind(answerAt) ?: run {
                _toast.value = "Nothing to ask again"
                return@launch
            }
            repo.forgetFrom(answerAt)
            _pendingChoice.value = null
            run(question, emptyList(), recordQuestion = false)
        }
    }

    private fun run(question: String, fileIds: List<Long>, recordQuestion: Boolean) {
        askJob = viewModelScope.launch {
            _sending.value = true
            _stage.value = ""
            try {
                if (repo.deviceToken.isBlank()) {
                    // Still records the turn and answers honestly, rather than
                    // dropping the question on the floor.
                    repo.ask(question, fileIds, recordQuestion)
                    _toast.value = "Set a device token in Settings to reach the backend"
                    return@launch
                }
                val answer = repo.ask(
                    question = question,
                    fileIds = fileIds,
                    recordQuestion = recordQuestion,
                    onStage = { _stage.value = it },
                ) { call -> askCall = call }

                if (answer?.status == ReynaApi.STATUS_NEEDS_CHOICE && answer.candidates.isNotEmpty()) {
                    _pendingChoice.value = PendingChoice(
                        question = question,
                        prompt = answer.reply,
                        candidates = answer.candidates,
                    )
                }
            } finally {
                askCall = null
                _stage.value = ""
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
        _stage.value = ""
        _sending.value = false
        viewModelScope.launch { repo.say("Stopped.") }
    }

    /** Wipes the conversation. Files and attribution are untouched. */
    fun clearChat() {
        viewModelScope.launch {
            stopAnswering()
            repo.clearConversation()
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
            path = f.path,
            sizeBytes = f.sizeBytes,
            mtime = f.mtime,
            postedAt = f.postedAt,
            isSent = f.isSent,
            extractedText = f.extractedText,
            remoteId = f.remoteId,
        )
    }

    /** Content candidates from Room FTS; final weighting stays in FileSearch. */
    suspend fun contentSearchIds(query: String): Set<Long> =
        repo.searchContentIds(FileSearch.contentIndexQuery(query))

    /** Returns local bytes for preview, restoring an uploaded file if needed. */
    suspend fun previewPath(file: SearchableFile): String? = repo.previewFile(
        localPath = file.path,
        remoteId = file.remoteId,
        fileName = file.fileName,
    )?.absolutePath

    /**
     * Finds the local row a citation refers to.
     *
     * A citation carries the backend's id for the file, which is a different
     * number from the one Room assigned, so looking it up directly matched
     * nothing and the Open button silently did nothing at all. Matched on
     * remoteId, which markUploaded records when the backend accepts a file,
     * falling back to the filename for rows uploaded before ids were tracked.
     *
     * Returns null when the file is genuinely not on this phone, which happens
     * when it reached the server from somewhere else.
     */
    fun localFileFor(remoteId: Long, fileName: String): FileEntity? {
        val files = _files.value
        // Priority 1: Match by exact filename (case-insensitive)
        if (fileName.isNotBlank()) {
            val byName = files.firstOrNull { it.name.equals(fileName, ignoreCase = true) }
            if (byName != null) return byName
        }
        // Priority 2: Match by server remoteId if recorded on local row
        if (remoteId > 0L) {
            val byRemote = files.firstOrNull { it.remoteId > 0L && it.remoteId == remoteId }
            if (byRemote != null) return byRemote
        }
        // Priority 3: Fuzzy filename match (strip .v2, extensions, or partial containment)
        if (fileName.isNotBlank()) {
            val cleanName = fileName.replace(Regex("\\.v\\d+\\."), ".")
            val byFuzzy = files.firstOrNull {
                it.name.equals(cleanName, ignoreCase = true) ||
                it.name.contains(cleanName, ignoreCase = true) ||
                cleanName.contains(it.name, ignoreCase = true)
            }
            if (byFuzzy != null) return byFuzzy
        }
        return null
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

    /** Opens a chat attachment, restoring backend-only fetch results into cache first. */
    fun openChatFile(file: FoundFile) {
        viewModelScope.launch {
            val local = when {
                file.id > 0L -> _files.value.firstOrNull { it.id == file.id }
                else -> localFileFor(file.remoteId, file.fileName)
            }
            val cached = repo.previewFile(
                localPath = local?.path.orEmpty(),
                remoteId = local?.remoteId?.takeIf { it > 0L } ?: file.remoteId,
                fileName = file.fileName,
            )
            if (cached == null) {
                _toast.value = "Could not download that file"
                return@launch
            }
            openPath(cached, file.fileName, file.isImage)
        }
    }

    /** Resolves a chat result for press-and-hold Quick Look. */
    suspend fun chatPreviewPath(file: FoundFile): String? {
        val local = when {
            file.id > 0L -> _files.value.firstOrNull { it.id == file.id }
            else -> localFileFor(file.remoteId, file.fileName)
        }
        return repo.previewFile(
            localPath = local?.path.orEmpty(),
            remoteId = local?.remoteId?.takeIf { it > 0L } ?: file.remoteId,
            fileName = file.fileName,
        )?.absolutePath
    }

    private fun openPath(file: java.io.File, name: String, isImage: Boolean) {
        val app = getApplication<Application>()
        runCatching {
            val uri = androidx.core.content.FileProvider.getUriForFile(
                app, "${app.packageName}.files", file,
            )
            app.startActivity(
                Intent(Intent.ACTION_VIEW)
                    .setDataAndType(uri, mimeOf(name, isImage))
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
            )
        }.onFailure { _toast.value = "No app on this phone can open that file" }
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
        if (_driveConnect.value is DriveConnectState.Connecting) return
        viewModelScope.launch {
            if (repo.deviceToken.isBlank()) {
                fail("Set a device token first")
                return@launch
            }
            _driveConnect.value = DriveConnectState.Connecting
            val url = when (val r = repo.driveConnect()) {
                is Repo.DriveConnect.Url -> r.value
                Repo.DriveConnect.NotConfigured -> {
                    fail("Drive is not set up on the server yet")
                    return@launch
                }
                Repo.DriveConnect.Unreachable -> {
                    fail("Cannot reach the backend. Check it is running and on the same network.")
                    return@launch
                }
            }
            val app = getApplication<Application>()
            val intent = Intent(Intent.ACTION_VIEW, android.net.Uri.parse(url))
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            runCatching { app.startActivity(intent) }
                .onFailure { fail("No browser on this phone") }
        }
    }

    /** One place to put a Drive failure, so the screen and the snackbar agree. */
    private fun fail(reason: String) {
        _driveConnect.value = DriveConnectState.Failed(reason)
        _toast.value = reason
    }

    /**
     * Called when the user comes back from the browser.
     *
     * The consent screen finishes on the server, not here, so the only way to
     * learn the outcome is to ask. Reporting it explicitly matters: a silent
     * return from a failed consent is indistinguishable from a successful one.
     */
    fun settleDriveConnect() {
        if (_driveConnect.value !is DriveConnectState.Connecting) return
        viewModelScope.launch {
            val state = repo.driveState()
            _driveState.value = state
            when {
                state == null -> fail("Could not reach the backend")
                state.connected -> {
                    _driveConnect.value = DriveConnectState.Connected(state.email.orEmpty())
                    _toast.value = "Drive connected as ${state.email}"
                }
                // Google sent them back without granting anything. Most often
                // a declined consent, or an account that is not on the test
                // user list while the OAuth app is unverified.
                else -> fail("Drive was not connected. The consent screen was closed or refused.")
            }
        }
    }

    /**
     * Asks the server whether Drive is already connected, without starting a
     * connection.
     *
     * Called when the Drive step opens, so someone who connected earlier and
     * came back through a reset is not asked to do it again.
     */
    fun refreshDriveConnect() {
        if (_driveConnect.value is DriveConnectState.Connecting) return
        viewModelScope.launch {
            val state = repo.driveState() ?: return@launch
            _driveState.value = state
            _driveConnect.value =
                if (state.connected) DriveConnectState.Connected(state.email.orEmpty())
                else DriveConnectState.Idle
        }
    }

    /**
     * Opens a file by the path a viewer already resolved, in another app.
     *
     * Separate from openFile because the PDF viewer holds the row already and
     * should not look it up a second time by an id it does not have.
     */
    fun openInOtherApp(file: FileEntity) {
        val app = getApplication<Application>()
        runCatching {
            val uri = androidx.core.content.FileProvider.getUriForFile(
                app, "${app.packageName}.files", java.io.File(file.path),
            )
            app.startActivity(
                Intent(Intent.ACTION_VIEW)
                    .setDataAndType(uri, mimeOf(file.name, file.isImage))
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
            )
        }.onFailure { _toast.value = "No app on this phone can open that file" }
    }

    /** Says something in the conversation, for outcomes with no other home. */
    fun note(text: String) {
        _toast.value = text
    }

    /** Forgets the Drive connection. Files already in Drive are left alone. */
    fun disconnectDrive() {
        viewModelScope.launch {
            val ok = repo.driveDisconnect()
            _toast.value =
                if (ok) "Disconnected. Files already in your Drive are untouched."
                else "Could not reach the backend"
            _driveState.value = repo.driveState()
            _driveConnect.value = DriveConnectState.Idle
        }
    }

    /**
     * Back to a first run, onboarding and all.
     *
     * Exists because demonstrating the app means showing how it opens, and the
     * only alternative was uninstalling it, which also loses the server address
     * and token and turns a thirty second reset into a setup session.
     */
    fun resetToFirstRun() {
        viewModelScope.launch {
            repo.resetToFirstRun()
            _onboardingStep.value = OnboardingStep.Welcome
            _needsOnboarding.value = true
            _driveState.value = null
            _driveConnect.value = DriveConnectState.Idle
            _toast.value = "Reset. Your files and Drive are untouched."
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
     * Reads back the passages stored with an answer.
     *
     * Unlike the file chips, these are not rebuilt from the current library.
     * A citation records what an answer was based on when it was given, and
     * quietly updating it later would make the evidence worthless.
     */
    private fun decodeSources(json: String): List<Source> {
        if (json.isBlank()) return emptyList()
        return runCatching {
            val arr = org.json.JSONArray(json)
            (0 until arr.length()).mapNotNull { i ->
                val o = arr.optJSONObject(i) ?: return@mapNotNull null
                val quote = o.optString("quote")
                if (quote.isBlank()) return@mapNotNull null
                Source(
                    fileId = o.optLong("file_id", 0),
                    fileName = o.optString("file_name"),
                    senderName = o.optString("sender").ifBlank { null },
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

    /** Fetch attachments share the evidence JSON so old Room rows need no schema rewrite. */
    private fun decodeFetchedFiles(json: String, localFiles: List<FileEntity>): List<FoundFile> {
        if (json.isBlank()) return emptyList()
        return runCatching {
            val arr = org.json.JSONArray(json)
            (0 until arr.length()).mapNotNull { i ->
                val o = arr.optJSONObject(i) ?: return@mapNotNull null
                if (o.optString("kind") != "file") return@mapNotNull null
                val name = o.optString("file_name")
                if (name.isBlank()) return@mapNotNull null
                val remoteId = o.optLong("file_id", 0)
                val local = localFiles.firstOrNull {
                    it.name.equals(name, ignoreCase = true) ||
                        (remoteId > 0 && it.remoteId == remoteId)
                }
                if (local != null) {
                    FoundFile(
                        id = local.id,
                        fileName = local.name,
                        senderName = local.senderName,
                        chatName = local.chatName,
                        whenText = relativeTime(local.postedAt),
                        confidence = local.confidence,
                        isImage = local.isImage,
                        remoteId = local.remoteId,
                    )
                } else {
                    val sender = o.optString("sender").ifBlank { null }
                    val folder = o.optString("folder").ifBlank { null }
                    FoundFile(
                        id = 0,
                        remoteId = remoteId,
                        fileName = name,
                        senderName = sender,
                        chatName = null,
                        whenText = "",
                        confidence = o.optDouble("confidence", 0.0),
                        isImage = app.reyna.attribution.Attribution.isImage(name),
                        subtitle = listOfNotNull(sender, folder).joinToString(" · ")
                            .ifBlank { "Available in Reyna" },
                    )
                }
            }
        }.getOrDefault(emptyList())
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
