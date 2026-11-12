# S47. «По дням» за «Год» и «Всё время» — столбик на месяц, месяц без данных — пустой столбик; за «Неделю» —
# столбик на день; у столбиков только время и учёба, без линий лимита и отметок «сверх лимита» (D-69, D-78).
# Часы ядра идут в 60 раз быстрее и начинаются в среду 18.11.2026 15:00 (ClockStart), сразу на паузе: сегодня
# почти ничего не набирается. Прошлые дни — тестовой командой TestSeedDay: 20.12.2025 — 40 мин, 15.01.2026 —
# 60 мин, 10.03.2026 — 30 мин и учёба 20 мин, 01.06.2026 — 45 мин, пн 16.11.2026 — 20 мин; в феврале данных нет.
# Тогда: «Год» — 11 столбиков янв…ноя, «фев» пустой, «мар» 30 мин и учёба 20 мин; «Всё время» — 12 столбиков с
# «дек 2025» (подписи с годом); «Неделя» — пн 16, вт 17, ср 18, у пн 20 мин. Снимок: s47-my-stats.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Поля столбика, которые отдаёт служба (RM5-3).
$BarFields = @("key", "label", "title", "totalS", "studyS", "today")

function Format-Bars($Reply) {
  return (@(List $Reply.bars) | ForEach-Object { "$($_.key) «$($_.label)» $($_.totalS)/$($_.studyS)$(if ($_.today) { ' сегодня' })" }) -join ' | '
}

# Проверка ряда столбиков: ключи, подписи и (для прошлых) секунды — по ожиданию Want: @(@{key; label; totalS; studyS}).
# У сегодняшнего столбика секунды — из ядра (Live), остальные — литералы.
function Assert-Bars($Reply, [string]$Period, [string]$By, $Want, $Live) {
  $bars = @(List $Reply.bars)
  Write-Log "«$Period» ($($Reply.barsBy)): $(Format-Bars $Reply)"
  Assert-That ([string]$Reply.barsBy -eq $By) "«$Period»: разбивка «$($Reply.barsBy)», ждали $By"
  Assert-That ($bars.Count -eq $Want.Count) "«$Period»: столбиков $($bars.Count), ждали $($Want.Count): $(Format-Bars $Reply)"
  for ($i = 0; $i -lt $Want.Count; $i++) {
    $b = $bars[$i]; $w = $Want[$i]
    $fields = @($b.PSObject.Properties.Name | Sort-Object)
    Assert-That (($fields -join ',') -eq (($BarFields | Sort-Object) -join ',')) "«$Period» $($w.key): поля $($fields -join ', '), ждали только $($BarFields -join ', ') (D-78)"
    Assert-That ([string]$b.key -eq $w.key -and [string]$b.label -eq $w.label) "«$Period» столбик ${i}: «$($b.key)» «$($b.label)», ждали «$($w.key)» «$($w.label)»"
    $isLast = ($i -eq $Want.Count - 1)
    Assert-That ([bool]$b.today -eq $isLast) "«$Period» $($w.key): today=$($b.today), ждали $isLast"
    $total = [int]$w.totalS + $(if ($isLast) { [int]$Live.usedS } else { 0 })
    $study = [int]$w.studyS + $(if ($isLast) { [int]$Live.studyS } else { 0 })
    Assert-That ([int]$b.totalS -eq $total -and [int]$b.studyS -eq $study) "«$Period» $($w.key): $($b.totalS)/$($b.studyS) с, ждали $total/$study"
  }
}

function New-Bar([string]$Key, [string]$Label, [int]$TotalMin = 0, [int]$StudyMin = 0) {
  return @{ key = $Key; label = $Label; totalS = 60 * $TotalMin; studyS = 60 * $StudyMin }
}

Invoke-Scenario "S47" "«Год» и «Всё время» по месяцам с пустым февралём, «Неделя» по дням, без лимита" {
  Initialize-Scenario 60 0 "2026-11-18T15:00:00"

  Step "Настройка: лимит 2 ч, часы ядра на паузе" {
    Start-Child 120 $false
    $script:ChildId = Use-Parent { Get-ChildId }
    $day = [DateTimeOffset]::FromUnixTimeMilliseconds((Get-CoreNowMs)).LocalDateTime.ToString("yyyy-MM-dd")
    Assert-That ($day -eq "2026-11-18") "Часы ядра показывают $day, ждали 2026-11-18 (ClockStart)"
  }

  Step "Прошлые дни подложены тестовой командой TestSeedDay (в феврале ничего)" {
    $id = $script:ChildId
    $days = @(
      @{ childId = $id; day = "2025-12-20"; totalS = 2400; gamesS = 0; studyS = 0; usage = @() },
      @{ childId = $id; day = "2026-01-15"; totalS = 3600; gamesS = 0; studyS = 0; usage = @() },
      @{ childId = $id; day = "2026-03-10"; totalS = 1800; gamesS = 0; studyS = 1200; usage = @() },
      @{ childId = $id; day = "2026-06-01"; totalS = 2700; gamesS = 0; studyS = 0; usage = @() },
      @{ childId = $id; day = "2026-11-16"; totalS = 1200; gamesS = 0; studyS = 0; usage = @() })
    foreach ($d in $days) {
      Assert-Reply (Send-Pcb "TestSeedDay" $d) "TestSeedDay $($d.day)"
    }
  }

  Step "Столбики «Года», «Всего времени» и «Недели» у ребёнка (MyToday) и у родителя (ParentStats)" {
    $live = Get-MyToday "today"
    Write-Log "Сегодня из ядра: $($live.usedS) с, учёба $($live.studyS) с"
    Assert-That ([int]$live.usedS -le 600) "Сегодня набралось $($live.usedS) с при часах на паузе, ждали почти 0"
    $year = @(
      (New-Bar "2026-01" "янв" 60), (New-Bar "2026-02" "фев"), (New-Bar "2026-03" "мар" 30 20), (New-Bar "2026-04" "апр"),
      (New-Bar "2026-05" "май"), (New-Bar "2026-06" "июн" 45), (New-Bar "2026-07" "июл"), (New-Bar "2026-08" "авг"),
      (New-Bar "2026-09" "сен"), (New-Bar "2026-10" "окт"), (New-Bar "2026-11" "ноя" 20))
    $all = @((New-Bar "2025-12" "дек 2025" 40)) + @($year | ForEach-Object { New-Bar $_.key ($_.label + " 2026") ($_.totalS / 60) ($_.studyS / 60) })
    $week = @((New-Bar "2026-11-16" "пн 16" 20), (New-Bar "2026-11-17" "вт 17"), (New-Bar "2026-11-18" "ср 18"))
    $cases = @(
      @{ Period = "year"; By = "month"; Want = $year },
      @{ Period = "all"; By = "month"; Want = $all },
      @{ Period = "week"; By = "day"; Want = $week })
    $mine = @{}
    foreach ($c in $cases) {
      $r = Get-MyToday $c.Period
      Assert-Bars $r $c.Period $c.By $c.Want $live
      $mine[$c.Period] = Format-Bars $r
    }
    $feb = @(List (Get-MyToday "year").bars)[1]
    Assert-That ([int]$feb.totalS -eq 0 -and [string]$feb.title -eq "февраль 2026: 0 мин, учёба 0 мин") "«фев» без данных: $($feb.totalS) с, подсказка «$($feb.title)», ждали пустой столбик «февраль 2026: 0 мин, учёба 0 мин»"
    $mar = @(List (Get-MyToday "year").bars)[2]
    Assert-That ([string]$mar.title -eq "март 2026: 30 мин, учёба 20 мин") "Подсказка «мар»: «$($mar.title)», ждали «март 2026: 30 мин, учёба 20 мин»"
    Use-Parent {
      foreach ($c in $cases) {
        $r = Send-Parent "ParentStats" @{ childId = $script:ChildId; period = $c.Period }
        Assert-Reply $r "ParentStats $($c.Period)"
        Assert-That ((Format-Bars $r.payload) -eq $mine[$c.Period]) "«$($c.Period)»: у родителя другие столбики (часы стоят): $(Format-Bars $r.payload) | у ребёнка: $($mine[$c.Period])"
      }
    }
  }

  Step "Снимок «Моя статистика»" {
    [void](Save-ProgramWindow "s47-my-stats" @("--mystats") 40 5)
  }

  Write-Log "S47: «Год» и «Всё время» — по месяцам, пустой февраль; «Неделя» — по дням; полей лимита у столбиков нет"
}
