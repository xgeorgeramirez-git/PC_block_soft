package ru.atlas404.pcblock.ui

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.ExperimentalTextApi
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import ru.atlas404.pcblock.R

/**
 * Шрифт Inter встроен (res/font/inter_variable.ttf, лицензия SIL OFL 1.1 — assets/licenses/Inter-OFL.txt):
 * один переменный файл, насыщенность — ось wght (ТЗ 8.0 п. 2).
 */
@OptIn(ExperimentalTextApi::class)
val Inter = FontFamily(
    listOf(400, 500, 600, 700, 800).map { w ->
        Font(
            R.font.inter_variable,
            weight = FontWeight(w),
            variationSettings = FontVariation.Settings(FontVariation.weight(w)),
        )
    },
)

/** Цифры моноширинные по ширине (tabular): остаток, минуты, коды не «прыгают» (ТЗ 8.0 п. 2). */
const val TNUM = "tnum"

/** Стиль чисел: Inter с tabular-цифрами (у Text нет параметра fontFeatureSettings, только style). */
val NumStyle = TextStyle(fontFamily = Inter, fontFeatureSettings = TNUM)

private val Base = Typography()

private fun TextStyle.inter(): TextStyle = copy(fontFamily = Inter)

private val AppTypography = Typography(
    displayLarge = Base.displayLarge.inter(),
    displayMedium = Base.displayMedium.inter(),
    displaySmall = Base.displaySmall.inter(),
    headlineLarge = Base.headlineLarge.inter(),
    headlineMedium = Base.headlineMedium.inter(),
    headlineSmall = Base.headlineSmall.inter(),
    titleLarge = Base.titleLarge.inter(),
    titleMedium = Base.titleMedium.inter(),
    titleSmall = Base.titleSmall.inter(),
    bodyLarge = Base.bodyLarge.inter(),
    bodyMedium = Base.bodyMedium.inter(),
    bodySmall = Base.bodySmall.inter(),
    labelLarge = Base.labelLarge.inter(),
    labelMedium = Base.labelMedium.inter(),
    labelSmall = Base.labelSmall.inter(),
)

// Всегда светлая тема (ТЗ 8.0 п. 1, 9): тёмной у окна родителя и телефона нет. Цвета — токены Tok.
private val Light = lightColorScheme(
    primary = Tok.Accent,
    onPrimary = Color.White,
    primaryContainer = Tok.AccentSoft,
    onPrimaryContainer = Tok.Ink,
    secondary = Tok.Accent,
    onSecondary = Color.White,
    secondaryContainer = Tok.AccentSoft,
    onSecondaryContainer = Tok.Ink,
    background = Tok.Ground,
    onBackground = Tok.Ink,
    surface = Tok.Surface,
    onSurface = Tok.Ink,
    surfaceVariant = Tok.Track,
    onSurfaceVariant = Tok.Muted,
    surfaceContainerLowest = Tok.Surface,
    surfaceContainerLow = Tok.Surface,
    surfaceContainer = Tok.Surface,
    surfaceContainerHigh = Tok.Surface,
    surfaceContainerHighest = Tok.Track,
    outline = Tok.FieldLine,
    outlineVariant = Tok.Line,
    error = Tok.RemainRed,
    onError = Color.White,
    errorContainer = Tok.DangerBg,
    onErrorContainer = Tok.Ink,
)

private val AppShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(10.dp),
    medium = RoundedCornerShape(12.dp),
    large = RoundedCornerShape(18.dp),
    extraLarge = RoundedCornerShape(24.dp),
)

@Composable
fun PCBlockTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = Light, typography = AppTypography, shapes = AppShapes, content = content)
}
