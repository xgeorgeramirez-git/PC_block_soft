# S46. Приложение магазина — одна строка на все версии пакета (D-66, FR-20, RM4-11, T10-12, PL-16, PL-22, PL-41).
# Настоящая WindowsApps защищена TrustedInstaller, поэтому в тестовом режиме её заменяет TestMode\StoreRoot — папка
# WindowsApps внутри WatchDir с поддельными папками версий «<имя>_<версия>_<арх>_<ресурс>_<издатель>» и
# AppxManifest.xml (без <Applications>: снимок программ при настройке сам строк не добавляет). exe в папках — копии
# системной программы (Copy-TestExe). Повторы «до обновления» (база 0.3.3: строка на каждую версию) подкладывает
# тестовая команда TestSeedPrograms; слияние идёт только при старте службы — его делает Restart-PcbService
# (перезапуск без перезаписи TestMode: часы ядра продолжаются, пауза восстанавливается).
# Дано: три строки msedge_proxy из папок Microsoft.MicrosoftEdge.Stable_128/129/130…, v129 скрыта родителем, v130
# ждёт решения; учёт 600 + 300 + 120 с. Когда: служба перезапущена (обновилась до M4). Тогда: одна строка с ключом
# без версии, издатель «Microsoft Corporation», скрыта, «Прочее», в «Ждут решения» нет. Затем Edge «обновился» в
# папку v131 и msedge_proxy запустилась оттуда: работает (скрытую не закрывает ни одно правило), строка та же.
# После «Показать» время за неделю — сумма по всем версиям, 1020 с. Издатель из манифеста доказывает второй
# пакет Contoso.Notes («Contoso Ltd»): у пакетов Microsoft манифест неотличим от запасного «Microsoft Corporation».
# Первый прогон — only=S46 одним (PL-41: Restart-PcbService и TestSeedPrograms исполняются здесь впервые).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Версии и имена: internal/programs/path.go StableKey (папка версии из 5 частей через «_», ключ «имя__издатель»);
# издатель: internal/service/winhost/storemeta.go (манифест, иначе «Microsoft Corporation» для 8wekyb3d8bbwe);
# строка журнала работы о слиянии: internal/service/programs.go mergeStorePrograms.
$script:V = @("128.0.2739.42", "129.0.2792.52", "130.0.2849.46")
$script:VNew = "131.0.2903.51"
$script:EdgePkg = "Microsoft.MicrosoftEdge.Stable"
$script:EdgePub = "8wekyb3d8bbwe"
$script:MsPublisher = "Microsoft Corporation"
$script:ContosoDir = "Contoso.Notes_1.0.0.0_x64__abcd1234efgh"
$script:ContosoPublisher = "Contoso Ltd"
$script:MergedLine = "Повторы приложений магазина слиты"
$script:DecidedAt = [int64]1760000000000

# AppxManifest.xml пакета без <Applications> (UTF-8 без BOM, как пишет Windows).
function New-StoreManifest([string]$Dir, [string]$DisplayName, [string]$Publisher) {
  $xml = '<?xml version="1.0" encoding="utf-8"?><Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"><Properties><DisplayName>' + $DisplayName + '</DisplayName><PublisherDisplayName>' + $Publisher + '</PublisherDisplayName></Properties></Package>'
  [IO.File]::WriteAllText((Join-Path $Dir "AppxManifest.xml"), $xml, [Text.UTF8Encoding]::new($false))
}

# Папка версии Edge с манифестом и msedge_proxy.exe; возвращает длинный путь exe.
function New-EdgeVersion([string]$Version) {
  $dir = Join-Path $script:Store "$($script:EdgePkg)_$($Version)_neutral__$($script:EdgePub)"
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  New-StoreManifest $dir "Microsoft Edge" $script:MsPublisher
  $exe = Copy-TestExe (Get-TestAppSource) $dir "msedge_proxy"
  Write-Log "Папка версии $Version`: $exe"
  return $exe
}

# Строки «Программ» и «Ждут решения» про msedge_proxy из папок Edge (версии и ключ).
function Select-EdgeRows($Rows) {
  return @(@(List $Rows) | Where-Object { $p = ([string]$_.path); $p.Contains("\microsoft.microsoftedge.stable_") -and $p.EndsWith("\msedge_proxy.exe") })
}

function Format-Rows($Rows) {
  return ((@(List $Rows) | ForEach-Object { "$($_.path) [$($_.category)/$($_.source), издатель «$($_.publisher)», скрыта $($_.hidden), за неделю $($_.weekS) с]" }) -join ' ; ')
}

# Число строк журнала работы службы с текстом Text (service.log в папке данных).
function Get-ServiceLogCount([string]$Text) {
  $log = Join-Path (Join-Path $script:Data "logs") "service.log"
  if (-not (Test-Path -LiteralPath $log)) { return 0 }
  return @(Get-Content -LiteralPath $log -Encoding UTF8 -ErrorAction SilentlyContinue | Where-Object { $_.Contains($Text) }).Count
}

Invoke-Scenario "S46" "Приложение магазина: одна строка msedge_proxy на все версии, решение и издатель сохранились, время — сумма" {
  Initialize-Scenario 60

  Step "Дано: папки трёх версий Edge и Contoso.Notes в TestMode\StoreRoot, служба перечитала TestMode" {
    $script:Store = Join-Path $script:Apps "WindowsApps"
    New-Item -ItemType Directory -Force -Path $script:Store | Out-Null
    $script:Store = [PcbNative]::LongPath((Resolve-Path -LiteralPath $script:Store).Path)
    $script:EdgeExe = @{}
    foreach ($v in $script:V) { $script:EdgeExe[$v] = New-EdgeVersion $v }
    $cdir = Join-Path $script:Store $script:ContosoDir
    New-Item -ItemType Directory -Force -Path $cdir | Out-Null
    New-StoreManifest $cdir "Contoso Notes" $script:ContosoPublisher
    $script:ContosoExe = Copy-TestExe (Get-TestAppSource) $cdir "notes"
    $low = $script:Store.ToLowerInvariant()
    $script:EdgeKey = "$low\$($script:EdgePkg.ToLowerInvariant())__$($script:EdgePub)\msedge_proxy.exe"
    $script:ContosoKey = "$low\contoso.notes__abcd1234efgh\notes.exe"
    New-ItemProperty -Path $script:TestKey -Name StoreRoot -Value $script:Store -PropertyType String -Force | Out-Null
    Write-Log "StoreRoot: $($script:Store); ключ Edge $($script:EdgeKey); ключ Contoso $($script:ContosoKey)"
    Restart-PcbService
    Assert-That (-not (Send-Pcb "Hello").payload.configured) "После перезапуска программа уже настроена, ждали чистую базу"
  }

  Step "Настройка: лимит 2 ч, ввода нет; служба приняла StoreRoot" {
    Start-Child 120 $false
    Wait-ProgramsReady
    $bad = Get-TestModeLog
    Assert-That ($bad -notmatch 'StoreRoot должен быть') "Служба не приняла StoreRoot: $bad"
    Write-Log "Журнал работы про тестовый режим: $bad"
    $list = Get-ProgramsList
    $edge = @(Select-EdgeRows $list.programs) + @(Select-EdgeRows $list.pending)
    Assert-That ($edge.Count -eq 0) "Снимок при настройке сам добавил строки msedge_proxy (манифесты без <Applications>): $(Format-Rows $edge)"
    $script:ChildId = Use-Parent { Get-ChildId }
  }

  Step "До обновления: три строки msedge_proxy (TestSeedPrograms), v129 скрыта, v130 ждёт решения" {
    $p = $script:EdgeExe
    # Издатель у v128 — как его записал снимок 0.3.3 (CompanyName exe); у v129 и v130 пусто: слияние берёт непустой.
    $req = @{
      programs = @(
        @{ path = $p[$script:V[0]]; name = "msedge_proxy"; publisher = $script:MsPublisher; category = "other"; source = "snapshot"; hidden = $false; decidedAt = 0 },
        @{ path = $p[$script:V[1]]; name = "msedge_proxy"; publisher = ""; category = "other"; source = "parent"; hidden = $true; decidedAt = $script:DecidedAt },
        @{ path = $p[$script:V[2]]; name = "msedge_proxy"; publisher = ""; category = "other"; source = "snapshot"; hidden = $false; decidedAt = 0 }
      )
      usage = @(
        @{ childId = $script:ChildId; day = "2026-10-16"; path = $p[$script:V[0]]; category = "other"; seconds = 600 },
        @{ childId = $script:ChildId; day = "2026-10-17"; path = $p[$script:V[1]]; category = "other"; seconds = 300 },
        @{ childId = $script:ChildId; day = "2026-10-18"; path = $p[$script:V[2]]; category = "other"; seconds = 120 }
      )
      pending = @(
        @{ path = $p[$script:V[2]]; name = "msedge_proxy"; publisher = ""; childId = $script:ChildId; attempts = 2; lastAttemptAt = $script:DecidedAt }
      )
    }
    Assert-Reply (Send-Pcb "TestSeedPrograms" $req) "TestSeedPrograms"
    $list = Get-ProgramsList
    $rows = @(Select-EdgeRows $list.programs)
    $pend = @(Select-EdgeRows $list.pending)
    Write-Log "До обновления: программы $(Format-Rows $rows); ждут решения $(Format-Rows $pend)"
    Assert-That ($rows.Count -eq 3) "Строк msedge_proxy до обновления: $($rows.Count), ждали 3 (по версиям): $(Format-Rows $rows)"
    $hid = @($rows | Where-Object { $_.hidden -eq $true })
    Assert-That ($hid.Count -eq 1 -and ([string]$hid[0].path).Contains("_$($script:V[1])_")) "Скрытых строк до обновления: $(Format-Rows $hid), ждали одну v$($script:V[1])"
    Assert-That ($pend.Count -eq 1) "В «Ждут решения» до обновления строк msedge_proxy: $($pend.Count), ждали 1 (v$($script:V[2]))"
  }

  Step "Служба обновилась до M4 (перезапуск): одна строка с ключом без версии, решение и издатель сохранились" {
    $n0 = Get-ServiceLogCount $script:MergedLine
    Restart-PcbService
    Wait-Until { (Get-ServiceLogCount $script:MergedLine) -gt $n0 } 30 "строка «$($script:MergedLine)» в service.log после перезапуска (было $n0)"
    $log = Join-Path (Join-Path $script:Data "logs") "service.log"
    $line = @(Get-Content -LiteralPath $log -Encoding UTF8 | Where-Object { $_.Contains($script:MergedLine) }) | Select-Object -Last 1
    Write-Log "service.log: $line"
    $list = Get-ProgramsList
    $rows = @(Select-EdgeRows $list.programs)
    $pend = @(Select-EdgeRows $list.pending)
    Write-Log "После слияния: программы $(Format-Rows $rows); ждут решения $(Format-Rows $pend)"
    Assert-That ($rows.Count -eq 1) "Строк msedge_proxy после слияния: $($rows.Count), ждали 1: $(Format-Rows $rows)"
    $r = $rows[0]
    Assert-That ([string]$r.path -eq $script:EdgeKey) "Путь строки «$($r.path)», ждали ключ без версии «$($script:EdgeKey)»"
    Assert-That ([string]$r.publisher -eq $script:MsPublisher) "Издатель «$($r.publisher)», ждали «$($script:MsPublisher)»"
    Assert-That ($r.hidden -eq $true) "Строка не скрыта: решение родителя (v$($script:V[1]) скрыта) потерялось при слиянии"
    Assert-That ([string]$r.category -eq "other") "Категория «$($r.category)», ждали other"
    Assert-That ($pend.Count -eq 0) "В «Ждут решения» после слияния: $(Format-Rows $pend), ждали пусто"
  }

  Step "Edge обновился в папку v$($script:VNew): msedge_proxy оттуда работает, строка та же" {
    $exe = New-EdgeVersion $script:VNew
    $proc = Start-TestApp $exe
    Start-Sleep -Seconds 3
    Assert-AppAlive $proc "msedge_proxy v$($script:VNew) (скрытая, «Прочее»)"
    $list = Get-ProgramsList
    $rows = @(Select-EdgeRows $list.programs)
    $pend = @(Select-EdgeRows $list.pending)
    Write-Log "После запуска v$($script:VNew): программы $(Format-Rows $rows); ждут решения $(Format-Rows $pend)"
    Assert-That ($rows.Count -eq 1 -and [string]$rows[0].path -eq $script:EdgeKey -and $rows[0].hidden -eq $true) "После запуска из v$($script:VNew) строки msedge_proxy: $(Format-Rows $rows), ждали одну скрытую $($script:EdgeKey)"
    Assert-That ($pend.Count -eq 0) "После запуска из v$($script:VNew) в «Ждут решения»: $(Format-Rows $pend)"
  }

  Step "«Показать»: время за неделю — сумма по всем версиям (600 + 300 + 120 с)" {
    $rep = Use-Parent { Send-Parent "SetProgramsHidden" @{ paths = @($script:EdgeKey); hidden = $false } }
    Assert-Reply $rep "SetProgramsHidden hidden=false"
    Write-Log "SetProgramsHidden: «$($rep.payload.message)»"
    $rows = @(Select-EdgeRows (Get-ProgramsList).programs)
    Write-Log "После «Показать»: $(Format-Rows $rows)"
    Assert-That ($rows.Count -eq 1 -and $rows[0].hidden -eq $false) "После «Показать» строки msedge_proxy: $(Format-Rows $rows), ждали одну не скрытую"
    Assert-That ([int]$rows[0].weekS -eq 1020) "Время за неделю $($rows[0].weekS) с, ждали 1020 (600 + 300 + 120, учёт слит на ключ)"
  }

  Step "Издатель из манифеста: Contoso.Notes незнакомая, в «Ждут решения» «Contoso Ltd»" {
    $before = @(@(List (Get-ProgramsList).pending) | Where-Object { ([string]$_.path) -eq $script:ContosoKey })
    Assert-That ($before.Count -eq 0) "До запуска notes.exe уже в «Ждут решения» (PL-16): $(Format-Rows $before)"
    $proc = Start-TestApp $script:ContosoExe
    [void](Assert-ClosedFast $proc "notes.exe Contoso (незнакомая)")
    $row = @()
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($row.Count -eq 0 -and $sw.Elapsed.TotalSeconds -lt 5) {
      $list = Get-ProgramsList
      $row = @(@(List $list.pending) | Where-Object { ([string]$_.path) -eq $script:ContosoKey })
      if ($row.Count -eq 0) { Start-Sleep -Milliseconds 500 }
    }
    Assert-That ($row.Count -eq 1) "notes.exe нет в «Ждут решения» с ключом $($script:ContosoKey): $(Format-Rows $list.pending)"
    Write-Log "Contoso в «Ждут решения»: $(Format-Rows $row)"
    Assert-That ([string]$row[0].publisher -eq $script:ContosoPublisher) "Издатель «$($row[0].publisher)», ждали «$($script:ContosoPublisher)» из AppxManifest.xml (у копии exe CompanyName другой)"
  }

  Write-Log "S46: одна строка msedge_proxy на все версии (ключ без версии), скрыта, «Microsoft Corporation», за неделю 1020 с; издатель Contoso из манифеста"
}
