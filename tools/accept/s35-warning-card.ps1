# S35. Плашка-табло предупреждения и звук заказчика (FR-9, ТЗ 8.2.2, D-54, D-55).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): минута ядра это секунда настоящего времени; сроки
# плашек (5 с у табло) идут по настоящим часам. Ребёнок в окне на весь экран (Show-Fullscreen), ввод
# имитируется движением мыши.
# Дано: лимит 40 мин, окно на весь экран. Когда: остаток 30, 15, 5, 1 мин. Тогда: за 30 мин плашка-табло
# (окно карточки PCBlockTrayCard в процессе стража, крестик warningClose в GuardState), фокус остаётся у
# окна на весь экран; через 6 с табло нет; за 15 мин крестик закрывает табло сразу; за 1 мин плашка
# (окно PCBlockGuardPlate) видна и через 20 с, без кнопок; звук по одному на каждый порог (GuardState.sounds);
# «+30» у плашки 1 мин: её сменяет «Добавлено 30 минут» (снимок), повторного «за 30» нет, каждый порог в
# журнале показанных предупреждений по одному разу.
# Снимки: s35-warning-30.png, s35-warning-1.png, s35-added.png.
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

# Видимые окна стража этого сеанса заданного класса (PCBlockTrayCard — табло, PCBlockGuardPlate — плашка 1 мин).
function Get-GuardWindows([string]$Class) {
  $pids = @(Get-SessionProcs "--guard" | ForEach-Object { [uint32]$_.ProcessId })
  if ($pids.Count -eq 0) { return @() }
  return @([PcbNative]::VisibleWindows($pids) | Where-Object { $_ -match ('\|' + [regex]::Escape($Class) + '\|') })
}

# Процесс окна на переднем плане.
function Get-ForegroundPid { $fg = [PcbNative]::Foreground(); if ($fg) { return $fg.Split("|")[0] } else { return "" } }

Invoke-Scenario "S35" "Плашка-табло 30, 15, 5, плашка 1 мин и звук на каждом пороге" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: лимит 40 мин, ребёнок в окне на весь экран" {
    Start-Child 40
    # Номер ребёнка заранее: родительская сессия в сеансе ребёнка прячет плашки (Ruling 19), поэтому
    # позже она открывается только на «+30» и сразу закрывается.
    Open-Parent
    $script:ChildId = Get-ChildId
    Assert-Reply (Send-Parent "Logout") "Logout"
    Show-Fullscreen
    $script:FullPid = Get-ForegroundPid
    $st = Get-Status
    Assert-That ($st.limitS -eq 2400) "Лимит дня $($st.limitS) с, ждали 2400"
    Write-Log "Окно на весь экран (pid $($script:FullPid)), остаток $($st.remainingS) с"
    Resume-Clock
  }

  Step "Остаток 30 мин: табло с крестиком, фокус у окна на весь экран, снимок" {
    [void](Wait-Remaining 1800 60 "остаток 30 мин")
    $g = Wait-GuardState { param($s) Test-HasButton $s "warningClose" } 10 "табло за 30 минут с крестиком"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 30"
    $w = @(List $g.warnings)[0]
    Assert-That ($w.text -eq "Через 30 минут время на сегодня закончится") "Текст табло: «$($w.text)»"
    Wait-Until { (Get-GuardWindows "PCBlockTrayCard").Count -eq 1 } 3 "окно табло в процессе стража видно"
    Write-Log "Табло: $((Get-GuardWindows 'PCBlockTrayCard') -join ' ; '); передний план: $([PcbNative]::Foreground())"
    Assert-That ((Get-ForegroundPid) -eq $script:FullPid) "Фокус ушёл от окна на весь экран: $([PcbNative]::Foreground())"
    Save-Screen "s35-warning-30"
  }

  Step "Через 6 с табло нет (закрылось само через 5 с)" {
    Start-Sleep -Seconds 6
    $g = Get-GuardState
    Assert-That (-not (Test-HasButton $g "warningClose")) "Через 6 с табло ещё видно: $(Format-GuardState $g)"
    Assert-That ((Get-GuardWindows "PCBlockTrayCard").Count -eq 0) "Окно табло видно через 6 с"
    Assert-That ((Get-ForegroundPid) -eq $script:FullPid) "Фокус ушёл от окна на весь экран: $([PcbNative]::Foreground())"
  }

  Step "Остаток 15 мин: табло, крестик закрывает сразу, фокус не уходит" {
    [void](Wait-Remaining 900 60 "остаток 15 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 2 -and (Test-HasButton $s "warningClose") } 10 "табло за 15 минут"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30,15") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 30,15"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Invoke-GuardClick "warningClose"
    $g = Wait-GuardState { param($s) -not (Test-HasButton $s "warningClose") } 3 "крестик закрыл табло"
    Write-Log ("Крестик закрыл табло за {0:N1} с" -f $sw.Elapsed.TotalSeconds)
    Assert-That ((Get-GuardWindows "PCBlockTrayCard").Count -eq 0) "Окно табло видно после крестика"
    Assert-That ((Get-ForegroundPid) -eq $script:FullPid) "Щелчок по крестику забрал фокус: $([PcbNative]::Foreground())"
  }

  Step "Остаток 5 мин: табло «Через 5 минут время закончится. Сохрани игру»" {
    [void](Wait-Remaining 300 60 "остаток 5 мин")
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 3 } 10 "табло за 5 минут"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30,15,5") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 30,15,5"
    Assert-That (@(List $g.warnings)[2].text -eq "Через 5 минут время закончится. Сохрани игру") "Текст за 5 мин: «$(@(List $g.warnings)[2].text)»"
  }

  Step "Остаток 1 мин: плашка без кнопок видна и через 20 с, снимок" {
    $st = Wait-Remaining 57 60 "остаток 1 мин"
    Suspend-Clock
    Write-Log "Часы остановлены при остатке $($st.remainingS) с"
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 4 } 10 "плашка за 1 минуту"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30,15,5,1") "Показанные плашки: $((Get-WarnThresholds $g) -join ','), ждали 30,15,5,1"
    Assert-That (@(List $g.buttons).Count -eq 0) "У плашки 1 мин есть кнопки: $(Format-GuardState $g)"
    Start-Sleep -Seconds 20
    $g = Get-GuardState
    Assert-That (-not $g.lockShown) "Экран блокировки показан при остановленных часах"
    Assert-That ((Get-GuardWindows "PCBlockGuardPlate").Count -eq 1) "Через 20 с плашки 1 мин нет: $((Get-GuardWindows 'PCBlockGuardPlate') -join ' ; ')"
    Assert-That ((Get-ForegroundPid) -eq $script:FullPid) "Фокус ушёл от окна на весь экран: $([PcbNative]::Foreground())"
    Save-Screen "s35-warning-1"
  }

  Step "Звук заказчика: по одному на каждый показанный порог" {
    $g = Get-GuardState
    Write-Log "Звуки: $(@(List $g.sounds | ForEach-Object { "$($_.threshold):$($_.ok)" }) -join ',')"
    Assert-That ((Get-SoundThresholds $g) -join "," -eq "30,15,5,1") "Звуки на порогах: $((Get-SoundThresholds $g) -join ','), ждали 30,15,5,1 по одному разу"
  }

  Step "«+30» у плашки 1 мин: «Добавлено 30 минут», повторного «за 30» нет" {
    # Родительская сессия в сеансе ребёнка прячет плашки (Ruling 19): вход, «+30» и выход подряд, между
    # опросами стража (раз в секунду); сообщение «Добавлено» страж получает следующим опросом.
    Open-Parent
    Assert-Reply (Send-Parent "AddExtra" @{ childId = $script:ChildId; minutes = 30 }) "AddExtra +30"
    Assert-Reply (Send-Parent "Logout") "Logout"
    Wait-Until { (Get-GuardWindows "PCBlockGuardPlate").Count -eq 1 } 5 "плашка «Добавлено 30 минут» видна"
    Start-Sleep -Milliseconds 300
    Save-Screen "s35-added"
    $st = Get-Status
    Write-Log "После «+30»: остаток $($st.remainingS) с"
    Assert-That ($st.remainingS -ge 1800) "Остаток $($st.remainingS) с, ждали не меньше 1800"
    Start-Sleep -Seconds 6
    $g = Get-GuardState
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "30,15,5,1") "После «+30» журнал предупреждений: $((Get-WarnThresholds $g) -join ','), ждали 30,15,5,1 без повтора"
    Assert-That ((Get-SoundThresholds $g) -join "," -eq "30,15,5,1") "После «+30» звуки: $((Get-SoundThresholds $g) -join ','), ждали без нового звука"
    Assert-That ((Get-GuardWindows "PCBlockGuardPlate").Count -eq 0) "«Добавлено 30 минут» висит дольше 5 с"
    Stop-Fullscreen
  }

  Write-Log "S35: табло 30, 15, 5 без перехвата фокуса, закрытие через 5 с и крестиком, плашка 1 мин держится, звук на каждом пороге"
}
