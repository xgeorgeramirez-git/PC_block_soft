# S11. Пауза в запретные часы (ТЗ 11, эталон G11). Часы ядра идут в 60 раз быстрее.
# Часы стартуют в понедельник 15:00. Запретный интервал пн 15:00 -> вт 07:00, ребёнок сразу
# заблокирован «forbidden». Родитель ставит паузу 1 ч: экран снят, по окончании (60 секунд
# настоящего времени при ускорении 60) блокировка возвращается. Родительскую сессию закрываем
# (Logout) до ожидания стража (Ruling 19).
# Предупреждения до конца паузы 30, 15, 5, 1 (D-54): секунды точно проверяет эталон G11; здесь по
# журналу показанных предупреждений стража: 30, 15, 5 по порядку с текстом про паузу, без повторов
# (плашку 1 мин экран блокировки при ускорении 60 может сменить в ту же секунду: её не требуем).
# Закончить раньше срока (D-96): снова пауза 1 ч, родитель нажимает значок ещё раз (EndPause) —
# служба сразу снова «forbidden», экран стража вернулся не позже 2 с после выхода родителя (его
# сессия прячет экран, Ruling 19), в журнале «пауза закончена раньше срока». Плашку «Пауза
# закончилась» страж показывает, только когда экрана блокировки нет (под экраном говорит экран):
# поэтому её проверяем отдельно — запрет на сегодня снят, пауза 15 мин и сразу конец.
# Снимки: s11-paused.png (экран снят во время паузы), s11-pause-ended.png (плашка «Пауза закончилась»).
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
    $thr = (Get-WarnThresholds $g) -join ","
    Assert-That ($thr -eq "30,15,5" -or $thr -eq "30,15,5,1") "Плашки до конца паузы: $thr, ждали 30,15,5 (и 1)"
    Assert-NoRepeatWarnings $g "S11"
    Assert-That (@(List $g.warnings)[0].text -eq "Через 30 минут закончится пауза") "Текст за 30 мин: «$(@(List $g.warnings)[0].text)»"
    Assert-That ($st.until -eq "Можно снова: завтра в 07:00") "Строка «$($st.until)», ждали «Можно снова: завтра в 07:00»"
  }

  Step "Журнал родителя: «Пауза 1 ч»" {
    Open-Parent
    $found = Find-Events 'Пауза 1 ч'
    Assert-That ($found.Count -ge 1) "Записей «Пауза 1 ч»: $($found.Count), ждали хотя бы одну"
  }

  Step "Снова пауза 1 ч и «Закончить паузу»: блокировка сразу, экран не позже 2 с" {
    Assert-Reply (Send-Parent "Logout") "Logout после журнала"
    $script:Token = ""
    Use-Parent { Assert-Reply (Send-Parent "Pause" @{ childId = (Get-ChildId); preset = "hour1" }) "Pause" }
    [void](Wait-LockHidden 10)
    Assert-That ((Get-Status).lock -eq "none") "Во время второй паузы блокировка '$((Get-Status).lock)', ждали none"
    Open-Parent
    $r = Send-Parent "EndPause" @{ childId = (Get-ChildId) }
    Assert-Reply $r "EndPause"
    Assert-That ($r.payload.message -eq "Пауза закончилась") "Ответ EndPause «$($r.payload.message)», ждали «Пауза закончилась»"
    Assert-That ($r.payload.card.status.status -eq "forbidden") "Карточка после EndPause: '$($r.payload.card.status.status)', ждали forbidden"
    $st = Get-Status
    Assert-That ($st.lock -eq "forbidden" -and $st.pauseUntil -eq "") "Служба после EndPause: блокировка '$($st.lock)', пауза до '$($st.pauseUntil)'"
    $r = Send-Parent "EndPause" @{ childId = (Get-ChildId) }
    Assert-That ((-not $r.ok) -and $r.error -eq "Паузы сейчас нет") "Повторный EndPause: ok=$($r.ok) «$($r.error)», ждали «Паузы сейчас нет»"
    Assert-Reply (Send-Parent "Logout") "Logout"
    $script:Token = ""
    $g = Wait-LockShown 2 "forbidden"
    Write-Log "Экран вернулся через $([math]::Round($script:WaitElapsed, 1)) с после выхода родителя"
  }

  Step "Журнал родителя: «пауза закончена раньше срока»" {
    Use-Parent {
      $found = Find-Events 'пауза закончена раньше срока'
      Assert-That ($found.Count -eq 1) "Записей «пауза закончена раньше срока»: $($found.Count), ждали одну"
    }
  }

  Step "Без экрана блокировки ребёнок видит плашку «Пауза закончилась»" {
    Use-Parent { Assert-Reply (Send-Parent "SetNoForbidden" @{ childId = (Get-ChildId); on = $true }) "SetNoForbidden on" }
    [void](Wait-LockHidden 10)
    Use-Parent { Assert-Reply (Send-Parent "Pause" @{ childId = (Get-ChildId); preset = "min15" }) "Pause 15 мин" }
    $g = Wait-GuardState { param($s) [string]$s.plate -eq "Пауза 15 мин" } 5 "плашка «Пауза 15 мин» (не «Добавлено 15 минут»)"
    Use-Parent { Assert-Reply (Send-Parent "EndPause" @{ childId = (Get-ChildId) }) "EndPause" }
    $g = Wait-GuardState { param($s) [string]$s.plate -eq "Пауза закончилась" } 5 "плашка «Пауза закончилась»"
    Assert-That ($g.plateKind -eq "notice") "Вид плашки «$($g.plateKind)», ждали notice"
    Assert-That (-not $g.lockShown) "Экран блокировки при снятом запрете: $(Format-GuardState $g)"
    Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
    Save-Screen "s11-pause-ended"
  }

  Write-Log "S11: пауза 1 ч сняла экран блокировки, по окончании блокировка forbidden вернулась; «Закончить паузу» вернула её сразу"
}
