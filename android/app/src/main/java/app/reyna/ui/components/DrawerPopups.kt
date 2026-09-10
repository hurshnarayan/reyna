package app.reyna.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.FastOutLinearInEasing
import androidx.compose.animation.core.LinearOutSlowInEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors
import kotlinx.coroutines.delay

/**
 * High-performance fluid spring specs measured from the sneaker drawer GIF.
 * Duration ~240ms - 270ms with gentle settle.
 */
private val DrawerSpringEnter = spring<androidx.compose.ui.unit.IntOffset>(
    dampingRatio = 0.82f,
    stiffness = 400f,
)

private val DrawerSpringExit = spring<androidx.compose.ui.unit.IntOffset>(
    dampingRatio = 0.90f,
    stiffness = 480f,
)

private val TopBannerSpringEnter = spring<androidx.compose.ui.unit.IntOffset>(
    dampingRatio = 0.78f,
    stiffness = 380f,
)

private val TopBannerSpringExit = spring<androidx.compose.ui.unit.IntOffset>(
    dampingRatio = 0.88f,
    stiffness = 450f,
)

/**
 * Sneaker-style bottom drawer / modal sheet.
 *
 * Slides up smoothly from the bottom over a dimmed scrim backdrop.
 * Features rounded top corners (24dp), drag handle, centered title,
 * custom content, and a centered "Cancel" button at the bottom.
 */
@Composable
fun FluidBottomDrawer(
    visible: Boolean,
    onDismiss: () -> Unit,
    title: String? = null,
    showDragHandle: Boolean = true,
    cancelText: String = "Cancel",
    content: @Composable ColumnScope.() -> Unit,
) {
    val c = reynaColors

    if (visible) {
        BackHandler(onBack = onDismiss)
    }

    val scrimAlpha by animateFloatAsState(
        targetValue = if (visible) 0.52f else 0f,
        animationSpec = tween(durationMillis = 240, easing = LinearOutSlowInEasing),
        label = "drawerScrimAlpha",
    )

    if (visible || scrimAlpha > 0.005f) {
        Box(Modifier.fillMaxSize()) {
            // Scrim backdrop
            if (scrimAlpha > 0.01f) {
                Box(
                    Modifier
                        .fillMaxSize()
                        .background(Color.Black.copy(alpha = scrimAlpha))
                        .clickable(
                            interactionSource = remember { MutableInteractionSource() },
                            indication = null,
                            onClick = onDismiss,
                        )
                )
            }

            // Animated Drawer Container
            AnimatedVisibility(
                visible = visible,
                modifier = Modifier.align(Alignment.BottomCenter),
                enter = slideInVertically(initialOffsetY = { it }, animationSpec = DrawerSpringEnter) +
                    fadeIn(animationSpec = tween(durationMillis = 180, easing = LinearOutSlowInEasing)),
                exit = slideOutVertically(targetOffsetY = { it }, animationSpec = DrawerSpringExit) +
                    fadeOut(animationSpec = tween(durationMillis = 160, easing = FastOutLinearInEasing)),
            ) {
            Column(
                Modifier
                    .fillMaxWidth()
                    .shadow(elevation = 16.dp, shape = RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp))
                    .clip(RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp))
                    .background(c.surface)
                    .border(1.dp, c.border, RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp))
                    .navigationBarsPadding()
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null,
                        onClick = { /* prevent tap through */ }
                    ),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                if (showDragHandle) {
                    Spacer(Modifier.height(10.dp))
                    Box(
                        Modifier
                            .width(36.dp)
                            .height(4.dp)
                            .clip(CircleShape)
                            .background(c.onSurfaceFaint.copy(alpha = 0.35f))
                    )
                    Spacer(Modifier.height(12.dp))
                } else {
                    Spacer(Modifier.height(18.dp))
                }

                if (!title.isNullOrBlank()) {
                    Text(
                        text = title,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = c.onSurface,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.padding(horizontal = Dimens.page),
                    )
                    Spacer(Modifier.height(16.dp))
                }

                // Child content with standard horizontal padding
                content()

                // Cancel footer button
                Spacer(Modifier.height(14.dp))
                Box(
                    Modifier
                        .fillMaxWidth()
                        .clickable { onDismiss() }
                        .padding(vertical = 14.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        text = cancelText,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Medium,
                        color = c.onSurfaceMuted,
                    )
                }
                Spacer(Modifier.height(4.dp))
            }
        }
    }
}
}

/**
 * Top notification drawer / popup (as seen in the sneaker GIF's "Please select a size").
 *
 * Slides down smoothly below the status bar with a soft elevation shadow,
 * colored status dot, bold title, descriptive subtitle, and close button.
 */
@Composable
fun FluidTopNotification(
    visible: Boolean,
    title: String,
    message: String,
    dotColor: Color = Color(0xFFFF5A5F),
    autoDismissMs: Long = 3500L,
    onDismiss: () -> Unit,
) {
    val c = reynaColors

    var lastTitle by remember { mutableStateOf(title) }
    var lastMessage by remember { mutableStateOf(message) }
    var lastColor by remember { mutableStateOf(dotColor) }

    if (visible && title.isNotEmpty()) {
        lastTitle = title
        lastMessage = message
        lastColor = dotColor
    }

    val displayTitle = if (visible) title else lastTitle
    val displayMessage = if (visible) message else lastMessage
    val displayColor = if (visible) dotColor else lastColor

    LaunchedEffect(visible, autoDismissMs) {
        if (visible && autoDismissMs > 0L) {
            delay(autoDismissMs)
            onDismiss()
        }
    }

    val scrimAlpha by animateFloatAsState(
        targetValue = if (visible) 0.18f else 0f,
        animationSpec = tween(durationMillis = 200),
        label = "topNotificationScrim",
    )

    if (visible || scrimAlpha > 0.005f) {
        Box(Modifier.fillMaxSize()) {
            if (scrimAlpha > 0.01f) {
                Box(
                    Modifier
                        .fillMaxSize()
                        .background(Color.Black.copy(alpha = scrimAlpha))
                        .clickable(
                            interactionSource = remember { MutableInteractionSource() },
                            indication = null,
                            onClick = onDismiss,
                        )
                )
            }

            AnimatedVisibility(
                visible = visible,
                modifier = Modifier
                    .align(Alignment.TopCenter)
                    .statusBarsPadding()
                    .padding(top = 6.dp),
                enter = slideInVertically(initialOffsetY = { -it }, animationSpec = TopBannerSpringEnter) +
                    fadeIn(animationSpec = tween(160)),
                exit = slideOutVertically(targetOffsetY = { -it }, animationSpec = TopBannerSpringExit) +
                    fadeOut(animationSpec = tween(140)),
            ) {
                Box(
                    Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 14.dp)
                        .shadow(elevation = 12.dp, shape = RoundedCornerShape(20.dp))
                        .clip(RoundedCornerShape(20.dp))
                        .background(c.surface)
                        .border(1.dp, c.border, RoundedCornerShape(20.dp))
                        .clickable(
                            interactionSource = remember { MutableInteractionSource() },
                            indication = null,
                            onClick = onDismiss,
                        )
                        .padding(horizontal = 16.dp, vertical = 14.dp)
                ) {
                    Row(
                        Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.Top,
                    ) {
                        // Status Dot Indicator (matching sneaker GIF frames 62-71)
                        Box(
                            Modifier
                                .padding(top = 5.dp)
                                .size(9.dp)
                                .clip(CircleShape)
                                .background(displayColor)
                        )
                        Spacer(Modifier.width(12.dp))
                        Column(Modifier.weight(1f)) {
                            Text(
                                text = displayTitle,
                                fontSize = 15.sp,
                                fontWeight = FontWeight.Bold,
                                color = c.onSurface,
                                lineHeight = 19.sp,
                            )
                            if (displayMessage.isNotBlank()) {
                                Spacer(Modifier.height(3.dp))
                                Text(
                                    text = displayMessage,
                                    fontSize = 12.5.sp,
                                    color = c.onSurfaceMuted,
                                    lineHeight = 16.sp,
                                )
                            }
                        }
                        Spacer(Modifier.width(8.dp))
                        Box(
                            Modifier
                                .size(26.dp)
                                .clip(CircleShape)
                                .clickable { onDismiss() },
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(
                                Icons.Rounded.Close,
                                contentDescription = "Dismiss",
                                tint = c.onSurfaceMuted,
                                modifier = Modifier.size(17.dp),
                            )
                        }
                    }
                }
            }
        }
    }
}

/**
 * Dual action pill buttons styled after the sneaker action drawers
 * (e.g. Coral "Sell now for $425" & Black "Place Ask").
 */
@Composable
fun SneakerDualActionRow(
    leftTitle: String,
    leftSub: String,
    leftColor: Color = Color(0xFFFF5A5F),
    leftOnColor: Color = Color.White,
    onLeftClick: () -> Unit,
    rightTitle: String,
    rightSub: String,
    rightColor: Color = Color(0xFF111111),
    rightOnColor: Color = Color.White,
    onRightClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = Dimens.page),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        SneakerActionButton(
            title = leftTitle,
            sub = leftSub,
            backgroundColor = leftColor,
            textColor = leftOnColor,
            modifier = Modifier.weight(1f),
            onClick = onLeftClick,
        )
        SneakerActionButton(
            title = rightTitle,
            sub = rightSub,
            backgroundColor = rightColor,
            textColor = rightOnColor,
            modifier = Modifier.weight(1f),
            onClick = onRightClick,
        )
    }
}

@Composable
fun SneakerActionButton(
    title: String,
    sub: String,
    backgroundColor: Color,
    textColor: Color,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val interactionSource = remember { MutableInteractionSource() }
    val isPressed by interactionSource.collectIsPressedAsState()
    val scale by animateFloatAsState(
        targetValue = if (isPressed) 0.96f else 1.0f,
        animationSpec = spring(dampingRatio = 0.75f, stiffness = 600f),
        label = "btnScale",
    )

    Box(
        modifier = modifier
            .graphicsLayer {
                scaleX = scale
                scaleY = scale
            }
            .height(52.dp)
            .clip(RoundedCornerShape(11.dp))
            .background(backgroundColor)
            .clickable(interactionSource = interactionSource, indication = null, onClick = onClick)
            .padding(horizontal = 10.dp, vertical = 6.dp),
        contentAlignment = Alignment.Center,
    ) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(
                text = title,
                fontSize = 14.sp,
                fontWeight = FontWeight.Bold,
                color = textColor,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (sub.isNotBlank()) {
                Text(
                    text = sub,
                    fontSize = 11.5.sp,
                    fontWeight = FontWeight.Medium,
                    color = textColor.copy(alpha = 0.9f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/**
 * 3-column / 2-row selection grid matrix as seen in the sneaker size picker.
 * Features subtle grid borders and a clean active indicator underline.
 */
data class MatrixOption(
    val id: String,
    val title: String,
    val subtitle: String = "",
)

@Composable
fun SneakerGridMatrix(
    options: List<MatrixOption>,
    selectedId: String,
    onSelect: (String) -> Unit,
    columns: Int = 3,
    modifier: Modifier = Modifier,
) {
    val c = reynaColors
    val rows = options.chunked(columns)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = Dimens.page)
            .clip(RoundedCornerShape(12.dp))
            .border(0.8.dp, c.border, RoundedCornerShape(12.dp))
            .background(c.surface)
    ) {
        rows.forEachIndexed { rowIndex, rowOptions ->
            Row(Modifier.fillMaxWidth()) {
                rowOptions.forEachIndexed { colIndex, opt ->
                    val isSelected = opt.id == selectedId
                    val interactionSource = remember { MutableInteractionSource() }
                    val isPressed by interactionSource.collectIsPressedAsState()
                    val scale by animateFloatAsState(
                        targetValue = if (isPressed) 0.96f else 1.0f,
                        label = "gridItemScale"
                    )

                    Box(
                        Modifier
                            .weight(1f)
                            .height(56.dp)
                            .graphicsLayer {
                                scaleX = scale
                                scaleY = scale
                            }
                            .background(
                                if (isSelected) c.accent.copy(alpha = 0.08f) else Color.Transparent
                            )
                            .border(0.4.dp, c.border)
                            .clickable(interactionSource = interactionSource, indication = null) {
                                onSelect(opt.id)
                            },
                        contentAlignment = Alignment.Center,
                    ) {
                        Column(
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.Center,
                            modifier = Modifier.padding(horizontal = 4.dp),
                        ) {
                            Text(
                                text = opt.title,
                                fontSize = 13.5.sp,
                                fontWeight = if (isSelected) FontWeight.Bold else FontWeight.Medium,
                                color = if (isSelected) c.accent else c.onSurface,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                            if (opt.subtitle.isNotBlank()) {
                                Spacer(Modifier.height(1.dp))
                                Text(
                                    text = opt.subtitle,
                                    fontSize = 11.sp,
                                    color = if (isSelected) c.accent.copy(alpha = 0.85f) else c.onSurfaceMuted,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
                            }
                        }

                        // Bottom active underline indicator (matching the sneaker gif)
                        if (isSelected) {
                            Box(
                                Modifier
                                    .align(Alignment.BottomCenter)
                                    .fillMaxWidth(0.7f)
                                    .height(2.5.dp)
                                    .clip(RoundedCornerShape(topStart = 2.dp, topEnd = 2.dp))
                                    .background(c.accent)
                            )
                        }
                    }
                }
                // Fill remaining empty cells in incomplete last row
                if (rowOptions.size < columns) {
                    repeat(columns - rowOptions.size) {
                        Spacer(Modifier.weight(1f).height(56.dp).border(0.4.dp, c.border))
                    }
                }
            }
        }
    }
}

enum class FileTypeFilter(val label: String, val subtitle: String) {
    ALL("All Files", "Everything"),
    DOCUMENTS("Documents", "PDF, Docs"),
    PHOTOS("Photos", "Images"),
}

/**
 * Sneaker-style Sort & Filter Drawer.
 * Slides up smoothly to present sorting modes in a 3-column matrix and
 * file type filters in a secondary row with active indicator lines.
 */
@Composable
fun SortAndFilterDrawer(
    visible: Boolean,
    currentSort: app.reyna.search.SortMode,
    ascending: Boolean,
    currentType: FileTypeFilter,
    totalFiles: Int,
    onSelectSort: (app.reyna.search.SortMode, Boolean) -> Unit,
    onSelectType: (FileTypeFilter) -> Unit,
    onDismiss: () -> Unit,
) {
    val c = reynaColors

    FluidBottomDrawer(
        visible = visible,
        title = "Sort & Filter Files",
        onDismiss = onDismiss,
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = Dimens.page),
        ) {
            Text(
                text = "SORT BY",
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold,
                color = c.onSurfaceMuted,
                letterSpacing = 0.5.sp,
            )
        }
        Spacer(Modifier.height(8.dp))

        val sortId = when (currentSort) {
            app.reyna.search.SortMode.LATEST -> if (ascending) "oldest" else "latest"
            app.reyna.search.SortMode.DATE -> if (ascending) "oldest" else "latest"
            app.reyna.search.SortMode.NAME -> if (ascending) "name_az" else "name_za"
            app.reyna.search.SortMode.KIND -> "kind"
            app.reyna.search.SortMode.SIZE -> if (ascending) "size_asc" else "size_desc"
        }

        SneakerGridMatrix(
            options = listOf(
                MatrixOption("latest", "Newest", "Latest first"),
                MatrixOption("oldest", "Oldest", "Earliest first"),
                MatrixOption("name_az", "Name A→Z", "A to Z"),
                MatrixOption("name_za", "Name Z→A", "Z to A"),
                MatrixOption("kind", "Type", "Docs & photos"),
                MatrixOption("size_desc", "Size", "Largest first"),
            ),
            selectedId = sortId,
            columns = 3,
            onSelect = { id ->
                when (id) {
                    "latest" -> onSelectSort(app.reyna.search.SortMode.LATEST, false)
                    "oldest" -> onSelectSort(app.reyna.search.SortMode.LATEST, true)
                    "name_az" -> onSelectSort(app.reyna.search.SortMode.NAME, true)
                    "name_za" -> onSelectSort(app.reyna.search.SortMode.NAME, false)
                    "kind" -> onSelectSort(app.reyna.search.SortMode.KIND, false)
                    "size_desc" -> onSelectSort(app.reyna.search.SortMode.SIZE, false)
                }
            },
        )

        Spacer(Modifier.height(18.dp))
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = Dimens.page),
        ) {
            Text(
                text = "FILE TYPE",
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold,
                color = c.onSurfaceMuted,
                letterSpacing = 0.5.sp,
            )
        }
        Spacer(Modifier.height(8.dp))

        val typeId = when (currentType) {
            FileTypeFilter.ALL -> "all"
            FileTypeFilter.DOCUMENTS -> "docs"
            FileTypeFilter.PHOTOS -> "photos"
        }

        SneakerGridMatrix(
            options = listOf(
                MatrixOption("all", "All Files", "$totalFiles files"),
                MatrixOption("docs", "Documents", "PDF, Word"),
                MatrixOption("photos", "Photos", "Images & Scans"),
            ),
            selectedId = typeId,
            columns = 3,
            onSelect = { id ->
                when (id) {
                    "all" -> onSelectType(FileTypeFilter.ALL)
                    "docs" -> onSelectType(FileTypeFilter.DOCUMENTS)
                    "photos" -> onSelectType(FileTypeFilter.PHOTOS)
                }
            },
        )
    }
}

/**
 * Sneaker-style attribution repair bottom drawer ("Who shared this?").
 * Replaces the full screen route with a fluid, seamless bottom sheet.
 */
@Composable
fun WhoSharedThisDrawer(
    file: app.reyna.search.SearchableFile?,
    knownSenders: List<String>,
    onAssignSender: (fileId: Long, sender: String) -> Unit,
    onImportExport: () -> Unit,
    onDismiss: () -> Unit,
) {
    val c = reynaColors
    var customSender by remember(file?.id) { mutableStateOf("") }
    var lastFile by remember { mutableStateOf<app.reyna.search.SearchableFile?>(null) }
    if (file != null) {
        lastFile = file
    }
    val currentFile = file ?: lastFile

    FluidBottomDrawer(
        visible = file != null,
        title = "Who shared this?",
        onDismiss = onDismiss,
    ) {
        if (currentFile != null) {
            Column(
                Modifier
                    .fillMaxWidth()
                    .padding(horizontal = Dimens.page),
            ) {
                // File summary card
                Row(
                    Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(c.bubbleIncoming)
                        .padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(
                        Modifier
                            .size(38.dp)
                            .clip(CircleShape)
                            .background(c.surface),
                        contentAlignment = Alignment.Center,
                    ) {
                        Icon(
                            if (currentFile.isImage) Icons.Rounded.Image
                            else Icons.Rounded.Description,
                            contentDescription = null,
                            tint = c.accent,
                            modifier = Modifier.size(20.dp),
                        )
                    }
                    Spacer(Modifier.width(12.dp))
                    Column(Modifier.weight(1f)) {
                        Text(
                            currentFile.fileName,
                            fontSize = 14.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = c.onSurface,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        Spacer(Modifier.height(2.dp))
                        Text(
                            "Found on phone · ${currentFile.whenText}",
                            fontSize = 12.sp,
                            color = c.onSurfaceMuted,
                        )
                    }
                }

                if (knownSenders.isNotEmpty()) {
                    Spacer(Modifier.height(16.dp))
                    Text(
                        text = "SUGGESTED CONTACTS",
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Bold,
                        color = c.onSurfaceMuted,
                        letterSpacing = 0.5.sp,
                    )
                    Spacer(Modifier.height(8.dp))

                    Row(
                        Modifier
                            .fillMaxWidth()
                            .horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        knownSenders.take(5).forEach { name ->
                            Box(
                                Modifier
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(c.surface)
                                    .border(1.dp, c.border, RoundedCornerShape(8.dp))
                                    .clickable {
                                        onAssignSender(currentFile.id, name)
                                        onDismiss()
                                    }
                                    .padding(horizontal = 12.dp, vertical = 8.dp),
                            ) {
                                Text(
                                    name,
                                    fontSize = 13.sp,
                                    fontWeight = FontWeight.Medium,
                                    color = c.onSurface,
                                )
                            }
                        }
                    }
                }

                Spacer(Modifier.height(16.dp))
                Text(
                    text = "OR ENTER SENDER NAME",
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Bold,
                    color = c.onSurfaceMuted,
                    letterSpacing = 0.5.sp,
                )
                Spacer(Modifier.height(8.dp))

                Row(
                    Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(
                        Modifier
                            .weight(1f)
                            .height(46.dp)
                            .clip(RoundedCornerShape(10.dp))
                            .background(c.surface)
                            .border(1.dp, c.border, RoundedCornerShape(10.dp))
                            .padding(horizontal = 12.dp, vertical = 12.dp),
                    ) {
                        if (customSender.isEmpty()) {
                            Text(
                                "e.g. Yash Bmsit, Priya...",
                                fontSize = 14.sp,
                                color = c.onSurfaceMuted,
                            )
                        }
                        BasicTextField(
                            value = customSender,
                            onValueChange = { customSender = it },
                            singleLine = true,
                            textStyle = TextStyle(
                                fontSize = 14.sp,
                                color = c.onSurface,
                            ),
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                    Spacer(Modifier.width(8.dp))
                    Box(
                        Modifier
                            .height(46.dp)
                            .clip(RoundedCornerShape(10.dp))
                            .background(if (customSender.isNotBlank()) c.accent else c.bubbleIncoming)
                            .clickable(enabled = customSender.isNotBlank()) {
                                onAssignSender(currentFile.id, customSender.trim())
                                onDismiss()
                            }
                            .padding(horizontal = 16.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        Text(
                            "Save",
                            fontSize = 14.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = if (customSender.isNotBlank()) Color.White else c.onSurfaceMuted,
                        )
                    }
                }

                Spacer(Modifier.height(14.dp))
                Box(
                    Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(10.dp))
                        .border(1.dp, c.border, RoundedCornerShape(10.dp))
                        .clickable {
                            onImportExport()
                            onDismiss()
                        }
                        .padding(vertical = 12.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        "Import chat export (.txt or .zip)",
                        fontSize = 13.5.sp,
                        fontWeight = FontWeight.Medium,
                        color = c.accent,
                    )
                }
            }
        }
    }
}
