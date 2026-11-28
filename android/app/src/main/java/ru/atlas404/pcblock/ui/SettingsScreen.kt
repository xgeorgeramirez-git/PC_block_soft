package ru.atlas404.pcblock.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Format

/**
 * «Настройки» (нижнее меню, FR-45): связь с компьютером, «Обновить», «Подключить заново» при сбое
 * привязки, «Отключить телефон»; «Шрифт Inter: лицензия SIL OFL 1.1» — текст лицензии (ТЗ 8.0 п. 2).
 */
@Composable
fun SettingsScreen(vm: AppViewModel, onUnpair: () -> Unit) {
    var showLicense by rememberSaveable { mutableStateOf(false) }
    Column(
        verticalArrangement = Arrangement.spacedBy(14.dp),
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(start = 16.dp, end = 16.dp, top = 18.dp, bottom = 16.dp),
    ) {
        ScreenTitle(stringResource(R.string.settings_title))

        AppCard {
            SectionHead(stringResource(R.string.settings_pc))
            val link = vm.link
            val err = vm.mainError
            Text(
                when {
                    err != null -> problemText(err, vm.lastSeenMs)
                    link == null -> stringResource(R.string.pc_checking)
                    link is Format.Link.Offline && link.sinceMs > 0 -> stringResource(R.string.pc_offline_since, clockText(link.sinceMs))
                    link is Format.Link.Offline -> stringResource(R.string.pc_offline)
                    else -> stringResource(R.string.pc_online)
                },
                fontSize = 16.sp,
                fontWeight = FontWeight.Medium,
                color = if (err != null) Tok.RemainRed else Tok.Ink,
            )
            vm.report?.let { MutedText(stringResource(R.string.data_at, clockText(it.ts)), size = 14) }
            MutedText(stringResource(R.string.settings_server, vm.url), size = 14)
            PlainButton(stringResource(R.string.settings_refresh), onClick = { vm.refreshNow() })
            if (vm.needRepair) MainButton(stringResource(R.string.reconnect), onClick = onUnpair)
        }

        AppCard(gap = 0.dp) {
            SectionHead(stringResource(R.string.settings_about))
            LinkRow(AppIcons.Info, stringResource(R.string.settings_font), onClick = { showLicense = true })
        }

        PlainButton(stringResource(R.string.menu_unpair), onClick = onUnpair, danger = true)
    }

    if (showLicense) {
        val context = LocalContext.current
        val text = remember {
            runCatching { context.assets.open("licenses/Inter-OFL.txt").bufferedReader(Charsets.UTF_8).use { it.readText() } }
                .getOrDefault("")
        }
        AlertDialog(
            onDismissRequest = { showLicense = false },
            title = { Text(stringResource(R.string.settings_font)) },
            text = {
                Text(
                    text,
                    fontSize = 12.sp,
                    color = Tok.Ink,
                    modifier = Modifier
                        .heightIn(max = 420.dp)
                        .verticalScroll(rememberScrollState()),
                )
            },
            confirmButton = {
                TextButton(onClick = { showLicense = false }, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(stringResource(R.string.settings_close))
                }
            },
        )
    }
}

@Composable
private fun SectionHead(text: String) {
    Text(
        text,
        fontSize = 16.sp,
        fontWeight = FontWeight.SemiBold,
        color = Tok.Ink,
        modifier = Modifier
            .padding(bottom = 4.dp)
            .semantics { heading() },
    )
}
