package ru.atlas404.pcblock.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Format
import ru.atlas404.pcblock.proto.Proto

/** Шаг 6: на связи ли компьютер, карточки детей, «+15», «−15», «Своё число»; меню «Отключить телефон». */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MainScreen(vm: AppViewModel) {
    // Обновление раз в 15 с, пока экран виден; при уходе в фон опрос останавливается.
    LifecycleResumeEffect(Unit) {
        vm.startPolling()
        onPauseOrDispose { vm.stopPolling() }
    }
    var menu by remember { mutableStateOf(false) }
    var askUnpair by rememberSaveable { mutableStateOf(false) }
    var customFor by remember { mutableStateOf<Proto.Child?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.main_title)) },
                actions = {
                    Box {
                        IconButton(onClick = { menu = true }) {
                            Icon(Icons.Filled.MoreVert, contentDescription = stringResource(R.string.menu_more))
                        }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.menu_refresh)) },
                                onClick = {
                                    menu = false
                                    vm.refreshNow()
                                },
                            )
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.menu_unpair)) },
                                onClick = {
                                    menu = false
                                    askUnpair = true
                                },
                            )
                        }
                    }
                },
            )
        },
    ) { pad ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(pad),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item(key = "status") { StatusBlock(vm, onRepair = { askUnpair = true }) }
            val rep = vm.report
            if (rep == null) {
                item(key = "nodata") {
                    Text(
                        stringResource(R.string.no_data),
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(vertical = 8.dp),
                    )
                }
            } else if (rep.children.isEmpty()) {
                item(key = "nochildren") {
                    Text(stringResource(R.string.no_children), style = MaterialTheme.typography.bodyLarge)
                }
            } else {
                items(rep.children, key = { it.id }) { c ->
                    ChildCard(
                        child = c,
                        busy = vm.busyChild == c.id,
                        locked = vm.busyChild != null,
                        note = vm.note?.takeIf { it.childId == c.id },
                        lastSeenMs = vm.lastSeenMs,
                        onDelta = { d -> vm.command(c.id, d) },
                        onCustom = { customFor = c },
                    )
                }
            }
        }
    }

    if (askUnpair) {
        AlertDialog(
            onDismissRequest = { askUnpair = false },
            title = { Text(stringResource(R.string.unpair_title)) },
            text = { Text(stringResource(R.string.unpair_text)) },
            confirmButton = {
                TextButton(onClick = {
                    askUnpair = false
                    vm.unpair()
                }) { Text(stringResource(R.string.unpair_confirm)) }
            },
            dismissButton = {
                TextButton(onClick = { askUnpair = false }) { Text(stringResource(R.string.cancel)) }
            },
        )
    }
    customFor?.let { c ->
        CustomDialog(
            child = c,
            onDismiss = { customFor = null },
            onSend = { d ->
                customFor = null
                vm.command(c.id, d)
            },
        )
    }
}

@Composable
private fun StatusBlock(vm: AppViewModel, onRepair: () -> Unit) {
    val link = vm.link
    val err = vm.mainError
    val online = link is Format.Link.Online
    val text = when {
        err != null -> problemText(err, vm.lastSeenMs)
        link == null -> stringResource(R.string.pc_checking)
        link is Format.Link.Offline && link.sinceMs > 0 -> stringResource(R.string.pc_offline_since, clockText(link.sinceMs))
        link is Format.Link.Offline -> stringResource(R.string.pc_offline)
        else -> stringResource(R.string.pc_online)
    }
    Card(
        colors = CardDefaults.cardColors(
            containerColor = if (err != null) MaterialTheme.colorScheme.errorContainer else MaterialTheme.colorScheme.surfaceContainer,
        ),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                // Точка дублирует текст, а не заменяет его: цвет один не несёт смысла.
                Box(
                    Modifier
                        .size(12.dp)
                        .background(
                            if (online) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline,
                            CircleShape,
                        ),
                )
                Spacer(Modifier.width(10.dp))
                Text(
                    text,
                    style = MaterialTheme.typography.titleMedium,
                    color = if (err != null) MaterialTheme.colorScheme.onErrorContainer else MaterialTheme.colorScheme.onSurface,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
            vm.report?.let { rep ->
                Text(
                    stringResource(R.string.data_at, clockText(rep.ts)),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (vm.needRepair) {
                Button(onClick = onRepair, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.reconnect))
                }
            }
        }
    }
}

@Composable
private fun ChildCard(
    child: Proto.Child,
    busy: Boolean,
    locked: Boolean,
    note: CmdNote?,
    lastSeenMs: Long,
    onDelta: (Int) -> Unit,
    onCustom: () -> Unit,
) {
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                child.name,
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.semantics { heading() },
            )
            Text(
                stringResource(R.string.child_used, durationText(child.usedSec)),
                style = MaterialTheme.typography.bodyLarge,
            )
            Text(
                if (child.leftSec < 0) stringResource(R.string.child_unlimited) else stringResource(R.string.child_left, durationText(child.leftSec)),
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.primary,
            )
            if (child.blocked) {
                Text(
                    stringResource(R.string.child_blocked),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (child.extraMin != 0) {
                Text(
                    stringResource(R.string.child_extra, durationText(child.extraMin * 60L)),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.size(4.dp))
            val add = stringResource(R.string.btn_add15_desc, child.name)
            val sub = stringResource(R.string.btn_sub15_desc, child.name)
            val own = stringResource(R.string.btn_custom_desc, child.name)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                FilledTonalButton(
                    onClick = { onDelta(15) },
                    enabled = !locked,
                    contentPadding = PaddingValues(horizontal = 8.dp),
                    modifier = Modifier
                        .weight(1f)
                        .heightIn(min = 52.dp)
                        .semantics { contentDescription = add },
                ) { Text(stringResource(R.string.btn_add15), textAlign = androidx.compose.ui.text.style.TextAlign.Center) }
                OutlinedButton(
                    onClick = { onDelta(-15) },
                    enabled = !locked,
                    contentPadding = PaddingValues(horizontal = 8.dp),
                    modifier = Modifier
                        .weight(1f)
                        .heightIn(min = 52.dp)
                        .semantics { contentDescription = sub },
                ) { Text(stringResource(R.string.btn_sub15), textAlign = androidx.compose.ui.text.style.TextAlign.Center) }
                OutlinedButton(
                    onClick = onCustom,
                    enabled = !locked,
                    contentPadding = PaddingValues(horizontal = 8.dp),
                    modifier = Modifier
                        .weight(1.2f)
                        .heightIn(min = 52.dp)
                        .semantics { contentDescription = own },
                ) { Text(stringResource(R.string.btn_custom), textAlign = androidx.compose.ui.text.style.TextAlign.Center) }
            }
            // Постоянная область итога команды: TalkBack зачитывает её сам.
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 24.dp)
                    .semantics { liveRegion = LiveRegionMode.Polite },
            ) {
                val problem = note?.problem
                when {
                    busy -> {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(8.dp))
                        Text(stringResource(R.string.cmd_sending), style = MaterialTheme.typography.bodyLarge)
                    }
                    note != null && note.ok -> Text(
                        note.message.ifEmpty { stringResource(R.string.cmd_done) },
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.primary,
                    )
                    problem != null -> Text(
                        problemText(problem, lastSeenMs),
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        }
    }
}

@Composable
private fun CustomDialog(child: Proto.Child, onDismiss: () -> Unit, onSend: (Int) -> Unit) {
    var text by rememberSaveable { mutableStateOf("") }
    var bad by remember { mutableStateOf(false) }
    // Поле получает фокус при открытии: родитель сразу набирает число (ревью M7, R12).
    val field = remember { FocusRequester() }
    LaunchedEffect(Unit) {
        withFrameNanos { } // окно диалога присоединено, поле уже в дереве фокуса
        field.requestFocus()
    }
    fun send(sign: Int) {
        val n = Format.minutes(text)
        if (n == null) bad = true else onSend(sign * n)
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.custom_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(child.name, style = MaterialTheme.typography.titleMedium)
                OutlinedTextField(
                    value = text,
                    onValueChange = { v ->
                        text = v.filter { it.isDigit() }.take(3)
                        bad = false
                    },
                    label = { Text(stringResource(R.string.custom_label)) },
                    singleLine = true,
                    isError = bad,
                    supportingText = { if (bad) Text(stringResource(R.string.err_range)) },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.focusRequester(field),
                )
            }
        },
        confirmButton = {
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                TextButton(onClick = { send(-1) }, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.custom_sub))
                }
                TextButton(onClick = { send(1) }, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.custom_add))
                }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, modifier = Modifier.heightIn(min = 48.dp)) {
                Text(stringResource(R.string.cancel))
            }
        },
    )
}
