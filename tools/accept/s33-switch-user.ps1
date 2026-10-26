# S33. Время не идёт, когда сеанс ребёнка не на экране: смена пользователя (ТЗ R2, D-52, фидбек-раунд 0.2.3).
# Сеанс отключается как при смене пользователя (tsdiscon): страж в нём не может открыть рабочий стол ввода и
# считает сеанс незаблокированным, поэтому служба сама знает состояние сеанса (WTS) и не верит стражу.
# Дано: ребёнок в сеансе, окно на весь экран без ввода (по R2б секунды идут). Когда: сеанс отключён.
# Тогда: в подробном журнале службы за 15 с: «событие сеанса N: отключение», «состояние WTS 4» и «снимок стража
# сеанса N не учитывается (экран: не на экране)». Пока сеанс отключён по журналу, секунды не растут.
# Образ раннера GitHub сам возвращает консоль в сеанс сразу после tsdiscon (около секунды): остановку на 8 с
# и возобновление после возврата здесь не измерить, это проверяют юнит-тесты службы (TestDisconnectedSession*,
# TestStaleListDoesNotOverrideDisconnectEvent и др.) и ручная приёмка на запасном ПК. Здесь проверена цепочка
# событий от Windows до решения службы. Блокировка (LockWorkStation) на раннере не вызывается: её не снять без пароля.
# Сценарий идёт последним.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Состояние сеанса по qwinsta: Active, Disc и так далее; пусто, если сеанса в списке нет.
function Get-SessionStateName([int]$Id) {
  foreach ($line in @(& qwinsta.exe 2>$null)) {
    if ($line -match "\s$Id\s+(\w+)") { return [string]$Matches[1] }
  }
  return ""
}

# Строки журнала службы не раньше $script:Since: время «2026/10/26 20:58:03 ...» и текст.
function Get-ServiceLines([string]$Pattern) {
  $logFile = Join-Path (Join-Path $script:Data "logs") "service.log"
  if (-not (Test-Path -LiteralPath $logFile)) { return @() }
  $out = @()
  foreach ($l in @(Get-Content -LiteralPath $logFile -Tail 800 -Encoding UTF8)) {
    if ($l -notmatch $Pattern) { continue }
    if ($l -match '^(\d{4}/\d\d/\d\d \d\d:\d\d:\d\d) (.*)$') {
      $t = [datetime]::ParseExact($Matches[1], "yyyy/MM/dd HH:mm:ss", $null)
      if ($t -ge $script:Since) { $out += [pscustomobject]@{ At = $t; Text = $Matches[2] } }
    }
  }
  return $out
}

Invoke-Scenario "S33" "Смена пользователя: время ребёнка не идёт, пока его сеанс отключён" {
  Initialize-Scenario 60

  Step "Ребёнок готов, окно на весь экран без ввода: секунды идут" {
    Test-InputSim
    Start-Child 120 $false
    # Подробные логи службы (FR-25): без них красный прогон не объясняет, откуда служба взяла «на экране или нет».
    Open-Parent
    Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы"
    Show-Fullscreen
    Resume-Clock
    $u0 = [int](Get-Status).usedS
    Wait-Until { [int](Get-Status).usedS -ge ($u0 + 60) } 20 "секунды ребёнка растут при окне на весь экран"
    Write-Log "Сеанс $($script:MySession): состояние «$(Get-SessionStateName $script:MySession)»; учтено $([int](Get-Status).usedS) с"
    Assert-That ((Get-SessionStateName $script:MySession) -eq "Active") "Сеанс раннера не в состоянии Active до испытания"
  }

  Step "Отключение сеанса (tsdiscon): в журнале службы событие, состояние WTS 4 и снимок не учитывается" {
    $sid = [int]$script:MySession
    $script:Before = [int](Get-Status).usedS
    Write-Log "qwinsta до отключения: $((@(& qwinsta.exe 2>&1)) -join ' / ')"
    $script:T0 = Get-Date
    $script:Since = $script:T0.AddSeconds(-1)
    & tsdiscon.exe $sid 2>&1 | ForEach-Object { Write-Log "tsdiscon: $_" }
    $pDisc = "событие сеанса ${sid}: отключение"
    $pWts = "сеанс ${sid}: состояние WTS 4"
    $pSkip = "снимок стража сеанса ${sid} не учитывается \(экран: не на экране"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $found = @{}
    while ($sw.Elapsed.TotalSeconds -lt 15) {
      foreach ($p in @($pDisc, $pWts, $pSkip)) {
        if (-not $found.ContainsKey($p) -and (Get-ServiceLines $p).Count -gt 0) { $found[$p] = $true }
      }
      if ($found.Count -eq 3) { break }
      Start-Sleep -Milliseconds 300
    }
    Write-ServiceSessionLog "S33"
    Assert-That ($found.ContainsKey($pDisc)) "За 15 с после tsdiscon в журнале службы нет строки «$pDisc»: событие отключения не дошло до службы с номером сеанса $sid"
    Assert-That ($found.ContainsKey($pWts)) "В журнале службы нет строки «$pWts»: список сеансов Windows не показал отключение сеанса"
    Assert-That ($found.ContainsKey($pSkip)) "В журнале службы нет строки «снимок стража сеанса $sid не учитывается (экран: не на экране)»: служба поверила стражу в отключённом сеансе"
    Write-Log "S33: цепочка отключения в журнале службы есть: событие, WTS 4, снимок не учитывается"
  }

  Step "Пока сеанс отключён по журналу, секунды не растут (если интервал не короче 2 с)" {
    $sid = [int]$script:MySession
    $disc = @(Get-ServiceLines "событие сеанса ${sid}: отключение") | Select-Object -First 1
    $conn = @(Get-ServiceLines "событие сеанса ${sid}: подключение") | Where-Object { $_.At -ge $disc.At } | Select-Object -First 1
    $state = Get-SessionStateName $sid
    if ($conn) {
      $gap = ($conn.At - $disc.At).TotalSeconds
      Write-Log ("Сеанс $sid был отключён по журналу службы {0:N0} с (отключение {1:HH:mm:ss}, подключение {2:HH:mm:ss}); сейчас «$state»" -f $gap, $disc.At, $conn.At)
      if ($gap -lt 2) {
        Write-Log "S33: интервал отключения короче 2 с (образ раннера сразу возвращает консоль): остановка счётчика здесь не измеряется, её доказывают юнит-тесты службы"
        return
      }
      # За время отключения (gap) счётчик не должен был расти: допустимый рост только за остальное время, 60 с на каждую настоящую.
      $total = ((Get-Date) - $script:T0).TotalSeconds
      $limit = [int](($total - $gap + 2) * 60)
      $grew = [int](Get-Status).usedS - $script:Before
      Write-Log "Учтено за $([int]$total) с после tsdiscon: $grew с, допустимо не больше $limit"
      Assert-That ($grew -le $limit) "За время после tsdiscon счётчик вырос на $grew с, допустимо не больше $limit (отключение $([int]$gap) с не должно считаться)"
    } else {
      Write-Log "Строки «подключение» нет: сеанс остался отключённым ($state); рост счётчика проверяется только как отсутствие роста"
      Start-Sleep -Seconds 8
      $after = [int](Get-Status).usedS
      $grew = $after - $script:Before
      Assert-That ($grew -le 240) "В отключённом сеансе секунды растут: за время отключения +$grew с (допустимо не больше 240)"
    }
  }

  Step "Возврат сеанса на экран: если консоль уже вернулась сама, это ожидаемо на раннере" {
    $sid = [int]$script:MySession
    if ((Get-SessionStateName $sid) -eq "Active") {
      Write-Log "S33: сеанс $sid уже Active без tscon: образ раннера сам возвращает консоль после tsdiscon (ожидаемо, не ошибка)"
      return
    }
    $task = "PcbS33Tscon"
    $cmd = "tscon.exe $sid /dest:console"
    & schtasks.exe /Create /TN $task /TR $cmd /SC ONCE /ST 23:59 /RU SYSTEM /RL HIGHEST /F 2>&1 | ForEach-Object { Write-Log "schtasks: $_" }
    & schtasks.exe /Run /TN $task 2>&1 | ForEach-Object { Write-Log "schtasks: $_" }
    $back = $false
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt 30) {
      if ((Get-SessionStateName $sid) -eq "Active") { $back = $true; break }
      Start-Sleep -Milliseconds 500
    }
    & schtasks.exe /Delete /TN $task /F 2>&1 | ForEach-Object { Write-Log "schtasks: $_" }
    if (-not $back) {
      $state = Get-SessionStateName $sid
      throw "Сеанс $sid не вернулся на экран за 30 с после tscon (состояние «$state»): шаг не засчитан"
    }
    Write-Log ("Сеанс снова Active через {0:N1} с после tscon" -f $sw.Elapsed.TotalSeconds)
  }

  $lock = @(Get-ServiceLines "событие сеанса $([int]$script:MySession): блокировка")
  Write-Log "S33: строк «блокировка» в журнале службы за время сценария: $($lock.Count) (мягкая отметка, LockWorkStation не вызывался)"
  Write-Log "S33: цепочка событий отключения дошла до решения службы"
}
