# S41. Последняя минута идёт по часам до нуля при любом выходе (ТЗ R2, S41, эталон G13, D-60; фидбек-раунд 0.3.1).
# Заказчик: «когда напоминание выскакивает 1 минута, то если все закрыть и выйти то время все равно идёт и все блокируется».
# Часы ядра идут в 10 раз быстрее: 3 мин лимита это 18 с настоящего времени, последняя минута 6 с.
# Второго пользователя у раннера нет, а сеанс раннера после tsdiscon сам возвращается на экран (PL-10), поэтому «выход»
# ребёнка изображает тестовая команда службы TestSession: «отключение» сеанса ребёнка тем же путём, что Windows. Пока
# событие повторяется (раз в 2 с), служба считает сеанс отключённым; после последнего через 5 с (удержание отключения)
# список сеансов Windows снова показывает его на экране: это «вернуть сеанс».
# Вариант 1. Дано: остаток 3 мин. Когда: пришло предупреждение 1 мин (признак: плашка 1 мин в GuardState), ввод
#   остановлен, сеанс отключён на 90 с часов ядра. Тогда: ещё до возврата остаток 0, статус «время вышло»; после возврата
#   экран блокировки «Время вышло», в журнале нет записи об обходе.
# Вариант 2. Служба убита в последнюю минуту (taskkill /F, как выключение питания), после подъёма сеанс сразу отключён
#   на 90 с часов ядра: время по R2 не идёт, поэтому остаток 0 — только если момент окончания сохранён сразу после
#   предупреждения и пережил перезапуск (без сохранения остаток стоял бы на месте).
# Вариант 3 (контроль отмены). Копилка 30 мин, у плашки 1 мин «Взять из копилки», затем то же отключение: остаток не 0.
# Выход Win+L и настоящая смена пользователя на раннере не делаются (их не снять без пароля): проверяет заказчик вручную.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Записи журнала родителя об обходе (тексты из internal/service/texts.go, как в FB5).
$script:BypassPattern = 'обойти защиту|Защита программы была остановлена|закрыто не кнопкой окна|Значок программы был закрыт'

function Send-TestSession([string]$Event, [int]$SessionId) {
  $r = Send-Pcb "TestSession" @{ event = $Event; sessionId = $SessionId }
  Assert-Reply $r "TestSession $Event $SessionId"
}

# Лимит 3 мин; ребёнок активен, часы идут; ждёт плашку предупреждения за 1 мин (событие, а не время: PL-16).
function Start-ToLastMinute([int]$BonusMin = 0) {
  Test-InputSim
  Start-Child 3
  Open-Parent
  Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы"
  if ($BonusMin -gt 0) {
    Assert-Reply (Send-Parent "GrantBonus" @{ childId = (Get-ChildId); minutes = $BonusMin; reason = "S41" }) "GrantBonus $BonusMin"
  }
  Assert-Reply (Send-Parent "Logout") "Logout"
  $st = Get-Status
  Assert-That ($st.limitS -eq 180) "Лимит дня $($st.limitS) с, ждали 180"
  Resume-Clock
  $g = Wait-GuardState { param($s) (Get-WarnThresholds $s) -contains 1 } 40 "предупреждение за 1 минуту"
  $st = Get-Status
  Write-Log "Предупреждение 1 мин: остаток $($st.remainingS) с, статус $($st.status); $(Format-GuardState $g)"
  Assert-That ($st.remainingS -gt 0 -and $st.lock -eq "none") "К предупреждению время уже вышло: остаток $($st.remainingS) с, экран '$($st.lock)'"
  return $g
}

# Ребёнок «вышел»: ввода нет, сеанс отключён на $Seconds настоящих секунд (событие повторяется раз в 2 с).
function Invoke-Away([double]$Seconds) {
  Stop-Input
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
    Send-TestSession "disconnect" ([int]$script:MySession)
    Start-Sleep -Milliseconds 2000
  }
  Write-Log ("Сеанс ребёнка отключён {0:N1} с настоящего времени (около {1:N0} с часов ядра)" -f $sw.Elapsed.TotalSeconds, ($sw.Elapsed.TotalSeconds * $script:ClockSpeed))
}

Invoke-Scenario "S41" "Последняя минута идёт по часам до нуля при выходе, перезапуске службы; копилка отменяет" {
  Initialize-Scenario 10

  Step "Вариант 1: остаток 3 мин, пришло предупреждение за 1 минуту" {
    [void](Start-ToLastMinute)
  }

  Step "Ребёнок вышел (сеанс отключён на 90 с часов ядра): остаток 0 и «время вышло» ещё до возврата" {
    Invoke-Away 9
    $st = Get-Status
    Write-Log "Пока сеанс отключён: остаток $($st.remainingS) с, статус $($st.status), экран '$($st.lock)', «$($st.used)»"
    # Без исправления остаток стоял бы на месте всё время отключения (секунды не засчитываются по R2).
    Assert-That ($st.remainingS -eq 0) "Пока ребёнка нет, остаток $($st.remainingS) с, ждали 0 (последняя минута идёт по часам)"
    Assert-That ($st.status -eq "timeUp" -and $st.lock -eq "timeUp") "Статус '$($st.status)', экран '$($st.lock)', ждали timeUp"
    $script:UsedAway = [string]$st.used
  }

  Step "Сеанс вернулся: экран «Время вышло» сразу, строка «Сегодня» та же, записи об обходе нет" {
    Start-Input 1000
    Start-Sleep -Seconds 7 # удержание отключения 5 с истекло: список Windows снова показывает сеанс на экране
    $g = Wait-LockShown 15 "timeUp"
    $st = Get-Status
    Write-Log "После возврата: остаток $($st.remainingS) с, статус $($st.status), причина «$($st.reason)», «$($st.used)»"
    Assert-That ($st.remainingS -eq 0 -and $st.lock -eq "timeUp") "После возврата остаток $($st.remainingS) с, экран '$($st.lock)', ждали 0 и timeUp"
    Assert-That ([string]$st.used -eq $script:UsedAway) "Строка «$($st.used)», ждали ту же, что при отключённом сеансе: «$($script:UsedAway)»"
    Start-Sleep -Seconds 1
    Save-Screen "s41-lock-after-return"
    Open-Parent
    Write-ServiceSessionLog "S41"
    Write-Log "Записи журнала: $(Format-Events)"
    $found = Find-Events $script:BypassPattern
    Assert-That ($found.Count -eq 0) "В журнале есть записи об обходе: $(($found | ForEach-Object { $_.text }) -join ' | ')"
  }

  Step "Вариант 2: свежая установка, предупреждение за 1 минуту, служба убита (как выключение питания)" {
    Initialize-Scenario 10
    [void](Start-ToLastMinute)
    Stop-Input
    # Момент сохраняется на ближайшем тике службы (0,1 с при ускорении 10). Ждём коротко: плановое сохранение раз в
    # 30 с часов ядра (3 с настоящего времени) за это время случается редко и не маскирует пропавшее немедленное.
    Start-Sleep -Milliseconds 300
    $oldPid = Get-ServicePid
    Assert-That ($oldPid -gt 0) "Процесс службы не найден"
    $script:Before = Get-Status
    Write-Log "Перед сбоем: остаток $($script:Before.remainingS) с"
    Close-PipeConn
    & taskkill.exe /F /PID $oldPid 2>&1 | ForEach-Object { Write-Log "taskkill: $_" }
    Wait-Until { (Get-ServiceState) -eq "Running" -and (Get-ServicePid) -gt 0 -and (Get-ServicePid) -ne $oldPid } 60 "служба поднята заново"
    $script:ClockPausedByUs = $false
    Wait-Until { try { (Send-Pcb "Hello").ok } catch { Close-PipeConn; $false } } 30 "служба отвечает по каналу"
  }

  Step "После подъёма службы сеанс отключён на 90 с часов ядра: остаток 0 и «время вышло»" {
    $st = Get-Status
    Write-Log "Сразу после подъёма: остаток $($st.remainingS) с, статус $($st.status), экран '$($st.lock)', «$($st.used)»"
    Invoke-Away 9
    $st = Get-Status
    Write-Log "После отключения: остаток $($st.remainingS) с, статус $($st.status), экран '$($st.lock)', «$($st.used)»"
    Assert-That ($st.remainingS -eq 0) "После перезапуска остаток $($st.remainingS) с, ждали 0 (момент окончания сохранён сразу после предупреждения)"
    Assert-That ($st.status -eq "timeUp" -and $st.lock -eq "timeUp") "Статус '$($st.status)', экран '$($st.lock)', ждали timeUp"
    Start-Input 1000
    Start-Sleep -Seconds 7
    [void](Wait-LockShown 40 "timeUp")
    Open-Parent
    # Только в журнал сценария: сбой самой службы (taskkill /F) к выходу ребёнка не относится, записи о нём здесь
    # не проверяются; запись об обходе при выходе ребёнка проверяет вариант 1.
    Write-Log "Записи журнала: $(Format-Events)"
  }

  Step "Вариант 3: свежая установка, копилка 30 мин, у плашки 1 мин «Взять из копилки»" {
    Initialize-Scenario 10
    $g = Start-ToLastMinute 30
    Suspend-Clock # последняя минута при ускорении 10 — 6 с: нажатие делаем на остановленных часах (как в S35)
    $g = Wait-GuardState { param($s) $s.plateKind -eq "oneMinute" -and (Test-HasButton $s "spendBonusPlate") } 10 "плашка 1 мин с «Взять из копилки»"
    Invoke-GuardClick "spendBonusPlate"
    $g = Wait-GuardState { param($s) [string]$s.plate -match "^Добавлено" } 5 "плашка «Добавлено» после «Взять из копилки»"
    $st = Get-Status
    Write-Log "После «Взять из копилки»: остаток $($st.remainingS) с, копилка $($st.bonusBalance) мин, плашка «$($g.plate)»"
    Assert-That ($st.remainingS -ge 1800) "Остаток $($st.remainingS) с, ждали не меньше 1800"
    $script:RemBonus = [int]$st.remainingS
    Resume-Clock
  }

  Step "Ребёнок вышел на 90 с часов ядра: отсчёт отменён, остаток не 0" {
    Invoke-Away 9
    $st = Get-Status
    Write-Log "Пока сеанс отключён: остаток $($st.remainingS) с (до выхода $($script:RemBonus)), статус $($st.status)"
    Assert-That ($st.remainingS -gt 0 -and $st.lock -eq "none") "Остаток $($st.remainingS) с, экран '$($st.lock)': копилка не отменила отсчёт последней минуты"
    # Отключённый сеанс не засчитывается (R2): остаток почти не изменился (допуск — секунды до отключения).
    Assert-That ($st.remainingS -ge ($script:RemBonus - 30)) "Остаток упал с $($script:RemBonus) до $($st.remainingS) с, пока ребёнка не было: время шло по часам"
  }

  Write-Log "S41: после предупреждения 1 мин время дошло до нуля без ребёнка и через перезапуск службы, без записи об обходе; копилка отменяет отсчёт"
}
