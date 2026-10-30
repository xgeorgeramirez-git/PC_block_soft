# S14. Незнакомая программа ждёт решения родителя (R12, FR-13, FR-20, 8.2.4, NFR-6, RM3-4, RM3-8, RM3-12).
# Часы ядра на паузе (Start-Child), кроме шага счёта игр: наблюдатель и сроки плашек (15 с у «ждёт
# разрешения», 10 с у решения, 5 с между плашками одной программы) идут по настоящим часам.
# superGame.exe — копия системной программы в папке WatchDir, не в списке программ (как файл из «Загрузок»).
# Родительская сессия в сеансе ребёнка прячет плашки (Ruling 19): плашки проверяются до входа родителя,
# каждая родительская команда — вход, команда, выход (Use-Parent).
# Дано: программа не в списке. Когда: запуск. Тогда: закрыта не позже 1,5 с, плашка «Программа
# «superGame» ждёт разрешения родителя» с «Попросить сейчас», в «Ждут решения» одна строка, попыток 1,
# второй запуск — 2; «Попросить сейчас» открывает окно родителя на «Программах» (--parent
# --screen=programs); «Игра» — плашка «Родитель разрешил «superGame»: это игра. Запусти её снова»;
# запуск работает, на переднем плане секунды идут в игры.
# Снимки: s14-pending-plate.png, s14-programs.png (окно родителя, открытое «Попросить сейчас», на запросе
# пароля: вход паролем в окне приёмка не автоматизирует; экран «Программы» снимает Playwright в ci.yml).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы (internal/service/texts.go textPlateUnknown, handlers_m3.go decidedPlateText).
$script:PendingText = "Программа «superGame» ждёт разрешения родителя"
$script:DecidedText = "Родитель разрешил «superGame»: это игра. Запусти её снова"

function Wait-PendingPlate([string]$What) {
  $g = Wait-GuardState { param($s) [string]$s.plate -eq $script:PendingText -and (Test-HasButton $s "askNow") } 6 "$What`: плашка «$($script:PendingText)» с «Попросить сейчас»"
  Assert-That ($g.plateKind -eq "unknown") "Вид плашки «$($g.plateKind)», ждали unknown"
  Assert-That (-not $g.lockShown) "Экран блокировки при незнакомой программе: $(Format-GuardState $g)"
  return $g
}

function Assert-PendingRow($List, [int]$Attempts) {
  $all = @(List $List.pending)
  Write-Log ("Ждут решения: " + (($all | ForEach-Object { "$($_.name) [$($_.path)] попыток $($_.attempts), издатель «$($_.publisher)»" }) -join " | "))
  Assert-That ($all.Count -eq 1) "В «Ждут решения» строк $($all.Count), ждали одну"
  $row = @(Find-Pending $List $script:SgPath)
  Assert-That ($row.Count -eq 1) "В «Ждут решения» нет строки с путём $($script:SgPath.ToLowerInvariant())"
  Assert-That ($row[0].attempts -eq $Attempts) "Попыток запуска $($row[0].attempts), ждали $Attempts"
  Assert-That ($row[0].name -eq "superGame") "Имя в «Ждут решения» «$($row[0].name)», ждали superGame"
}

Invoke-Scenario "S14" "Незнакомая программа: закрыта, ждёт решения, после «Игра» считается игрой" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовая программа superGame.exe в папке наблюдения (не в списке)" {
    $script:SgPath = New-TestApp "superGame"
  }

  Step "Настройка: лимит 10 ч, игры 1 ч, программ мастера нет" {
    Start-Child 600 $true 60
    Wait-ProgramsReady
    $st = Get-Status
    Assert-That ($st.gamesLimitS -eq 3600 -and $st.gamesRemainingS -eq 3600) "Лимит игр: $($st.gamesRemainingS) с из $($st.gamesLimitS), ждали 3600"
  }

  Step "Первый запуск: закрыта, плашка «ждёт разрешения» с «Попросить сейчас», снимок" {
    $p = Start-TestApp $script:SgPath
    [void](Assert-ClosedFast $p "первый запуск superGame.exe")
    [void](Wait-PendingPlate "первый запуск")
    $script:PlateAt = [Diagnostics.Stopwatch]::StartNew()
    Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
    Save-Screen "s14-pending-plate"
  }

  Step "«Ждут решения» у родителя: одна строка, 1 попытка" {
    Assert-PendingRow (Get-ProgramsList) 1
  }

  Step "Второй запуск: закрыта, 2 попытки" {
    # Плашка об одной программе не чаще раза в 5 с (plateEvery): второй запуск после паузы даёт свою.
    while ($script:PlateAt.Elapsed.TotalSeconds -lt 6) { Start-Sleep -Milliseconds 200 }
    $p = Start-TestApp $script:SgPath
    [void](Assert-ClosedFast $p "второй запуск superGame.exe")
    [void](Wait-PendingPlate "второй запуск")
    Assert-PendingRow (Get-ProgramsList) 2
  }

  Step "«Попросить сейчас»: окно родителя на «Программах», снимок" {
    # После выхода родителя плашка (15 с) снова видна со следующим опросом стража.
    [void](Wait-PendingPlate "после выхода родителя")
    Invoke-GuardClick "askNow"
    Wait-Until { @(Get-SessionProcs "--screen=programs").Count -ge 1 } 40 "служба запустила окно родителя с --screen=programs"
    $wp = @(Get-SessionProcs "--screen=programs")[0]
    Write-Log "Окно родителя: PID $($wp.ProcessId), $($wp.CommandLine)"
    Assert-That ($wp.CommandLine -match '--parent') "Окно на «Программах» не окно родителя: $($wp.CommandLine)"
    $g = Get-GuardState
    Assert-That (-not (Test-HasButton $g "askNow")) "Плашка «ждёт разрешения» осталась после «Попросить сейчас»: $(Format-GuardState $g)"
    $proc = Get-Process -Id ([int]$wp.ProcessId)
    try {
      [void](Save-ProcWindow $proc "s14-programs" 40 4)
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

  Step "Родитель: «Игра» — плашка «Родитель разрешил «superGame»: это игра. Запусти её снова»" {
    $rep = Set-ProgramCategory $script:SgPath "games"
    Assert-That ($rep.message -eq "superGame: игра") "Сообщение родителю «$($rep.message)», ждали «superGame: игра»"
    $g = Wait-GuardState { param($s) [string]$s.plate -eq $script:DecidedText } 6 "плашка «$($script:DecidedText)»"
    Assert-That ($g.plateKind -eq "decided") "Вид плашки «$($g.plateKind)», ждали decided"
    $list = Get-ProgramsList
    Assert-That (@(List $list.pending).Count -eq 0) "После решения в «Ждут решения» строк $(@(List $list.pending).Count), ждали 0"
    $row = @(@(List $list.programs) | Where-Object { ([string]$_.path) -eq $script:SgPath.ToLowerInvariant() })
    Assert-That ($row.Count -eq 1 -and $row[0].category -eq "games" -and $row[0].source -eq "parent") "В списке программ superGame: $(($row | ForEach-Object { "$($_.category)/$($_.source)" }) -join ','), ждали games/parent"
  }

  Step "Запуск после «Игра»: работает, на переднем плане секунды идут в игры" {
    $script:Sg = Start-TestApp $script:SgPath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Sg "superGame.exe после «Игра»"
    Set-TestForeground $script:Sg
    $before = Get-Status
    Resume-Clock
    Start-Sleep -Seconds 3
    Suspend-Clock
    $after = Get-Status
    $games = $before.gamesRemainingS - $after.gamesRemainingS
    $total = $before.remainingS - $after.remainingS
    Assert-Between $games 120 260 "Игры: остаток убыл за 3 с настоящего времени (часы ядра), с"
    Assert-That ($total -ge $games - 5) "Общий остаток убыл на $total с, игры на $games с: игры идут и в общее время"
    $m = Get-MyToday
    Assert-That ($m.gamesS -ge 120) "Игры за сегодня (MyToday.gamesS) $($m.gamesS) с, ждали не меньше 120"
    Assert-AppAlive $script:Sg "superGame.exe во время игры"
  }

  Step "Журнал родителя: «ждёт решения», «вы отнесли «superGame» к категории «Игры»», без обхода" {
    Use-Parent {
      $pend = @(Find-Events '«superGame» ждёт решения')
      $dec = @(Find-Events 'вы отнесли «superGame» к категории «Игры»')
      @($pend + $dec) | ForEach-Object { Write-Log "Журнал: $($_.text)" }
      Assert-That ($pend.Count -ge 1) "В журнале нет записи «программа «superGame» ждёт решения»"
      Assert-That ($dec.Count -eq 1) "Записей о решении «Игры» $($dec.Count), ждали одну"
      $bypass = @(Find-Events 'попытка обойти защиту')
      Assert-That ($bypass.Count -eq 0) "В журнале запись об обходе: $(($bypass | ForEach-Object { $_.text }) -join ' | ')"
    }
  }

  Write-Log "S14: незнакомая закрыта и ждёт решения, попытки 1 и 2, «Попросить сейчас» открыл «Программы», после «Игра» считается игрой"
}
