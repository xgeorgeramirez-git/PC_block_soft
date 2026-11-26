package ru.atlas404.pcblock.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items as gridItems
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.RadioButton
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextOverflow
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
import ru.atlas404.pcblock.proto.NoteMark
import ru.atlas404.pcblock.proto.Proto
import ru.atlas404.pcblock.store.SavedNote

/**
 * Шаг 6: на связи ли компьютер, карточки детей: ±15/±30/±60, «Своё число», «Снять запрет на сегодня»,
 * «Сообщение», «Статистика за сегодня»; меню «Отключить телефон».
 */
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
    var noteFor by rememberSaveable { mutableStateOf<Long?>(null) }
    var noteName by rememberSaveable { mutableStateOf("") }
    var statsFor by rememberSaveable { mutableStateOf<Long?>(null) }

    // «Статистика за сегодня» поверх главного экрана: опрос (выше) продолжается, данные обновляются.
    val statsId = statsFor
    if (statsId != null) {
        StatsScreen(child = vm.report?.children?.firstOrNull { it.id == statsId }, onBack = { statsFor = null })
        return
    }
    // «Сообщение для <имя>» (FR-41) — отдельный экран; опрос идёт, отметки обновляются.
    val noteId = noteFor
    if (noteId != null) {
        NoteScreen(
            childId = noteId,
            name = vm.report?.children?.firstOrNull { it.id == noteId }?.name ?: noteName,
            vm = vm,
            onBack = { if (!vm.noteBusy) noteFor = null },
            onSent = { noteFor = null },
        )
        return
    }

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
                        locked = vm.busyChild != null || vm.noteBusy,
                        note = vm.note?.takeIf { it.childId == c.id },
                        lastSeenMs = vm.lastSeenMs,
                        onDelta = { d -> vm.command(c.id, d) },
                        onCustom = { customFor = c },
                        onClearForbidden = { vm.clearForbidden(c.id) },
                        onStats = { statsFor = c.id },
                        notes = vm.notes.filter { it.child == c.id }.sortedByDescending { it.sentAtMs }.take(3),
                        onNote = {
                            vm.startNote()
                            noteName = c.name
                            noteFor = c.id
                        },
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
    onClearForbidden: () -> Unit,
    onStats: () -> Unit,
    notes: List<SavedNote>,
    onNote: () -> Unit,
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
            child.activeForbidUntilMs?.let { until ->
                Text(
                    stringResource(R.string.forbid_until, clockText(until)),
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            if (child.forbidView == Proto.ForbidView.CLEARED) {
                Text(
                    stringResource(R.string.forbid_cleared),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.size(4.dp))
            // Две строки по три: «+15/+30/+60» и «−15/−30/−60» (FR-42), ширина делится поровну.
            for (sign in listOf(1, -1)) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                    for (m in Proto.QUICK_ADD) {
                        val label = stringResource(if (sign > 0) R.string.btn_add else R.string.btn_sub, m)
                        val desc = stringResource(if (sign > 0) R.string.btn_add_desc else R.string.btn_sub_desc, m, child.name)
                        val mod = Modifier
                            .weight(1f)
                            .heightIn(min = 52.dp)
                            .semantics { contentDescription = desc }
                        val content: @Composable androidx.compose.foundation.layout.RowScope.() -> Unit = {
                            Text(label, maxLines = 1, textAlign = androidx.compose.ui.text.style.TextAlign.Center)
                        }
                        if (sign > 0) {
                            FilledTonalButton(
                                onClick = { onDelta(m) },
                                enabled = !locked,
                                contentPadding = PaddingValues(horizontal = 4.dp),
                                modifier = mod,
                                content = content,
                            )
                        } else {
                            OutlinedButton(
                                onClick = { onDelta(-m) },
                                enabled = !locked,
                                contentPadding = PaddingValues(horizontal = 4.dp),
                                modifier = mod,
                                content = content,
                            )
                        }
                    }
                }
            }
            val own = stringResource(R.string.btn_custom_desc, child.name)
            OutlinedButton(
                onClick = onCustom,
                enabled = !locked,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 52.dp)
                    .semantics { contentDescription = own },
            ) { Text(stringResource(R.string.btn_custom)) }
            if (child.forbidView == Proto.ForbidView.BUTTON) {
                val nfDesc = stringResource(R.string.btn_no_forbidden_desc, child.name)
                FilledTonalButton(
                    onClick = onClearForbidden,
                    enabled = !locked,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = 52.dp)
                        .semantics { contentDescription = nfDesc },
                ) { Text(stringResource(R.string.btn_no_forbidden), textAlign = androidx.compose.ui.text.style.TextAlign.Center) }
            }
            val noteDesc = stringResource(R.string.btn_note_desc, child.name)
            OutlinedButton(
                onClick = onNote,
                enabled = !locked,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 52.dp)
                    .semantics { contentDescription = noteDesc },
            ) { Text(stringResource(R.string.btn_note)) }
            if (child.today != null) {
                // Статистика только читается: доступна и пока идёт команда.
                val statsDesc = stringResource(R.string.btn_stats_desc, child.name)
                OutlinedButton(
                    onClick = onStats,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = 52.dp)
                        .semantics { contentDescription = statsDesc },
                ) { Text(stringResource(R.string.btn_stats)) }
            }
            if (notes.isNotEmpty()) NoteList(notes)
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

/** Последние сообщения ребёнка (до 3): текст, время отправки и отметка из отчёта компьютера. */
@Composable
private fun NoteList(notes: List<SavedNote>) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.fillMaxWidth()) {
        Text(
            stringResource(R.string.note_recent),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        for (n in notes) {
            val mark = NoteMark.of(n.state, n.showSec)
            val markText = stringResource(mark.res())
            val time = clockText(n.sentAtMs)
            val oneLine = n.text.replace('\n', ' ')
            val desc = stringResource(R.string.note_item_desc, oneLine, time, markText)
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .clearAndSetSemantics {
                        contentDescription = desc
                        liveRegion = LiveRegionMode.Polite // смена отметки зачитывается сама
                    },
            ) {
                Text(
                    oneLine,
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(
                    stringResource(R.string.note_item_meta, time, markText),
                    style = MaterialTheme.typography.bodyMedium,
                    color = when (mark) {
                        NoteMark.NOT_DELIVERED -> MaterialTheme.colorScheme.error
                        NoteMark.READ, NoteMark.SHOWN -> MaterialTheme.colorScheme.primary
                        else -> MaterialTheme.colorScheme.onSurfaceVariant
                    },
                )
            }
        }
    }
}

/**
 * «Сообщение для <имя>» (FR-41, план 0.8.3, «UX-сценарий»): текст до 300 знаков, стикер (сетка 4 в ряд:
 * «Без стикера» и первые 11, «Все N» — полный список), звук с прослушиванием, сколько показывать,
 * «Отправить». Отправка и отметки — как раньше (протокол, раздел 5а).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun NoteScreen(childId: Long, name: String, vm: AppViewModel, onBack: () -> Unit, onSent: () -> Unit) {
    var text by rememberSaveable { mutableStateOf("") }
    var tried by remember { mutableStateOf(false) }
    var allStickers by rememberSaveable { mutableStateOf(false) }
    val sounds = rememberSoundPreview()
    val field = remember { FocusRequester() }
    LaunchedEffect(Unit) {
        withFrameNanos { }
        // Сразу набирать текст; после поворота на полном списке стикеров поля в дереве нет.
        if (!allStickers) runCatching { field.requestFocus() }
    }
    if (allStickers) {
        StickerListScreen(
            stickers = vm.noteStickers,
            selected = vm.noteSticker,
            onPick = { id ->
                vm.chooseSticker(id)
                allStickers = false
            },
            onBack = { allStickers = false },
        )
        return
    }
    BackHandler(onBack = onBack)
    val len = Proto.noteLength(text)
    val ok = Proto.noteOk(text)
    val busy = vm.noteBusy
    val err = vm.noteError
    // Ошибка видна, когда текст уже набран неверно (длиннее 300, больше 5 строк) или после «Отправить».
    val showBad = !ok && (tried || text.isNotBlank())
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.note_title, name), maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    IconButton(onClick = onBack, enabled = !busy) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.note_back))
                    }
                },
            )
        },
    ) { pad ->
        Column(
            verticalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier
                .fillMaxSize()
                .padding(pad)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 12.dp),
        ) {
            val counter = stringResource(R.string.note_counter, len, Proto.MAX_NOTE)
            val counterDesc = stringResource(R.string.note_counter_desc, len, Proto.MAX_NOTE)
            OutlinedTextField(
                value = text,
                onValueChange = { v ->
                    text = v
                    vm.clearNoteError()
                },
                label = { Text(stringResource(R.string.note_label)) },
                minLines = 3,
                maxLines = 6,
                enabled = !busy,
                isError = showBad,
                supportingText = {
                    Row(Modifier.fillMaxWidth()) {
                        if (showBad) {
                            val tooManyLines = Proto.cleanNote(text).count { it == '\n' } > Proto.MAX_NOTE_LINES
                            Text(
                                stringResource(if (tooManyLines && len <= Proto.MAX_NOTE) R.string.err_note_lines else R.string.err_note_range),
                                modifier = Modifier.weight(1f),
                            )
                        } else {
                            Spacer(Modifier.weight(1f))
                        }
                        Text(counter, modifier = Modifier.semantics { contentDescription = counterDesc })
                    }
                },
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                modifier = Modifier
                    .fillMaxWidth()
                    .focusRequester(field),
            )

            // ---------- стикер ----------
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                SectionTitle(stringResource(R.string.note_sticker_title), Modifier.weight(1f))
                if (vm.noteStickers.size > Proto.STICKER_GRID) {
                    val allDesc = stringResource(R.string.note_sticker_all_desc, vm.noteStickers.size)
                    TextButton(
                        onClick = {
                            sounds.stop()
                            allStickers = true
                        },
                        enabled = !busy,
                        modifier = Modifier
                            .heightIn(min = 48.dp)
                            .semantics { contentDescription = allDesc },
                    ) { Text(stringResource(R.string.note_sticker_all, vm.noteStickers.size)) }
                }
            }
            val selected = vm.noteSticker
            val first = vm.noteStickers.take(Proto.STICKER_GRID)
            // Стикер, выбранный из полного списка, виден в сетке первым после «Без стикера».
            val shown = if (selected.isNotEmpty() && selected !in first) listOf(selected) + first.dropLast(1) else first
            Column(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.selectableGroup()) {
                for (row in (listOf("") + shown).chunked(STICKER_COLUMNS)) {
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                        for (id in row) {
                            StickerCell(
                                id = id,
                                selected = id == selected,
                                enabled = !busy,
                                onClick = { vm.chooseSticker(id) },
                                modifier = Modifier.weight(1f),
                            )
                        }
                        repeat(STICKER_COLUMNS - row.size) { Spacer(Modifier.weight(1f)) }
                    }
                }
            }

            // ---------- звук ----------
            SectionTitle(stringResource(R.string.note_sound_title))
            Column(Modifier.selectableGroup()) {
                SoundRow(
                    label = stringResource(R.string.note_sound_none),
                    selected = vm.noteSound == Proto.NO_SOUND,
                    enabled = !busy,
                    onSelect = { vm.chooseSound(Proto.NO_SOUND) },
                    playDesc = null,
                    onPlay = {},
                )
                for (n in vm.noteSounds) {
                    SoundRow(
                        label = stringResource(R.string.note_sound_n, n),
                        selected = vm.noteSound == n,
                        enabled = !busy,
                        onSelect = { vm.chooseSound(n) },
                        playDesc = stringResource(R.string.note_sound_play_desc, n),
                        onPlay = {
                            // Как на макете: прослушанный звук становится выбранным.
                            if (!busy) vm.chooseSound(n)
                            sounds.play(n)
                        },
                    )
                }
            }

            // ---------- сколько показывать ----------
            SectionTitle(stringResource(R.string.note_show_title))
            Column(Modifier.selectableGroup()) {
                for (sec in Proto.SHOW_SEC) {
                    val on = vm.noteShowSec == sec
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(min = 48.dp)
                            .selectable(
                                selected = on,
                                enabled = !busy,
                                role = Role.RadioButton,
                                onClick = { vm.chooseShowSec(sec) },
                            ),
                    ) {
                        RadioButton(selected = on, onClick = null, enabled = !busy)
                        Spacer(Modifier.width(12.dp))
                        Text(stringResource(showSecRes(sec)), style = MaterialTheme.typography.bodyLarge)
                    }
                }
            }

            // Постоянная область итога: TalkBack зачитывает отправку и ошибку сам.
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 24.dp)
                    .semantics { liveRegion = LiveRegionMode.Polite },
            ) {
                when {
                    busy -> {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(8.dp))
                        Text(stringResource(R.string.note_sending), style = MaterialTheme.typography.bodyLarge)
                    }
                    err != null -> Text(
                        problemText(err, vm.lastSeenMs),
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
            Button(
                // Кнопка доступна и при неверном тексте: нажатие объясняет, что не так (не «молчит»).
                onClick = {
                    tried = true
                    if (ok) {
                        sounds.stop()
                        vm.sendNote(childId, text, onSent)
                    } else {
                        runCatching { field.requestFocus() }
                    }
                },
                enabled = !busy,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 52.dp),
            ) { Text(stringResource(R.string.note_send)) }
        }
    }
}

private const val STICKER_COLUMNS = 4

@Composable
private fun SectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(
        text,
        style = MaterialTheme.typography.titleMedium,
        modifier = modifier
            .padding(top = 4.dp)
            .semantics { heading() },
    )
}

/**
 * Клетка стикера: квадрат со скруглением, выбранная — рамка 3 dp цвета primary темы и движущийся
 * стикер; остальные — PNG первого кадра. "" — «Без стикера» (подпись вместо картинки).
 */
@Composable
private fun StickerCell(id: String, selected: Boolean, enabled: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(12.dp)
    val desc = if (id.isEmpty()) stringResource(R.string.note_sticker_none) else stringResource(R.string.note_sticker_one, id)
    BoxWithConstraints(
        contentAlignment = Alignment.Center,
        modifier = modifier
            .aspectRatio(1f)
            .heightIn(min = 48.dp)
            .clip(shape)
            .background(MaterialTheme.colorScheme.surfaceContainerLowest)
            .border(
                width = if (selected) 3.dp else 1.dp,
                color = if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outlineVariant,
                shape = shape,
            )
            .selectable(selected = selected, enabled = enabled, role = Role.RadioButton, onClick = onClick)
            // Выбор, роль и действие — от selectable выше; картинка и подпись внутри заменены одной подписью.
            .clearAndSetSemantics { contentDescription = desc },
    ) {
        val img = maxWidth - 14.dp
        when {
            id.isEmpty() -> Text(
                desc,
                style = MaterialTheme.typography.labelMedium,
                textAlign = TextAlign.Center,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(4.dp),
            )
            selected -> StickerAnimated(id, img)
            else -> StickerPreview(id, img)
        }
    }
}

/** Строка звука: выбор (RadioButton) и, у звуков, кнопка ▶ «Прослушать Звук N» рядом. */
@Composable
private fun SoundRow(
    label: String,
    selected: Boolean,
    enabled: Boolean,
    onSelect: () -> Unit,
    playDesc: String?,
    onPlay: () -> Unit,
) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier
                .weight(1f)
                .heightIn(min = 48.dp)
                .selectable(selected = selected, enabled = enabled, role = Role.RadioButton, onClick = onSelect),
        ) {
            RadioButton(selected = selected, onClick = null, enabled = enabled)
            Spacer(Modifier.width(12.dp))
            Text(label, style = MaterialTheme.typography.bodyLarge)
        }
        if (playDesc != null) {
            IconButton(onClick = onPlay) {
                Icon(Icons.Filled.PlayArrow, contentDescription = playDesc, tint = MaterialTheme.colorScheme.primary)
            }
        } else {
            Spacer(Modifier.size(48.dp))
        }
    }
}

/** «Все стикеры»: полный набор сеткой 4 в ряд; выбор возвращает на экран сообщения. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun StickerListScreen(stickers: List<String>, selected: String, onPick: (String) -> Unit, onBack: () -> Unit) {
    BackHandler(onBack = onBack)
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.note_stickers_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.note_back))
                    }
                },
            )
        },
    ) { pad ->
        LazyVerticalGrid(
            columns = GridCells.Fixed(STICKER_COLUMNS),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier
                .fillMaxSize()
                .padding(pad)
                .selectableGroup(),
        ) {
            item(key = "none") { StickerCell("", selected.isEmpty(), true, { onPick("") }) }
            gridItems(stickers, key = { it }) { id -> StickerCell(id, id == selected, true, { onPick(id) }) }
        }
    }
}
