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
import androidx.compose.material.icons.rounded.MoreVert
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
        )

        LazyColumn(
            state = listState,
            modifier = Modifier.weight(1f).fillMaxWidth(),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(
                horizontal = Dimens.page, vertical = 10.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            items(messages.size) { i -> MessageRow(messages[i]) }
        }

        Composer(onSend = onSend)
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
) {
    val c = reynaColors
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
            IconButton(Icons.Rounded.MoreVert, "More") {}
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

@Composable
private fun MessageRow(msg: ChatMessage) {
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
private fun Composer(onSend: (String) -> Unit) {
    val c = reynaColors
    var text by remember { mutableStateOf("") }
    val canSend = text.isNotBlank()

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
            Box(
                Modifier.size(40.dp).clip(CircleShape).clickable { },
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    Icons.Rounded.AddCircleOutline,
                    contentDescription = "Import a chat",
                    tint = c.onSurfaceMuted,
                    modifier = Modifier.size(24.dp),
                )
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
            Box(
                Modifier
                    .size(40.dp)
                    .clip(CircleShape)
                    // Disabled until there is something to send, rather than
                    // failing silently on an empty tap.
                    .background(if (canSend) c.bubbleOutgoing else c.bubbleIncoming)
                    .clickable(enabled = canSend) {
                        onSend(text.trim())
                        text = ""
                    },
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    Icons.Rounded.ArrowUpward,
                    contentDescription = "Send",
                    tint = if (canSend) c.onBubbleOutgoing else c.onSurfaceMuted,
                    modifier = Modifier.size(20.dp),
                )
            }
        }
    }
}
