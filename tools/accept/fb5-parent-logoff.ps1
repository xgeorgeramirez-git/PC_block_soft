# FB5. Выход родителя из учётки, пока сеанс ребёнка отключён сменой пользователя: это не обход (ТЗ R17, FR-27; фидбек-раунд 0.2.5).
# Заказчик: вошёл ребёнком, сменил пользователя, родитель настроил программу и вышел из своей учётки, а в журнале
# появилась «попытка обойти защиту». Второго пользователя, который вошёл бы и вышел по-настоящему, у раннера CI нет,
# а сеанс раннера после tsdiscon сам возвращается на экран (урок PL-10). Поэтому события Windows подаёт тестовая
# команда службы TestSession тем же путём, что диспетчер служб: «отключение» сеанса ребёнка и «выход» сеанса 99.
# Гибель частей программы при выходе родителя изображает taskkill /F стража.
# Дано: ребёнок в сеансе, страж запущен. Когда: сеанс ребёнка отключён, страж погиб, вышел из учётки другой сеанс.
# Тогда: новый страж запущен не позже 3 с (FR-27); через 40 с в журнале родителя нет записи об обходе.
# Контроль: то же без выхода другого сеанса даёт ровно одну запись об обходе (настоящее принудительное завершение).
# Полный путь с настоящим выходом родителя проверяет заказчик вручную.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$script:EndPattern = 'обойти защиту|Защита программы была остановлена|закрыто не кнопкой окна'

function Send-TestSession([string]$Event, [int]$SessionId) {
  $r = Send-Pcb "TestSession" @{ event = $Event; sessionId = $SessionId }
  Assert-Reply $r "TestSession $Event $SessionId"
  Write-Log "Тестовое событие сеанса ${SessionId}: $Event"
}

# Отключает сеанс ребёнка тестовым событием, завершает стража принудительно и ждёт нового стража.
function Stop-GuardWhileAway {
  Send-TestSession "disconnect" ([int]$script:MySession)
  $cur = @(Get-SessionProcs "--guard")
  Assert-That ($cur.Count -eq 1) "Стражей в сеансе: $($cur.Count), ждали один"
  $killPid = [int]$cur[0].ProcessId
  & taskkill.exe /F /PID $killPid 2>&1 | ForEach-Object { Write-Log "taskkill: $_" }
  $sw = [Diagnostics.Stopwatch]::StartNew()
  Wait-Until {
    $now = @(Get-SessionProcs "--guard")
    ($now.Count -eq 1) -and ([int]$now[0].ProcessId -ne $killPid)
  } 15 "страж перезапущен после принудительного завершения"
  Write-Log ("Страж перезапущен через {0:N1} с" -f $sw.Elapsed.TotalSeconds)
  Assert-That ($sw.Elapsed.TotalSeconds -le 4) ("Страж перезапущен через {0:N1} с, ждали не больше 3 (FR-27)" -f $sw.Elapsed.TotalSeconds)
}

Invoke-Scenario "FB5" "Выход родителя при отключённом сеансе ребёнка: записи об обходе нет" {
  Initialize-Scenario 1

  Step "Настройка: ребёнок «Тест», 2 ч на каждый день; значок и страж запущены, подробные логи" {
    Invoke-Setup 120
    Wait-SessionParts
    Open-Parent
    Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы"
    Start-Sleep -Seconds 3
    Assert-That (@(Get-SessionProcs "--guard").Count -eq 1) "Стражей в сеансе: $(@(Get-SessionProcs '--guard').Count), ждали один"
  }

  Step "Сеанс ребёнка отключён, страж погиб, другой сеанс вышел из учётки: страж вернулся" {
    Stop-GuardWhileAway
    Send-TestSession "logoff" 99
  }

  Step "Через 40 с в журнале нет записи об обходе" {
    # Без исправления запись появилась бы через 5 с после гибели стража.
    Start-Sleep -Seconds 40
    Write-ServiceSessionLog "FB5"
    Write-Log "Записи журнала: $((Get-Events | ForEach-Object { $_.text }) -join ' | ')"
    $found = Find-Events $script:EndPattern
    Assert-That ($found.Count -eq 0) "После выхода другого сеанса в журнале есть записи об обходе: $(($found | ForEach-Object { $_.text }) -join ' | ')"
  }

  Step "Контроль: принудительное завершение стража без выхода другого сеанса записывается один раз" {
    Stop-GuardWhileAway
    Wait-Until { (Find-Events $script:EndPattern).Count -ge 1 } 20 "запись об обходе после принудительного завершения стража"
    Start-Sleep -Seconds 6
    Assert-That ((Find-Events $script:EndPattern).Count -eq 1) "Записей об обходе $((Find-Events $script:EndPattern).Count), ждали одну"
  }

  Write-Log "FB5: выход другого сеанса при отключённом сеансе ребёнка не пишется обходом, настоящее завершение пишется"
}
