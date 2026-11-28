# S63. Окно при входе в Windows (FR-46, D-108, R-13).
# Часы ядра идут в 60 раз быстрее (Start-Child ставит их на паузу). Вход и выход учётки изображает тестовая команда
# службы TestSession тем же путём, что события Windows (второго пользователя у раннера нет, PL-10): logon — вход,
# logoff — выход (после него следующий вход снова открывает окно), lock/unlock — блокировка и разблокировка сеанса.
# Учётка раннера — ребёнок; «учётка родителя» — та же учётка, снятая с детей (SetChildren remove).
# Дано: программа настроена, у ребёнка лимит 10 мин. Когда: служба стартовала после входа; первый вход после
# установки; выход и снова вход; окно закрыто крестиком; блокировка и разблокировка; время вышло, выход и вход;
# учётка снята с детей, выход и вход.
# Тогда: служба, стартовавшая после входа, окон не открывает (R-13); первый вход — окно первого входа (FR-32), после
# «Понятно» — «Моя статистика»; следующий вход — «Моя статистика» не позже 10 с (снимок); после закрытия крестиком
# учёт идёт, записи обхода нет, окно не открывается снова; после блокировки и разблокировки окна нет 15 с; при
# исчерпанном времени — экран блокировки без окна; вход учётки родителя — окно родителя (--parent без --handover,
# пароль спросит окно; снимок). Снимки: s63-firstrun.png, s63-mystats.png, s63-parent.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Ждёт окно программы с ключом Flag в сеансе раннера не дольше Seconds после события входа (Since — Stopwatch
# от события). Возвращает процесс Win32_Process.
function Wait-LogonWindow([string]$Flag, $Since, [double]$Seconds = 10) {
  $re = [regex]::Escape($Flag) + '(\s|$|=)'
  try {
    Wait-Until { @(Get-PcbProcs | Where-Object { $_.SessionId -eq $script:MySession -and $_.CommandLine -match $re }).Count -ge 1 } $Seconds "окно $Flag после входа"
  } catch {
    Write-ServiceSessionLog "S63"
    throw "$($_.Exception.Message); окна в сеансе: $(Format-LogonWindowProcs)"
  }
  $p = @(Get-PcbProcs | Where-Object { $_.SessionId -eq $script:MySession -and $_.CommandLine -match $re })[0]
  Write-Log ("Окно {0} через {1:N1} с после входа: PID {2}, {3}" -f $Flag, $Since.Elapsed.TotalSeconds, $p.ProcessId, $p.CommandLine)
  Assert-That ($Since.Elapsed.TotalSeconds -le $Seconds) ("Окно {0} через {1:N1} с после входа, ждали не позже {2} с (FR-46)" -f $Flag, $Since.Elapsed.TotalSeconds, $Seconds)
  return $p
}

# Окон при входе в сеансе раннера нет Seconds секунд (PL-16: отсутствие проверяется после события).
function Assert-NoLogonWindow([double]$Seconds, [string]$What) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
    $l = @(Get-LogonWindowProcs)
    if ($l.Count -gt 0) {
      Write-ServiceSessionLog "S63"
      throw ("{0}: через {1:N1} с открыто окно: {2}" -f $What, $sw.Elapsed.TotalSeconds, (Format-LogonWindowProcs))
    }
    Start-Sleep -Milliseconds 250
  }
  Write-Log ("{0}: окон программы в сеансе нет {1:N0} с" -f $What, $Seconds)
}

function Get-BypassEvents { return @(Find-Events 'попытка обойти защиту') }

Invoke-Scenario "S63" "Окно при входе: ребёнку «Моя статистика», родителю окно с паролем, экран блокировки главнее" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: лимит 10 мин, часы на паузе; служба, стартовавшая после входа, окон не открывает (R-13)" {
    Start-Child 10
    Use-Parent { Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы" }
    Assert-NoLogonWindow 5 "Служба запущена после входа"
  }

  Step "Первый вход после установки: окно первого входа не позже 10 с, после «Понятно» — «Моя статистика»" {
    $since = [Diagnostics.Stopwatch]::StartNew()
    Send-SessionEvent "logon"
    $fr = Wait-LogonWindow "--firstrun" $since
    $p = Get-Process -Id ([int]$fr.ProcessId)
    try { [void](Save-ProcWindow $p "s63-firstrun" 30 3) } catch { Write-Log "Снимок окна первого входа не сделан: $($_.Exception.Message)" }
    # «Понятно» окна первого входа — команда FirstRunSeen от имени ребёнка (учётка раннера).
    $sinceOk = [Diagnostics.Stopwatch]::StartNew()
    Assert-Reply (Send-Pcb "FirstRunSeen") "FirstRunSeen («Понятно»)"
    $ms = Wait-LogonWindow "--mystats" $sinceOk
    [void](Close-PcbWindow ([int]$fr.ProcessId) "Окно первого входа")
    [void](Close-PcbWindow ([int]$ms.ProcessId) "«Моя статистика» после «Понятно»")
    Wait-Until { @(Get-LogonWindowProcs).Count -eq 0 } 10 "окна первого входа закрыты"
  }

  $script:Stats = $null
  Step "Выход и снова вход: «Моя статистика» не позже 10 с, снимок" {
    Send-SessionEvent "logoff"
    Start-Sleep -Seconds 1
    $since = [Diagnostics.Stopwatch]::StartNew()
    Send-SessionEvent "logon"
    $script:Stats = Wait-LogonWindow "--mystats" $since
    $p = Get-Process -Id ([int]$script:Stats.ProcessId)
    $title = Save-ProcWindow $p "s63-mystats" 30 5
    Write-Log "«Моя статистика» при входе: заголовок «$title»"
    Assert-That (@(Get-LogonWindowProcs | Where-Object { $_.CommandLine -match '--(firstrun|parent)(\s|$|=)' }).Count -eq 0) "Кроме «Моей статистики» открыто: $(Format-LogonWindowProcs)"
  }

  Step "Окно закрыто крестиком: учёт идёт, записи обхода нет, окно не открывается снова" {
    $closed = Close-PcbWindow ([int]$script:Stats.ProcessId) "«Моя статистика» при входе"
    Assert-That $closed "«Моя статистика» не закрылась крестиком за 10 с"
    $before = Get-Status
    Resume-Clock
    Start-Sleep -Seconds 3
    Suspend-Clock
    $after = Get-Status
    $dropped = [int]$before.remainingS - [int]$after.remainingS
    Write-Log "Остаток до $($before.remainingS) с, после 3 с настоящего времени $($after.remainingS) с, убыло $dropped с"
    Assert-Between $dropped 100 260 "Учёт после закрытия окна: остаток убыл за 3 с (часы ядра x60), с"
    Assert-NoLogonWindow 5 "После закрытия «Моей статистики»"
    Use-Parent {
      $b = @(Get-BypassEvents)
      Assert-That ($b.Count -eq 0) "Закрытие окна записано обходом: $(($b | ForEach-Object { $_.text }) -join ' | ')"
    }
  }

  Step "Блокировка и разблокировка сеанса: окно не открывается 15 с" {
    Send-SessionEvent "lock"
    Start-Sleep -Seconds 2
    Send-SessionEvent "unlock"
    Assert-NoLogonWindow 15 "После блокировки и разблокировки"
  }

  Step "Время вышло, выход и снова вход: экран блокировки, окна нет" {
    Resume-Clock
    $g = Wait-LockShown 60 "timeUp"
    Suspend-Clock
    Write-Log "Экран блокировки: $(Format-GuardState $g)"
    Send-SessionEvent "logoff"
    Start-Sleep -Seconds 1
    Send-SessionEvent "logon"
    Assert-NoLogonWindow 15 "Вход при исчерпанном времени"
    $g = Wait-LockShown 15 "timeUp"
    Write-Log "После входа: $(Format-GuardState $g)"
    Save-Screen "s63-timeup-logon"
  }

  Step "Учётка раннера снята с детей, выход и вход: окно родителя с паролем (без --handover), снимок" {
    Use-Parent {
      $id = Get-ChildId
      $r = Send-Parent "SetChildren" @{ add = @(); remove = @($id); rename = @() }
      Assert-Reply $r "SetChildren remove $id"
      Write-Log "Учётка раннера снята с детей: детей осталось $(@(List $r.payload.children).Count)"
    }
    Wait-Until { (Get-Status).status -eq "notChild" } 30 "сеанс раннера не детский (notChild)"
    try {
      Wait-Until { @(Get-SessionProcs "--guard").Count -eq 0 } 30 "страж в сеансе раннера закрыт"
    } catch { Write-Log "Страж в сеансе раннера ещё работает: $($_.Exception.Message)" }
    Send-SessionEvent "logoff"
    Start-Sleep -Seconds 1
    $since = [Diagnostics.Stopwatch]::StartNew()
    Send-SessionEvent "logon"
    $pw = Wait-LogonWindow "--parent" $since
    Assert-That ($pw.CommandLine -notmatch '--handover') "Окно родителя при входе с готовой сессией: $($pw.CommandLine), ждали без --handover (пароль спросит окно)"
    Assert-That ($pw.CommandLine -notmatch '--screen') "Окно родителя при входе не на главном экране: $($pw.CommandLine)"
    $p = Get-Process -Id ([int]$pw.ProcessId)
    [void](Save-ProcWindow $p "s63-parent" 30 5)
    [void](Close-PcbWindow ([int]$pw.ProcessId) "Окно родителя при входе")
  }

  Write-Log "S63: окно при входе — первый вход, «Моя статистика», без повтора после блокировки, экран блокировки главнее, окно родителя у взрослой учётки"
}
