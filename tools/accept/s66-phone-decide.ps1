# S66. Решение о программе с телефона (FR-47, D-112, протокол связи 1.5, разделы 5д и 6).
# Сервер связи pcblock-relay.exe на раннере (127.0.0.1:18080), тестовый телефон remote-phone.exe вместо приложения.
# Часы ядра на паузе (Start-Child): наблюдатель и плашки идут по настоящим часам. SuperGame.exe — копия системной
# программы в папке WatchDir, не в списке программ (как S14). Журнал и список программ читаются через Use-Parent
# (родительская сессия в сеансе ребёнка прячет плашки, Ruling 19).
# Дано: телефон привязан (как S55), ребёнок запустил незнакомую SuperGame (закрыта, ждёт решения).
# Когда: тестовый телефон читает отчёт и отправляет решение «Игра» (decide -key <id из pending> -cat games).
# Тогда: в отчёте SuperGame в ждущих (издатель — как в окне родителя, попытки ≥ 1, путь, ребёнок), число
# ждущих 1; ответ «SuperGame: игра»; в окне родителя SuperGame в «Играх», ждущих нет, отчёт телефону без ждущих;
# у ребёнка плашка «Можно играть!» со стикером 033 и «Уведомлением»; повторный запуск не закрывается; журнал
# «Решение о «SuperGame» с телефона: игра»; второе решение тем же ключом — not_found без второй записи.
# Снимок: s66-decided.png (плашка «Можно играть!» после решения с телефона).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S66" "Решение о незнакомой программе с телефона: «Игра», плашка «Можно играть!», запуск работает" {
  Initialize-Scenario 60
  try {
    Step "Имитация ввода доходит до сеанса" {
      Test-InputSim
    }

    Step "Тестовая программа SuperGame.exe в папке наблюдения (не в списке)" {
      $script:SgPath = New-TestApp "SuperGame"
    }

    Step "Настройка: лимит 10 ч, часы на паузе; телефон привязан" {
      Start-Child 600
      Wait-ProgramsReady
      $script:ChildId = Connect-Phone "Pixel-S66"
    }

    Step "Ребёнок запускает SuperGame: закрыта, «Ждёт разрешения»" {
      $script:Launched = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
      $p = Start-TestApp $script:SgPath
      [void](Assert-ClosedFast $p "запуск SuperGame.exe")
      [void](Wait-GuardState { param($s) [string]$s.plateKind -eq "unknown" -and [string]$s.plateTitle -eq "Ждёт разрешения" } 6 "плашка «Ждёт разрешения»")
    }

    Step "Отчёт телефону: SuperGame в ждущих — издатель, попытки, путь, ребёнок" {
      Wait-Until {
        $script:Phone = (Invoke-Phone @("status")).Result
        $script:Phone.report -and [int64]$script:Phone.reportTs -ge $script:Launched -and @(@(List $script:Phone.pending) | Where-Object { [string]$_.name -eq "SuperGame" }).Count -ge 1
      } 30 "отчёт телефону с SuperGame в ждущих" 1000
      $row = @(@(List $script:Phone.pending) | Where-Object { [string]$_.name -eq "SuperGame" })[0]
      Write-Log "Ждущие в отчёте ($($script:Phone.pendingCount)): $((@(List $script:Phone.pending) | ForEach-Object { "$($_.name) [$($_.path)] издатель «$($_.pub)», попыток $($_.tries), дети $((@(List $_.kids) | ForEach-Object { "$($_.id):$($_.tries)" }) -join ',')" }) -join ' | ')"
      Assert-That ([int]$script:Phone.pendingCount -eq 1) "Ждущих в отчёте $($script:Phone.pendingCount), ждали 1"
      Assert-That ([int]$row.tries -ge 1) "Попыток в отчёте $($row.tries), ждали не меньше 1"
      # Издатель — как в «Ждут решения» окна родителя (пусто — телефон пишет «Без подписи издателя»).
      $win = @(Find-Pending (Get-ProgramsList) $script:SgPath)
      Assert-That ($win.Count -eq 1) "В «Ждут решения» окна нет SuperGame"
      Write-Log "Издатель: в отчёте «$($row.pub)», в окне «$($win[0].publisher)»"
      $want = if ([string]$win[0].publisher -eq "Без подписи издателя") { "" } else { [string]$win[0].publisher }
      Assert-That ([string]$row.pub -eq $want) "Издатель в отчёте «$($row.pub)», в окне родителя «$($win[0].publisher)» (без подписи в отчёте пусто, протокол раздел 6)"
      Assert-That (([string]$row.path).ToLowerInvariant().EndsWith("\supergame.exe")) "Путь в отчёте «$($row.path)», ждали …\SuperGame.exe"
      Assert-That ([int64]$row.lastMs -ge $script:Launched - 5000) "Последняя попытка в отчёте $($row.lastMs), запуск $($script:Launched)"
      Assert-That (@(@(List $row.kids) | Where-Object { [int64]$_.id -eq $script:ChildId -and [int]$_.tries -ge 1 }).Count -eq 1) "Ребёнок $($script:ChildId) не среди запускавших: $($row | ConvertTo-Json -Compress -Depth 4)"
      $script:Key = [string]$row.id
    }

    Step "Телефон: «Игра» — «SuperGame: игра»; у ребёнка «Можно играть!», стикер 033, «Уведомление»; снимок" {
      $before = @(Get-SystemSounds (Get-GuardState)).Count
      $script:SinceDecide = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
      $r = Invoke-Phone @("decide", "-key", $script:Key, "-cat", "games")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Решение с телефона не принято: код $($r.Code), $($r.Result | ConvertTo-Json -Compress -Depth 5)"
      Assert-That ([string]$r.Result.message -eq "SuperGame: игра") "Ответ телефону «$($r.Result.message)», ждали «SuperGame: игра»"
      $text = "Родитель разрешил «SuperGame»: это игра. Запусти её снова"
      [void](Wait-LobbyPlate "decided" "Можно играть!" "033" $script:SoundNotify $before "Решение с телефона" $text)
      Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
      Save-Screen "s66-decided"
      $rep = $r.Result.report
      if ($rep) {
        Assert-That (@(List $rep.pending).Count -eq 0 -and [int]$rep.pendingCount -eq 0) "Отчёт в ответе телефону ещё с ждущими: $(@(List $rep.pending | ForEach-Object { $_.name }) -join ', ') ($($rep.pendingCount))"
      }
    }

    Step "Повторный запуск SuperGame не закрывается" {
      $script:Sg = Start-TestApp $script:SgPath
      Start-Sleep -Seconds 2
      Assert-AppAlive $script:Sg "SuperGame.exe после решения с телефона"
    }

    Step "Окно родителя: SuperGame в «Играх», ждущих нет; журнал «Решение о «SuperGame» с телефона: игра»" {
      $list = Get-ProgramsList
      Assert-That (@(List $list.pending).Count -eq 0) "После решения в «Ждут решения» строк $(@(List $list.pending).Count), ждали 0"
      $row = @(@(List $list.programs) | Where-Object { ([string]$_.path) -eq $script:SgPath.ToLowerInvariant() })
      Assert-That ($row.Count -eq 1 -and [string]$row[0].category -eq "games") "В списке программ SuperGame: $(($row | ForEach-Object { "$($_.category)/$($_.source)" }) -join ','), ждали games"
      Use-Parent {
        $ev = @(Find-Events 'Решение о «SuperGame» с телефона: игра')
        $ev | ForEach-Object { Write-Log "Журнал: $($_.text)" }
        Assert-That ($ev.Count -eq 1) "Записей «Решение о «SuperGame» с телефона: игра»: $($ev.Count), ждали одну; журнал: $(Format-Events)"
      }
    }

    Step "Второе решение тем же ключом — not_found, журнал без второй записи; отчёт без ждущих" {
      $r = Invoke-Phone @("decide", "-key", $script:Key, "-cat", "games")
      Assert-That ($r.Code -eq 2 -and -not $r.Result.ok -and [string]$r.Result.error -eq "not_found") "Повторное решение: код $($r.Code), $($r.Result | ConvertTo-Json -Compress -Depth 5), ждали отказ not_found"
      Use-Parent {
        Assert-That (@(Find-Events 'Решение о «SuperGame» с телефона').Count -eq 1) "Повторное решение записано в журнал: $(Format-Events)"
      }
      $st = Wait-PhoneReport $script:SinceDecide 30 "отчёт после решения"
      Assert-That (@(List $st.pending).Count -eq 0 -and [int]$st.pendingCount -eq 0) "В отчёте телефону ещё ждут: $((@(List $st.pending) | ForEach-Object { $_.name }) -join ', ') ($($st.pendingCount))"
    }
  } catch {
    Write-RelayLog
    throw
  } finally {
    Stop-Relay
  }

  Write-Log "S66: SuperGame в ждущих отчёта, решение «Игра» с телефона — «Можно играть!» 033, запуск работает, журнал «с телефона», повтор — not_found"
}
