package app.reyna.ui.components

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Color as AndroidColor
import android.graphics.Matrix
import android.graphics.pdf.PdfRenderer
import android.media.ExifInterface
import android.os.ParcelFileDescriptor
import android.util.Log
import android.util.LruCache
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Audiotrack
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.FolderZip
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.InsertDriveFile
import androidx.compose.material.icons.rounded.PictureAsPdf
import androidx.compose.material.icons.rounded.Slideshow
import androidx.compose.material.icons.rounded.TableChart
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.search.FileKind
import app.reyna.search.SearchableFile
import app.reyna.ui.theme.reynaColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.util.Locale

/**
 * Loads and caches local file thumbnails (images and PDF first pages) with low memory footprint.
 */
object ThumbnailLoader {
    private const val TAG = "ThumbnailLoader"

    // 24MB in-memory LRU cache
    private val cache = object : LruCache<String, Bitmap>(24 * 1024 * 1024) {
        override fun sizeOf(key: String, value: Bitmap): Int = value.byteCount
    }

    fun getCached(path: String, mtime: Long, targetSize: Int = 140): Bitmap? {
        return cache.get(cacheKey(path, mtime, targetSize))
    }

    private fun cacheKey(path: String, mtime: Long, targetSize: Int): String =
        "$path:$mtime:$targetSize"

    suspend fun loadThumbnail(
        path: String,
        isImage: Boolean,
        targetSize: Int = 140,
    ): Bitmap? = withContext(Dispatchers.IO) {
        if (path.isBlank()) return@withContext null
        val file = File(path)
        if (!file.isFile || file.length() == 0L) return@withContext null

        val key = cacheKey(path, file.lastModified(), targetSize)
        cache.get(key)?.let { return@withContext it }

        val ext = file.extension.lowercase(Locale.ROOT)
        val bitmap = if (isImage || ext in listOf("jpg", "jpeg", "png", "webp", "heic", "gif", "bmp")) {
            decodeSampledImage(file, targetSize, targetSize)
        } else if (ext == "pdf") {
            renderPdfFirstPage(file, targetSize, targetSize)
        } else {
            null
        }

        if (bitmap != null) {
            cache.put(key, bitmap)
        }
        bitmap
    }

    private fun decodeSampledImage(file: File, reqWidth: Int, reqHeight: Int): Bitmap? = runCatching {
        val options = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeFile(file.absolutePath, options)
        if (options.outWidth <= 0 || options.outHeight <= 0) return null

        options.inSampleSize = calculateInSampleSize(options, reqWidth, reqHeight)
        options.inJustDecodeBounds = false
        options.inPreferredConfig = Bitmap.Config.RGB_565

        val decoded = BitmapFactory.decodeFile(file.absolutePath, options) ?: return null

        // Apply EXIF rotation if needed
        val rotation = runCatching {
            val exif = ExifInterface(file.absolutePath)
            when (exif.getAttributeInt(ExifInterface.TAG_ORIENTATION, ExifInterface.ORIENTATION_NORMAL)) {
                ExifInterface.ORIENTATION_ROTATE_90 -> 90f
                ExifInterface.ORIENTATION_ROTATE_180 -> 180f
                ExifInterface.ORIENTATION_ROTATE_270 -> 270f
                else -> 0f
            }
        }.getOrDefault(0f)

        if (rotation != 0f) {
            val matrix = Matrix().apply { postRotate(rotation) }
            val rotated = Bitmap.createBitmap(decoded, 0, 0, decoded.width, decoded.height, matrix, true)
            if (rotated != decoded) decoded.recycle()
            rotated
        } else {
            decoded
        }
    }.onFailure { Log.w(TAG, "failed decoding image ${file.name}: ${it.message}") }.getOrNull()

    private fun renderPdfFirstPage(file: File, reqWidth: Int, reqHeight: Int): Bitmap? = runCatching {
        ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { pfd ->
            PdfRenderer(pfd).use { renderer ->
                if (renderer.pageCount <= 0) return@use null
                renderer.openPage(0).use { page ->
                    val pw = page.width
                    val ph = page.height
                    val scale = (reqWidth.toFloat() / pw.toFloat()).coerceAtMost(reqHeight.toFloat() / ph.toFloat())
                    val w = (pw * scale).toInt().coerceIn(32, reqWidth * 2)
                    val h = (ph * scale).toInt().coerceIn(32, reqHeight * 2)
                    // PdfRenderer rejects RGB_565 on some Android builds
                    // (including the Pixel build used by Reyna). It requires
                    // a renderable 32-bit target.
                    val bmp = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888)
                    bmp.eraseColor(AndroidColor.WHITE)
                    page.render(bmp, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                    bmp
                }
            }
        }
    }.onFailure { Log.w(TAG, "failed rendering pdf thumb ${file.name}: ${it.message}") }.getOrNull()

    private fun calculateInSampleSize(options: BitmapFactory.Options, reqWidth: Int, reqHeight: Int): Int {
        val (height: Int, width: Int) = options.outHeight to options.outWidth
        var inSampleSize = 1
        if (height > reqHeight || width > reqWidth) {
            val halfHeight: Int = height / 2
            val halfWidth: Int = width / 2
            while (halfHeight / inSampleSize >= reqHeight && halfWidth / inSampleSize >= reqWidth) {
                inSampleSize *= 2
            }
        }
        return inSampleSize.coerceAtLeast(1)
    }
}

/**
 * File thumbnail composable that displays a cached or asynchronously loaded preview,
 * or a styled file-type badge.
 */
@Composable
fun FileThumbnail(
    file: SearchableFile,
    modifier: Modifier = Modifier.size(46.dp),
    cornerRadius: Dp = 8.dp,
    loadPreviewPath: suspend (SearchableFile) -> String? = { candidate ->
        candidate.path.takeIf { File(it).isFile }
    },
) {
    val c = reynaColors
    val isPreviewable = file.isImage || file.extension == "pdf"

    var bitmap by remember(file.path, file.mtime) {
        mutableStateOf(ThumbnailLoader.getCached(file.path, file.mtime))
    }

    LaunchedEffect(file.path, file.mtime, file.remoteId) {
        if (bitmap == null && isPreviewable) {
            val previewPath = loadPreviewPath(file)
            val loaded = previewPath?.let { ThumbnailLoader.loadThumbnail(it, file.isImage) }
            if (loaded != null) {
                bitmap = loaded
            }
        }
    }

    Box(
        modifier = modifier
            .clip(RoundedCornerShape(cornerRadius))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(cornerRadius)),
        contentAlignment = Alignment.Center,
    ) {
        val bmp = bitmap
        if (bmp != null) {
            Image(
                bitmap = bmp.asImageBitmap(),
                contentDescription = null,
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxSize(),
            )
        } else {
            KindFallbackBadge(file.kind, file.extension)
        }
    }
}

@Composable
private fun KindFallbackBadge(kind: FileKind, extension: String) {
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
    val tint = pair.second

    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier = Modifier.padding(2.dp),
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = tint.copy(alpha = 0.85f),
            modifier = Modifier.size(20.dp),
        )
        if (extension.isNotBlank() && extension.length <= 4) {
            Text(
                text = extension.uppercase(Locale.ROOT),
                fontSize = 8.sp,
                fontWeight = FontWeight.Bold,
                color = tint.copy(alpha = 0.9f),
                maxLines = 1,
            )
        }
    }
}
