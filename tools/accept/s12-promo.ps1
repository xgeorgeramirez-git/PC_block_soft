# S12. Промокод на время (R26, FR-35, D-50): родитель создаёт коды, ребёнок вводит код на экране
# блокировки «Время вышло». Верный код (без учёта регистра) снимает блокировку и добавляет минуты
# к сегодняшнему лимиту; неверный и уже использованный — «Код не подходит», без изменений.
# Часы ядра идут в 60 раз быстрее. Лимит 5 мин; коды A1234 на 30 мин и zz9 на 15 мин.
# Ввод в поле стража и нажатия идут через TestGuard (настоящие сообщения окон, Ruling 5).
# Родительскую сессию закрываем (Logout) до ожидания стража (Ruling 19).
# Снимки: s12-promo-panel.png (панель ввода кода), s12-promo-ok.png (экран снят после кода).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Вводит текст в поле промокода на панели стража и ждёт, пока страж покажет его в GuardState.
function Enter-GuardPromo([string]$Text) {
  Invoke-TestGuard "type" "promo" $Text
  [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" -and $_.text -eq $Text }).Count -ge 1 } 8 "в поле промокода набран текст")
}

# Нажимает «Ввести промокод», вводит код и нажимает «Применить».
function Submit-GuardPromo([string]$Code) {
  Invoke-GuardClick "promo"
  [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" }).Count -ge 1 } 8 "панель промокода открыта")
  Enter-GuardPromo $Code
  Invoke-GuardClick "submitPromo"
}

Invoke-Scenario "S12" "Промокод: ребёнок вводит код на экране блокировки" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Родитель создаёт коды: A1234 на 30 мин, zz9 на 15 мин; повтор и неверные минуты отклоняются" {
    Start-Child 5
    Open-Parent
    $a = Send-Parent "CreatePromo" @{ code = "A1234"; minutes = 30 }
    Assert-Reply $a "CreatePromo A1234"
    Assert-That ($a.payload.promo.minutes -eq 30) "У кода A1234 минут $($a.payload.promo.minutes), ждали 30"
    Assert-Reply (Send-Parent "CreatePromo" @{ code = "zz9"; minutes = 15 }) "CreatePromo zz9"
    $dup = Send-Parent "CreatePromo" @{ code = " a1234 "; minutes = 10 }
    Assert-That (-not $dup.ok -and $dup.field -eq "code") "Повтор кода принят или ошибка не у поля code: ok=$($dup.ok) field=$($dup.field)"
    $zero = Send-Parent "CreatePromo" @{ code = "q1"; minutes = 0 }
    Assert-That (-not $zero.ok -and $zero.field -eq "minutes") "Ноль минут принят или ошибка не у поля minutes: ok=$($zero.ok) field=$($zero.field)"
    $l = Send-Parent "ListPromos"
    Assert-Reply $l "ListPromos"
    Assert-That (@(List $l.payload.active).Count -eq 2 -and @(List $l.payload.used).Count -eq 0) "Списки: действующих $(@(List $l.payload.active).Count), использованных $(@(List $l.payload.used).Count), ждали 2 и 0"
    Assert-Reply (Send-Parent "Logout") "Logout"
    Resume-Clock
  }

  Step "Время вышло: на экране блокировки есть «Ввести промокод»" {
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    Assert-That (Test-HasButton $g "promo") "Нет кнопки «Ввести промокод»: $(Format-GuardState $g)"
  }

  Step "Неверный код: экран остаётся, лимит прежний, коды целы" {
    Submit-GuardPromo "nope77"
    Start-Sleep -Seconds 2
    $g = Get-GuardState
    $st = Get-Status
    Write-Log "После неверного кода: $(Format-GuardState $g); лимит $($st.limitS) с, блокировка '$($st.lock)'"
    Assert-That ($g.lockShown) "После неверного кода экран блокировки снят"
    Assert-That ($st.limitS -eq 300) "Лимит дня $($st.limitS) с, ждали 300 (неверный код ничего не добавляет)"
    Save-Screen "s12-promo-panel"
  }

  Step "Верный код в другом регистре: блокировка снята, лимит +30 мин" {
    # Панель после отказа остаётся открытой: заменяем текст в поле и применяем.
    Enter-GuardPromo "a1234"
    Invoke-GuardClick "submitPromo"
    [void](Wait-LockHidden 10)
    $st = Get-Status
    Write-Log "После кода: статус $($st.status), остаток $($st.remainingS) с, лимит $($st.limitS) с"
    Assert-That ($st.lock -eq "none") "Блокировка после верного кода: '$($st.lock)'"
    Assert-That ($st.limitS -eq 2100) "Лимит дня $($st.limitS) с, ждали 2100 (5 мин + 30 мин по коду)"
    Assert-Between $st.remainingS 1795 1800 "Остаток после кода, с"
    Save-Screen "s12-promo-ok"
  }

  Step "Родитель: A1234 в использованных с именем ребёнка, zz9 действует; запись в журнале" {
    Open-Parent
    $l = Send-Parent "ListPromos"
    Assert-Reply $l "ListPromos"
    $used = @(List $l.payload.used)
    Assert-That ($used.Count -eq 1 -and $used[0].usedByName -ne "") "Использованных $($used.Count) или нет имени ребёнка"
    Assert-That (@(List $l.payload.active).Count -eq 1) "Действующих $(@(List $l.payload.active).Count), ждали 1 (zz9)"
    Assert-That ((Find-Events "Промокод: \+30 мин").Count -ge 1) "В журнале нет записи «Промокод: +30 мин»"
    Assert-That ((Find-Events "Создан промокод").Count -ge 2) "В журнале нет записей о создании кодов"
    Assert-Reply (Send-Parent "Logout") "Logout"
    Resume-Clock
  }

  Step "Время снова вышло: повторный ввод A1234 отклонён, zz9 принят (+15 мин)" {
    $g = Wait-LockShown 60 "timeUp"
    Suspend-Clock
    Submit-GuardPromo "A1234"
    Start-Sleep -Seconds 2
    $g = Get-GuardState
    $st = Get-Status
    Assert-That ($g.lockShown) "Использованный код снял блокировку: $(Format-GuardState $g)"
    Assert-That ($st.limitS -eq 2100) "Лимит дня $($st.limitS) с после повторного кода, ждали 2100"
    Enter-GuardPromo "ZZ9"
    Invoke-GuardClick "submitPromo"
    [void](Wait-LockHidden 10)
    $st = Get-Status
    Assert-That ($st.lock -eq "none") "Блокировка после zz9: '$($st.lock)'"
    Assert-That ($st.limitS -eq 3000) "Лимит дня $($st.limitS) с, ждали 3000 (2100 + 15 мин)"
  }

  Write-Log "S12: коды работают один раз без учёта регистра, неверный и использованный не добавляют время"
}
