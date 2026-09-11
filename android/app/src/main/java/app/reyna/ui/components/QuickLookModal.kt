package app.reyna.ui.components

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Color as AndroidColor
import android.graphics.Matrix
import android.graphics.pdf.PdfRenderer
import android.media.ExifInterface
import android.os.ParcelFileDescriptor
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Audiotrack
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.FolderZip
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.InsertDriveFile
import androidx.compose.material.icons.rounded.PictureAsPdf
import androidx.compose.material.icons.rounded.Slideshow
import androidx.compose.material.icons.rounded.TableChart
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.attribution.Attribution
import app.reyna.search.FileKind
import app.reyna.search.SearchableFile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/**
 * Clean macOS Quick Look-style floating peek preview overlay.
 *
 * Appears on long-press (hold) and dismisses instantly upon release.
 * Intentionally free of buttons, headers, and clutter. Shows only the full
 * preview content with minimal hairline borders and a translucent dark
 * gradient at the bottom for the filename, author, and time.
 */
@Composable
fun QuickLookModal(
    file: SearchableFile,
    loadPreviewPath: suspend (SearchableFile) -> String? = { candidate ->
        candidate.path.takeIf { File(it).isFile }
    },
    onDismiss: () -> Unit = {},
    onOpen: () -> Unit = {},
    onAskWhoShared: () -> Unit = {},
    modifier: Modifier = Modifier,
) {
    BackHandler(onBack = onDismiss)

    var contentRatio by remember(file.path) { mutableStateOf<Float?>(null) }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.68f))
            .clickable(
                interactionSource = remember { MutableInteractionSource() },
                indication = null,
                onClick = onDismiss,
            )
            .padding(horizontal = 20.dp, vertical = 36.dp),
        contentAlignment = Alignment.Center,
    ) {
        BoxWithConstraints(
            contentAlignment = Alignment.Center,
        ) {
            val maxW = maxWidth * 0.94f
            val maxH = maxHeight * 0.74f

            val cardModifier = if (contentRatio != null && contentRatio!! > 0f) {
                val ratio = contentRatio!!.coerceIn(0.5f, 2.0f)
                val containerRatio = maxW.value / maxH.value
                if (ratio >= containerRatio) {
                    val w = maxW
                    val h = (maxW.value / ratio).dp.coerceIn(180.dp, maxH)
                    Modifier.size(w, h)
                } else {
                    val h = maxH
                    val w = (maxH.value * ratio).dp.coerceIn(180.dp, maxW)
                    Modifier.size(w, h)
                }
            } else {
                Modifier
                    .widthIn(min = 280.dp, max = maxW)
                    .heightIn(min = 260.dp, max = 380.dp)
            }

            Box(
                modifier = cardModifier
                    .clip(RoundedCornerShape(18.dp))
                    .background(Color(0xFF13151A))
                    .border(1.dp, Color.White.copy(alpha = 0.14f), RoundedCornerShape(18.dp))
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null,
                        enabled = false,
                    ) {},
                contentAlignment = Alignment.Center,
            ) {
                // ── Preview Canvas ──
                when {
                    file.isImage || file.kind == FileKind.IMAGE -> {
                        ImagePreviewCanvas(file, loadPreviewPath) { contentRatio = it }
                    }
                    file.extension == "pdf" -> {
                        PdfPreviewCanvas(file, loadPreviewPath) { contentRatio = it }
                    }
                    file.extension in listOf("txt", "csv", "tsv", "json", "xml", "log", "md", "java", "kt", "py", "c", "cpp", "h") -> {
                        TextPreviewCanvas(file.path)
                    }
                    else -> {
                        GenericFileInfoCanvas(file)
                    }
                }

                // ── Bottom Info Overlay (Frosted dark gradient for that info only) ──
                Box(
                    modifier = Modifier
                        .align(Alignment.BottomCenter)
                        .fillMaxWidth()
                        .background(
                            Brush.verticalGradient(
                                colors = listOf(
                                    Color.Transparent,
                                    Color.Black.copy(alpha = 0.70f),
                                    Color.Black.copy(alpha = 0.92f),
                                ),
                            ),
                        )
                        .padding(horizontal = 16.dp, vertical = 13.dp),
                ) {
                    Column {
                        Text(
                            text = file.fileName,
                            fontSize = 14.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = Color.White,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        Spacer(Modifier.height(2.dp))
                        Text(
                            text = buildAuthorTimeText(file),
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Normal,
                            color = Color.White.copy(alpha = 0.75f),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                }
            }
        }
    }
}

/**
 * Builds clean author, time, and size metadata text for the bottom info overlay.
 */
private fun buildAuthorTimeText(file: SearchableFile): String {
    val author = when {
        file.confidence >= Attribution.MIN_NAMED && !file.senderName.isNullOrBlank() -> file.senderName
        !file.chatName.isNullOrBlank() -> file.chatName
        else -> null
    }
    val parts = mutableListOf<String>()
    if (!author.isNullOrBlank()) {
        parts += author
    }
    if (file.whenText.isNotBlank()) {
        parts += file.whenText
    }
    if (parts.isEmpty()) {
        parts += "Found on phone"
    }
    if (file.formattedSize.isNotBlank()) {
        parts += file.formattedSize
    }
    return parts.joinToString(" · ")
}

@Composable
private fun KindIconBadge(kind: FileKind) {
    val pair: Pair<ImageVector, Color> = when (kind) {
        FileKind.IMAGE -> Icons.Rounded.Image to Color(0xFF42A5F5)
        FileKind.PDF -> Icons.Rounded.PictureAsPdf to Color(0xFFEF5350)
        FileKind.DOCUMENT -> Icons.Rounded.Description to Color(0xFF5C6BC0)
        FileKind.SPREADSHEET -> Icons.Rounded.TableChart to Color(0xFF66BB6A)
        FileKind.PRESENTATION -> Icons.Rounded.Slideshow to Color(0xFFFFA726)
        FileKind.ARCHIVE -> Icons.Rounded.FolderZip to Color(0xFFAB47BC)
        FileKind.AUDIO -> Icons.Rounded.Audiotrack to Color(0xFF26A69A)
        FileKind.OTHER -> Icons.Rounded.InsertDriveFile to Color(0xFF78909C)
    }
    val icon = pair.first
    val color = pair.second

    Box(
        modifier = Modifier
            .size(44.dp)
            .clip(RoundedCornerShape(10.dp))
            .background(color.copy(alpha = 0.16f))
            .border(1.dp, color.copy(alpha = 0.35f), RoundedCornerShape(10.dp)),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = color,
            modifier = Modifier.size(24.dp),
        )
    }
}

@Composable
private fun ImagePreviewCanvas(
    file: SearchableFile,
    loadPreviewPath: suspend (SearchableFile) -> String?,
    onAspectRatio: (Float) -> Unit,
) {
    var bitmap by remember(file.id) { mutableStateOf<Bitmap?>(null) }
    var loading by remember(file.id) { mutableStateOf(true) }

    LaunchedEffect(file.id, file.path, file.remoteId) {
        loading = true
        val result = withContext(Dispatchers.IO) {
            val path = loadPreviewPath(file) ?: return@withContext null
            val f = File(path)
            if (!f.isFile || f.length() == 0L) return@withContext null
            runCatching {
                val opts = BitmapFactory.Options().apply { inJustDecodeBounds = true }
                BitmapFactory.decodeFile(f.absolutePath, opts)
                var inSample = 1
                val maxDim = 1200
                while (opts.outWidth / inSample > maxDim || opts.outHeight / inSample > maxDim) {
                    inSample *= 2
                }
                opts.inSampleSize = inSample
                opts.inJustDecodeBounds = false
                val bmp = BitmapFactory.decodeFile(f.absolutePath, opts) ?: return@runCatching null
                val rot = runCatching {
                    val exif = ExifInterface(f.absolutePath)
                    when (exif.getAttributeInt(ExifInterface.TAG_ORIENTATION, ExifInterface.ORIENTATION_NORMAL)) {
                        ExifInterface.ORIENTATION_ROTATE_90 -> 90f
                        ExifInterface.ORIENTATION_ROTATE_180 -> 180f
                        ExifInterface.ORIENTATION_ROTATE_270 -> 270f
                        else -> 0f
                    }
                }.getOrDefault(0f)
                val finalBmp = if (rot != 0f) {
                    val matrix = Matrix().apply { postRotate(rot) }
                    Bitmap.createBitmap(bmp, 0, 0, bmp.width, bmp.height, matrix, true)
                } else bmp
                val ratio = finalBmp.width.toFloat() / finalBmp.height.toFloat().coerceAtLeast(1f)
                Pair(finalBmp, ratio)
            }.getOrNull()
        }
        if (result != null) {
            bitmap = result.first
            onAspectRatio(result.second)
        }
        loading = false
    }

    Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = Alignment.Center,
    ) {
        if (loading) {
            CircularProgressIndicator(strokeWidth = 2.dp, color = Color.White.copy(alpha = 0.6f))
        } else if (bitmap != null) {
            Image(
                bitmap = bitmap!!.asImageBitmap(),
                contentDescription = "Image Preview",
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxSize(),
            )
        } else {
            Text("Could not preview image", fontSize = 13.sp, color = Color.White.copy(alpha = 0.6f))
        }
    }
}

@Composable
private fun PdfPreviewCanvas(
    source: SearchableFile,
    loadPreviewPath: suspend (SearchableFile) -> String?,
    onAspectRatio: (Float) -> Unit,
) {
    var pageCount by remember(source.id) { mutableIntStateOf(0) }
    var bitmap by remember(source.id) { mutableStateOf<Bitmap?>(null) }
    var loading by remember(source.id) { mutableStateOf(true) }

    LaunchedEffect(source.id, source.path, source.remoteId) {
        loading = true
        val result = withContext(Dispatchers.IO) {
            val path = loadPreviewPath(source) ?: return@withContext null
            val file = File(path)
            if (!file.isFile) return@withContext null
            runCatching {
                ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { pfd ->
                    PdfRenderer(pfd).use { renderer ->
                        val count = renderer.pageCount
                        if (count <= 0) return@use null
                        renderer.openPage(0).use { page ->
                            val scale = 2
                            val bmp = Bitmap.createBitmap(
                                (page.width * scale).coerceAtMost(1600),
                                (page.height * scale).coerceAtMost(2400),
                                Bitmap.Config.ARGB_8888,
                            )
                            bmp.eraseColor(AndroidColor.WHITE)
                            page.render(bmp, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                            val ratio = page.width.toFloat() / page.height.toFloat().coerceAtLeast(1f)
                            Triple(bmp, ratio, count)
                        }
                    }
                }
            }.getOrNull()
        }
        if (result != null) {
            bitmap = result.first
            onAspectRatio(result.second)
            pageCount = result.third
        }
        loading = false
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.White),
        contentAlignment = Alignment.Center,
    ) {
        if (loading) {
            CircularProgressIndicator(strokeWidth = 2.dp, color = Color(0xFF6B7280))
        } else if (bitmap != null) {
            Image(
                bitmap = bitmap!!.asImageBitmap(),
                contentDescription = "PDF Preview",
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxSize(),
            )
            if (pageCount > 1) {
                // Subtle non-intrusive page badge (no buttons)
                Box(
                    modifier = Modifier
                        .align(Alignment.TopEnd)
                        .padding(10.dp)
                        .clip(RoundedCornerShape(6.dp))
                        .background(Color.Black.copy(alpha = 0.55f))
                        .padding(horizontal = 7.dp, vertical = 3.dp),
                ) {
                    Text(
                        text = "$pageCount pages",
                        fontSize = 10.sp,
                        fontWeight = FontWeight.Medium,
                        color = Color.White.copy(alpha = 0.9f),
                    )
                }
            }
        } else {
            Text("Could not preview PDF", fontSize = 13.sp, color = Color(0xFF6B7280))
        }
    }
}

@Composable
private fun TextPreviewCanvas(path: String) {
    var textContent by remember(path) { mutableStateOf<String?>(null) }
    var loading by remember(path) { mutableStateOf(true) }

    LaunchedEffect(path) {
        loading = true
        textContent = withContext(Dispatchers.IO) {
            val f = File(path)
            if (!f.isFile) return@withContext null
            runCatching {
                f.bufferedReader().useLines { lines ->
                    lines.take(200).joinToString("\n")
                }
            }.getOrNull()
        }
        loading = false
    }

    if (loading) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator(strokeWidth = 2.dp, color = Color.White.copy(alpha = 0.6f))
        }
    } else if (!textContent.isNullOrBlank()) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(bottom = 54.dp)
                .verticalScroll(rememberScrollState())
                .padding(14.dp),
        ) {
            Text(
                text = textContent!!,
                fontFamily = FontFamily.Monospace,
                fontSize = 11.sp,
                lineHeight = 16.sp,
                color = Color(0xFFE2E8F0),
            )
        }
    } else {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text("Empty file or cannot read text", fontSize = 13.sp, color = Color.White.copy(alpha = 0.6f))
        }
    }
}

@Composable
private fun GenericFileInfoCanvas(file: SearchableFile) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 24.dp, vertical = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        KindIconBadge(file.kind)
        Spacer(Modifier.height(14.dp))
        Text(
            text = file.fileName,
            fontSize = 15.sp,
            fontWeight = FontWeight.Medium,
            color = Color.White,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        if (file.formattedSize.isNotBlank()) {
            Spacer(Modifier.height(4.dp))
            Text(
                text = file.formattedSize,
                fontSize = 12.sp,
                color = Color.White.copy(alpha = 0.6f),
            )
        }
    }
}
