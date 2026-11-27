package ru.atlas404.pcblock.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.error
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.LifecycleResumeEffect
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Problem
import ru.atlas404.pcblock.proto.PromoRules
import ru.atlas404.pcblock.proto.Proto

/**
 * «Промокоды» (FR-45, 8.3.10, макет phones_more.py): вкладки «Действующие» и «Использованные», корзина
 * у кода с подтверждением, «Создать код» и «Придумать коды» (нижние листы, те же поля и тексты, что окно),
 * подсказка, «Данные на HH:MM». Список запрашивается при открытии вкладки и при возврате в приложение;
 * компьютер не на связи — последний список и «Компьютер не на связи с HH:MM», кнопки недоступны.
 */
@Composable
fun PromosScreen(vm: AppViewModel) {
    LifecycleResumeEffect(Unit) {
        vm.loadPromos()
        onPauseOrDispose { }
    }
    val online = vm.pcOnline
    // Связь появилась, пока вкладка открыта (в том числе первая проверка после запуска) — запросить список.
    LaunchedEffect(online) { if (online) vm.loadPromos() }

    var used by rememberSaveable { mutableStateOf(false) }
    var sheet by rememberSaveable { mutableStateOf(0) } // 0 — нет, 1 — «Создать код», 2 — «Придумать коды»
    var delCode by rememberSaveable { mutableStateOf<String?>(null) }
    val list = vm.promos
    val canAct = online && !vm.isBusy

    Column(
        verticalArrangement = Arrangement.spacedBy(14.dp),
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(start = 16.dp, end = 16.dp, top = 18.dp, bottom = 16.dp),
    ) {
        ScreenTitle(stringResource(R.string.promo_title))
        Segments(used, onPick = { used = it })
        if (!online) OfflineHint(vm.lastSeenMs)
        when {
            list == null -> MutedText(
                stringResource(if (vm.promoBusy == PromoAct.LIST) R.string.promo_loading else R.string.promo_none_yet),
                size = 15,
            )
            !used -> ActiveList(list, canAct, onDelete = { delCode = it })
            else -> UsedList(list)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.fillMaxWidth()) {
            MainButton(
                stringResource(R.string.promo_create),
                onClick = {
                    vm.clearPromoNote()
                    sheet = 1
                },
                enabled = canAct,
                modifier = Modifier.weight(1f),
            )
            PlainButton(
                stringResource(R.string.promo_gen),
                onClick = {
                    vm.clearPromoNote()
                    sheet = 2
                },
                enabled = canAct,
                modifier = Modifier.weight(1f),
            )
        }
        MutedText(stringResource(R.string.promo_hint))
        // Итог удаления, ошибки запроса списка и «Промокод … готов» (лист «Создать код» уже закрыт).
        val note = vm.promoNote?.takeIf { it.act == PromoAct.DELETE || it.act == PromoAct.LIST || (it.act == PromoAct.CREATE && it.ok) }
        StatusLine(
            busyText = if (vm.promoBusy == PromoAct.DELETE) stringResource(R.string.cmd_sending) else null,
            okText = note?.takeIf { it.ok && it.message.isNotEmpty() }?.message,
            problem = note?.problem,
            lastSeenMs = vm.lastSeenMs,
        )
        if (vm.promosTs > 0) MutedText(stringResource(R.string.data_at, clockText(vm.promosTs)))
    }

    when (sheet) {
        1 -> CreateSheet(vm, onClose = { sheet = 0 })
        2 -> GenSheet(vm, onClose = { sheet = 0 })
    }
    delCode?.let { code ->
        AlertDialog(
            onDismissRequest = { delCode = null },
            text = { Text(stringResource(R.string.promo_delete_confirm, code), fontSize = 16.sp, color = Tok.Ink) },
            confirmButton = {
                TextButton(
                    onClick = {
                        delCode = null
                        vm.deletePromo(code)
                    },
                    modifier = Modifier.heightIn(min = 48.dp),
                ) { Text(stringResource(R.string.promo_delete_yes), color = Tok.RemainRed) }
            },
            dismissButton = {
                TextButton(onClick = { delCode = null }, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.cancel))
                }
            },
        )
    }
}

/** Переключатель «Действующие» / «Использованные»: фон #E3E8F2, выбранный — белый. */
@Composable
private fun Segments(used: Boolean, onPick: (Boolean) -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(Tok.Track)
            .padding(4.dp)
            .selectableGroup(),
    ) {
        for ((value, res) in listOf(false to R.string.promo_tab_active, true to R.string.promo_tab_used)) {
            val on = used == value
            Box(
                contentAlignment = Alignment.Center,
                modifier = Modifier
                    .weight(1f)
                    .heightIn(min = 48.dp)
                    .clip(RoundedCornerShape(9.dp))
                    .background(if (on) Tok.Surface else Color.Transparent)
                    .selectable(selected = on, role = Role.Tab, onClick = { onPick(value) }),
            ) {
                Text(
                    stringResource(res),
                    fontSize = 15.sp,
                    fontWeight = if (on) FontWeight.SemiBold else FontWeight.Medium,
                    color = if (on) Tok.Ink else Tok.Muted,
                )
            }
        }
    }
}

/** Действующие: код 18 sp с разрядкой, «создан 25 ноября», минуты, корзина 48 dp. */
@Composable
private fun ActiveList(list: Proto.PromoList, canAct: Boolean, onDelete: (String) -> Unit) {
    AppCard(gap = 0.dp) {
        if (list.active.isEmpty()) MutedText(stringResource(R.string.promo_empty_active), size = 15)
        list.active.forEachIndexed { i, p ->
            if (i > 0) HorizontalDivider(thickness = 1.dp, color = Tok.Line)
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(vertical = 10.dp),
            ) {
                Column(Modifier.weight(1f)) {
                    CodeText(p.code)
                    if (p.createdMs > 0) MutedText(stringResource(R.string.promo_created, dayText(p.createdMs)))
                }
                Text(stringResource(R.string.promo_min, p.min), fontSize = 16.sp, fontWeight = FontWeight.Bold, color = Tok.Ink, style = NumStyle)
                IconButton(
                    onClick = { onDelete(p.code) },
                    enabled = canAct,
                    modifier = Modifier
                        .size(48.dp)
                        .border(1.dp, Tok.Line, Tok.ChipShape),
                ) {
                    Icon(
                        AppIcons.Trash,
                        contentDescription = stringResource(R.string.promo_delete_desc, p.code),
                        tint = if (canAct) Tok.Muted else Tok.Line,
                        modifier = Modifier.size(20.dp),
                    )
                }
            }
        }
    }
    if (list.moreActive > 0) MutedText(stringResource(R.string.promo_more, list.moreActive))
}

/** Использованные: код, «использован 26 ноября в 18:40, Миша», минуты. */
@Composable
private fun UsedList(list: Proto.PromoList) {
    AppCard(gap = 0.dp) {
        if (list.used.isEmpty()) MutedText(stringResource(R.string.promo_empty_used), size = 15)
        list.used.forEachIndexed { i, p ->
            if (i > 0) HorizontalDivider(thickness = 1.dp, color = Tok.Line)
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(vertical = 10.dp),
            ) {
                Column(Modifier.weight(1f)) {
                    CodeText(p.code)
                    if (p.usedMs > 0) {
                        val who = p.childName.ifEmpty { stringResource(R.string.promo_child_gone) }
                        MutedText(stringResource(R.string.promo_used, dayText(p.usedMs), hmText(p.usedMs), who))
                    }
                }
                Text(stringResource(R.string.promo_min, p.min), fontSize = 16.sp, fontWeight = FontWeight.Bold, color = Tok.Ink, style = NumStyle)
            }
        }
    }
    if (list.moreUsed > 0) MutedText(stringResource(R.string.promo_more, list.moreUsed))
}

@Composable
private fun CodeText(code: String) {
    Text(code, fontSize = 18.sp, fontWeight = FontWeight.Bold, letterSpacing = 1.sp, color = Tok.Ink, style = NumStyle)
}

/** Поле листа: подпись сверху, ошибка под полем и в семантике (ТЗ 8.0 п. 7). */
@Composable
private fun FormField(label: String, value: String, onChange: (String) -> Unit, problem: Problem?, keyboard: KeyboardOptions) {
    val errText = problem?.let { stringResource(it.res()) }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(label, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = Tok.Ink)
        OutlinedTextField(
            value = value,
            onValueChange = onChange,
            singleLine = true,
            isError = errText != null,
            textStyle = NumStyle.copy(fontSize = 18.sp, fontWeight = FontWeight.SemiBold, color = Tok.Ink),
            shape = Tok.ButtonShape,
            keyboardOptions = keyboard,
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = 52.dp)
                .semantics {
                    contentDescription = if (errText != null) "$label. $errText" else label
                    if (errText != null) error(errText)
                },
        )
        if (errText != null) Text(errText, fontSize = 13.sp, fontWeight = FontWeight.Medium, color = Tok.RemainRed)
    }
}

private val DIGITS = KeyboardOptions(keyboardType = KeyboardType.Number)

private fun digits(v: String, max: Int): String = v.filter { it in '0'..'9' }.take(max)

/** «Создать код»: «Код», «Минуты» → «Создать»; «Такой код уже есть…» — у поля кода. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun CreateSheet(vm: AppViewModel, onClose: () -> Unit) {
    val state = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var code by rememberSaveable { mutableStateOf("") }
    var min by rememberSaveable { mutableStateOf("30") }
    var tried by rememberSaveable { mutableStateOf(false) }
    val note = vm.promoNote?.takeIf { it.act == PromoAct.CREATE }
    // Код создан — лист закрывается, «Промокод … готов» видно на экране под кнопками.
    LaunchedEffect(note) { if (note?.ok == true) onClose() }
    val codeProblem = PromoRules.checkCode(code).second
    val minProblem = PromoRules.checkMinutes(min)
    val serverCode = note?.problem?.takeIf { it == Problem.PROMO_EXISTS }
    ModalBottomSheet(onDismissRequest = onClose, sheetState = state, containerColor = Tok.Surface) {
        Column(
            verticalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .imePadding()
                .padding(start = 16.dp, end = 16.dp, bottom = 24.dp),
        ) {
            ScreenTitle(stringResource(R.string.promo_create), size = 20)
            FormField(
                label = stringResource(R.string.promo_field_code),
                value = code,
                onChange = {
                    code = it.take(PromoRules.MAX_CODE + 4)
                    vm.clearPromoNote()
                },
                problem = (if (tried) codeProblem else null) ?: serverCode,
                keyboard = KeyboardOptions(keyboardType = KeyboardType.Ascii, capitalization = KeyboardCapitalization.Characters),
            )
            FormField(
                label = stringResource(R.string.promo_field_min),
                value = min,
                onChange = { min = digits(it, 3) },
                problem = if (tried) minProblem else null,
                keyboard = DIGITS,
            )
            MainButton(
                stringResource(R.string.promo_create_btn),
                onClick = {
                    tried = true
                    if (codeProblem == null && minProblem == null) vm.createPromo(code, min)
                },
                enabled = vm.pcOnline && !vm.isBusy,
            )
            if (!vm.pcOnline) OfflineHint(vm.lastSeenMs)
            StatusLine(
                busyText = if (vm.promoBusy == PromoAct.CREATE) stringResource(R.string.cmd_sending) else null,
                okText = null,
                problem = note?.problem?.takeIf { it != Problem.PROMO_EXISTS },
                lastSeenMs = vm.lastSeenMs,
            )
        }
    }
}

/** «Придумать коды»: «Сколько», «Длина», «Из чего», «Минуты» → «Придумать» → итог, коды, «Скопировать все». */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun GenSheet(vm: AppViewModel, onClose: () -> Unit) {
    val state = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var count by rememberSaveable { mutableStateOf("10") }
    var len by rememberSaveable { mutableStateOf("6") }
    var charset by rememberSaveable { mutableStateOf(Proto.CHARSET_MIXED) }
    var min by rememberSaveable { mutableStateOf("30") }
    var tried by rememberSaveable { mutableStateOf(false) }
    var copied by remember { mutableStateOf(false) }
    val clipboard = LocalClipboardManager.current
    val note = vm.promoNote?.takeIf { it.act == PromoAct.GEN }
    val countProblem = PromoRules.checkCount(count)
    val lenProblem = PromoRules.checkLen(len)
    val minProblem = PromoRules.checkMinutes(min)
    val codes = vm.genCodes
    ModalBottomSheet(onDismissRequest = onClose, sheetState = state, containerColor = Tok.Surface) {
        Column(
            verticalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .imePadding()
                .padding(start = 16.dp, end = 16.dp, bottom = 24.dp),
        ) {
            ScreenTitle(stringResource(R.string.promo_gen), size = 20)
            FormField(stringResource(R.string.promo_field_count), count, { count = digits(it, 2) }, if (tried) countProblem else null, DIGITS)
            FormField(stringResource(R.string.promo_field_len), len, { len = digits(it, 2) }, if (tried) lenProblem else null, DIGITS)
            Text(
                stringResource(R.string.promo_field_charset),
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                color = Tok.Ink,
                modifier = Modifier.semantics { heading() },
            )
            Column(Modifier.selectableGroup()) {
                for ((id, res) in listOf(
                    Proto.CHARSET_DIGITS to R.string.promo_charset_digits,
                    Proto.CHARSET_LETTERS to R.string.promo_charset_letters,
                    Proto.CHARSET_MIXED to R.string.promo_charset_mixed,
                )) {
                    val on = charset == id
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(min = 48.dp)
                            .selectable(selected = on, role = Role.RadioButton, onClick = { charset = id }),
                    ) {
                        RadioButton(selected = on, onClick = null)
                        Spacer(Modifier.width(12.dp))
                        Text(stringResource(res), fontSize = 16.sp, color = Tok.Ink)
                    }
                }
            }
            FormField(stringResource(R.string.promo_field_min), min, { min = digits(it, 3) }, if (tried) minProblem else null, DIGITS)
            MainButton(
                stringResource(R.string.promo_gen_btn),
                onClick = {
                    tried = true
                    copied = false
                    if (countProblem == null && lenProblem == null && minProblem == null) vm.genPromos(count, len, charset, min)
                },
                enabled = vm.pcOnline && !vm.isBusy,
            )
            if (!vm.pcOnline) OfflineHint(vm.lastSeenMs)
            StatusLine(
                busyText = if (vm.promoBusy == PromoAct.GEN) stringResource(R.string.cmd_sending) else null,
                okText = note?.takeIf { it.ok && it.message.isNotEmpty() }?.message,
                problem = note?.problem,
                lastSeenMs = vm.lastSeenMs,
            )
            if (note?.ok == true && codes.isNotEmpty()) {
                val label = stringResource(R.string.promo_codes_label)
                Column(
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(Tok.ButtonShape)
                        .background(Tok.Ground)
                        .padding(12.dp)
                        .semantics(mergeDescendants = true) { contentDescription = label },
                ) {
                    for (c in codes) CodeText(c)
                }
                PlainButton(
                    stringResource(R.string.promo_copy_all),
                    onClick = {
                        clipboard.setText(AnnotatedString(codes.joinToString("\n")))
                        copied = true
                    },
                )
                // «Коды скопированы» — сразу под кнопкой (8.3.10 п. 3), строка постоянная для TalkBack.
                Text(
                    if (copied) stringResource(R.string.promo_copied) else "",
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Medium,
                    color = Tok.PlusText,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = 20.dp)
                        .semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
        }
    }
}
