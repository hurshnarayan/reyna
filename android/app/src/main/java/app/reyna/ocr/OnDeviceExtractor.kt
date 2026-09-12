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
        return formatVisionText(visionText).ifBlank { null }
    }

    /**
     * Reconstructs 2D reading order from OCR text blocks.
     *
     * ML Kit's default `visionText.text` concatenates text blocks in arbitrary column order,
     * which scrambles tables, multi-column forms, timetables, and proctor lists.
     * This clusters detected lines into visual rows by vertical position, sorts each row
     * left-to-right by horizontal coordinate, and joins table columns with " | ".
     */
    fun formatVisionText(visionText: com.google.mlkit.vision.text.Text): String {
        val allLines = visionText.textBlocks.flatMap { it.lines }
        if (allLines.isEmpty()) {
            return visionText.text.trim()
        }

        val validLines = allLines.filter { it.boundingBox != null && it.text.isNotBlank() }
        if (validLines.isEmpty()) {
            return visionText.text.trim()
        }

        val sorted = validLines.sortedWith(compareBy({ it.boundingBox!!.top }, { it.boundingBox!!.left }))

        class VisualRow(
            var top: Int,
            var bottom: Int,
            val lines: MutableList<com.google.mlkit.vision.text.Text.Line> = mutableListOf()
        ) {
            fun add(line: com.google.mlkit.vision.text.Text.Line) {
                val box = line.boundingBox!!
                lines.add(line)
                top = minOf(top, box.top)
                bottom = maxOf(bottom, box.bottom)
            }
        }

        val rows = mutableListOf<VisualRow>()
        for (line in sorted) {
            val box = line.boundingBox!!
            val lineH = (box.bottom - box.top).coerceAtLeast(1)
            val centerY = (box.top + box.bottom) / 2

            val matching = rows.find { r ->
                val rH = (r.bottom - r.top).coerceAtLeast(1)
                val overlapTop = maxOf(r.top, box.top)
                val overlapBottom = minOf(r.bottom, box.bottom)
                val overlap = (overlapBottom - overlapTop).coerceAtLeast(0)
                val minH = minOf(lineH, rH)
                overlap >= (minH * 0.45f) || (centerY in r.top..r.bottom)
            }

            if (matching != null) {
                matching.add(line)
            } else {
                val newRow = VisualRow(box.top, box.bottom)
                newRow.add(line)
                rows.add(newRow)
            }
        }

        rows.sortBy { it.top }

        val sb = StringBuilder()
        for (r in rows) {
            r.lines.sortBy { it.boundingBox!!.left }
            if (r.lines.size > 1) {
                sb.append(r.lines.joinToString(" | ") { it.text.trim() }).append("\n")
            } else {
                sb.append(r.lines.first().text.trim()).append("\n")
            }
        }

        return sb.toString().trim()
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
                        val visionText = Tasks.await(recognizer.process(input), 15, TimeUnit.SECONDS)
                        formatVisionText(visionText)
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
