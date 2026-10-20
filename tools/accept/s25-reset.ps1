# S25. Сброс пароля родителя администратором (ТЗ 11, R20).
# Когда: в учётке администратора с повышением «Сброс пароля родителя». Тогда: новый пароль работает, старый нет;
# в обычной учётке без прав администратора сброс невозможен («Нужны права администратора»).
# Окно подтверждения Windows (UAC) в CI не показывается: ключ --reset-password --new-password= идёт мимо окна
# от повышенного администратора раннера, UAC на запасном ПК проверяется вручную.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S25" "Сброс пароля родителя администратором" {
  Initialize-Scenario 1
  $oldPw = $script:ParentPassword
  $newPw = "PcbNew-2026-x"
  $hackPw = "PcbHack-2026-x"

  Step "Настройка: пароль родителя, ребёнок «Тест»" {
    Test-Reply (Invoke-Pcb @("--test-setup", "--password=$oldPw", "--user=$env:USERNAME") "setup.json") "Первичная настройка"
    $r = Send-Pcb "Login" @{ password = $oldPw }
    Assert-That $r.ok "Прежний пароль не принят до сброса: $($r.error)"
  }

  Step "Обычный пользователь pcbtest: сброс отклонён, пароль прежний" {
    [void](New-TestUser)
    $res = Invoke-AsTestUser @("--reset-password", "--new-password=$hackPw") "reset-by-user.json"
    Assert-That ($null -ne $res) "От имени pcbtest программа не запустилась (результата нет)"
    Assert-That (-not $res.ok) "Обычный пользователь сбросил пароль родителя"
    Write-Log "Отказ для обычного пользователя: «$($res.error)»"
    Assert-That ($res.error -match "права администратора") "В отказе нет слов о правах администратора: «$($res.error)»"
    $r = Send-Pcb "Login" @{ password = $oldPw }
    Assert-That $r.ok "После отказа прежний пароль не принят: $($r.error)"
    $r = Send-Pcb "Login" @{ password = $hackPw }
    Assert-That (-not $r.ok) "Пароль обычного пользователя принят как пароль родителя"
  }

  Step "Администратор с повышением: сброс на новый пароль" {
    Test-Reply (Invoke-Pcb @("--reset-password", "--new-password=$newPw") "reset-by-admin.json") "Сброс пароля администратором"
  }

  Step "Новый пароль работает, старый нет" {
    $r = Send-Pcb "Login" @{ password = $newPw }
    Assert-That $r.ok "Новый пароль не принят: $($r.error)"
    $script:Token = [string]$r.payload.token
    $r = Send-Pcb "Login" @{ password = $oldPw }
    Write-Log "Прежний пароль после сброса: ok=$($r.ok), «$($r.error)»"
    Assert-That (-not $r.ok) "Прежний пароль всё ещё принимается после сброса"
  }

  Step "В журнале запись о сбросе пароля" {
    $found = Find-Events 'пароль родителя сброшен'
    Write-Log "Записи журнала: $((Get-Events | ForEach-Object { $_.text }) -join ' | ')"
    Assert-That ($found.Count -eq 1) "Записей о сбросе пароля: $($found.Count), ждали одну"
  }

  Write-Log "S25: администратор сбросил пароль, новый работает, старый нет; обычному пользователю отказано"
}
