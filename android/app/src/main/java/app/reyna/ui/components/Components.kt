package app.reyna.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Bolt
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.R
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors
import androidx.compose.foundation.Canvas

/**
 * The base card.
 *
 * Separation from the page comes from the background tint, not from a drop
 * shadow: the shadow here is almost invisible by design, matching the reference.
 * A heavier one makes the whole screen look cheap.
 */
@Composable
fun ReynaCard(
    modifier: Modifier = Modifier,
    radius: Dp = Dimens.cardRadius,
    padding: Dp = Dimens.cardPadding,
    content: @Composable () -> Unit,
) {
    val c = reynaColors
    Surface(
        modifier = modifier,
        shape = RoundedCornerShape(radius),
        color = c.surface,
        shadowElevation = 1.dp,
    ) {
        Box(Modifier.padding(padding)) { content() }
    }
}

/**
 * The bolt plus wordmark, as it appears in the top bar.
 *
 * The mark is a vector so it stays sharp and tints with the theme. It appears
 * only here, on the launcher icon, and on onboarding.
 */
@Composable
fun ReynaWordmark(modifier: Modifier = Modifier, size: Dp = 22.dp) {
    val c = reynaColors
    Row(modifier, verticalAlignment = Alignment.CenterVertically) {
        Icon(
            painter = painterResource(R.drawable.ic_bolt_logo),
            contentDescription = null,
            tint = c.onSurface,
            modifier = Modifier.size(size),
        )
        Spacer(Modifier.width(6.dp))
        Text(
            "Reyna",
            style = MaterialTheme.typography.titleLarge,
            color = c.onSurface,
        )
    }
}

/**
 * Status pill, top right. Occupies the position the reference gives its streak
 * counter: something the user wants confirmed at a glance without tapping.
 */
@Composable
fun StatusPill(text: String, active: Boolean) {
    val c = reynaColors
    val tint = if (active) c.confident else c.onSurfaceMuted
    Surface(
        shape = CircleShape,
        color = c.surface,
        shadowElevation = 1.dp,
    ) {
        Row(
            Modifier.padding(horizontal = 12.dp, vertical = 7.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(Icons.Rounded.Bolt, null, tint = tint, modifier = Modifier.size(15.dp))
            Spacer(Modifier.width(4.dp))
            Text(text, fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = c.onSurface)
        }
    }
}

/**
 * A progress ring with something at its center.
 *
 * The track is the same color at low alpha rather than a neutral gray, which is
 * what keeps the three stat rings reading as a set.
 */
@Composable
fun ProgressRing(
    progress: Float,
    color: Color,
    modifier: Modifier = Modifier,
    diameter: Dp = 44.dp,
    stroke: Dp = 4.dp,
    center: @Composable () -> Unit = {},
) {
    Box(modifier.size(diameter), contentAlignment = Alignment.Center) {
        Canvas(Modifier.size(diameter)) {
            val w = stroke.toPx()
            val inset = w / 2
            val arcSize = androidx.compose.ui.geometry.Size(size.width - w, size.height - w)
            val topLeft = androidx.compose.ui.geometry.Offset(inset, inset)
            drawArc(
                color = color.copy(alpha = 0.18f),
                startAngle = 0f, sweepAngle = 360f, useCenter = false,
                topLeft = topLeft, size = arcSize,
                style = Stroke(width = w, cap = StrokeCap.Round),
            )
            if (progress > 0f) {
                drawArc(
                    color = color,
                    // Start at twelve o'clock, which is where a progress ring is
                    // read from.
                    startAngle = -90f,
                    sweepAngle = 360f * progress.coerceIn(0f, 1f),
                    useCenter = false,
                    topLeft = topLeft, size = arcSize,
                    style = Stroke(width = w, cap = StrokeCap.Round),
                )
            }
        }
        center()
    }
}

/**
 * The hero card: one headline number against a total, with a ring showing the
 * fraction resolved. The card that overhangs the section above it.
 */
@Composable
fun HeroCard(
    value: Int,
    total: Int,
    label: String,
    modifier: Modifier = Modifier,
) {
    val c = reynaColors
    val fraction = if (total > 0) value.toFloat() / total else 0f
    ReynaCard(modifier.fillMaxWidth(), padding = 20.dp) {
        Row(
            Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column {
                Row(verticalAlignment = Alignment.Bottom) {
                    Text("$value", style = MaterialTheme.typography.displayLarge, color = c.onSurface)
                    Text(
                        "/$total",
                        fontSize = 17.sp,
                        fontWeight = FontWeight.Medium,
                        color = c.onSurfaceMuted,
                        modifier = Modifier.padding(bottom = 7.dp, start = 2.dp),
                    )
                }
                Text(label, fontSize = 13.sp, color = c.onSurfaceMuted)
            }
            ProgressRing(
                progress = fraction,
                color = c.onSurface,
                diameter = 72.dp,
                stroke = 7.dp,
            ) {
                Icon(
                    painter = painterResource(R.drawable.ic_bolt_logo),
                    contentDescription = null,
                    tint = c.onSurface,
                    modifier = Modifier.size(22.dp),
                )
            }
        }
    }
}

/**
 * One of the three confidence buckets.
 *
 * [color] is the band's color and carries the meaning: how much is known about
 * who shared these files, not whether that is good.
 */
@Composable
fun StatCard(
    value: Int,
    total: Int,
    label: String,
    icon: ImageVector,
    color: Color,
    modifier: Modifier = Modifier,
) {
    val c = reynaColors
    val fraction = if (total > 0) value.toFloat() / total else 0f
    ReynaCard(modifier, radius = Dimens.smallCardRadius, padding = 14.dp) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text("$value", style = MaterialTheme.typography.titleLarge, color = c.onSurface)
            Text(label, fontSize = 11.sp, color = c.onSurfaceMuted)
            Spacer(Modifier.size(10.dp))
            ProgressRing(progress = fraction, color = color, diameter = 40.dp, stroke = 4.dp) {
                Icon(icon, null, tint = color, modifier = Modifier.size(15.dp))
            }
        }
    }
}

/** Carousel position indicator, matching the reference. */
@Composable
fun PageDots(count: Int, active: Int, modifier: Modifier = Modifier) {
    val c = reynaColors
    Row(modifier, horizontalArrangement = Arrangement.spacedBy(5.dp)) {
        repeat(count) { i ->
            Box(
                Modifier
                    .size(if (i == active) 6.dp else 5.dp)
                    .clip(CircleShape)
                    .background(if (i == active) c.onSurface else c.outline)
            )
        }
    }
}

/**
 * One day in the week strip.
 *
 * A day with nothing captured gets a dotted outline rather than a filled circle,
 * so an empty day reads as empty rather than as zero of something.
 */
@Composable
fun DayCircle(
    label: String,
    count: Int,
    isToday: Boolean,
    modifier: Modifier = Modifier,
) {
    val c = reynaColors
    Column(modifier, horizontalAlignment = Alignment.CenterHorizontally) {
        Text(
            label,
            fontSize = 11.sp,
            fontWeight = if (isToday) FontWeight.SemiBold else FontWeight.Normal,
            color = if (isToday) c.onSurface else c.onSurfaceMuted,
        )
        Spacer(Modifier.size(5.dp))
        Box(
            Modifier
                .size(30.dp)
                .clip(CircleShape)
                .background(if (count > 0) c.surface else Color.Transparent)
                .border(
                    width = if (isToday) 2.dp else 1.dp,
                    color = when {
                        isToday -> c.onSurface
                        count > 0 -> c.confident
                        else -> c.outline
                    },
                    shape = CircleShape,
                ),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                if (count > 0) "$count" else "",
                fontSize = 12.sp,
                fontWeight = FontWeight.SemiBold,
                color = c.onSurface,
            )
        }
    }
}
