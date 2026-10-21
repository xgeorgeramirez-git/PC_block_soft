# S8. Запретный интервал: предупреждения перед началом, блокировка «forbidden», «Взять из копилки»
# недоступна (ТЗ 11, эталон G6). Часы ядра идут в 60 раз быстрее (ClockSpeed=60).
# Часы стартуют в понедельник 19.10.2026 15:00. Ставим запретный интервал пн 15:16 -> вт 07:00
# (как G6 21:00-07:00, только ближе к старту, чтобы предупреждения успели за секунды).
# Родительскую сессию закрываем (Logout) до ожидания стража: пока она открыта, страж прячет
# экран блокировки (Ruling 19).
# Снимки: s08-warn-15.png, s08-lock.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S8" "Запретный интервал: предупреждения, блокировка, «Взять из копилки» недоступна" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: ребёнок активен, лимит дня 600 мин (интервал наступит раньше)" {
    Start-Child 600
    $st = Get-Status
    Assert-That ($st.limitS -eq 36000) "Лимит дня $($st.limitS) с, ждали 36000"
  }

  Step "Родитель: запретный интервал пн 15:16 -> вт 07:00 и копилка 30 мин, затем выход" {
    Open-Parent
    $cid = Get-ChildId
    $iv = @(@{ weekday = 1; startMin = 916; endMin = 420 })
    $r = Send-Parent "SaveSchedule" @{ childId = $cid; intervals = $iv }
    Assert-Reply $r "SaveSchedule"
    Write-Log "Интервалы сохранены: $(($r.payload.intervals | ForEach-Object { "$($_.weekday):$($_.startMin)-$($_.endMin)" }) -join ', ')"
    $rb = Send-Parent "GrantBonus" @{ childId = $cid; minutes = 30; reason = "за уборку" }
    Assert-Reply $rb "GrantBonus"
    Assert-That ($rb.payload.balanceAfter -eq 30) "Баланс копилки $($rb.payload.balanceAfter), ждали 30"
    $rl = Send-Parent "Logout"
    Assert-Reply $rl "Logout"
    Resume-Clock
  }

  Step "Плашка за 15 минут до интервала, снимок на паузе часов" {
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 1 } 20 "плашка за 15 минут до интервала"
    Suspend-Clock
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15") "Плашки: $((Get-WarnThresholds $g) -join ','), ждали только 15"
    Assert-That (-not $g.lockShown) "Экран блокировки показан раньше интервала"
    Start-Sleep -Milliseconds 600
    Save-Screen "s08-warn-15"
    Resume-Clock
  }

  Step "Плашки за 5 и 1 минуту по порядку" {
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 2 } 20 "плашка за 5 минут"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15,5") "Плашки: $((Get-WarnThresholds $g) -join ','), ждали 15,5"
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s).Count -ge 3 } 20 "плашка за 1 минуту"
    Assert-That ((Get-WarnThresholds $g) -join "," -eq "15,5,1") "Плашки: $((Get-WarnThresholds $g) -join ','), ждали 15,5,1"
  }

  Step "Интервал наступил: экран блокировки «forbidden», «Можно снова: завтра в 07:00»" {
    $g = Wait-LockShown 20 "forbidden"
    Suspend-Clock
    Write-Log "Экран показан: $(Format-GuardState $g)"
    $st = Get-Status
    Write-Log "Статус: $($st.status), причина «$($st.reason)», «$($st.until)»"
    Assert-That ($st.lock -eq "forbidden") "Вид блокировки '$($st.lock)', ждали forbidden"
    Assert-That ($st.until -eq "Можно снова: завтра в 07:00") "Строка «$($st.until)», ждали «Можно снова: завтра в 07:00»"
    Assert-That (Test-HasButton $g "spendBonus") "Нет кнопки «Взять из копилки» на экране: $(Format-GuardState $g)"
    Assert-That (Test-HasButton $g "parentPassword") "Нет кнопки «Пароль родителя»: $(Format-GuardState $g)"
    Start-Sleep -Seconds 1
    Save-Screen "s08-lock"
  }

  Step "«Взять из копилки» в запретный интервал недоступна: служба отказывает" {
    $r = Send-Pcb "SpendBonus" @{ childId = 0; minutes = 30 }
    Assert-That (-not $r.ok) "Служба позволила взять из копилки в запретный интервал: $($r | ConvertTo-Json -Compress)"
    Write-Log "Отказ службы на «Взять из копилки» в интервал: «$($r.error)»"
    $st = Get-Status
    Assert-That ($st.lock -eq "forbidden") "После отказа блокировка '$($st.lock)', ждали forbidden"
  }

  Write-Log "S8: предупреждения 15,5,1 перед интервалом, блокировка forbidden «завтра в 07:00», копилка недоступна"
}
