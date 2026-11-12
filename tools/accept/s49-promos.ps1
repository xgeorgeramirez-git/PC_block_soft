# S49. Раздел «Промокоды»: новые коды только A–Z и 0–9, показ заглавными, «Придумать коды», «Очистить историю»
# (R26, FR-35, D-75, D-75a, RM5-11, RM5-12, RM5-14).
# Часы ядра идут в 60 раз быстрее. Лимит 5 мин. Ввод в поле стража и нажатия — через TestGuard (Ruling 5);
# родительскую сессию закрываем до ожидания стража (Ruling 19).
# Когда: родитель создаёт «ПРИВЕТ», «ab 12», «код!», затем «ab12»; «Придумать коды»: 20 кодов, длина 6, «Буквы
# и цифры», 15 мин; время вышло, ребёнок вводит один из придуманных строчными на экране блокировки; родитель
# «Очистить историю».
# Тогда: первые три — «Только латинские буквы A-Z и цифры» у поля code, не созданы; «ab12» показан «AB12»;
# 20 разных кодов по 6 знаков из A–Z и 2–9 без похожих I, O, 0, 1 (FR-35), у каждого 15 мин; введённый код —
# «Добавлено 15 минут», экран снят, лимит дня +15 мин, код в «Использованных» с датой, временем и именем ребёнка;
# после очистки «Использованные» пусты, в «Действующих» 20 кодов, в журнале «История промокодов очищена».
# Снимки: s49-promo-ok.png (экран снят, плашка «Добавлено 15 минут»), s49-promos.png (окно родителя).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

function Enter-GuardPromo([string]$Text) {
  Invoke-TestGuard "type" "promo" $Text
  [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" -and $_.text -eq $Text }).Count -ge 1 } 8 "в поле промокода набран текст")
}

function Get-Promos {
  $l = Send-Parent "ListPromos"
  Assert-Reply $l "ListPromos"
  return $l.payload
}

function Format-Promos($L) {
  $a = (@(List $L.active) | ForEach-Object { "$($_.codeDisplay)/$($_.minutes)" }) -join ','
  $u = (@(List $L.used) | ForEach-Object { "$($_.codeDisplay)/$($_.minutes)@$($_.usedAtMs):$($_.usedByName)" }) -join ','
  return "действующие [$a]; использованные [$u]"
}

Invoke-Scenario "S49" "Промокоды: только A–Z и цифры, «Придумать коды», ввод строчными, «Очистить историю»" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Новые коды «ПРИВЕТ», «ab 12», «код!» отклонены у поля code, «ab12» создан и показан «AB12»" {
    Start-Child 5
    Open-Parent
    foreach ($bad in @("ПРИВЕТ", "ab 12", "код!")) {
      $r = Send-Parent "CreatePromo" @{ code = $bad; minutes = 10 }
      Write-Log "CreatePromo «$bad»: ok=$($r.ok), поле $($r.field), «$($r.error)»"
      Assert-That (-not $r.ok) "Код «$bad» создан, а нельзя (R26: только A–Z и 0–9)"
      Assert-That ([string]$r.field -eq "code" -and [string]$r.error -eq "Только латинские буквы A-Z и цифры") "Код «$bad»: поле «$($r.field)», текст «$($r.error)», ждали code и «Только латинские буквы A-Z и цифры»"
    }
    $l = Get-Promos
    Assert-That (@(List $l.active).Count -eq 0 -and @(List $l.used).Count -eq 0) "После отказов в списках есть коды: $(Format-Promos $l)"
    $ok = Send-Parent "CreatePromo" @{ code = "ab12"; minutes = 10 }
    Assert-Reply $ok "CreatePromo ab12"
    Assert-That ([string]$ok.payload.promo.codeDisplay -ceq "AB12") "Код ab12 показан «$($ok.payload.promo.codeDisplay)», ждали «AB12»"
    $l = Get-Promos
    $a = @(List $l.active)
    Assert-That ($a.Count -eq 1 -and [string]$a[0].codeDisplay -ceq "AB12") "«Действующие»: $(Format-Promos $l), ждали один «AB12»"
  }

  Step "«Придумать коды»: 20 разных по 6 знаков из A–Z и 2–9 без I, O, 0, 1, у каждого 15 мин" {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $g = Send-Parent "GeneratePromos" @{ count = 20; length = 6; charset = "mixed"; minutes = 15 }
    Assert-Reply $g "GeneratePromos"
    Write-Log ("GeneratePromos за {0} мс: «{1}»; коды: {2}" -f $sw.ElapsedMilliseconds, $g.payload.message, ((@(List $g.payload.promos) | ForEach-Object { $_.codeDisplay }) -join ' '))
    Assert-That ([string]$g.payload.message -eq "Готово: 20 кодов на 15 мин") "Сообщение «$($g.payload.message)», ждали «Готово: 20 кодов на 15 мин»"
    $codes = @(List $g.payload.promos | ForEach-Object { [string]$_.codeDisplay })
    Assert-That ($codes.Count -eq 20) "Придумано $($codes.Count) кодов, ждали 20"
    foreach ($p in @(List $g.payload.promos)) {
      Assert-That ([string]$p.codeDisplay -cmatch '^[A-HJ-NP-Z2-9]{6}$') "Код «$($p.codeDisplay)» не из 6 знаков A–Z и 2–9 без I, O, 0, 1"
      Assert-That ([int]$p.minutes -eq 15) "У кода $($p.codeDisplay) $($p.minutes) мин, ждали 15"
    }
    $uniq = @($codes | ForEach-Object { $_.ToUpperInvariant() } | Sort-Object -Unique)
    Assert-That ($uniq.Count -eq 20 -and -not ($uniq -contains "AB12")) "Коды не все разные или совпали с AB12: $($codes -join ' ')"
    $l = Get-Promos
    $active = @(List $l.active)
    Assert-That ($active.Count -eq 21) "«Действующих» $($active.Count), ждали 21 (AB12 и 20 придуманных)"
    foreach ($c in $codes) {
      $row = @($active | Where-Object { [string]$_.codeDisplay -ceq $c })
      Assert-That ($row.Count -eq 1 -and [int]$row[0].minutes -eq 15) "Кода $c нет в «Действующих» с 15 мин: $(Format-Promos $l)"
    }
    Assert-That ((Find-Events "Придумано 20 промокодов").Count -eq 1) "В журнале нет одной записи «Придумано 20 промокодов»: $(Format-Events)"
    $script:Code = $codes[7]
    $script:LimitBefore = [int](Get-Status).limitS
    Assert-Reply (Send-Parent "Logout") "Logout"
    $script:Token = ""
    Resume-Clock
  }

  Step "Время вышло: ребёнок вводит придуманный код строчными — «Добавлено 15 минут», экран снят, лимит +15 мин" {
    $g = Wait-LockShown 40 "timeUp"
    Suspend-Clock
    Assert-That (Test-HasButton $g "promo") "Нет кнопки «Ввести промокод»: $(Format-GuardState $g)"
    Invoke-GuardClick "promo"
    [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" }).Count -ge 1 } 8 "панель промокода открыта")
    $lower = $script:Code.ToLowerInvariant()
    Write-Log "Ребёнок вводит «$lower» (код $($script:Code))"
    Enter-GuardPromo $lower
    Invoke-GuardClick "submitPromo"
    $g = Wait-GuardState { param($s) (-not $s.lockShown) -and [string]$s.plate -eq "Добавлено 15 минут" } 10 "экран снят и плашка «Добавлено 15 минут»"
    Save-Screen "s49-promo-ok"
    $st = Get-Status
    Write-Log "После кода: статус $($st.status), лимит $($st.limitS) с (был $($script:LimitBefore)), остаток $($st.remainingS) с"
    Assert-That ($st.lock -eq "none") "Блокировка после верного кода: '$($st.lock)'"
    Assert-That ([int]$st.limitS -eq $script:LimitBefore + 900) "Лимит дня $($st.limitS) с, ждали $($script:LimitBefore + 900) (+15 мин по коду)"
  }

  Step "Родитель: код в «Использованных» с датой, временем и именем ребёнка" {
    Open-Parent
    $l = Get-Promos
    Write-Log "Списки: $(Format-Promos $l)"
    $used = @(List $l.used)
    Assert-That ($used.Count -eq 1 -and [string]$used[0].codeDisplay -ceq $script:Code) "«Использованные»: $(Format-Promos $l), ждали один $($script:Code)"
    Assert-That ([int64]$used[0].usedAtMs -gt 0 -and [string]$used[0].usedByName -eq "Тест") "У использованного кода момент $($used[0].usedAtMs), ребёнок «$($used[0].usedByName)», ждали момент и «Тест»"
    Assert-That (@(List $l.active).Count -eq 20) "«Действующих» $(@(List $l.active).Count), ждали 20"
    Assert-That ((Find-Events "Промокод: \+15 мин").Count -ge 1) "В журнале нет записи об использовании кода: $(Format-Events)"
  }

  Step "«Очистить историю»: «Использованные» пусты, «Действующих» 20, запись в журнале" {
    $r = Send-Parent "ClearPromoHistory"
    Assert-Reply $r "ClearPromoHistory"
    Assert-That ([string]$r.payload.message -eq "История промокодов очищена") "Сообщение «$($r.payload.message)», ждали «История промокодов очищена»"
    $l = Get-Promos
    Write-Log "После очистки: $(Format-Promos $l)"
    Assert-That (@(List $l.used).Count -eq 0) "«Использованные» не пусты: $(Format-Promos $l)"
    $active = @(List $l.active)
    Assert-That ($active.Count -eq 20 -and @($active | Where-Object { [string]$_.codeDisplay -ceq "AB12" }).Count -eq 1) "«Действующих» $($active.Count), ждали 20 с AB12"
    Assert-That ((Find-Events "История промокодов очищена").Count -ge 1) "В журнале нет «История промокодов очищена»: $(Format-Events)"
    Assert-Reply (Send-Parent "Logout") "Logout"
    $script:Token = ""
  }

  Step "Снимок окна родителя" {
    [void](Save-ProgramWindow "s49-promos" @("--parent") 40 5)
  }

  Write-Log "S49: новые коды только A–Z и цифры заглавными, придуманные коды разные и действуют, история очищается"
}
