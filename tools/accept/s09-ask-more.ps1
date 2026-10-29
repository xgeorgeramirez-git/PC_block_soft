# S9. «Попросить ещё» с экрана блокировки: пароль родителя и «+30» (ТЗ 11, эталон G7).
# Часы ядра идут в 60 раз быстрее. Лимит 5 мин: экран блокировки через 5 секунд.
# Дано: время вышло. Когда: «Попросить ещё», верный пароль, «+30». Тогда: можно ещё 30 мин
# активного времени; в журнале «Доп. время +30 мин»; до нового конца предупреждения за 15 и 5 мин,
# за 30 мин нет (R9: равное новому расстоянию не показывается, D-54).
# Секунды порогов точно проверяют эталон G7 и тесты ядра; здесь по журналу показанных предупреждений
# стража после «+30»: 15 и 5 есть, 30 нет, повторов нет (плашку 1 мин экран блокировки при ускорении
# 60 может сменить в ту же секунду: её не требуем).
# Снимки: s09-ask-password.png (панель пароля после «Попросить ещё»), s09-unlocked.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S9" "«Попросить ещё»: пароль и +30 минут" {
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

  Step "Верный пароль: выбор доп. времени, «+30»; экран снят, ещё 30 мин" {
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
    $script:WarnBefore = @(Get-WarnThresholds $g).Count
  }

  Step "До нового конца: плашки 15 и 5, без «за 30»" {
    Resume-Clock
    $g = Wait-LockShown 60 "timeUp"
    Suspend-Clock
    $after = @(Get-WarnThresholds $g | Select-Object -Skip $script:WarnBefore)
    Write-Log "Плашки после «+30»: $($after -join ',')"
    Assert-That (-not ($after -contains 30)) "После «+30» показано предупреждение за 30 мин: $($after -join ',')"
    $head = ($after | Select-Object -First 2) -join ","
    Assert-That ($head -eq "15,5") "После «+30» плашки $($after -join ','), ждали 15,5 (и 1)"
    $seen = @{}
    foreach ($t in $after) { Assert-That (-not $seen.ContainsKey($t)) "Порог $t после «+30» показан дважды"; $seen[$t] = $true }
  }

  Step "Журнал родителя: «Доп. время +30 мин»" {
    Open-Parent
    $found = Find-Events 'доп\. время \+30 мин'
    Assert-That ($found.Count -eq 1) "Записей «Доп. время +30 мин»: $($found.Count), ждали одну"
  }

  Write-Log "S9: «Попросить ещё» +30 с экрана блокировки дало ещё 30 мин активного времени, запись в журнале"
}
