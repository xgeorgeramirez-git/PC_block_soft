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

    /** Время в статистике: «1 ч 20 мин», «15 мин», «меньше минуты» (1…59 с), «0 мин» (ни секунды). */
    fun spent(seconds: Long, minTpl: String, hmTpl: String, lessTpl: String): String =
        if (seconds in 1..59) lessTpl else duration(seconds, minTpl, hmTpl)

    private val HM = DateTimeFormatter.ofPattern("HH:mm", Locale.ROOT)
    private val DM = DateTimeFormatter.ofPattern("dd.MM", Locale.ROOT)

    /** «19:05» для сегодняшнего времени, «17.11 19:05» для другого дня (местное время телефона). */
    fun clock(ms: Long, nowMs: Long, zone: ZoneId, otherDayTpl: String): String {
        val t = Instant.ofEpochMilli(ms).atZone(zone)
        val now = Instant.ofEpochMilli(nowMs).atZone(zone)
        val hm = HM.format(t)
        return if (t.toLocalDate() == now.toLocalDate()) hm else String.format(Locale.ROOT, otherDayTpl, DM.format(t), hm)
    }

    /** «25 ноября» (местное время телефона); months — родительный падеж из strings.xml (months_gen). */
    fun dayMonth(ms: Long, zone: ZoneId, months: List<String>): String {
        val t = Instant.ofEpochMilli(ms).atZone(zone)
        return "${t.dayOfMonth} ${months.getOrElse(t.monthValue - 1) { t.monthValue.toString() }}"
    }

    /** Один ли день у двух моментов по поясу телефона. */
    fun sameDay(a: Long, b: Long, zone: ZoneId): Boolean =
        Instant.ofEpochMilli(a).atZone(zone).toLocalDate() == Instant.ofEpochMilli(b).atZone(zone).toLocalDate()

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

    /** Шаблоны числа по-русски («%1$d минуту», «%1$d минуты», «%1$d минут») из strings.xml. */
    data class Plural(val one: String, val few: String, val many: String)

    /** 1, 21 — one; 2…4, 22…24 — few; 0, 5…20, 11…14 — many. */
    fun plural(n: Int, p: Plural): String {
        val a = Math.abs(n)
        val form = when {
            a % 10 == 1 && a % 100 != 11 -> p.one
            a % 10 in 2..4 && a % 100 !in 12..14 -> p.few
            else -> p.many
        }
        return String.format(Locale.ROOT, form, n)
    }

    /**
     * Имя в родительном падеже («Копилка Миши») — те же правила, что genitiveName окна
     * (frontend/src/lib/format.ts): только кириллица и дефис, иначе имя как есть.
     */
    fun genitive(name: String): String {
        val n = name.trim()
        if (n.isEmpty() || !CYR_NAME.matches(n)) return n
        val last = n.last().lowercaseChar()
        val stem = n.dropLast(1)
        val prev = if (n.length >= 2) n[n.length - 2].lowercaseChar() else ' '
        return when (last) {
            'а' -> stem + if (prev in "гкхжчшщ") "и" else "ы"
            'я' -> stem + "и"
            'й', 'ь' -> stem + "я"
            'е', 'и', 'о', 'у', 'ы', 'ю', 'э' -> n
            else -> n + "а"
        }
    }

    private val CYR_NAME = Regex("^[А-ЯЁа-яё-]+$")

    /** Своё число минут: 1…240, иначе null. */
    fun minutes(raw: String): Int? {
        val n = raw.trim().toIntOrNull() ?: return null
        return if (n in 1..Proto.MAX_DELTA) n else null
    }
}
