param(
    [string]$Config = ".\configs\windows\three-machine\win-a.sample.json",
    [ValidateSet("list", "status", "start", "stop", "restart", "start-all", "stop-all", "restart-all", "open-terminal", "start-all-terminals", "tail-log", "run-foreground")]
    [string]$Action = "status",
    [string]$Name = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

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
    if ([string]::IsNullOrWhiteSpace($Url)) {
        return
    }

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

function Resolve-VolumeMounts(
    [object[]]$Mounts,
    [string]$ProjectRoot,
    [string]$ConfigDir
) {
    $out = @()
    foreach ($mount in @($Mounts)) {
        if ($null -eq $mount) {
            continue
        }
        $hostPath = Resolve-AbsolutePath ([string]$mount.host_path) $ProjectRoot $ConfigDir
        $containerPath = [string]$mount.container_path
        if ([string]::IsNullOrWhiteSpace($hostPath) -or [string]::IsNullOrWhiteSpace($containerPath)) {
            Fail "volume mount requires host_path and container_path"
        }
        $out += [pscustomobject]@{
            host_path      = $hostPath
            container_path = $containerPath
        }
    }
    return $out
}

function Resolve-HostPortAddress(
    [string]$Address,
    [string]$HostAddr,
    [object]$Port,
    [string]$FieldName
) {
    if (-not [string]::IsNullOrWhiteSpace($Address)) {
        return $Address
    }
    if ([string]::IsNullOrWhiteSpace($HostAddr) -or $null -eq $Port -or [int]$Port -le 0) {
        return ""
    }
    return ("{0}:{1}" -f $HostAddr.Trim(), [int]$Port)
}

function Resolve-HealthURL(
    [string]$HealthURL,
    [string]$HTTPAddr
) {
    if (-not [string]::IsNullOrWhiteSpace($HealthURL)) {
        return $HealthURL
    }
    if ([string]::IsNullOrWhiteSpace($HTTPAddr)) {
        return ""
    }
    if ($HTTPAddr.StartsWith("http://") -or $HTTPAddr.StartsWith("https://")) {
        return ($HTTPAddr.TrimEnd("/") + "/health")
    }
    return ("http://{0}/health" -f $HTTPAddr)
}

function Resolve-Target(
    [object]$Target,
    [string]$ProjectRoot,
    [string]$ConfigDir,
    [object]$Defaults
) {
    $targetName = ""
    if ($null -ne $Target.PSObject.Properties['name']) {
        $targetName = [string]$Target.name
    }
    $localIP = [string]$Defaults.local_ip
    $dataRoot = [string]$Defaults.data_root
    $etcdHost = [string]$Defaults.etcd_host
    $etcdPort = [int]$Defaults.etcd_port
    $defaultJoinHost = [string]$Defaults.default_join_host

    $httpAddr = ""
    $httpAddrInput = ""
    if ($null -ne $Target.PSObject.Properties['http_addr']) {
        $httpAddrInput = [string]$Target.http_addr
    }
    $httpPort = $null
    if ($null -ne $Target.PSObject.Properties['http_port']) {
        $httpPort = $Target.http_port
    }
    $httpAddr = Resolve-HostPortAddress $httpAddrInput $localIP $httpPort "http_addr"

    $raftAddr = ""
    $raftAddrInput = ""
    if ($null -ne $Target.PSObject.Properties['raft_addr']) {
        $raftAddrInput = [string]$Target.raft_addr
    }
    $raftPort = $null
    if ($null -ne $Target.PSObject.Properties['raft_port']) {
        $raftPort = $Target.raft_port
    }
    $raftAddr = Resolve-HostPortAddress $raftAddrInput $localIP $raftPort "raft_addr"

    $joinHost = ""
    if ($null -ne $Target.PSObject.Properties['join_host']) {
        $joinHost = [string]$Target.join_host
    }
    if ([string]::IsNullOrWhiteSpace($joinHost)) {
        $joinHost = $defaultJoinHost
    }

    $joinAddr = ""
    $joinAddrInput = ""
    if ($null -ne $Target.PSObject.Properties['join_addr']) {
        $joinAddrInput = [string]$Target.join_addr
    }
    $joinPort = $null
    if ($null -ne $Target.PSObject.Properties['join_port']) {
        $joinPort = $Target.join_port
    }
    $joinAddr = Resolve-HostPortAddress $joinAddrInput $joinHost $joinPort "join_addr"

    $etcdAddrInput = ""
    if ($null -ne $Target.PSObject.Properties['etcd']) {
        $etcdAddrInput = [string]$Target.etcd
    }
    $etcdAddr = Resolve-HostPortAddress $etcdAddrInput $etcdHost $etcdPort "etcd"

    $raftDirInput = ""
    if ($null -ne $Target.PSObject.Properties['raft_dir']) {
        $raftDirInput = [string]$Target.raft_dir
    }
    if ([string]::IsNullOrWhiteSpace($raftDirInput) -and -not [string]::IsNullOrWhiteSpace($targetName)) {
        $raftDirInput = Join-Path (Join-Path $dataRoot $targetName) "raft"
    }

    $dbPathInput = ""
    if ($null -ne $Target.PSObject.Properties['db_path']) {
        $dbPathInput = [string]$Target.db_path
    }
    if ([string]::IsNullOrWhiteSpace($dbPathInput) -and -not [string]::IsNullOrWhiteSpace($targetName)) {
        $dbPathInput = Join-Path (Join-Path $dataRoot $targetName) "data.db"
    }

    $runner = ""
    if ($null -ne $Target.PSObject.Properties['runner']) {
        $runner = [string]$Target.runner
    }

    $healthUrl = ""
    if ($null -ne $Target.PSObject.Properties['health_url']) {
        $healthUrl = [string]$Target.health_url
    }

    $nodeId = $targetName
    if ($null -ne $Target.PSObject.Properties['node_id']) {
        $nodeIdInput = [string]$Target.node_id
        if (-not [string]::IsNullOrWhiteSpace($nodeIdInput)) {
            $nodeId = $nodeIdInput
        }
    }

    $role = ""
    if ($null -ne $Target.PSObject.Properties['role']) {
        $role = [string]$Target.role
    }

    $groupId = ""
    if ($null -ne $Target.PSObject.Properties['group_id']) {
        $groupId = [string]$Target.group_id
    }

    $bootstrap = $false
    if ($null -ne $Target.PSObject.Properties['bootstrap']) {
        $bootstrap = [bool]$Target.bootstrap
    }

    $rejoin = $false
    if ($null -ne $Target.PSObject.Properties['rejoin']) {
        $rejoin = [bool]$Target.rejoin
    }

    $containerName = ""
    if ($null -ne $Target.PSObject.Properties['container_name']) {
        $containerName = [string]$Target.container_name
    }

    $image = ""
    if ($null -ne $Target.PSObject.Properties['image']) {
        $image = [string]$Target.image
    }

    $ports = @()
    if ($null -ne $Target.PSObject.Properties['ports']) {
        $ports = @([string[]]$Target.ports)
    }

    $command = @()
    if ($null -ne $Target.PSObject.Properties['command']) {
        $command = @([string[]]$Target.command)
    }

    $volumes = @()
    if ($null -ne $Target.PSObject.Properties['volumes']) {
        $volumes = Resolve-VolumeMounts @($Target.volumes) $ProjectRoot $ConfigDir
    }

    $resolved = [pscustomobject]@{
        name           = $targetName
        runner         = $runner
        health_url     = Resolve-HealthURL $healthUrl $httpAddr
        node_id        = $nodeId
        role           = $role
        group_id       = $groupId
        http_addr      = $httpAddr
        raft_addr      = $raftAddr
        bootstrap      = $bootstrap
        rejoin         = $rejoin
        join_addr      = $joinAddr
        etcd           = $etcdAddr
        container_name = $containerName
        image          = $image
        ports          = $ports
        command        = $command
        volumes        = $volumes
        raft_dir       = Resolve-AbsolutePath $raftDirInput $ProjectRoot $ConfigDir
        db_path        = Resolve-AbsolutePath $dbPathInput $ProjectRoot $ConfigDir
    }

    if ([string]::IsNullOrWhiteSpace($resolved.name)) {
        Fail "every target requires a name"
    }
    if ([string]::IsNullOrWhiteSpace($resolved.runner)) {
        Fail "target '$($resolved.name)' requires runner"
    }
    if ($resolved.runner -eq "ddb-process") {
        if ([string]::IsNullOrWhiteSpace($resolved.http_addr)) {
            Fail "ddb-process target '$($resolved.name)' requires http_addr or local_ip + http_port"
        }
        if ([string]::IsNullOrWhiteSpace($resolved.raft_addr)) {
            Fail "ddb-process target '$($resolved.name)' requires raft_addr or local_ip + raft_port"
        }
        if ([string]::IsNullOrWhiteSpace($resolved.raft_dir)) {
            Fail "ddb-process target '$($resolved.name)' requires raft_dir or a usable data_root"
        }
        if ([string]::IsNullOrWhiteSpace($resolved.db_path)) {
            Fail "ddb-process target '$($resolved.name)' requires db_path or a usable data_root"
        }
    }

    return $resolved
}

function Load-ConfigContext([string]$ConfigPath) {
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
    
    $dataRoot = ""
    if ($null -ne $cfg.PSObject.Properties['data_root']) {
        $dataRoot = Resolve-AbsolutePath ([string]$cfg.data_root) $projectRoot $configDir
    }
    if ([string]::IsNullOrWhiteSpace($dataRoot)) {
        $dataRoot = Join-Path $projectRoot ".ddb-data"
    }
    
    $logDir = ""
    if ($null -ne $cfg.PSObject.Properties['log_dir']) {
        $logDir = Resolve-AbsolutePath ([string]$cfg.log_dir) $projectRoot $configDir
    }
    if ([string]::IsNullOrWhiteSpace($logDir)) {
        $logDir = Join-Path $projectRoot ".ddb-logs"
    }

    $stateDir = ""
    if ($null -ne $cfg.PSObject.Properties['state_dir']) {
        $stateDir = Resolve-AbsolutePath ([string]$cfg.state_dir) $projectRoot $configDir
    }
    if ([string]::IsNullOrWhiteSpace($stateDir)) {
        $stateDir = Join-Path $projectRoot ".ddb-state"
    }

    $machineName = ""
    if ($null -ne $cfg.PSObject.Properties['machine_name']) {
        $machineName = [string]$cfg.machine_name
    }
    if ([string]::IsNullOrWhiteSpace($machineName)) {
        $machineName = "default"
    }

    $localIP = ""
    if ($null -ne $cfg.PSObject.Properties['local_ip']) {
        $localIP = [string]$cfg.local_ip
    }
    
    $etcdHost = ""
    if ($null -ne $cfg.PSObject.Properties['etcd_host']) {
        $etcdHost = [string]$cfg.etcd_host
    }
    
    $etcdPort = 2379
    if ($null -ne $cfg.PSObject.Properties['etcd_port'] -and [int]$cfg.etcd_port -gt 0) {
        $etcdPort = [int]$cfg.etcd_port
    }
    
    $defaultJoinHost = ""
    if ($null -ne $cfg.PSObject.Properties['default_join_host']) {
        $defaultJoinHost = [string]$cfg.default_join_host
    }

    $serverBinary = ""
    if ($null -ne $cfg.PSObject.Properties['server_binary']) {
        $serverBinary = Resolve-AbsolutePath ([string]$cfg.server_binary) $projectRoot $configDir
    }
    if ([string]::IsNullOrWhiteSpace($serverBinary)) {
        $serverBinary = Join-Path $projectRoot "bin\ddb-server.exe"
    }

    $buildServerBinary = $true
    if ($null -ne $cfg.build_server_binary) {
        $buildServerBinary = [bool]$cfg.build_server_binary
    }

    $targets = @()
    $defaults = [pscustomobject]@{
        local_ip          = $localIP
        data_root         = $dataRoot
        etcd_host         = $etcdHost
        etcd_port         = $etcdPort
        default_join_host = $defaultJoinHost
    }
    foreach ($item in @($cfg.targets)) {
        $targets += Resolve-Target $item $projectRoot $configDir $defaults
    }
    if ($targets.Count -eq 0) {
        Fail "config requires at least one target"
    }

    Ensure-Directory $logDir
    Ensure-Directory $stateDir

    return [pscustomobject]@{
        config_path          = [string]$resolvedConfigPath
        config_dir           = $configDir
        machine_name         = $machineName
        project_root         = $projectRoot
        data_root            = $dataRoot
        local_ip             = $localIP
        etcd_host            = $etcdHost
        etcd_port            = $etcdPort
        default_join_host    = $defaultJoinHost
        log_dir              = $logDir
        state_dir            = $stateDir
        state_file           = Join-Path $stateDir ($machineName + ".json")
        server_binary        = $serverBinary
        build_server_binary  = $buildServerBinary
        targets              = $targets
    }
}

function Load-State([string]$StateFile) {
    if (-not (Test-Path $StateFile)) {
        return @{}
    }

    $raw = Get-Content -Path $StateFile -Raw -Encoding UTF8
    if ([string]::IsNullOrWhiteSpace($raw)) {
        return @{}
    }

    $parsed = $raw | ConvertFrom-Json
    $table = @{}
    foreach ($entry in @($parsed.targets)) {
        $table[[string]$entry.name] = $entry
    }
    return $table
}

function Save-State([string]$StateFile, [hashtable]$Table) {
    $items = @()
    foreach ($key in ($Table.Keys | Sort-Object)) {
        $items += $Table[$key]
    }
    $payload = [pscustomobject]@{
        targets = $items
    }
    $payload | ConvertTo-Json -Depth 8 | Set-Content -Path $StateFile -Encoding UTF8
}

function Set-StateEntry([hashtable]$Table, [string]$Name, [object]$Entry) {
    $Table[$Name] = $Entry
}

function Remove-StateEntry([hashtable]$Table, [string]$Name) {
    if ($Table.ContainsKey($Name)) {
        $Table.Remove($Name)
    }
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

function Get-TargetByName([object]$Context, [string]$Name) {
    foreach ($target in $Context.targets) {
        if ($target.name -eq $Name) {
            return $target
        }
    }
    Fail "target not found: $Name"
}

function Test-ProcessRunning([int]$ProcessId) {
    return $null -ne (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue)
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

function Wait-ForProcessExit([int]$ProcessId, [int]$Attempts = 40, [int]$SleepMilliseconds = 250) {
    if ($ProcessId -le 0) {
        return
    }

    for ($i = 0; $i -lt $Attempts; $i++) {
        if (-not (Test-ProcessRunning $ProcessId)) {
            return
        }
        Start-Sleep -Milliseconds $SleepMilliseconds
    }

    Fail "process did not exit in time: pid=$ProcessId"
}

function Stop-ProcessesByPorts([int[]]$Ports) {
    $allPids = @()
    foreach ($port in @($Ports)) {
        $allPids += Get-OwningProcessesByPort $port
    }

    foreach ($processId in @($allPids | Sort-Object -Unique)) {
        if ($processId -le 0) {
            continue
        }
        Write-WarnLine "killing stale process pid=$processId occupying target port"
        Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue
        Wait-ForProcessExit $processId
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
            $portProcesses = Get-OwningProcessesByPort $port
            if ($null -ne $portProcesses -and $portProcesses.Count -gt 0) {
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

    Fail "db file is still locked: $Path"
}

function Get-TargetPorts([object]$Target) {
    $ports = @()
    $httpPort = Get-PortFromAddress ([string]$Target.http_addr)
    $raftPort = Get-PortFromAddress ([string]$Target.raft_addr)
    if ($httpPort -gt 0) {
        $ports += $httpPort
    }
    if ($raftPort -gt 0) {
        $ports += $raftPort
    }
    return @($ports | Sort-Object -Unique)
}

function Cleanup-ProcessTargetBeforeStart([object]$Target) {
    $ports = Get-TargetPorts $Target
    Stop-ProcessesByPorts $ports
    Wait-ForPortsFree $ports
    Wait-ForFileReleased ([string]$Target.db_path)
}

function Get-DockerStatus([string]$ContainerName) {
    Require-Command docker
    $status = & docker ps -a --filter "name=^/$ContainerName$" --format "{{.Status}}" 2>$null
    if ($LASTEXITCODE -ne 0) {
        return ""
    }
    return (($status | Select-Object -First 1) -as [string]).Trim()
}

function Get-TargetStatus(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    $stateEntry = $null
    if ($State.ContainsKey($Target.name)) {
        $stateEntry = $State[$Target.name]
    }

    if ($Target.runner -eq "ddb-process") {
        if ($null -eq $stateEntry) {
            return [pscustomobject]@{
                Name    = $Target.name
                Runner  = $Target.runner
                Status  = "not_tracked"
                Detail  = ""
                LogPath = Join-Path $Context.log_dir ($Target.name + ".log")
            }
        }

        $processId = [int]$stateEntry.pid
        $running = Test-ProcessRunning $processId
        return [pscustomobject]@{
            Name    = $Target.name
            Runner  = $Target.runner
            Status  = $(if ($running) { "running" } else { "stopped" })
            Detail  = "pid=$processId"
            LogPath = [string]$stateEntry.log_path
        }
    }

    if ($Target.runner -eq "docker") {
        $containerName = $Target.container_name
        $status = Get-DockerStatus $containerName
        if ([string]::IsNullOrWhiteSpace($status)) {
            $status = "not_found"
        }
        return [pscustomobject]@{
            Name    = $Target.name
            Runner  = $Target.runner
            Status  = $status
            Detail  = $containerName
            LogPath = ""
        }
    }

    Fail "unsupported runner: $($Target.runner)"
}

function Get-TargetLogPath(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    if ($State.ContainsKey($Target.name)) {
        $logPath = [string]$State[$Target.name].log_path
        if (-not [string]::IsNullOrWhiteSpace($logPath)) {
            return $logPath
        }
    }
    return (Join-Path $Context.log_dir ($Target.name + ".log"))
}

function Show-TargetLogTail(
    [object]$Context,
    [hashtable]$State,
    [object]$Target,
    [int]$Tail = 40
) {
    $logPath = Get-TargetLogPath $Context $State $Target
    if (-not (Test-Path $logPath)) {
        Write-WarnLine "log not found: $logPath"
        return
    }

    Write-Host ""
    Write-Host "== log: $($Target.name) =="
    Get-Content -Path $logPath -Tail $Tail
}

function Open-TargetTerminal(
    [object]$Context,
    [object]$Target,
    [bool]$AutoStart
) {
    if ($Target.runner -ne "ddb-process") {
        Fail "open-terminal only supports ddb-process targets"
    }

    $terminalScript = Join-Path $Context.project_root "scripts\ddb-win-node-terminal.ps1"
    if (-not (Test-Path $terminalScript)) {
        Fail "terminal script not found: $terminalScript"
    }

    $args = @(
        "-NoExit",
        "-ExecutionPolicy", "Bypass",
        "-File", $terminalScript,
        "-Config", $Context.config_path,
        "-Name", $Target.name
    )
    if ($AutoStart) {
        $args += "-AutoStart"
    }

    Start-Process -FilePath "powershell.exe" -ArgumentList $args | Out-Null
}

function Start-DDBProcessTarget(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    Ensure-ServerBinary $Context

    if ($State.ContainsKey($Target.name)) {
        $existingPid = [int]$State[$Target.name].pid
        if (Test-ProcessRunning $existingPid) {
            Write-Info "target already running: $($Target.name) pid=$existingPid"
            return
        }
        Remove-StateEntry $State $Target.name
    }

    Ensure-Directory $Target.raft_dir
    Ensure-ParentDirectory $Target.db_path
    Cleanup-ProcessTargetBeforeStart $Target

    $logPath = Join-Path $Context.log_dir ($Target.name + ".log")
    $args = Get-ProcessStartArgs $Target

    Write-Info "start process target: $($Target.name)"
    $proc = Start-Process `
        -FilePath $Context.server_binary `
        -WorkingDirectory $Context.project_root `
        -ArgumentList $args `
        -RedirectStandardOutput $logPath `
        -RedirectStandardError ($logPath -replace '\.log$', '.err.log') `
        -WindowStyle Hidden `
        -PassThru

    $entry = [pscustomobject]@{
        name      = $Target.name
        runner    = $Target.runner
        pid       = $proc.Id
        log_path  = $logPath
        started_at = (Get-Date).ToString("s")
    }
    Set-StateEntry $State $Target.name $entry
    Save-State $Context.state_file $State
    Wait-ForHttp $Target.health_url 60 1
}

function Get-ProcessStartArgs([object]$Target) {
    $args = @(
        "--role=$($Target.role)",
        "--node-id=$($Target.node_id)",
        "--group-id=$($Target.group_id)",
        "--http-addr=$($Target.http_addr)",
        "--raft-addr=$($Target.raft_addr)",
        "--raft-dir=$($Target.raft_dir)",
        "--db-path=$($Target.db_path)",
        "--bootstrap=$(([bool]$Target.bootstrap).ToString().ToLowerInvariant())"
    )

    if (-not [string]::IsNullOrWhiteSpace($Target.etcd)) {
        $args += "--etcd=$($Target.etcd)"
    }
    if (-not [string]::IsNullOrWhiteSpace($Target.join_addr)) {
        $args += "--join=$($Target.join_addr)"
    }
    if ([bool]$Target.rejoin) {
        $args += "--rejoin=true"
    }

    return $args
}

function Run-DDBProcessForeground(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    Ensure-ServerBinary $Context
    Ensure-Directory $Target.raft_dir
    Ensure-ParentDirectory $Target.db_path
    Cleanup-ProcessTargetBeforeStart $Target

    $args = Get-ProcessStartArgs $Target
    $logPath = Join-Path $Context.log_dir ($Target.name + ".log")

    Write-Host ""
    Write-Host "== starting foreground node: $($Target.name) =="
    Write-Host "Press Ctrl+C in this window to stop the node."
    Write-Host ""

    $proc = Start-Process `
        -FilePath $Context.server_binary `
        -WorkingDirectory $Context.project_root `
        -ArgumentList $args `
        -NoNewWindow `
        -PassThru

    $entry = [pscustomobject]@{
        name       = $Target.name
        runner     = $Target.runner
        pid        = $proc.Id
        log_path   = $logPath
        started_at = (Get-Date).ToString("s")
    }
    Set-StateEntry $State $Target.name $entry
    Save-State $Context.state_file $State

    try {
        Wait-Process -Id $proc.Id
    }
    finally {
        Wait-ForPortsFree (Get-TargetPorts $Target)
        Wait-ForFileReleased ([string]$Target.db_path)
        Remove-StateEntry $State $Target.name
        Save-State $Context.state_file $State
    }
}

function Start-DockerTarget(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    Require-Command docker

    $containerName = $Target.container_name
    if ([string]::IsNullOrWhiteSpace($containerName)) {
        Fail "docker target '$($Target.name)' requires container_name"
    }
    if ([string]::IsNullOrWhiteSpace($Target.image)) {
        Fail "docker target '$($Target.name)' requires image"
    }

    foreach ($mount in $Target.volumes) {
        Ensure-Directory ([string]$mount.host_path)
    }

    Write-Info "start docker target: $($Target.name)"
    $existingContainer = & docker ps -a --filter "name=^/$containerName$" --format "{{.Names}}" 2>$null
    if (-not [string]::IsNullOrWhiteSpace($existingContainer)) {
        $null = & docker rm -f $containerName 2>$null
    }

    $args = @("run", "-d", "--name", $containerName, "--restart", "unless-stopped")
    foreach ($port in $Target.ports) {
        $args += @("-p", $port)
    }
    foreach ($mount in $Target.volumes) {
        $args += @("-v", ("{0}:{1}" -f $mount.host_path, $mount.container_path))
    }
    $args += $Target.image
    foreach ($arg in $Target.command) {
        $args += $arg
    }

    & docker @args | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Fail "docker run failed for target '$($Target.name)'"
    }

    $entry = [pscustomobject]@{
        name       = $Target.name
        runner     = $Target.runner
        container  = $containerName
        started_at = (Get-Date).ToString("s")
    }
    Set-StateEntry $State $Target.name $entry
    Save-State $Context.state_file $State
    Wait-ForHttp $Target.health_url 60 1
}

function Start-Target(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    switch ($Target.runner) {
        "ddb-process" { Start-DDBProcessTarget $Context $State $Target; break }
        "docker" { Start-DockerTarget $Context $State $Target; break }
        default { Fail "unsupported runner: $($Target.runner)" }
    }
}

function Stop-ProcessTarget(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    if (-not $State.ContainsKey($Target.name)) {
        Write-Info "target not tracked, skip stop: $($Target.name)"
        return
    }

    $processId = [int]$State[$Target.name].pid
    if (Test-ProcessRunning $processId) {
        Write-Info "stop process target: $($Target.name) pid=$processId"
        Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue
        Wait-ForProcessExit $processId
    }
    else {
        Write-Info "process already stopped: $($Target.name)"
    }

    Wait-ForPortsFree (Get-TargetPorts $Target)
    Wait-ForFileReleased ([string]$Target.db_path)

    Remove-StateEntry $State $Target.name
    Save-State $Context.state_file $State
}

function Stop-DockerTarget(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    Require-Command docker
    $containerName = $Target.container_name
    if (-not [string]::IsNullOrWhiteSpace($containerName)) {
        Write-Info "stop docker target: $($Target.name)"
        $existingContainer = & docker ps -a --filter "name=^/$containerName$" --format "{{.Names}}" 2>$null
        if (-not [string]::IsNullOrWhiteSpace($existingContainer)) {
            $null = & docker rm -f $containerName 2>$null
        }
    }

    Remove-StateEntry $State $Target.name
    Save-State $Context.state_file $State
}

function Stop-Target(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    switch ($Target.runner) {
        "ddb-process" { Stop-ProcessTarget $Context $State $Target; break }
        "docker" { Stop-DockerTarget $Context $State $Target; break }
        default { Fail "unsupported runner: $($Target.runner)" }
    }
}

function Restart-Target(
    [object]$Context,
    [hashtable]$State,
    [object]$Target
) {
    Stop-Target $Context $State $Target
    Start-Target $Context $State $Target
}

function Show-TargetList([object]$Context) {
    $rows = foreach ($target in $Context.targets) {
        [pscustomobject]@{
            Name   = $target.name
            Runner = $target.runner
            Detail = $(if ($target.runner -eq "docker") { $target.container_name } else { $target.http_addr })
        }
    }
    $rows | Format-Table -AutoSize
}

function Show-TargetStatus(
    [object]$Context,
    [hashtable]$State
) {
    $rows = foreach ($target in $Context.targets) {
        Get-TargetStatus $Context $State $Target
    }
    $rows | Format-Table -AutoSize
}

$context = Load-ConfigContext $Config
$state = Load-State $context.state_file

switch ($Action) {
    "list" {
        Show-TargetList $context
        break
    }
    "status" {
        Show-TargetStatus $context $state
        break
    }
    "start" {
        if ([string]::IsNullOrWhiteSpace($Name)) {
            Fail "-Name is required for Action=start"
        }
        Start-Target $context $state (Get-TargetByName $context $Name)
        Show-TargetStatus $context $state
        break
    }
    "stop" {
        if ([string]::IsNullOrWhiteSpace($Name)) {
            Fail "-Name is required for Action=stop"
        }
        Stop-Target $context $state (Get-TargetByName $context $Name)
        Show-TargetStatus $context $state
        break
    }
    "restart" {
        if ([string]::IsNullOrWhiteSpace($Name)) {
            Fail "-Name is required for Action=restart"
        }
        Restart-Target $context $state (Get-TargetByName $context $Name)
        Show-TargetStatus $context $state
        break
    }
    "start-all" {
        foreach ($target in $context.targets) {
            Start-Target $context $state $target
        }
        Show-TargetStatus $context $state
        break
    }
    "stop-all" {
        $reversed = @($context.targets)
        [array]::Reverse($reversed)
        foreach ($target in $reversed) {
            Stop-Target $context $state $target
        }
        Show-TargetStatus $context $state
        break
    }
    "restart-all" {
        $reversed = @($context.targets)
        [array]::Reverse($reversed)
        foreach ($target in $reversed) {
            Stop-Target $context $state $target
        }
        foreach ($target in $context.targets) {
            Start-Target $context $state $target
        }
        Show-TargetStatus $context $state
        break
    }
    "open-terminal" {
        if ([string]::IsNullOrWhiteSpace($Name)) {
            Fail "-Name is required for Action=open-terminal"
        }
        Open-TargetTerminal $context (Get-TargetByName $context $Name) $false
        break
    }
    "start-all-terminals" {
        foreach ($target in $context.targets) {
            Open-TargetTerminal $context $target $true
            Start-Sleep -Milliseconds 300
        }
        break
    }
    "tail-log" {
        if ([string]::IsNullOrWhiteSpace($Name)) {
            Fail "-Name is required for Action=tail-log"
        }
        Show-TargetLogTail $context $state (Get-TargetByName $context $Name) 60
        break
    }
    "run-foreground" {
        if ([string]::IsNullOrWhiteSpace($Name)) {
            Fail "-Name is required for Action=run-foreground"
        }
        $target = Get-TargetByName $context $Name
        if ($target.runner -ne "ddb-process") {
            Fail "run-foreground only supports ddb-process targets"
        }
        Run-DDBProcessForeground $context $state $target
        break
    }
    default {
        Fail "unsupported action: $Action"
    }
}
