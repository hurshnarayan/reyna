package app.reyna.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
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
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Sort
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
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
import app.reyna.search.SortMode
import app.reyna.ui.components.ConfidenceDot
import app.reyna.ui.components.FileThumbnail
import app.reyna.ui.components.FileTypeFilter
import app.reyna.ui.components.QuickLookModal
import app.reyna.ui.components.SortAndFilterDrawer
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors

/**
 * The file list, with search and sorting.
 *
 * Reyna's filenames are hostile to exact search: half are
 * `DOC-20260818-WA0041.pdf`, and the named ones get typed from memory months
 * later. So the box is fuzzy by default and shows what matched, which is what
 * makes a loose hit legible rather than mysterious.
 *
 * Files are sorted by captured latest by default, with additional modes for
 * date, name, kind, and size. Long-pressing any row triggers a macOS Quick Look
 * preview popup.
 */
@Composable
fun FilesScreen(
    files: List<SearchableFile>,
    searchContentIds: suspend (String) -> Set<Long> = { emptySet() },
    onOpen: (SearchableFile) -> Unit = {},
    onAskWhoShared: (SearchableFile) -> Unit = {},
    loadPreviewPath: suspend (SearchableFile) -> String? = { file ->
        file.path.takeIf { java.io.File(it).isFile }
    },
) {
    val c = reynaColors
    var query by remember { mutableStateOf("") }
    var indexedQuery by remember { mutableStateOf("") }
    var contentCandidateIds by remember { mutableStateOf<Set<Long>>(emptySet()) }
    var chatFilter by remember { mutableStateOf<String?>(null) }
    var sortMode by remember { mutableStateOf(SortMode.LATEST) }
    var sortAscending by remember { mutableStateOf(false) }
    var quickLookFile by remember { mutableStateOf<SearchableFile?>(null) }
    var lastQuickLookFile by remember { mutableStateOf<SearchableFile?>(null) }
    if (quickLookFile != null) {
        lastQuickLookFile = quickLookFile
    }
    var sortDrawerOpen by remember { mutableStateOf(false) }
    var typeFilter by remember { mutableStateOf(FileTypeFilter.ALL) }

    LaunchedEffect(query, files) {
        val requested = query.trim()
        if (requested.isEmpty()) {
            indexedQuery = ""
            contentCandidateIds = emptySet()
        } else {
            // Debounce typing; SQLite FTS is fast, but obsolete queries should
            // not compete with the one the person is still entering.
            delay(120)
            contentCandidateIds = runCatching { searchContentIds(requested) }.getOrDefault(emptySet())
            indexedQuery = requested
        }
    }

    val indexedCandidates = if (indexedQuery == query.trim()) contentCandidateIds else emptySet()
    val allHits = remember(query, indexedCandidates, files) {
        FileSearch.search(query, files, fuzzy = true, contentCandidateIds = indexedCandidates)
    }
    val typeFilteredHits = remember(allHits, typeFilter) {
        when (typeFilter) {
            FileTypeFilter.ALL -> allHits
            FileTypeFilter.DOCUMENTS -> allHits.filter { !it.file.isImage }
            FileTypeFilter.PHOTOS -> allHits.filter { it.file.isImage }
        }
    }
    val facets = remember(typeFilteredHits) { FileSearch.chatFacets(typeFilteredHits) }
    val filtered = remember(typeFilteredHits, chatFilter) {
        if (chatFilter == null) typeFilteredHits else typeFilteredHits.filter { it.file.chatName == chatFilter }
    }
    val hits = remember(filtered, sortMode, sortAscending, query) {
        // When searching with non-empty query and default LATEST descending,
        // search relevance score is preserved unless user picked a sort mode.
        if (query.isNotBlank() && sortMode == SortMode.LATEST && !sortAscending) {
            filtered
        } else {
            FileSearch.sortFiles(filtered, sortMode, sortAscending)
        }
    }

    // Only computed when a search fails, since it walks the whole vocabulary.
    val suggestions = remember(query, allHits) {
        if (query.isNotBlank() && allHits.isEmpty()) {
            FuzzySearch.suggestions(query.trim(), FileSearch.vocabulary(files))
        } else emptyList()
    }
    Box(Modifier.fillMaxSize().background(c.background)) {
        Column(Modifier.fillMaxSize()) {
            SearchBar(
                query = query,
                onQuery = { query = it; chatFilter = null },
            )

            Row(
                Modifier
                    .fillMaxWidth()
                    .padding(horizontal = Dimens.page, vertical = 3.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                val hasFilter = typeFilter != FileTypeFilter.ALL || sortMode != SortMode.LATEST || sortAscending
                val interactionSource = remember { MutableInteractionSource() }
                val isPressed by interactionSource.collectIsPressedAsState()
                val btnScale by animateFloatAsState(
                    targetValue = if (isPressed) 0.94f else 1.0f,
                    animationSpec = spring(dampingRatio = 0.75f, stiffness = 600f),
                    label = "sortFilterScale",
                )

                Box(
                    Modifier
                        .graphicsLayer {
                            scaleX = btnScale
                            scaleY = btnScale
                        }
                        .clip(RoundedCornerShape(8.dp))
                        .background(if (hasFilter) c.accent.copy(alpha = 0.12f) else c.surface)
                        .border(
                            1.dp,
                            if (hasFilter) c.accent else c.border,
                            RoundedCornerShape(8.dp),
                        )
                        .clickable(interactionSource = interactionSource, indication = null) {
                            sortDrawerOpen = true
                        }
                        .padding(horizontal = 10.dp, vertical = 6.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(
                            Icons.Rounded.Tune,
                            contentDescription = "Sort & Filter",
                            tint = if (hasFilter) c.accent else c.onSurface,
                            modifier = Modifier.size(15.dp),
                        )
                        Spacer(Modifier.width(5.dp))
                        Text(
                            text = if (typeFilter != FileTypeFilter.ALL) typeFilter.label else "Filter",
                            fontSize = 12.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = if (hasFilter) c.accent else c.onSurface,
                        )
                    }
                }

                SortRow(
                    currentMode = sortMode,
                    ascending = sortAscending,
                    onSelectMode = { mode ->
                        if (sortMode == mode) {
                            sortAscending = !sortAscending
                        } else {
                            sortMode = mode
                            sortAscending = (mode == SortMode.NAME)
                        }
                    },
                    modifier = Modifier.weight(1f),
                )
            }

            if (query.isNotBlank() && allHits.isNotEmpty()) {
                Text(
                    "Showing ${hits.size} of ${files.size} files",
                    fontSize = 12.sp,
                    color = c.onSurfaceMuted,
                    modifier = Modifier.padding(horizontal = Dimens.page, vertical = 6.dp),
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
                    onSuggestion = { query = it },
                )
            } else {
                LazyColumn(Modifier.fillMaxSize()) {
                    items(hits.size, key = { hits[it].file.id }) { i ->
                        ResultRow(
                            hit = hits[i],
                            onOpen = onOpen,
                            onHoldStart = { quickLookFile = it },
                            onHoldEnd = { quickLookFile = null },
                            onAskWhoShared = onAskWhoShared,
                            loadPreviewPath = loadPreviewPath,
                        )
                    }
                    item { Spacer(Modifier.height(16.dp)) }
                }
            }
        }

        AnimatedVisibility(
            visible = quickLookFile != null,
            enter = fadeIn(tween(120)) + scaleIn(tween(120), initialScale = 0.95f),
            exit = fadeOut(tween(80)) + scaleOut(tween(80), targetScale = 0.95f),
        ) {
            lastQuickLookFile?.let { file ->
                QuickLookModal(
                    file = file,
                    loadPreviewPath = loadPreviewPath,
                    onDismiss = { quickLookFile = null },
                    onOpen = { onOpen(file) },
                    onAskWhoShared = { onAskWhoShared(file) },
                )
            }
        }

        SortAndFilterDrawer(
            visible = sortDrawerOpen,
            currentSort = sortMode,
            ascending = sortAscending,
            currentType = typeFilter,
            totalFiles = files.size,
            onSelectSort = { mode, asc ->
                sortMode = mode
                sortAscending = asc
            },
            onSelectType = { type ->
                typeFilter = type
            },
            onDismiss = { sortDrawerOpen = false },
        )
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
    onQuery: (String) -> Unit,
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
                Text("Search names or text inside files", fontSize = 15.sp, color = c.onSurfaceMuted)
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
private fun SortRow(
    currentMode: SortMode,
    ascending: Boolean,
    onSelectMode: (SortMode) -> Unit,
    modifier: Modifier = Modifier,
) {
    val c = reynaColors
    Row(
        modifier
            .horizontalScroll(rememberScrollState())
            .padding(vertical = 3.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            Icons.Rounded.Sort,
            contentDescription = "Sort",
            tint = c.onSurfaceMuted,
            modifier = Modifier.size(15.dp),
        )
        SortMode.entries.forEach { mode ->
            val isActive = currentMode == mode
            val arrow = if (isActive) {
                if (mode == SortMode.NAME) {
                    if (ascending) " A→Z" else " Z→A"
                } else {
                    if (ascending) " ↑" else " ↓"
                }
            } else ""

            Box(
                Modifier
                    .clip(RoundedCornerShape(8.dp))
                    .background(if (isActive) c.accent.copy(alpha = 0.16f) else c.surface)
                    .border(
                        1.dp,
                        if (isActive) c.accent else c.border,
                        RoundedCornerShape(8.dp),
                    )
                    .clickable { onSelectMode(mode) }
                    .padding(horizontal = 9.dp, vertical = 5.dp),
            ) {
                Text(
                    text = "${mode.label}$arrow",
                    fontSize = 12.sp,
                    fontWeight = if (isActive) FontWeight.SemiBold else FontWeight.Normal,
                    color = if (isActive) c.accent else c.onSurfaceMuted,
                )
            }
        }
    }
}

@Composable
private fun ResultRow(
    hit: FileHit,
    onOpen: (SearchableFile) -> Unit,
    onHoldStart: (SearchableFile) -> Unit,
    onHoldEnd: () -> Unit,
    onAskWhoShared: (SearchableFile) -> Unit,
    loadPreviewPath: suspend (SearchableFile) -> String?,
) {
    val c = reynaColors
    val f = hit.file
    val haptic = LocalHapticFeedback.current
    val scope = rememberCoroutineScope()
    var isPressed by remember { mutableStateOf(false) }

    Row(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = Dimens.page, vertical = 4.dp)
            .clip(RoundedCornerShape(11.dp))
            .background(if (isPressed) c.bubbleIncoming else c.surface)
            .border(1.dp, c.border, RoundedCornerShape(11.dp))
            .pointerInput(f) {
                var isHeld = false
                detectTapGestures(
                    onTap = {
                        if (!isHeld) {
                            onOpen(f)
                        }
                    },
                    onLongPress = {
                        // Consumes Compose's long press to ensure onTap does not fire
                    },
                    onPress = {
                        isHeld = false
                        isPressed = true
                        val holdJob = scope.launch {
                            delay(220)
                            isHeld = true
                            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                            onHoldStart(f)
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
            .padding(horizontal = 11.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        FileThumbnail(
            file = f,
            modifier = Modifier.size(44.dp),
            cornerRadius = 8.dp,
            loadPreviewPath = loadPreviewPath,
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
                    Attribution.describe(f.confidence, f.senderName, f.chatName, f.whenText),
                    fontSize = 12.sp,
                    color = c.onSurfaceMuted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                if (f.formattedSize.isNotBlank()) {
                    Text(
                        " · ${f.formattedSize}",
                        fontSize = 12.sp,
                        color = c.onSurfaceFaint,
                        maxLines = 1,
                    )
                }
            }
            hit.reason?.let { reason ->
                Spacer(Modifier.height(5.dp))
                Text(
                    reason.label,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Medium,
                    color = c.accent,
                )
            }
            hit.snippet?.let { snippet ->
                Spacer(Modifier.height(2.dp))
                Text(
                    snippet,
                    fontSize = 11.sp,
                    lineHeight = 15.sp,
                    color = c.onSurfaceMuted,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            if (f.confidence < Attribution.MIN_NAMED) {
                Spacer(Modifier.height(6.dp))
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
    onSuggestion: (String) -> Unit,
) {
    val c = reynaColors
    Column(
        Modifier.fillMaxWidth().padding(horizontal = Dimens.page, vertical = 28.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Nothing matched \"$query\"", fontSize = 15.sp, color = c.onSurface)
        if (suggestions.isNotEmpty()) {
            Spacer(Modifier.height(14.dp))
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(7.dp),
            ) {
                Text("Did you mean", fontSize = 13.sp, color = c.onSurfaceMuted)
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
