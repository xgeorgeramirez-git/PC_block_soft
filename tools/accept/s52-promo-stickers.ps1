# S52. Стикеры и надписи в ответ на промокод (R26, FR-35, D-83, план M5b T9).
# Дано: время вышло, экран блокировки «Время вышло»; родитель создал коды AB12 на 30 мин и CD34 на 1 мин.
# Когда: на экране блокировки неверный код два раза подряд, затем CD34; время (1 мин) кончается, экран
# возвращается; снова CD34, затем AB12. Ввод CD34 в окне «Моя статистика» проверяют vitest и e2e (T8): скрипт
# приёмки работает повышенным администратором, команду ребёнка от него служба не принимает (Ruling).
# Тогда: неверный — стикер из набора неудачи и надпись неудачи (GuardState promoSticker/promoText), второй
# неверный — другие стикер и надпись, лимит прежний; CD34 — стикер удачи и надпись с «+1 минута», экран снят,
# лимит +1 мин; повтор CD34 — стикер неудачи, лимит прежний; AB12 — другой стикер удачи и надпись с «+30 минут»,
# экран держится одно проигрывание стикера (не меньше 2 с) и снимается (не позже 6 с), плашки «Добавлено» нет,
# лимит +31 мин всего.
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

  Step "CD34 на экране блокировки: стикер удачи с «+1 минута», экран снят, лимит +1 мин" {
    Enter-GuardPromo "cd34"
    Invoke-GuardClick "submitPromo"
    $g = Wait-PromoOk "1 минута"
    Write-Log "CD34: стикер $($g.promoSticker), «$($g.promoText)»"
    $script:FirstOk = [string]$g.promoSticker
    [void](Wait-LockHidden 10)
    $st = Get-Status
    Assert-That ($st.lock -eq "none") "Блокировка после CD34: '$($st.lock)'"
    Assert-That ([int]$st.limitS -eq $script:LimitBefore + 60) "Лимит дня $($st.limitS) с, ждали $($script:LimitBefore + 60) (+1 мин по CD34)"
  }

  Step "Минута кончилась, экран вернулся: повтор CD34 — стикер неудачи, лимит прежний" {
    Resume-Clock
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    Invoke-GuardClick "promo"
    [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" }).Count -ge 1 } 8 "панель промокода открыта")
    Enter-GuardPromo "CD34"
    Invoke-GuardClick "submitPromo"
    $c = Wait-PromoBad
    Write-Log "Повтор CD34: стикер $($c.promoSticker), «$($c.promoText)»"
    $st = Get-Status
    Assert-That ([int]$st.limitS -eq $script:LimitBefore + 60 -and $st.lock -ne "none") "После повтора CD34 лимит $($st.limitS) с (ждали $($script:LimitBefore + 60)), блокировка '$($st.lock)'"
  }

  Step "AB12: стикер удачи с «+30 минут», экран держится одно проигрывание и снимается, лимит +30 мин" {
    Enter-GuardPromo "ab12"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Invoke-GuardClick "submitPromo"
    $g = Wait-PromoOk "30 минут"
    Write-Log ("Удача за {0:N1} с: стикер {1}, «{2}»" -f $sw.Elapsed.TotalSeconds, $g.promoSticker, $g.promoText)
    Assert-That ([string]$g.promoSticker -ne $script:FirstOk) "AB12 показал тот же стикер удачи, что CD34: $($g.promoSticker)"
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
    Assert-That ([int]$st.limitS -eq $script:LimitBefore + 1860) "Лимит дня $($st.limitS) с, ждали $($script:LimitBefore + 1860) (+1 мин CD34 и +30 мин AB12)"
  }

  Write-Log "S52: неверные коды — разные стикеры и надписи неудачи, верный — стикер удачи на одно проигрывание"
}
