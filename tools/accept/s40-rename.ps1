# S40. Своё название программы (D-59, FR-20, 8.3.4 п.5, PL-16, PL-22).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): минута ядра это секунда настоящего времени.
# Тестовая программа — копия системной программы в папке WatchDir (New-TestApp): game.exe отмечена «Игры»
# шагом мастера (Setup.programs) до запуска (PL-11). Ввод имитируется движением мыши.
# Дано: game.exe «Игры» с названием из файла, лимит игр 2 мин. Когда: родитель переименовывает её в
# «Моя игра» командой RenameProgram, ребёнок играет до конца лимита игр. Тогда: в ListPrograms «Моя игра»
# и «из файла» (fileName) прежнее название; в журнале «Вы переименовали «X» в «Моя игра»» и «закрыта
# «Моя игра»: время на игры закончилось»; длинное название — отказ «Название до 60 символов» у поля name;
# после очистки названия снова название из файла и журнал «Вы вернули …». Снимок: s40-renamed-closed.png.
# Статистика по программам (вкладки FB-10) в ветке ещё нет: её название проверит S39.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы (internal/service/texts.go textProgramRenamed, textNameTooLong; journaltext.go).
$script:GamesOver = "Время на игры на сегодня закончилось"
$script:Custom = "Моя игра"

# Строка списка программ с путём Path (путь в канале нормализован: нижний регистр).
function Find-Program($List, [string]$Path) {
  $norm = $Path.ToLowerInvariant()
  return @(@(List $List.programs) | Where-Object { ([string]$_.path) -eq $norm })
}

# «Переименовать» через родительскую сессию; возвращает ответ целиком (успех или отказ).
function Send-Rename([string]$Path, [string]$Name) {
  return (Use-Parent { Send-Parent "RenameProgram" @{ path = $Path; name = $Name } })
}

Invoke-Scenario "S40" "Своё название программы: в списке и журнале, сброс возвращает название из файла" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Тестовая программа game.exe в папке наблюдения" {
    $script:GamePath = New-TestApp "game"
  }

  Step "Настройка: общий лимит 2 ч, игры 2 мин; game.exe «Игры»" {
    Start-Child 120 $true 2 @(@{ path = $script:GamePath; category = "games" })
    Wait-ProgramsReady
    $st = Get-Status
    Assert-That ($st.gamesLimitS -eq 120 -and $st.gamesRemainingS -eq 120) "Лимит игр: $($st.gamesRemainingS) с из $($st.gamesLimitS), ждали 120 из 120"
  }

  Step "Родитель переименовывает game.exe в «Моя игра»: сообщение, «Отменить», список и журнал" {
    # Название из файла — свойства копии системной программы, поэтому образцы строятся по ответу программы (PL-22).
    $script:FileName = Get-ProgramName $script:GamePath
    Assert-That ($script:FileName -ne $script:Custom) "Название из файла уже «$($script:FileName)»"
    $rep = Send-Rename $script:GamePath "  $($script:Custom)  "
    Assert-Reply $rep "RenameProgram"
    Write-Log "RenameProgram: «$($rep.payload.message)»"
    Assert-That ([string]$rep.payload.message -eq "Название изменено") "Сообщение «$($rep.payload.message)», ждали «Название изменено»"
    Assert-That ([string]$rep.payload.undoId -ne "") "Нет «Отменить» (undoId) у переименования"
    $row = @(Find-Program (Get-ProgramsList) $script:GamePath)
    Assert-That ($row.Count -eq 1 -and [string]$row[0].name -eq $script:Custom) "game.exe в списке: $($row | ConvertTo-Json -Compress), ждали название «$($script:Custom)»"
    Assert-That ([string]$row[0].fileName -eq $script:FileName) "«из файла» «$($row[0].fileName)», ждали «$($script:FileName)»"
    Assert-That ($row[0].category -eq "games") "Категория «$($row[0].category)», ждали games (переименование категорию не меняет)"
    Use-Parent {
      $want = "Вы переименовали «$($script:FileName)» в «$($script:Custom)»"
      Assert-That (@(Find-Events ([regex]::Escape($want))).Count -eq 1) "В журнале нет одной записи «$want». Журнал: $(Format-Events)"
    }
  }

  Step "Название длиннее 60 символов: отказ у поля, название прежнее" {
    $rep = Send-Rename $script:GamePath ("я" * 61)
    Assert-That (-not $rep.ok) "Название в 61 символ принято"
    Assert-That ([string]$rep.error -eq "Название до 60 символов" -and [string]$rep.field -eq "name") "Отказ «$($rep.error)» у поля «$($rep.field)», ждали «Название до 60 символов» у name"
    $row = @(Find-Program (Get-ProgramsList) $script:GamePath)
    Assert-That ([string]$row[0].name -eq $script:Custom) "После отказа название «$($row[0].name)», ждали «$($script:Custom)»"
  }

  Step "Игра на переднем плане до конца лимита игр: закрыта, плашка, снимок" {
    $script:Game = Start-TestApp $script:GamePath
    Start-Sleep -Seconds 2
    Assert-AppAlive $script:Game "game.exe («Моя игра», время на игры есть)"
    Set-TestForeground $script:Game
    Start-Sleep -Seconds 1
    Resume-Clock
    $st = Wait-Status { param($st) $st.closeGames } 30 "игры закрыты (closeGames)"
    $since = [Diagnostics.Stopwatch]::StartNew()
    Suspend-Clock
    Write-Log "Игры закрыты: остаток игр $($st.gamesRemainingS) с; часы остановлены"
    [void](Assert-ClosedFast $script:Game "game.exe в 0 игр" $since)
    [void](Wait-GuardState { param($s) [string]$s.plate -eq $script:GamesOver } 5 "плашка «$($script:GamesOver)»")
    Save-Screen "s40-renamed-closed"
  }

  Step "Журнал: «закрыта «Моя игра»: время на игры закончилось», записи с названием из файла нет" {
    Use-Parent {
      # Ждём именно запись о закрытии (PL-16): она появляется только после события.
      $want = "закрыта «$($script:Custom)»: время на игры закончилось"
      Wait-Until { @(Find-Events ([regex]::Escape($want))).Count -ge 1 } 10 "запись «$want»"
      $closed = @(Get-Events | Where-Object { $_.kind -eq "program_closed_games" })
      Assert-That ($closed.Count -eq 1) "Записей о закрытии игры $($closed.Count), ждали одну. Журнал: $(Format-Events)"
      Assert-That ([string]$closed[0].text -match [regex]::Escape($want)) "Запись о закрытии «$($closed[0].text)», ждали «…$want»"
      $old = "закрыта «$($script:FileName)»"
      Assert-That (@(Find-Events ([regex]::Escape($old))).Count -eq 0) "В журнале название из файла: «$old». Журнал: $(Format-Events)"
    }
  }

  Step "Сброс пустым названием: снова название из файла, журнал «Вы вернули …»" {
    $rep = Send-Rename $script:GamePath "   "
    Assert-Reply $rep "RenameProgram (сброс)"
    $row = @(Find-Program $rep.payload.programs $script:GamePath)
    Assert-That ($row.Count -eq 1 -and [string]$row[0].name -eq $script:FileName) "После сброса: $($row | ConvertTo-Json -Compress), ждали «$($script:FileName)»"
    Assert-That ([string]$row[0].fileName -eq "") "После сброса «из файла» «$($row[0].fileName)», ждали пусто"
    Use-Parent {
      $want = "Вы вернули «$($script:Custom)» название из файла «$($script:FileName)»"
      Assert-That (@(Find-Events ([regex]::Escape($want))).Count -eq 1) "В журнале нет «$want». Журнал: $(Format-Events)"
      $bypass = @(Find-Events 'попытка обойти защиту')
      Assert-That ($bypass.Count -eq 0) "В журнале запись об обходе: $(($bypass | ForEach-Object { $_.text }) -join ' | ')"
    }
  }

  Write-Log "S40: «$($script:Custom)» в списке и журнале закрытия, длинное название отклонено, сброс вернул «$($script:FileName)»"
}
