# S53. Перевод часов: во время работы при ребёнке на экране и назад при включении (ТЗ 11, R17а, FR-28, D-87).
# Дано: ребёнок в сеансе. Когда: часы переведены на 10 мин вперёд, затем обратно; затем поправка на 30 с;
# затем служба остановлена, часы переведены на 1 ч назад, служба запущена.
# Тогда: экран не блокируется; две записи «Часы компьютера переведены» у ребёнка; поправка 30 с без записи;
# после запуска службы запись «Часы компьютера ушли назад». Время возвращается при остановленной службе.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Сдвиг часов, который сценарий ещё не вернул (секунды): возвращается в finally.
$script:Shift = 0

function Move-Clock([int]$Seconds) {
  Set-Date -Adjust ([TimeSpan]::FromSeconds($Seconds)) | Out-Null
  $script:Shift += $Seconds
  Write-Log "Часы сдвинуты на $Seconds с, сейчас $((Get-Date).ToString('dd.MM HH:mm:ss')); всего сдвинуто $($script:Shift) с"
}

# Останавливает службу, выполняет Body и запускает её снова (ждёт ответа в тестовом режиме).
function Invoke-ServiceStopped([scriptblock]$Body) {
  Close-PipeConn
  $script:Token = ""
  Stop-Service -Name $script:ServiceName -Force
  Wait-Until { (Get-ServiceState) -eq "Stopped" } 40 "служба остановлена"
  & $Body
  Start-Service -Name $script:ServiceName
  Wait-Until { (Get-ServiceState) -eq "Running" } 40 "служба запущена"
  Wait-Until { try { $h = Send-Pcb "Hello"; $h.ok -and $h.payload.testMode } catch { Close-PipeConn; $false } } 40 "служба отвечает в тестовом режиме"
  Wait-SessionParts
}

function Get-ClockEvents([string]$Pattern) { return @(Find-Events $Pattern) }

Invoke-Scenario "S53" "Перевод часов: записи в журнале без блокировки, поправка 30 с без записи, часы назад при включении" {
  Initialize-Scenario 1
  $w32 = Get-Service -Name w32time -ErrorAction SilentlyContinue
  $script:W32Was = if ($w32) { [string]$w32.Status } else { "" }
  try {
    if ($script:W32Was -eq "Running") {
      Stop-Service -Name w32time -Force # синхронизация не поправит сдвиг посреди сценария
      Write-Log "Служба времени Windows остановлена на время сценария"
    }
    Step "Настройка: ребёнок «Тест», 2 ч на каждый день; значок и страж запущены" {
      Invoke-Setup 120
      Wait-SessionParts
      Start-Sleep -Seconds 3
      Assert-That (-not (Get-GuardState).lockShown) "Экран блокировки показан до испытания"
    }

    Step "Часы на 10 мин вперёд и обратно: две записи у ребёнка" {
      Move-Clock 600
      Start-Sleep -Seconds 3
      Move-Clock -600
      Start-Sleep -Seconds 3
      Open-Parent
      $found = Get-ClockEvents 'попытка обойти защиту\. Часы компьютера переведены'
      Write-Log "Записи: $(($found | ForEach-Object { $_.text }) -join ' | ')"
      Assert-That ($found.Count -eq 2) "Записей о переводе часов: $($found.Count), ждали 2"
      Assert-That (-not (Get-GuardState).lockShown) "После перевода часов показан экран блокировки (D-41)"
      Save-Screen "s53-clock"
    }

    Step "Поправка на 30 с вперёд и назад: записей не прибавилось" {
      Move-Clock 30
      Start-Sleep -Seconds 3
      Move-Clock -30
      Start-Sleep -Seconds 3
      Open-Parent
      $found = Get-ClockEvents 'Часы компьютера переведены'
      Assert-That ($found.Count -eq 2) "После поправки 30 с записей о переводе часов: $($found.Count), ждали 2"
    }

    Step "Служба остановлена, часы на 1 ч назад, служба запущена: запись «Часы компьютера ушли назад»" {
      Invoke-ServiceStopped { Move-Clock -3600 }
      Open-Parent
      $found = @()
      Wait-Until { $script:found = Get-ClockEvents 'Часы компьютера ушли назад'; $script:found.Count -ge 1 } 15 "запись «Часы компьютера ушли назад»"
      Write-Log "Запись: $($script:found[0].text)"
      Assert-That ($script:found.Count -eq 1) "Записей «ушли назад»: $($script:found.Count), ждали 1"
    }

    Step "Время возвращено при остановленной службе: новых записей нет" {
      Invoke-ServiceStopped { Move-Clock (-$script:Shift) }
      Start-Sleep -Seconds 3
      Open-Parent
      $all = Get-ClockEvents 'Часы компьютера'
      Assert-That ($all.Count -eq 3) "После возврата времени записей о часах: $($all.Count), ждали 3: $(($all | ForEach-Object { $_.text }) -join ' | ')"
    }
  } finally {
    if ($script:Shift -ne 0) {
      try { Set-Date -Adjust ([TimeSpan]::FromSeconds(-$script:Shift)) | Out-Null; Write-Log "Часы возвращены на $(-$script:Shift) с" } catch { Write-Log "Часы не возвращены: $_" }
    }
    if ($script:W32Was -eq "Running") {
      try { Start-Service -Name w32time; & w32tm.exe /resync /nowait 2>&1 | Out-Null; Write-Log "Служба времени Windows запущена" } catch { Write-Log "Служба времени не запущена: $_" }
    }
  }
  Write-Log "S53: перевод часов записан в журнал без блокировки, поправка без записи, часы назад при включении записаны"
}
