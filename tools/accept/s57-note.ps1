# S57. Сообщение ребёнку с телефона: плашка «Сообщение от родителя», «Доставлено» и «Прочитано», поверх экрана
# блокировки, подделка не показывается (FR-41, D-97, протокол связи, раздел 5а).
# Сервер связи pcblock-relay.exe на раннере (127.0.0.1:18080), тестовый телефон remote-phone.exe. Часы ядра идут
# в 60 раз быстрее: лимит 5 мин кончается примерно через 5 с настоящего времени (часы на паузе до шага блокировки).
# Дано: телефон привязан (как S55), ребёнок на компьютере. Когда: сообщение «Иди ужинать» с показом «Пока не
# нажмёт «Понял»». Тогда: не позже 5 с плашка с текстом и кнопкой «Понял», звук; отчёт телефону — shown; «Понял» —
# read; в журнале «Тест: сообщение с телефона: «Иди ужинать»». Когда: сообщение на 10 с при экране блокировки.
# Тогда: оно видно поверх экрана и через 10 с исчезает, отметка read. Сообщение с чужой подписью не показывается.
# Журнал читается только после закрытия плашки: родительская сессия в сеансе ребёнка прячет плашки (Ruling 19).
# Затем длинное сообщение (300 знаков, 5 переводов строк) поверх экрана блокировки: плашка не закрывает ни одной
# кнопки экрана («Пароль родителя» видна, NFR-10), не выходит за экран и не выше 55 % его (окно со стикером и тенью, Ruling FB28-1), «Понял» её закрывает.
# Текст целиком (FR-41, ТЗ 8.2) — по признаку стража: noteTextFits (высота текста, намеренная шрифтом окна в его
# ширину, не больше места текста, и место на экране); шрифт короткого — 42 px при 100 % (D-107), длинного — не мельче
# 14 px и не крупнее короткого (лестница noteFontSteps). Пиксели пересчитываются по dpi системы, как Scaled у стража.
# Снимки: s57-note.png (плашка на рабочем столе), s57-note-lock.png (плашка поверх экрана блокировки),
# s57-note-long-lock.png (длинное сообщение поверх экрана блокировки).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

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

# Dpi системы (процесс приёмки знает о dpi, SetProcessDPIAware в common.ps1) — как systemDPI стража.
function Get-SystemDpi {
  $gr = [System.Drawing.Graphics]::FromHwnd([IntPtr]::Zero)
  try { return [int]$gr.DpiY } finally { $gr.Dispose() }
}

# Размер px при 96 dpi в пикселях экрана при dpi — как Scaled стража: (px·dpi + 48) / 96 нацело.
function Get-ScaledPx([int]$Px, [int]$Dpi) { return [int][math]::Floor(($Px * $Dpi + 48) / 96) }

Invoke-Scenario "S57" "Сообщение ребёнку с телефона: плашка, «Понял», поверх экрана блокировки, подделка" {
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
      $r = Invoke-Phone @("pair", "-url", $script:RelayUrl, "-code", [string]$script:LastRemote.pair.code, "-name", "Pixel-S57")
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

    $script:Id1 = ""
    Step "«Иди ужинать» до «Понял»: плашка не позже 5 с, звук, отметка «Доставлено»" {
      $before = @(Get-NoteSounds (Get-GuardState)).Count
      $sw = [Diagnostics.Stopwatch]::StartNew()
      $r = Invoke-Phone @("note", "-child", "$($script:ChildId)", "-text", "Иди ужинать", "-show", "0")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Сообщение не принято: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      $script:Id1 = [string]$r.Result.id
      $g = Wait-GuardState { param($s) $s.noteId -eq $script:Id1 } 5 "плашка «Сообщение от родителя» у ребёнка"
      $shownAt = $sw.Elapsed.TotalSeconds
      Write-Log ("Плашка через {0:N2} с (ответ телефону {1} мс): '{2}', кнопки [{3}]" -f $shownAt, $r.Result.ms, $g.note, ((@(List $g.buttons)) -join ','))
      Assert-That ($shownAt -le 5) ("Плашка через {0:N2} с, ждали не больше 5" -f $shownAt)
      Assert-That ([string]$g.note -eq "Иди ужинать") "Текст плашки '$($g.note)', ждали «Иди ужинать»"
      Assert-That (Test-HasButton $g "noteOk") "На плашке нет кнопки «Понял»: $(Format-GuardState $g)"
      Assert-That (-not $g.lockShown) "Экран блокировки при сообщении: $(Format-GuardState $g)"
      $g = Wait-GuardState { param($s) @(Get-NoteSounds $s).Count -gt $before } 5 "звук плашки сообщения"
      $snd = @(Get-NoteSounds $g) | Select-Object -Last 1
      Write-Log "Звук плашки сообщения: ok=$($snd.ok)"
      Wait-NoteState $script:Id1 "shown" 10
      Write-Log "Отметка в отчёте телефону: $($script:LastNoteState) через $([math]::Round($script:WaitElapsed, 1)) с"
      Start-Sleep -Milliseconds 500
      Save-Screen "s57-note"
      # Без «Понял» плашка держится.
      Start-Sleep -Seconds 3
      $g = Get-GuardState
      Assert-That ($g.noteId -eq $script:Id1) "Плашка «до «Понял»» исчезла сама: $(Format-GuardState $g)"
    }

    Step "«Понял»: плашка закрыта, отметка «Прочитано»" {
      Invoke-GuardClick "noteOk"
      $g = Wait-GuardState { param($s) -not $s.noteId } 5 "плашка закрыта после «Понял»"
      Wait-NoteState $script:Id1 "read" 10
      Write-Log "После «Понял»: отметка $($script:LastNoteState), экран: $(Format-GuardState $g)"
    }

    Step "Журнал родителя: «Тест: сообщение с телефона: «Иди ужинать»»" {
      Use-Parent {
        $found = Find-Events 'сообщение с телефона: «Иди ужинать»'
        Write-Log "Журнал: $((Get-Events | Where-Object { $_.text -match 'сообщение с телефона' } | ForEach-Object { $_.text }) -join ' | ')"
        Assert-That ($found.Count -eq 1) "Записей «сообщение с телефона: «Иди ужинать»»: $($found.Count), ждали одну"
      }
    }

    Step "Сообщение с чужой подписью: отказ, не показано, журнал без записи" {
      $f = Invoke-Phone @("forge-note", "-child", "$($script:ChildId)", "-text", "Чужое сообщение", "-show", "0")
      Assert-That ($f.Code -eq 2 -and -not $f.Result.ok -and $f.Result.error -eq "bad_sig") "Подделка: код $($f.Code), $($f.Result | ConvertTo-Json -Compress)"
      $sw = [Diagnostics.Stopwatch]::StartNew()
      while ($sw.Elapsed.TotalSeconds -lt 6) {
        $g = Get-GuardState
        Assert-That (-not $g.noteId) "Подделка показана ребёнку: $(Format-GuardState $g)"
        Start-Sleep -Milliseconds 300
      }
      Assert-That ((Get-NoteState $f.Result.id) -eq "") "Подделка в отчёте телефону"
      Use-Parent {
        $n = (Find-Events 'Чужое сообщение').Count
        Assert-That ($n -eq 0) "Подделка в журнале родителя: $n записей"
      }
    }

    $script:Id2 = ""
    Step "Время вышло: экран блокировки" {
      Resume-Clock
      $g = Wait-LockShown 40 "timeUp"
      Suspend-Clock
      Write-Log "Экран блокировки: $(Format-GuardState $g)"
      # Место кнопок без сообщения: короткое сообщение 42 px (D-107, FB-35) тоже раздвигает экран блокировки,
      # поэтому «вернулись на место» сверяется с экраном до первого сообщения, а не с экраном под коротким.
      Start-Sleep -Milliseconds 700
      $script:PwdTop = (@(Get-PcbWindowRects -Children | Where-Object { $_.Class -eq "Button" -and $_.Text -eq "Пароль родителя" }) | Select-Object -First 1).Top
      Assert-That ($null -ne $script:PwdTop) "Кнопки «Пароль родителя» на экране блокировки нет среди окон"
      Write-Log "«Пароль родителя» без сообщения: верх $($script:PwdTop)"
    }

    Step "Сообщение на 10 с при экране блокировки: видно поверх, через 10 с исчезает, отметка «Прочитано»" {
      $r = Invoke-Phone @("note", "-child", "$($script:ChildId)", "-text", "Скоро ужин, собирайся", "-show", "10")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Сообщение не принято: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      $script:Id2 = [string]$r.Result.id
      $g = Wait-GuardState { param($s) $s.noteId -eq $script:Id2 -and $s.noteFontPx -gt 0 } 5 "плашка сообщения поверх экрана блокировки"
      $sw = [Diagnostics.Stopwatch]::StartNew()
      # Шрифт короткого — 42 px при 100 % (D-107), текст целиком по признаку стража.
      $script:Dpi = Get-SystemDpi
      $script:ShortFontPx = [int]$g.noteFontPx
      $want = Get-ScaledPx 42 $script:Dpi
      Write-Log "Короткое поверх экрана блокировки: шрифт $($script:ShortFontPx) px при dpi $($script:Dpi), целиком=$($g.noteTextFits)"
      Assert-That ($script:ShortFontPx -eq $want) "Шрифт короткого сообщения $($script:ShortFontPx) px, ждали $want (42 при 100 %, D-107)"
      Assert-That ($g.noteTextFits -eq $true) "Короткое сообщение не поместилось целиком (noteTextFits): $(Format-GuardState $g)"
      Assert-That ($g.lockShown -and $g.lockKind -eq "timeUp") "Экран блокировки снят сообщением: $(Format-GuardState $g)"
      Assert-That ((Test-HasButton $g "parentPassword") -and (Test-HasButton $g "noteOk")) "Нет «Пароль родителя» или «Понял»: $(Format-GuardState $g)"
      Wait-NoteState $script:Id2 "shown" 10
      $note = @(Get-PcbWindowRects | Where-Object { $_.Class -eq "PCBlockGuardNote" }) | Select-Object -First 1
      Assert-That ($null -ne $note) "Окно плашки сообщения не видно поверх экрана блокировки"
      foreach ($b in @(Get-PcbWindowRects -Children | Where-Object { $_.Class -eq "Button" })) {
        Assert-That (-not (Test-RectsOverlap $note $b)) "Плашка $(Format-Rect $note) закрывает кнопку «$($b.Text)» $(Format-Rect $b)"
      }
      Save-Screen "s57-note-lock"
      $g = Wait-GuardState { param($s) -not $s.noteId } 15 "плашка исчезла по сроку 10 с"
      $gone = $sw.Elapsed.TotalSeconds
      Write-Log ("Плашка поверх экрана исчезла через {0:N1} с после появления у стража" -f $gone)
      Assert-Between $gone 7 13 "Плашка на 10 с видна, с"
      Assert-That ($g.lockShown) "Экран блокировки пропал вместе с плашкой: $(Format-GuardState $g)"
      Wait-NoteState $script:Id2 "read" 10
    }

    Step "Длинное сообщение (300 знаков, 5 переводов строк) поверх экрана блокировки: кнопки видны, «Понял» на экране" {
      $line = ("Иди ужинать скорее, " * 3).Substring(0, 49)
      $long = ((@($line) * 6) -join "`n") + "!"
      Assert-That ($long.Length -eq 300) "Длинный текст $($long.Length) знаков, ждали 300"
      $r = Invoke-Phone @("note", "-child", "$($script:ChildId)", "-text", $long, "-show", "0")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Длинное сообщение не принято: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      $script:Id3 = [string]$r.Result.id
      $g = Wait-GuardState { param($s) $s.noteId -eq $script:Id3 -and $s.noteFontPx -gt 0 } 5 "длинное сообщение поверх экрана блокировки"
      Assert-That ($g.lockShown -and (Test-HasButton $g "parentPassword") -and (Test-HasButton $g "noteOk")) "Нет экрана, «Пароль родителя» или «Понял»: $(Format-GuardState $g)"
      Assert-That (([string]$g.note).Split("`n").Count -eq 6) "Переводы строк потерялись: '$($g.note)'"
      Start-Sleep -Milliseconds 700
      $screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
      $note = @(Get-PcbWindowRects | Where-Object { $_.Class -eq "PCBlockGuardNote" }) | Select-Object -First 1
      Assert-That ($null -ne $note) "Окно плашки сообщения не видно: $((Get-PcbWindowRects | ForEach-Object { $_.Class }) -join ', ')"
      $buttons = @(Get-PcbWindowRects -Children | Where-Object { $_.Class -eq "Button" })
      Write-Log ("Экран {0}×{1}; плашка {2}; кнопки: {3}" -f $screen.Width, $screen.Height, (Format-Rect $note), (($buttons | ForEach-Object { "$($_.Text) $(Format-Rect $_)" }) -join '; '))
      $pwdBtn = @($buttons | Where-Object { $_.Text -eq "Пароль родителя" }) | Select-Object -First 1
      Assert-That ($null -ne $pwdBtn) "Кнопки «Пароль родителя» на экране блокировки нет среди окон"
      foreach ($b in $buttons) {
        Assert-That (-not (Test-RectsOverlap $note $b)) "Плашка $(Format-Rect $note) закрывает кнопку «$($b.Text)» $(Format-Rect $b)"
        Assert-That ($b.Top -ge $screen.Top -and $b.Bottom -le $screen.Bottom) "Кнопка «$($b.Text)» $(Format-Rect $b) за краем экрана: экран блокировки раздвинут слишком"
      }
      # Текст целиком (FR-41, ТЗ 8.2): страж намерил текст шрифтом окна в его ширину, и он встал в своё место на
      # экране; шрифт по лестнице D-107 — не мельче 14 px и не крупнее короткого (длинное мельче, но целиком).
      $min = Get-ScaledPx 14 $script:Dpi
      Write-Log "Длинное: шрифт $($g.noteFontPx) px (короткое $($script:ShortFontPx), не мельче $min), целиком=$($g.noteTextFits)"
      Assert-That ($g.noteTextFits -eq $true) "Длинное сообщение обрезано: текст не помещается в своё место (noteTextFits), шрифт $($g.noteFontPx) px, плашка $(Format-Rect $note)"
      Assert-That ([int]$g.noteFontPx -ge $min) "Шрифт длинного сообщения $($g.noteFontPx) px мельче $min (14 при 100 %)"
      Assert-That ([int]$g.noteFontPx -le $script:ShortFontPx) "Шрифт длинного сообщения $($g.noteFontPx) px крупнее короткого $($script:ShortFontPx) px"
      Assert-That ($note.Top -ge $screen.Top -and $note.Bottom -le $screen.Bottom -and $note.Left -ge $screen.Left -and $note.Right -le $screen.Right) "Плашка $(Format-Rect $note) за краем экрана"
      Assert-That (($note.Bottom - $note.Top) -le [int]($screen.Height * 0.55) + 1) "Плашка выше 55 % экрана: $(Format-Rect $note)"
      Save-Screen "s57-note-long-lock"
      Invoke-GuardClick "noteOk"
      $g = Wait-GuardState { param($s) -not $s.noteId } 5 "длинное сообщение закрыто «Понял»"
      Assert-That ($g.lockShown) "Экран блокировки пропал вместе с плашкой: $(Format-GuardState $g)"
      Start-Sleep -Milliseconds 700
      $back = (@(Get-PcbWindowRects -Children | Where-Object { $_.Class -eq "Button" -and $_.Text -eq "Пароль родителя" }) | Select-Object -First 1).Top
      Assert-That ([math]::Abs($back - $script:PwdTop) -le 1) "Кнопки экрана блокировки не вернулись на место после «Понял»: «Пароль родителя» сверху $back, было $($script:PwdTop)"
      Wait-NoteState $script:Id3 "read" 10
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

  Write-Log "S57: сообщение показано не позже 5 с со звуком, «Понял» и срок 10 с поверх экрана блокировки дали «Прочитано», длинное не закрыло кнопки экрана, подделка не показана"
}
