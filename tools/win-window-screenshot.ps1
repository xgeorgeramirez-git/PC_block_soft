# Запускает программу, ждёт её окно, снимает окно в PNG и закрывает программу.
# Только для CI на windows-latest (pwsh). Код выхода 1, если окно не появилось или программа упала.
param(
  [Parameter(Mandatory = $true)][string]$ExePath,
  [Parameter(Mandatory = $true)][string]$Out,
  [string]$Title = "PC Block",
  [int]$WaitSeconds = 30
)
$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public static class Win {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
  [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr hwnd, int attr, out RECT rect, int size);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
}
"@
[Win]::SetProcessDPIAware() | Out-Null

function Save-Region([int]$x, [int]$y, [int]$w, [int]$h, [string]$path) {
  $bmp = New-Object System.Drawing.Bitmap $w, $h
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($x, $y, 0, 0, (New-Object System.Drawing.Size $w, $h))
  $bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
  $g.Dispose(); $bmp.Dispose()
}

function Save-FullScreen([string]$path) {
  Add-Type -AssemblyName System.Windows.Forms
  $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  Save-Region $b.X $b.Y $b.Width $b.Height $path
}

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Out) | Out-Null
$proc = Start-Process -FilePath $ExePath -PassThru
$deadline = (Get-Date).AddSeconds($WaitSeconds)
$found = $false
while ((Get-Date) -lt $deadline) {
  Start-Sleep -Milliseconds 500
  $proc.Refresh()
  if ($proc.HasExited) {
    Write-Host "::error::Программа завершилась сама, код $($proc.ExitCode)"
    Save-FullScreen ($Out -replace '\.png$', '-fail.png')
    exit 1
  }
  if ($proc.MainWindowHandle -ne [IntPtr]::Zero -and $proc.MainWindowTitle -eq $Title) { $found = $true; break }
}
if (-not $found) {
  Write-Host "::error::Окно «$Title» не появилось за $WaitSeconds с (заголовок: '$($proc.MainWindowTitle)')"
  Save-FullScreen ($Out -replace '\.png$', '-fail.png')
  Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
  exit 1
}

# Даём WebView2 дорисовать страницу.
Start-Sleep -Seconds 4
$h = $proc.MainWindowHandle
[Win]::ShowWindow($h, 9) | Out-Null
[Win]::SetForegroundWindow($h) | Out-Null
Start-Sleep -Milliseconds 800
$r = New-Object Win+RECT
[Win]::DwmGetWindowAttribute($h, 9, [ref]$r, [System.Runtime.InteropServices.Marshal]::SizeOf($r)) | Out-Null
Save-Region $r.Left $r.Top ($r.Right - $r.Left) ($r.Bottom - $r.Top) $Out
Write-Host "Снимок окна: $Out ($($r.Right - $r.Left)x$($r.Bottom - $r.Top))"

$proc.Refresh()
if ($proc.HasExited) {
  Write-Host "::error::Программа упала после показа окна"
  exit 1
}
Stop-Process -Id $proc.Id -Force
exit 0
