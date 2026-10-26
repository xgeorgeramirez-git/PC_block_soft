# S34. Табло остатка у значка (D-53, FR-8, ТЗ 8.2.1 п.3, 8.2.6 п.1).
# Дано: ребёнок работает за компьютером, лимит 90 мин. Когда: открыто табло у значка (в тестовом режиме
# его открывает событие Local\PCBlockTrayCard, как щелчок по значку). Тогда: карточка "ОСТАЛОСЬ ВРЕМЕНИ:"
# с плитками; через 3 с остаток по данным службы меньше на 2...5 с, а карточка всё ещё открыта; Esc
# закрывает карточку; стрелка (Enter на кнопке со стрелкой) открывает окно "Моя статистика" на "Сегодня".
# Часы ядра идут в настоящем времени (ускорение 1): секунды на плитках должны идти вживую.
# Снимки: s34-tray-countdown-1.png и s34-tray-countdown-2.png (два кадра с разницей около 3 с),
# s34-mystats-today.png (окно "Моя статистика").
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Взводит событие тестового режима: значок открывает табло так же, как по щелчку мыши.
function Send-OpenTrayCard {
  $ev = [System.Threading.EventWaitHandle]::OpenExisting("Local\PCBlockTrayCard")
  try { [void]$ev.Set() } finally { $ev.Dispose() }
}

# Видимые окна табло (класс PCBlockTrayCard) в процессах значка этого сеанса.
function Get-CardWindows {
  $pids = @(Get-SessionProcs "--tray" | ForEach-Object { [uint32]$_.ProcessId })
  if ($pids.Count -eq 0) { return @() }
  return @([PcbNative]::VisibleWindows($pids) | Where-Object { $_ -match '\|PCBlockTrayCard\|' })
}

# Открывает табло и ждёт, пока оно появится и получит фокус (Esc и Enter идут окну переднего плана).
function Open-CardAndFocus {
  Wait-Until { try { Send-OpenTrayCard; $true } catch { $false } } 40 "событие тестового режима значка (Local\PCBlockTrayCard)"
  # Значок узнаёт о настроенном ребёнке при очередном опросе (до 5 с); до этого он считает учётку
  # не детской и открывает мини-панель. Событие шлём повторно, пока не откроется именно табло.
  Start-Sleep -Seconds 6
  $tries = 0
  Wait-Until {
    if ((Get-CardWindows).Count -ge 1) { return $true }
    if (($tries % 30) -eq 0) { try { Send-OpenTrayCard } catch {} }
    $tries++
    $false
  } 45 "табло у значка открылось"
  Wait-Until { [PcbNative]::Foreground() -match '\|PCBlockTrayCard\|' } 10 "табло получило фокус (передний план: $([PcbNative]::Foreground()))"
}

Invoke-Scenario "S34" "Табло остатка у значка: секунды идут, Esc закрывает, стрелка открывает Мою статистику" {
  Initialize-Scenario 1

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Настройка: лимит 90 мин, ребёнок работает, время считается" {
    Start-Input 1000
    Invoke-Setup 90
    Wait-SessionParts
    Wait-Until { $s = Get-Status; $s.limitS -eq 5400 -and $s.usedS -ge 2 } 60 "учёт времени пошёл (лимит 5400 с)"
    $st = Get-Status
    Write-Log "Ребёнок работает: остаток $($st.remainingS) с, использовано $($st.usedS) с"
    Assert-That ($st.remainingS -ge 5000 -and $st.remainingS -le 5398) "Остаток $($st.remainingS) с, ждали около 5400 минус пара секунд"
  }

  Step "Табло открыто у значка: два кадра с разницей около 3 с, остаток по данным службы уменьшился" {
    Open-CardAndFocus
    Start-Sleep -Milliseconds 700
    $before = Get-Status
    Save-Screen "s34-tray-countdown-1"
    Start-Sleep -Seconds 3
    Save-Screen "s34-tray-countdown-2"
    $after = Get-Status
    $dropped = $before.remainingS - $after.remainingS
    Write-Log "Остаток на первом кадре $($before.remainingS) с, на втором $($after.remainingS) с, разница $dropped с"
    Assert-That ($dropped -ge 2 -and $dropped -le 6) "За 3 с остаток уменьшился на $dropped с, ждали от 2 до 6"
    Assert-That ((Get-CardWindows).Count -eq 1) "Через 3 с табло закрылось само"
  }

  Step "Esc закрывает табло" {
    [System.Windows.Forms.SendKeys]::SendWait("{ESC}")
    Wait-Until { (Get-CardWindows).Count -eq 0 } 5 "Esc закрыл табло"
  }

  Step "Стрелка (Enter на кнопке со стрелкой) открывает окно Моя статистика" {
    Start-Sleep -Milliseconds 800
    Open-CardAndFocus
    [System.Windows.Forms.SendKeys]::SendWait("{ENTER}")
    Wait-Until { @(Get-SessionProcs "--mystats").Count -ge 1 } 40 "служба запустила окно Моя статистика"
    Wait-Until { (Get-CardWindows).Count -eq 0 } 5 "табло закрылось после открытия окна"
    $proc = Get-Process -Id ([int]@(Get-SessionProcs "--mystats")[0].ProcessId)
    Wait-Until { $proc.Refresh(); $proc.HasExited -or ($proc.MainWindowHandle -ne [IntPtr]::Zero -and $proc.MainWindowTitle -ne "") } 40 "окно Моя статистика показано"
    Assert-That (-not $proc.HasExited) "Окно Моя статистика закрылось само, код $($proc.ExitCode)"
    Start-Sleep -Seconds 5
    $proc.Refresh()
    $h = $proc.MainWindowHandle
    [void][PcbNative]::ForceForeground($h)
    Start-Sleep -Milliseconds 800
    $f = [PcbNative]::WindowFrame($h)
    $w = $f[2] - $f[0]
    $hgt = $f[3] - $f[1]
    Assert-That ($w -gt 100 -and $hgt -gt 100) "Окно Моя статистика имеет размер ${w}x${hgt}"
    $bmp = New-Object System.Drawing.Bitmap $w, $hgt
    try {
      $g = [System.Drawing.Graphics]::FromImage($bmp)
      try { $g.CopyFromScreen($f[0], $f[1], 0, 0, (New-Object System.Drawing.Size $w, $hgt)) } finally { $g.Dispose() }
      $bmp.Save((Join-Path $script:ShotDir "s34-mystats-today.png"), [System.Drawing.Imaging.ImageFormat]::Png)
    } finally { $bmp.Dispose() }
    Write-Log "Снимок окна: s34-mystats-today.png, заголовок «$($proc.MainWindowTitle)», ${w}x${hgt}"
  }

  Write-Log "S34: табло открылось у значка, остаток уменьшался вживую, Esc закрыл, стрелка открыла Мою статистику"
}
