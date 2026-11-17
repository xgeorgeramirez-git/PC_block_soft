# S23. Смена часового пояса в детском сеансе: только запись в журнал (ТЗ 11, R17б, FR-28, D-41).
# Дано: ребёнок в сеансе, экран не заблокирован. Когда: часовой пояс компьютера сменён (Set-TimeZone).
# Тогда: экран не блокируется; запись «попытка обойти защиту. Часовой пояс изменён: был …, стал …» не позже 2 с
# (допуск раннера 3 с: опрос журнала через окно родителя). Пояс возвращается в конце сценария.
# Снимок: s23-timezone.png.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$script:TzWas = (Get-TimeZone).Id

Invoke-Scenario "S23" "Смена часового пояса у ребёнка: запись в журнал за 2 с, экран не блокируется" {
  Initialize-Scenario 1
  try {
    Step "Настройка: ребёнок «Тест», 2 ч на каждый день; значок и страж запущены" {
      Invoke-Setup 120
      Wait-SessionParts
      Start-Sleep -Seconds 3
      $g = Get-GuardState
      Assert-That (-not $g.lockShown) "Экран блокировки показан до испытания: $(Format-GuardState $g)"
      Open-Parent
      Assert-That ((Find-Events 'Часовой пояс изменён').Count -eq 0) "Запись о смене пояса есть до испытания"
    }

    Step "Смена часового пояса: запись в журнале не позже 2 с (допуск 3 с)" {
      $to = if ($script:TzWas -eq "Ekaterinburg Standard Time") { "Russian Standard Time" } else { "Ekaterinburg Standard Time" }
      $sw = [Diagnostics.Stopwatch]::StartNew()
      Set-TimeZone -Id $to
      Write-Log "Пояс: $($script:TzWas) → $to"
      $found = @()
      while ($found.Count -eq 0) {
        $found = @(Find-Events 'попытка обойти защиту\. Часовой пояс изменён')
        if ($found.Count -eq 0) {
          if ($sw.Elapsed.TotalSeconds -gt 10) { throw "За 10 с записи о смене пояса нет; журнал: $((Get-Events | ForEach-Object { $_.text }) -join ' | ')" }
          Start-Sleep -Milliseconds 100
        }
      }
      $secs = $sw.Elapsed.TotalSeconds
      Write-Log ("Запись через {0:N2} с: {1}" -f $secs, $found[0].text)
      Assert-That ($secs -le 3) ("Запись о смене пояса через {0:N2} с, ждали не больше 2 (допуск 3)" -f $secs)
      Assert-That (([string]$found[0].text).Contains("стал «")) "В записи нет нового пояса: $($found[0].text)"
    }

    Step "Экран НЕ блокируется (D-41)" {
      Start-Sleep -Seconds 2
      $g = Get-GuardState
      Assert-That (-not $g.lockShown) "После смены пояса показан экран блокировки: $(Format-GuardState $g)"
      Save-Screen "s23-timezone"
    }
  } finally {
    Set-TimeZone -Id $script:TzWas
    Write-Log "Пояс возвращён: $($script:TzWas)"
  }
  Write-Log "S23: смена пояса записана в журнал, экран не блокируется"
}
