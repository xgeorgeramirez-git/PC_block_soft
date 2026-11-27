package ru.atlas404.pcblock.ui

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Format
import ru.atlas404.pcblock.proto.NoteMark
import ru.atlas404.pcblock.proto.Problem
import java.time.ZoneId

@StringRes
fun Problem.res(): Int = when (this) {
    Problem.BAD_ADDRESS -> R.string.err_bad_address
    Problem.CODE_FORMAT -> R.string.err_code_format
    Problem.BAD_CODE -> R.string.err_bad_code
    Problem.TOO_MANY -> R.string.err_too_many
    Problem.NETWORK -> R.string.err_network
    Problem.PC_OFFLINE -> R.string.err_pc_offline
    Problem.PC_OFFLINE_PAIR -> R.string.err_pc_offline_pair
    Problem.BUSY -> R.string.err_busy
    Problem.NOT_PAIRED -> R.string.err_not_paired
    Problem.STALE -> R.string.err_stale
    Problem.NO_CHILD -> R.string.err_no_child
    Problem.RANGE -> R.string.err_range
    Problem.PAIR_FAILED -> R.string.err_pair_failed
    Problem.PAIR_CANCELLED -> R.string.err_pair_cancelled
    Problem.BAD_REPLY -> R.string.err_bad_reply
    Problem.SERVER -> R.string.err_server
    Problem.KEY -> R.string.err_key
    Problem.NOTE_RANGE -> R.string.err_note_range
    Problem.NOTE_PC_OFFLINE -> R.string.err_note_pc_offline
    Problem.PROMO_EXISTS -> R.string.err_promo_exists
    Problem.PROMO_GEN -> R.string.err_promo_gen
    Problem.PROMO_GONE -> R.string.err_promo_gone
    Problem.DECIDED_ALREADY -> R.string.err_decided_already
    Problem.BANK_RANGE -> R.string.err_bank_range
    Problem.PROMO_CODE_EMPTY -> R.string.err_promo_code_empty
    Problem.PROMO_CODE_LEN -> R.string.err_promo_code_len
    Problem.PROMO_CODE_LATIN -> R.string.err_promo_code_latin
    Problem.PROMO_MINUTES -> R.string.err_promo_minutes
    Problem.PROMO_COUNT -> R.string.err_promo_count
    Problem.PROMO_LEN -> R.string.err_promo_len
    Problem.PROMO_RANGE -> R.string.err_promo_range
    Problem.DECIDE_RANGE -> R.string.err_decide_range
}

/** «19:05» или «17.11 19:05» по местному времени телефона. */
@Composable
fun clockText(ms: Long): String =
    Format.clock(ms, System.currentTimeMillis(), ZoneId.systemDefault(), stringResource(R.string.time_other_day))

/** «25 ноября» по местному времени телефона (промокоды, «Ждут решения»). */
@Composable
fun dayText(ms: Long): String = Format.dayMonth(ms, ZoneId.systemDefault(), stringArrayResource(R.array.months_gen).toList())

/** «18:40» по местному времени телефона (всегда без даты). */
fun hmText(ms: Long): String = Format.clock(ms, ms, ZoneId.systemDefault(), "%1\$s %2\$s")

/** Текст ошибки; «компьютер не на связи» с временем, если оно известно. */
@Composable
fun problemText(p: Problem, lastSeenMs: Long): String =
    if (p == Problem.PC_OFFLINE && lastSeenMs > 0) {
        stringResource(R.string.err_pc_offline_since, clockText(lastSeenMs))
    } else if (p == Problem.NOTE_PC_OFFLINE && lastSeenMs > 0) {
        stringResource(R.string.err_note_pc_offline_since, clockText(lastSeenMs))
    } else {
        stringResource(p.res())
    }

/** Отметка сообщения на телефоне. */
@StringRes
fun NoteMark.res(): Int = when (this) {
    NoteMark.SENT -> R.string.note_mark_sent
    NoteMark.WAITING_OFFLINE -> R.string.note_mark_waiting_offline
    NoteMark.DELIVERED -> R.string.note_mark_delivered
    NoteMark.READ -> R.string.note_mark_read
    NoteMark.SHOWN -> R.string.note_mark_shown
    NoteMark.NOT_DELIVERED -> R.string.note_mark_expired
}

/** Подпись выбора показа. */
@StringRes
fun showSecRes(sec: Int): Int = when (sec) {
    10 -> R.string.note_show_10
    30 -> R.string.note_show_30
    60 -> R.string.note_show_60
    else -> R.string.note_show_until
}

@Composable
fun durationText(seconds: Long): String =
    Format.duration(seconds, stringResource(R.string.dur_min), stringResource(R.string.dur_hm))
