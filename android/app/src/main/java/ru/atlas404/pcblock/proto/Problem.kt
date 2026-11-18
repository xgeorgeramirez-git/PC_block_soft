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

        /** Отказ компьютера в ответе cmdr (протокол, раздел 5). */
        fun fromCmd(code: String): Problem = when (code) {
            "replay", "stale" -> STALE
            "not_paired", "bad_sig" -> NOT_PAIRED
            "no_child" -> NO_CHILD
            "range" -> RANGE
            else -> BAD_REPLY
        }
    }
}
