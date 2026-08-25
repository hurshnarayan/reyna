package app.reyna.ui.components

import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Fill
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/**
 * The Reyna mark: three rising drops forming a crown.
 *
 * Every drop is the same curve placed three times, rotated about a pivot below
 * the mark, the outer pair splayed thirty degrees from the axis. The tails
 * stop level and short of that pivot, so the three shapes never touch. That
 * gap is the design; fuse them and the mark becomes a blob with notches in it.
 * The centre drop is longer and heavier than the outer pair, which is what
 * makes the silhouette read as a crown rather than a fan.
 *
 * Drawn rather than shipped as a bitmap so it takes whatever colour it is
 * given and stays exact at every size. The numbers are the artwork's own
 * hundred by hundred grid, scaled at draw time. Nothing here should be nudged
 * to make one particular size look right: the mark is specified, not
 * eyeballed, and a tweak that flatters a 24dp avatar is a different logo at
 * 112dp.
 */
object ReynaMark {

    /** The grid the geometry is specified on. */
    private const val GRID = 100f

    /** Below this the gaps close up and it stops reading as three shapes. */
    val MinSize: Dp = 16.dp

    /**
     * The three drops, centre first, then left, then right.
     *
     * Each list is a start point followed by cubic segments, six numbers per
     * segment: two control points and an end point.
     */
    private val DROPS: List<FloatArray> = listOf(
        floatArrayOf(
            50.00f, 84.54f,
            44.86f, 61.83f, 40.66f, 42.72f, 40.66f, 24.79f,
            40.66f, 19.66f, 44.86f, 15.46f, 50.00f, 15.46f,
            55.13f, 15.46f, 59.33f, 19.66f, 59.33f, 24.79f,
            59.33f, 42.72f, 55.13f, 61.83f, 50.00f, 84.54f,
        ),
        floatArrayOf(
            43.77f, 81.22f,
            31.35f, 68.74f, 20.98f, 58.17f, 14.26f, 46.53f,
            12.00f, 42.62f, 13.35f, 37.57f, 17.27f, 35.31f,
            21.18f, 33.05f, 26.23f, 34.40f, 28.49f, 38.31f,
            35.21f, 49.96f, 39.17f, 64.22f, 43.77f, 81.22f,
        ),
        floatArrayOf(
            56.22f, 81.22f,
            60.82f, 64.22f, 64.79f, 49.96f, 71.51f, 38.31f,
            73.77f, 34.40f, 78.82f, 33.05f, 82.73f, 35.31f,
            86.64f, 37.57f, 88.00f, 42.62f, 85.74f, 46.53f,
            79.02f, 58.17f, 68.65f, 68.74f, 56.22f, 81.22f,
        ),
    )

    /** How many drops the mark has, for anything animating them in turn. */
    const val DROP_COUNT = 3

    /**
     * The mark as a path filling a square of [side] pixels.
     *
     * [only] draws a single drop by index, for the places that bring them in
     * one at a time.
     */
    fun path(side: Float, only: Int? = null): Path {
        val k = side / GRID
        val path = Path()
        DROPS.forEachIndexed { i, d ->
            if (only != null && only != i) return@forEachIndexed
            path.moveTo(d[0] * k, d[1] * k)
            var j = 2
            while (j + 5 < d.size) {
                path.cubicTo(
                    d[j] * k, d[j + 1] * k,
                    d[j + 2] * k, d[j + 3] * k,
                    d[j + 4] * k, d[j + 5] * k,
                )
                j += 6
            }
            path.close()
        }
        return path
    }

    /** Draws the mark centred in this scope, as large as it will go. */
    fun DrawScope.drawMark(color: Color, only: Int? = null, alpha: Float = 1f) {
        val side = minOf(size.width, size.height)
        drawPath(path(side, only), color, alpha = alpha, style = Fill)
    }
}

/**
 * The mark, at whatever size the modifier gives it, in [color].
 *
 * Size it with the modifier rather than a parameter, so it composes with
 * everything else the way an icon does.
 */
@Composable
fun ReynaLogo(
    color: Color,
    modifier: Modifier = Modifier,
    contentDescription: String? = "Reyna",
) {
    val described = if (contentDescription != null) {
        modifier.semantics { this.contentDescription = contentDescription }
    } else {
        modifier
    }
    Canvas(described) {
        val side = minOf(size.width, size.height)
        drawPath(ReynaMark.path(side), color, style = Fill)
    }
}
