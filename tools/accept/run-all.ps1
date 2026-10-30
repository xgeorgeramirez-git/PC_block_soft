# Приёмка M1+M2+0.2.3...0.2.5+M3 на настоящей Windows: сценарии S1...S15, S25...S27, S34...S37, LE1, LE2 (выходы с экрана блокировки), FB1, FB5 и S33 по очереди, каждый в своём процессе
# на свежей установке и свежей папке данных (счётчик сбоев службы, Ruling 27). Падение одного не мешает
# остальным: прогоняются все, код выхода 1 при любом падении. Снимки в папке accept-screens (PCB_ACCEPT_SHOTS).
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
  @{ Id = "S8";  File = "s08-forbidden.ps1" },
  @{ Id = "S9";  File = "s09-ask-more.ps1" },
  @{ Id = "S10"; File = "s10-bonus.ps1" },
  @{ Id = "S11"; File = "s11-pause.ps1" },
  @{ Id = "S12"; File = "s12-games-limit.ps1" },
  @{ Id = "S13"; File = "s13-study.ps1" },
  @{ Id = "S14"; File = "s14-unknown.ps1" },
  @{ Id = "S15"; File = "s15-study-only.ps1" },
  @{ Id = "S36"; File = "s36-promo.ps1" },
  @{ Id = "S37"; File = "s37-parent-tools.ps1" },
  @{ Id = "S25"; File = "s25-reset.ps1" },
  @{ Id = "S26"; File = "s26-uninstall.ps1" },
  @{ Id = "S27"; File = "s27-power-loss.ps1" },
  @{ Id = "S34"; File = "s34-tray-countdown.ps1" },
  @{ Id = "S35"; File = "s35-warning-card.ps1" },
  @{ Id = "LE1"; File = "s-lockexits.ps1" },
  @{ Id = "LE2"; File = "s-lockexits-auto.ps1" },
  @{ Id = "FB1"; File = "fb1-end-session.ps1" },
  @{ Id = "FB5"; File = "fb5-parent-logoff.ps1" },
  @{ Id = "S33"; File = "s33-switch-user.ps1" }  # последним: после отключения сеанса у раннера нет экрана
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
  Write-Host "::notice title=Приёмка::S1...S15, S25...S27, S33...S37, LE1, LE2, FB1, FB5: PASS"
}
exit 0
