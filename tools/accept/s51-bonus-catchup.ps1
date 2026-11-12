# S51. Автобонус за дни, когда полночь прошла при выключенном компьютере (FB-21, R25, D-82, план M5 T7c).
# Выключить компьютер в CI нельзя: «включили в четверг» — перезапуск службы (как S46, без перезаписи TestMode).
# Часы ядра идут как настоящие (ClockSpeed=1) и начинаются в четверг 19.11.2026 09:00 (ClockStart).
# Дано: лимит Миши 120 мин на каждый день, бонус 5 мин за каждые 10 мин (по умолчанию), копилка 0. Вторник 17.11 —
# 40 мин, день ждёт автобонуса (TestSeedDay bonusPending: true, служба тогда не работала до полуночи); среды нет
# (компьютер не включали). Понедельник 16.11 — 0 мин, засеян без bonusPending: засеянная статистика бонусов не даёт.
# Когда: служба запускается заново (утро четверга), затем ещё раз.
# Тогда: в копилке +40 мин за вторник (120 − 40 = 80 мин, 8 × 5) одной записью «Бонус за свободное время: 40 мин»,
# в журнале «Бонус за свободное время 17 ноября: +40 мин»; за среду и понедельник ничего; второй перезапуск ничего
# не добавляет.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

function Get-Bonus {
  $r = Send-Parent "GetBonus" @{ childId = $script:ChildId }
  Assert-Reply $r "GetBonus"
  return $r.payload
}

function Format-Bonus($B) {
  return "баланс $($B.balance) мин; записи: " + ((@(List $B.entries) | ForEach-Object { "$($_.deltaMin): $($_.reason)" }) -join ' | ')
}

# Записи журнала об автобонусе (bonus_granted) этого ребёнка.
function Get-BonusEvents {
  $r = Send-Parent "ListEvents" @{ childId = $script:ChildId; group = "time"; limit = 200 }
  Assert-Reply $r "ListEvents"
  return @(@(List $r.payload.events) | Where-Object { [string]$_.kind -eq "bonus_granted" })
}

Invoke-Scenario "S51" "Автобонус за вторник при запуске в четверг: +40 мин один раз, за среду ничего" {
  Initialize-Scenario 1 0 "2026-11-19T09:00:00"

  Step "Настройка: Миша, лимит 120 мин на каждый день; часы ядра — четверг 19.11.2026" {
    Invoke-Setup 120 $script:MySid "Миша"
    Open-Parent
    $script:ChildId = Get-ChildId
    $day = [DateTimeOffset]::FromUnixTimeMilliseconds((Get-CoreNowMs)).LocalDateTime.ToString("yyyy-MM-dd")
    Assert-That ($day -eq "2026-11-19") "Часы ядра показывают $day, ждали 2026-11-19 (ClockStart)"
    $rate = Send-Parent "GetBonusRate"
    Assert-Reply $rate "GetBonusRate"
    Assert-That ([int]$rate.payload.minutes -eq 5) "Бонус за 10 мин: $($rate.payload.minutes), ждали 5 (по умолчанию)"
    $b = Get-Bonus
    Assert-That ([int]$b.balance -eq 0 -and @(List $b.entries).Count -eq 0) "Копилка до испытания: $(Format-Bonus $b), ждали пусто"
  }

  Step "Вторник 40 мин ждёт автобонуса, понедельник 0 мин засеян как рассмотренный; среды нет" {
    Assert-Reply (Send-Pcb "TestSeedDay" @{ childId = $script:ChildId; day = "2026-11-17"; totalS = 2400; gamesS = 0; studyS = 0; bonusPending = $true; usage = @() }) "TestSeedDay вторник"
    Assert-Reply (Send-Pcb "TestSeedDay" @{ childId = $script:ChildId; day = "2026-11-16"; totalS = 0; gamesS = 0; studyS = 0; usage = @() }) "TestSeedDay понедельник"
    $b = Get-Bonus
    Assert-That ([int]$b.balance -eq 0) "Копилка до перезапуска: $(Format-Bonus $b), ждали 0 (служба начисляет пропущенные дни при запуске)"
  }

  Step "Утро четверга: служба запускается — копилка +40 мин за вторник, запись в журнале" {
    Restart-PcbService
    Open-Parent
    $b = Get-Bonus
    Write-Log "Копилка после запуска: $(Format-Bonus $b)"
    Assert-That ([int]$b.balance -eq 40) "Баланс копилки $($b.balance) мин, ждали 40 (120 − 40 = 80 мин, 8 × 5)"
    $entries = @(List $b.entries)
    Assert-That ($entries.Count -eq 1 -and [int]$entries[0].deltaMin -eq 40 -and [string]$entries[0].reason -eq "Бонус за свободное время: 40 мин") "Записи копилки: $(Format-Bonus $b), ждали одну «Бонус за свободное время: 40 мин» (+40)"
    $ev = @(Get-BonusEvents)
    Write-Log "Журнал автобонуса: $(($ev | ForEach-Object { $_.text }) -join ' | ')"
    Assert-That ($ev.Count -eq 1 -and [string]$ev[0].text -match "Бонус за свободное время 17 ноября: \+40 мин") "В журнале автобонуса $($ev.Count) записей, ждали одну «Бонус за свободное время 17 ноября: +40 мин»: $(($ev | ForEach-Object { $_.text }) -join ' | ')"
    $script:Token = ""
  }

  Step "Повторный перезапуск службы в четверг ничего не добавляет" {
    Restart-PcbService
    Open-Parent
    $b = Get-Bonus
    Write-Log "Копилка после второго запуска: $(Format-Bonus $b)"
    Assert-That ([int]$b.balance -eq 40 -and @(List $b.entries).Count -eq 1) "После второго запуска: $(Format-Bonus $b), ждали прежние 40 мин одной записью"
    Assert-That (@(Get-BonusEvents).Count -eq 1) "В журнале автобонуса $(@(Get-BonusEvents).Count) записей, ждали одну"
  }

  Write-Log "S51: пропущенная полночь вторника начислена при запуске один раз, за среду и засеянный понедельник ничего"
}
