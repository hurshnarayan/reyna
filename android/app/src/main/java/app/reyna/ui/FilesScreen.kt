package app.reyna.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material3.Icon
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
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.attribution.Attribution
import app.reyna.search.FileHit
import app.reyna.search.FileSearch
import app.reyna.search.FuzzySearch
import app.reyna.search.SearchableFile
import app.reyna.ui.components.ConfidenceDot
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

/**
 * The file list, with search.
 *
 * Reyna's filenames are hostile to exact search: half are
 * `DOC-20260818-WA0041.pdf`, and the named ones get typed from memory months
 * later. So the box is fuzzy by default and shows what matched, which is what
 * makes a loose hit legible rather than mysterious.
 */
@Composable
fun FilesScreen(
    files: List<SearchableFile>,
    onOpen: (SearchableFile) -> Unit = {},
    onAskWhoShared: (SearchableFile) -> Unit = {},
) {
    val c = reynaColors
    var query by remember { mutableStateOf("") }
    var fuzzy by remember { mutableStateOf(true) }
    var chatFilter by remember { mutableStateOf<String?>(null) }

    val allHits = remember(query, fuzzy, files) { FileSearch.search(query, files, fuzzy) }
    val facets = remember(allHits) { FileSearch.chatFacets(allHits) }
    val hits = remember(allHits, chatFilter) {
        if (chatFilter == null) allHits else allHits.filter { it.file.chatName == chatFilter }
    }

    // Only computed when a search fails, since it walks the whole vocabulary.
    val suggestions = remember(query, allHits) {
        if (query.isNotBlank() && allHits.isEmpty()) {
            FuzzySearch.suggestions(query.trim(), FileSearch.vocabulary(files))
        } else emptyList()
    }
    // A strict search that finds nothing is usually a spelling the user is sure
    // about, so offering fuzzy is more useful than offering corrections.
    val fuzzyWouldHelp = remember(query, fuzzy, allHits) {
        !fuzzy && allHits.isEmpty() && query.isNotBlank() &&
            FileSearch.search(query, files, fuzzy = true).isNotEmpty()
    }

    Column(Modifier.fillMaxSize().background(c.background)) {
        SearchBar(
            query = query,
            fuzzy = fuzzy,
            onQuery = { query = it; chatFilter = null },
            onToggleFuzzy = { fuzzy = !fuzzy },
        )

        if (query.isNotBlank() && allHits.isNotEmpty()) {
            Text(
                "Showing ${hits.size} of ${files.size} files",
                fontSize = 12.sp,
                color = c.onSurfaceMuted,
                modifier = Modifier.padding(horizontal = Dimens.page, vertical = 8.dp),
            )
        }

        if (facets.size > 1) {
            FacetRow(
                facets = facets,
                total = allHits.size,
                selected = chatFilter,
                onSelect = { chatFilter = it },
            )
        }

        if (allHits.isEmpty() && query.isNotBlank()) {
            EmptyResults(
                query = query.trim(),
                suggestions = suggestions,
                offerFuzzy = fuzzyWouldHelp,
                onTryFuzzy = { fuzzy = true },
                onSuggestion = { query = it },
            )
        } else {
            LazyColumn(Modifier.fillMaxSize()) {
                items(hits.size) { i -> ResultRow(hits[i], onOpen, onAskWhoShared) }
                item { Spacer(Modifier.height(16.dp)) }
            }
        }
    }
}

/**
 * The search box.
 *
 * The fuzzy toggle is a visible control rather than a hidden default, because
 * loose matching is occasionally wrong in ways the user can see, and being able
 * to tighten it is the fix.
 */
@Composable
private fun SearchBar(
    query: String,
    fuzzy: Boolean,
    onQuery: (String) -> Unit,
    onToggleFuzzy: () -> Unit,
) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = Dimens.page, vertical = 10.dp)
            .clip(RoundedCornerShape(11.dp))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(11.dp))
            .padding(horizontal = 12.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Rounded.Search, null, tint = c.onSurfaceMuted, modifier = Modifier.size(19.dp))
        Spacer(Modifier.width(10.dp))
        Box(Modifier.weight(1f)) {
            if (query.isEmpty()) {
                Text("Search files, people, chats", fontSize = 15.sp, color = c.onSurfaceMuted)
            }
            BasicTextField(
                value = query,
                onValueChange = onQuery,
                singleLine = true,
                textStyle = TextStyle(fontSize = 15.sp, color = c.onSurface),
                cursorBrush = SolidColor(c.accent),
                modifier = Modifier.fillMaxWidth(),
            )
        }
        Spacer(Modifier.width(6.dp))
        // Fuzzy matching, named rather than symbolised. A bold tilde in a
        // tinted circle told nobody what it did, and it was the loudest thing
        // on a screen whose job is to get out of the way of the filenames.
        Box(
            Modifier
                .clip(RoundedCornerShape(7.dp))
                .clickable { onToggleFuzzy() }
                .padding(horizontal = 8.dp, vertical = 3.dp),
        ) {
            Text(
                "Fuzzy",
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
                color = if (fuzzy) c.accent else c.onSurfaceFaint,
            )
        }
        if (query.isNotEmpty()) {
            Spacer(Modifier.width(2.dp))
            Box(
                Modifier.size(28.dp).clip(CircleShape).clickable { onQuery("") },
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.Rounded.Close, "Clear", tint = c.onSurfaceMuted, modifier = Modifier.size(17.dp))
            }
        }
    }
}

/**
 * Chat filter chips.
 *
 * Counts come from the current results, not the whole library. A chip promising
 * twelve that then shows three is worse than no chip at all.
 */
@Composable
private fun FacetRow(
    facets: List<Pair<String, Int>>,
    total: Int,
    selected: String?,
    onSelect: (String?) -> Unit,
) {
    val c = reynaColors
    Row(
        Modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState())
            .padding(horizontal = Dimens.page, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(7.dp),
    ) {
        Chip("All", total, selected == null) { onSelect(null) }
        facets.forEach { (name, count) ->
            Chip(name, count, selected == name) { onSelect(name) }
        }
    }
}

@Composable
private fun Chip(label: String, count: Int, active: Boolean, onClick: () -> Unit) {
    val c = reynaColors
    Row(
        Modifier
            .clip(CircleShape)
            .background(if (active) c.bubbleOutgoing else c.bubbleIncoming)
            .clickable { onClick() }
            .padding(horizontal = 12.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            label,
            fontSize = 13.sp,
            fontWeight = FontWeight.Medium,
            color = if (active) c.onBubbleOutgoing else c.onSurface,
        )
        Spacer(Modifier.width(5.dp))
        Text(
            "$count",
            fontSize = 12.sp,
            color = if (active) c.onBubbleOutgoing.copy(alpha = 0.75f) else c.onSurfaceMuted,
        )
    }
}

@Composable
private fun ResultRow(
    hit: FileHit,
    onOpen: (SearchableFile) -> Unit,
    onAskWhoShared: (SearchableFile) -> Unit,
) {
    val c = reynaColors
    val f = hit.file
    Row(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = Dimens.page, vertical = 4.dp)
            .clip(RoundedCornerShape(11.dp))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(11.dp))
            .clickable { onOpen(f) }
            .padding(horizontal = 12.dp, vertical = 11.dp),
        verticalAlignment = Alignment.Top,
    ) {
        Icon(
            if (f.isImage) Icons.Rounded.Image else Icons.Rounded.Description,
            null,
            tint = c.onSurfaceFaint,
            modifier = Modifier.size(19.dp).padding(top = 1.dp),
        )
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(
                highlight(f.fileName, hit.nameSpans, c.accent),
                fontSize = 14.sp,
                fontWeight = FontWeight.Medium,
                color = c.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(3.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                ConfidenceDot(f.confidence)
                Spacer(Modifier.width(6.dp))
                Text(
                    // The one place the attribution line comes from, so a result
                    // can never print a name the confidence does not support.
                    Attribution.describe(f.confidence, f.senderName, f.chatName, f.whenText),
                    fontSize = 12.sp,
                    color = c.onSurfaceMuted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            if (f.confidence < Attribution.MIN_NAMED) {
                // The repair affordance sits where the gap is noticed.
                Spacer(Modifier.height(8.dp))
                Box(
                    Modifier
                        .clip(RoundedCornerShape(7.dp))
                        .border(1.dp, c.border, RoundedCornerShape(7.dp))
                        .clickable { onAskWhoShared(f) }
                        .padding(horizontal = 9.dp, vertical = 4.dp),
                ) {
                    Text(
                        "Who shared this?",
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Medium,
                        color = c.onSurfaceMuted,
                    )
                }
            }
        }
    }
}

/**
 * Marks the characters that matched.
 *
 * Without this a fuzzy hit looks arbitrary: the user sees a file they did not
 * type the name of and cannot tell why it is there.
 */
private fun highlight(text: String, spans: List<Int>, color: Color): AnnotatedString {
    if (spans.isEmpty()) return AnnotatedString(text)
    val set = spans.toHashSet()
    return buildAnnotatedString {
        text.forEachIndexed { i, ch ->
            if (i in set) {
                withStyle(SpanStyle(color = color, fontWeight = FontWeight.Bold)) { append(ch) }
            } else {
                append(ch)
            }
        }
    }
}

/**
 * What to show when nothing matched.
 *
 * Never a dead end. Either loosening the match will help, or some word the user
 * actually has is one typo away, and both are one tap.
 */
@Composable
private fun EmptyResults(
    query: String,
    suggestions: List<String>,
    offerFuzzy: Boolean,
    onTryFuzzy: () -> Unit,
    onSuggestion: (String) -> Unit,
) {
    val c = reynaColors
    Column(
        Modifier.fillMaxWidth().padding(horizontal = Dimens.page, vertical = 28.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("No results for \"$query\"", fontSize = 15.sp, color = c.onSurface)
        if (offerFuzzy || suggestions.isNotEmpty()) {
            Spacer(Modifier.height(14.dp))
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(7.dp),
            ) {
                if (offerFuzzy) {
                    Box(
                        Modifier
                            .clip(CircleShape)
                            .background(c.bubbleOutgoing.copy(alpha = 0.14f))
                            .clickable { onTryFuzzy() }
                            .padding(horizontal = 12.dp, vertical = 7.dp),
                    ) {
                        Text(
                            "Try fuzzy search",
                            fontSize = 13.sp,
                            fontWeight = FontWeight.Medium,
                            color = c.bubbleOutgoing,
                        )
                    }
                }
                if (suggestions.isNotEmpty()) {
                    Text("Did you mean", fontSize = 13.sp, color = c.onSurfaceMuted)
                }
            }
            if (suggestions.isNotEmpty()) {
                Spacer(Modifier.height(9.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(7.dp)) {
                    suggestions.forEach { s ->
                        Box(
                            Modifier
                                .clip(CircleShape)
                                .background(c.bubbleIncoming)
                                .clickable { onSuggestion(s) }
                                .padding(horizontal = 12.dp, vertical = 7.dp),
                        ) {
                            Text(s, fontSize = 13.sp, color = c.onSurface)
                        }
                    }
                }
            }
        }
    }
}
