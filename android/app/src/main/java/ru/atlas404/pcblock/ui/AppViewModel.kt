package ru.atlas404.pcblock.ui

import android.app.Application
import android.os.Build
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.withContext
import ru.atlas404.pcblock.crypto.PhoneKey
import ru.atlas404.pcblock.net.RelayClient
import ru.atlas404.pcblock.net.RelayException
import ru.atlas404.pcblock.proto.B64
import ru.atlas404.pcblock.proto.BankRules
import ru.atlas404.pcblock.proto.Crypto
import ru.atlas404.pcblock.proto.Format
import ru.atlas404.pcblock.proto.NoteRetry
import ru.atlas404.pcblock.proto.Problem
import ru.atlas404.pcblock.proto.PromoRules
import ru.atlas404.pcblock.proto.Proto
import ru.atlas404.pcblock.proto.ProtoException
import ru.atlas404.pcblock.proto.RecentStickers
import ru.atlas404.pcblock.proto.SoundNames
import ru.atlas404.pcblock.store.Prefs
import ru.atlas404.pcblock.store.SavedNote
import java.security.SecureRandom

enum class Screen { CONNECT, COMPARE, MAIN }

/** Итог команды для карточки ребёнка: текст компьютера («Добавлено 15 минут») или ошибка. */
data class CmdNote(val childId: Long, val ok: Boolean, val message: String, val problem: Problem?)

/** Нижнее меню (FR-45): «Дети», «Промокоды», «Настройки». */
enum class Tab { KIDS, PROMOS, SETTINGS }

/** Вложенный экран поверх вкладки «Дети»; системная «Назад» его закрывает. */
sealed interface Sub {
    data class Bank(val child: Long) : Sub
    data class Pending(val child: Long) : Sub
    data class Note(val child: Long) : Sub
    data class Stats(val child: Long) : Sub
}

/** Что делали на экране «Промокоды». */
enum class PromoAct { LIST, CREATE, GEN, DELETE }

/** Итог команды «Промокодов»: текст компьютера («Промокод SUMMER на 30 минут готов») или ошибка. */
data class PromoNote(val act: PromoAct, val ok: Boolean, val message: String, val problem: Problem?)

/** Итог решения о программе («SuperGame: игра»); id — чтобы одинаковый ответ показался снова. */
data class DecideNote(val key: String, val ok: Boolean, val message: String, val problem: Problem?, val id: Long)

/** Состояние приложения и сценарий родителя (план M7, «UX-сценарий», шаги 4–8). */
class AppViewModel(app: Application) : AndroidViewModel(app) {
    companion object {
        const val POLL_MS = 15_000L

        /** После отправки сообщения отметки обновляются чаще: раз в 3 с в течение минуты. */
        const val FAST_POLL_MS = 3_000L
        const val FAST_WINDOW_MS = 60_000L
        const val COMPARE_TTL_MS = 10 * 60_000L
    }

    private val prefs = Prefs(app)
    private val rnd = SecureRandom()
    private val noteRetry = NoteRetry() // повтор сообщения без ответа — с тем же id

    var screen by mutableStateOf(Screen.CONNECT)
        private set

    // ---------- подключение ----------
    var url by mutableStateOf(prefs.url)
    var code by mutableStateOf("")
    var connectBusy by mutableStateOf(false)
        private set
    var connectError by mutableStateOf<Problem?>(null)
        private set

    // ---------- сверка ----------
    private data class PairWait(
        val url: String,
        val pcId: String,
        val phoneId: String,
        val token: String,
        val pcPub: String,
        val sas: String,
        val startedMs: Long,
    )

    private var pending: PairWait? = null
    var sas by mutableStateOf("")
        private set
    var compareDeadlineMs by mutableStateOf(0L)
        private set

    // ---------- главный экран ----------
    var link by mutableStateOf<Format.Link?>(null)
        private set
    var report by mutableStateOf<Proto.Report?>(null)
        private set
    var mainError by mutableStateOf<Problem?>(null)
        private set
    var needRepair by mutableStateOf(false)
        private set
    var busyChild by mutableStateOf<Long?>(null)
        private set
    var note by mutableStateOf<CmdNote?>(null)
        private set

    // ---------- сообщения (FR-41) ----------
    /** Последние сообщения по детям (до 3 на ребёнка), отметки из отчёта. */
    var notes by mutableStateOf<List<SavedNote>>(prefs.notes.all())
        private set
    var noteBusy by mutableStateOf(false)
        private set
    var noteError by mutableStateOf<Problem?>(null)
        private set

    /** Выбор «сколько показывать», запоминается на телефоне. */
    var noteShowSec by mutableStateOf(prefs.notes.showSec)
        private set

    /** Стикеры и звуки сообщения — по файлам assets (ui/StickerAssets.kt), не константа. */
    val noteStickers: List<String> = MessageAssets.stickers(app)
    val noteSounds: List<Int> = MessageAssets.sounds(app)

    /** Звук сообщения: последний выбранный (0 — «Без звука»); звука нет в наборе — первый из набора. */
    var noteSound by mutableStateOf(knownSound(prefs.notes.sound))
        private set

    /** Стикер нового сообщения: «Без стикера» (""), не запоминается между сообщениями (FR-41). */
    var noteSticker by mutableStateOf("")
        private set
    private var fastUntilMs = 0L

    /** Когда компьютер последний раз был на связи (для «не на связи с HH:MM» в отказе команды). */
    var lastSeenMs by mutableStateOf(0L)
        private set

    /** Названия звуков (assets/sounds/message/names.json, D-104); нет файла — «Звук N». */
    val soundNames: Map<Int, String> = runCatching {
        app.assets.open("${MessageAssets.SOUNDS}/names.json").bufferedReader(Charsets.UTF_8).use { it.readText() }
    }.map(SoundNames::parse).getOrDefault(emptyMap())

    /** Последние выбранные стикеры (D-103): попадают сюда после принятой отправки (R-15). */
    var recentStickers by mutableStateOf(prefs.recentStickers)
        private set

    /** Сетка экрана сообщения: 7 последних, добивка — первые по номеру. */
    val noteGrid: List<String> get() = RecentStickers.grid(recentStickers, noteStickers)

    // ---------- навигация (FR-45) ----------
    var tab by mutableStateOf(Tab.KIDS)
        private set
    var sub by mutableStateOf<Sub?>(null)
        private set

    /** Развёрнутая карточка ребёнка; null — первая. */
    var expandedChild by mutableStateOf<Long?>(null)

    // ---------- копилка, промокоды, решения (FR-44, FR-45, FR-47) ----------
    /** Последний список промокодов (кэш на телефоне) и время ответа компьютера. */
    var promos by mutableStateOf(Proto.parsePromos(prefs.promosRaw))
        private set
    var promosTs by mutableStateOf(prefs.promosTs)
        private set
    var promoBusy by mutableStateOf<PromoAct?>(null)
        private set
    var promoNote by mutableStateOf<PromoNote?>(null)
        private set

    /** Новые коды «Придумать» для «Скопировать все». */
    var genCodes by mutableStateOf<List<String>>(emptyList())
        private set
    var decideBusy by mutableStateOf<String?>(null)
        private set
    var decideNote by mutableStateOf<DecideNote?>(null)
        private set

    /** Компьютер на связи (опрашивал сервер не дальше минуты назад). */
    val pcOnline: Boolean get() = link is Format.Link.Online

    /** Идёт любая отправка: одна команда за раз (общий счёт seq). */
    val isBusy: Boolean get() = busyChild != null || noteBusy || promoBusy != null || decideBusy != null

    private var peer: Proto.Peer? = null
    private var pollJob: Job? = null
    private val refreshLock = Mutex()

    init {
        if (prefs.paired && PhoneKey.load() != null) {
            peer = loadPeer()
            if (peer != null) {
                screen = Screen.MAIN
                loadSavedReport()
            }
        }
        if (screen != Screen.MAIN && prefs.paired) forgetAll()
    }

    // ---------- подключение (шаг 4) ----------

    fun connect() {
        if (connectBusy) return
        val base = Format.baseUrl(url)
        if (base == null) {
            connectError = Problem.BAD_ADDRESS
            return
        }
        val c = Format.pairCode(code)
        if (c == null) {
            connectError = Problem.CODE_FORMAT
            return
        }
        connectBusy = true
        connectError = null
        viewModelScope.launch {
            val r = withContext(Dispatchers.IO) { pairNow(base, c) }
            connectBusy = false
            r.onSuccess { p ->
                pending = p
                sas = p.sas
                compareDeadlineMs = p.startedMs + COMPARE_TTL_MS
                screen = Screen.COMPARE
            }.onFailure { e ->
                PhoneKey.forget()
                connectError = when (e) {
                    is RelayException -> Problem.fromRelay(e.code, pairing = true)
                    is KeyFailure -> Problem.KEY
                    else -> Problem.PAIR_FAILED
                }
            }
        }
    }

    private class KeyFailure : Exception()

    /** Привязка по протоколу, раздел 4: pair1 → pair1r → pair2 → pair2r, проверка обязательства, число. */
    private fun pairNow(base: String, code: String): Result<PairWait> = runCatching {
        val key = try {
            PhoneKey.create()
        } catch (e: Exception) {
            throw KeyFailure()
        }
        val client = RelayClient(base)
        val res = client.pair(code, Proto.pair1Body(key.publicDer, deviceName()))
        if (res.reply.from != res.pcId) throw ProtoException("ответ не от компьютера")
        val p1 = Proto.parsePair1R(res.reply.kind, res.reply.body)
        val nPhone = ByteArray(Proto.NONCE_LEN).also { rnd.nextBytes(it) }
        val m2 = client.send(res.phoneToken, Proto.KIND_PAIR2, Proto.pair2Body(nPhone))
        if (m2.from != res.pcId) throw ProtoException("ответ не от компьютера")
        val nPc = Proto.parsePair2R(m2.kind, m2.body)
        if (!Proto.commitMatches(p1, key.publicDer, nPc)) throw ProtoException("обязательство не сошлось")
        PairWait(
            url = base,
            pcId = res.pcId,
            phoneId = res.phoneId,
            token = res.phoneToken,
            pcPub = B64.enc(p1.pcPubDer),
            sas = Crypto.sas(p1.pcPubDer, key.publicDer, nPc, nPhone),
            startedMs = System.currentTimeMillis(),
        )
    }

    private fun deviceName(): String {
        val maker = Build.MANUFACTURER.orEmpty().replaceFirstChar { it.uppercase() }
        val model = Build.MODEL.orEmpty()
        val name = if (model.startsWith(maker, ignoreCase = true)) model else "$maker $model"
        return Proto.cleanName(name).ifEmpty { "Android" }
    }

    // ---------- сверка (шаги 5, 8) ----------

    fun confirmMatch() {
        val p = pending ?: return
        if (System.currentTimeMillis() - p.startedMs > COMPARE_TTL_MS) {
            cancelCompare()
            return
        }
        prefs.savePairing(p.url, p.pcId, p.phoneId, p.token, p.pcPub)
        pending = null
        sas = ""
        code = ""
        peer = loadPeer()
        report = null
        note = null
        notes = emptyList()
        noteError = null
        mainError = null
        needRepair = false
        resetScreens()
        screen = Screen.MAIN
        refreshNow()
    }

    /** «Не совпадает» или 10 минут без ответа: ключи забыты, начать заново. */
    fun cancelCompare() {
        pending = null
        sas = ""
        PhoneKey.forget()
        connectError = Problem.PAIR_CANCELLED
        code = ""
        screen = Screen.CONNECT
    }

    // ---------- главный экран (шаг 6) ----------

    fun startPolling() {
        if (screen != Screen.MAIN || pollJob?.isActive == true) return
        pollJob = viewModelScope.launch {
            while (isActive) {
                refresh()
                delay(if (System.currentTimeMillis() < fastUntilMs) FAST_POLL_MS else POLL_MS)
            }
        }
    }

    fun stopPolling() {
        pollJob?.cancel()
        pollJob = null
    }

    fun refreshNow() {
        viewModelScope.launch { refresh() }
    }

    private suspend fun refresh() {
        val pr = peer ?: return
        if (!refreshLock.tryLock()) return
        try {
            val token = prefs.phoneToken
            val r = withContext(Dispatchers.IO) { runCatching { RelayClient(prefs.url).status(token) } }
            if (peer !== pr) return // телефон отключён, пока шёл запрос
            r.onSuccess { st ->
                link = Format.link(st.serverNowMs, st.pcLastSeenMs)
                lastSeenMs = st.pcLastSeenMs
                mainError = null
                needRepair = false
                st.report?.let { s ->
                    try {
                        val rep = Proto.openReport(pr, s, prefs.lastReportTs)
                        report = rep
                        prefs.saveReport(rep.ts, Proto.signedToJson(s).toString())
                        applyNoteStates(rep)
                    } catch (e: ProtoException) {
                        // Отчёт без подписи компьютера, чужой или старее показанного не показываем.
                    }
                }
            }.onFailure { e ->
                val code = (e as? RelayException)?.code ?: RelayClient.BAD_RESPONSE
                link = null
                mainError = Problem.fromRelay(code)
                needRepair = code == RelayClient.UNAUTHORIZED
            }
        } finally {
            refreshLock.unlock()
        }
    }

    // ---------- навигация ----------

    fun selectTab(t: Tab) {
        sub = null
        tab = t
    }

    /** Открыть вложенный экран: прежний итог команды на нём не показывается. */
    fun open(s: Sub) {
        when (s) {
            is Sub.Bank -> note = null
            is Sub.Pending -> decideNote = null
            is Sub.Note -> startNote()
            is Sub.Stats -> {}
        }
        sub = s
    }

    /** Закрыть вложенный экран; only — закрыть, только если открыт именно он. */
    fun closeSub(only: Sub? = null) {
        if (only == null || sub == only) sub = null
    }

    // ---------- команды (протокол, разделы 5, 5б, 5в) ----------

    /** Команда доп. времени: +N или −N минут ребёнку (протокол, раздел 5). */
    fun command(childId: Long, delta: Int) {
        if (delta == 0 || delta !in -Proto.MAX_DELTA..Proto.MAX_DELTA) {
            if (!isBusy && peer != null) note = CmdNote(childId, false, "", Problem.RANGE)
            return
        }
        runCommand(childId, "") { pr, seq, ts -> Proto.commandData(pr.pcId, pr.phoneId, seq, ts, childId, delta) }
    }

    /** «Снять запрет на сегодня» (протокол, раздел 5б): та же нумерация seq, подпись и проверка ответа. */
    fun clearForbidden(childId: Long) {
        runCommand(childId, Proto.OP_NO_FORBIDDEN) { pr, seq, ts -> Proto.noForbiddenData(pr.pcId, pr.phoneId, seq, ts, childId) }
    }

    /** «Положить N минут» в копилку (раздел 5в): неверное поле (не 1…600) не отправляется. */
    fun bankAdd(childId: Long, raw: String) {
        val m = BankRules.minutes(raw) ?: return
        if (!pcOnline) return
        runCommand(childId, Proto.OP_BANK_ADD) { pr, seq, ts -> Proto.bankAddData(pr.pcId, pr.phoneId, seq, ts, childId, m) }
    }

    /** «Обнулить копилку» после подтверждения (раздел 5в). */
    fun bankClear(childId: Long) {
        if (!pcOnline) return
        runCommand(childId, Proto.OP_BANK_CLEAR) { pr, seq, ts -> Proto.bankClearData(pr.pcId, pr.phoneId, seq, ts, childId) }
    }

    private fun runCommand(childId: Long, op: String, build: (Proto.Peer, Long, Long) -> ByteArray) {
        if (isBusy || peer == null) return
        busyChild = childId
        note = null
        viewModelScope.launch {
            val out = withContext(Dispatchers.IO) { sendCommand(op, build) }
            busyChild = null
            // В отчёте из ответа today нет: статистика остаётся из последнего отдельного отчёта.
            out.reply?.report?.let { report = Proto.keepToday(it, report) }
            if (out.repair) needRepair = true
            note = CmdNote(childId, out.problem == null, out.reply?.message.orEmpty(), out.problem)
            refresh()
        }
    }

    // ---------- промокоды (протокол, раздел 5г) ----------

    /**
     * Запросить список (вкладка открыта, возврат в приложение). Компьютер не на связи — показываем кэш.
     * Отчёта ещё нет или компьютер старше 0.8.4 — не шлём (Proto.newPcTabs).
     */
    fun loadPromos() {
        if (!pcOnline || promoBusy != null || !Proto.newPcTabs(report)) return
        runPromo(PromoAct.LIST, Proto.OP_PROMO_LIST) { pr, seq, ts -> Proto.promoListData(pr.pcId, pr.phoneId, seq, ts) }
    }

    /** «Создать код»: поля проверены теми же правилами, что окно (R26); неверное не отправляется. */
    fun createPromo(code: String, min: String) {
        val c = PromoRules.checkCode(code).first ?: return
        if (PromoRules.checkMinutes(min) != null) return
        val m = PromoRules.number(min) ?: return
        runPromo(PromoAct.CREATE, Proto.OP_PROMO_CREATE) { pr, seq, ts -> Proto.promoCreateData(pr.pcId, pr.phoneId, seq, ts, c, m) }
    }

    /** «Придумать коды»: сколько 1…50, длина 4…12, состав digits/letters/mixed, минуты 1…600. */
    fun genPromos(count: String, len: String, charset: String, min: String) {
        if (PromoRules.checkCount(count) != null || PromoRules.checkLen(len) != null || PromoRules.checkMinutes(min) != null) return
        if (charset !in listOf(Proto.CHARSET_DIGITS, Proto.CHARSET_LETTERS, Proto.CHARSET_MIXED)) return
        val n = PromoRules.number(count) ?: return
        val l = PromoRules.number(len) ?: return
        val m = PromoRules.number(min) ?: return
        runPromo(PromoAct.GEN, Proto.OP_PROMO_GEN) { pr, seq, ts -> Proto.promoGenData(pr.pcId, pr.phoneId, seq, ts, n, l, charset, m) }
    }

    /** Удалить действующий код (после подтверждения «Удалить код SUMMER?»). */
    fun deletePromo(code: String) {
        if (code.isEmpty() || code.length > Proto.MAX_DELETE_CODE) return
        runPromo(PromoAct.DELETE, Proto.OP_PROMO_DELETE) { pr, seq, ts -> Proto.promoDeleteData(pr.pcId, pr.phoneId, seq, ts, code) }
    }

    /** Новая форма (лист «Создать код» или «Придумать коды»): прежний итог и прежние коды не показываются. */
    fun clearPromoNote() {
        if (promoBusy != null) return
        promoNote = null
        genCodes = emptyList()
    }

    private fun runPromo(act: PromoAct, op: String, build: (Proto.Peer, Long, Long) -> ByteArray) {
        if (isBusy || peer == null || !pcOnline) return
        promoBusy = act
        promoNote = null
        if (act == PromoAct.GEN) genCodes = emptyList()
        viewModelScope.launch {
            val out = withContext(Dispatchers.IO) { sendCommand(op, build) }
            promoBusy = null
            if (out.repair) needRepair = true
            // Список пришёл (ok:true, promo_exists, not_found) — показываем его; нет — остаётся прежний кэш.
            out.reply?.promos?.let {
                promos = it
                promosTs = prefs.promosTs
            }
            if (act == PromoAct.GEN && out.problem == null) genCodes = out.reply?.codes.orEmpty()
            promoNote = PromoNote(act, out.problem == null, out.reply?.message.orEmpty(), out.problem)
        }
    }

    // ---------- решения о программах (протокол, раздел 5д) ----------

    /** «Игра», «Учёба», «Прочее», «Запретить»: решение для всех детей (R12). */
    fun decide(key: String, cat: String) {
        if (key.isEmpty() || cat !in Proto.CATS || isBusy || peer == null || !pcOnline) return
        decideBusy = key
        decideNote = null
        viewModelScope.launch {
            val out = withContext(Dispatchers.IO) {
                sendCommand(Proto.OP_DECIDE) { pr, seq, ts -> Proto.decideData(pr.pcId, pr.phoneId, seq, ts, key, cat) }
            }
            decideBusy = null
            out.reply?.report?.let { report = Proto.keepToday(it, report) }
            if (out.repair) needRepair = true
            decideNote = DecideNote(key, out.problem == null, out.reply?.message.orEmpty(), out.problem, System.nanoTime())
            refresh() // «Решение уже принято» — список обновляется отчётом
        }
    }

    private class CmdOutcome(val reply: Proto.CmdReply? = null, val problem: Problem? = null, val repair: Boolean = false)

    /**
     * Отправка в фоновом потоке; состояние экрана меняет вызывающий в главном. op — какую команду шлём:
     * от него зависит текст отказа (Problem.fromCmd). Номер seq записан до отправки и не возвращается.
     */
    private fun sendCommand(op: String, build: (Proto.Peer, Long, Long) -> ByteArray): CmdOutcome {
        fun fail(p: Problem, repair: Boolean = false) = CmdOutcome(problem = p, repair = repair)
        val pr = peer ?: return fail(Problem.NOT_PAIRED)
        val key = PhoneKey.load() ?: return fail(Problem.NOT_PAIRED, repair = true)
        return try {
            val seq = prefs.nextSeq()
            val data = build(pr, seq, System.currentTimeMillis())
            val signed = Crypto.seal(key, data)
            val m = RelayClient(prefs.url).send(prefs.phoneToken, Proto.KIND_CMD, Proto.signedToJson(signed))
            if (m.kind != Proto.KIND_CMDR) throw ProtoException("ожидался cmdr")
            val reply = Proto.openCmdReply(pr, Proto.signedFromJson(m.body), data, prefs.lastReportTs)
            reply.report?.let { prefs.saveReportTs(it.ts) }
            reply.promos?.let { prefs.savePromos(Proto.promosJson(it), if (reply.ts > 0) reply.ts else System.currentTimeMillis()) }
            if (reply.ok) {
                CmdOutcome(reply)
            } else {
                reply.lastSeq?.let { prefs.raiseSeq(it) }
                CmdOutcome(reply, Problem.fromCmd(reply.error, op))
            }
        } catch (e: RelayException) {
            fail(Problem.fromRelay(e.code), repair = e.code == RelayClient.UNAUTHORIZED)
        } catch (e: ProtoException) {
            fail(Problem.BAD_REPLY)
        } catch (e: Exception) {
            fail(Problem.KEY)
        }
    }

    // ---------- сообщение ребёнку (протокол, раздел 5а) ----------

    fun chooseShowSec(sec: Int) {
        prefs.notes.showSec = sec
        noteShowSec = prefs.notes.showSec
    }

    fun chooseSound(n: Int) {
        prefs.notes.sound = knownSound(n)
        noteSound = knownSound(prefs.notes.sound)
    }

    /** Выбран стикер ("" — «Без стикера»); незнакомый номер не выбирается. */
    fun chooseSticker(id: String) {
        if (noteBusy) return
        noteSticker = if (id in noteStickers) id else ""
    }

    private fun knownSound(n: Int): Int = when {
        n == Proto.NO_SOUND || n in noteSounds -> n
        else -> noteSounds.firstOrNull() ?: Proto.NO_SOUND
    }

    /** Открыт новый экран сообщения: прежняя ошибка не показывается, стикер — «Без стикера». */
    fun startNote() {
        if (noteBusy) return
        noteError = null
        noteSticker = ""
    }

    /** Текст поменялся: прежняя ошибка отправки не показывается. */
    fun clearNoteError() {
        if (!noteBusy) noteError = null
    }

    /** Отправить сообщение; onSent вызывается, когда компьютер его принял (экран закрывается). */
    fun sendNote(childId: Long, raw: String, onSent: () -> Unit) {
        if (isBusy || peer == null) return
        val text = Proto.cleanNote(raw)
        Proto.noteProblem(text)?.let {
            noteError = it
            return
        }
        val showSec = noteShowSec
        val sticker = noteSticker
        val sound = noteSound
        noteBusy = true
        noteError = null
        viewModelScope.launch {
            val out = withContext(Dispatchers.IO) { sendNoteNow(childId, text, showSec, sticker, sound) }
            noteBusy = false
            if (out.repair) needRepair = true
            if (out.saved != null) {
                notes = prefs.notes.all()
                recentStickers = prefs.recentStickers
                fastUntilMs = System.currentTimeMillis() + FAST_WINDOW_MS
                onSent()
                if (pollJob?.isActive == true) {
                    stopPolling()
                    startPolling() // сразу опрос, дальше раз в 3 с
                } else {
                    refresh()
                }
            } else {
                noteError = out.problem
            }
        }
    }

    private class NoteOutcome(val saved: SavedNote? = null, val problem: Problem? = null, val repair: Boolean = false)

    private fun sendNoteNow(childId: Long, text: String, showSec: Int, sticker: String, sound: Int): NoteOutcome {
        fun fail(p: Problem, repair: Boolean = false) = NoteOutcome(problem = p, repair = repair)
        val pr = peer ?: return fail(Problem.NOT_PAIRED)
        val key = PhoneKey.load() ?: return fail(Problem.NOT_PAIRED, repair = true)
        return try {
            val seq = prefs.nextSeq() // записан до отправки: общий счёт с командами
            // Повтор после обрыва без ответа — тот же id: компьютер не покажет сообщение дважды.
            val id = noteRetry.idFor(childId, text, showSec, sticker, sound) { Proto.newNoteId(rnd) }
            val now = System.currentTimeMillis()
            val data = Proto.noteData(pr.pcId, pr.phoneId, seq, now, childId, id, text, showSec, sticker, sound)
            val signed = Crypto.seal(key, data)
            val m = RelayClient(prefs.url).send(prefs.phoneToken, Proto.KIND_NOTE, Proto.signedToJson(signed))
            if (m.kind != Proto.KIND_NOTER) throw ProtoException("ожидался noter")
            val reply = Proto.openNoteReply(pr, Proto.signedFromJson(m.body), data)
            noteRetry.answered()
            if (reply.ok) {
                val n = SavedNote(id, childId, text, showSec, now)
                prefs.notes.add(n)
                // R-15: стикер попадает в «7 последних» только после принятой компьютером отправки.
                if (sticker.isNotEmpty()) prefs.pushRecentSticker(sticker)
                NoteOutcome(saved = n)
            } else {
                reply.lastSeq?.let { prefs.raiseSeq(it) }
                fail(Problem.fromNote(reply.error))
            }
        } catch (e: RelayException) {
            fail(Problem.fromRelayNote(e.code), repair = e.code == RelayClient.UNAUTHORIZED)
        } catch (e: ProtoException) {
            fail(Problem.BAD_REPLY)
        } catch (e: Exception) {
            fail(Problem.KEY)
        }
    }

    /** Отметки сообщений только из проверенного отчёта. */
    private fun applyNoteStates(rep: Proto.Report) {
        if (prefs.notes.apply(rep.notes)) notes = prefs.notes.all()
    }

    /**
     * «Отключить телефон»: забыть ключ и привязку только на телефоне. Компьютер отвязывает родитель
     * («Отвязать» в блоке «Телефон»): об этом говорит текст подтверждения (unpair_text).
     */
    fun unpair() {
        stopPolling()
        forgetAll()
        connectError = null
        screen = Screen.CONNECT
    }

    private fun forgetAll() {
        PhoneKey.forget()
        prefs.forget()
        peer = null
        report = null
        link = null
        note = null
        mainError = null
        needRepair = false
        busyChild = null
        notes = emptyList()
        noteBusy = false
        noteError = null
        noteRetry.answered() // новая привязка — неотвеченное прежнее не повторяется
        fastUntilMs = 0
        url = prefs.url
        resetScreens()
    }

    /** Новая привязка или отключение: вкладки, кэш промокодов и последние стикеры — с нуля. */
    private fun resetScreens() {
        tab = Tab.KIDS
        sub = null
        expandedChild = null
        promos = Proto.parsePromos(prefs.promosRaw)
        promosTs = prefs.promosTs
        promoBusy = null
        promoNote = null
        genCodes = emptyList()
        decideBusy = null
        decideNote = null
        recentStickers = prefs.recentStickers
    }

    private fun loadPeer(): Proto.Peer? = try {
        Proto.Peer(prefs.pcId, prefs.phoneId, Crypto.parsePublicB64(prefs.pcPub))
    } catch (e: ProtoException) {
        null
    }

    /** Последний отчёт, сохранённый на телефоне: проверяется подписью заново. */
    private fun loadSavedReport() {
        val pr = peer ?: return
        val raw = prefs.lastReport
        if (raw.isEmpty()) return
        report = try {
            Proto.openReport(pr, Proto.signedFromJson(org.json.JSONObject(raw)), 0)
        } catch (e: Exception) {
            null
        }
    }

    override fun onCleared() {
        stopPolling()
    }
}
