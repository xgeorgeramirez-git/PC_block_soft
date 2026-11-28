package ru.atlas404.pcblock.proto

/**
 * Цвета телефона (ТЗ 8.0 п. 3, 9) как ARGB-числа: ui/Tokens.kt берёт их отсюда, а JVM-тест
 * (ToneContrastTest) проверяет контраст каждой пары текст/фон по WCAG.
 */
object Palette {
    const val GROUND = 0xFFF3F5F9
    const val SURFACE = 0xFFFFFFFF
    const val LINE = 0xFFDDE3EE
    const val INK = 0xFF1A2238
    const val MUTED = 0xFF5B6680
    const val ACCENT = 0xFF2552D9
    const val ACCENT_SOFT = 0xFFE6EDFF

    /** Дорожка полосы остатка и фон переключателя вкладок. */
    const val TRACK = 0xFFE3E8F2

    /** Рамка поля ввода в покое: 3:1 к поверхности (у карточек — светлая LINE). */
    const val FIELD_LINE = 0xFF7A849C

    const val REMAIN_GREEN = 0xFF138A4F
    const val REMAIN_GREEN_BAR = 0xFF1FA463
    const val REMAIN_BLUE = 0xFF2552D9
    const val REMAIN_RED = 0xFFC81E1E
    const val REMAIN_RED_BAR = 0xFFE5484D

    const val PLUS_TEXT = 0xFF0F7A45
    const val PLUS_BG = 0xFFE7F5EC
    const val MINUS_TEXT = 0xFFC2410C
    const val MINUS_BG = 0xFFFCEDE6

    const val COIN = 0xFFFFC53A
    const val COIN_EDGE = 0xFFB87800
    const val COIN_HAND = 0xFF8A5A00
    const val GOLD_TEXT = 0xFF5A3B00
    const val GOLD_BG = 0xFFFFE7AD

    const val PENDING_BG = 0xFFFFF6E0
    const val PENDING_LINE = 0xFFF3D99A
    const val PENDING_TEXT = 0xFF5A3B00
    const val PENDING_ICON = 0xFF8A5A00

    const val DANGER_BG = 0xFFFCE8E8
    const val DANGER_LINE = 0xFFF3C4C4

    /** «Ждёт: компьютер не в сети» у сообщения (макет phones_more.py). */
    const val WAITING = 0xFFB45309

    /** Фон аватара по порядку ребёнка. */
    val AVATARS = listOf(0xFFFFE7AD, 0xFFD6E4FF, 0xFFDDF3E4)

    /** Крупный текст WCAG: от 24 sp или от 18,66 sp (14 pt) жирным. */
    fun isLarge(sp: Float, bold: Boolean): Boolean = sp >= 24f || (bold && sp >= 18.66f)

    /** Цвет числа остатка: крупный — цвет тона; мельче — тон с контрастом ≥ 4,5 (зелёный — как «+»). */
    fun remainText(tone: Remain.Tone, large: Boolean): Long = when (tone) {
        Remain.Tone.GREEN -> if (large) REMAIN_GREEN else PLUS_TEXT
        Remain.Tone.BLUE -> REMAIN_BLUE
        Remain.Tone.RED -> REMAIN_RED
    }

    fun remainBar(tone: Remain.Tone): Long = when (tone) {
        Remain.Tone.GREEN -> REMAIN_GREEN_BAR
        Remain.Tone.BLUE -> REMAIN_BLUE
        Remain.Tone.RED -> REMAIN_RED_BAR
    }

    private fun channel(v: Long, shift: Int): Double {
        val c = ((v shr shift) and 0xFF).toDouble() / 255.0
        return if (c <= 0.03928) c / 12.92 else Math.pow((c + 0.055) / 1.055, 2.4)
    }

    fun luminance(argb: Long): Double = 0.2126 * channel(argb, 16) + 0.7152 * channel(argb, 8) + 0.0722 * channel(argb, 0)

    fun contrast(a: Long, b: Long): Double {
        val la = luminance(a)
        val lb = luminance(b)
        return (maxOf(la, lb) + 0.05) / (minOf(la, lb) + 0.05)
    }
}
