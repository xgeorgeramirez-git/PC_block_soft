# S61. Копилка с телефона (FR-44, D-105, D-113, протокол связи 1.5, раздел 5в).
# Сервер связи pcblock-relay.exe на раннере (127.0.0.1:18080), тестовый телефон remote-phone.exe вместо приложения.
# Часы ядра обычные (ClockSpeed=1): копилка от времени не зависит.
# Дано: телефон привязан (как S55), в копилке Миши 45 мин (GrantBonus окна родителя).
# Когда: телефон кладёт 30 мин, затем 600 мин, затем 601 мин, затем обнуляет копилку.
# Тогда: после 30 — ответ «Положено 30 минут. В копилке 75 мин», в отчёте телефону bankMin 75, в окне родителя 75,
# в журнале «Миша: +30 мин в копилку с телефона»; после 600 — 675 (предела баланса нет, D-113); 601 — отказ range,
# номер команды не потрачен (lastSeq — номер команды «600»), баланс 675; после обнуления 0 и «Миша: копилка
# обнулена с телефона»; баланс на телефоне совпадает с окном. Журнал читается через Use-Parent (Ruling 19).
# Снимок: s61-bank.png (окно родителя, «Сегодня» с копилкой 0 после обнуления).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

function Get-Bank {
  return [int](@(List (Get-Today).children)[0].bonusBalance)
}

# Баланс копилки ребёнка в подписанном отчёте телефону не старше SinceMs.
function Get-ReportBank([int64]$SinceMs, [string]$What) {
  $st = Wait-PhoneReport $SinceMs 15 $What
  $ch = @(List $st.children) | Where-Object { [int64]$_.id -eq $script:ChildId } | Select-Object -First 1
  Assert-That ($null -ne $ch) "${What}: в отчёте нет ребёнка $($script:ChildId): $($st | ConvertTo-Json -Compress -Depth 5)"
  Write-Log "${What}: в отчёте телефону bankMin $($ch.bankMin)"
  return [int]$ch.bankMin
}

Invoke-Scenario "S61" "Копилка с телефона: +30, +600 без предела баланса, 601 — отказ, обнуление" {
  Initialize-Scenario 1
  try {
    Step "Настройка: ребёнок «Миша», лимит 2 ч; телефон привязан" {
      Invoke-Setup 120 $script:MySid "Миша"
      $script:ChildId = Connect-Phone "Pixel-S61"
    }

    Step "В копилке 45 мин (окно родителя), телефон видит 45" {
      $since = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
      Use-Parent {
        $r = Send-Parent "GrantBonus" @{ childId = $script:ChildId; minutes = 45; reason = "за уборку" }
        Assert-Reply $r "GrantBonus 45"
        Assert-That ((Get-Bank) -eq 45) "В копилке $(Get-Bank) мин, ждали 45"
      }
      $b = Get-ReportBank $since "после начисления 45 в окне"
      Assert-That ($b -eq 45) "bankMin в отчёте $b, ждали 45"
    }

    Step "Телефон кладёт 30: «Положено 30 минут. В копилке 75 мин», окно и отчёт — 75, журнал «с телефона»" {
      $since = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
      $r = Invoke-Phone @("bank", "-child", "$($script:ChildId)", "-min", "30")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Команда «+30 в копилку» не принята: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      Assert-That ([string]$r.Result.message -eq "Положено 30 минут. В копилке 75 мин") "Ответ телефону «$($r.Result.message)», ждали «Положено 30 минут. В копилке 75 мин»"
      Assert-That ([int]$r.Result.bankMin -eq 75) "bankMin в ответе $($r.Result.bankMin), ждали 75"
      Use-Parent {
        Assert-That ((Get-Bank) -eq 75) "В окне родителя копилка $(Get-Bank) мин, ждали 75"
        $ev = @(Find-Events '^Миша: \+30 мин в копилку с телефона$')
        Assert-That ($ev.Count -eq 1) "Записей «Миша: +30 мин в копилку с телефона»: $($ev.Count), ждали одну; журнал: $(Format-Events)"
      }
      $b = Get-ReportBank $since "после «+30» с телефона"
      Assert-That ($b -eq 75) "bankMin в отчёте $b, ждали 75 (как в окне)"
    }

    $script:Seq600 = 0
    Step "Телефон кладёт 600: в копилке 675 (предела баланса нет, D-113)" {
      $r = Invoke-Phone @("bank", "-child", "$($script:ChildId)", "-min", "600")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Команда «+600 в копилку» не принята: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      Assert-That ([string]$r.Result.message -eq "Положено 600 минут. В копилке 675 мин") "Ответ телефону «$($r.Result.message)», ждали «Положено 600 минут. В копилке 675 мин»"
      Assert-That ([int]$r.Result.bankMin -eq 675) "bankMin в ответе $($r.Result.bankMin), ждали 675"
      $script:Seq600 = [int64]$r.Result.seq
      Use-Parent { Assert-That ((Get-Bank) -eq 675) "В окне родителя копилка $(Get-Bank) мин, ждали 675" }
    }

    Step "Телефон кладёт 601: отказ range, номер команды не потрачен, в копилке 675, журнал без записи" {
      $r = Invoke-Phone @("bank", "-child", "$($script:ChildId)", "-min", "601")
      Assert-That ($r.Code -eq 2 -and -not $r.Result.ok -and [string]$r.Result.error -eq "range") "601: ждали отказ range, код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      Write-Log "601: seq команды $($r.Result.seq), lastSeq компьютера $($r.Result.lastSeq), номер команды «600» $($script:Seq600)"
      Assert-That ($null -ne $r.Result.lastSeq -and [int64]$r.Result.lastSeq -eq $script:Seq600) "lastSeq после 601 — $($r.Result.lastSeq), ждали $($script:Seq600) (range не тратит номер, PL-85)"
      Use-Parent {
        Assert-That ((Get-Bank) -eq 675) "После 601 копилка $(Get-Bank) мин, ждали 675"
        Assert-That ((Find-Events '601 мин в копилку').Count -eq 0) "Отказ 601 попал в журнал: $(Format-Events)"
      }
    }

    Step "Телефон обнуляет копилку: 0 в окне и в отчёте, журнал «копилка обнулена с телефона»" {
      $since = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
      $r = Invoke-Phone @("bank-clear", "-child", "$($script:ChildId)")
      Assert-That ($r.Code -eq 0 -and $r.Result.ok) "Обнуление копилки не принято: код $($r.Code), $($r.Result | ConvertTo-Json -Compress)"
      Assert-That ([string]$r.Result.message -eq "Копилка обнулена") "Ответ телефону «$($r.Result.message)», ждали «Копилка обнулена»"
      Assert-That ([int]$r.Result.bankMin -eq 0) "bankMin в ответе $($r.Result.bankMin), ждали 0"
      Use-Parent {
        Assert-That ((Get-Bank) -eq 0) "В окне родителя копилка $(Get-Bank) мин, ждали 0"
        $ev = @(Find-Events '^Миша: копилка обнулена с телефона$')
        Assert-That ($ev.Count -eq 1) "Записей «Миша: копилка обнулена с телефона»: $($ev.Count), ждали одну; журнал: $(Format-Events)"
      }
      $b = Get-ReportBank $since "после обнуления с телефона"
      Assert-That ($b -eq 0) "bankMin в отчёте $b, ждали 0 (как в окне)"
    }

    Step "Снимок окна родителя «Сегодня»" {
      [void](Save-ParentWindow "s61-bank" "")
    }
  } catch {
    Write-RelayLog
    throw
  } finally {
    Stop-Relay
  }

  Write-Log "S61: копилка с телефона +30 и +600 без предела, 601 отклонено без траты номера, обнуление; окно и телефон совпадают"
}
