# S4. Пять неверных паролей на экране блокировки (ТЗ 11, R19).
# Часы ядра идут в 60 раз быстрее. Пауза после пятой ошибки идёт по настоящим часам: 60 секунд.
# Дано: экран блокировки. Когда: 5 неверных паролей. Тогда: ввод заблокирован на 60 с с отсчётом,
# в журнале 5 записей «Неверный пароль», верный пароль во время паузы не принимается.
# Снимок: s04-blocked-input.png сразу после пятой ошибки (отсчёт «1:00»).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S4" "Пять неверных паролей: пауза 60 секунд" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Лимит 5 мин: показан экран блокировки, открыта панель пароля" {
    Start-Child 5
    Resume-Clock
    [void](Wait-LockShown 40 "timeUp")
    Suspend-Clock
    Invoke-GuardClick "parentPassword"
    [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "password" }).Count -ge 1 } 8 "панель пароля с полем")
  }

  $script:BlockedAt = $null
  Step "Пять неверных паролей подряд" {
    foreach ($n in 1..5) {
      $wrong = "Неверный-$n"
      Enter-GuardPassword $wrong
      Invoke-GuardClick "submit"
      # Страж очищает поле, когда служба ответила на проверку: это и есть конец попытки.
      [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "password" -and $_.text -eq "" }).Count -ge 1 } 10 "ответ службы на неверный пароль $n (поле очищено)")
      Write-Log "Неверный пароль $n отправлен, ответ получен за $([math]::Round($script:WaitElapsed, 1)) с"
      if ($n -eq 5) { $script:BlockedAt = Get-Date; Save-Screen "s04-blocked-input" }
    }
  }

  Step "Ввод заблокирован: верный пароль в паузе не принимается ни на экране, ни по каналу" {
    Enter-GuardPassword $script:ParentPassword
    Invoke-GuardClick "submit"
    Start-Sleep -Seconds 3
    $g = Get-GuardState
    Write-Log "После верного пароля во время паузы: $(Format-GuardState $g)"
    Assert-That $g.lockShown "Экран блокировки исчез во время паузы"
    Assert-That (-not (Test-HasButton $g "add15")) "Во время паузы верный пароль открыл выбор действий: $(Format-GuardState $g)"
    $r = Send-Pcb "Login" @{ password = $script:ParentPassword }
    Write-Log "Вход по каналу во время паузы: ok=$($r.ok), «$($r.error)», пауза $($r.payload.pauseS) с"
    Assert-That (-not $r.ok) "Служба приняла верный пароль во время паузы"
    Assert-Between ([double]$r.payload.pauseS) 1 60 "Осталось паузы по ответу службы, с"
    Assert-That ($r.error -match "\d:\d\d") "В отказе нет отсчёта м:сс: «$($r.error)»"
  }

  Step "Пауза кончилась: верный пароль принят, в журнале ровно 5 записей «Неверный пароль»" {
    $left = 62 - ((Get-Date) - $script:BlockedAt).TotalSeconds
    if ($left -gt 0) { Write-Log ("Ждём конец паузы: {0:N0} с" -f $left); Start-Sleep -Seconds ([int][math]::Ceiling($left)) }
    Open-Parent
    $wrong = Find-Events 'неверный пароль'
    Write-Log "Записи журнала: $((Get-Events | ForEach-Object { $_.text }) -join ' | ')"
    Assert-That ($wrong.Count -eq 5) "Записей «Неверный пароль»: $($wrong.Count), ждали 5"
  }

  Write-Log "S4: после 5 ошибок пауза 60 с, верный пароль в паузе не принят, 5 записей в журнале"
}
