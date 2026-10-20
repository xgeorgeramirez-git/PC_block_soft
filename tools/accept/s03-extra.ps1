# S3. Аварийный выход: с экрана блокировки пароль родителя и «+15» (ТЗ 11, эталон G7).
# Часы ядра идут в 60 раз быстрее. Лимит 5 мин: экран блокировки появляется через 5 секунд.
# Дано: экран блокировки. Когда: «Пароль родителя», верный пароль, «+15». Тогда: экран снят, остаток 15 мин,
# в журнале «Доп. время +15 мин». Нажатия идут стражу через TestGuard (настоящие сообщения его окон).
# Снимки: s03-password.png (панель пароля, поле заполнено), s03-unlocked.png (экран снят, плашка «Добавлено»).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S3" "Экран блокировки: пароль родителя и +15 минут" {
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
    Assert-That ((Test-HasButton $g "parentPassword") -and (Test-HasButton $g "logoff")) "Нет двух кнопок экрана: $(Format-GuardState $g)"
    $st = Get-Status
    Assert-That ($st.remainingS -eq 0) "Остаток $($st.remainingS) с, ждали 0"
  }

  Step "«Пароль родителя»: открылась панель пароля, пароль набран" {
    Invoke-GuardClick "parentPassword"
    $g = Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "password" }).Count -ge 1 } 8 "панель пароля с полем"
    Assert-That (Test-HasButton $g "submit") "На панели пароля нет кнопки «Продолжить»: $(Format-GuardState $g)"
    Enter-GuardPassword $script:ParentPassword
    Start-Sleep -Milliseconds 600
    Save-Screen "s03-password"
  }

  Step "Верный пароль: открылся выбор действий" {
    Invoke-GuardClick "submit"
    $g = Wait-GuardState { param($s) Test-HasButton $s "add15" } 8 "после верного пароля появились кнопки «+15», «+30», «+60»"
    Assert-That ((Test-HasButton $g "add30") -and (Test-HasButton $g "add60")) "Нет кнопок «+30», «+60»: $(Format-GuardState $g)"
    Assert-That (-not (Test-HasButton $g "clearBlock")) "При блокировке по времени есть «Снять блокировку»: $(Format-GuardState $g)"
  }

  Step "«+15»: экран снят, остаток 15 мин" {
    Invoke-GuardClick "add15"
    $g = Wait-LockHidden 10
    Write-Log "Экран снят через $([math]::Round($script:WaitElapsed, 1)) с"
    $st = Get-Status
    Write-Log "Статус: $($st.status), остаток $($st.remainingS) с, надпись «$($st.label)», $($st.summary)"
    Assert-That ($st.lock -eq "none") "Блокировка в статусе службы: '$($st.lock)'"
    Assert-Between $st.remainingS 895 900 "Остаток после «+15», с"
    Assert-That ($st.limitS -eq 1200) "Лимит дня с доп. временем $($st.limitS) с, ждали 1200 (5 мин и 15 мин)"
    Save-Screen "s03-unlocked"
  }

  Step "Журнал родителя: «Доп. время +15 мин»" {
    Open-Parent
    $found = Find-Events 'доп\. время \+15 мин'
    Write-Log "Записи журнала: $((Get-Events | ForEach-Object { $_.text }) -join ' | ')"
    Assert-That ($found.Count -eq 1) "Записей «Доп. время +15 мин»: $($found.Count), ждали одну"
    $card = @(List (Get-Today).children)[0]
    Assert-That ($card.extraMin -eq 15) "Доп. время на сегодня $($card.extraMin) мин, ждали 15"
  }

  Write-Log "S3: пароль и +15 с экрана блокировки сняли экран, остаток 15 мин, запись в журнале"
}
