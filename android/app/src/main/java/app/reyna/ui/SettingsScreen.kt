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
import androidx.compose.material.icons.rounded.CloudUpload
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.DeleteOutline
import androidx.compose.material.icons.rounded.RestartAlt
import androidx.compose.material.icons.rounded.IosShare
import androidx.compose.material.icons.rounded.NotificationsActive
import androidx.compose.material.icons.rounded.PauseCircleOutline
import androidx.compose.material.icons.rounded.Storage
import androidx.compose.material3.Icon
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.ui.components.SectionHeader
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

@Composable
fun SettingsScreen(vm: ReynaViewModel, onImport: () -> Unit) {
    val c = reynaColors
    var capturing by remember { mutableStateOf(vm.capturing) }
    var dailyDigest by remember { mutableStateOf(vm.dailyDigest) }
    var backend by remember { mutableStateOf(vm.backendUrl) }
    var token by remember { mutableStateOf(vm.deviceToken) }
    var confirmDelete by remember { mutableStateOf(false) }
    var confirmReset by remember { mutableStateOf(false) }
    val scanning by vm.scanning.collectAsState()

    LazyColumn(Modifier.fillMaxSize().background(c.background)) {
        item { SectionHeader("Capture") }
        item {
            ToggleRow(
                icon = Icons.Rounded.PauseCircleOutline,
                title = "Watch for new files",
                // What actually stops, rather than a restatement of the label.
                subtitle = "Reyna notices files WhatsApp saves to this phone",
                checked = capturing,
                onChange = { capturing = it; vm.setCapturing(it) },
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
                onChange = { dailyDigest = it; vm.setDailyDigest(it) },
            )
        }

        item {
            ActionRow(
                icon = Icons.Rounded.Refresh,
                title = if (scanning) "Scanning..." else "Scan for files now",
                subtitle = "Checks the WhatsApp folder immediately",
                onClick = vm::rescanNow,
            )
        }

        item { SectionHeader("History") }
        item {
            ActionRow(
                icon = Icons.Rounded.IosShare,
                title = "Import a chat",
                subtitle = "Teaches Reyna who shared the older files",
                onClick = onImport,
            )
        }

        item { SectionHeader("Backend") }
        item {
            FieldRow("Server", backend, "http://10.0.2.2:8080") {
                backend = it; vm.backendUrl = it
            }
        }
        item {
            // Without this the routes reject every call. Printed by the server
            // at startup when DEVICE_TOKEN is unset.
            FieldRow("Device token", token, "paste from the server log", secret = true) {
                token = it; vm.deviceToken = it
            }
        }
        item {
            ActionRow(
                icon = Icons.Rounded.CloudUpload,
                title = "Send pending files now",
                subtitle = "Uploads anything the backend has not accepted yet",
                onClick = vm::syncNow,
            )
        }

        item { SectionHeader("Storage") }
        item {
            // The connected case states the account and what is outstanding,
            // because "Connect" sitting there forever gives no way to tell a
            // working setup from one that silently stopped filing.
            val drive by vm.driveState.collectAsState()
            val pushing by vm.pushing.collectAsState()
            val connecting by vm.connectingDrive.collectAsState()
            val state = drive
            val connected = state?.connected == true
            ActionRow(
                icon = Icons.Rounded.CloudUpload,
                title = if (connected) "Google Drive" else "Connect Google Drive",
                subtitle = when {
                    connecting -> "Waiting for Google"
                    state == null -> "Files are filed into folders in your own Drive"
                    !state.connected -> "Not connected. Files stay on this phone."
                    state.pending > 0 ->
                        "${state.email}. ${state.inDrive} filed, ${state.pending} waiting."
                    else -> "${state.email}. ${state.inDrive} filed, nothing waiting."
                },
                trailing = when {
                    connecting -> "Connecting"
                    connected -> "Disconnect"
                    else -> "Connect"
                },
                onClick = when {
                    connecting -> null
                    connected -> vm::disconnectDrive
                    else -> vm::connectDrive
                },
            )
            if (state?.connected == true && state.pending > 0) {
                ActionRow(
                    icon = Icons.Rounded.CloudUpload,
                    title = if (pushing) "Filing into Drive" else "File into Drive now",
                    subtitle = "Otherwise this happens on its own within a day",
                    onClick = if (pushing) null else vm::pushToDrive,
                )
            }
        }
        item {
            val tracking = vm.trackingState()
            ActionRow(
                icon = Icons.Rounded.Storage,
                title = "On this device",
                subtitle = "${tracking.total} files, ${tracking.onDeviceLabel}",
            )
        }

        item { SectionHeader("Data") }
        item {
            ActionRow(
                icon = Icons.Rounded.DeleteOutline,
                title = if (confirmDelete) "Tap again to confirm" else "Delete everything",
                subtitle = "Removes Reyna's records. Your files and Drive stay.",
                tint = c.partial,
                // Two taps, because this is not undoable and a single tap on a
                // destructive row is too easy to hit by accident.
                onClick = {
                    if (confirmDelete) { vm.deleteEverything(); confirmDelete = false }
                    else confirmDelete = true
                },
            )
        }
        item {
            // Kept next to Delete everything because it is the same kind of
            // action with one extra consequence, and separating them would hide
            // that this one also throws the records away.
            ActionRow(
                icon = Icons.Rounded.RestartAlt,
                title = if (confirmReset) "Tap again to confirm" else "Start over",
                subtitle = "Clears records and shows the welcome screens again. Server address kept.",
                tint = c.partial,
                onClick = {
                    if (confirmReset) { vm.resetToFirstRun(); confirmReset = false }
                    else confirmReset = true
                },
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
    onClick: (() -> Unit)? = null,
) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .clickable(enabled = onClick != null) { onClick?.invoke() }
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

/**
 * An editable setting.
 *
 * The backend URL and device token have to be reachable from the UI: without
 * them every network call fails, and a user with no way to fix that from inside
 * the app is stuck.
 */
@Composable
private fun FieldRow(
    label: String,
    value: String,
    placeholder: String,
    secret: Boolean = false,
    onChange: (String) -> Unit,
) {
    val c = reynaColors
    Column(Modifier.fillMaxWidth().padding(horizontal = Dimens.page, vertical = 10.dp)) {
        Text(label, fontSize = 13.sp, fontWeight = FontWeight.Medium, color = c.onSurfaceMuted)
        Spacer(Modifier.size(6.dp))
        Box(
            Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(12.dp))
                .background(c.bubbleIncoming)
                .padding(horizontal = 12.dp, vertical = 11.dp),
        ) {
            if (value.isEmpty()) {
                Text(placeholder, fontSize = 14.sp, color = c.onSurfaceMuted)
            }
            BasicTextField(
                value = value,
                onValueChange = onChange,
                singleLine = true,
                textStyle = TextStyle(fontSize = 14.sp, color = c.onSurface),
                cursorBrush = SolidColor(c.bubbleOutgoing),
                visualTransformation = if (secret && value.isNotEmpty())
                    androidx.compose.ui.text.input.PasswordVisualTransformation()
                else androidx.compose.ui.text.input.VisualTransformation.None,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}
