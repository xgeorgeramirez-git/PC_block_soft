package ru.atlas404.pcblock.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Format
import ru.atlas404.pcblock.proto.PendingView
import ru.atlas404.pcblock.proto.Proto
import ru.atlas404.pcblock.proto.Remain

/** Действия карточки ребёнка: всё, что открывает экраны или шлёт команды. */
class CardActions(
    val onDelta: (Int) -> Unit,
    val onCustom: () -> Unit,
    val onBank: () -> Unit,
    val onPending: () -> Unit,
    val onNote: () -> Unit,
    val onStats: () -> Unit,
    val onClearForbidden: () -> Unit,
)

/** «За компьютером с 18:20» / «Не за компьютером с 17:05»; статистики нет — «Данные на HH:MM». */
@Composable
private fun presence(child: Proto.Child, reportTs: Long): String {
    val t = child.today ?: return stringResource(R.string.data_at, clockText(reportTs))
    return when {
        t.nowAt && t.sinceMs > 0 -> stringResource(R.string.stats_at_pc_since, clockText(t.sinceMs))
        !t.nowAt && t.sinceMs > 0 -> stringResource(R.string.stats_away_since, clockText(t.sinceMs))
        else -> stringResource(R.string.stats_away)
    }
}

/** Доля остатка для полосы: остаток / (потрачено + остаток); без лимита — полная. */
private fun remainFraction(child: Proto.Child): Float {
    if (child.leftSec < 0) return 1f
    val total = child.usedSec + child.leftSec
    return if (total <= 0) 0f else child.leftSec.toFloat() / total.toFloat()
}

/**
 * Развёрнутая карточка ребёнка (D-109, макет phone2.py): аватар, имя, присутствие, монета копилки;
 * остаток «1:25» цветом тона (Remain.tone) с полосой и «осталось из 2:00»; игры и видео;
 * сетка «+15 +30 +60 Своё / −15 −30 −60 В копилку»; строка ждущих программ; ссылки.
 */
@Composable
fun ChildCard(
    child: Proto.Child,
    index: Int,
    report: Proto.Report,
    busy: Boolean,
    locked: Boolean,
    note: CmdNote?,
    lastSeenMs: Long,
    actions: CardActions,
) {
    AppCard {
        // ---------- шапка ----------
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Avatar(child.name, index)
            Column(Modifier.weight(1f)) {
                Text(
                    child.name,
                    fontSize = 18.sp,
                    fontWeight = FontWeight.Bold,
                    color = Tok.Ink,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.semantics { heading() },
                )
                MutedText(presence(child, report.ts))
            }
            child.bankMin?.let { min ->
                val desc = stringResource(R.string.card_bank_min_desc, min)
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    modifier = Modifier.clearAndSetSemantics { contentDescription = desc },
                ) {
                    CoinIcon(20.dp)
                    Text(stringResource(R.string.card_bank_min, min), fontSize = 14.sp, fontWeight = FontWeight.SemiBold, color = Tok.Ink, style = NumStyle)
                }
            }
        }

        // ---------- остаток ----------
        RemainBlock(child)
        if (child.blocked) MutedText(stringResource(R.string.child_blocked), size = 14)
        if (child.extraMin != 0) MutedText(stringResource(R.string.child_extra, durationText(child.extraMin * 60L)), size = 14)
        LimitsLine(child)

        // ---------- кнопки времени и копилки ----------
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
            for (m in Proto.QUICK_ADD) {
                Chip(
                    label = stringResource(R.string.card_plus, m),
                    desc = stringResource(R.string.btn_add_desc, m, child.name),
                    fg = Tok.PlusText,
                    bg = Tok.PlusBg,
                    line = Tok.PlusBg,
                    enabled = !locked,
                    onClick = { actions.onDelta(m) },
                    modifier = Modifier.weight(1f),
                )
            }
            Chip(
                label = stringResource(R.string.card_own),
                desc = stringResource(R.string.btn_custom_desc, child.name),
                fg = Tok.Ink,
                bg = Tok.Surface,
                line = Tok.Line,
                enabled = !locked,
                onClick = actions.onCustom,
                modifier = Modifier.weight(1f),
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
            for (m in Proto.QUICK_ADD) {
                Chip(
                    label = stringResource(R.string.card_minus, m),
                    desc = stringResource(R.string.btn_sub_desc, m, child.name),
                    fg = Tok.MinusText,
                    bg = Tok.MinusBg,
                    line = Tok.MinusBg,
                    enabled = !locked,
                    onClick = { actions.onDelta(-m) },
                    modifier = Modifier.weight(1f),
                )
            }
            if (child.bankMin != null) {
                // Открывает экран «Копилка»: доступна и без связи (там же объяснено, почему кнопки недоступны).
                Chip(
                    label = stringResource(R.string.card_bank_btn),
                    desc = stringResource(R.string.card_bank_desc, child.name),
                    fg = Tok.GoldText,
                    bg = Tok.GoldBg,
                    line = Tok.GoldBg,
                    enabled = true,
                    onClick = actions.onBank,
                    modifier = Modifier.weight(1f),
                    fontSize = 13.sp,
                    coin = true,
                )
            } else {
                Spacer(Modifier.weight(1f))
            }
        }

        // ---------- ждут решения (FR-47) ----------
        val pendingLine = PendingView.lineFor(
            report,
            child.id,
            Format.Plural(
                stringResource(R.string.card_pending_one),
                stringResource(R.string.card_pending_few),
                stringResource(R.string.card_pending_many),
            ),
            stringResource(R.string.card_pending_some),
        )
        if (pendingLine != null && report.hasBank) PendingLine(pendingLine, actions.onPending)

        // ---------- ссылки ----------
        Column(Modifier.fillMaxWidth()) {
            LinkRow(AppIcons.Msg, stringResource(R.string.card_note), onClick = actions.onNote, enabled = !locked)
            // Статистика только читается: доступна и пока идёт команда.
            if (child.today != null) LinkRow(AppIcons.Chart, stringResource(R.string.btn_stats), onClick = actions.onStats)
            when (child.forbidView) {
                Proto.ForbidView.BUTTON -> LinkRow(
                    AppIcons.Unlock,
                    stringResource(R.string.btn_no_forbidden),
                    onClick = actions.onClearForbidden,
                    sub = child.activeForbidUntilMs?.let { stringResource(R.string.forbid_until, clockText(it)) },
                    enabled = !locked,
                )
                Proto.ForbidView.CLEARED -> MutedText(stringResource(R.string.forbid_cleared), Modifier.padding(top = 8.dp), size = 14)
                Proto.ForbidView.HIDDEN -> {}
            }
        }

        // Постоянная область итога команды: TalkBack зачитывает её сам.
        StatusLine(
            busyText = if (busy) stringResource(R.string.cmd_sending) else null,
            okText = note?.takeIf { it.ok }?.let { it.message.ifEmpty { stringResource(R.string.cmd_done) } },
            problem = note?.problem,
            lastSeenMs = lastSeenMs,
        )
    }
}

/** «1:25» 40 sp цветом тона + «осталось из 2:00» и полоса 10 dp; без лимита — «Без лимита». */
@Composable
private fun RemainBlock(child: Proto.Child) {
    val tone = Remain.tone(child.leftSec)
    if (child.leftSec < 0) {
        Text(
            stringResource(R.string.child_unlimited),
            fontSize = 28.sp,
            fontWeight = FontWeight.ExtraBold,
            color = Tok.remainText(tone, large = true),
        )
        return
    }
    val left = Remain.remainMinutes(child.leftSec)
    val total = Remain.remainMinutes(child.usedSec + child.leftSec)
    val desc = stringResource(R.string.card_left_desc, durationText(left * 60), durationText(total * 60))
    Column(verticalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.clearAndSetSemantics { contentDescription = desc }) {
        Row(verticalAlignment = Alignment.Bottom, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                Remain.hm(left),
                fontSize = 40.sp,
                fontWeight = FontWeight.ExtraBold,
                letterSpacing = (-0.5).sp,
                color = Tok.remainText(tone, large = true),
                style = NumStyle,
                modifier = Modifier.alignByBaseline(),
            )
            Text(
                stringResource(R.string.card_left_of, Remain.hm(total)),
                fontSize = 14.sp,
                fontWeight = FontWeight.Medium,
                color = Tok.Muted,
                style = NumStyle,
                modifier = Modifier.alignByBaseline(),
            )
        }
        RemainBar(remainFraction(child), Tok.remainBar(tone), 10.dp)
    }
}

/** «Игры 40 из 60 мин», «Видео и соцсети 10 из 30 мин»; лимит −1 — «Игры 40 мин»; поля нет — строки нет. */
@Composable
private fun LimitsLine(child: Proto.Child) {
    val games = child.gamesSec?.let { used ->
        val lim = child.gamesLimSec ?: -1
        if (lim < 0) stringResource(R.string.card_games, used / 60) else stringResource(R.string.card_games_of, used / 60, lim / 60)
    }
    val video = child.videoSec?.let { used ->
        val lim = child.videoLimSec ?: -1
        if (lim < 0) stringResource(R.string.card_video, used / 60) else stringResource(R.string.card_video_of, used / 60, lim / 60)
    }
    if (games == null && video == null) return
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        games?.let { Text(it, fontSize = 13.sp, fontWeight = FontWeight.Medium, color = Tok.Muted, style = NumStyle) }
        video?.let { Text(it, fontSize = 13.sp, fontWeight = FontWeight.Medium, color = Tok.Muted, style = NumStyle) }
    }
}

/** Кнопка сетки карточки: 48 dp (касание), скругление 10 dp, подпись для TalkBack — с именем ребёнка. */
@Composable
private fun Chip(
    label: String,
    desc: String,
    fg: Color,
    bg: Color,
    line: Color,
    enabled: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    fontSize: TextUnit = 15.sp,
    coin: Boolean = false,
) {
    Surface(
        onClick = onClick,
        enabled = enabled,
        shape = Tok.ChipShape,
        color = bg,
        contentColor = fg,
        border = BorderStroke(1.dp, line),
        modifier = modifier
            .heightIn(min = 48.dp)
            .alpha(if (enabled) 1f else 0.45f)
            .semantics {
                contentDescription = desc
                role = Role.Button
            },
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.Center,
            modifier = Modifier.padding(PaddingValues(horizontal = 4.dp, vertical = 6.dp)),
        ) {
            if (coin) {
                CoinIcon(16.dp)
                Spacer(Modifier.width(4.dp))
            }
            Text(
                label,
                fontSize = fontSize,
                fontWeight = FontWeight.SemiBold,
                color = fg,
                textAlign = TextAlign.Center,
                style = NumStyle,
                maxLines = 2,
                lineHeight = fontSize * 1.1f,
                modifier = Modifier.clearAndSetSemantics { },
            )
        }
    }
}

/** «1 программа ждёт решения» на жёлтой плашке (FR-47) — открывает «Ждут решения». */
@Composable
private fun PendingLine(text: String, onClick: () -> Unit) {
    val shape = RoundedCornerShape(12.dp)
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .clip(shape)
            .background(Tok.PendingBg)
            .border(1.dp, Tok.PendingLine, shape)
            .clickable(role = Role.Button, onClick = onClick)
            .padding(horizontal = 12.dp),
    ) {
        Icon(AppIcons.Grid, contentDescription = null, tint = Tok.PendingIcon, modifier = Modifier.size(22.dp))
        Text(text, fontSize = 15.sp, fontWeight = FontWeight.Bold, color = Tok.PendingText, modifier = Modifier.weight(1f))
        Icon(AppIcons.Chevron, contentDescription = null, tint = Tok.PendingIcon, modifier = Modifier.size(18.dp))
    }
}

/**
 * Компактная карточка остальных детей: аватар, имя, остаток 19 sp жирным цветом тона (крупный текст
 * WCAG, Palette.isLarge), полоса 8 dp, присутствие. Нажатие разворачивает карточку.
 */
@Composable
fun ChildCompact(child: Proto.Child, index: Int, report: Proto.Report, onExpand: () -> Unit) {
    val tone = Remain.tone(child.leftSec)
    val remainText = if (child.leftSec < 0) stringResource(R.string.child_unlimited) else Remain.hm(Remain.remainMinutes(child.leftSec))
    val leftDesc = if (child.leftSec < 0) {
        stringResource(R.string.child_unlimited)
    } else {
        stringResource(R.string.card_left_short, durationText(Remain.remainMinutes(child.leftSec) * 60))
    }
    val desc = stringResource(R.string.card_open_desc, child.name, leftDesc)
    val pres = presence(child, report.ts)
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        modifier = Modifier
            .fillMaxWidth()
            .clip(Tok.CardShape)
            .background(Tok.Surface)
            .border(1.dp, Tok.Line, Tok.CardShape)
            .clickable(role = Role.Button, onClick = onExpand)
            .clearAndSetSemantics {
                contentDescription = "$desc. $pres"
                role = Role.Button
            }
            .padding(horizontal = 16.dp, vertical = 14.dp),
    ) {
        Avatar(child.name, index)
        Column(verticalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    child.name,
                    fontSize = 16.sp,
                    fontWeight = FontWeight.Bold,
                    color = Tok.Ink,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                Text(
                    remainText,
                    fontSize = 19.sp,
                    fontWeight = FontWeight.Bold,
                    color = Tok.remainText(tone, large = true),
                    style = NumStyle,
                )
            }
            RemainBar(remainFraction(child), Tok.remainBar(tone), 8.dp)
            MutedText(pres)
        }
        Icon(AppIcons.Chevron, contentDescription = null, tint = Tok.Muted, modifier = Modifier.size(18.dp))
    }
}
