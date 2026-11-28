package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Цвета телефона (ТЗ 8.0 п. 3, 9): каждый текст на своём фоне проходит порог WCAG. Цвет остатка:
 * крупный (≥ 19 sp жирный) — 3:1, мельче — 4,5:1 (зелёный тона заменяется на «+» #0F7A45).
 */
class ToneContrastTest {
    private fun ok(fg: Long, bg: Long, min: Double) {
        val c = Palette.contrast(fg, bg)
        assertTrue("%08X на %08X: %.2f < %.1f".format(fg, bg, c, min), c >= min)
    }

    @Test
    fun knownValues() {
        assertEquals(21.0, Palette.contrast(0xFF000000, 0xFFFFFFFF), 0.01)
        assertEquals(4.40, Palette.contrast(Palette.REMAIN_GREEN, Palette.SURFACE), 0.01)
        assertEquals(4.81, Palette.contrast(Palette.PLUS_TEXT, Palette.PLUS_BG), 0.02) // ТЗ 8.0 п. 9 округляет до 4,81
    }

    @Test
    fun remainTones() {
        for (t in Remain.Tone.values()) {
            ok(Palette.remainText(t, large = true), Palette.SURFACE, 3.0)
            ok(Palette.remainText(t, large = false), Palette.SURFACE, 4.5)
        }
        assertEquals(Palette.REMAIN_GREEN, Palette.remainText(Remain.Tone.GREEN, large = true))
        assertEquals(Palette.PLUS_TEXT, Palette.remainText(Remain.Tone.GREEN, large = false))
        assertEquals(Palette.REMAIN_BLUE, Palette.remainText(Remain.Tone.BLUE, large = false))
        assertEquals(Palette.REMAIN_RED, Palette.remainText(Remain.Tone.RED, large = false))
        assertEquals(Palette.REMAIN_GREEN_BAR, Palette.remainBar(Remain.Tone.GREEN))
        assertEquals(Palette.REMAIN_BLUE, Palette.remainBar(Remain.Tone.BLUE))
        assertEquals(Palette.REMAIN_RED_BAR, Palette.remainBar(Remain.Tone.RED))
    }

    /** Крупный текст по WCAG: от 24 sp или от 18,66 sp жирным; компактная карточка — 19 sp жирным. */
    @Test
    fun largeThreshold() {
        assertTrue(Palette.isLarge(19f, bold = true))
        assertTrue(Palette.isLarge(40f, bold = true))
        assertTrue(Palette.isLarge(24f, bold = false))
        assertTrue(!Palette.isLarge(18f, bold = true))
        assertTrue(!Palette.isLarge(19f, bold = false))
    }

    @Test
    fun textOnBackgrounds() {
        for (bg in listOf(Palette.GROUND, Palette.SURFACE, Palette.TRACK)) {
            ok(Palette.INK, bg, 4.5)
            ok(Palette.MUTED, bg, 4.5)
        }
        ok(Palette.ACCENT, Palette.GROUND, 4.5)
        ok(Palette.ACCENT, Palette.ACCENT_SOFT, 4.5)
        ok(0xFFFFFFFF, Palette.ACCENT, 4.5)
        ok(Palette.PLUS_TEXT, Palette.PLUS_BG, 4.5)
        ok(Palette.MINUS_TEXT, Palette.MINUS_BG, 4.5)
        ok(Palette.GOLD_TEXT, Palette.GOLD_BG, 4.5)
        ok(Palette.PENDING_TEXT, Palette.PENDING_BG, 4.5)
        ok(Palette.REMAIN_RED, Palette.DANGER_BG, 4.5)
        ok(Palette.WAITING, Palette.SURFACE, 4.5)
        // Рамка поля ввода — не текст: 3:1 к поверхности (WCAG 1.4.11).
        ok(Palette.FIELD_LINE, Palette.SURFACE, 3.0)
    }
}
