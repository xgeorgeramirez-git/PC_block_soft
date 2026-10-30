# S13. Учёба не входит в общее время (R4, FR-12, FR-23, G4, RM3-7, RM3-8).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): 40 мин ядра — около 40 с настоящего времени. Тестовая
# программа study.exe (копия системной программы в папке WatchDir) отмечена «Учёба» шагом мастера
# (Setup.programs) до запуска (PL-11). Ввод имитируется движением мыши.
# Дано: учебная программа. Когда: 40 мин на переднем плане. Тогда: общее время (usedS) не изменилось
# (±1 мин часов ядра); в «Моей статистике» (MyToday) и статистике родителя (ParentStats) учёба 40 мин.
# Снимок: s13-stats.png (окно «Моя статистика»).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S13" "Учёба 40 мин: общее время не изменилось, в статистике «Учёба 40 мин»" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовая программа study.exe в папке наблюдения" {
    $script:StudyPath = New-TestApp "study"
  }

  Step "Настройка: лимит 2 ч, study.exe «Учёба»" {
    Start-Child 120 $true -1 @(@{ path = $script:StudyPath; category = "study" })
    Wait-ProgramsReady
    $m = Get-MyToday
    $script:Used0 = [int]$m.status.usedS
    Write-Log "До учёбы: общее время $($script:Used0) с, учёба $($m.studyS) с, игры $($m.gamesS) с"
    Assert-That ($m.studyS -eq 0) "Учёба до запуска $($m.studyS) с, ждали 0"
  }

  Step "study.exe работает и стоит на переднем плане" {
    $script:Study = Start-TestApp $script:StudyPath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Study "study.exe («Учёба»)"
    Set-TestForeground $script:Study
    Resume-Clock
  }

  Step "40 мин часов ядра в учёбе: учёба 40 мин, общее время то же (±1 мин)" {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($true) {
      $m = Get-MyToday
      if ($m.studyS -ge 2400) { break }
      if ($sw.Elapsed.TotalSeconds -gt 90) {
        throw "За 90 с не набралось 40 мин учёбы: учёба $($m.studyS) с, общее $($m.status.usedS) с, игры $($m.gamesS) с; экран: $(Format-GuardState (Get-GuardState))"
      }
      Start-Sleep -Milliseconds 40
    }
    Suspend-Clock
    Write-Log ("40 мин учёбы за {0:N1} с настоящего времени" -f $sw.Elapsed.TotalSeconds)
    $script:Mine = Get-MyToday
    Assert-Between $script:Mine.studyS 2400 2460 "Учёба за сегодня (MyToday.studyS), с"
    Assert-Between ([int]$script:Mine.status.usedS - $script:Used0) 0 60 "Общее время за 40 мин учёбы (рост usedS), с"
    Assert-That ($script:Mine.gamesS -eq 0) "Игры $($script:Mine.gamesS) с, ждали 0"
    Assert-AppAlive $script:Study "study.exe после 40 мин"
    $g = Get-GuardState
    Assert-That (-not $g.lockShown) "Экран блокировки во время учёбы: $(Format-GuardState $g)"
  }

  Step "Статистика родителя (ParentStats): учёба 40 мин, общее время то же" {
    $p = Use-Parent {
      $r = Send-Parent "ParentStats" @{ childId = (Get-ChildId) }
      Assert-Reply $r "ParentStats"
      $r.payload
    }
    Write-Log "ParentStats: учёба $($p.studyS) с, игры $($p.gamesS) с, общее $($p.status.usedS) с; «Сегодня»: $((@(List $p.today)) -join ' | ')"
    Assert-Between $p.studyS 2400 2460 "Учёба в статистике родителя (ParentStats.studyS), с"
    Assert-That ([int]$p.status.usedS -eq [int]$script:Mine.status.usedS) "Общее время у родителя $($p.status.usedS) с, у ребёнка $($script:Mine.status.usedS) с (часы стоят)"
  }

  Step "Снимок «Моя статистика» с учёбой 40 мин" {
    [void](Save-ProgramWindow "s13-stats" @("--mystats") 40 5)
  }

  Write-Log "S13: 40 мин учёбы не вошли в общее время, «Учёба 40 мин» у ребёнка и родителя"
}
