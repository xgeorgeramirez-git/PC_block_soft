package ru.atlas404.pcblock.proto

/**
 * Отметка у сообщения на телефоне (протокол, раздел 5а). resName — строка в strings.xml
 * (тест сверяет), экран переводит её в R.string в ui/Texts.kt.
 */
enum class NoteMark(val resName: String) {
    /** Компьютер принял, ребёнок ещё не видел (queued или отчёта ещё нет). */
    SENT("note_mark_sent"),

    /** Плашка на экране ребёнка (shown). */
    DELIVERED("note_mark_delivered"),

    /** Нажал «Понял» (read при показе до «Понял»). */
    READ("note_mark_read"),

    /** Время показа вышло (read при показе на время). */
    SHOWN("note_mark_shown"),

    /** 10 минут ждало ребёнка и погасло (expired). */
    NOT_DELIVERED("note_mark_expired"),
    ;

    companion object {
        fun of(state: Proto.NoteState?, showSec: Int): NoteMark = when (state) {
            null, Proto.NoteState.QUEUED -> SENT
            Proto.NoteState.SHOWN -> DELIVERED
            Proto.NoteState.READ -> if (showSec > 0) SHOWN else READ
            Proto.NoteState.EXPIRED -> NOT_DELIVERED
        }
    }
}
