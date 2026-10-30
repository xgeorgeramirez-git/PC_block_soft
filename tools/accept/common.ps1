# Общие функции приёмки PC Block на настоящей Windows (план M1, задачи T9 и T16).
# Подключается точкой из smoke.ps1 и сценариев s01...s27: . "$PSScriptRoot\common.ps1".
# Только для CI на windows-latest (pwsh), только в тестовом режиме программы (ТЗ 10.8):
# блокировки идут на тестовой учётке раннера, данные лежат во временной папке.
#
# Что внутри: журнал, ожидание условий, установка и сброс машины, включение тестового режима,
# канал связи службы прямо из PowerShell (одна родительская сессия на процесс, R19),
# управление экраном блокировки (TestGuard, GuardState, TestClock), имитация ввода,
# снимки экрана, полноэкранное окно, обёртка сценария.
# Журнал дописывается в ci.log: ошибки видны пометками сборки (tools/annotate-errors.sh).
$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$script:ServiceName = "PCBlock"
$script:TestKey = "HKLM:\SOFTWARE\PCBlock\TestMode"
$script:SafeBootKeys = @(
  "HKLM:\SYSTEM\CurrentControlSet\Control\SafeBoot\Minimal\PCBlock",
  "HKLM:\SYSTEM\CurrentControlSet\Control\SafeBoot\Network\PCBlock"
)
$script:ParentPassword = "PcbAccept-2026"
$script:TestUserName = "pcbtest"
$script:TestUserPassword = "Pcb!Test-2026x"
$script:DefaultData = Join-Path $env:ProgramData "PCBlock"
$script:TempRoot = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [IO.Path]::GetTempPath() }
$script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$script:LogPath = if ($env:PCB_ACCEPT_LOG) { $env:PCB_ACCEPT_LOG } else { Join-Path $script:TempRoot "ci.log" }
$script:ShotDir = if ($env:PCB_ACCEPT_SHOTS) { $env:PCB_ACCEPT_SHOTS } else { Join-Path $script:RepoRoot "accept-screens" }
$script:InstallerDir = if ($env:PCB_INSTALLER_DIR) { $env:PCB_INSTALLER_DIR } else { Join-Path $script:RepoRoot "build\bin" }
$script:Work = Join-Path $script:TempRoot "pcb-accept"
$script:Data = Join-Path $script:TempRoot "pcb-data"
$script:Public = Join-Path $env:PUBLIC "pcb-accept"
# Папка тестовых программ (WatchDir, RM3-8): в тестовом режиме наблюдатель трогает только её.
$script:Apps = Join-Path $script:Work "apps"
# Папка пробного запуска копии системной программы: вне WatchDir, наблюдатель её не трогает.
$script:Probe = Join-Path $script:Work "probe"
$script:TestProcs = @()  # обычный массив: @() вокруг List[object] в pwsh 7 падает «Argument types do not match» (прогон 41008763410)
$script:TestAppSource = ""
$script:FgOverride = $false
# Закрытие программы (NFR-6: не позже 1 с): допуск на раннере 1500 мс, в журнал — фактическое время.
$script:CloseToleranceMs = 1500
$script:MySid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$script:MySession = (Get-Process -Id $PID).SessionId
$script:PsExe = (Get-Process -Id $PID).Path
$script:Tag = ""
$script:Exe = $null
$script:InstallDir = $null
$script:Pipe = $null
$script:Token = ""
$script:WaitElapsed = 0.0
New-Item -ItemType Directory -Force -Path $script:Work | Out-Null
New-Item -ItemType Directory -Force -Path $script:ShotDir | Out-Null
if (-not (Test-Path -LiteralPath $script:LogPath)) { New-Item -ItemType File -Force -Path $script:LogPath | Out-Null }

# --- Win32: окна, ввод, передний план -------------------------------------------------------------
if (-not ('PcbNative' -as [type])) {
  Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;

public static class PcbNative {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
  [StructLayout(LayoutKind.Sequential)] struct LASTINPUTINFO { public uint cbSize; public uint dwTime; }
  delegate bool EnumProc(IntPtr h, IntPtr l);

  [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc cb, IntPtr l);
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("dwmapi.dll")] static extern int DwmGetWindowAttribute(IntPtr hwnd, int attr, out RECT rect, int size);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr h, StringBuilder sb, int max);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder sb, int max);
  [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] static extern bool BringWindowToTop(IntPtr h);
  [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr h, int cmd);
  [DllImport("user32.dll")] static extern bool AttachThreadInput(uint a, uint b, bool attach);
  [DllImport("user32.dll")] static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] static extern bool GetLastInputInfo(ref LASTINPUTINFO info);
  [DllImport("user32.dll")] static extern void mouse_event(uint flags, int dx, int dy, uint data, UIntPtr extra);
  [DllImport("user32.dll")] static extern bool SystemParametersInfo(uint action, uint param, IntPtr value, uint flags);
  [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
  [DllImport("kernel32.dll")] static extern uint GetTickCount();
  [DllImport("kernel32.dll", CharSet = CharSet.Unicode)] static extern uint GetLongPathName(string s, StringBuilder b, uint n);

  // Длинный путь без 8.3-имён (RUNNER~1): так путь exe видит наблюдатель службы (RM3-8).
  public static string LongPath(string p) {
    StringBuilder b = new StringBuilder(1024);
    uint n = GetLongPathName(p, b, (uint)b.Capacity);
    return (n == 0 || n >= b.Capacity) ? p : b.ToString();
  }

  static Timer pinger;
  static int flip;

  public static void DpiAware() { try { SetProcessDPIAware(); } catch (Exception) { } }

  // Миллисекунды с последнего ввода в этом сеансе (то же, что читает страж).
  public static long IdleMs() {
    LASTINPUTINFO li = new LASTINPUTINFO();
    li.cbSize = (uint)Marshal.SizeOf(typeof(LASTINPUTINFO));
    if (!GetLastInputInfo(ref li)) return -1;
    return (long)(uint)(GetTickCount() - li.dwTime);
  }

  // Один "ввод": мышь на пиксель вправо или влево. Настоящее событие ввода сеанса, как от человека.
  public static void Ping() {
    flip = flip ^ 1;
    mouse_event(0x0001, flip == 1 ? 1 : -1, 0, 0, UIntPtr.Zero);
  }

  public static void StartPing(int intervalMs) {
    StopPing();
    pinger = new Timer(delegate(object state) { Ping(); }, null, 0, intervalMs);
  }

  public static void StopPing() {
    if (pinger != null) { pinger.Dispose(); pinger = null; }
  }

  // Разрешает любому процессу выводить своё окно на передний план без ввода (тайм-аут блокировки = 0).
  public static void AllowForeground() {
    SystemParametersInfo(0x2001, 0, IntPtr.Zero, 2);
  }

  public static bool ForceForeground(IntPtr h) {
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

  static string Describe(IntPtr h) {
    uint pid;
    GetWindowThreadProcessId(h, out pid);
    StringBuilder cls = new StringBuilder(128);
    GetClassName(h, cls, cls.Capacity);
    StringBuilder title = new StringBuilder(128);
    GetWindowText(h, title, title.Capacity);
    RECT r;
    GetWindowRect(h, out r);
    return pid + "|" + cls + "|" + r.Left + "," + r.Top + "," + r.Right + "," + r.Bottom + "|" + title;
  }

  // Окно переднего плана: "pid|класс|l,t,r,b|заголовок"; пусто, если окна нет.
  public static string Foreground() {
    IntPtr h = GetForegroundWindow();
    return h == IntPtr.Zero ? "" : Describe(h);
  }

  // Видимые окна верхнего уровня заданных процессов.
  public static string[] VisibleWindows(uint[] pids) {
    List<string> list = new List<string>();
    HashSet<uint> want = new HashSet<uint>(pids);
    EnumWindows(delegate(IntPtr h, IntPtr l) {
      uint pid;
      GetWindowThreadProcessId(h, out pid);
      if (want.Contains(pid) && IsWindowVisible(h)) list.Add(Describe(h));
      return true;
    }, IntPtr.Zero);
    return list.ToArray();
  }

  // Рамка окна без тени: l, t, r, b (как видит человек).
  public static int[] WindowFrame(IntPtr h) {
    RECT r;
    if (DwmGetWindowAttribute(h, 9, out r, Marshal.SizeOf(typeof(RECT))) != 0) GetWindowRect(h, out r);
    return new int[] { r.Left, r.Top, r.Right, r.Bottom };
  }
}
'@
}
[PcbNative]::DpiAware()
[PcbNative]::AllowForeground()

# --- Журнал и шаги ----------------------------------------------------------------------------------
function Write-Log([string]$Text) {
  $tag = if ($script:Tag) { "[$($script:Tag)] " } else { "" }
  $line = "{0:HH:mm:ss} {1}{2}" -f (Get-Date), $tag, $Text
  Write-Host $line
  Add-Content -LiteralPath $script:LogPath -Value $line -Encoding UTF8
}

function Write-Fail([string]$Text) { Write-Log "ERROR: $Text" }

function Step([string]$Name, [scriptblock]$Body) {
  Write-Log "== $Name"
  try {
    & $Body
  } catch {
    Write-Fail ("Шаг '" + $Name + "': " + $_.Exception.Message)
    throw
  }
  Write-Log "готово: $Name"
}

# Ждёт условие. Ничего не возвращает: сколько секунд ждали, лежит в $script:WaitElapsed.
function Wait-Until([scriptblock]$Cond, [double]$Seconds, [string]$What, [int]$PollMs = 200) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($true) {
    if (& $Cond) { $script:WaitElapsed = $sw.Elapsed.TotalSeconds; return }
    if ($sw.Elapsed.TotalSeconds -gt $Seconds) { throw "За $Seconds с не дождались: $What" }
    Start-Sleep -Milliseconds $PollMs
  }
}

function Assert-That([bool]$Cond, [string]$Message) {
  if (-not $Cond) { throw $Message }
}

# Одна строка аргументов для Start-Process: пробелы и кавычки экранируются так, как их разбирает Go.
function ConvertTo-ArgString([string[]]$List) {
  $parts = foreach ($a in $List) {
    if ($a -match '[\s"]') {
      $s = $a -replace '(\\*)"', '$1$1\"'
      $s = $s -replace '(\\+)$', '$1$1'
      '"' + $s + '"'
    } else {
      $a
    }
  }
  return ($parts -join ' ')
}

# --- Процессы и служба --------------------------------------------------------------------------------
# Запускает PCBlock.exe с ключами, ждёт, читает JSON из --out. Возвращает объект Code и Result.
function Invoke-Pcb([string[]]$ArgList, [string]$OutName) {
  $out = Join-Path $script:Work $OutName
  Remove-Item -LiteralPath $out -ErrorAction SilentlyContinue
  $all = @($ArgList) + @("--out=$out")
  $p = Start-Process -FilePath $script:Exe -ArgumentList (ConvertTo-ArgString $all) -Wait -PassThru -WindowStyle Hidden
  $res = $null
  $shown = "нет файла результата"
  if (Test-Path -LiteralPath $out) {
    $raw = Get-Content -LiteralPath $out -Raw -Encoding UTF8
    $res = $raw | ConvertFrom-Json
    $shown = ($raw -replace '\s+', ' ')
  }
  Write-Log ("{0} -> код {1}; {2}" -f $ArgList[0], $p.ExitCode, $shown)
  return [pscustomobject]@{ Code = $p.ExitCode; Result = $res }
}

function Test-Reply($r, [string]$What) {
  if ($r.Code -ne 0 -or -not $r.Result -or -not $r.Result.ok) {
    $why = if ($r.Result -and $r.Result.error) { $r.Result.error } else { "код выхода $($r.Code)" }
    throw "$What не удалось: $why"
  }
}

function Get-PcbProcs {
  Get-CimInstance Win32_Process -Filter "Name='PCBlock.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.ProcessId -ne $PID }
}

function Get-SessionProcs([string]$Flag) {
  Get-PcbProcs | Where-Object { $_.SessionId -eq $script:MySession -and $_.CommandLine -match [regex]::Escape($Flag) }
}

function Get-ServiceState {
  $s = Get-Service -Name $script:ServiceName -ErrorAction SilentlyContinue
  if ($s) { return [string]$s.Status } else { return "нет" }
}

function Get-ServicePid {
  $c = Get-CimInstance Win32_Service -Filter "Name='$($script:ServiceName)'" -ErrorAction SilentlyContinue
  if ($c) { return [int]$c.ProcessId } else { return 0 }
}

function Wait-SessionParts {
  Wait-Until { (@(Get-SessionProcs "--tray").Count -ge 1) -and (@(Get-SessionProcs "--guard").Count -ge 1) } 60 "значок и страж в сеансе раннера"
}

function Get-MemoryMb([int]$ProcessId) {
  $p = Get-Process -Id $ProcessId -ErrorAction Stop
  return [math]::Round($p.WorkingSet64 / 1MB, 1)
}

# Память процесса, МБ: общий рабочий набор, личный рабочий набор (столбец «Память» Диспетчера задач) и личные байты.
function Get-MemoryInfo([int]$ProcessId) {
  $w = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$ProcessId" -ErrorAction Stop | Select-Object -First 1
  if (-not $w) { throw "Нет счётчиков памяти процесса $ProcessId" }
  return [pscustomobject]@{
    Ws = [math]::Round($w.WorkingSet / 1MB, 1)
    Private = [math]::Round($w.WorkingSetPrivate / 1MB, 1)
    Bytes = [math]::Round($w.PrivateBytes / 1MB, 1)
  }
}

function Stop-AllPcb {
  try {
    Get-PcbProcs | ForEach-Object { & taskkill.exe /F /T /PID $_.ProcessId 2>&1 | Out-Null }
  } catch {}
}

# --- Канал связи службы из PowerShell -------------------------------------------------------------------
# Кадр: 4 байта длины (little-endian) и тело в JSON. Родительская сессия привязана к процессу, а не
# к соединению, поэтому один процесс PowerShell может войти и потом менять правила (R19).
function Close-PipeConn {
  if ($script:Pipe) { try { $script:Pipe.Dispose() } catch {} }
  $script:Pipe = $null
}

function Open-PipeConn {
  Close-PipeConn
  # Права канала узкие (без создания экземпляров): общий GENERIC_WRITE .NET для обычного пользователя не подошёл бы.
  $rights = [System.IO.Pipes.PipeAccessRights]'ReadData, WriteData, ReadExtendedAttributes, WriteExtendedAttributes, ReadAttributes, WriteAttributes, Synchronize'
  $p = New-Object System.IO.Pipes.NamedPipeClientStream('.', 'PCBlock', $rights, [System.IO.Pipes.PipeOptions]::None, [System.Security.Principal.TokenImpersonationLevel]::Identification, [System.IO.HandleInheritability]::None)
  $p.Connect(10000)
  $script:Pipe = $p
}

function Read-Exact([int]$Count, [int]$TimeoutMs) {
  $buf = New-Object byte[] $Count
  $got = 0
  while ($got -lt $Count) {
    $t = $script:Pipe.ReadAsync($buf, $got, $Count - $got)
    if (-not $t.Wait($TimeoutMs)) { throw "Служба не ответила за $([int]($TimeoutMs / 1000)) с" }
    $k = $t.Result
    if ($k -le 0) { throw "Служба закрыла канал связи" }
    $got += $k
  }
  return , $buf
}

# Отправляет команду и возвращает разобранный ответ (ok, error, field, payload). Отказ службы это
# обычный ответ с ok = $false; исключение только если связи нет.
function Send-Pcb([string]$Cmd, $Payload = $null, [string]$Token = "", [int]$TimeoutMs = 15000) {
  $msg = [ordered]@{ cmd = $Cmd }
  if ($Token) { $msg.token = $Token }
  if ($null -ne $Payload) { $msg.payload = $Payload }
  $json = $msg | ConvertTo-Json -Depth 10 -Compress
  $body = [Text.Encoding]::UTF8.GetBytes($json)
  $frame = New-Object byte[] (4 + $body.Length)
  [BitConverter]::GetBytes([uint32]$body.Length).CopyTo($frame, 0)
  [Array]::Copy($body, 0, $frame, 4, $body.Length)
  for ($attempt = 0; $attempt -lt 2; $attempt++) {
    if (-not $script:Pipe -or -not $script:Pipe.IsConnected) { Open-PipeConn }
    try {
      $script:Pipe.Write($frame, 0, $frame.Length)
      $script:Pipe.Flush()
      break
    } catch {
      Close-PipeConn
      if ($attempt -eq 1) { throw }
    }
  }
  try {
    $hdr = Read-Exact 4 $TimeoutMs
    $len = [int][BitConverter]::ToUInt32($hdr, 0)
    if ($len -le 0 -or $len -gt 1048576) { throw "Служба прислала кадр длиной $len" }
    $bytes = Read-Exact $len $TimeoutMs
  } catch {
    Close-PipeConn
    throw
  }
  return ([Text.Encoding]::UTF8.GetString($bytes) | ConvertFrom-Json)
}

function Assert-Reply($Reply, [string]$What) {
  if (-not $Reply.ok) { throw "$What не удалось: $($Reply.error)" }
}

function Get-Status {
  $r = Send-Pcb "GetChildStatus"
  Assert-Reply $r "GetChildStatus"
  return $r.payload
}

function Get-GuardState {
  $r = Send-Pcb "GuardState"
  Assert-Reply $r "GuardState"
  return $r.payload
}

# Часы ядра: пауза и продолжение (Ruling 4). Возвращает ответ с nowMs (часы ядра, UTC мс).
function Set-ClockPaused([bool]$Paused) {
  $r = Send-Pcb "TestClock" @{ paused = $Paused }
  Assert-Reply $r "TestClock"
  return $r.payload
}

function Get-CoreNowMs {
  $r = Send-Pcb "TestClock" @{ paused = $script:ClockPausedByUs }
  Assert-Reply $r "TestClock"
  return [int64]$r.payload.nowMs
}

$script:ClockPausedByUs = $false
$script:ClockSpeed = 1
function Suspend-Clock { $script:ClockPausedByUs = $true; [void](Set-ClockPaused $true) }
function Resume-Clock { $script:ClockPausedByUs = $false; [void](Set-ClockPaused $false) }

# Действие стражу через настоящие сообщения его окон (Ruling 5): click, type, wmClose, altF4, esc.
function Invoke-TestGuard([string]$Action, [string]$Target = "", [string]$Text = "") {
  $act = [ordered]@{ action = $Action }
  if ($Target) { $act.target = $Target }
  if ($Text) { $act.text = $Text }
  $r = Send-Pcb "TestGuard" @{ action = $act }
  Assert-Reply $r "TestGuard $Action $Target"
}

# Родительская сессия на этот процесс PowerShell.
function Open-Parent([string]$Password = $script:ParentPassword) {
  $r = Send-Pcb "Login" @{ password = $Password }
  Assert-Reply $r "Вход родителя"
  $script:Token = [string]$r.payload.token
  Assert-That ($script:Token -ne "") "Служба не вернула токен родительской сессии"
}

function Send-Parent([string]$Cmd, $Payload = $null) {
  return (Send-Pcb $Cmd $Payload $script:Token)
}

function Get-Events {
  $r = Send-Parent "ListEvents" @{ childId = 0; limit = 500 }
  Assert-Reply $r "ListEvents"
  return @($r.payload.events)
}

function Get-Today {
  $r = Send-Parent "GetToday"
  Assert-Reply $r "GetToday"
  return $r.payload
}

# Лимит одного дня для Setup и SaveLimits. GamesMin < 0: игры без отдельного лимита (как до M3);
# 0 и больше: лимит игр в минутах (R5, FR-16; 0 — игры в этот день нельзя).
function New-DayLimit([int]$Weekday, [int]$TotalMin, [int]$GamesMin = -1) {
  return @{ weekday = $Weekday; totalMin = $TotalMin; unlimited = $false; gamesMin = [Math]::Max($GamesMin, 0); gamesUnlimited = ($GamesMin -lt 0) }
}

# Первичная настройка по команде Setup (то же, что делает мастер): пароль, ребёнок, лимиты на семь дней.
# Programs — категории программ шага мастера «Программы» (FR-2): @(@{ path = ...; category = "games" }).
# DecideProgram принимает только программу из «Ждут решения» или списка, поэтому категорию тестовой
# программе до её первого запуска даёт мастер (PL-11: сначала настройка, потом запуск).
function Invoke-Setup([int]$LimitMin = 120, [string]$ChildSid = $script:MySid, [string]$Name = "Тест", $Limits = $null, [int]$GamesMin = -1, $Programs = $null) {
  if (-not $Limits) {
    $Limits = @(1..7 | ForEach-Object { New-DayLimit $_ $LimitMin $GamesMin })
  }
  $child = @{ sid = $ChildSid; name = $Name; limits = @($Limits) }
  $req = @{ password = $script:ParentPassword; children = @($child) }
  if ($Programs) { $req.programs = @($Programs) }
  $r = Send-Pcb "Setup" $req
  Assert-Reply $r "Первичная настройка"
  $games = if ($GamesMin -lt 0) { "игры без отдельного лимита" } else { "игры $GamesMin мин" }
  $progs = (@(List $Programs) | ForEach-Object { "$(Split-Path -Leaf $_.path)=$($_.category)" }) -join ", "
  Write-Log "Первичная настройка: ребёнок $Name, лимит $LimitMin мин на каждый день, $games; программы мастера: [$progs]"
}

# --- Установка, тестовый режим, сброс -------------------------------------------------------------------
function Install-Pcb {
  $inst = Get-ChildItem -LiteralPath $script:InstallerDir -Filter "*installer*.exe" -ErrorAction Stop | Select-Object -First 1
  if (-not $inst) { throw "Установщик не найден в $($script:InstallerDir)" }
  Start-Process -FilePath $inst.FullName -ArgumentList "/S" -Wait
  $exe = Join-Path (Join-Path $env:ProgramFiles "PCBlock") "PCBlock.exe"
  Wait-Until { Test-Path -LiteralPath $exe } 30 "PCBlock.exe в Program Files после установки"
  $script:Exe = $exe
  $script:InstallDir = Split-Path -Parent $exe
  Wait-Until { (Get-ServiceState) -eq "Running" } 40 "служба в состоянии Running после установки"
  Write-Log "Установлено: $exe"
}

# Включает тестовый режим: временная папка данных, ускорение, начало часов ядра в понедельник 19.10.2026
# в 15:00 (день недели не зависит от времени запуска CI), учётка раннера как "ребёнок" (ТЗ 10.8).
# ClockRealStart пишется сейчас: после перезапуска службы время ядра продолжается, а не идёт назад.
function Enable-TestMode([int]$Speed = 1, [string]$ChildSid = $script:MySid, [bool]$ExpectFresh = $true, [int]$LockAutoSwitchS = 0) {
  New-Item -Path $script:TestKey -Force | Out-Null
  $realStart = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
  New-ItemProperty -Path $script:TestKey -Name DataDir -Value $script:Data -PropertyType String -Force | Out-Null
  New-ItemProperty -Path $script:TestKey -Name ClockSpeed -Value $Speed -PropertyType DWord -Force | Out-Null
  New-ItemProperty -Path $script:TestKey -Name ClockStart -Value "2026-10-19T15:00:00" -PropertyType String -Force | Out-Null
  New-ItemProperty -Path $script:TestKey -Name ClockRealStart -Value $realStart -PropertyType String -Force | Out-Null
  New-ItemProperty -Path $script:TestKey -Name TestChildSID -Value $ChildSid -PropertyType String -Force | Out-Null
  # Папка тестовых программ (RM3-8): наблюдатель в тестовом режиме трогает только процессы внутри неё,
  # иначе он закрыл бы программы раннера. Путь длинный, как его видит служба.
  New-Item -ItemType Directory -Force -Path $script:Apps | Out-Null
  $script:Apps = [PcbNative]::LongPath((Resolve-Path -LiteralPath $script:Apps).Path)
  New-ItemProperty -Path $script:TestKey -Name WatchDir -Value $script:Apps -PropertyType String -Force | Out-Null
  # Срок автосмены пользователя на экране блокировки в секундах (1...900); 0: по умолчанию, 15 минут.
  if ($LockAutoSwitchS -gt 0) {
    New-ItemProperty -Path $script:TestKey -Name LockAutoSwitchS -Value $LockAutoSwitchS -PropertyType DWord -Force | Out-Null
  }
  $script:ClockSpeed = $Speed
  $script:ClockPausedByUs = $false
  Close-PipeConn
  Restart-Service -Name $script:ServiceName -Force
  Wait-Until { (Get-ServiceState) -eq "Running" } 40 "служба в состоянии Running после перезапуска"
  Wait-Until {
    try {
      $h = Send-Pcb "Hello"
      $h.ok -and $h.payload.testMode
    } catch { Close-PipeConn; $false }
  } 40 "служба отвечает в тестовом режиме"
  if ($ExpectFresh) {
    Assert-That (-not (Send-Pcb "Hello").payload.configured) "Программа уже настроена, ждали чистую базу"
  }
  Write-Log "Тестовый режим: ClockSpeed=$Speed, ClockStart=2026-10-19T15:00:00, данные $($script:Data), WatchDir $($script:Apps)"
}

# Приводит машину к чистому состоянию. Вызывается в начале и в конце сценария и ничего не бросает.
function Reset-Machine {
  Close-PipeConn
  Stop-TestApps
  $exe = Join-Path (Join-Path $env:ProgramFiles "PCBlock") "PCBlock.exe"
  try {
    if ((Get-ServiceState) -ne "нет" -and (Test-Path -LiteralPath $exe)) {
      Start-Process -FilePath $exe -ArgumentList (ConvertTo-ArgString @("--remove-service", "--silent", "--delete-data", "--out=$(Join-Path $script:Work 'remove-final.json')")) -Wait -WindowStyle Hidden
    }
  } catch {}
  Stop-AllPcb
  try { Remove-Item -Path $script:TestKey -Recurse -Force -ErrorAction SilentlyContinue } catch {}
  try { foreach ($k in $script:SafeBootKeys) { Remove-Item -Path $k -Recurse -Force -ErrorAction SilentlyContinue } } catch {}
  try {
    $un = Join-Path (Join-Path $env:ProgramFiles "PCBlock") "uninstall.exe"
    if (Test-Path -LiteralPath $un) {
      Start-Process -FilePath $un -ArgumentList "/S" -Wait
      # Деинсталлятор NSIS копирует себя во временную папку и завершается раньше удаления: ждём.
      for ($i = 0; $i -lt 60 -and (Test-Path -LiteralPath $exe); $i++) { Start-Sleep -Seconds 1 }
    }
  } catch {}
  Stop-AllPcb
  try { Remove-ItemProperty -Path "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Policies\System" -Name DisableTaskMgr -ErrorAction SilentlyContinue } catch {}
  try { Remove-Item -LiteralPath $script:Data -Recurse -Force -ErrorAction SilentlyContinue } catch {}
  try { Remove-Item -LiteralPath $script:DefaultData -Recurse -Force -ErrorAction SilentlyContinue } catch {}
  try { Remove-TestUser } catch {}
  foreach ($dir in @($script:Apps, $script:Probe)) {
    try { Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction SilentlyContinue } catch {}
  }
  $script:Exe = $null
  $script:InstallDir = $null
  $script:Token = ""
}

# Свежая установка и свежая папка данных (счётчик сбоев службы, Ruling 27), тестовый режим включён.
function Initialize-Scenario([int]$Speed = 1, [int]$LockAutoSwitchS = 0) {
  Reset-Machine
  Install-Pcb
  Enable-TestMode $Speed $script:MySid $true $LockAutoSwitchS
}

# Строки журнала службы про сеансы и решения «на экране или нет» (подробные логи, FR-25) и qwinsta.
function Write-ServiceSessionLog([string]$Tag) {
  try { (@(& qwinsta.exe 2>&1)) | ForEach-Object { Write-Log "[$Tag] qwinsta: $_" } } catch {}
  foreach ($dir in @($script:Data, $script:DefaultData)) {
    $sl = Join-Path (Join-Path $dir "logs") "service.log"
    if (Test-Path -LiteralPath $sl) {
      Get-Content -LiteralPath $sl -Tail 600 -Encoding UTF8 | Where-Object { $_ -match 'событие сеанса|сеанс \d+:|снимок стража|удержание|Обход|Тестовое событие' } |
        Select-Object -Last 60 | ForEach-Object { Write-Log "[$Tag] service.log: $_" }
    }
  }
}

function Write-Diagnostics {
  Write-Log "ERROR: [диагностика] состояние службы: $(Get-ServiceState)"
  try { (& sc.exe query $script:ServiceName) | ForEach-Object { Write-Log "ERROR: [диагностика] sc query: $_" } } catch {}
  try {
    Get-PcbProcs | ForEach-Object {
      Write-Log "ERROR: [диагностика] процесс $($_.ProcessId) сеанс $($_.SessionId): $($_.CommandLine)"
    }
  } catch {}
  try {
    $st = Send-Pcb "GetChildStatus"
    Write-Log ("ERROR: [диагностика] GetChildStatus: " + ($st | ConvertTo-Json -Depth 6 -Compress))
    $gs = Send-Pcb "GuardState"
    Write-Log ("ERROR: [диагностика] GuardState: " + ($gs | ConvertTo-Json -Depth 6 -Compress))
  } catch { Write-Log "ERROR: [диагностика] канал связи не отвечает: $($_.Exception.Message)" }
  try { Write-Log ("ERROR: [диагностика] передний план: " + [PcbNative]::Foreground() + "; простой ввода, мс: " + [PcbNative]::IdleMs()) } catch {}
  try { (@(& qwinsta.exe 2>&1)) | ForEach-Object { Write-Log "ERROR: [диагностика] qwinsta: $_" } } catch {}
  foreach ($dir in @($script:Data, $script:DefaultData)) {
    $sl = Join-Path (Join-Path $dir "logs") "service.log"
    if (Test-Path -LiteralPath $sl) {
      Get-Content -LiteralPath $sl -Tail 400 -Encoding UTF8 | Where-Object { $_ -match 'событие сеанса|сеанс \d+:|снимок стража|удержание|Обход|Тестовое событие' } |
        Select-Object -Last 40 | ForEach-Object { Write-Log "ERROR: [диагностика] service.log (сеансы): $_" }
      Get-Content -LiteralPath $sl -Tail 600 -Encoding UTF8 | Where-Object { $_ -match 'наблюдатель|[Сс]нимок программ|Корни программ|WatchDir|отпечаток|«Ждут решения»' } |
        Select-Object -Last 30 | ForEach-Object { Write-Log "ERROR: [диагностика] service.log (программы): $_" }
    }
  }
  Write-SessionProcs "ERROR: [диагностика]"
  foreach ($dir in @($script:Data, $script:DefaultData)) {
    foreach ($name in @("service.log", "boot.log")) {
      $f = Join-Path (Join-Path $dir "logs") $name
      if (Test-Path -LiteralPath $f) {
        Get-Content -LiteralPath $f -Tail 30 -Encoding UTF8 | ForEach-Object { Write-Log "ERROR: [диагностика] $name`: $_" }
      }
    }
  }
  foreach ($tl in @(Get-ChildItem -Path (Join-Path $env:SystemDrive "Users\*\AppData\Local\Temp\pcb-tray-test.log") -ErrorAction SilentlyContinue)) {
    Get-Content -LiteralPath $tl.FullName -Tail 20 -Encoding UTF8 | ForEach-Object { Write-Log "ERROR: [диагностика] журнал значка: $_" }
  }
  try {
    Get-WinEvent -FilterHashtable @{ LogName = 'System'; ProviderName = 'Service Control Manager'; StartTime = (Get-Date).AddMinutes(-15) } -MaxEvents 40 -ErrorAction SilentlyContinue |
      Where-Object { $_.Message -match 'PCBlock|PC Block' } | Select-Object -First 8 |
      ForEach-Object { Write-Log "ERROR: [диагностика] журнал Windows: $($_.TimeCreated.ToString('HH:mm:ss')) $(($_.Message -replace '\s+', ' '))" }
  } catch {}
  try { Save-Screen ("fail-" + $script:Tag.ToLower()) } catch {}
}

# Обёртка сценария: диагностика при падении, чистая машина в конце, код выхода 0 или 1.
function Invoke-Scenario([string]$Id, [string]$Title, [scriptblock]$Body) {
  $script:Tag = $Id
  $ok = $false
  $sw = [Diagnostics.Stopwatch]::StartNew()
  Write-Log "=== $Id $Title"
  try {
    & $Body
    $ok = $true
  } catch {
    Write-Fail ("Сценарий не пройден: " + $_.Exception.Message)
    $where = (($_.ScriptStackTrace -split "`n") | Select-Object -First 3) -join " | "
    Write-Fail ("Место: " + $where)
    Write-Diagnostics
  } finally {
    try { [PcbNative]::StopPing() } catch {}
    Stop-Fullscreen
    Reset-Machine
  }
  Write-Log ("{0} {1}, {2:N0} с" -f $Id, $(if ($ok) { "PASS" } else { "FAIL" }), $sw.Elapsed.TotalSeconds)
  if ($ok) { exit 0 } else { exit 1 }
}

# --- Снимки экрана ---------------------------------------------------------------------------------------
function Save-Screen([string]$Name) {
  New-Item -ItemType Directory -Force -Path $script:ShotDir | Out-Null
  $path = Join-Path $script:ShotDir "$Name.png"
  $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  $bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height
  try {
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    try { $g.CopyFromScreen($b.X, $b.Y, 0, 0, $bmp.Size) } finally { $g.Dispose() }
    $bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
  } finally { $bmp.Dispose() }
  Write-Log "Снимок: $Name.png ($($b.Width)x$($b.Height))"
}

# Правый нижний угол экрана (панель задач и значки у часов), увеличенный в Scale раз.
function Save-TrayCorner([string]$Name, [int]$Width = 520, [int]$Height = 90, [int]$Scale = 3) {
  $path = Join-Path $script:ShotDir "$Name.png"
  $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  $w = [Math]::Min($Width, $b.Width)
  $h = [Math]::Min($Height, $b.Height)
  $src = New-Object System.Drawing.Bitmap $w, $h
  $dst = New-Object System.Drawing.Bitmap ($w * $Scale), ($h * $Scale)
  try {
    $g = [System.Drawing.Graphics]::FromImage($src)
    try { $g.CopyFromScreen($b.Right - $w, $b.Bottom - $h, 0, 0, $src.Size) } finally { $g.Dispose() }
    $g2 = [System.Drawing.Graphics]::FromImage($dst)
    try {
      $g2.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::NearestNeighbor
      $g2.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::Half
      $g2.DrawImage($src, 0, 0, $w * $Scale, $h * $Scale)
    } finally { $g2.Dispose() }
    $dst.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
  } finally { $src.Dispose(); $dst.Dispose() }
  Write-Log "Снимок: $Name.png (угол экрана ${w}x${h}, увеличение x$Scale)"
}

# Запускает окно программы с ключами, ждёт появления окна, снимает его и закрывает. Окно с заголовком
# не обязательно "PC Block": заголовок зависит от режима. Возвращает заголовок.
function Save-ProgramWindow([string]$Name, [string[]]$ArgList, [int]$WaitSeconds = 40, [int]$SettleSeconds = 4) {
  $p = Start-Process -FilePath $script:Exe -ArgumentList (ConvertTo-ArgString $ArgList) -PassThru
  try {
    return (Save-ProcWindow $p $Name $WaitSeconds $SettleSeconds)
  } finally {
    try { if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force } } catch {}
  }
}

# Ждёт окно уже запущенного процесса, выводит его вперёд и снимает рамку окна. Возвращает заголовок.
function Save-ProcWindow([Diagnostics.Process]$p, [string]$Name, [int]$WaitSeconds = 40, [int]$SettleSeconds = 4) {
  Wait-Until { $p.Refresh(); $p.HasExited -or ($p.MainWindowHandle -ne [IntPtr]::Zero -and $p.MainWindowTitle -ne "") } $WaitSeconds "окно программы (PID $($p.Id))"
  Assert-That (-not $p.HasExited) "Программа завершилась сама, код $($p.ExitCode)"
  $title = $p.MainWindowTitle
  Start-Sleep -Seconds $SettleSeconds
  $p.Refresh()
  $h = $p.MainWindowHandle
  [void][PcbNative]::ForceForeground($h)
  Start-Sleep -Milliseconds 800
  $f = [PcbNative]::WindowFrame($h)
  $w = $f[2] - $f[0]
  $hgt = $f[3] - $f[1]
  Assert-That ($w -gt 100 -and $hgt -gt 100) "Окно «$title» имеет размер ${w}x${hgt}"
  $bmp = New-Object System.Drawing.Bitmap $w, $hgt
  try {
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    try { $g.CopyFromScreen($f[0], $f[1], 0, 0, (New-Object System.Drawing.Size $w, $hgt)) } finally { $g.Dispose() }
    $bmp.Save((Join-Path $script:ShotDir "$Name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  } finally { $bmp.Dispose() }
  Write-Log "Снимок окна: $Name.png, заголовок «$title», ${w}x${hgt}"
  return $title
}

# Значок у часов: ищет его в области уведомлений через UI Automation и нажимает, чтобы открылась
# мини-панель. Всё необязательное: нет значка в дереве или нажатие не прошло, сценарий не падает.
function Show-TrayPanel([string]$Name) {
  try {
    Add-Type -AssemblyName UIAutomationClient
    Add-Type -AssemblyName UIAutomationTypes
    $root = [System.Windows.Automation.AutomationElement]::RootElement
    $tray = $root.FindFirst([System.Windows.Automation.TreeScope]::Children,
      (New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ClassNameProperty, "Shell_TrayWnd")))
    if (-not $tray) { Write-Log "Мини-панель: панель задач не найдена в UI Automation"; return $false }
    $names = New-Object System.Collections.Generic.List[string]
    $found = $null
    foreach ($round in 1..2) {
      $all = $tray.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
      foreach ($e in $all) {
        $n = [string]$e.Current.Name
        if ($n) { $names.Add($n) }
        if (-not $found -and $n -match 'PC Block|Осталось|Время на сегодня|Сегодня без') { $found = $e }
      }
      if ($found) { break }
      # Значок мог уйти в скрытые: открываем окно скрытых значков и ищем ещё раз.
      $chevron = $null
      foreach ($e in $all) {
        if ([string]$e.Current.Name -match 'скрытые значки|Show Hidden|Hidden icons') { $chevron = $e; break }
      }
      if (-not $chevron) { break }
      try { ($chevron.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)).Invoke() } catch {}
      Start-Sleep -Milliseconds 800
      $overflow = $root.FindFirst([System.Windows.Automation.TreeScope]::Children,
        (New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ClassNameProperty, "TopLevelWindowForOverflowXamlIsland")))
      if ($overflow) { $tray = $overflow }
    }
    if (-not $found) {
      Write-Log ("Мини-панель: значок не найден в UI Automation. Элементы панели задач: " + (($names | Select-Object -First 25) -join " | "))
      return $false
    }
    Write-Log "Мини-панель: значок найден «$($found.Current.Name)», нажимаю"
    try {
      ($found.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)).Invoke()
    } catch {
      $pt = $found.GetClickablePoint()
      [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point ([int]$pt.X), ([int]$pt.Y)
    }
    Start-Sleep -Seconds 2
    Save-Screen $Name
    return $true
  } catch {
    Write-Log "Мини-панель: не удалось ($($_.Exception.Message))"
    return $false
  }
}

# --- Ввод и полноэкранное окно -------------------------------------------------------------------------
# Проверка, что имитация ввода доходит до сеанса: простой ввода сбрасывается после "движения мыши".
function Test-InputSim {
  Start-Sleep -Milliseconds 1500
  $before = [PcbNative]::IdleMs()
  [PcbNative]::Ping()
  Start-Sleep -Milliseconds 200
  $after = [PcbNative]::IdleMs()
  Write-Log "Проверка ввода: простой до $before мс, после $after мс (сеанс $($script:MySession))"
  Assert-That ($after -ge 0 -and $after -lt 1500) "Имитация ввода не дошла до сеанса: простой после движения мыши $after мс"
}

function Start-Input([int]$IntervalMs = 1000) { [PcbNative]::StartPing($IntervalMs) }
function Stop-Input { [PcbNative]::StopPing() }

$script:FullscreenProc = $null

# Окно без рамки на весь экран, как плеер в полноэкранном режиме; ждёт, пока оно станет передним.
function Show-Fullscreen {
  Stop-Fullscreen
  $ready = Join-Path $script:Work "fullscreen.ready"
  Remove-Item -LiteralPath $ready -ErrorAction SilentlyContinue
  $fsScript = Join-Path $PSScriptRoot "fullscreen-window.ps1"
  $argLine = ConvertTo-ArgString @("-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-File", $fsScript, "-ReadyFile", $ready)
  $script:FullscreenProc = Start-Process -FilePath $script:PsExe -ArgumentList $argLine -PassThru
  $fpid = $script:FullscreenProc.Id
  Wait-Until { Test-Path -LiteralPath $ready } 40 "полноэкранное окно показано"
  Wait-Until {
    $fg = [PcbNative]::Foreground()
    $fg -and ($fg.Split("|")[0] -eq [string]$fpid)
  } 15 "полноэкранное окно стало передним (передний план: $([PcbNative]::Foreground()))"
  Write-Log "Полноэкранное окно: $([PcbNative]::Foreground())"
}

function Stop-Fullscreen {
  if ($script:FullscreenProc) {
    try { if (-not $script:FullscreenProc.HasExited) { Stop-Process -Id $script:FullscreenProc.Id -Force } } catch {}
    $script:FullscreenProc = $null
  }
}

# Видны ли на экране окна процессов программы размером почти с весь экран (экран блокировки).
function Get-BigPcbWindows {
  $pids = @([Diagnostics.Process]::GetProcessesByName("PCBlock") | ForEach-Object { [uint32]$_.Id })
  if ($pids.Count -eq 0) { return @() }
  $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  $big = @()
  foreach ($w in [PcbNative]::VisibleWindows([uint32[]]$pids)) {
    $rect = $w.Split("|")[2].Split(",")
    $ww = [int]$rect[2] - [int]$rect[0]
    $hh = [int]$rect[3] - [int]$rect[1]
    if ($ww -ge $b.Width - 4 -and $hh -ge $b.Height - 4) { $big += $w }
  }
  return $big
}

# --- Экран стража и проверки чисел ---------------------------------------------------------------------
# Элементы массива из ответа службы: пустое и null дают пустой список (в PowerShell @($null) это один элемент).
function List($x) { if ($null -ne $x) { $x } }

function Format-GuardState($s) {
  if (-not $s) { return "нет данных" }
  $w = (@(List $s.warnings) | ForEach-Object { $_.threshold }) -join ","
  $b = (@(List $s.buttons)) -join ","
  $f = (@(List $s.fields) | ForEach-Object { "$($_.id)='$($_.text)'" }) -join ","
  $x = (@(List $s.exits)) -join ","
  return "экран=$($s.lockShown)/$($s.lockKind); плашка=$($s.plateKind)'$($s.plate)'; кнопки=[$b]; поля=[$f]; предупреждения=[$w]; выходы=[$x]; передний план='$($s.foregroundPath)'"
}

function Get-WarnThresholds($State) { return @(List $State.warnings | ForEach-Object { [int]$_.threshold }) }

# Порог показан не больше одного раза (R9: одно предупреждение на событие, класс A).
function Assert-NoRepeatWarnings($State, [string]$What) {
  $seen = @{}
  foreach ($t in (Get-WarnThresholds $State)) {
    Assert-That (-not $seen.ContainsKey($t)) "${What}: порог $t показан дважды ($((Get-WarnThresholds $State) -join ','))"
    $seen[$t] = $true
  }
}

# Пороги сыгранных звуков предупреждений (GuardState.sounds, D-55): по одному на каждую плашку.
function Get-SoundThresholds($State) { return @(List $State.sounds | ForEach-Object { [int]$_.threshold }) }

# Ждёт, пока вид стража (GuardState) подойдёт под условие; условие получает состояние первым аргументом.
# Возвращает последнее состояние; секунды ожидания лежат в $script:WaitElapsed.
function Wait-GuardState([scriptblock]$Cond, [double]$Seconds, [string]$What, [int]$PollMs = 150) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($true) {
    $s = Get-GuardState
    if (& $Cond $s) { $script:WaitElapsed = $sw.Elapsed.TotalSeconds; return $s }
    if ($sw.Elapsed.TotalSeconds -gt $Seconds) { throw "За $Seconds с не дождались: $What (экран: $(Format-GuardState $s))" }
    Start-Sleep -Milliseconds $PollMs
  }
}

function Test-HasButton($State, [string]$Name) { return (@(List $State.buttons) -contains $Name) }

# Выход, о котором страж просил службу в тестовом режиме (GuardState.Exits): "shutdown" или "switchUser".
# Служба в тестовом режиме их не выполняет, а записывает (раннер CI нельзя выключить или отключить).
function Test-HasExit($State, [string]$Name) { return (@(List $State.exits) -contains $Name) }

# Нажимает кнопку экрана стража и ждёт, пока она исполнится: команда доходит до стража при его опросе (раз в секунду).
function Invoke-GuardClick([string]$Target) { Invoke-TestGuard "click" $Target }

# Вводит пароль в поле экрана стража и ждёт, пока страж покажет набранный текст в GuardState.
function Enter-GuardPassword([string]$Text) {
  Invoke-TestGuard "type" "password" $Text
  [void](Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "password" -and $_.text -eq $Text }).Count -ge 1 } 8 "в поле пароля набран текст")
}

# Ждёт, пока экран блокировки станет виден (GuardState), и возвращает состояние.
function Wait-LockShown([double]$Seconds = 30, [string]$Kind = "") {
  return (Wait-GuardState { param($s) $s.lockShown -and (-not $Kind -or $s.lockKind -eq $Kind) } $Seconds "экран блокировки $Kind показан")
}

function Wait-LockHidden([double]$Seconds = 10) {
  return (Wait-GuardState { param($s) -not $s.lockShown } $Seconds "экран блокировки снят")
}

# Проверка числа в границах; в сообщении видно и значение, и границы.
function Assert-Between([double]$Value, [double]$Min, [double]$Max, [string]$What) {
  Write-Log ("{0}: {1} (допустимо {2} ... {3})" -f $What, $Value, $Min, $Max)
  if ($Value -lt $Min -or $Value -gt $Max) { throw "$What вне допуска: $Value, ждали от $Min до $Max" }
}

# Идентификатор первого ребёнка в списке родителя (нужна родительская сессия: Open-Parent).
function Get-ChildId {
  $t = Get-Today
  $card = @(List $t.children)[0]
  Assert-That ($null -ne $card) "В списке детей никого нет"
  return [int64]$card.childId
}

# Запись журнала родителя по тексту (регулярное выражение, без учёта регистра); пусто, если нет.
function Find-Events([string]$Pattern) {
  return @(Get-Events | Where-Object { $_.text -match $Pattern })
}

# Windows 11: выносит значок программы из скрытых значков на панель (HKCU\Control Panel\NotifyIconSettings).
# Необязательно: нет раздела или значка, ничего не происходит. Нужно только для читаемого снимка.
function Show-TrayIconOnTaskbar {
  try {
    $root = "HKCU:\Control Panel\NotifyIconSettings"
    if (-not (Test-Path -LiteralPath $root)) { Write-Log "Значки области уведомлений: раздела NotifyIconSettings нет"; return $false }
    $done = $false
    foreach ($k in Get-ChildItem -LiteralPath $root -ErrorAction SilentlyContinue) {
      $p = Get-ItemProperty -LiteralPath $k.PSPath -ErrorAction SilentlyContinue
      if ($p.ExecutablePath -and $p.ExecutablePath -match 'PCBlock') {
        New-ItemProperty -LiteralPath $k.PSPath -Name IsPromoted -Value 1 -PropertyType DWord -Force | Out-Null
        $done = $true
        Write-Log "Значок вынесен на панель задач: $($p.ExecutablePath)"
      }
    }
    if (-not $done) { Write-Log "Значка программы в NotifyIconSettings нет" }
    return $done
  } catch {
    Write-Log "Значок на панель не вынесен: $($_.Exception.Message)"
    return $false
  }
}

# Ребёнок готов к испытанию: ввод идёт, настройка сделана, значок и страж запущены, часы ядра на паузе
# (ускорение больше 1), чтобы время не уходило, пока готовится проверка. Дальше Resume-Clock.
function Start-Child([int]$LimitMin, [bool]$Pinging = $true, [int]$GamesMin = -1, $Programs = $null) {
  Assert-That ($script:ClockSpeed -gt 1) "Start-Child работает только с ускорением часов (Initialize-Scenario 60)"
  if ($Pinging) { Start-Input 1000 }
  Invoke-Setup $LimitMin $script:MySid "Тест" $null $GamesMin $Programs
  Suspend-Clock
  Wait-SessionParts
  Start-Sleep -Seconds 3
  $st = Get-Status
  Write-Log "Ребёнок готов: статус $($st.status), остаток $($st.remainingS) с, лимит $($st.limitS) с, часы на паузе"
}

# --- Служебная учётка pcbtest (обычный пользователь) ----------------------------------------------------
function New-TestUser {
  Remove-TestUser
  & net.exe user $script:TestUserName $script:TestUserPassword /add /y | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "Учётка $($script:TestUserName) не создана (net user: код $LASTEXITCODE)" }
  $sid = (New-Object System.Security.Principal.NTAccount($script:TestUserName)).Translate([System.Security.Principal.SecurityIdentifier]).Value
  Write-Log "Создана учётка $($script:TestUserName), SID $sid"
  return $sid
}

function Remove-TestUser {
  & net.exe user $script:TestUserName /delete 2>&1 | Out-Null
  $global:LASTEXITCODE = 0
}

# Запускает PCBlock.exe с ключами от имени pcbtest (обычный пользователь). Сначала через служебный ключ
# программы --test-runas, затем запасным путём Start-Process -Credential. Возвращает результат из файла
# OutName (папка C:\Users\Public\pcb-accept доступна всем) или $null.
function Invoke-AsTestUser([string[]]$ArgList, [string]$OutName) {
  New-Item -ItemType Directory -Force -Path $script:Public | Out-Null
  & icacls.exe $script:Public /grant "*S-1-1-0:(OI)(CI)M" /T /C | Out-Null
  $out = Join-Path $script:Public $OutName
  Remove-Item -LiteralPath $out -ErrorAction SilentlyContinue
  $cmd = ConvertTo-ArgString (@($script:Exe) + @($ArgList) + @("--out=$out"))
  $viaProduct = Invoke-Pcb @("--test-runas", "--user=$($script:TestUserName)", "--password=$($script:TestUserPassword)", "--exe=$cmd") "runas-$OutName"
  $how = "--test-runas"
  if (-not (Test-Path -LiteralPath $out)) {
    Write-Log "Через --test-runas результата нет (код $($viaProduct.Code)), пробую Start-Process -Credential"
    $how = "Start-Process -Credential"
    $cred = New-Object System.Management.Automation.PSCredential(".\$($script:TestUserName)", (ConvertTo-SecureString $script:TestUserPassword -AsPlainText -Force))
    try {
      $p = Start-Process -FilePath $script:Exe -ArgumentList (ConvertTo-ArgString (@($ArgList) + @("--out=$out"))) -Credential $cred -WorkingDirectory $script:Public -Wait -PassThru -WindowStyle Hidden
      Write-Log "Start-Process -Credential: код $($p.ExitCode)"
    } catch { Write-Log "Start-Process -Credential: $($_.Exception.Message)" }
  }
  if (-not (Test-Path -LiteralPath $out)) { return $null }
  $raw = Get-Content -LiteralPath $out -Raw -Encoding UTF8
  Write-Log ("От имени {0} ({1}): {2}" -f $script:TestUserName, $how, ($raw -replace '\s+', ' '))
  return ($raw | ConvertFrom-Json)
}

# --- Программы и игры (M3, RM3-8) --------------------------------------------------------------------
# Тестовые программы — копии системной программы с обычным окном в папке $script:Apps (WatchDir): вне
# доверенных каталогов, поэтому незнакомые по определению. Наблюдатель в тестовом режиме трогает только
# эту папку; всё остальное на раннере (pwsh, PCBlock.exe, браузеры) он не закрывает.

# Родительская сессия на время Body и сразу выход: родительская сессия в сеансе ребёнка прячет плашки
# стража (Ruling 19), поэтому она не должна оставаться открытой, пока проверяются плашки.
function Use-Parent([scriptblock]$Body) {
  Open-Parent
  try {
    & $Body
  } finally {
    try { [void](Send-Parent "Logout") } catch {}
    $script:Token = ""
  }
}

# Ждёт снимок программ службы (RM3-10): пока его нет, наблюдатель ничего не закрывает. Признак — строка
# «Снимок программ: найдено …» в журнале работы. Заодно проверяет, что служба приняла WatchDir.
function Wait-ProgramsReady([double]$Seconds = 120) {
  $log = Join-Path (Join-Path $script:Data "logs") "service.log"
  $script:SeedLine = ""
  Wait-Until {
    if (-not (Test-Path -LiteralPath $log)) { return $false }
    $lines = @(Get-Content -LiteralPath $log -Encoding UTF8 -ErrorAction SilentlyContinue)
    $bad = @($lines | Where-Object { $_ -match 'снимок программ не (сделан|записан)' }) | Select-Object -Last 1
    if ($bad) { throw "Служба не сделала снимок программ, наблюдатель ничего не закроет: $bad" }
    $ok = @($lines | Where-Object { $_ -match 'Снимок программ: найдено' }) | Select-Object -Last 1
    if ($ok) { $script:SeedLine = $ok }
    [bool]$ok
  } $Seconds "снимок программ службы (строка «Снимок программ: найдено» в service.log; без него наблюдатель ничего не закрывает, RM3-10)" 500
  Write-Log ("Список программ готов за {0:N1} с: {1}" -f $script:WaitElapsed, $script:SeedLine)
  $roots = @(Get-Content -LiteralPath $log -Encoding UTF8 -ErrorAction SilentlyContinue | Where-Object { $_ -match 'папка тестовых программ' }) | Select-Object -Last 1
  Write-Log "Корни программ службы: $roots"
  Assert-That ($roots -and $roots -notmatch 'папка тестовых программ ""') "Служба не приняла WatchDir (папка тестовых программ пуста): $roots"
}

# Копирует exe (и его файлы .mui из языковых папок System32, без них у копии нет строк интерфейса) в
# Dir под именем Name.exe. Возвращает длинный путь копии.
function Copy-TestExe([string]$Src, [string]$Dir, [string]$Name) {
  New-Item -ItemType Directory -Force -Path $Dir | Out-Null
  $dst = Join-Path $Dir "$Name.exe"
  Copy-Item -LiteralPath $Src -Destination $dst -Force
  $base = Split-Path -Leaf $Src
  foreach ($mui in @(Get-ChildItem -Path (Join-Path (Split-Path -Parent $Src) "*\$base.mui") -ErrorAction SilentlyContinue)) {
    $lang = Split-Path -Leaf (Split-Path -Parent $mui.FullName)
    $ldir = Join-Path $Dir $lang
    New-Item -ItemType Directory -Force -Path $ldir | Out-Null
    Copy-Item -LiteralPath $mui.FullName -Destination (Join-Path $ldir "$Name.exe.mui") -Force
  }
  return [PcbNative]::LongPath($dst)
}

# Какую системную программу копировать: первую, у копии которой (вне WatchDir, в папке probe) за 10 с
# появилось обычное окно. Порядок: notepad.exe (классический Блокнот образа Windows Server); если на
# образе он заглушка Магазина (копия сразу завершается) — cmd.exe (окно консоли). Ни у одной нет окна —
# notepad.exe, а передний план даёт подмена TestGuard foreground (Set-TestForeground).
function Get-TestAppSource {
  if ($script:TestAppSource) { return $script:TestAppSource }
  $sys = Join-Path $env:SystemRoot "System32"
  foreach ($cand in @("notepad.exe", "cmd.exe")) {
    $src = Join-Path $sys $cand
    if (-not (Test-Path -LiteralPath $src)) { Write-Log "Тестовая программа: $src нет на образе"; continue }
    $p = $null
    $since = Get-Date
    try {
      Remove-Item -LiteralPath $script:Probe -Recurse -Force -ErrorAction SilentlyContinue
      $dst = Copy-TestExe $src $script:Probe "pcb-probe"
      $p = Start-Process -FilePath $dst -PassThru -WorkingDirectory $script:Probe
      $ok = $false
      $sw = [Diagnostics.Stopwatch]::StartNew()
      while ($sw.Elapsed.TotalSeconds -lt 10) {
        $p.Refresh()
        if ($p.HasExited) { break }
        if ($p.MainWindowHandle -ne [IntPtr]::Zero) { $ok = $true; break }
        Start-Sleep -Milliseconds 200
      }
      $why = if ($ok) { "окно «$($p.MainWindowTitle)»" } elseif ($p.HasExited) { "копия завершилась сама, код $($p.ExitCode)" } else { "окна нет за 10 с" }
      Write-Log "Проба копии $cand`: $why"
      if ($ok) { $script:TestAppSource = $src; return $src }
    } catch {
      Write-Log "Проба копии $cand не удалась: $($_.Exception.Message)"
    } finally {
      if ($p) { try { if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force } } catch {} }
      # Заглушка Блокнота открывает Блокнот из Магазина: его окно тоже закрыть.
      Get-Process -Name "Notepad" -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -match '\\WindowsApps\\' -and $_.StartTime -ge $since } |
        ForEach-Object { try { Stop-Process -Id $_.Id -Force } catch {} }
      Remove-Item -LiteralPath $script:Probe -Recurse -Force -ErrorAction SilentlyContinue
    }
  }
  $script:TestAppSource = Join-Path $sys "notepad.exe"
  Write-Log "Тестовая программа: ни у одной копии нет окна, беру notepad.exe; передний план подменой TestGuard foreground"
  return $script:TestAppSource
}

# Тестовая программа $script:Apps\<Name>.exe (копия системной программы, см. Get-TestAppSource).
# Запуск — Start-TestApp, категорию до первого запуска даёт Invoke-Setup -Programs.
function New-TestApp([string]$Name) {
  $src = Get-TestAppSource
  $path = Copy-TestExe $src $script:Apps $Name
  Write-Log "Тестовая программа $Name.exe: копия $(Split-Path -Leaf $src), $path"
  return $path
}

# Запускает тестовую программу в сеансе ребёнка (пользователь раннера) и возвращает процесс. Процесс
# запоминается: Stop-TestApps закрывает его в конце сценария.
function Start-TestApp([string]$Path) {
  $p = Start-Process -FilePath $Path -PassThru -WorkingDirectory (Split-Path -Parent $Path)
  $p | Add-Member -NotePropertyName PcbPath -NotePropertyValue $Path -Force
  $p | Add-Member -NotePropertyName PcbStart -NotePropertyValue ([Diagnostics.Stopwatch]::StartNew()) -Force
  $script:TestProcs = @($script:TestProcs) + @($p)
  Write-Log "Запущена $(Split-Path -Leaf $Path) (PID $($p.Id))"
  return $p
}

# Ждёт исчезновения процесса и возвращает миллисекунды: от запуска (время жизни процесса по часам
# Windows) или от момента Since (Stopwatch, например «игры кончились»). Ждёт до max(4·MaxMs, 5 с), чтобы
# в журнал попало фактическое время и при превышении допуска; процесс жив и тогда — возвращает -1.
function Wait-ProcessGone($Proc, [int]$MaxMs = 1500, $Since = $null) {
  $clock = if ($Since) { $Since } elseif ($Proc.PcbStart) { $Proc.PcbStart } else { [Diagnostics.Stopwatch]::StartNew() }
  $limit = [Math]::Max(4 * $MaxMs, 5000)
  while (-not $Proc.HasExited) {
    if ($clock.ElapsedMilliseconds -gt $limit) { return -1 }
    Start-Sleep -Milliseconds 15
  }
  if (-not $Since) {
    try { return [int]($Proc.ExitTime - $Proc.StartTime).TotalMilliseconds } catch {}
  }
  return [int]$clock.ElapsedMilliseconds
}

# Процесс закрыт не позже допуска раннера (NFR-6: не позже 1 с; допуск 1500 мс). Фактическое время в журнал.
function Assert-ClosedFast($Proc, [string]$What, $Since = $null) {
  $name = Split-Path -Leaf ([string]$Proc.PcbPath)
  $ms = Wait-ProcessGone $Proc $script:CloseToleranceMs $Since
  if ($ms -lt 0) {
    Write-SessionProcs "[$What]"
    throw "${What}: $name (PID $($Proc.Id)) не закрыт за $([Math]::Max(4 * $script:CloseToleranceMs, 5000)) мс; экран стража: $(Format-GuardState (Get-GuardState))"
  }
  $from = if ($Since) { "от события" } else { "от запуска" }
  $note = if ($ms -gt 1000) { "; дольше 1 с (NFR-6), в пределах допуска раннера" } else { "" }
  Write-Log "${What}: $name закрыт за $ms мс $from (NFR-6: не позже 1000 мс, допуск раннера $($script:CloseToleranceMs) мс)$note"
  Assert-That ($ms -le $script:CloseToleranceMs) "${What}: $name закрыт за $ms мс $from, допуск $($script:CloseToleranceMs) мс"
  return $ms
}

function Assert-AppAlive($Proc, [string]$What) {
  $Proc.Refresh()
  if ($Proc.HasExited) {
    Write-SessionProcs "[$What]"
    throw "${What}: $(Split-Path -Leaf ([string]$Proc.PcbPath)) (PID $($Proc.Id)) закрыта, код $($Proc.ExitCode); экран стража: $(Format-GuardState (Get-GuardState))"
  }
}

# Закрывает все тестовые программы (запущенные Start-TestApp и любые процессы из папки $script:Apps).
# Ничего не бросает: вызывается из Reset-Machine в начале и в конце сценария.
function Stop-TestApps {
  foreach ($p in $script:TestProcs) {
    try { if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } } catch {}
  }
  $script:TestProcs = @()
  foreach ($dir in @($script:Apps, $script:Probe)) {
    try {
      $prefix = $dir.TrimEnd('\') + '\'
      Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Path -and $_.Path.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase) } |
        ForEach-Object { try { Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue } catch {} }
    } catch {}
  }
}

# Процессы сеанса раннера с путями (диагностика): всё вне папки Windows и отдельно тестовые программы.
function Write-SessionProcs([string]$Prefix) {
  try {
    $all = @(Get-CimInstance Win32_Process -ErrorAction Stop | Where-Object { $_.SessionId -eq $script:MySession })
    $win = $env:SystemRoot.TrimEnd('\') + '\'
    $shown = @($all | Where-Object { $_.ExecutablePath -and -not $_.ExecutablePath.StartsWith($win, [StringComparison]::OrdinalIgnoreCase) })
    Write-Log "$Prefix процессов в сеансе $($script:MySession): $($all.Count), вне папки Windows: $($shown.Count)"
    $shown | Select-Object -First 60 | ForEach-Object {
      $mark = if ($_.ExecutablePath.StartsWith($script:Apps.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase)) { " [тестовая]" } else { "" }
      Write-Log "$Prefix процесс $($_.ProcessId)$mark`: $($_.ExecutablePath)"
    }
  } catch { Write-Log "$Prefix список процессов сеанса не получен: $($_.Exception.Message)" }
}

# «Программы» родителя (ListPrograms): ждут решения, все программы, дети.
function Get-ProgramsList {
  return (Use-Parent {
    $r = Send-Parent "ListPrograms"
    Assert-Reply $r "ListPrograms"
    $r.payload
  })
}

# Имя программы, как его показывает журнал родителя: имя из списка программ по пути (у копии Блокнота
# это свойства файла, «Блокнот»/«Notepad», а не имя файла); программы в списке нет: имя файла без .exe.
function Get-ProgramName([string]$Path) {
  $norm = $Path.ToLowerInvariant()
  $p = @(@(List (Get-ProgramsList).programs) | Where-Object { ([string]$_.path) -eq $norm })
  if ($p.Count -gt 0 -and [string]$p[0].name) { return [string]$p[0].name }
  return [IO.Path]::GetFileNameWithoutExtension($Path)
}

# Все строки журнала одной строкой: для текста ошибки, когда нужной записи нет.
function Format-Events {
  return (@(Get-Events | ForEach-Object { [string]$_.text }) -join ' | ')
}

# Строки «Ждут решения» с путём Path (путь в канале нормализован: нижний регистр).
function Find-Pending($List, [string]$Path) {
  $norm = $Path.ToLowerInvariant()
  return @(@(List $List.pending) | Where-Object { ([string]$_.path) -eq $norm })
}

# Категория программы через родительскую сессию (games, study, other, blocked): ждущей решения —
# DecideProgram, уже известной — SetProgramsCategory. Программы нет ни там, ни там — ошибка: до первого
# запуска категорию даёт Invoke-Setup -Programs. Возвращает ответ (message «superGame: игра», undoId).
# DecideProgram — последняя команда перед выходом: плашку о решении страж получает следующим опросом
# уже без родительской сессии (Ruling 19).
function Set-ProgramCategory([string]$Path, [string]$Category) {
  $norm = $Path.ToLowerInvariant()
  $rep = Use-Parent {
    $r = Send-Parent "ListPrograms"
    Assert-Reply $r "ListPrograms"
    $pend = @(Find-Pending $r.payload $Path)
    $known = @(@(List $r.payload.programs) | Where-Object { ([string]$_.path) -eq $norm })
    if ($pend.Count -gt 0) {
      $script:SetHow = "DecideProgram"
      $d = Send-Parent "DecideProgram" @{ path = $Path; category = $Category }
    } elseif ($known.Count -gt 0) {
      $script:SetHow = "SetProgramsCategory"
      $d = Send-Parent "SetProgramsCategory" @{ paths = @($Path); category = $Category }
    } else {
      throw "Программы $Path нет ни в «Ждут решения», ни в списке программ: до первого запуска категорию даёт мастер (Invoke-Setup -Programs)"
    }
    $d
  }
  Assert-Reply $rep "$($script:SetHow) $Category"
  Write-Log "Категория $(Split-Path -Leaf $Path): $Category через $($script:SetHow), сообщение «$($rep.payload.message)»"
  return $rep.payload
}

# Ставит тестовую программу на передний план и ждёт, пока страж пришлёт её путь (GuardState.foregroundPath).
# Сначала по-настоящему (ForceForeground, как Show-Fullscreen); не вышло за 5 с — подмена пути переднего
# плана в страже (TestGuard foreground, только тестовый режим). Процесс при подмене должен работать:
# отпечаток файла служба берёт у наблюдателя.
function Set-TestForeground($Proc) {
  $fgWant = ([string]$Proc.PcbPath).ToLowerInvariant()
  if ($script:FgOverride) {
    Invoke-TestGuard "foreground"
    $script:FgOverride = $false
  }
  $h = [IntPtr]::Zero
  for ($i = 0; $i -lt 25 -and $h -eq [IntPtr]::Zero -and -not $Proc.HasExited; $i++) {
    $Proc.Refresh()
    $h = $Proc.MainWindowHandle
    if ($h -eq [IntPtr]::Zero) { Start-Sleep -Milliseconds 200 }
  }
  if ($h -ne [IntPtr]::Zero) {
    $fg = [PcbNative]::ForceForeground($h)
    try {
      [void](Wait-GuardState { param($s) ([string]$s.foregroundPath).ToLowerInvariant() -eq $fgWant } 5 "страж видит $fgWant на переднем плане")
      Write-Log "Передний план: $(Split-Path -Leaf $fgWant) по-настоящему (ForceForeground=$fg, $([PcbNative]::Foreground()))"
      return
    } catch {
      Write-Log "Передний план по-настоящему не вышел: $($_.Exception.Message)"
    }
  } else {
    Write-Log "У $(Split-Path -Leaf $fgWant) нет окна: передний план подменой"
  }
  Invoke-TestGuard "foreground" "" ([string]$Proc.PcbPath)
  $script:FgOverride = $true
  [void](Wait-GuardState { param($s) ([string]$s.foregroundPath).ToLowerInvariant() -eq $fgWant } 5 "подмена переднего плана в страже: $fgWant")
  Write-Log "Передний план: $(Split-Path -Leaf $fgWant) подменой TestGuard foreground"
}

# «Моя статистика» ребёнка этого сеанса (MyToday): статус, gamesS, studyS — счётчики дня ядра.
function Get-MyToday {
  $r = Send-Pcb "MyToday"
  Assert-Reply $r "MyToday"
  return $r.payload
}

# Ждёт, пока статус ребёнка (GetChildStatus) подойдёт под условие; опрос 40 мс (при ускорении 60 это
# около 2 с часов ядра). Возвращает статус; при провале в сообщении статус и передний план стража.
function Wait-Status([scriptblock]$Cond, [double]$Seconds, [string]$What, [int]$PollMs = 40) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($true) {
    $st = Get-Status
    if (& $Cond $st) { $script:WaitElapsed = $sw.Elapsed.TotalSeconds; return $st }
    if ($sw.Elapsed.TotalSeconds -gt $Seconds) {
      throw "За $Seconds с не дождались: $What (статус $($st.status), остаток $($st.remainingS) с, игры $($st.gamesRemainingS) с из $($st.gamesLimitS), closeGames=$($st.closeGames), studyOnly=$($st.studyOnly), usedS=$($st.usedS); экран: $(Format-GuardState (Get-GuardState)))"
    }
    Start-Sleep -Milliseconds $PollMs
  }
}

# Пороги показанных предупреждений с текстами (для журнала сценария).
function Format-Warnings($State) {
  return ((@(List $State.warnings) | ForEach-Object { "$($_.threshold):'$($_.text)'" }) -join "; ")
}
