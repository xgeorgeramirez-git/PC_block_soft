# LE1. Выходы с экрана блокировки: «Выключить компьютер» и «Смена пользователя» (ТЗ 8.2.3 п.3, 8, 9; D-40).
# Часы ядра идут в 60 раз быстрее. Лимит 5 мин: экран блокировки появляется через 5 секунд.
# В тестовом режиме служба выключение и смену пользователя не выполняет (раннер CI нельзя выключить),
# а записывает в GuardState.Exits: "shutdown", "switchUser". «Выйти из учётной записи» здесь не нажимается:
# его настоящий вызов завершил бы сеанс раннера, проверяется только то, что кнопка видна.
# Дано: экран блокировки. Тогда: четыре выхода; «Выключить компьютер» требует подтверждения и до него ничего
# не выполняет; «Смена пользователя» выполняется сразу, без подтверждения.
# Снимки: s-lockexits.png (экран с четырьмя выходами), s-lockexits-confirm.png (подтверждение выключения).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "LE1" "Экран блокировки: выключение и смена пользователя" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Лимит 5 мин: время выходит, показан экран блокировки" {
    Start-Child 5
    Resume-Clock
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    Write-Log "Экран показан через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-GuardState $g)"
  }

  Step "На экране все четыре выхода; ничего ещё не запрошено" {
    $g = Get-GuardState
    foreach ($b in @("parentPassword", "shutdown", "logoff", "switchUser")) {
      Assert-That (Test-HasButton $g $b) "На экране блокировки нет кнопки '$b': $(Format-GuardState $g)"
    }
    Assert-That (@(List $g.exits).Count -eq 0) "До нажатий уже есть выходы: $(Format-GuardState $g)"
    Start-Sleep -Milliseconds 600
    Save-Screen "s-lockexits"
  }

  Step "«Выключить компьютер»: панель подтверждения, пока не подтвердили, ничего не запрошено" {
    Invoke-GuardClick "shutdown"
    $g = Wait-GuardState { param($s) Test-HasButton $s "confirmShutdown" } 8 "панель подтверждения выключения"
    Assert-That (Test-HasButton $g "cancel") "На панели подтверждения нет кнопки «Отмена»: $(Format-GuardState $g)"
    Assert-That (-not (Test-HasExit $g "shutdown")) "Выключение запрошено без подтверждения: $(Format-GuardState $g)"
    Start-Sleep -Milliseconds 600
    Save-Screen "s-lockexits-confirm"
  }

  Step "«Отмена»: назад на экран с выходами, выключение не запрошено" {
    Invoke-GuardClick "cancel"
    $g = Wait-GuardState { param($s) (Test-HasButton $s "shutdown") -and (-not (Test-HasButton $s "confirmShutdown")) } 8 "возврат на экран блокировки после «Отмена»"
    Start-Sleep -Seconds 2
    $g = Get-GuardState
    Assert-That (-not (Test-HasExit $g "shutdown")) "После «Отмена» выключение запрошено: $(Format-GuardState $g)"
    Assert-That ($g.lockShown) "После «Отмена» экран блокировки пропал: $(Format-GuardState $g)"
  }

  Step "«Выключить» в подтверждении: служба записала выключение, раннер работает" {
    Invoke-GuardClick "shutdown"
    [void](Wait-GuardState { param($s) Test-HasButton $s "confirmShutdown" } 8 "панель подтверждения выключения")
    Invoke-GuardClick "confirmShutdown"
    $g = Wait-GuardState { param($s) Test-HasExit $s "shutdown" } 10 "в GuardState.Exits появилось shutdown"
    Write-Log "Выключение записано через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-GuardState $g)"
    Assert-That (-not (Test-HasExit $g "switchUser")) "Смена пользователя запрошена без нажатия: $(Format-GuardState $g)"
    Assert-That ((Get-ServiceState) -eq "Running") "Служба не работает после выключения в тестовом режиме"
  }

  Step "«Смена пользователя»: без подтверждения, служба записала смену" {
    # После выключения экран может быть ещё на панели подтверждения или уже вернулся на главный вид.
    $g = Get-GuardState
    if (-not (Test-HasButton $g "switchUser")) {
      Invoke-GuardClick "cancel"
      [void](Wait-GuardState { param($s) Test-HasButton $s "switchUser" } 8 "кнопка «Смена пользователя» на экране")
    }
    Invoke-GuardClick "switchUser"
    $g = Wait-GuardState { param($s) Test-HasExit $s "switchUser" } 10 "в GuardState.Exits появилось switchUser"
    Write-Log "Смена пользователя записана через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-GuardState $g)"
    Assert-That (-not (Test-HasButton $g "confirmShutdown")) "Для смены пользователя показано подтверждение: $(Format-GuardState $g)"
    Assert-That ((Get-ServiceState) -eq "Running") "Служба не работает после смены пользователя в тестовом режиме"
  }

  Write-Log "LE1: четыре выхода на экране; выключение только после подтверждения; смена пользователя без него"
}
