# S62. Промокоды с телефона (FR-45, D-106, R26, протокол связи 1.5, раздел 5г).
# Сервер связи pcblock-relay.exe на раннере (127.0.0.1:18080), тестовый телефон remote-phone.exe вместо приложения.
# Часы ядра идут в 60 раз быстрее и стоят на паузе; лимит 5 мин: перед вводом кода часы идут, время выходит, и ребёнок
# вводит код на экране блокировки («Ввести промокод» стража, как S49). Команда RedeemPromo из PowerShell раннера
# не подходит: учётка раннера — администратор, служба отвечает «недоступно из этой учётной записи» (B-70).
# Дано: телефон привязан (как S55). Когда: телефон создаёт SUMMER на 30 мин, придумывает 5 кодов по 15 мин длиной 6
# («Буквы и цифры»), удаляет SUMMER; ребёнок вводит один из придуманных; телефон снова создаёт SUMMER.
# Тогда: «Действующие» телефона и окна родителя совпадают (5 кодов без SUMMER); введённый код — в «Использованных»
# на телефоне и в окне с именем ребёнка и временем; SUMMER после удаления создаётся снова без «promo_exists»;
# журнал — «Промокод SUMMER на 30 мин создан с телефона», «Придумано 5 промокодов на 15 мин с телефона»,
# «Промокод SUMMER на 30 мин удалён с телефона». Снимок: s62-promos.png (окно родителя, «Промокоды»).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

function Get-WindowPromos {
  $l = Send-Parent "ListPromos"
  Assert-Reply $l "ListPromos"
  return $l.payload
}

function Format-PhonePromos($P) {
  $a = (@(List $P.active) | ForEach-Object { "$($_.code)/$($_.min)" }) -join ','
  $u = (@(List $P.used) | ForEach-Object { "$($_.code)/$($_.min)@$($_.usedMs):$($_.childName)" }) -join ','
  return "действующие [$a]; использованные [$u]"
}

# Команда промокодов с телефона; ждёт ok и возвращает вывод тестового телефона.
function Invoke-PhonePromo([string[]]$ArgList, [string]$What) {
  $r = Invoke-Phone $ArgList
  Assert-That ($r.Code -eq 0 -and $r.Result.ok) "${What}: не принято, код $($r.Code), $($r.Result | ConvertTo-Json -Compress -Depth 6)"
  Assert-That ($null -ne $r.Result.promos) "${What}: в ответе нет списка промокодов: $($r.Result | ConvertTo-Json -Compress -Depth 6)"
  Write-Log "${What}: «$($r.Result.message)»; $(Format-PhonePromos $r.Result.promos)"
  return $r.Result
}

Invoke-Scenario "S62" "Промокоды с телефона: создать, придумать, удалить; списки телефона и окна совпадают" {
  Initialize-Scenario 60
  try {
    Step "Настройка: ребёнок «Миша», лимит 5 мин, часы на паузе; телефон привязан" {
      Test-InputSim
      Start-Input 1000
      Invoke-Setup 5 $script:MySid "Миша"
      Suspend-Clock
      Wait-SessionParts
      $script:ChildId = Connect-Phone "Pixel-S62"
    }

    Step "Телефон создаёт SUMMER на 30 мин" {
      $r = Invoke-PhonePromo @("promo-create", "-code", "SUMMER", "-min", "30") "Создать SUMMER"
      Assert-That ([string]$r.message -eq "Промокод SUMMER на 30 минут готов") "Ответ «$($r.message)», ждали «Промокод SUMMER на 30 минут готов»"
      $row = @(@(List $r.promos.active) | Where-Object { [string]$_.code -ceq "SUMMER" })
      Assert-That ($row.Count -eq 1 -and [int]$row[0].min -eq 30) "SUMMER на 30 мин нет в «Действующих» телефона: $(Format-PhonePromos $r.promos)"
    }

    $script:Codes = @()
    Step "Телефон придумывает 5 кодов по 15 мин длиной 6 («Буквы и цифры»)" {
      $r = Invoke-PhonePromo @("promo-gen", "-count", "5", "-len", "6", "-charset", "mixed", "-min", "15") "Придумать 5 кодов"
      Assert-That ([string]$r.message -eq "Готово: 5 кодов на 15 мин") "Ответ «$($r.message)», ждали «Готово: 5 кодов на 15 мин»"
      $script:Codes = @(List $r.codes | ForEach-Object { [string]$_ })
      Write-Log "Придуманные коды: $($script:Codes -join ' ')"
      Assert-That ($script:Codes.Count -eq 5) "Придумано $($script:Codes.Count) кодов, ждали 5"
      foreach ($c in $script:Codes) {
        Assert-That ($c -cmatch '^[A-HJ-NP-Z2-9]{6}$') "Код «$c» не из 6 знаков A–Z и 2–9 без I, O, 0, 1 (FR-35)"
      }
      Assert-That (@($script:Codes | Sort-Object -Unique).Count -eq 5) "Коды повторяются: $($script:Codes -join ' ')"
    }

    Step "Телефон удаляет SUMMER: «Действующие» телефона и окна совпадают — 5 кодов без SUMMER" {
      $r = Invoke-PhonePromo @("promo-delete", "-code", "SUMMER") "Удалить SUMMER"
      Assert-That ([string]$r.message -eq "Промокод SUMMER удалён") "Ответ «$($r.message)», ждали «Промокод SUMMER удалён»"
      $p = Invoke-PhonePromo @("promos") "Список промокодов"
      $phone = @(@(List $p.promos.active) | ForEach-Object { [string]$_.code } | Sort-Object)
      $win = Use-Parent { @(@(List (Get-WindowPromos).active) | ForEach-Object { [string]$_.codeDisplay } | Sort-Object) }
      $win = @($win)
      Write-Log "«Действующие»: телефон [$($phone -join ' ')], окно [$($win -join ' ')]"
      Assert-That ($phone.Count -eq 5 -and -not ($phone -contains "SUMMER")) "На телефоне «Действующих» [$($phone -join ' ')], ждали 5 кодов без SUMMER"
      Assert-That (($phone -join ',') -ceq ($win -join ',')) "«Действующие» телефона [$($phone -join ' ')] и окна [$($win -join ' ')] не совпадают"
      Assert-That ((($script:Codes | Sort-Object) -join ',') -ceq ($phone -join ',')) "«Действующие» [$($phone -join ' ')] не те, что придуманы [$($script:Codes -join ' ')]"
    }

    Step "Ребёнок вводит придуманный код: он во «Использованных» с именем и временем — на телефоне и в окне" {
      $script:Used = $script:Codes[2]
      $since = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
      Resume-Clock
      $g = Wait-LockShown 40 "timeUp"
      Suspend-Clock
      Assert-That (Test-HasButton $g "promo") "Нет кнопки «Ввести промокод»: $(Format-GuardState $g)"
      Invoke-GuardClick "promo"
      [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" }).Count -ge 1 } 8 "панель промокода открыта")
      $lower = $script:Used.ToLowerInvariant()
      Invoke-TestGuard "type" "promo" $lower
      [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "promo" -and $_.text -eq $lower }).Count -ge 1 } 8 "в поле промокода набран текст")
      Invoke-GuardClick "submitPromo"
      [void](Wait-PromoOk "15 минут")
      [void](Wait-LockHidden 10)
      Write-Log "Ребёнок ввёл «$lower» на экране блокировки: стикер удачи, экран снят"
      $p = Invoke-PhonePromo @("promos") "Список промокодов после ввода"
      $u = @(@(List $p.promos.used) | Where-Object { [string]$_.code -ceq $script:Used })
      Assert-That ($u.Count -eq 1 -and [string]$u[0].childName -eq "Миша" -and [int64]$u[0].usedMs -ge $since - 5000 -and [int]$u[0].min -eq 15) "На телефоне в «Использованных» нет $($script:Used) от Миши на 15 мин со временем: $(Format-PhonePromos $p.promos)"
      Assert-That (@(List $p.promos.active).Count -eq 4 -and -not (@(@(List $p.promos.active) | ForEach-Object { [string]$_.code }) -contains $script:Used)) "Введённый код остался в «Действующих» телефона: $(Format-PhonePromos $p.promos)"
      Use-Parent {
        $l = Get-WindowPromos
        $wu = @(@(List $l.used) | Where-Object { [string]$_.codeDisplay -ceq $script:Used })
        Assert-That ($wu.Count -eq 1 -and [string]$wu[0].usedByName -eq "Миша" -and [int64]$wu[0].usedAtMs -eq [int64]$u[0].usedMs) "В окне «Использованные» не совпадают с телефоном: окно $(($wu | ForEach-Object { "$($_.codeDisplay)@$($_.usedAtMs):$($_.usedByName)" }) -join ','), телефон $($u[0].code)@$($u[0].usedMs):$($u[0].childName)"
      }
    }

    Step "SUMMER создаётся снова без «Такой код уже есть»" {
      $r = Invoke-PhonePromo @("promo-create", "-code", "SUMMER", "-min", "30") "Создать SUMMER снова"
      Assert-That ([string]$r.message -eq "Промокод SUMMER на 30 минут готов") "Ответ «$($r.message)», ждали «Промокод SUMMER на 30 минут готов»"
    }

    Step "Журнал: создание, придумывание и удаление — записи «с телефона»" {
      Use-Parent {
        $created = @(Find-Events '^Промокод SUMMER на 30 мин создан с телефона$')
        $gen = @(Find-Events '^Придумано 5 промокодов на 15 мин с телефона$')
        $deleted = @(Find-Events '^Промокод SUMMER на 30 мин удалён с телефона$')
        @($created + $gen + $deleted) | ForEach-Object { Write-Log "Журнал: $($_.text)" }
        Assert-That ($created.Count -eq 2) "Записей «Промокод SUMMER на 30 мин создан с телефона»: $($created.Count), ждали 2; журнал: $(Format-Events)"
        Assert-That ($gen.Count -eq 1) "Записей «Придумано 5 промокодов на 15 мин с телефона»: $($gen.Count), ждали 1; журнал: $(Format-Events)"
        Assert-That ($deleted.Count -eq 1) "Записей «Промокод SUMMER на 30 мин удалён с телефона»: $($deleted.Count), ждали 1; журнал: $(Format-Events)"
      }
    }

    Step "Снимок окна родителя «Промокоды»" {
      [void](Save-ParentWindow "s62-promos" "promos")
    }
  } catch {
    Write-RelayLog
    throw
  } finally {
    Stop-Relay
  }

  Write-Log "S62: промокоды с телефона созданы, придуманы и удалены, списки телефона и окна совпадают, SUMMER создаётся снова"
}
