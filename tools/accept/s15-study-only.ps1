# S15. Режим «Оставить только учёбу» (R8, FR-18, FR-13, NFR-6, RM3-2, RM3-8, D-9).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): лимит 3 мин кончается за 3 с настоящего времени.
# game.exe «Игра», notes.exe «Прочее», study.exe «Учёба» — копии системной программы в папке WatchDir,
# категории шагом мастера (Setup.programs), режим окончания — SaveLimits родителя; всё до запуска (PL-11).
# На переднем плане notes.exe: учёба в общее время не идёт, поэтому время тратит «Прочее».
# Дано: режим «Оставить только учёбу», лимит 3 мин. Когда: время вышло. Тогда: game.exe и notes.exe
# закрыты не позже 1,5 с, study.exe работает, экрана блокировки нет, плашка «Время вышло. Сейчас доступна
# только учёба»; повторный запуск game.exe закрыт; в журнале родителя «время вышло, доступна только
# учёба», нет «попытка обойти защиту» и «попытка открыть запрещённое».
# Браузер с неучебным сайтом — M4 (в M3 браузер «Прочее», RM3-2): в тестовом режиме наблюдатель трогает
# только папку WatchDir, браузер раннера вне её, поэтому шаг браузера записывается в журнал и пропускается.
# Снимок: s15-study-only.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Текст ядра (internal/core/texts.go StudyOnlyPlateText).
$script:StudyOnlyText = "Время вышло. Сейчас доступна только учёба"

Invoke-Scenario "S15" "«Оставить только учёбу»: время вышло, закрыто всё, кроме учёбы, экрана нет" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовые программы game.exe, notes.exe, study.exe в папке наблюдения" {
    $script:GamePath = New-TestApp "game"
    $script:NotesPath = New-TestApp "notes"
    $script:StudyPath = New-TestApp "study"
  }

  Step "Настройка: лимит 3 мин, «Оставить только учёбу»; категории игра, прочее, учёба" {
    $progs = @(
      @{ path = $script:GamePath; category = "games" },
      @{ path = $script:NotesPath; category = "other" },
      @{ path = $script:StudyPath; category = "study" }
    )
    Start-Child 3 $true -1 $progs
    Wait-ProgramsReady
    $r = Use-Parent { Send-Parent "SaveLimits" @{ childId = (Get-ChildId); limits = @(); endMode = "study_only" } }
    Assert-Reply $r "SaveLimits endMode=study_only"
    Write-Log "SaveLimits: «$($r.payload.message)», режим $($r.payload.rules.endMode)"
    $st = Get-Status
    Assert-That ($st.endMode -eq "study_only") "Режим окончания «$($st.endMode)», ждали study_only"
    Assert-That ($st.limitS -eq 180 -and -not $st.studyOnly) "Лимит $($st.limitS) с, studyOnly=$($st.studyOnly); ждали 180 с и режим ещё не действует"
  }

  Step "Запуск game, notes, study: все работают, на переднем плане notes.exe" {
    $script:Game = Start-TestApp $script:GamePath
    $script:Notes = Start-TestApp $script:NotesPath
    $script:Study = Start-TestApp $script:StudyPath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Game "game.exe («Игра») до конца времени"
    Assert-AppAlive $script:Notes "notes.exe («Прочее») до конца времени"
    Assert-AppAlive $script:Study "study.exe («Учёба») до конца времени"
    Set-TestForeground $script:Notes
    Resume-Clock
  }

  Step "Время вышло: game.exe и notes.exe закрыты, study.exe работает, экрана нет, снимок" {
    $st = Wait-Status { param($st) $st.studyOnly } 30 "режим «только учёба» (studyOnly)"
    $since = [Diagnostics.Stopwatch]::StartNew()
    Suspend-Clock
    Write-Log "Время вышло: остаток $($st.remainingS) с, блокировка $($st.lock); часы остановлены"
    [void](Assert-ClosedFast $script:Game "game.exe в режиме «только учёба»" $since)
    [void](Assert-ClosedFast $script:Notes "notes.exe в режиме «только учёба»" $since)
    Assert-AppAlive $script:Study "study.exe («Учёба») после конца времени"
    $g = Wait-GuardState { param($s) [string]$s.plate -eq $script:StudyOnlyText } 5 "плашка «$($script:StudyOnlyText)»"
    Assert-That ($g.plateKind -eq "studyOnly") "Вид плашки «$($g.plateKind)», ждали studyOnly"
    Assert-That (-not $g.lockShown) "Экран блокировки в режиме «только учёба»: $(Format-GuardState $g)"
    Assert-That (@(Get-BigPcbWindows).Count -eq 0) "Окно PC Block на весь экран: $((Get-BigPcbWindows) -join ' ; ')"
    $st = Get-Status
    Assert-That ($st.lock -eq "none") "Блокировка «$($st.lock)», ждали none (экрана нет, R8)"
    Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
    Save-Screen "s15-study-only"
  }

  Step "Повторный запуск game.exe закрыт; study.exe работает" {
    $again = Start-TestApp $script:GamePath
    [void](Assert-ClosedFast $again "повторный запуск game.exe")
    Assert-AppAlive $script:Study "study.exe после повторного запуска игры"
  }

  Step "Учёба после конца времени: study.exe на переднем плане, экрана нет" {
    Set-TestForeground $script:Study
    Resume-Clock
    Start-Sleep -Seconds 3
    Suspend-Clock
    Assert-AppAlive $script:Study "study.exe через 3 мин часов ядра в режиме «только учёба»"
    $g = Get-GuardState
    Assert-That (-not $g.lockShown) "Экран блокировки, пока идёт учёба: $(Format-GuardState $g)"
  }

  Step "Браузер (RM3-2): в M3 «Прочее», в тестовом режиме вне папки наблюдения" {
    $browsers = @(
      (Join-Path ${env:ProgramFiles(x86)} "Microsoft\Edge\Application\msedge.exe"),
      (Join-Path $env:ProgramFiles "Microsoft\Edge\Application\msedge.exe"),
      (Join-Path $env:ProgramFiles "BraveSoftware\Brave-Browser\Application\brave.exe")
    ) | Where-Object { Test-Path -LiteralPath $_ }
    $found = if ($browsers) { ($browsers -join ", ") } else { "нет" }
    Write-Log "Шаг браузера пропущен: браузеры на раннере: $found. Наблюдатель в тестовом режиме трогает только папку WatchDir (RM3-8), копия браузера без его файлов не запускается; закрытие «Прочего» в режиме «только учёба» доказано notes.exe, браузер с сайтами — веха M4"
  }

  Step "Журнал родителя: «время вышло, доступна только учёба», без обхода и «попытка открыть запрещённое»" {
    Use-Parent {
      $closed = @(Find-Events 'время вышло, доступна только учёба')
      $closed | ForEach-Object { Write-Log "Журнал: $($_.text)" }
      Assert-That (@($closed | Where-Object { $_.text -match '«game»' }).Count -ge 1) "В журнале нет записи о закрытии «game»: время вышло, доступна только учёба"
      Assert-That (@($closed | Where-Object { $_.text -match '«notes»' }).Count -ge 1) "В журнале нет записи о закрытии «notes»: время вышло, доступна только учёба"
      $bypass = @(Find-Events 'попытка обойти защиту')
      Assert-That ($bypass.Count -eq 0) "В журнале запись об обходе: $(($bypass | ForEach-Object { $_.text }) -join ' | ')"
      $forbidden = @(Find-Events 'попытка открыть запрещённое')
      Assert-That ($forbidden.Count -eq 0) "В журнале «попытка открыть запрещённое»: $(($forbidden | ForEach-Object { $_.text }) -join ' | ')"
    }
  }

  Write-Log "S15: время вышло, игра и «Прочее» закрыты, учёба работает, экрана нет, журнал без обхода"
}
