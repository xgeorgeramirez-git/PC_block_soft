# S65. Плашки про программы и сайты в стиле «Игровое лобби» со стикером и звуком Windows (8.2.4, 8.2.5, D-111,
# D-112, D-114, FB-38).
# Часы ядра на паузе (Start-Child): наблюдатель и сроки плашек идут по настоящим часам. Тестовые программы — копии
# системной программы в папке WatchDir (как S14): SuperGame.exe и MegaGame.exe не в списке (незнакомые),
# bloknot-copy.exe — копия node.exe раннера (копия notepad.exe под этим именем на раннере не стартует: «side-by-side
# configuration is incorrect»; подписанный node.exe с обычным окном консоли, как TMP1, PL-74), «Запрещено» шагом
# мастера, название на плашке — из ответа программы (ListPrograms, PL-22); ручной запуск (как S45) — передний план PowerShell подменой
# TestGuard foreground, ввод идёт). Сайт example.com — в «Запрещённых», в hosts раннера ведёт на 127.0.0.1 (шаг
# windows.yml «Brave для пробы и приёмки»), открывается в Brave раннера (как S17).
# Родительская сессия в сеансе ребёнка прячет плашки (Ruling 19): каждая команда родителя — вход, команда, выход.
# Дано: ребёнок на компьютере, SuperGame незнакома, «Блокнот-копия» запрещена, example.com в «Запрещённых».
# Когда: ребёнок запускает обе программы и открывает сайт; родитель позже разрешает SuperGame из своего окна (без
# «Попросить сейчас»); затем MegaGame: «Попросить сейчас», окно родителя этого сеанса, «Игра».
# Тогда: плашки «Ждёт разрешения» (стикер 031, «Попросить сейчас»), «Эта программа запрещена» (110), «Сайт закрыт»
# (110), «Можно играть!» (033); звук схемы Windows по журналу стража: «Восклицание» (SystemExclamation) у первых
# трёх, «Уведомление» (Notification.Default) у последней; после решения SuperGame запускается; после «Попросить
# сейчас» и «Игра» в этом сеансе плашки решения нет, MegaGame запускается (8.2.4 п. 2). Анимации Windows выключены
# (раннер) — стикер стоит на первом кадре (plateFrame 0, R26, PL-89), включены — кадры сменяются.
# Снимки: s65-unknown, s65-forbidden, s65-site, s65-decided, s65-asknow-window (окно родителя после «Попросить сейчас»).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

if (-not ('PcbS65Anim' -as [type])) {
  Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class PcbS65Anim {
  [DllImport("user32.dll")] static extern bool SystemParametersInfo(uint action, uint param, ref int value, uint flags);
  // Анимации Windows (SPI_GETCLIENTAREAANIMATION) у пользователя сценария: выключены - стикер стоит на первом кадре.
  public static bool On() { int v = 0; SystemParametersInfo(0x1042, 0, ref v, 0); return v != 0; }
}
'@
}

# Тексты службы (internal/service/texts.go textPlate*, handlers_m3.go decidedPlateText) — ТЗ 8.2.4, 8.2.5 дословно.
$script:UnknownTitle = "Ждёт разрешения"
$script:ForbiddenTitle = "Эта программа запрещена"
$script:SiteTitle = "Сайт закрыт"
$script:AllowedTitle = "Можно играть!"
function Get-UnknownText([string]$Name) { return "Программа «$Name» новая. Родитель должен решить, можно ли её запускать" }

# Подмена переднего плана в страже путём Path; ждёт, пока страж пришлёт его службе (как S45).
function Set-ForegroundPath([string]$Path) {
  Invoke-TestGuard "foreground" "" $Path
  $script:FgOverride = $true
  $want = $Path.ToLowerInvariant()
  [void](Wait-GuardState { param($s) ([string]$s.foregroundPath).ToLowerInvariant() -eq $want } 5 "подмена переднего плана в страже: $want")
  Write-Log "Передний план (подмена): $Path"
}

# Кадры стикера плашки за 0,9 с: анимации выключены — только 0 (R26, PL-89), включены — не меньше двух разных.
function Assert-PlateFrames([string]$Kind, [string]$What) {
  $frames = @()
  for ($i = 0; $i -lt 4; $i++) {
    $s = Get-GuardState
    Assert-That ([string]$s.plateKind -eq $Kind -and [string]$s.plateSticker) "${What}: плашка ушла или стикера нет: $(Format-GuardState $s)"
    $frames += [int]$s.plateFrame
    if ($i -lt 3) { Start-Sleep -Milliseconds 300 }
  }
  $distinct = @($frames | Sort-Object -Unique).Count
  if ($script:AnimOn) {
    Write-Log "${What}: анимации Windows включены, кадры стикера за 0,9 с: $($frames -join ', ')"
    Assert-That ($distinct -ge 2) "${What}: кадр стикера не меняется за 0,9 с (кадры $($frames -join ', ')) при включённых анимациях Windows"
  } else {
    Write-Log "${What}: анимации Windows выключены, стикер на первом кадре (R26): кадры $($frames -join ', ')"
    Assert-That ($distinct -eq 1 -and $frames[0] -eq 0) "${What}: анимации Windows выключены, а кадры стикера $($frames -join ', '): ждали только 0 (R26)"
  }
}

function Get-SoundsBefore { return @(Get-SystemSounds (Get-GuardState)).Count }

Invoke-Scenario "S65" "Плашки «Игрового лобби»: ждёт разрешения, запрещена, сайт закрыт, можно играть; звук Windows; «Попросить сейчас» без плашки решения" {
  Initialize-Scenario 60
  try {
    Step "Имитация ввода доходит до сеанса; Brave установлен; анимации Windows" {
      Test-InputSim
      [void](Assert-BraveInstalled)
      $script:AnimOn = [PcbS65Anim]::On()
      Write-Log "Анимации Windows у пользователя сценария: $(if ($script:AnimOn) { 'включены' } else { 'выключены (стикер на первом кадре)' })"
    }

    Step "Тестовые программы: SuperGame.exe, MegaGame.exe (незнакомые), bloknot-copy.exe (запрещена мастером)" {
      $script:SgPath = New-TestApp "SuperGame"
      $script:MgPath = New-TestApp "MegaGame"
      # Запрещённой программе нужно реально стартовать (наблюдатель закрывает живой процесс): копия node.exe, как TMP1.
      $node = (Get-Command node.exe -ErrorAction SilentlyContinue).Source
      Assert-That ($node -and (Test-Path -LiteralPath $node)) "На раннере нет node.exe для «Блокнот-копии»"
      $script:BnPath = Copy-TestExe $node $script:Apps "bloknot-copy"
      Write-Log "Тестовая программа bloknot-copy.exe: копия $node, $($script:BnPath)"
    }

    Step "Настройка: лимит 10 ч, bloknot-copy «Запрещено»; example.com в «Запрещённых»" {
      Start-Child 600 $true -1 @(@{ path = $script:BnPath; category = "blocked" })
      Wait-ProgramsReady
      $script:BnName = Get-ProgramName $script:BnPath
      Write-Log "Название bloknot-copy.exe из ответа программы: «$($script:BnName)»"
      $script:F = Add-PcbSite "example.com" "blocked"
      Assert-That ($script:F -eq "example.com") "AddSite вернул домен «$($script:F)», ждали example.com"
    }

    Step "SuperGame: закрыта, «Ждёт разрешения» — текст 8.2.4 п. 1, стикер 031, «Попросить сейчас», «Восклицание»; снимок" {
      $before = Get-SoundsBefore
      $p = Start-TestApp $script:SgPath
      [void](Assert-ClosedFast $p "запуск SuperGame.exe")
      $g = Wait-LobbyPlate "unknown" $script:UnknownTitle "031" $script:SoundExclamation $before "SuperGame" (Get-UnknownText "SuperGame")
      Assert-That (Test-HasButton $g "askNow") "На плашке «$($script:UnknownTitle)» нет «Попросить сейчас»: $(Format-GuardState $g)"
      Assert-That (-not $g.lockShown) "Экран блокировки при незнакомой программе: $(Format-GuardState $g)"
      Assert-PlateFrames "unknown" "Плашка «$($script:UnknownTitle)»"
      Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
      Save-Screen "s65-unknown"
    }

    Step "Блокнот-копия вручную (ввод идёт, впереди PowerShell): закрыта, «Эта программа запрещена», стикер 110, «Восклицание»; снимок" {
      Set-ForegroundPath $script:PsExe
      Start-Sleep -Seconds 2 # страж успевает прислать снимок с вводом и передним планом
      $before = Get-SoundsBefore
      $p = Start-TestApp $script:BnPath
      [void](Assert-ClosedFast $p "ручной запуск bloknot-copy.exe")
      $text = "Родитель запретил «$($script:BnName)». Выбери другую игру"
      $g = Wait-LobbyPlate "forbidden" $script:ForbiddenTitle "110" $script:SoundExclamation $before "Блокнот-копия" $text
      Assert-PlateFrames "forbidden" "Плашка «$($script:ForbiddenTitle)»"
      Start-Sleep -Milliseconds 500
      Save-Screen "s65-forbidden"
      Invoke-TestGuard "foreground"
      $script:FgOverride = $false
    }

    Step "example.com в Brave: вкладка закрыта, «Сайт закрыт» — текст 8.2.5 п. 1, стикер 110, «Восклицание»; снимок" {
      Open-BraveHome
      $g0 = Get-GuardState
      Assert-That ([string]$g0.closedTab -ne "example.com") "До открытия closedTab уже «$($g0.closedTab)» (PL-16)"
      $before = @(Get-SystemSounds $g0).Count
      $sw = Open-Brave "http://example.com/"
      [void](Wait-TabClosed "example.com" $sw)
      $text = "$($script:F) в списке запрещённых. Если он нужен, попроси родителей"
      [void](Wait-SitePlate $text "site" 5 -Title $script:SiteTitle -Sticker "110" -Sound $script:SoundExclamation -SoundsBefore $before)
      Assert-PlateFrames "site" "Плашка «$($script:SiteTitle)»"
      Start-Sleep -Milliseconds 500
      Save-Screen "s65-site"
      Stop-Brave
    }

    Step "Родитель разрешил SuperGame из своего окна (без «Попросить сейчас»): «Можно играть!», стикер 033, «Уведомление»; снимок" {
      $before = Get-SoundsBefore
      $rep = Set-ProgramCategory $script:SgPath "games"
      Assert-That ($rep.message -eq "SuperGame: игра") "Сообщение родителю «$($rep.message)», ждали «SuperGame: игра»"
      $text = "Родитель разрешил «SuperGame»: это игра. Запусти её снова"
      [void](Wait-LobbyPlate "decided" $script:AllowedTitle "033" $script:SoundNotify $before "Решение о SuperGame" $text)
      $new = @(Get-SystemSounds (Get-GuardState) | Select-Object -Skip $before)
      Assert-That (@($new | Where-Object { [string]$_.system -eq $script:SoundExclamation }).Count -eq 0) "У плашки «$($script:AllowedTitle)» запрошено «Восклицание»: $(Format-SystemSounds $new)"
      Assert-PlateFrames "decided" "Плашка «$($script:AllowedTitle)»"
      Start-Sleep -Milliseconds 500
      Save-Screen "s65-decided"
    }

    Step "SuperGame после решения: запускается и работает" {
      $script:Sg = Start-TestApp $script:SgPath
      Start-Sleep -Seconds 2
      Assert-AppAlive $script:Sg "SuperGame.exe после «Можно играть!»"
    }

    Step "MegaGame: «Ждёт разрешения», «Попросить сейчас» — окно родителя на «Программах» этого сеанса, пароль; снимок" {
      $before = Get-SoundsBefore
      $p = Start-TestApp $script:MgPath
      [void](Assert-ClosedFast $p "запуск MegaGame.exe")
      [void](Wait-LobbyPlate "unknown" $script:UnknownTitle "031" $script:SoundExclamation $before "MegaGame" (Get-UnknownText "MegaGame"))
      Invoke-GuardClick "askNow"
      Wait-Until { @(Get-SessionProcs "--screen=programs").Count -ge 1 } 40 "служба запустила окно родителя с --screen=programs"
      $wp = @(Get-SessionProcs "--screen=programs")[0]
      Write-Log "Окно родителя после «Попросить сейчас»: PID $($wp.ProcessId), $($wp.CommandLine)"
      Assert-That ($wp.CommandLine -match '--parent') "Окно на «Программах» не окно родителя: $($wp.CommandLine)"
      $proc = Get-Process -Id ([int]$wp.ProcessId)
      try {
        Wait-Until { $proc.Refresh(); $proc.HasExited -or ($proc.MainWindowHandle -ne [IntPtr]::Zero -and $proc.MainWindowTitle -ne "") } 40 "окно родителя (PID $($proc.Id))"
        Start-Sleep -Seconds 4
        $proc.Refresh()
        [void][PcbNative]::ForceForeground($proc.MainWindowHandle)
        Start-Sleep -Milliseconds 800
        # Пароль с клавиатуры (поле пароля в фокусе при открытии), как Save-ParentWindow: вход не удался — снимок
        # покажет экран пароля; решение «Игра» ниже идёт командой родителя из этого же сеанса.
        try {
          [System.Windows.Forms.SendKeys]::SendWait($script:ParentPassword)
          [System.Windows.Forms.SendKeys]::SendWait("{ENTER}")
          Write-Log "Окно родителя: пароль введён с клавиатуры"
        } catch { Write-Log "Окно родителя: пароль не введён: $_" }
        [void](Save-ProcWindow $proc "s65-asknow-window" 10 4)
      } catch {
        Write-Log "Снимок окна родителя не сделан: $($_.Exception.Message)"
      } finally {
        try {
          if (-not $proc.HasExited) {
            [void]$proc.CloseMainWindow()
            if (-not $proc.WaitForExit(10000)) { Stop-Process -Id $proc.Id -Force }
          }
        } catch {}
      }
    }

    Step "«Игра» для MegaGame в этом сеансе: плашки решения нет (8.2.4 п. 2, D-114), MegaGame запускается" {
      $rep = Set-ProgramCategory $script:MgPath "games"
      Assert-That ($rep.message -eq "MegaGame: игра") "Сообщение родителю «$($rep.message)», ждали «MegaGame: игра»"
      # Плашка решения пришла бы со следующим опросом стража (раз в секунду) и держится 10 с (PL-16).
      $sw = [Diagnostics.Stopwatch]::StartNew()
      while ($sw.Elapsed.TotalSeconds -lt 6) {
        $g = Get-GuardState
        Assert-That (-not ([string]$g.plateKind -eq "decided" -and ([string]$g.plate).Contains("MegaGame"))) "После «Попросить сейчас» и решения в этом сеансе показана плашка решения: $(Format-GuardState $g)"
        Start-Sleep -Milliseconds 150
      }
      Write-Log "Плашки решения о MegaGame нет 6 с после «Игра»"
      $script:Mg = Start-TestApp $script:MgPath
      Start-Sleep -Seconds 2
      Assert-AppAlive $script:Mg "MegaGame.exe после «Попросить сейчас» и «Игра»"
    }

    Step "Звуки схемы Windows по журналу стража" {
      $all = @(Get-SystemSounds (Get-GuardState))
      Write-Log "Звуки схемы Windows за сценарий: $(Format-SystemSounds $all)"
    }
  } finally {
    if ($script:FgOverride) { try { Invoke-TestGuard "foreground" } catch {}; $script:FgOverride = $false }
    Stop-Brave
  }

  Write-Log "S65: плашки «Игрового лобби» 031, 110, 110, 033 со звуками Windows; «Попросить сейчас» и «Игра» в этом сеансе — без плашки решения"
}
