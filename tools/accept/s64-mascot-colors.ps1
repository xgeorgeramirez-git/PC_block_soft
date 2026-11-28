# S64. Талисман окна ребёнка и цвет остатка у родителя (8.0, 8.2.6 п. 1, 8.3.2 п. 3, D-109).
# Часы ядра идут в 60 раз быстрее и стоят на паузе: остаток меняют только команды родителя.
# Дано: Миша (учётка раннера, его сеанс) и Коля (pcbtest, без сеанса), лимит дня 2 ч обоим.
# Когда: доп. время родителя (ClearExtra, затем ReduceExtra) ставит остаток 90, 30, 10, 0 мин, затем у Миши
# запретные часы (SaveSchedule пн 15:00 -> вт 07:00, часы ядра — пн 15:00).
# Тогда: по MyToday (окно ребёнка) и GetToday (окно родителя) остаток 90, 30, 10, 0 мин (минуты вверх, как в
# окне); при 0 — экран блокировки timeUp, при запретных часах — forbidden. Снимки окна «Моя статистика»
# s64-mystats-90, -30, -10, -0, -forbidden (талисман 033, 093, 031, 048, 104) и окна родителя s64-parent-90, -30,
# -10, -0 (число остатка зелёное, синее, красное, красное) сверяет с макетом визуальная приёмка.
# Окно родителя при 0 снимается, пока у Миши 10 мин, а 0 — у Коли (без сеанса): под экраном блокировки Миши окно
# родителя не войти паролем с клавиатуры. Окно ребёнка под экраном блокировки (0 и запретные часы) снимается
# PrintWindow (содержимое окна, а не экрана): экран блокировки поверх всех окон.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

if (-not ('PcbS64Print' -as [type])) {
  Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class PcbS64Print {
  // PW_RENDERFULLCONTENT (2): окно WebView2 (DirectComposition) рисуется целиком, даже под другим окном.
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
}
'@
}

# Остаток ребёнка в карточке родителя (GetToday), с.
function Get-CardRemainS([int64]$ChildId) {
  $c = @(@(List (Get-Today).children) | Where-Object { [int64]$_.childId -eq $ChildId })[0]
  Assert-That ($null -ne $c) "Нет карточки ребёнка $ChildId в GetToday"
  return [int]$c.status.remainingS
}

# Минуты остатка, как их показывают окна: вверх (15 мин 1 с — «16 мин»).
function Get-RemainMin([int]$Seconds) { return [int][Math]::Ceiling([Math]::Max(0, $Seconds) / 60.0) }

# Остаток ребёнка ChildId — Target минут: доп. время обнуляется и убавляется на столько, чтобы минуты вверх дали
# ровно Target (у Миши часть секунд уже потрачена до паузы часов).
function Set-RemainMin([int64]$ChildId, [string]$Name, [int]$Target) {
  Use-Parent {
    Assert-Reply (Send-Parent "ClearExtra" @{ childId = $ChildId }) "ClearExtra $Name"
    $r0 = Get-CardRemainS $ChildId
    $n = [int][Math]::Ceiling(($r0 - $Target * 60) / 60.0)
    if ($n -gt 0) { Assert-Reply (Send-Parent "ReduceExtra" @{ childId = $ChildId; minutes = $n }) "ReduceExtra $Name $n" }
    $r1 = Get-CardRemainS $ChildId
    Write-Log "${Name}: остаток без доп. времени $r0 с, убавлено $n мин, стало $r1 с ($(Get-RemainMin $r1) мин)"
    Assert-That ((Get-RemainMin $r1) -eq $Target) "${Name}: в окне родителя остаток $r1 с ($(Get-RemainMin $r1) мин), ждали $Target мин"
  }
}

# Окно «Моя статистика» снимком содержимого окна (PrintWindow): под экраном блокировки CopyFromScreen снял бы экран.
function Save-MyStatsPrint([string]$Name) {
  $p = Start-Process -FilePath $script:Exe -ArgumentList (ConvertTo-ArgString @("--mystats")) -PassThru
  try {
    Wait-Until { $p.Refresh(); $p.HasExited -or ($p.MainWindowHandle -ne [IntPtr]::Zero -and $p.MainWindowTitle -ne "") } 40 "окно «Моя статистика» (PID $($p.Id))"
    Assert-That (-not $p.HasExited) "Окно «Моя статистика» завершилось само, код $($p.ExitCode)"
    Start-Sleep -Seconds 5
    $p.Refresh()
    $h = $p.MainWindowHandle
    $f = [PcbNative]::WindowFrame($h)
    $w = $f[2] - $f[0]
    $hgt = $f[3] - $f[1]
    Assert-That ($w -gt 100 -and $hgt -gt 100) "Окно «Моя статистика» имеет размер ${w}x${hgt}"
    New-Item -ItemType Directory -Force -Path $script:ShotDir | Out-Null
    $bmp = New-Object System.Drawing.Bitmap $w, $hgt
    try {
      $g = [System.Drawing.Graphics]::FromImage($bmp)
      $ok = $false
      try {
        $dc = $g.GetHdc()
        try { $ok = [PcbS64Print]::PrintWindow($h, $dc, 2) } finally { $g.ReleaseHdc($dc) }
      } finally { $g.Dispose() }
      $bmp.Save((Join-Path $script:ShotDir "$Name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
    } finally { $bmp.Dispose() }
    Write-Log "Снимок окна (PrintWindow, удалось: $ok): $Name.png, заголовок «$($p.MainWindowTitle)», ${w}x${hgt}"
  } finally {
    try { if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force } } catch {}
  }
}

# Остаток и статус по MyToday (окно ребёнка).
function Assert-MyRemain([int]$Target, [string]$Status) {
  $m = Get-MyToday
  $s = $m.status
  Write-Log "MyToday: статус $($s.status), блокировка $($s.lock), остаток $($s.remainingS) с ($(Get-RemainMin ([int]$s.remainingS)) мин), лимит $($s.limitS) с"
  Assert-That ((Get-RemainMin ([int]$s.remainingS)) -eq $Target) "«Моя статистика»: остаток $($s.remainingS) с, ждали $Target мин"
  if ($Status) { Assert-That ([string]$s.status -eq $Status) "«Моя статистика»: статус «$($s.status)», ждали $Status" }
}

Invoke-Scenario "S64" "Талисман окна ребёнка и цвет остатка у родителя: 90, 30, 10, 0 мин и запретные часы" {
  Initialize-Scenario 60

  Step "Настройка: Миша (учётка раннера) и Коля (pcbtest), лимит 2 ч каждому; часы на паузе" {
    $kolyaSid = New-TestUser
    $limits = @(1..7 | ForEach-Object { New-DayLimit $_ 120 })
    $r = Send-Pcb "Setup" @{ password = $script:ParentPassword; children = @(
        @{ sid = $script:MySid; name = "Миша"; limits = @($limits) },
        @{ sid = $kolyaSid; name = "Коля"; limits = @($limits) }) }
    Assert-Reply $r "Первичная настройка на двух детей"
    Suspend-Clock
    Wait-SessionParts
    Start-Sleep -Seconds 3
    Use-Parent {
      foreach ($c in @(List (Get-Today).children)) {
        if ($c.name -eq "Миша") { $script:Misha = [int64]$c.childId }
        if ($c.name -eq "Коля") { $script:Kolya = [int64]$c.childId }
      }
    }
    Assert-That ($script:Misha -gt 0 -and $script:Kolya -gt 0 -and $script:Misha -ne $script:Kolya) "Дети не найдены: Миша $($script:Misha), Коля $($script:Kolya)"
    Write-Log "Миша — ребёнок $($script:Misha) (сеанс раннера), Коля — ребёнок $($script:Kolya) (pcbtest)"
  }

  foreach ($t in @(90, 30, 10)) {
    Step "Остаток $t мин: MyToday и GetToday — $t мин, экрана нет; снимки окна ребёнка и родителя" {
      Set-RemainMin $script:Misha "Миша" $t
      Set-RemainMin $script:Kolya "Коля" $t
      Assert-MyRemain $t ""
      $g = Get-GuardState
      Assert-That (-not $g.lockShown) "Экран блокировки при остатке $t мин: $(Format-GuardState $g)"
      [void](Save-ProgramWindow "s64-mystats-$t" @("--mystats") 40 5)
      [void](Save-ParentWindow "s64-parent-$t" "")
    }
  }

  Step "Остаток 0 у Коли (у Миши 10): снимок окна родителя — «Время вышло» красным" {
    Set-RemainMin $script:Kolya "Коля" 0
    Use-Parent {
      $c = @(@(List (Get-Today).children) | Where-Object { [int64]$_.childId -eq $script:Kolya })[0]
      Write-Log "Коля в окне родителя: статус $($c.status.status) «$($c.status.statusText)», остаток $($c.status.remainingS) с"
      Assert-That ([string]$c.status.status -eq "timeUp") "Коля: статус «$($c.status.status)», ждали timeUp"
    }
    [void](Save-ParentWindow "s64-parent-0" "")
  }

  Step "Остаток 0 у Миши: экран блокировки timeUp, MyToday 0 и timeUp; снимок окна ребёнка под экраном" {
    Set-RemainMin $script:Misha "Миша" 0
    $g = Wait-LockShown 20 "timeUp"
    Write-Log "Экран блокировки: $(Format-GuardState $g)"
    Assert-MyRemain 0 "timeUp"
    Save-MyStatsPrint "s64-mystats-0"
  }

  Step "Запретные часы у Миши (пн 15:00 -> вт 07:00): экран forbidden, MyToday forbidden; снимок окна ребёнка" {
    Use-Parent {
      Assert-Reply (Send-Parent "ClearExtra" @{ childId = $script:Misha }) "ClearExtra Миша"
      $iv = @(@{ weekday = 1; startMin = 900; endMin = 420 })
      Assert-Reply (Send-Parent "SaveSchedule" @{ childId = $script:Misha; intervals = $iv }) "SaveSchedule Миша"
    }
    $g = Wait-LockShown 20 "forbidden"
    Write-Log "Экран блокировки: $(Format-GuardState $g)"
    $m = Get-MyToday
    Write-Log "MyToday: статус $($m.status.status), блокировка $($m.status.lock), остаток $($m.status.remainingS) с, «$($m.status.until)»"
    Assert-That ([string]$m.status.status -eq "forbidden" -and [string]$m.status.lock -eq "forbidden") "«Моя статистика»: статус «$($m.status.status)», блокировка «$($m.status.lock)», ждали forbidden"
    Save-MyStatsPrint "s64-mystats-forbidden"
  }

  Write-Log "S64: остаток 90, 30, 10, 0 мин и запретные часы в окне ребёнка и у родителя; снимки s64-mystats-*, s64-parent-* — визуальной приёмке"
}
