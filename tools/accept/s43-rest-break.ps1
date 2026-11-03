# S43. Перерыв на отдых (ТЗ R27, FR-37, S43, эталон G14, D-62; фидбек-раунд 0.3.2, FB-14).
# Заказчик: «х минут поиграл и уведомление отдохни и через 5 минут комп блокируется на определенное количество минут».
# Пределы настройки (FR-37, CHECK миграции 008) не дают «3 мин / 1 мин / 2 мин» из S43, поэтому сценарий берёт
# наименьшие допустимые «Через сколько» 15 мин, «Предупредить за» 1 мин, «Отдых» 10 мин, «Засчитать как отдых» 10 мин,
# «Только игры», а часы ядра идут в 20 раз быстрее: 15 мин игр это 45 с настоящего времени, предупреждение 3 с,
# перерыв 30 с, порог «сам отошёл» 30 с. Тестовые программы: game.exe («Игра») и notes.exe («Прочее»); «не играл» —
# notes.exe на переднем плане (в режиме «Только игры» это не засчитывается к перерыву), «вышел» — отключение сеанса
# тестовой командой TestSession, как в S41 (второго пользователя у раннера нет, PL-10).
# Дано: перерывы включены, сообщение «Пойди погуляй с собакой». Когда: ребёнок играет в game.exe.
# Тогда: на 15-й минуте игр звук и плашка «ПОРА ОТДОХНУТЬ» (снимок s43-rest-plate), через 1 мин экран «Перерыв на
#   отдых» с «Можно снова в …», картинкой, таймером плитками и сообщением (снимки s43-rest-screen-1 и через 3 с
#   s43-rest-screen-2: таймер меньше), время за экраном не идёт; через 10 мин экран исчезает, счёт с нуля.
# Вариант «вышел и вернулся до конца»: снова экран перерыва, конец перерыва не сдвинулся; после конца экрана нет.
# Вариант «сам отошёл»: не играл меньше порога — счёт продолжается; не играл дольше порога — плашки нет, счёт с нуля.
# Вариант «Закончить перерыв»: пароль родителя на экране перерыва, экран исчезает, в журнале запись.
# Записей об обходе нет.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$script:Speed = 20
$script:Message = "Пойди погуляй с собакой"
$script:Break = @{ enabled = $true; afterMin = 15; counts = "games"; warnMin = 1; restMin = 10; selfRestMin = 10; message = $script:Message }
# Тексты ядра (internal/core/rest.go RestPlateText, RestBreakText, RestEndedText) и записи об обходе (как в S41).
$script:PlateText = "Через 1 минуту перерыв на 10 минут. Сохрани игру"
$script:BreakEvent = 'перерыв на отдых: 10 мин после 15 мин игр'
$script:EndedEvent = 'вы закончили перерыв'
$script:BypassPattern = 'обойти защиту|Защита программы была остановлена|закрыто не кнопкой окна|Значок программы был закрыт'
$script:Buttons = "parentPassword,shutdown,logoff,switchUser"

# Секунды настоящего времени для минут часов ядра.
function Get-RealS([double]$CoreMin) { return $CoreMin * 60 / $script:Speed }

# Плашки «ПОРА ОТДОХНУТЬ» в журнале показанных предупреждений стража.
function Get-RestWarnings($State) { return @(List $State.warnings | Where-Object { [string]$_.text -eq $script:PlateText }) }

# Таймер экрана перерыва «00:09:57» → секунды.
function ConvertFrom-Timer([string]$Text) {
  Assert-That ($Text -match '^(\d\d):(\d\d):(\d\d)$') "Таймер экрана перерыва «$Text», ждали ЧЧ:ММ:СС"
  return [int]$Matches[1] * 3600 + [int]$Matches[2] * 60 + [int]$Matches[3]
}

function Send-TestSession([string]$Event, [int]$SessionId) {
  $r = Send-Pcb "TestSession" @{ event = $Event; sessionId = $SessionId }
  Assert-Reply $r "TestSession $Event $SessionId"
}

# Ребёнок «вышел»: сеанс отключён на $Seconds настоящих секунд (событие повторяется раз в 2 с, как в S41).
function Invoke-Away([double]$Seconds) {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
    Send-TestSession "disconnect" ([int]$script:MySession)
    Start-Sleep -Milliseconds 2000
  }
  Write-Log ("Сеанс ребёнка отключён {0:N1} с настоящего времени (около {1:N0} с часов ядра)" -f $sw.Elapsed.TotalSeconds, ($sw.Elapsed.TotalSeconds * $script:Speed))
}

# Ждёт новую плашку «ПОРА ОТДОХНУТЬ» (их стало больше $Before, PL-16) и проверяет её вид, текст и звук.
function Wait-RestPlate([int]$Before, [double]$Seconds, [string]$What) {
  $g = Wait-GuardState { param($s) (Get-RestWarnings $s).Count -gt $Before -and $s.plateKind -eq "rest" } $Seconds $What
  Write-Log "Плашка отдыха через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-GuardState $g)"
  Assert-That ([string]$g.plate -eq $script:PlateText) "Текст плашки «$($g.plate)», ждали «$($script:PlateText)»"
  Assert-That ((Test-HasButton $g "warningClose") -and -not $g.lockShown) "У плашки отдыха нет крестика или уже экран: $(Format-GuardState $g)"
  return $g
}

# Ждёт экран перерыва и проверяет его: причина, «Можно снова в …», кнопки, сообщение, таймер.
function Assert-RestScreen([double]$Seconds, [string]$What) {
  $g = Wait-LockShown $Seconds "rest"
  $st = Get-Status
  Write-Log "Экран перерыва ($What) через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-GuardState $g); таймер $($g.restTimer); статус: экран '$($st.lock)', причина «$($st.reason)», «$($st.until)», до конца $($st.restLeftS) с, остаток дня $($st.remainingS) с"
  Assert-That ($st.lock -eq "rest" -and $st.reason -eq "Перерыв на отдых") "Статус: экран '$($st.lock)', причина «$($st.reason)», ждали rest и «Перерыв на отдых»"
  Assert-That ([string]$st.until -match '^Можно снова в \d\d:\d\d$') "Строка «$($st.until)», ждали «Можно снова в ЧЧ:ММ»"
  Assert-That ((@(List $g.buttons) -join ",") -eq $script:Buttons) "Кнопки экрана перерыва [$((@(List $g.buttons)) -join ',')], ждали [$($script:Buttons)] (без «Попросить ещё», копилки и промокода)"
  Assert-That ([string]$g.restMessage -eq $script:Message) "Сообщение экрана «$($g.restMessage)», ждали «$($script:Message)»"
  [void](ConvertFrom-Timer ([string]$g.restTimer))
  return $g
}

Invoke-Scenario "S43" "Перерыв на отдых: плашка, экран по часам, «сам отошёл», «Закончить перерыв», без обхода" {
  Initialize-Scenario $script:Speed

  Step "Тестовые программы game.exe («Игра») и notes.exe («Прочее»); лимит дня 4 ч" {
    Test-InputSim
    $script:GamePath = New-TestApp "game"
    $script:NotesPath = New-TestApp "notes"
    $progs = @(@{ path = $script:GamePath; category = "games" }, @{ path = $script:NotesPath; category = "other" })
    Start-Child 240 $true -1 $progs
    Wait-ProgramsReady
  }

  Step "«Отдых»: включено, 15 мин, «Только игры», за 1 мин, отдых 10 мин, «сам отошёл» 10 мин, своё сообщение" {
    Open-Parent
    $id = Get-ChildId
    Assert-Reply (Send-Parent "SaveLimits" @{ childId = $id; limits = @(); break = $script:Break }) "SaveLimits «Отдых»"
    $r = Send-Parent "GetChildRules" @{ childId = $id }
    Assert-Reply $r "GetChildRules"
    $b = $r.payload.break
    $got = "$($b.enabled)/$($b.afterMin)/$($b.counts)/$($b.warnMin)/$($b.restMin)/$($b.selfRestMin)/$($b.message)"
    Assert-That ($got -eq "True/15/games/1/10/10/$($script:Message)") "Сохранённый «Отдых» $got, ждали True/15/games/1/10/10/$($script:Message)"
    Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы"
    Assert-Reply (Send-Parent "Logout") "Logout"
  }

  Step "Ребёнок играет: на 15-й минуте игр звук и плашка «ПОРА ОТДОХНУТЬ», снимок" {
    $script:Notes = Start-TestApp $script:NotesPath
    $script:Game = Start-TestApp $script:GamePath
    Start-Sleep -Seconds 2
    Set-TestForeground $script:Game
    Resume-Clock
    $g = Wait-RestPlate 0 ((Get-RealS 15) + 30) "плашка «ПОРА ОТДОХНУТЬ» после 15 мин игр"
    # Остановленные часы держат плашку до снимка: при ускорении 20 до перерыва 3 с (как плашка 1 мин в S41).
    Suspend-Clock
    Assert-Between $script:WaitElapsed ((Get-RealS 15) * 0.8) ((Get-RealS 15) + 20) "Плашка через, с настоящего времени (15 мин игр)"
    Write-Log "Звуки: $(@(List $g.sounds | ForEach-Object { "$($_.threshold):$($_.ok)" }) -join ',')"
    Assert-That (@(List $g.sounds).Count -ge 1) "Звука у плашки отдыха нет: $(Format-GuardState $g)"
    Save-Screen "s43-rest-plate"
    Assert-AppAlive $script:Game "game.exe под плашкой отдыха"
  }

  Step "Через 1 мин экран «Перерыв на отдых»: картинка, таймер, «Можно снова в …», сообщение; время дня не идёт" {
    Resume-Clock
    $g = Assert-RestScreen 15 "первый"
    Start-Sleep -Milliseconds 800 # поток окон дорисовал экран
    $script:Rem1 = [int](Get-Status).remainingS
    $t1 = ConvertFrom-Timer ([string](Get-GuardState).restTimer)
    Save-Screen "s43-rest-screen-1"
    Start-Sleep -Seconds 3
    $g2 = Get-GuardState
    $t2 = ConvertFrom-Timer ([string]$g2.restTimer)
    Save-Screen "s43-rest-screen-2"
    $rem2 = [int](Get-Status).remainingS
    Write-Log "Таймер экрана: $t1 с → через 3 с $t2 с; остаток дня $($script:Rem1) → $rem2 с"
    Assert-That ($t2 -lt $t1) "Таймер экрана перерыва не уменьшился за 3 с: $t1 → $t2 с ($($g2.restTimer))"
    Assert-That ($script:Rem1 - $rem2 -le 2) "Остаток дня за экраном перерыва ушёл с $($script:Rem1) до $rem2 с: время за экраном идти не должно"
    Assert-AppAlive $script:Game "game.exe под экраном перерыва (игры не закрываются, FB14-2)"
  }

  Step "Через 10 мин экран исчезает сам; счёт к перерыву с нуля: новой плашки нет ещё 9 мин игр" {
    [void](Wait-LockHidden ((Get-RealS 10) + 10))
    $st = Get-Status
    Write-Log "Экран перерыва снят через $([math]::Round($script:WaitElapsed, 1)) с: экран '$($st.lock)', остаток дня $($st.remainingS) с"
    Assert-That ($st.lock -eq "none") "После перерыва экран '$($st.lock)', ждали none"
    Set-TestForeground $script:Game
    $before = (Get-RestWarnings (Get-GuardState)).Count
    Start-Sleep -Seconds ([int](Get-RealS 9))
    $g = Get-GuardState
    Assert-That ((Get-RestWarnings $g).Count -eq $before -and -not $g.lockShown) "Через 9 мин игр после перерыва снова плашка или экран (счёт не с нуля): $(Format-GuardState $g)"
  }

  Step "Вариант «вышел и вернулся до конца»: вторая плашка, экран, сеанс отключён и вернулся — снова экран" {
    [void](Wait-RestPlate 1 ((Get-RealS 6) + 20) "вторая плашка «ПОРА ОТДОХНУТЬ» после ещё 6 мин игр")
    $g = Assert-RestScreen 15 "второй"
    $script:Until = [int64](Get-Status).restUntilMs
    Invoke-Away 6
    Start-Sleep -Seconds 7 # удержание отключения 5 с истекло: сеанс снова на экране (S41)
    $st = Get-Status
    Write-Log "Вернулся до конца перерыва: экран '$($st.lock)', до конца $($st.restLeftS) с, конец $($st.restUntilMs) (был $($script:Until))"
    Assert-That ($st.lock -eq "rest" -and $st.restLeftS -gt 0) "Вернулся до конца перерыва, а экран '$($st.lock)', до конца $($st.restLeftS) с"
    Assert-That ([int64]$st.restUntilMs -eq $script:Until) "Конец перерыва сдвинулся: $($st.restUntilMs), был $($script:Until) (перерыв идёт по часам)"
    [void](Assert-RestScreen 5 "после возврата")
  }

  Step "Вышел снова и вернулся после конца перерыва: экрана нет" {
    $left = [int](Get-Status).restLeftS
    Invoke-Away ($left / $script:Speed + 4)
    Start-Sleep -Seconds 7
    $st = Get-Status
    Write-Log "Вернулся после конца перерыва: экран '$($st.lock)', до конца $($st.restLeftS) с"
    Assert-That ($st.lock -eq "none") "После конца перерыва экран '$($st.lock)', ждали none"
    [void](Wait-LockHidden 10)
  }

  Step "Вариант «сам отошёл» меньше порога: 5 мин игр, 5 мин не играл, счёт продолжается — плашка через 10 мин игр" {
    Set-TestForeground $script:Game
    $before = (Get-RestWarnings (Get-GuardState)).Count
    Start-Sleep -Seconds ([int](Get-RealS 5))
    Set-TestForeground $script:Notes
    Start-Sleep -Seconds ([int](Get-RealS 5))
    Assert-That ((Get-RestWarnings (Get-GuardState)).Count -eq $before) "Плашка отдыха, пока ребёнок не играл: $(Format-GuardState (Get-GuardState))"
    Set-TestForeground $script:Game
    [void](Wait-RestPlate $before ((Get-RealS 15) + 10) "плашка после отлучки короче порога")
    # Счёт продолжился: до плашки около 10 мин игр (15 минус 5 до отлучки), а не 15 заново.
    Assert-Between $script:WaitElapsed ((Get-RealS 10) * 0.6) ((Get-RealS 13)) "Плашка после возврата через, с настоящего времени (≈10 мин игр)"
  }

  Step "Вариант «Закончить перерыв»: пароль родителя на экране перерыва, экран исчезает" {
    [void](Assert-RestScreen 15 "третий")
    Invoke-GuardClick "parentPassword"
    $g = Wait-GuardState { param($s) (@(List $s.fields) | Where-Object { $_.id -eq "password" }).Count -ge 1 } 8 "панель пароля на экране перерыва"
    Enter-GuardPassword $script:ParentPassword
    Invoke-GuardClick "submit"
    $g = Wait-GuardState { param($s) Test-HasButton $s "endBreak" } 8 "после пароля кнопка «Закончить перерыв»"
    Assert-That (-not (Test-HasButton $g "add15")) "На экране перерыва после пароля «+15»: $(Format-GuardState $g)"
    Invoke-GuardClick "endBreak"
    [void](Wait-LockHidden 10)
    $st = Get-Status
    Write-Log "После «Закончить перерыв»: экран '$($st.lock)', до конца $($st.restLeftS) с"
    Assert-That ($st.lock -eq "none" -and [int64]$st.restUntilMs -eq 0) "После «Закончить перерыв» экран '$($st.lock)', конец $($st.restUntilMs), ждали none и 0"
  }

  Step "Вариант «сам отошёл» дольше порога: 5 мин игр, 12 мин не играл — плашки нет, счёт с нуля" {
    Set-TestForeground $script:Game
    $before = (Get-RestWarnings (Get-GuardState)).Count
    Start-Sleep -Seconds ([int](Get-RealS 5))
    Set-TestForeground $script:Notes
    Start-Sleep -Seconds ([int](Get-RealS 12))
    Set-TestForeground $script:Game
    # Если бы счёт продолжился, плашка пришла бы через 10 мин игр; ждём 13 мин — её нет.
    Start-Sleep -Seconds ([int](Get-RealS 13))
    $g = Get-GuardState
    Assert-That ((Get-RestWarnings $g).Count -eq $before -and -not $g.lockShown) "После отлучки дольше порога плашка или экран за 13 мин игр (счёт не с нуля): $(Format-GuardState $g)"
    Suspend-Clock
  }

  Step "Журнал родителя: перерывы и «Вы закончили перерыв», записей об обходе нет" {
    Open-Parent
    Write-ServiceSessionLog "S43"
    Write-Log "Записи журнала: $(Format-Events)"
    $breaks = Find-Events $script:BreakEvent
    Assert-That ($breaks.Count -eq 3) "Записей «…$($script:BreakEvent)»: $($breaks.Count), ждали 3 (первый, второй с возвратом, третий); журнал: $(Format-Events)"
    $ended = Find-Events $script:EndedEvent
    Assert-That ($ended.Count -eq 1) "Записей «…$($script:EndedEvent)»: $($ended.Count), ждали 1; журнал: $(Format-Events)"
    $found = Find-Events $script:BypassPattern
    Assert-That ($found.Count -eq 0) "В журнале есть записи об обходе: $(($found | ForEach-Object { $_.text }) -join ' | ')"
    Stop-TestApps
  }

  Write-Log "S43: плашка, экран перерыва по часам с таймером и сообщением, «сам отошёл» по порогу, «Закончить перерыв» паролем, без записей об обходе"
}
