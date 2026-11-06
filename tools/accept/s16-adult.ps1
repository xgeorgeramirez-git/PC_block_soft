# S16. Сайты 18+ в Brave (R14, R23, NFR-6, 8.2.5 п. 2, RM4-3, RM4-5, RM4-9, T10-1…T10-6, PL-12, PL-16, PL-22).
# Часы ядра на паузе (Start-Child): закрытие вкладки, плашки и журнал идут по настоящим часам. Brave раннера
# запускается от имени ребёнка (пользователь раннера) со своим профилем; сначала новым окном на разрешённом
# http://probe-site.test/, проверяемый адрес — новой вкладкой того же окна (T10-2): закрыта вкладка, а не окно.
# Домен «18+» — из образца сборки PCB_ADULT_SAMPLE (шаг windows.yml «Полный список 18+», PL-22), не IP и не
# .test; в hosts раннера он ведёт на 127.0.0.1 (шаг «Brave для пробы и приёмки»): из сети ничего не грузится.
# Дано: «18+» у ребёнка включён (по умолчанию), домена нет в своих списках родителя (свои списки сильнее, RM4-5).
# Когда: домен открыт в Brave. Тогда: вкладка закрыта не позже 2 с (допуск раннера 3 с), активна probe-site.test,
# плашка «Сайт закрыт: он в списке сайтов для взрослых» (без домена), в журнале одна запись «попытка открыть
# запрещённое: сайт для взрослых <домен>». Затем «18+» выключен: тот же домен открывается и не закрывается.
# Снимок: s16-adult-plate.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Тексты службы: internal/service/texts.go textPlateAdult, textAdultOff; internal/service/journaltext.go
# (evSiteForbiddenTry с kind adult); вид записи — internal/service/sites_svc.go evSiteForbiddenTry, evSiteAdultFilter.
$script:AdultPlate = "Сайт закрыт: он в списке сайтов для взрослых"
$script:AdultOffText = "сайты 18+ не блокируются"
$script:TryKind = "site_forbidden_try"

# Домен «18+» из образца сборки (T10-3): первый, кто не IP и не .test.
function Get-AdultDomain {
  $raw = [string]$env:PCB_ADULT_SAMPLE
  Assert-That ($raw -ne "") "PCB_ADULT_SAMPLE пуст: шаг windows.yml «Полный список 18+» не передал образец"
  $cand = @($raw -split ',' | ForEach-Object { $_.Trim().ToLowerInvariant() } |
    Where-Object { $_ -match '^(?!\d+(\.\d+){3}$)[a-z0-9-]+(\.[a-z0-9-]+)+$' -and -not $_.EndsWith(".test") })
  Write-Log "Образец 18+ сборки: $raw; подходят: $($cand -join ', ')"
  Assert-That ($cand.Count -gt 0) "PCB_ADULT_SAMPLE «$raw»: ни одного домена (только IP или .test)"
  return $cand[0]
}

# Записи «попытка открыть запрещённое: сайт для взрослых <A>» в журнале родителя.
function Get-AdultTries {
  return @(Get-SiteEvents $script:TryKind "попытка открыть запрещённое: сайт для взрослых $($script:A)")
}

Invoke-Scenario "S16" "Сайт 18+ в Brave: вкладка закрыта, плашка без домена, одна запись; «18+» выключен: открывается" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Brave установлен" {
    $script:BraveExe = Assert-BraveInstalled
  }

  Step "Домен 18+ из образца сборки, в hosts раннера" {
    $script:A = Get-AdultDomain
    Assert-That (Test-InHosts $script:A) "Домена $($script:A) нет в hosts раннера: Brave ушёл бы в сеть (шаг windows.yml «Brave для пробы и приёмки»)"
    Write-Log "Домен 18+ для проверки: $($script:A)"
  }

  Step "Настройка: лимит 2 ч" {
    Start-Child 120
    Wait-ProgramsReady
  }

  Step "Дано: «18+» у ребёнка включён, домена нет в своих списках" {
    $sites = Get-PcbSites
    $kids = @(List $sites.children)
    Write-Log "Переключатели 18+: $(($kids | ForEach-Object { "$($_.name)=$($_.on)" }) -join ', '); свои списки: $((@(List $sites.sites) | ForEach-Object { "$($_.domain):$($_.kind)" }) -join ', ')"
    Assert-That ($kids.Count -ge 1 -and $kids[0].on -eq $true) "«Блокировать сайты 18+» у ребёнка не включён по умолчанию: $(($kids | ForEach-Object { "$($_.name)=$($_.on)" }) -join ', ')"
    $a = $script:A
    $own = @(@(List $sites.sites) | Where-Object { $a -eq [string]$_.domain -or $a.EndsWith("." + [string]$_.domain) })
    Assert-That ($own.Count -eq 0) "Домен $a совпадает со своим списком родителя (он сильнее 18+, RM4-5): $(($own | ForEach-Object { "$($_.domain):$($_.kind)" }) -join ', ')"
  }

  Step "Brave новым окном на probe-site.test, на переднем плане" {
    Open-BraveHome
  }

  Step "Brave разрешён службой (диагностика T10-6)" {
    $bp = ([PcbNative]::LongPath($script:BraveExe)).ToLowerInvariant()
    $list = Get-ProgramsList
    $pend = @(Find-Pending $list $bp)
    $row = @(@(List $list.programs) | Where-Object { ([string]$_.path) -eq $bp })
    if ($row.Count -gt 0) {
      Write-Log "Brave в «Программах»: категория $($row[0].category), источник $($row[0].source), издатель «$($row[0].publisher)», скрыт $($row[0].hidden)"
    } else {
      Write-Log "Строки $bp в «Программах» нет (снимок мог Brave в профиле не взять); итоговое доказательство: закрытие вкладки"
    }
    Assert-That ($pend.Count -eq 0) "Служба считает Brave незнакомым (правило профиля PL-20, T6-5): $bp в «Ждут решения», издатель «$($pend[0].publisher)»"
  }

  Step "1. Домен 18+ новой вкладкой: закрыта, активна probe-site.test, плашка без домена" {
    $g0 = Get-GuardState
    Assert-That ([string]$g0.closedTab -ne $script:A) "До открытия closedTab уже «$($g0.closedTab)» (PL-16)"
    $sw = Open-Brave "http://$($script:A)/"
    [void](Wait-TabClosed $script:A $sw)
    $g = Wait-SitePlate $script:AdultPlate "adult" 5
    Assert-That (-not ([string]$g.plate).Contains($script:A)) "Плашка 18+ показывает домен: «$($g.plate)» (R23, 8.2.5 п. 2)"
    Start-Sleep -Milliseconds 500 # поток окон перерисовывает плашку по своему таймеру
    Save-Screen "s16-adult-plate"
  }

  Step "2. Журнал: одна запись «попытка открыть запрещённое: сайт для взрослых»" {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $tries = @(Get-AdultTries)
    while ($tries.Count -lt 1 -and $sw.Elapsed.TotalSeconds -lt 5) {
      Start-Sleep -Milliseconds 500
      $tries = @(Get-AdultTries)
    }
    if ($tries.Count -lt 1) { throw "За 5 с в журнале нет «попытка открыть запрещённое: сайт для взрослых $($script:A)»; $(Get-SiteDiag)" }
    Start-Sleep -Seconds 2 # повторы closeTab за это время записей не добавляют (раз в 60 с на домен, PL-12)
    $tries = @(Get-AdultTries)
    $tries | ForEach-Object { Write-Log "Журнал: $($_.kind) $($_.text)" }
    if ($tries.Count -ne 1) { throw "Записей о попытке открыть $($script:A): $($tries.Count), ждали 1; $(Get-SiteDiag)" }
  }

  Step "3. «18+» выключен: тот же домен открывается и не закрывается" {
    $r = Use-Parent { Send-Parent "SetAdultFilter" @{ childId = (Get-ChildId); on = $false } }
    Assert-Reply $r "SetAdultFilter on=false"
    Write-Log "SetAdultFilter: «$($r.payload.message)»"
    $off = @(Get-SiteEvents "site_adult_filter" $script:AdultOffText)
    if ($off.Count -lt 1) { throw "В журнале нет «$($script:AdultOffText)»; $(Get-SiteDiag)" }
    [void](Wait-PlateGone "adult" 12)
    $sw = Open-Brave "http://$($script:A)/"
    [void](Wait-BraveDomain $script:A 10)
    Write-Log "Домен $($script:A) открыт снова за $($sw.ElapsedMilliseconds) мс"
    [void](Assert-TabStays $script:A 6 "adult")
    $tries = @(Get-AdultTries)
    if ($tries.Count -ne 1) { throw "После выключения 18+ записей о попытке открыть $($script:A): $($tries.Count), ждали по-прежнему 1; $(Get-SiteDiag)" }
  }

  Step "Brave закрыт" {
    Stop-Brave
  }

  Write-Log "S16: сайт 18+ закрыт вкладкой с плашкой без домена и одной записью; при выключенном 18+ открывается"
}
