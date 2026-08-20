package app.reyna.ui.components

import androidx.compose.foundation.background
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
 * Reyna's avatar: the mark on a solid disc.
 *
 * Reyna is the other party in this conversation, so it gets an avatar in the
 * toolbar exactly where a messaging app puts the person you are talking to.
 */
@Composable
fun ReynaAvatar(size: Dp = 34.dp) {
    val c = reynaColors
    Box(
        Modifier.size(size).clip(CircleShape).background(c.onSurface),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            painter = painterResource(R.drawable.ic_bolt_logo),
            contentDescription = null,
            tint = c.background,
            modifier = Modifier.size(size * 0.58f),
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
            // Never let a bubble run the full width; a message that touches both
            // edges stops reading as a message.
            .widthIn(max = 300.dp)
            .clip(shape)
            .background(if (fromUser) c.bubbleOutgoing else c.bubbleIncoming)
            .padding(horizontal = 14.dp, vertical = 10.dp),
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
    onOpen: () -> Unit = {},
    onAskWhoShared: () -> Unit = {},
) {
    val c = reynaColors
    val band = c.forConfidence(confidence)
    val uncertain = confidence < Attribution.MIN_NAMED

    Column(
        Modifier
            .widthIn(max = 272.dp)
            .clip(RoundedCornerShape(Dimens.chip))
            .background(c.surfaceRaised)
            .clickable { onOpen() }
            .padding(10.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier
                    .size(38.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(band.copy(alpha = 0.14f)),
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    if (isImage) Icons.Rounded.Image else Icons.Rounded.Description,
                    contentDescription = null,
                    tint = band,
                    modifier = Modifier.size(19.dp),
                )
            }
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    fileName,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = c.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Spacer(Modifier.height(2.dp))
                Text(
                    Attribution.describe(confidence, senderName, chatName, whenText),
                    fontSize = 12.sp,
                    color = c.onSurfaceMuted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        if (uncertain) {
            // The repair affordance sits where the user notices the gap, not
            // buried in settings.
            Spacer(Modifier.height(8.dp))
            Box(
                Modifier
                    .clip(CircleShape)
                    .background(band.copy(alpha = 0.12f))
                    .clickable { onAskWhoShared() }
                    .padding(horizontal = 10.dp, vertical = 5.dp),
            ) {
                Text("Who shared this?", fontSize = 12.sp, fontWeight = FontWeight.Medium, color = band)
            }
        }
    }
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
 * A list row, in Signal's shape: a round-ish tile where an avatar would be, a
 * title, one muted preview line, and a right-aligned time. No card, no border.
 */
@Composable
fun FileRow(
    fileName: String,
    subtitle: String,
    whenText: String,
    band: Color,
    icon: ImageVector,
    onClick: () -> Unit = {},
) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .clickable { onClick() }
            .padding(horizontal = Dimens.page, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier
                .size(46.dp)
                .clip(RoundedCornerShape(14.dp))
                .background(band.copy(alpha = 0.14f)),
            contentAlignment = Alignment.Center,
        ) {
            Icon(icon, null, tint = band, modifier = Modifier.size(21.dp))
        }
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(
                fileName,
                fontSize = 15.sp,
                fontWeight = FontWeight.Medium,
                color = c.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(2.dp))
            Text(
                subtitle,
                fontSize = 13.sp,
                color = c.onSurfaceMuted,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Spacer(Modifier.width(10.dp))
        Text(whenText, fontSize = 12.sp, color = c.onSurfaceMuted)
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
