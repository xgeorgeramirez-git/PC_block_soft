# PROBE. Проба M4 T0 (RM4-1, PL-16, PL-22): страж от имени системы в сеансе ребёнка читает домен активной
# вкладки Brave через UI Automation. Ребёнок — пользователь раннера (Start-Child). Brave запускается от его имени
# на http://probe-site.test/ (в hosts раннера — 127.0.0.1, из сети ничего не грузится; шаг windows.yml «Brave для
# пробы и приёмки»), окно выводится на передний план, сценарий ждёт домен в GuardState.foregroundDomain (страж
# присылает его в тестовом режиме). Итог — факт ::notice title=Проба Brave от SYSTEM, его собирает оркестратор.
# Не дождались домена — FAIL с тем, что на переднем плане и что прислал страж.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$script:ProbeDomain = "probe-site.test"
# Find-Brave, Stop-Brave, Get-BraveWindow и профиль $script:BraveProfile: common.ps1 (общие помощники Brave, M4 T10).

function Write-ProbeFact([string]$Text) {
  Write-Host "::notice title=Проба Brave от SYSTEM::$Text"
  Write-Log "Проба Brave от SYSTEM: $Text"
}

Invoke-Scenario "PROBE" "Страж от имени системы читает домен вкладки Brave в сеансе ребёнка" {
  Initialize-Scenario 60

  Step "Brave установлен" {
    $script:BraveExe = Find-Brave
    Assert-That ($script:BraveExe -ne "") "brave.exe не найден: шаг windows.yml «Brave для пробы и приёмки» не поставил Brave"
    Write-Log "Brave: $($script:BraveExe), версия $((Get-Item -LiteralPath $script:BraveExe).VersionInfo.FileVersion)"
    Stop-Brave
  }

  Step "Ребёнок — пользователь раннера, лимит 2 ч" {
    Start-Child 120
  }

  Step "Brave от имени ребёнка на http://$($script:ProbeDomain)/" {
    New-Item -ItemType Directory -Force -Path $script:BraveProfile | Out-Null
    $braveArgs = @("--no-first-run", "--no-default-browser-check", "--user-data-dir=$($script:BraveProfile)", "--new-window", "http://$($script:ProbeDomain)/")
    $script:BraveProc = Start-Process -FilePath $script:BraveExe -ArgumentList $braveArgs -PassThru
    Wait-Until { $null -ne (Get-BraveWindow) } 20 "окно Brave"
    Write-Log "Окно Brave: $(Get-BraveWindow)"
  }

  Step "Окно Brave на переднем плане" {
    $pids = @(Get-Process -Name brave -ErrorAction SilentlyContinue | ForEach-Object { $_.Id })
    $fgOk = $false
    for ($i = 0; $i -lt 10 -and -not $fgOk; $i++) {
      $fg = [PcbNative]::Foreground()
      $fgPid = if ($fg) { [int](($fg -split '\|')[0]) } else { 0 }
      if ($pids -contains $fgPid) { $fgOk = $true; break }
      $ws = New-Object -ComObject WScript.Shell
      [void]$ws.AppActivate([int]$script:BraveProc.Id)
      Start-Sleep -Milliseconds 500
    }
    $script:FgDesc = [PcbNative]::Foreground()
    Write-Log "Передний план: $($script:FgDesc); Brave на переднем плане: $fgOk"
    $script:BraveFg = $fgOk
  }

  Step "Страж присылает домен вкладки Brave (GuardState.foregroundDomain)" {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $g = $null
    while ($sw.Elapsed.TotalSeconds -lt 15) {
      $g = Get-GuardState
      if ([string]$g.foregroundDomain -eq $script:ProbeDomain) { break }
      Start-Sleep -Milliseconds 200
    }
    $secs = "{0:N1}" -f $sw.Elapsed.TotalSeconds
    $dom = [string]$g.foregroundDomain
    $path = [string]$g.foregroundPath
    if ($dom -eq $script:ProbeDomain) {
      Write-ProbeFact "работает: страж прочитал домен «$dom» за $secs с; путь переднего плана в страже «$path»"
    } else {
      $fg = [PcbNative]::Foreground()
      Write-ProbeFact "не работает за 15 с: домен в страже «$dom», путь переднего плана в страже «$path»; Brave на переднем плане (по сценарию): $($script:BraveFg); передний план сейчас: $fg"
      Stop-Brave
      throw "За 15 с страж не прислал домен «$($script:ProbeDomain)»: $(Format-GuardState $g); домен «$dom»; передний план: $fg"
    }
  }

  Step "Brave закрыт" {
    Stop-Brave
  }

  Write-Log "PROBE: домен вкладки Brave прочитан стражем от имени системы"
}
