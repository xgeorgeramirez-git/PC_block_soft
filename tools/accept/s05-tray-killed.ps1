# S5. Значок завершён принудительно: защита от обхода (ТЗ 11, R17в, FR-27).
# Дано: ребёнок в сеансе. Когда: процесс значка завершён принудительно (taskkill /F).
# Тогда: новый значок не позже 3 с; экран блокировки «Попытка обойти защиту» только с кнопками
# «Пароль родителя» и «Выйти из учётной записи»; запись в журнале родителя.
# Снимок: s05-bypass.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S5" "Значок убит: новый значок и блокировка за обход" {
  Initialize-Scenario 1

  Step "Настройка: ребёнок «Тест», 2 ч на каждый день; значок и страж запущены" {
    Invoke-Setup 120
    Wait-SessionParts
    Start-Sleep -Seconds 3
    $g = Get-GuardState
    Assert-That (-not $g.lockShown) "Экран блокировки показан до испытания: $(Format-GuardState $g)"
  }

  Step "taskkill значка: новый значок не позже 3 с" {
    $old = @(Get-SessionProcs "--tray")
    Assert-That ($old.Count -eq 1) "Значков в сеансе: $($old.Count), ждали один"
    $oldPid = [int]$old[0].ProcessId
    $before = @(Get-Process -Name PCBlock -ErrorAction SilentlyContinue | ForEach-Object { $_.Id })
    $sw = [Diagnostics.Stopwatch]::StartNew()
    & taskkill.exe /F /PID $oldPid 2>&1 | ForEach-Object { Write-Log "taskkill: $_" }
    $newPid = 0
    while ($newPid -eq 0) {
      foreach ($p in @(Get-Process -Name PCBlock -ErrorAction SilentlyContinue)) {
        if ($before -notcontains $p.Id -and $p.SessionId -eq $script:MySession) { $newPid = $p.Id; break }
      }
      if ($newPid -eq 0) {
        if ($sw.Elapsed.TotalSeconds -gt 15) { throw "Новый значок не появился за 15 с" }
        Start-Sleep -Milliseconds 50
      }
    }
    $secs = $sw.Elapsed.TotalSeconds
    $cmd = (Get-CimInstance Win32_Process -Filter "ProcessId=$newPid").CommandLine
    Write-Log ("Новый процесс {0} через {1:N2} с: {2}" -f $newPid, $secs, $cmd)
    Assert-That ($cmd -match "--tray") "Новый процесс не значок: $cmd"
    Assert-That ($secs -le 3) ("Новый значок появился через {0:N2} с, ждали не больше 3" -f $secs)
    Wait-Until { @(Get-SessionProcs "--tray").Count -eq 1 } 10 "в сеансе ровно один значок"
  }

  Step "Экран блокировки «Попытка обойти защиту»: пароль родителя и выходы, без доступа ко времени" {
    $g = Wait-LockShown 15 "bypass"
    Write-Log "Экран показан через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-GuardState $g)"
    # При обходе время не возвращают: «Попросить ещё» и «Взять из копилки» не показываются
    # (ТЗ 8.2.3). Доступны «Пароль родителя» и выходы (выходы добавлены в 0.1.1, D-40).
    $buttons = @(List $g.buttons | Sort-Object) -join ","
    Assert-That ($buttons -eq "logoff,parentPassword,shutdown,switchUser") "На экране кнопки [$buttons], ждали parentPassword и выходы"
    Assert-That (-not (Test-HasButton $g "askMore")) "При обходе показана «Попросить ещё»: $(Format-GuardState $g)"
    Assert-That (-not (Test-HasButton $g "spendBonus")) "При обходе показана «Взять из копилки»: $(Format-GuardState $g)"
    $st = Get-Status
    Write-Log "Статус: $($st.status), причина «$($st.reason)», подробности «$($st.detail)»"
    Assert-That ($st.lock -eq "bypass") "Блокировка в статусе службы '$($st.lock)', ждали bypass"
    Assert-That ($st.reason -eq "Попытка обойти защиту") "Причина «$($st.reason)», ждали «Попытка обойти защиту»"
    Start-Sleep -Seconds 1
    Save-Screen "s05-bypass"
  }

  Step "Запись в журнале родителя" {
    Open-Parent
    $found = Find-Events 'попытка обойти защиту'
    Write-Log "Записи журнала: $((Get-Events | ForEach-Object { $_.text }) -join ' | ')"
    Assert-That ($found.Count -ge 1) "В журнале нет записи о попытке обойти защиту"
    $card = @(List (Get-Today).children)[0]
    Assert-That ($card.canClearBlock) "У ребёнка нет кнопки «Снять блокировку»"
  }

  Write-Log "S5: новый значок за срок, экран «Попытка обойти защиту» с двумя кнопками, запись в журнале"
}
