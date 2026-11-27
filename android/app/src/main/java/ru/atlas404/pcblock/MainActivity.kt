package ru.atlas404.pcblock

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.Composable
import androidx.lifecycle.viewmodel.compose.viewModel
import ru.atlas404.pcblock.ui.AppViewModel
import ru.atlas404.pcblock.ui.CompareScreen
import ru.atlas404.pcblock.ui.ConnectScreen
import ru.atlas404.pcblock.ui.MainScreen
import ru.atlas404.pcblock.ui.PCBlockTheme
import ru.atlas404.pcblock.ui.Screen

/** Фон экранов #F3F5F9 (ТЗ 8.0 п. 9) как ARGB для системных полос. */
private const val GROUND = 0xFFF3F5F9.toInt()

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Вид всегда светлый (ТЗ 8.0 п. 1): тёмные значки строки состояния и при тёмной теме телефона.
        val bars = SystemBarStyle.light(GROUND, GROUND)
        enableEdgeToEdge(statusBarStyle = bars, navigationBarStyle = bars)
        setContent {
            PCBlockTheme { App() }
        }
    }
}

@Composable
private fun App(vm: AppViewModel = viewModel()) {
    when (vm.screen) {
        Screen.CONNECT -> ConnectScreen(vm)
        Screen.COMPARE -> CompareScreen(vm)
        Screen.MAIN -> MainScreen(vm)
    }
}
