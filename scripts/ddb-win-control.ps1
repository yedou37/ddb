param(
    [ValidateSet("validate", "status", "start", "stop", "restart")]
    [string]$Action = "status",
    [string]$Config = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ScriptProjectRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
if ([string]::IsNullOrWhiteSpace($Config)) {
    $Config = Join-Path $ScriptProjectRoot "configs\windows\control-plane.local.json"
}

function Write-Info([string]$Message) {
    Write-Host "[INFO] $Message"
}

function Write-WarnLine([string]$Message) {
    Write-Warning $Message
}

function Fail([string]$Message) {
    throw $Message
}

function Resolve-AbsolutePath(
    [string]$Value,
    [string]$ProjectRoot,
    [string]$ConfigDir
) {
    if ([string]::IsNullOrWhiteSpace($Value)) {
        return ""
    }

    $expanded = [Environment]::ExpandEnvironmentVariables($Value)
    if ([System.IO.Path]::IsPathRooted($expanded)) {
        return [System.IO.Path]::GetFullPath($expanded)
    }

    $relativeBase = $ProjectRoot
    if ($expanded.StartsWith(".\") -or $expanded.StartsWith("./") -or $expanded.StartsWith("..\")) {
        $relativeBase = $ConfigDir
    }

    return [System.IO.Path]::GetFullPath((Join-Path $relativeBase $expanded))
}

function Ensure-Directory([string]$Path) {
    if ([string]::IsNullOrWhiteSpace($Path)) {
        return
    }
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
}

function Ensure-ParentDirectory([string]$Path) {
    if ([string]::IsNullOrWhiteSpace($Path)) {
        return
    }
    $parent = Split-Path -Parent $Path
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        Ensure-Directory $parent
    }
}

function Test-Command([string]$Name) {
    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Require-Command([string]$Name) {
    if (-not (Test-Command $Name)) {
        Fail "required command not found: $Name"
    }
}

function Wait-ForHttp([string]$Url, [int]$Attempts = 60, [int]$SleepSeconds = 1) {
    for ($i = 0; $i -lt $Attempts; $i++) {
        try {
            Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 2 | Out-Null
            return
        }
        catch {
            Start-Sleep -Seconds $SleepSeconds
        }
    }
    Fail "timeout waiting for $Url"
}

function Resolve-HealthURL(
    [string]$Value,
    [string]$Host,
    [int]$Port
) {
    if (-not [string]::IsNullOrWhiteSpace($Value)) {
        return $Value
    }
    return ("http://{0}:{1}/health" -f $Host, $Port)
}

function Load-ControlPlaneConfig([string]$ConfigPath) {
    $resolvedConfigPath = Resolve-Path $ConfigPath -ErrorAction Stop
    $configDir = Split-Path -Parent $resolvedConfigPath
    $raw = Get-Content -Path $resolvedConfigPath -Raw -Encoding UTF8
    if ([string]::IsNullOrWhiteSpace($raw)) {
        Fail "config file is empty: $resolvedConfigPath"
    }

    $cfg = $raw | ConvertFrom-Json
    $projectRootInput = [string]$cfg.project_root
    if ([string]::IsNullOrWhiteSpace($projectRootInput)) {
        Fail "config requires project_root"
    }

    $projectRoot = Resolve-AbsolutePath $projectRootInput $configDir $configDir
    $localIP = [string]$cfg.local_ip
    if ([string]::IsNullOrWhiteSpace($localIP)) {
        Fail "config requires local_ip"
    }

    $machineName = [string]$cfg.machine_name
    if ([string]::IsNullOrWhiteSpace($machineName)) {
        $machineName = "control-plane"
    }

    $dataRoot = Resolve-AbsolutePath ([string]$cfg.data_root) $projectRoot $configDir
    if ([string]::IsNullOrWhiteSpace($dataRoot)) {
        $dataRoot = Join-Path $projectRoot ".ddb-data"
    }
    $logDir = Resolve-AbsolutePath ([string]$cfg.log_dir) $projectRoot $configDir
    if ([string]::IsNullOrWhiteSpace($logDir)) {
        $logDir = Join-Path $projectRoot ".ddb-logs\control-plane"
    }
    $stateDir = Resolve-AbsolutePath ([string]$cfg.state_dir) $projectRoot $configDir
    if ([string]::IsNullOrWhiteSpace($stateDir)) {
        $stateDir = Join-Path $projectRoot ".ddb-state"
    }

    $serverBinary = Resolve-AbsolutePath ([string]$cfg.server_binary) $projectRoot $configDir
    if ([string]::IsNullOrWhiteSpace($serverBinary)) {
        $serverBinary = Join-Path $projectRoot "bin\ddb-server.exe"
    }
    $buildServerBinary = $true
    if ($null -ne $cfg.build_server_binary) {
        $buildServerBinary = [bool]$cfg.build_server_binary
    }

    $etcdCfg = $cfg.etcd
    if ($null -eq $etcdCfg) {
        Fail "config requires etcd section"
    }
    $etcdRunner = [string]$etcdCfg.runner
    if ([string]::IsNullOrWhiteSpace($etcdRunner)) {
        $etcdRunner = "docker"
    }
    if (@("docker", "native") -notcontains $etcdRunner) {
        Fail "etcd.runner must be docker or native"
    }
    $etcdPort = 2379
    if ($null -ne $etcdCfg.port -and [int]$etcdCfg.port -gt 0) {
        $etcdPort = [int]$etcdCfg.port
    }
    $etcdDataDir = Resolve-AbsolutePath ([string]$etcdCfg.data_dir) $projectRoot $configDir
    if ([string]::IsNullOrWhiteSpace($etcdDataDir)) {
        $etcdDataDir = Join-Path $projectRoot ".ddb-control\etcd"
    }
    $etcdHealthURL = Resolve-HealthURL ([string]$etcdCfg.health_url) $localIP $etcdPort
    $etcdBinary = Resolve-AbsolutePath ([string]$etcdCfg.binary_path) $projectRoot $configDir
    if ([string]::IsNullOrWhiteSpace($etcdBinary)) {
        $etcdBinary = Join-Path $projectRoot "tools\etcd-v3.5.9-windows-amd64\etcd.exe"
    }
    $etcdContainerName = [string]$etcdCfg.container_name
    if ([string]::IsNullOrWhiteSpace($etcdContainerName)) {
        $etcdContainerName = "ddb-etcd"
    }
    $etcdImage = [string]$etcdCfg.image
    if ([string]::IsNullOrWhiteSpace($etcdImage)) {
        $etcdImage = "quay.io/coreos/etcd:v3.5.9"
    }

    $apiCfg = $cfg.apiserver
    if ($null -eq $apiCfg) {
        Fail "config requires apiserver section"
    }
    $apiNodeID = [string]$apiCfg.node_id
    if ([string]::IsNullOrWhiteSpace($apiNodeID)) {
        $apiNodeID = "api-1"
    }
    $apiGroupID = [string]$apiCfg.group_id
    if ([string]::IsNullOrWhiteSpace($apiGroupID)) {
        $apiGroupID = "control"
    }
    $apiHTTPPort = 18100
    if ($null -ne $apiCfg.http_port -and [int]$apiCfg.http_port -gt 0) {
        $apiHTTPPort = [int]$apiCfg.http_port
    }
    $apiRaftPort = 30100
    if ($null -ne $apiCfg.raft_port -and [int]$apiCfg.raft_port -gt 0) {
        $apiRaftPort = [int]$apiCfg.raft_port
    }
    $apiBootstrap = $false
    if ($null -ne $apiCfg.bootstrap) {
        $apiBootstrap = [bool]$apiCfg.bootstrap
    }
    $apiRaftDir = Resolve-AbsolutePath ([string]$apiCfg.raft_dir) $projectRoot $configDir
    if ([string]::IsNullOrWhiteSpace($apiRaftDir)) {
        $apiRaftDir = Join-Path (Join-Path $dataRoot $apiNodeID) "raft"
    }
    $apiDbPath = Resolve-AbsolutePath ([string]$apiCfg.db_path) $projectRoot $configDir
    if ([string]::IsNullOrWhiteSpace($apiDbPath)) {
        $apiDbPath = Join-Path (Join-Path $dataRoot $apiNodeID) "apiserver.db"
    }
    $apiHealthURL = Resolve-HealthURL ([string]$apiCfg.health_url) $localIP $apiHTTPPort

    Ensure-Directory $logDir
    Ensure-Directory $stateDir

    return [pscustomobject]@{
        config_path         = [string]$resolvedConfigPath
        config_dir          = $configDir
        machine_name        = $machineName
        project_root        = $projectRoot
        local_ip            = $localIP
        data_root           = $dataRoot
        log_dir             = $logDir
        state_dir           = $stateDir
        state_file          = Join-Path $stateDir ($machineName + ".json")
        server_binary       = $serverBinary
        build_server_binary = $buildServerBinary
        etcd                = [pscustomobject]@{
            runner         = $etcdRunner
            port           = $etcdPort
            health_url     = $etcdHealthURL
            data_dir       = $etcdDataDir
            binary_path    = $etcdBinary
            container_name = $etcdContainerName
            image          = $etcdImage
            log_path       = Join-Path $logDir "etcd.log"
        }
        apiserver           = [pscustomobject]@{
            node_id     = $apiNodeID
            group_id    = $apiGroupID
            http_addr   = ("{0}:{1}" -f $localIP, $apiHTTPPort)
            raft_addr   = ("{0}:{1}" -f $localIP, $apiRaftPort)
            bootstrap   = $apiBootstrap
            raft_dir    = $apiRaftDir
            db_path     = $apiDbPath
            health_url  = $apiHealthURL
            etcd        = ("{0}:{1}" -f $localIP, $etcdPort)
            log_path    = Join-Path $logDir "apiserver.log"
        }
    }
}

function Load-State([string]$StateFile) {
    if (-not (Test-Path $StateFile)) {
        return @{
            etcd = $null
            apiserver = $null
        }
    }

    $raw = Get-Content -Path $StateFile -Raw -Encoding UTF8
    if ([string]::IsNullOrWhiteSpace($raw)) {
        return @{
            etcd = $null
            apiserver = $null
        }
    }

    $parsed = $raw | ConvertFrom-Json
    return @{
        etcd = $parsed.etcd
        apiserver = $parsed.apiserver
    }
}

function Save-State([string]$StateFile, [hashtable]$State) {
    $payload = [pscustomobject]@{
        etcd = $State.etcd
        apiserver = $State.apiserver
    }
    $payload | ConvertTo-Json -Depth 8 | Set-Content -Path $StateFile -Encoding UTF8
}

function Ensure-ServerBinary([object]$Context) {
    if (Test-Path $Context.server_binary) {
        return
    }
    if (-not $Context.build_server_binary) {
        Fail "ddb-server.exe not found: $($Context.server_binary)"
    }

    Require-Command go
    Write-Info "build ddb-server.exe"
    Push-Location $Context.project_root
    try {
        & go build -o ".\bin\ddb-server.exe" .\cmd\server
    }
    finally {
        Pop-Location
    }

    if (-not (Test-Path $Context.server_binary)) {
        Fail "failed to build ddb-server.exe"
    }
}

function Test-ProcessRunning([int]$Pid) {
    return $null -ne (Get-Process -Id $Pid -ErrorAction SilentlyContinue)
}

function Wait-ForProcessExit([int]$Pid, [int]$Attempts = 40, [int]$SleepMilliseconds = 250) {
    if ($Pid -le 0) {
        return
    }
    for ($i = 0; $i -lt $Attempts; $i++) {
        if (-not (Test-ProcessRunning $Pid)) {
            return
        }
        Start-Sleep -Milliseconds $SleepMilliseconds
    }
    Fail "process did not exit in time: pid=$Pid"
}

function Get-PortFromAddress([string]$Address) {
    if ([string]::IsNullOrWhiteSpace($Address)) {
        return 0
    }
    $parts = $Address.Split(":")
    if ($parts.Count -lt 2) {
        return 0
    }
    return [int]$parts[$parts.Count - 1]
}

function Get-OwningProcessesByPort([int]$Port) {
    if ($Port -le 0) {
        return @()
    }
    if (-not (Test-Command Get-NetTCPConnection)) {
        return @()
    }

    try {
        $connections = Get-NetTCPConnection -LocalPort $Port -ErrorAction SilentlyContinue
        if ($null -eq $connections) {
            return @()
        }
        $pids = @()
        foreach ($conn in @($connections)) {
            if ($null -ne $conn.OwningProcess -and [int]$conn.OwningProcess -gt 0) {
                $pids += [int]$conn.OwningProcess
            }
        }
        return @($pids | Sort-Object -Unique)
    }
    catch {
        return @()
    }
}

function Stop-ProcessesByPorts([int[]]$Ports) {
    $allPids = @()
    foreach ($port in @($Ports)) {
        $allPids += Get-OwningProcessesByPort $port
    }

    foreach ($pid in @($allPids | Sort-Object -Unique)) {
        if ($pid -le 0) {
            continue
        }
        Write-WarnLine "killing stale process pid=$pid occupying target port"
        Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
        Wait-ForProcessExit $pid
    }
}

function Wait-ForPortsFree([int[]]$Ports, [int]$Attempts = 40, [int]$SleepMilliseconds = 250) {
    $normalized = @($Ports | Where-Object { $_ -gt 0 } | Sort-Object -Unique)
    if ($normalized.Count -eq 0) {
        return
    }

    for ($i = 0; $i -lt $Attempts; $i++) {
        $busy = $false
        foreach ($port in $normalized) {
            if ((Get-OwningProcessesByPort $port).Count -gt 0) {
                $busy = $true
                break
            }
        }
        if (-not $busy) {
            return
        }
        Start-Sleep -Milliseconds $SleepMilliseconds
    }

    Fail "ports still busy: $($normalized -join ',')"
}

function Wait-ForFileReleased([string]$Path, [int]$Attempts = 40, [int]$SleepMilliseconds = 250) {
    if ([string]::IsNullOrWhiteSpace($Path) -or -not (Test-Path $Path)) {
        return
    }

    for ($i = 0; $i -lt $Attempts; $i++) {
        try {
            $stream = [System.IO.File]::Open($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
            $stream.Close()
            return
        }
        catch {
            Start-Sleep -Milliseconds $SleepMilliseconds
        }
    }

    Fail "file is still locked: $Path"
}

function Get-DockerStatus([string]$ContainerName) {
    Require-Command docker
    $status = & docker ps -a --filter "name=^/$ContainerName$" --format "{{.Status}}" 2>$null
    if ($LASTEXITCODE -ne 0) {
        return ""
    }
    return (($status | Select-Object -First 1) -as [string]).Trim()
}

function Start-Etcd([object]$Context, [hashtable]$State) {
    $cfg = $Context.etcd
    if ($cfg.runner -eq "docker") {
        Require-Command docker
        $dockerStatus = Get-DockerStatus $cfg.container_name
        if ($dockerStatus -like "Up*") {
            Write-Info "etcd already running in docker: $($cfg.container_name)"
            $State.etcd = [pscustomobject]@{
                runner = "docker"
                container = $cfg.container_name
                started_at = (Get-Date).ToString("s")
            }
            Save-State $Context.state_file $State
            return
        }

        Ensure-Directory $cfg.data_dir
        Write-Info "start etcd docker container"
        & docker rm -f $cfg.container_name 2>$null | Out-Null
        & docker run -d `
            --name $cfg.container_name `
            --restart unless-stopped `
            -p "$($cfg.port):2379" `
            -v "$($cfg.data_dir):/etcd-data" `
            $cfg.image `
            etcd `
            "--name=$($cfg.container_name)" `
            "--data-dir=/etcd-data" `
            "--advertise-client-urls=http://$($Context.local_ip):$($cfg.port)" `
            "--listen-client-urls=http://0.0.0.0:2379" | Out-Null
        if ($LASTEXITCODE -ne 0) {
            Fail "failed to start etcd docker container"
        }

        $State.etcd = [pscustomobject]@{
            runner = "docker"
            container = $cfg.container_name
            started_at = (Get-Date).ToString("s")
        }
        Save-State $Context.state_file $State
        Wait-ForHttp $cfg.health_url 60 1
        return
    }

    if ($null -ne $State.etcd -and $null -ne $State.etcd.pid) {
        $existingPid = [int]$State.etcd.pid
        if (Test-ProcessRunning $existingPid) {
            Write-Info "etcd already running: pid=$existingPid"
            return
        }
    }

    if (-not (Test-Path $cfg.binary_path)) {
        Fail "native etcd binary not found: $($cfg.binary_path)"
    }

    Ensure-Directory $cfg.data_dir
    Stop-ProcessesByPorts @($cfg.port)
    Wait-ForPortsFree @($cfg.port)

    Write-Info "start native etcd"
    $args = @(
        "--name=ddb-etcd",
        "--data-dir=$($cfg.data_dir)",
        "--advertise-client-urls=http://$($Context.local_ip):$($cfg.port)",
        "--listen-client-urls=http://0.0.0.0:$($cfg.port)"
    )
    $proc = Start-Process `
        -FilePath $cfg.binary_path `
        -WorkingDirectory (Split-Path -Parent $cfg.binary_path) `
        -ArgumentList $args `
        -RedirectStandardOutput $cfg.log_path `
        -RedirectStandardError $cfg.log_path `
        -WindowStyle Hidden `
        -PassThru

    $State.etcd = [pscustomobject]@{
        runner = "native"
        pid = $proc.Id
        log_path = $cfg.log_path
        started_at = (Get-Date).ToString("s")
    }
    Save-State $Context.state_file $State
    Wait-ForHttp $cfg.health_url 60 1
}

function Stop-Etcd([object]$Context, [hashtable]$State) {
    $cfg = $Context.etcd
    if ($cfg.runner -eq "docker") {
        if (Test-Command docker) {
            Write-Info "stop etcd docker container"
            & docker rm -f $cfg.container_name 2>$null | Out-Null
        }
        $State.etcd = $null
        Save-State $Context.state_file $State
        return
    }

    $pid = 0
    if ($null -ne $State.etcd -and $null -ne $State.etcd.pid) {
        $pid = [int]$State.etcd.pid
    }
    if ($pid -gt 0 -and (Test-ProcessRunning $pid)) {
        Write-Info "stop native etcd pid=$pid"
        Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
        Wait-ForProcessExit $pid
    }
    else {
        Stop-ProcessesByPorts @($cfg.port)
        Wait-ForPortsFree @($cfg.port)
    }

    $State.etcd = $null
    Save-State $Context.state_file $State
}

function Start-ApiServer([object]$Context, [hashtable]$State) {
    Ensure-ServerBinary $Context

    if ($null -ne $State.apiserver -and $null -ne $State.apiserver.pid) {
        $existingPid = [int]$State.apiserver.pid
        if (Test-ProcessRunning $existingPid) {
            Write-Info "apiserver already running: pid=$existingPid"
            return
        }
    }

    Ensure-Directory $Context.apiserver.raft_dir
    Ensure-ParentDirectory $Context.apiserver.db_path
    $ports = @(
        (Get-PortFromAddress $Context.apiserver.http_addr),
        (Get-PortFromAddress $Context.apiserver.raft_addr)
    )
    Stop-ProcessesByPorts $ports
    Wait-ForPortsFree $ports
    Wait-ForFileReleased $Context.apiserver.db_path

    Write-Info "start apiserver"
    $args = @(
        "--role=apiserver",
        "--node-id=$($Context.apiserver.node_id)",
        "--group-id=$($Context.apiserver.group_id)",
        "--http-addr=$($Context.apiserver.http_addr)",
        "--raft-addr=$($Context.apiserver.raft_addr)",
        "--raft-dir=$($Context.apiserver.raft_dir)",
        "--db-path=$($Context.apiserver.db_path)",
        "--bootstrap=$($Context.apiserver.bootstrap.ToString().ToLowerInvariant())",
        "--etcd=$($Context.apiserver.etcd)"
    )
    $proc = Start-Process `
        -FilePath $Context.server_binary `
        -WorkingDirectory $Context.project_root `
        -ArgumentList $args `
        -RedirectStandardOutput $Context.apiserver.log_path `
        -RedirectStandardError $Context.apiserver.log_path `
        -WindowStyle Hidden `
        -PassThru

    $State.apiserver = [pscustomobject]@{
        pid = $proc.Id
        log_path = $Context.apiserver.log_path
        started_at = (Get-Date).ToString("s")
    }
    Save-State $Context.state_file $State
    Wait-ForHttp $Context.apiserver.health_url 60 1
}

function Stop-ApiServer([object]$Context, [hashtable]$State) {
    $ports = @(
        (Get-PortFromAddress $Context.apiserver.http_addr),
        (Get-PortFromAddress $Context.apiserver.raft_addr)
    )
    $pid = 0
    if ($null -ne $State.apiserver -and $null -ne $State.apiserver.pid) {
        $pid = [int]$State.apiserver.pid
    }
    if ($pid -gt 0 -and (Test-ProcessRunning $pid)) {
        Write-Info "stop apiserver pid=$pid"
        Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue
        Wait-ForProcessExit $pid
    }
    else {
        Stop-ProcessesByPorts $ports
        Wait-ForPortsFree $ports
    }
    Wait-ForFileReleased $Context.apiserver.db_path

    $State.apiserver = $null
    Save-State $Context.state_file $State
}

function Show-Status([object]$Context, [hashtable]$State) {
    $etcdStatus = "stopped"
    $etcdDetail = ""
    if ($Context.etcd.runner -eq "docker") {
        if (Test-Command docker) {
            $dockerStatus = Get-DockerStatus $Context.etcd.container_name
            if (-not [string]::IsNullOrWhiteSpace($dockerStatus)) {
                $etcdStatus = $dockerStatus
            }
            else {
                $etcdStatus = "not_found"
            }
        }
        else {
            $etcdStatus = "docker_missing"
        }
        $etcdDetail = $Context.etcd.container_name
    }
    else {
        if ($null -ne $State.etcd -and $null -ne $State.etcd.pid -and (Test-ProcessRunning ([int]$State.etcd.pid))) {
            $etcdStatus = "running"
            $etcdDetail = "pid=$([int]$State.etcd.pid)"
        }
    }

    $apiStatus = "stopped"
    $apiDetail = ""
    if ($null -ne $State.apiserver -and $null -ne $State.apiserver.pid -and (Test-ProcessRunning ([int]$State.apiserver.pid))) {
        $apiStatus = "running"
        $apiDetail = "pid=$([int]$State.apiserver.pid)"
    }

    @(
        [pscustomobject]@{ Name = "etcd"; Runner = $Context.etcd.runner; Status = $etcdStatus; Detail = $etcdDetail; Health = $Context.etcd.health_url },
        [pscustomobject]@{ Name = "apiserver"; Runner = "ddb-process"; Status = $apiStatus; Detail = $apiDetail; Health = $Context.apiserver.health_url }
    ) | Format-Table -AutoSize
}

function Validate-Config([object]$Context) {
    Write-Host ""
    Write-Host "Config:   $($Context.config_path)"
    Write-Host "Root:     $($Context.project_root)"
    Write-Host "Local IP: $($Context.local_ip)"
    Write-Host "ETCD:     $($Context.etcd.runner) -> $($Context.etcd.health_url)"
    Write-Host "API:      $($Context.apiserver.http_addr) -> $($Context.apiserver.health_url)"
    Write-Host ""
}

$context = Load-ControlPlaneConfig $Config
$state = Load-State $context.state_file

switch ($Action) {
    "validate" {
        Validate-Config $context
        break
    }
    "status" {
        Show-Status $context $state
        break
    }
    "start" {
        Start-Etcd $context $state
        Start-ApiServer $context $state
        Show-Status $context $state
        Write-Host ""
        Write-Host "Dashboard: http://$($context.apiserver.http_addr)/dashboard/"
        break
    }
    "stop" {
        Stop-ApiServer $context $state
        Stop-Etcd $context $state
        Show-Status $context $state
        break
    }
    "restart" {
        Stop-ApiServer $context $state
        Stop-Etcd $context $state
        Start-Etcd $context $state
        Start-ApiServer $context $state
        Show-Status $context $state
        Write-Host ""
        Write-Host "Dashboard: http://$($context.apiserver.http_addr)/dashboard/"
        break
    }
    default {
        Fail "unsupported action: $Action"
    }
}
