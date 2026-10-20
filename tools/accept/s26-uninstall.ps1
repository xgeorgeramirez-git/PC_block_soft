# S26. Удаление программы с паролем родителя (ТЗ 11, FR-4, Ruling 21).
# Тихое удаление (/S) идёт тем же путём, что «Параметры, Приложения»: деинсталлятор спрашивает программу.
# Тогда: без пароля и с неверным паролем отказ, программа и служба на месте; с верным паролем служба и папка
# программы удалены, база осталась, политик и записи безопасного режима нет, процессов нет;
# повтор с /DELETEDATA удаляет и базу. Правила браузера появятся в M5: здесь снимать нечего.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$pw = $script:ParentPassword
$prog = Join-Path $env:ProgramFiles "PCBlock"
$un = Join-Path $prog "uninstall.exe"
$gateLog = Join-Path $script:Data "logs\boot.log"

# Деинсталлятор доходит до проверки и пишет её итог в журнал запуска; потом процесс Au_ завершается.
function Wait-GateAnswer([string]$Pattern) {
  Wait-Until { (Test-Path -LiteralPath $gateLog) -and (Select-String -LiteralPath $gateLog -Pattern $Pattern -Quiet) } 60 "ответ проверки удаления '$Pattern' в boot.log"
  Wait-Until { -not (Get-Process -Name "Au_" -ErrorAction SilentlyContinue) } 60 "завершение деинсталлятора"
}

function Assert-Installed([string]$When) {
  Assert-That (Test-Path -LiteralPath (Join-Path $prog "PCBlock.exe")) "${When}: программа удалена, а должна остаться"
  Assert-That ((Get-ServiceState) -eq "Running") "${When}: служба не работает ($(Get-ServiceState)), а должна работать"
}

function Assert-Removed([string]$When) {
  Wait-Until { -not (Test-Path -LiteralPath (Join-Path $prog "PCBlock.exe")) } 60 "${When}: PCBlock.exe удалён"
  Wait-Until { (Get-ServiceState) -eq "нет" } 30 "${When}: служба удалена"
  Wait-Until { -not (Get-Process -Name "Au_" -ErrorAction SilentlyContinue) } 60 "${When}: деинсталлятор завершён"
  Wait-Until { -not (Test-Path -LiteralPath $prog) } 30 "${When}: папка программы $prog удалена"
  Wait-Until { @(Get-PcbProcs).Count -eq 0 } 20 "${When}: процессов PCBlock.exe нет"
  foreach ($k in $script:SafeBootKeys) { Assert-That (-not (Test-Path -Path $k)) "${When}: осталась запись безопасного режима $k" }
  $key = "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Policies\System"
  $v = Get-ItemProperty -Path $key -Name DisableTaskMgr -ErrorAction SilentlyContinue
  Assert-That (-not ($v -and $v.DisableTaskMgr -eq 1)) "${When}: остался запрет Диспетчера задач"
}

Invoke-Scenario "S26" "Удаление с паролем родителя: отказ, удаление, база, /DELETEDATA" {
  Initialize-Scenario 1

  Step "Настройка: пароль родителя, ребёнок «Тест»; защита работает" {
    Test-Reply (Invoke-Pcb @("--test-setup", "--password=$pw", "--user=$env:USERNAME") "setup.json") "Первичная настройка"
    Wait-SessionParts
    $key = "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Policies\System"
    Wait-Until { $v = Get-ItemProperty -Path $key -Name DisableTaskMgr -ErrorAction SilentlyContinue; $v -and $v.DisableTaskMgr -eq 1 } 20 "запрет Диспетчера задач стоит до удаления"
  }

  Step "/S без пароля: отказ, программа и служба на месте" {
    Start-Process -FilePath $un -ArgumentList "/S" -Wait
    Wait-GateAnswer "не указан"
    Assert-Installed "удаление без пароля"
  }

  Step "/S /PASSWORD= с неверным паролем: отказ" {
    Start-Process -FilePath $un -ArgumentList "/S", "/PASSWORD=Wrong-Pass-1" -Wait
    Wait-GateAnswer "Неверный пароль"
    Assert-Installed "удаление с неверным паролем"
  }

  Step "/S /PASSWORD= с верным паролем: удалено, база осталась" {
    Start-Process -FilePath $un -ArgumentList "/S", "/PASSWORD=$pw" -Wait
    Assert-Removed "верный пароль"
    Assert-That (Test-Path -LiteralPath (Join-Path $script:Data "pcblock.db")) "Статистика пропала без /DELETEDATA"
    Write-Log "Удалено: служба, папка программы, запись безопасного режима, политики; база $($script:Data)\pcblock.db на месте"
  }

  Step "Повторная установка: пароль и статистика вернулись" {
    Install-Pcb
    Enable-TestMode 1 $script:MySid $false
    $hello = Send-Pcb "Hello"
    Assert-That $hello.payload.configured "После переустановки программа считается ненастроенной, ждали прежние данные"
    $r = Send-Pcb "Login" @{ password = $pw }
    Assert-That $r.ok "Прежний пароль после переустановки не принят: $($r.error)"
  }

  Step "/S /PASSWORD= /DELETEDATA: удалено вместе с базой" {
    Start-Process -FilePath $un -ArgumentList "/S", "/PASSWORD=$pw", "/DELETEDATA" -Wait
    Assert-Removed "верный пароль и /DELETEDATA"
    Wait-Until { -not (Test-Path -LiteralPath $script:Data) } 30 "папка данных $($script:Data) удалена по /DELETEDATA"
    Write-Log "Папка общих данных $($script:DefaultData) после /DELETEDATA: $(if (Test-Path -LiteralPath $script:DefaultData) { 'осталась' } else { 'удалена' })"
  }

  Write-Log "S26: без пароля и с неверным отказ; с верным удалено, база осталась; с /DELETEDATA база удалена"
}
