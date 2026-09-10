package app.reyna.ui.components

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Image
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import app.reyna.R

private val MascotFrames = intArrayOf(
    R.drawable.reyna_mascot_1,
    R.drawable.reyna_mascot_2,
    R.drawable.reyna_mascot_3,
    R.drawable.reyna_mascot_4,
    R.drawable.reyna_mascot_5,
)

/** Reyna at rest. Frame three looks straight ahead and anchors the sequence. */
@Composable
fun ReynaMascot(
    modifier: Modifier = Modifier,
    contentDescription: String? = "Reyna",
) {
    Image(
        painter = painterResource(R.drawable.reyna_mascot_3),
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
