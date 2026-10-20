# S6. Бездействие и полноэкранное видео (ТЗ 11, эталоны G2 и G3, R2а, R2б).
# Часы ядра идут в 60 раз быстрее; ускорение учтено и в датчике бездействия.
# Дано: остаток 60 мин. Когда: 20 мин без ввода в обычном окне, затем 20 мин полноэкранного окна без ввода.
# Тогда: учтено 5 + 20 = 25 мин (допуск 1 мин). Ввод имитируется движением мыши, видео окном на весь экран.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

Invoke-Scenario "S6" "Бездействие 20 мин и полноэкранное видео 20 мин: учтено 25 мин" {
  Initialize-Scenario 60

  Step "Имитация ввода доходит до сеанса" {
    Test-InputSim
  }

  $base = 0
  Step "Лимит 60 мин, ребёнок готов, часы на паузе" {
    Start-Child 60 $false
    $st = Get-Status
    $base = [int]$st.usedS
    Write-Log "Начало испытания: израсходовано $base с, остаток $($st.remainingS) с"
    Assert-That ($st.remainingS -ge 3500) "Остаток перед испытанием $($st.remainingS) с, ждали около 3600"
  }

  Step "20 мин без ввода в обычном окне: учтено 5 мин (G2)" {
    [PcbNative]::Ping()
    Write-Log "Последний ввод; передний план: $([PcbNative]::Foreground())"
    Assert-That ((Get-BigPcbWindows).Count -eq 0) "Перед испытанием на экране окно размером с экран"
    Resume-Clock
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Start-Sleep -Seconds 20
    Suspend-Clock
    $used = [int](Get-Status).usedS - $base
    Write-Log ("Прошло {0:N1} с настоящего времени (около 20 мин часов ядра); учтено {1} с, простой ввода {2} мс" -f $sw.Elapsed.TotalSeconds, $used, [PcbNative]::IdleMs())
    Assert-Between ($used / 60.0) 4 6 "Учтено за 20 мин без ввода, мин"
  }

  Step "20 мин полноэкранного окна без ввода: учтено 20 мин (G3)" {
    $afterA = [int](Get-Status).usedS
    Show-Fullscreen
    Write-Log "Полноэкранное окно на переднем плане; простой ввода $([PcbNative]::IdleMs()) мс"
    Resume-Clock
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Start-Sleep -Seconds 20
    Suspend-Clock
    $usedB = [int](Get-Status).usedS - $afterA
    Write-Log ("Прошло {0:N1} с настоящего времени; учтено {1} с" -f $sw.Elapsed.TotalSeconds, $usedB)
    Assert-Between ($usedB / 60.0) 19 21 "Учтено за 20 мин полноэкранного окна, мин"
  }

  Step "Итого учтено 25 мин (допуск 1 мин)" {
    $total = ([int](Get-Status).usedS - $base) / 60.0
    Assert-Between $total 24 26 "Всего учтено, мин"
    Save-Screen "s06-fullscreen"
  }

  Write-Log "S6: бездействие дало 5 мин, полноэкранное окно 20 мин, итого 25 мин"
}
