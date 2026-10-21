# S11. Пауза в запретные часы (ТЗ 11, эталон G11). Часы ядра идут в 60 раз быстрее.
# Часы стартуют в понедельник 15:00. Запретный интервал пн 15:00 -> вт 07:00, ребёнок сразу
# заблокирован «forbidden». Родитель ставит паузу 1 ч: экран снят, по окончании (60 секунд
# настоящего времени при ускорении 60) блокировка возвращается. Родительскую сессию закрываем
# (Logout) до ожидания стража (Ruling 19).
# Предупреждения 15/5/1 до конца паузы детерминированно проверяет эталон G11; на ускоренных
# часах устойчиво снять их здесь нельзя.
# Снимки: s11-paused.png (экран снят во время паузы).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S11" "Пауза в запретные часы: экран снят, по окончании блокировка возвращается" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: лимит дня 300 мин, запретный интервал с 15:00; ребёнок сразу заблокирован" {
    Start-Child 300
    Open-Parent
    $iv = @(@{ weekday = 1; startMin = 900; endMin = 420 })
    Assert-Reply (Send-Parent "SaveSchedule" @{ childId = (Get-ChildId); intervals = $iv }) "SaveSchedule"
    Assert-Reply (Send-Parent "Logout") "Logout"
    $g = Wait-LockShown 20 "forbidden"
    Assert-That ($g.lockKind -eq "forbidden") "Вид блокировки '$($g.lockKind)', ждали forbidden"
  }

  Step "Родитель ставит паузу 1 ч: экран блокировки снят" {
    Open-Parent
    $r = Send-Parent "Pause" @{ childId = (Get-ChildId); preset = "hour1" }
    Assert-Reply $r "Pause"
    Write-Log "Пауза поставлена: «$($r.payload.message)»"
    Assert-Reply (Send-Parent "Logout") "Logout"
    $g = Wait-LockHidden 10
    $st = Get-Status
    Assert-That ($st.lock -eq "none") "Во время паузы блокировка '$($st.lock)', ждали none"
    Assert-That ($st.pauseUntil -ne "") "Статус не содержит конца паузы"
    Start-Sleep -Milliseconds 600
    Save-Screen "s11-paused"
    Resume-Clock
  }

  Step "Пауза кончилась: экран блокировки «forbidden» вернулся" {
    $g = Wait-LockShown 90 "forbidden"
    Suspend-Clock
    Write-Log "Блокировка вернулась через $([math]::Round($script:WaitElapsed, 0)) с; плашки до возврата: $((Get-WarnThresholds $g) -join ',')"
    $st = Get-Status
    Assert-That ($st.lock -eq "forbidden") "После паузы блокировка '$($st.lock)', ждали forbidden"
    Assert-That ($st.until -eq "Можно снова: завтра в 07:00") "Строка «$($st.until)», ждали «Можно снова: завтра в 07:00»"
  }

  Step "Журнал родителя: «Пауза 1 ч»" {
    Open-Parent
    $found = Find-Events 'Пауза 1 ч'
    Assert-That ($found.Count -ge 1) "Записей «Пауза 1 ч»: $($found.Count), ждали хотя бы одну"
  }

  Write-Log "S11: пауза 1 ч сняла экран блокировки, по окончании блокировка forbidden вернулась"
}
