# S50. Автозагрузка ребёнка: новые записи выключены и ждут решения, родитель включает и выключает (FR-39, D-76, D-80,
# RM5b-1…RM5b-4, уточнения плана 1 и 15, PL-22).
# Часы ядра на паузе (Initialize-Scenario 60, Suspend-Clock): день учёта не меняется, наблюдатель идёт по настоящим часам.
# Дано (до первой проверки): общая запись PCBSharedTest в HKLM\…\Run (pcbshared.exe в папке WatchDir); два ребёнка —
# Миша (учётка раннера, его сеанс) и Коля (pcbtest, без сеанса). Программы notes.exe и pcbshared.exe — «Прочее»
# (разрешены мастером): закрыть их может только правило автозагрузки, а не «незнакомая» или «запрещённая».
# Когда: первая проверка у обоих; после неё у Миши своя запись PCBNotesTest в HKU\<SID>\…\Run; «вход» Миши —
# тестовое событие TestSession logon (служба забывает момент входа и проходит путь «ребёнок появился в сеансе»).
# «Запуск Проводником при входе»: из скрипта, после простоя ввода ≥ 10 с, в первые 60 с после входа — так правило
# наблюдателя отличает автозапуск от ручного (launchKind, как S45). Настоящий запуск Run Проводником при входе на
# раннере не воспроизвести (ребёнок уже вошёл); «после включения запускается» проверяется по StartupApproved (0x02)
# и тем, что запуск при входе больше не закрывается.
# Тогда: после первой проверки PCBSharedTest у обоих «включена»; новая PCBNotesTest — в «Ждут решения», выключена штатно
# (StartupApproved\Run первый байт 0x03), значение в Run на месте, журнал startup_new; автозапуск notes закрыт без
# плашки и нарушения, журнал startup_closed. Родитель включил notes — «Будет запускаться при входе Миши», байт 0x02,
# автозапуск notes живёт. Родитель выключил PCBSharedTest Мише — «Не будет запускаться при входе Миши», HKLM не тронут,
# автозапуск pcbshared закрыт без плашки и нарушения; у Коли PCBSharedTest «включена».
# Ruling T6-1: запуск pcbshared от имени pcbtest (Invoke-AsTestUser) попадает в сеанс раннера процессом другого
# пользователя, а такой процесс наблюдатель не трогает никогда (RM3-13) — «живёт» ничего бы не доказало. У Коли
# проверяется решение службы (ListStartup: on), а то, что общая off у одного ребёнка не закрывает её у другого, —
# юнит-тест службы T4b.
# Ruling T6-2: снимка окна родителя на вкладке «Автозагрузка» нет: окно открывается с паролем на главном экране, ключа
# запуска на вкладку ребёнка нет (launch.Screens — только programs). Вкладку снимают e2e-снимки s50-startup*.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$script:RunOwn = "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Run"
$script:ApprovedOwn = "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run"
$script:RunShared = "Registry::HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run"
$script:ApprovedShared = "Registry::HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run"
$script:NotesName = $script:StartupTestNotes
$script:SharedName = $script:StartupTestShared
# Плашки о закрытой программе (ipc.Blocked*): ни одной из них при автозапуске быть не должно.
$script:BlockedKinds = @("unknown", "forbidden", "gamesOver", "studyOnly", "decided")

# Строки журнала службы про наблюдатель и автозагрузку (для текста ошибки).
function Get-StartupLog {
  $log = Join-Path (Join-Path $script:Data "logs") "service.log"
  if (-not (Test-Path -LiteralPath $log)) { return "service.log нет" }
  $lines = @(Get-Content -LiteralPath $log -Encoding UTF8 -ErrorAction SilentlyContinue | Where-Object { $_ -match 'наблюдатель|автозагрузк|Тестовое событие' }) | Select-Object -Last 10
  if ($lines.Count -eq 0) { return "в service.log нет строк о наблюдателе и автозагрузке" }
  return ($lines -join ' | ')
}

function Send-Logon {
  $r = Send-Pcb "TestSession" @{ event = "logon"; sessionId = [int]$script:MySession }
  Assert-Reply $r "TestSession logon $($script:MySession)"
  Write-Log "Вход ребёнка (TestSession logon, сеанс $($script:MySession))"
}

function Get-Startup([int64]$ChildId, [bool]$Refresh = $false) {
  $r = Send-Parent "ListStartup" @{ childId = $ChildId; refresh = $Refresh }
  Assert-Reply $r "ListStartup $ChildId"
  return $r.payload
}

function Format-Startup($L) {
  $f = { param($b) (@(List $b) | ForEach-Object { "$($_.name)=$($_.decision)/$($_.scope)/$($_.source)" }) -join ', ' }
  return "ждут [$(& $f $L.pending)]; свои [$(& $f $L.own)]; общие [$(& $f $L.shared)]; проверено $($L.checkedAt), идёт $($L.checking)"
}

# Строка Name блока Block ("pending", "own", "shared") или $null.
function Find-StartupItem($L, [string]$Block, [string]$Name) {
  return @(@(List $L.$Block) | Where-Object { [string]$_.name -eq $Name }) | Select-Object -First 1
}

# Ждёт законченную проверку ребёнка не раньше момента SinceMs (UTC мс); Refresh — сначала попросить проверку.
function Wait-StartupCheck([int64]$ChildId, [int64]$SinceMs, [bool]$Refresh, [string]$What) {
  if ($Refresh) { [void](Get-Startup $ChildId $true) }
  $script:LastStartup = $null
  Wait-Fresh {
    $l = Get-Startup $ChildId
    $script:LastStartup = $l
    ([int64]$l.checkedAt -ge $SinceMs) -and (-not $l.checking)
  } 30 "проверка автозагрузки: $What" { "последний ответ: $(if ($script:LastStartup) { Format-Startup $script:LastStartup } else { 'нет' }); журнал службы: $(Get-StartupLog)" }
  Write-Log "Автозагрузка, $What`: $(Format-Startup $script:LastStartup)"
  return $script:LastStartup
}

# Wait-Until, но в тексте ошибки — свежие подробности (Detail вычисляется в момент сбоя, а не при вызове).
function Wait-Fresh([scriptblock]$Cond, [double]$Seconds, [string]$What, [scriptblock]$Detail) {
  try { Wait-Until $Cond $Seconds $What 300 } catch { throw "$($_.Exception.Message); $(& $Detail)" }
}

function Get-NowMs { return [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() }

# Первый байт значения StartupApproved (0x02 — включено, 0x03 — выключено); -1 — значения нет.
function Get-ApprovedByte([string]$Key, [string]$Name) {
  $v = (Get-ItemProperty -LiteralPath $Key -Name $Name -ErrorAction SilentlyContinue).$Name
  if ($null -eq $v -or @($v).Count -lt 1) { return -1 }
  return [int]@($v)[0]
}

function Assert-RunValue([string]$Key, [string]$Name, [string]$Path) {
  $v = [string](Get-ItemProperty -LiteralPath $Key -Name $Name -ErrorAction SilentlyContinue).$Name
  Assert-That ($v -eq "`"$Path`"") "Значение $Name в ${Key}: «$v», ждали «`"$Path`"» (служба не удаляет запись, RM5b-2)"
}

function Get-ViolationCount([int64]$ChildId) {
  $r = Send-Parent "ListEvents" @{ childId = $ChildId; group = "violations"; limit = 200 }
  Assert-Reply $r "ListEvents violations"
  return @(List $r.payload.events).Count
}

function Find-StartupEvents([string]$Kind, [string]$Name) {
  return @(Get-Events | Where-Object { [string]$_.kind -eq $Kind -and ([string]$_.text).Contains("«$Name»") })
}

# Плашки о закрытой программе нет Seconds секунд (PL-16: отсутствие проверяется после события).
function Assert-NoBlockedPlate([string]$What, [double]$Seconds = 3) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
    $g = Get-GuardState
    if ($script:BlockedKinds -contains [string]$g.plateKind) {
      throw "${What}: ребёнку показана плашка о закрытой программе: $(Format-GuardState $g); журнал службы: $(Get-StartupLog)"
    }
    Start-Sleep -Milliseconds 150
  }
}

# «Проводник запустил при входе»: простой ввода ≥ 10 с, вход ребёнка, запуск Path в первые 60 с после входа.
function Invoke-LogonLaunch([string]$Path, [string]$What) {
  Stop-Input
  Wait-Until { [PcbNative]::IdleMs() -ge 11000 } 40 "простой ввода 11 с (сейчас $([PcbNative]::IdleMs()) мс)"
  Send-Logon
  $since = [Diagnostics.Stopwatch]::StartNew()
  Start-Sleep -Seconds 1 # сверка сеансов после события входа
  $p = Start-TestApp $Path
  Write-Log "${What}: $(Split-Path -Leaf $Path) (PID $($p.Id)) через $([int]$since.Elapsed.TotalSeconds) с после входа, простой ввода $([PcbNative]::IdleMs()) мс"
  Assert-That ($since.Elapsed.TotalSeconds -lt 50) "${What}: запуск через $([int]$since.Elapsed.TotalSeconds) с после входа, ждали в первые 60 с"
  return $p
}

Invoke-Scenario "S50" "Автозагрузка: новая запись ждёт решения и не запускается, включение и выключение родителем, общая запись у двух детей" {
  Initialize-Scenario 60

  Step "Тестовые программы notes.exe и pcbshared.exe в папке наблюдения; общая запись PCBSharedTest в HKLM\…\Run" {
    $script:NotesPath = New-TestApp "notes"
    $script:SharedPath = New-TestApp "pcbshared"
    New-ItemProperty -LiteralPath $script:RunShared -Name $script:SharedName -Value "`"$($script:SharedPath)`"" -PropertyType String -Force | Out-Null
    Assert-RunValue $script:RunShared $script:SharedName $script:SharedPath
    Assert-That ((Get-ApprovedByte $script:ApprovedShared $script:SharedName) -eq -1) "В HKLM StartupApproved\Run уже есть $($script:SharedName)"
    Assert-That ($null -eq (Get-ItemProperty -LiteralPath $script:RunOwn -Name $script:NotesName -ErrorAction SilentlyContinue)) "$($script:NotesName) уже в Run ребёнка до первой проверки"
  }

  Step "Настройка: Миша (учётка раннера) и Коля (pcbtest), лимит 2 ч; notes и pcbshared — «Прочее»; часы на паузе" {
    $kolyaSid = New-TestUser
    $limits = @(1..7 | ForEach-Object { New-DayLimit $_ 120 })
    $r = Send-Pcb "Setup" @{ password = $script:ParentPassword; children = @(
        @{ sid = $script:MySid; name = "Миша"; limits = @($limits) },
        @{ sid = $kolyaSid; name = "Коля"; limits = @($limits) });
      programs = @(@{ path = $script:NotesPath; category = "other" }, @{ path = $script:SharedPath; category = "other" }) }
    Assert-Reply $r "Первичная настройка на двух детей"
    Suspend-Clock
    Wait-SessionParts
    Wait-ProgramsReady
    Use-Parent {
      Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы"
      foreach ($c in @(List (Get-Today).children)) {
        if ($c.name -eq "Миша") { $script:Misha = [int64]$c.childId }
        if ($c.name -eq "Коля") { $script:Kolya = [int64]$c.childId }
      }
    }
    Assert-That ($script:Misha -gt 0 -and $script:Kolya -gt 0 -and $script:Misha -ne $script:Kolya) "Дети не найдены: Миша $($script:Misha), Коля $($script:Kolya)"
    Write-Log "Миша — ребёнок $($script:Misha) (сеанс раннера), Коля — ребёнок $($script:Kolya) (pcbtest)"
  }

  Step "Первая проверка у обоих: PCBSharedTest в «Для всех пользователей», включена" {
    $since = Get-NowMs
    Use-Parent {
      foreach ($c in @(@{ id = $script:Misha; name = "Миша" }, @{ id = $script:Kolya; name = "Коля" })) {
        $l = Wait-StartupCheck $c.id $since $true "первая проверка, $($c.name)"
        $it = Find-StartupItem $l "shared" $script:SharedName
        Assert-That ($null -ne $it -and [string]$it.decision -eq "on") "У ребёнка $($c.name) $($script:SharedName) не «включена» в общих: $(Format-Startup $l)"
        Assert-That ($null -eq (Find-StartupItem $l "pending" $script:SharedName)) "У ребёнка $($c.name) $($script:SharedName) в «Ждут решения» после первой проверки: $(Format-Startup $l)"
      }
    }
  }

  Step "После первой проверки у Миши своя запись PCBNotesTest; вход: «Ждут решения», выключена штатно, значение на месте" {
    New-ItemProperty -LiteralPath $script:RunOwn -Name $script:NotesName -Value "`"$($script:NotesPath)`"" -PropertyType String -Force | Out-Null
    $since = Get-NowMs
    Send-Logon
    Use-Parent {
      $script:LastStartup = $null
      Wait-Fresh {
        $l = Get-Startup $script:Misha
        $script:LastStartup = $l
        ([int64]$l.checkedAt -ge $since) -and (-not $l.checking) -and ($null -ne (Find-StartupItem $l "pending" $script:NotesName))
      } 30 "$($script:NotesName) в «Ждут решения» после входа" { "последний ответ: $(if ($script:LastStartup) { Format-Startup $script:LastStartup } else { 'нет' }); журнал службы: $(Get-StartupLog)" }
      $l = $script:LastStartup
      Write-Log "Автозагрузка Миши после входа: $(Format-Startup $l)"
      $it = Find-StartupItem $l "pending" $script:NotesName
      Assert-That ([string]$it.scope -eq "own" -and [string]$it.source -eq "run" -and [string]$it.decision -eq "pending") "Строка $($script:NotesName): $($it.scope)/$($it.source)/$($it.decision), ждали own/run/pending"
      $script:NotesId = [int64]$it.id
      $sh = Find-StartupItem $l "shared" $script:SharedName
      Assert-That ($null -ne $sh -and [string]$sh.decision -eq "on") "После входа $($script:SharedName) не «включена»: $(Format-Startup $l)"
      $script:SharedIdMisha = [int64]$sh.id
      Wait-Fresh { (Get-ApprovedByte $script:ApprovedOwn $script:NotesName) -eq 3 } 15 "StartupApproved\Run\$($script:NotesName) первый байт 0x03" { "сейчас $(Get-ApprovedByte $script:ApprovedOwn $script:NotesName); журнал службы: $(Get-StartupLog)" }
      Assert-RunValue $script:RunOwn $script:NotesName $script:NotesPath
      Wait-Fresh { (Find-StartupEvents "startup_new" $script:NotesName).Count -ge 1 } 10 "запись журнала startup_new о «$($script:NotesName)»" { "журнал: $(Format-Events)" }
      $ev = @(Find-StartupEvents "startup_new" $script:NotesName)
      Write-Log "Журнал: $($ev[0].text)"
      Assert-That ($ev.Count -eq 1) "Записей startup_new о «$($script:NotesName)»: $($ev.Count), ждали 1"
      $script:Violations = Get-ViolationCount $script:Misha
    }
  }

  Step "Автозапуск notes при входе (ждёт решения): закрыт быстро, без плашки и нарушения; журнал startup_closed" {
    $p = Invoke-LogonLaunch $script:NotesPath "Автозапуск notes (ждёт решения)"
    [void](Assert-ClosedFast $p "Автозапуск notes (ждёт решения)")
    Assert-NoBlockedPlate "Автозапуск notes (ждёт решения)"
    Use-Parent {
      Wait-Fresh { (Find-StartupEvents "startup_closed" $script:NotesName).Count -ge 1 } 10 "запись журнала startup_closed о «$($script:NotesName)»" { "журнал: $(Format-Events); журнал службы: $(Get-StartupLog)" }
      Write-Log "Журнал: $((Find-StartupEvents 'startup_closed' $script:NotesName)[0].text)"
      $v = Get-ViolationCount $script:Misha
      Assert-That ($v -eq $script:Violations) "Нарушений у Миши $v, до автозапуска $($script:Violations): автозапуск записан нарушением ($(Format-Events))"
    }
  }

  Step "Родитель включает PCBNotesTest: «Будет запускаться при входе Миши», байт 0x02, значение на месте" {
    Use-Parent {
      $r = Send-Parent "SetStartup" @{ childId = $script:Misha; id = $script:NotesId; on = $true }
      Assert-Reply $r "SetStartup $($script:NotesName) on"
      Write-Log "SetStartup: «$($r.payload.message)», решение $($r.payload.item.decision)"
      Assert-That ([string]$r.payload.message -eq "Будет запускаться при входе Миши") "Сообщение «$($r.payload.message)», ждали «Будет запускаться при входе Миши»"
      Assert-That ([string]$r.payload.item.decision -eq "on") "Решение службы «$($r.payload.item.decision)», ждали on"
      $l = Get-Startup $script:Misha
      Assert-That ($null -eq (Find-StartupItem $l "pending" $script:NotesName) -and [string](Find-StartupItem $l "own" $script:NotesName).decision -eq "on") "После включения: $(Format-Startup $l), ждали $($script:NotesName) в своих с on"
      Assert-That ((Find-StartupEvents "startup_decided" $script:NotesName).Count -ge 1) "Нет записи журнала startup_decided о «$($script:NotesName)»: $(Format-Events)"
    }
    $b = Get-ApprovedByte $script:ApprovedOwn $script:NotesName
    Assert-That ($b -eq 2) "StartupApproved\Run\$($script:NotesName) первый байт $b, ждали 0x02 (включено)"
    Assert-RunValue $script:RunOwn $script:NotesName $script:NotesPath
  }

  Step "Автозапуск notes при следующем входе (включена): живёт" {
    $p = Invoke-LogonLaunch $script:NotesPath "Автозапуск notes (включена)"
    Start-Sleep -Seconds 4
    Assert-AppAlive $p "Автозапуск notes (включена)"
    Assert-NoBlockedPlate "Автозапуск notes (включена)" 1
    Write-Log "notes.exe после входа живёт (PID $($p.Id))"
    Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
  }

  Step "Родитель выключает PCBSharedTest Мише: «Не будет запускаться при входе Миши», HKLM не тронут" {
    Use-Parent {
      $r = Send-Parent "SetStartup" @{ childId = $script:Misha; id = $script:SharedIdMisha; on = $false }
      Assert-Reply $r "SetStartup $($script:SharedName) off"
      Write-Log "SetStartup: «$($r.payload.message)», решение $($r.payload.item.decision)"
      Assert-That ([string]$r.payload.message -eq "Не будет запускаться при входе Миши") "Сообщение «$($r.payload.message)», ждали «Не будет запускаться при входе Миши»"
      Assert-That ([string]$r.payload.item.decision -eq "off") "Решение службы «$($r.payload.item.decision)», ждали off"
      $script:Violations = Get-ViolationCount $script:Misha
    }
    Assert-RunValue $script:RunShared $script:SharedName $script:SharedPath
    $b = Get-ApprovedByte $script:ApprovedShared $script:SharedName
    Assert-That ($b -eq -1) "HKLM StartupApproved\Run\$($script:SharedName): первый байт $b, ждали «значения нет» (общую запись служба в Windows не выключает, уточнение 1)"
  }

  Step "Автозапуск pcbshared при входе Миши: закрыт без плашки и нарушения; журнал startup_closed" {
    $p = Invoke-LogonLaunch $script:SharedPath "Автозапуск pcbshared (выключена Мише)"
    [void](Assert-ClosedFast $p "Автозапуск pcbshared (выключена Мише)")
    Assert-NoBlockedPlate "Автозапуск pcbshared (выключена Мише)"
    Use-Parent {
      Wait-Fresh { (Find-StartupEvents "startup_closed" $script:SharedName).Count -ge 1 } 10 "запись журнала startup_closed о «$($script:SharedName)»" { "журнал: $(Format-Events); журнал службы: $(Get-StartupLog)" }
      Write-Log "Журнал: $((Find-StartupEvents 'startup_closed' $script:SharedName)[0].text)"
      $v = Get-ViolationCount $script:Misha
      Assert-That ($v -eq $script:Violations) "Нарушений у Миши $v, до автозапуска $($script:Violations): автозапуск записан нарушением ($(Format-Events))"
    }
  }

  Step "У Коли PCBSharedTest по-прежнему включена (Ruling T6-1)" {
    $since = Get-NowMs
    Use-Parent {
      $l = Wait-StartupCheck $script:Kolya $since $true "Коля после выключения у Миши"
      $it = Find-StartupItem $l "shared" $script:SharedName
      Assert-That ($null -ne $it -and [string]$it.decision -eq "on") "У Коли $($script:SharedName) не «включена»: $(Format-Startup $l)"
      $m = Get-Startup $script:Misha
      Assert-That ([string](Find-StartupItem $m "shared" $script:SharedName).decision -eq "off") "У Миши $($script:SharedName) не «выключена»: $(Format-Startup $m)"
    }
  }

  Write-Log "S50: новая запись ждёт решения и закрывается при входе, включённая живёт, общая выключена только Мише, записи в реестре на месте"
}
