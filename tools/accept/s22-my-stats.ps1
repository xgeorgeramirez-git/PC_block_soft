# S22. «Моя статистика» ребёнка: те же цифры о себе, что у родителя; нет данных второго ребёнка; статистика
# родителя, очистка статистики и журнала из сеанса ребёнка без пароля не открываются (R21, FR-12, FR-38).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60) и начинаются в среду 21.10.2026 15:00, сразу на паузе.
# Дано: два ребёнка — Миша (учётка раннера, его сеанс) и Коля (созданный обычный пользователь pcbtest).
# Прошлые дни (TestSeedDay): Миша пн 19.10 30 мин; Коля пн 19.10 50 мин и вт 20.10 20 мин. Копилка: Мише +10 мин
# «Мише за уборку», Коле +25 мин «Коле за пятёрку». Нарушение у Миши — значок завершён принудительно (как S5).
# Ruling T10-S22: нарушение второму ребёнку в CI не сделать (второго интерактивного сеанса у раннера нет, обход
# пишется только ребёнку своего сеанса); поэтому разделение нарушений проверяется в обе стороны так: у Миши его
# нарушение есть, у Коли (ParentStats) нарушений нет — нарушение Миши к Коле не попало; копилка — у обоих.
# Тогда: MyToday за пять периодов совпадает с ParentStats Миши (итоги, столбики, нарушения, копилка, «Статистика
# ведётся с»); в ответе ребёнка нет копилки и дней Коли; лишнее поле childId Коли в запросе ничего не меняет;
# ParentStats, ClearStats и ClearJournal без родительской сессии — отказ, данные целы.
# Снимок: s22-my-stats.png (окно «Моя статистика» → visual-reviewer: нет полей ввода, выбора ребёнка и кнопок очистки).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$Periods = @("today", "week", "month", "year", "all")

# Ответ статистики одной строкой для сравнения ребёнка и родителя (часы стоят — ответы должны совпасть).
function Format-Stats($R) {
  $bars = (@(List $R.bars) | ForEach-Object { "$($_.key)=$($_.totalS)/$($_.studyS)" }) -join ','
  $viol = (@(List $R.violations) | ForEach-Object { "$($_.childId):$($_.kind)@$($_.atMs)" }) -join ','
  $bonus = (@(List $R.bonus) | ForEach-Object { "$($_.deltaMin):$($_.reason)" }) -join ','
  $rows = (@(List $R.programs) | ForEach-Object { "$($_.kind)/$($_.category):$($_.path)=$($_.seconds)" }) -join ','
  return "период $($R.period): исп $($R.usedS), игры $($R.gamesS), учёба $($R.studyS), видео $($R.videoS), прочее $($R.otherS), всего $($R.allS), доп $($R.extraMin), из копилки $($R.bonusSpentMin); столбики [$bars]; нарушения [$viol]; копилка [$bonus]; строки [$rows]; с $($R.statsSinceDay)"
}

function Get-ParentPeriod([int64]$ChildId, [string]$Period) {
  $r = Send-Parent "ParentStats" @{ childId = $ChildId; period = $Period }
  Assert-Reply $r "ParentStats $ChildId $Period"
  return $r.payload
}

Invoke-Scenario "S22" "«Моя статистика»: свои цифры, как у родителя; чужих нет; без пароля статистики родителя нет" {
  Initialize-Scenario 60 0 "2026-10-21T15:00:00"

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: Миша (учётка раннера) и Коля (pcbtest), лимит 2 ч каждому; часы на паузе" {
    $kolyaSid = New-TestUser
    $limits = @(1..7 | ForEach-Object { New-DayLimit $_ 120 })
    Start-Input 1000
    $r = Send-Pcb "Setup" @{ password = $script:ParentPassword; children = @(
        @{ sid = $script:MySid; name = "Миша"; limits = @($limits) },
        @{ sid = $kolyaSid; name = "Коля"; limits = @($limits) }) }
    Assert-Reply $r "Первичная настройка на двух детей"
    Suspend-Clock
    Wait-SessionParts
    Start-Sleep -Seconds 3
    Use-Parent {
      foreach ($c in @(List (Get-Today).children)) {
        if ($c.name -eq "Миша") { $script:Misha = [int64]$c.childId }
        if ($c.name -eq "Коля") { $script:Kolya = [int64]$c.childId }
      }
    }
    Assert-That ($script:Misha -gt 0 -and $script:Kolya -gt 0 -and $script:Misha -ne $script:Kolya) "Дети не найдены: Миша $($script:Misha), Коля $($script:Kolya)"
    Write-Log "Миша — ребёнок $($script:Misha) (сеанс раннера), Коля — ребёнок $($script:Kolya) (pcbtest)"
  }

  Step "Прошлые дни (TestSeedDay) и копилка обоих" {
    $days = @(
      @{ childId = $script:Misha; day = "2026-10-19"; totalS = 1800; gamesS = 0; studyS = 0; usage = @() },
      @{ childId = $script:Kolya; day = "2026-10-19"; totalS = 3000; gamesS = 0; studyS = 0; usage = @() },
      @{ childId = $script:Kolya; day = "2026-10-20"; totalS = 1200; gamesS = 0; studyS = 0; usage = @() })
    foreach ($d in $days) { Assert-Reply (Send-Pcb "TestSeedDay" $d) "TestSeedDay $($d.childId) $($d.day)" }
    Use-Parent {
      Assert-Reply (Send-Parent "GrantBonus" @{ childId = $script:Misha; minutes = 10; reason = "Мише за уборку" }) "GrantBonus Мише"
      Assert-Reply (Send-Parent "GrantBonus" @{ childId = $script:Kolya; minutes = 25; reason = "Коле за пятёрку" }) "GrantBonus Коле"
    }
  }

  Step "Нарушение у Миши: значок завершён принудительно, запись «попытка обойти защиту»" {
    $old = @(Get-SessionProcs "--tray")
    Assert-That ($old.Count -eq 1) "Значков в сеансе: $($old.Count), ждали один"
    & taskkill.exe /F /PID ([int]$old[0].ProcessId) 2>&1 | ForEach-Object { Write-Log "taskkill: $_" }
    Wait-Until { @(Get-SessionProcs "--tray").Count -eq 1 } 15 "новый значок в сеансе"
    Use-Parent {
      # Запись откладывается на 5 с (FB-1): ждём её до 15 с в группе «Нарушения» журнала Миши.
      Wait-Until {
        $r = Send-Parent "ListEvents" @{ childId = $script:Misha; group = "violations"; limit = 50 }
        $r.ok -and @(List $r.payload.events).Count -ge 1
      } 15 "нарушение Миши в журнале"
    }
  }

  Step "MyToday за пять периодов: свои цифры, как ParentStats Миши; копилки и дней Коли нет" {
    $script:Mine = @{}
    $script:TodayUsed = [int](Get-MyToday "today").usedS
    Write-Log "Сегодня у Миши из ядра: $($script:TodayUsed) с"
    foreach ($p in $Periods) {
      $sw = [Diagnostics.Stopwatch]::StartNew()
      $m = Get-MyToday $p
      $ms = $sw.ElapsedMilliseconds
      Assert-That ($ms -le 1000) "MyToday за «$p» отвечал $ms мс, ждали не дольше 1000 мс"
      $script:Mine[$p] = Format-Stats $m
      Write-Log "MyToday: $($script:Mine[$p])"
      $reasons = @(List $m.bonus | ForEach-Object { [string]$_.reason })
      Assert-That ($reasons -contains "Мише за уборку") "MyToday «$p»: в копилке нет «Мише за уборку»: $($reasons -join '; ')"
      Assert-That (-not ($reasons -contains "Коле за пятёрку")) "MyToday «$p»: в ответе ребёнка копилка Коли: $($reasons -join '; ')"
      $viol = @(List $m.violations)
      Assert-That ($viol.Count -ge 1) "MyToday «$p»: нет нарушения Миши"
      foreach ($v in $viol) {
        Assert-That ([int64]$v.childId -eq $script:Misha) "MyToday «$p»: нарушение другого ребёнка ($($v.childId)): $($v.text)"
      }
      if ($p -ne "today") {
        # Прошлые дни — только пн 19.10 Миши (30 мин); у Коли 50 + 20 мин.
        $want = 1800 + $script:TodayUsed
        Assert-That ([int]$m.usedS -eq $want) "MyToday «$p»: использовано $($m.usedS) с, ждали $want (1800 с Миши + сегодня $($script:TodayUsed)); 4200 с сверх — дни Коли"
      }
      Assert-That ([string]$m.statsSinceDay -ne "") "MyToday «$p»: пустое «Статистика ведётся с»"
    }
    $week = Get-MyToday "week"
    $mon = @(@(List $week.bars) | Where-Object { $_.key -eq "2026-10-19" })
    Assert-That ($mon.Count -eq 1 -and [int]$mon[0].totalS -eq 1800) "Столбик пн 19 у Миши: $(if ($mon.Count) { $mon[0].totalS } else { 'нет' }) с, ждали 1800 (у Коли 3000)"
    $tue = @(@(List $week.bars) | Where-Object { $_.key -eq "2026-10-20" })
    Assert-That ($tue.Count -eq 1 -and [int]$tue[0].totalS -eq 0) "Столбик вт 20 у Миши: $(if ($tue.Count) { $tue[0].totalS } else { 'нет' }) с, ждали 0 (20 мин — у Коли)"
  }

  Step "Те же цифры у родителя; у Коли свои цифры и копилка, нарушения Миши к нему не попали" {
    Use-Parent {
      foreach ($p in $Periods) {
        $r = Get-ParentPeriod $script:Misha $p
        $s = Format-Stats $r
        Assert-That ($s -eq $script:Mine[$p]) "«$p»: у родителя другие цифры Миши (часы стоят): $s | у ребёнка: $($script:Mine[$p])"
      }
      $k = Get-ParentPeriod $script:Kolya "all"
      Write-Log "ParentStats Коли: $(Format-Stats $k)"
      Assert-That ([int]$k.usedS -eq 4200) "У Коли за всё время $($k.usedS) с, ждали 4200 (50 + 20 мин)"
      Assert-That (@(List $k.violations).Count -eq 0) "У Коли нарушения: $((@(List $k.violations) | ForEach-Object { $_.text }) -join '; ') (нарушение Миши попало к Коле)"
      $kr = @(List $k.bonus | ForEach-Object { [string]$_.reason })
      Assert-That (($kr -contains "Коле за пятёрку") -and -not ($kr -contains "Мише за уборку")) "Копилка Коли: $($kr -join '; '), ждали только «Коле за пятёрку»"
    }
  }

  Step "Лишнее поле childId Коли в запросе ребёнка ничего не меняет" {
    foreach ($p in @("today", "all")) {
      $r = Send-Pcb "MyToday" @{ period = $p; childId = $script:Kolya }
      Assert-Reply $r "MyToday с чужим childId"
      $s = Format-Stats $r.payload
      Assert-That ($s -eq $script:Mine[$p]) "MyToday «$p» с childId Коли отдал другое: $s | своё: $($script:Mine[$p])"
    }
  }

  Step "Без родительской сессии: ParentStats, ClearStats и ClearJournal — отказ, данные целы" {
    $script:Token = ""
    Use-Parent {
      $script:EventsBefore = @(List (Send-Parent "ListEvents" @{ childId = $script:Misha; limit = 1000 }).payload.events).Count
    }
    $script:Token = ""
    $checks = @(
      @{ Cmd = "ParentStats"; Payload = @{ childId = $script:Kolya; period = "all" } },
      @{ Cmd = "ParentStats"; Payload = @{ childId = $script:Misha; period = "all" } },
      @{ Cmd = "ClearStats"; Payload = @{ childId = $script:Misha } },
      @{ Cmd = "ClearJournal"; Payload = @{ childId = $script:Misha } },
      @{ Cmd = "ClearStats"; Payload = @{ childId = $script:Kolya } })
    foreach ($c in $checks) {
      $r = Send-Pcb $c.Cmd $c.Payload
      Write-Log "$($c.Cmd) $($c.Payload.childId) без пароля: ok=$($r.ok), «$($r.error)»"
      Assert-That (-not $r.ok) "$($c.Cmd) для ребёнка $($c.Payload.childId) прошёл без родительской сессии"
    }
    foreach ($p in $Periods) {
      $s = Format-Stats (Get-MyToday $p)
      Assert-That ($s -eq $script:Mine[$p]) "После отказов статистика Миши «$p» изменилась: $s | была: $($script:Mine[$p])"
    }
    Use-Parent {
      $after = @(List (Send-Parent "ListEvents" @{ childId = $script:Misha; limit = 1000 }).payload.events).Count
      Assert-That ($after -eq $script:EventsBefore) "Журнал Миши: было $($script:EventsBefore) записей, стало $after (очистка без пароля?)"
      $k = Get-ParentPeriod $script:Kolya "all"
      Assert-That ([int]$k.usedS -eq 4200) "У Коли после отказов $($k.usedS) с, ждали 4200"
    }
  }

  Step "Снимок «Моя статистика»" {
    [void](Save-ProgramWindow "s22-my-stats" @("--mystats") 40 5)
  }

  Write-Log "S22: ребёнок видит только себя и те же цифры, что родитель; без пароля статистики и очистки нет"
}
