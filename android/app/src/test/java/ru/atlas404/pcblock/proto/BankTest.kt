package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.fail
import org.junit.Test

/** Копилка с телефона (FR-44, D-113): за раз 1…600, предела баланса нет; неверное не отправляется. */
class BankTest {
    private val put = Format.Plural(Strings["bank_put_one"], Strings["bank_put_few"], Strings["bank_put_many"])
    private fun label(raw: String) = BankRules.putLabel(raw, put, Strings["bank_put"])

    @Test
    fun minutesRange() {
        for (ok in listOf("1", "30", "600", " 45 ", "0600")) assertNull(ok, BankRules.checkBankMinutes(ok))
        for (bad in listOf("", " ", "0", "601", "-5", "+5", "1.5", "abc", "99999999999", "٣")) {
            assertEquals(bad, Problem.BANK_RANGE, BankRules.checkBankMinutes(bad))
        }
        assertEquals("Можно от 1 до 600 минут за раз", Strings[Problem.BANK_RANGE.resName])
        assertEquals(600, Proto.MAX_BANK_ADD)
    }

    @Test
    fun putLabels() {
        assertEquals("Положить 30 минут", label("30"))
        assertEquals("Положить 1 минуту", label("1"))
        assertEquals("Положить 2 минуты", label("2"))
        assertEquals("Положить 5 минут", label("5"))
        assertEquals("Положить 11 минут", label("11"))
        assertEquals("Положить 21 минуту", label("21"))
        assertEquals("Положить 22 минуты", label("22"))
        assertEquals("Положить 112 минут", label("112"))
        assertEquals("Положить", label(""))
        assertEquals("Положить", label("0"))
        assertEquals("Положить", label("601"))
    }

    @Test
    fun bankAddDataRefusesOutOfRange() {
        for (bad in listOf(0, -1, 601)) {
            try {
                Proto.bankAddData("pc", "ph", 1, 1, 3, bad)
                fail("ожидался отказ для $bad")
            } catch (e: IllegalArgumentException) {
                // ожидаемо: телефон такую команду не шлёт
            }
        }
        Proto.bankAddData("pc", "ph", 1, 1, 3, 600)
    }
}
