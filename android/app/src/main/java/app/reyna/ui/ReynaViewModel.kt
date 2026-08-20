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
            repo.observeMessages().collect { rows ->
                _messages.value = rows.map { m ->
                    ChatMessage(
                        text = m.text,
                        fromUser = m.fromUser,
                        time = timeOf(m.at),
                        files = emptyList(),
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
        viewModelScope.launch {
            if (repo.deviceToken.isBlank()) {
                repo.ask(question) // still records the turn, replies honestly
                _toast.value = "Set a device token in Settings to reach the backend"
                return@launch
            }
            repo.ask(question)
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
            val uri = androidx.core.content.FileProvider.getUriForFile(
                app, "${app.packageName}.files", file,
            )
            val intent = Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(uri, if (f.isImage) "image/*" else "application/pdf")
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            runCatching { app.startActivity(intent) }
                .onFailure { _toast.value = "No app on this phone can open that file" }
        }
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

    fun clearToast() { _toast.value = null }

    // ── Formatting ──

    private fun timeOf(millis: Long): String {
        val cal = java.util.Calendar.getInstance().apply { timeInMillis = millis }
        return "%d:%02d".format(
            cal.get(java.util.Calendar.HOUR_OF_DAY),
            cal.get(java.util.Calendar.MINUTE),
        )
    }

    private fun relativeTime(millis: Long): String {
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
