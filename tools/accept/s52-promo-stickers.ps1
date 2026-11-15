# S52. Стикеры и надписи в ответ на промокод (R26, FR-35, D-83, план M5b T9).
# Дано: время вышло, экран блокировки «Время вышло»; родитель создал коды AB12 на 30 мин и CD34 на 1 мин.
# Когда: на экране блокировки неверный код два раза подряд, затем AB12; затем ребёнок вводит CD34 (как в окне
# «Моя статистика»: RedeemPromo от ребёнка) и CD34 ещё раз.
# Тогда: неверный — стикер из набора неудачи и надпись неудачи (GuardState promoSticker/promoText), второй
# неверный — другие стикер и надпись, лимит прежний; AB12 — стикер удачи и надпись с «+30 минут», экран держится
# одно проигрывание стикера (не меньше 2 с) и снимается (не позже 6 с), плашки «Добавлено» нет, лимит +30 мин;
# CD34 — служба принимает (+1 мин), повтор — отказ у поля code, лимит прежний. Надпись «+1 минута» в окне
# ребёнка проверяют vitest и снимки Playwright (T8).
# Часы ядра идут в 60 раз быстрее (лимит 5 мин). Ввод в поле стража и нажатия идут через TestGuard (Ruling 5).
# Снимки настоящего экрана в момент показа: s52-lock-bad.png (стикер неудачи над полем), s52-lock-ok.png (стикер удачи).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Вводит текст в поле промокода на панели стража и ждёт, пока страж покажет его в GuardState.
function Enter-GuardPromo([string]$Text) {
  Invoke-TestGuard "type" "promo" $Text
  [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" -and $_.text -eq $Text }).Count -ge 1 } 8 "в поле промокода набран текст")
}

Invoke-Scenario "S52" "Промокод: стикер и надпись на экране блокировки" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Родитель создаёт коды AB12 на 30 мин и CD34 на 1 мин" {
    Start-Child 5
    Open-Parent
    Assert-Reply (Send-Parent "CreatePromo" @{ code = "AB12"; minutes = 30 }) "CreatePromo AB12"
    Assert-Reply (Send-Parent "CreatePromo" @{ code = "CD34"; minutes = 1 }) "CreatePromo CD34"
    Assert-Reply (Send-Parent "Logout") "Logout"
    $script:Token = ""
    Resume-Clock
  }

  Step "Время вышло: экран блокировки с «Ввести промокод»" {
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    Assert-That (Test-HasButton $g "promo") "Нет кнопки «Ввести промокод»: $(Format-GuardState $g)"
    Assert-That ([string]$g.promoSticker -eq "" -and [string]$g.promoText -eq "") "Стикер до ввода кода: $(Format-GuardState $g)"
    $script:LimitBefore = [int](Get-Status).limitS
  }

  Step "Неверный код два раза: стикеры и надписи неудачи, второй ответ другой, лимит прежний" {
    Invoke-GuardClick "promo"
    [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" }).Count -ge 1 } 8 "панель промокода открыта")
    Enter-GuardPromo "WRONG1"
    Invoke-GuardClick "submitPromo"
    $a = Wait-PromoBad
    Write-Log "Первый неверный: стикер $($a.promoSticker), «$($a.promoText)»"
    Assert-That ((@(List $a.fields) | Where-Object { $_.id -eq "promo" -and $_.text -eq "WRONG1" }).Count -eq 1) "Код в поле после отказа не остался: $(Format-GuardState $a)"
    Save-Screen "s52-lock-bad"
    # Новый ввод убирает прошлый ответ (R26: ответ остаётся, пока ребёнок не начнёт вводить новый код).
    Enter-GuardPromo "WRONG2"
    [void](Wait-GuardState { param($s) [string]$s.promoSticker -eq "" } 5 "ответ исчез после нового ввода")
    Invoke-GuardClick "submitPromo"
    $b = Wait-PromoBad
    Write-Log "Второй неверный: стикер $($b.promoSticker), «$($b.promoText)»"
    Assert-That ([string]$b.promoSticker -ne [string]$a.promoSticker) "Второй неверный код показал тот же стикер $($b.promoSticker)"
    Assert-That ([string]$b.promoText -ne [string]$a.promoText) "Второй неверный код показал ту же надпись «$($b.promoText)»"
    $st = Get-Status
    Assert-That ([int]$st.limitS -eq $script:LimitBefore -and $st.lock -ne "none") "После неверных кодов лимит $($st.limitS) с (был $($script:LimitBefore)), блокировка '$($st.lock)'"
  }

  Step "AB12: стикер удачи с «+30 минут», экран держится одно проигрывание и снимается, лимит +30 мин" {
    Enter-GuardPromo "ab12"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Invoke-GuardClick "submitPromo"
    $g = Wait-PromoOk "30 минут"
    Write-Log ("Удача за {0:N1} с: стикер {1}, «{2}»" -f $sw.Elapsed.TotalSeconds, $g.promoSticker, $g.promoText)
    Save-Screen "s52-lock-ok"
    Assert-That (@(List $g.fields).Count -eq 0) "Во время показа удачи осталось поле: $(Format-GuardState $g)"
    Assert-That (Test-HasButton $g "parentPassword") "Во время показа удачи нет «Пароль родителя»: $(Format-GuardState $g)"
    [void](Wait-LockHidden 10)
    $held = $sw.Elapsed.TotalSeconds
    Assert-Between $held 2 6 "Экран после верного кода держался, с"
    Start-Sleep -Milliseconds 1500
    $after = Get-GuardState
    Assert-That (-not $after.lockShown -and [string]$after.plateKind -ne "added") "После показа: $(Format-GuardState $after) (плашки «Добавлено» быть не должно)"
    Assert-That ([string]$after.promoSticker -eq "") "Стикер остался после снятия экрана: $(Format-GuardState $after)"
    $st = Get-Status
    Assert-That ($st.lock -eq "none") "Блокировка после AB12: '$($st.lock)'"
    Assert-That ([int]$st.limitS -eq $script:LimitBefore + 1800) "Лимит дня $($st.limitS) с, ждали $($script:LimitBefore + 1800) (+30 мин по коду)"
  }

  Step "CD34 от ребёнка (окно «Моя статистика»): +1 мин; повтор — отказ у поля code, лимит прежний" {
    $r = Send-Pcb "RedeemPromo" @{ code = "cd34" }
    Assert-Reply $r "RedeemPromo CD34"
    Write-Log "CD34: добавлено $($r.payload.addedMin) мин, «$($r.payload.message)»"
    Assert-That ([int]$r.payload.addedMin -eq 1) "CD34 добавил $($r.payload.addedMin) мин, ждали 1"
    $st = Get-Status
    Assert-That ([int]$st.limitS -eq $script:LimitBefore + 1860) "Лимит дня $($st.limitS) с, ждали $($script:LimitBefore + 1860) (+1 мин по CD34)"
    $again = Send-Pcb "RedeemPromo" @{ code = "CD34" }
    Write-Log "Повтор CD34: ok=$($again.ok), поле $($again.field), «$($again.error)»"
    Assert-That (-not $again.ok -and [string]$again.field -eq "code") "Повтор CD34 принят или отказ не у поля code: ok=$($again.ok) field=$($again.field)"
    $st = Get-Status
    Assert-That ([int]$st.limitS -eq $script:LimitBefore + 1860) "После повтора CD34 лимит $($st.limitS) с, ждали $($script:LimitBefore + 1860)"
  }

  Write-Log "S52: неверные коды — разные стикеры и надписи неудачи, верный — стикер удачи на одно проигрывание"
}
