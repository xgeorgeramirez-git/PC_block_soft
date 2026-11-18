package ru.atlas404.pcblock.ui

import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import ru.atlas404.pcblock.R
import ru.atlas404.pcblock.proto.Problem

/** Шаг 4: адрес сервера (заполнен), код с экрана компьютера, «Подключить». */
@Composable
fun ConnectScreen(vm: AppViewModel) {
    val focus = LocalFocusManager.current
    val err = vm.connectError
    Scaffold { pad ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(pad)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 24.dp, vertical = 32.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            Text(
                stringResource(R.string.connect_title),
                style = MaterialTheme.typography.headlineMedium,
                modifier = Modifier.semantics { heading() },
            )
            Text(
                stringResource(R.string.connect_hint),
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            OutlinedTextField(
                value = vm.url,
                onValueChange = { vm.url = it },
                label = { Text(stringResource(R.string.connect_url_label)) },
                singleLine = true,
                enabled = !vm.connectBusy,
                isError = err == Problem.BAD_ADDRESS,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Next),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = vm.code,
                onValueChange = { v -> vm.code = v.filter { it.isDigit() }.take(6) },
                label = { Text(stringResource(R.string.connect_code_label)) },
                supportingText = { Text(stringResource(R.string.connect_code_support)) },
                singleLine = true,
                enabled = !vm.connectBusy,
                isError = err == Problem.CODE_FORMAT || err == Problem.BAD_CODE,
                textStyle = MaterialTheme.typography.headlineSmall,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = {
                    focus.clearFocus()
                    vm.connect()
                }),
                modifier = Modifier.fillMaxWidth(),
            )
            // Постоянная область для ошибок: TalkBack зачитывает новое сообщение сам.
            Text(
                text = err?.let { stringResource(it.res()) } ?: "",
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier
                    .fillMaxWidth()
                    .semantics { liveRegion = LiveRegionMode.Polite },
            )
            Button(
                onClick = {
                    focus.clearFocus()
                    vm.connect()
                },
                enabled = !vm.connectBusy,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 56.dp),
            ) {
                if (vm.connectBusy) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        CircularProgressIndicator(modifier = Modifier.size(20.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(12.dp))
                        Text(stringResource(R.string.connect_busy), style = MaterialTheme.typography.titleMedium)
                    }
                } else {
                    Text(stringResource(R.string.connect_button), style = MaterialTheme.typography.titleMedium)
                }
            }
        }
    }
}
