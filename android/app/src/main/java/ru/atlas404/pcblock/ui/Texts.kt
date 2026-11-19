package ru.atlas404.pcblock.ui

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Format
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
}

/** «19:05» или «17.11 19:05» по местному времени телефона. */
@Composable
fun clockText(ms: Long): String =
    Format.clock(ms, System.currentTimeMillis(), ZoneId.systemDefault(), stringResource(R.string.time_other_day))

/** Текст ошибки; «компьютер не на связи» с временем, если оно известно. */
@Composable
fun problemText(p: Problem, lastSeenMs: Long): String =
    if (p == Problem.PC_OFFLINE && lastSeenMs > 0) {
        stringResource(R.string.err_pc_offline_since, clockText(lastSeenMs))
    } else {
        stringResource(p.res())
    }

@Composable
fun durationText(seconds: Long): String =
    Format.duration(seconds, stringResource(R.string.dur_min), stringResource(R.string.dur_hm))
