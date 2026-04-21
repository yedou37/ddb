param(
    [ValidateSet("", "start-only", "verify-only", "seed-only", "cleanup-only")]
    [string]$Mode = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RootDir = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$BinServer = Join-Path $RootDir "bin\ddb-server.exe"
$TmpDir = Join-Path $env:TEMP "ddb-demo-win"
$LogDir = Join-Path $TmpDir "logs"
$StatePath = Join-Path $TmpDir "processes.json"
$ApiUrl = "http://127.0.0.1:18100"
$EtcdEndpoint = "127.0.0.1:2379"
$EtcdContainer = "ddb-etcd-win"
$SeedRowCount = if ($env:DDB_SEED_ROW_COUNT) { [int]$env:DDB_SEED_ROW_COUNT } else { 40 }

function Write-Info([string]$Message) {
    Write-Host "[INFO] $Message"
}

function Write-Warn([string]$Message) {
    Write-Warning $Message
}

function Fail([string]$Message) {
    throw $Message
}

function Ensure-Directories {
    New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null
    New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
}

function Require-Command([string]$Name) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        Fail "required command not found: $Name"
    }
}

function Ensure-Binaries {
    if (-not (Test-Path $BinServer)) {
        Write-Info "build ddb-server.exe and ddb-cli.exe"
        Push-Location $RootDir
        try {
            & go build -o ".\bin\ddb-server.exe" .\cmd\server
            & go build -o ".\bin\ddb-cli.exe" .\cmd\cli
        }
        finally {
            Pop-Location
        }
    }
}

function Save-State([object[]]$Items) {
    Ensure-Directories
    $Items | ConvertTo-Json -Depth 4 | Set-Content -Path $StatePath -Encoding UTF8
}

function Load-State {
    if (-not (Test-Path $StatePath)) {
        return @()
    }
    $raw = Get-Content -Path $StatePath -Raw -Encoding UTF8
    if ([string]::IsNullOrWhiteSpace($raw)) {
        return @()
    }
    $parsed = $raw | ConvertFrom-Json
    if ($parsed -is [System.Array]) {
        return $parsed
    }
    return @($parsed)
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

function Invoke-Sql([string]$Sql) {
    $body = @{ sql = $Sql } | ConvertTo-Json -Compress
    return Invoke-RestMethod -Method Post -ContentType "application/json" -Uri "$ApiUrl/sql" -Body $body
}

function Start-EtcdDocker {
    Require-Command docker
    Write-Info "start etcd docker container"
    & docker rm -f $EtcdContainer 2>$null | Out-Null
    & docker run -d --name $EtcdContainer -p 2379:2379 quay.io/coreos/etcd:v3.5.9 `
        etcd --advertise-client-urls=http://127.0.0.1:2379 --listen-client-urls=http://0.0.0.0:2379 | Out-Null
    Wait-ForHttp "http://127.0.0.1:2379/health" 60 1
}

function Start-NodeProcess(
    [string]$NodeId,
    [string]$Role,
    [string]$GroupId,
    [string]$HttpAddr,
    [string]$RaftAddr,
    [string]$RaftDir,
    [string]$DbPath,
    [bool]$Bootstrap,
    [string]$JoinAddr
) {
    Ensure-Directories
    $logPath = Join-Path $LogDir "$NodeId.log"
    $args = @(
        "--role=$Role",
        "--node-id=$NodeId",
        "--group-id=$GroupId",
        "--http-addr=$HttpAddr",
        "--raft-addr=$RaftAddr",
        "--raft-dir=$RaftDir",
        "--db-path=$DbPath",
        "--bootstrap=$($Bootstrap.ToString().ToLowerInvariant())",
        "--etcd=$EtcdEndpoint"
    )
    if (-not [string]::IsNullOrWhiteSpace($JoinAddr)) {
        $args += "--join=$JoinAddr"
    }

    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $RaftDir) | Out-Null
    $proc = Start-Process -FilePath $BinServer -ArgumentList $args -RedirectStandardOutput $logPath -RedirectStandardError $logPath -PassThru
    return [pscustomobject]@{
        NodeId = $NodeId
        Role   = $Role
        Pid    = $proc.Id
        Log    = $logPath
    }
}

function Start-Demo {
    Ensure-Binaries
    Start-EtcdDocker

    $items = @()
    Write-Info "start g1/g2/g3 shard nodes"
    $items += Start-NodeProcess "g1-n1" "shard" "g1" "127.0.0.1:21080" "127.0.0.1:22080" (Join-Path $TmpDir "g1-n1\raft") (Join-Path $TmpDir "g1-n1\data.db") $true ""
    $items += Start-NodeProcess "g1-n2" "shard" "g1" "127.0.0.1:21180" "127.0.0.1:22180" (Join-Path $TmpDir "g1-n2\raft") (Join-Path $TmpDir "g1-n2\data.db") $false "http://127.0.0.1:21080"
    $items += Start-NodeProcess "g1-n3" "shard" "g1" "127.0.0.1:21280" "127.0.0.1:22280" (Join-Path $TmpDir "g1-n3\raft") (Join-Path $TmpDir "g1-n3\data.db") $false "http://127.0.0.1:21080"

    $items += Start-NodeProcess "g2-n1" "shard" "g2" "127.0.0.1:21081" "127.0.0.1:22081" (Join-Path $TmpDir "g2-n1\raft") (Join-Path $TmpDir "g2-n1\data.db") $true ""
    $items += Start-NodeProcess "g2-n2" "shard" "g2" "127.0.0.1:21181" "127.0.0.1:22181" (Join-Path $TmpDir "g2-n2\raft") (Join-Path $TmpDir "g2-n2\data.db") $false "http://127.0.0.1:21081"
    $items += Start-NodeProcess "g2-n3" "shard" "g2" "127.0.0.1:21281" "127.0.0.1:22281" (Join-Path $TmpDir "g2-n3\raft") (Join-Path $TmpDir "g2-n3\data.db") $false "http://127.0.0.1:21081"

    $items += Start-NodeProcess "g3-n1" "shard" "g3" "127.0.0.1:21082" "127.0.0.1:22082" (Join-Path $TmpDir "g3-n1\raft") (Join-Path $TmpDir "g3-n1\data.db") $true ""
    $items += Start-NodeProcess "g3-n2" "shard" "g3" "127.0.0.1:21182" "127.0.0.1:22182" (Join-Path $TmpDir "g3-n2\raft") (Join-Path $TmpDir "g3-n2\data.db") $false "http://127.0.0.1:21082"
    $items += Start-NodeProcess "g3-n3" "shard" "g3" "127.0.0.1:21282" "127.0.0.1:22282" (Join-Path $TmpDir "g3-n3\raft") (Join-Path $TmpDir "g3-n3\data.db") $false "http://127.0.0.1:21082"

    Start-Sleep -Seconds 3

    Write-Info "start apiserver"
    $items += Start-NodeProcess "api-1" "apiserver" "control" "127.0.0.1:18100" "127.0.0.1:30100" (Join-Path $TmpDir "api-1\raft") (Join-Path $TmpDir "api-1\apiserver.db") $false ""

    Save-State $items
    Wait-ForHttp "$ApiUrl/health" 60 1
}

function Seed-Demo {
    Write-Info "wait for apiserver before seeding"
    Wait-ForHttp "$ApiUrl/health" 60 1

    try {
        Invoke-Sql "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)" | Out-Null
    }
    catch {
        Write-Warn "CREATE TABLE users may already exist; continue"
    }

    Write-Info "clear users rows in range 1..$SeedRowCount"
    foreach ($i in 1..$SeedRowCount) {
        try {
            Invoke-Sql "DELETE FROM users WHERE id = $i" | Out-Null
        }
        catch {
        }
    }

    Write-Info "insert $SeedRowCount demo rows"
    foreach ($i in 1..$SeedRowCount) {
        $name = "user-{0:D3}" -f $i
        $result = Invoke-Sql "INSERT INTO users VALUES ($i, '$name')"
        if (-not $result.success) {
            Fail "insert failed for id=$i"
        }
    }
}

function Verify-Demo {
    Write-Info "verify apiserver health"
    Wait-ForHttp "$ApiUrl/health" 60 1

    $groups = Invoke-RestMethod -Method Get -Uri "$ApiUrl/groups"
    if ($groups.Count -lt 3) {
        Fail "expected at least 3 groups"
    }

    $shards = Invoke-RestMethod -Method Get -Uri "$ApiUrl/shards"
    if ($shards.assignments.Count -lt 1) {
        Fail "expected shard assignments"
    }

    $select = Invoke-Sql "SELECT * FROM users WHERE id = 1"
    if (-not $select.success -or $select.result.rows.Count -lt 1) {
        Fail "expected seeded row id=1"
    }
}

function Cleanup-Demo {
    Write-Info "cleanup windows demo processes"
    foreach ($item in Load-State) {
        try {
            Stop-Process -Id ([int]$item.Pid) -Force -ErrorAction SilentlyContinue
        }
        catch {
        }
    }

    if (Test-Command docker) {
        & docker rm -f $EtcdContainer 2>$null | Out-Null
    }

    Remove-Item -Force -Recurse $TmpDir -ErrorAction SilentlyContinue
}

switch ($Mode) {
    "cleanup-only" {
        Cleanup-Demo
        break
    }
    "verify-only" {
        Verify-Demo
        break
    }
    "seed-only" {
        Seed-Demo
        break
    }
    "start-only" {
        Cleanup-Demo
        Start-Demo
        Write-Host ""
        Write-Host "== environment started =="
        Write-Host "Dashboard: $ApiUrl/dashboard/"
        break
    }
    default {
        Cleanup-Demo
        Start-Demo
        Seed-Demo
        Verify-Demo
        Write-Host ""
        Write-Host "== environment started and verified =="
        Write-Host "Dashboard: $ApiUrl/dashboard/"
        break
    }
}
