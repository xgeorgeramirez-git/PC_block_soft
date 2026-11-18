package ru.atlas404.pcblock.proto

import org.json.JSONArray
import org.json.JSONException
import org.json.JSONObject
import java.security.PublicKey

/**
 * Сторона телефона в протоколе связи (docs/07_Протокол_связи.md, разделы 2, 4–6).
 * Чистый Kotlin + java.security + org.json: в Android org.json встроен, в JVM-тестах — из Maven.
 */
object Proto {
    const val VERSION = 1
    const val MAX_DELTA = 240
    const val NONCE_LEN = 16
    const val MAX_NAME = 40

    const val KIND_PAIR1 = "pair1"
    const val KIND_PAIR1R = "pair1r"
    const val KIND_PAIR2 = "pair2"
    const val KIND_PAIR2R = "pair2r"
    const val KIND_CMD = "cmd"
    const val KIND_CMDR = "cmdr"

    /** Привязанный компьютер: чьи подписи и для какого телефона принимаем. */
    data class Peer(val pcId: String, val phoneId: String, val pcPub: PublicKey)

    /** Строка ребёнка из отчёта. leftSec < 0 — лимита нет. */
    data class Child(
        val id: Long,
        val name: String,
        val usedSec: Long,
        val leftSec: Long,
        val extraMin: Int,
        val blocked: Boolean,
    )

    data class Report(val ts: Long, val children: List<Child>)

    /** Проверенный ответ на команду. */
    data class CmdReply(
        val ok: Boolean,
        val message: String,
        val error: String,
        val lastSeq: Long?,
        val report: Report?,
    )

    // ---------- команда (раздел 5) ----------

    /**
     * Байты data команды. Порядок полей для компьютера неважен (подписываются байты как есть),
     * но фиксирован, чтобы совпадать с вектором Go байт в байт.
     */
    fun commandData(pc: String, phone: String, seq: Long, ts: Long, child: Long, delta: Int): ByteArray {
        require(delta != 0 && delta in -MAX_DELTA..MAX_DELTA) { "delta вне 1…240" }
        val json = "{\"v\":$VERSION,\"t\":\"cmd\",\"pc\":${JSONObject.quote(pc)},\"phone\":${JSONObject.quote(phone)}," +
            "\"seq\":$seq,\"ts\":$ts,\"child\":$child,\"delta\":$delta}"
        return json.toByteArray(Charsets.UTF_8)
    }

    fun signedToJson(s: Signed): JSONObject = JSONObject().put("data", s.data).put("sig", s.sig)

    fun signedFromJson(v: Any?): Signed {
        val o = v as? JSONObject ?: throw ProtoException("нет подписанного блока")
        val data = o.optString("data", "")
        val sig = o.optString("sig", "")
        if (data.isEmpty() || sig.isEmpty() || o.isNull("data") || o.isNull("sig")) {
            throw ProtoException("пустой подписанный блок")
        }
        return Signed(data, sig)
    }

    // ---------- отчёт (раздел 6) ----------

    /**
     * Отчёт с подписи компьютера: подпись → t, v → pc, phone → ts не старее показанного.
     * Любой отказ — исключение (отчёт не показывается).
     */
    fun openReport(peer: Peer, s: Signed, lastShownTs: Long): Report {
        val data = Crypto.open(peer.pcPub, s)
        return parseReport(peer, parseObject(data), lastShownTs)
    }

    private fun parseReport(peer: Peer, o: JSONObject, lastShownTs: Long): Report = guard {
        expectHeader(o, "report", peer)
        val ts = o.getLong("ts")
        if (ts < lastShownTs) throw ProtoException("отчёт старее показанного")
        val arr = o.optJSONArray("children") ?: JSONArray()
        val children = (0 until arr.length()).map { i ->
            val c = arr.getJSONObject(i)
            Child(
                id = c.getLong("id"),
                name = c.optString("name", ""),
                usedSec = c.optLong("usedSec", 0),
                leftSec = c.optLong("leftSec", 0),
                extraMin = c.optInt("extraMin", 0),
                blocked = c.optBoolean("blocked", false),
            )
        }
        Report(ts, children)
    }

    // ---------- ответ на команду (раздел 5) ----------

    /**
     * Ответ cmdr: подпись компьютера → t, v → pc, phone → cmd = хэш именно нашей команды.
     * Вложенный отчёт проверяется так же, как отдельный (кроме подписи: он внутри подписанного).
     */
    fun openCmdReply(peer: Peer, s: Signed, cmdData: ByteArray, lastShownTs: Long): CmdReply {
        val data = Crypto.open(peer.pcPub, s)
        val o = parseObject(data)
        return guard {
            expectHeader(o, "cmdr", peer)
            if (o.optString("cmd", "") != Crypto.hashData(cmdData)) throw ProtoException("ответ не на эту команду")
            val ok = o.optBoolean("ok", false)
            val report = o.optJSONObject("report")?.let {
                try {
                    parseReport(peer, it, lastShownTs)
                } catch (e: ProtoException) {
                    null // старый или чужой отчёт не показываем, но сам ответ принят
                }
            }
            CmdReply(
                ok = ok,
                message = o.optString("message", ""),
                error = if (ok) "" else o.optString("error", ""),
                lastSeq = if (o.has("lastSeq") && !o.isNull("lastSeq")) o.getLong("lastSeq") else null,
                report = report,
            )
        }
    }

    // ---------- привязка (раздел 4) ----------

    fun pair1Body(phonePubDer: ByteArray, name: String): JSONObject =
        JSONObject().put("phonePub", B64.enc(phonePubDer)).put("name", cleanName(name))

    fun pair2Body(nPhone: ByteArray): JSONObject = JSONObject().put("nPhone", B64.enc(nPhone))

    data class Pair1R(val pcPubDer: ByteArray, val commit: ByteArray)

    /** Ответ компьютера на pair1: его ключ (P-256) и обязательство (32 байта). */
    fun parsePair1R(kind: String, body: JSONObject?): Pair1R = guard {
        if (kind != KIND_PAIR1R || body == null) throw ProtoException("ожидался pair1r")
        val pub = B64.dec(body.getString("pcPub"))
        Crypto.parsePublic(pub)
        val commit = B64.dec(body.getString("commit"))
        if (commit.size != 32) throw ProtoException("обязательство не 32 байта")
        Pair1R(pub, commit)
    }

    /** Ответ на pair2: nPc (16 байт). */
    fun parsePair2R(kind: String, body: JSONObject?): ByteArray = guard {
        if (kind != KIND_PAIR2R || body == null) throw ProtoException("ожидался pair2r")
        val n = B64.dec(body.getString("nPc"))
        if (n.size != NONCE_LEN) throw ProtoException("nPc не 16 байт")
        n
    }

    /** Обязательство компьютера раскрыто верно? Не сошлось — привязка прервана. */
    fun commitMatches(p: Pair1R, phonePubDer: ByteArray, nPc: ByteArray): Boolean =
        Crypto.same(Crypto.commit(p.pcPubDer, phonePubDer, nPc), p.commit)

    /** Имя телефона: без управляющих знаков, пробелы схлопнуты, не длиннее 40 знаков. */
    fun cleanName(s: String): String {
        val cleaned = s.map { if (it.isISOControl() || it == '�') ' ' else it }.joinToString("")
            .trim().split(Regex("\\s+")).filter { it.isNotEmpty() }.joinToString(" ")
        val cps = cleaned.codePoints().toArray()
        return if (cps.size <= MAX_NAME) cleaned else String(cps, 0, MAX_NAME).trim()
    }

    // ---------- общее ----------

    private fun expectHeader(o: JSONObject, type: String, peer: Peer) {
        if (o.optString("t", "") != type) throw ProtoException("чужой тип: ожидался $type")
        if (o.optInt("v", 0) != VERSION) throw ProtoException("чужая версия")
        if (o.optString("pc", "") != peer.pcId) throw ProtoException("чужой компьютер")
        if (o.optString("phone", "") != peer.phoneId) throw ProtoException("чужой телефон")
    }

    fun parseObject(data: ByteArray): JSONObject {
        val text = String(data, Charsets.UTF_8).trim()
        if (!text.startsWith("{")) throw ProtoException("не объект JSON")
        return try {
            JSONObject(text)
        } catch (e: JSONException) {
            throw ProtoException("неверный JSON")
        }
    }

    private inline fun <T> guard(block: () -> T): T = try {
        block()
    } catch (e: JSONException) {
        throw ProtoException("неверные данные: ${e.message}")
    }
}
