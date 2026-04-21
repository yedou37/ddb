param(
    [ValidateSet("", "start-only", "verify-only", "seed-only", "cleanup-only")]
    [string]$Mode = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RootDir = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$Image = if ($env:DDB_IMAGE) { $env:DDB_IMAGE } else { "ghcr.io/yedou37/ddb:latest" }
$NetworkName = if ($env:DDB_DOCKER_NETWORK) { $env:DDB_DOCKER_NETWORK } else { "ddb-demo-net" }
$EtcdContainer = if ($env:DDB_ETCD_CONTAINER) { $env:DDB_ETCD_CONTAINER } else { "ddb-demo-etcd" }
$ApiUrl = if ($env:DDB_API_URL) { $env:DDB_API_URL } else { "http://127.0.0.1:18100" }
$SeedRowCount = if ($env:DDB_SEED_ROW_COUNT) { [int]$env:DDB_SEED_ROW_COUNT } else { 40 }
$TmpDir = Join-Path $env:TEMP "ddb-demo-docker-win"

function Write-Info([string]$Message) {
    Write-Host "[INFO] $Message"
}

function Fail([string]$Message) {
    throw $Message
}

function Require-Command([string]$Name) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
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

function Invoke-Sql([string]$Sql) {
    $body = @{ sql = $Sql } | ConvertTo-Json -Compress
    return Invoke-RestMethod -Method Post -ContentType "application/json" -Uri "$ApiUrl/sql" -Body $body
}

function Container-Name([string]$Name) {
    return "ddb-demo-$Name"
}

function Docker-RmIfExists([string]$Name) {
    & docker rm -f $Name 2>$null | Out-Null
}

function Docker-VolumeRmIfExists([string]$Name) {
    & docker volume rm $Name 2>$null | Out-Null
}

function Ensure-Image {
    $null = & docker image inspect $Image 2>$null
    if ($LASTEXITCODE -ne 0) {
        Write-Info "pull image $Image"
        & docker pull $Image
    }
}

function Ensure-Network {
    $null = & docker network inspect $NetworkName 2>$null
    if ($LASTEXITCODE -ne 0) {
        & docker network create $NetworkName | Out-Null
    }
}

function Cleanup-Demo {
    Write-Info "cleanup docker demo containers"
    @(
        $EtcdContainer,
        (Container-Name "api-1"),
        (Container-Name "g1-n1"), (Container-Name "g1-n2"), (Container-Name "g1-n3"),
        (Container-Name "g2-n1"), (Container-Name "g2-n2"), (Container-Name "g2-n3"),
        (Container-Name "g3-n1"), (Container-Name "g3-n2"), (Container-Name "g3-n3")
    ) | ForEach-Object { Docker-RmIfExists $_ }

    Write-Info "cleanup docker volumes"
    @(
        "ddb-demo-api-1-data",
        "ddb-demo-g1-n1-data", "ddb-demo-g1-n2-data", "ddb-demo-g1-n3-data",
        "ddb-demo-g2-n1-data", "ddb-demo-g2-n2-data", "ddb-demo-g2-n3-data",
        "ddb-demo-g3-n1-data", "ddb-demo-g3-n2-data", "ddb-demo-g3-n3-data"
    ) | ForEach-Object { Docker-VolumeRmIfExists $_ }

    & docker network rm $NetworkName 2>$null | Out-Null
    Remove-Item -Force -Recurse $TmpDir -ErrorAction SilentlyContinue
}

function Run-ServerContainer(
    [string]$Name,
    [int]$HostHttpPort,
    [int]$HostRaftPort,
    [string]$VolumeName,
    [hashtable]$EnvMap
) {
    $args = @(
        "run", "-d",
        "--name", $Name,
        "--restart", "unless-stopped",
        "--network", $NetworkName,
        "-p", "${HostHttpPort}:${HostHttpPort}",
        "-p", "${HostRaftPort}:${HostRaftPort}",
        "-v", "${VolumeName}:/data"
    )
    foreach ($key in $EnvMap.Keys) {
        $args += "-e"
        $args += "${key}=$($EnvMap[$key])"
    }
    $args += $Image
    & docker @args | Out-Null
}

function Start-Etcd {
    Write-Info "start etcd container"
    & docker run -d `
        --name $EtcdContainer `
        --restart unless-stopped `
        --network $NetworkName `
        -p 2379:2379 `
        quay.io/coreos/etcd:v3.5.9 `
        etcd `
        --advertise-client-urls=http://${EtcdContainer}:2379 `
        --listen-client-urls=http://0.0.0.0:2379 | Out-Null
    Wait-ForHttp "http://127.0.0.1:2379/health" 60 1
}

function Start-ApiServer {
    Write-Info "start apiserver container"
    Run-ServerContainer (Container-Name "api-1") 18100 30100 "ddb-demo-api-1-data" @{
        ROLE      = "apiserver"
        NODE_ID   = "api-1"
        GROUP_ID  = "control"
        HTTP_ADDR = "$(Container-Name "api-1"):18100"
        RAFT_ADDR = "$(Container-Name "api-1"):30100"
        RAFT_DIR  = "/data/raft"
        DB_PATH   = "/data/apiserver.db"
        BOOTSTRAP = "false"
        ETCD_ADDR = "${EtcdContainer}:2379"
    }
    Wait-ForHttp "$ApiUrl/health" 60 1
}

function Start-ShardGroups {
    Write-Info "start g1 containers"
    Run-ServerContainer (Container-Name "g1-n1") 21080 22080 "ddb-demo-g1-n1-data" @{
        ROLE="shard"; NODE_ID="g1-n1"; GROUP_ID="g1"; HTTP_ADDR="$(Container-Name "g1-n1"):21080"; RAFT_ADDR="$(Container-Name "g1-n1"):22080"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="true"; ETCD_ADDR="${EtcdContainer}:2379"
    }
    Run-ServerContainer (Container-Name "g1-n2") 21180 22180 "ddb-demo-g1-n2-data" @{
        ROLE="shard"; NODE_ID="g1-n2"; GROUP_ID="g1"; HTTP_ADDR="$(Container-Name "g1-n2"):21180"; RAFT_ADDR="$(Container-Name "g1-n2"):22180"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="false"; JOIN_ADDR="http://$(Container-Name "g1-n1"):21080"; ETCD_ADDR="${EtcdContainer}:2379"
    }
    Run-ServerContainer (Container-Name "g1-n3") 21280 22280 "ddb-demo-g1-n3-data" @{
        ROLE="shard"; NODE_ID="g1-n3"; GROUP_ID="g1"; HTTP_ADDR="$(Container-Name "g1-n3"):21280"; RAFT_ADDR="$(Container-Name "g1-n3"):22280"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="false"; JOIN_ADDR="http://$(Container-Name "g1-n1"):21080"; ETCD_ADDR="${EtcdContainer}:2379"
    }

    Write-Info "start g2 containers"
    Run-ServerContainer (Container-Name "g2-n1") 21081 22081 "ddb-demo-g2-n1-data" @{
        ROLE="shard"; NODE_ID="g2-n1"; GROUP_ID="g2"; HTTP_ADDR="$(Container-Name "g2-n1"):21081"; RAFT_ADDR="$(Container-Name "g2-n1"):22081"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="true"; ETCD_ADDR="${EtcdContainer}:2379"
    }
    Run-ServerContainer (Container-Name "g2-n2") 21181 22181 "ddb-demo-g2-n2-data" @{
        ROLE="shard"; NODE_ID="g2-n2"; GROUP_ID="g2"; HTTP_ADDR="$(Container-Name "g2-n2"):21181"; RAFT_ADDR="$(Container-Name "g2-n2"):22181"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="false"; JOIN_ADDR="http://$(Container-Name "g2-n1"):21081"; ETCD_ADDR="${EtcdContainer}:2379"
    }
    Run-ServerContainer (Container-Name "g2-n3") 21281 22281 "ddb-demo-g2-n3-data" @{
        ROLE="shard"; NODE_ID="g2-n3"; GROUP_ID="g2"; HTTP_ADDR="$(Container-Name "g2-n3"):21281"; RAFT_ADDR="$(Container-Name "g2-n3"):22281"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="false"; JOIN_ADDR="http://$(Container-Name "g2-n1"):21081"; ETCD_ADDR="${EtcdContainer}:2379"
    }

    Write-Info "start g3 containers"
    Run-ServerContainer (Container-Name "g3-n1") 21082 22082 "ddb-demo-g3-n1-data" @{
        ROLE="shard"; NODE_ID="g3-n1"; GROUP_ID="g3"; HTTP_ADDR="$(Container-Name "g3-n1"):21082"; RAFT_ADDR="$(Container-Name "g3-n1"):22082"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="true"; ETCD_ADDR="${EtcdContainer}:2379"
    }
    Run-ServerContainer (Container-Name "g3-n2") 21182 22182 "ddb-demo-g3-n2-data" @{
        ROLE="shard"; NODE_ID="g3-n2"; GROUP_ID="g3"; HTTP_ADDR="$(Container-Name "g3-n2"):21182"; RAFT_ADDR="$(Container-Name "g3-n2"):22182"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="false"; JOIN_ADDR="http://$(Container-Name "g3-n1"):21082"; ETCD_ADDR="${EtcdContainer}:2379"
    }
    Run-ServerContainer (Container-Name "g3-n3") 21282 22282 "ddb-demo-g3-n3-data" @{
        ROLE="shard"; NODE_ID="g3-n3"; GROUP_ID="g3"; HTTP_ADDR="$(Container-Name "g3-n3"):21282"; RAFT_ADDR="$(Container-Name "g3-n3"):22282"; RAFT_DIR="/data/raft"; DB_PATH="/data/data.db"; BOOTSTRAP="false"; JOIN_ADDR="http://$(Container-Name "g3-n1"):21082"; ETCD_ADDR="${EtcdContainer}:2379"
    }
}

function Start-Demo {
    Ensure-Image
    Ensure-Network
    Start-Etcd
    Start-ShardGroups
    Start-Sleep -Seconds 3
    Start-ApiServer
}

function Seed-Demo {
    Write-Info "wait for apiserver before seeding"
    Wait-ForHttp "$ApiUrl/health" 60 1
    try {
        Invoke-Sql "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)" | Out-Null
    }
    catch {
        Write-Host "[WARN] CREATE TABLE users may already exist; continue"
    }
    foreach ($i in 1..$SeedRowCount) {
        try {
            Invoke-Sql "DELETE FROM users WHERE id = $i" | Out-Null
        }
        catch {
        }
    }
    foreach ($i in 1..$SeedRowCount) {
        $name = "user-{0:D3}" -f $i
        $result = Invoke-Sql "INSERT INTO users VALUES ($i, '$name')"
        if (-not $result.success) {
            Fail "insert failed for id=$i"
        }
    }
}

function Verify-Demo {
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

Require-Command docker

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
        Write-Host "== docker environment started =="
        Write-Host "Dashboard: $ApiUrl/dashboard/"
        break
    }
    default {
        Cleanup-Demo
        Start-Demo
        Seed-Demo
        Verify-Demo
        Write-Host ""
        Write-Host "== docker environment started and verified =="
        Write-Host "Dashboard: $ApiUrl/dashboard/"
        break
    }
}
