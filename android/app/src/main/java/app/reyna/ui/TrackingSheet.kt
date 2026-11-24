package app.reyna.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.Forum
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.ui.components.SectionHeader
import app.reyna.ui.components.StatBar
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

data class WatchedChat(val name: String, val fileCount: Int, val lastSeen: String)

/**
 * A permission Reyna needs, and what breaks without it.
 *
 * [consequence] rather than a description, because a bare toggle labelled
 * "Notification access" tells the user nothing about why they should grant it.
 */
data class PermissionState(val name: String, val granted: Boolean, val consequence: String)

data class TrackingState(
    val total: Int,
    val named: Int,
    val chatOnly: Int,
    val unknown: Int,
    val chats: List<WatchedChat>,
    val permissions: List<PermissionState>,
    val onDeviceLabel: String,
    val inDriveLabel: String,
)

/**
 * Everything Reyna is tracking, in one place the user opens deliberately.
 *
 * The counts used to be spread across the home surface, which made the product
 * read as something you check up on rather than something you talk to. They
 * belong here.
 */
@Composable
fun TrackingScreen(state: TrackingState, onRepairUnknown: () -> Unit = {}) {
    val c = reynaColors
    LazyColumn(Modifier.fillMaxWidth().background(c.background)) {
        item {
            Column(Modifier.padding(horizontal = Dimens.page, vertical = 14.dp)) {
                Text(
                    state.total.toString(),
                    fontSize = 34.sp,
                    fontWeight = FontWeight.Bold,
                    color = c.onSurface,
                )
                Text("files captured", fontSize = 14.sp, color = c.onSurfaceMuted)
            }
        }

        item { SectionHeader("Who shared them") }
        item {
            StatBar("Named", state.named, state.total, c.confident)
        }
        item {
            StatBar("Chat only", state.chatOnly, state.total, c.partial)
        }
        item {
            // Tapping the unknown bar is the entry point to the repair flow,
            // because this is where the user notices the gap.
            StatBar("Unknown", state.unknown, state.total, c.unknown, onClick = onRepairUnknown)
        }
        item {
            Text(
                "Reyna never guesses a name. Below its confidence threshold it tells you the chat and the date instead.",
                fontSize = 12.sp,
                color = c.onSurfaceMuted,
                modifier = Modifier.padding(horizontal = Dimens.page, vertical = 8.dp),
            )
        }

        item { SectionHeader("Watching") }
        items(state.chats.size) { i ->
            val chat = state.chats[i]
            Row(
                Modifier.fillMaxWidth().padding(horizontal = Dimens.page, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(
                    Modifier.size(40.dp).clip(CircleShape).background(c.bubbleIncoming),
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(Icons.Rounded.Forum, null, tint = c.onSurfaceMuted, modifier = Modifier.size(19.dp))
                }
                Spacer(Modifier.size(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(chat.name, fontSize = 15.sp, fontWeight = FontWeight.Medium, color = c.onSurface)
                    Text("${chat.fileCount} files · ${chat.lastSeen}", fontSize = 13.sp, color = c.onSurfaceMuted)
                }
            }
        }

        item { SectionHeader("Permissions") }
        items(state.permissions.size) { i ->
            val p = state.permissions[i]
            Row(
                Modifier.fillMaxWidth().padding(horizontal = Dimens.page, vertical = 10.dp),
                verticalAlignment = Alignment.Top,
            ) {
                Icon(
                    if (p.granted) Icons.Rounded.CheckCircle else Icons.Rounded.ErrorOutline,
                    contentDescription = null,
                    tint = if (p.granted) c.confident else c.partial,
                    modifier = Modifier.size(20.dp),
                )
                Spacer(Modifier.size(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(p.name, fontSize = 15.sp, fontWeight = FontWeight.Medium, color = c.onSurface)
                    Text(p.consequence, fontSize = 13.sp, color = c.onSurfaceMuted)
                }
            }
        }

        item { SectionHeader("Storage") }
        item {
            Column(Modifier.padding(horizontal = Dimens.page, vertical = 4.dp)) {
                StorageLine("On this device", state.onDeviceLabel)
                StorageLine("In your Drive", state.inDriveLabel)
                Spacer(Modifier.height(6.dp))
                Text(
                    "Files never touch Reyna's servers. Text pulled from them is sent to answer your questions.",
                    fontSize = 12.sp,
                    color = c.onSurfaceMuted,
                )
                Spacer(Modifier.height(20.dp))
            }
        }
    }
}

@Composable
private fun StorageLine(label: String, value: String) {
    val c = reynaColors
    Row(
        Modifier.fillMaxWidth().padding(vertical = 7.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(label, fontSize = 14.sp, color = c.onSurface)
        Text(value, fontSize = 14.sp, fontWeight = FontWeight.Medium, color = c.onSurfaceMuted)
    }
}
