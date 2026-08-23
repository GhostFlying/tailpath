#Requires -Version 5.1

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Binary
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepositoryRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$TestDirectory = Join-Path ([IO.Path]::GetTempPath()) ("tailpath-runner-" + [Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Path $TestDirectory | Out-Null
    Copy-Item -LiteralPath $Binary -Destination (Join-Path $TestDirectory "tailpath.exe")
    Copy-Item `
        -LiteralPath (Join-Path $RepositoryRoot "packaging/windows/run-collector.ps1") `
        -Destination (Join-Path $TestDirectory "run-collector.ps1")
    [IO.File]::WriteAllText(
        (Join-Path $TestDirectory "collector.env"),
        "TAILPATH_SERVER_URL=http://tailpath:8080`n",
        [Text.UTF8Encoding]::new($false)
    )

    $RunningOnWindows = $env:OS -eq "Windows_NT"
    if (-not $RunningOnWindows) {
        & chmod 0755 (Join-Path $TestDirectory "tailpath.exe")
    }
    $PowerShellPath = (Get-Process -Id $PID).Path
    $RunnerArgument = '"{0}"' -f (Join-Path $TestDirectory "run-collector.ps1")
    $Process = Start-Process `
        -FilePath $PowerShellPath `
        -ArgumentList @("-NoProfile", "-File", $RunnerArgument) `
        -NoNewWindow `
        -Wait `
        -PassThru
    if ($Process.ExitCode -ne 7) {
        throw "Runner exit code was $($Process.ExitCode), expected 7."
    }

    $Logs = @(Get-ChildItem -LiteralPath (Join-Path $TestDirectory "Logs") -Filter "collector.log*")
    if ($Logs.Count -lt 2 -or $Logs.Count -gt 5) {
        throw "Runner retained $($Logs.Count) logs, expected between 2 and 5."
    }
    $MaximumExpectedBytes = 5MB + 70KB
    foreach ($Log in $Logs) {
        if ($Log.Length -gt $MaximumExpectedBytes) {
            throw "$($Log.Name) is $($Log.Length) bytes, exceeding the bounded log size."
        }
    }
} finally {
    Remove-Item -LiteralPath $TestDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
