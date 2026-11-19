package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/** Отчёт и ответ, которые телефон обязан отвергнуть (Review Focus 1: «посредник»). */
class RejectTest {
    private val peer = Vectors.peer()
    private val cmdBytes = Vectors.s("cmdJson").toByteArray(Charsets.UTF_8)
    private val reportJson = Vectors.s("reportJson")

    private fun pcSigned(json: String) = Crypto.seal(SoftSigner.pc(), json.toByteArray(Charsets.UTF_8))

    private fun rejects(block: () -> Unit) {
        try {
            block()
            fail("ожидался отказ")
        } catch (e: ProtoException) {
            // ожидаемо
        }
    }

    @Test
    fun reportWithBrokenSignature() {
        val s = Vectors.signed("report")
        val sig = B64.dec(s.sig).also { it[it.size - 1] = (it[it.size - 1].toInt() xor 1).toByte() }
        rejects { Proto.openReport(peer, Signed(s.data, B64.enc(sig)), 0) }
    }

    @Test
    fun reportWithChangedData() {
        val s = Vectors.signed("report")
        val forged = reportJson.replace("\"leftSec\":1800", "\"leftSec\":9999")
        rejects { Proto.openReport(peer, Signed(B64.enc(forged.toByteArray()), s.sig), 0) }
    }

    @Test
    fun reportSignedByAnotherKey() {
        val s = Crypto.seal(SoftSigner.phone(), reportJson.toByteArray())
        rejects { Proto.openReport(peer, s, 0) }
    }

    @Test
    fun reportForAnotherPhone() {
        rejects { Proto.openReport(peer, pcSigned(reportJson.replace("phone-vector", "phone-other")), 0) }
    }

    @Test
    fun reportFromAnotherPc() {
        rejects { Proto.openReport(peer, pcSigned(reportJson.replace("pc-vector", "pc-other")), 0) }
    }

    @Test
    fun reportOlderThanShown() {
        rejects { Proto.openReport(peer, Vectors.signed("report"), lastShownTs = 1763490001001) }
        // Тот же отчёт ещё раз — не старее показанного, принимается.
        assertEquals(1763490001000, Proto.openReport(peer, Vectors.signed("report"), 1763490001000).ts)
    }

    @Test
    fun otherSignedTypeIsNotReport() {
        rejects { Proto.openReport(peer, Vectors.signed("cmdr"), 0) }
        rejects { Proto.openReport(peer, pcSigned(reportJson.replace("\"t\":\"report\"", "\"t\":\"cmdr\"")), 0) }
        rejects { Proto.openReport(peer, pcSigned(reportJson.replace("\"v\":1", "\"v\":2")), 0) }
    }

    @Test
    fun cmdReplyForAnotherCommand() {
        val other = Proto.commandData(Vectors.PC, Vectors.PHONE, 18, 1763490000000, 3, 15)
        rejects { Proto.openCmdReply(peer, Vectors.signed("cmdr"), other, 0) }
    }

    @Test
    fun reportIsNotCmdReply() {
        rejects { Proto.openCmdReply(peer, Vectors.signed("report"), cmdBytes, 0) }
    }

    @Test
    fun cmdReplyWithBrokenSignature() {
        val s = Vectors.signed("cmdr")
        rejects { Proto.openCmdReply(peer, Signed(s.data, Vectors.signed("report").sig), cmdBytes, 0) }
    }

    @Test
    fun refusedCmdReplyCarriesCodeAndLastSeq() {
        val json = """{"v":1,"t":"cmdr","pc":"pc-vector","phone":"phone-vector","seq":17,"cmd":"${Crypto.hashData(cmdBytes)}","ts":1,"ok":false,"error":"replay","lastSeq":20}"""
        val r = Proto.openCmdReply(peer, pcSigned(json), cmdBytes, 0)
        assertTrue(!r.ok)
        assertEquals("replay", r.error)
        assertEquals(20L, r.lastSeq)
        assertEquals(Problem.STALE, Problem.fromCmd(r.error))
    }

    @Test
    fun oldReportInsideReplyIsDroppedButReplyAccepted() {
        val r = Proto.openCmdReply(peer, Vectors.signed("cmdr"), cmdBytes, lastShownTs = Long.MAX_VALUE)
        assertTrue(r.ok)
        assertEquals(null, r.report)
    }

    @Test
    fun badPairReplies() {
        val pub = Vectors.s("pcPub")
        rejects { Proto.parsePair1R(Proto.KIND_PAIR2R, org.json.JSONObject().put("pcPub", pub).put("commit", Vectors.s("commit"))) }
        rejects { Proto.parsePair1R(Proto.KIND_PAIR1R, org.json.JSONObject().put("pcPub", "AAAA").put("commit", Vectors.s("commit"))) }
        rejects { Proto.parsePair1R(Proto.KIND_PAIR1R, org.json.JSONObject().put("pcPub", pub).put("commit", "AAAA")) }
        rejects { Proto.parsePair2R(Proto.KIND_PAIR2R, org.json.JSONObject().put("nPc", "AAAA")) }
        rejects { Proto.parsePair2R(Proto.KIND_PAIR2R, null) }
    }
}
