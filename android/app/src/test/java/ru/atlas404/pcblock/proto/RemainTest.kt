package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Test

/** Цвет и число остатка (ТЗ 8.0 п. 9, Review Focus 6): одна функция округления вверх для числа и цвета. */
class RemainTest {
    @Test
    fun toneBorders() {
        val sec = listOf(0L, 1L, 900L, 901L, 960L, 3600L, 3601L, 3660L, -1L)
        val want = listOf(
            Remain.Tone.RED, Remain.Tone.RED, Remain.Tone.RED,
            Remain.Tone.BLUE, Remain.Tone.BLUE, Remain.Tone.BLUE,
            Remain.Tone.GREEN, Remain.Tone.GREEN, Remain.Tone.GREEN,
        )
        assertEquals(want, sec.map(Remain::tone))
    }

    @Test
    fun minutesRoundUp() {
        assertEquals(listOf(0L, 1L, 15L, 16L, 16L, 60L, 61L, 61L, 0L), listOf(0L, 1L, 900L, 901L, 960L, 3600L, 3601L, 3660L, -1L).map(Remain::remainMinutes))
    }

    @Test
    fun hm() {
        assertEquals("1:25", Remain.hm(85))
        assertEquals("0:05", Remain.hm(5))
        assertEquals("2:00", Remain.hm(120))
        assertEquals("0:00", Remain.hm(0))
        assertEquals("0:00", Remain.hm(-3))
        assertEquals("10:00", Remain.hm(600))
    }
}
