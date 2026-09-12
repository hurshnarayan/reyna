package app.reyna.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
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
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.AddCircleOutline
import androidx.compose.material.icons.rounded.ArrowUpward
import androidx.compose.material.icons.rounded.Article
import androidx.compose.material.icons.rounded.BarChart
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.CloudOff
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Forum
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Schedule
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.UploadFile
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
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalHapticFeedback
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.text.style.TextOverflow
import app.reyna.attribution.Attribution
import app.reyna.ui.components.ConfidenceDot
import app.reyna.ui.components.ReynaMascot
import app.reyna.ui.components.ReynaMascotAnimated
import app.reyna.ui.components.Bubble
import app.reyna.ui.components.BubbleText
import app.reyna.ui.components.BubbleTime
import app.reyna.ui.components.FileChip
import app.reyna.ui.components.QuickLookModal
import app.reyna.ui.components.ReynaAvatar
import app.reyna.search.SearchableFile
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
    /** Backend id when a fetch result is not present in Room on this phone. */
    val remoteId: Long = 0,
    /** Optional truthful subtitle for backend-only results. */
    val subtitle: String? = null,
)

/**
 * A passage an answer rests on.
 *
 * [quote] is the line itself and [context] the lines around it, so the sheet
 * can show where an answer came from rather than asserting that it came from
 * somewhere.
 */
data class Source(
    val fileId: Long,
    val fileName: String,
    val senderName: String?,
    val sharedAt: String?,
    val folder: String?,
    val quote: String,
    val context: String,
    val confidence: Double,
    /** Page the quote sits on, so the viewer can open there. */
    val page: Int,
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
    /** When this turn was written, which is where a retry rewinds to. */
    val at: Long = 0,
    /**
     * Set when this is Reyna reporting its own state rather than answering.
     * Rendered as a notice, not as prose.
     */
    val notice: String = "",
    val files: List<FoundFile> = emptyList(),
    /** Evidence, shown behind a button rather than under the answer. */
    val sources: List<Source> = emptyList(),
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
    onOpenFile: (FoundFile) -> Unit = {},
    onOpenSource: (Source) -> Unit = {},
    onAskWhoShared: (Long) -> Unit = {},
    pendingToDrive: Int = 0,
    pushing: Boolean = false,
    onPushToDrive: () -> Unit = {},
    onOpenSettings: () -> Unit = {},
    onCopy: (String) -> Unit = {},
    onRetry: (ChatMessage) -> Unit = {},
    sending: Boolean = false,
    /** What Reyna is doing right now. Empty falls back to a generic line. */
    stage: String = "",
    /** Set when Reyna cannot tell which document was meant and is asking. */
    choice: ReynaViewModel.PendingChoice? = null,
    onChooseCandidate: (List<Long>) -> Unit = {},
    onPreviewCandidate: (Long, String) -> Unit = { _, _ -> },
    loadPreviewPath: suspend (FoundFile) -> String? = { null },
    loadSourcePreviewPath: suspend (Source) -> String? = { null },
) {
    val c = reynaColors
    val listState = rememberLazyListState()
    var sheetSources by remember { mutableStateOf<List<Source>>(emptyList()) }
    var quickLookFile by remember { mutableStateOf<FoundFile?>(null) }
    var quickLookSource by remember { mutableStateOf<Source?>(null) }

    // Whether the choice sheet is on screen, kept apart from whether there is
    // a choice to make. Dismissing the sheet used to throw the candidates away
    // and strand the question; now it only closes the sheet, and the answer
    // that asked keeps a way back into it.
    var sheetOpen by remember { mutableStateOf(false) }
    LaunchedEffect(choice) { if (choice != null) sheetOpen = true }

    // A conversation opens at the newest message, not the oldest.
    //
    // The mark sits after the last message, so scrolling to the last message
    // leaves it just below the fold. Scrolling to the mark keeps the end of
    // the conversation actually visible, which is where the stage text appears
    // while a question is being answered.
    LaunchedEffect(messages.size, sending) {
        if (messages.isNotEmpty()) listState.animateScrollToItem(messages.size)
    }

    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize().background(c.background)) {
        ChatToolbar(
            watchingChats = watchingChats,
            fileCount = fileCount,
            onOpenTracking = onOpenTracking,
            onClearChat = onClearChat,
            onOpenSettings = onOpenSettings,
        )

        // Clean empty state with zero button clutter
        if (messages.isEmpty() && !sending) {
            EmptyChatState(modifier = Modifier.weight(1f).fillMaxWidth())
        } else LazyColumn(
            state = listState,
            modifier = Modifier.weight(1f).fillMaxWidth(),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(
                horizontal = Dimens.page, vertical = 10.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            items(messages.size) { i ->
                MessageRow(
                    msg = messages[i],
                    onOpenFile = onOpenFile,
                    onAskWhoShared = onAskWhoShared,
                    onPreviewStart = { quickLookFile = it },
                    onPreviewEnd = { quickLookFile = null },
                    onShowSources = { sheetSources = it },
                    onCopy = onCopy,
                    onRetry = onRetry,
                    // Only the newest answer offers another go. Re-running an
                    // older one would have to discard everything said after
                    // it, which is a bigger thing than the button looks like.
                    canRetry = !sending && i == messages.lastIndex,
                )
            }

			if (choice != null && !sheetOpen) item {
				ActionChip(
					icon = Icons.Rounded.Article,
					label = "Choose a document",
					onClick = { sheetOpen = true },
				)
			}
            // The mark stays, and it is the same mark either way.
            //
            // It used to be created when a question went out and destroyed
            // when the answer landed, so the thing that had been working
            // vanished at the moment it finished. Left standing under the last
            // answer it becomes the end of the conversation, and asking again
            // wakes the object already on the screen rather than replacing it
            // with a different one.
            if (messages.isNotEmpty()) item { MarkRow(sending = sending, stage = stage) }
        }

        if (choice != null && sheetOpen) {
            ChoiceSheet(
                choice = choice,
                onChoose = onChooseCandidate,
                onPreview = onPreviewCandidate,
                onDismiss = { sheetOpen = false },
            )
        }

        if (sheetSources.isNotEmpty()) {
            SourcesSheet(
                sources = sheetSources,
                onOpenSource = {
                    sheetSources = emptyList()
                    onOpenSource(it)
                },
                onPreviewSource = { src ->
                    quickLookSource = src
                },
                onHoldStart = { src ->
                    quickLookSource = src
                },
                onHoldEnd = {
                    quickLookSource = null
                },
                onAskWhoShared = onAskWhoShared,
                onDismiss = { sheetSources = emptyList() },
            )
        }

        Composer(
            onSend = onSend,
            onImport = onImport,
            onAddFile = onAddFile,
            onStop = onStop,
            sending = sending,
        )
        }

        quickLookFile?.let { found ->
            val preview = SearchableFile(
                id = if (found.id > 0L) found.id else -found.remoteId,
                fileName = found.fileName,
                senderName = found.senderName,
                chatName = found.chatName,
                whenText = found.whenText,
                confidence = found.confidence,
                isImage = found.isImage,
                remoteId = found.remoteId,
            )
            QuickLookModal(
                file = preview,
                loadPreviewPath = { loadPreviewPath(found) },
                onDismiss = { quickLookFile = null },
            )
        }

        quickLookSource?.let { src ->
            val preview = SearchableFile(
                id = if (src.fileId > 0L) -src.fileId else -1L,
                fileName = src.fileName,
                senderName = src.senderName,
                chatName = null,
                whenText = src.sharedAt.orEmpty(),
                confidence = src.confidence,
                isImage = app.reyna.attribution.Attribution.isImage(src.fileName),
                remoteId = src.fileId,
            )
            QuickLookModal(
                file = preview,
                loadPreviewPath = { loadSourcePreviewPath(src) },
                onDismiss = { quickLookSource = null },
                onOpen = {
                    quickLookSource = null
                    sheetSources = emptyList()
                    onOpenSource(src)
                },
                onAskWhoShared = {
                    val id = src.fileId
                    quickLookSource = null
                    onAskWhoShared(id)
                },
            )
        }
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
            Box(
                Modifier
                    .size(38.dp)
                    .clip(CircleShape)
                    .background(c.surfaceRaised)
                    .border(1.dp, c.border, CircleShape)
                    .clickable { onOpenTracking() },
                contentAlignment = Alignment.Center,
            ) {
                ReynaAvatar(size = 26.dp)
            }
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
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(
                        Modifier
                            .size(6.dp)
                            .clip(CircleShape)
                            .background(c.confident)
                    )
                    Spacer(Modifier.width(5.dp))
                    Text(
                        "Watching $watchingChats chats · $fileCount files",
                        fontSize = 12.sp,
                        color = c.onSurfaceMuted,
                    )
                }
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
/**
 * Reyna itself, at the end of the conversation.
 *
 * At rest it is a quiet mark under the last answer, the way a signature sits
 * at the foot of a letter: it says who has been speaking and marks where the
 * conversation currently ends. While a question is being answered the same
 * mark darkens and its drops begin to move, and the stage text appears beside
 * it.
 *
 * One object in two states rather than two objects. A separate indicator that
 * appeared on send and disappeared on arrival meant the thing that had been
 * working vanished at the exact moment it finished, and the next question
 * created a new one from nothing. This wakes up and settles instead.
 *
 * No bubble, because the answer above it has none either.
 */
@Composable
private fun MarkRow(sending: Boolean, stage: String) {
    val c = reynaColors

    Row(
        Modifier.fillMaxWidth().padding(top = 8.dp, bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (sending) {
            ReynaMascotAnimated(
                modifier = Modifier.size(36.dp),
                contentDescription = null,
            )
        } else {
            ReynaMascot(
                modifier = Modifier.size(36.dp),
                contentDescription = null,
            )
        }

        // The server says which document it is reading, and reading is where
        // nearly all the time goes. One fixed label for the whole wait made a
        // question progressing normally look exactly like one that had hung.
        AnimatedVisibility(
            visible = sending,
            enter = fadeIn(tween(220)),
            exit = fadeOut(tween(160)),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Spacer(Modifier.width(10.dp))
                Text(
                    stage.ifBlank { "Looking through your files" },
                    fontSize = 14.sp,
                    color = c.onSurfaceMuted,
                    maxLines = 2,
                    overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis,
                )
            }
        }
    }
}


@Composable
private fun MessageRow(
    msg: ChatMessage,
    onShowSources: (List<Source>) -> Unit,
    onOpenFile: (FoundFile) -> Unit,
    onAskWhoShared: (Long) -> Unit,
    onPreviewStart: (FoundFile) -> Unit,
    onPreviewEnd: () -> Unit,
    onCopy: (String) -> Unit = {},
    onRetry: (ChatMessage) -> Unit = {},
    canRetry: Boolean = false,
) {
    if (msg.fromUser) {
        // A question is a thing somebody said, so it keeps its bubble. Both
        // sides losing theirs would leave a page of undifferentiated text with
        // no way to see who was speaking.
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            Bubble(fromUser = true) { BubbleText(msg.text, fromUser = true) }
        }
        return
    }
    if (msg.notice.isNotEmpty()) {
        NoticeCard(msg)
        return
    }
    AnswerBlock(
        msg, onShowSources, onOpenFile, onAskWhoShared,
        onPreviewStart, onPreviewEnd, onCopy, onRetry, canRetry,
    )
}

/**
 * Reyna reporting its own state, rather than answering.
 *
 * Running out of the day's reading allowance is the case this exists for. As
 * prose it read as an answer that had gone wrong, sitting in the same place
 * and the same type as a real reply, so the eye had to finish the sentence
 * before learning nothing had been searched. Given its own shape it is legible
 * before it is read.
 *
 * Amber, not red, and no exclamation. Nothing has failed and nothing is lost:
 * the documents are all still there and the allowance comes back on its own.
 * Red would say something is broken and send the user looking for a fix that
 * does not exist.
 */
@Composable
private fun NoticeCard(msg: ChatMessage) {
    val c = reynaColors
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 8.dp, bottom = 2.dp)
            .clip(RoundedCornerShape(14.dp))
            .background(c.noticeSurface)
            .border(1.dp, c.notice.copy(alpha = 0.28f), RoundedCornerShape(14.dp))
            .padding(horizontal = 14.dp, vertical = 13.dp),
    ) {
        Row(verticalAlignment = Alignment.Top) {
            Icon(
                // A clock for a wait, a cloud for a server that is not there.
                // Same amber either way: neither has lost anything.
                if (msg.notice == app.reyna.net.ReynaApi.NOTICE_UNREACHABLE) {
                    Icons.Rounded.CloudOff
                } else {
                    Icons.Rounded.Schedule
                },
                contentDescription = null,
                tint = c.notice,
                // Nudged to sit on the first line's baseline rather than
                // centred on a block of text three lines tall.
                modifier = Modifier.size(17.dp).padding(top = 2.dp),
            )
            Spacer(Modifier.width(10.dp))
            Text(
                msg.text,
                fontSize = 14.sp,
                lineHeight = 21.sp,
                color = c.notice,
            )
        }

    }
}

/**
 * An answer, with no bubble around it.
 *
 * A bubble is a container for something one party said to another, which is
 * the right shape for a question and the wrong one for a document being read
 * back. Boxing the answer capped it at 286dp, broke every line short of the
 * margin, and made a four sentence reply look like a wall of chat. Set on the
 * page instead, the answer is just the text, which is what the user came for,
 * and the eye goes to the words rather than to the shape holding them.
 *
 * What replaces the bubble as a boundary is the space above it and the row of
 * controls below. Both only exist on answers, so the alternation between a
 * right-aligned bubble and full-width prose is what carries the turn-taking.
 */
@Composable
private fun AnswerBlock(
    msg: ChatMessage,
    onShowSources: (List<Source>) -> Unit,
    onOpenFile: (FoundFile) -> Unit,
    onAskWhoShared: (Long) -> Unit,
    onPreviewStart: (FoundFile) -> Unit,
    onPreviewEnd: () -> Unit,
    onCopy: (String) -> Unit,
    onRetry: (ChatMessage) -> Unit,
    canRetry: Boolean,
) {
    val c = reynaColors
    Column(Modifier.fillMaxWidth().padding(top = 6.dp, bottom = 2.dp)) {
        if (msg.text.isNotEmpty()) {
            Text(
                msg.text,
                fontSize = 15.5.sp,
                // Looser than a bubble's, because a full width line needs more
                // room between rows to stay readable across the measure.
                lineHeight = 24.sp,
                color = c.onSurface,
            )
        }

        if (msg.files.isNotEmpty()) {
            Spacer(Modifier.height(10.dp))
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                msg.files.forEach { file ->
                    FileChip(
                        fileName = file.fileName,
                        senderName = file.senderName,
                        chatName = file.chatName,
                        whenText = file.whenText,
                        confidence = file.confidence,
                        isImage = file.isImage,
                        subtitle = file.subtitle,
                        canAskWhoShared = file.id > 0,
                        onOpen = { onOpenFile(file) },
                        onAskWhoShared = { onAskWhoShared(file.id) },
                        onHoldStart = { onPreviewStart(file) },
                        onHoldEnd = onPreviewEnd,
                    )
                }
            }
        }

        Spacer(Modifier.height(9.dp))
        AnswerActions(
            msg = msg,
            onShowSources = onShowSources,
            onCopy = onCopy,
            onRetry = onRetry,
            canRetry = canRetry,
        )
    }
}

/**
 * The controls under an answer: what it rests on, and what to do with it.
 *
 * Quiet by default and never in the way of the reading. Everything here is an
 * action on the answer above it, which is why it sits under that answer rather
 * than in a menu somewhere: the moment you want to copy a reply is the moment
 * you have finished reading it.
 *
 * Deliberately absent: rating buttons, because nothing records a rating and a
 * control that silently discards what you tell it is worse than no control;
 * and read aloud, which needs the text to speech engine wired up and is not
 * free to add.
 */
@Composable
private fun AnswerActions(
    msg: ChatMessage,
    onShowSources: (List<Source>) -> Unit,
    onCopy: (String) -> Unit,
    onRetry: (ChatMessage) -> Unit,
    canRetry: Boolean,
) {
    val c = reynaColors
    var copied by remember(msg.text) { mutableStateOf(false) }

    Row(verticalAlignment = Alignment.CenterVertically) {
        // Evidence first, because it is the only control here that changes
        // what the user knows rather than what they have.
        //
        // A one line answer used to arrive beneath four file cards, which made
        // the reply three times taller than the thing asked for and made a
        // correct answer look like a guess. The sources are still one tap
        // away, because an app that claims not to invent things has to let
        // people check.
        if (msg.sources.isNotEmpty()) {
            ActionChip(
                icon = Icons.Rounded.Article,
                label = if (msg.sources.size == 1) "1 source" else "${msg.sources.size} sources",
                onClick = { onShowSources(msg.sources) },
            )
            Spacer(Modifier.width(2.dp))
        }

        ActionIcon(
            icon = if (copied) Icons.Rounded.Check else Icons.Rounded.ContentCopy,
            label = if (copied) "Copied" else "Copy",
        ) {
            onCopy(msg.text)
            copied = true
        }

        if (canRetry) {
            ActionIcon(icon = Icons.Rounded.Refresh, label = "Try again") { onRetry(msg) }
        }

        Spacer(Modifier.width(6.dp))
        Text(msg.time, fontSize = 11.sp, color = c.onSurfaceFaint)
    }
}

/** One icon-only control in the row under an answer. */
@Composable
private fun ActionIcon(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    onClick: () -> Unit,
) {
    val c = reynaColors
    Box(
        Modifier
            .clip(RoundedCornerShape(7.dp))
            .clickable(onClick = onClick)
            .padding(6.dp),
    ) {
        Icon(icon, contentDescription = label, tint = c.onSurfaceMuted, modifier = Modifier.size(15.dp))
    }
}

/** One labelled control in the row under an answer. */
@Composable
private fun ActionChip(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    onClick: () -> Unit,
) {
    val c = reynaColors
    Row(
        Modifier
            .clip(RoundedCornerShape(7.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 6.dp, vertical = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, null, tint = c.onSurfaceMuted, modifier = Modifier.size(14.dp))
        Spacer(Modifier.width(5.dp))
        Text(label, fontSize = 12.sp, fontWeight = FontWeight.Medium, color = c.onSurfaceMuted)
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
    val haptic = LocalHapticFeedback.current
    var text by remember { mutableStateOf("") }
    var attachOpen by remember { mutableStateOf(false) }
    val canSend = text.isNotBlank() && !sending

    val sendScale by animateFloatAsState(
        targetValue = if (canSend || sending) 1f else 0.94f,
        label = "send-scale",
    )

    Column(Modifier.background(c.background)) {
        Box(Modifier.fillMaxWidth().height(1.dp).background(c.divider))
        Row(
            Modifier
                .fillMaxWidth()
                .imePadding()
                .padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            // Attach button with dropdown
            Box {
                Box(
                    Modifier
                        .size(42.dp)
                        .clip(CircleShape)
                        .background(c.surfaceRaised)
                        .border(1.dp, c.border, CircleShape)
                        .clickable { attachOpen = true },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(
                        Icons.Rounded.Add,
                        contentDescription = "Attach",
                        tint = c.onSurface,
                        modifier = Modifier.size(22.dp),
                    )
                }
                DropdownMenu(
                    expanded = attachOpen,
                    onDismissRequest = { attachOpen = false },
                    modifier = Modifier.background(c.surface),
                ) {
                    DropdownMenuItem(
                        text = { Text("Add a file") },
                        leadingIcon = {
                            Icon(Icons.Rounded.UploadFile, null, tint = c.accent, modifier = Modifier.size(20.dp))
                        },
                        onClick = { attachOpen = false; onAddFile() },
                    )
                    DropdownMenuItem(
                        text = { Text("Import a chat") },
                        leadingIcon = {
                            Icon(Icons.Rounded.Forum, null, tint = c.accent, modifier = Modifier.size(20.dp))
                        },
                        onClick = { attachOpen = false; onImport() },
                    )
                }
            }

            Spacer(Modifier.width(8.dp))

            // Text input capsule with clear button
            Box(
                Modifier
                    .weight(1f)
                    .clip(RoundedCornerShape(22.dp))
                    .background(c.surfaceRaised)
                    .border(
                        1.dp,
                        if (text.isNotBlank()) c.accent.copy(alpha = 0.5f) else c.border,
                        RoundedCornerShape(22.dp),
                    )
                    .padding(horizontal = 15.dp, vertical = 10.dp),
            ) {
                Row(
                    Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(Modifier.weight(1f)) {
                        if (text.isEmpty()) {
                            Text("Ask Reyna...", fontSize = 15.sp, color = c.onSurfaceMuted)
                        }
                        BasicTextField(
                            value = text,
                            onValueChange = { text = it },
                            textStyle = TextStyle(fontSize = 15.sp, color = c.onSurface),
                            cursorBrush = SolidColor(c.accent),
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                    if (text.isNotEmpty()) {
                        Box(
                            Modifier
                                .size(22.dp)
                                .clip(CircleShape)
                                .clickable { text = "" },
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(
                                Icons.Rounded.Close,
                                contentDescription = "Clear",
                                tint = c.onSurfaceMuted,
                                modifier = Modifier.size(15.dp),
                            )
                        }
                    }
                }
            }

            Spacer(Modifier.width(8.dp))

            // Send / Stop button
            Box(
                Modifier
                    .size(42.dp)
                    .graphicsLayer {
                        scaleX = sendScale
                        scaleY = sendScale
                    }
                    .clip(CircleShape)
                    .background(
                        when {
                            sending -> Color(0xFFEF4444)
                            canSend -> c.onSurface
                            else -> c.surfaceRaised
                        }
                    )
                    .then(
                        if (!canSend && !sending) Modifier.border(1.dp, c.border, CircleShape)
                        else Modifier
                    )
                    .clickable(enabled = sending || canSend) {
                        haptic.performHapticFeedback(HapticFeedbackType.LongPress)
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
                        sending -> Color.White
                        canSend -> c.background
                        else -> c.onSurfaceFaint
                    },
                    modifier = Modifier.size(if (sending) 18.dp else 20.dp),
                )
            }
        }
    }
}

/**
 * Minimal empty state: centered mascot, clean title and subtitle, zero clutter.
 */
@Composable
private fun EmptyChatState(
    modifier: Modifier = Modifier,
) {
    val c = reynaColors

    Box(
        modifier = modifier.padding(horizontal = Dimens.page, vertical = 40.dp),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Box(
                Modifier
                    .size(80.dp)
                    .clip(CircleShape)
                    .background(c.surfaceRaised)
                    .border(1.dp, c.border, CircleShape),
                contentAlignment = Alignment.Center,
            ) {
                ReynaMascot(
                    modifier = Modifier.size(54.dp),
                    contentDescription = null,
                )
            }

            Spacer(Modifier.height(18.dp))

            Text(
                "What can I find?",
                fontSize = 20.sp,
                fontWeight = FontWeight.SemiBold,
                color = c.onSurface,
                textAlign = TextAlign.Center,
            )

            Spacer(Modifier.height(8.dp))

            Text(
                "Ask for any document, note, or receipt.",
                fontSize = 14.sp,
                color = c.onSurfaceMuted,
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(horizontal = 24.dp),
            )
        }
    }
}

/**
 * Where an answer came from.
 *
 * Shows the passage in its surroundings with the quoted line marked, rather
 * than the passage alone. A sentence lifted out of a document proves less than
 * the same sentence sitting between the lines that came before and after it,
 * and the point of this sheet is to let someone check rather than trust.
 */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
private fun SourcesSheet(
    sources: List<Source>,
    onOpenSource: (Source) -> Unit,
    onPreviewSource: (Source) -> Unit,
    onHoldStart: (Source) -> Unit,
    onHoldEnd: () -> Unit,
    onAskWhoShared: (Long) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = reynaColors
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = c.background,
        dragHandle = { BottomSheetDefaults.DragHandle(color = c.onSurfaceFaint) },
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .padding(bottom = 28.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                if (sources.size == 1) "Where this came from" else "Where this came from",
                fontSize = 17.sp,
                fontWeight = FontWeight.SemiBold,
                color = c.onSurface,
            )
            sources.forEach { s ->
                SourceCard(
                    source = s,
                    onOpenSource = onOpenSource,
                    onPreviewSource = onPreviewSource,
                    onHoldStart = onHoldStart,
                    onHoldEnd = onHoldEnd,
                    onAskWhoShared = onAskWhoShared,
                )
            }
        }
    }
}

@Composable
private fun SourceCard(
    source: Source,
    onOpenSource: (Source) -> Unit,
    onPreviewSource: (Source) -> Unit,
    onHoldStart: (Source) -> Unit,
    onHoldEnd: () -> Unit,
    onAskWhoShared: (Long) -> Unit,
) {
    val c = reynaColors
    val haptic = LocalHapticFeedback.current
    val scope = rememberCoroutineScope()
    var isPressed by remember { mutableStateOf(false) }

    Column(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(11.dp))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(11.dp))
            .pointerInput(source.fileId, source.fileName) {
                var isHeld = false
                detectTapGestures(
                    onTap = {
                        if (!isHeld) {
                            onPreviewSource(source)
                        }
                    },
                    onLongPress = {
                        // Consumed to prevent conflict with press detection
                    },
                    onPress = {
                        isHeld = false
                        isPressed = true
                        val holdJob = scope.launch {
                            delay(220)
                            isHeld = true
                            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                            onHoldStart(source)
                        }
                        try {
                            tryAwaitRelease()
                        } finally {
                            holdJob.cancel()
                            isPressed = false
                            if (isHeld) {
                                onHoldEnd()
                            }
                        }
                    },
                )
            }
            .padding(13.dp),
    ) {
        Text(
            source.fileName,
            fontSize = 14.sp,
            fontWeight = FontWeight.Medium,
            color = c.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        Spacer(Modifier.height(4.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            ConfidenceDot(source.confidence)
            Spacer(Modifier.width(6.dp))
            Text(
                Attribution.describe(
                    source.confidence, source.senderName, source.folder, source.sharedAt.orEmpty(),
                ),
                fontSize = 12.sp,
                color = c.onSurfaceMuted,
            )
        }

        Spacer(Modifier.height(11.dp))
        // The passage, with the quoted line marked inside its own context. A
        // long document scrolls here rather than stretching the sheet off the
        // screen.
        Box(
            Modifier
                .fillMaxWidth()
                .heightIn(max = 220.dp)
                .clip(RoundedCornerShape(8.dp))
                .background(c.bubbleIncoming)
                .verticalScroll(rememberScrollState())
                .padding(10.dp),
        ) {
            Text(
                highlightQuote(source.context, source.quote, c.accent),
                fontSize = 12.5.sp,
                lineHeight = 19.sp,
                color = c.onSurfaceMuted,
                fontFamily = FontFamily.Monospace,
            )
        }

        Spacer(Modifier.height(11.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            SheetAction("Preview") { onPreviewSource(source) }
            Spacer(Modifier.width(8.dp))
            SheetAction("Open") { onOpenSource(source) }
            if (source.confidence < Attribution.MIN_NAMED) {
                Spacer(Modifier.width(8.dp))
                SheetAction("Who shared this?") { onAskWhoShared(source.fileId) }
            }
        }
    }
}

@Composable
private fun SheetAction(label: String, onClick: () -> Unit) {
    val c = reynaColors
    Box(
        Modifier
            .clip(RoundedCornerShape(7.dp))
            .border(1.dp, c.border, RoundedCornerShape(7.dp))
            .clickable { onClick() }
            .padding(horizontal = 11.dp, vertical = 6.dp),
    ) {
        Text(label, fontSize = 12.sp, fontWeight = FontWeight.Medium, color = c.onSurfaceMuted)
    }
}

/**
 * Marks the quoted line inside the surrounding text.
 *
 * Matched on collapsed whitespace, because the quote and the context come from
 * the same store but a line can differ by a space and failing to highlight the
 * one line that matters would defeat the sheet.
 */
private fun highlightQuote(context: String, quote: String, accent: Color): AnnotatedString {
    val q = quote.trim()
    if (q.isEmpty()) return AnnotatedString(context)
    val norm = { s: String -> s.trim().replace(Regex("\\s+"), " ").lowercase() }
    val target = norm(q)

    return buildAnnotatedString {
        var matched = false
        context.split("\n").forEachIndexed { i, line ->
            if (i > 0) append("\n")
            if (!matched && norm(line) == target) {
                matched = true
                withStyle(
                    SpanStyle(
                        color = accent,
                        fontWeight = FontWeight.SemiBold,
                        background = accent.copy(alpha = 0.10f),
                    )
                ) { append(line) }
            } else {
                append(line)
            }
        }
        if (!matched) {
            // The quote spans lines or is not line aligned. Better to show the
            // passage unmarked than to highlight the wrong thing.
        }
    }
}

/**
 * The choice Reyna offers when it cannot tell which document was meant.
 *
 * A library holding eleven files called "Module 1" cannot answer "what is
 * module 1 about" from any one of them. Picking the top of a ranking and
 * answering as though that had been the question is how a question about
 * ordinary differential equations came back explaining the four ways to look
 * at artificial intelligence: both files were called module 1, and one of them
 * had to be first.
 *
 * Every row opens. Choosing between documents by filename alone is the same
 * guess Reyna just declined to make, so the file itself is one tap away and
 * the sheet stays open behind the preview.
 */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
private fun ChoiceSheet(
    choice: ReynaViewModel.PendingChoice,
    onChoose: (List<Long>) -> Unit,
    onPreview: (Long, String) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = reynaColors
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        containerColor = c.background,
        dragHandle = { BottomSheetDefaults.DragHandle(color = c.onSurfaceFaint) },
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = Dimens.page)
                .padding(bottom = 28.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            // The prompt itself is the title. Reyna already said it in the
            // conversation behind this sheet, and printing it a second time
            // under a generic heading made the same sentence appear twice on
            // one screen.
            Text(
                choice.prompt.ifBlank { "Which one did you mean?" },
                fontSize = 16.sp,
                lineHeight = 22.sp,
                fontWeight = FontWeight.SemiBold,
                color = c.onSurface,
            )
            choice.candidates.forEach { cand ->
                CandidateCard(cand, onChoose = { onChoose(listOf(cand.fileId)) }, onPreview = { onPreview(cand.fileId, cand.fileName) })
            }
            // Answering from all of them is a real answer to "what is module 1
            // about" when the person genuinely meant the set, and it is the
            // only way out of the sheet that is not either a choice or a
            // dismissal.
            if (choice.candidates.size > 1) {
                Text(
                    "Use all ${choice.candidates.size}",
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Medium,
                    color = c.accent,
                    modifier = Modifier
                        .clip(RoundedCornerShape(10.dp))
                        .clickable { onChoose(choice.candidates.map { it.fileId }) }
                        .padding(vertical = 10.dp, horizontal = 4.dp),
                )
            }
        }
    }
}

@Composable
private fun CandidateCard(
    cand: app.reyna.net.ReynaApi.Candidate,
    onChoose: () -> Unit,
    onPreview: () -> Unit,
) {
    val c = reynaColors
    Column(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(14.dp))
            .border(1.dp, c.border, RoundedCornerShape(14.dp))
            .clickable { onChoose() }
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(cand.fileName, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface)

        val where = listOfNotNull(
            cand.folder?.takeIf { it.isNotBlank() },
            cand.sharedAt?.takeIf { it.isNotBlank() },
        ).joinToString(" · ")
        if (where.isNotBlank()) {
            Text(where, fontSize = 12.sp, color = c.onSurfaceMuted)
        }

        // What the document actually opens with, which is the thing that tells
        // two identically named modules apart. When Reyna has not managed to
        // read it, say so rather than showing an empty line: a file it has
        // never opened cannot answer anything, and the user is entitled to
        // know that before spending a tap on it.
        if (cand.readable && !cand.summary.isNullOrBlank()) {
            Text(
                cand.summary,
                fontSize = 13.sp,
                color = c.onSurfaceMuted,
                maxLines = 2,
                overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis,
            )
        } else {
            Text("Not read yet. Opening this will read it first.", fontSize = 12.sp, color = c.onSurfaceFaint)
        }

        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            SheetAction("Use this") { onChoose() }
            SheetAction("Open") { onPreview() }
        }
    }
}
