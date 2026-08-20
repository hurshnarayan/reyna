package app.reyna.ui

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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Groups
import androidx.compose.material.icons.rounded.HelpOutline
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.Person
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.attribution.Attribution
import app.reyna.ui.components.DayCircle
import app.reyna.ui.components.HeroCard
import app.reyna.ui.components.PageDots
import app.reyna.ui.components.ReynaCard
import app.reyna.ui.components.ReynaWordmark
import app.reyna.ui.components.StatCard
import app.reyna.ui.components.StatusPill
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

/**
 * One captured file, as a row shows it.
 *
 * Carries the confidence rather than a pre-rendered sender string, so the row
 * cannot accidentally state a name the attribution does not support. The
 * subtitle comes from [Attribution.describe] and nowhere else.
 */
data class CaptureCard(
    val fileName: String,
    val senderName: String?,
    val chatName: String?,
    val whenText: String,
    val confidence: Double,
    val isImage: Boolean = false,
) {
    val subtitle: String get() = Attribution.describe(confidence, senderName, chatName, whenText)
    val uncertain: Boolean get() = confidence < Attribution.MIN_NAMED
    val icon: ImageVector get() = if (isImage) Icons.Rounded.Image else Icons.Rounded.Description
}

/** A day in the week strip. */
data class DayCount(val label: String, val count: Int, val isToday: Boolean = false)

/**
 * What Home renders. Held as one object so the screen stays a pure function of
 * its state and can be previewed without a database.
 */
data class HomeState(
    val attributed: Int,
    val total: Int,
    val named: Int,
    val chatOnly: Int,
    val unknown: Int,
    val week: List<DayCount>,
    val recent: List<CaptureCard>,
    val watching: Boolean,
)

@Composable
fun HomeScreen(state: HomeState, onAsk: () -> Unit = {}) {
    val c = reynaColors
    LazyColumn(
        modifier = Modifier
            .background(c.background)
            .fillMaxWidth(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(
            start = Dimens.page, end = Dimens.page, top = 8.dp,
            // Clear the bottom nav and the floating action button.
            bottom = 108.dp,
        ),
        verticalArrangement = Arrangement.spacedBy(Dimens.cardGap),
    ) {
        item {
            Row(
                Modifier.fillMaxWidth().padding(vertical = 6.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                ReynaWordmark()
                StatusPill(if (state.watching) "Watching" else "Paused", state.watching)
            }
        }

        item { AskBox(onClick = onAsk) }

        item {
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                state.week.forEach { d ->
                    DayCircle(d.label, d.count, d.isToday)
                }
            }
        }

        item {
            HeroCard(
                value = state.attributed,
                total = state.total,
                label = "Files attributed",
            )
        }

        item {
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                StatCard(
                    value = state.named, total = state.total, label = "Named",
                    icon = Icons.Rounded.Person, color = c.confident,
                    modifier = Modifier.weight(1f),
                )
                StatCard(
                    value = state.chatOnly, total = state.total, label = "Chat only",
                    icon = Icons.Rounded.Groups, color = c.partial,
                    modifier = Modifier.weight(1f),
                )
                StatCard(
                    value = state.unknown, total = state.total, label = "Unknown",
                    icon = Icons.Rounded.HelpOutline, color = c.unknown,
                    modifier = Modifier.weight(1f),
                )
            }
        }

        item {
            Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                PageDots(count = 3, active = 0)
            }
        }

        item {
            Text(
                "Recently captured",
                style = MaterialTheme.typography.titleMedium,
                color = c.onSurface,
                modifier = Modifier.padding(top = 4.dp),
            )
        }

        items(state.recent) { card -> CaptureRow(card) }
    }
}

/**
 * The primary interaction. It sits above the hero card because asking is what
 * the product is for; everything below it is Reyna reporting on itself.
 */
@Composable
private fun AskBox(onClick: () -> Unit) {
    val c = reynaColors
    Surface(
        shape = RoundedCornerShape(Dimens.smallCardRadius),
        color = c.surface,
        shadowElevation = 1.dp,
        modifier = Modifier.fillMaxWidth().clickable { onClick() },
    ) {
        Row(
            Modifier.padding(horizontal = 16.dp, vertical = 15.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(Icons.Rounded.Search, null, tint = c.onSurfaceMuted, modifier = Modifier.size(19.dp))
            Spacer(Modifier.width(10.dp))
            Text(
                "that thing about the deposit",
                fontSize = 15.sp,
                color = c.onSurfaceMuted,
            )
        }
    }
}

/**
 * One row in the feed.
 *
 * The glyph tile is tinted by confidence band, which makes the split legible
 * while scrolling without any row having to spell it out.
 */
@Composable
private fun CaptureRow(card: CaptureCard) {
    val c = reynaColors
    val bandColor = c.forConfidence(card.confidence)
    ReynaCard(Modifier.fillMaxWidth(), radius = Dimens.smallCardRadius, padding = 12.dp) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier
                    .size(46.dp)
                    .clip(RoundedCornerShape(14.dp))
                    .background(bandColor.copy(alpha = 0.14f)),
                contentAlignment = Alignment.Center,
            ) {
                Icon(card.icon, null, tint = bandColor, modifier = Modifier.size(21.dp))
            }
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    card.fileName,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = c.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Spacer(Modifier.height(2.dp))
                Text(
                    card.subtitle,
                    fontSize = 12.sp,
                    color = c.onSurfaceMuted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                if (card.uncertain) {
                    // The repair affordance sits where the user notices the gap,
                    // not buried in settings. Tapping it opens the catch-up flow.
                    Spacer(Modifier.height(3.dp))
                    Text(
                        "not sure who shared this",
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Medium,
                        color = bandColor,
                    )
                }
            }
        }
    }
}
