package app.reyna.ui

import android.graphics.Bitmap
import android.graphics.Color as AndroidColor
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.reyna.ui.theme.Dimens
import app.reyna.ui.theme.reynaColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.io.File

/**
 * The page a quote came from.
 *
 * Deliberately does one thing: show which page of which document the answer was
 * taken from. No zoom, no selection, no search. Anyone who wants to actually
 * read the document is one tap from the reader they already have, and competing
 * with that reader would be a poor use of this screen.
 *
 * Rendering uses Android's own PdfRenderer, which has been present since API 21
 * and needs no dependency. It hands back a picture of a page and nothing else,
 * which is exactly enough for this and not enough to highlight words on it.
 */
@Composable
fun PdfViewerScreen(
    file: File,
    quote: String,
    startPage: Int,
) {
    val c = reynaColors
    val source = remember(file.path) { PdfSource(file) }
    DisposableEffect(source) { onDispose { source.close() } }

    val pageCount = source.pageCount
    if (pageCount <= 0) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text(
                "This file could not be opened as a PDF.",
                fontSize = 14.sp,
                color = c.onSurfaceMuted,
            )
        }
        return
    }

    // startPage is one based and comes from a model's page markers, so it is
    // clamped rather than trusted. A wrong page is a worse answer; a crash is
    // not an answer at all.
    val initial = (startPage - 1).coerceIn(0, pageCount - 1)
    val pager = rememberPagerState(initialPage = initial) { pageCount }

    Column(Modifier.fillMaxSize().background(c.background)) {
        if (quote.isNotBlank()) {
            QuoteBar(quote, pager.currentPage + 1, pageCount)
        }

        HorizontalPager(
            state = pager,
            modifier = Modifier.fillMaxSize(),
            pageSpacing = 10.dp,
            contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = Dimens.page),
        ) { index ->
            PdfPage(source, index)
        }
    }
}

@Composable
private fun QuoteBar(quote: String, page: Int, total: Int) {
    val c = reynaColors
    Column(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = Dimens.page, vertical = 8.dp)
            .clip(RoundedCornerShape(10.dp))
            .background(c.surface)
            .border(1.dp, c.border, RoundedCornerShape(10.dp))
            .padding(11.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                "The answer came from",
                fontSize = 11.sp,
                fontWeight = FontWeight.Medium,
                color = c.onSurfaceFaint,
                modifier = Modifier.weight(1f),
            )
            Spacer(Modifier.width(8.dp))
            Text("Page $page of $total", fontSize = 11.sp, color = c.onSurfaceFaint)
        }
        Spacer(Modifier.height(5.dp))
        Text(
            quote,
            fontSize = 12.5.sp,
            lineHeight = 18.sp,
            color = c.accent,
            fontFamily = FontFamily.Monospace,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
private fun PdfPage(source: PdfSource, index: Int) {
    val c = reynaColors
    // Rendered when the page is reached rather than up front. A three hundred
    // page document rendered eagerly would exhaust memory before it was shown.
    val bitmap by produceState<Bitmap?>(initialValue = null, source, index) {
        value = withContext(Dispatchers.IO) { source.render(index) }
    }

    Box(
        Modifier
            .fillMaxSize()
            .padding(vertical = 6.dp),
        contentAlignment = Alignment.Center,
    ) {
        val bmp = bitmap
        if (bmp == null) {
            CircularProgressIndicator(strokeWidth = 2.dp, color = c.onSurfaceFaint)
        } else {
            Image(
                bitmap = bmp.asImageBitmap(),
                contentDescription = "Page ${index + 1}",
                contentScale = ContentScale.Fit,
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(6.dp))
                    .background(androidx.compose.ui.graphics.Color.White)
                    .border(1.dp, c.border, RoundedCornerShape(6.dp)),
            )
        }
    }
}

/**
 * A PDF held open for as long as the screen is.
 *
 * PdfRenderer permits exactly one open page at a time across the whole
 * instance, so every render is serialised. Without that, a fast swipe renders
 * two pages at once and the second throws.
 */
private class PdfSource(file: File) {
    private var fd: ParcelFileDescriptor? = null
    private var renderer: PdfRenderer? = null
    private val lock = Mutex()

    val pageCount: Int

    init {
        var count = 0
        runCatching {
            val descriptor = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
            val r = PdfRenderer(descriptor)
            fd = descriptor
            renderer = r
            count = r.pageCount
        }.onFailure { Log.w(TAG, "cannot open ${file.name}: ${it.message}") }
        pageCount = count
    }

    suspend fun render(index: Int): Bitmap? = lock.withLock {
        val r = renderer ?: return null
        if (index !in 0 until r.pageCount) return null
        runCatching {
            r.openPage(index).use { page ->
                // Twice the page's natural size, so text stays legible on a
                // phone screen without rendering at a resolution nobody sees.
                val scale = 2
                val bmp = Bitmap.createBitmap(
                    page.width * scale,
                    page.height * scale,
                    Bitmap.Config.ARGB_8888,
                )
                // PdfRenderer draws only marks, leaving the rest transparent,
                // which renders as black on a dark theme. Paper is white.
                bmp.eraseColor(AndroidColor.WHITE)
                page.render(bmp, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                bmp
            }
        }.onFailure { Log.w(TAG, "render page $index failed: ${it.message}") }.getOrNull()
    }

    fun close() {
        runCatching { renderer?.close() }
        runCatching { fd?.close() }
        renderer = null
        fd = null
    }

    companion object {
        private const val TAG = "ReynaPdf"
    }
}
