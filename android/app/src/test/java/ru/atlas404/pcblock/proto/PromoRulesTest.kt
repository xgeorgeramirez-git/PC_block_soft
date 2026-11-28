package ru.atlas404.pcblock.proto

import org.json.JSONArray
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.fail
import org.junit.Test

/** Промокоды с телефона (FR-45, R26): те же правила и тексты, что окно (testdata/remote/promo-rules.json из Go). */
class PromoRulesTest {
    private val rules: JSONArray by lazy {
        val stream = PromoRulesTest::class.java.classLoader!!.getResourceAsStream("promo-rules.json")
            ?: error("promo-rules.json не найден среди ресурсов тестов")
        JSONArray(stream.bufferedReader(Charsets.UTF_8).readText())
    }

    @Test
    fun codeRulesMatchGo() {
        assertEquals(6, rules.length())
        for (i in 0 until rules.length()) {
            val r = rules.getJSONObject(i)
            val (code, problem) = PromoRules.checkCode(r.getString("in"))
            if (r.getBoolean("ok")) {
                assertEquals(r.getString("in"), r.getString("code"), code)
                assertNull(problem)
            } else {
                assertNull(code)
                assertEquals(r.getString("in"), r.getString("error"), Strings[problem!!.resName])
            }
        }
    }

    @Test
    fun codeEdges() {
        assertEquals("ABC" to null, PromoRules.checkCode("abc"))
        assertEquals("A1B2C3D4E5F6G7H8" to null, PromoRules.checkCode("a1b2c3d4e5f6g7h8"))
        assertEquals(Problem.PROMO_CODE_LATIN, PromoRules.checkCode("ﬁﬁﬁ").second)
        assertEquals(Problem.PROMO_CODE_LATIN, PromoRules.checkCode("AB-12").second)
        assertEquals(Problem.PROMO_CODE_EMPTY, PromoRules.checkCode("   ").second)
    }

    @Test
    fun numbers() {
        assertNull(PromoRules.checkMinutes("1"))
        assertNull(PromoRules.checkMinutes("600"))
        for (bad in listOf("", "0", "601", "x", "-1")) assertEquals(bad, "Можно от 1 до 600 минут", Strings[PromoRules.checkMinutes(bad)!!.resName])
        assertNull(PromoRules.checkCount("1"))
        assertNull(PromoRules.checkCount("50"))
        for (bad in listOf("", "0", "51")) assertEquals(bad, "Можно от 1 до 50 кодов", Strings[PromoRules.checkCount(bad)!!.resName])
        assertNull(PromoRules.checkLen("4"))
        assertNull(PromoRules.checkLen("12"))
        for (bad in listOf("", "3", "13")) assertEquals(bad, "Длина от 4 до 12 знаков", Strings[PromoRules.checkLen(bad)!!.resName])
    }

    /** Неверный вид полей телефон не шлёт (компьютер ответил бы range без траты seq). */
    @Test
    fun buildersRefuseWrongFields() {
        val bad: List<() -> Unit> = listOf(
            { Proto.promoCreateData("pc", "ph", 1, 1, "AB", 30) },
            { Proto.promoCreateData("pc", "ph", 1, 1, "SUMMER", 0) },
            { Proto.promoGenData("pc", "ph", 1, 1, 51, 6, Proto.CHARSET_MIXED, 15) },
            { Proto.promoGenData("pc", "ph", 1, 1, 5, 3, Proto.CHARSET_MIXED, 15) },
            { Proto.promoGenData("pc", "ph", 1, 1, 5, 6, "emoji", 15) },
            { Proto.promoGenData("pc", "ph", 1, 1, 5, 6, Proto.CHARSET_DIGITS, 601) },
            { Proto.promoDeleteData("pc", "ph", 1, 1, "") },
            { Proto.promoDeleteData("pc", "ph", 1, 1, "x".repeat(65)) },
            { Proto.decideData("pc", "ph", 1, 1, "", Proto.CAT_GAMES) },
            { Proto.decideData("pc", "ph", 1, 1, "k", "video") },
        )
        for ((i, b) in bad.withIndex()) {
            try {
                b()
                fail("ожидался отказ №$i")
            } catch (e: IllegalArgumentException) {
                // ожидаемо
            }
        }
        // Старый код с кириллицей (до 0.5.0) удаляется как есть (R-5).
        val del = String(Proto.promoDeleteData("pc", "ph", 1, 1, "ЁЛКА"), Charsets.UTF_8)
        assertEquals(true, del.endsWith(",\"op\":\"promoDelete\",\"code\":\"ЁЛКА\"}"))
        for (c in listOf(Proto.CHARSET_DIGITS, Proto.CHARSET_LETTERS, Proto.CHARSET_MIXED)) Proto.promoGenData("pc", "ph", 1, 1, 1, 4, c, 1)
        for (c in listOf(Proto.CAT_GAMES, Proto.CAT_STUDY, Proto.CAT_OTHER, Proto.CAT_BLOCKED)) Proto.decideData("pc", "ph", 1, 1, "k", c)
        assertEquals(listOf("digits", "letters", "mixed"), listOf(Proto.CHARSET_DIGITS, Proto.CHARSET_LETTERS, Proto.CHARSET_MIXED))
        assertEquals(listOf("games", "study", "other", "blocked"), listOf(Proto.CAT_GAMES, Proto.CAT_STUDY, Proto.CAT_OTHER, Proto.CAT_BLOCKED))
    }
}
