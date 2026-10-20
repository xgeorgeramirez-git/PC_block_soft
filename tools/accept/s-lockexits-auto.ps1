# LE2. Автосмена пользователя на экране блокировки без единого нажатия (ТЗ 8.2.3 п.10; D-40).
# В настоящем сеансе срок 15 минут; в тестовом режиме его сокращает значение реестра LockAutoSwitchS (здесь 5 с),
# чтобы не ждать. Часы ядра идут в 60 раз быстрее: лимит 5 мин даёт экран блокировки через 5 секунд.
# Дано: экран блокировки, родитель пароль не вводит, ребёнок ничего не нажимает. Когда: проходит срок.
# Тогда: служба записала смену пользователя (GuardState.Exits = switchUser) без единого нажатия и не раньше срока.
# Снимок: s-lockexits-auto.png (экран перед автосменой).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "LE2" "Экран блокировки: автосмена пользователя через срок" {
  Initialize-Scenario 60 5

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Лимит 5 мин: время выходит, показан экран блокировки; смена ещё не запрошена" {
    Start-Child 5
    Resume-Clock
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    $script:ShownAt = [Diagnostics.Stopwatch]::StartNew()
    Write-Log "Экран показан через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-GuardState $g)"
    Assert-That (-not (Test-HasExit $g "switchUser")) "Смена пользователя запрошена сразу при показе экрана: $(Format-GuardState $g)"
    Assert-That (Test-HasButton $g "switchUser") "На экране нет кнопки «Смена пользователя»: $(Format-GuardState $g)"
    # Смену могла запросить только автосмена, а её срок 5 с ещё не вышел (экран показан только что).
    Start-Sleep -Milliseconds 600
    Save-Screen "s-lockexits-auto"
  }

  Step "Ничего не нажимая, ждём автосмену: switchUser появляется сам" {
    $g = Wait-GuardState { param($s) Test-HasExit $s "switchUser" } 40 "автосмена: в GuardState.Exits появилось switchUser"
    Write-Log ("Автосмена записана через {0:N1} с после показа экрана: {1}" -f $script:ShownAt.Elapsed.TotalSeconds, (Format-GuardState $g))
    Assert-That ($script:ShownAt.Elapsed.TotalSeconds -ge 3) ("Автосмена сработала слишком рано: через {0:N1} с, срок 5 с" -f $script:ShownAt.Elapsed.TotalSeconds)
    Assert-That (-not (Test-HasExit $g "shutdown")) "Запрошено выключение, которого никто не просил: $(Format-GuardState $g)"
    Assert-That ((Get-ServiceState) -eq "Running") "Служба не работает после автосмены в тестовом режиме"
  }

  Write-Log "LE2: без нажатий через срок 5 с служба записала смену пользователя"
}
