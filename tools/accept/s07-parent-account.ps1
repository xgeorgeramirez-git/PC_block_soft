# S7. Аварийный выход: учётка родителя никогда не ограничена (ТЗ 11, NFR-10).
# Дано: ребёнку блокировка (созданный обычный пользователь pcbtest, на весь день «Нельзя»).
# Когда: вход в другую учётку, здесь это учётка раннера (в её сеансе работает проверка).
# Тогда: страж в сеансе не запущен, значок без таймера (статус notChild), вход родителя по паролю проходит,
# окно родителя (--parent) открывается. Второй интерактивный сеанс одновременно в CI не воспроизвести:
# вход в настоящую учётку родителя рядом с сеансом ребёнка проверяется вручную на запасном ПК.
# Снимок: s07-parent-window.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S7" "Учётка родителя без ограничений, окно родителя открывается" {
  Initialize-Scenario 1

  $childSid = $null
  Step "Ребёнок pcbtest с запретом на весь день" {
    $childSid = New-TestUser
    $limits = @(1..7 | ForEach-Object { @{ weekday = $_; totalMin = 0; unlimited = $false } })
    Invoke-Setup 0 $childSid "Миша" $limits
  }

  Step "В сеансе раннера значок есть, стража нет, экрана блокировки нет" {
    Wait-Until { @(Get-SessionProcs "--tray").Count -ge 1 } 40 "значок в сеансе раннера"
    Start-Sleep -Seconds 8
    $guards = @(Get-SessionProcs "--guard")
    Assert-That ($guards.Count -eq 0) "В сеансе раннера стражей: $($guards.Count), ждали ноль"
    Assert-That ((Get-BigPcbWindows).Count -eq 0) "В сеансе раннера показано окно размером с экран"
    $st = Get-Status
    Write-Log "Статус сеанса раннера: $($st.status), надпись «$($st.label)», блокировка '$($st.lock)'"
    Assert-That ($st.status -eq "notChild") "Статус '$($st.status)', ждали notChild (значок без таймера)"
    Assert-That (-not $st.label) "У значка есть надпись «$($st.label)», ждали пусто"
  }

  Step "Запрет Диспетчера задач учётке раннера не ставится" {
    $key = "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Policies\System"
    $v = Get-ItemProperty -Path $key -Name DisableTaskMgr -ErrorAction SilentlyContinue
    Assert-That (-not ($v -and $v.DisableTaskMgr -eq 1)) "У учётки раннера стоит запрет Диспетчера задач"
  }

  Step "Вход родителя по паролю проходит, ребёнок виден заблокированным" {
    Open-Parent
    $card = @(List (Get-Today).children)[0]
    Write-Log "Карточка: $($card.name), статус $($card.status.status), «$($card.status.statusText)»"
    Assert-That ($card.name -eq "Миша") "В списке ребёнок '$($card.name)', ждали Миша"
    Assert-That ($card.status.status -eq "timeUp") "Статус ребёнка '$($card.status.status)', ждали timeUp (блокировка)"
  }

  Step "Окно родителя (--parent) открывается" {
    $title = Save-ProgramWindow "s07-parent-window" @("--parent") 40 5
    Write-Log "Окно родителя открыто, заголовок «$title»"
  }

  Write-Log "S7: в сеансе взрослого стража нет, значок без таймера, вход родителя и окно работают"
}
