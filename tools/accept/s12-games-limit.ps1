# S12. Лимит игр (R5, R9, FR-13, FR-16, D-54, D-55, NFR-6, RM3-8).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): минута ядра это секунда настоящего времени; сроки
# плашек идут по настоящим часам. Тестовые программы — копии системной программы в папке WatchDir
# (New-TestApp): game.exe отмечена «Игра», notes.exe «Прочее» шагом мастера (Setup.programs) до запуска
# (PL-11). Ввод имитируется движением мыши.
# Дано: общий лимит 2 ч, игры 30 мин. Когда: 30 мин игры на переднем плане. Тогда: предупреждения игр
# 15, 5, 1 (порог 30 при лимите ровно 30 мин не взводится, PL-14), звук на каждом; в 0 игр game.exe
# закрыта не позже 1,5 с, плашка «Время на игры на сегодня закончилось», экрана блокировки нет,
# notes.exe работает; общий остаток 1 ч 30 мин (±1 мин часов ядра); повторный запуск game.exe закрыт.
# Снимок: s12-games-over.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы и ядра (internal/ipc/status.go NewGuardWarningFor, internal/core/texts.go).
$script:Games15 = "Через 15 минут время на игры закончится"
$script:Games5 = "Сохрани игру: через 5 минут время на игры закончится"
$script:Games1 = "Осталась 1 минута на игры"
$script:GamesOver = "Время на игры на сегодня закончилось"

Invoke-Scenario "S12" "Игры 30 мин из 2 ч: предупреждения, игра закрыта, прочее работает" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовые программы game.exe и notes.exe в папке наблюдения" {
    $script:GamePath = New-TestApp "game"
    $script:NotesPath = New-TestApp "notes"
  }

  Step "Настройка: общий лимит 2 ч, игры 30 мин; game.exe «Игра», notes.exe «Прочее»" {
    $progs = @(@{ path = $script:GamePath; category = "games" }, @{ path = $script:NotesPath; category = "other" })
    Start-Child 120 $true 30 $progs
    Wait-ProgramsReady
    $st = Get-Status
    Assert-That ($st.limitS -eq 7200) "Лимит дня $($st.limitS) с, ждали 7200"
    Assert-That ((-not $st.gamesUnlimited) -and $st.gamesLimitS -eq 1800 -and $st.gamesRemainingS -eq 1800) "Лимит игр: $($st.gamesRemainingS) с из $($st.gamesLimitS), без лимита=$($st.gamesUnlimited); ждали 1800 из 1800"
    Assert-That (-not $st.closeGames) "Игры закрыты с начала дня (closeGames)"
  }

  Step "Запуск notes.exe и game.exe: обе работают, game.exe на переднем плане" {
    $script:Notes = Start-TestApp $script:NotesPath
    $script:Game = Start-TestApp $script:GamePath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Notes "notes.exe («Прочее»)"
    Assert-AppAlive $script:Game "game.exe («Игра», время на игры есть)"
    Set-TestForeground $script:Game
    Resume-Clock
  }

  Step "Предупреждения игр 15, 5, 1 (30 не взводится при лимите ровно 30 мин), звук на каждом" {
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s) -contains 1 } 60 "плашка «Осталась 1 минута на игры»"
    Write-Log "Показанные плашки: $(Format-Warnings $g)"
    $thr = (Get-WarnThresholds $g) -join ","
    Assert-That ($thr -eq "15,5,1") "Показанные плашки: $thr, ждали 15,5,1 (только игры; порог 30 не взводится, PL-14)"
    $w = @(List $g.warnings)
    Assert-That ($w[0].text -eq $script:Games15) "Текст за 15 мин: «$($w[0].text)», ждали «$($script:Games15)»"
    Assert-That ($w[1].text -eq $script:Games5) "Текст за 5 мин: «$($w[1].text)», ждали «$($script:Games5)»"
    Assert-That (([string]$w[2].text).StartsWith($script:Games1)) "Текст за 1 мин: «$($w[2].text)», ждали «$($script:Games1)…»"
    Assert-That (-not $g.lockShown) "Экран блокировки показан до конца игр: $(Format-GuardState $g)"
    Write-Log "Звуки: $(@(List $g.sounds | ForEach-Object { "$($_.threshold):$($_.ok)" }) -join ',')"
    Assert-That ((Get-SoundThresholds $g) -join "," -eq "15,5,1") "Звуки на порогах: $((Get-SoundThresholds $g) -join ','), ждали 15,5,1 по одному разу"
  }

  Step "Игры кончились: game.exe закрыта, плашка, экрана нет, notes.exe работает, снимок" {
    $st = Wait-Status { param($st) $st.closeGames } 90 "игры закрыты (closeGames)"
    $since = [Diagnostics.Stopwatch]::StartNew()
    Suspend-Clock
    Write-Log "Игры закрыты: остаток игр $($st.gamesRemainingS) с, общий $($st.remainingS) с; часы остановлены"
    [void](Assert-ClosedFast $script:Game "game.exe в 0 игр" $since)
    $g = Wait-GuardState { param($s) [string]$s.plate -eq $script:GamesOver } 5 "плашка «$($script:GamesOver)»"
    Assert-That ($g.plateKind -eq "gamesOver") "Вид плашки «$($g.plateKind)», ждали gamesOver"
    Assert-That (-not $g.lockShown) "Экран блокировки показан, ждали только плашку: $(Format-GuardState $g)"
    Assert-That (@(Get-BigPcbWindows).Count -eq 0) "Окно PC Block на весь экран: $((Get-BigPcbWindows) -join ' ; ')"
    Assert-AppAlive $script:Notes "notes.exe («Прочее») после конца игр"
    Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
    Save-Screen "s12-games-over"
  }

  Step "Общий остаток 1 ч 30 мин (±1 мин часов ядра), остаток игр 0" {
    $st = Get-Status
    Assert-Between $st.remainingS 5340 5460 "Общий остаток после 30 мин игры, с"
    Assert-That ($st.gamesRemainingS -eq 0) "Остаток игр $($st.gamesRemainingS) с, ждали 0"
    Assert-That ($st.lock -eq "none") "Блокировка «$($st.lock)» (статус $($st.status)): ждали без блокировки"
    $m = Get-MyToday
    Assert-Between $m.gamesS 1740 1800 "Игры за сегодня (MyToday.gamesS), с"
  }

  Step "Повторный запуск game.exe закрыт; notes.exe работает" {
    $again = Start-TestApp $script:GamePath
    [void](Assert-ClosedFast $again "повторный запуск game.exe")
    Start-Sleep -Milliseconds 500
    Assert-AppAlive $script:Notes "notes.exe («Прочее») после повторного запуска игры"
    $g = Get-GuardState
    Assert-That (-not $g.lockShown) "Экран блокировки после повторного запуска игры: $(Format-GuardState $g)"
  }

  Step "Журнал родителя: «время на игры закончилось», без «попытка обойти защиту»" {
    # Имя в журнале — из свойств файла (копия Блокнота), поэтому образец строится по списку программ.
    $gameName = Get-ProgramName $script:GamePath
    Use-Parent {
      $want = "закрыта «$gameName»: время на игры закончилось"
      $closed = @(Find-Events ([regex]::Escape($want)))
      $closed | ForEach-Object { Write-Log "Журнал: $($_.text)" }
      Assert-That ($closed.Count -ge 1) "В журнале нет записи «$want». Журнал: $(Format-Events)"
      $bypass = @(Find-Events 'попытка обойти защиту')
      Assert-That ($bypass.Count -eq 0) "В журнале запись об обходе: $(($bypass | ForEach-Object { $_.text }) -join ' | ')"
    }
  }

  Write-Log "S12: игры 15, 5, 1 со звуком, в 0 игра закрыта, плашка без экрана, «Прочее» работает, общий остаток 1 ч 30 мин"
}
