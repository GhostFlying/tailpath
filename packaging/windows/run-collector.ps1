#Requires -Version 5.1

[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$MaxLogBytes = 5MB
$LogFileCount = 5
$MaxLineCharacters = 65536
$InstallDirectory = $PSScriptRoot
$BinaryPath = Join-Path $InstallDirectory "tailpath.exe"
$ConfigPath = Join-Path $InstallDirectory "collector.env"
$LogsDirectory = Join-Path $InstallDirectory "Logs"
$LogPath = Join-Path $LogsDirectory "collector.log"
$Utf8 = [Text.UTF8Encoding]::new($false)

function Rotate-TailpathLog {
    param([switch]$Force)

    if (-not (Test-Path -LiteralPath $LogPath)) {
        return
    }
    if (-not $Force -and (Get-Item -LiteralPath $LogPath).Length -lt $MaxLogBytes) {
        return
    }

    $LastBackup = "$LogPath.$($LogFileCount - 1)"
    Remove-Item -LiteralPath $LastBackup -Force -ErrorAction SilentlyContinue
    for ($Index = $LogFileCount - 2; $Index -ge 1; $Index--) {
        $Source = "$LogPath.$Index"
        if (Test-Path -LiteralPath $Source) {
            Move-Item -LiteralPath $Source -Destination "$LogPath.$($Index + 1)" -Force
        }
    }
    Move-Item -LiteralPath $LogPath -Destination "$LogPath.1" -Force
}

function Write-TailpathLog {
    param([AllowNull()][object]$InputObject)

    $Message = if ($null -eq $InputObject) { "" } else { [string]$InputObject }
    if ($Message.Length -gt $MaxLineCharacters) {
        $Message = $Message.Substring(0, $MaxLineCharacters) + " [truncated]"
    }
    $Record = "{0} {1}{2}" -f ([DateTimeOffset]::Now.ToString("o")), $Message, [Environment]::NewLine
    $RecordBytes = $Utf8.GetByteCount($Record)
    if ((Test-Path -LiteralPath $LogPath) -and
        ((Get-Item -LiteralPath $LogPath).Length + $RecordBytes -gt $MaxLogBytes)) {
        Rotate-TailpathLog -Force
    }
    [IO.File]::AppendAllText($LogPath, $Record, $Utf8)
}

New-Item -ItemType Directory -Path $LogsDirectory -Force | Out-Null
Rotate-TailpathLog

if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    Write-TailpathLog "tailpath.exe is missing"
    exit 1
}

$ServerUrl = ""
$Socket = ""
$RelayTelemetry = ""
if (Test-Path -LiteralPath $ConfigPath -PathType Leaf) {
    foreach ($Line in [IO.File]::ReadAllLines($ConfigPath)) {
        if ($Line -match '^TAILPATH_SERVER_URL=(.*)$') {
            $ServerUrl = $Matches[1]
        } elseif ($Line -match '^TAILPATH_SOCKET=(.*)$') {
            $Socket = $Matches[1]
        } elseif ($Line -match '^TAILPATH_RELAY_TELEMETRY=(.*)$') {
            $RelayTelemetry = $Matches[1]
        }
    }
}

$CollectorArguments = [System.Collections.Generic.List[string]]::new()
$CollectorArguments.Add("collector")
if ($ServerUrl) {
    $CollectorArguments.Add("--server")
    $CollectorArguments.Add($ServerUrl)
}
if ($Socket) {
    $CollectorArguments.Add("--socket")
    $CollectorArguments.Add($Socket)
}
if ($RelayTelemetry) {
    $CollectorArguments.Add("--relay-telemetry")
    $CollectorArguments.Add($RelayTelemetry)
}

try {
    & $BinaryPath @CollectorArguments 2>&1 | ForEach-Object {
        Write-TailpathLog $_
    }
    $CollectorExitCode = $LASTEXITCODE
} catch {
    Write-TailpathLog $_
    exit 1
}

if ($null -eq $CollectorExitCode) {
    $CollectorExitCode = 1
}
exit $CollectorExitCode
