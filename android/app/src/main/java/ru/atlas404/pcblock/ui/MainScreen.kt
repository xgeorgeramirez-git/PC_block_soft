package ru.atlas404.pcblock.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items as gridItems
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.LifecycleResumeEffect
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Format
import ru.atlas404.pcblock.proto.NoteMark
import ru.atlas404.pcblock.proto.Proto
import ru.atlas404.pcblock.proto.RecentStickers
import ru.atlas404.pcblock.proto.SoundNames
import ru.atlas404.pcblock.store.SavedNote

/**
 * Главный экран (FR-45, D-109): нижнее меню «Дети», «Промокоды», «Настройки»; вложенные экраны
 * (копилка, ждущие программы, сообщение, статистика) — поверх вкладки, меню остаётся видно.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun MainScreen(vm: AppViewModel) {
    // Обновление раз в 15 с, пока экран виден; при уходе в фон опрос останавливается.
    LifecycleResumeEffect(Unit) {
        vm.startPolling()
        onPauseOrDispose { vm.stopPolling() }
    }
    var askUnpair by rememberSaveable { mutableStateOf(false) }
    var customFor by remember { mutableStateOf<Proto.Child?>(null) }
    val rep = vm.report
    // Компьютер старше 0.8.4 (в отчёте нет bankMin) или отчёта ещё нет: «Промокодов» и ждущих нет
    // (протокол, раздел 6) — иначе promoList ушёл бы старому компьютеру и вернул range.
    val showNew = Proto.newPcTabs(rep)
    LaunchedEffect(showNew) {
        if (!showNew && vm.tab == Tab.PROMOS) vm.selectTab(Tab.KIDS)
    }
    // «Назад» на вкладке «Промокоды» или «Настройки» возвращает к детям.
    BackHandler(enabled = vm.sub == null && vm.tab != Tab.KIDS) { vm.selectTab(Tab.KIDS) }

    Scaffold(
        containerColor = Tok.Ground,
        bottomBar = { BottomNav(vm.tab, showNew, vm::selectTab) },
    ) { pad ->
        Box(
            Modifier
                .fillMaxSize()
                .padding(pad)
                .consumeWindowInsets(pad),
        ) {
            when (val s = vm.sub) {
                is Sub.Stats -> StatsScreen(child = rep?.children?.firstOrNull { it.id == s.child }, onBack = { vm.closeSub() })
                is Sub.Note -> NoteScreen(
                    childId = s.child,
                    name = rep?.children?.firstOrNull { it.id == s.child }?.name.orEmpty(),
                    vm = vm,
                    onBack = { if (!vm.noteBusy) vm.closeSub() },
                    onSent = { vm.closeSub(s) },
                )
                is Sub.Bank -> BankScreen(vm, s.child)
                is Sub.Pending -> PendingScreen(vm, s.child)
                null -> when (vm.tab) {
                    Tab.KIDS -> KidsTab(vm, onCustom = { customFor = it }, onRepair = { askUnpair = true })
                    Tab.PROMOS -> PromosScreen(vm)
                    Tab.SETTINGS -> SettingsScreen(vm, onUnpair = { askUnpair = true })
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

/** Вкладка «Дети» (макет phone2.py): заголовок и связь, развёрнутая карточка, остальные — компактно. */
@Composable
private fun KidsTab(vm: AppViewModel, onCustom: (Proto.Child) -> Unit, onRepair: () -> Unit) {
    val rep = vm.report
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 18.dp, bottom = 16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        item(key = "head") { KidsHeader(vm) }
        val err = vm.mainError
        if (err != null) {
            item(key = "error") {
                AppCard(gap = 10.dp) {
                    Text(
                        problemText(err, vm.lastSeenMs),
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Medium,
                        color = Tok.RemainRed,
                        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    )
                    if (vm.needRepair) MainButton(stringResource(R.string.reconnect), onClick = onRepair)
                }
            }
        }
        when {
            rep == null -> item(key = "nodata") {
                Text(stringResource(R.string.no_data), fontSize = 16.sp, color = Tok.Muted, modifier = Modifier.padding(vertical = 8.dp))
            }
            rep.children.isEmpty() -> item(key = "nochildren") {
                Text(stringResource(R.string.no_children), fontSize = 16.sp, color = Tok.Ink)
            }
            else -> {
                if (!rep.hasBank) {
                    item(key = "old") {
                        AppCard(gap = 6.dp) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                                Icon(AppIcons.Info, contentDescription = null, tint = Tok.Accent, modifier = Modifier.size(22.dp))
                                Text(stringResource(R.string.update_pc), fontSize = 15.sp, fontWeight = FontWeight.Medium, color = Tok.Ink)
                            }
                        }
                    }
                }
                val open = vm.expandedChild?.takeIf { id -> rep.children.any { it.id == id } } ?: rep.children.first().id
                itemsIndexed(rep.children, key = { _, c -> c.id }) { i, c ->
                    if (c.id == open) {
                        ChildCard(
                            child = c,
                            index = i,
                            report = rep,
                            busy = vm.busyChild == c.id,
                            locked = vm.isBusy,
                            note = vm.note?.takeIf { it.childId == c.id },
                            lastSeenMs = vm.lastSeenMs,
                            actions = CardActions(
                                onDelta = { d -> vm.command(c.id, d) },
                                onCustom = { onCustom(c) },
                                onBank = { vm.open(Sub.Bank(c.id)) },
                                onPending = { vm.open(Sub.Pending(c.id)) },
                                onNote = { vm.open(Sub.Note(c.id)) },
                                onStats = { vm.open(Sub.Stats(c.id)) },
                                onClearForbidden = { vm.clearForbidden(c.id) },
                            ),
                        )
                    } else {
                        ChildCompact(c, i, rep, onExpand = { vm.expandedChild = c.id })
                    }
                }
                item(key = "at") { MutedText(stringResource(R.string.data_at, clockText(rep.ts))) }
            }
        }
    }
}

/** «Дети» 26 sp и «Компьютер на связи» с точкой (точка дублирует текст, а не заменяет его). */
@Composable
private fun KidsHeader(vm: AppViewModel) {
    val link = vm.link
    val online = link is Format.Link.Online
    val text = when {
        link == null -> stringResource(R.string.pc_checking)
        link is Format.Link.Offline && link.sinceMs > 0 -> stringResource(R.string.pc_offline_since, clockText(link.sinceMs))
        link is Format.Link.Offline -> stringResource(R.string.pc_offline)
        else -> stringResource(R.string.pc_online)
    }
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
        ScreenTitle(stringResource(R.string.kids_title), size = 26)
        Spacer(Modifier.width(12.dp))
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.End,
            modifier = Modifier
                .weight(1f)
                .semantics(mergeDescendants = true) { liveRegion = LiveRegionMode.Polite },
        ) {
            Box(
                Modifier
                    .size(8.dp)
                    .background(if (online) Tok.RemainGreen else Tok.Muted, CircleShape),
            )
            Spacer(Modifier.width(6.dp))
            Text(text, fontSize = 13.sp, fontWeight = FontWeight.Medium, color = Tok.Muted, textAlign = TextAlign.End)
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

/**
 * Последние сообщения ребёнка (до 3, макет phones_more.py): текст, время отправки и отметка.
 * Отметки нет и компьютер не на связи — «Ждёт: компьютер не в сети» (FB-29, D-102).
 */
@Composable
private fun NoteList(notes: List<SavedNote>, pcOnline: Boolean) {
    AppCard(gap = 0.dp) {
        Text(
            stringResource(R.string.note_recent),
            fontSize = 15.sp,
            fontWeight = FontWeight.SemiBold,
            color = Tok.Ink,
            modifier = Modifier
                .padding(bottom = 8.dp)
                .semantics { heading() },
        )
        for (n in notes) {
            val mark = NoteMark.of(n.state, n.showSec, pcOnline)
            val markText = stringResource(mark.res())
            val time = clockText(n.sentAtMs)
            val oneLine = n.text.replace('\n', ' ')
            val desc = stringResource(R.string.note_item_desc, oneLine, time, markText)
            HorizontalDivider(thickness = 1.dp, color = Tok.Line)
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp),
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(vertical = 10.dp)
                    .clearAndSetSemantics {
                        contentDescription = desc
                        liveRegion = LiveRegionMode.Polite // смена отметки зачитывается сама
                    },
            ) {
                Column(Modifier.weight(1f)) {
                    Text(oneLine, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = Tok.Ink, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    MutedText(time)
                }
                Text(
                    markText,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.SemiBold,
                    textAlign = TextAlign.End,
                    color = when (mark) {
                        NoteMark.NOT_DELIVERED -> Tok.RemainRed
                        NoteMark.WAITING_OFFLINE -> Tok.Waiting
                        NoteMark.READ, NoteMark.SHOWN -> Tok.PlusText
                        else -> Tok.Muted
                    },
                    modifier = Modifier.width(150.dp),
                )
            }
        }
    }
}

/**
 * «Сообщение для <имя>» (FR-41): текст до 300 знаков, стикер (сетка 4×2: «Без стикера» и 7 последних
 * выбранных, D-103; «Все N» — полный список), звук с названием и прослушиванием (D-104), сколько показывать,
 * «Отправить»; ниже — последние сообщения с отметками (FB-29). Отправка — протокол, раздел 5а.
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
                if (vm.noteStickers.size > RecentStickers.SIZE) {
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
            val first = vm.noteGrid
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
            MutedText(stringResource(R.string.note_sticker_recent))

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
                val soundTpl = stringResource(R.string.note_sound_n)
                for (n in vm.noteSounds) {
                    val label = SoundNames.label(n, vm.soundNames, soundTpl)
                    SoundRow(
                        label = label,
                        selected = vm.noteSound == n,
                        enabled = !busy,
                        onSelect = { vm.chooseSound(n) },
                        playDesc = stringResource(R.string.note_sound_play_desc, label),
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
            // Кнопка доступна и при неверном тексте: нажатие объясняет, что не так (не «молчит»).
            MainButton(
                stringResource(R.string.note_send),
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
            )
            val recent = vm.notes.filter { it.child == childId }.sortedByDescending { it.sentAtMs }.take(3)
            if (recent.isNotEmpty()) NoteList(recent, vm.pcOnline)
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
