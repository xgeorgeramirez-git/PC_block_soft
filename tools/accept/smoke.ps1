# Ранний смоук службы PC Block на настоящей Windows (план M1, Ruling 26, задача T9).
# Запускается из windows.yml вручную (workflow_dispatch, scenario=smoke), только на windows-latest (pwsh).
# Проверяет: тихая установка, служба зарегистрирована и работает, права на папку данных, тестовый режим,
# первичная настройка, значок и страж есть в сеансе раннера (один на сеанс, страж от имени системы),
# GetChildStatus отвечает, политика Диспетчера задач, остановка службы убирает значок и стража,
# удаление не оставляет ничего. Сложные сценарии (S1...S7, S25...S27) здесь не дублируются.
# Журнал дописывается в ci.yml-овский ci.log, ошибки видны пометками сборки (tools/annotate-errors.sh).
param(
  [string]$InstallerDir = "build/bin",
  [string]$LogPath = ""
)
$ErrorActionPreference = "Stop"
if (-not $LogPath) { $LogPath = Join-Path ([IO.Path]::GetTempPath()) "ci.log" }

$ServiceName = "PCBlock"
$Password = "PcbSmoke-2026"
$TestKey = "HKLM:\SOFTWARE\PCBlock\TestMode"
$SafeBootKeys = @(
  "HKLM:\SYSTEM\CurrentControlSet\Control\SafeBoot\Minimal\PCBlock",
  "HKLM:\SYSTEM\CurrentControlSet\Control\SafeBoot\Network\PCBlock"
)
$DefaultData = Join-Path $env:ProgramData "PCBlock"
$TempRoot = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [IO.Path]::GetTempPath() }
$Work = Join-Path $TempRoot "pcb-smoke"
$Data = Join-Path $TempRoot "pcb-data"
$MySid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$MySession = (Get-Process -Id $PID).SessionId
$script:Exe = $null
$script:InstallDir = $null
New-Item -ItemType Directory -Force -Path $Work | Out-Null

function Write-Log([string]$Text) {
  $line = "{0:HH:mm:ss} {1}" -f (Get-Date), $Text
  Write-Host $line
  Add-Content -LiteralPath $LogPath -Value $line -Encoding UTF8
}

function Write-Fail([string]$Text) {
  Write-Log "ERROR: $Text"
}

function Step([string]$Name, [scriptblock]$Body) {
  Write-Log "== $Name"
  try {
    & $Body
  } catch {
    Write-Fail ("Шаг '" + $Name + "': " + $_.Exception.Message)
    throw
  }
  Write-Log "готово: $Name"
}

function Wait-Until([scriptblock]$Cond, [int]$Seconds, [string]$What) {
  $deadline = (Get-Date).AddSeconds($Seconds)
  while ($true) {
    if (& $Cond) { return }
    if ((Get-Date) -gt $deadline) { throw "За $Seconds с не дождались: $What" }
    Start-Sleep -Milliseconds 500
  }
}

# Одна строка аргументов для Start-Process: пробелы и кавычки экранируются так, как их разбирает Go.
function ConvertTo-ArgString([string[]]$List) {
  $parts = foreach ($a in $List) {
    if ($a -match '[\s"]') {
      $s = $a -replace '(\\*)"', '$1$1\"'
      $s = $s -replace '(\\+)$', '$1$1'
      '"' + $s + '"'
    } else {
      $a
    }
  }
  return ($parts -join ' ')
}

# Запускает PCBlock.exe с ключами, ждёт, читает JSON из --out. Возвращает объект Code и Result.
function Invoke-Pcb([string[]]$ArgList, [string]$OutName) {
  $out = Join-Path $Work $OutName
  Remove-Item -LiteralPath $out -ErrorAction SilentlyContinue
  $all = @($ArgList) + @("--out=$out")
  $p = Start-Process -FilePath $script:Exe -ArgumentList (ConvertTo-ArgString $all) -Wait -PassThru -WindowStyle Hidden
  $res = $null
  $shown = "нет файла результата"
  if (Test-Path -LiteralPath $out) {
    $raw = Get-Content -LiteralPath $out -Raw -Encoding UTF8
    $res = $raw | ConvertFrom-Json
    $shown = ($raw -replace '\s+', ' ')
  }
  Write-Log ("{0} -> код {1}; {2}" -f $ArgList[0], $p.ExitCode, $shown)
  return [pscustomobject]@{ Code = $p.ExitCode; Result = $res }
}

function Get-PcbProcs {
  Get-CimInstance Win32_Process -Filter "Name='PCBlock.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.ProcessId -ne $PID }
}

function Get-SessionProcs([string]$Flag) {
  Get-PcbProcs | Where-Object { $_.SessionId -eq $MySession -and $_.CommandLine -match [regex]::Escape($Flag) }
}

function Get-ServiceState {
  $s = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
  if ($s) { return [string]$s.Status } else { return "нет" }
}

function Wait-SessionParts {
  Wait-Until { (@(Get-SessionProcs "--tray").Count -ge 1) -and (@(Get-SessionProcs "--guard").Count -ge 1) } 60 "значок и страж в сеансе раннера"
}

function Test-Reply($r, [string]$What) {
  if ($r.Code -ne 0 -or -not $r.Result -or -not $r.Result.ok) {
    $why = if ($r.Result -and $r.Result.error) { $r.Result.error } else { "код выхода $($r.Code)" }
    throw "$What не удалось: $why"
  }
}

function Write-Diagnostics {
  Write-Log "ERROR: [диагностика] состояние службы: $(Get-ServiceState)"
  try { (& sc.exe query $ServiceName) | ForEach-Object { Write-Log "ERROR: [диагностика] sc query: $_" } } catch {}
  try {
    Get-PcbProcs | ForEach-Object {
      Write-Log "ERROR: [диагностика] процесс $($_.ProcessId) сеанс $($_.SessionId): $($_.CommandLine)"
    }
  } catch {}
  foreach ($dir in @($Data, $DefaultData)) {
    foreach ($name in @("service.log", "boot.log")) {
      $f = Join-Path (Join-Path $dir "logs") $name
      if (Test-Path -LiteralPath $f) {
        Get-Content -LiteralPath $f -Tail 25 -Encoding UTF8 | ForEach-Object { Write-Log "ERROR: [диагностика] $name`: $_" }
      }
    }
  }
  try {
    Get-WinEvent -FilterHashtable @{ LogName = 'System'; ProviderName = 'Service Control Manager'; StartTime = (Get-Date).AddMinutes(-15) } -MaxEvents 40 -ErrorAction SilentlyContinue |
      Where-Object { $_.Message -match 'PCBlock|PC Block' } | Select-Object -First 8 |
      ForEach-Object { Write-Log "ERROR: [диагностика] журнал Windows: $($_.TimeCreated.ToString('HH:mm:ss')) $(($_.Message -replace '\s+', ' '))" }
  } catch {}
}

# Приводит машину к чистому состоянию. Вызывается в конце в любом случае и ничего не бросает.
function Reset-Machine {
  try {
    if ((Get-ServiceState) -ne "нет" -and $script:Exe -and (Test-Path -LiteralPath $script:Exe)) {
      Start-Process -FilePath $script:Exe -ArgumentList (ConvertTo-ArgString @("--remove-service", "--silent", "--delete-data", "--out=$(Join-Path $Work 'remove-final.json')")) -Wait -WindowStyle Hidden
    }
  } catch {}
  try { Get-PcbProcs | ForEach-Object { & taskkill.exe /F /T /PID $_.ProcessId | Out-Null } } catch {}
  try { Remove-Item -Path $TestKey -Recurse -Force -ErrorAction SilentlyContinue } catch {}
  try { foreach ($k in $SafeBootKeys) { Remove-Item -Path $k -Recurse -Force -ErrorAction SilentlyContinue } } catch {}
  try {
    if ($script:InstallDir) {
      $un = Join-Path $script:InstallDir "uninstall.exe"
      if (Test-Path -LiteralPath $un) { Start-Process -FilePath $un -ArgumentList "/S" -Wait }
    }
  } catch {}
  try { Remove-Item -LiteralPath $Data -Recurse -Force -ErrorAction SilentlyContinue } catch {}
  try { Remove-Item -LiteralPath $DefaultData -Recurse -Force -ErrorAction SilentlyContinue } catch {}
}

$ok = $false
try {
  Step "Установка программы (установщик, тихо)" {
    $inst = Get-ChildItem -LiteralPath $InstallerDir -Filter "*installer*.exe" | Select-Object -First 1
    if (-not $inst) { throw "Установщик не найден в $InstallerDir" }
    Start-Process -FilePath $inst.FullName -ArgumentList "/S" -Wait
    $found = Get-ChildItem -Path $env:ProgramFiles -Recurse -Depth 3 -Filter "PCBlock.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $found) { throw "После установки PCBlock.exe не найден в Program Files" }
    $script:Exe = $found.FullName
    $script:InstallDir = $found.DirectoryName
    Write-Log "Установлено: $script:Exe"
  }

  Step "Регистрация службы (--install-service, два раза подряд)" {
    Test-Reply (Invoke-Pcb @("--install-service", "--silent") "install.json") "Первая установка службы"
    Test-Reply (Invoke-Pcb @("--install-service", "--silent") "install-again.json") "Повторная установка службы"
  }

  Step "Служба работает, настроена, папка данных закрыта" {
    Wait-Until { (Get-ServiceState) -eq "Running" } 35 "служба в состоянии Running"
    $cim = Get-CimInstance Win32_Service -Filter "Name='$ServiceName'"
    if ($cim.StartMode -ne "Auto") { throw "Тип запуска '$($cim.StartMode)', ждали Auto" }
    if ($cim.StartName -ne "LocalSystem") { throw "Служба работает от имени '$($cim.StartName)', ждали LocalSystem" }
    if ($cim.PathName -notmatch "--service") { throw "В команде запуска нет --service: $($cim.PathName)" }
    $qf = (& sc.exe qfailure $ServiceName) -join " "
    foreach ($ms in 5000, 10000, 30000) {
      if ($qf -notmatch "\b$ms\b") { throw "В настройках сбоя нет перезапуска через $ms мс: $qf" }
    }
    foreach ($k in $SafeBootKeys) {
      if (-not (Test-Path -Path $k)) { throw "Нет записи безопасного режима $k" }
    }
    $acl = Get-Acl -LiteralPath $DefaultData
    if (-not $acl.AreAccessRulesProtected) { throw "Права папки данных наследуются от родителя" }
    $allowed = @("S-1-5-18", "S-1-5-32-544")
    foreach ($rule in $acl.Access) {
      $sid = $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
      if ($allowed -notcontains $sid) { throw "У папки данных лишний доступ: $($rule.IdentityReference)" }
    }
  }

  Step "Тестовый режим включён, службу перезапускаем" {
    New-Item -Path $TestKey -Force | Out-Null
    New-ItemProperty -Path $TestKey -Name DataDir -Value $Data -PropertyType String -Force | Out-Null
    New-ItemProperty -Path $TestKey -Name ClockSpeed -Value 1 -PropertyType DWord -Force | Out-Null
    New-ItemProperty -Path $TestKey -Name TestChildSID -Value $MySid -PropertyType String -Force | Out-Null
    Restart-Service -Name $ServiceName -Force
    Wait-Until { (Get-ServiceState) -eq "Running" } 35 "служба в состоянии Running после перезапуска"
    $hello = Invoke-Pcb @("--test-ipc", '--payload={"cmd":"Hello"}') "hello.json"
    Test-Reply $hello "Hello"
    if (-not $hello.Result.payload.testMode) { throw "Служба не в тестовом режиме" }
    if ($hello.Result.payload.configured) { throw "Программа уже настроена, ждали чистую базу" }
    if (-not (Test-Path -LiteralPath (Join-Path $Data "pcblock.db"))) { throw "База не лежит в папке данных тестового режима $Data" }
  }

  Step "Первичная настройка (--test-setup)" {
    Test-Reply (Invoke-Pcb @("--test-setup", "--password=$Password", "--user=$env:USERNAME") "setup.json") "Первичная настройка"
    $hello = Invoke-Pcb @("--test-ipc", '--payload={"cmd":"Hello"}') "hello2.json"
    Test-Reply $hello "Hello после настройки"
    if (-not $hello.Result.payload.configured) { throw "После настройки программа считается ненастроенной" }
  }

  Step "Значок и страж запущены в сеансе раннера" {
    Wait-SessionParts
    $trays = @(Get-SessionProcs "--tray")
    $guards = @(Get-SessionProcs "--guard")
    if ($trays.Count -ne 1) { throw "Значков в сеансе $MySession : $($trays.Count), ждали ровно один" }
    if ($guards.Count -ne 1) { throw "Стражей в сеансе $MySession : $($guards.Count), ждали ровно один" }
    $guardOwner = Invoke-CimMethod -InputObject $guards[0] -MethodName GetOwner
    if ($guardOwner.User -ne "SYSTEM") { throw "Страж работает от имени '$($guardOwner.User)', ждали SYSTEM" }
    $trayOwner = Invoke-CimMethod -InputObject $trays[0] -MethodName GetOwner
    if ($trayOwner.User -eq "SYSTEM") { throw "Значок работает от имени системы, ждали пользователя сеанса" }
  }

  Step "Канал связи отвечает (GetChildStatus)" {
    $st = Invoke-Pcb @("--test-ipc", '--payload={"cmd":"GetChildStatus"}') "status.json"
    Test-Reply $st "GetChildStatus"
    $p = $st.Result.payload
    if ($p.status -eq "notChild" -or $p.status -eq "safe") { throw "Статус ребёнка '$($p.status)', ждали обычный" }
    if ($p.limitS -ne 7200) { throw "Лимит дня $($p.limitS) с, ждали 7200 (2 часа из настройки)" }
  }

  Step "Запрет Диспетчера задач у ребёнка (FR-29)" {
    $key = "Registry::HKEY_USERS\$MySid\Software\Microsoft\Windows\CurrentVersion\Policies\System"
    Wait-Until {
      $v = Get-ItemProperty -Path $key -Name DisableTaskMgr -ErrorAction SilentlyContinue
      $v -and $v.DisableTaskMgr -eq 1
    } 20 "DisableTaskMgr=1 в ветке ребёнка"
  }

  Step "Остановка службы убирает значок и стража, запуск возвращает" {
    Stop-Service -Name $ServiceName -Force
    Wait-Until { (Get-ServiceState) -eq "Stopped" } 35 "служба остановлена"
    Wait-Until { @(Get-PcbProcs | Where-Object { $_.CommandLine -match "--(tray|guard)" }).Count -eq 0 } 15 "значок и страж завершены вместе со службой"
    Start-Service -Name $ServiceName
    Wait-Until { (Get-ServiceState) -eq "Running" } 35 "служба снова работает"
    Wait-SessionParts
  }

  Step "Удаление: служба, процессы, запись безопасного режима, политика" {
    Test-Reply (Invoke-Pcb @("--remove-service", "--silent") "remove.json") "Удаление службы"
    if ((Get-ServiceState) -ne "нет") { throw "Служба осталась после удаления: $(Get-ServiceState)" }
    Wait-Until { @(Get-PcbProcs | Where-Object { $_.CommandLine -match "--(tray|guard|service)" }).Count -eq 0 } 15 "процессы программы завершены"
    foreach ($k in $SafeBootKeys) {
      if (Test-Path -Path $k) { throw "Запись безопасного режима осталась: $k" }
    }
    $key = "Registry::HKEY_USERS\$MySid\Software\Microsoft\Windows\CurrentVersion\Policies\System"
    $v = Get-ItemProperty -Path $key -Name DisableTaskMgr -ErrorAction SilentlyContinue
    if ($v -and $v.DisableTaskMgr -eq 1) { throw "Запрет Диспетчера задач остался после удаления" }
    if (-not (Test-Path -LiteralPath (Join-Path $Data "pcblock.db"))) { throw "Статистика пропала без просьбы её удалять" }
    Remove-Item -Path $TestKey -Recurse -Force
    $un = Join-Path $script:InstallDir "uninstall.exe"
    if (-not (Test-Path -LiteralPath $un)) { throw "Нет uninstall.exe в $script:InstallDir" }
    Start-Process -FilePath $un -ArgumentList "/S" -Wait
    Wait-Until { -not (Test-Path -LiteralPath $script:Exe) } 30 "установщик удалил программу"
  }

  $ok = $true
} catch {
  Write-Diagnostics
} finally {
  Reset-Machine
}

if (-not $ok) {
  Write-Host "::error title=Смоук службы::Смоук не пройден, подробности в пометках сборки и журнале шага"
  exit 1
}
Write-Log "Смоук службы пройден"
