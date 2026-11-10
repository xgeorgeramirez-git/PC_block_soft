# Запускает программу, ждёт её окно, снимает окно в PNG и закрывает программу.
# Только для CI на windows-latest (pwsh). Код выхода 1, если окно не появилось или программа упала.
#
# Ожидание по этапам (B-47): Wails создаёт главное окно скрытым и показывает его только после
# первой загрузки страницы в WebView2, а на медленном раннере холодный WebView2 грузится долго.
# Поэтому сначала ждём, что окно процесса создано (скрытое тоже) — до CreateSeconds, затем, что
# оно показано — до ShowSeconds от запуска. Падение называет этап, который не наступил.
param(
  [Parameter(Mandatory = $true)][string]$ExePath,
  [Parameter(Mandatory = $true)][string]$Out,
  [string]$Title = "PC Block",
  [int]$CreateSeconds = 30,
  [int]$ShowSeconds = 120
)
$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;
public static class Win {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
  delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc cb, IntPtr l);
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder sb, int max);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
  [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr hwnd, int attr, out RECT rect, int size);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();

  static string TextOf(IntPtr h) {
    var sb = new StringBuilder(512);
    GetWindowText(h, sb, sb.Capacity);
    return sb.ToString();
  }

  // Заголовки всех верхних окон процесса, видимых и скрытых (для сообщения падения).
  public static string[] Titles(uint procId) {
    var list = new List<string>();
    EnumWindows((h, l) => {
      uint pid; GetWindowThreadProcessId(h, out pid);
      if (pid == procId) list.Add(TextOf(h));
      return true;
    }, IntPtr.Zero);
    return list.ToArray();
  }

  // Верхнее окно процесса с точным заголовком, видимое или нет; IntPtr.Zero — такого нет.
  public static IntPtr FindProcWindow(uint procId, string title) {
    IntPtr found = IntPtr.Zero;
    EnumWindows((h, l) => {
      uint pid; GetWindowThreadProcessId(h, out pid);
      if (pid == procId && TextOf(h) == title) { found = h; return false; }
      return true;
    }, IntPtr.Zero);
    return found;
  }
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

# Сбой этапа: строка ошибки, снимок экрана, программа закрывается.
function Fail-Stage([string]$message) {
  Write-Host "::error::$message"
  Save-FullScreen ($Out -replace '\.png$', '-fail.png')
  Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
  exit 1
}

# Программа завершилась сама — на любом этапе ожидания.
function Assert-Alive {
  $proc.Refresh()
  if ($proc.HasExited) {
    Write-Host "::error::Программа завершилась сама, код $($proc.ExitCode)"
    Save-FullScreen ($Out -replace '\.png$', '-fail.png')
    exit 1
  }
}

# Сколько процессов WebView2 запустила программа; диагностика не должна сорвать сообщение падения.
function Get-WebViewCount {
  try {
    @(Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" |
      Where-Object { $_.ParentProcessId -eq $proc.Id }).Count
  } catch {
    "не прочитано ($($_.Exception.Message))"
  }
}

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Out) | Out-Null
$proc = Start-Process -FilePath $ExePath -PassThru
$started = Get-Date
$elapsed = { [math]::Round(((Get-Date) - $started).TotalSeconds, 1) }

# Этап 1: окно процесса создано (Wails создаёт его скрытым).
$h = [IntPtr]::Zero
while ($h -eq [IntPtr]::Zero) {
  Start-Sleep -Milliseconds 500
  Assert-Alive
  $h = [Win]::FindProcWindow([uint32]$proc.Id, $Title)
  if ($h -eq [IntPtr]::Zero -and (& $elapsed) -ge $CreateSeconds) {
    $titles = [Win]::Titles([uint32]$proc.Id)
    $list = ($titles | ForEach-Object { "'$_'" }) -join ', '
    Fail-Stage "Окно «$Title» не создано за $CreateSeconds с: процесс жив, окон у процесса $($titles.Count) (заголовки: $list)"
  }
}
$createdAt = & $elapsed

# Этап 2: окно показано (Wails показывает его после загрузки страницы в WebView2).
while (-not [Win]::IsWindowVisible($h)) {
  Start-Sleep -Milliseconds 500
  Assert-Alive
  if ((& $elapsed) -ge $ShowSeconds) {
    Fail-Stage "Окно «$Title» создано за $createdAt с, но не показано за $ShowSeconds с: Wails показывает окно после загрузки страницы в WebView2; процессов msedgewebview2.exe у программы: $(Get-WebViewCount)"
  }
}
$shownAt = & $elapsed
Write-Host "::notice::Окно создано за $createdAt с, показано за $shownAt с"

# Даём WebView2 дорисовать страницу.
Start-Sleep -Seconds 4
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
