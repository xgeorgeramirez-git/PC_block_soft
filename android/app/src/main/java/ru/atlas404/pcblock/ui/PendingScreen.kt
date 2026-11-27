package ru.atlas404.pcblock.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Format
import ru.atlas404.pcblock.proto.PendingView
import ru.atlas404.pcblock.proto.Proto
import java.time.ZoneId

/**
 * «Ждут решения» (FR-47, макет phone_pending.py): все ждущие программы отчёта, сверху — этого ребёнка;
 * название, издатель или «Без подписи издателя», кто и сколько раз запускал, путь; «Игра», «Учёба»,
 * «Прочее», «Запретить» — решение для всех детей (R12). Ответ — снэкбар «SuperGame: игра»; решение уже
 * принято — «Решение уже принято», список обновляется отчётом. Без связи кнопки недоступны.
 */
@Composable
fun PendingScreen(vm: AppViewModel, childId: Long) {
    BackHandler { vm.closeSub() }
    val rep = vm.report
    val online = vm.pcOnline
    val snack = remember { SnackbarHostState() }
    val dn = vm.decideNote
    val dnText = dn?.let { n ->
        val p = n.problem
        if (p != null) problemText(p, vm.lastSeenMs) else n.message.ifEmpty { stringResource(R.string.cmd_done) }
    }
    LaunchedEffect(dn) {
        if (dnText != null) snack.showSnackbar(dnText)
    }
    val names = rep?.children?.associate { it.id to it.name }.orEmpty()
    val list = if (rep == null) emptyList() else PendingView.sorted(rep, childId)

    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            SubTopBar(stringResource(R.string.pending_title), onBack = { vm.closeSub() })
            LazyColumn(
                verticalArrangement = Arrangement.spacedBy(14.dp),
                contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 80.dp),
                modifier = Modifier.fillMaxSize(),
            ) {
                item(key = "lead") { MutedText(stringResource(R.string.pending_lead), size = 14) }
                if (!online) item(key = "offline") { OfflineHint(vm.lastSeenMs) }
                if (list.isEmpty()) {
                    item(key = "empty") { Text(stringResource(R.string.pending_empty), fontSize = 16.sp, color = Tok.Ink) }
                }
                items(list, key = { it.id }) { p ->
                    PendingCard(
                        p = p,
                        names = names,
                        enabled = online && !vm.isBusy,
                        busy = vm.decideBusy == p.id,
                        onDecide = { cat -> vm.decide(p.id, cat) },
                    )
                }
                if (rep != null) item(key = "at") { MutedText(stringResource(R.string.data_at, clockText(rep.ts))) }
            }
        }
        SnackbarHost(snack, Modifier.align(Alignment.BottomCenter))
    }
}

@Composable
private fun PendingCard(p: Proto.Pending, names: Map<Long, String>, enabled: Boolean, busy: Boolean, onDecide: (String) -> Unit) {
    val zone = ZoneId.systemDefault()
    val who = PendingView.who(
        p,
        names,
        Format.Plural(
            stringResource(R.string.pending_tries_one),
            stringResource(R.string.pending_tries_few),
            stringResource(R.string.pending_tries_many),
        ),
        stringResource(R.string.pending_kid_gone),
    )
    val whoLine = when {
        p.lastMs <= 0 -> who
        Format.sameDay(p.lastMs, System.currentTimeMillis(), zone) -> stringResource(R.string.pending_last_today, who, clockText(p.lastMs))
        else -> stringResource(R.string.pending_last_day, who, dayText(p.lastMs), hmText(p.lastMs))
    }
    AppCard {
        Column {
            Text(p.name, fontSize = 18.sp, fontWeight = FontWeight.Bold, color = Tok.Ink)
            MutedText(p.pub.ifEmpty { stringResource(R.string.pending_no_pub) })
        }
        Text(whoLine, fontSize = 14.sp, fontWeight = FontWeight.Medium, color = Tok.Ink)
        Text(PendingView.breakable(p.path), fontSize = 12.sp, fontWeight = FontWeight.Medium, color = Tok.Muted)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
            DecideButton(stringResource(R.string.pending_game), p.name, Tok.PlusText, Tok.PlusBg, Tok.PlusBg, enabled, Modifier.weight(1f)) {
                onDecide(Proto.CAT_GAMES)
            }
            DecideButton(stringResource(R.string.pending_study), p.name, Tok.Accent, Tok.AccentSoft, Tok.AccentSoft, enabled, Modifier.weight(1f)) {
                onDecide(Proto.CAT_STUDY)
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
            DecideButton(stringResource(R.string.pending_other), p.name, Tok.Ink, Tok.Surface, Tok.Line, enabled, Modifier.weight(1f)) {
                onDecide(Proto.CAT_OTHER)
            }
            DecideButton(stringResource(R.string.pending_block), p.name, Tok.RemainRed, Tok.DangerBg, Tok.DangerBg, enabled, Modifier.weight(1f)) {
                onDecide(Proto.CAT_BLOCKED)
            }
        }
        if (busy) StatusLine(busyText = stringResource(R.string.pending_sending), okText = null, problem = null, lastSeenMs = 0)
    }
}

/** Кнопка решения 48 dp; для TalkBack — «Игра: SuperGame». */
@Composable
private fun DecideButton(
    label: String,
    program: String,
    fg: Color,
    bg: Color,
    line: Color,
    enabled: Boolean,
    modifier: Modifier,
    onClick: () -> Unit,
) {
    val desc = stringResource(R.string.pending_decide_desc, label, program)
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        shape = Tok.ButtonShape,
        border = BorderStroke(1.dp, line),
        colors = ButtonDefaults.outlinedButtonColors(
            containerColor = bg,
            contentColor = fg,
            disabledContainerColor = Tok.Surface,
            disabledContentColor = Tok.Muted,
        ),
        contentPadding = PaddingValues(horizontal = 8.dp),
        modifier = modifier
            .heightIn(min = 48.dp)
            .semantics { contentDescription = desc },
    ) { Text(label, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.clearAndSetSemantics { }) }
}
