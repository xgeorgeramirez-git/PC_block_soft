# S44. Запрещённые программы не показываются в «Итогах» статистики (D-64, RF16-1, FR-12, FR-23, PL-22, PL-28).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60) и начинаются в среду 21.10.2026 15:00 (ClockStart): понедельник
# этой недели — прошлый день, «Неделя» отличается от «Сегодня». Прошлый день подкладывает тестовая команда
# TestSeedDay. Тестовая программа — копия системной программы в папке WatchDir (New-TestApp): tool.exe отмечена
# «Прочее» шагом мастера (Setup.programs) до запуска (PL-11). Ввод имитируется движением мыши. Строки ищутся по
# пути, название — из ответа программы (ListPrograms, PL-22).
# Дано: пн 19.10 tool.exe 10 мин «Прочего»; сегодня tool.exe 5 мин на переднем плане. Когда: родитель запрещает
# tool.exe (часы на паузе) и открывает «Итоги» за «Сегодня» и «Неделю»; то же в окне ребёнка. Тогда: строки
# tool.exe нет ни на одной вкладке; её секунды — в строке «Остальное время» вкладки «Прочее» (прибавились ровно
# секунды её строки до запрета); «Всего», «Прочее» и «Использовано» не изменились; суммы строк = итоги вкладок;
# у родителя то же, что у ребёнка; в «Программах» tool.exe видна как «Запрещено» (blocked, не скрыта).
# Снимок: s44-stats.png (окно «Моя статистика»).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Текст службы (internal/service/stats.go textOtherTime): строка вкладки для времени без программы.
$script:OtherTime = "Остальное время"
$Periods = @("today", "week")

# Строки programs ответа с путём Path в любой категории.
function Find-PathRows($Reply, [string]$Path) {
  $norm = $Path.ToLowerInvariant()
  return @(@(List $Reply.programs) | Where-Object { ([string]$_.path) -eq $norm })
}

# Секунды строки «Остальное время» категории Category (0 — строки нет).
function Get-RestSeconds($Reply, [string]$Category) {
  $row = @(@(List $Reply.programs) | Where-Object { -not $_.path -and [string]$_.category -eq $Category })
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

# Проверки ответа Reply за период Period после запрета: Before — ответ ребёнка за тот же период до запрета.
function Assert-NoBlocked($Reply, $Before, [string]$Period, [string]$Who) {
  $text = Format-Totals $Reply
  Write-Log "${Who} «$Period» после запрета: $text"
  $rows = @(Find-PathRows $Reply $script:ToolPath)
  Assert-That ($rows.Count -eq 0) "${Who} «$Period»: запрещённая tool.exe («$($script:ToolName)», $($script:ToolPath.ToLowerInvariant())) осталась строкой: $(($rows | ForEach-Object { "$($_.category):$($_.name)=$($_.seconds)" }) -join ', '); $text"
  foreach ($k in @("usedS", "gamesS", "studyS", "otherS", "allS")) {
    Assert-That ([int]$Reply.$k -eq [int]$Before.$k) "${Who} «$Period»: $k $($Reply.$k) с, до запрета $($Before.$k) с (итоги от запрета не меняются); $text"
  }
  $toolBefore = [int](@(Find-PathRows $Before $script:ToolPath)[0].seconds)
  $restWant = (Get-RestSeconds $Before "other") + $toolBefore
  $rest = Get-RestSeconds $Reply "other"
  Assert-That ($rest -eq $restWant) "${Who} «$Period»: «$($script:OtherTime)» в «Прочем» $rest с, ждали $restWant с (было $(Get-RestSeconds $Before 'other') + строка tool.exe $toolBefore); $text"
  $restRow = @(@(List $Reply.programs) | Where-Object { -not $_.path -and [string]$_.category -eq "other" })
  Assert-That ([string]$restRow[0].name -eq $script:OtherTime) "${Who} «$Period»: строка без программы «$($restRow[0].name)», ждали «$($script:OtherTime)»; $text"
  foreach ($c in @(@{ Cat = "games"; Key = "gamesS" }, @{ Cat = "study"; Key = "studyS" }, @{ Cat = "other"; Key = "otherS" })) {
    $sum = Measure-Rows $Reply $c.Cat
    Assert-That ($sum -eq [int]$Reply.($c.Key)) "${Who} «$Period»: сумма строк «$($c.Cat)» $sum с, итог $($Reply.($c.Key)) с; $text"
  }
}

Invoke-Scenario "S44" "Запрещённая tool.exe не видна в «Итогах» за «Сегодня» и «Неделю», её минуты в «Остальном времени»" {
  Initialize-Scenario 60 0 "2026-10-21T15:00:00"

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовая программа tool.exe в папке наблюдения" {
    $script:ToolPath = New-TestApp "tool"
  }

  Step "Настройка: лимит 2 ч, игры без отдельного лимита; tool.exe «Прочее»" {
    Start-Child 120 $true -1 @(@{ path = $script:ToolPath; category = "other" })
    Wait-ProgramsReady
    $script:ToolName = Get-ProgramName $script:ToolPath
    $script:ChildId = Use-Parent { Get-ChildId }
    Write-Log "Название tool.exe из списка программ: «$($script:ToolName)»"
  }

  Step "Часы ядра: среда 21.10.2026" {
    $day = [DateTimeOffset]::FromUnixTimeMilliseconds((Get-CoreNowMs)).LocalDateTime.ToString("yyyy-MM-dd")
    Assert-That ($day -eq "2026-10-21") "Часы ядра показывают $day, ждали 2026-10-21 (ClockStart)"
  }

  Step "Прошлый день: пн 19.10 tool.exe 10 мин «Прочего» (TestSeedDay)" {
    $d = @{ childId = $script:ChildId; day = "2026-10-19"; totalS = 600; gamesS = 0; studyS = 0; usage = @(@{ path = $script:ToolPath; category = "other"; seconds = 600 }) }
    Assert-Reply (Send-Pcb "TestSeedDay" $d) "TestSeedDay 2026-10-19"
  }

  Step "Сегодня tool.exe 5 мин на переднем плане" {
    $script:Tool = Start-TestApp $script:ToolPath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Tool "tool.exe («Прочее»)"
    Set-TestForeground $script:Tool
    Start-Sleep -Seconds 2 # часы на паузе: служба получает снимок стража уже с этой программой
    Resume-Clock
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($true) {
      $m = Get-MyToday "today"
      $row = @(Find-PathRows $m $script:ToolPath)
      if ($row.Count -eq 1 -and [int]$row[0].seconds -ge 300) { break }
      if ($sw.Elapsed.TotalSeconds -gt 90) {
        Suspend-Clock
        throw "За 90 с не набралось 300 с у tool.exe; $(Format-Totals $m); экран: $(Format-GuardState (Get-GuardState))"
      }
      Start-Sleep -Milliseconds 40
    }
    Suspend-Clock
    Write-Log ("tool.exe: 300 с часов ядра за {0:N1} с настоящего времени" -f $sw.Elapsed.TotalSeconds)
  }

  Step "До запрета: строка tool.exe в «Прочем» за «Сегодня» и «Неделю» (часы стоят)" {
    $script:Before = @{}
    foreach ($p in $Periods) {
      $r = Get-MyToday $p
      Write-Log "MyToday «$p» до запрета: $(Format-Totals $r)"
      $row = @(Find-PathRows $r $script:ToolPath)
      Assert-That ($row.Count -eq 1 -and [string]$row[0].category -eq "other") "«$p» до запрета: строк tool.exe $($row.Count), ждали одну в «Прочем» (иначе проверка ничего не проверяет); $(Format-Totals $r)"
      Assert-That ([string]$row[0].name -eq $script:ToolName) "«$p» до запрета: название tool.exe «$($row[0].name)», ждали «$($script:ToolName)» (как в списке программ)"
      $script:Before[$p] = $r
    }
    $today = [int](@(Find-PathRows ($script:Before["today"]) $script:ToolPath)[0].seconds)
    $week = [int](@(Find-PathRows ($script:Before["week"]) $script:ToolPath)[0].seconds)
    Assert-That ($today -ge 300 -and $week -eq 600 + $today) "До запрета строка tool.exe: сегодня $today с, неделя $week с; ждали сегодня ≥ 300 и неделя = 600 + сегодня"
  }

  Step "Родитель запрещает tool.exe" {
    $rep = Set-ProgramCategory $script:ToolPath "blocked"
    Assert-That ([string]$rep.message -ne "") "Запрет tool.exe без сообщения: $($rep | ConvertTo-Json -Compress)"
  }

  Step "«Моя статистика» (MyToday) за «Сегодня» и «Неделю»: строки tool.exe нет, её секунды в «Остальном времени»" {
    $script:Mine = @{}
    foreach ($p in $Periods) {
      $r = Get-MyToday $p
      Assert-NoBlocked $r $script:Before[$p] $p "MyToday"
      $script:Mine[$p] = $r
    }
  }

  Step "Статистика родителя (ParentStats) за «Сегодня» и «Неделю»: то же, что у ребёнка" {
    Use-Parent {
      foreach ($p in $Periods) {
        $r = Send-Parent "ParentStats" @{ childId = $script:ChildId; period = $p }
        Assert-Reply $r "ParentStats $p"
        Assert-NoBlocked $r.payload $script:Before[$p] $p "ParentStats"
        Assert-That ((Format-Rows $r.payload) -eq (Format-Rows $script:Mine[$p])) "«$p»: у родителя другие строки (часы стоят): $(Format-Rows $r.payload) | у ребёнка: $(Format-Rows $script:Mine[$p])"
      }
    }
  }

  Step "«Программы»: tool.exe видна как «Запрещено»" {
    $list = Get-ProgramsList
    $norm = $script:ToolPath.ToLowerInvariant()
    $row = @(@(List $list.programs) | Where-Object { ([string]$_.path) -eq $norm })
    Assert-That ($row.Count -eq 1 -and [string]$row[0].category -eq "blocked" -and $row[0].hidden -ne $true) "tool.exe в «Программах»: $($row | ConvertTo-Json -Compress), ждали одну строку blocked, не скрытую"
  }

  Step "Снимок «Моя статистика»" {
    [void](Save-ProgramWindow "s44-stats" @("--mystats") 40 5)
  }

  Write-Log "S44: запрещённая tool.exe не видна в «Итогах» у ребёнка и родителя, её минуты в «Остальном времени», итоги прежние"
}
