package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Совместимость отчёта (протокол, раздел 6; Review Focus 7): компьютер 0.8.3 не шлёт bankMin, pending,
 * games*, video* — телефон прячет копилку, промокоды и ждущие программы, карточка в старом виде.
 * Кривые строки pending пропускаются, сам отчёт принят.
 */
class ReportCompatTest {
    private val peer = Vectors.peer()

    private fun pcSigned(json: String) = Crypto.seal(SoftSigner.pc(), json.toByteArray(Charsets.UTF_8))

    private fun report(body: String) =
        "{\"v\":1,\"t\":\"report\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"ts\":1763490012000,$body}"

    @Test
    fun oldReportHasNoBankAndNoPending() {
        for (name in listOf("report", "reportForbid", "reportToday", "reportNotes")) {
            val rep = Proto.openReport(peer, Vectors.signed(name), lastShownTs = 0)
            assertFalse(name, rep.hasBank)
            assertTrue(name, rep.pending.isEmpty())
            assertEquals(name, 0, rep.pendingCount)
            for (c in rep.children) {
                assertNull(name, c.bankMin)
                assertNull(name, c.gamesSec)
                assertNull(name, c.gamesLimSec)
                assertNull(name, c.videoSec)
                assertNull(name, c.videoLimSec)
            }
        }
    }

    @Test
    fun oldCmdReplyReportHasNoBank() {
        val cmd = Vectors.s("cmdJson").toByteArray(Charsets.UTF_8)
        val r = Proto.openCmdReply(peer, Vectors.signed("cmdr"), cmd, lastShownTs = 0)
        assertFalse(r.report!!.hasBank)
        assertNull(r.promos)
        assertTrue(r.codes.isEmpty())
    }

    /** Компьютер 0.8.4 без детей: pendingCount есть — копилки нет, но «Промокоды» доступны. */
    @Test
    fun newReportWithoutChildrenStillNew() {
        val rep = Proto.openReport(peer, pcSigned(report("\"children\":[],\"pending\":[],\"pendingCount\":0")), 0)
        assertTrue(rep.hasBank)
        val old = Proto.openReport(peer, pcSigned(report("\"children\":[]")), 0)
        assertFalse(old.hasBank)
    }

    /**
     * Ловит (ревью 0.8.4 M3): до первого отчёта (report == null) вкладка «Промокоды» была видна и слала
     * promoList; компьютер 0.8.3 отвечал range — текст ошибки вместо «Обновите программу на компьютере».
     * Вкладка и запрос списка — только когда отчёт пришёл и компьютер 0.8.4 и новее.
     */
    @Test
    fun promosOnlyAfterNewReport() {
        assertFalse(Proto.newPcTabs(null))
        assertFalse(Proto.newPcTabs(Proto.openReport(peer, pcSigned(report("\"children\":[]")), 0)))
        assertFalse(Proto.newPcTabs(Proto.openReport(peer, Vectors.signed("report"), lastShownTs = 0)))
        assertTrue(Proto.newPcTabs(Proto.openReport(peer, pcSigned(report("\"children\":[],\"pending\":[],\"pendingCount\":0")), 0)))
    }

    /** Один ребёнок без bankMin (смешанный отчёт) — считаем компьютер старым. */
    @Test
    fun mixedChildrenMeanOld() {
        val rep = Proto.openReport(
            peer,
            pcSigned(
                report(
                    "\"children\":[{\"id\":3,\"name\":\"Миша\",\"usedSec\":0,\"leftSec\":60,\"bankMin\":5}," +
                        "{\"id\":4,\"name\":\"Коля\",\"usedSec\":0,\"leftSec\":60}],\"pending\":[],\"pendingCount\":0",
                ),
            ),
            0,
        )
        assertFalse(rep.hasBank)
    }

    @Test
    fun brokenPendingRowsAreSkipped() {
        val rows = listOf(
            "1",
            "{\"name\":\"Без id\",\"kids\":[],\"tries\":1,\"lastMs\":5}",
            "{\"id\":\"\",\"name\":\"Пустой id\"}",
            "{\"id\":\"k2\",\"name\":\"Game\",\"pub\":\"Valve\",\"path\":\"C:\\\\g.exe\"," +
                "\"kids\":[{\"id\":3,\"tries\":2},\"x\",{\"tries\":1},{\"id\":4,\"tries\":1}],\"tries\":3,\"lastMs\":7}",
        ).joinToString(",")
        val rep = Proto.openReport(
            peer,
            pcSigned(report("\"children\":[{\"id\":3,\"name\":\"Миша\",\"bankMin\":0}],\"pending\":[$rows],\"pendingCount\":5")),
            0,
        )
        assertEquals(
            listOf(Proto.Pending("k2", "Game", "Valve", "C:\\g.exe", listOf(Proto.PendingKid(3, 2), Proto.PendingKid(4, 1)), 3, 7)),
            rep.pending,
        )
        assertEquals(5, rep.pendingCount)
    }

    /** pending не массив — ждущих нет, отчёт принят. */
    @Test
    fun pendingNotArray() {
        val rep = Proto.openReport(
            peer,
            pcSigned(report("\"children\":[{\"id\":3,\"name\":\"Миша\",\"bankMin\":0}],\"pending\":{},\"pendingCount\":1")),
            0,
        )
        assertTrue(rep.pending.isEmpty())
        assertEquals(1, rep.pendingCount)
        assertEquals(0, rep.children.single().bankMin)
    }

    /** Ответ ok:true без promos (список не прочитался) — promos = null: телефон держит прежний кэш. */
    @Test
    fun promoReplyWithoutList() {
        val cmd = Vectors.s("cmdPromoListJson").toByteArray(Charsets.UTF_8)
        val body = "{\"v\":1,\"t\":\"cmdr\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"seq\":22," +
            "\"cmd\":\"${Vectors.s("cmdPromoListHash")}\",\"ts\":1763490007500,\"ok\":true,\"message\":\"\"}"
        val r = Proto.openCmdReply(peer, pcSigned(body), cmd, 0)
        assertTrue(r.ok)
        assertNull(r.promos)
        assertEquals(1763490007500, r.ts)
    }

    /** Отказ not_found со списком: список читается и при ok:false. Кривые строки списка пропускаются. */
    @Test
    fun promoRejectCarriesList() {
        val cmd = Vectors.s("cmdPromoDeleteJson").toByteArray(Charsets.UTF_8)
        val body = "{\"v\":1,\"t\":\"cmdr\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"seq\":25," +
            "\"cmd\":\"${Vectors.s("cmdPromoDeleteHash")}\",\"ts\":1763490010500,\"ok\":false,\"error\":\"not_found\"," +
            "\"lastSeq\":25,\"promos\":{\"active\":[{\"code\":\"WINTER\",\"min\":15,\"createdMs\":9},{\"min\":5},7]," +
            "\"used\":[{\"code\":\"OLD\",\"min\":10,\"usedMs\":8,\"child\":4,\"childName\":\"\"}],\"moreActive\":2,\"moreUsed\":3}}"
        val r = Proto.openCmdReply(peer, pcSigned(body), cmd, 0)
        assertFalse(r.ok)
        assertEquals("not_found", r.error)
        assertEquals(Problem.PROMO_GONE, Problem.fromCmd(r.error, Proto.OP_PROMO_DELETE))
        assertEquals(
            Proto.PromoList(listOf(Proto.PromoActive("WINTER", 15, 9)), listOf(Proto.PromoUsed("OLD", 10, 8, 4, "")), 2, 3),
            r.promos,
        )
    }

    /** Кэш списка промокодов: что сохранили, то и прочитали; кривая строка — кэша нет. */
    @Test
    fun promoListRoundTrip() {
        val list = Proto.PromoList(
            active = listOf(Proto.PromoActive("SUMMER", 30, 1763490000000), Proto.PromoActive("ЁЛКА", 5, 1)),
            used = listOf(Proto.PromoUsed("AB12", 30, 1763480000000, 3, "Миша \"Мишутка\"")),
            moreActive = 1,
            moreUsed = 0,
        )
        assertEquals(list, Proto.parsePromos(Proto.promosJson(list)))
        assertNull(Proto.parsePromos(""))
        assertNull(Proto.parsePromos("[1,2]"))
        assertNull(Proto.parsePromos("{битый"))
    }
}
