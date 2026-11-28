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

    /** Сообщение со стикером и звуком (раздел 5а, 0.8.3): Go = Kotlin байт в байт, хэш тот же. */
    @Test
    fun goNoteStickerVectorMatches() {
        val goJson = Vectors.s("noteStickerJson")
        val goBytes = goJson.toByteArray(Charsets.UTF_8)
        assertArrayEquals(goBytes, Crypto.open(Crypto.parsePublicB64(Vectors.s("phonePub")), Vectors.signed("noteSticker")))
        val o = org.json.JSONObject(goJson)
        assertEquals("033", o.getString("sticker"))
        assertEquals(2, o.getInt("sound"))
        val ours = Proto.noteData(
            o.getString("pc"), o.getString("phone"), o.getLong("seq"), o.getLong("ts"), o.getLong("child"),
            o.getString("id"), o.getString("text"), o.getInt("showSec"), o.getString("sticker"), o.getInt("sound"),
        )
        assertArrayEquals(goBytes, ours)
        assertEquals(goJson, String(ours, Charsets.UTF_8))
        assertEquals(Vectors.s("noteStickerHash"), Crypto.hashData(ours))
    }

    @Test
    fun kotlinSignatureVerifiesWithOwnKey() {
        val signer = SoftSigner.phone()
        val s = Crypto.seal(signer, cmdBytes)
        assertArrayEquals(cmdBytes, Crypto.open(Crypto.parsePublic(signer.publicDer), s))
        assertEquals(Vectors.signed("cmd").data, s.data)
    }

    // ---------- протокол 1.5: копилка, промокоды, решения (разделы 5в, 5г, 5д, 6) ----------

    /** Вектор команды Go: подпись проверяется ключом телефона, байты Kotlin те же, хэш тот же. */
    private fun sameAsGo(name: String, ours: ByteArray) {
        val goJson = Vectors.s(name + "Json")
        val goBytes = goJson.toByteArray(Charsets.UTF_8)
        assertArrayEquals(goBytes, Crypto.open(Crypto.parsePublicB64(Vectors.s("phonePub")), Vectors.signed(name)))
        assertEquals(goJson, String(ours, Charsets.UTF_8))
        assertEquals(Vectors.signed(name).data, B64.enc(ours))
        assertEquals(Vectors.s(name + "Hash"), Crypto.hashData(ours))
    }

    @Test
    fun bankAddMatchesGo() {
        sameAsGo("cmdBankAdd", Proto.bankAddData(Vectors.PC, Vectors.PHONE, seq = 20, ts = 1763490005000, child = 3, minutes = 30))
    }

    @Test
    fun bankClearMatchesGo() {
        sameAsGo("cmdBankClear", Proto.bankClearData(Vectors.PC, Vectors.PHONE, seq = 21, ts = 1763490006000, child = 3))
    }

    @Test
    fun promoListMatchesGo() {
        sameAsGo("cmdPromoList", Proto.promoListData(Vectors.PC, Vectors.PHONE, seq = 22, ts = 1763490007000))
    }

    @Test
    fun promoCreateMatchesGo() {
        sameAsGo("cmdPromoCreate", Proto.promoCreateData(Vectors.PC, Vectors.PHONE, seq = 23, ts = 1763490008000, code = "SUMMER", min = 30))
    }

    @Test
    fun promoGenMatchesGo() {
        sameAsGo(
            "cmdPromoGen",
            Proto.promoGenData(Vectors.PC, Vectors.PHONE, seq = 24, ts = 1763490009000, count = 5, len = 6, charset = "mixed", min = 15),
        )
    }

    @Test
    fun promoDeleteMatchesGo() {
        sameAsGo("cmdPromoDelete", Proto.promoDeleteData(Vectors.PC, Vectors.PHONE, seq = 25, ts = 1763490010000, code = "SUMMER"))
    }

    @Test
    fun decideMatchesGo() {
        assertEquals("TgOw4J51Seo0S6yK66bHgw", Vectors.s("pendingKey"))
        sameAsGo(
            "cmdDecide",
            Proto.decideData(Vectors.PC, Vectors.PHONE, seq = 26, ts = 1763490011000, key = "TgOw4J51Seo0S6yK66bHgw", cat = "games"),
        )
    }

    /** Ключ решения (5д): первые 16 байт SHA-256 от UTF-8 полного пути, base64url, как в Go. */
    @Test
    fun pendingKeyIsPathHash() {
        val sha = java.security.MessageDigest.getInstance("SHA-256").digest(Vectors.s("pendingPath").toByteArray(Charsets.UTF_8))
        assertEquals(Vectors.s("pendingKey"), B64.enc(sha.copyOf(16)))
    }

    @Test
    fun goReportWithPendingOpens() {
        val rep = Proto.openReport(Vectors.peer(), Vectors.signed("reportPending"), lastShownTs = 0)
        assertEquals(1763490012000, rep.ts)
        assertTrue(rep.hasBank)
        val misha = rep.children[0]
        assertEquals(45, misha.bankMin)
        assertEquals(2400L, misha.gamesSec)
        assertEquals(3600L, misha.gamesLimSec)
        assertEquals(600L, misha.videoSec)
        assertEquals(1800L, misha.videoLimSec)
        val kolya = rep.children[1]
        assertEquals(0, kolya.bankMin)
        assertEquals(-1L, kolya.gamesLimSec)
        assertEquals(-1L, kolya.videoLimSec)
        assertEquals(1, rep.pendingCount)
        assertEquals(
            listOf(
                Proto.Pending(
                    id = "TgOw4J51Seo0S6yK66bHgw",
                    name = "SuperGame",
                    pub = "",
                    path = "C:\\Users\\Misha\\Downloads\\SuperGame\\game.exe",
                    kids = listOf(Proto.PendingKid(3, 3)),
                    tries = 3,
                    lastMs = 1763488000000,
                ),
            ),
            rep.pending,
        )
    }

    @Test
    fun goPromoReplyOpens() {
        val cmd = Vectors.s("cmdPromoGenJson").toByteArray(Charsets.UTF_8)
        val r = Proto.openCmdReply(Vectors.peer(), Vectors.signed("cmdrPromos"), cmd, lastShownTs = 0)
        assertTrue(r.ok)
        assertEquals("Готово: 2 кода на 15 мин", r.message)
        assertEquals(listOf("K7PX", "M3QA"), r.codes)
        assertEquals(null, r.report)
        assertEquals(
            Proto.PromoList(
                active = listOf(Proto.PromoActive("SUMMER", 30, 1763490000000)),
                used = listOf(Proto.PromoUsed("AB12", 30, 1763480000000, 3, "Миша")),
                moreActive = 0,
                moreUsed = 0,
            ),
            r.promos,
        )
    }

    /** Ответ не на эту команду (хэш другой) — отказ, даже со списком промокодов. */
    @Test(expected = ProtoException::class)
    fun promoReplyForOtherCommandRejected() {
        val other = Vectors.s("cmdPromoListJson").toByteArray(Charsets.UTF_8)
        Proto.openCmdReply(Vectors.peer(), Vectors.signed("cmdrPromos"), other, lastShownTs = 0)
    }
}
