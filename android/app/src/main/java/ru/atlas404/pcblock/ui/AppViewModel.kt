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
import ru.atlas404.pcblock.proto.Crypto
import ru.atlas404.pcblock.proto.Format
import ru.atlas404.pcblock.proto.Problem
import ru.atlas404.pcblock.proto.Proto
import ru.atlas404.pcblock.proto.ProtoException
import ru.atlas404.pcblock.store.Prefs
import java.security.SecureRandom

enum class Screen { CONNECT, COMPARE, MAIN }

/** Итог команды для карточки ребёнка: текст компьютера («Добавлено 15 минут») или ошибка. */
data class CmdNote(val childId: Long, val ok: Boolean, val message: String, val problem: Problem?)

/** Состояние приложения и сценарий родителя (план M7, «UX-сценарий», шаги 4–8). */
class AppViewModel(app: Application) : AndroidViewModel(app) {
    companion object {
        const val POLL_MS = 15_000L
        const val COMPARE_TTL_MS = 10 * 60_000L
    }

    private val prefs = Prefs(app)
    private val rnd = SecureRandom()

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
    private data class Pending(
        val url: String,
        val pcId: String,
        val phoneId: String,
        val token: String,
        val pcPub: String,
        val sas: String,
        val startedMs: Long,
    )

    private var pending: Pending? = null
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

    /** Когда компьютер последний раз был на связи (для «не на связи с HH:MM» в отказе команды). */
    var lastSeenMs by mutableStateOf(0L)
        private set

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
    private fun pairNow(base: String, code: String): Result<Pending> = runCatching {
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
        Pending(
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
        mainError = null
        needRepair = false
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
                delay(POLL_MS)
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

    /** Команда доп. времени: +N или −N минут ребёнку (протокол, раздел 5). */
    fun command(childId: Long, delta: Int) {
        if (busyChild != null || peer == null) return
        if (delta == 0 || delta !in -Proto.MAX_DELTA..Proto.MAX_DELTA) {
            note = CmdNote(childId, false, "", Problem.RANGE)
            return
        }
        busyChild = childId
        note = null
        viewModelScope.launch {
            val out = withContext(Dispatchers.IO) { sendCommand(childId, delta) }
            busyChild = null
            out.report?.let { report = it }
            if (out.repair) needRepair = true
            note = out.note
            refresh()
        }
    }

    private class CmdOutcome(val note: CmdNote, val report: Proto.Report? = null, val repair: Boolean = false)

    /** Отправка в фоновом потоке; состояние экрана меняет вызывающий в главном. */
    private fun sendCommand(childId: Long, delta: Int): CmdOutcome {
        fun fail(p: Problem, repair: Boolean = false) = CmdOutcome(CmdNote(childId, false, "", p), repair = repair)
        val pr = peer ?: return fail(Problem.NOT_PAIRED)
        val key = PhoneKey.load() ?: return fail(Problem.NOT_PAIRED, repair = true)
        return try {
            val seq = prefs.nextSeq()
            val data = Proto.commandData(pr.pcId, pr.phoneId, seq, System.currentTimeMillis(), childId, delta)
            val signed = Crypto.seal(key, data)
            val m = RelayClient(prefs.url).send(prefs.phoneToken, Proto.KIND_CMD, Proto.signedToJson(signed))
            if (m.kind != Proto.KIND_CMDR) throw ProtoException("ожидался cmdr")
            val reply = Proto.openCmdReply(pr, Proto.signedFromJson(m.body), data, prefs.lastReportTs)
            reply.report?.let { prefs.saveReportTs(it.ts) }
            if (reply.ok) {
                CmdOutcome(CmdNote(childId, true, reply.message, null), reply.report)
            } else {
                reply.lastSeq?.let { prefs.raiseSeq(it) }
                CmdOutcome(CmdNote(childId, false, "", Problem.fromCmd(reply.error)), reply.report)
            }
        } catch (e: RelayException) {
            fail(Problem.fromRelay(e.code), repair = e.code == RelayClient.UNAUTHORIZED)
        } catch (e: ProtoException) {
            fail(Problem.BAD_REPLY)
        } catch (e: Exception) {
            fail(Problem.KEY)
        }
    }

    /** «Отключить телефон»: забыть ключ и привязку. */
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
        url = prefs.url
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
