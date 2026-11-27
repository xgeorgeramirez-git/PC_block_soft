# S60. Сообщение с телефона со стикером и звуком (FR-41, D-101, протокол связи, раздел 5а).
# Сервер связи pcblock-relay.exe на раннере (127.0.0.1:18080), тестовый телефон remote-phone.exe (-sticker, -sound).
# Часы ядра идут в 60 раз быстрее: лимит 5 мин кончается примерно через 5 с (часы на паузе до шага блокировки).
# Дано: телефон привязан (как S55), ребёнок на компьютере.
# (а) «Молодец!» со стикером 033 и звуком 2: окно не позже 5 с после ответа телефону, noteSticker = "033", кадры
#     стикера (noteFrame) сменяются за 1 с, звук 2 сыгран один раз; «Понял» закрывает окно.
# (б) без стикера и «Без звука» (-sound 0): окно без стикера, ни одного звука сообщения за 3 с.
# (в) стикер 999 и звук 9, которых программа не знает: окно без стикера, звук 1.
# (г) со стикером поверх экрана блокировки: окно PCBlockGuardNote (рамка с тенью) не пересекает заголовок экрана
#     блокировки и его кнопки. Кнопки: дочерние окна. Заголовок рисуется в окне экрана, поэтому он ищется на снимке
#     экрана: первая сверху полоса текста над кнопками до сообщения и та же полоса (та же высота и ширина, может
#     сдвинуться вверх раздвижкой экрана) при сообщении, строки окна сообщения не смотрятся.
# Экраны 1024x768, 1280x720, 1366x768, 1920x1080 при 100 % и 150 %: тест раскладки (Ruling FB28-2), здесь экран раннера.
# Снимки: s60-note-sticker.png (окно на рабочем столе), s60-note-sticker-lock.png (поверх экрана блокировки).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

if (-not ('PcbScreenInk' -as [type])) {
  Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class PcbScreenInk {
  [StructLayout(LayoutKind.Sequential)]
  struct BITMAPINFOHEADER {
    public uint biSize; public int biWidth; public int biHeight; public ushort biPlanes; public ushort biBitCount;
    public uint biCompression; public uint biSizeImage; public int biXPelsPerMeter; public int biYPelsPerMeter;
    public uint biClrUsed; public uint biClrImportant;
  }
  [DllImport("user32.dll")] static extern IntPtr GetDC(IntPtr h);
  [DllImport("user32.dll")] static extern int ReleaseDC(IntPtr h, IntPtr dc);
  [DllImport("user32.dll")] static extern bool SystemParametersInfo(uint action, uint param, ref int value, uint flags);
  [DllImport("gdi32.dll")] static extern IntPtr CreateCompatibleDC(IntPtr dc);
  [DllImport("gdi32.dll")] static extern IntPtr CreateCompatibleBitmap(IntPtr dc, int w, int h);
  [DllImport("gdi32.dll")] static extern IntPtr SelectObject(IntPtr dc, IntPtr o);
  [DllImport("gdi32.dll")] static extern bool BitBlt(IntPtr d, int x, int y, int w, int h, IntPtr s, int sx, int sy, uint rop);
  [DllImport("gdi32.dll")] static extern int GetDIBits(IntPtr dc, IntPtr bmp, uint start, uint lines, byte[] bits, ref BITMAPINFOHEADER bi, uint usage);
  [DllImport("gdi32.dll")] static extern bool DeleteObject(IntPtr o);
  [DllImport("gdi32.dll")] static extern bool DeleteDC(IntPtr dc);

  // Снимок области экрана в BGRA сверху вниз (шаг строки w*4). CAPTUREBLT: layered-окна (окно сообщения) тоже на снимке.
  public static byte[] Capture(int x, int y, int w, int h) {
    IntPtr scr = GetDC(IntPtr.Zero);
    IntPtr mem = CreateCompatibleDC(scr);
    IntPtr bmp = CreateCompatibleBitmap(scr, w, h);
    IntPtr old = SelectObject(mem, bmp);
    BitBlt(mem, 0, 0, w, h, scr, x, y, 0x00CC0020u | 0x40000000u);
    SelectObject(mem, old);
    var bi = new BITMAPINFOHEADER();
    bi.biSize = 40; bi.biWidth = w; bi.biHeight = -h; bi.biPlanes = 1; bi.biBitCount = 32;
    var px = new byte[w * h * 4];
    int got = GetDIBits(mem, bmp, 0, (uint)h, px, ref bi, 0);
    DeleteObject(bmp); DeleteDC(mem); ReleaseDC(IntPtr.Zero, scr);
    if (got != h) throw new InvalidOperationException("GetDIBits: " + got + " строк из " + h);
    return px;
  }

  // Для строк y0..y1-1 снимка шириной w: самый левый и самый правый x в [x0, x1), где цвет отличается от фона
  // (b, g, r) больше thr (сумма модулей по каналам); -1, -1: строка без текста.
  public static int[] Rows(byte[] px, int w, int x0, int x1, int y0, int y1, int b, int g, int r, int thr) {
    int n = Math.Max(0, y1 - y0);
    var res = new int[n * 2];
    for (int i = 0; i < n; i++) {
      int row = (y0 + i) * w * 4, lo = -1, hi = -1;
      for (int x = x0; x < x1; x++) {
        int o = row + x * 4;
        int d = Math.Abs(px[o] - b) + Math.Abs(px[o + 1] - g) + Math.Abs(px[o + 2] - r);
        if (d > thr) { if (lo < 0) lo = x; hi = x; }
      }
      res[2 * i] = lo; res[2 * i + 1] = hi;
    }
    return res;
  }

  // Анимации Windows (SPI_GETCLIENTAREAANIMATION) у пользователя сценария: выключены - стикер стоит на первом кадре.
  public static bool Animations() { int v = 0; SystemParametersInfo(0x1042, 0, ref v, 0); return v != 0; }
}
'@
}

# Состояние сообщения id в подписанном отчёте телефону (или пусто).
function Get-NoteState([string]$Id) {
  $r = (Invoke-Phone @("notes")).Result
  $n = @(List $r.notes) | Where-Object { $_.id -eq $Id } | Select-Object -First 1
  if ($null -eq $n) { return "" }
  return [string]$n.state
}

function Wait-NoteState([string]$Id, [string]$State, [double]$Seconds) {
  Wait-Until { $script:LastNoteState = Get-NoteState $Id; $script:LastNoteState -eq $State } $Seconds "отметка '$State' у сообщения $Id в отчёте телефону" 700
}

function Get-NoteSounds($State) { return @(List $State.sounds | Where-Object { $_.note }) }

function Format-NoteSounds($List) { return ((@($List) | ForEach-Object { "звук $([int]$_.noteSound) ok=$($_.ok)" }) -join '; ') }

# Снимок основного экрана: пиксели BGRA, размер и начало в координатах экрана. Name: сохранить ещё и PNG.
function Get-ScreenShot([string]$Name = "") {
  $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  $px = [PcbScreenInk]::Capture($b.X, $b.Y, $b.Width, $b.Height)
  if ($Name) {
    New-Item -ItemType Directory -Force -Path $script:ShotDir | Out-Null
    $bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height, ([System.Drawing.Imaging.PixelFormat]::Format32bppRgb)
    try {
      $rc = New-Object System.Drawing.Rectangle 0, 0, $b.Width, $b.Height
      $data = $bmp.LockBits($rc, [System.Drawing.Imaging.ImageLockMode]::WriteOnly, [System.Drawing.Imaging.PixelFormat]::Format32bppRgb)
      try { [Runtime.InteropServices.Marshal]::Copy($px, 0, $data.Scan0, $px.Length) } finally { $bmp.UnlockBits($data) }
      $bmp.Save((Join-Path $script:ShotDir "$Name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
    } finally { $bmp.Dispose() }
    Write-Log "Снимок: $Name.png ($($b.Width)x$($b.Height), с layered-окнами)"
  }
  return @{ Px = $px; X = $b.X; Y = $b.Y; W = $b.Width; H = $b.Height }
}

# Полосы текста на снимке экрана блокировки в строках экрана [Top, Bottom): подряд идущие строки, где в средних 80 %
# ширины цвет отличается от фона (точка у левого края на середине высоты). Строки Skip (окно сообщения: Top, Bottom)
# не смотрятся и рвут полосу; Touch - полоса вплотную к ним (могла уйти под окно).
function Get-InkBands($Shot, [int]$Top, [int]$Bottom, $Skip = $null) {
  $y0 = [math]::Max(0, $Top - $Shot.Y); $y1 = [math]::Min($Shot.H, $Bottom - $Shot.Y)
  $x0 = [int]($Shot.W * 0.1); $x1 = $Shot.W - $x0
  $o = ([int][math]::Floor($Shot.H / 2) * $Shot.W + 3) * 4
  $bg = @([int]$Shot.Px[$o], [int]$Shot.Px[$o + 1], [int]$Shot.Px[$o + 2])
  $rows = [PcbScreenInk]::Rows($Shot.Px, $Shot.W, $x0, $x1, $y0, $y1, $bg[0], $bg[1], $bg[2], 60)
  $sk0 = if ($Skip) { $Skip.Top - $Shot.Y } else { -1 }
  $sk1 = if ($Skip) { $Skip.Bottom - $Shot.Y } else { -1 }
  $bands = New-Object System.Collections.Generic.List[object]
  $cur = $null
  for ($i = 0; $i -lt ($y1 - $y0); $i++) {
    $y = $y0 + $i
    $skipped = ($y -ge $sk0 -and $y -lt $sk1)
    $lo = $rows[2 * $i]; $hi = $rows[2 * $i + 1]
    if (-not $skipped -and $lo -ge 0) {
      if ($null -eq $cur) {
        $cur = [pscustomobject]@{ Top = $y; Bottom = $y + 1; Left = $lo; Right = $hi + 1; Touch = ($Skip -and $y -eq $sk1) }
      } else {
        $cur.Bottom = $y + 1; $cur.Left = [math]::Min($cur.Left, $lo); $cur.Right = [math]::Max($cur.Right, $hi + 1)
      }
      continue
    }
    if ($null -ne $cur) {
      if ($skipped) { $cur.Touch = $true }
      $bands.Add($cur); $cur = $null
    }
  }
  if ($null -ne $cur) { $bands.Add($cur) }
  foreach ($bd in $bands) {
    $bd.Top += $Shot.Y; $bd.Bottom += $Shot.Y; $bd.Left += $Shot.X; $bd.Right += $Shot.X
  }
  Write-Log ("Фон экрана BGR {0}; полосы текста в строках {1}..{2}: {3}" -f ($bg -join ','), $Top, $Bottom, (($bands | ForEach-Object { "$(Format-Rect $_)$(if ($_.Touch) { ' у окна' })" }) -join '; '))
  return $bands.ToArray()
}

# Верх столбца поля и кнопок экрана блокировки: верх самого верхнего дочернего окна PC Block.
function Get-LockControlsTop {
  $c = @(Get-PcbWindowRects -Children | Where-Object { $_.Class -eq "Button" -or $_.Class -eq "Edit" })
  Assert-That ($c.Count -gt 0) "Кнопок экрана блокировки нет среди окон: $((Get-PcbWindowRects -Children | ForEach-Object { $_.Class }) -join ', ')"
  return [int](($c | Measure-Object -Property Top -Minimum).Minimum)
}

function Get-NoteRect {
  return (@(Get-PcbWindowRects | Where-Object { $_.Class -eq "PCBlockGuardNote" }) | Select-Object -First 1)
}

# Окно сообщения на экране и не выше 55 % экрана (Ruling FB28-1: стикер, карточка и тень вместе).
function Assert-NoteFits($Note, [string]$What) {
  $screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  Assert-That ($null -ne $Note) "$What`: окна сообщения PCBlockGuardNote не видно: $((Get-PcbWindowRects | ForEach-Object { $_.Class }) -join ', ')"
  Write-Log ("{0}: экран {1}x{2}, окно сообщения {3}" -f $What, $screen.Width, $screen.Height, (Format-Rect $Note))
  Assert-That ($Note.Top -ge $screen.Top -and $Note.Bottom -le $screen.Bottom -and $Note.Left -ge $screen.Left -and $Note.Right -le $screen.Right) "$What`: окно сообщения $(Format-Rect $Note) за краем экрана"
  Assert-That (($Note.Bottom - $Note.Top) -le [int]($screen.Height * 0.55) + 1) "$What`: окно сообщения выше 55 % экрана: $(Format-Rect $Note)"
}

# Отправляет сообщение с телефона и ждёт его окно у ребёнка не дольше 5 с после ответа телефону. Возвращает id,
# состояние стража и время показа.
function Send-PhoneNote([string]$Text, [string[]]$Extra, [string]$What) {
  $r = Invoke-Phone (@("note", "-child", "$($script:ChildId)", "-text", $Text, "-show", "0") + $Extra)
  Assert-That ($r.Code -eq 0 -and $r.Result.ok) "$What`: сообщение не принято: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
  $sw = [Diagnostics.Stopwatch]::StartNew()
  $id = [string]$r.Result.id
  $g = Wait-GuardState { param($s) $s.noteId -eq $id } 5 "$What`: окно «Сообщение от родителя» у ребёнка"
  $at = $sw.Elapsed.TotalSeconds
  Write-Log ("{0}: окно через {1:N2} с после ответа телефону ({2} мс), текст '{3}', стикер '{4}', кадр {5}, кнопки [{6}]" -f $What, $at, $r.Result.ms, $g.note, $g.noteSticker, $g.noteFrame, ((@(List $g.buttons)) -join ','))
  Assert-That ($at -le 5) ("{0}: окно через {1:N2} с, ждали не больше 5" -f $What, $at)
  Assert-That ([string]$g.note -eq $Text) "$What`: текст окна '$($g.note)', ждали '$Text'"
  Assert-That (Test-HasButton $g "noteOk") "$What`: в окне нет кнопки «Понял»: $(Format-GuardState $g)"
  return @{ Id = $id; State = $g; Watch = $sw; ShownAt = $at }
}

# «Понял» закрывает окно, отметка «Прочитано».
function Close-PhoneNote([string]$Id, [string]$What) {
  Invoke-GuardClick "noteOk"
  $g = Wait-GuardState { param($s) -not $s.noteId } 5 "$What`: окно закрыто после «Понял»"
  Assert-That ($null -eq (Get-NoteRect)) "$What`: после «Понял» окно PCBlockGuardNote ещё на экране"
  Wait-NoteState $Id "read" 10
  Write-Log "$What`: после «Понял» отметка $($script:LastNoteState), экран: $(Format-GuardState $g)"
  return $g
}

# Ровно один новый звук сообщения с номером Want: ждёт появления до 5 с, потом ещё 2 с, что второго нет.
function Assert-OneNoteSound([int]$Before, [int]$Want, [string]$What) {
  $g = Wait-GuardState { param($s) @(Get-NoteSounds $s).Count -gt $Before } 5 "$What`: звук сообщения"
  Start-Sleep -Seconds 2
  $new = @(Get-NoteSounds (Get-GuardState) | Select-Object -Skip $Before)
  Write-Log "$What`: новые звуки сообщения: $(Format-NoteSounds $new)"
  Assert-That ($new.Count -eq 1) "$What`: новых звуков сообщения $($new.Count) ($(Format-NoteSounds $new)), ждали один"
  Assert-That ([int]$new[0].noteSound -eq $Want) "$What`: сыгран звук $([int]$new[0].noteSound), ждали звук $Want"
}

Invoke-Scenario "S60" "Сообщение с телефона со стикером и звуком: стикер 033 движется, звук 2, без звука, неизвестные номера, поверх экрана блокировки" {
  Initialize-Scenario 60
  try {
    Step "Имитация ввода доходит до сеанса" {
      Test-InputSim
    }

    Step "Настройка: лимит 5 мин, часы на паузе; сервер связи запущен, связь включена" {
      Start-Child 5
      Start-Relay
      Use-Parent {
        Assert-Reply (Send-Parent "SetRemote" @{ enabled = $true; url = $script:RelayUrl }) "SetRemote"
        $v = Wait-RemoteState "online" 30
        Write-Log "На связи: $(Format-Remote $v)"
      }
    }

    Step "Телефон привязан: код, сверка числа, «Совпадает» на обоих" {
      Use-Parent {
        Assert-Reply (Send-Parent "RemotePairStart") "RemotePairStart"
        Wait-Until { $script:LastRemote = Get-Remote; $script:LastRemote.pair.stage -eq "code" -and $script:LastRemote.pair.code -match '^\d{6}$' } 10 "код привязки в окне" 300
      }
      $r = Invoke-Phone @("pair", "-url", $script:RelayUrl, "-code", [string]$script:LastRemote.pair.code, "-name", "Pixel-S60")
      Assert-That ($r.Code -eq 0) "Привязка с телефона не удалась: $($r.Result | ConvertTo-Json -Compress)"
      $sas = [string]$r.Result.sas
      Use-Parent {
        Wait-Until { $script:LastRemote = Get-Remote; $script:LastRemote.pair.stage -eq "compare" } 10 "сверка числа в окне" 300
        Assert-That ([string]$script:LastRemote.pair.sas -eq $sas) "Число сверки окна '$($script:LastRemote.pair.sas)', у телефона '$sas'"
        Assert-Reply (Send-Parent "RemotePairConfirm" @{ match = $true }) "RemotePairConfirm"
        Wait-Until { $script:LastRemote = Get-Remote; $null -ne $script:LastRemote.phone } 10 "телефон подключён" 300
        $script:ChildId = Get-ChildId
      }
      $c = Invoke-Phone @("confirm")
      Assert-That ($c.Code -eq 0) "confirm на телефоне: код $($c.Code)"
    }

    Step "(а) «Молодец!» со стикером 033 и звуком 2: окно не позже 5 с, стикер 033 движется, звук 2 один раз" {
      $what = "Стикер 033, звук 2"
      $before = @(Get-NoteSounds (Get-GuardState)).Count
      $n = Send-PhoneNote "Молодец!" @("-sticker", "033", "-sound", "2") $what
      $script:IdA = $n.Id
      $left = [math]::Max(0.3, 5 - $n.Watch.Elapsed.TotalSeconds)
      $g = Wait-GuardState { param($s) [string]$s.noteSticker -eq "033" } $left "$what`: стикер 033 в окне не позже 5 с после ответа телефону"
      Assert-That (-not $g.lockShown) "$what`: экран блокировки при сообщении: $(Format-GuardState $g)"
      $frames = @()
      for ($i = 0; $i -lt 4; $i++) {
        $s = Get-GuardState
        Assert-That ($s.noteId -eq $script:IdA -and [string]$s.noteSticker -eq "033") "$what`: стикер пропал или окно закрылось: стикер '$($s.noteSticker)', id '$($s.noteId)'"
        $frames += [int]$s.noteFrame
        if ($i -lt 3) { Start-Sleep -Milliseconds 300 }
      }
      $distinct = @($frames | Sort-Object -Unique).Count
      Write-Log "$what`: кадры стикера за 0,9 с: $($frames -join ', ')"
      # Ruling S60-1: на раннере GitHub анимации Windows выключены, а при выключенных анимациях стикер
      # по правилу R26 стоит на первом кадре. Тогда сценарий проверяет именно это, а движение кадров
      # в настоящем окне проверяет Windows-тест TestNoteWindowStickerAnimates (windows.yml, «Тесты Go на Windows»).
      if ([PcbScreenInk]::Animations()) {
        Assert-That ($distinct -ge 2) "$what`: кадр стикера не меняется за 0,9 с (кадры $($frames -join ', ')) при включённых анимациях Windows"
      } else {
        Write-Log "$what`: анимации Windows выключены, стикер должен стоять на первом кадре (R26)"
        Assert-That ($distinct -eq 1 -and $frames[0] -eq 0) "$what`: анимации Windows выключены, а кадры стикера $($frames -join ', '): ждали только 0 (R26)"
      }
      Assert-OneNoteSound $before 2 $what
      Wait-NoteState $script:IdA "shown" 10
      Assert-NoteFits (Get-NoteRect) $what
      [void](Get-ScreenShot "s60-note-sticker")
      [void](Close-PhoneNote $script:IdA $what)
    }

    Step "(б) без стикера и «Без звука»: окно без стикера, звука нет" {
      $what = "Без стикера, без звука"
      $before = @(Get-NoteSounds (Get-GuardState)).Count
      $n = Send-PhoneNote "Ложись спать" @("-sound", "0") $what
      Assert-That ([string]$n.State.noteSticker -eq "") "$what`: в окне стикер '$($n.State.noteSticker)', ждали без стикера"
      while ($n.Watch.Elapsed.TotalSeconds -lt $n.ShownAt + 3) {
        $s = Get-GuardState
        Assert-That ($s.noteId -eq $n.Id) "$what`: окно закрылось само: $(Format-GuardState $s)"
        Assert-That ([string]$s.noteSticker -eq "") "$what`: в окне появился стикер '$($s.noteSticker)'"
        $new = @(Get-NoteSounds $s | Select-Object -Skip $before)
        Assert-That ($new.Count -eq 0) "$what`: при «Без звука» сыграно: $(Format-NoteSounds $new)"
        Start-Sleep -Milliseconds 250
      }
      Write-Log "$what`: за 3 с после показа звуков сообщения нет, стикера нет"
      Assert-NoteFits (Get-NoteRect) $what
      [void](Close-PhoneNote $n.Id $what)
    }

    Step "(в) стикер 999 и звук 99, которых программа не знает: окно без стикера, звук 1" {
      $what = "Стикер 999, звук 99"
      $before = @(Get-NoteSounds (Get-GuardState)).Count
      $n = Send-PhoneNote "Привет" @("-sticker", "999", "-sound", "99") $what
      Assert-OneNoteSound $before 1 $what
      $s = Get-GuardState
      Assert-That ($s.noteId -eq $n.Id) "$what`: окно закрылось само: $(Format-GuardState $s)"
      Assert-That ([string]$s.noteSticker -eq "") "$what`: в окне стикер '$($s.noteSticker)', ждали без стикера (999 не встроен)"
      Assert-NoteFits (Get-NoteRect) $what
      [void](Close-PhoneNote $n.Id $what)
    }

    Step "Время вышло: экран блокировки" {
      Resume-Clock
      $g = Wait-LockShown 40 "timeUp"
      Suspend-Clock
      Write-Log "Экран блокировки: $(Format-GuardState $g)"
    }

    Step "(г) со стикером поверх экрана блокировки: окно не закрывает заголовок и кнопки экрана" {
      $what = "Стикер поверх экрана блокировки"
      Start-Sleep -Milliseconds 700
      $ctl0 = Get-LockControlsTop
      $pwd0 = (@(Get-PcbWindowRects -Children | Where-Object { $_.Class -eq "Button" -and $_.Text -eq "Пароль родителя" }) | Select-Object -First 1)
      Assert-That ($null -ne $pwd0) "Кнопки «Пароль родителя» на экране блокировки нет среди окон"
      $screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
      $title0 = @(Get-InkBands (Get-ScreenShot) $screen.Top $ctl0) | Select-Object -First 1
      Assert-That ($null -ne $title0 -and ($title0.Bottom - $title0.Top) -ge 8) "Заголовок экрана блокировки не найден на снимке над кнопками (верх кнопок $ctl0)"
      Write-Log "Заголовок экрана блокировки до сообщения: $(Format-Rect $title0)"

      $before = @(Get-NoteSounds (Get-GuardState)).Count
      $n = Send-PhoneNote "Молодец, отдыхай!" @("-sticker", "033") $what
      $g = $n.State
      Assert-That ($g.lockShown -and $g.lockKind -eq "timeUp") "$what`: экран блокировки снят сообщением: $(Format-GuardState $g)"
      Assert-That ((Test-HasButton $g "parentPassword") -and (Test-HasButton $g "noteOk")) "$what`: нет «Пароль родителя» или «Понял»: $(Format-GuardState $g)"
      Wait-NoteState $n.Id "shown" 10
      Start-Sleep -Milliseconds 700
      $g = Get-GuardState
      Write-Log "$what`: стикер '$($g.noteSticker)' (на низком экране окно над блокировкой может быть без стикера), кадр $($g.noteFrame)"
      $note = Get-NoteRect
      Assert-NoteFits $note $what
      $buttons = @(Get-PcbWindowRects -Children | Where-Object { $_.Class -eq "Button" })
      Write-Log ("{0}: кнопки {1}" -f $what, (($buttons | ForEach-Object { "$($_.Text) $(Format-Rect $_)" }) -join '; '))
      foreach ($b in $buttons) {
        Assert-That (-not (Test-RectsOverlap $note $b)) "$what`: окно сообщения $(Format-Rect $note) закрывает кнопку «$($b.Text)» $(Format-Rect $b)"
        Assert-That ($b.Top -ge $screen.Top -and $b.Bottom -le $screen.Bottom) "$what`: кнопка «$($b.Text)» $(Format-Rect $b) за краем экрана: экран блокировки раздвинут слишком"
      }
      $ctl1 = Get-LockControlsTop
      $title1 = @(Get-InkBands (Get-ScreenShot "s60-note-sticker-lock") $screen.Top $ctl1 $note) | Select-Object -First 1
      Assert-That ($null -ne $title1) "$what`: заголовка экрана блокировки не видно: над кнопками (верх $ctl1) без окна сообщения $(Format-Rect $note) текста нет"
      $h0 = $title0.Bottom - $title0.Top; $w0 = $title0.Right - $title0.Left
      $h1 = $title1.Bottom - $title1.Top; $w1 = $title1.Right - $title1.Left
      Write-Log ("{0}: заголовок {1} (до сообщения {2}), окно сообщения {3}" -f $what, (Format-Rect $title1), (Format-Rect $title0), (Format-Rect $note))
      Assert-That (-not $title1.Touch -and [math]::Abs($h1 - $h0) -le 2 -and [math]::Abs($w1 - $w0) -le 6) ("{0}: окно сообщения {1} закрывает заголовок экрана блокировки: видна полоса {2} ({3}x{4} px{5}), заголовок до сообщения {6} ({7}x{8} px)" -f $what, (Format-Rect $note), (Format-Rect $title1), $w1, $h1, $(if ($title1.Touch) { ', вплотную к окну' } else { '' }), (Format-Rect $title0), $w0, $h0)
      Assert-That (-not (Test-RectsOverlap $note $title1)) "$what`: окно сообщения $(Format-Rect $note) пересекает заголовок $(Format-Rect $title1)"
      Assert-OneNoteSound $before 1 "$what (телефон без поля звука)"

      $g = Close-PhoneNote $n.Id $what
      Assert-That ($g.lockShown) "$what`: экран блокировки пропал вместе с окном сообщения: $(Format-GuardState $g)"
      Start-Sleep -Milliseconds 700
      $back = (@(Get-PcbWindowRects -Children | Where-Object { $_.Class -eq "Button" -and $_.Text -eq "Пароль родителя" }) | Select-Object -First 1).Top
      Assert-That ([math]::Abs($back - $pwd0.Top) -le 1) "$what`: кнопки экрана блокировки не вернулись на место после «Понял»: «Пароль родителя» сверху $back, было $($pwd0.Top)"
    }

    Step "Аварийный выход на месте: «Пароль родителя» открывает панель пароля" {
      Invoke-GuardClick "parentPassword"
      $g = Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "password" }).Count -ge 1 } 8 "панель пароля"
      Assert-That (Test-HasButton $g "submit") "На панели пароля нет «Продолжить»: $(Format-GuardState $g)"
    }
  } catch {
    Write-RelayLog
    throw
  } finally {
    Stop-Relay
  }

  Write-Log "S60: стикер 033 движется и звук 2 сыгран один раз, без стикера и без звука тихо, неизвестные 999 и 9 дали окно без стикера и звук 1, поверх экрана блокировки заголовок и кнопки видны"
}
