# S55. Связь с телефоном: привязка кодом со сверкой числа, «+15» и «−15» с телефона (FR-40, NFR-16, M7).
# Сервер связи pcblock-relay.exe на раннере (127.0.0.1:18080), тестовый телефон remote-phone.exe вместо
# приложения (подписи и число сверки — функции internal/remote). Часы ядра обычные (ClockSpeed=1).
# Дано: связь включена, сервер запущен. Когда: неверный код; верный код, «Совпадает» на обоих; «+15», «−15»;
# команда с чужой подписью и повтор последней; 20 команд подряд. Тогда: неверный код отклонён (bad_code);
# число сверки окна равно числу телефона; доп. время +15 не позже 5 с и обратно, в журнале «вы добавили 15 мин
# с телефона» и «вы убавили 15 мин с телефона»; телефон видит новый остаток в подписанном отчёте; подделка и
# повтор отклонены и в журнал не попали; 95-й процентиль задержки команды не больше 5 с (NFR-16).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Записи журнала про доп. время (с окна и с телефона).
function Get-ExtraEvents {
  return @(Get-Events | Where-Object { $_.text -match 'с телефона|доп\. время' })
}

function Get-Card {
  return @(List (Get-Today).children)[0]
}

Invoke-Scenario "S55" "Связь с телефоном: привязка, +15 и −15 с телефона, подделка и повтор" {
  Initialize-Scenario 1
  try {
    Step "Настройка: ребёнок «Тест», лимит 120 мин; сервер связи запущен" {
      Invoke-Setup 120
      Start-Relay
    }

    Step "Связь включена: адрес http://127.0.0.1:18080, «На связи»" {
      Use-Parent {
        $v = Get-Remote
        Assert-That ($v.state -eq "off" -and -not $v.enabled) "До включения связь не выключена: $(Format-Remote $v)"
        $r = Send-Parent "SetRemote" @{ enabled = $true; url = $script:RelayUrl }
        Assert-Reply $r "SetRemote"
        $v = Wait-RemoteState "online" 30
        Write-Log "На связи через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-Remote $v)"
        Assert-That ((Find-Events 'Связь с телефоном включена').Count -eq 1) "В журнале нет «Связь с телефоном включена»"
      }
    }

    $script:Code = ""
    Step "«Подключить телефон»: код из 6 цифр" {
      Use-Parent {
        Assert-Reply (Send-Parent "RemotePairStart") "RemotePairStart"
        Wait-Until { $script:LastRemote = Get-Remote; $script:LastRemote.pair.stage -eq "code" -and $script:LastRemote.pair.code -match '^\d{6}$' } 10 "код привязки в окне" 300
        $script:Code = [string]$script:LastRemote.pair.code
        Write-Log "Код привязки через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-Remote $script:LastRemote)"
      }
    }

    Step "Неверный код: телефон получает bad_code, привязки нет" {
      $bad = if ($script:Code -eq "000000") { "000001" } else { "000000" }
      $r = Invoke-Phone @("pair", "-url", $script:RelayUrl, "-code", $bad, "-name", "Pixel-S55") (Join-Path $script:Work "phone-bad.json")
      Assert-That ($r.Code -ne 0 -and $r.Result.error -eq "bad_code") "Неверный код: код выхода $($r.Code), ответ $($r.Result | ConvertTo-Json -Compress)"
      Use-Parent {
        $v = Get-Remote
        Assert-That ($v.pair.stage -eq "code" -and $v.pair.code -eq $script:Code -and -not $v.phone) "После неверного кода окно изменилось: $(Format-Remote $v)"
      }
    }

    $script:Sas = ""
    Step "Верный код: число сверки окна равно числу телефона" {
      $r = Invoke-Phone @("pair", "-url", $script:RelayUrl, "-code", $script:Code, "-name", "Pixel-S55")
      Assert-That ($r.Code -eq 0 -and [string]$r.Result.sas -match '^\d{6}$') "Привязка с телефона не удалась: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      $script:Sas = [string]$r.Result.sas
      Use-Parent {
        Wait-Until { $script:LastRemote = Get-Remote; $script:LastRemote.pair.stage -eq "compare" } 10 "сверка числа в окне" 300
        $v = $script:LastRemote
        Write-Log "Сверка: телефон $($script:Sas), окно $(Format-Remote $v)"
        Assert-That ([string]$v.pair.sas -eq $script:Sas) "Число сверки окна '$($v.pair.sas)' не равно числу телефона '$($script:Sas)'"
        Assert-That ([string]$v.pair.phoneName -eq "Pixel-S55") "Имя телефона в окне '$($v.pair.phoneName)', ждали Pixel-S55"
      }
    }

    Step "«Совпадает» на компьютере и на телефоне: телефон подключён" {
      Use-Parent {
        Assert-Reply (Send-Parent "RemotePairConfirm" @{ match = $true }) "RemotePairConfirm"
        Wait-Until { $script:LastRemote = Get-Remote; $null -ne $script:LastRemote.phone -and $script:LastRemote.pair.stage -eq "idle" } 10 "телефон подключён в окне" 300
        Write-Log "Подключён: $(Format-Remote $script:LastRemote)"
        Assert-That ([string]$script:LastRemote.phone.name -eq "Pixel-S55") "Имя подключённого телефона '$($script:LastRemote.phone.name)'"
        Wait-Until { (Find-Events 'Телефон подключён').Count -ge 1 } 10 "запись «Телефон подключён» в журнале"
      }
      $r = Invoke-Phone @("confirm")
      Assert-That ($r.Code -eq 0) "confirm на телефоне: код $($r.Code)"
    }

    $script:ChildId = 0
    $script:Left = 0
    Step "«+15» с телефона: доп. время +15 не позже 5 с, «вы добавили 15 мин с телефона»" {
      Use-Parent {
        $script:ChildId = Get-ChildId
        $card = Get-Card
        Assert-That ($card.extraMin -eq 0) "До команды доп. время $($card.extraMin) мин, ждали 0"
      }
      $sw = [Diagnostics.Stopwatch]::StartNew()
      $r = Invoke-Phone @("cmd", "-child", "$($script:ChildId)", "-delta", "15")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Команда +15 не принята: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      Use-Parent {
        Wait-Until { (Get-Card).extraMin -eq 15 } 5 "доп. время +15 у ребёнка"
        $applied = $sw.Elapsed.TotalSeconds
        Write-Log ("+15 применено за {0:N2} с (задержка команды у телефона {1} мс, ответ «{2}», осталось {3} с)" -f $applied, $r.Result.ms, $r.Result.message, $r.Result.leftSec)
        Assert-That ($applied -le 5) ("+15 применено за {0:N2} с, ждали не больше 5" -f $applied)
        $card = Get-Card
        Assert-That ([math]::Abs([int64]$r.Result.leftSec - [int64]$card.status.remainingS) -le 10) "Остаток в ответе телефону $($r.Result.leftSec) с, в окне $($card.status.remainingS) с"
        Write-Log "Журнал: $((Get-ExtraEvents | ForEach-Object { $_.text }) -join ' | ')"
        Assert-That ((Find-Events 'вы добавили 15 мин с телефона').Count -eq 1) "Записей «вы добавили 15 мин с телефона»: $((Find-Events 'вы добавили 15 мин с телефона').Count), ждали одну"
      }
    }

    Step "«−15» с телефона: доп. время обратно, «вы убавили 15 мин с телефона»" {
      $script:BeforeMinus = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
      $sw = [Diagnostics.Stopwatch]::StartNew()
      $r = Invoke-Phone @("cmd", "-child", "$($script:ChildId)", "-delta", "-15")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Команда −15 не принята: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      $script:Left = [int64]$r.Result.leftSec
      Use-Parent {
        Wait-Until { (Get-Card).extraMin -eq 0 } 5 "доп. время обратно 0"
        Write-Log ("−15 применено за {0:N2} с (задержка {1} мс, «{2}», осталось {3} с)" -f $sw.Elapsed.TotalSeconds, $r.Result.ms, $r.Result.message, $r.Result.leftSec)
        Assert-That ((Find-Events 'вы убавили 15 мин с телефона').Count -eq 1) "Записей «вы убавили 15 мин с телефона»: $((Find-Events 'вы убавили 15 мин с телефона').Count), ждали одну"
      }
    }

    Step "Телефон видит новый остаток: подписанный отчёт после команды" {
      Wait-Until {
        $script:LastStatus = (Invoke-Phone @("status")).Result
        $script:LastStatus.report -and [int64]$script:LastStatus.reportTs -ge $script:BeforeMinus
      } 15 "отчёт компьютера новее команды «−15»" 1000
      $st = $script:LastStatus
      $ch = @(List $st.children) | Where-Object { [int64]$_.id -eq $script:ChildId } | Select-Object -First 1
      Assert-That ($null -ne $ch) "В отчёте нет ребёнка $($script:ChildId): $($st | ConvertTo-Json -Compress -Depth 5)"
      Write-Log "Отчёт: на связи $($st.online), ребёнок $($ch.name): осталось $($ch.leftSec) с, доп. время $($ch.extraMin) мин, за компьютером $($ch.usedSec) с"
      Assert-That ($st.online) "Телефон видит компьютер не на связи: $($st | ConvertTo-Json -Compress -Depth 5)"
      Assert-That ([int]$ch.extraMin -eq 0) "Доп. время в отчёте $($ch.extraMin), ждали 0"
      Assert-That ([math]::Abs([int64]$ch.leftSec - $script:Left) -le 30) "Остаток в отчёте $($ch.leftSec) с, после команды было $($script:Left) с"
    }

    Step "Команда с чужой подписью и повтор последней: отказ, журнал без новых записей" {
      $before = @(Use-Parent { Get-ExtraEvents }).Count
      $f = Invoke-Phone @("forge-cmd", "-child", "$($script:ChildId)", "-delta", "15")
      Assert-That ($f.Code -eq 2 -and -not $f.Result.ok -and $f.Result.error -eq "bad_sig") "Подделка: код $($f.Code), $($f.Result | ConvertTo-Json -Compress)"
      $p = Invoke-Phone @("replay")
      Assert-That ($p.Code -eq 2 -and -not $p.Result.ok -and $p.Result.error -eq "replay") "Повтор: код $($p.Code), $($p.Result | ConvertTo-Json -Compress)"
      Start-Sleep -Seconds 2
      Use-Parent {
        $after = @(Get-ExtraEvents).Count
        Assert-That ($after -eq $before) "Записей о доп. времени было $before, стало ${after}: $((Get-ExtraEvents | ForEach-Object { $_.text }) -join ' | ')"
        $card = Get-Card
        Assert-That ($card.extraMin -eq 0) "После подделки и повтора доп. время $($card.extraMin) мин, ждали 0"
      }
    }

    Step "20 команд подряд: 95-й процентиль задержки не больше 5 с (NFR-16)" {
      $ms = @()
      for ($i = 1; $i -le 20; $i++) {
        $d = if ($i % 2 -eq 1) { "1" } else { "-1" }
        $r = Invoke-Phone @("cmd", "-child", "$($script:ChildId)", "-delta", $d)
        Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Команда $i ($d) не принята: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
        $ms += [int64]$r.Result.ms
      }
      $sorted = @($ms | Sort-Object)
      $p95 = $sorted[18]
      Write-Log "Задержки 20 команд, мс: $($ms -join ', '); p95 $p95, наибольшая $($sorted[19])"
      Write-Host "::notice title=S55 NFR-16::20 команд с телефона: p95 $p95 мс, наибольшая $($sorted[19]) мс"
      Assert-That ($p95 -le 5000) "95-й процентиль задержки $p95 мс, ждали не больше 5000"
      Use-Parent {
        Assert-That ((Get-Card).extraMin -eq 0) "После 20 команд ±1 доп. время $((Get-Card).extraMin) мин, ждали 0"
        $plus = (Find-Events 'вы добавили 1 мин с телефона').Count
        $minus = (Find-Events 'вы убавили 1 мин с телефона').Count
        Assert-That ($plus -eq 10 -and $minus -eq 10) "Записей «+1» $plus и «−1» $minus, ждали по 10"
      }
    }
  } catch {
    Write-RelayLog
    throw
  } finally {
    Stop-Relay
  }

  Write-Log "S55: привязка со сверкой числа, +15 и −15 с телефона в журнале, подделка и повтор отклонены, p95 в норме"
}
