# S2. Лимит 20 минут: предупреждения при остатке 15, 5, 1 мин и экран блокировки (ТЗ 11, эталон G1).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): 20 минут лимита это 20 секунд настоящего времени.
# Ребёнок активен: ввод имитируется движением мыши раз в секунду (настоящий ввод сеанса, как от человека).
# Дано: лимит 20 мин. Когда: проходит время. Тогда: плашки 15, 5, 1 по одному разу по порядку (GuardState),
# на 20-й минуте экран блокировки не позже 2 с, «Сегодня: 20 мин из 20 мин».
# Снимки: s02-warn-15.png и s02-warn-1.png (часы на паузе TestClock, плашка видна), s02-lock.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Ждёт, пока остаток дня опустится до MaxS секунд (опрос раз в 40 мс: при ускорении 60 это около 2 с часов ядра).
function Wait-Remaining([int]$MaxS, [double]$Seconds, [string]$What) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($true) {
    $st = Get-Status
    if ($st.remainingS -le $MaxS) { return $st }
    if ($sw.Elapsed.TotalSeconds -gt $Seconds) { throw "За $Seconds с не дождались: $What (остаток $($st.remainingS) с, статус $($st.status))" }
    Start-Sleep -Milliseconds 40
  }
}

Invoke-Scenario "S2" "Лимит 20 минут: предупреждения 15, 5, 1 и экран блокировки" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: ребёнок «Тест», лимит 20 мин на каждый день; ребёнок активен" {
    Start-Child 20
    $st = Get-Status
    Assert-That ($st.limitS -eq 1200) "Лимит дня $($st.limitS) с, ждали 1200"
    Assert-That ($st.remainingS -ge 1100) "До начала испытания уже израсходовано $(1200 - $st.remainingS) с"
    Resume-Clock
  }

  Step "Остаток 15 мин: плашка «Через 15 минут...», снимок на паузе часов" {
    # Плашки в GuardState накопительные (сохраняются до экрана блокировки), поэтому ловим
    # плашку на ИДУЩИХ часах, а замораживаем уже после поимки — для чистого снимка.
    # Замораживать до того, как сработает тик на пороге, нельзя: на паузе часов тики не идут
    # (loop.go), и плашка не появится вовсе.
    [void](Wait-Remaining 900 60 "остаток 15 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 1 } 20 "плашка за 15 минут показана"
    Suspend-Clock
    $g = Get-GuardState
    Write-Log "Плашка поймана, часы остановлены: $(Format-GuardState $g)"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали только 15"
    Start-Sleep -Milliseconds 300
    Save-Screen "s02-warn-15"
    Resume-Clock
  }

  Step "Остаток 5 мин: плашка «Через 5 минут...»" {
    [void](Wait-Remaining 330 60 "остаток 5 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 2 } 15 "плашка за 5 минут показана"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15,5") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 15,5"
  }

  Step "Остаток 1 мин: плашка «Осталась 1 минута», снимок на паузе часов" {
    # Между плашкой за минуту и экраном блокировки одна настоящая секунда: останавливаем часы сразу.
    $st = Wait-Remaining 57 60 "остаток 1 мин"
    Suspend-Clock
    Write-Log "Часы остановлены при остатке $($st.remainingS) с"
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 3 } 15 "плашка за 1 минуту показана"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15,5,1") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 15,5,1"
    Start-Sleep -Milliseconds 600
    Save-Screen "s02-warn-1"
    Assert-That (-not $g.lockShown) "Экран блокировки показан раньше времени"
    Resume-Clock
  }

  Step "20-я минута: экран блокировки не позже 2 с после решения службы" {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $tStatus = $null
    $tGuard = $null
    while ($null -eq $tGuard) {
      $st = Get-Status
      if ($null -eq $tStatus -and $st.lock -eq "timeUp") { $tStatus = $sw.Elapsed.TotalSeconds }
      $g = Get-GuardState
      if ($g.lockShown) { $tGuard = $sw.Elapsed.TotalSeconds }
      if ($null -eq $tGuard -and $sw.Elapsed.TotalSeconds -gt 30) { throw "Экран блокировки не показан за 30 с (статус $($st.status), остаток $($st.remainingS) с)" }
      if ($null -eq $tGuard) { Start-Sleep -Milliseconds 50 }
    }
    if ($null -eq $tStatus) { $tStatus = $tGuard }
    $delay = $tGuard - $tStatus
    Write-Log ("Решение службы «время вышло» и экран: разница {0:N2} с" -f $delay)
    Assert-That ($delay -le 2) ("Экран блокировки показан через {0:N2} с после решения службы, ждали не больше 2" -f $delay)
    Assert-That ($g.lockKind -eq "timeUp") "Вид блокировки '$($g.lockKind)', ждали timeUp"
  }

  Step "Экран блокировки: тексты, кнопки, «Сегодня: 20 мин из 20 мин»" {
    Suspend-Clock
    $st = Get-Status
    Write-Log "Статус: $($st.status), причина «$($st.reason)», «$($st.until)», «$($st.used)», израсходовано $($st.usedS) с"
    Assert-That ($st.used -eq "Сегодня: 20 мин из 20 мин") "Строка «$($st.used)», ждали «Сегодня: 20 мин из 20 мин»"
    Assert-Between $st.usedS 1195 1210 "Израсходовано к моменту блокировки, с"
    Assert-That ($st.status -eq "timeUp") "Статус '$($st.status)', ждали timeUp"
    $g = Get-GuardState
    Assert-That ((Test-HasButton $g "parentPassword") -and (Test-HasButton $g "logoff")) "На экране нет двух кнопок: $(Format-GuardState $g)"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15,5,1") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 15,5,1 по одному разу"
    Start-Sleep -Seconds 1
    Save-Screen "s02-lock"
  }

  Write-Log "S2: плашки 15, 5, 1 по одному разу, экран блокировки в срок, «Сегодня: 20 мин из 20 мин»"
}
