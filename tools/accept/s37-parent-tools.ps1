# S37. Инструменты родителя 0.2.2: снять запретные часы на день (R24, D-51), убавить и обнулить
# копилку (D-46), настройка автобонуса (R25, D-49), путь к папке логов (D-48).
# Часы ядра идут в 60 раз быстрее и стоят на паузе; старт в понедельник 15:00 (как в S8).
# Автобонус на смене суток доказывают тесты службы (TestAutoBonusAtMidnight): перейти полночь на
# ускоренных часах в приёмке нельзя, момент старта часов зависит от времени запуска.
# Родительскую сессию закрываем (Logout) до ожидания стража (Ruling 19).
# Снимки: s37-forbidden.png (экран «forbidden»), s37-forbidden-lifted.png (после снятия запрета).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S37" "Снять запрет на день, убавить и обнулить копилку, настройка бонуса, папка логов" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Лимит 600 мин, запретный интервал пн 15:00 -> вт 07:00: показан экран «forbidden»" {
    Start-Child 600
    Open-Parent
    $iv = @(@{ weekday = 1; startMin = 900; endMin = 420 })
    Assert-Reply (Send-Parent "SaveSchedule" @{ childId = (Get-ChildId); intervals = $iv }) "SaveSchedule"
    Assert-Reply (Send-Parent "Logout") "Logout"
    [void](Wait-LockShown 20 "forbidden")
    Start-Sleep -Seconds 1
    Save-Screen "s37-forbidden"
  }

  Step "Родитель снимает запрет на сегодня: экран снят, лимит дня остаётся, запись в журнале" {
    Open-Parent
    $r = Send-Parent "SetNoForbidden" @{ childId = (Get-ChildId); on = $true }
    Assert-Reply $r "SetNoForbidden on"
    Assert-That ($r.payload.card.noForbiddenToday -eq $true) "В карточке запрет не снят"
    Assert-That ($r.payload.message -eq "Сегодня запретные часы не действуют") "Сообщение «$($r.payload.message)»"
    Assert-That ((Find-Events "Запретные часы сняты на сегодня").Count -ge 1) "В журнале нет записи о снятии запрета"
    Assert-Reply (Send-Parent "Logout") "Logout"
    [void](Wait-LockHidden 10)
    $st = Get-Status
    Write-Log "После снятия запрета: блокировка '$($st.lock)', лимит $($st.limitS) с"
    Assert-That ($st.lock -eq "none") "Блокировка после снятия запрета: '$($st.lock)'"
    Assert-That ($st.limitS -eq 36000) "Лимит дня $($st.limitS) с, ждали 36000 (общий лимит остаётся)"
    Save-Screen "s37-forbidden-lifted"
  }

  Step "Родитель возвращает запретные часы: экран «forbidden» снова на месте" {
    Open-Parent
    $r = Send-Parent "SetNoForbidden" @{ childId = (Get-ChildId); on = $false }
    Assert-Reply $r "SetNoForbidden off"
    Assert-That ($r.payload.card.noForbiddenToday -eq $false) "В карточке запрет всё ещё снят"
    Assert-Reply (Send-Parent "Logout") "Logout"
    [void](Wait-LockShown 20 "forbidden")
  }

  Step "Копилка: +60, убавить на 15 (45), убавить на 100 (дно 0), +20 и обнулить; история и журнал" {
    Open-Parent
    $id = Get-ChildId
    $g = Send-Parent "GrantBonus" @{ childId = $id; minutes = 60; reason = "за уборку" }
    Assert-Reply $g "GrantBonus"
    $a = Send-Parent "ReduceBonus" @{ childId = $id; minutes = 15 }
    Assert-Reply $a "ReduceBonus 15"
    Assert-That ($a.payload.balanceAfter -eq 45) "После убавления на 15 баланс $($a.payload.balanceAfter), ждали 45"
    $b = Send-Parent "ReduceBonus" @{ childId = $id; minutes = 100 }
    Assert-Reply $b "ReduceBonus 100"
    Assert-That ($b.payload.balanceAfter -eq 0) "После убавления на 100 баланс $($b.payload.balanceAfter), ждали 0 (не ниже нуля)"
    $z = Send-Parent "ReduceBonus" @{ childId = $id; minutes = 0 }
    Assert-That (-not $z.ok -and $z.field -eq "minutes") "Убавление на 0 принято или ошибка не у поля minutes: ok=$($z.ok) field=$($z.field)"
    Assert-Reply (Send-Parent "GrantBonus" @{ childId = $id; minutes = 20; reason = "ещё" }) "GrantBonus 20"
    $c = Send-Parent "ClearBonus" @{ childId = $id }
    Assert-Reply $c "ClearBonus"
    Assert-That ($c.payload.balanceAfter -eq 0 -and $c.payload.message -eq "Копилка обнулена") "Обнуление: баланс $($c.payload.balanceAfter), сообщение «$($c.payload.message)»"
    $c2 = Send-Parent "ClearBonus" @{ childId = $id }
    Assert-That ($c2.ok -and $c2.payload.message -eq "Копилка уже пуста") "Повторное обнуление: «$($c2.payload.message)»"
    $rb = Send-Parent "GetBonus" @{ childId = $id }
    Assert-Reply $rb "GetBonus"
    Assert-That ($rb.payload.balance -eq 0) "Баланс копилки $($rb.payload.balance), ждали 0"
    $reasons = @(List $rb.payload.entries | ForEach-Object { $_.reason })
    Assert-That ($reasons -contains "Убавлено родителем" -and $reasons -contains "Обнулено родителем") "В истории нет причин «Убавлено родителем»/«Обнулено родителем»: $($reasons -join '; ')"
    Assert-That ((Find-Events "Копилка убавлена родителем").Count -ge 1) "В журнале нет записи об убавлении копилки"
    Assert-That ((Find-Events "Копилка обнулена родителем").Count -ge 1) "В журнале нет записи об обнулении копилки"
  }

  Step "Настройка автобонуса: по умолчанию 5, ставится 10, 601 отклоняется" {
    $r0 = Send-Parent "GetBonusRate"
    Assert-Reply $r0 "GetBonusRate"
    Assert-That ($r0.payload.minutes -eq 5) "Автобонус по умолчанию $($r0.payload.minutes), ждали 5"
    Assert-Reply (Send-Parent "SetBonusRate" @{ minutes = 10 }) "SetBonusRate 10"
    $r1 = Send-Parent "GetBonusRate"
    Assert-That ($r1.payload.minutes -eq 10) "После установки автобонус $($r1.payload.minutes), ждали 10"
    $bad = Send-Parent "SetBonusRate" @{ minutes = 601 }
    Assert-That (-not $bad.ok -and $bad.field -eq "minutes") "601 принят или ошибка не у поля minutes: ok=$($bad.ok) field=$($bad.field)"
    Assert-Reply (Send-Parent "SetBonusRate" @{ minutes = 5 }) "SetBonusRate 5"
  }

  Step "Папка логов: команда отвечает путём, в тестовом режиме проводник не запускается" {
    $o = Send-Parent "OpenLogs"
    Assert-Reply $o "OpenLogs"
    Assert-That ($o.payload.path -match '\\logs$') "Путь к логам «$($o.payload.path)» не оканчивается на \logs"
    $d = Send-Parent "GetDiag"
    Assert-Reply $d "GetDiag"
    Assert-That ($d.payload.logsPath -eq $o.payload.path) "Путь в настройках «$($d.payload.logsPath)» не совпал с путём команды «$($o.payload.path)»"
    Assert-That ((Get-Process -Name explorer -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowTitle -match "logs" }).Count -eq 0) "В тестовом режиме открылось окно проводника с логами"
    Assert-Reply (Send-Parent "Logout") "Logout"
  }

  Write-Log "S37: запрет снят и возвращён, копилка убавлена с дном 0 и обнулена, автобонус настраивается, путь к логам отдан"
}
