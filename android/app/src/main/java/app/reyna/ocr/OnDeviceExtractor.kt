package app.reyna.ocr

import android.content.Context
import android.graphics.Bitmap
import android.graphics.pdf.PdfRenderer
import android.net.Uri
import android.os.ParcelFileDescriptor
import android.util.Log
import com.google.android.gms.tasks.Tasks
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.latin.TextRecognizerOptions
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.util.concurrent.TimeUnit

/**
 * Extracts text on-device from images and documents.
 *
 * Uses Google Play Services ML Kit Text Recognition for images and Android's
 * native PdfRenderer for rendering and OCR-ing PDF pages locally.
 * Enables offline searching inside media without network latency or API costs.
 */
object OnDeviceExtractor {
    private const val TAG = "OnDeviceExtractor"
    private const val MAX_EXTRACTED_CHARS = 100_000
    private val recognizer by lazy {
        TextRecognition.getClient(TextRecognizerOptions.DEFAULT_OPTIONS)
    }

    suspend fun extract(context: Context, file: File, isImage: Boolean): String? = withContext(Dispatchers.IO) {
        if (!file.exists() || !file.isFile || file.length() == 0L) return@withContext null
        val ext = file.name.substringAfterLast('.', "").lowercase()

        runCatching {
            when {
                isImage || ext in setOf("jpg", "jpeg", "png", "webp") -> {
                    extractFromImage(context, file)
                }
                ext == "pdf" -> {
                    extractFromPdf(file)
                }
                ext in setOf("txt", "csv", "json", "log", "md") -> {
                    file.readText().take(50000).ifBlank { null }
                }
                else -> null
            }
        }.onFailure {
            Log.w(TAG, "extract failed for ${file.name}: ${it.message}")
        }.getOrNull()
    }

    private fun extractFromImage(context: Context, file: File): String? {
        val image = InputImage.fromFilePath(context, Uri.fromFile(file))
        val visionText = Tasks.await(recognizer.process(image), 15, TimeUnit.SECONDS)
        return visionText.text.trim().ifBlank { null }
    }

    private fun extractFromPdf(file: File): String? {
        var pfd: ParcelFileDescriptor? = null
        var renderer: PdfRenderer? = null
        return try {
            pfd = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
            renderer = PdfRenderer(pfd)
            val sb = StringBuilder()

            // This runs on Dispatchers.IO from capture/reconcile workers, not
            // on the UI thread. One page and bitmap at a time bounds memory;
            // the text cap bounds Room/FTS size while allowing normal PDFs to
            // be indexed beyond the old three-page cutoff.
            for (i in 0 until renderer.pageCount) {
                if (sb.length >= MAX_EXTRACTED_CHARS) break
                val text = renderer.openPage(i).use { page ->
                    val scale = if (page.width > 1200) 1200f / page.width else 1f
                    val w = (page.width * scale).toInt().coerceAtLeast(1)
                    val h = (page.height * scale).toInt().coerceAtLeast(1)
                    val bitmap = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888)
                    try {
                        page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                        val input = InputImage.fromBitmap(bitmap, 0)
                        Tasks.await(recognizer.process(input), 15, TimeUnit.SECONDS).text.trim()
                    } finally {
                        bitmap.recycle()
                    }
                }
                if (text.isNotBlank()) {
                    val heading = "--- Page ${i + 1} ---\n"
                    val remaining = MAX_EXTRACTED_CHARS - sb.length - heading.length
                    if (remaining <= 0) break
                    sb.append(heading).append(text.take(remaining)).append("\n\n")
                }
            }
            sb.toString().trim().ifBlank { null }
        } catch (e: Exception) {
            Log.w(TAG, "PDF OCR failed for ${file.name}: ${e.message}")
            null
        } finally {
            try { renderer?.close() } catch (_: Throwable) {}
            try { pfd?.close() } catch (_: Throwable) {}
        }
    }
}
