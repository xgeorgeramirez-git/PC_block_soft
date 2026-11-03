# S42. Периоды «Итогов»: сегодня, неделя, месяц, год и всё время у родителя и в окне ребёнка (D-61, FR-12, FR-23, PL-22).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60) и начинаются в среду 21.10.2026 15:00 (ClockStart): понедельник
# этой недели, 1-е число и 2 января — прошлые дни при любой дате раннера, ни один период не пустой.
# Прошлые дни подкладывает тестовая команда TestSeedDay (только тестовый режим, день строго раньше сегодняшнего):
# тем же путём, что служба сохраняет день (SaveDayAndUsage). Сегодня — game.exe на переднем плане 10 мин часов ядра.
# Дано: пн 19.10 игры 30 мин (game.exe), 01.10 игры 20 мин, 02.01 учёба 40 мин (study.exe), 15.06.2025 прочее 50 мин
# без программы; сегодня game.exe 10 мин. Тогда (часы на паузе, сегодняшние цифры — из ответа «Сегодня», PL-22):
# «Игры» неделя = 30 мин + сегодня, месяц = год = всё время = 50 мин + сегодня; «Учёба» за год 40 мин; «Всего» за всё
# время = 50 + 50 + 40 мин + сегодня; строка game.exe с суммой за период; суммы строк = итоги вкладок; у родителя то
# же, что у ребёнка; каждый ответ периода ≤ 1 с. Снимок: s42-stats.png (окно «Моя статистика»).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$Periods = @("today", "week", "month", "year", "all")

# Строки programs ответа с путём Path и категорией Category.
function Find-Row($Reply, [string]$Path, [string]$Category) {
  $norm = $Path.ToLowerInvariant()
  return @(@(List $Reply.programs) | Where-Object { ([string]$_.path) -eq $norm -and [string]$_.category -eq $Category })
}

# Секунды строки программы Path в категории Category (0 — строки нет).
function Get-RowSeconds($Reply, [string]$Path, [string]$Category) {
  $row = @(Find-Row $Reply $Path $Category)
  if ($row.Count -eq 0) { return 0 }
  return [int]$row[0].seconds
}

# Строки ответа одной строкой: для текста ошибки (журналы Actions недоступны, видны только строки-ошибки).
function Format-Rows($Reply) {
  return (@(List $Reply.programs) | ForEach-Object { "$($_.category):$($_.name)=$($_.seconds)" + $(if ($_.path) { "" } else { " (без программы)" }) }) -join ' | '
}

# Итоги ответа одной строкой.
function Format-Totals($Reply) {
  return "период $($Reply.period): использовано $($Reply.usedS) с, игры $($Reply.gamesS) с, учёба $($Reply.studyS) с, прочее $($Reply.otherS) с, всего $($Reply.allS) с; строки: $(Format-Rows $Reply)"
}

# Секунды категории Category по строкам ответа.
function Measure-Rows($Reply, [string]$Category) {
  $n = 0
  foreach ($r in @(List $Reply.programs)) { if ([string]$r.category -eq $Category) { $n += [int]$r.seconds } }
  return $n
}

# ParentStats за период с замером времени ответа (переключение периода ≤ 1 с, D-61).
function Get-ParentPeriod([string]$Period) {
  $req = @{ childId = $script:ChildId; period = $Period }
  $sw = [Diagnostics.Stopwatch]::StartNew()
  $r = Send-Parent "ParentStats" $req
  $ms = $sw.ElapsedMilliseconds
  Assert-Reply $r "ParentStats $Period"
  Assert-That ($ms -le 1000) "ParentStats за «$Period» отвечал $ms мс, ждали не дольше 1000 мс"
  return $r.payload
}

# Проверки одного ответа Reply за период Period по ожиданиям Want (секунды).
function Assert-Period($Reply, [string]$Period, $Want, [string]$Who) {
  $text = Format-Totals $Reply
  Write-Log "${Who}: $text"
  Assert-That ([string]$Reply.period -eq $Period) "${Who}: в ответе период «$($Reply.period)», ждали «$Period»; $text"
  foreach ($k in @("gamesS", "studyS", "otherS", "allS", "usedS")) {
    Assert-That ([int]$Reply.$k -eq [int]$Want[$k]) "${Who} «$Period»: $k $($Reply.$k) с, ждали $($Want[$k]) с; $text"
  }
  Assert-That ((Get-RowSeconds $Reply $script:GamePath "games") -eq [int]$Want.gameRow) "${Who} «$Period»: строка game.exe $(Get-RowSeconds $Reply $script:GamePath 'games') с, ждали $($Want.gameRow) с; $text"
  Assert-That ((Get-RowSeconds $Reply $script:StudyPath "study") -eq [int]$Want.studyRow) "${Who} «$Period»: строка study.exe $(Get-RowSeconds $Reply $script:StudyPath 'study') с, ждали $($Want.studyRow) с; $text"
  if ([int]$Want.gameRow -gt 0) {
    $row = @(Find-Row $Reply $script:GamePath "games")
    Assert-That ([string]$row[0].name -eq $script:GameName) "${Who} «$Period»: название game.exe «$($row[0].name)», ждали «$($script:GameName)» (как в списке программ)"
  }
  foreach ($c in @(@{ Cat = "games"; Key = "gamesS" }, @{ Cat = "study"; Key = "studyS" }, @{ Cat = "other"; Key = "otherS" })) {
    $sum = Measure-Rows $Reply $c.Cat
    Assert-That ($sum -eq [int]$Reply.($c.Key)) "${Who} «$Period»: сумма строк «$($c.Cat)» $sum с, итог $($Reply.($c.Key)) с; $text"
  }
  Assert-That ([int]$Reply.allS -eq [int]$Reply.usedS + [int]$Reply.studyS) "${Who} «$Period»: «Всего» $($Reply.allS) с, ждали $($Reply.usedS) + $($Reply.studyS); $text"
}

Invoke-Scenario "S42" "Периоды «Итогов»: игры 10 / 40 / 60 / 60 / 60 мин, учёба за год 40 мин, всего за всё время 150 мин" {
  Initialize-Scenario 60 0 "2026-10-21T15:00:00"

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовые программы game.exe и study.exe в папке наблюдения" {
    $script:GamePath = New-TestApp "game"
    $script:StudyPath = New-TestApp "study"
  }

  Step "Настройка: лимит 4 ч, игры без отдельного лимита; game.exe «Игры», study.exe «Учёба»" {
    $progs = @(
      @{ path = $script:GamePath; category = "games" },
      @{ path = $script:StudyPath; category = "study" })
    Start-Child 240 $true -1 $progs
    Wait-ProgramsReady
    $script:GameName = Get-ProgramName $script:GamePath
    $script:ChildId = Use-Parent { Get-ChildId }
    Write-Log "Название game.exe из списка программ: «$($script:GameName)»"
  }

  Step "Часы ядра: среда 21.10.2026" {
    $day = [DateTimeOffset]::FromUnixTimeMilliseconds((Get-CoreNowMs)).LocalDateTime.ToString("yyyy-MM-dd")
    Assert-That ($day -eq "2026-10-21") "Часы ядра показывают $day, ждали 2026-10-21 (ClockStart)"
  }

  Step "Прошлые дни подложены тестовой командой TestSeedDay" {
    $id = $script:ChildId
    $days = @(
      @{ childId = $id; day = "2026-10-19"; totalS = 1800; gamesS = 1800; studyS = 0; usage = @(@{ path = $script:GamePath; category = "games"; seconds = 1800 }) },
      @{ childId = $id; day = "2026-10-01"; totalS = 1200; gamesS = 1200; studyS = 0; usage = @(@{ path = $script:GamePath; category = "games"; seconds = 1200 }) },
      @{ childId = $id; day = "2026-01-02"; totalS = 0; gamesS = 0; studyS = 2400; usage = @(@{ path = $script:StudyPath; category = "study"; seconds = 2400 }) },
      @{ childId = $id; day = "2025-06-15"; totalS = 3000; gamesS = 0; studyS = 0; usage = @() })
    foreach ($d in $days) {
      Assert-Reply (Send-Pcb "TestSeedDay" $d) "TestSeedDay $($d.day)"
    }
    $r = Send-Pcb "TestSeedDay" @{ childId = $id; day = "2026-10-21"; totalS = 60; gamesS = 0; studyS = 0; usage = @() }
    Assert-That (-not $r.ok) "TestSeedDay принял сегодняшний день: его ведёт ядро, он посчитался бы дважды"
  }

  Step "Сегодня game.exe 10 мин на переднем плане" {
    $script:Game = Start-TestApp $script:GamePath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Game "game.exe"
    Set-TestForeground $script:Game
    Start-Sleep -Seconds 2 # часы на паузе: служба получает снимок стража уже с этой программой
    Resume-Clock
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($true) {
      $m = Get-MyToday "today"
      if ((Get-RowSeconds $m $script:GamePath "games") -ge 600) { break }
      if ($sw.Elapsed.TotalSeconds -gt 90) {
        Suspend-Clock
        throw "За 90 с не набралось 600 с у game.exe; $(Format-Totals $m); экран: $(Format-GuardState (Get-GuardState))"
      }
      Start-Sleep -Milliseconds 40
    }
    Suspend-Clock
    Write-Log ("game.exe: 600 с часов ядра за {0:N1} с настоящего времени" -f $sw.Elapsed.TotalSeconds)
  }

  Step "Итоги за пять периодов у ребёнка (MyToday): прошлые дни + сегодня, без двойного счёта" {
    $t = Get-MyToday "today"
    Assert-That ([int]$t.gamesS -ge 600 -and [int]$t.gamesS -le 660) "«Сегодня»: игры $($t.gamesS) с, ждали 600…660; $(Format-Totals $t)"
    Assert-That ([int]$t.studyS -eq 0) "«Сегодня»: учёба $($t.studyS) с, ждали 0 (прошлые дни попали в сегодня); $(Format-Totals $t)"
    $g = [int]$t.gamesS; $o = [int]$t.otherS; $u = [int]$t.usedS; $gr = Get-RowSeconds $t $script:GamePath "games"
    $script:Want = @{
      today = @{ gamesS = $g; studyS = 0; otherS = $o; usedS = $u; allS = $u; gameRow = $gr; studyRow = 0 }
      week  = @{ gamesS = 1800 + $g; studyS = 0; otherS = $o; usedS = 1800 + $u; allS = 1800 + $u; gameRow = 1800 + $gr; studyRow = 0 }
      month = @{ gamesS = 3000 + $g; studyS = 0; otherS = $o; usedS = 3000 + $u; allS = 3000 + $u; gameRow = 3000 + $gr; studyRow = 0 }
      year  = @{ gamesS = 3000 + $g; studyS = 2400; otherS = $o; usedS = 3000 + $u; allS = 5400 + $u; gameRow = 3000 + $gr; studyRow = 2400 }
      all   = @{ gamesS = 3000 + $g; studyS = 2400; otherS = 3000 + $o; usedS = 6000 + $u; allS = 8400 + $u; gameRow = 3000 + $gr; studyRow = 2400 }
    }
    $script:Mine = @{}
    foreach ($p in $Periods) {
      $sw = [Diagnostics.Stopwatch]::StartNew()
      $r = Get-MyToday $p
      $ms = $sw.ElapsedMilliseconds
      Assert-That ($ms -le 1000) "MyToday за «$p» отвечал $ms мс, ждали не дольше 1000 мс"
      Assert-Period $r $p $script:Want[$p] "MyToday"
      Assert-That ([int]$r.status.usedS -eq $u) "MyToday «$p»: статус за сегодня $($r.status.usedS) с, ждали $u с (остаток от периода не зависит)"
      $script:Mine[$p] = $r
    }
    # S42 на заранее известной дате: «Игры» 10 / 40 / 60 / 60 / 60 мин, «Всего» за всё время 150 мин (+ секунды сегодня сверх 10 мин).
    Write-Log ("S42 в минутах: игры {0} / {1} / {2} / {3} / {4}, учёба за год {5}, всего за всё время {6}" -f `
        [math]::Floor($script:Mine.today.gamesS / 60), [math]::Floor($script:Mine.week.gamesS / 60), [math]::Floor($script:Mine.month.gamesS / 60),
        [math]::Floor($script:Mine.year.gamesS / 60), [math]::Floor($script:Mine.all.gamesS / 60), [math]::Floor($script:Mine.year.studyS / 60), [math]::Floor($script:Mine.all.allS / 60))
  }

  Step "Статистика родителя (ParentStats) за те же периоды: то же, что у ребёнка" {
    Use-Parent {
      foreach ($p in $Periods) {
        $r = Get-ParentPeriod $p
        Assert-Period $r $p $script:Want[$p] "ParentStats"
        Assert-That ((Format-Rows $r) -eq (Format-Rows $script:Mine[$p])) "«$p»: у родителя другие строки (часы стоят): $(Format-Rows $r) | у ребёнка: $(Format-Rows $script:Mine[$p])"
      }
      $bad = Send-Parent "ParentStats" @{ childId = $script:ChildId; period = "decade" }
      Assert-That (-not $bad.ok) "ParentStats принял неизвестный период «decade»"
    }
  }

  Step "Снимок «Моя статистика»" {
    [void](Save-ProgramWindow "s42-stats" @("--mystats") 40 5)
  }

  Write-Log "S42: периоды «Итогов» у ребёнка и родителя сходятся с прошлыми днями и сегодня, ответы не дольше 1 с"
}
