# S5. Значок завершён принудительно: только запись в журнал, без блокировки (ТЗ 11, R17в, FR-27, D-41).
# Дано: ребёнок в сеансе. Когда: процесс значка завершён принудительно (taskkill /F).
# Тогда: новый значок не позже 3 с; экран НЕ блокируется; в журнале родителя подробная запись (через 5 с, FB-1).
# Снимок: s05-bypass.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S5" "Значок убит: новый значок, запись в журнал без блокировки (D-41)" {
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

  Step "Экран НЕ блокируется (D-41): обход только пишется в журнал" {
    # Даём службе время заметить гибель значка; экран блокировки не должен появиться.
    Start-Sleep -Seconds 5
    $g = Get-GuardState
    Write-Log "Состояние стража: $(Format-GuardState $g)"
    Assert-That (-not $g.lockShown) "После закрытия значка показан экран блокировки, а не должен (D-41): $(Format-GuardState $g)"
    $st = Get-Status
    Write-Log "Статус: $($st.status), блокировка «$($st.lock)»"
    Assert-That ($st.lock -ne "bypass") "Статус службы остался в блокировке за обход: $($st.lock) (D-41)"
    Save-Screen "s05-bypass"
  }

  Step "Запись в журнале родителя (служба пишет её через 5 с после гибели значка, FB-1)" {
    Open-Parent
    # Запись откладывается на 5 с: если за это время придёт конец сеанса, это не обход. Ждём её до 10 с.
    Wait-Until { (Find-Events 'попытка обойти защиту').Count -ge 1 } 10 "запись о попытке обойти защиту"
    $found = Find-Events 'попытка обойти защиту'
    Write-Log "Записи журнала: $((Get-Events | ForEach-Object { $_.text }) -join ' | ')"
    Assert-That ($found.Count -ge 1) "В журнале нет записи о попытке обойти защиту"
    $card = @(List (Get-Today).children)[0]
    Assert-That (-not $card.canClearBlock) "У ребёнка есть кнопка «Снять блокировку», а блокировки быть не должно (D-41)"
  }

  Write-Log "S5: новый значок за срок, экран не блокируется, подробная запись в журнале (D-41)"
}
