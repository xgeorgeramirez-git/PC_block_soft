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
      # Сессия родителя в сеансе ребёнка — перевод родителя, не обход: проверка через Use-Parent (вход и выход).
      Use-Parent { Assert-That ((Find-Events 'Часовой пояс изменён').Count -eq 0) "Запись о смене пояса есть до испытания" }
    }

    Step "Смена часового пояса: через 2 с запись уже в журнале" {
      $to = if ($script:TzWas -eq "Ekaterinburg Standard Time") { "Russian Standard Time" } else { "Ekaterinburg Standard Time" }
      $sw = [Diagnostics.Stopwatch]::StartNew()
      Set-TimeZone -Id $to
      Write-Log "Пояс: $($script:TzWas) → $to"
      # Журнал читается ровно через 2 с: запись должна уже быть (ТЗ: не позже 2 с). Раньше читать нельзя:
      # сессия родителя в этом сеансе сделала бы смену «родительской».
      Start-Sleep -Milliseconds 2000
      $found = @(Use-Parent { @(Find-Events 'попытка обойти защиту\. Часовой пояс изменён') })
      $secs = $sw.Elapsed.TotalSeconds
      Assert-That ($found.Count -eq 1) ("Записей о смене пояса через {0:N2} с: {1}, ждали 1; журнал: {2}" -f $secs, $found.Count, (Use-Parent { (Get-Events | ForEach-Object { $_.text }) -join ' | ' }))
      Write-Log ("Запись есть через {0:N2} с: {1}" -f $secs, $found[0].text)
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
