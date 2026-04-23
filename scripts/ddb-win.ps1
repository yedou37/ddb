param(
    [ValidateSet("list", "status", "start", "stop", "restart", "start-all", "stop-all", "restart-all", "open-terminal", "start-all-terminals", "tail-log")]
    [string]$Action = "status",
    [string]$Name = "",
    [string]$Config = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
if ([string]::IsNullOrWhiteSpace($Config)) {
    $Config = Join-Path $ProjectRoot "configs\windows\local.json"
}

$Manager = Join-Path $ProjectRoot "scripts\manage-windows-cluster.ps1"

& powershell -ExecutionPolicy Bypass -File $Manager -Config $Config -Action $Action -Name $Name
