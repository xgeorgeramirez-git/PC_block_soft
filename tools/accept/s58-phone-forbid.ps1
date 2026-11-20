# S58. Кнопки времени и снятие запрета с телефона (FR-42, D-98, протокол связи, раздел 5б).
# Сервер связи pcblock-relay.exe на раннере (127.0.0.1:18080), тестовый телефон remote-phone.exe. Часы ядра идут
# в 60 раз быстрее и стоят на паузе (старт в понедельник 15:00, как S37): время не уходит, запрет не кончается.
# Дано: телефон привязан (как S55), у ребёнка идут запретные часы (пн 15:00 -> вт 07:00), экран блокировки
# «forbidden»; лимит 600 мин, поэтому после «+30» и «−60» время остаётся. Когда: «+30», затем «−60», затем
# «Снять запрет на сегодня». Тогда: доп. время +30 и −30 итого (журнал «с телефона»); экран блокировки снят не
# позже 5 с, ребёнок видит плашку «Сегодня запретные часы не действуют»; в журнале «Тест: запрет на сегодня снят
# с телефона»; отчёт телефону — forbid=cleared (до снятия — active с концом интервала). Команда с чужой подписью
# не действует; повторное снятие — «уже снят» без второй записи; повтор seq — replay.
# Журнал читается только через Use-Parent: родительская сессия в сеансе ребёнка прячет плашки (Ruling 19).
# Снимок: s58-forbid-lifted.png (после снятия запрета с телефона).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

function Get-Card {
  return @(List (Get-Today).children)[0]
}

# Ребёнок в подписанном отчёте телефону (status), только если отчёт новее AfterMs.
function Get-PhoneChild([int64]$AfterMs = 0) {
  $st = (Invoke-Phone @("status")).Result
  if (-not $st.report -or [int64]$st.reportTs -lt $AfterMs) { return $null }
  return @(List $st.children) | Where-Object { [int64]$_.id -eq $script:ChildId } | Select-Object -First 1
}

Invoke-Scenario "S58" "Кнопки времени и снятие запрета на сегодня с телефона" {
  Initialize-Scenario 60
  try {
    Step "Имитация ввода доходит до сеанса" {
      Test-InputSim
    }

    Step "Настройка: лимит 600 мин, часы на паузе; сервер связи запущен, связь включена" {
      Start-Child 600
      Start-Relay
      Use-Parent {
        Assert-Reply (Send-Parent "SetRemote" @{ enabled = $true; url = $script:RelayUrl }) "SetRemote"
        $v = Wait-RemoteState "online" 30
        Write-Log "На связи: $(Format-Remote $v)"
      }
    }

    Step "Телефон привязан: код, сверка числа, «Совпадает» на обоих" {
      Use-Parent {
        Assert-Reply (Send-Parent "RemotePairStart") "RemotePairStart"
        Wait-Until { $script:LastRemote = Get-Remote; $script:LastRemote.pair.stage -eq "code" -and $script:LastRemote.pair.code -match '^\d{6}$' } 10 "код привязки в окне" 300
      }
      $r = Invoke-Phone @("pair", "-url", $script:RelayUrl, "-code", [string]$script:LastRemote.pair.code, "-name", "Pixel-S58")
      Assert-That ($r.Code -eq 0) "Привязка с телефона не удалась: $($r.Result | ConvertTo-Json -Compress)"
      $sas = [string]$r.Result.sas
      Use-Parent {
        Wait-Until { $script:LastRemote = Get-Remote; $script:LastRemote.pair.stage -eq "compare" } 10 "сверка числа в окне" 300
        Assert-That ([string]$script:LastRemote.pair.sas -eq $sas) "Число сверки окна '$($script:LastRemote.pair.sas)', у телефона '$sas'"
        Assert-Reply (Send-Parent "RemotePairConfirm" @{ match = $true }) "RemotePairConfirm"
        Wait-Until { $script:LastRemote = Get-Remote; $null -ne $script:LastRemote.phone } 10 "телефон подключён" 300
        $script:ChildId = Get-ChildId
      }
      $c = Invoke-Phone @("confirm")
      Assert-That ($c.Code -eq 0) "confirm на телефоне: код $($c.Code)"
    }

    Step "Запретные часы пн 15:00 -> вт 07:00: экран «forbidden», телефон видит «идут до…»" {
      $script:AfterSchedule = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
      Use-Parent {
        $iv = @(@{ weekday = 1; startMin = 900; endMin = 420 })
        Assert-Reply (Send-Parent "SaveSchedule" @{ childId = $script:ChildId; intervals = $iv }) "SaveSchedule"
      }
      $g = Wait-LockShown 20 "forbidden"
      Write-Log "Экран блокировки: $(Format-GuardState $g)"
      Wait-Until { $script:PhoneChild = Get-PhoneChild $script:AfterSchedule; $script:PhoneChild -and $script:PhoneChild.forbid -eq "active" } 75 "отчёт телефону: forbid=active" 1000
      Write-Log "Отчёт телефону: forbid=$($script:PhoneChild.forbid), до $($script:PhoneChild.forbidUntilMs), заблокирован $($script:PhoneChild.blocked)"
      Assert-That ([int64]$script:PhoneChild.forbidUntilMs -gt 0) "Нет конца идущего интервала (forbidUntilMs): $($script:PhoneChild | ConvertTo-Json -Compress)"
      $end = [DateTimeOffset]::FromUnixTimeMilliseconds([int64]$script:PhoneChild.forbidUntilMs)
      # Пояс службы — пояс раннера (часы ядра идут в нём): конец интервала — 07:00 по местному времени.
      Assert-That ($end.ToLocalTime().ToString("HH:mm") -eq "07:00") "Конец интервала в отчёте $($end.ToLocalTime().ToString('o')), ждали 07:00"
    }

    Step "«+30» с телефона: доп. время +30, «вы добавили 30 мин с телефона», экран запрета остаётся" {
      $r = Invoke-Phone @("cmd", "-child", "$($script:ChildId)", "-delta", "30")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok -and $r.Result.message -eq "Добавлено 30 минут") "Команда +30: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      Use-Parent {
        Wait-Until { (Get-Card).extraMin -eq 30 } 5 "доп. время +30"
        Assert-That ((Find-Events 'вы добавили 30 мин с телефона').Count -eq 1) "Нет одной записи «вы добавили 30 мин с телефона»"
      }
      $g = Get-GuardState
      Assert-That ($g.lockShown -and $g.lockKind -eq "forbidden") "Доп. время сняло экран запретных часов: $(Format-GuardState $g)"
    }

    Step "«−60» с телефона: доп. время −30 итого, «вы убавили 60 мин с телефона»" {
      $r = Invoke-Phone @("cmd", "-child", "$($script:ChildId)", "-delta", "-60")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Команда −60: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      Use-Parent {
        Wait-Until { (Get-Card).extraMin -eq -30 } 5 "доп. время −30 итого"
        Assert-That ((Find-Events 'вы убавили 60 мин с телефона').Count -eq 1) "Нет одной записи «вы убавили 60 мин с телефона»"
      }
      # Плашки доп. времени закрываются сами: дальше ждём только плашку о запрете.
      [void](Wait-GuardState { param($s) -not $s.plate } 20 "плашки доп. времени закрылись")
    }

    Step "«Снять запрет» с чужой подписью: отказ, экран запрета на месте, журнал без записи" {
      $f = Invoke-Phone @("forge-forbid-off", "-child", "$($script:ChildId)")
      Assert-That ($f.Code -eq 2 -and -not $f.Result.ok -and $f.Result.error -eq "bad_sig") "Подделка: код $($f.Code), $($f.Result | ConvertTo-Json -Compress)"
      Start-Sleep -Seconds 3
      $g = Get-GuardState
      Assert-That ($g.lockShown -and $g.lockKind -eq "forbidden") "Подделка сняла экран запрета: $(Format-GuardState $g)"
      Use-Parent {
        Assert-That ((Find-Events 'запрет на сегодня снят').Count -eq 0) "Подделка в журнале родителя"
        Assert-That (-not (Get-Card).noForbiddenToday) "Подделка сняла запрет в карточке"
      }
    }

    Step "«Снять запрет на сегодня»: экран снят не позже 5 с, плашка ребёнку, отчёт «снят»" {
      $sw = [Diagnostics.Stopwatch]::StartNew()
      $r = Invoke-Phone @("forbid-off", "-child", "$($script:ChildId)")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok -and $r.Result.message -eq "Запрет на сегодня снят") "Снять запрет: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      Assert-That ($r.Result.forbid -eq "cleared") "Отчёт в ответе: forbid=$($r.Result.forbid), ждали cleared"
      $g = Wait-LockHidden 5
      $hidden = $sw.Elapsed.TotalSeconds
      Write-Log ("Экран снят через {0:N2} с (ответ телефону {1} мс, осталось {2} с)" -f $hidden, $r.Result.ms, $r.Result.leftSec)
      Assert-That ($hidden -le 5) ("Экран снят через {0:N2} с, ждали не больше 5" -f $hidden)
      $g = Wait-GuardState { param($s) [string]$s.plate -eq "Сегодня запретные часы не действуют" } 5 "плашка «Сегодня запретные часы не действуют»"
      Assert-That ($g.plateKind -eq "notice") "Вид плашки «$($g.plateKind)», ждали notice"
      Save-Screen "s58-forbid-lifted"
      $st = Get-Status
      Write-Log "После снятия: блокировка '$($st.lock)', остаток $($st.remainingS) с, лимит $($st.limitS) с"
      Assert-That ($st.lock -eq "none") "Блокировка после снятия запрета: '$($st.lock)'"
      Assert-That ([int64]$st.remainingS -gt 0) "Время вышло: остаток $($st.remainingS) с"
    }

    Step "Журнал «запрет на сегодня снят с телефона», карточка «снят», отчёт телефону forbid=cleared" {
      [void](Wait-GuardState { param($s) -not $s.plate } 20 "плашка закрылась")
      Use-Parent {
        $found = Find-Events 'запрет на сегодня снят с телефона'
        Write-Log "Журнал: $((Get-Events | Where-Object { $_.text -match 'с телефона|[Зз]апрет' } | ForEach-Object { $_.text }) -join ' | ')"
        Assert-That ($found.Count -eq 1) "Записей «запрет на сегодня снят с телефона»: $($found.Count), ждали одну"
        Assert-That ((Get-Card).noForbiddenToday) "В карточке запрет не снят"
      }
      $ch = Get-PhoneChild
      Assert-That ($ch -and $ch.forbid -eq "cleared" -and -not $ch.blocked) "Отчёт телефону: $($ch | ConvertTo-Json -Compress)"
    }

    Step "Повтор: «уже снят» без второй записи; тот же номер команды — replay" {
      $r = Invoke-Phone @("forbid-off", "-child", "$($script:ChildId)")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok -and $r.Result.message -eq "Запрет на сегодня уже снят") "Повторное снятие: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      $p = Invoke-Phone @("replay")
      Assert-That ($p.Code -eq 2 -and $p.Result.error -eq "replay") "Повтор номера: код $($p.Code), $($p.Result | ConvertTo-Json -Compress)"
      Use-Parent {
        Assert-That ((Find-Events 'запрет на сегодня снят с телефона').Count -eq 1) "Повтор записал журнал второй раз"
      }
    }
  } catch {
    Write-RelayLog
    throw
  } finally {
    Stop-Relay
  }

  Write-Log "S58: +30 и −60 с телефона в журнале, запрет снят с телефона не позже 5 с с плашкой и записью, отчёт «снят», подделка и повтор не подействовали"
}
