package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.ZoneId

/** «Статистика за сегодня» на телефоне (протокол, раздел 6, поле today; FR-43, S59). */
class TodayTest {
    private val peer = Vectors.peer()

    private fun pcSigned(json: String) = Crypto.seal(SoftSigner.pc(), json.toByteArray(Charsets.UTF_8))

    private fun goToday() = Proto.openReport(peer, Vectors.signed("reportToday"), 0)

    @Test
    fun goReportTodayParsed() {
        val rep = goToday()
        assertEquals(1763490006000, rep.ts)
        val by = rep.children.associateBy { it.id }
        val t = by[3]!!.today!!
        assertEquals(1763463600000, t.firstMs)
        assertTrue(t.nowAt)
        assertEquals(1763485200000, t.sinceMs)
        assertEquals(5400L, t.usedSec)
        assertEquals(1800L, t.leftSec)
        assertEquals(2700L, t.gamesSec)
        assertEquals(1200L, t.studySec)
        assertEquals(900L, t.videoSec)
        assertEquals(
            listOf(
                Proto.Item("Minecraft", "game", 2700),
                Proto.Item("GeoGebra Classic", "study", 1200),
                Proto.Item("Блокнот", "other", 300),
            ),
            t.programs,
        )
        assertEquals(listOf(Proto.Item("youtube.com", "video", 900), Proto.Item("uchi.ru", "study", 240)), t.sites)
        assertEquals(0, t.morePrograms)
        assertEquals(3, t.moreSites)
        assertEquals(2, t.violations)
        assertEquals(1, t.bypass)
        assertEquals(rep.ts, t.asOfMs) // «данные на HH:MM» — время отчёта

        val k = by[4]!!.today!!
        assertEquals(0L, k.firstMs)
        assertTrue(!k.nowAt)
        assertEquals(emptyList<Proto.Item>(), k.programs)
        assertEquals(emptyList<Proto.Item>(), k.sites)
        // У Даши today нет (не поместилось или старый компьютер): кнопки статистики нет.
        assertNull(by[5]!!.today)
    }

    @Test
    fun brokenTodayDoesNotBreakReport() {
        val json = "{\"v\":1,\"t\":\"report\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"ts\":7," +
            "\"children\":[{\"id\":3,\"name\":\"Миша\",\"today\":\"много\"}," +
            "{\"id\":4,\"name\":\"Коля\",\"today\":{\"usedSec\":60,\"programs\":[{\"name\":\"Roblox\",\"cat\":\"game\",\"sec\":60},7,{\"sec\":5}]}}]}"
        val rep = Proto.openReport(peer, pcSigned(json), 0)
        assertNull(rep.children[0].today)
        val t = rep.children[1].today!!
        assertEquals(listOf(Proto.Item("Roblox", "game", 60)), t.programs)
        assertEquals(emptyList<Proto.Item>(), t.sites)
        assertEquals(60L, t.usedSec)
        assertEquals(0, t.violations)
    }

    // ---------- today не затирается отчётом из ответа на команду ----------

    @Test
    fun cmdrReportKeepsTodayFromSeparateReport() {
        val prev = goToday()
        val nfBytes = Vectors.s("cmdNoForbiddenJson").toByteArray(Charsets.UTF_8)
        val fresh = Proto.openCmdReply(peer, Vectors.signed("cmdrNoForbidden"), nfBytes, 0).report!!
        assertNull(fresh.children.first { it.id == 3L }.today) // в cmdr today не едет
        val merged = Proto.keepToday(fresh, prev)
        val misha = merged.children.first { it.id == 3L }
        // Свежее из ответа (запрет снят, остаток) + today из последнего отдельного отчёта со своим временем.
        assertEquals(Proto.Forbid.CLEARED, misha.forbid)
        assertEquals(prev.children.first { it.id == 3L }.today, misha.today)
        assertEquals(1763490006000, misha.today!!.asOfMs)
        assertEquals(fresh.ts, merged.ts)
        // Петя (6) в прежнем отчёте не было: статистики нет.
        assertNull(merged.children.first { it.id == 6L }.today)
        // Без прежнего отчёта — как есть.
        assertEquals(fresh, Proto.keepToday(fresh, null))
    }

    @Test
    fun freshTodayWins() {
        val prev = goToday()
        val json = "{\"v\":1,\"t\":\"report\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"ts\":1763490009000," +
            "\"children\":[{\"id\":3,\"name\":\"Миша\",\"today\":{\"usedSec\":6000,\"programs\":[],\"sites\":[]}}]}"
        val fresh = Proto.openReport(peer, pcSigned(json), 0)
        val t = Proto.keepToday(fresh, prev).children.single().today!!
        assertEquals(6000L, t.usedSec)
        assertEquals(1763490009000, t.asOfMs)
    }

    // ---------- «Во что играл» и списки ----------

    @Test
    fun gamesFirstSortedThenOthers() {
        val json = "{\"v\":1,\"t\":\"report\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"ts\":8," +
            "\"children\":[{\"id\":3,\"name\":\"Миша\",\"today\":{\"gamesSec\":4000,\"programs\":[" +
            "{\"name\":\"Блокнот\",\"cat\":\"other\",\"sec\":100}," +
            "{\"name\":\"Roblox\",\"cat\":\"game\",\"sec\":600}," +
            "{\"name\":\"Word\",\"cat\":\"study\",\"sec\":900}," +
            "{\"name\":\"Minecraft\",\"cat\":\"game\",\"sec\":3000}],\"sites\":[" +
            "{\"name\":\"uchi.ru\",\"cat\":\"study\",\"sec\":50}," +
            "{\"name\":\"poki.com\",\"cat\":\"game\",\"sec\":400}," +
            "{\"name\":\"youtube.com\",\"cat\":\"video\",\"sec\":700}]}}]}"
        val t = Proto.openReport(peer, pcSigned(json), 0).children.single().today!!
        val v = TodayView.of(t)
        assertEquals(listOf("Minecraft", "Roblox", "poki.com"), v.games.map { it.name })
        assertEquals(listOf("Word", "Блокнот"), v.programs.map { it.name })
        assertEquals(listOf("youtube.com", "uchi.ru"), v.sites.map { it.name })
    }

    @Test
    fun spentTexts() {
        fun s(sec: Long) = Format.spent(sec, Strings["dur_min"], Strings["dur_hm"], Strings["dur_less_min"])
        assertEquals("1 ч 20 мин", s(80 * 60))
        assertEquals("15 мин", s(15 * 60 + 59))
        assertEquals("меньше минуты", s(59))
        assertEquals("меньше минуты", s(1))
        assertEquals("0 мин", s(0))
        assertEquals("2 ч 05 мин", s(125 * 60))
    }

    @Test
    fun statsTexts() {
        val zone = ZoneId.of("Europe/Moscow")
        val rep = goToday()
        val t = rep.children.first { it.id == 3L }.today!!
        fun hm(ms: Long) = Format.clock(ms, rep.ts, zone, Strings["time_other_day"])
        assertEquals("За компьютером с 20:00", Strings["stats_at_pc_since"].format(hm(t.sinceMs)))
        assertEquals("Не за компьютером с 18:33", Strings["stats_away_since"].format(hm(1763480000000)))
        assertEquals("Первый вход сегодня в 14:00", Strings["stats_first"].format(hm(t.firstMs)))
        assertEquals("Сегодня входа ещё не было", Strings["stats_first_none"])
        assertEquals("Данные на 21:20", Strings["data_at"].format(hm(t.asOfMs)))
        assertEquals("Нарушений сегодня: 2 (из них попыток обхода: 1)", Strings["stats_violations_bypass"].format(2, 1))
        assertEquals("Нарушений сегодня: 0", Strings["stats_violations"].format(0))
        assertEquals("Ещё 3", Strings["stats_more"].format(3))
        assertEquals("Статистика за сегодня", Strings["btn_stats"])
        assertEquals("Во что играл", Strings["stats_games"])
        assertNotNull(Strings["stats_no_games"])
    }
}
