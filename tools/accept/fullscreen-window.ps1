# Окно без рамки на весь экран: имитация видео в полноэкранном режиме (сценарий S6).
# Запускается отдельным процессом (pwsh -STA) из common.ps1: Show-Fullscreen; закрывается завершением процесса.
# Ввод не имитирует: передний план берётся без нажатий клавиш, иначе сбросился бы простой ввода.
param([Parameter(Mandatory = $true)][string]$ReadyFile)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class PcbFg {
  [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] static extern bool BringWindowToTop(IntPtr h);
  [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr h, int cmd);
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] static extern bool AttachThreadInput(uint a, uint b, bool attach);
  [DllImport("user32.dll")] static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] static extern bool SystemParametersInfo(uint action, uint param, IntPtr value, uint flags);
  [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
  public static void Prepare() { SetProcessDPIAware(); SystemParametersInfo(0x2001, 0, IntPtr.Zero, 2); }
  public static bool Force(IntPtr h) {
    uint pid;
    IntPtr fg = GetForegroundWindow();
    uint fgThread = fg == IntPtr.Zero ? 0 : GetWindowThreadProcessId(fg, out pid);
    uint me = GetCurrentThreadId();
    bool attached = false;
    if (fgThread != 0 && fgThread != me) attached = AttachThreadInput(me, fgThread, true);
    ShowWindow(h, 5);
    BringWindowToTop(h);
    SetForegroundWindow(h);
    if (attached) AttachThreadInput(me, fgThread, false);
    return GetForegroundWindow() == h;
  }
}
'@
[PcbFg]::Prepare()

$bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$form = New-Object System.Windows.Forms.Form
$form.FormBorderStyle = [System.Windows.Forms.FormBorderStyle]::None
$form.StartPosition = [System.Windows.Forms.FormStartPosition]::Manual
$form.Bounds = $bounds
$form.TopMost = $true
$form.ShowInTaskbar = $false
$form.BackColor = [System.Drawing.Color]::Black
$label = New-Object System.Windows.Forms.Label
$label.Text = "Видео на весь экран (проверка учёта PC Block)"
$label.ForeColor = [System.Drawing.Color]::White
$label.AutoSize = $true
$label.Font = New-Object System.Drawing.Font("Segoe UI", 28)
$label.Location = New-Object System.Drawing.Point(80, 80)
$form.Controls.Add($label)

$form.Add_Shown({
  $form.Bounds = $bounds
  $form.Activate()
  [void][PcbFg]::Force($form.Handle)
  Set-Content -LiteralPath $ReadyFile -Value "ready" -Encoding ASCII
})
# Пока окно живёт, раз в секунду напоминаем системе, что оно переднее (ввод при этом не нужен).
$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 1000
$timer.Add_Tick({ [void][PcbFg]::Force($form.Handle) })
$timer.Start()
[System.Windows.Forms.Application]::Run($form)
