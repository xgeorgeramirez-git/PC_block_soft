# S2. Лимит 40 минут: предупреждения при остатке 30, 15, 5, 1 мин и экран блокировки (ТЗ 11, эталон G1, D-54).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): 40 минут лимита это 40 секунд настоящего времени.
# Ребёнок активен: ввод имитируется движением мыши раз в секунду (настоящий ввод сеанса, как от человека).
# Дано: лимит 40 мин. Когда: проходит время. Тогда: плашки 30, 15, 5, 1 по одному разу по порядку (GuardState),
# на каждом пороге один звук (GuardState.sounds), на 40-й минуте экран блокировки не позже 2 с,
# «Сегодня: 40 мин из 40 мин».
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

Invoke-Scenario "S2" "Лимит 40 минут: предупреждения 30, 15, 5, 1 и экран блокировки" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: ребёнок «Тест», лимит 40 мин на каждый день; ребёнок активен" {
    Start-Child 40
    $st = Get-Status
    Assert-That ($st.limitS -eq 2400) "Лимит дня $($st.limitS) с, ждали 2400"
    Assert-That ($st.remainingS -ge 2300) "До начала испытания уже израсходовано $(2400 - $st.remainingS) с"
    Resume-Clock
  }

  Step "Остаток 30 мин: плашка-табло «Через 30 минут...» с крестиком" {
    [void](Wait-Remaining 1800 60 "остаток 30 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 1 } 20 "плашка за 30 минут показана"
    Write-Log "Плашка за 30 минут: $(Format-GuardState $g)"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали только 30"
  }

  Step "Остаток 15 мин: плашка «Через 15 минут...», снимок на паузе часов" {
    # Плашки в GuardState накопительные (сохраняются до экрана блокировки), поэтому ловим
    # плашку на ИДУЩИХ часах, а замораживаем уже после поимки — для чистого снимка.
    # Замораживать до того, как сработает тик на пороге, нельзя: на паузе часов тики не идут
    # (loop.go), и плашка не появится вовсе.
    [void](Wait-Remaining 900 60 "остаток 15 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 2 } 20 "плашка за 15 минут показана"
    Suspend-Clock
    $g = Get-GuardState
    Write-Log "Плашка поймана, часы остановлены: $(Format-GuardState $g)"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30,15") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 30,15"
    Start-Sleep -Milliseconds 300
    Save-Screen "s02-warn-15"
    Resume-Clock
  }

  Step "Остаток 5 мин: плашка «Через 5 минут...»" {
    [void](Wait-Remaining 330 60 "остаток 5 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 3 } 15 "плашка за 5 минут показана"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30,15,5") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 30,15,5"
  }

  Step "Остаток 1 мин: плашка «Осталась 1 минута», снимок на паузе часов" {
    # Между плашкой за минуту и экраном блокировки одна настоящая секунда: останавливаем часы сразу.
    $st = Wait-Remaining 57 60 "остаток 1 мин"
    Suspend-Clock
    Write-Log "Часы остановлены при остатке $($st.remainingS) с"
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 4 } 15 "плашка за 1 минуту показана"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30,15,5,1") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 30,15,5,1"
    Assert-That (-not (Test-HasButton $g "warningClose")) "У плашки за 1 минуту есть кнопка: $(Format-GuardState $g)"
    Start-Sleep -Milliseconds 600
    Save-Screen "s02-warn-1"
    Assert-That (-not $g.lockShown) "Экран блокировки показан раньше времени"
    Resume-Clock
  }

  Step "40-я минута: экран блокировки не позже 2 с после решения службы" {
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

  Step "Экран блокировки: тексты, кнопки, «Сегодня: 40 мин из 40 мин», звук на каждом пороге" {
    Suspend-Clock
    $st = Get-Status
    Write-Log "Статус: $($st.status), причина «$($st.reason)», «$($st.until)», «$($st.used)», израсходовано $($st.usedS) с"
    Assert-That ($st.used -eq "Сегодня: 40 мин из 40 мин") "Строка «$($st.used)», ждали «Сегодня: 40 мин из 40 мин»"
    Assert-Between $st.usedS 2395 2410 "Израсходовано к моменту блокировки, с"
    Assert-That ($st.status -eq "timeUp") "Статус '$($st.status)', ждали timeUp"
    $g = Get-GuardState
    Assert-That ((Test-HasButton $g "parentPassword") -and (Test-HasButton $g "logoff")) "На экране нет двух кнопок: $(Format-GuardState $g)"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30,15,5,1") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 30,15,5,1 по одному разу"
    Write-Log "Звуки: $(@(List $g.sounds | ForEach-Object { "$($_.threshold):$($_.ok)" }) -join ',')"
    Assert-That ((Get-SoundThresholds $g) -join "," -eq "30,15,5,1") "Звуки на порогах: $((Get-SoundThresholds $g) -join ','), ждали 30,15,5,1 по одному разу"
    Start-Sleep -Seconds 1
    Save-Screen "s02-lock"
  }

  Write-Log "S2: плашки и звуки 30, 15, 5, 1 по одному разу, экран блокировки в срок, «Сегодня: 40 мин из 40 мин»"
}
