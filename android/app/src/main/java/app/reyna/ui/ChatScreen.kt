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
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.AddCircleOutline
import androidx.compose.material.icons.rounded.ArrowUpward
import androidx.compose.material.icons.rounded.BarChart
import androidx.compose.material.icons.rounded.CloudUpload
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
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
import app.reyna.ui.components.Bubble
import app.reyna.ui.components.BubbleText
import app.reyna.ui.components.BubbleTime
import app.reyna.ui.components.FileChip
import app.reyna.ui.components.ReynaAvatar
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

/** A file Reyna attached to an answer. */
data class FoundFile(
    /** The local row, so a chip in the conversation can open the real file. */
    val id: Long,
    val fileName: String,
    val senderName: String?,
    val chatName: String?,
    val whenText: String,
    val confidence: Double,
    val isImage: Boolean = false,
)

/**
 * One turn in the conversation.
 *
 * Reyna's answers may carry files; a question never does. Attribution travels as
 * a confidence on each file rather than as text in [text], so the prose cannot
 * contradict what the chips are allowed to say.
 */
data class ChatMessage(
    val text: String,
    val fromUser: Boolean,
    val time: String,
    val files: List<FoundFile> = emptyList(),
)

@Composable
fun ChatScreen(
    messages: List<ChatMessage>,
    watchingChats: Int,
    fileCount: Int,
    onOpenTracking: () -> Unit,
    onSend: (String) -> Unit = {},
    onImport: () -> Unit = {},
    onAddFile: () -> Unit = {},
    onStop: () -> Unit = {},
    onClearChat: () -> Unit = {},
    onOpenFile: (Long) -> Unit = {},
    onAskWhoShared: (Long) -> Unit = {},
    pendingToDrive: Int = 0,
    pushing: Boolean = false,
    onPushToDrive: () -> Unit = {},
    onOpenSettings: () -> Unit = {},
    sending: Boolean = false,
) {
    val c = reynaColors
    val listState = rememberLazyListState()

    // A conversation opens at the newest message, not the oldest.
    LaunchedEffect(messages.size) {
        if (messages.isNotEmpty()) listState.animateScrollToItem(messages.lastIndex)
    }

    Column(Modifier.fillMaxSize().background(c.background)) {
        ChatToolbar(
            watchingChats = watchingChats,
            fileCount = fileCount,
            onOpenTracking = onOpenTracking,
            onClearChat = onClearChat,
            onOpenSettings = onOpenSettings,
        )

        // Files that reached the server but not the user's Drive are the one
        // thing Reyna must not stay quiet about. Everything else it does is
        // recoverable by asking again; this is the case where the user thinks
        // their documents are filed and they are not.
        if (pendingToDrive > 0) {
            PendingBanner(pendingToDrive, pushing, onPushToDrive)
        }

        // A thread with nothing in it is the normal way Reyna opens now. It
        // used to greet with a file count on every launch, which read as the
        // app talking to itself, and the count belongs on the tracking screen
        // where it can be looked at rather than in the way of the first
        // question.
        if (messages.isEmpty() && !sending) {
            Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                Text(
                    "Ask for something you half remember.",
                    fontSize = 14.sp,
                    color = c.onSurfaceMuted,
                )
            }
        } else LazyColumn(
            state = listState,
            modifier = Modifier.weight(1f).fillMaxWidth(),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(
                horizontal = Dimens.page, vertical = 10.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            items(messages.size) { i ->
                MessageRow(messages[i], onOpenFile = onOpenFile, onAskWhoShared = onAskWhoShared)
            }
            // A visible "working on it" turn. Twenty seconds of nothing reads
            // as a broken app, and this is also what the Stop button refers to.
            if (sending) item { ThinkingRow() }
        }

        Composer(
            onSend = onSend,
            onImport = onImport,
            onAddFile = onAddFile,
            onStop = onStop,
            sending = sending,
        )
    }
}

/**
 * Toolbar.
 *
 * The subtitle is the whole point: the user needs to know Reyna is working
 * without having to ask it. It states what is being watched in one line, and
 * both it and the chart icon open Tracking, where the counts live.
 */
@Composable
private fun ChatToolbar(
    watchingChats: Int,
    fileCount: Int,
    onOpenTracking: () -> Unit,
    onClearChat: () -> Unit,
    onOpenSettings: () -> Unit,
) {
    val c = reynaColors
    var menuOpen by remember { mutableStateOf(false) }
    Column {
        Row(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = Dimens.page, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ReynaAvatar()
            Spacer(Modifier.width(11.dp))
            Column(
                Modifier
                    .weight(1f)
                    .clip(RoundedCornerShape(6.dp))
                    .clickable { onOpenTracking() }
                    .padding(vertical = 2.dp)
            ) {
                Text(
                    "Reyna",
                    fontSize = 17.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = c.onSurface,
                )
                Text(
                    "Watching $watchingChats chats · $fileCount files",
                    fontSize = 12.sp,
                    color = c.onSurfaceMuted,
                )
            }
            IconButton(Icons.Rounded.BarChart, "Tracking", onOpenTracking)
            Box {
                IconButton(Icons.Rounded.MoreVert, "More") { menuOpen = true }
                DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                    DropdownMenuItem(
                        text = { Text("Clear conversation") },
                        onClick = { menuOpen = false; onClearChat() },
                    )
                    DropdownMenuItem(
                        text = { Text("Settings") },
                        onClick = { menuOpen = false; onOpenSettings() },
                    )
                }
            }
        }
        Box(Modifier.fillMaxWidth().height(1.dp).background(c.divider))
    }
}

@Composable
private fun IconButton(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    onClick: () -> Unit,
) {
    val c = reynaColors
    Box(
        Modifier
            .size(40.dp)
            .clip(CircleShape)
            .clickable { onClick() },
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = label, tint = c.onSurface, modifier = Modifier.size(22.dp))
    }
}

/**
 * Reyna working, shown as its own incoming bubble.
 *
 * Deliberately a message rather than a spinner in the corner: it occupies the
 * place the answer will, so the conversation does not jump when the reply
 * lands.
 */
@Composable
private fun ThinkingRow() {
    val c = reynaColors
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Start) {
        Bubble(fromUser = false) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator(
                    modifier = Modifier.size(14.dp),
                    strokeWidth = 2.dp,
                    color = c.onSurfaceMuted,
                )
                Spacer(Modifier.width(9.dp))
                Text("Looking through your files", fontSize = 14.sp, color = c.onSurfaceMuted)
            }
        }
    }
}

@Composable
private fun MessageRow(
    msg: ChatMessage,
    onOpenFile: (Long) -> Unit,
    onAskWhoShared: (Long) -> Unit,
) {
    Row(
        Modifier.fillMaxWidth(),
        horizontalArrangement = if (msg.fromUser) Arrangement.End else Arrangement.Start,
    ) {
        Bubble(fromUser = msg.fromUser) {
            Column {
                if (msg.text.isNotEmpty()) {
                    BubbleText(msg.text, msg.fromUser)
                }
                if (msg.files.isNotEmpty()) {
                    // Prose first, then one chip per result. The chips carry the
                    // attribution; the prose never restates it.
                    if (msg.text.isNotEmpty()) Spacer(Modifier.height(9.dp))
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        msg.files.forEach { f ->
                            FileChip(
                                fileName = f.fileName,
                                senderName = f.senderName,
                                chatName = f.chatName,
                                whenText = f.whenText,
                                confidence = f.confidence,
                                isImage = f.isImage,
                                onOpen = { onOpenFile(f.id) },
                                onAskWhoShared = { onAskWhoShared(f.id) },
                            )
                        }
                    }
                }
                Spacer(Modifier.height(3.dp))
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                    BubbleTime(msg.time, msg.fromUser)
                }
            }
        }
    }
}

/**
 * The composer. Always present, pinned to the bottom, the way a messaging app
 * keeps the input reachable no matter where the conversation is scrolled.
 */
@Composable
private fun Composer(
    onSend: (String) -> Unit,
    onImport: () -> Unit,
    onAddFile: () -> Unit,
    onStop: () -> Unit,
    sending: Boolean,
) {
    val c = reynaColors
    var text by remember { mutableStateOf("") }
    var attachOpen by remember { mutableStateOf(false) }
    val canSend = text.isNotBlank() && !sending

    Column {
        Box(Modifier.fillMaxWidth().height(1.dp).background(c.divider))
        Row(
            Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .imePadding()
                .padding(horizontal = 10.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            // Attach offers importing a chat, which is how Reyna learns who
            // shared the older files.
            Box {
                Box(
                    Modifier.size(40.dp).clip(CircleShape).clickable { attachOpen = true },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(
                        Icons.Rounded.AddCircleOutline,
                        contentDescription = "Attach",
                        tint = c.onSurfaceMuted,
                        modifier = Modifier.size(24.dp),
                    )
                }
                DropdownMenu(expanded = attachOpen, onDismissRequest = { attachOpen = false }) {
                    // Two different things, and conflating them was the bug:
                    // an export teaches Reyna who shared the older files, a
                    // file is a document to keep.
                    DropdownMenuItem(
                        text = { Text("Add a file") },
                        onClick = { attachOpen = false; onAddFile() },
                    )
                    DropdownMenuItem(
                        text = { Text("Import a chat") },
                        onClick = { attachOpen = false; onImport() },
                    )
                }
            }

            Box(
                Modifier
                    .weight(1f)
                    .clip(CircleShape)
                    .background(c.bubbleIncoming)
                    .padding(horizontal = 16.dp, vertical = 11.dp),
            ) {
                if (text.isEmpty()) {
                    Text("Ask Reyna", fontSize = 15.sp, color = c.onSurfaceMuted)
                }
                BasicTextField(
                    value = text,
                    onValueChange = { text = it },
                    textStyle = TextStyle(fontSize = 15.sp, color = c.onSurface),
                    cursorBrush = SolidColor(c.bubbleOutgoing),
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            Spacer(Modifier.width(8.dp))
            // While an answer is in flight the same button becomes Stop, so
            // changing your mind is one tap in the place you already looked.
            Box(
                Modifier
                    .size(40.dp)
                    .clip(CircleShape)
                    .background(
                        when {
                            sending -> c.onSurface
                            canSend -> c.bubbleOutgoing
                            else -> c.bubbleIncoming
                        }
                    )
                    .clickable(enabled = sending || canSend) {
                        if (sending) {
                            onStop()
                        } else {
                            onSend(text.trim())
                            text = ""
                        }
                    },
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    if (sending) Icons.Rounded.Stop else Icons.Rounded.ArrowUpward,
                    contentDescription = if (sending) "Stop" else "Send",
                    tint = when {
                        sending -> c.background
                        canSend -> c.onBubbleOutgoing
                        else -> c.onSurfaceMuted
                    },
                    modifier = Modifier.size(if (sending) 18.dp else 20.dp),
                )
            }
        }
    }
}


/**
 * The one warning in the app.
 *
 * Stated as a fact with the fix attached, not as an alert. Nothing has gone
 * wrong when files are waiting, they are simply not filed yet, and dressing
 * that in red would teach the user to ignore it by the third time they saw it.
 */
@Composable
private fun PendingBanner(count: Int, pushing: Boolean, onPush: () -> Unit) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = Dimens.page, vertical = 6.dp)
            .clip(RoundedCornerShape(11.dp))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(11.dp))
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            Icons.Rounded.CloudUpload,
            null,
            tint = c.onSurfaceFaint,
            modifier = Modifier.size(17.dp),
        )
        Spacer(Modifier.width(10.dp))
        Text(
            if (count == 1) "1 document is not in your Drive yet"
            else "$count documents are not in your Drive yet",
            fontSize = 13.sp,
            color = c.onSurface,
            modifier = Modifier.weight(1f),
        )
        Spacer(Modifier.width(8.dp))
        Box(
            Modifier
                .clip(RoundedCornerShape(7.dp))
                .border(1.dp, c.border, RoundedCornerShape(7.dp))
                .clickable(enabled = !pushing) { onPush() }
                .padding(horizontal = 10.dp, vertical = 5.dp),
        ) {
            Text(
                if (pushing) "Filing" else "File now",
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
                color = if (pushing) c.onSurfaceFaint else c.accent,
            )
        }
    }
}
