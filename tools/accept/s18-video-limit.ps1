# S18. Лимит «Видео и соцсети» 20 мин в Brave (R6, R9, R14, NFR-6, 8.2.2, 8.2.5 п. 3, D-54, D-55, RM4-6, RM4-7,
# RM4-9, T10-1, T10-2, T10-4, T10-7, PL-14, PL-16, PL-22).
# Часы ядра идут в 60 раз быстрее (ClockSpeed=60): минута ядра это секунда настоящего времени; сроки плашек идут
# по настоящим часам. Brave раннера — от имени ребёнка, своим профилем; youtube.com — новой вкладкой окна с
# разрешённой probe-site.test (T10-2). youtube.com в hosts раннера ведёт на 127.0.0.1: из сети ничего не грузится.
# Дано: общий лимит 60 мин, видео и соцсети 20 мин; youtube.com в стартовом наборе «Видео и соцсетей» (R14, из
# ответа программы). Когда: 20 мин youtube.com на переднем плане. Тогда: предупреждения видео 15, 5, 1 (30 при
# лимите ровно 20 мин не взводится, PL-14; общих порогов нет: общего остаётся 40 мин), звук на каждом; в 0 видео
# вкладка youtube.com закрыта не позже 2 с (допуск раннера 3 с), активна probe-site.test, плашка «Время на видео и
# соцсети на сегодня закончилось. Общего времени осталось 40 мин» (±1 мин часов ядра), экрана блокировки нет;
# другие сайты открываются; повторное открытие youtube.com снова закрыто с той же плашкой; в журнале «закрыт сайт
# youtube.com: время на видео и соцсети закончилось», без нарушений и обхода.
# Снимки: s18-video-board.png (табло на пороге 15: заголовок «ОСТАЛОСЬ НА ВИДЕО:», разбирает visual-reviewer),
# s18-video-over.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы и ядра: internal/ipc/status.go NewGuardWarningFor и warnOneMinute для core.WarnVideo (страж
# дописывает к 1 мин отсчёт «: 0:59»); internal/core/texts.go VideoOverPlateText (минуты общего остатка вверх,
# RemainingMinutes); internal/service/journaltext.go evSiteClosedVideo.
$script:Video15 = "Через 15 минут время на видео и соцсети закончится"
$script:Video5 = "Через 5 минут время на видео и соцсети закончится"
$script:Video1 = "Осталась 1 минута на видео и соцсети"
$script:VideoOverRe = '^Время на видео и соцсети на сегодня закончилось\. Общего времени осталось (39|40|41) мин$'
$script:Video = "youtube.com"
$script:ClosedText = "закрыт сайт youtube.com: время на видео и соцсети закончилось"

Invoke-Scenario "S18" "Видео и соцсети 20 мин из 60: предупреждения, вкладка YouTube закрыта, другие сайты открываются" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Brave установлен" {
    [void](Assert-BraveInstalled)
  }

  Step "Настройка: общий лимит 60 мин, видео и соцсети 20 мин" {
    Start-Child 60 $true -1 $null 20
    Wait-ProgramsReady
    $st = Get-Status
    Assert-That ($st.limitS -eq 3600) "Лимит дня $($st.limitS) с, ждали 3600"
    Assert-That ((-not $st.videoUnlimited) -and $st.videoLimitS -eq 1200 -and $st.videoRemainingS -eq 1200) "Лимит видео: $($st.videoRemainingS) с из $($st.videoLimitS), без лимита=$($st.videoUnlimited); ждали 1200 из 1200"
    Assert-That (-not $st.closeVideo) "Видео закрыто с начала дня (closeVideo)"
  }

  Step "Дано: youtube.com в стартовом наборе «Видео и соцсетей» (R14)" {
    $sites = Get-PcbSites
    $row = @(@(List $sites.sites) | Where-Object { [string]$_.domain -eq $script:Video })
    $video = (@(List $sites.sites) | Where-Object { [string]$_.kind -eq "video_social" } | ForEach-Object { $_.domain }) -join ', '
    Write-Log "«Видео и соцсети» программы: $video"
    Assert-That ($row.Count -eq 1 -and [string]$row[0].kind -eq "video_social") "youtube.com в списках: $(($row | ForEach-Object { "$($_.domain):$($_.kind)" }) -join ', '); ждали одну строку video_social (стартовый набор R14)"
  }

  Step "Brave новым окном на probe-site.test, youtube.com новой вкладкой на переднем плане" {
    Open-BraveHome
    $g0 = Get-GuardState
    Assert-That ([string]$g0.closedTab -ne $script:Video) "До открытия closedTab уже «$($g0.closedTab)» (PL-16)"
    [void](Open-Brave "http://$($script:Video)/")
    [void](Wait-BraveDomain $script:Video 15)
  }

  Step "1. Предупреждения видео 15, 5, 1 со звуком, снимок табло" {
    Resume-Clock
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s) -contains 15 } 30 "табло «Через 15 минут время на видео и соцсети закончится»"
    Save-Screen "s18-video-board"
    Write-Log "Табло за 15 мин: $(Format-GuardState $g)"
    $g = Wait-GuardState { param($s) (Get-WarnThresholds $s) -contains 1 } 30 "плашка «Осталась 1 минута на видео и соцсети»"
    Write-Log "Показанные плашки: $(Format-Warnings $g)"
    $thr = (Get-WarnThresholds $g) -join ","
    Assert-That ($thr -eq "15,5,1") "Показанные плашки: $thr, ждали 15,5,1 (только видео; 30 не взводится, PL-14; общих порогов нет)"
    $w = @(List $g.warnings)
    Assert-That ([string]$w[0].text -eq $script:Video15) "Текст за 15 мин: «$($w[0].text)», ждали «$($script:Video15)»"
    Assert-That ([string]$w[1].text -eq $script:Video5) "Текст за 5 мин: «$($w[1].text)», ждали «$($script:Video5)»"
    Assert-That (([string]$w[2].text).StartsWith($script:Video1)) "Текст за 1 мин: «$($w[2].text)», ждали «$($script:Video1)…»"
    Assert-That (-not $g.lockShown) "Экран блокировки показан до конца видео: $(Format-GuardState $g)"
    Write-Log "Звуки: $(@(List $g.sounds | ForEach-Object { "$($_.threshold):$($_.ok)" }) -join ',')"
    $snd = (Get-SoundThresholds $g) -join ","
    Assert-That ($snd -eq "15,5,1") "Звуки на порогах: $snd, ждали 15,5,1 по одному разу"
  }

  Step "2. Видео кончилось: вкладка youtube.com закрыта, плашка, экрана нет, остатки" {
    $st = Wait-Status { param($st) $st.closeVideo } 60 "видео закрыто (closeVideo)"
    $since = [Diagnostics.Stopwatch]::StartNew()
    Suspend-Clock
    Write-Log "Видео закрыто: остаток видео $($st.videoRemainingS) с, общий $($st.remainingS) с; часы остановлены"
    [void](Wait-TabClosed $script:Video $since)
    $g = Wait-SitePlate $script:VideoOverRe "videoOver" 5 -Regex
    Assert-That (-not $g.lockShown) "Экран блокировки показан, ждали только плашку: $(Format-GuardState $g)"
    Assert-That (@(Get-BigPcbWindows).Count -eq 0) "Окно PC Block на весь экран: $((Get-BigPcbWindows) -join ' ; ')"
    Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
    Save-Screen "s18-video-over"
    $st = Get-Status
    Assert-Between $st.remainingS 2340 2460 "Общий остаток после 20 мин видео, с"
    Assert-That ([int]$st.videoRemainingS -eq 0) "Остаток видео $($st.videoRemainingS) с, ждали 0"
    Assert-That ($st.lock -eq "none") "Блокировка «$($st.lock)» (статус $($st.status)): ждали без блокировки"
    $m = Get-MyToday
    Assert-Between $m.videoS 1140 1200 "Видео и соцсети за сегодня (MyToday.videoS), с"
    $rows = @(@(List $m.programs) | Where-Object { [string]$_.kind -eq "site" -and [string]$_.path -eq $script:Video -and [string]$_.category -eq "video_social" })
    $shown = (@(List $m.programs) | ForEach-Object { "$($_.kind):$($_.path):$($_.category)=$($_.seconds)" }) -join ' | '
    Assert-That ($rows.Count -eq 1) "Строка сайта youtube.com в «Видео и соцсетях» (kind site): $($rows.Count), ждали 1; строки: $shown"
  }

  Step "3. Другие сайты открываются; youtube.com снова закрыт с той же плашкой" {
    [void](Wait-PlateGone "videoOver" 12)
    [void](Assert-TabStays "probe-site.test" 6 "videoOver")
    $sw = Open-Brave "http://$($script:Video)/"
    [void](Wait-SitePlate $script:VideoOverRe "videoOver" 5 -Regex)
    Write-Log "Повторное открытие youtube.com: плашка через $($sw.ElapsedMilliseconds) мс"
    [void](Wait-BraveDomain "probe-site.test" 5)
    $g = Get-GuardState
    Assert-That (-not $g.lockShown) "Экран блокировки после повторного открытия youtube.com: $(Format-GuardState $g)"
  }

  Step "4. Журнал: «закрыт сайт youtube.com: время на видео и соцсети закончилось», без нарушений и обхода" {
    $closed = @(Get-SiteEvents "site_closed_video" $script:ClosedText)
    $closed | ForEach-Object { Write-Log "Журнал: $($_.kind) $($_.text)" }
    if ($closed.Count -lt 1) { throw "В журнале нет «$($script:ClosedText)»; $(Get-SiteDiag)" }
    $try = @(Use-Parent { @(Get-Events | Where-Object { [string]$_.kind -eq "site_forbidden_try" }) })
    if ($try.Count -ne 0) { throw "Конец видео записан нарушением: $(($try | ForEach-Object { $_.text }) -join ' | ')" }
    $bypass = @(Use-Parent { Find-Events 'попытка обойти защиту' })
    if ($bypass.Count -ne 0) { throw "В журнале запись об обходе: $(($bypass | ForEach-Object { $_.text }) -join ' | ')" }
  }

  Step "Brave закрыт" {
    Stop-Brave
  }

  Write-Log "S18: видео 15, 5, 1 со звуком, в 0 вкладка YouTube закрыта с плашкой об остатке 40 мин, другие сайты открываются"
}
