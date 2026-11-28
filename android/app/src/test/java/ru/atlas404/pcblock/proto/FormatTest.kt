package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalDateTime
import java.time.ZoneId

/** Тексты экранов: шаблоны берутся из того же strings.xml, что и в приложении. */
class FormatTest {
    private fun dur(sec: Long) = Format.duration(sec, Strings["dur_min"], Strings["dur_hm"])

    @Test
    fun durations() {
        assertEquals("1 ч 20 мин", dur(80 * 60))
        assertEquals("40 мин", dur(40 * 60 + 59))
        assertEquals("1 ч 05 мин", dur(65 * 60))
        assertEquals("0 мин", dur(59))
        assertEquals("0 мин", dur(-30))
        assertEquals("Сегодня за компьютером 1 ч 20 мин", Strings["child_used"].format(dur(4800)))
        assertEquals("Осталось 40 мин", Strings["child_left"].format(dur(2400)))
    }

    @Test
    fun offlineSince() {
        val zone = ZoneId.of("Europe/Moscow")
        val seen = LocalDateTime.of(2026, 11, 18, 19, 5).atZone(zone).toInstant().toEpochMilli()
        val now = seen + 30 * 60_000
        val hm = Format.clock(seen, now, zone, Strings["time_other_day"])
        assertEquals("Компьютер не на связи с 19:05", Strings["pc_offline_since"].format(hm))
        assertEquals("Компьютер не на связи с 19:05. Команда не отправлена", Strings["err_pc_offline_since"].format(hm))
        val nextDay = seen + 24 * 3600_000L
        assertEquals("18.11 19:05", Format.clock(seen, nextDay, zone, Strings["time_other_day"]))
    }

    /** «25 ноября» для промокодов и «Ждут решения» (8.3.10 п. 1), местное время телефона. */
    @Test
    fun dayMonth() {
        val zone = ZoneId.of("Europe/Moscow")
        val months = Strings.array("months_gen")
        assertEquals(12, months.size)
        val ms = LocalDateTime.of(2026, 11, 25, 0, 30).atZone(zone).toInstant().toEpochMilli()
        assertEquals("25 ноября", Format.dayMonth(ms, zone, months))
        val jan = LocalDateTime.of(2027, 1, 1, 23, 59).atZone(zone).toInstant().toEpochMilli()
        assertEquals("1 января", Format.dayMonth(jan, zone, months))
        assertEquals("использован 25 ноября в 00:30, Миша", Strings["promo_used"].format("25 ноября", "00:30", "Миша"))
        // Граница суток — по поясу телефона: 23:59 и 00:00 — разные дни.
        val late = LocalDateTime.of(2026, 11, 25, 23, 59).atZone(zone).toInstant().toEpochMilli()
        assertTrue(Format.sameDay(ms, late, zone))
        assertTrue(!Format.sameDay(late, late + 60_000, zone))
    }

    @Test
    fun onlineWindow() {
        assertEquals(Format.Link.Online, Format.link(1_000_000, 1_000_000 - 60_000))
        assertEquals(Format.Link.Offline(1_000_000 - 60_001), Format.link(1_000_000, 1_000_000 - 60_001))
        assertEquals(Format.Link.Offline(0), Format.link(1_000_000, 0))
    }

    @Test
    fun serverAddress() {
        assertEquals("https://pcblock.atlas404.ru", Format.baseUrl(" https://pcblock.atlas404.ru/ "))
        assertEquals("https://example.org/relay", Format.baseUrl("https://example.org/relay"))
        assertNull(Format.baseUrl("http://pcblock.atlas404.ru"))
        assertNull(Format.baseUrl("pcblock.atlas404.ru"))
        assertNull(Format.baseUrl("https://"))
        assertNull(Format.baseUrl("https://a.ru/?x=1"))
        assertNull(Format.baseUrl("https://user@a.ru"))
    }

    @Test
    fun codeAndMinutes() {
        assertEquals("123456", Format.pairCode("123 456"))
        assertNull(Format.pairCode("12345"))
        assertNull(Format.pairCode("12345a"))
        assertEquals(240, Format.minutes("240"))
        assertNull(Format.minutes("0"))
        assertNull(Format.minutes("241"))
        assertNull(Format.minutes(""))
    }

    @Test
    fun b64IsStrictLikeGo() {
        assertEquals("AA", B64.enc(byteArrayOf(0)))
        assertEquals(1, B64.dec("AA").size)
        for (bad in listOf("AA==", "A+", "A/", "AB", "A", "A A")) {
            try {
                B64.dec(bad)
                throw AssertionError("принят $bad")
            } catch (e: ProtoException) {
                // ожидаемо
            }
        }
    }

    @Test
    fun phoneName() {
        assertEquals("Pixel 7", Proto.cleanName("  Pixel\u0000 \n 7 "))
        assertEquals(40, Proto.cleanName("x".repeat(100)).length)
    }

    @Test
    fun everyProblemHasText() {
        for (p in Problem.values()) assertTrue(p.resName, Strings.all.containsKey(p.resName))
        assertEquals("Команда устарела, повторите", Strings[Problem.fromCmd("stale").resName])
        assertEquals("Можно от 1 до 240 минут", Strings[Problem.fromCmd("range").resName])
        assertEquals("Ребёнок не найден", Strings[Problem.fromCmd("no_child").resName])
        assertEquals(Problem.NOT_PAIRED, Problem.fromCmd("bad_sig"))
        assertEquals(Problem.PC_OFFLINE_PAIR, Problem.fromRelay("pc_offline", pairing = true))
    }

    @Test
    fun textsHaveNoLongDashes() {
        for ((k, v) in Strings.all) assertTrue("$k: $v", '—' !in v && '–' !in v)
    }

    /** «Копилка Миши» (FR-44): те же правила, что genitiveName окна (frontend/src/lib/format.ts), та же таблица. */
    @Test
    fun genitive() {
        val cases = listOf(
            "Миша" to "Миши", "Даша" to "Даши", "Маша" to "Маши", "Анна" to "Анны", "Ольга" to "Ольги",
            "Ника" to "Ники", "Катя" to "Кати", "Ваня" to "Вани", "Мария" to "Марии", "Иван" to "Ивана",
            "Артём" to "Артёма", "Андрей" to "Андрея", "Игорь" to "Игоря", "Дмитрий" to "Дмитрия",
            "Тимофей" to "Тимофея", "Женя" to "Жени", "Саша" to "Саши", "Tom" to "Tom",
            "Лиза Петрова" to "Лиза Петрова", "" to "", "Аня" to "Ани", "Никита" to "Никиты", "John" to "John",
            " Миша " to "Миши", "Анна-Мария" to "Анна-Марии",
        )
        for ((name, want) in cases) assertEquals(name, want, Format.genitive(name))
    }
}
