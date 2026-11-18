package ru.atlas404.pcblock

import android.os.Bundle
import androidx.activity.ComponentActivity
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

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
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
