# S56. Обрыв связи с телефоном: учёт и блокировки идут как обычно, связь возвращается сама, выключенная
# связь не держит ни одного соединения (NFR-1, NFR-16, M7).
# Часы ядра идут в 60 раз быстрее: лимит 5 мин кончается примерно через 5 с настоящего времени.
# Дано: связь включена (сервер pcblock-relay.exe на 127.0.0.1:18080). Когда: сервер остановлен на 2 мин, затем
# запущен; связь выключена. Тогда: до первого включения у процессов PC Block 0 соединений; без сервера время
# считается и экран блокировки по лимиту появляется как обычно; после запуска сервера «На связи» не позже
# 60 с; через 5 с после выключения 0 соединений (TCP без TimeWait и UDP).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

function Assert-NoConns([string]$When) {
  $c = @(Get-PcbNetConns)
  Write-Log "Соединения PC Block ($When): $(if ($c.Count) { $c -join '; ' } else { 'нет' })"
  Assert-That ($c.Count -eq 0) "$When у процессов PC Block есть соединения: $($c -join '; ')"
}

Invoke-Scenario "S56" "Обрыв связи с телефоном: учёт идёт, связь возвращается, выключено — 0 соединений" {
  Initialize-Scenario 60
  try {
    Step "Имитация ввода доходит до сеанса" {
      Test-InputSim
    }

    Step "Настройка: лимит 5 мин, часы на паузе; до первого включения связи 0 соединений" {
      Start-Child 5
      Use-Parent {
        $v = Get-Remote
        Assert-That ($v.state -eq "off" -and -not $v.enabled) "Связь включена до настройки: $(Format-Remote $v)"
      }
      Assert-NoConns "до первого включения связи"
    }

    Step "Связь включена: «На связи», соединения только с адресом из настроек" {
      Start-Relay
      Use-Parent {
        Assert-Reply (Send-Parent "SetRemote" @{ enabled = $true; url = $script:RelayUrl }) "SetRemote"
        $v = Wait-RemoteState "online" 30
        Write-Log "На связи через $([math]::Round($script:WaitElapsed, 1)) с: $(Format-Remote $v)"
      }
      $c = @(Get-PcbNetConns)
      Write-Log "Соединения при включённой связи: $($c -join '; ')"
      $foreign = @($c | Where-Object { $_ -notmatch ("-> 127\.0\.0\.1:" + $script:RelayPort + " ") })
      Assert-That ($foreign.Count -eq 0) "Соединения не с сервером из настроек: $($foreign -join '; ')"
    }

    Step "Сервер связи остановлен на 2 мин: время идёт, экран блокировки по лимиту" {
      Stop-Relay
      $script:Down = [Diagnostics.Stopwatch]::StartNew()
      Resume-Clock
      $g = Wait-LockShown 60 "timeUp"
      Write-Log "Без сервера экран блокировки показан через $([math]::Round($script:Down.Elapsed.TotalSeconds, 1)) с: $(Format-GuardState $g)"
      $st = Get-Status
      Write-Log "Статус: $($st.status), «$($st.used)», израсходовано $($st.usedS) с"
      Assert-That ($st.lock -eq "timeUp") "Блокировка '$($st.lock)', ждали timeUp"
      Assert-Between $st.usedS 295 330 "Израсходовано к блокировке без сервера, с"
      while ($script:Down.Elapsed.TotalSeconds -lt 120) {
        Start-Sleep -Seconds 10
        $st = Get-Status
        Assert-That ($st.lock -eq "timeUp") "Без сервера блокировка пропала: '$($st.lock)' на $([math]::Round($script:Down.Elapsed.TotalSeconds)) с"
        Assert-That ((Get-GuardState).lockShown) "Без сервера экран блокировки снят на $([math]::Round($script:Down.Elapsed.TotalSeconds)) с"
      }
      Write-Log "Сервер был остановлен $([math]::Round($script:Down.Elapsed.TotalSeconds)) с; служба отвечала, экран блокировки на месте"
    }

    Step "Сервер связи снова запущен: «На связи» не позже 60 с" {
      Use-Parent {
        $v = Get-Remote
        Write-Log "Перед запуском сервера: $(Format-Remote $v)"
        Assert-That ($v.state -eq "offline") "Без сервера связь в состоянии '$($v.state)', ждали offline"
      }
      Start-Relay
      $sw = [Diagnostics.Stopwatch]::StartNew()
      Use-Parent {
        $v = Wait-RemoteState "online" 60
        Write-Log ("Связь вернулась через {0:N1} с после запуска сервера: {1}" -f $sw.Elapsed.TotalSeconds, (Format-Remote $v))
        Write-Host ("::notice title=S56 NFR-16::связь вернулась через {0:N1} с после запуска сервера" -f $sw.Elapsed.TotalSeconds)
        Assert-That ($sw.Elapsed.TotalSeconds -le 60) ("Связь вернулась через {0:N1} с, ждали не больше 60" -f $sw.Elapsed.TotalSeconds)
      }
      $g = Wait-LockShown 10 "timeUp"
      Assert-That ($g.lockShown) "После возврата связи экран блокировки снят"
    }

    Step "Связь выключена: через 5 с 0 соединений" {
      Use-Parent {
        $r = Send-Parent "SetRemote" @{ enabled = $false; url = $script:RelayUrl }
        Assert-Reply $r "SetRemote выкл"
        Assert-That ($r.payload.state -eq "off") "После выключения состояние '$($r.payload.state)', ждали off"
        Assert-That ((Find-Events 'Связь с телефоном выключена').Count -eq 1) "В журнале нет «Связь с телефоном выключена»"
      }
      Start-Sleep -Seconds 5
      Assert-NoConns "через 5 с после выключения связи"
      Start-Sleep -Seconds 10
      Assert-NoConns "через 15 с после выключения связи"
    }
  } catch {
    Write-RelayLog
    throw
  } finally {
    Stop-Relay
  }

  Write-Log "S56: без сервера учёт и блокировка как обычно, связь вернулась сама, выключено — 0 соединений"
}
