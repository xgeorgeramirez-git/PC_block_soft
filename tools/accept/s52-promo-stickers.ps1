# S52. Стикеры и надписи в ответ на промокод (R26, FR-35, D-83, D-84, D-85, план M5b T9, фидбек 0.6.1).
# Дано: время вышло, экран блокировки «Время вышло»; родитель создал коды AB12 на 30 мин и CD34 на 1 мин.
# Когда: на экране блокировки неверный код два раза подряд, затем CD34; время (1 мин) кончается, экран
# возвращается; снова CD34, минута без нажатий, затем AB12. Ввод кода в окне «Моя статистика» проверяют vitest и e2e (T8), команду ребёнка с обычными правами — шаг EF56 (B-70):
# скрипт работает повышенным администратором, а от него служба команду ребёнка не принимает, поэтому через runas /trustlevel (Ruling).
# Тогда: неверный — стикер из набора неудачи и надпись неудачи (GuardState promoSticker/promoText); ввод в
# поле ответ не убирает (стикер идёт по кругу, пока не введён верный код); второй неверный — другие стикер и
# надпись, лимит прежний; без нажатий 60 с панель промокода закрывается сама (обычные кнопки, ответа нет); CD34 — стикер удачи и надпись с «+1 минута», экран снят,
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
    # Новый ввод прошлый ответ не убирает (R26, D-85: стикер неудачи остаётся, пока не введён верный код).
    Enter-GuardPromo "WRONG2"
    Start-Sleep -Milliseconds 700
    $k = Get-GuardState
    Assert-That ([string]$k.promoSticker -eq [string]$a.promoSticker -and [string]$k.promoText -eq [string]$a.promoText) "Ввод в поле убрал или сменил ответ на неверный код: $(Format-GuardState $k)"
    Invoke-GuardClick "submitPromo"
    $first = [string]$a.promoSticker
    $b = Wait-GuardState { param($s) $s.lockShown -and ($script:PromoBadStickers -contains [string]$s.promoSticker) -and [string]$s.promoSticker -ne $first -and ($script:PromoBadTexts -contains [string]$s.promoText) } 8 "новый стикер и надпись неудачи после второго неверного кода"
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

  Step "Без нажатий 60 с панель промокода закрывается сама: обычные кнопки, стикера и надписи нет" {
    # Ввод в поле — нажатие: отсчёт 60 с идёт от него. Стикер неудачи при этом остаётся и идёт по кругу.
    Enter-GuardPromo "ZZ"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $bad = [string](Get-GuardState).promoSticker
    Assert-That ($script:PromoBadStickers -contains $bad) "После ввода в поле стикера неудачи нет: '$bad'"
    Start-Sleep -Seconds 40
    $g = Get-GuardState
    Assert-That ($g.lockShown -and [string]$g.promoSticker -eq $bad -and (@(List $g.fields) | Where-Object { $_.id -eq "promo" }).Count -eq 1) "Через 40 с без нажатий панель или стикер пропали: $(Format-GuardState $g)"
    $g = Wait-GuardState { param($s) $s.lockShown -and (@(List $s.fields) | Where-Object { $_.id -eq "promo" }).Count -eq 0 } 35 "панель промокода закрылась сама"
    Assert-Between $sw.Elapsed.TotalSeconds 55 72 "Панель промокода закрылась без нажатий через, с"
    Assert-That ([string]$g.promoSticker -eq "" -and [string]$g.promoText -eq "") "После закрытия панели остался ответ: $(Format-GuardState $g)"
    Assert-That (Test-HasButton $g "promo") "После закрытия панели нет кнопки «Ввести промокод»: $(Format-GuardState $g)"
    $st = Get-Status
    Assert-That ([int]$st.limitS -eq $script:LimitBefore + 60 -and $st.lock -ne "none") "После закрытия панели лимит $($st.limitS) с, блокировка '$($st.lock)'"
  }

  Step "AB12: стикер удачи с «+30 минут», экран держится один круг стикера и снимается, лимит +30 мин" {
    Invoke-GuardClick "promo"
    [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" }).Count -ge 1 } 8 "панель промокода открыта")
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

  Step "EF56 командой ребёнка (обычные права, как окно «Моя статистика»): +1 мин (B-70)" {
    Use-Parent {
      Assert-Reply (Send-Parent "CreatePromo" @{ code = "EF56"; minutes = 1 }) "CreatePromo EF56"
      $script:EfChild = Get-ChildId
    }
    $before = [int](Get-Status).limitS
    New-Item -ItemType Directory -Force -Path $script:Public | Out-Null
    & icacls.exe $script:Public /grant "*S-1-1-0:(OI)(CI)M" /T /C | Out-Null
    $out = Join-Path $script:Public "redeem-ef56.json"
    Remove-Item -LiteralPath $out -ErrorAction SilentlyContinue
    $payload = (@{ cmd = "RedeemPromo"; payload = @{ childId = $script:EfChild; code = "ef56" } } | ConvertTo-Json -Compress -Depth 4)
    $cmdFile = Join-Path $script:Public "redeem-ef56.cmd"
    Set-Content -LiteralPath $cmdFile -Encoding ASCII -Value ("@" + (ConvertTo-ArgString @($script:Exe, "--test-ipc", "--payload=$payload", "--out=$out")))
    # runas /trustlevel — тот же пользователь сеанса без повышения прав: служба видит окно ребёнка, а не администратора.
    & runas.exe /trustlevel:0x20000 "cmd.exe /c `"$cmdFile`"" 2>&1 | Out-Null
    Wait-Until { Test-Path -LiteralPath $out } 30 "ответ службы на RedeemPromo от обычных прав"
    $raw = Get-Content -LiteralPath $out -Raw -Encoding UTF8
    Write-Log "RedeemPromo от обычных прав: $($raw -replace '\s+', ' ')"
    $rep = $raw | ConvertFrom-Json
    Assert-That ($rep.ok) "RedeemPromo от окна ребёнка отклонён: $($rep.error)"
    $st = Get-Status
    Assert-That ([int]$st.limitS -eq $before + 60) "Лимит дня $($st.limitS) с, ждали $($before + 60) (+1 мин по EF56)"
  }

  Write-Log "S52: неверные коды — разные стикеры и надписи неудачи, ввод их не убирает, панель закрывается через 60 с без нажатий, верный — стикер удачи на один круг"
}
