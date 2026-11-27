package ru.atlas404.pcblock.proto

/** Строка карточки ребёнка про ждущие программы (FR-47, протокол, раздел 6). */
object PendingView {
    /** Сколько строк pending с этим ребёнком в kids. */
    fun countFor(r: Proto.Report, child: Long): Int = r.pending.count { p -> p.kids.any { it.id == child } }

    /**
     * «1 программа ждёт решения» по строкам ребёнка; строк ребёнка нет, но список урезан
     * (pendingCount больше строк) — some («Есть программы, ждущие решения»); иначе null.
     */
    fun lineFor(r: Proto.Report, child: Long, tpl: Format.Plural, some: String): String? {
        val n = countFor(r, child)
        if (n > 0) return Format.plural(n, tpl)
        return if (r.pendingCount > r.pending.size) some else null
    }
}
