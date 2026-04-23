param(
    [string]$Config = "",
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [switch]$AutoStart
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
if ([string]::IsNullOrWhiteSpace($Config)) {
    $Config = Join-Path $ProjectRoot "configs\windows\local.json"
}

$Manager = Join-Path $ProjectRoot "scripts\manage-windows-cluster.ps1"
$Host.UI.RawUI.WindowTitle = "ddb node - $Name"

function Invoke-Manager([string]$Action) {
    & powershell -ExecutionPolicy Bypass -File $Manager -Config $Config -Action $Action -Name $Name
}

function Show-Help {
    Write-Host ""
    Write-Host "Commands:"
    Write-Host "  status   - show node status"
    Write-Host "  start    - run this node in foreground in this terminal"
    Write-Host "  restart  - rerun this node in foreground in this terminal"
    Write-Host "  stop     - stop tracked process when prompt is available"
    Write-Host "  kill     - same as stop"
    Write-Host "  tail     - show recent log lines"
    Write-Host "  follow   - follow log output until Ctrl+C"
    Write-Host "  clear    - clear this terminal"
    Write-Host "  help     - show help"
    Write-Host "  exit     - close this terminal"
}

function Resolve-LogPathFromConfig {
    $cfg = (Get-Content -Path $Config -Raw -Encoding UTF8) | ConvertFrom-Json
    $projectRoot = [string]$cfg.project_root
    if ([string]::IsNullOrWhiteSpace($projectRoot)) {
        $projectRoot = $ProjectRoot
    }

    $logDir = [string]$cfg.log_dir
    if ([string]::IsNullOrWhiteSpace($logDir)) {
        $logDir = ".ddb-logs"
    }

    if ([System.IO.Path]::IsPathRooted($logDir)) {
        return (Join-Path $logDir ($Name + ".log"))
    }

    return (Join-Path (Join-Path $projectRoot $logDir) ($Name + ".log"))
}

function Follow-Log {
    $logPath = Resolve-LogPathFromConfig
    if (-not (Test-Path $logPath)) {
        Write-Warning "log not found: $logPath"
        return
    }

    Write-Host ""
    Write-Host "Following log: $logPath"
    Write-Host "Press Ctrl+C to stop following and return to command prompt."
    Get-Content -Path $logPath -Tail 30 -Wait
}

Write-Host "Node terminal for: $Name"
Write-Host "Config: $Config"
Show-Help

if ($AutoStart) {
    Write-Host ""
    Write-Host "Auto starting node in foreground..."
    Invoke-Manager "run-foreground"
}
else {
    Invoke-Manager "status"
}

while ($true) {
    Write-Host ""
    $command = (Read-Host "[$Name]").Trim().ToLowerInvariant()
    switch ($command) {
        "" { continue }
        "help" { Show-Help; continue }
        "status" { Invoke-Manager "status"; continue }
        "start" { Invoke-Manager "run-foreground"; Invoke-Manager "status"; continue }
        "stop" { Invoke-Manager "stop"; continue }
        "kill" { Invoke-Manager "stop"; continue }
        "restart" { Invoke-Manager "run-foreground"; Invoke-Manager "status"; continue }
        "tail" { Invoke-Manager "tail-log"; continue }
        "follow" { Follow-Log; continue }
        "clear" { Clear-Host; Show-Help; continue }
        "exit" { break }
        default {
            Write-Warning "unknown command: $command"
            Show-Help
            continue
        }
    }
}
