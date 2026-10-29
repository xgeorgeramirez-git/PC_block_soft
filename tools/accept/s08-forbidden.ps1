# S8. Запретный интервал: предупреждения 30, 15, 5 перед ним, блокировка «forbidden», «Взять из копилки»
# недоступна (ТЗ 11, эталон G6, D-54). Часы ядра идут в 60 раз быстрее. Часы стартуют в понедельник
# 15:00. Ставим запретный интервал пн 15:36 -> вт 07:00 (как G6 21:00-07:00): до него больше 30 мин,
# порог 30 взводится. Родительскую сессию закрываем (Logout) до ожидания стража (Ruling 19).
# Секунды порогов точно проверяют эталон G6 и тест internal/service; здесь по журналу показанных
# предупреждений стража: 30, 15, 5 по порядку, текст про запретные часы, без повторов. Плашку за
# 1 мин при ускорении 60 экран блокировки может сменить в ту же секунду: её не требуем.
# Снимки: s08-lock.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S8" "Запретный интервал: блокировка forbidden, «Взять из копилки» недоступна" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: ребёнок активен, лимит дня 600 мин (интервал наступит раньше)" {
    Start-Child 600
    $st = Get-Status
    Assert-That ($st.limitS -eq 36000) "Лимит дня $($st.limitS) с, ждали 36000"
  }

  Step "Родитель: запретный интервал пн 15:36 -> вт 07:00 и копилка 30 мин, затем выход" {
    Open-Parent
    $iv = @(@{ weekday = 1; startMin = 936; endMin = 420 })
    $r = Send-Parent "SaveSchedule" @{ childId = (Get-ChildId); intervals = $iv }
    Assert-Reply $r "SaveSchedule"
    $rb = Send-Parent "GrantBonus" @{ childId = (Get-ChildId); minutes = 30; reason = "за уборку" }
    Assert-Reply $rb "GrantBonus"
    Assert-That ($rb.payload.balanceAfter -eq 30) "Баланс копилки $($rb.payload.balanceAfter), ждали 30"
    Assert-Reply (Send-Parent "Logout") "Logout"
    Resume-Clock
  }

  Step "Интервал наступил: экран блокировки «forbidden», «Можно снова: завтра в 07:00»" {
    $g = Wait-LockShown 60 "forbidden"
    Suspend-Clock
    Write-Log "Экран показан: $(Format-GuardState $g)"
    $st = Get-Status
    Write-Log "Статус: $($st.status), причина «$($st.reason)», «$($st.until)»; плашки до блокировки: $((Get-WarnThresholds $g) -join ',')"
    Assert-That ($st.lock -eq "forbidden") "Вид блокировки '$($st.lock)', ждали forbidden"
    $thr = (Get-WarnThresholds $g) -join ","
    Assert-That ($thr -eq "30,15,5" -or $thr -eq "30,15,5,1") "Плашки перед интервалом: $thr, ждали 30,15,5 (и 1)"
    Assert-NoRepeatWarnings $g "S8"
    $w30 = @(List $g.warnings)[0]
    Assert-That ($w30.text -eq "Через 30 минут начнутся запретные часы") "Текст за 30 мин: «$($w30.text)»"
    Assert-That ($st.until -eq "Можно снова: завтра в 07:00") "Строка «$($st.until)», ждали «Можно снова: завтра в 07:00»"
    Assert-That (Test-HasButton $g "spendBonus") "Нет кнопки «Взять из копилки» на экране: $(Format-GuardState $g)"
    Assert-That (Test-HasButton $g "parentPassword") "Нет кнопки «Пароль родителя»: $(Format-GuardState $g)"
    Start-Sleep -Seconds 1
    Save-Screen "s08-lock"
  }

  Step "«Взять из копилки» в запретный интервал недоступна: нажатие стража ничего не меняет" {
    Invoke-GuardClick "spendBonus"
    Start-Sleep -Seconds 2
    $g = Get-GuardState
    Assert-That ($g.lockShown -and $g.lockKind -eq "forbidden") "После нажатия «Взять из копилки» экран изменился: $(Format-GuardState $g)"
    Open-Parent
    $rb = Send-Parent "GetBonus" @{ childId = (Get-ChildId) }
    Assert-Reply $rb "GetBonus"
    Assert-That ($rb.payload.balance -eq 30) "Баланс копилки $($rb.payload.balance), ждали 30 (трата в интервал не прошла)"
  }

  Write-Log "S8: блокировка forbidden «завтра в 07:00», «Взять из копилки» видна, но недоступна; копилка цела"
}
