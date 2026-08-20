package app.reyna.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.IosShare
import androidx.compose.material.icons.rounded.Person
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.attribution.Attribution
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

/**
 * Fixing an attribution Reyna could not make.
 *
 * Reached from any "who shared this?" affordance, which sits where the gap is
 * noticed rather than buried in settings. Two routes out: name the person, or
 * import the chat and fix every unattributed file in it at once.
 */
@Composable
fun RepairScreen(
    vm: ReynaViewModel,
    fileId: Long,
    onDone: () -> Unit,
    onImport: () -> Unit,
) {
    val c = reynaColors
    val files by vm.files.collectAsState()
    val file = files.firstOrNull { it.id == fileId }
    var typed by remember { mutableStateOf("") }
    var known by remember { mutableStateOf<List<String>>(emptyList()) }

    LaunchedEffect(Unit) { known = vm.knownSenders() }

    if (file == null) {
        LaunchedEffect(Unit) { onDone() }
        return
    }

    LazyColumn(Modifier.fillMaxSize().background(c.background)) {
        item {
            Column(Modifier.padding(Dimens.page)) {
                Text(file.name, fontSize = 17.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface)
                Spacer(Modifier.height(4.dp))
                Text(
                    // What Reyna currently believes, stated exactly as it would
                    // be shown anywhere else.
                    Attribution.describe(
                        file.confidence, file.senderName, file.chatName,
                        "on this phone",
                    ),
                    fontSize = 13.sp, color = c.onSurfaceMuted,
                )
                Spacer(Modifier.height(16.dp))
                Text(
                    if (file.confidence < 0.30)
                        "Reyna could not match this file to a message, so it does not know who shared it."
                    else
                        "Reyna matched this by time rather than by name, which is not certain enough to state.",
                    fontSize = 14.sp, color = c.onSurfaceMuted, lineHeight = 20.sp,
                )
            }
        }

        if (known.isNotEmpty()) {
            item {
                Text(
                    "People Reyna knows",
                    fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = c.onSurfaceMuted,
                    modifier = Modifier.padding(horizontal = Dimens.page, vertical = 6.dp),
                )
            }
            items(known.size) { i ->
                val name = known[i]
                Row(
                    Modifier
                        .fillMaxWidth()
                        .clickable { vm.setSender(fileId, name); onDone() }
                        .padding(horizontal = Dimens.page, vertical = 11.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(
                        Modifier.size(38.dp).clip(CircleShape).background(c.bubbleIncoming),
                        contentAlignment = Alignment.Center,
                    ) {
                        Icon(Icons.Rounded.Person, null, tint = c.onSurfaceMuted, modifier = Modifier.size(19.dp))
                    }
                    Spacer(Modifier.size(12.dp))
                    Text(name, fontSize = 15.sp, color = c.onSurface)
                }
            }
        }

        item {
            Column(Modifier.padding(horizontal = Dimens.page, vertical = 10.dp)) {
                Text(
                    "Someone else",
                    fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = c.onSurfaceMuted,
                )
                Spacer(Modifier.height(8.dp))
                Box(
                    Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(14.dp))
                        .background(c.bubbleIncoming)
                        .padding(horizontal = 14.dp, vertical = 12.dp),
                ) {
                    if (typed.isEmpty()) {
                        Text("Type a name", fontSize = 15.sp, color = c.onSurfaceMuted)
                    }
                    BasicTextField(
                        value = typed,
                        onValueChange = { typed = it },
                        singleLine = true,
                        textStyle = TextStyle(fontSize = 15.sp, color = c.onSurface),
                        cursorBrush = SolidColor(c.bubbleOutgoing),
                        modifier = Modifier.fillMaxWidth(),
                    )
                }
                Spacer(Modifier.height(12.dp))
                PrimaryButton("Save", enabled = typed.isNotBlank()) {
                    vm.setSender(fileId, typed.trim())
                    onDone()
                }
            }
        }

        item {
            Column(Modifier.padding(Dimens.page)) {
                Box(Modifier.fillMaxWidth().height(1.dp).background(c.divider))
                Spacer(Modifier.height(18.dp))
                Text(
                    "Fix all of them at once",
                    fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface,
                )
                Spacer(Modifier.height(6.dp))
                Text(
                    "A chat export tells Reyna who shared every file in that chat, going back years. " +
                        "It is read on this phone and only the file records are kept.",
                    fontSize = 13.sp, color = c.onSurfaceMuted, lineHeight = 19.sp,
                )
                Spacer(Modifier.height(14.dp))
                Row(
                    Modifier
                        .clip(CircleShape)
                        .border(1.5.dp, c.onSurface, CircleShape)
                        .clickable { onImport() }
                        .padding(horizontal = 18.dp, vertical = 11.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(Icons.Rounded.IosShare, null, tint = c.onSurface, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.size(8.dp))
                    Text("Import a chat", fontSize = 14.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface)
                }
                Spacer(Modifier.height(24.dp))
            }
        }
    }
}
