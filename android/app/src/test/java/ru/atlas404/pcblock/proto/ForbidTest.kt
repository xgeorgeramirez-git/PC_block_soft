package ru.atlas404.pcblock.proto

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.ZoneId

/** Кнопки времени и «Снять запрет на сегодня» с телефона (протокол, раздел 5б; FR-42, S58). */
class ForbidTest {
    private val peer = Vectors.peer()
    private val nfBytes = Vectors.s("cmdNoForbiddenJson").toByteArray(Charsets.UTF_8)

    private fun pcSigned(json: String) = Crypto.seal(SoftSigner.pc(), json.toByteArray(Charsets.UTF_8))

    // ---------- команда noForbidden ----------

    @Test
    fun noForbiddenDataMatchesGoByteForByte() {
        val data = Proto.noForbiddenData(Vectors.PC, Vectors.PHONE, seq = 19, ts = 1763490004000, child = 3)
        assertArrayEquals(nfBytes, data)
        assertEquals(Vectors.signed("cmdNoForbidden").data, B64.enc(data))
        assertEquals(Vectors.s("cmdNoForbiddenHash"), Crypto.hashData(data))
        // Подпись Go проверяется ключом телефона; Kotlin подписывает те же байты своим ключом.
        assertArrayEquals(nfBytes, Crypto.open(Crypto.parsePublicB64(Vectors.s("phonePub")), Vectors.signed("cmdNoForbidden")))
        val s = Crypto.seal(SoftSigner.phone(), data)
        assertArrayEquals(nfBytes, Crypto.open(Crypto.parsePublic(SoftSigner.phone().publicDer), s))
        // Поля delta в команде нет.
        assertFalse(org.json.JSONObject(String(data, Charsets.UTF_8)).has("delta"))
    }

    @Test
    fun oldCommandBytesUnchanged() {
        // «+/−» без op: те же байты, что вектор Go раздела 5 (старый компьютер понимает).
        val data = Proto.commandData(Vectors.PC, Vectors.PHONE, seq = 17, ts = 1763490000000, child = 3, delta = 15)
        assertArrayEquals(Vectors.s("cmdJson").toByteArray(Charsets.UTF_8), data)
        assertFalse(String(data, Charsets.UTF_8).contains("\"op\""))
    }

    @Test
    fun allQuickButtonsAreValidDeltas() {
        assertEquals(listOf(15, 30, 60), Proto.QUICK_ADD)
        for (d in Proto.QUICK_ADD.flatMap { listOf(it, -it) }) {
            val o = org.json.JSONObject(String(Proto.commandData("p", "f", 1, 1, 3, d), Charsets.UTF_8))
            assertEquals(d, o.getInt("delta"))
        }
    }

    // ---------- ответ cmdr ----------

    @Test
    fun goCmdrNoForbiddenOpens() {
        val r = Proto.openCmdReply(peer, Vectors.signed("cmdrNoForbidden"), nfBytes, lastShownTs = 0)
        assertTrue(r.ok)
        assertEquals("Запрет на сегодня снят", r.message)
        val rep = r.report!!
        assertEquals(Proto.Forbid.CLEARED, rep.children.first { it.id == 3L }.forbid)
        // Ответ на другую команду (байты «+15») не принимается.
        try {
            Proto.openCmdReply(peer, Vectors.signed("cmdrNoForbidden"), Vectors.s("cmdJson").toByteArray(), 0)
            throw AssertionError("принят ответ на чужую команду")
        } catch (e: ProtoException) {
            // ожидаемо
        }
    }

    @Test
    fun alreadyClearedAndNoForbiddenAreOk() {
        for (msg in listOf("Запрет на сегодня уже снят", "Сегодня запретных часов нет")) {
            val json = "{\"v\":1,\"t\":\"cmdr\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"seq\":19," +
                "\"cmd\":\"${Crypto.hashData(nfBytes)}\",\"ts\":1763490005000,\"ok\":true,\"message\":\"$msg\"}"
            val r = Proto.openCmdReply(peer, pcSigned(json), nfBytes, 0)
            assertTrue(r.ok)
            assertEquals(msg, r.message)
        }
        val bad = "{\"v\":1,\"t\":\"cmdr\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"seq\":19," +
            "\"cmd\":\"${Crypto.hashData(nfBytes)}\",\"ts\":1763490005000,\"ok\":false,\"error\":\"replay\",\"lastSeq\":25}"
        val r = Proto.openCmdReply(peer, pcSigned(bad), nfBytes, 0)
        assertFalse(r.ok)
        assertEquals(25L, r.lastSeq)
        assertEquals("Команда устарела, повторите", Strings[Problem.fromCmd(r.error).resName])
    }

    // ---------- отчёт: forbid ----------

    @Test
    fun goReportForbidStates() {
        val rep = Proto.openReport(peer, Vectors.signed("reportForbid"), 0)
        val by = rep.children.associateBy { it.id }
        assertEquals(Proto.Forbid.CLEARED, by[3]!!.forbid)
        assertEquals(Proto.Forbid.ACTIVE, by[4]!!.forbid)
        assertEquals(1763496000000, by[4]!!.forbidUntilMs)
        assertEquals(Proto.Forbid.AHEAD, by[5]!!.forbid)
        assertEquals(Proto.Forbid.NONE, by[6]!!.forbid)
        assertEquals(0L, by[5]!!.forbidUntilMs)

        // Кнопка — при ahead и active; надпись «снят» — при cleared; ничего — при none.
        assertEquals(Proto.ForbidView.CLEARED, by[3]!!.forbidView)
        assertEquals(Proto.ForbidView.BUTTON, by[4]!!.forbidView)
        assertEquals(Proto.ForbidView.BUTTON, by[5]!!.forbidView)
        assertEquals(Proto.ForbidView.HIDDEN, by[6]!!.forbidView)
        assertEquals(1763496000000, by[4]!!.activeForbidUntilMs)
        assertNull(by[5]!!.activeForbidUntilMs)
        // Подделанное состояние без подписи компьютера не принимается.
        val s = Vectors.signed("reportForbid")
        val forged = Vectors.s("reportForbidJson").replace("\"forbid\":\"active\"", "\"forbid\":\"cleared\"")
        try {
            Proto.openReport(peer, Signed(B64.enc(forged.toByteArray()), s.sig), 0)
            throw AssertionError("принят подделанный отчёт")
        } catch (e: ProtoException) {
            // ожидаемо
        }
    }

    @Test
    fun oldReportHasNoForbidButton() {
        val c = Proto.openReport(peer, Vectors.signed("report"), 0).children.single()
        assertNull(c.forbid)
        assertEquals(Proto.ForbidView.HIDDEN, c.forbidView)
        assertNull(c.today)
    }

    @Test
    fun unknownForbidIgnored() {
        val json = "{\"v\":1,\"t\":\"report\",\"pc\":\"pc-vector\",\"phone\":\"phone-vector\",\"ts\":5," +
            "\"children\":[{\"id\":3,\"name\":\"Миша\",\"forbid\":\"soon\"},{\"id\":4,\"name\":\"Коля\",\"forbid\":\"active\"}]}"
        val rep = Proto.openReport(peer, pcSigned(json), 0)
        assertNull(rep.children[0].forbid)
        assertEquals(Proto.ForbidView.HIDDEN, rep.children[0].forbidView)
        // active без конца интервала: кнопка есть, строки «до HH:MM» нет.
        assertEquals(Proto.ForbidView.BUTTON, rep.children[1].forbidView)
        assertNull(rep.children[1].activeForbidUntilMs)
    }

    @Test
    fun forbidTexts() {
        val zone = ZoneId.of("Europe/Moscow")
        val until = 1763496000000 // 23:00 по Москве
        val hm = Format.clock(until, until - 3600_000, zone, Strings["time_other_day"])
        assertEquals("Запретные часы до 23:00", Strings["forbid_until"].format(hm))
        assertEquals("Снять запрет на сегодня", Strings["btn_no_forbidden"])
        assertEquals("Запрет на сегодня снят", Strings["forbid_cleared"])
        assertEquals("Снять запрет на сегодня, Миша", Strings["btn_no_forbidden_desc"].format("Миша"))
        assertEquals("Добавить 30 минут, Миша", Strings["btn_add_desc"].format(30, "Миша"))
        assertEquals("Убавить 60 минут, Миша", Strings["btn_sub_desc"].format(60, "Миша"))
        assertEquals("+30 мин", Strings["btn_add"].format(30))
        assertEquals("−60 мин", Strings["btn_sub"].format(60))
    }
}
