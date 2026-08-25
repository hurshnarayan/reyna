package app.reyna.ui

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ArrowBack
import androidx.compose.material.icons.rounded.BarChart
import androidx.compose.material.icons.rounded.Forum
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material3.Icon
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.permissions.Permissions
import app.reyna.ui.theme.ReynaTheme
import app.reyna.ui.theme.reynaColors

class MainActivity : ComponentActivity() {

    private val vm: ReynaViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        handleShareIntent(intent)
        setContent {
            ReynaTheme { ReynaApp(vm, this) }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleShareIntent(intent)
    }

    /**
     * A chat export shared from WhatsApp.
     *
     * Registering as a share target is what keeps the import honest: the flow
     * is WhatsApp, Export chat, Reyna, entirely inside WhatsApp's own UI, with
     * no file picker and no browsing of the user's storage.
     */
    private fun handleShareIntent(intent: Intent?) {
        if (intent?.action != Intent.ACTION_SEND) return
        val uri = intent.getParcelableExtra<Uri>(Intent.EXTRA_STREAM) ?: return
        vm.importExport(uri) { }
    }

    override fun onResume() {
        super.onResume()
        // Every permission is granted on a Settings screen outside the app, so
        // returning here is the only reliable moment to re-check.
        vm.refreshPermissions()
        vm.startCaptureIfPossible()
        vm.refreshDriveState()
        // Consent finishes in a browser, so coming back is the only moment the
        // app can learn whether it worked.
        vm.settleDriveConnect()
    }
}

/**
 * The three places, and only three.
 *
 * Settings left the bar for the toolbar overflow: it is somewhere you go
 * occasionally to change something, not a destination you switch between, and
 * giving it a third of the bar said otherwise. Activity took its place because
 * "what has Reyna actually done with my files" is the question this app has to
 * be able to answer on demand.
 */
private enum class Tab(val label: String, val icon: ImageVector) {
    Chat("Chat", Icons.Rounded.Forum),
    Search("Search", Icons.Rounded.Search),
    Activity("Activity", Icons.Rounded.BarChart),
}

@Composable
private fun ReynaApp(vm: ReynaViewModel, activity: ComponentActivity) {
    val c = reynaColors
    val needsOnboarding by vm.needsOnboarding.collectAsState()
    val toast by vm.toast.collectAsState()
    val snackbar = remember { SnackbarHostState() }

    LaunchedEffect(toast) {
        toast?.let {
            snackbar.showSnackbar(it)
            vm.clearToast()
        }
    }

    Box(Modifier.fillMaxSize().background(c.background)) {
        if (needsOnboarding) {
            Onboarding(vm, activity)
        } else {
            MainShell(vm)
        }
        SnackbarHost(
            snackbar,
            Modifier.align(Alignment.BottomCenter).navigationBarsPadding().padding(bottom = 80.dp),
        )
    }
}

@Composable
private fun Onboarding(vm: ReynaViewModel, activity: ComponentActivity) {
    val step by vm.onboardingStep.collectAsState()
    val permissions by vm.permissions.collectAsState()
    val scanning by vm.scanning.collectAsState()
    val scanProgress by vm.scanProgress.collectAsState()
    val driveConnect by vm.driveConnect.collectAsState()
    val files by vm.files.collectAsState()

    BackHandler(enabled = step != OnboardingStep.Welcome) { vm.onboardingBack() }

    OnboardingScreen(
        step = step,
        permissions = permissions,
        // While scanning, the live count from the scanner; afterwards the real
        // number of rows, which is lower when files were already known.
        scannedCount = if (scanning) scanProgress else files.size,
        scanning = scanning,
        onBack = vm::onboardingBack,
        onGrant = { Permissions.request(activity, it) },
        onContinue = vm::onboardingNext,
        onSkip = vm::onboardingSkip,
        driveConnect = driveConnect,
        onConnectDrive = vm::connectDrive,
    )
}

@Composable
private fun MainShell(vm: ReynaViewModel) {
    val c = reynaColors
    var tab by remember { mutableIntStateOf(0) }
    var trackingOpen by remember { mutableStateOf(false) }
    var settingsOpen by remember { mutableStateOf(false) }
    var repairFor by remember { mutableStateOf<Long?>(null) }
    var viewing by remember { mutableStateOf<ViewingPdf?>(null) }

    val messages by vm.messages.collectAsState()
    val files by vm.files.collectAsState()
    val sending by vm.sending.collectAsState()
    val driveState by vm.driveState.collectAsState()
    val pushing by vm.pushing.collectAsState()

    // Any file type: Reyna keeps documents and photographed notes alike, and a
    // narrow filter would hide exactly the scans people want kept.
    val pickFile = rememberLauncherForActivityResult(
        ActivityResultContracts.OpenDocument(),
    ) { uri -> uri?.let { vm.addFile(it) } }

    // Import runs through the system picker when it is not a share, so the
    // button is never a dead end for someone who exported to Files first.
    val pickExport = rememberLauncherForActivityResult(
        ActivityResultContracts.OpenDocument(),
    ) { uri -> uri?.let { vm.importExport(it) { } } }

    BackHandler(enabled = trackingOpen || settingsOpen || repairFor != null || viewing != null) {
        when {
            viewing != null -> viewing = null
            repairFor != null -> repairFor = null
            settingsOpen -> settingsOpen = false
            else -> trackingOpen = false
        }
    }

    Box(Modifier.fillMaxSize().background(c.background).statusBarsPadding()) {
        when {
            viewing != null -> Column(Modifier.fillMaxSize()) {
                val v = viewing!!
                SubToolbar(
                    title = v.file.name,
                    action = "Open in another app",
                    onAction = { vm.openInOtherApp(v.file) },
                ) { viewing = null }
                PdfViewerScreen(
                    file = java.io.File(v.file.path),
                    quote = v.quote,
                    startPage = v.page,
                )
            }

            repairFor != null -> Column(Modifier.fillMaxSize()) {
                SubToolbar("Who shared this?") { repairFor = null }
                RepairScreen(
                    vm = vm,
                    fileId = repairFor!!,
                    onDone = { repairFor = null },
                    onImport = { pickExport.launch(arrayOf("text/plain", "application/zip")) },
                )
            }

            settingsOpen -> Column(Modifier.fillMaxSize()) {
                SubToolbar("Settings") { settingsOpen = false }
                SettingsScreen(
                    vm = vm,
                    onImport = { pickExport.launch(arrayOf("text/plain", "application/zip")) },
                )
            }

            trackingOpen -> Column(Modifier.fillMaxSize()) {
                SubToolbar("Tracking") { trackingOpen = false }
                TrackingScreen(
                    state = vm.trackingState(),
                    onRepairUnknown = {
                        trackingOpen = false
                        tab = 1
                    },
                )
            }

            else -> Column(Modifier.fillMaxSize()) {
                Box(Modifier.weight(1f)) {
                    when (Tab.entries[tab]) {
                        Tab.Chat -> ChatScreen(
                            messages = messages,
                            watchingChats = vm.watchedChatCount,
                            fileCount = files.size,
                            onOpenTracking = { trackingOpen = true },
                            onSend = vm::ask,
                            onImport = { pickExport.launch(arrayOf("text/plain", "application/zip")) },
                            onAddFile = { pickFile.launch(arrayOf("*/*")) },
                            onStop = vm::stopAnswering,
                            onClearChat = vm::clearChat,
                            onOpenFile = vm::openFile,
                            onOpenSource = { src ->
                                // A citation names the backend's file, so the
                                // local row has to be resolved before anything
                                // can be opened. PDFs get the page viewer;
                                // everything else has no page to jump to and
                                // goes to whatever app the phone already uses.
                                val local = vm.localFileFor(src.fileId, src.fileName)
                                when {
                                    local == null ->
                                        vm.note("That file is not on this phone.")
                                    local.name.endsWith(".pdf", ignoreCase = true) ->
                                        viewing = ViewingPdf(local, src.quote, src.page)
                                    else -> vm.openInOtherApp(local)
                                }
                            },
                            onAskWhoShared = { repairFor = it },
                            pendingToDrive = driveState?.pending ?: 0,
                            pushing = pushing,
                            onPushToDrive = vm::pushToDrive,
                            onOpenSettings = { settingsOpen = true },
                            sending = sending,
                        )
                        Tab.Search -> FilesScreen(
                            files = vm.searchableFiles(),
                            onOpen = { vm.openFile(it.id) },
                            onAskWhoShared = { repairFor = it.id },
                        )
                        Tab.Activity -> TrackingScreen(
                            state = vm.trackingState(),
                            onRepairUnknown = { tab = 1 },
                        )
                    }
                }
                TabBar(tab) { tab = it }
            }
        }
    }
}

/** What the PDF viewer is currently showing. */
private data class ViewingPdf(
    val file: app.reyna.data.FileEntity,
    val quote: String,
    val page: Int,
)

@Composable
private fun SubToolbar(
    title: String,
    action: String? = null,
    onAction: (() -> Unit)? = null,
    onBack: () -> Unit,
) {
    val c = reynaColors
    Column {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 6.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                Modifier.size(42.dp).clip(CircleShape).clickable { onBack() },
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.Rounded.ArrowBack, "Back", tint = c.onSurface, modifier = Modifier.size(22.dp))
            }
            Spacer(Modifier.size(4.dp))
            Text(
                title,
                fontSize = 17.sp,
                fontWeight = FontWeight.SemiBold,
                color = c.onSurface,
                maxLines = 1,
                overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            if (action != null && onAction != null) {
                Spacer(Modifier.size(8.dp))
                Box(
                    Modifier
                        .clip(androidx.compose.foundation.shape.RoundedCornerShape(7.dp))
                        .clickable { onAction() }
                        .padding(horizontal = 10.dp, vertical = 6.dp),
                ) {
                    Text(action, fontSize = 12.sp, fontWeight = FontWeight.Medium, color = c.accent)
                }
            }
        }
        Box(Modifier.fillMaxWidth().height(1.dp).background(c.divider))
    }
}

@Composable
private fun TabBar(current: Int, onSelect: (Int) -> Unit) {
    val c = reynaColors
    Column {
        Box(Modifier.fillMaxWidth().height(1.dp).background(c.divider))
        Row(
            Modifier
                .fillMaxWidth()
                .background(c.background)
                .navigationBarsPadding()
                .padding(top = 6.dp, bottom = 6.dp),
            horizontalArrangement = Arrangement.SpaceEvenly,
        ) {
            Tab.entries.forEachIndexed { i, t ->
                val active = i == current
                Column(
                    horizontalAlignment = Alignment.CenterHorizontally,
                    modifier = Modifier
                        .clip(CircleShape)
                        .clickable { onSelect(i) }
                        .padding(horizontal = 14.dp, vertical = 4.dp),
                ) {
                    Box(
                        Modifier
                            .clip(CircleShape)
                            .background(if (active) c.bubbleIncoming else Color.Transparent)
                            .padding(horizontal = 16.dp, vertical = 4.dp),
                    ) {
                        Icon(t.icon, t.label, tint = c.onSurface, modifier = Modifier.size(21.dp))
                    }
                    Spacer(Modifier.height(3.dp))
                    Text(
                        t.label,
                        fontSize = 11.sp,
                        fontWeight = if (active) FontWeight.SemiBold else FontWeight.Normal,
                        color = if (active) c.onSurface else c.onSurfaceMuted,
                    )
                }
            }
        }
    }
}
