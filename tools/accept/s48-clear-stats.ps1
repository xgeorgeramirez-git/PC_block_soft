# S48. «Очистить статистику» и «Очистить журнал» у одного ребёнка (FR-38, D-73, D-79, RM5-8, RM5-9, RM5-10).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60) и начинаются в среду 25.11.2026 15:00 (ClockStart).
# Дано: Миша (учётка раннера, его сеанс) и Коля (созданный обычный пользователь pcbtest), лимит 2 ч каждому;
# 3 недели прошлых дней 04.11…24.11 (TestSeedDay): Миша по 30 мин (из них YouTube 10 мин), Коля по 20 мин
# (YouTube 5 мин); в журнале у обоих записи (копилка от родителя); Миша сегодня 40 мин — остаток 1 ч 20 мин.
# Когда: родитель «Очистить статистику» у Миши, затем минута активности, затем «Очистить журнал» у Миши,
# затем перезапуск службы.
# Тогда: у Миши «Итоги» за все пять периодов 0, «Статистика ведётся с 25 ноября 2026»; остаток прежний 1 ч 20 мин,
# через минуту — 1 ч 19 мин, а «Итоги» за сегодня 1 мин (то же за все периоды); у Коли статистика и журнал прежние;
# в журнале Миши одна запись «Журнал очищен»; после перезапуска остаток и «Итоги» Миши те же (снимок в базе).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$Periods = @("today", "week", "month", "year", "all")
$Today = "2026-11-25"

function Format-Stats($R) {
  $bars = (@(List $R.bars) | ForEach-Object { "$($_.key)=$($_.totalS)/$($_.studyS)" }) -join ','
  $rows = (@(List $R.programs) | ForEach-Object { "$($_.kind)/$($_.category):$($_.path)=$($_.seconds)" }) -join ','
  return "период $($R.period): исп $($R.usedS), игры $($R.gamesS), учёба $($R.studyS), видео $($R.videoS), прочее $($R.otherS), всего $($R.allS), доп $($R.extraMin), из копилки $($R.bonusSpentMin); столбики [$bars]; строки [$rows]; с $($R.statsSinceDay)"
}

function Get-ParentPeriod([int64]$ChildId, [string]$Period) {
  $r = Send-Parent "ParentStats" @{ childId = $ChildId; period = $Period }
  Assert-Reply $r "ParentStats $ChildId $Period"
  return $r.payload
}

function Get-ChildEvents([int64]$ChildId) {
  $r = Send-Parent "ListEvents" @{ childId = $ChildId; limit = 1000 }
  Assert-Reply $r "ListEvents $ChildId"
  return @(List $r.payload.events)
}

# «Итоги» Миши за все периоды равны Want секунд (после очистки все периоды — только сегодня с очистки).
function Assert-MishaTotals([int]$Want, [string]$When, [int]$Tolerance = 0) {
  foreach ($p in $Periods) {
    $m = Get-MyToday $p
    Write-Log "Миша «$p» ${When}: $(Format-Stats $m)"
    Assert-That ([math]::Abs([int]$m.usedS - $Want) -le $Tolerance -and [math]::Abs([int]$m.allS - $Want) -le $Tolerance) "Миша «$p» ${When}: использовано $($m.usedS) с, всего $($m.allS) с, ждали $Want (допуск $Tolerance)"
    foreach ($k in @("gamesS", "studyS", "videoS", "extraMin", "bonusSpentMin")) {
      Assert-That ([int]$m.$k -eq 0) "Миша «$p» ${When}: $k = $($m.$k), ждали 0"
    }
    $sum = 0
    foreach ($b in @(List $m.bars)) { $sum += [int]$b.totalS }
    Assert-That ($sum -eq [int]$m.usedS) "Миша «$p» ${When}: сумма столбиков $sum с, «Итоги» $($m.usedS) с"
    $yt = @(@(List $m.programs) | Where-Object { [string]$_.kind -eq "site" })
    Assert-That ($yt.Count -eq 0) "Миша «$p» ${When}: в «Сайтах» остались строки: $(($yt | ForEach-Object { "$($_.path)=$($_.seconds)" }) -join ', ')"
    Assert-That ([string]$m.statsSinceDay -eq $Today -and [string]$m.statsSinceText -eq "Статистика ведётся с 25 ноября 2026") "Миша «$p» ${When}: «$($m.statsSinceDay)» «$($m.statsSinceText)», ждали $Today и «Статистика ведётся с 25 ноября 2026»"
  }
}

# Остаток в минутах, как его показывают окна (неполная минута — целая, RemainingMinutes).
function Get-RemainMin([int]$Seconds) { return [int][math]::Ceiling($Seconds / 60) }

Invoke-Scenario "S48" "Очистка статистики и журнала Миши: «Итоги» с нуля, остаток прежний, у Коли всё на месте" {
  Initialize-Scenario 60 0 "2026-11-25T15:00:00"

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
    $day = [DateTimeOffset]::FromUnixTimeMilliseconds((Get-CoreNowMs)).LocalDateTime.ToString("yyyy-MM-dd")
    Assert-That ($day -eq $Today) "Часы ядра показывают $day, ждали $Today (ClockStart)"
  }

  Step "Три недели прошлых дней у обоих (TestSeedDay) и записи журнала у обоих" {
    $start = [datetime]::ParseExact("2026-11-04", "yyyy-MM-dd", $null)
    for ($i = 0; $i -lt 21; $i++) {
      $d = $start.AddDays($i).ToString("yyyy-MM-dd")
      Assert-Reply (Send-Pcb "TestSeedDay" @{ childId = $script:Misha; day = $d; totalS = 1800; gamesS = 0; videoS = 600; studyS = 0
          usage = @(@{ path = "youtube.com"; category = "video_social"; seconds = 600; kind = "site" }) }) "TestSeedDay Миша $d"
      Assert-Reply (Send-Pcb "TestSeedDay" @{ childId = $script:Kolya; day = $d; totalS = 1200; gamesS = 0; videoS = 300; studyS = 0
          usage = @(@{ path = "youtube.com"; category = "video_social"; seconds = 300; kind = "site" }) }) "TestSeedDay Коля $d"
    }
    Use-Parent {
      Assert-Reply (Send-Parent "GrantBonus" @{ childId = $script:Misha; minutes = 10; reason = "Мише за уборку" }) "GrantBonus Мише"
      Assert-Reply (Send-Parent "GrantBonus" @{ childId = $script:Kolya; minutes = 20; reason = "Коле за уборку" }) "GrantBonus Коле"
      $script:KolyaEvents = @(Get-ChildEvents $script:Kolya).Count
      $mishaEvents = @(Get-ChildEvents $script:Misha).Count
      Write-Log "Записей журнала: у Миши $mishaEvents, у Коли $($script:KolyaEvents)"
      Assert-That ($mishaEvents -ge 1 -and $script:KolyaEvents -ge 1) "В журнале нет записей одного из детей: Миша $mishaEvents, Коля $($script:KolyaEvents)"
      # Статистика Коли до очистки у Миши: литералы — 21 день по 1200 с, в неделе пн 23 и вт 24.
      $script:KolyaBefore = @{}
      $want = @{ today = 0; week = 2400; month = 25200; year = 25200; all = 25200 }
      foreach ($p in $Periods) {
        $k = Get-ParentPeriod $script:Kolya $p
        Assert-That ([int]$k.usedS -eq $want[$p]) "Коля «$p»: $($k.usedS) с, ждали $($want[$p])"
        $script:KolyaBefore[$p] = Format-Stats $k
      }
    }
  }

  Step "Миша сегодня 40 мин: остаток 1 ч 20 мин" {
    Resume-Clock
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($true) {
      $st = Get-Status
      if ([int]$st.usedS -ge 2400) { break }
      if ($sw.Elapsed.TotalSeconds -gt 120) { Suspend-Clock; throw "За 120 с не набралось 40 мин: учтено $($st.usedS) с" }
      Start-Sleep -Milliseconds 40
    }
    Suspend-Clock
    $st = Get-Status
    $script:UsedAtClear = [int]$st.usedS
    $script:RemainAtClear = [int]$st.remainingS
    Write-Log "Миша: учтено $($st.usedS) с, остаток $($st.remainingS) с ($(Get-RemainMin $st.remainingS) мин)"
    Assert-That ((Get-RemainMin $st.remainingS) -eq 80) "Остаток Миши $($st.remainingS) с, ждали 1 ч 20 мин"
    $w = Get-MyToday "all"
    Assert-That ([int]$w.usedS -eq 21 * 1800 + $script:UsedAtClear) "Миша за всё время до очистки $($w.usedS) с, ждали $(21 * 1800 + $script:UsedAtClear)"
    Assert-That ([string]$w.statsSinceDay -eq "2026-11-04") "«Статистика ведётся с» до очистки: $($w.statsSinceDay), ждали 2026-11-04 (первый день)"
  }

  Step "«Очистить статистику» у Миши: «Итоги» за все периоды 0, остаток прежний" {
    Use-Parent {
      $r = Send-Parent "ClearStats" @{ childId = $script:Misha }
      Assert-Reply $r "ClearStats Миши"
      Write-Log "ClearStats: «$($r.payload.message)»"
      Assert-That ([string]$r.payload.message -eq "Статистика Миши очищена") "Сообщение «$($r.payload.message)», ждали «Статистика Миши очищена»"
    }
    Assert-MishaTotals 0 "после очистки"
    $st = Get-Status
    Assert-That ([int]$st.remainingS -eq $script:RemainAtClear) "Остаток после очистки $($st.remainingS) с, ждали прежние $($script:RemainAtClear) с (D-73: учёт лимита не меняется)"
    Use-Parent {
      $card = @(@(List (Get-Today).children) | Where-Object { [int64]$_.childId -eq $script:Misha })[0]
      Write-Log "Карточка Миши у родителя: остаток $($card.status.remainingS) с, «$($card.status.statusText)»"
      Assert-That ([int]$card.status.remainingS -eq $script:RemainAtClear) "У родителя остаток Миши $($card.status.remainingS) с, ждали $($script:RemainAtClear)"
      foreach ($p in $Periods) {
        $k = Format-Stats (Get-ParentPeriod $script:Kolya $p)
        Assert-That ($k -eq $script:KolyaBefore[$p]) "Статистика Коли «$p» изменилась: $k | была: $($script:KolyaBefore[$p])"
      }
    }
  }

  Step "Минута активности: остаток 1 ч 19 мин, «Итоги» за сегодня 1 мин" {
    Resume-Clock
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($true) {
      $st = Get-Status
      if ([int]$st.usedS -ge $script:UsedAtClear + 60) { break }
      if ($sw.Elapsed.TotalSeconds -gt 30) { Suspend-Clock; throw "За 30 с не набралась минута: учтено $($st.usedS) с, было $($script:UsedAtClear)" }
      Start-Sleep -Milliseconds 40
    }
    Suspend-Clock
    $st = Get-Status
    $since = [int]$st.usedS - $script:UsedAtClear
    $script:RemainAfter = [int]$st.remainingS
    Write-Log "После минуты: учтено $($st.usedS) с (с очистки $since с), остаток $($st.remainingS) с ($(Get-RemainMin $st.remainingS) мин)"
    Assert-That ((Get-RemainMin $st.remainingS) -eq 79) "Остаток Миши $($st.remainingS) с, ждали 1 ч 19 мин"
    Assert-That ([math]::Floor($since / 60) -eq 1) "С очистки учтено $since с, ждали 60…119"
    Assert-MishaTotals $since "через минуту"
    $script:SinceClear = $since
  }

  Step "«Очистить журнал» у Миши: одна запись «Журнал очищен», журнал Коли прежний" {
    Use-Parent {
      $r = Send-Parent "ClearJournal" @{ childId = $script:Misha }
      Assert-Reply $r "ClearJournal Миши"
      Write-Log "ClearJournal: «$($r.payload.message)»"
      Assert-That ([string]$r.payload.message -eq "Журнал Миши очищен") "Сообщение «$($r.payload.message)», ждали «Журнал Миши очищен»"
      $m = @(Get-ChildEvents $script:Misha)
      Write-Log "Журнал Миши: $(($m | ForEach-Object { "$($_.kind): $($_.text)" }) -join ' | ')"
      Assert-That ($m.Count -eq 1 -and [string]$m[0].kind -eq "journal_cleared" -and [string]$m[0].text -match "Журнал очищен") "В журнале Миши $($m.Count) записей, ждали одну «Журнал очищен»"
      $k = @(Get-ChildEvents $script:Kolya).Count
      Assert-That ($k -eq $script:KolyaEvents) "В журнале Коли $k записей, было $($script:KolyaEvents)"
      foreach ($p in $Periods) {
        $s = Format-Stats (Get-ParentPeriod $script:Kolya $p)
        Assert-That ($s -eq $script:KolyaBefore[$p]) "Статистика Коли «$p» изменилась: $s | была: $($script:KolyaBefore[$p])"
      }
    }
  }

  Step "Перезапуск службы: остаток и «Итоги» Миши те же (снимок очистки в базе)" {
    Restart-PcbService
    Wait-Until { try { (Get-Status).status -ne "notChild" } catch { Close-PipeConn; $false } } 40 "служба снова считает Мишу"
    $st = Get-Status
    Write-Log "После перезапуска: учтено $($st.usedS) с, остаток $($st.remainingS) с (до перезапуска $($script:RemainAfter) с)"
    # База отстаёт от ядра не больше чем на 30 с (G10), а до повторной паузы часы ядра шли доли секунды.
    Assert-That ([math]::Abs([int]$st.remainingS - $script:RemainAfter) -le 30) "Остаток после перезапуска $($st.remainingS) с, до $($script:RemainAfter) с (допуск 30 с)"
    $now = [int]$st.usedS - $script:UsedAtClear
    Assert-MishaTotals $now "после перезапуска" 0
    Assert-That ([math]::Abs($now - $script:SinceClear) -le 30) "«Итоги» сегодня после перезапуска $now с, до $($script:SinceClear) с (допуск 30 с)"
  }

  Write-Log "S48: статистика и журнал Миши очищены, остаток не изменился, у Коли всё прежнее; после перезапуска так же"
}
