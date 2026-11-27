package ru.atlas404.pcblock.proto

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Отказы протокола 1.5 (разделы 5в–5д): тексты телефона те же, что окна; каждый resName есть в strings.xml. */
class ProblemTest {
    private fun text(code: String, op: String = "") = Strings[Problem.fromCmd(code, op).resName]

    @Test
    fun everyResNameExists() {
        for (p in Problem.values()) assertTrue(p.resName, Strings.all.containsKey(p.resName))
    }

    @Test
    fun notFoundDependsOnOp() {
        assertEquals("Этого кода уже нет. Список обновлён", text("not_found", Proto.OP_PROMO_DELETE))
        assertEquals("Решение уже принято", text("not_found", Proto.OP_DECIDE))
        assertEquals(Problem.BAD_REPLY, Problem.fromCmd("not_found"))
    }

    @Test
    fun rangeOfBankIsPerCall() {
        assertEquals("Можно от 1 до 600 минут за раз", text("range", Proto.OP_BANK_ADD))
        assertEquals("Можно от 1 до 600 минут за раз", text("range", Proto.OP_BANK_CLEAR))
        assertEquals("Можно от 1 до 240 минут", text("range"))
        assertEquals("Можно от 1 до 240 минут", text("range", Proto.OP_NO_FORBIDDEN))
    }

    /**
     * Ruling 7 T3: range у промокодов и решения — свои тексты, а не «от 1 до 240 минут». Телефон
     * проверяет поля сам, range приходит, только если компьютер прочёл поле иначе.
     */
    @Test
    fun rangeOfPromosAndDecide() {
        for (op in listOf(Proto.OP_PROMO_LIST, Proto.OP_PROMO_CREATE, Proto.OP_PROMO_GEN, Proto.OP_PROMO_DELETE)) {
            assertEquals(op, "Проверьте код и минуты", text("range", op))
        }
        assertEquals("Не удалось отправить решение, обновите список", text("range", Proto.OP_DECIDE))
    }

    @Test
    fun promoRejects() {
        assertEquals("Такой код уже есть, придумайте другой", text("promo_exists", Proto.OP_PROMO_CREATE))
        assertEquals("Не получилось придумать столько разных кодов. Увеличьте длину", text("promo_gen", Proto.OP_PROMO_GEN))
    }

    /** Прежние отказы не зависят от op. */
    @Test
    fun oldRejectsKeepTexts() {
        for (op in listOf("", Proto.OP_BANK_ADD, Proto.OP_PROMO_LIST, Proto.OP_DECIDE)) {
            assertEquals("Команда устарела, повторите", text("stale", op))
            assertEquals("Команда устарела, повторите", text("replay", op))
            assertEquals("Ребёнок не найден", text("no_child", op))
            assertEquals(Problem.NOT_PAIRED, Problem.fromCmd("bad_sig", op))
        }
    }
}
