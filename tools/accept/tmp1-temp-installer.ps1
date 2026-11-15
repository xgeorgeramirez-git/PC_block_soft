# TMP1. Установщик во временной папке приложения перезаписан тем же издателем: решение родителя действует (FB-19,
# FB-22, B-67, RM3-13, PL-16).
# Папка тестовых программ (WatchDir) этого сценария лежит в профиле раннера: %TEMP%\pcb-tmp1 = <профиль>\AppData\
# Local\Temp\pcb-tmp1 — для службы это папка временных файлов приложения (Roots.tempRel), как Temp\Roblox у
# заказчика. Тестовые программы — копии системной программы (издатель Microsoft Corporation, New-TestApp).
# Дано: pcbinst.exe «Другое» шагом мастера (служба сохраняет отпечаток и издателя), подробный журнал службы.
# (а) Контроль: pcbother.exe в той же папке без решения закрыта (наблюдатель в этой папке работает).
# (б) Когда: pcbinst.exe перезаписан той же копией с новым временем изменения (новый отпечаток, тот же издатель) и
#     запущен. Тогда: не закрыт за 4 с (дольше повторного чтения пустого издателя tempPubRetry 2 с), в «Ждут
#     решения» его нет, в журнале службы строка о первом решении: «разрешена», отпечаток не совпал с сохранённым.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

# Папка временных файлов раннера (короткое имя RUNNER~1 разворачивается): WatchDir этого сценария.
$tmp = ([IO.Path]::GetTempPath()).TrimEnd('\')
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
$tmp = [PcbNative]::LongPath($tmp)
$script:Apps = Join-Path $tmp "pcb-tmp1"

# Строки подробного журнала службы о первом решении по Path (internal/service/watch.go firstDecisionNote).
function Get-FirstDecisionLines([string]$Path) {
  $log = Join-Path (Join-Path $script:Data "logs") "service.log"
  if (-not (Test-Path -LiteralPath $log)) { return @() }
  $want = "первое решение по " + $Path.ToLowerInvariant()
  return @(Get-Content -LiteralPath $log -Encoding UTF8 -ErrorAction SilentlyContinue | Where-Object { $_.ToLowerInvariant().Contains($want) })
}

# Отпечаток файла, как его видит служба (RM3-12): «размер:время изменения unix».
function Get-Fingerprint([string]$Path) {
  $f = Get-Item -LiteralPath $Path
  return "{0}:{1}" -f $f.Length, ([DateTimeOffset]$f.LastWriteTimeUtc).ToUnixTimeSeconds()
}

Invoke-Scenario "TMP1" "Установщик во временной папке перезаписан тем же издателем: не закрыт, не ждёт решения" {
  Step "WatchDir в папке временных файлов профиля" {
    Assert-That ($script:Apps -match '^[a-z]:\\users\\[^\\]+\\appdata\\local\\temp\\pcb-tmp1$') "Папка временных файлов раннера не в профиле: $($script:Apps); сценарий проверяет правило для <профиль>\AppData\Local\Temp\<папка>"
    Write-Log "WatchDir сценария: $($script:Apps)"
  }

  Initialize-Scenario 60

  Step "Тестовые программы pcbinst.exe и pcbother.exe во временной папке" {
    $script:Inst = New-TestApp "pcbinst"
    $script:Other = New-TestApp "pcbother"
  }

  Step "Настройка: лимит 2 ч; pcbinst.exe «Другое»; подробный журнал службы" {
    Start-Child 120 $false -1 @(@{ path = $script:Inst; category = "other" })
    Wait-ProgramsReady
    Use-Parent { Assert-Reply (Send-Parent "SetDiag" @{ verbose = $true }) "Подробные логи службы" }
    $norm = $script:Inst.ToLowerInvariant()
    $p = @(@(List (Get-ProgramsList).programs) | Where-Object { ([string]$_.path) -eq $norm })
    Assert-That ($p.Count -eq 1) "pcbinst.exe нет в списке программ после мастера"
    Write-Log "pcbinst.exe в списке: категория $($p[0].category), издатель «$($p[0].publisher)», отпечаток файла $(Get-Fingerprint $script:Inst)"
    Assert-That ([string]$p[0].category -eq "other") "Категория pcbinst.exe «$($p[0].category)», ждали other"
  }

  Step "(а) Контроль: pcbother.exe без решения в той же папке закрыта" {
    $p = Start-TestApp $script:Other
    [void](Assert-ClosedFast $p "pcbother.exe без решения")
  }

  Step "(б) pcbinst.exe перезаписан: новый отпечаток, тот же издатель" {
    $before = Get-Fingerprint $script:Inst
    [void](Copy-TestExe (Get-TestAppSource) $script:Apps "pcbinst")
    (Get-Item -LiteralPath $script:Inst).LastWriteTime = Get-Date
    $after = Get-Fingerprint $script:Inst
    Write-Log "Отпечаток pcbinst.exe: $before -> $after"
    Assert-That ($before -ne $after) "Отпечаток не изменился ($after): перезапись не проверяет правило FB-19"
  }

  Step "(б) Запуск перезаписанного pcbinst.exe: не закрыт, не ждёт решения" {
    $p = Start-TestApp $script:Inst
    Start-Sleep -Seconds 4
    Assert-AppAlive $p "перезаписанный pcbinst.exe того же издателя"
    $pend = @(Find-Pending (Get-ProgramsList) $script:Inst)
    Assert-That ($pend.Count -eq 0) "pcbinst.exe снова ждёт решения: $(($pend | ForEach-Object { "$($_.path) издатель «$($_.publisher)»" }) -join ' | ')"
    $lines = @(Get-FirstDecisionLines $script:Inst)
    $lines | ForEach-Object { Write-Log "Журнал службы: $_" }
    Assert-That ($lines.Count -ge 1) "В service.log нет строки «первое решение по $($script:Inst)» при подробных логах (B-67)"
    $last = [string]$lines[-1]
    Assert-That ($last.Contains("разрешена") -and $last.Contains("не совпал")) "Первое решение по перезаписанному pcbinst.exe: «$last», ждали «разрешена» и «не совпал» (новый отпечаток, решение по издателю)"
  }
}
