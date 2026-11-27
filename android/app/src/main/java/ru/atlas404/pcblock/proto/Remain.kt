package ru.atlas404.pcblock.proto

/**
 * Остаток на карточке ребёнка (ТЗ 8.0 п. 9): число и цвет считаются от одних минут, округлённых вверх
 * (как remainMinutes окна и core.RemainingMinutes), чтобы 15/16 и 60/61 совпадали в подписи и в цвете.
 */
object Remain {
    enum class Tone { GREEN, BLUE, RED }

    /** Минуты остатка вверх: 1 с — 1 мин, 901 с — 16 мин; 0 и меньше — 0. */
    fun remainMinutes(leftSec: Long): Long = if (leftSec <= 0) 0 else (leftSec + 59) / 60

    /** Без лимита (leftSec < 0) и больше 60 мин — зелёный; 16…60 — синий; 15 и меньше — красный. */
    fun tone(leftSec: Long): Tone {
        if (leftSec < 0) return Tone.GREEN
        val m = remainMinutes(leftSec)
        return when {
            m > 60 -> Tone.GREEN
            m > 15 -> Tone.BLUE
            else -> Tone.RED
        }
    }

    /** «1:25» — часы и минуты для крупного числа карточки. */
    fun hm(min: Long): String {
        val m = maxOf(0L, min)
        return "${m / 60}:${(m % 60).toString().padStart(2, '0')}"
    }
}
