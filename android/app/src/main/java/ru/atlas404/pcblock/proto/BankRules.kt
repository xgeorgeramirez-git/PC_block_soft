package ru.atlas404.pcblock.proto

/**
 * Экран «Копилка» на телефоне (FR-44, D-113): за раз 1…600 минут, предела баланса нет.
 * Пустое, 0 или больше 600 — «Можно от 1 до 600 минут за раз» у поля, ничего не отправлено.
 */
object BankRules {
    /** null — верно, иначе BANK_RANGE. */
    fun checkBankMinutes(raw: String): Problem? =
        if (PromoRules.inRange(raw, 1, Proto.MAX_BANK_ADD)) null else Problem.BANK_RANGE

    /** Минуты из поля для команды: 1…600 или null. */
    fun minutes(raw: String): Int? = if (checkBankMinutes(raw) == null) PromoRules.number(raw) else null

    /** Надпись кнопки: «Положить 30 минут», «Положить 1 минуту»; пусто или неверно — plain («Положить»). */
    fun putLabel(raw: String, tpl: Format.Plural, plain: String): String =
        minutes(raw)?.let { Format.plural(it, tpl) } ?: plain
}
