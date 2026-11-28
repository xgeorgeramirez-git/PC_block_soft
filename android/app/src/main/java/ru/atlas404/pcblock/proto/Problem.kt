package ru.atlas404.pcblock.proto

/**
 * Что сказать родителю при сбое (протокол, разделы 3 и 5). resName — имя строки в strings.xml
 * (тест сверяет, что каждая строка есть); экран переводит её в R.string в ui/Texts.kt.
 */
enum class Problem(val resName: String) {
    BAD_ADDRESS("err_bad_address"),
    CODE_FORMAT("err_code_format"),
    BAD_CODE("err_bad_code"),
    TOO_MANY("err_too_many"),
    NETWORK("err_network"),
    PC_OFFLINE("err_pc_offline"),
    PC_OFFLINE_PAIR("err_pc_offline_pair"),
    BUSY("err_busy"),
    NOT_PAIRED("err_not_paired"),
    STALE("err_stale"),
    NO_CHILD("err_no_child"),
    RANGE("err_range"),
    PAIR_FAILED("err_pair_failed"),
    PAIR_CANCELLED("err_pair_cancelled"),
    BAD_REPLY("err_bad_reply"),
    SERVER("err_server"),
    KEY("err_key"),
    NOTE_RANGE("err_note_range"),
    NOTE_PC_OFFLINE("err_note_pc_offline"),

    // Протокол 1.5 (разделы 5в–5д): отказы компьютера и проверки полей на телефоне (тексты окна, R26).
    PROMO_EXISTS("err_promo_exists"),
    PROMO_GEN("err_promo_gen"),
    PROMO_GONE("err_promo_gone"),
    DECIDED_ALREADY("err_decided_already"),
    BANK_RANGE("err_bank_range"),
    PROMO_CODE_EMPTY("err_promo_code_empty"),
    PROMO_CODE_LEN("err_promo_code_len"),
    PROMO_CODE_LATIN("err_promo_code_latin"),
    PROMO_MINUTES("err_promo_minutes"),
    PROMO_COUNT("err_promo_count"),
    PROMO_LEN("err_promo_len"),

    // range у промокодов и решения (Ruling 7 T3): телефон поля проверил, значит компьютер прочёл их иначе.
    PROMO_RANGE("err_promo_range"),
    DECIDE_RANGE("err_decide_range"),
    ;

    companion object {
        /** Ошибка сервера связи или сети (код из RelayException). */
        fun fromRelay(code: String, pairing: Boolean = false): Problem = when (code) {
            "bad_code" -> BAD_CODE
            "too_many" -> TOO_MANY
            "network" -> NETWORK
            "bad_address" -> BAD_ADDRESS
            "pc_offline" -> if (pairing) PC_OFFLINE_PAIR else PC_OFFLINE
            "busy" -> BUSY
            "unauthorized" -> NOT_PAIRED
            "bad_response" -> if (pairing) PAIR_FAILED else BAD_REPLY
            else -> SERVER
        }

        /**
         * Отказ компьютера в ответе cmdr (протокол, разделы 5, 5в–5д). op — какую команду слали:
         * not_found у promoDelete — «Этого кода уже нет», у decide — «Решение уже принято»;
         * range у копилки — «Можно от 1 до 600 минут за раз», у промокодов — «Проверьте код и минуты»,
         * у решения — «Не удалось отправить решение, обновите список», у остальных — прежний текст.
         */
        fun fromCmd(code: String, op: String = ""): Problem = when (code) {
            "replay", "stale" -> STALE
            "not_paired", "bad_sig" -> NOT_PAIRED
            "no_child" -> NO_CHILD
            "range" -> when (op) {
                Proto.OP_BANK_ADD, Proto.OP_BANK_CLEAR -> BANK_RANGE
                Proto.OP_PROMO_LIST, Proto.OP_PROMO_CREATE, Proto.OP_PROMO_GEN, Proto.OP_PROMO_DELETE -> PROMO_RANGE
                Proto.OP_DECIDE -> DECIDE_RANGE
                else -> RANGE
            }
            "promo_exists" -> PROMO_EXISTS
            "promo_gen" -> PROMO_GEN
            "not_found" -> when (op) {
                Proto.OP_PROMO_DELETE -> PROMO_GONE
                Proto.OP_DECIDE -> DECIDED_ALREADY
                else -> BAD_REPLY
            }
            else -> BAD_REPLY
        }

        /** Отказ компьютера в ответе noter (раздел 5а): range — про длину текста. */
        fun fromNote(code: String): Problem = if (code == "range") NOTE_RANGE else fromCmd(code)

        /** Сбой связи при отправке сообщения: «не на связи» говорит, что не отправлено именно сообщение. */
        fun fromRelayNote(code: String): Problem = fromRelay(code).let { if (it == PC_OFFLINE) NOTE_PC_OFFLINE else it }
    }
}
