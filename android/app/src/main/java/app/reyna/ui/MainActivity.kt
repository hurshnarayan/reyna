package app.reyna.ui

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Forum
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.InsertDriveFile
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.search.SearchableFile
import app.reyna.ui.components.SectionHeader
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.ReynaTheme
import app.reyna.ui.theme.reynaColors

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            ReynaTheme { ReynaApp() }
        }
    }
}

private enum class Tab(val label: String, val icon: ImageVector) {
    Chat("Chat", Icons.Rounded.Forum),
    Files("Files", Icons.Rounded.InsertDriveFile),
    Settings("Settings", Icons.Rounded.Settings),
}

@Composable
private fun ReynaApp() {
    val c = reynaColors
    var tab by remember { mutableIntStateOf(0) }
    var trackingOpen by remember { mutableStateOf(false) }
    var messages by remember { mutableStateOf(sampleConversation()) }

    // Tracking is a pushed screen, so back should close it before leaving the app.
    BackHandler(enabled = trackingOpen) { trackingOpen = false }

    Box(Modifier.fillMaxSize().background(c.background).statusBarsPadding()) {
        if (trackingOpen) {
            Column(Modifier.fillMaxSize()) {
                SubToolbar("Tracking") { trackingOpen = false }
                TrackingScreen(sampleTracking())
            }
        } else {
            Column(Modifier.fillMaxSize()) {
                Box(Modifier.weight(1f)) {
                    when (Tab.entries[tab]) {
                        Tab.Chat -> ChatScreen(
                            messages = messages,
                            watchingChats = 3,
                            fileCount = 247,
                            onOpenTracking = { trackingOpen = true },
                            onSend = { q ->
                                messages = messages + ChatMessage(q, fromUser = true, time = "now")
                            },
                        )
                        Tab.Files -> FilesScreen(files = sampleSearchableFiles())
                        Tab.Settings -> SettingsScreen()
                    }
                }
                // The composer owns the bottom of the chat, so the tab bar only
                // appears on the surfaces that do not have one.
                if (Tab.entries[tab] != Tab.Chat) {
                    TabBar(tab) { tab = it }
                } else {
                    TabBar(tab) { tab = it }
                }
            }
        }
    }
}

@Composable
private fun SubToolbar(title: String, onBack: () -> Unit) {
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
            Text(title, fontSize = 17.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface)
        }
        Box(Modifier.fillMaxWidth().height(1.dp).background(c.divider))
    }
}

/**
 * Tab bar in Signal's shape: icon over label, and the active item marked with a
 * filled pill behind the icon rather than a color change alone.
 */
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
                            .background(if (active) c.bubbleIncoming else androidx.compose.ui.graphics.Color.Transparent)
                            .padding(horizontal = 16.dp, vertical = 4.dp),
                    ) {
                        Icon(
                            t.icon,
                            contentDescription = t.label,
                            tint = c.onSurface,
                            modifier = Modifier.size(21.dp),
                        )
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

@Composable
private fun Placeholder(title: String, subtitle: String) {
    val c = reynaColors
    Column(
        Modifier.fillMaxSize().padding(Dimens.page),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(title, fontSize = 19.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface)
        Spacer(Modifier.height(6.dp))
        Text(subtitle, fontSize = 14.sp, color = c.onSurfaceMuted)
    }
}

// ── Stand-in data until the local store is wired in ──
//
// Deliberately spans all three confidence bands, including files Reyna cannot
// attribute at all, because that distinction is the thing most likely to be
// quietly lost while the rest of the app is built.

private fun sampleConversation() = listOf(
    ChatMessage(
        "I am watching 3 chats and have 247 files. Ask me for something you half remember.",
        fromUser = false, time = "9:02",
    ),
    ChatMessage("that compiler thing mohit sent", fromUser = true, time = "9:04"),
    ChatMessage(
        "Found it.",
        fromUser = false, time = "9:04",
        files = listOf(
            FoundFile("Compiler_Lab_Manual.pdf", "Mohit", "Sem 5 CS", "18 August", 0.95),
        ),
    ),
    ChatMessage("anything else from that week", fromUser = true, time = "9:05"),
    ChatMessage(
        "Two more. I am not sure who shared the second one, so I have only given you the chat and the date.",
        fromUser = false, time = "9:05",
        files = listOf(
            FoundFile("OS_Module_3.pdf", "Priya", "Sem 5 CS", "17 August", 0.85),
            FoundFile("DOC-20260818-WA0041.pdf", null, "Sem 5 CS", "18 August", 0.45),
        ),
    ),
)

private fun sampleFiles() = listOf(
    FoundFile("Compiler_Lab_Manual.pdf", "Mohit", "Sem 5 CS", "09:04", 0.95),
    FoundFile("OS_Module_3.pdf", "Priya", "Sem 5 CS", "08:41", 0.85),
    FoundFile("DOC-20260818-WA0041.pdf", null, "Sem 5 CS", "18 Aug", 0.45),
    FoundFile("IMG-20260812-WA0007.jpg", null, null, "12 Aug", 0.0, isImage = true),
    FoundFile("DBMS_PYQ_2025.pdf", "Rakesh", "Sem 5 CS", "9 Aug", 0.95),
)

private fun sampleTracking() = TrackingState(
    total = 247,
    named = 189,
    chatOnly = 34,
    unknown = 24,
    chats = listOf(
        WatchedChat("Sem 5 CS", 184, "2 hours ago"),
        WatchedChat("Hostel Block C", 41, "yesterday"),
        WatchedChat("Placement 2026", 22, "3 days ago"),
    ),
    permissions = listOf(
        PermissionState("Notification access", true, "Lets Reyna learn who shared a file"),
        PermissionState("All files access", true, "Lets Reyna read WhatsApp's media folder"),
        PermissionState("Battery exemption", false, "Without this, Reyna stops while you sleep"),
    ),
    onDeviceLabel = "1.2 GB",
    inDriveLabel = "890 MB",
)

/**
 * Stand-in library until the local store is wired in.
 *
 * Names are deliberately a mix of the two kinds Reyna actually sees: files
 * people named, and the `DOC-YYYYMMDD-WAnnnn` ones WhatsApp renamed, which are
 * the reason exact search is not enough on its own.
 */
private fun sampleSearchableFiles() = listOf(
    SearchableFile(1, "Compiler_Design_Lab_Manual.pdf", "Mohit", "Sem 5 CS", "09:04", 0.95),
    SearchableFile(2, "OS_Module_3_Scheduling.pdf", "Priya", "Sem 5 CS", "08:41", 0.85),
    SearchableFile(3, "DOC-20260818-WA0041.pdf", null, "Sem 5 CS", "18 Aug", 0.45),
    SearchableFile(4, "IMG-20260812-WA0007.jpg", null, null, "12 Aug", 0.0, isImage = true),
    SearchableFile(5, "DBMS_PYQ_2025.pdf", "Rakesh", "Sem 5 CS", "9 Aug", 0.95),
    SearchableFile(6, "Computer_Networks_Reference.pdf", "Priya", "Sem 5 CS", "4 Aug", 0.95),
    SearchableFile(7, "Hostel_Mess_Menu_August.pdf", "Warden", "Hostel Block C", "1 Aug", 0.95),
    SearchableFile(8, "Placement_Prep_DSA_Sheet.pdf", "Ananya", "Placement 2026", "28 Jul", 0.95),
    SearchableFile(9, "DOC-20260726-WA0013.pdf", null, "Placement 2026", "26 Jul", 0.30),
    SearchableFile(10, "Syllabus_Sem5_Final.pdf", "Mohit", "Sem 5 CS", "20 Jul", 0.95),
)
