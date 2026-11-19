# S59. Статистика за сегодня на телефоне (FR-43, D-99, протокол связи, раздел 6).
# Сервер связи pcblock-relay.exe на раннере (127.0.0.1:18080), тестовый телефон remote-phone.exe. Часы ядра идут
# в 60 раз быстрее: минута ядра это секунда настоящего времени. Тестовые программы — копии из New-TestApp:
# game.exe «Игры», study.exe «Учёба» отмечены шагом мастера (Setup.programs) до запуска (PL-11), как S12…S14.
# Дано: телефон привязан (как S55), ребёнок за компьютером: game.exe 10 мин, study.exe 5 мин на переднем плане.
# Когда: часы на паузе, тестовый телефон читает подписанный отчёт (today). Тогда: время первого входа сегодня
# (до начала игры) и «за компьютером сейчас», использовано и осталось, время игр и учёбы, список программ (название,
# категория, время) совпадают с «Итогами» за сегодня окна родителя (ParentStats, period today) до минуты.
# Журнал и ParentStats читаются через Use-Parent (Ruling 19).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Категория вкладки окна для телефона (протокол, раздел 6: game, study, video, other).
function ConvertTo-PhoneCat([string]$Category) {
  switch ($Category) {
    "games" { return "game" }
    "study" { return "study" }
    "video_social" { return "video" }
  }
  return "other"
}

# Программа App на переднем плане, пока её время в «Итогах» (MyToday) не наберёт Seconds.
function Invoke-Foreground($App, [string]$Path, [string]$Category, [int]$Seconds, [string]$What) {
  Set-TestForeground $App
  Start-Sleep -Seconds 2 # часы на паузе: служба получает снимок стража уже с этой программой
  Resume-Clock
  $norm = $Path.ToLowerInvariant()
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($true) {
    $m = Get-MyToday
    $row = @(@(List $m.programs) | Where-Object { ([string]$_.path) -eq $norm -and [string]$_.category -eq $Category })
    if ($row.Count -eq 1 -and [int]$row[0].seconds -ge $Seconds) { break }
    if ($sw.Elapsed.TotalSeconds -gt 90) {
      Suspend-Clock
      throw "За 90 с не набралось $Seconds с у $What; игры $($m.gamesS) с, учёба $($m.studyS) с, общее $($m.status.usedS) с; экран: $(Format-GuardState (Get-GuardState))"
    }
    Start-Sleep -Milliseconds 40
  }
  Suspend-Clock
  Write-Log ("{0}: {1} с часов ядра за {2:N1} с настоящего времени" -f $What, $Seconds, $sw.Elapsed.TotalSeconds)
}

Invoke-Scenario "S59" "Статистика за сегодня на телефоне совпадает с «Итогами» окна родителя" {
  Initialize-Scenario 60
  try {
    Step "Имитация ввода доходит до сеанса" {
      Test-InputSim
    }

    Step "Тестовые программы game.exe и study.exe в папке наблюдения" {
      $script:GamePath = New-TestApp "game"
      $script:StudyPath = New-TestApp "study"
    }

    Step "Настройка: лимит 2 ч, game.exe «Игры», study.exe «Учёба», часы на паузе; сервер связи, связь включена" {
      Start-Child 120 $true -1 @(@{ path = $script:GamePath; category = "games" }, @{ path = $script:StudyPath; category = "study" })
      Wait-ProgramsReady
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
      $r = Invoke-Phone @("pair", "-url", $script:RelayUrl, "-code", [string]$script:LastRemote.pair.code, "-name", "Pixel-S59")
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

    Step "game.exe 10 мин, study.exe 5 мин на переднем плане" {
      $script:CoreStartMs = Get-CoreNowMs
      $script:Game = Start-TestApp $script:GamePath
      $script:Study = Start-TestApp $script:StudyPath
      Start-Sleep -Seconds 2
      Assert-AppAlive $script:Game "game.exe"
      Assert-AppAlive $script:Study "study.exe"
      Invoke-Foreground $script:Game $script:GamePath "games" 600 "game.exe"
      Invoke-Foreground $script:Study $script:StudyPath "study" 300 "study.exe"
      $script:CoreNowMs = Get-CoreNowMs
      $script:AfterPlay = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    }

    Step "Отчёт телефону со статистикой за сегодня (часы на паузе, отчёт раз в 60 с)" {
      Wait-Until {
        $t = Invoke-Phone @("today", "-child", "$($script:ChildId)")
        $script:Phone = $t.Result
        $t.Code -eq 0 -and [int64]$script:Phone.reportTs -ge $script:AfterPlay -and $null -ne $script:Phone.today
      } 75 "отчёт со статистикой новее конца игры" 2000
      $script:Td = $script:Phone.today
      Write-Log "Телефон: $($script:Td | ConvertTo-Json -Compress -Depth 5)"
    }

    Step "Сверка с «Итогами» за сегодня окна родителя (ParentStats) до минуты" {
      $p = Use-Parent {
        $r = Send-Parent "ParentStats" @{ childId = $script:ChildId; period = "today" }
        Assert-Reply $r "ParentStats"
        $r.payload
      }
      $td = $script:Td
      $rows = (@(List $p.programs) | ForEach-Object { "$($_.kind)/$($_.category):$($_.name)=$($_.seconds)" }) -join ' | '
      Write-Log "Окно: общее $($p.usedS) с, игры $($p.gamesS) с, учёба $($p.studyS) с, видео $($p.videoS) с, осталось $($p.status.remainingS) с; строки: $rows"
      Assert-Between ([int64]$td.usedSec - [int64]$p.usedS) -60 60 "Использовано: телефон $($td.usedSec) с минус окно $($p.usedS) с"
      Assert-Between ([int64]$td.leftSec - [int64]$p.status.remainingS) -60 60 "Осталось: телефон $($td.leftSec) с минус окно $($p.status.remainingS) с"
      Assert-Between ([int64]$td.gamesSec - [int64]$p.gamesS) -60 60 "Игры: телефон $($td.gamesSec) с минус окно $($p.gamesS) с"
      Assert-Between ([int64]$td.studySec - [int64]$p.studyS) -60 60 "Учёба: телефон $($td.studySec) с минус окно $($p.studyS) с"
      Assert-Between ([int64]$td.videoSec - [int64]$p.videoS) -60 60 "Видео: телефон $($td.videoSec) с минус окно $($p.videoS) с"
      Assert-That ([int64]$td.gamesSec -ge 600 -and [int64]$td.studySec -ge 300) "Игры $($td.gamesSec) с, учёба $($td.studySec) с: ждали не меньше 600 и 300"

      $want = @(@(List $p.programs) | Where-Object { $_.path -and [string]$_.kind -ne "site" })
      $got = @(List $td.programs)
      Assert-That ($got.Count -eq $want.Count) "Программ у телефона $($got.Count), в окне $($want.Count); окно: $rows"
      foreach ($w in $want) {
        $cat = ConvertTo-PhoneCat ([string]$w.category)
        $g = @($got | Where-Object { [string]$_.name -eq [string]$w.name -and [string]$_.cat -eq $cat })
        Assert-That ($g.Count -eq 1) "Нет у телефона строки «$($w.name)» ($cat); телефон: $(($got | ForEach-Object { "$($_.cat):$($_.name)=$($_.sec)" }) -join ' | ')"
        Assert-Between ([int64]$g[0].sec - [int64]$w.seconds) -60 60 "«$($w.name)» ($cat): телефон $($g[0].sec) с минус окно $($w.seconds) с"
      }
      $gameRow = @($got | Where-Object { [string]$_.cat -eq "game" })
      Assert-That ($gameRow.Count -ge 1 -and [int64]$gameRow[0].sec -ge 600) "Нет игры 10 мин в списке телефона («Во что играл»)"
      $sites = @(@(List $p.programs) | Where-Object { $_.path -and [string]$_.kind -eq "site" })
      Assert-That (@(List $td.sites).Count -eq $sites.Count) "Сайтов у телефона $(@(List $td.sites).Count), в окне $($sites.Count)"

      $viol = @(List $p.violations)
      Assert-That ([int]$td.violations -eq $viol.Count) "Нарушений у телефона $($td.violations), в окне $($viol.Count)"
      $byp = @($viol | Where-Object { [string]$_.kind -eq "bypass" }).Count
      Assert-That ([int]$td.bypass -eq $byp) "Попыток обхода у телефона $($td.bypass), в окне $byp"

      # Первый вход сегодня — до начала игры (сеанс ребёнка на экране с настройки), «за компьютером» сейчас.
      $first = [int64]$td.firstMs
      Write-Log ("Первый вход {0} мс, начало игры {1} мс (часы ядра), сейчас {2} мс; за компьютером {3} с {4} мс" -f $first, $script:CoreStartMs, $script:CoreNowMs, $td.nowAt, $td.sinceMs)
      Assert-That ($first -gt 0 -and $first -le $script:CoreStartMs + 60000 -and $first -ge $script:CoreStartMs - 1800000) "Первый вход $first вне [начало игры − 30 мин; начало игры + 1 мин] ($($script:CoreStartMs))"
      Assert-That ([bool]$td.nowAt) "Телефон пишет «Не за компьютером», а ребёнок за компьютером"
      Assert-That ([int64]$td.sinceMs -ge $first -and [int64]$td.sinceMs -le $script:CoreNowMs) "«За компьютером с» $($td.sinceMs) вне [первый вход; сейчас]"
    }
  } catch {
    Write-RelayLog
    throw
  } finally {
    Stop-Relay
  }

  Write-Log "S59: статистика за сегодня на телефоне совпала с «Итогами» окна родителя: первый вход, за компьютером, игры и учёба, программы"
}
