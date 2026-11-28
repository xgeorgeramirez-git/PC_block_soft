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

    /**
     * Кто запускал (экран «Ждут решения»): «Миша, 3 попытки; Аня, 1 попытка». Ребёнка нет среди детей
     * отчёта — gone («ребёнок удалён»); kids пуст — только общее число попыток.
     */
    fun who(p: Proto.Pending, names: Map<Long, String>, tries: Format.Plural, gone: String): String {
        if (p.kids.isEmpty()) return Format.plural(p.tries, tries)
        return p.kids.joinToString("; ") { k -> "${names[k.id] ?: gone}, ${Format.plural(k.tries, tries)}" }
    }

    /** Весь список ждущих (решение действует для всех детей), сверху строки этого ребёнка; порядок отчёта сохраняется. */
    fun sorted(r: Proto.Report, child: Long): List<Proto.Pending> {
        val (mine, rest) = r.pending.partition { p -> p.kids.any { it.id == child } }
        return mine + rest
    }

    /** Путь программы с точками переноса (U+200B) после каждой «\\»: длинный путь переносится по папкам. */
    fun breakable(path: String): String = path.replace("\\", "\\\u200B")
}
