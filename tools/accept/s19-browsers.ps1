# S19. Другие браузеры в детском сеансе (R13, R15, RM4-8, D-65, D-68, NFR-6, 8.2.5 п. 4, T10-13, PL-12, PL-16, PL-22).
# Часы ядра на паузе (Start-Child): наблюдатель и плашки идут по настоящим часам. Браузеры — копии системной
# программы с именами msedge.exe, chrome.exe, firefox.exe в папке WatchDir (New-TestApp, RM3-8): каталог программы
# запрещает браузеры, кроме Brave, по имени файла в любом каталоге. firefox.exe родитель отнёс к «Прочему» шагом
# мастера: решение родителя сильнее каталога (RM4-8). Передний план задаёт подмена в страже (TestGuard foreground),
# как в S45; ввод — движение мыши (Start-Input), простой — GetLastInputInfo.
# (а) Автозапуск: ребёнок 30 с ничего не нажимал, впереди Проводник, msedge.exe запускает PowerShell. Тогда: закрыт
#     не позже допуска раннера, плашки нет, в журнале «запускалась сама и закрыта», нарушения нет (R13, D-65).
# (б) Ручной запуск: ребёнок двигает мышь, впереди PowerShell, он запускает msedge.exe, затем chrome.exe. Тогда:
#     закрыты, плашка «Этот браузер запрещён. Пользуйся Brave», по одному нарушению «попытка открыть запрещённое».
# (в) firefox.exe (решение родителя «Прочее») работает, плашки браузера нет.
# Настоящий Edge из «Пуска» и его служебные процессы — вручную на запасном ПК. Снимок: s19-browser-plate.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы: internal/service/texts.go textPlateBrowser; internal/service/journaltext.go evProgramForbiddenTry,
# evProgramAutoClosed; виды записей — internal/service/handlers_m3.go; вид плашки — internal/ipc/commands_m4.go BlockedBrowser.
$script:BrowserPlate = "Этот браузер запрещён. Пользуйся Brave"
$script:TryKind = "program_forbidden_try"
$script:AutoKind = "program_auto_closed"

# Подмена переднего плана в страже путём Path; ждёт, пока страж пришлёт его службе (как S45).
function Set-ForegroundPath([string]$Path) {
  Invoke-TestGuard "foreground" "" $Path
  $script:FgOverride = $true
  $want = $Path.ToLowerInvariant()
  [void](Wait-GuardState { param($s) ([string]$s.foregroundPath).ToLowerInvariant() -eq $want } 5 "подмена переднего плана в страже: $want")
  Write-Log "Передний план (подмена): $Path"
}

# Строки подробного журнала службы о закрытых программах: для текста ошибки (журналы Actions не скачиваются).
function Get-WatchLog {
  $log = Join-Path (Join-Path $script:Data "logs") "service.log"
  if (-not (Test-Path -LiteralPath $log)) { return "service.log нет" }
  $lines = @(Get-Content -LiteralPath $log -Encoding UTF8 -ErrorAction SilentlyContinue | Where-Object { $_ -match 'наблюдатель (закрыл|: запуск|: .*«сама»)' }) | Select-Object -Last 8
  if ($lines.Count -eq 0) { return "в service.log нет строк «наблюдатель закрыл»" }
  return ($lines -join ' | ')
}

# Записи журнала родителя вида Kind (сама открывает родительскую сессию: не звать внутри Use-Parent).
function Get-KindEvents([string]$Kind) {
  return @(Use-Parent { @(Get-Events | Where-Object { [string]$_.kind -eq $Kind }) })
}

# Плашки браузера нет Seconds секунд (PL-16: отсутствие проверяется после события; плашка пришла бы со следующим
# опросом стража, раз в секунду).
function Assert-NoBrowserPlate([string]$What, [double]$Seconds = 3) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
    $g = Get-GuardState
    if ([string]$g.plateKind -eq "browser" -or ([string]$g.plate).Contains("Этот браузер запрещён")) {
      throw "${What}: ребёнку показана плашка о браузере: $(Format-GuardState $g); журнал службы: $(Get-WatchLog)"
    }
    Start-Sleep -Milliseconds 150
  }
}

# Запуск браузера этим процессом PowerShell; ждёт закрытия наблюдателем (NFR-6) и возвращает процесс.
function Invoke-BrowserLaunch([string]$Path, [string]$What) {
  $p = Start-TestApp $Path
  Write-Log "${What}: запущен $(Split-Path -Leaf $Path) (PID $($p.Id)), родитель PowerShell (PID $PID); простой ввода $([PcbNative]::IdleMs()) мс"
  [void](Assert-ClosedFast $p $What)
  return $p
}

Invoke-Scenario "S19" "Другие браузеры: Edge и Chrome закрыты с плашкой «Этот браузер запрещён», автозапуск молча, Firefox по решению родителя работает" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Браузеры-копии msedge.exe, chrome.exe, firefox.exe в папке наблюдения" {
    $script:EdgePath = New-TestApp "msedge"
    $script:ChromePath = New-TestApp "chrome"
    $script:FirefoxPath = New-TestApp "firefox"
  }

  Step "Настройка: лимит 2 ч; firefox.exe «Прочее» шагом мастера; подробный журнал службы" {
    Start-Child 120 $false -1 @(@{ path = $script:FirefoxPath; category = "other" })
    Wait-ProgramsReady
    Use-Parent { Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы" }
  }

  Step "Дано: firefox.exe в «Программах» — «Прочее» решением родителя" {
    $norm = $script:FirefoxPath.ToLowerInvariant()
    $list = Get-ProgramsList
    $row = @(@(List $list.programs) | Where-Object { ([string]$_.path) -eq $norm })
    Assert-That ($row.Count -eq 1) "Строк $norm в «Программах»: $($row.Count), ждали 1 (шаг мастера «Программы»)"
    Write-Log "firefox.exe в «Программах»: категория $($row[0].category), источник $($row[0].source), название «$($row[0].name)»"
    Assert-That ([string]$row[0].category -eq "other") "Категория firefox.exe «$($row[0].category)», ждали other (решение родителя сильнее каталога, RM4-8)"
    Assert-That ([string]$row[0].source -eq "parent") "Источник категории firefox.exe «$($row[0].source)», ждали parent (шаг мастера)"
  }

  Step "(а) Ребёнок 30 с ничего не нажимает, на переднем плане Проводник" {
    Stop-Input
    Set-ForegroundPath (Join-Path $env:SystemRoot "explorer.exe")
    Wait-Until { [PcbNative]::IdleMs() -ge 30000 } 60 "простой ввода 30 с (сейчас $([PcbNative]::IdleMs()) мс)"
    Write-Log "Простой ввода: $([PcbNative]::IdleMs()) мс"
  }

  Step "(а) msedge.exe запустился сам: закрыт, плашки нет" {
    $before = @(Get-KindEvents $script:AutoKind).Count
    Assert-That ($before -eq 0) "До автозапуска в журнале уже $before записей $($script:AutoKind) (PL-16)"
    [void](Invoke-BrowserLaunch $script:EdgePath "Автозапуск msedge")
    Assert-NoBrowserPlate "Автозапуск msedge"
  }

  Step "(а) Журнал: одна запись «запускалась сама и закрыта», нарушений нет" {
    # Запись «запускалась сама» служба делает через 2 с после закрытия (launchSettle): 3 с плашки уже прошли.
    $script:EdgeName = Get-ProgramName $script:EdgePath
    $auto = @(Get-KindEvents $script:AutoKind)
    $try = @(Get-KindEvents $script:TryKind)
    $all = Use-Parent { Format-Events }
    Write-Log "Название msedge.exe из ответа программы: «$($script:EdgeName)»; журнал: $all"
    Assert-That ($try.Count -eq 0) "Автозапуск записан нарушением $($script:TryKind): $(($try | ForEach-Object { $_.text }) -join ' | '); журнал службы: $(Get-WatchLog)"
    Assert-That ($auto.Count -eq 1) "Записей $($script:AutoKind): $($auto.Count), ждали 1; журнал: $all; журнал службы: $(Get-WatchLog)"
    $want = "программа «$($script:EdgeName)» запускалась сама и закрыта"
    Assert-That (([string]$auto[0].text).Contains($want)) "Текст записи «$($auto[0].text)», ждали «…$want»"
  }

  Step "(б) Ребёнок двигает мышь, на переднем плане PowerShell, он запускает msedge.exe: плашка браузера" {
    Start-Input 1000
    Set-ForegroundPath $script:PsExe
    Wait-Until { $i = [PcbNative]::IdleMs(); $i -ge 0 -and $i -lt 2000 } 10 "ввод ребёнка (простой $([PcbNative]::IdleMs()) мс)"
    Start-Sleep -Seconds 2 # страж успевает прислать снимок с вводом и передним планом
    $g0 = Get-GuardState
    Assert-That ([string]$g0.plate -ne $script:BrowserPlate) "До ручного запуска уже видна плашка «$($g0.plate)» (PL-16)"
    [void](Invoke-BrowserLaunch $script:EdgePath "Ручной запуск msedge")
    $g = Wait-GuardState { param($s) [string]$s.plate -eq $script:BrowserPlate } 6 "плашка «$($script:BrowserPlate)» (журнал службы: $(Get-WatchLog))"
    Assert-That ([string]$g.plateKind -eq "browser") "Вид плашки «$($g.plateKind)», ждали browser: $(Format-GuardState $g)"
    Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
    Save-Screen "s19-browser-plate"
  }

  Step "(б) chrome.exe вручную: закрыт" {
    [void](Invoke-BrowserLaunch $script:ChromePath "Ручной запуск chrome")
    $script:ChromeName = Get-ProgramName $script:ChromePath
    Write-Log "Название chrome.exe из ответа программы: «$($script:ChromeName)»"
  }

  Step "(б) Журнал: по одному нарушению на браузер, записей «запускалась сама» по-прежнему одна" {
    $want = @("попытка открыть запрещённое: «$($script:EdgeName)»", "попытка открыть запрещённое: «$($script:ChromeName)»")
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $try = @(Get-KindEvents $script:TryKind)
    while ($try.Count -lt 2 -and $sw.Elapsed.TotalSeconds -lt 5) {
      Start-Sleep -Milliseconds 500
      $try = @(Get-KindEvents $script:TryKind)
    }
    $auto = @(Get-KindEvents $script:AutoKind)
    $all = Use-Parent { Format-Events }
    $try | ForEach-Object { Write-Log "Журнал: $($_.kind) $($_.text)" }
    Assert-That ($try.Count -eq 2) "Записей $($script:TryKind): $($try.Count), ждали 2 (по одной на msedge и chrome, PL-12); журнал: $all; журнал службы: $(Get-WatchLog)"
    foreach ($w in $want) {
      $hit = @($try | Where-Object { ([string]$_.text).Contains($w) })
      Assert-That ($hit.Count -eq 1) "Записей с «$w»: $($hit.Count), ждали 1; журнал: $all"
    }
    Assert-That ($auto.Count -eq 1) "Записей $($script:AutoKind) после ручных запусков: $($auto.Count), ждали по-прежнему 1; журнал: $all"
  }

  Step "(в) firefox.exe по решению родителя работает, плашки браузера нет" {
    [void](Wait-PlateGone "browser" 12)
    $p = Start-TestApp $script:FirefoxPath
    Assert-NoBrowserPlate "firefox.exe" 3
    Assert-AppAlive $p "firefox.exe «Прочее»"
    Write-Log "firefox.exe (PID $($p.Id)) работает 3 с"
  }

  Step "Подмена переднего плана снята" {
    Invoke-TestGuard "foreground"
    $script:FgOverride = $false
  }

  Write-Log "S19: Edge и Chrome закрыты с плашкой «$($script:BrowserPlate)» и нарушением, автозапуск Edge молча, Firefox по решению родителя работает"
}
