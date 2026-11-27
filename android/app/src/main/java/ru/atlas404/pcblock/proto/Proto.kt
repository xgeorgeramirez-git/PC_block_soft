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
    const val KIND_NOTE = "note"
    const val KIND_NOTER = "noter"

    /** Сообщение с телефона (раздел 5а): до 300 знаков, не больше 5 переводов строк. */
    const val MAX_NOTE = 300
    const val MAX_NOTE_LINES = 5

    /** Сколько показывать: 0 — пока не нажмёт «Понял», иначе секунды. */
    val SHOW_SEC = listOf(0, 10, 30, 60)

    /** Звук сообщения (раздел 5а): 0 — без звука, 1…99 — номер; нет поля — 1. */
    const val NO_SOUND = 0
    const val DEFAULT_SOUND = 1
    const val MAX_SOUND = 99

    /** Сколько стикеров в сетке экрана сообщения рядом с «Без стикера» (остальные — «Все N»). */
    const val STICKER_GRID = 11

    private val STICKER_FILE = Regex("^([0-9]{3})\\.tgs$")
    private val SOUND_FILE = Regex("^([1-9][0-9]?)\\.mp3$")

    /** Номера стикеров по именам файлов папки stickers (NNN.tgs), по возрастанию. */
    fun stickerIds(files: Array<String>?): List<String> =
        files.orEmpty().mapNotNull { STICKER_FILE.matchEntire(it)?.groupValues?.get(1) }.distinct().sorted()

    /** Номера звуков по именам файлов папки sounds/message (N.mp3, 1…99), по возрастанию. */
    fun soundIds(files: Array<String>?): List<Int> =
        files.orEmpty().mapNotNull { SOUND_FILE.matchEntire(it)?.groupValues?.get(1)?.toInt() }.distinct().sorted()

    /** Привязанный компьютер: чьи подписи и для какого телефона принимаем. */
    data class Peer(val pcId: String, val phoneId: String, val pcPub: PublicKey)

    /** Быстрые кнопки карточки ребёнка (FR-42): «+N» и «−N» минут. */
    val QUICK_ADD = listOf(15, 30, 60)

    const val OP_NO_FORBIDDEN = "noForbidden"

    /** Команды протокола 1.5 (разделы 5в–5д): копилка, промокоды, решения о программах. */
    const val OP_BANK_ADD = "bankAdd"
    const val OP_BANK_CLEAR = "bankClear"
    const val OP_PROMO_LIST = "promoList"
    const val OP_PROMO_CREATE = "promoCreate"
    const val OP_PROMO_GEN = "promoGen"
    const val OP_PROMO_DELETE = "promoDelete"
    const val OP_DECIDE = "decide"

    /** Одно начисление в копилку: 1…600 минут (FR-44, D-113); предела баланса нет. */
    const val MAX_BANK_ADD = 600

    /** Минуты промокода (R26): 1…600. */
    const val MAX_PROMO_MIN = 600

    /** «Придумать коды» (8.3.10 п. 3): сколько 1…50, длина 4…12. */
    const val MAX_PROMO_COUNT = 50
    const val MIN_PROMO_LEN = 4
    const val MAX_PROMO_LEN = 12

    /** Код для удаления (R-5): любой непустой до 64 знаков (коды до 0.5.0 могли быть с кириллицей). */
    const val MAX_DELETE_CODE = 64

    /** Из чего придумывать коды (раздел 5г). */
    const val CHARSET_DIGITS = "digits"
    const val CHARSET_LETTERS = "letters"
    const val CHARSET_MIXED = "mixed"
    val CHARSETS = listOf(CHARSET_DIGITS, CHARSET_LETTERS, CHARSET_MIXED)

    /** Решение о программе (раздел 5д): «Игра», «Учёба», «Прочее», «Запретить». */
    const val CAT_GAMES = "games"
    const val CAT_STUDY = "study"
    const val CAT_OTHER = "other"
    const val CAT_BLOCKED = "blocked"
    val CATS = listOf(CAT_GAMES, CAT_STUDY, CAT_OTHER, CAT_BLOCKED)

    /** Запретные часы ребёнка сегодня (раздел 6, поле forbid). */
    enum class Forbid(val wire: String) {
        NONE("none"),
        AHEAD("ahead"),
        ACTIVE("active"),
        CLEARED("cleared"),
        ;

        companion object {
            fun of(s: String): Forbid? = values().firstOrNull { it.wire == s }
        }
    }

    /** Что показать на карточке про запрет: ничего, кнопку «Снять запрет на сегодня» или «Запрет на сегодня снят». */
    enum class ForbidView { HIDDEN, BUTTON, CLEARED }

    /** Строка «Итогов» за сегодня: программа или сайт, вкладка cat (game/study/video/other), секунды. */
    data class Item(val name: String, val cat: String, val sec: Long)

    /**
     * «Статистика за сегодня» (раздел 6, поле today; FR-43). asOfMs — время отчёта, в котором она
     * пришла («данные на HH:MM»): отчёт из ответа на команду today не несёт, телефон держит прежний.
     */
    data class Today(
        val firstMs: Long,
        val nowAt: Boolean,
        val sinceMs: Long,
        val usedSec: Long,
        val leftSec: Long,
        val gamesSec: Long,
        val studySec: Long,
        val videoSec: Long,
        val programs: List<Item>,
        val sites: List<Item>,
        val morePrograms: Int,
        val moreSites: Int,
        val violations: Int,
        val bypass: Int,
        val asOfMs: Long,
    )

    /**
     * Строка ребёнка из отчёта. leftSec < 0 — лимита нет. forbid = null — компьютер старой версии
     * (кнопки снятия запрета нет); today = null — статистики за сегодня нет (кнопки нет).
     * bankMin = null — компьютер старше 0.8.4 (копилки на телефоне нет). gamesSec…videoLimSec = null —
     * поля нет (карточка в старом виде); лимит −1 — отдельного лимита нет («Игры 40 мин»).
     */
    data class Child(
        val id: Long,
        val name: String,
        val usedSec: Long,
        val leftSec: Long,
        val extraMin: Int,
        val blocked: Boolean,
        val forbid: Forbid? = null,
        val forbidUntilMs: Long = 0,
        val today: Today? = null,
        val bankMin: Int? = null,
        val gamesSec: Long? = null,
        val gamesLimSec: Long? = null,
        val videoSec: Long? = null,
        val videoLimSec: Long? = null,
    ) {
        val forbidView: ForbidView
            get() = when (forbid) {
                Forbid.AHEAD, Forbid.ACTIVE -> ForbidView.BUTTON
                Forbid.CLEARED -> ForbidView.CLEARED
                Forbid.NONE, null -> ForbidView.HIDDEN
            }

        /** Конец идущих запретных часов («Запретные часы до HH:MM»), null — не идут или конец неизвестен. */
        val activeForbidUntilMs: Long?
            get() = if (forbid == Forbid.ACTIVE && forbidUntilMs > 0) forbidUntilMs else null
    }

    /** Состояние сообщения из отчёта (раздел 5а). */
    enum class NoteState(val wire: String) {
        QUEUED("queued"),
        SHOWN("shown"),
        READ("read"),
        EXPIRED("expired"),
        ;

        companion object {
            fun of(s: String): NoteState? = values().firstOrNull { it.wire == s }
        }
    }

    data class NoteStatus(val id: String, val child: Long, val state: NoteState, val atMs: Long)

    /** Кто из детей запускал ждущую программу и сколько раз. */
    data class PendingKid(val id: Long, val tries: Int)

    /**
     * Незнакомая программа, ждущая решения (раздел 6, pending): id — ключ для decide, pub пусто —
     * «Без подписи издателя», path может быть урезан посередине, lastMs — последняя попытка.
     */
    data class Pending(
        val id: String,
        val name: String,
        val pub: String,
        val path: String,
        val kids: List<PendingKid>,
        val tries: Int,
        val lastMs: Long,
    )

    /**
     * Отчёт компьютера. pending — до 10 ждущих программ, pendingCount — сколько всего.
     * newPc — в отчёте есть pendingCount (компьютер 0.8.4 и новее шлёт его всегда).
     */
    data class Report(
        val ts: Long,
        val children: List<Child>,
        val notes: List<NoteStatus> = emptyList(),
        val pending: List<Pending> = emptyList(),
        val pendingCount: Int = 0,
        val newPc: Boolean = false,
    ) {
        /**
         * Компьютер 0.8.4 и новее: копилка, «Промокоды» и ждущие программы на телефоне есть.
         * Хоть у одного ребёнка нет bankMin — компьютер старше («Обновите программу на компьютере»).
         * Детей нет — по полю pendingCount.
         */
        val hasBank: Boolean
            get() = if (children.isEmpty()) newPc else children.all { it.bankMin != null }
    }

    /** Действующий промокод (раздел 5г). */
    data class PromoActive(val code: String, val min: Int, val createdMs: Long)

    /** Использованный промокод: childName пусто — ребёнок удалён. */
    data class PromoUsed(val code: String, val min: Int, val usedMs: Long, val child: Long, val childName: String)

    /** Список промокодов из ответа; moreActive, moreUsed — сколько строк не вошло (смотреть в окне родителя). */
    data class PromoList(val active: List<PromoActive>, val used: List<PromoUsed>, val moreActive: Int, val moreUsed: Int)

    /** Проверенный ответ на сообщение: ok — компьютер принял (ещё не показал). */
    data class NoteReply(val ok: Boolean, val error: String, val lastSeq: Long?)

    /**
     * Проверенный ответ на команду. ts — время ответа по часам компьютера («данные на HH:MM» у промокодов).
     * promos — список промокодов (promo* при ok:true, promo_exists, not_found); null — списка нет, телефон
     * держит прежний. codes — новые коды promoGen («Скопировать все»).
     */
    data class CmdReply(
        val ok: Boolean,
        val message: String,
        val error: String,
        val lastSeq: Long?,
        val report: Report?,
        val ts: Long = 0,
        val promos: PromoList? = null,
        val codes: List<String> = emptyList(),
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

    /**
     * Байты data команды «Снять запрет на сегодня» (раздел 5б): поля как у команды доп. времени,
     * вместо delta — op. Порядок фиксирован, чтобы совпадать с вектором Go байт в байт.
     */
    fun noForbiddenData(pc: String, phone: String, seq: Long, ts: Long, child: Long): ByteArray {
        val json = "{\"v\":$VERSION,\"t\":\"cmd\",\"pc\":${JSONObject.quote(pc)},\"phone\":${JSONObject.quote(phone)}," +
            "\"seq\":$seq,\"ts\":$ts,\"child\":$child,\"op\":\"$OP_NO_FORBIDDEN\"}"
        return json.toByteArray(Charsets.UTF_8)
    }

    /** Начало data команды до поля child включительно: общий порядок полей Go (v, t, pc, phone, seq, ts, child). */
    private fun cmdHead(pc: String, phone: String, seq: Long, ts: Long, child: Long): String =
        "{\"v\":$VERSION,\"t\":\"cmd\",\"pc\":${goQuote(pc)},\"phone\":${goQuote(phone)}," +
            "\"seq\":$seq,\"ts\":$ts,\"child\":$child"

    /** Копилка (раздел 5в): положить minutes (1…600) в копилку ребёнка; минуты — в поле delta. */
    fun bankAddData(pc: String, phone: String, seq: Long, ts: Long, child: Long, minutes: Int): ByteArray {
        require(minutes in 1..MAX_BANK_ADD) { "минуты вне 1…600" }
        return (cmdHead(pc, phone, seq, ts, child) + ",\"delta\":$minutes,\"op\":\"$OP_BANK_ADD\"}").toByteArray(Charsets.UTF_8)
    }

    /** Копилка (раздел 5в): обнулить копилку ребёнка, delta нет. */
    fun bankClearData(pc: String, phone: String, seq: Long, ts: Long, child: Long): ByteArray =
        (cmdHead(pc, phone, seq, ts, child) + ",\"op\":\"$OP_BANK_CLEAR\"}").toByteArray(Charsets.UTF_8)

    /** Промокоды (раздел 5г): только прочитать список; child 0, номер тратится. */
    fun promoListData(pc: String, phone: String, seq: Long, ts: Long): ByteArray =
        (cmdHead(pc, phone, seq, ts, 0) + ",\"op\":\"$OP_PROMO_LIST\"}").toByteArray(Charsets.UTF_8)

    /** Промокоды: создать код (уже проверенный PromoRules.checkCode, заглавными) на min минут. */
    fun promoCreateData(pc: String, phone: String, seq: Long, ts: Long, code: String, min: Int): ByteArray {
        require(PromoRules.checkCode(code).let { it.second == null && it.first == code }) { "код не по R26" }
        require(min in 1..MAX_PROMO_MIN) { "минуты вне 1…600" }
        return (cmdHead(pc, phone, seq, ts, 0) + ",\"op\":\"$OP_PROMO_CREATE\",\"code\":${goQuote(code)},\"min\":$min}")
            .toByteArray(Charsets.UTF_8)
    }

    /** Промокоды: «Придумать коды» — count (1…50) кодов длины len (4…12) из charset на min минут. */
    fun promoGenData(pc: String, phone: String, seq: Long, ts: Long, count: Int, len: Int, charset: String, min: Int): ByteArray {
        require(count in 1..MAX_PROMO_COUNT) { "count вне 1…50" }
        require(len in MIN_PROMO_LEN..MAX_PROMO_LEN) { "len вне 4…12" }
        require(charset in CHARSETS) { "charset не из digits, letters, mixed" }
        require(min in 1..MAX_PROMO_MIN) { "минуты вне 1…600" }
        return (
            cmdHead(pc, phone, seq, ts, 0) +
                ",\"op\":\"$OP_PROMO_GEN\",\"min\":$min,\"count\":$count,\"len\":$len,\"charset\":${goQuote(charset)}}"
            ).toByteArray(Charsets.UTF_8)
    }

    /** Промокоды: удалить действующий код — как он пришёл в списке (непустой, до 64 знаков). */
    fun promoDeleteData(pc: String, phone: String, seq: Long, ts: Long, code: String): ByteArray {
        require(code.isNotBlank() && code.codePointCount(0, code.length) <= MAX_DELETE_CODE) { "код пустой или длиннее 64" }
        return (cmdHead(pc, phone, seq, ts, 0) + ",\"op\":\"$OP_PROMO_DELETE\",\"code\":${goQuote(code)}}").toByteArray(Charsets.UTF_8)
    }

    /** Решение о программе (раздел 5д): key — id строки pending, cat — games, study, other, blocked. */
    fun decideData(pc: String, phone: String, seq: Long, ts: Long, key: String, cat: String): ByteArray {
        require(key.isNotEmpty()) { "пустой key" }
        require(cat in CATS) { "cat не из games, study, other, blocked" }
        return (cmdHead(pc, phone, seq, ts, 0) + ",\"op\":\"$OP_DECIDE\",\"key\":${goQuote(key)},\"cat\":${goQuote(cat)}}")
            .toByteArray(Charsets.UTF_8)
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
                forbid = Forbid.of(c.optString("forbid", "")),
                forbidUntilMs = c.optLong("forbidUntilMs", 0),
                today = parseToday(c.optJSONObject("today"), ts),
                bankMin = if (c.has("bankMin")) c.optInt("bankMin", 0) else null,
                gamesSec = optLongOrNull(c, "gamesSec"),
                gamesLimSec = optLongOrNull(c, "gamesLimSec"),
                videoSec = optLongOrNull(c, "videoSec"),
                videoLimSec = optLongOrNull(c, "videoLimSec"),
            )
        }
        Report(
            ts = ts,
            children = children,
            notes = parseNotes(o.optJSONArray("notes")),
            pending = parsePending(o.optJSONArray("pending")),
            pendingCount = maxOf(0, o.optInt("pendingCount", 0)),
            newPc = o.has("pendingCount"),
        )
    }

    private fun optLongOrNull(o: JSONObject, key: String): Long? = if (o.has(key)) o.optLong(key, 0) else null

    /** Строки pending: не объект, без id или кривой kids — строка (или ребёнок в ней) пропускается. */
    private fun parsePending(arr: JSONArray?): List<Pending> {
        if (arr == null) return emptyList()
        val out = ArrayList<Pending>()
        for (i in 0 until arr.length()) {
            val p = arr.optJSONObject(i) ?: continue
            val id = p.optString("id", "")
            if (id.isEmpty() || p.isNull("id")) continue
            val kidsArr = p.optJSONArray("kids")
            val kids = ArrayList<PendingKid>()
            if (kidsArr != null) {
                for (k in 0 until kidsArr.length()) {
                    val kid = kidsArr.optJSONObject(k) ?: continue
                    if (!kid.has("id")) continue
                    val kidId = kid.optLong("id", Long.MIN_VALUE)
                    if (kidId == Long.MIN_VALUE) continue
                    kids.add(PendingKid(kidId, maxOf(0, kid.optInt("tries", 0))))
                }
            }
            out.add(
                Pending(
                    id = id,
                    name = p.optString("name", ""),
                    pub = p.optString("pub", ""),
                    path = p.optString("path", ""),
                    kids = kids,
                    tries = maxOf(0, p.optInt("tries", 0)),
                    lastMs = p.optLong("lastMs", 0),
                ),
            )
        }
        return out
    }

    // ---------- список промокодов (раздел 5г) ----------

    /** Поле promos ответа: строки без кода пропускаются; не объект — null (списка нет). */
    private fun parsePromoList(o: JSONObject?): PromoList? {
        if (o == null) return null
        val active = ArrayList<PromoActive>()
        val a = o.optJSONArray("active")
        if (a != null) {
            for (i in 0 until a.length()) {
                val r = a.optJSONObject(i) ?: continue
                val code = r.optString("code", "")
                if (code.isEmpty()) continue
                active.add(PromoActive(code, r.optInt("min", 0), r.optLong("createdMs", 0)))
            }
        }
        val used = ArrayList<PromoUsed>()
        val u = o.optJSONArray("used")
        if (u != null) {
            for (i in 0 until u.length()) {
                val r = u.optJSONObject(i) ?: continue
                val code = r.optString("code", "")
                if (code.isEmpty()) continue
                used.add(PromoUsed(code, r.optInt("min", 0), r.optLong("usedMs", 0), r.optLong("child", 0), r.optString("childName", "")))
            }
        }
        return PromoList(active, used, maxOf(0, o.optInt("moreActive", 0)), maxOf(0, o.optInt("moreUsed", 0)))
    }

    /** Кэш списка промокодов на телефоне (Prefs): JSON того же вида, что поле promos. */
    fun promosJson(p: PromoList): String {
        val active = JSONArray()
        for (a in p.active) active.put(JSONObject().put("code", a.code).put("min", a.min).put("createdMs", a.createdMs))
        val used = JSONArray()
        for (u in p.used) {
            used.put(
                JSONObject().put("code", u.code).put("min", u.min).put("usedMs", u.usedMs)
                    .put("child", u.child).put("childName", u.childName),
            )
        }
        return JSONObject().put("active", active).put("used", used)
            .put("moreActive", p.moreActive).put("moreUsed", p.moreUsed).toString()
    }

    /** Список из кэша; пусто или кривое — null. */
    fun parsePromos(raw: String): PromoList? {
        val t = raw.trim()
        if (!t.startsWith("{")) return null
        return try {
            parsePromoList(JSONObject(t))
        } catch (e: JSONException) {
            null
        }
    }

    /** Поле today: кривые строки списков пропускаются; не объект — статистики нет (сам отчёт принят). */
    private fun parseToday(t: JSONObject?, ts: Long): Today? {
        if (t == null) return null
        val more = t.optJSONObject("more")
        return Today(
            firstMs = t.optLong("firstMs", 0),
            nowAt = t.optBoolean("nowAt", false),
            sinceMs = t.optLong("sinceMs", 0),
            usedSec = t.optLong("usedSec", 0),
            leftSec = t.optLong("leftSec", 0),
            gamesSec = t.optLong("gamesSec", 0),
            studySec = t.optLong("studySec", 0),
            videoSec = t.optLong("videoSec", 0),
            programs = parseItems(t.optJSONArray("programs")),
            sites = parseItems(t.optJSONArray("sites")),
            morePrograms = more?.optInt("programs", 0) ?: 0,
            moreSites = more?.optInt("sites", 0) ?: 0,
            violations = t.optInt("violations", 0),
            bypass = t.optInt("bypass", 0),
            asOfMs = ts,
        )
    }

    private fun parseItems(arr: JSONArray?): List<Item> {
        if (arr == null) return emptyList()
        val out = ArrayList<Item>()
        for (i in 0 until arr.length()) {
            val it = arr.optJSONObject(i) ?: continue
            val name = it.optString("name", "")
            if (name.isEmpty()) continue
            out.add(Item(name, it.optString("cat", "other"), maxOf(0L, it.optLong("sec", 0))))
        }
        return out
    }

    /**
     * Вкладка «Промокоды» и ждущие программы (и запрос promoList): только когда отчёт пришёл и компьютер
     * 0.8.4 и новее. До первого отчёта неизвестно, какой компьютер: старый на promoList ответит range,
     * а нужно «Обновите программу на компьютере до версии 0.8.4» (протокол, раздел 6).
     */
    fun newPcTabs(report: Report?): Boolean = report?.hasBank == true

    /**
     * Отчёт из ответа на команду (cmdr) today не несёт (раздел 6): у детей без today берётся
     * статистика из прежнего отчёта (со своим временем «данные на»). Остальное — из свежего.
     */
    fun keepToday(fresh: Report, prev: Report?): Report {
        if (prev == null) return fresh
        val old = prev.children.associate { it.id to it.today }
        return fresh.copy(children = fresh.children.map { c -> if (c.today != null) c else c.copy(today = old[c.id]) })
    }

    /** Строки notes: непонятное состояние или кривая строка пропускаются, остальное показывается. */
    private fun parseNotes(arr: JSONArray?): List<NoteStatus> {
        if (arr == null) return emptyList()
        val out = ArrayList<NoteStatus>()
        for (i in 0 until arr.length()) {
            val n = arr.optJSONObject(i) ?: continue
            val id = n.optString("id", "")
            val state = NoteState.of(n.optString("state", "")) ?: continue
            if (id.isEmpty() || !n.has("child")) continue
            val child = n.optLong("child", Long.MIN_VALUE)
            if (child == Long.MIN_VALUE) continue
            out.add(NoteStatus(id, child, state, n.optLong("atMs", 0)))
        }
        return out
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
                ts = o.optLong("ts", 0),
                promos = parsePromoList(o.optJSONObject("promos")),
                codes = parseCodes(o.optJSONArray("codes")),
            )
        }
    }

    private fun parseCodes(arr: JSONArray?): List<String> {
        if (arr == null) return emptyList()
        return (0 until arr.length()).mapNotNull { i -> (arr.opt(i) as? String)?.takeIf { it.isNotEmpty() } }
    }

    // ---------- сообщение (раздел 5а) ----------

    /**
     * Текст, который уйдёт на компьютер, — как NoteText в Go (internal/remote/proto.go): переводы
     * строк \n, табуляция — пробел; управляющие знаки (Cc), невидимые знаки форматирования (Cf, кроме
     * U+200D — склейки эмодзи), знак замены U+FFFD и одинокие половинки UTF-16 убраны; края обрезаны.
     * Текст только из невидимых знаков — пустой: «Текст от 1 до 300 знаков» ещё на телефоне.
     */
    fun cleanNote(raw: String): String {
        val sb = StringBuilder(raw.length)
        val s = raw.replace("\r\n", "\n").replace('\r', '\n')
        var i = 0
        while (i < s.length) {
            val cp = s.codePointAt(i)
            i += Character.charCount(cp)
            val type = Character.getType(cp)
            when {
                cp == '\n'.code -> sb.append('\n')
                cp == '\t'.code -> sb.append(' ')
                type == Character.CONTROL.toInt() || type == Character.SURROGATE.toInt() || cp == 0xFFFD -> {}
                type == Character.FORMAT.toInt() && cp != 0x200D -> {}
                else -> sb.appendCodePoint(cp)
            }
        }
        return sb.toString().trim()
    }

    /** Ошибка текста для экрана: null — годится, иначе NOTE_RANGE («Текст от 1 до 300 знаков»). */
    fun noteProblem(raw: String): Problem? = if (noteOk(raw)) null else Problem.NOTE_RANGE

    /** Длина в знаках Unicode (как считает компьютер) после обрезки: для счётчика «12/300». */
    fun noteLength(raw: String): Int = cleanNote(raw).let { it.codePointCount(0, it.length) }

    /** Текст годится: 1…300 знаков после обрезки и не больше 5 переводов строк. */
    fun noteOk(raw: String): Boolean {
        val t = cleanNote(raw)
        val n = t.codePointCount(0, t.length)
        return n in 1..MAX_NOTE && t.count { it == '\n' } <= MAX_NOTE_LINES
    }

    /** Выбор показа из настроек: незнакомое значение — «Пока не нажмёт «Понял»». */
    fun normalShowSec(v: Int): Int = if (v in SHOW_SEC) v else 0

    /** Стикер годится для протокола: "" (без стикера) или ровно три цифры ASCII. */
    fun stickerOk(s: String): Boolean = s.isEmpty() || (s.length == 3 && s.all { it in '0'..'9' })

    /** Звук годится для протокола: 0…99. */
    fun soundOk(n: Int): Boolean = n in NO_SOUND..MAX_SOUND

    /** Выбор звука из настроек: незнакомое значение — «Звук 1». */
    fun normalSound(v: Int): Int = if (soundOk(v)) v else DEFAULT_SOUND

    /** Случайный id сообщения: 16 байт base64url. */
    fun newNoteId(rnd: java.util.Random): String = B64.enc(ByteArray(NONCE_LEN).also { rnd.nextBytes(it) })

    /**
     * Байты data сообщения. Строки кодируются как json.Marshal в Go (экранирование <, >, &,
     * U+2028/2029), порядок полей фиксирован: так байты совпадают с вектором Go. sticker "" — поля
     * нет (без стикера); sound уходит всегда (0 — без звука).
     */
    fun noteData(
        pc: String,
        phone: String,
        seq: Long,
        ts: Long,
        child: Long,
        id: String,
        text: String,
        showSec: Int,
        sticker: String,
        sound: Int,
    ): ByteArray {
        require(noteOk(text) && cleanNote(text) == text) { "текст вне 1…300 знаков" }
        require(showSec in SHOW_SEC) { "showSec не из 0, 10, 30, 60" }
        require(stickerOk(sticker)) { "sticker не из трёх цифр" }
        require(soundOk(sound)) { "sound вне 0…99" }
        val stickerField = if (sticker.isEmpty()) "" else ",\"sticker\":${goQuote(sticker)}"
        val json = "{\"v\":$VERSION,\"t\":\"note\",\"pc\":${goQuote(pc)},\"phone\":${goQuote(phone)}," +
            "\"seq\":$seq,\"ts\":$ts,\"child\":$child,\"id\":${goQuote(id)},\"text\":${goQuote(text)}," +
            "\"showSec\":$showSec$stickerField,\"sound\":$sound}"
        return json.toByteArray(Charsets.UTF_8)
    }

    /** Строка JSON так, как её пишет encoding/json Go. */
    fun goQuote(s: String): String {
        val sb = StringBuilder(s.length + 2).append('"')
        var i = 0
        while (i < s.length) {
            val c = s[i]
            when {
                c == '"' -> sb.append("\\\"")
                c == '\\' -> sb.append("\\\\")
                c == '\n' -> sb.append("\\n")
                c == '\r' -> sb.append("\\r")
                c == '\t' -> sb.append("\\t")
                c == '\b' -> sb.append("\\b")
                c == '\u000c' -> sb.append("\\f")
                c < ' ' || c == '<' || c == '>' || c == '&' || c == '\u2028' || c == '\u2029' ->
                    sb.append(String.format(java.util.Locale.ROOT, "\\u%04x", c.code))
                c.isHighSurrogate() && i + 1 < s.length && s[i + 1].isLowSurrogate() -> {
                    sb.append(c).append(s[i + 1])
                    i++
                }
                c.isSurrogate() -> sb.append("\\ufffd") // одиночная половинка: Go пишет U+FFFD
                else -> sb.append(c)
            }
            i++
        }
        return sb.append('"').toString()
    }

    /** Ответ noter: подпись компьютера → t, v → pc, phone → cmd = хэш именно нашего сообщения. */
    fun openNoteReply(peer: Peer, s: Signed, noteData: ByteArray): NoteReply {
        val data = Crypto.open(peer.pcPub, s)
        val o = parseObject(data)
        return guard {
            expectHeader(o, KIND_NOTER, peer)
            if (o.optString("cmd", "") != Crypto.hashData(noteData)) throw ProtoException("ответ не на это сообщение")
            val ok = o.optBoolean("ok", false)
            NoteReply(
                ok = ok,
                error = if (ok) "" else o.optString("error", ""),
                lastSeq = if (o.has("lastSeq") && !o.isNull("lastSeq")) o.getLong("lastSeq") else null,
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

/**
 * Повтор сообщения после обрыва (протокол, раздел 5а): если на отправку не пришёл ответ компьютера
 * (сеть оборвалась, сервер связи не дождался компьютера, ответ не разобран), сообщение могло дойти.
 * Повтор того же текста тому же ребёнку с тем же показом, стикером и звуком идёт с тем же id: компьютер
 * считает тот же id с новым seq одним сообщением. Ответ компьютера (принято или отказ) — повтор не нужен, следующее
 * сообщение получает новый id.
 */
class NoteRetry {
    private data class Pending(val child: Long, val text: String, val showSec: Int, val sticker: String, val sound: Int, val id: String)

    private var pending: Pending? = null

    /** id для отправки: тот же, что у неотвеченного такого же сообщения, иначе newId(). */
    fun idFor(child: Long, text: String, showSec: Int, sticker: String, sound: Int, newId: () -> String): String {
        val p = pending
        if (p != null && p.child == child && p.text == text && p.showSec == showSec && p.sticker == sticker && p.sound == sound) {
            return p.id
        }
        val id = newId()
        pending = Pending(child, text, showSec, sticker, sound, id)
        return id
    }

    /** Компьютер ответил (принято или отказ): повторять не нужно. */
    fun answered() {
        pending = null
    }
}
