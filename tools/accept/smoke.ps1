# Ранний смоук службы PC Block на настоящей Windows (план M1, Ruling 26, задача T9).
# Запускается из windows.yml вручную (workflow_dispatch, scenario=smoke), только на windows-latest (pwsh).
# Проверяет: тихая установка, служба зарегистрирована и работает, права на папку данных, тестовый режим,
# первичная настройка, значок и страж есть в сеансе раннера (один на сеанс, страж от имени системы),
# GetChildStatus отвечает, политика Диспетчера задач, остановка службы убирает значок и стража,
# удаление не оставляет ничего. Сложные сценарии (S1...S7, S25...S27) лежат в s01...s27 и run-all.ps1.
# Общие функции (установка, тестовый режим, журнал, сброс машины) живут в common.ps1.
# Журнал дописывается в ci.yml-овский ci.log, ошибки видны пометками сборки (tools/annotate-errors.sh).
param(
  [string]$InstallerDir = "build/bin",
  [string]$LogPath = ""
)
$ErrorActionPreference = "Stop"
if ($LogPath) { $env:PCB_ACCEPT_LOG = $LogPath }
$env:PCB_INSTALLER_DIR = (Resolve-Path -LiteralPath $InstallerDir).Path
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "smoke" "Ранний смоук службы" {
  Reset-Machine

  Step "Установка программы (установщик, тихо)" {
    Install-Pcb
  }

  Step "Регистрация службы (--install-service, два раза подряд)" {
    Test-Reply (Invoke-Pcb @("--install-service", "--silent") "install.json") "Первая установка службы"
    Test-Reply (Invoke-Pcb @("--install-service", "--silent") "install-again.json") "Повторная установка службы"
  }

  Step "Служба работает, настроена, папка данных закрыта" {
    Wait-Until { (Get-ServiceState) -eq "Running" } 35 "служба в состоянии Running"
    $cim = Get-CimInstance Win32_Service -Filter "Name='$($script:ServiceName)'"
    if ($cim.StartMode -ne "Auto") { throw "Тип запуска '$($cim.StartMode)', ждали Auto" }
    if ($cim.StartName -ne "LocalSystem") { throw "Служба работает от имени '$($cim.StartName)', ждали LocalSystem" }
    if ($cim.PathName -notmatch "--service") { throw "В команде запуска нет --service: $($cim.PathName)" }
    $qf = (& sc.exe qfailure $script:ServiceName) -join " "
    foreach ($ms in 5000, 10000, 30000) {
      if ($qf -notmatch "\b$ms\b") { throw "В настройках сбоя нет перезапуска через $ms мс: $qf" }
    }
    foreach ($k in $script:SafeBootKeys) {
      if (-not (Test-Path -Path $k)) { throw "Нет записи безопасного режима $k" }
    }
    $acl = Get-Acl -LiteralPath $script:DefaultData
    if (-not $acl.AreAccessRulesProtected) { throw "Права папки данных наследуются от родителя" }
    $allowed = @("S-1-5-18", "S-1-5-32-544")
    foreach ($rule in $acl.Access) {
      $sid = $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
      if ($allowed -notcontains $sid) { throw "У папки данных лишний доступ: $($rule.IdentityReference)" }
    }
  }

  Step "Тестовый режим включён, службу перезапускаем" {
    Enable-TestMode 1
    $hello = Invoke-Pcb @("--test-ipc", '--payload={"cmd":"Hello"}') "hello.json"
    Test-Reply $hello "Hello"
    if (-not $hello.Result.payload.testMode) { throw "Служба не в тестовом режиме" }
    if ($hello.Result.payload.configured) { throw "Программа уже настроена, ждали чистую базу" }
    if (-not (Test-Path -LiteralPath (Join-Path $script:Data "pcblock.db"))) { throw "База не лежит в папке данных тестового режима $($script:Data)" }
  }

  Step "Первичная настройка (--test-setup)" {
    Test-Reply (Invoke-Pcb @("--test-setup", "--password=$($script:ParentPassword)", "--user=$env:USERNAME") "setup.json") "Первичная настройка"
    $hello = Invoke-Pcb @("--test-ipc", '--payload={"cmd":"Hello"}') "hello2.json"
    Test-Reply $hello "Hello после настройки"
    if (-not $hello.Result.payload.configured) { throw "После настройки программа считается ненастроенной" }
  }

  Step "Значок и страж запущены в сеансе раннера" {
    Wait-SessionParts
    $trays = @(Get-SessionProcs "--tray")
    $guards = @(Get-SessionProcs "--guard")
    if ($trays.Count -ne 1) { throw "Значков в сеансе $($script:MySession) : $($trays.Count), ждали ровно один" }
    if ($guards.Count -ne 1) { throw "Стражей в сеансе $($script:MySession) : $($guards.Count), ждали ровно один" }
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
    $key = "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Policies\System"
    Wait-Until {
      $v = Get-ItemProperty -Path $key -Name DisableTaskMgr -ErrorAction SilentlyContinue
      $v -and $v.DisableTaskMgr -eq 1
    } 20 "DisableTaskMgr=1 в ветке ребёнка"
  }

  Step "Остановка службы убирает значок и стража, запуск возвращает" {
    Stop-Service -Name $script:ServiceName -Force
    Wait-Until { (Get-ServiceState) -eq "Stopped" } 35 "служба остановлена"
    Wait-Until { @(Get-PcbProcs | Where-Object { $_.CommandLine -match "--(tray|guard)" }).Count -eq 0 } 15 "значок и страж завершены вместе со службой"
    Start-Service -Name $script:ServiceName
    Wait-Until { (Get-ServiceState) -eq "Running" } 35 "служба снова работает"
    Wait-SessionParts
  }

  Step "Удаление: служба, процессы, запись безопасного режима, политика" {
    Test-Reply (Invoke-Pcb @("--remove-service", "--silent") "remove.json") "Удаление службы"
    if ((Get-ServiceState) -ne "нет") { throw "Служба осталась после удаления: $(Get-ServiceState)" }
    Wait-Until { @(Get-PcbProcs | Where-Object { $_.CommandLine -match "--(tray|guard|service)" }).Count -eq 0 } 15 "процессы программы завершены"
    foreach ($k in $script:SafeBootKeys) {
      if (Test-Path -Path $k) { throw "Запись безопасного режима осталась: $k" }
    }
    $key = "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Policies\System"
    $v = Get-ItemProperty -Path $key -Name DisableTaskMgr -ErrorAction SilentlyContinue
    if ($v -and $v.DisableTaskMgr -eq 1) { throw "Запрет Диспетчера задач остался после удаления" }
    if (-not (Test-Path -LiteralPath (Join-Path $script:Data "pcblock.db"))) { throw "Статистика пропала без просьбы её удалять" }
    Remove-Item -Path $script:TestKey -Recurse -Force
    $un = Join-Path $script:InstallDir "uninstall.exe"
    if (-not (Test-Path -LiteralPath $un)) { throw "Нет uninstall.exe в $($script:InstallDir)" }
    Start-Process -FilePath $un -ArgumentList "/S" -Wait
    Wait-Until { -not (Test-Path -LiteralPath $script:Exe) } 30 "установщик удалил программу"
  }

  Write-Log "Смоук службы пройден"
}
