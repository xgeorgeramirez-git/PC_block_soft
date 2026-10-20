# Приёмка M1 на настоящей Windows: сценарии S1...S7 и S25...S27 по очереди, каждый в своём процессе
# на свежей установке и свежей папке данных (счётчик сбоев службы, Ruling 27). Падение одного не мешает
# остальным: прогоняются все, код выхода 1 при любом падении. Снимки в папке accept-screens (PCB_ACCEPT_SHOTS).
# S28 (подсказки и обучение) браузерный: frontend/e2e/s28-helper.spec.ts, идёт в ci.yml.
param(
  [string]$InstallerDir = "build/bin",
  [string]$LogPath = "",
  [string[]]$Only = @()
)
$ErrorActionPreference = "Continue"
if ($LogPath) { $env:PCB_ACCEPT_LOG = $LogPath }
$env:PCB_INSTALLER_DIR = (Resolve-Path -LiteralPath $InstallerDir).Path
$ps = (Get-Process -Id $PID).Path

$all = @(
  @{ Id = "S1";  File = "s01-install.ps1" },
  @{ Id = "S2";  File = "s02-limit.ps1" },
  @{ Id = "S3";  File = "s03-extra.ps1" },
  @{ Id = "S4";  File = "s04-wrong-password.ps1" },
  @{ Id = "S5";  File = "s05-tray-killed.ps1" },
  @{ Id = "S6";  File = "s06-inactivity.ps1" },
  @{ Id = "S7";  File = "s07-parent-account.ps1" },
  @{ Id = "S25"; File = "s25-reset.ps1" },
  @{ Id = "S26"; File = "s26-uninstall.ps1" },
  @{ Id = "S27"; File = "s27-power-loss.ps1" }
)
if ($Only.Count -gt 0) { $all = @($all | Where-Object { $Only -contains $_.Id }) }

$results = [ordered]@{}
foreach ($s in $all) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  Write-Host "::group::$($s.Id) $($s.File)"
  & $ps -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot $s.File)
  $code = $LASTEXITCODE
  Write-Host "::endgroup::"
  $results[$s.Id] = $code
  $line = "{0}: {1} ({2:N0} с)" -f $s.Id, $(if ($code -eq 0) { "PASS" } else { "FAIL, код $code" }), $sw.Elapsed.TotalSeconds
  Write-Host $line
  $logFile = if ($env:PCB_ACCEPT_LOG) { $env:PCB_ACCEPT_LOG } else { Join-Path $(if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [IO.Path]::GetTempPath() }) "ci.log" }
  Add-Content -LiteralPath $logFile -Value ("{0:HH:mm:ss} [итог] {1}" -f (Get-Date), $line) -Encoding UTF8
}

$failed = @($results.GetEnumerator() | Where-Object { $_.Value -ne 0 } | ForEach-Object { $_.Key })
$passed = @($results.GetEnumerator() | Where-Object { $_.Value -eq 0 } | ForEach-Object { $_.Key })
if ($failed.Count -gt 0) {
  Write-Host "::error title=Приёмка M1::не пройдены: $($failed -join ', '); пройдены: $($passed -join ', ')"
  exit 1
}
if ($Only.Count -gt 0) {
  Write-Host "::notice title=Приёмка M1::$($passed -join ', '): PASS (выборочный прогон)"
} else {
  Write-Host "::notice title=Приёмка M1::S1...S7, S25...S27: PASS"
}
exit 0
