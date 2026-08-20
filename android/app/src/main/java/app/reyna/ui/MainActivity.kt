package app.reyna.ui

import android.os.Bundle
import androidx.activity.ComponentActivity
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.FolderOpen
import androidx.compose.material.icons.rounded.Home
import androidx.compose.material.icons.rounded.IosShare
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.ReynaTheme
import app.reyna.ui.theme.reynaColors

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            ReynaTheme {
                ReynaApp()
            }
        }
    }
}

private enum class Destination(val label: String, val icon: ImageVector) {
    Home("Home", Icons.Rounded.Home),
    Library("Library", Icons.Rounded.FolderOpen),
    Settings("Settings", Icons.Rounded.Settings),
}

@Composable
private fun ReynaApp() {
    val c = reynaColors
    var current by remember { mutableIntStateOf(0) }

    Box(
        Modifier
            .fillMaxSize()
            .background(c.background)
            .statusBarsPadding()
    ) {
        when (Destination.entries[current]) {
            Destination.Home -> HomeScreen(sampleHomeState())
            Destination.Library -> Placeholder("Library", "Your Drive folders, mirrored.")
            Destination.Settings -> Placeholder("Settings", "Drive account, permissions, delete everything.")
        }

        Column(
            Modifier
                .align(Alignment.BottomCenter)
                .navigationBarsPadding()
        ) {
            BottomBar(current) { current = it }
        }

        // Import a chat: the one primary action that is not already on screen.
        // Asking lives at the top of Home, so duplicating it here would be dead
        // weight.
        Surface(
            shape = CircleShape,
            color = c.accent,
            shadowElevation = 6.dp,
            modifier = Modifier
                .align(Alignment.BottomEnd)
                .navigationBarsPadding()
                .padding(end = Dimens.page, bottom = 78.dp)
                .size(56.dp)
                .clickable { },
        ) {
            Box(contentAlignment = Alignment.Center) {
                Icon(
                    Icons.Rounded.IosShare,
                    contentDescription = "Import a chat",
                    tint = c.onAccent,
                    modifier = Modifier.size(23.dp),
                )
            }
        }
    }
}

@Composable
private fun BottomBar(current: Int, onSelect: (Int) -> Unit) {
    val c = reynaColors
    Surface(color = c.surface, shadowElevation = 8.dp) {
        Row(
            Modifier
                .fillMaxWidth()
                .padding(top = 10.dp, bottom = 12.dp),
            horizontalArrangement = Arrangement.SpaceEvenly,
        ) {
            Destination.entries.forEachIndexed { i, d ->
                val active = i == current
                Column(
                    horizontalAlignment = Alignment.CenterHorizontally,
                    modifier = Modifier
                        .clip(CircleShape)
                        .clickable { onSelect(i) }
                        .padding(horizontal = 18.dp, vertical = 2.dp),
                ) {
                    Icon(
                        d.icon,
                        contentDescription = d.label,
                        tint = if (active) c.onSurface else c.onSurfaceMuted,
                        modifier = Modifier.size(23.dp),
                    )
                    Spacer(Modifier.height(3.dp))
                    Text(
                        d.label,
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
        Text(title, fontSize = 20.sp, fontWeight = FontWeight.Bold, color = c.onSurface)
        Spacer(Modifier.height(6.dp))
        Text(subtitle, fontSize = 14.sp, color = c.onSurfaceMuted)
    }
}

/**
 * Stand-in data until the local store is wired in.
 *
 * Deliberately spans all three confidence bands, including files Reyna cannot
 * attribute at all, because that distinction is the thing most likely to be
 * quietly lost while the rest of the app is built.
 */
private fun sampleHomeState() = HomeState(
    attributed = 189,
    total = 247,
    named = 189,
    chatOnly = 34,
    unknown = 24,
    watching = true,
    week = listOf(
        DayCount("Sun", 0),
        DayCount("Mon", 4),
        DayCount("Tue", 12),
        DayCount("Wed", 7, isToday = true),
        DayCount("Thu", 0),
        DayCount("Fri", 0),
        DayCount("Sat", 0),
    ),
    recent = listOf(
        CaptureCard(
            fileName = "Compiler_Lab_Manual.pdf",
            senderName = "Mohit", chatName = "Sem 5 CS",
            whenText = "2 hours ago", confidence = 0.95,
        ),
        CaptureCard(
            fileName = "OS_Module_3.pdf",
            senderName = "Priya", chatName = "Sem 5 CS",
            whenText = "yesterday", confidence = 0.85,
        ),
        CaptureCard(
            fileName = "DOC-20260818-WA0041.pdf",
            senderName = null, chatName = "Sem 5 CS",
            whenText = "18 August", confidence = 0.45,
        ),
        CaptureCard(
            fileName = "IMG-20260812-WA0007.jpg",
            senderName = null, chatName = null,
            whenText = "3 weeks ago", confidence = 0.0, isImage = true,
        ),
    ),
)
