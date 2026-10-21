# S10. Копилка: ребёнок берёт бонус с экрана блокировки (ТЗ 8.2.3 п.3, FR-11, эталон G8); в
# запретный интервал трата недоступна. Часы ядра идут в 60 раз быстрее. Лимит 5 мин.
# Кнопка «Взять из копилки» берёт весь баланс (ТЗ 8.2.3: подпись «Взять из копилки (N мин)»),
# поэтому ребёнок забирает все 60 мин: лимит +60, баланс 0. Трату ведём настоящим стражем —
# служба не даёт звать SpendBonus напрямую из учётной записи администратора.
# Родительскую сессию закрываем (Logout) до ожидания стража (Ruling 19).
# Снимки: s10-unlocked.png (экран снят после траты).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S10" "Копилка: ребёнок берёт бонус с экрана блокировки, в интервал недоступно" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Лимит 5 мин, копилка 60 мин; время выходит, показан экран блокировки" {
    Start-Child 5
    Open-Parent
    $r = Send-Parent "GrantBonus" @{ childId = (Get-ChildId); minutes = 60; reason = "за помощь" }
    Assert-Reply $r "GrantBonus"
    Assert-That ($r.payload.balanceAfter -eq 60) "Баланс $($r.payload.balanceAfter), ждали 60"
    Assert-Reply (Send-Parent "Logout") "Logout"
    Resume-Clock
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    Assert-That (Test-HasButton $g "spendBonus") "Нет кнопки «Взять из копилки»: $(Format-GuardState $g)"
  }

  Step "Ребёнок берёт из копилки весь баланс: лимит +60, баланс 0, экран снят" {
    Invoke-GuardClick "spendBonus"
    $g = Wait-LockHidden 10
    $st = Get-Status
    Write-Log "После траты: статус $($st.status), остаток $($st.remainingS) с, лимит $($st.limitS) с"
    Assert-That ($st.lock -eq "none") "Блокировка после траты: '$($st.lock)'"
    Assert-That ($st.limitS -eq 3900) "Лимит дня $($st.limitS) с, ждали 3900 (5 мин + 60 мин из копилки)"
    Assert-Between $st.remainingS 3595 3600 "Остаток после траты, с"
    Save-Screen "s10-unlocked"
  }

  Step "Копилка родителя: баланс 0, в истории трата -60; затем интервал и новая копилка 30" {
    Open-Parent
    $rb = Send-Parent "GetBonus" @{ childId = (Get-ChildId) }
    Assert-Reply $rb "GetBonus"
    Assert-That ($rb.payload.balance -eq 0) "Баланс копилки $($rb.payload.balance), ждали 0"
    $spent = @(List $rb.payload.entries | Where-Object { $_.deltaMin -eq -60 })
    Assert-That ($spent.Count -ge 1) "В истории нет траты -60: $(($rb.payload.entries | ForEach-Object { $_.deltaMin }) -join ',')"
    $iv = @(@{ weekday = 1; startMin = 900; endMin = 420 })
    Assert-Reply (Send-Parent "SaveSchedule" @{ childId = (Get-ChildId); intervals = $iv }) "SaveSchedule"
    Assert-Reply (Send-Parent "GrantBonus" @{ childId = (Get-ChildId); minutes = 30; reason = "ещё" }) "GrantBonus"
    Assert-Reply (Send-Parent "Logout") "Logout"
  }

  Step "Запретный интервал: «Взять из копилки» нажата, но трата не проходит" {
    $g = Wait-LockShown 20 "forbidden"
    Assert-That (Test-HasButton $g "spendBonus") "Нет кнопки «Взять из копилки» в интервал: $(Format-GuardState $g)"
    Invoke-GuardClick "spendBonus"
    Start-Sleep -Seconds 2
    $g = Get-GuardState
    Assert-That ($g.lockShown -and $g.lockKind -eq "forbidden") "После нажатия экран изменился: $(Format-GuardState $g)"
    Open-Parent
    $rb = Send-Parent "GetBonus" @{ childId = (Get-ChildId) }
    Assert-That ($rb.payload.balance -eq 30) "После отказа баланс $($rb.payload.balance), ждали 30 (трата не прошла)"
  }

  Write-Log "S10: ребёнок забрал всю копилку (лимит +60, баланс 0); в запретный интервал трата не прошла"
}
