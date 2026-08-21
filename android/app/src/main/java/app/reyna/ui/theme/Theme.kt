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
    /** Cards and rows that sit on the background. One step, never two. */
    val surface: Color,
    /** The hairline that separates things. Does the work a shadow would. */
    val border: Color,
    /** Reyna's answers. */
    val bubbleIncoming: Color,
    /** Your questions. The only saturated color in the chrome. */
    val bubbleOutgoing: Color,
    val onBubbleOutgoing: Color,
    val onSurface: Color,
    val onSurfaceMuted: Color,
    /** Fainter still, for timestamps and counts that must not compete. */
    val onSurfaceFaint: Color,
    val divider: Color,
    /** File chips nested inside an incoming bubble, which must recede. */
    val surfaceRaised: Color,
    val accent: Color,

    /**
     * The confidence triad, the one place color carries meaning.
     *
     * Green is not "good" and gray is not "bad": they say how much is known
     * about who shared a file. A gray dot is Reyna being honest, and is never
     * styled as an error. These are used as small dots and as text, never as
     * filled tiles behind an icon: tinting every row by confidence made the
     * library look like a status dashboard and buried the filenames, which are
     * the thing people actually scan for.
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

/**
 * Light is a warm off-white rather than pure white.
 *
 * Paper, not a screen. Pure white with gray cards reads as a form; a warm
 * ground with white cards and a hairline reads as a document, which is what
 * Reyna holds.
 */
private val LightColors = ReynaColors(
    background = Color(0xFFFCFCFB),
    surface = Color(0xFFFFFFFF),
    border = Color(0xFFE7E7E3),
    bubbleIncoming = Color(0xFFF3F3F1),
    bubbleOutgoing = Electric,
    onBubbleOutgoing = Color(0xFFFFFFFF),
    onSurface = Color(0xFF1A1A18),
    onSurfaceMuted = Color(0xFF73736D),
    onSurfaceFaint = Color(0xFFA1A19A),
    divider = Color(0xFFEDEDEA),
    surfaceRaised = Color(0xFFFFFFFF),
    accent = Electric,
    confident = Color(0xFF3F8F5B),
    partial = Color(0xFFB07A22),
    unknown = Color(0xFF9A9A93),
)

private val DarkColors = ReynaColors(
    background = Color(0xFF141415),
    surface = Color(0xFF1B1B1D),
    border = Color(0xFF2C2C30),
    bubbleIncoming = Color(0xFF26262A),
    bubbleOutgoing = Electric,
    onBubbleOutgoing = Color(0xFFFFFFFF),
    onSurface = Color(0xFFF0F0EE),
    onSurfaceMuted = Color(0xFF9A9A95),
    onSurfaceFaint = Color(0xFF6E6E6A),
    divider = Color(0xFF262629),
    surfaceRaised = Color(0xFF212125),
    accent = Color(0xFF6D86FF),
    confident = Color(0xFF5FB07B),
    partial = Color(0xFFD9A24A),
    unknown = Color(0xFF8A8A85),
)

val LocalReynaColors = staticCompositionLocalOf { LightColors }

object Dimens {
    val page = 16.dp

    /**
     * Bubble radius. The corner nearest the sender is tightened to [bubbleTail],
     * which is what makes a bubble read as coming from a side rather than
     * floating.
     */
    val bubble = 14.dp
    val bubbleTail = 5.dp
    val chip = 10.dp
}

private val ReynaTypography = Typography(
    titleMedium = TextStyle(fontSize = 17.sp, fontWeight = FontWeight.SemiBold, letterSpacing = (-0.2).sp),
    bodyLarge = TextStyle(fontSize = 15.sp, fontWeight = FontWeight.Normal, lineHeight = 22.sp),
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
