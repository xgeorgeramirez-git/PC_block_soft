# S10. Копилка: трата 30 из 60 добавляет 30 мин к лимиту; в запретный интервал трата недоступна
# (ТЗ 11, эталон G8). Часы ядра идут в 60 раз быстрее. Лимит 5 мин: время выходит за 5 секунд.
# Дано: копилка 60 мин, время вышло. Когда: ребёнок берёт из копилки 30. Тогда: баланс 30,
# лимит +30, экран снят. Затем запретный интервал: повторная трата отклонена службой.
# Снимки: s10-unlocked.png (экран снят после траты).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S10" "Копилка: трата 30 из 60, лимит +30, в запретный интервал недоступно" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Лимит 5 мин, копилка 60 мин; время выходит, показан экран блокировки" {
    Start-Child 5
    Open-Parent
    $cid = Get-ChildId
    $r = Send-Parent "GrantBonus" @{ childId = $cid; minutes = 60; reason = "за помощь" }
    Assert-Reply $r "GrantBonus"
    Assert-That ($r.payload.balanceAfter -eq 60) "Баланс $($r.payload.balanceAfter), ждали 60"
    Resume-Clock
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    Assert-That (Test-HasButton $g "spendBonus") "Нет кнопки «Взять из копилки»: $(Format-GuardState $g)"
  }

  Step "Ребёнок берёт из копилки 30: баланс 30, лимит +30, экран снят" {
    $r = Send-Pcb "SpendBonus" @{ childId = 0; minutes = 30 }
    Assert-Reply $r "SpendBonus"
    Assert-That ($r.payload.balanceAfter -eq 30) "Баланс после траты $($r.payload.balanceAfter), ждали 30"
    $g = Wait-LockHidden 10
    $st = Get-Status
    Write-Log "После траты: статус $($st.status), остаток $($st.remainingS) с, лимит $($st.limitS) с"
    Assert-That ($st.lock -eq "none") "Блокировка после траты: '$($st.lock)'"
    Assert-That ($st.limitS -eq 2100) "Лимит дня $($st.limitS) с, ждали 2100 (5 мин + 30 мин из копилки)"
    Assert-Between $st.remainingS 1795 1800 "Остаток после траты 30, с"
    Save-Screen "s10-unlocked"
  }

  Step "Копилка родителя: баланс 30, в истории трата -30" {
    $cid = Get-ChildId
    $rb = Send-Parent "GetBonus" @{ childId = $cid }
    Assert-Reply $rb "GetBonus"
    Assert-That ($rb.payload.balance -eq 30) "Баланс копилки $($rb.payload.balance), ждали 30"
    $spent = @(List $rb.payload.entries | Where-Object { $_.deltaMin -eq -30 })
    Assert-That ($spent.Count -ge 1) "В истории нет траты -30: $(($rb.payload.entries | ForEach-Object { $_.deltaMin }) -join ',')"
  }

  Step "Запретный интервал сейчас: повторная трата из копилки отклонена" {
    $cid = Get-ChildId
    $iv = @(@{ weekday = 1; startMin = 900; endMin = 420 })
    $r = Send-Parent "SaveSchedule" @{ childId = $cid; intervals = $iv }
    Assert-Reply $r "SaveSchedule"
    $g = Wait-LockShown 20 "forbidden"
    Assert-That ($g.lockKind -eq "forbidden") "Вид блокировки '$($g.lockKind)', ждали forbidden"
    $r = Send-Pcb "SpendBonus" @{ childId = 0; minutes = 30 }
    Assert-That (-not $r.ok) "Служба позволила взять из копилки в запретный интервал: $($r | ConvertTo-Json -Compress)"
    Write-Log "Отказ службы: «$($r.error)»"
    $rb = Send-Parent "GetBonus" @{ childId = $cid }
    Assert-That ($rb.payload.balance -eq 30) "После отказа баланс $($rb.payload.balance), ждали 30 (трата не прошла)"
  }

  Write-Log "S10: трата 30 из 60 дала +30 мин, баланс 30; в запретный интервал трата отклонена"
}
