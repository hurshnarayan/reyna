package app.reyna.ui

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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.CloudDone
import androidx.compose.material.icons.rounded.DeleteOutline
import androidx.compose.material.icons.rounded.IosShare
import androidx.compose.material.icons.rounded.NotificationsActive
import androidx.compose.material.icons.rounded.PauseCircleOutline
import androidx.compose.material.icons.rounded.Storage
import androidx.compose.material3.Icon
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
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
import app.reyna.ui.components.SectionHeader
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

@Composable
fun SettingsScreen() {
    val c = reynaColors
    var capturing by remember { mutableStateOf(true) }
    var dailyDigest by remember { mutableStateOf(false) }

    LazyColumn(Modifier.fillMaxSize().background(c.background)) {
        item { SectionHeader("Capture") }
        item {
            ToggleRow(
                icon = Icons.Rounded.PauseCircleOutline,
                title = "Watch for new files",
                // What actually stops, rather than a restatement of the label.
                subtitle = "Reyna notices files WhatsApp saves to this phone",
                checked = capturing,
                onChange = { capturing = it },
            )
        }
        item {
            ToggleRow(
                icon = Icons.Rounded.NotificationsActive,
                title = "Daily digest",
                // Off by default: capture is meant to be silent, and a
                // notification per file would be a habit change.
                subtitle = "One summary a day. Reyna never notifies per file.",
                checked = dailyDigest,
                onChange = { dailyDigest = it },
            )
        }

        item { SectionHeader("History") }
        item {
            ActionRow(
                icon = Icons.Rounded.IosShare,
                title = "Import a chat",
                subtitle = "Teaches Reyna who shared the older files",
            )
        }

        item { SectionHeader("Storage") }
        item {
            ActionRow(
                icon = Icons.Rounded.CloudDone,
                title = "Google Drive",
                subtitle = "Connected. Files sync to your own Drive.",
                trailing = "Change",
            )
        }
        item {
            ActionRow(
                icon = Icons.Rounded.Storage,
                title = "On this device",
                subtitle = "1.2 GB of captured files",
            )
        }

        item { SectionHeader("Data") }
        item {
            ActionRow(
                icon = Icons.Rounded.DeleteOutline,
                title = "Delete everything",
                subtitle = "Removes Reyna's records. Your files and Drive stay.",
                tint = c.partial,
            )
        }

        item {
            // The privacy claim, stated where it can be checked rather than only
            // in a store listing. It is true only because extraction streams
            // through the server without being written to disk.
            Text(
                "Files never touch Reyna's servers. Text pulled from a file is sent to answer your questions, and nothing else leaves this phone.",
                fontSize = 12.sp,
                color = c.onSurfaceMuted,
                modifier = Modifier.padding(horizontal = Dimens.page, vertical = 18.dp),
            )
        }
    }
}

@Composable
private fun ToggleRow(
    icon: ImageVector,
    title: String,
    subtitle: String,
    checked: Boolean,
    onChange: (Boolean) -> Unit,
) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .clickable { onChange(!checked) }
            .padding(horizontal = Dimens.page, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RowIcon(icon, c.onSurfaceMuted)
        Spacer(Modifier.size(14.dp))
        Column(Modifier.weight(1f)) {
            Text(title, fontSize = 15.sp, fontWeight = FontWeight.Medium, color = c.onSurface)
            Text(subtitle, fontSize = 13.sp, color = c.onSurfaceMuted)
        }
        Switch(
            checked = checked,
            onCheckedChange = onChange,
            colors = SwitchDefaults.colors(checkedTrackColor = c.bubbleOutgoing),
        )
    }
}

@Composable
private fun ActionRow(
    icon: ImageVector,
    title: String,
    subtitle: String,
    trailing: String? = null,
    tint: Color? = null,
) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .clickable { }
            .padding(horizontal = Dimens.page, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RowIcon(icon, tint ?: c.onSurfaceMuted)
        Spacer(Modifier.size(14.dp))
        Column(Modifier.weight(1f)) {
            Text(
                title,
                fontSize = 15.sp,
                fontWeight = FontWeight.Medium,
                color = tint ?: c.onSurface,
            )
            Text(subtitle, fontSize = 13.sp, color = c.onSurfaceMuted)
        }
        if (trailing != null) {
            Text(trailing, fontSize = 13.sp, color = c.bubbleOutgoing, fontWeight = FontWeight.Medium)
        }
    }
}

@Composable
private fun RowIcon(icon: ImageVector, tint: Color) {
    val c = reynaColors
    Box(
        Modifier.size(38.dp).clip(CircleShape).background(c.bubbleIncoming),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, null, tint = tint, modifier = Modifier.size(19.dp))
    }
}
