package ru.atlas404.pcblock.proto

import java.net.URI
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/**
 * Чистые функции экранов: длительности, время, «на связи», адрес. Шаблоны текстов приходят
 * из strings.xml (тесты читают тот же файл), чтобы тексты жили в одном месте.
 */
object Format {
    /** Компьютер «на связи», если опрашивал сервер не дальше минуты назад (протокол, раздел 6). */
    const val ONLINE_WINDOW_MS = 60_000L

    /** Длительность в секундах: «40 мин», «1 ч 05 мин» (как окно родителя). Неполная минута отбрасывается. */
    fun duration(seconds: Long, minTpl: String, hmTpl: String): String {
        val total = maxOf(0L, seconds / 60)
        return if (total < 60) {
            String.format(Locale.ROOT, minTpl, total)
        } else {
            String.format(Locale.ROOT, hmTpl, total / 60, total % 60)
        }
    }

    private val HM = DateTimeFormatter.ofPattern("HH:mm", Locale.ROOT)
    private val DM = DateTimeFormatter.ofPattern("dd.MM", Locale.ROOT)

    /** «19:05» для сегодняшнего времени, «17.11 19:05» для другого дня (местное время телефона). */
    fun clock(ms: Long, nowMs: Long, zone: ZoneId, otherDayTpl: String): String {
        val t = Instant.ofEpochMilli(ms).atZone(zone)
        val now = Instant.ofEpochMilli(nowMs).atZone(zone)
        val hm = HM.format(t)
        return if (t.toLocalDate() == now.toLocalDate()) hm else String.format(Locale.ROOT, otherDayTpl, DM.format(t), hm)
    }

    /** Доступность компьютера по ответу phone/status. */
    sealed interface Link {
        data object Online : Link
        /** sinceMs = 0 — компьютер ещё ни разу не выходил на связь. */
        data class Offline(val sinceMs: Long) : Link
    }

    fun link(serverNowMs: Long, pcLastSeenMs: Long): Link =
        if (pcLastSeenMs > 0 && serverNowMs - pcLastSeenMs <= ONLINE_WINDOW_MS) Link.Online else Link.Offline(pcLastSeenMs)

    /** Адрес сервера: только https, есть имя сервера, без запроса и якоря; «/» в конце убираем. null — неверный. */
    fun baseUrl(raw: String): String? {
        val s = raw.trim().trimEnd('/')
        if (!s.startsWith("https://", ignoreCase = true)) return null
        val uri = try {
            URI(s)
        } catch (e: Exception) {
            return null
        }
        if (!uri.scheme.equals("https", ignoreCase = true)) return null
        if (uri.host.isNullOrEmpty() || uri.rawQuery != null || uri.rawFragment != null || uri.rawUserInfo != null) return null
        return s
    }

    /** Код привязки: ровно 6 цифр (пробелы, которые родитель мог набрать, убираем). */
    fun pairCode(raw: String): String? {
        val s = raw.filterNot { it.isWhitespace() }
        return if (s.length == 6 && s.all { it in '0'..'9' }) s else null
    }

    /** Число сверки для экрана: «731004» → «731 004». */
    fun sas(s: String): String = if (s.length == 6) s.substring(0, 3) + " " + s.substring(3) else s

    /** Своё число минут: 1…240, иначе null. */
    fun minutes(raw: String): Int? {
        val n = raw.trim().toIntOrNull() ?: return null
        return if (n in 1..Proto.MAX_DELTA) n else null
    }
}
