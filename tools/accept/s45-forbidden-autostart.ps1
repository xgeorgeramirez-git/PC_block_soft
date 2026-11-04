# S45. Запрещённая программа, запустившаяся сама, закрывается молча (R13, D-65, FB-18, NFR-6, PL-1, PL-16, PL-22).
# Часы ядра на паузе (Start-Child): день учёта не меняется, наблюдатель и плашки идут по настоящим часам.
# tool.exe — копия системной программы в папке WatchDir (New-TestApp), «Запрещено» шагом мастера до запуска
# (PL-11). Родитель каждого запуска — этот процесс PowerShell (Start-TestApp). Передний план задаёт подмена в
# страже (TestGuard foreground). Ввод — движение мыши (Start-Input), простой — GetLastInputInfo.
# Дано: tool.exe «Запрещено».
# (а) Ребёнок 30 с ничего не нажимал, на переднем плане Проводник, tool.exe запускает PowerShell (не на переднем
#     плане) — дважды, второй раз через 61 с (дольше ограничителя повторов журнала 60 с, PL-12). Тогда: tool.exe
#     закрыта не позже допуска раннера, плашки нет, «попытка открыть запрещённое» нет, в журнале одна запись
#     «программа «X» запускалась сама и закрыта».
# (б) Ребёнок двигает мышь, на переднем плане PowerShell, он запускает tool.exe (как «Играть» в Steam). Тогда:
#     закрыта, плашка «Программа «X» запрещена родителем», нарушение «попытка открыть запрещённое: «X»».
# Запуск из самого Проводника (рабочий стол, «Пуск») сценарий не воспроизводит: из скрипта программу запускает
# не Проводник сеанса. Эту ветку правила (Проводник не в первые 60 с после входа) проверяет тест службы
# TestWatchForbiddenManualStillWarns (Ruling RF18-1).
# Имя программы — из ответа программы (ListPrograms, PL-22). Снимок: s45-manual-plate.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы (internal/service/texts.go textPlateForbidden, journaltext.go).
$script:AutoPattern = "запускалась сама и закрыта"
$script:TryPattern = "попытка открыть запрещённое"

# Строки подробного журнала службы о закрытых программах (родитель, почему запуск ручной или сам, уточнение
# по следующему снимку стража): для текста ошибки. Запись «запускалась сама» служба делает через 2 с после
# закрытия (launchSettle): проверки журнала идут после 3 с наблюдения за плашкой.
function Get-WatchLog {
  $log = Join-Path (Join-Path $script:Data "logs") "service.log"
  if (-not (Test-Path -LiteralPath $log)) { return "service.log нет" }
  $lines = @(Get-Content -LiteralPath $log -Encoding UTF8 -ErrorAction SilentlyContinue | Where-Object { $_ -match 'наблюдатель (закрыл|: запуск|: .*«сама»)' }) | Select-Object -Last 8
  if ($lines.Count -eq 0) { return "в service.log нет строк «наблюдатель закрыл»" }
  return ($lines -join ' | ')
}

# Записи журнала родителя по образцу, только про tool.exe (имя из ответа программы).
function Find-ToolEvents([string]$Pattern) {
  return @(Use-Parent { @(Find-Events $Pattern | Where-Object { ([string]$_.text).Contains("«$($script:ToolName)»") }) })
}

# Подмена переднего плана в страже путём Path; ждёт, пока страж пришлёт его службе.
function Set-ForegroundPath([string]$Path) {
  Invoke-TestGuard "foreground" "" $Path
  $script:FgOverride = $true
  $want = $Path.ToLowerInvariant()
  [void](Wait-GuardState { param($s) ([string]$s.foregroundPath).ToLowerInvariant() -eq $want } 5 "подмена переднего плана в страже: $want")
  Write-Log "Передний план (подмена): $Path"
}

# Плашки «запрещена» нет Seconds секунд: она пришла бы со следующим опросом стража (раз в секунду) и
# держится 10 с, поэтому 3 с наблюдения хватает (PL-16: отсутствие проверяется после события).
function Assert-NoForbiddenPlate([string]$What, [double]$Seconds = 3) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
    $g = Get-GuardState
    if ($g.plateKind -eq "forbidden" -or ([string]$g.plate).Contains("запрещена родителем")) {
      throw "${What}: ребёнку показана плашка о запрещённой программе: $(Format-GuardState $g); журнал службы: $(Get-WatchLog)"
    }
    Start-Sleep -Milliseconds 150
  }
}

# Запуск tool.exe этим процессом PowerShell; ждёт закрытия наблюдателем и возвращает процесс.
function Invoke-ToolLaunch([string]$What) {
  $p = Start-TestApp $script:ToolPath
  Write-Log "${What}: запущена tool.exe (PID $($p.Id)), родитель: PowerShell (PID $PID, $($script:PsExe)); простой ввода $([PcbNative]::IdleMs()) мс"
  [void](Assert-ClosedFast $p $What)
  return $p
}

Invoke-Scenario "S45" "Запрещённая tool.exe: запустилась сама, закрыта молча и раз в день в журнал; вручную плашка и нарушение" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовая программа tool.exe в папке наблюдения" {
    $script:ToolPath = New-TestApp "tool"
  }

  Step "Настройка: лимит 2 ч; tool.exe «Запрещено»; подробный журнал службы" {
    Start-Child 120 $false -1 @(@{ path = $script:ToolPath; category = "blocked" })
    Wait-ProgramsReady
    $script:ToolName = Get-ProgramName $script:ToolPath
    Use-Parent { Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы" }
    Write-Log "Название tool.exe из списка программ: «$($script:ToolName)»"
    $script:PlateText = "Программа «$($script:ToolName)» запрещена родителем"
  }

  Step "(а) Ребёнок 30 с ничего не нажимает, на переднем плане Проводник" {
    Stop-Input
    Set-ForegroundPath (Join-Path $env:SystemRoot "explorer.exe")
    Wait-Until { [PcbNative]::IdleMs() -ge 30000 } 60 "простой ввода 30 с (сейчас $([PcbNative]::IdleMs()) мс)"
    Write-Log "Простой ввода: $([PcbNative]::IdleMs()) мс"
  }

  Step "(а) Первый автозапуск: закрыта, плашки нет" {
    [void](Invoke-ToolLaunch "Автозапуск 1")
    Assert-NoForbiddenPlate "Автозапуск 1"
  }

  Step "(а) Второй автозапуск через 61 с: закрыта, плашки нет" {
    Start-Sleep -Seconds 61
    [void](Invoke-ToolLaunch "Автозапуск 2")
    Assert-NoForbiddenPlate "Автозапуск 2"
  }

  Step "(а) Журнал: одна запись «запускалась сама и закрыта», нарушений нет" {
    $auto = @(Find-ToolEvents $script:AutoPattern)
    $try = @(Find-ToolEvents $script:TryPattern)
    $all = Use-Parent { Format-Events }
    Write-Log "Журнал после автозапусков: $all"
    Assert-That ($try.Count -eq 0) "Автозапуск записан нарушением «$($script:TryPattern)»: $(($try | ForEach-Object { $_.text }) -join ' | '); журнал службы: $(Get-WatchLog)"
    Assert-That ($auto.Count -eq 1) "Записей «$($script:AutoPattern)» про «$($script:ToolName)»: $($auto.Count), ждали 1 на два запуска в один день; журнал: $all; журнал службы: $(Get-WatchLog)"
    $want = "программа «$($script:ToolName)» $($script:AutoPattern)"
    Assert-That (([string]$auto[0].text).Contains($want)) "Текст записи «$($auto[0].text)», ждали «…: $want»"
    Assert-That ([string]$auto[0].kind -eq "program_auto_closed") "Вид записи «$($auto[0].kind)», ждали program_auto_closed (не нарушение)"
  }

  Step "(б) Ребёнок двигает мышь, на переднем плане PowerShell, он запускает tool.exe: плашка" {
    Start-Input 1000
    Set-ForegroundPath $script:PsExe
    Wait-Until { $i = [PcbNative]::IdleMs(); $i -ge 0 -and $i -lt 2000 } 10 "ввод ребёнка (простой $([PcbNative]::IdleMs()) мс)"
    Start-Sleep -Seconds 2 # страж успевает прислать снимок с вводом и передним планом
    [void](Invoke-ToolLaunch "Ручной запуск")
    $g = Wait-GuardState { param($s) [string]$s.plate -eq $script:PlateText } 6 "плашка «$($script:PlateText)» (журнал службы: $(Get-WatchLog))"
    Assert-That ($g.plateKind -eq "forbidden") "Вид плашки «$($g.plateKind)», ждали forbidden: $(Format-GuardState $g)"
    Save-Screen "s45-manual-plate"
  }

  Step "(б) Журнал: нарушение «попытка открыть запрещённое», записей «запускалась сама» по-прежнему одна" {
    $try = @(Find-ToolEvents $script:TryPattern)
    $auto = @(Find-ToolEvents $script:AutoPattern)
    $all = Use-Parent { Format-Events }
    Assert-That ($try.Count -eq 1 -and [string]$try[0].kind -eq "program_forbidden_try") "Нарушений «$($script:TryPattern)» про «$($script:ToolName)»: $($try.Count), ждали 1 (program_forbidden_try); журнал: $all; журнал службы: $(Get-WatchLog)"
    Assert-That ($auto.Count -eq 1) "Записей «$($script:AutoPattern)» после ручного запуска: $($auto.Count), ждали 1; журнал: $all"
  }

  Step "Подмена переднего плана снята" {
    Invoke-TestGuard "foreground"
    $script:FgOverride = $false
  }

  Write-Log "S45: автозапуск tool.exe закрыт молча, одна запись в день; ручной запуск: плашка и нарушение"
}
