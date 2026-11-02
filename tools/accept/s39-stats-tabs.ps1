# S39. Вкладки «Итогов»: время каждой программы за сегодня (D-58, FR-12, FR-23, PL-16, PL-22).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): минута ядра это секунда настоящего времени.
# Тестовые программы — копии системной программы в папке WatchDir (New-TestApp): game.exe и game2.exe
# «Игры», study.exe «Учёба» отмечены шагом мастера (Setup.programs) до запуска (PL-11). Ввод имитируется
# движением мыши. Строки ищутся по пути, названия — из ответа программы (ListPrograms): у копий одной
# системной программы одно название из свойств файла (PL-22).
# Дано: game.exe 10 мин, game2.exe 5 мин, study.exe 3 мин на переднем плане по очереди. Тогда: в MyToday
# и ParentStats строки programs с этими секундами (допуск часов +60 с), названия как в списке программ,
# сумма строк «Игры» = gamesS, «Учёба» = studyS, «Игры» по убыванию: game, game2; у родителя то же, что у
# ребёнка. Снимок: s39-stats.png (окно «Моя статистика»).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Строки programs ответа MyToday или ParentStats с путём Path и категорией Category.
function Find-Row($Reply, [string]$Path, [string]$Category) {
  $norm = $Path.ToLowerInvariant()
  return @(@(List $Reply.programs) | Where-Object { ([string]$_.path) -eq $norm -and [string]$_.category -eq $Category })
}

# Строки ответа одной строкой: для текста ошибки (журналы Actions недоступны, видны только строки-ошибки).
function Format-Rows($Reply) {
  return (@(List $Reply.programs) | ForEach-Object { "$($_.category):$($_.name)=$($_.seconds)" + $(if ($_.path) { "" } else { " (без программы)" }) }) -join ' | '
}

# Секунды категории Category по строкам ответа.
function Measure-Rows($Reply, [string]$Category) {
  $n = 0
  foreach ($r in @(List $Reply.programs)) { if ([string]$r.category -eq $Category) { $n += [int]$r.seconds } }
  return $n
}

# Программа App на переднем плане, пока её строка не наберёт Seconds (ожидание признаком, PL-16).
function Invoke-Foreground($App, [string]$Path, [string]$Category, [int]$Seconds, [string]$What) {
  Set-TestForeground $App
  Start-Sleep -Seconds 2 # часы на паузе: служба получает снимок стража уже с этой программой
  Resume-Clock
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($true) {
    $m = Get-MyToday
    $row = @(Find-Row $m $Path $Category)
    if ($row.Count -eq 1 -and [int]$row[0].seconds -ge $Seconds) { break }
    if ($sw.Elapsed.TotalSeconds -gt 90) {
      Suspend-Clock
      throw "За 90 с не набралось $Seconds с у $What; строки: $(Format-Rows $m); игры $($m.gamesS) с, учёба $($m.studyS) с, общее $($m.status.usedS) с; экран: $(Format-GuardState (Get-GuardState))"
    }
    Start-Sleep -Milliseconds 40
  }
  Suspend-Clock
  Write-Log ("{0}: {1} с часов ядра за {2:N1} с настоящего времени" -f $What, $Seconds, $sw.Elapsed.TotalSeconds)
}

# Проверки ответа Reply (MyToday или ParentStats) по образцу S39.
function Assert-S39($Reply, [string]$Who) {
  $rows = Format-Rows $Reply
  Write-Log "${Who}: игры $($Reply.gamesS) с, учёба $($Reply.studyS) с, прочее $($Reply.otherS) с, всего $($Reply.allS) с; строки: $rows"
  foreach ($c in @(
      @{ Path = $script:GamePath; Cat = "games"; Min = 600; What = "game.exe" },
      @{ Path = $script:Game2Path; Cat = "games"; Min = 300; What = "game2.exe" },
      @{ Path = $script:StudyPath; Cat = "study"; Min = 180; What = "study.exe" })) {
    $row = @(Find-Row $Reply $c.Path $c.Cat)
    Assert-That ($row.Count -eq 1) "${Who}: строк $($c.What) в «$($c.Cat)» $($row.Count), ждали одну; строки: $rows"
    Assert-Between ([int]$row[0].seconds) $c.Min ($c.Min + 60) "${Who}: $($c.What), с"
    $want = $script:Names[$c.Path]
    Assert-That ([string]$row[0].name -eq $want) "${Who}: название $($c.What) «$($row[0].name)», ждали «$want» (как в списке программ); строки: $rows"
  }
  $games = Measure-Rows $Reply "games"
  Assert-That ($games -eq [int]$Reply.gamesS) "${Who}: сумма строк «Игры» $games с, итог «Игры» $($Reply.gamesS) с; строки: $rows"
  $study = Measure-Rows $Reply "study"
  Assert-That ($study -eq [int]$Reply.studyS) "${Who}: сумма строк «Учёба» $study с, итог «Учёба» $($Reply.studyS) с; строки: $rows"
  $other = Measure-Rows $Reply "other"
  Assert-That ($other -eq [int]$Reply.otherS) "${Who}: сумма строк «Прочее» $other с, итог «Прочее» $($Reply.otherS) с; строки: $rows"
  Assert-That ([int]$Reply.allS -eq [int]$Reply.status.usedS + [int]$Reply.studyS) "${Who}: «Всего» $($Reply.allS) с, ждали общее $($Reply.status.usedS) + учёба $($Reply.studyS); строки: $rows"
  $order = @(@(List $Reply.programs) | Where-Object { [string]$_.category -eq "games" -and $_.path } | ForEach-Object { ([string]$_.path).ToLowerInvariant() })
  $wantOrder = @($script:GamePath.ToLowerInvariant(), $script:Game2Path.ToLowerInvariant())
  Assert-That (($order -join '|') -eq ($wantOrder -join '|')) "${Who}: порядок «Игр» $($order -join ', '), ждали game.exe, game2.exe (самые долгие сверху); строки: $rows"
}

Invoke-Scenario "S39" "Вкладки «Итогов»: game 10 мин, game2 5 мин, study 3 мин у ребёнка и родителя" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовые программы game.exe, game2.exe и study.exe в папке наблюдения" {
    $script:GamePath = New-TestApp "game"
    $script:Game2Path = New-TestApp "game2"
    $script:StudyPath = New-TestApp "study"
  }

  Step "Настройка: лимит 2 ч, игры без отдельного лимита; game.exe и game2.exe «Игры», study.exe «Учёба»" {
    $progs = @(
      @{ path = $script:GamePath; category = "games" },
      @{ path = $script:Game2Path; category = "games" },
      @{ path = $script:StudyPath; category = "study" })
    Start-Child 120 $true -1 $progs
    Wait-ProgramsReady
    $script:Names = @{}
    foreach ($p in @($script:GamePath, $script:Game2Path, $script:StudyPath)) { $script:Names[$p] = Get-ProgramName $p }
    Write-Log "Названия из списка программ: $(($script:Names.GetEnumerator() | ForEach-Object { "$(Split-Path -Leaf $_.Key)=«$($_.Value)»" }) -join ', ')"
    $m = Get-MyToday
    Assert-That (@(List $m.programs | Where-Object { $_.path }).Count -eq 0) "До запуска уже есть программы: $(Format-Rows $m)"
  }

  Step "Программы запущены и работают" {
    $script:Game = Start-TestApp $script:GamePath
    $script:Game2 = Start-TestApp $script:Game2Path
    $script:Study = Start-TestApp $script:StudyPath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Game "game.exe"
    Assert-AppAlive $script:Game2 "game2.exe"
    Assert-AppAlive $script:Study "study.exe"
  }

  Step "game.exe 10 мин, game2.exe 5 мин, study.exe 3 мин на переднем плане" {
    Invoke-Foreground $script:Game $script:GamePath "games" 600 "game.exe"
    Invoke-Foreground $script:Game2 $script:Game2Path "games" 300 "game2.exe"
    Invoke-Foreground $script:Study $script:StudyPath "study" 180 "study.exe"
  }

  Step "«Моя статистика» (MyToday): строки программ, суммы сходятся с итогами" {
    $script:Mine = Get-MyToday
    Assert-S39 $script:Mine "MyToday"
  }

  Step "Статистика родителя (ParentStats): то же, что у ребёнка" {
    $p = Use-Parent {
      $r = Send-Parent "ParentStats" @{ childId = (Get-ChildId) }
      Assert-Reply $r "ParentStats"
      $r.payload
    }
    Assert-S39 $p "ParentStats"
    Assert-That ((Format-Rows $p) -eq (Format-Rows $script:Mine)) "У родителя другие строки (часы стоят): $(Format-Rows $p) | у ребёнка: $(Format-Rows $script:Mine)"
  }

  Step "Снимок «Моя статистика» со строками программ" {
    [void](Save-ProgramWindow "s39-stats" @("--mystats") 40 5)
  }

  Write-Log "S39: game 10 мин, game2 5 мин, study 3 мин у ребёнка и родителя, суммы строк равны итогам"
}
