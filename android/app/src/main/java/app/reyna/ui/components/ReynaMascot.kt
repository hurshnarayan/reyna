package app.reyna.ui.components

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Image
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import app.reyna.R
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlin.random.Random

private val MascotFrames = intArrayOf(
    R.drawable.reyna_mascot_1,
    R.drawable.reyna_mascot_2,
    R.drawable.reyna_mascot_3,
    R.drawable.reyna_mascot_4,
    R.drawable.reyna_mascot_5,
)

private val NavMascotFrames = intArrayOf(
    R.drawable.reyna_nav_1,
    R.drawable.reyna_nav_2,
    R.drawable.reyna_nav_3,
    R.drawable.reyna_nav_4,
    R.drawable.reyna_nav_5,
    R.drawable.reyna_nav_6,
    R.drawable.reyna_nav_7,
    R.drawable.reyna_nav_8,
    R.drawable.reyna_nav_9,
)

/** Reyna at rest. Looks straight ahead and anchors the mark. */
@Composable
fun ReynaMascot(
    modifier: Modifier = Modifier,
    contentDescription: String? = "Reyna",
) {
    Image(
        painter = painterResource(R.drawable.reyna_nav_1),
        contentDescription = contentDescription,
        modifier = modifier,
        contentScale = ContentScale.Fit,
    )
}

/** Reyna's signature paw print left at the end of answers. */
@Composable
fun ReynaPaw(
    modifier: Modifier = Modifier,
    contentDescription: String? = "Reyna",
) {
    Image(
        painter = painterResource(R.drawable.reyna_paw),
        contentDescription = contentDescription,
        modifier = modifier,
        contentScale = ContentScale.Fit,
    )
}

/**
 * Navigation bar mascot.
 *
 * Rather than a continuous rapid loop, it rests statically and triggers an
 * organic "alive" sequence (ears perk, blink, look around, return) at a
 * relaxed pace (200ms per frame, 1.8s total), waiting 15-20 seconds between triggers.
 */
@Composable
fun ReynaNavMascot(
    modifier: Modifier = Modifier,
    contentDescription: String? = "Reyna",
    frameDurationMillis: Long = 200L,
) {
    var frameIndex by remember { mutableStateOf(0) }
    LaunchedEffect(Unit) {
        // Initial gentle pause before first alive sequence
        delay(3_000L)
        while (isActive) {
            for (i in NavMascotFrames.indices) {
                frameIndex = i
                delay(frameDurationMillis)
            }
            frameIndex = 0
            // Subsequent animations hit after 15 to 20 seconds
            val restInterval = Random.nextLong(15_000L, 20_000L)
            delay(restInterval)
        }
    }

    Image(
        painter = painterResource(NavMascotFrames[frameIndex]),
        contentDescription = contentDescription,
        modifier = modifier,
        contentScale = ContentScale.Fit,
    )
}

/**
 * The five supplied drawings played as one mascot animation.
 *
 * Compose's animation clock respects the device animation scale, unlike a
 * coroutine delay loop, so Android still owns reduced-motion behavior.
 */
@Composable
fun ReynaMascotAnimated(
    modifier: Modifier = Modifier,
    contentDescription: String? = "Reyna is thinking",
) {
    val transition = rememberInfiniteTransition(label = "reyna-mascot")
    val position by transition.animateFloat(
        initialValue = 0f,
        targetValue = MascotFrames.size.toFloat(),
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 1_100, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "reyna-mascot-frame",
    )
    val frame = position.toInt().coerceIn(MascotFrames.indices)

    Image(
        painter = painterResource(MascotFrames[frame]),
        contentDescription = contentDescription,
        modifier = modifier,
        contentScale = ContentScale.Fit,
    )
}
