# S1. Установка и первая настройка (ТЗ 11, план M1 Task 16).
# Дано: чистая Windows. Когда: установка и мастер (пароль, ребёнок «Тест», каждый день 2 ч).
# Тогда: служба с автозапуском работает; значок появился не позже 10 с и показывает «120».
# Заодно: память службы, стража и значка не больше 50, 40, 40 МБ; запрет Диспетчера задач у ребёнка;
# права папки данных (только система и администраторы); перезапуск службы при сбое через 5, 10, 30 с.
# Снимки: s01-wizard.png (окно мастера, если открылось), s01-tray.png, s01-minipanel.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S1" "Установка, мастер и значок «120»" {
  Reset-Machine

  Step "Установка программы (установщик, тихо)" {
    Install-Pcb
  }

  Step "Служба: автозапуск, система, перезапуск при сбое 5/10/30 с, запись безопасного режима" {
    $cim = Get-CimInstance Win32_Service -Filter "Name='$($script:ServiceName)'"
    Assert-That ($cim.StartMode -eq "Auto") "Тип запуска '$($cim.StartMode)', ждали Auto"
    Assert-That ($cim.State -eq "Running") "Состояние службы '$($cim.State)', ждали Running"
    Assert-That ($cim.StartName -eq "LocalSystem") "Служба работает от имени '$($cim.StartName)', ждали LocalSystem"
    Assert-That ($cim.PathName -match "--service") "В команде запуска нет --service: $($cim.PathName)"
    $qf = (& sc.exe qfailure $script:ServiceName) -join " "
    Write-Log "sc qfailure: $($qf -replace '\s+', ' ')"
    foreach ($ms in 5000, 10000, 30000) {
      Assert-That ($qf -match "\b$ms\b") "В настройках сбоя нет перезапуска через $ms мс"
    }
    foreach ($k in $script:SafeBootKeys) {
      Assert-That (Test-Path -Path $k) "Нет записи безопасного режима $k"
    }
  }

  Step "Права папки данных: только система и администраторы" {
    $acl = Get-Acl -LiteralPath $script:DefaultData
    Assert-That $acl.AreAccessRulesProtected "Права папки данных наследуются от родителя"
    $allowed = @("S-1-5-18", "S-1-5-32-544")
    foreach ($rule in $acl.Access) {
      $sid = $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
      Write-Log "Доступ к $($script:DefaultData): $($rule.IdentityReference) ($sid) $($rule.FileSystemRights)"
      Assert-That ($allowed -contains $sid) "У папки данных лишний доступ: $($rule.IdentityReference)"
    }
  }

  Step "Тестовый режим: временная папка данных, учётка раннера как ребёнок" {
    Enable-TestMode 1
  }

  Step "Мастер первой настройки: окно открывается (снимок s01-wizard, необязательный)" {
    try {
      $title = Save-ProgramWindow "s01-wizard" @("--wizard") 40 3
      Write-Log "Окно мастера: «$title»"
    } catch {
      Write-Log "Снимок мастера не получился (не считается сбоем): $($_.Exception.Message)"
    }
    Wait-Until { (Get-ServiceState) -eq "Running" } 20 "служба работает после закрытия окна мастера"
  }

  $sw = [Diagnostics.Stopwatch]::StartNew()
  Step "Первичная настройка, как в мастере: пароль, ребёнок «Тест», 2 ч на каждый день" {
    Invoke-Setup 120
    $sw.Restart()
  }

  Step "Значок появился не позже 10 с и показывает «120»" {
    Wait-Until { (@(Get-SessionProcs "--tray").Count -ge 1) -and ((Get-Status).label -eq "120") } 10 "значок в сеансе и надпись «120» (после настройки)"
    $secs = $sw.Elapsed.TotalSeconds
    Write-Log ("Значок и «120»: {0:N1} с после настройки" -f $secs)
    Assert-That ($secs -le 10) ("Значок «120» появился через {0:N1} с, ждали не больше 10" -f $secs)
    $st = Get-Status
    Write-Log "Статус: $($st.status), надпись «$($st.label)», подсказка «$($st.tooltip)», мини-панель «$($st.summary)»"
    Assert-That ($st.limitS -eq 7200) "Лимит дня $($st.limitS) с, ждали 7200"
    Wait-SessionParts
    Assert-That (@(Get-SessionProcs "--tray").Count -eq 1) "Значков в сеансе: $(@(Get-SessionProcs '--tray').Count), ждали один"
    Assert-That (@(Get-SessionProcs "--guard").Count -eq 1) "Стражей в сеансе: $(@(Get-SessionProcs '--guard').Count), ждали один"
  }

  Step "Память в простое: служба ≤ 50 МБ, страж ≤ 40 МБ, значок ≤ 40 МБ (личный рабочий набор)" {
    # Первичная настройка считает пароль (Argon2id, 64 МБ) в службе: после неё память возвращается системе
    # не сразу. Измеряем «в простое» (NFR-5): ждём до 90 с, пока личная память не уложится в предел.
    $limits = @{ "служба" = 50; "страж" = 40; "значок" = 40 }
    $pids = @{
      "служба" = (Get-ServicePid)
      "страж" = [int](@(Get-SessionProcs "--guard") | Select-Object -First 1).ProcessId
      "значок" = [int](@(Get-SessionProcs "--tray") | Select-Object -First 1).ProcessId
    }
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $over = @()
    while ($true) {
      $over = @()
      $line = @()
      foreach ($name in "служба", "страж", "значок") {
        $m = Get-MemoryInfo $pids[$name]
        $line += "$name рабочий набор $($m.Ws), личный $($m.Private), личные байты $($m.Bytes)"
        if ($m.Private -gt $limits[$name]) { $over += "$name $($m.Private) МБ при пределе $($limits[$name])" }
      }
      Write-Log ("Память, МБ ({0:N0} с): {1}" -f $sw.Elapsed.TotalSeconds, ($line -join "; "))
      if ($over.Count -eq 0 -or $sw.Elapsed.TotalSeconds -gt 90) { break }
      Start-Sleep -Seconds 6
    }
    Assert-That ($over.Count -eq 0) ("Память в простое выше предела через {0:N0} с: {1}" -f $sw.Elapsed.TotalSeconds, ($over -join "; "))
  }

  Step "Запрет Диспетчера задач у ребёнка (DisableTaskMgr = 1)" {
    $key = "Registry::HKEY_USERS\$($script:MySid)\Software\Microsoft\Windows\CurrentVersion\Policies\System"
    Wait-Until {
      $v = Get-ItemProperty -Path $key -Name DisableTaskMgr -ErrorAction SilentlyContinue
      $v -and $v.DisableTaskMgr -eq 1
    } 20 "DisableTaskMgr=1 в ветке ребёнка"
  }

  Step "Снимки значка у часов и мини-панели" {
    [void](Show-TrayIconOnTaskbar)
    Start-Sleep -Seconds 3
    Save-TrayCorner "s01-tray"
    if (-not (Show-TrayPanel "s01-minipanel")) {
      Write-Log "Мини-панель по значку не открылась; для разбора сохраняю весь экран (s01-minipanel-fallback)"
      Save-Screen "s01-minipanel-fallback"
    }
  }

  Write-Log "S1: служба Auto/Running, значок «120» за срок, память и права в норме"
}
