# S54. Системные инструменты у ребёнка (ТЗ 11, R17г, FR-29, D-86).
# Дано: ребёнок в сеансе; в папке тестовых программ копия программы с именем cmd.exe (в тестовом режиме это
# «Командная строка»). Когда: (б) cmd.exe запускает PowerShell сценария не на переднем плане, ребёнок 30 с
# ничего не нажимал; (а) ребёнок двигает мышь, на переднем плане PowerShell, он запускает cmd.exe.
# Тогда: в (б) cmd.exe работает, записи нет; в (а) cmd.exe закрыта не позже 1 с, плашка
# «Программа «Командная строка» недоступна», в журнале «попытка обойти защиту» с названием инструмента.
# Запуск из Проводника сценарий не воспроизводит: его проверяет тест службы (TestSysToolManualFromExplorerClosed).
# Снимок: s54-system-tool.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$script:PsExe = (Get-Process -Id $PID).Path
$script:PlateText = "Программа «Командная строка» недоступна"
$script:Pattern = 'попытка обойти защиту\. Открыта программа «Командная строка»'

function Set-ForegroundPath([string]$Path) {
  Invoke-TestGuard "foreground" "" $Path
  $want = $Path.ToLowerInvariant()
  [void](Wait-GuardState { param($s) ([string]$s.foregroundPath).ToLowerInvariant() -eq $want } 5 "подмена переднего плана в страже: $want")
  Write-Log "Передний план (подмена): $Path"
}

Invoke-Scenario "S54" "Командная строка: открыл ребёнок — закрыта с плашкой и записью; без ребёнка — не тронута" {
  Initialize-Scenario 60
  try {
    Step "Имитация ввода доходит до сеанса" {
      Test-InputSim
    }

    Step "Копия программы cmd.exe в папке наблюдения" {
      $script:CmdPath = New-TestApp "cmd"
    }

    Step "Настройка: лимит 2 ч" {
      Start-Child 120 $false
      Wait-ProgramsReady
    }

    Step "(б) Ребёнок 30 с ничего не нажимает, на переднем плане Проводник: cmd.exe не трогается" {
      Set-ForegroundPath (Join-Path $env:SystemRoot "explorer.exe")
      Wait-Until { [PcbNative]::IdleMs() -ge 30000 } 45 "простой ввода 30 с"
      $p = Start-TestApp $script:CmdPath
      Write-Log "cmd.exe (PID $($p.Id)), родитель: PowerShell сценария (PID $PID, $($script:PsExe)); простой $([PcbNative]::IdleMs()) мс"
      Start-Sleep -Seconds 4
      Assert-AppAlive $p "cmd.exe без ввода ребёнка"
      Open-Parent
      Assert-That ((Find-Events $script:Pattern).Count -eq 0) "Запуск без ребёнка записан обходом"
      Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    }

    Step "(а) Ребёнок двигает мышь, на переднем плане PowerShell, он запускает cmd.exe: закрыта, плашка" {
      Start-Input 1000
      Set-ForegroundPath $script:PsExe
      Wait-Until { $i = [PcbNative]::IdleMs(); $i -ge 0 -and $i -lt 2000 } 10 "ввод ребёнка (простой $([PcbNative]::IdleMs()) мс)"
      Start-Sleep -Seconds 2 # страж успевает прислать снимок с вводом и передним планом
      $p = Start-TestApp $script:CmdPath
      [void](Assert-ClosedFast $p "Ручной запуск cmd.exe")
      $g = Wait-GuardState { param($s) [string]$s.plate -eq $script:PlateText } 6 "плашка «$($script:PlateText)»"
      Assert-That ($g.plateKind -eq "forbidden") "Вид плашки «$($g.plateKind)», ждали forbidden"
      Assert-That (-not $g.lockShown) "Показан экран блокировки (D-41)"
      Save-Screen "s54-system-tool"
    }

    Step "(а) Журнал: одна запись «попытка обойти защиту» с названием инструмента" {
      Open-Parent
      Wait-Until { (Find-Events $script:Pattern).Count -ge 1 } 6 "запись обхода о командной строке"
      $found = @(Find-Events $script:Pattern)
      Write-Log "Запись: $($found[0].text)"
      Assert-That ($found.Count -eq 1) "Записей обхода о командной строке: $($found.Count), ждали 1"
    }
  } finally {
    try { Invoke-TestGuard "foreground" } catch {}
    Stop-Input
  }
  Write-Log "S54: командная строка, открытая ребёнком, закрыта с плашкой и записью; запущенная без ребёнка не тронута"
}
