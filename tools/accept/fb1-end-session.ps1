# FB1. Конец сеанса: страж закрывается штатно, это не обход (ТЗ R17в, FR-27; фидбек-раунд 0.2.3).
# Windows при выключении, перезагрузке и выходе из учётки закрывает стража раньше, чем служба узнаёт о конце сеанса.
# Выключить компьютер в CI нельзя, поэтому страж проходит свой путь WM_ENDSESSION по тестовой команде endSession:
# сообщает службе GuardClosing и завершается.
# Дано: ребёнок в сеансе, страж запущен. Когда: страж получает «конец сеанса».
# Тогда: старый страж завершился; новый запущен заново; через 6 с после этого в журнале родителя нет записи об обходе.
# Контроль: сеанс при этом НЕ закончился (как отменённое выключение), поэтому страж, закрывшийся с GuardClosing,
# и принудительное завершение сразу после него дают ровно одну запись об обходе, не позже closingLife (30 с) плюс запас;
# перезапуск стража при этом не откладывается (FR-27, не позже 3 с).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$script:EndPattern = 'обойти защиту|Защита программы была остановлена|закрыто не кнопкой окна'

Invoke-Scenario "FB1" "Конец сеанса: страж закрылся штатно, записи об обходе нет" {
  Initialize-Scenario 1

  Step "Настройка: ребёнок «Тест», 2 ч на каждый день; значок и страж запущены" {
    Invoke-Setup 120
    Wait-SessionParts
    Start-Sleep -Seconds 3
    $g = Get-GuardState
    Assert-That (-not $g.lockShown) "Экран блокировки показан до испытания: $(Format-GuardState $g)"
    Assert-That (@(Get-SessionProcs "--guard").Count -eq 1) "Стражей в сеансе: $(@(Get-SessionProcs '--guard').Count), ждали один"
  }

  Step "Тестовый конец сеанса: старый страж завершился, новый запущен заново" {
    $old = @(Get-SessionProcs "--guard")
    $script:OldGuard = [int]$old[0].ProcessId
    Invoke-TestGuard "endSession"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Wait-Until { -not (Get-Process -Id $script:OldGuard -ErrorAction SilentlyContinue) } 10 "старый страж завершился"
    Write-Log ("Старый страж {0} завершился через {1:N1} с" -f $script:OldGuard, $sw.Elapsed.TotalSeconds)
    Wait-Until {
      $now = @(Get-SessionProcs "--guard")
      ($now.Count -eq 1) -and ([int]$now[0].ProcessId -ne $script:OldGuard)
    } 40 "страж запущен заново"
    Write-Log ("Новый страж через {0:N1} с после завершения старого" -f $sw.Elapsed.TotalSeconds)
  }

  Step "Через 6 с после перезапуска в журнале нет записи об обходе" {
    # Без сообщения GuardClosing запись появилась бы через 5 с после гибели стража, то есть раньше.
    Start-Sleep -Seconds 6
    Open-Parent
    Write-Log "Записи журнала: $((Get-Events | ForEach-Object { $_.text }) -join ' | ')"
    $found = Find-Events $script:EndPattern
    Assert-That ($found.Count -eq 0) "После конца сеанса в журнале есть записи об обходе: $(($found | ForEach-Object { $_.text }) -join ' | ')"
    $g = Get-GuardState
    Assert-That (-not $g.lockShown) "После конца сеанса показан экран блокировки (обход только пишется в журнал, D-41): $(Format-GuardState $g)"
  }

  Step "Контроль: принудительное завершение стража записывается" {
    $cur = @(Get-SessionProcs "--guard")
    Assert-That ($cur.Count -eq 1) "Стражей в сеансе: $($cur.Count), ждали один"
    $killPid = [int]$cur[0].ProcessId
    & taskkill.exe /F /PID $killPid 2>&1 | ForEach-Object { Write-Log "taskkill: $_" }
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Wait-Until {
      $now = @(Get-SessionProcs "--guard")
      ($now.Count -eq 1) -and ([int]$now[0].ProcessId -ne $killPid)
    } 15 "страж перезапущен сразу после принудительного завершения"
    Write-Log ("Страж перезапущен через {0:N1} с" -f $sw.Elapsed.TotalSeconds)
    Assert-That ($sw.Elapsed.TotalSeconds -le 4) ("Страж перезапущен через {0:N1} с, ждали не больше 3 (FR-27)" -f $sw.Elapsed.TotalSeconds)
    # Решение об обходе после GuardClosing ждёт до 30 с с первой гибели (страж закрылся, а сеанс жив), поэтому до 45 с от kill.
    Wait-Until { (Find-Events $script:EndPattern).Count -ge 1 } 45 "запись об обходе после закрытия и принудительного завершения стража"
    Write-Log ("Запись об обходе появилась через {0:N1} с после завершения" -f $sw.Elapsed.TotalSeconds)
    Assert-That ((Find-Events $script:EndPattern).Count -eq 1) "Записей об обходе $((Find-Events $script:EndPattern).Count), ждали одну"
  }

  Write-Log "FB1: конец сеанса не пишется как обход, настоящее завершение стража пишется один раз"
}
