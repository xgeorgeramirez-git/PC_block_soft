package ru.atlas404.pcblock.proto

/**
 * Проверки полей «Промокодов» на телефоне (FR-45, R26, 8.3.10 п. 2–3): те же правила и тексты, что окно
 * родителя (internal/service/bonus_promo_shared.go). Неверное не отправляется: компьютер ответил бы range.
 * Тексты — строки err_promo_* в strings.xml (Problem.resName).
 */
object PromoRules {
    const val MIN_CODE = 3
    const val MAX_CODE = 16

    /**
     * Новый код: после обрезки пробелов только ASCII A–Z, a–z, 0–9, 3…16 знаков. Ответ — (код заглавными, null)
     * или (null, ошибка): пусто — «Введите код»; латиница, но не та длина — «Код от 3 до 16 знаков»;
     * иначе — «Только латинские буквы A-Z и цифры».
     */
    fun checkCode(raw: String): Pair<String?, Problem?> {
        val c = raw.trim()
        if (c.isEmpty()) return null to Problem.PROMO_CODE_EMPTY
        if (!c.all { it in 'A'..'Z' || it in 'a'..'z' || it in '0'..'9' }) return null to Problem.PROMO_CODE_LATIN
        if (c.length !in MIN_CODE..MAX_CODE) return null to Problem.PROMO_CODE_LEN
        return c.uppercase(java.util.Locale.ROOT) to null
    }

    /** Минуты промокода: null — верно (1…600), иначе «Можно от 1 до 600 минут». */
    fun checkMinutes(raw: String): Problem? = if (inRange(raw, 1, Proto.MAX_PROMO_MIN)) null else Problem.PROMO_MINUTES

    /** Сколько кодов придумать: 1…50, иначе «Можно от 1 до 50 кодов». */
    fun checkCount(raw: String): Problem? = if (inRange(raw, 1, Proto.MAX_PROMO_COUNT)) null else Problem.PROMO_COUNT

    /** Длина придуманных кодов: 4…12, иначе «Длина от 4 до 12 знаков». */
    fun checkLen(raw: String): Problem? = if (inRange(raw, Proto.MIN_PROMO_LEN, Proto.MAX_PROMO_LEN)) null else Problem.PROMO_LEN

    /** Число поля: только цифры ASCII (без знака), в пределах lo…hi. */
    fun number(raw: String): Int? {
        val t = raw.trim()
        if (t.isEmpty() || !t.all { it in '0'..'9' }) return null
        return t.toIntOrNull()
    }

    internal fun inRange(raw: String, lo: Int, hi: Int): Boolean = number(raw)?.let { it in lo..hi } ?: false
}
