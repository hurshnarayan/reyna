package app.reyna.ui.components

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Fill
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.drawscope.translate
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * The mark assembling itself: a drop falls, lands, and the three drops splash
 * up out of the point where it hit.
 *
 * The animation is the mark's own construction rather than something laid over
 * it. All three drops are rotations about a single pivot below the mark, and
 * their tails already point down at that pivot, so the silhouette is a splash
 * caught mid-air. Growing them outward from that point is the motion the shape
 * was drawn for, and it is why nothing here needs a second visual idea to
 * carry it.
 *
 * The falling drop is the same bulb the mark is built from, so the piece that
 * lands is recognisably the thing that then becomes the logo.
 */
private const val GRID = 100f

/** Where the three drops converge, just below the mark, on the 100 grid. */
private const val PIVOT_X = 50f
private const val PIVOT_Y = 88f

/** Gravity: slow at the top of the fall, fastest at the moment of impact. */
private val Gravity = CubicBezierEasing(0.55f, 0f, 0.85f, 0.3f)

/** The order the drops arrive in: outer pair first, so the crown completes. */
private val ARRIVAL = intArrayOf(1, 2, 0)

/**
 * Plays the splash once and leaves the mark standing.
 *
 * [play] restarts it. Give it a value that changes when the screen is entered
 * rather than a constant, or returning to the screen shows a mark that is
 * already assembled and the animation is never seen.
 */
@Composable
fun ReynaMarkSplash(
    color: Color,
    modifier: Modifier = Modifier,
    play: Any = Unit,
) {
    // One clock for the whole sequence, in units of the phases below, so the
    // fall, the ripple and the three drops cannot drift apart.
    val t = remember { Animatable(0f) }

    LaunchedEffect(play) {
        t.snapTo(0f)
        t.animateTo(1f, tween(durationMillis = 1750, easing = LinearEasing))
    }

    // A slow breath afterwards, so the finished mark is not a dead object on
    // the screen. Far gentler than the working indicator: this one is resting.
    val idle = rememberInfiniteTransition(label = "idle")
    val sway by idle.animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(
            animation = tween(2600, easing = FastOutSlowInEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "sway",
    )

    Canvas(modifier) {
        val side = minOf(size.width, size.height)
        val k = side / GRID
        val clock = t.value

        // ── the fall ──
        //
        // 0.00 to 0.34 of the sequence. The drop enters above the frame and
        // accelerates into the pivot.
        val fall = ((clock - 0f) / 0.34f).coerceIn(0f, 1f)
        if (fall < 1f) {
            val eased = Gravity.transform(fall)
            // Starts just inside the top edge rather than above it. A Canvas
            // clips to its own bounds, so a drop released above the frame
            // spends the first fifth of a second being drawn where nothing can
            // see it, and the animation opens on an empty screen. Fading in
            // over the first moments is what keeps it from simply appearing.
            val y = 3f + (PIVOT_Y - 3f) * eased
            val stretch = 1f + 0.9f * eased * eased
            val fadeIn = (fall / 0.14f).coerceIn(0f, 1f)
            drawDroplet(color, PIVOT_X * k, y * k, 6.2f * k, stretch, fadeIn)
        }

        // ── the ripple ──
        //
        // 0.30 to 0.72. Starts a fraction before the drop lands so the impact
        // reads as one event rather than a drop and then a separate circle.
        val ripple = ((clock - 0.30f) / 0.42f).coerceIn(0f, 1f)
        if (ripple > 0f && ripple < 1f) {
            drawRipple(color, PIVOT_X * k, PIVOT_Y * k, ripple, k)
        }

        // ── the splash ──
        //
        // Each drop grows out of the pivot, overshoots a little and settles.
        // Anchored at the pivot rather than at its own centre, so it reads as
        // thrown up from the impact instead of fading in where it will end up.
        ARRIVAL.forEachIndexed { order, drop ->
            val start = 0.34f + order * 0.10f
            val p = ((clock - start) / 0.42f).coerceIn(0f, 1f)
            if (p <= 0f) return@forEachIndexed

            val grow = overshoot(p)
            val settled = if (clock >= 1f) 1f + 0.012f * sway else 1f
            val s = grow * settled

            translate(PIVOT_X * k, PIVOT_Y * k) {
                scale(s, s, Offset.Zero) {
                    translate(-PIVOT_X * k, -PIVOT_Y * k) {
                        drawPath(ReynaMark.path(side, only = drop), color, alpha = p.coerceIn(0f, 1f), style = Fill)
                    }
                }
            }
        }
    }
}

/**
 * Eases to 1 with a small overshoot, so a drop arrives with some weight
 * behind it rather than gliding to a stop.
 */
private fun overshoot(p: Float): Float {
    if (p >= 1f) return 1f
    val c = 2.2f
    val x = p - 1f
    return 1f + (c + 1f) * x * x * x + c * x * x
}

/** The falling drop: a bulb, stretched along the direction of travel. */
private fun DrawScope.drawDroplet(
    color: Color,
    cx: Float,
    cy: Float,
    r: Float,
    stretch: Float,
    alpha: Float,
) {
    translate(cx, cy) {
        scale(1f / stretch, stretch, Offset.Zero) {
            drawCircle(color, radius = r, center = Offset.Zero, alpha = alpha)
        }
    }
}

/**
 * Two rings spreading from the impact.
 *
 * Flattened to a third of their height, because the surface is being read as
 * lying away from the viewer rather than facing them, and a perfect circle
 * here looks like a halo instead of water. The second ring trails the first,
 * which is what makes it read as a ripple rather than one expanding circle.
 */
private fun DrawScope.drawRipple(color: Color, cx: Float, cy: Float, p: Float, k: Float) {
    translate(cx, cy) {
        scale(1f, 0.34f, Offset.Zero) {
            for (ring in 0..1) {
                val lag = ring * 0.28f
                val q = ((p - lag) / (1f - lag)).coerceIn(0f, 1f)
                if (q <= 0f) continue
                val radius = (4f + 42f * q) * k
                val fade = (1f - q) * (1f - q) * (if (ring == 0) 0.75f else 0.4f)
                drawCircle(
                    color = color,
                    radius = radius,
                    center = Offset.Zero,
                    alpha = fade,
                    style = Stroke(width = (2.4f - 1.4f * q).coerceAtLeast(0.4f) * k),
                )
            }
        }
    }
}

/**
 * The mark working: the drops jiggling in place.
 *
 * Used while an answer is being put together. The drops lean and settle
 * slightly out of step with each other, which reads as something alive
 * thinking rather than a progress indicator counting. A spinner would say the
 * same thing while introducing a second piece of visual language onto a screen
 * that already has one.
 */
@Composable
fun ReynaMarkThinking(
    color: Color,
    modifier: Modifier = Modifier,
) {
    val wobble = rememberInfiniteTransition(label = "thinking")
    val phase by wobble.animateFloat(
        initialValue = 0f,
        targetValue = (2f * Math.PI).toFloat(),
        animationSpec = infiniteRepeatable(
            animation = tween(1500, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "phase",
    )

    Canvas(modifier) {
        val side = minOf(size.width, size.height)
        for (drop in 0 until ReynaMark.DROP_COUNT) {
            // Each drop a third of a cycle behind the last, so they ripple
            // through the mark rather than pulsing as one lump.
            val a = phase - drop * 2.1f
            val lift = kotlin.math.sin(a.toDouble()).toFloat()
            val s = 1f + 0.075f * lift
            val alpha = 0.55f + 0.45f * ((lift + 1f) / 2f)

            translate(PIVOT_X * side / GRID, PIVOT_Y * side / GRID) {
                scale(s, s, Offset.Zero) {
                    translate(-PIVOT_X * side / GRID, -PIVOT_Y * side / GRID) {
                        drawPath(ReynaMark.path(side, only = drop), color, alpha = alpha, style = Fill)
                    }
                }
            }
        }
    }
}
