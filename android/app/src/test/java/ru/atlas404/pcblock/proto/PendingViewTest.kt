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
}
