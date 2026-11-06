# S20. Правила Brave в учётке ребёнка (R15, FR-31, ТЗ 3.4, RM4-10, RM4-10-итог, T10-10, T10-11, PL-12, PL-16, PL-41).
# Ребёнок — пользователь раннера: служба пишет ветку HKU\<SID раннера>\Software\Policies\BraveSoftware\Brave, Brave
# раннера применяет её как пользовательские политики (проба T0). Каждый Brave — со свежим профилем приёмки.
# 1. Контроль до правил (ребёнка ещё нет): ветки нет; в окне Brave есть кнопка с «VPN» (UIA) и Alt+Shift+N
#    открывает новое окно (Tor). Нет кнопки или окна — ::warning, дальше VPN и Tor проверяются по реестру.
# 2. Ребёнок настроен, probe «blocked-probe.test» в «Запрещённых». Тогда: TorDisabled=1, BraveVPNDisabled=1 (DWORD),
#    DnsOverHttpsMode="off", ExtensionInstallBlocklist\1="*", URLBlocklist\1="blocked-probe.test".
# 3. Brave ребёнка: кнопок окна не меньше 5, ни одной с «VPN»; Alt+Shift+N окна Tor не открывает.
# 4. URLBlocklist: родитель поставил паузу (служба на паузе вкладки не закрывает), сервер приёмки на 127.0.0.1:8765
#    (порт 80 на раннере не поднять, RM4-10-итог). Разрешённый probe-site.test:8765 открывается страницей сервера
#    (контроль), blocked-probe.test:8765 — нет: страница Brave «заблокировано», к серверу запроса нет.
# 5. brave://policy — снимок (UIA страницу не читает, разбирает visual-reviewer).
# 6. Ребёнок стёр ветку: сверка раз в 60 с вернула все значения, журнал родителя «правила Brave восстановлены».
# Пункт меню «Новое окно Tor» и кнопку VPN руками — вручную на запасном ПК. Снимки: s20-brave-toolbar.png,
# s20-brave-policy.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Ветка и значения: internal/service/brave.go Values; текст журнала: internal/service/journaltext.go evBraveRestored,
# строка журнала работы: internal/service/brave.go («правила Brave у <имя> восстановлены»).
$script:BraveSub = "$($script:MySid)\Software\Policies\BraveSoftware\Brave"
$script:B = "Registry::HKEY_USERS\$($script:BraveSub)"
$script:Blocked = "blocked-probe.test"
$script:RestoredKind = "brave_restored"
$script:Port = 8765
$script:VpnControl = $false
$script:TorControl = $false
$script:Listener = $null
$script:ServerJob = $null
$script:HostLog = $null

# Расхождения ветки правил Brave с ожидаемой (шаг 2); пусто — всё на месте. Читает реестр напрямую (.NET).
function Get-BraveRulesProblems {
  $k = [Microsoft.Win32.Registry]::Users.OpenSubKey($script:BraveSub)
  if (-not $k) { return @("ветки $($script:B) нет") }
  $p = @()
  try {
    foreach ($n in @("TorDisabled", "BraveVPNDisabled")) {
      $v = $k.GetValue($n, $null)
      if ($null -eq $v) { $p += "$n нет"; continue }
      $kind = $k.GetValueKind($n)
      if ($kind -ne [Microsoft.Win32.RegistryValueKind]::DWord -or [int]$v -ne 1) { $p += "$n=$v ($kind), ждали 1 (DWord)" }
    }
    $dns = $k.GetValue("DnsOverHttpsMode", $null)
    if ($null -eq $dns) { $p += "DnsOverHttpsMode нет" }
    elseif ($k.GetValueKind("DnsOverHttpsMode") -ne [Microsoft.Win32.RegistryValueKind]::String -or [string]$dns -ne "off") { $p += "DnsOverHttpsMode=«$dns» ($($k.GetValueKind('DnsOverHttpsMode'))), ждали «off» (String)" }
    foreach ($sub in @(@{ Key = "ExtensionInstallBlocklist"; Want = "*" }, @{ Key = "URLBlocklist"; Want = $script:Blocked })) {
      $sk = $k.OpenSubKey($sub.Key)
      if (-not $sk) { $p += "подключа $($sub.Key) нет"; continue }
      try {
        $v = $sk.GetValue("1", $null)
        if ([string]$v -ne $sub.Want) { $p += "$($sub.Key)\1=«$v», ждали «$($sub.Want)»" }
      } finally { $sk.Close() }
    }
  } finally { $k.Close() }
  return $p
}

# Все значения ветки одной строкой (журнал сценария).
function Format-BraveRules {
  $k = [Microsoft.Win32.Registry]::Users.OpenSubKey($script:BraveSub)
  if (-not $k) { return "ветки нет" }
  try {
    $parts = @($k.GetValueNames() | ForEach-Object { "$_=$($k.GetValue($_)) ($($k.GetValueKind($_)))" })
    foreach ($sn in $k.GetSubKeyNames()) {
      $sk = $k.OpenSubKey($sn)
      try { $parts += @($sk.GetValueNames() | ForEach-Object { "$sn\$_=$($sk.GetValue($_))" }) } finally { $sk.Close() }
    }
    return ($parts -join '; ')
  } finally { $k.Close() }
}

# Кнопки с «VPN» в названии (без учёта регистра).
function Select-Vpn($Names) {
  return @(@($Names) | Where-Object { ([string]$_).ToUpperInvariant().Contains("VPN") })
}

# Alt+Shift+N в окне Brave на переднем плане; возвращает, прибавилось ли окно Brave за Seconds секунд.
function Test-TorWindowOpens([double]$Seconds = 5) {
  $n0 = Get-BraveWindowCount
  Send-BraveKeys "%+n"
  $sw = [Diagnostics.Stopwatch]::StartNew()
  while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
    $n = Get-BraveWindowCount
    if ($n -gt $n0) { Write-Log ("Alt+Shift+N: окон Brave {0} → {1} за {2:N1} с" -f $n0, $n, $sw.Elapsed.TotalSeconds); return $true }
    Start-Sleep -Milliseconds 250
  }
  Write-Log "Alt+Shift+N: окон Brave по-прежнему $n0 за $Seconds с"
  return $false
}

# Удаляет профиль приёмки после Stop-Brave (файлы отпускаются не сразу): следующий Brave — со свежим профилем.
function Remove-BraveProfile {
  for ($i = 0; $i -lt 10 -and (Test-Path -LiteralPath $script:BraveProfile); $i++) {
    Remove-Item -LiteralPath $script:BraveProfile -Recurse -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $script:BraveProfile) { Start-Sleep -Milliseconds 500 }
  }
  Assert-That (-not (Test-Path -LiteralPath $script:BraveProfile)) "Профиль Brave $($script:BraveProfile) не удалён за 5 с"
}

# Сервер приёмки на 127.0.0.1:8765 в своём потоке: на любой запрос — страница с заголовком «probe-ok»; заголовок
# Host каждого запроса (и строка запроса) — в очередь HostLog. Чтение запроса — не дольше 1 с (Brave может
# открыть соединение и ничего не прислать или начать с TLS).
function Start-ProbeServer {
  $script:HostLog = [System.Collections.Concurrent.ConcurrentQueue[string]]::new()
  $script:Listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $script:Port)
  $script:Listener.Start()
  $script:ServerJob = Start-ThreadJob -ArgumentList $script:Listener, $script:HostLog -ScriptBlock {
    param($l, $log)
    $body = [Text.Encoding]::ASCII.GetBytes("<!DOCTYPE html><html><head><title>probe-ok</title></head><body>probe-ok</body></html>")
    while ($true) {
      try { $c = $l.AcceptTcpClient() } catch { break }
      try {
        $c.ReceiveTimeout = 1000
        $c.SendTimeout = 2000
        $st = $c.GetStream()
        $rd = [IO.StreamReader]::new($st, [Text.Encoding]::ASCII, $false, 1024, $true)
        $first = [string]$rd.ReadLine()
        $hostName = ""
        while ($true) {
          $line = $rd.ReadLine()
          if ($null -eq $line -or $line -eq "") { break }
          if ($line -match '^(?i)host:\s*(.+)$') { $hostName = $Matches[1].Trim() }
        }
        $log.Enqueue("$hostName|$first")
        $hdr = [Text.Encoding]::ASCII.GetBytes("HTTP/1.1 200 OK`r`nContent-Type: text/html; charset=utf-8`r`nContent-Length: $($body.Length)`r`nConnection: close`r`n`r`n")
        $st.Write($hdr, 0, $hdr.Length)
        $st.Write($body, 0, $body.Length)
        $st.Flush()
      } catch {
        $log.Enqueue("|ошибка: $($_.Exception.Message)")
      } finally {
        try { $c.Close() } catch {}
      }
    }
  }
  Write-Log "Сервер приёмки: 127.0.0.1:$($script:Port)"
}

# Останавливает сервер приёмки. Ничего не бросает (зовётся из finally шага).
function Stop-ProbeServer {
  try { if ($script:Listener) { $script:Listener.Stop() } } catch {}
  try { if ($script:ServerJob) { Stop-Job $script:ServerJob -ErrorAction SilentlyContinue; Remove-Job $script:ServerJob -Force -ErrorAction SilentlyContinue } } catch {}
  $script:Listener = $null
  $script:ServerJob = $null
}

# Запросы к серверу приёмки: «Host|строка запроса».
function Get-ProbeRequests {
  if (-not $script:HostLog) { return @() }
  return @($script:HostLog.ToArray())
}

# Запросы с заголовком Host = HostName (порт не важен).
function Select-HostRequests([string]$HostName) {
  return @(Get-ProbeRequests | Where-Object { (([string]$_ -split '\|', 2)[0] -split ':', 2)[0] -eq $HostName })
}

Invoke-Scenario "S20" "Правила Brave: нет кнопки VPN и окна Tor, запрещённый домен не грузится, ветку правил служба возвращает" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  Step "Brave установлен" {
    [void](Assert-BraveInstalled)
  }

  Step "1. Контроль до правил: ветки нет, в Brave есть кнопка VPN и окно Tor" {
    Assert-That (-not (Test-Path -LiteralPath $script:B)) "До настройки ребёнка ветка правил Brave уже есть: $(Format-BraveRules)"
    Import-Uia
    $sw = Open-Brave "http://probe-site.test/" -NewWindow
    Wait-Until { $null -ne (Get-BraveWindow) } 20 "окно Brave"
    Write-Log "Окно Brave за $($sw.ElapsedMilliseconds) мс: $(Get-BraveWindow)"
    Set-BraveForeground -NoGuard
    $names = @(Wait-BraveButtons 5 15)
    $vpn = @(Select-Vpn $names)
    if ($vpn.Count -ge 1) {
      $script:VpnControl = $true
      Write-Log "Контроль VPN: кнопки без правил: $($vpn -join ', ')"
    } else {
      Write-Host "::warning title=S20 контроль VPN::в Brave без правил нет кнопки с «VPN» (кнопки: $($names -join ', ')); «нет VPN» проверяется только по BraveVPNDisabled в реестре"
      Write-Log "Контроль VPN не прошёл: кнопок с «VPN» нет"
    }
    Set-BraveForeground -NoGuard
    $script:TorControl = Test-TorWindowOpens 5
    if (-not $script:TorControl) {
      Write-Host "::warning title=S20 контроль Tor::в Brave без правил Alt+Shift+N не открыл новое окно; «нет окна Tor» проверяется только по TorDisabled в реестре"
    }
    Stop-Brave
    Remove-BraveProfile
  }

  Step "2. Ребёнок настроен, blocked-probe.test в «Запрещённых»: правила в ветке ребёнка" {
    Start-Child 120
    $d = Add-PcbSite $script:Blocked "blocked"
    Assert-That ($d -eq $script:Blocked) "AddSite вернул домен «$d», ждали $($script:Blocked)"
    $script:LastProblems = @()
    Wait-Until { $script:LastProblems = @(Get-BraveRulesProblems); $script:LastProblems.Count -eq 0 } 10 "правила Brave в ветке ребёнка ($($script:B))"
    Write-Log ("Правила Brave за {0:N1} с: {1}" -f $script:WaitElapsed, (Format-BraveRules))
  }

  Step "3. Brave ребёнка: нет кнопки VPN, Alt+Shift+N не открывает окно Tor" {
    $sw = Open-Brave "http://probe-site.test/" -NewWindow
    Wait-Until { $null -ne (Get-BraveWindow) } 20 "окно Brave"
    Write-Log "Окно Brave за $($sw.ElapsedMilliseconds) мс: $(Get-BraveWindow)"
    Set-BraveForeground
    $names = @(Wait-BraveButtons 5 15)
    $vpn = @(Select-Vpn $names)
    Assert-That ($vpn.Count -eq 0) "В Brave ребёнка кнопки с «VPN»: $($vpn -join ', ') (BraveVPNDisabled не применён; контроль до правил: $($script:VpnControl)); ветка: $(Format-BraveRules)"
    Write-Log "Кнопок с «VPN» нет (контроль до правил: $($script:VpnControl))"
    Save-Screen "s20-brave-toolbar"
    Set-BraveForeground
    $opened = Test-TorWindowOpens 5
    Assert-That (-not $opened) "Alt+Shift+N в Brave ребёнка открыл новое окно (TorDisabled не применён; контроль до правил: $($script:TorControl)); ветка: $(Format-BraveRules)"
    Write-Log "Окно Tor не открылось (контроль до правил: $($script:TorControl))"
  }

  Step "4. URLBlocklist: разрешённый домен — страница сервера, blocked-probe.test — нет" {
    $r = Use-Parent { Send-Parent "Pause" @{ childId = (Get-ChildId); preset = "hour1" } }
    Assert-Reply $r "Pause hour1"
    $st = Wait-Status { param($x) $x.status -eq "paused" } 10 "статус «пауза» после Pause hour1"
    Write-Log "Пауза: статус $($st.status) (служба на паузе вкладки не закрывает)"
    try {
      Start-ProbeServer
      [void](Open-Brave "http://probe-site.test:$($script:Port)/")
      [void](Wait-BraveDomain "probe-site.test" 15)
      $title = ""
      try {
        Wait-Until { $script:T = Get-BraveTitle; $script:T.StartsWith("probe-ok") } 10 "страница сервера приёмки в Brave"
      } catch {
        throw "Контроль не прошёл: http://probe-site.test:$($script:Port)/ за 10 с не открылся страницей сервера (заголовок окна «$(Get-BraveTitle)», передний план $([PcbNative]::Foreground()); запросы к серверу: $((Get-ProbeRequests) -join ' ; ')); без контроля «не грузится» неотличимо от «сервер не отвечает» (T10-11)"
      }
      Write-Log "Контроль: заголовок окна «$($script:T)»; запросы к серверу: $((Get-ProbeRequests) -join ' ; ')"
      Assert-That (@(Select-HostRequests "probe-site.test").Count -ge 1) "Заголовок «$($script:T)», но к серверу нет запроса с Host probe-site.test: $((Get-ProbeRequests) -join ' ; ')"
      [void](Open-Brave "http://$($script:Blocked):$($script:Port)/")
      [void](Wait-BraveDomain $script:Blocked 15)
      $sw = [Diagnostics.Stopwatch]::StartNew()
      while ($sw.Elapsed.TotalSeconds -lt 3) {
        $title = Get-BraveTitle
        if ($title.StartsWith("probe-ok")) { throw "$($script:Blocked) открылся страницей сервера (заголовок «$title»): URLBlocklist не применён; ветка: $(Format-BraveRules)" }
        Start-Sleep -Milliseconds 200
      }
      $hits = @(Select-HostRequests $script:Blocked)
      Assert-That ($hits.Count -eq 0) "К серверу приёмки дошли запросы с Host $($script:Blocked): $($hits -join ' ; ') (URLBlocklist не применён)"
      Write-Log "$($script:Blocked): заголовок окна «$title», запросов к серверу нет; все запросы: $((Get-ProbeRequests) -join ' ; ')"
    } finally {
      Stop-ProbeServer
    }
  }

  Step "5. brave://policy: снимок" {
    [void](Open-Brave "brave://policy" -NewWindow)
    Start-Sleep -Seconds 4
    Write-Log "Передний план: $([PcbNative]::Foreground())"
    Save-Screen "s20-brave-policy"
    Stop-Brave
  }

  Step "6. Ребёнок стёр ветку правил: сверка вернула её, журнал «правила Brave восстановлены»" {
    $before = @(Use-Parent { @(Get-Events | Where-Object { [string]$_.kind -eq $script:RestoredKind }) })
    Assert-That ($before.Count -eq 0) "До стирания в журнале уже $($before.Count) записей $($script:RestoredKind) (PL-16)"
    Remove-Item -LiteralPath $script:B -Recurse -Force
    Assert-That (-not (Test-Path -LiteralPath $script:B)) "Ветка $($script:B) не стёрта"
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $script:LastProblems = @()
    try {
      Wait-Until { $script:LastProblems = @(Get-BraveRulesProblems); $script:LastProblems.Count -eq 0 } 75 "правила Brave вернулись"
    } catch {
      throw "За 75 с служба не вернула правила Brave (сверка раз в 60 с): $($script:LastProblems -join '; '); $(Get-SiteDiag)"
    }
    Assert-Between $sw.Elapsed.TotalSeconds 0 65 "Правила Brave вернулись через, с (сверка раз в 60 с)"
    Write-Log "Ветка после сверки: $(Format-BraveRules)"
    $ev = @()
    $wait = [Diagnostics.Stopwatch]::StartNew()
    while ($wait.Elapsed.TotalSeconds -lt 5) {
      $ev = @(Use-Parent { @(Get-Events | Where-Object { [string]$_.kind -eq $script:RestoredKind }) })
      if ($ev.Count -ge 1) { break }
      Start-Sleep -Milliseconds 500
    }
    $ev | ForEach-Object { Write-Log "Журнал: $($_.kind) $($_.text)" }
    if ($ev.Count -ne 1) { throw "Записей $($script:RestoredKind): $($ev.Count), ждали 1; $(Get-SiteDiag)" }
    if ([string]$ev[0].text -notmatch 'правила Brave восстановлены$') { throw "Текст записи «$($ev[0].text)», ждали «…правила Brave восстановлены»" }
    $log = Join-Path (Join-Path $script:Data "logs") "service.log"
    $line = @(Get-Content -LiteralPath $log -Encoding UTF8 -ErrorAction SilentlyContinue | Where-Object { $_.Contains("правила Brave у Тест восстановлены") }) | Select-Object -Last 1
    if (-not $line) { throw "В service.log нет строки «правила Brave у Тест восстановлены»; $(Get-SiteDiag)" }
    Write-Log "service.log: $line"
  }

  Write-Log "S20: правила Brave в ветке ребёнка, кнопки VPN и окна Tor нет, запрещённый домен не грузится, стёртая ветка возвращена"
}
