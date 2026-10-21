# S9. «Попросить ещё» с экрана блокировки: пароль родителя и «+30» (ТЗ 11, эталон G7).
# Часы ядра идут в 60 раз быстрее. Лимит 5 мин: экран блокировки через 5 секунд.
# Дано: время вышло. Когда: «Попросить ещё», верный пароль, «+30». Тогда: можно ещё 30 мин
# активного времени; предупреждения за 15, 5, 1 мин до нового конца и блокировка.
# Проверка пароля стражем делает страж «bound», родительскую сессию не открывает, поэтому
# после «+30» страж снова виден и показывает предупреждения. Вход родителя — только в конце,
# для чтения журнала.
# Снимки: s09-ask-password.png (панель пароля после «Попросить ещё»), s09-unlocked.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

function Wait-Remaining([int]$MaxS, [double]$Seconds, [string]$What) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($true) {
    $st = Get-Status
    if ($st.remainingS -le $MaxS) { return $st }
    if ($sw.Elapsed.TotalSeconds -gt $Seconds) { throw "За $Seconds с не дождались: $What (остаток $($st.remainingS) с)" }
    Start-Sleep -Milliseconds 40
  }
}

Invoke-Scenario "S9" "«Попросить ещё»: пароль и +30 минут, предупреждения до нового конца" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Лимит 5 мин: время выходит, показан экран блокировки" {
    Start-Child 5
    Resume-Clock
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    Assert-That (Test-HasButton $g "askMore") "На экране нет кнопки «Попросить ещё»: $(Format-GuardState $g)"
    $st = Get-Status
    Assert-That ($st.remainingS -eq 0) "Остаток $($st.remainingS) с, ждали 0"
  }

  Step "«Попросить ещё»: открылась панель пароля, пароль набран" {
    Invoke-GuardClick "askMore"
    $g = Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "password" }).Count -ge 1 } 8 "панель пароля с полем"
    Assert-That (Test-HasButton $g "submit") "На панели пароля нет «Продолжить»: $(Format-GuardState $g)"
    Enter-GuardPassword $script:ParentPassword
    Start-Sleep -Milliseconds 600
    Save-Screen "s09-ask-password"
  }

  Step "Верный пароль: выбор доп. времени, «+30»" {
    Invoke-GuardClick "submit"
    $g = Wait-GuardState { param($s) Test-HasButton $s "add30" } 8 "после пароля кнопки «+15», «+30», «+60»"
    Invoke-GuardClick "add30"
    $g = Wait-LockHidden 10
    $st = Get-Status
    Write-Log "После +30: статус $($st.status), остаток $($st.remainingS) с, лимит $($st.limitS) с"
    Assert-That ($st.lock -eq "none") "Блокировка после +30: '$($st.lock)'"
    Assert-That ($st.limitS -eq 2100) "Лимит дня с доп. временем $($st.limitS) с, ждали 2100 (5 мин + 30 мин)"
    Assert-Between $st.remainingS 1795 1800 "Остаток после «+30», с"
    Save-Screen "s09-unlocked"
  }

  Step "Предупреждения 15, 5, 1 до нового конца и блокировка" {
    Resume-Clock
    [void](Wait-Remaining 900 60 "остаток 15 мин до нового конца")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 1 } 10 "плашка за 15 минут"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15") "Плашки: $((Get-WarnThresholds $g) -join ','), ждали 15"
    [void](Wait-Remaining 300 60 "остаток 5 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 2 } 10 "плашка за 5 минут"
    [void](Wait-Remaining 57 60 "остаток 1 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 3 } 10 "плашка за 1 минуту"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15,5,1") "Плашки: $((Get-WarnThresholds $g) -join ','), ждали 15,5,1"
    $g = Wait-LockShown 20 "timeUp"
    Suspend-Clock
    Write-Log "Новый конец: экран блокировки показан снова"
  }

  Step "Журнал родителя: «Доп. время +30 мин»" {
    Open-Parent
    $found = Find-Events 'доп\. время \+30 мин'
    Assert-That ($found.Count -eq 1) "Записей «Доп. время +30 мин»: $($found.Count), ждали одну"
  }

  Write-Log "S9: «Попросить ещё» +30 дало ещё 30 мин, предупреждения перезавелись, блокировка в новый срок"
}
