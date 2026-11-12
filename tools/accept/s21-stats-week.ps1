# S21. Эталонная неделя G15 у родителя: «Итоги», столбики «По дням» (время дня и учёба, без лимита), таблицы
# «Программы» и «Сайты» совпадают с эталоном до минуты; ответ не дольше 1 с (FR-23, D-69, D-72, D-78, NFR-13).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60) и начинаются в вс 22.11.2026 14:50 (ClockStart): через 10 мин
# игры сегодня часы ядра — 15:00, как «now» в testdata/golden/G15.json.
# Дано: лимиты G15 (пн–чт 120, пт 180, сб–вс 240 мин); прошлые дни пн 16 … сб 21 — тестовой командой TestSeedDay
# из G15.json (ср 18 — строка дня с нулями): game.exe «Игры», study.exe «Учёба», other.exe «Прочее», сайт
# youtube.com «Видео и соцсети»; сегодня — game.exe 10 мин на переднем плане (из ядра).
# Тогда (часы на паузе): «Итоги» недели 753 / 285 / 220 / 95 / 248 / 848 мин, доп. время 45 мин; 7 столбиков с
# ключом, подписью, временем, учёбой и подсказкой из G15 (у столбика нет полей лимита); «Программы»: game.exe 285,
# other.exe 248, study.exe 95 мин; «Сайты»: youtube.com 220 мин; «Год» — 11 столбиков, «ноя» 753 мин, учёба 95 мин.
# Снимок: s21-parent-stats.png (окно родителя).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$script:G15 = Get-Content -LiteralPath (Join-Path $script:RepoRoot "testdata\golden\G15.json") -Raw -Encoding UTF8 | ConvertFrom-Json

# Поля столбика, которые отдаёт служба (RM5-3): только время, без лимита и превышений (D-72, D-78).
$BarFields = @("key", "label", "title", "totalS", "studyS", "today")

# ParentStats за период с замером времени ответа (≤ 1 с, NFR-13).
function Get-ParentPeriod([string]$Period) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  $r = Send-Parent "ParentStats" @{ childId = $script:ChildId; period = $Period }
  $ms = $sw.ElapsedMilliseconds
  Assert-Reply $r "ParentStats $Period"
  Write-Log "ParentStats «$Period»: $ms мс"
  Assert-That ($ms -le 1000) "ParentStats за «$Period» отвечал $ms мс, ждали не дольше 1000 мс"
  return $r.payload
}

# Секунды строк программ (kind program) или сайтов (kind site) по пути из всех категорий (RM5-4: таблица
# называет только программы и сайты; строки «Остальное время» без пути в неё не входят).
function Get-TableSeconds($Reply, [string]$Kind) {
  $t = [ordered]@{}
  foreach ($r in @(List $Reply.programs)) {
    $k = if ([string]$r.kind) { [string]$r.kind } else { "program" }
    if ($k -ne $Kind -or -not [string]$r.path) { continue }
    $p = ([string]$r.path).ToLowerInvariant()
    if (-not $t.Contains($p)) { $t[$p] = 0 }
    $t[$p] += [int]$r.seconds
  }
  return $t
}

function Format-Rows($Reply) {
  return (@(List $Reply.programs) | ForEach-Object { "$($_.kind)/$($_.category):$($_.path)=$($_.seconds)" }) -join ' | '
}

function Format-Bars($Reply) {
  return (@(List $Reply.bars) | ForEach-Object { "$($_.key) «$($_.label)» $($_.totalS)/$($_.studyS) «$($_.title)»$(if ($_.today) { ' сегодня' })" }) -join ' | '
}

# Минуты, как их показывает окно: неполная минута отбрасывается (FormatDuration).
function Get-Min([int]$Seconds) { return [int][math]::Floor($Seconds / 60) }

Invoke-Scenario "S21" "Эталонная неделя G15: «Итоги», столбики, «Программы» и «Сайты» до минуты, ответ ≤ 1 с" {
  Initialize-Scenario 60 0 "2026-11-22T14:50:00"

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовые программы game.exe, study.exe и other.exe в папке наблюдения" {
    $script:Paths = @{
      games = New-TestApp "game"
      study = New-TestApp "study"
      other = New-TestApp "other"
    }
  }

  Step "Настройка по G15: пн–чт 120, пт 180, сб–вс 240 мин; game.exe «Игры», study.exe «Учёба», other.exe «Прочее»" {
    $lim = $script:G15.limits_min
    $mins = @($lim.mon, $lim.tue, $lim.wed, $lim.thu, $lim.fri, $lim.sat, $lim.sun)
    $limits = @(1..7 | ForEach-Object { New-DayLimit $_ ([int]$mins[$_ - 1]) })
    $progs = @(
      @{ path = $script:Paths.games; category = "games" },
      @{ path = $script:Paths.study; category = "study" },
      @{ path = $script:Paths.other; category = "other" })
    Start-Input 1000
    Invoke-Setup 120 $script:MySid "Тест" $limits -1 $progs
    Suspend-Clock
    Wait-SessionParts
    Start-Sleep -Seconds 3
    Wait-ProgramsReady
    $script:ChildId = Use-Parent { Get-ChildId }
    $day = [DateTimeOffset]::FromUnixTimeMilliseconds((Get-CoreNowMs)).LocalDateTime.ToString("yyyy-MM-dd")
    Assert-That ($day -eq "2026-11-22") "Часы ядра показывают $day, ждали 2026-11-22 (ClockStart)"
  }

  Step "Прошлые дни G15 подложены тестовой командой TestSeedDay" {
    foreach ($d in @($script:G15.days | Where-Object { -not $_.today })) {
      $usage = @()
      if ([int]$d.games_min -gt 0) { $usage += @{ path = $script:Paths.games; category = "games"; seconds = 60 * [int]$d.games_min } }
      if ([int]$d.study_min -gt 0) { $usage += @{ path = $script:Paths.study; category = "study"; seconds = 60 * [int]$d.study_min } }
      if ([int]$d.other_min -gt 0) { $usage += @{ path = $script:Paths.other; category = "other"; seconds = 60 * [int]$d.other_min } }
      if ([int]$d.video_min -gt 0) { $usage += @{ path = [string]$script:G15.subjects.video_social; category = "video_social"; seconds = 60 * [int]$d.video_min; kind = "site" } }
      $req = @{
        childId = $script:ChildId; day = [string]$d.day
        totalS = 60 * [int]$d.used_min; gamesS = 60 * [int]$d.games_min; videoS = 60 * [int]$d.video_min
        studyS = 60 * [int]$d.study_min; extraMin = [int]$d.extra_min; usage = @($usage)
      }
      Assert-Reply (Send-Pcb "TestSeedDay" $req) "TestSeedDay $($d.day)"
    }
  }

  Step "Сегодня game.exe 10 мин на переднем плане" {
    $script:Game = Start-TestApp $script:Paths.games
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Game "game.exe"
    Set-TestForeground $script:Game
    Start-Sleep -Seconds 2 # часы на паузе: служба получает снимок стража уже с этой программой
    Resume-Clock
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($true) {
      $m = Get-MyToday "today"
      if ([int]$m.gamesS -ge 600) { break }
      if ($sw.Elapsed.TotalSeconds -gt 90) {
        Suspend-Clock
        throw "За 90 с не набралось 600 с игр; использовано $($m.usedS) с, игры $($m.gamesS) с; экран: $(Format-GuardState (Get-GuardState))"
      }
      Start-Sleep -Milliseconds 40
    }
    Suspend-Clock
    $script:Today = Get-MyToday "today"
    Write-Log ("Сегодня: использовано {0} с, игры {1} с, прочее {2} с, видео {3} с ({4:N1} с настоящего времени)" -f $script:Today.usedS, $script:Today.gamesS, $script:Today.otherS, $script:Today.videoS, $sw.Elapsed.TotalSeconds)
    # Эталон G15 за сегодня — 10 мин игры: неполная минута сверх не меняет минут (окно отбрасывает секунды).
    Assert-That ((Get-Min ([int]$script:Today.usedS)) -eq 10) "Сегодня использовано $($script:Today.usedS) с, ждали 600…659 (10 мин G15)"
    Assert-That ([int]$script:Today.studyS -eq 0 -and [int]$script:Today.videoS -eq 0) "Сегодня учёба $($script:Today.studyS) с, видео $($script:Today.videoS) с, ждали 0"
  }

  Step "Неделя у родителя: «Итоги» по G15 до минуты, ответ ≤ 1 с" {
    Use-Parent {
      $script:Week = Get-ParentPeriod "week"
      $script:Year = Get-ParentPeriod "year"
    }
    $w = $script:Week
    $t = $script:Today
    Write-Log "Итоги недели: использовано $($w.usedS) с, игры $($w.gamesS), видео $($w.videoS), учёба $($w.studyS), прочее $($w.otherS), всего $($w.allS), доп. $($w.extraMin) мин; строки: $(Format-Rows $w)"
    # Прошлые дни G15 точно в секундах (литералы эталона × 60) плюс сегодня из ядра (PL-31).
    $past = @{ usedS = 45180; gamesS = 16500; videoS = 13200; studyS = 5700; otherS = 14880 }
    foreach ($k in $past.Keys) {
      $want = $past[$k] + [int]$t.$k
      Assert-That ([int]$w.$k -eq $want) "Неделя: $k $($w.$k) с, ждали $want (G15 $($past[$k]) + сегодня $($t.$k))"
    }
    $e = $script:G15.expect.week
    $mins = @{ usedS = $e.used_min; gamesS = $e.games_min; videoS = $e.video_min; studyS = $e.study_min; otherS = $e.other_min; allS = $e.all_min }
    foreach ($k in $mins.Keys) {
      $got = Get-Min ([int]$w.$k)
      Assert-That ([math]::Abs($got - [int]$mins[$k]) -le 1) "Неделя: $k $got мин, по G15 $($mins[$k]) мин (допуск до минуты)"
    }
    Assert-That ([int]$w.allS -eq [int]$w.usedS + [int]$w.studyS) "Неделя: «Всего» $($w.allS) с, ждали $($w.usedS) + $($w.studyS)"
    Assert-That ([int]$w.extraMin -eq [int]$e.extra_min) "Неделя: доп. время $($w.extraMin) мин, по G15 $($e.extra_min) мин"
  }

  Step "Столбики недели: 7 дней по G15, только время и учёба, сегодня из ядра" {
    $w = $script:Week
    $bars = @(List $w.bars)
    Write-Log "Столбики недели ($($w.barsBy)): $(Format-Bars $w)"
    Assert-That ([string]$w.barsBy -eq "day") "Разбивка недели «$($w.barsBy)», ждали day"
    $want = @($script:G15.expect.bars_week)
    Assert-That ($bars.Count -eq $want.Count) "Столбиков недели $($bars.Count), ждали $($want.Count)"
    $sumT = 0; $sumS = 0
    for ($i = 0; $i -lt $want.Count; $i++) {
      $b = $bars[$i]; $g = $want[$i]
      $fields = @($b.PSObject.Properties.Name | Sort-Object)
      Assert-That (($fields -join ',') -eq (($BarFields | Sort-Object) -join ',')) "Поля столбика $($g.key): $($fields -join ', '), ждали только $($BarFields -join ', ') (без лимита, D-78)"
      Assert-That ([string]$b.key -eq [string]$g.key -and [string]$b.label -eq [string]$g.label) "Столбик ${i}: «$($b.key)» «$($b.label)», ждали «$($g.key)» «$($g.label)»"
      $isToday = [bool]$g.today
      Assert-That ([bool]$b.today -eq $isToday) "Столбик $($g.key): today=$($b.today), ждали $isToday"
      if ($isToday) {
        Assert-That ([int]$b.totalS -eq [int]$script:Today.usedS -and [int]$b.studyS -eq [int]$script:Today.studyS) "Столбик сегодня: $($b.totalS)/$($b.studyS) с, ждали из ядра $($script:Today.usedS)/$($script:Today.studyS)"
      } else {
        Assert-That ([int]$b.totalS -eq 60 * [int]$g.total_min -and [int]$b.studyS -eq 60 * [int]$g.study_min) "Столбик $($g.key): $($b.totalS)/$($b.studyS) с, ждали $(60 * [int]$g.total_min)/$(60 * [int]$g.study_min)"
      }
      # Сегодня 600…659 с: в подсказке «10 мин», как в G15 (неполная минута отбрасывается).
      Assert-That ([string]$b.title -eq [string]$g.title) "Подсказка $($g.key): «$($b.title)», ждали «$($g.title)»"
      $sumT += [int]$b.totalS; $sumS += [int]$b.studyS
    }
    Assert-That ($sumT -eq [int]$w.usedS -and $sumS -eq [int]$w.studyS) "Сумма столбиков $sumT/$sumS с, «Итоги» $($w.usedS)/$($w.studyS) с"
  }

  Step "Таблицы недели: «Программы» и «Сайты» по G15 до минуты" {
    $w = $script:Week
    $progs = Get-TableSeconds $w "program"
    $sites = Get-TableSeconds $w "site"
    Write-Log ("«Программы»: " + (($progs.GetEnumerator() | ForEach-Object { "$($_.Key)=$($_.Value)" }) -join ', ') + "; «Сайты»: " + (($sites.GetEnumerator() | ForEach-Object { "$($_.Key)=$($_.Value)" }) -join ', '))
    $byName = @{
      "game.exe" = $script:Paths.games.ToLowerInvariant()
      "study.exe" = $script:Paths.study.ToLowerInvariant()
      "other.exe" = $script:Paths.other.ToLowerInvariant()
    }
    $expP = @($script:G15.expect.programs)
    Assert-That ($progs.Count -eq $expP.Count) "Строк «Программ» $($progs.Count), ждали $($expP.Count): $(Format-Rows $w)"
    $sumP = 0
    foreach ($p in $progs.Values) { $sumP += $p }
    foreach ($row in $expP) {
      $path = $byName[[string]$row.name]
      Assert-That ($progs.Contains($path)) "В «Программах» нет $($row.name): $(Format-Rows $w)"
      $got = Get-Min ([int]$progs[$path])
      Assert-That ([math]::Abs($got - [int]$row.min) -le 1) "«Программы»: $($row.name) $got мин, по G15 $($row.min) мин"
      $pct = [int][math]::Round(100.0 * $progs[$path] / $sumP, [MidpointRounding]::AwayFromZero)
      Write-Log "«Программы»: $($row.name) $got мин, доля $pct % (G15 $($row.pct) %; долю считает окно)"
    }
    $order = @($progs.GetEnumerator() | Sort-Object -Property Value -Descending | ForEach-Object { $_.Key })
    Assert-That (($order -join '|') -eq (($expP | ForEach-Object { $byName[[string]$_.name] }) -join '|')) "Порядок «Программ» по времени: $($order -join ', ')"
    $expS = @($script:G15.expect.sites)
    Assert-That ($sites.Count -eq $expS.Count) "Строк «Сайтов» $($sites.Count), ждали $($expS.Count): $(Format-Rows $w)"
    foreach ($row in $expS) {
      Assert-That ($sites.Contains([string]$row.name)) "В «Сайтах» нет $($row.name): $(Format-Rows $w)"
      $got = Get-Min ([int]$sites[[string]$row.name])
      Assert-That ($got -eq [int]$row.min) "«Сайты»: $($row.name) $got мин, по G15 $($row.min) мин"
    }
  }

  Step "Год: 11 столбиков по месяцам, «ноя» по G15" {
    $y = $script:Year
    $bars = @(List $y.bars)
    Write-Log "Столбики года ($($y.barsBy)): $(Format-Bars $y)"
    Assert-That ([string]$y.barsBy -eq "month") "Разбивка года «$($y.barsBy)», ждали month"
    Assert-That ($bars.Count -eq [int]$script:G15.expect.year.bars) "Столбиков года $($bars.Count), ждали $($script:G15.expect.year.bars)"
    $n = $script:G15.expect.year.nov
    $nov = $bars[$bars.Count - 1]
    Assert-That ([string]$nov.key -eq [string]$n.key -and [string]$nov.label -eq [string]$n.label -and [bool]$nov.today) "Последний столбик года «$($nov.key)» «$($nov.label)» today=$($nov.today), ждали «$($n.key)» «$($n.label)», сегодня"
    Assert-That ([int]$nov.totalS -eq [int]$script:Week.usedS -and [int]$nov.studyS -eq [int]$script:Week.studyS) "«ноя»: $($nov.totalS)/$($nov.studyS) с, ждали как неделя $($script:Week.usedS)/$($script:Week.studyS)"
    Assert-That ([string]$nov.title -eq [string]$n.title) "Подсказка «ноя»: «$($nov.title)», ждали «$($n.title)»"
    foreach ($b in $bars[0..($bars.Count - 2)]) {
      Assert-That ([int]$b.totalS -eq 0 -and [int]$b.studyS -eq 0) "Месяц $($b.key) без данных: $($b.totalS)/$($b.studyS) с, ждали пустой столбик"
    }
  }

  Step "Снимок окна родителя" {
    [void](Save-ProgramWindow "s21-parent-stats" @("--parent") 40 5)
  }

  Write-Log "S21: неделя G15 у родителя — «Итоги», столбики и таблицы совпадают с эталоном, ответ ≤ 1 с"
}
