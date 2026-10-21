# S11. Пауза в запретные часы (ТЗ 11, эталон G11). Часы ядра идут в 60 раз быстрее.
# Часы стартуют в понедельник 15:00. Ставим запретный интервал пн 15:00 -> вт 07:00, поэтому
# ребёнок сразу заблокирован «forbidden». Родитель ставит паузу 1 ч: экран снят на 1 ч
# (60 секунд настоящего времени), предупреждения за 15, 5, 1 мин до конца паузы, по окончании
# экран блокировки возвращается.
# Снимки: s11-paused.png (экран снят во время паузы).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S11" "Пауза в запретные часы: экран снят на 1 ч, предупреждения и возврат блокировки" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: лимит дня 300 мин, запретный интервал с 15:00; ребёнок сразу заблокирован" {
    Start-Child 300
    Open-Parent
    $cid = Get-ChildId
    $iv = @(@{ weekday = 1; startMin = 900; endMin = 420 })
    $r = Send-Parent "SaveSchedule" @{ childId = $cid; intervals = $iv }
    Assert-Reply $r "SaveSchedule"
    $g = Wait-LockShown 20 "forbidden"
    Assert-That ($g.lockKind -eq "forbidden") "Вид блокировки '$($g.lockKind)', ждали forbidden"
  }

  Step "Родитель ставит паузу 1 ч: экран блокировки снят" {
    $cid = Get-ChildId
    $r = Send-Parent "Pause" @{ childId = $cid; preset = "hour1" }
    Assert-Reply $r "Pause"
    Write-Log "Пауза поставлена: «$($r.payload.message)»"
    $g = Wait-LockHidden 10
    $st = Get-Status
    Assert-That ($st.lock -eq "none") "Во время паузы блокировка '$($st.lock)', ждали none"
    Assert-That ($st.pauseUntil -ne "") "Статус не содержит конца паузы"
    Start-Sleep -Milliseconds 600
    Save-Screen "s11-paused"
    Resume-Clock
  }

  Step "Предупреждения 15, 5, 1 мин до конца паузы по порядку" {
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 1 } 60 "плашка за 15 минут до конца паузы"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15") "Плашки: $((Get-WarnThresholds $g) -join ','), ждали 15"
    Assert-That (-not $g.lockShown) "Экран блокировки вернулся раньше конца паузы"
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 2 } 30 "плашка за 5 минут"
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 3 } 30 "плашка за 1 минуту"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15,5,1") "Плашки: $((Get-WarnThresholds $g) -join ','), ждали 15,5,1"
  }

  Step "Пауза кончилась: экран блокировки «forbidden» вернулся" {
    $g = Wait-LockShown 20 "forbidden"
    Suspend-Clock
    $st = Get-Status
    Assert-That ($st.lock -eq "forbidden") "После паузы блокировка '$($st.lock)', ждали forbidden"
    Assert-That ($st.until -eq "Можно снова: завтра в 07:00") "Строка «$($st.until)», ждали «Можно снова: завтра в 07:00»"
  }

  Step "Журнал родителя: «Пауза 1 ч»" {
    $found = Find-Events 'Пауза 1 ч'
    Assert-That ($found.Count -ge 1) "Записей «Пауза 1 ч»: $($found.Count), ждали хотя бы одну"
  }

  Write-Log "S11: пауза 1 ч сняла экран, предупреждения 15,5,1 до конца паузы, блокировка вернулась"
}
