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
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat

/**
 * Design tokens. See DESIGN.md, which is the source of truth.
 *
 * Kept as a plain object rather than folded into Material's color scheme because
 * several of these have no Material equivalent: the sunken tone used for rows
 * nested inside a card, and the three confidence colors, which carry meaning
 * rather than decoration.
 */
data class ReynaColors(
    val background: Color,
    val surface: Color,
    /** For rows nested inside a card, which must recede rather than stack. */
    val surfaceSunken: Color,
    val onSurface: Color,
    val onSurfaceMuted: Color,
    /** Filled buttons, active nav, FAB. Inverts between themes. */
    val accent: Color,
    val onAccent: Color,
    val outline: Color,

    /**
     * The confidence triad.
     *
     * Green is not "good" and gray is not "bad": they say how much is known
     * about who shared a file. A gray row is Reyna being honest, and must never
     * be styled as a defect or an error.
     */
    val confident: Color,
    val partial: Color,
    val unknown: Color,
) {
    /** The color for a given attribution confidence. */
    fun forConfidence(confidence: Double): Color = when {
        confidence >= 0.70 -> confident
        confidence >= 0.30 -> partial
        else -> unknown
    }
}

private val LightColors = ReynaColors(
    // Warm off-white, never pure white: the cards are white, and the page has to
    // sit behind them without a drop shadow doing the work.
    background = Color(0xFFF6F6F4),
    surface = Color(0xFFFFFFFF),
    surfaceSunken = Color(0xFFF2F2F0),
    onSurface = Color(0xFF111113),
    onSurfaceMuted = Color(0xFF8A8A8E),
    accent = Color(0xFF000000),
    onAccent = Color(0xFFFFFFFF),
    outline = Color(0xFFEAEAE8),
    confident = Color(0xFF34C759),
    partial = Color(0xFFFF9F0A),
    unknown = Color(0xFF8A8A8E),
)

private val DarkColors = ReynaColors(
    background = Color(0xFF0B0B0C),
    surface = Color(0xFF1A1A1C),
    surfaceSunken = Color(0xFF232326),
    onSurface = Color(0xFFF5F5F7),
    onSurfaceMuted = Color(0xFF8A8A8E),
    accent = Color(0xFFFFFFFF),
    onAccent = Color(0xFF000000),
    outline = Color(0xFF2A2A2E),
    confident = Color(0xFF30D158),
    partial = Color(0xFFFFA023),
    unknown = Color(0xFF8A8A8E),
)

val LocalReynaColors = staticCompositionLocalOf { LightColors }

/** Spacing, from DESIGN.md. Named so the numbers are not scattered. */
object Dimens {
    val page = 16.dp
    val cardPadding = 18.dp
    val cardGap = 12.dp
    val cardRadius = 24.dp
    val smallCardRadius = 20.dp
}

/**
 * One family, weight doing the work. Numbers are heavy with tight tracking,
 * which is what gives the hero card its presence.
 */
private val ReynaTypography = Typography(
    displayLarge = TextStyle(fontSize = 40.sp, fontWeight = FontWeight.Bold, letterSpacing = (-1.5).sp),
    titleLarge = TextStyle(fontSize = 20.sp, fontWeight = FontWeight.Bold, letterSpacing = (-0.5).sp),
    titleMedium = TextStyle(fontSize = 16.sp, fontWeight = FontWeight.SemiBold, letterSpacing = (-0.2).sp),
    bodyLarge = TextStyle(fontSize = 15.sp, fontWeight = FontWeight.Medium),
    bodyMedium = TextStyle(fontSize = 13.sp, fontWeight = FontWeight.Normal),
    labelSmall = TextStyle(fontSize = 12.sp, fontWeight = FontWeight.Medium),
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
            window.statusBarColor = colors.background.value.toInt()
            // Status bar icons invert with the theme, or they vanish against it.
            WindowCompat.getInsetsController(window, view)
                .isAppearanceLightStatusBars = !darkTheme
        }
    }

    CompositionLocalProvider(LocalReynaColors provides colors) {
        MaterialTheme(
            colorScheme = if (darkTheme) {
                darkColorScheme(
                    background = colors.background,
                    surface = colors.surface,
                    onBackground = colors.onSurface,
                    onSurface = colors.onSurface,
                    primary = colors.accent,
                    onPrimary = colors.onAccent,
                )
            } else {
                lightColorScheme(
                    background = colors.background,
                    surface = colors.surface,
                    onBackground = colors.onSurface,
                    onSurface = colors.onSurface,
                    primary = colors.accent,
                    onPrimary = colors.onAccent,
                )
            },
            typography = ReynaTypography,
            content = content,
        )
    }
}

/** Shorthand for the tokens, since every component needs them. */
val reynaColors: ReynaColors
    @Composable get() = LocalReynaColors.current
