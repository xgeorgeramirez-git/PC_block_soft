package ru.atlas404.pcblock.ui

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import ru.atlas404.pcblock.proto.Palette
import ru.atlas404.pcblock.proto.Remain

/**
 * Взрослый вид телефона (ТЗ 8.0 п. 1, 3, 9; D-109): всегда светлый фон, белые карточки с линией 1 dp,
 * без градиентов и теней. Числа цветов — proto/Palette.kt (контраст проверяет ToneContrastTest).
 */
object Tok {
    val Ground = Color(Palette.GROUND)
    val Surface = Color(Palette.SURFACE)
    val Line = Color(Palette.LINE)
    val Ink = Color(Palette.INK)
    val Muted = Color(Palette.MUTED)
    val Accent = Color(Palette.ACCENT)
    val AccentSoft = Color(Palette.ACCENT_SOFT)
    val Track = Color(Palette.TRACK)
    val FieldLine = Color(Palette.FIELD_LINE)

    val RemainGreen = Color(Palette.REMAIN_GREEN)
    val RemainRed = Color(Palette.REMAIN_RED)

    val PlusText = Color(Palette.PLUS_TEXT)
    val PlusBg = Color(Palette.PLUS_BG)
    val MinusText = Color(Palette.MINUS_TEXT)
    val MinusBg = Color(Palette.MINUS_BG)

    val Coin = Color(Palette.COIN)
    val CoinEdge = Color(Palette.COIN_EDGE)
    val CoinHand = Color(Palette.COIN_HAND)
    val GoldText = Color(Palette.GOLD_TEXT)
    val GoldBg = Color(Palette.GOLD_BG)

    val PendingBg = Color(Palette.PENDING_BG)
    val PendingLine = Color(Palette.PENDING_LINE)
    val PendingText = Color(Palette.PENDING_TEXT)
    val PendingIcon = Color(Palette.PENDING_ICON)

    val DangerBg = Color(Palette.DANGER_BG)
    val DangerLine = Color(Palette.DANGER_LINE)
    val Waiting = Color(Palette.WAITING)

    /** Скругления: кнопки 10–12 dp, карточки 18 dp. */
    val ChipShape = RoundedCornerShape(10.dp)
    val ButtonShape = RoundedCornerShape(12.dp)
    val CardShape = RoundedCornerShape(18.dp)

    /** Цвет числа остатка: large — крупный текст WCAG (Palette.isLarge), иначе тон с контрастом ≥ 4,5. */
    fun remainText(tone: Remain.Tone, large: Boolean): Color = Color(Palette.remainText(tone, large))

    fun remainBar(tone: Remain.Tone): Color = Color(Palette.remainBar(tone))

    /** Фон аватара-буквы по порядку ребёнка. */
    fun avatar(index: Int): Color = Color(Palette.AVATARS[Math.floorMod(index, Palette.AVATARS.size)])
}
