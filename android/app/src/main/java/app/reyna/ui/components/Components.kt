package app.reyna.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.R
import app.reyna.attribution.Attribution
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

/**
 * Reyna's avatar: the mark, and nothing behind it.
 *
 * Reyna is the other party in this conversation, so it sits in the toolbar
 * where a messaging app puts the person you are talking to.
 *
 * No disc. The mark used to be reversed out of a black circle, which is a
 * container the logo does not need and which reads as a sticker pasted onto
 * the page rather than part of it. Drawn straight onto the background in the
 * foreground colour, it is dark on a light theme and light on a dark one
 * without anything having to switch.
 */
@Composable
fun ReynaAvatar(size: Dp = 34.dp) {
    val c = reynaColors
    Box(Modifier.size(size), contentAlignment = Alignment.Center) {
        ReynaLogo(
            color = c.onSurface,
            modifier = Modifier.size(size * 0.82f),
            contentDescription = null,
        )
    }
}

/**
 * A message bubble.
 *
 * The corner nearest the sender is tightened, which is what makes a bubble read
 * as coming from a side rather than floating in the column.
 */
@Composable
fun Bubble(
    fromUser: Boolean,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    val c = reynaColors
    val shape = if (fromUser) {
        RoundedCornerShape(Dimens.bubble, Dimens.bubble, Dimens.bubbleTail, Dimens.bubble)
    } else {
        RoundedCornerShape(Dimens.bubble, Dimens.bubble, Dimens.bubble, Dimens.bubbleTail)
    }
    Box(
        modifier
            // Never let a bubble run the full width; a message that touches
            // both edges stops reading as a message.
            .widthIn(max = 286.dp)
            .clip(shape)
            .background(if (fromUser) c.bubbleOutgoing else c.bubbleIncoming)
            .padding(horizontal = 13.dp, vertical = 9.dp),
    ) { content() }
}

/** Message text, colored for whichever side it is on. */
@Composable
fun BubbleText(text: String, fromUser: Boolean) {
    val c = reynaColors
    Text(
        text,
        fontSize = 15.sp,
        lineHeight = 21.sp,
        color = if (fromUser) c.onBubbleOutgoing else c.onSurface,
    )
}

/** Timestamp, inside the bubble, bottom aligned. */
@Composable
fun BubbleTime(text: String, fromUser: Boolean) {
    val c = reynaColors
    Text(
        text,
        fontSize = 11.sp,
        color = if (fromUser) c.onBubbleOutgoing.copy(alpha = 0.7f) else c.onSurfaceMuted,
    )
}

/**
 * A file Reyna found, rendered inside its answer.
 *
 * This is the shape Signal uses for a PDF attachment, carrying attribution
 * instead of a file size, because who shared it is the thing the user actually
 * asked about.
 *
 * [confidence] rather than a pre-rendered sender string, so the chip cannot
 * state a name the attribution does not support. The subtitle comes from
 * [Attribution.describe] and nowhere else.
 */
@Composable
fun FileChip(
    fileName: String,
    senderName: String?,
    chatName: String?,
    whenText: String,
    confidence: Double,
    isImage: Boolean = false,
    inDrive: Boolean = true,
    onOpen: () -> Unit = {},
    onAskWhoShared: () -> Unit = {},
) {
    val c = reynaColors
    val uncertain = confidence < Attribution.MIN_NAMED

    Column(
        Modifier
            .widthIn(max = 300.dp)
            .clip(RoundedCornerShape(Dimens.chip))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(Dimens.chip))
            .clickable { onOpen() }
            .padding(11.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            // One monochrome glyph, no tinted tile behind it. Colouring the
            // tile by confidence meant every row shouted its status before the
            // user had read the filename, which is what they are scanning for.
            Icon(
                if (isImage) Icons.Rounded.Image else Icons.Rounded.Description,
                contentDescription = null,
                tint = c.onSurfaceFaint,
                modifier = Modifier.size(18.dp),
            )
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    fileName,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Medium,
                    color = c.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Spacer(Modifier.height(3.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    ConfidenceDot(confidence)
                    Spacer(Modifier.width(6.dp))
                    Text(
                        Attribution.describe(confidence, senderName, chatName, whenText),
                        fontSize = 12.sp,
                        color = c.onSurfaceMuted,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                if (!inDrive) {
                    Spacer(Modifier.height(3.dp))
                    Text(
                        "Not in Drive yet",
                        fontSize = 11.sp,
                        color = c.onSurfaceFaint,
                    )
                }
            }
        }
        if (uncertain) {
            // The repair affordance sits where the user notices the gap, not
            // buried in settings.
            Spacer(Modifier.height(9.dp))
            Box(
                Modifier
                    .clip(RoundedCornerShape(7.dp))
                    .border(1.dp, c.border, RoundedCornerShape(7.dp))
                    .clickable { onAskWhoShared() }
                    .padding(horizontal = 9.dp, vertical = 5.dp),
            ) {
                Text(
                    "Who shared this?",
                    fontSize = 12.sp,
                    fontWeight = FontWeight.Medium,
                    color = c.onSurfaceMuted,
                )
            }
        }
    }
}

/**
 * How sure Reyna is, as four pixels.
 *
 * Small on purpose. The sentence beside it already says what is known; the dot
 * only makes a list scannable, so it must not become the loudest thing on the
 * row.
 */
@Composable
fun ConfidenceDot(confidence: Double) {
    val c = reynaColors
    Box(
        Modifier
            .size(5.dp)
            .clip(CircleShape)
            .background(c.forConfidence(confidence)),
    )
}

/** Section header in a list. Quiet, left aligned, the way Signal groups chats. */
@Composable
fun SectionHeader(text: String, modifier: Modifier = Modifier) {
    val c = reynaColors
    Text(
        text,
        fontSize = 13.sp,
        fontWeight = FontWeight.SemiBold,
        color = c.onSurfaceMuted,
        modifier = modifier.padding(horizontal = Dimens.page, vertical = 8.dp),
    )
}

/**
 * A row in the library.
 *
 * A hairline card rather than a coloured tile. The previous version painted a
 * 46dp block tinted by attribution confidence behind every icon, which turned a
 * list of documents into a grid of green and gray squares and made the
 * filenames, the only thing anyone scans for, the second thing seen.
 */
@Composable
fun FileRow(
    fileName: String,
    subtitle: String,
    whenText: String,
    confidence: Double,
    icon: ImageVector,
    inDrive: Boolean = true,
    onClick: () -> Unit = {},
) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = Dimens.page, vertical = 4.dp)
            .clip(RoundedCornerShape(11.dp))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(11.dp))
            .clickable { onClick() }
            .padding(horizontal = 12.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, null, tint = c.onSurfaceFaint, modifier = Modifier.size(19.dp))
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(
                fileName,
                fontSize = 14.sp,
                fontWeight = FontWeight.Medium,
                color = c.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(3.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                ConfidenceDot(confidence)
                Spacer(Modifier.width(6.dp))
                Text(
                    subtitle,
                    fontSize = 12.sp,
                    color = c.onSurfaceMuted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false),
                )
                if (!inDrive) {
                    Spacer(Modifier.width(7.dp))
                    Text("Not in Drive", fontSize = 11.sp, color = c.onSurfaceFaint)
                }
            }
        }
        Spacer(Modifier.width(10.dp))
        Text(whenText, fontSize = 11.sp, color = c.onSurfaceFaint)
    }
}

/**
 * A labelled bar. Used in Tracking to show the attribution split, where three
 * bars make the proportions legible in a way three numbers do not.
 */
@Composable
fun StatBar(
    label: String,
    value: Int,
    total: Int,
    color: Color,
    onClick: (() -> Unit)? = null,
) {
    val c = reynaColors
    val fraction = if (total > 0) value.toFloat() / total else 0f
    Column(
        Modifier
            .fillMaxWidth()
            .then(if (onClick != null) Modifier.clickable { onClick() } else Modifier)
            .padding(horizontal = Dimens.page, vertical = 9.dp)
    ) {
        Row(
            Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Text(label, fontSize = 14.sp, color = c.onSurface)
            Text("$value", fontSize = 14.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface)
        }
        Spacer(Modifier.height(7.dp))
        Box(
            Modifier
                .fillMaxWidth()
                .height(6.dp)
                .clip(CircleShape)
                .background(color.copy(alpha = 0.16f))
        ) {
            Box(
                Modifier
                    .fillMaxWidth(fraction.coerceIn(0f, 1f))
                    .height(6.dp)
                    .clip(CircleShape)
                    .background(color)
            )
        }
    }
}
