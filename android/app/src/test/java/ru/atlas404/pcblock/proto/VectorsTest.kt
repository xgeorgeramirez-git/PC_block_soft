package ru.atlas404.pcblock.proto

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Go = Kotlin байт в байт (protocol, разделы 2, 4, 5): векторы из internal/remote. */
class VectorsTest {
    private val cmdBytes = Vectors.s("cmdJson").toByteArray(Charsets.UTF_8)

    @Test
    fun goCommandSignatureVerifiesWithPhoneKey() {
        val data = Crypto.open(Crypto.parsePublicB64(Vectors.s("phonePub")), Vectors.signed("cmd"))
        assertArrayEquals(cmdBytes, data)
    }

    @Test
    fun commandDataMatchesGoByteForByte() {
        val data = Proto.commandData(Vectors.PC, Vectors.PHONE, seq = 17, ts = 1763490000000, child = 3, delta = 15)
        assertArrayEquals(cmdBytes, data)
        assertEquals(Vectors.signed("cmd").data, B64.enc(data))
    }

    @Test
    fun commandHashMatchesGo() {
        assertEquals(Vectors.s("cmdHash"), Crypto.hashData(cmdBytes))
    }

    @Test
    fun commitAndSasMatchGo() {
        val pc = Vectors.b("pcPub")
        val phone = Vectors.b("phonePub")
        val nPc = Vectors.b("nPc")
        val nPhone = Vectors.b("nPhone")
        assertArrayEquals(Vectors.b("commit"), Crypto.commit(pc, phone, nPc))
        assertEquals(Vectors.s("sas"), Crypto.sas(pc, phone, nPc, nPhone))
        assertEquals("127 518", Format.sas(Vectors.s("sas")))
    }

    @Test
    fun pairingRepliesParseAndCommitChecks() {
        val body1 = org.json.JSONObject().put("pcPub", Vectors.s("pcPub")).put("commit", Vectors.s("commit"))
        val p1 = Proto.parsePair1R(Proto.KIND_PAIR1R, body1)
        val nPc = Proto.parsePair2R(Proto.KIND_PAIR2R, org.json.JSONObject().put("nPc", Vectors.s("nPc")))
        assertTrue(Proto.commitMatches(p1, Vectors.b("phonePub"), nPc))
        // Сервер подменил nPc (или ключ телефона) — обязательство не сходится.
        val other = nPc.clone().also { it[0] = (it[0] + 1).toByte() }
        assertTrue(!Proto.commitMatches(p1, Vectors.b("phonePub"), other))
        assertTrue(!Proto.commitMatches(p1, Vectors.b("pcPub"), nPc))
    }

    @Test
    fun goReportOpens() {
        val rep = Proto.openReport(Vectors.peer(), Vectors.signed("report"), lastShownTs = 0)
        assertEquals(1763490001000, rep.ts)
        assertEquals(listOf(Proto.Child(3, "Миша", 3600, 1800, 15, false)), rep.children)
        assertArrayEquals(
            Vectors.s("reportJson").toByteArray(Charsets.UTF_8),
            Crypto.open(Vectors.peer().pcPub, Vectors.signed("report")),
        )
    }

    @Test
    fun goCmdReplyOpens() {
        val r = Proto.openCmdReply(Vectors.peer(), Vectors.signed("cmdr"), cmdBytes, lastShownTs = 0)
        assertTrue(r.ok)
        assertEquals("Добавлено 15 минут", r.message)
        assertEquals(1800, r.report!!.children.single().leftSec)
    }

    @Test
    fun kotlinSignatureVerifiesWithOwnKey() {
        val signer = SoftSigner.phone()
        val s = Crypto.seal(signer, cmdBytes)
        assertArrayEquals(cmdBytes, Crypto.open(Crypto.parsePublic(signer.publicDer), s))
        assertEquals(Vectors.signed("cmd").data, s.data)
    }
}
