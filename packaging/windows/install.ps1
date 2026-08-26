#Requires -Version 5.1
#Requires -RunAsAdministrator

[CmdletBinding()]
param(
    [ValidateNotNullOrEmpty()]
    [string]$ServerUrl = "http://tailpath:8080",
    [string]$Socket = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($ServerUrl -match "[\r\n]" -or $Socket -match "[\r\n]") {
    throw "Configuration values cannot contain newlines."
}

$TaskName = "Tailpath Collector"
$InstallDirectory = Join-Path $env:ProgramFiles "Tailpath"
$BinarySource = Join-Path $PSScriptRoot "tailpath.exe"
$RunnerSource = Join-Path $PSScriptRoot "run-collector.ps1"
$BinaryPath = Join-Path $InstallDirectory "tailpath.exe"
$RunnerPath = Join-Path $InstallDirectory "run-collector.ps1"
$ConfigPath = Join-Path $InstallDirectory "collector.env"

if (-not (Test-Path -LiteralPath $BinarySource -PathType Leaf)) {
    throw "tailpath.exe is missing beside install.ps1."
}
if (-not (Test-Path -LiteralPath $RunnerSource -PathType Leaf)) {
    throw "run-collector.ps1 is missing beside install.ps1."
}

& $BinarySource version | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "tailpath.exe failed its version check."
}

New-Item -ItemType Directory -Path $InstallDirectory -Force | Out-Null
Copy-Item -LiteralPath $BinarySource -Destination $BinaryPath -Force
Copy-Item -LiteralPath $RunnerSource -Destination $RunnerPath -Force

if (-not (Test-Path -LiteralPath $ConfigPath)) {
    $Lines = [System.Collections.Generic.List[string]]::new()
    $Lines.Add("TAILPATH_SERVER_URL=$ServerUrl")
    $Lines.Add("TAILPATH_RELAY_TELEMETRY=auto")
    if ($Socket) {
        $Lines.Add("TAILPATH_SOCKET=$Socket")
    }
    [IO.File]::WriteAllLines(
        $ConfigPath,
        $Lines,
        [Text.UTF8Encoding]::new($false)
    )
} else {
    Write-Host "Preserving existing $ConfigPath"
}

$ExistingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($null -ne $ExistingTask) {
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
}

$PowerShellPath = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"
$ActionArguments = '-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "{0}"' -f $RunnerPath
$Action = New-ScheduledTaskAction -Execute $PowerShellPath -Argument $ActionArguments
$Trigger = New-ScheduledTaskTrigger -AtStartup
$Principal = New-ScheduledTaskPrincipal `
    -UserId "SYSTEM" `
    -LogonType ServiceAccount `
    -RunLevel Highest
$Settings = New-ScheduledTaskSettingsSet `
    -MultipleInstances IgnoreNew `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -StartWhenAvailable

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $Action `
    -Trigger $Trigger `
    -Principal $Principal `
    -Settings $Settings `
    -Description "Tailpath passive Tailscale collector" `
    -Force | Out-Null
Start-ScheduledTask -TaskName $TaskName

Write-Host "Tailpath collector installed."
Write-Host "Configuration: $ConfigPath"
Write-Host "Logs: $(Join-Path $InstallDirectory 'Logs')"
Write-Warning "Windows collector support is preview and has not been verified on a real Tailnet node."
