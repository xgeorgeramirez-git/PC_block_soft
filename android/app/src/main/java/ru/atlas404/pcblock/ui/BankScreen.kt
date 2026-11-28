package ru.atlas404.pcblock.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.error
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.BankRules
import ru.atlas404.pcblock.proto.Format

/**
 * «Копилка Миши» (FR-44, D-113, макет phones_more.py): баланс, поле «Сколько минут положить» (1…600 за раз,
 * предела баланса нет), «Положить 30 минут»; «Обнулить копилку» с подтверждением. Компьютер не на связи —
 * кнопки недоступны и рядом «Компьютер не на связи» (команды не копятся).
 */
@Composable
fun BankScreen(vm: AppViewModel, childId: Long) {
    BackHandler { vm.closeSub() }
    val child = vm.report?.children?.firstOrNull { it.id == childId }
    val name = Format.genitive(child?.name.orEmpty())
    var text by rememberSaveable { mutableStateOf("30") }
    var tried by rememberSaveable { mutableStateOf(false) }
    var askClear by rememberSaveable { mutableStateOf(false) }
    val online = vm.pcOnline
    val locked = vm.isBusy
    val busy = vm.busyChild == childId

    Column(Modifier.fillMaxSize()) {
        SubTopBar(stringResource(R.string.bank_title, name), onBack = { vm.closeSub() })
        val balance = child?.bankMin
        if (child == null || balance == null) {
            // Ребёнка нет в отчёте или компьютер старше 0.8.4 (копилки на телефоне нет).
            Text(
                stringResource(if (child == null) R.string.err_no_child else R.string.update_pc),
                fontSize = 16.sp,
                color = Tok.Ink,
                modifier = Modifier.padding(16.dp),
            )
            return@Column
        }
        Column(
            verticalArrangement = Arrangement.spacedBy(14.dp),
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 16.dp),
        ) {
            val problem = BankRules.checkBankMinutes(text)
            val showErr = problem != null && (tried || text.isNotEmpty())
            val errText = problem?.let { stringResource(it.res()) }.orEmpty()
            val fieldLabel = stringResource(R.string.bank_field)
            val putLabel = BankRules.putLabel(
                text,
                Format.Plural(
                    stringResource(R.string.bank_put_one),
                    stringResource(R.string.bank_put_few),
                    stringResource(R.string.bank_put_many),
                ),
                stringResource(R.string.bank_put),
            )
            fun put() {
                tried = true
                if (problem == null) vm.bankAdd(childId, text)
            }

            AppCard {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    CoinIcon(44.dp)
                    Column {
                        MutedText(stringResource(R.string.bank_now), size = 14)
                        Text(
                            stringResource(R.string.bank_min, balance),
                            fontSize = 32.sp,
                            fontWeight = FontWeight.ExtraBold,
                            color = Tok.Ink,
                            style = NumStyle,
                        )
                    }
                }
                Text(fieldLabel, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = Tok.Ink)
                OutlinedTextField(
                    value = text,
                    onValueChange = { v -> text = v.filter { it in '0'..'9' }.take(3) },
                    singleLine = true,
                    isError = showErr,
                    textStyle = NumStyle.copy(fontSize = 22.sp, fontWeight = FontWeight.Bold, color = Tok.Ink),
                    shape = Tok.ButtonShape,
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedBorderColor = Tok.Accent,
                        unfocusedBorderColor = Tok.Accent,
                        errorBorderColor = Tok.RemainRed,
                    ),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { if (online && !locked) put() }),
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = 52.dp)
                        .semantics {
                            // Подпись поля и ошибка зачитываются вместе (ТЗ 8.0 п. 7).
                            contentDescription = if (showErr) "$fieldLabel. $errText" else fieldLabel
                            if (showErr) error(errText)
                        },
                )
                if (showErr) {
                    Text(errText, fontSize = 13.sp, fontWeight = FontWeight.Medium, color = Tok.RemainRed)
                } else {
                    MutedText(stringResource(R.string.bank_hint))
                }
                MainButton(putLabel, onClick = { put() }, enabled = online && !locked)
                if (!online) OfflineHint(vm.lastSeenMs)
                val note = vm.note?.takeIf { it.childId == childId }
                StatusLine(
                    busyText = if (busy) stringResource(R.string.cmd_sending) else null,
                    okText = note?.takeIf { it.ok }?.let { it.message.ifEmpty { stringResource(R.string.cmd_done) } },
                    problem = note?.problem,
                    lastSeenMs = vm.lastSeenMs,
                )
            }

            AppCard {
                Text(stringResource(R.string.bank_clear), fontSize = 16.sp, fontWeight = FontWeight.SemiBold, color = Tok.Ink)
                MutedText(
                    Format.plural(
                        balance,
                        Format.Plural(
                            stringResource(R.string.bank_clear_text_one),
                            stringResource(R.string.bank_clear_text_few),
                            stringResource(R.string.bank_clear_text_many),
                        ),
                    ),
                    size = 14,
                )
                PlainButton(stringResource(R.string.bank_clear), onClick = { askClear = true }, enabled = online && !locked, danger = true)
                if (!online) OfflineHint(vm.lastSeenMs)
            }
        }
        if (askClear) {
            AlertDialog(
                onDismissRequest = { askClear = false },
                text = { Text(stringResource(R.string.bank_clear_confirm, name, balance), fontSize = 16.sp, color = Tok.Ink) },
                confirmButton = {
                    TextButton(
                        onClick = {
                            askClear = false
                            vm.bankClear(childId)
                        },
                        modifier = Modifier.heightIn(min = 48.dp),
                    ) { Text(stringResource(R.string.bank_clear_yes), color = Tok.RemainRed) }
                },
                dismissButton = {
                    TextButton(onClick = { askClear = false }, modifier = Modifier.heightIn(min = 48.dp)) {
                        Text(stringResource(R.string.cancel))
                    }
                },
            )
        }
    }
}
