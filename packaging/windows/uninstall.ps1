#Requires -Version 5.1
#Requires -RunAsAdministrator

[CmdletBinding()]
param(
    [switch]$Purge
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$TaskName = "Tailpath Collector"
$InstallDirectory = Join-Path $env:ProgramFiles "Tailpath"
$ConfigPath = Join-Path $InstallDirectory "collector.env"
$LogsDirectory = Join-Path $InstallDirectory "Logs"

$ExistingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($null -ne $ExistingTask) {
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

Remove-Item -LiteralPath (Join-Path $InstallDirectory "tailpath.exe") -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath (Join-Path $InstallDirectory "run-collector.ps1") -Force -ErrorAction SilentlyContinue

if ($Purge) {
    Remove-Item -LiteralPath $ConfigPath -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $LogsDirectory -Recurse -Force -ErrorAction SilentlyContinue
}

if (Test-Path -LiteralPath $InstallDirectory) {
    $Remaining = @(Get-ChildItem -LiteralPath $InstallDirectory -Force)
    if ($Remaining.Count -eq 0) {
        Remove-Item -LiteralPath $InstallDirectory -Force
    }
}

Write-Host "Tailpath collector uninstalled."
if (-not $Purge) {
    Write-Host "Preserved configuration: $ConfigPath"
    Write-Host "Preserved logs: $LogsDirectory"
}
