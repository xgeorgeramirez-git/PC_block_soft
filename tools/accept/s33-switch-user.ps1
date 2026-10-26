# S33. Время не идёт, когда сеанс ребёнка не на экране: смена пользователя (ТЗ R2, D-52, фидбек-раунд 0.2.3).
# Сеанс отключается как при смене пользователя (tsdiscon): страж в нём не может открыть рабочий стол ввода и
# считает сеанс незаблокированным, поэтому служба сама знает состояние сеанса (WTS) и не верит стражу.
# Дано: ребёнок в сеансе, окно на весь экран без ввода (по R2б секунды идут). Когда: сеанс отключён.
# Тогда: секунды перестают расти не позже 2 с; за следующие 8 с не растут совсем.
# Возврат (tscon от имени системы, как это делает пользователь, вернувшийся на экран): секунды снова растут.
# Блокировка (Win+L) стражем распознаётся и раньше; события блокировки и разблокировки проверены юнит-тестами службы.
# Сценарий идёт последним: после отключения сеанса у раннера нет экрана, если вернуть его не удалось.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Состояние сеанса по qwinsta: Active, Disc и так далее; пусто, если сеанса в списке нет.
function Get-SessionStateName([int]$Id) {
  foreach ($line in @(& qwinsta.exe 2>$null)) {
    if ($line -match "\s$Id\s+(\w+)") { return [string]$Matches[1] }
  }
  return ""
}

Invoke-Scenario "S33" "Смена пользователя: время ребёнка не идёт, пока его сеанс отключён" {
  Initialize-Scenario 60

  Step "Ребёнок готов, окно на весь экран без ввода: секунды идут" {
    Test-InputSim
    # Подробные логи службы (FR-25): без них красный прогон не объясняет, откуда служба взяла «на экране или нет».
    Open-Parent
    Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы"
    Start-Child 120 $false
    Show-Fullscreen
    Resume-Clock
    $u0 = [int](Get-Status).usedS
    Wait-Until { [int](Get-Status).usedS -ge ($u0 + 60) } 20 "секунды ребёнка растут при окне на весь экран"
    Write-Log "Сеанс $($script:MySession): состояние «$(Get-SessionStateName $script:MySession)»; учтено $([int](Get-Status).usedS) с"
    Assert-That ((Get-SessionStateName $script:MySession) -eq "Active") "Сеанс раннера не в состоянии Active до испытания"
  }

  Step "Отключение сеанса (tsdiscon): секунды перестают расти не позже 2 с" {
    $script:Before = [int](Get-Status).usedS
    Write-Log "qwinsta до отключения: $((@(& qwinsta.exe 2>&1)) -join ' / ')"
    & tsdiscon.exe $script:MySession 2>&1 | ForEach-Object { Write-Log "tsdiscon: $_" }
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Wait-Until { (Get-SessionStateName $script:MySession) -match "^Disc" } 15 "сеанс отключён (qwinsta)"
    Write-Log ("Сеанс отключён через {0:N1} с после команды" -f $sw.Elapsed.TotalSeconds)
    # До 2 с по ТЗ плюс период опроса; ускорение часов 60: за 3 с настоящего времени набегает не больше 180 с.
    while ($sw.Elapsed.TotalSeconds -lt 3) { Start-Sleep -Milliseconds 100 }
    $script:Mid = [int](Get-Status).usedS
    Write-Log "Учтено до отключения $($script:Before) с, через 3 с после отключения $($script:Mid) с"
    Assert-That (($script:Mid - $script:Before) -le 240) "После отключения набежало $($script:Mid - $script:Before) с, ждали не больше 240 (2 с настоящего времени с запасом)"
    Start-Sleep -Seconds 8
    $after = [int](Get-Status).usedS
    Write-Log "Через 11 с после отключения учтено $after с"
    Assert-That ($after -eq $script:Mid) "В отключённом сеансе секунды растут: $($script:Mid) -> $after за 8 с (ждали ни секунды)"
  }

  Step "Возврат сеанса на экран (tscon от имени системы): секунды снова растут" {
    $task = "PcbS33Tscon"
    $cmd = "tscon.exe $($script:MySession) /dest:console"
    & schtasks.exe /Create /TN $task /TR $cmd /SC ONCE /ST 23:59 /RU SYSTEM /RL HIGHEST /F 2>&1 | ForEach-Object { Write-Log "schtasks: $_" }
    & schtasks.exe /Run /TN $task 2>&1 | ForEach-Object { Write-Log "schtasks: $_" }
    $back = $false
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt 30) {
      if ((Get-SessionStateName $script:MySession) -eq "Active") { $back = $true; break }
      Start-Sleep -Milliseconds 500
    }
    & schtasks.exe /Delete /TN $task /F 2>&1 | ForEach-Object { Write-Log "schtasks: $_" }
    if (-not $back) {
      $state = Get-SessionStateName $script:MySession
      Write-Log "Сеанс вернуть не удалось (состояние «$state»)"
      throw "Сеанс $($script:MySession) не вернулся на экран за 30 с после tscon (состояние «$state»): проверка возврата времени не выполнена, шаг не засчитан"
    }
    Write-Log ("Сеанс снова Active через {0:N1} с" -f $sw.Elapsed.TotalSeconds)
    $u1 = [int](Get-Status).usedS
    Wait-Until { [int](Get-Status).usedS -ge ($u1 + 60) } 30 "секунды ребёнка снова растут после возврата сеанса"
    Write-Log "После возврата учтено $([int](Get-Status).usedS) с"
  }

  Write-Log "S33: в отключённом сеансе секунды стоят, после возврата идут"
}
