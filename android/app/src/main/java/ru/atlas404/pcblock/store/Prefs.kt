package ru.atlas404.pcblock.store

import android.content.Context
import android.content.SharedPreferences

/**
 * Настройки телефона: адрес сервера, привязка (pcId, phoneId, токен, ключ компьютера),
 * номер последней команды и время последнего показанного отчёта. Резервное копирование
 * запрещено манифестом: восстановленный seq без ключа Keystore сломал бы привязку.
 */
class Prefs(context: Context) {
    private val sp: SharedPreferences = context.getSharedPreferences("pcblock", Context.MODE_PRIVATE)

    companion object {
        const val DEFAULT_URL = "https://pcblock.atlas404.ru"
        private const val URL = "url"
        private const val PC_ID = "pcId"
        private const val PHONE_ID = "phoneId"
        private const val PHONE_TOKEN = "phoneToken"
        private const val PC_PUB = "pcPub"
        private const val SEQ = "seq"
        private const val LAST_REPORT_TS = "lastReportTs"
        private const val LAST_REPORT = "lastReport"
    }

    var url: String
        get() = sp.getString(URL, null) ?: DEFAULT_URL
        set(v) = sp.edit().putString(URL, v).apply()

    val pcId: String get() = sp.getString(PC_ID, "") ?: ""
    val phoneId: String get() = sp.getString(PHONE_ID, "") ?: ""
    val phoneToken: String get() = sp.getString(PHONE_TOKEN, "") ?: ""
    val pcPub: String get() = sp.getString(PC_PUB, "") ?: ""
    val seq: Long get() = sp.getLong(SEQ, 0)
    val lastReportTs: Long get() = sp.getLong(LAST_REPORT_TS, 0)

    /** Последний принятый отчёт ({"data","sig"} строкой): при запуске проверяется заново. */
    val lastReport: String get() = sp.getString(LAST_REPORT, "") ?: ""

    val paired: Boolean get() = pcId.isNotEmpty() && phoneId.isNotEmpty() && phoneToken.isNotEmpty() && pcPub.isNotEmpty()

    /** Привязка подтверждена («Совпадает»): счёт команд с 1, отчётов ещё нет. */
    fun savePairing(url: String, pcId: String, phoneId: String, phoneToken: String, pcPub: String) {
        sp.edit()
            .putString(URL, url)
            .putString(PC_ID, pcId)
            .putString(PHONE_ID, phoneId)
            .putString(PHONE_TOKEN, phoneToken)
            .putString(PC_PUB, pcPub)
            .putLong(SEQ, 0)
            .putLong(LAST_REPORT_TS, 0)
            .remove(LAST_REPORT)
            .commit()
    }

    /** Следующий номер команды; записан до отправки, чтобы номер не повторился даже после сбоя. */
    fun nextSeq(): Long {
        val n = seq + 1
        sp.edit().putLong(SEQ, n).commit()
        return n
    }

    /** Компьютер сообщил больший последний номер (отказ replay): догоняем его. */
    fun raiseSeq(atLeast: Long) {
        if (atLeast > seq) sp.edit().putLong(SEQ, atLeast).commit()
    }

    fun saveReport(ts: Long, raw: String) {
        sp.edit().putLong(LAST_REPORT_TS, ts).putString(LAST_REPORT, raw).apply()
    }

    /** Отчёт из ответа на команду (он внутри подписанного ответа): запоминаем только время. */
    fun saveReportTs(ts: Long) {
        if (ts > lastReportTs) sp.edit().putLong(LAST_REPORT_TS, ts).apply()
    }

    /** Забыть привязку; адрес сервера остаётся, чтобы не набирать его заново. */
    fun forget() {
        sp.edit()
            .remove(PC_ID).remove(PHONE_ID).remove(PHONE_TOKEN).remove(PC_PUB)
            .remove(SEQ).remove(LAST_REPORT_TS).remove(LAST_REPORT)
            .commit()
    }
}
