# S27. Внезапный сбой службы и остановка службы при показанном экране (ТЗ 11, NFR-7, эталон G10, Ruling 15).
# Питание компьютера в CI не выключить: сбой изображаем завершением процесса службы (taskkill /F), это та же
# потеря памяти без сохранения; выключение и включение питания остаются ручной проверкой на запасном ПК.
# Часть 1. Ребёнок активен 40 мин (часы ядра в 60 раз быстрее), служба убита, SCM поднимает её сам.
#   Тогда: учтено не меньше 39 мин 30 с (потеря не больше 30 с, допуск G10).
# Часть 2. Экран блокировки показан. Служба остановлена: экран снят не позже 10 с, процессов нет.
#   Служба запущена: экран вернулся, страж и значок по одному.
# Снимки: s27-lock-restored.png (экран вернулся после запуска службы).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S27" "Сбой службы после 40 минут и остановка службы при экране блокировки" {
  Initialize-Scenario 60

  Step "Часть 1: ребёнок активен 40 мин" {
    Test-InputSim
    Start-Child 120
    Resume-Clock
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $st = $null
    while ($true) {
      $st = Get-Status
      if ($st.usedS -ge 2400) { break }
      if ($sw.Elapsed.TotalSeconds -gt 120) { throw "За 120 с не набралось 40 мин: учтено $($st.usedS) с" }
      Start-Sleep -Milliseconds 100
    }
    Write-Log ("Учтено {0} с ({1:N1} с настоящего времени)" -f $st.usedS, $sw.Elapsed.TotalSeconds)
    $script:UsedBefore = [int]$st.usedS
  }

  Step "Служба убита (taskkill /F): SCM поднимает её, учёт не потерян" {
    Stop-Input
    $oldPid = Get-ServicePid
    Assert-That ($oldPid -gt 0) "Процесс службы не найден"
    $before = [int](Get-Status).usedS
    Close-PipeConn
    & taskkill.exe /F /PID $oldPid 2>&1 | ForEach-Object { Write-Log "taskkill: $_" }
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Wait-Until { (Get-ServiceState) -eq "Running" -and (Get-ServicePid) -gt 0 -and (Get-ServicePid) -ne $oldPid } 60 "служба поднята заново"
    Write-Log ("Служба поднята заново через {0:N1} с, новый процесс {1}" -f $sw.Elapsed.TotalSeconds, (Get-ServicePid))
    $script:ClockPausedByUs = $false
    Wait-Until { try { (Send-Pcb "Hello").ok } catch { Close-PipeConn; $false } } 30 "служба отвечает по каналу"
    $after = [int](Get-Status).usedS
    $lost = $before - $after
    Write-Log "До сбоя учтено $before с, после восстановления $after с, потеря $lost с"
    Assert-That ($after -ge 2370) "После восстановления учтено $after с (ждали не меньше 2370 = 39:30)"
    Assert-That ($lost -le 30) "Потеряно $lost с, ждали не больше 30 с"
  }

  Step "Часть 2: свежая установка, экран блокировки показан" {
    Initialize-Scenario 60
    Test-InputSim
    Start-Child 2
    Resume-Clock
    [void](Wait-LockShown 40 "timeUp")
    Suspend-Clock
    Wait-Until { (Get-BigPcbWindows).Count -ge 1 } 10 "экран блокировки виден как окно на весь экран"
    Write-Log "Экран блокировки: $(Get-BigPcbWindows)"
  }

  Step "Остановка службы: экран снят не позже 10 с, процессов нет" {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Close-PipeConn
    Stop-Service -Name $script:ServiceName -Force
    Wait-Until { (Get-BigPcbWindows).Count -eq 0 } 10 "экран блокировки снят после остановки службы"
    $secs = $sw.Elapsed.TotalSeconds
    Write-Log ("Экран снят через {0:N1} с после команды остановки" -f $secs)
    Assert-That ($secs -le 10) ("Экран снят через {0:N1} с, ждали не больше 10" -f $secs)
    Wait-Until { (Get-ServiceState) -eq "Stopped" } 35 "служба остановлена"
    Wait-Until { @(Get-PcbProcs).Count -eq 0 } 15 "процессов PCBlock.exe нет"
  }

  Step "Запуск службы: экран вернулся, страж и значок по одному" {
    Start-Service -Name $script:ServiceName
    Wait-Until { (Get-ServiceState) -eq "Running" } 35 "служба снова работает"
    Wait-Until { (Get-BigPcbWindows).Count -ge 1 } 40 "экран блокировки вернулся"
    Wait-SessionParts
    Start-Sleep -Seconds 5
    $guards = @(Get-SessionProcs "--guard").Count
    $trays = @(Get-SessionProcs "--tray").Count
    Write-Log "После запуска: стражей $guards, значков $trays, окон на весь экран $((Get-BigPcbWindows).Count)"
    Assert-That ($guards -eq 1) "Стражей в сеансе: $guards, ждали один"
    Assert-That ($trays -eq 1) "Значков в сеансе: $trays, ждали один"
    $g = Wait-LockShown 20 "timeUp"
    Assert-That (Test-HasButton $g "parentPassword") "На вернувшемся экране нет кнопки «Пароль родителя»"
    Start-Sleep -Seconds 1
    Save-Screen "s27-lock-restored"
  }

  Write-Log "S27: после сбоя службы учёт цел, остановка службы снимает экран, запуск возвращает его"
}
