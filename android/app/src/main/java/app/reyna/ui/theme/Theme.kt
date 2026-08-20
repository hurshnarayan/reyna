package app.reyna.ui.theme

import android.app.Activity
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat

/**
 * Design tokens. DESIGN.md is the source of truth.
 *
 * Reyna is a conversation with your own archive, so the surface is a messaging
 * app rather than a dashboard: a plain background, no cards, and separation
 * carried by bubble color and whitespace.
 */
data class ReynaColors(
    val background: Color,
    /** Reyna's answers. */
    val bubbleIncoming: Color,
    /** Your questions. The only saturated color in the chrome. */
    val bubbleOutgoing: Color,
    val onBubbleOutgoing: Color,
    val onSurface: Color,
    val onSurfaceMuted: Color,
    val divider: Color,
    /** File chips nested inside an incoming bubble, which must recede. */
    val surfaceRaised: Color,

    /**
     * The confidence triad, the one place color carries meaning.
     *
     * Green is not "good" and gray is not "bad": they say how much is known
     * about who shared a file. A gray chip is Reyna being honest, and is never
     * styled as an error.
     */
    val confident: Color,
    val partial: Color,
    val unknown: Color,
) {
    fun forConfidence(confidence: Double): Color = when {
        confidence >= 0.70 -> confident
        confidence >= 0.30 -> partial
        else -> unknown
    }
}

/** The mark is a lightning bolt, so the accent is electric. */
private val Electric = Color(0xFF3B5BFE)

private val LightColors = ReynaColors(
    background = Color(0xFFFFFFFF),
    bubbleIncoming = Color(0xFFF1F1F4),
    bubbleOutgoing = Electric,
    onBubbleOutgoing = Color(0xFFFFFFFF),
    onSurface = Color(0xFF0A0A0B),
    onSurfaceMuted = Color(0xFF6B6B70),
    divider = Color(0xFFE8E8EA),
    surfaceRaised = Color(0xFFF7F7F9),
    confident = Color(0xFF2FA84F),
    partial = Color(0xFFE08600),
    unknown = Color(0xFF8A8A90),
)

private val DarkColors = ReynaColors(
    background = Color(0xFF121214),
    bubbleIncoming = Color(0xFF29292E),
    bubbleOutgoing = Electric,
    onBubbleOutgoing = Color(0xFFFFFFFF),
    onSurface = Color(0xFFF2F2F4),
    onSurfaceMuted = Color(0xFF96969C),
    divider = Color(0xFF2A2A2F),
    surfaceRaised = Color(0xFF1E1E22),
    confident = Color(0xFF3FBF62),
    partial = Color(0xFFF0A030),
    unknown = Color(0xFF96969C),
)

val LocalReynaColors = staticCompositionLocalOf { LightColors }

object Dimens {
    val page = 16.dp

    /**
     * Bubble radius. The corner nearest the sender is tightened to [bubbleTail],
     * which is what makes a bubble read as coming from a side rather than
     * floating.
     */
    val bubble = 18.dp
    val bubbleTail = 4.dp
    val chip = 12.dp
}

private val ReynaTypography = Typography(
    titleMedium = TextStyle(fontSize = 17.sp, fontWeight = FontWeight.SemiBold, letterSpacing = (-0.2).sp),
    bodyLarge = TextStyle(fontSize = 15.sp, fontWeight = FontWeight.Normal, lineHeight = 21.sp),
    bodyMedium = TextStyle(fontSize = 13.sp, fontWeight = FontWeight.Normal),
    labelSmall = TextStyle(fontSize = 11.sp, fontWeight = FontWeight.Medium),
)

@Composable
fun ReynaTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    val colors = if (darkTheme) DarkColors else LightColors

    val view = LocalView.current
    if (!view.isInEditMode) {
        SideEffect {
            val window = (view.context as Activity).window
            window.statusBarColor = colors.background.toArgb()
            window.navigationBarColor = colors.background.toArgb()
            // Bars invert with the theme, or their icons vanish against it.
            WindowCompat.getInsetsController(window, view).apply {
                isAppearanceLightStatusBars = !darkTheme
                isAppearanceLightNavigationBars = !darkTheme
            }
        }
    }

    CompositionLocalProvider(LocalReynaColors provides colors) {
        MaterialTheme(
            colorScheme = if (darkTheme) {
                darkColorScheme(
                    background = colors.background,
                    surface = colors.background,
                    onBackground = colors.onSurface,
                    onSurface = colors.onSurface,
                    primary = colors.bubbleOutgoing,
                    onPrimary = colors.onBubbleOutgoing,
                )
            } else {
                lightColorScheme(
                    background = colors.background,
                    surface = colors.background,
                    onBackground = colors.onSurface,
                    onSurface = colors.onSurface,
                    primary = colors.bubbleOutgoing,
                    onPrimary = colors.onBubbleOutgoing,
                )
            },
            typography = ReynaTypography,
            content = content,
        )
    }
}

val reynaColors: ReynaColors
    @Composable get() = LocalReynaColors.current
