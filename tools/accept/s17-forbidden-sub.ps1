# S17. Поддомен «Запрещённых» в Brave, учёба на сайте и подмена домена (R3, R14, R23, NFR-6, 8.2.5 п. 1, RM4-5,
# RM4-7, RM4-9, RM4-12, T10-1, T10-2, T10-5, T10-8, T10-9, PL-16, PL-22, PL-41).
# Часы ядра на паузе (Start-Child), кроме шага учёбы (10 с настоящего времени = около 10 мин ядра при ×60).
# Brave раннера — от имени ребёнка, своим профилем; проверяемые адреса — новыми вкладками окна с разрешённой
# probe-site.test (T10-2). Все домены в hosts раннера ведут на 127.0.0.1: из сети ничего не грузится.
# Дано: forbidden-site.test в «Запрещённых», study-site.test в «Учёбе» (до открытия Brave, T10-5).
# (а) Когда: открыт http://video.forbidden-site.test/pcb-secret-path?q=pcb-secret-query. Тогда: вкладка закрыта не
#     позже 2 с (допуск раннера 3 с), плашка «Сайт forbidden-site.test закрыт: он в списке запрещённых. Если он нужен, попроси родителей» (запись
#     списка из ответа AddSite), в журнале одна запись «попытка открыть запрещённое: сайт video.forbidden-site.test»
#     (домен вкладки); ни путь, ни параметры адреса не попали ни в журналы работы, ни в журнал родителя (R23, T10-9).
# (б) Учёба: 10 мин на study-site.test не тратят общий лимит (R3), идут в учёбу строкой сайта (RM4-7), не закрыты.
# (в) Запасная ветка подмены домена (PL-41, T10-8): на настоящей вкладке probe-two.test подмена TestGuard domain
#     даёт forbidden-site.test, служба просит закрыть, страж закрывает настоящую активную вкладку; подмена сразу снята.
#     Что закрыта именно настоящая вкладка, доказывает то, что после снятия подмены: окно Brave на месте, активна
#     вкладка, которая была активной до открытия probe-two.test (study-site.test из (б): соседняя слева), способ
#     закрытия ctrlW или invoke (как в Wait-TabClosed; wmClose закрыл бы окно целиком).
# Снимок: s17-site-plate.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы: internal/service/texts.go textPlateSite («Сайт %s закрыт: он в списке запрещённых. Если он нужен, попроси родителей», %s — запись
# списка); internal/service/journaltext.go evSiteForbiddenTry («попытка открыть запрещённое: сайт <домен вкладки>»).
$script:TryKind = "site_forbidden_try"
$script:TryPrefix = "попытка открыть запрещённое: сайт "
$script:SubDomain = "video.forbidden-site.test"
$script:SecretUrl = "http://video.forbidden-site.test/pcb-secret-path?q=pcb-secret-query"
$script:Secret = "pcb-secret"

Invoke-Scenario "S17" "Поддомен запрещённого сайта закрыт с плашкой, учёба на сайте не тратит лимит, подмена домена" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Brave установлен" {
    [void](Assert-BraveInstalled)
  }

  Step "Настройка: лимит 2 ч" {
    Start-Child 120
    Wait-ProgramsReady
  }

  Step "Дано: forbidden-site.test в «Запрещённых», study-site.test в «Учёбе» (до открытия Brave)" {
    $script:F = Add-PcbSite "forbidden-site.test" "blocked"
    Assert-That ($script:F -eq "forbidden-site.test") "AddSite вернул домен «$($script:F)», ждали forbidden-site.test"
    $script:S = Add-PcbSite "study-site.test" "study"
    Assert-That ($script:S -eq "study-site.test") "AddSite вернул домен «$($script:S)», ждали study-site.test"
    # Текст плашки — с записью списка из ответа программы (PL-22).
    $script:SitePlate = "Сайт $($script:F) закрыт: он в списке запрещённых. Если он нужен, попроси родителей"
  }

  Step "Brave новым окном на probe-site.test, на переднем плане" {
    Open-BraveHome
  }

  Step "(а) Поддомен запрещённого новой вкладкой: закрыта, активна probe-site.test, плашка" {
    $g0 = Get-GuardState
    Assert-That ([string]$g0.closedTab -ne $script:SubDomain) "До открытия closedTab уже «$($g0.closedTab)» (PL-16)"
    $sw = Open-Brave $script:SecretUrl
    [void](Wait-TabClosed $script:SubDomain $sw)
    [void](Wait-SitePlate $script:SitePlate "site" 5)
    Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
    Save-Screen "s17-site-plate"
  }

  Step "(а) Журнал: одна запись о попытке с доменом вкладки" {
    $want = $script:TryPrefix + $script:SubDomain
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $tries = @(Get-SiteEvents $script:TryKind $want)
    while ($tries.Count -lt 1 -and $sw.Elapsed.TotalSeconds -lt 5) {
      Start-Sleep -Milliseconds 500
      $tries = @(Get-SiteEvents $script:TryKind $want)
    }
    if ($tries.Count -lt 1) { throw "За 5 с в журнале нет «$want»; $(Get-SiteDiag)" }
    Start-Sleep -Seconds 2 # повторы closeTab записей не добавляют (раз в 60 с на домен, PL-12)
    $tries = @(Get-SiteEvents $script:TryKind $want)
    $tries | ForEach-Object { Write-Log "Журнал: $($_.kind) $($_.text)" }
    if ($tries.Count -ne 1) { throw "Записей «$want»: $($tries.Count), ждали 1; $(Get-SiteDiag)" }
  }

  Step "(а) R23: путь и параметры адреса не попали в журналы (T10-9)" {
    $logs = Join-Path (Join-Path $script:Data "logs") "*"
    $hits = @(Select-String -Path $logs -Pattern $script:Secret -SimpleMatch -ErrorAction SilentlyContinue)
    $files = @(Get-ChildItem -Path $logs -File -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }) -join ", "
    Write-Log "Журналы работы проверены на «$($script:Secret)»: файлы [$files], совпадений $($hits.Count)"
    Assert-That ($files -ne "") "В $($script:Data)\logs нет ни одного файла журнала: проверять нечего"
    Assert-That ($hits.Count -eq 0) "Адрес страницы в журнале работы (R23): $(($hits | ForEach-Object { "$($_.Filename):$($_.LineNumber): $($_.Line)" }) -join ' | ')"
    $all = Use-Parent { Format-Events }
    Assert-That (-not $all.Contains($script:Secret)) "Адрес страницы в журнале родителя (R23): $all"
  }

  Step "(б) Учёба на сайте: 10 мин не тратят лимит, идут в учёбу строкой сайта" {
    $sw = Open-Brave "http://$($script:S)/"
    [void](Wait-BraveDomain $script:S 15)
    $st0 = Get-Status
    $m0 = Get-MyToday
    Resume-Clock
    Start-Sleep -Seconds 10
    Suspend-Clock
    $g = Get-GuardState
    $st1 = Get-Status
    $m1 = Get-MyToday
    Write-Log "Учёба: остаток $($st0.remainingS) -> $($st1.remainingS) с, учёба $($m0.studyS) -> $($m1.studyS) с; $(Format-GuardState $g)"
    if ([string]$g.foregroundDomain -ne $script:S) { throw "За время учёбы вкладка $($script:S) перестала быть активной; $(Get-SiteDiag $g)" }
    if ([string]$g.closedTab -eq $script:S) { throw "Вкладка учёбы $($script:S) закрыта; $(Get-SiteDiag $g)" }
    $spent = [Math]::Abs([int]$st1.remainingS - [int]$st0.remainingS)
    Assert-Between $spent 0 60 "Общий остаток за 10 мин учёбы на сайте изменился (R3), с"
    Assert-Between ([int]$m1.studyS - [int]$m0.studyS) 540 660 "Учёба за 10 мин на сайте (MyToday.studyS), с"
    $rows = @(@(List $m1.programs) | Where-Object { [string]$_.kind -eq "site" -and [string]$_.path -eq $script:S -and [string]$_.category -eq "study" })
    $shown = (@(List $m1.programs) | ForEach-Object { "$($_.kind):$($_.path):$($_.category)=$($_.seconds)" }) -join ' | '
    Assert-That ($rows.Count -eq 1) "Строка сайта $($script:S) в «Учёбе» (kind site): $($rows.Count), ждали 1; строки: $shown"
    Assert-That ([int]$rows[0].seconds -ge 540) "Строка сайта $($script:S): $($rows[0].seconds) с, ждали не меньше 540; строки: $shown"
  }

  Step "(в) Подмена домена: настоящая вкладка probe-two.test закрыта как forbidden-site.test" {
    # Вкладка, которая станет активной, когда закроется probe-two.test (последняя вкладка окна, соседняя слева).
    $before = [string](Get-GuardState).foregroundDomain
    Assert-That ($before -ne "" -and $before -ne "probe-two.test") "До открытия probe-two.test активная вкладка «$before», ждали домен вкладки из (б)"
    $sw = Open-Brave "http://probe-two.test/"
    [void](Wait-BraveDomain "probe-two.test" 15)
    $g0 = Get-GuardState
    Assert-That ([string]$g0.closedTab -ne $script:F) "До подмены closedTab уже «$($g0.closedTab)» (PL-16)"
    Invoke-TestGuard "domain" "" $script:F
    $g = $null
    $closeMs = -1
    try {
      $wait = [Diagnostics.Stopwatch]::StartNew()
      while ($true) {
        $g = Get-GuardState
        if ([string]$g.closedTab -eq $script:F) { $closeMs = [int]$wait.ElapsedMilliseconds; break }
        if ($wait.Elapsed.TotalSeconds -gt 6) { break }
        Start-Sleep -Milliseconds 100
      }
    } finally {
      # Сразу снять: пока подмена висит, страж закрывал бы вкладку за вкладкой (T10-8).
      Invoke-TestGuard "domain"
    }
    if ([string]$g.closedTab -ne $script:F) { throw "За 6 с подмены домена $($script:F) вкладка не закрыта; $(Get-SiteDiag $g)" }
    $way = [string]$g.closedTabWay
    Write-Log ("Подмена домена: вкладка закрыта за {0} мс от подмены, способ {1}" -f $closeMs, $way)
    # Закрыта настоящая вкладка: после снятия подмены страж видит прежнюю вкладку, окно Brave на месте.
    try {
      [void](Wait-BraveDomain $before 5)
    } catch {
      throw "(в) После подмены и её снятия активной не стала вкладка $before, которая была активной до probe-two.test (закрыто окно или не та вкладка); $($_.Exception.Message)"
    }
    $win = Get-BraveWindow
    if ($null -eq $win) { throw "(в) После закрытия вкладки по подмене домена окна Brave нет (закрыто окно целиком, способ «$way»); $(Get-SiteDiag)" }
    if ($way -ne "ctrlW" -and $way -ne "invoke") { throw "(в) Вкладка по подмене домена закрыта способом «$way», ждали ctrlW или invoke (wmClose закрывает окно целиком); $(Get-SiteDiag)" }
    if ($way -eq "invoke") {
      Write-Host "::warning title=Ctrl+W от SYSTEM::вкладка probe-two.test (подмена $($script:F)) закрыта кнопкой вкладки (invoke), Ctrl+W не сработал (Open T7)"
    }
    Write-Log "Подмена домена: после снятия активна $before, окно Brave: $win"
    $want = $script:TryPrefix + $script:F
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $tries = @(Get-SiteEvents $script:TryKind $want)
    while ($tries.Count -lt 1 -and $sw.Elapsed.TotalSeconds -lt 5) {
      Start-Sleep -Milliseconds 500
      $tries = @(Get-SiteEvents $script:TryKind $want)
    }
    $tries | ForEach-Object { Write-Log "Журнал: $($_.kind) $($_.text)" }
    if ($tries.Count -lt 1) { throw "В журнале нет «$want» после подмены домена; $(Get-SiteDiag)" }
  }

  Step "Brave закрыт" {
    Stop-Brave
  }

  Write-Log "S17: поддомен запрещённого закрыт с плашкой записи списка, адрес страницы не в журналах, учёба на сайте не тратит лимит, подмена домена закрыла вкладку"
}
