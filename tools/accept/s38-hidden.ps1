# S38. Скрытая программа (D-57, FR-20, R3, RF9-1…RF9-4, PL-16, PL-22).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): минута ядра это секунда настоящего времени.
# Тестовые программы — копии системной программы в папке WatchDir (New-TestApp): game.exe и launcher.exe
# отмечены «Игры» шагом мастера (Setup.programs) до запуска (PL-11). Ввод имитируется движением мыши.
# Дано: game.exe и launcher.exe «Игры», лимит игр 2 мин. Когда: родитель скрывает launcher.exe командой
# SetProgramsHidden, ребёнок запускает обе, игры кончаются. Тогда: launcher.exe нет в списке программ
# (только с флагом hidden для фильтра «Скрытые») и в «Ждут решения»; минуты launcher.exe на переднем
# плане идут в общее время, но не в игры; в 0 игр game.exe закрыта, launcher.exe работает; в журнале
# «вы скрыли «X»»; после «Показать» launcher.exe снова «Игры» и закрывается, повторный запуск тоже.
# Снимок: s38-hidden-alive.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы (internal/service/texts.go textProgramHidden, journaltext.go, internal/core/texts.go).
$script:GamesOver = "Время на игры на сегодня закончилось"

# Строки списка программ с путём Path (путь в канале нормализован: нижний регистр).
function Find-Program($List, [string]$Path) {
  $norm = $Path.ToLowerInvariant()
  return @(@(List $List.programs) | Where-Object { ([string]$_.path) -eq $norm })
}

# «Скрыть» (Hidden = $true) или «Показать» через родительскую сессию; возвращает ответ.
function Set-Hidden([string]$Path, [bool]$Hidden) {
  $rep = Use-Parent { Send-Parent "SetProgramsHidden" @{ paths = @($Path); hidden = $Hidden } }
  Assert-Reply $rep "SetProgramsHidden hidden=$Hidden"
  Write-Log "SetProgramsHidden $(Split-Path -Leaf $Path) hidden=${Hidden}: «$($rep.payload.message)»"
  return $rep.payload
}

Invoke-Scenario "S38" "Скрытая программа: не закрывается и не идёт в игры, после «Показать» снова игра" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовые программы game.exe и launcher.exe в папке наблюдения" {
    $script:GamePath = New-TestApp "game"
    $script:LauncherPath = New-TestApp "launcher"
  }

  Step "Настройка: общий лимит 2 ч, игры 2 мин; game.exe и launcher.exe «Игры»" {
    $progs = @(@{ path = $script:GamePath; category = "games" }, @{ path = $script:LauncherPath; category = "games" })
    Start-Child 120 $true 2 $progs
    Wait-ProgramsReady
    $st = Get-Status
    Assert-That ($st.gamesLimitS -eq 120 -and $st.gamesRemainingS -eq 120) "Лимит игр: $($st.gamesRemainingS) с из $($st.gamesLimitS), ждали 120 из 120"
  }

  Step "Родитель скрывает launcher.exe: сообщение «… скрыта», в списке только с флагом hidden" {
    # Имя — из свойств файла (копия системной программы), поэтому образцы строятся по ответу программы (PL-22).
    $script:LauncherName = Get-ProgramName $script:LauncherPath
    $rep = Set-Hidden $script:LauncherPath $true
    Assert-That ([string]$rep.message -eq "$($script:LauncherName) скрыта") "Сообщение «$($rep.message)», ждали «$($script:LauncherName) скрыта»"
    Assert-That ([string]$rep.undoId -ne "") "Нет «Отменить» (undoId) у скрытия"
    $list = Get-ProgramsList
    $row = @(Find-Program $list $script:LauncherPath)
    Assert-That ($row.Count -eq 1 -and $row[0].hidden -eq $true) "launcher.exe в списке: $($row | ConvertTo-Json -Compress), ждали одну строку с hidden=true (только фильтр «Скрытые»)"
    Assert-That ($row[0].category -eq "games") "Категория скрытой «$($row[0].category)», ждали games (скрытие категорию не меняет)"
    $visible = @(@(List $list.programs) | Where-Object { $_.hidden -ne $true -and ([string]$_.path) -eq $script:LauncherPath.ToLowerInvariant() })
    Assert-That ($visible.Count -eq 0) "launcher.exe виден в списке без фильтра «Скрытые»"
    Assert-That (@(Find-Pending $list $script:LauncherPath).Count -eq 0) "launcher.exe в «Ждут решения»"
    $game = @(Find-Program $list $script:GamePath)
    Assert-That ($game.Count -eq 1 -and $game[0].hidden -ne $true) "game.exe скрыта вместе с launcher.exe: $($game | ConvertTo-Json -Compress)"
  }

  Step "Запуск launcher.exe и game.exe: обе работают" {
    $script:Launcher = Start-TestApp $script:LauncherPath
    $script:Game = Start-TestApp $script:GamePath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Launcher "launcher.exe (скрыта)"
    Assert-AppAlive $script:Game "game.exe («Игры», время на игры есть)"
  }

  Step "launcher.exe на переднем плане: общее время идёт, игры нет" {
    Set-TestForeground $script:Launcher
    Start-Sleep -Seconds 2 # часы на паузе: служба получает снимок стража уже со скрытой на переднем плане
    $before = Get-Status
    Resume-Clock
    $st = Wait-Status { param($st) $st.usedS -ge $before.usedS + 60 } 30 "минута общего времени со скрытой на переднем плане"
    Suspend-Clock
    $st = Get-Status
    Write-Log "Скрытая на переднем плане: общее $($before.usedS) → $($st.usedS) с, остаток игр $($st.gamesRemainingS) с"
    Assert-That ($st.gamesRemainingS -eq 120) "Остаток игр $($st.gamesRemainingS) с, ждали 120: минуты скрытой ушли в игры"
    Assert-That (-not $st.closeGames) "Игры закрыты, хотя играли только в скрытую"
  }

  Step "game.exe на переднем плане, игры кончились: game.exe закрыта, launcher.exe работает, снимок" {
    Set-TestForeground $script:Game
    Start-Sleep -Seconds 2
    Resume-Clock
    $st = Wait-Status { param($st) $st.closeGames } 30 "игры закрыты (closeGames)"
    $since = [Diagnostics.Stopwatch]::StartNew()
    Suspend-Clock
    Write-Log "Игры закрыты: остаток игр $($st.gamesRemainingS) с, общий $($st.remainingS) с; часы остановлены"
    [void](Assert-ClosedFast $script:Game "game.exe в 0 игр" $since)
    [void](Wait-GuardState { param($s) [string]$s.plate -eq $script:GamesOver } 5 "плашка «$($script:GamesOver)»")
    Start-Sleep -Seconds 2 # два прохода наблюдателя: скрытая не закрыта и потом
    Assert-AppAlive $script:Launcher "launcher.exe (скрыта) после конца игр"
    Save-Screen "s38-hidden-alive"
  }

  Step "Журнал родителя: «вы скрыли «X»», о закрытии launcher.exe записи нет" {
    Use-Parent {
      $want = "Вы скрыли «$($script:LauncherName)»"
      $hid = @(Find-Events ([regex]::Escape($want)))
      Assert-That ($hid.Count -eq 1) "В журнале $($hid.Count) записей «$want», ждали одну. Журнал: $(Format-Events)"
      # У копий одной системной программы одно имя из свойств файла (PL-22): различаем по числу записей.
      $closed = @(Get-Events | Where-Object { $_.kind -eq "program_closed_games" })
      Assert-That ($closed.Count -eq 1) "Записей «закрыта … время на игры закончилось» $($closed.Count), ждали одну (только game.exe, launcher.exe скрыта). Журнал: $(Format-Events)"
      $bypass = @(Find-Events 'попытка обойти защиту')
      Assert-That ($bypass.Count -eq 0) "В журнале запись об обходе: $(($bypass | ForEach-Object { $_.text }) -join ' | ')"
    }
  }

  Step "«Показать»: launcher.exe снова «Игры» и закрыта; повторный запуск тоже закрыт" {
    $rep = Set-Hidden $script:LauncherPath $false
    $since = [Diagnostics.Stopwatch]::StartNew() # после ответа: вход родителя паролем в допуск 1,5 с не входит
    Assert-That ([string]$rep.message -eq "$($script:LauncherName) снова в списке") "Сообщение «$($rep.message)», ждали «$($script:LauncherName) снова в списке»"
    $row = @(Find-Program $rep.programs $script:LauncherPath)
    Assert-That ($row.Count -eq 1 -and $row[0].hidden -ne $true -and $row[0].category -eq "games") "launcher.exe после «Показать»: $($row | ConvertTo-Json -Compress), ждали видна и «Игры»"
    [void](Assert-ClosedFast $script:Launcher "запущенная launcher.exe после «Показать»" $since)
    $again = Start-TestApp $script:LauncherPath
    [void](Assert-ClosedFast $again "повторный запуск launcher.exe")
    Use-Parent {
      $want = "Вы снова показали «$($script:LauncherName)»"
      Assert-That (@(Find-Events ([regex]::Escape($want))).Count -eq 1) "В журнале нет «$want». Журнал: $(Format-Events)"
    }
  }

  Write-Log "S38: скрытая launcher.exe не закрыта и не идёт в игры, game.exe закрыта в 0 игр, после «Показать» launcher.exe снова игра"
}
