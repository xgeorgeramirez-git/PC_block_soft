package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Строка карточки «1 программа ждёт решения» (FR-47, протокол, раздел 6). */
class PendingViewTest {
    private val tpl = Format.Plural(Strings["card_pending_one"], Strings["card_pending_few"], Strings["card_pending_many"])
    private fun line(r: Proto.Report, child: Long) = PendingView.lineFor(r, child, tpl, Strings["card_pending_some"])

    private fun p(id: String, vararg kids: Long) = Proto.Pending(id, id, "", "C:\\$id.exe", kids.map { Proto.PendingKid(it, 1) }, kids.size, 1)

    private fun rep(n: Int, kid: Long = 3, total: Int = n) =
        Proto.Report(1, emptyList(), pending = (1..n).map { p("g$it", kid) }, pendingCount = total)

    @Test
    fun forms() {
        assertEquals("1 программа ждёт решения", line(rep(1), 3))
        assertEquals("2 программы ждут решения", line(rep(2), 3))
        assertEquals("5 программ ждут решения", line(rep(5), 3))
        assertEquals("21 программа ждёт решения", line(rep(21), 3))
        assertEquals("11 программ ждут решения", line(rep(11), 3))
        assertEquals("12 программ ждут решения", line(rep(12), 3))
        assertEquals("22 программы ждут решения", line(rep(22), 3))
    }

    @Test
    fun onlyThisChild() {
        val r = Proto.Report(1, emptyList(), pending = listOf(p("a", 3, 4), p("b", 4), p("c", 4)), pendingCount = 3)
        assertEquals(1, PendingView.countFor(r, 3))
        assertEquals(3, PendingView.countFor(r, 4))
        assertEquals("1 программа ждёт решения", line(r, 3))
        assertNull(line(r, 5))
    }

    /** Список урезан (pendingCount больше строк), строк ребёнка нет — общая фраза; ничего не ждёт — строки нет. */
    @Test
    fun clipped() {
        assertEquals("Есть программы, ждущие решения", line(rep(0, total = 4), 3))
        assertEquals("Есть программы, ждущие решения", line(rep(2, kid = 4, total = 5), 3))
        assertNull(line(rep(2, kid = 4, total = 2), 3))
        assertNull(line(rep(0), 3))
    }

    private val tries = Format.Plural(Strings["pending_tries_one"], Strings["pending_tries_few"], Strings["pending_tries_many"])
    private val names = mapOf(3L to "Миша", 4L to "Аня")

    /** «Миша, 3 попытки; Аня, 1 попытка» — кто запускал (FR-47); ребёнка нет в отчёте — «ребёнок удалён». */
    @Test
    fun who() {
        val a = Proto.Pending("a", "SuperGame", "", "C:\\a.exe", listOf(Proto.PendingKid(3, 3), Proto.PendingKid(4, 1)), 4, 1)
        assertEquals("Миша, 3 попытки; Аня, 1 попытка", PendingView.who(a, names, tries, Strings["pending_kid_gone"]))
        val b = a.copy(kids = listOf(Proto.PendingKid(9, 5)))
        assertEquals("ребёнок удалён, 5 попыток", PendingView.who(b, names, tries, Strings["pending_kid_gone"]))
        val c = a.copy(kids = emptyList(), tries = 2)
        assertEquals("2 попытки", PendingView.who(c, names, tries, Strings["pending_kid_gone"]))
    }

    /** Весь список ждущих, сверху строки этого ребёнка; внутри групп порядок отчёта. */
    @Test
    fun sortedForChild() {
        val r = Proto.Report(1, emptyList(), pending = listOf(p("a", 4), p("b", 3), p("c", 4, 3), p("d", 5)), pendingCount = 4)
        assertEquals(listOf("b", "c", "a", "d"), PendingView.sorted(r, 3).map { it.id })
        assertEquals(listOf("a", "c", "b", "d"), PendingView.sorted(r, 4).map { it.id })
    }

    /** Путь переносится в любом месте: после каждой «\\» — точка переноса нулевой ширины. */
    @Test
    fun breakablePath() {
        assertEquals("C:\\\u200BUsers\\\u200Bx.exe", PendingView.breakable("C:\\Users\\x.exe"))
    }
}
