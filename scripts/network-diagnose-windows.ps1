<#
.SYNOPSIS
  Windows network diagnostics script for Windows 10/11.
.DESCRIPTION
  Collects read-only network data, saves a structured report, and prints
  auto-diagnosis findings for common connectivity issues.
.PARAMETER TargetIP
  Target IPv4 address, for example 192.168.1.20
.PARAMETER TargetHost
  Target hostname, for example example.com
.PARAMETER Ports
  Target port list, for example 22,80,443,3389
.PARAMETER OutputDir
  Output directory, defaults to ./network-diagnostics
.NOTES
  - Works in non-admin mode with graceful degradation.
  - Does not modify system network configuration.
#>

[CmdletBinding()]
param(
    [string]$TargetIP,
    [string]$TargetHost,
    [int[]]$Ports,
    [string]$OutputDir = (Join-Path (Get-Location).Path "network-diagnostics")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

$script:Findings = New-Object System.Collections.ArrayList
$script:Recommendations = New-Object System.Collections.ArrayList
$script:TargetHostResolvedIPs = @()
$script:TargetIPPingOk = $null
$script:TargetHostPingOk = $null
$script:GatewayPingOk = @{}
$script:TargetTcpOpenCount = 0
$script:TargetHostTcpOpenCount = 0
$script:RouteToTarget = $null
$script:AdminMode = $false
$script:ReportFile = $null

function Test-Command {
    param([Parameter(Mandatory = $true)][string]$Name)
    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Write-Log {
    param([string]$Text = "")
    Write-Host $Text
    Add-Content -Path $script:ReportFile -Value $Text -Encoding UTF8
}

function Write-Section {
    param([Parameter(Mandatory = $true)][string]$Title)
    Write-Log ""
    Write-Log ("=" * 78)
    Write-Log $Title
    Write-Log ("=" * 78)
}

function Write-Status {
    param(
        [Parameter(Mandatory = $true)][ValidateSet("OK","WARN","FAIL","INFO")] [string]$Level,
        [Parameter(Mandatory = $true)][string]$Message
    )
    Write-Log ("[{0}] {1}" -f $Level, $Message)
}

function Add-Finding {
    param(
        [Parameter(Mandatory = $true)][ValidateSet("OK","WARN","FAIL","INFO")] [string]$Level,
        [Parameter(Mandatory = $true)][string]$Message
    )
    [void]$script:Findings.Add([pscustomobject]@{
        Level   = $Level
        Message = $Message
    })
}

function Add-Recommendation {
    param([Parameter(Mandatory = $true)][string]$Message)
    [void]$script:Recommendations.Add($Message)
}

function Invoke-ExternalText {
    param(
        [Parameter(Mandatory = $true)][string]$Command,
        [string[]]$Arguments = @()
    )

    if (-not (Test-Command $Command)) {
        return "command not found: $Command"
    }

    try {
        return (& $Command @Arguments 2>&1 | Out-String).TrimEnd()
    }
    catch {
        return "command failed: $Command`n$($_.Exception.Message)"
    }
}

function Test-Administrator {
    try {
        $current = [Security.Principal.WindowsIdentity]::GetCurrent()
        $principal = New-Object Security.Principal.WindowsPrincipal($current)
        return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    }
    catch {
        return $false
    }
}

function Convert-IPv4ToUInt32 {
    param([Parameter(Mandatory = $true)][string]$IpAddress)
    $parsed = [System.Net.IPAddress]::Parse($IpAddress)
    $bytes = $parsed.GetAddressBytes()
    [Array]::Reverse($bytes)
    return [BitConverter]::ToUInt32($bytes, 0)
}

function Test-SameSubnet {
    param(
        [Parameter(Mandatory = $true)][string]$LocalIP,
        [Parameter(Mandatory = $true)][int]$PrefixLength,
        [Parameter(Mandatory = $true)][string]$RemoteIP
    )

    if ($PrefixLength -lt 0 -or $PrefixLength -gt 32) {
        return $false
    }

    $mask = if ($PrefixLength -eq 0) { [uint32]0 } else { ([uint32]::MaxValue) -shl (32 - $PrefixLength) }
    $localInt = Convert-IPv4ToUInt32 -IpAddress $LocalIP
    $remoteInt = Convert-IPv4ToUInt32 -IpAddress $RemoteIP
    return (($localInt -band $mask) -eq ($remoteInt -band $mask))
}

function Get-VirtualAdapterHint {
    param(
        [string]$Name,
        [string]$Description
    )
    $joined = ("{0} {1}" -f $Name, $Description).ToLowerInvariant()
    return ($joined -match 'vmware|virtualbox|hyper-v|vethernet|docker|wsl|vpn|wireguard|zerotier|tailscale|parallels|tap|tun|loopback|virtual')
}

function Resolve-TargetHostIPs {
    param([Parameter(Mandatory = $true)][string]$HostName)

    $ips = @()

    if (Test-Command "Resolve-DnsName") {
        try {
            $answers = Resolve-DnsName -Name $HostName -ErrorAction Stop |
                Where-Object { $_.Type -in @("A", "AAAA") } |
                Select-Object -ExpandProperty IPAddress -ErrorAction SilentlyContinue
            if ($answers) {
                $ips += $answers
            }
        }
        catch {
        }
    }

    if (-not $ips) {
        try {
            $ips += [System.Net.Dns]::GetHostAddresses($HostName) |
                ForEach-Object { $_.IPAddressToString }
        }
        catch {
        }
    }

    return $ips | Sort-Object -Unique
}

function Test-Icmp {
    param(
        [Parameter(Mandatory = $true)][string]$Address,
        [int]$Count = 2
    )

    try {
        $ok = Test-Connection -ComputerName $Address -Count $Count -Quiet -ErrorAction Stop
        return [pscustomobject]@{
            Address = $Address
            Success = [bool]$ok
        }
    }
    catch {
        return [pscustomobject]@{
            Address = $Address
            Success = $false
        }
    }
}

function Test-TcpPort {
    param(
        [Parameter(Mandatory = $true)][string]$ComputerName,
        [Parameter(Mandatory = $true)][int]$Port
    )

    try {
        if (Test-Command "Test-NetConnection") {
            $r = Test-NetConnection -ComputerName $ComputerName -Port $Port -WarningAction SilentlyContinue
            return [pscustomobject]@{
                ComputerName   = $ComputerName
                Port           = $Port
                Success        = [bool]$r.TcpTestSucceeded
                RemoteAddress  = $r.RemoteAddress
                InterfaceAlias = $r.InterfaceAlias
                SourceAddress  = $r.SourceAddress
            }
        }

        $client = New-Object System.Net.Sockets.TcpClient
        $iar = $client.BeginConnect($ComputerName, $Port, $null, $null)
        $ok = $iar.AsyncWaitHandle.WaitOne(2000, $false)
        if (-not $ok) {
            $client.Close()
            return [pscustomobject]@{
                ComputerName   = $ComputerName
                Port           = $Port
                Success        = $false
                RemoteAddress  = $null
                InterfaceAlias = $null
                SourceAddress  = $null
            }
        }

        $client.EndConnect($iar)
        $client.Close()
        return [pscustomobject]@{
            ComputerName   = $ComputerName
            Port           = $Port
            Success        = $true
            RemoteAddress  = $null
            InterfaceAlias = $null
            SourceAddress  = $null
        }
    }
    catch {
        return [pscustomobject]@{
            ComputerName   = $ComputerName
            Port           = $Port
            Success        = $false
            RemoteAddress  = $null
            InterfaceAlias = $null
            SourceAddress  = $null
        }
    }
}

function Get-RouteToTarget {
    param([string]$Address)

    if (-not $Address) {
        return $null
    }

    try {
        if (Test-Command "Test-NetConnection") {
            $tnc = Test-NetConnection -ComputerName $Address -InformationLevel Detailed -WarningAction SilentlyContinue
            if ($tnc) {
                return [pscustomobject]@{
                    SourceAddress  = $tnc.SourceAddress
                    InterfaceAlias = $tnc.InterfaceAlias
                    NetRoute       = $tnc.NetRoute
                }
            }
        }
    }
    catch {
    }

    return $null
}

try {
    if ($TargetIP) {
        $parsedIp = $null
        if (-not [System.Net.IPAddress]::TryParse($TargetIP, [ref]$parsedIp) -or
            $parsedIp.AddressFamily -ne [System.Net.Sockets.AddressFamily]::InterNetwork) {
            throw "TargetIP must be a valid IPv4 address"
        }
    }

    if ($Ports) {
        foreach ($p in $Ports) {
            if ($p -lt 1 -or $p -gt 65535) {
                throw "Ports contains an invalid port: $p"
            }
        }
    }

    if ((-not $Ports) -and ($TargetIP -or $TargetHost)) {
        $Ports = @(22,80,443,3389)
    }

    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $script:ReportFile = Join-Path $OutputDir ("network-diagnose-windows-{0}.txt" -f $timestamp)
    New-Item -ItemType File -Path $script:ReportFile -Force | Out-Null
    $script:AdminMode = Test-Administrator

    $os = Get-CimInstance Win32_OperatingSystem -ErrorAction SilentlyContinue
    $computerSystem = Get-CimInstance Win32_ComputerSystem -ErrorAction SilentlyContinue
    $adapters = @(Get-NetAdapter -ErrorAction SilentlyContinue | Sort-Object ifIndex)
    $ipConfigs = @(Get-NetIPConfiguration -All -ErrorAction SilentlyContinue)
    $dnsAddrs = @(Get-DnsClientServerAddress -AddressFamily IPv4,IPv6 -ErrorAction SilentlyContinue)
    $defaultRoutes = @(Get-NetRoute -AddressFamily IPv4 -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue | Sort-Object RouteMetric,InterfaceMetric)
    $neighborTable = @()
    $activeIPv4Configs = @()
    $activeVirtualAdapters = @()

    $cfgByAlias = @{}
    foreach ($cfg in $ipConfigs) {
        $cfgByAlias[$cfg.InterfaceAlias] = $cfg
    }

    Write-Section "1. Basic System Information"
    Write-Log ("Timestamp           : {0}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss zzz"))
    Write-Log ("Hostname            : {0}" -f $env:COMPUTERNAME)
    Write-Log ("Current User        : {0}" -f ([Environment]::UserDomainName + "\" + [Environment]::UserName))
    Write-Log ("Administrator Mode  : {0}" -f ($(if ($script:AdminMode) { "Yes" } else { "No" })))
    if ($os) {
        Write-Log ("OS                  : {0}" -f $os.Caption)
        Write-Log ("OS Version          : {0}" -f $os.Version)
        Write-Log ("Build Number        : {0}" -f $os.BuildNumber)
    }
    if ($computerSystem) {
        Write-Log ("Model               : {0}" -f $computerSystem.Model)
        Write-Log ("Domain/Workgroup    : {0}" -f $computerSystem.Domain)
    }

    Write-Section "2. Adapters and IP Information"
    if (-not $adapters) {
        Write-Status FAIL "No network adapter information was collected"
        Add-Finding FAIL "No network adapter information was collected"
    } else {
        foreach ($adapter in $adapters) {
            $cfg = $cfgByAlias[$adapter.Name]
            $dns = @($dnsAddrs | Where-Object { $_.InterfaceIndex -eq $adapter.ifIndex })
            $ipIf = Get-NetIPInterface -InterfaceIndex $adapter.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue
            $isVirtual = Get-VirtualAdapterHint -Name $adapter.Name -Description $adapter.InterfaceDescription
            $ipv4List = @()
            $ipv6List = @()
            $gateways = @()
            $dnsServers = @()

            if ($cfg) {
                $ipv4List = @($cfg.IPv4Address | ForEach-Object { "{0}/{1}" -f $_.IPAddress, $_.PrefixLength })
                $ipv6List = @($cfg.IPv6Address | ForEach-Object { "{0}/{1}" -f $_.IPAddress, $_.PrefixLength })
                $gateways = @($cfg.IPv4DefaultGateway | ForEach-Object { $_.NextHop })
            }

            foreach ($row in $dns) {
                $dnsServers += $row.ServerAddresses
            }
            $dnsServers = $dnsServers | Where-Object { $_ } | Sort-Object -Unique

            Write-Log ("- Adapter Name        : {0}" -f $adapter.Name)
            Write-Log ("  Description         : {0}" -f $adapter.InterfaceDescription)
            Write-Log ("  Status              : {0}" -f $adapter.Status)
            Write-Log ("  MAC                 : {0}" -f $adapter.MacAddress)
            Write-Log ("  LinkSpeed           : {0}" -f $adapter.LinkSpeed)
            Write-Log ("  InterfaceIndex      : {0}" -f $adapter.ifIndex)
            Write-Log ("  Virtual/Tunnel Hint : {0}" -f ($(if ($isVirtual) { "Yes" } else { "No" })))
            Write-Log ("  IPv4                : {0}" -f ($(if ($ipv4List) { $ipv4List -join ", " } else { "(none)" })))
            Write-Log ("  IPv6                : {0}" -f ($(if ($ipv6List) { $ipv6List -join ", " } else { "(none)" })))
            Write-Log ("  Default Gateway     : {0}" -f ($(if ($gateways) { $gateways -join ", " } else { "(none)" })))
            Write-Log ("  DNS                 : {0}" -f ($(if ($dnsServers) { $dnsServers -join ", " } else { "(none)" })))
            Write-Log ("  DHCP                : {0}" -f ($(if ($ipIf -and $ipIf.Dhcp -ne "Disabled") { $ipIf.Dhcp } else { "Disabled/Static/Unknown" })))
            Write-Log ""

            if ($adapter.Status -eq "Up" -and $cfg -and $cfg.IPv4Address) {
                foreach ($ip in $cfg.IPv4Address) {
                    $item = [pscustomobject]@{
                        InterfaceAlias = $adapter.Name
                        InterfaceIndex = $adapter.ifIndex
                        IPAddress      = $ip.IPAddress
                        PrefixLength   = $ip.PrefixLength
                        Gateway        = ($gateways | Select-Object -First 1)
                        IsVirtual      = $isVirtual
                    }
                    $activeIPv4Configs += $item
                    if ($isVirtual) {
                        $activeVirtualAdapters += $item
                    }
                }
            }
        }
    }

    if (-not $activeIPv4Configs) {
        Add-Finding FAIL "No Up adapter with IPv4 address was detected"
        Add-Recommendation "Check whether the machine is connected and has a valid IPv4 address"
    } else {
        Add-Finding OK ("Detected {0} active local IPv4 addresses" -f $activeIPv4Configs.Count)
    }

    $physicalActiveCount = @($activeIPv4Configs | Where-Object { -not $_.IsVirtual }).Count
    if ($physicalActiveCount -gt 1) {
        Add-Finding WARN ("Detected multiple active physical IPv4 interfaces ({0})" -f $physicalActiveCount)
        Add-Recommendation "If you only expect one network path, disable unrelated adapters or review interface metrics"
    }

    if ($activeVirtualAdapters.Count -gt 0) {
        Add-Finding WARN ("Detected active virtual or tunnel adapters ({0})" -f $activeVirtualAdapters.Count)
        Add-Recommendation "Check VPN, Docker, Hyper-V, VMware, WireGuard, or Tailscale adapters for route hijacking"
    }

    Write-Section "3. Route and Egress Decision"
    if ($defaultRoutes) {
        Write-Status INFO "Default IPv4 routes"
        foreach ($r in $defaultRoutes) {
            Write-Log ("- IfIndex={0} NextHop={1} RouteMetric={2} InterfaceMetric={3}" -f $r.InterfaceIndex, $r.NextHop, $r.RouteMetric, $r.InterfaceMetric)
        }

        $defaultGatewayCount = @($defaultRoutes | Select-Object -ExpandProperty NextHop -Unique).Count
        if ($defaultGatewayCount -gt 1) {
            Add-Finding WARN ("Detected multiple default gateways ({0})" -f $defaultGatewayCount)
            Add-Recommendation "Review default route priority and interface metrics"
        } else {
            Add-Finding OK "Default route count looks normal"
        }
    } else {
        Write-Status WARN "No IPv4 default route was found"
        Add-Finding FAIL "No IPv4 default route was found"
        Add-Recommendation "If the target is off-subnet, traffic will fail without a default gateway or static route"
    }

    Write-Status INFO "Full route table (route print)"
    Write-Log (Invoke-ExternalText -Command "route" -Arguments @("print"))

    if ($TargetIP) {
        $script:RouteToTarget = Get-RouteToTarget -Address $TargetIP
        if ($script:RouteToTarget) {
            Write-Status INFO ("Route for target {0}" -f $TargetIP)
            Write-Log ("- SourceAddress : {0}" -f $script:RouteToTarget.SourceAddress)
            Write-Log ("- InterfaceAlias: {0}" -f $script:RouteToTarget.InterfaceAlias)
            if ($script:RouteToTarget.NetRoute) {
                Write-Log ("- NextHop       : {0}" -f $script:RouteToTarget.NetRoute.NextHop)
                Write-Log ("- RouteMetric   : {0}" -f $script:RouteToTarget.NetRoute.RouteMetric)
                Write-Log ("- Destination   : {0}" -f $script:RouteToTarget.NetRoute.DestinationPrefix)
            }

            if ($script:RouteToTarget.InterfaceAlias -and (Get-VirtualAdapterHint -Name $script:RouteToTarget.InterfaceAlias -Description $script:RouteToTarget.InterfaceAlias)) {
                Add-Finding WARN ("Traffic to {0} appears to use virtual or tunnel interface {1}" -f $TargetIP, $script:RouteToTarget.InterfaceAlias)
            }
        } else {
            Write-Status WARN ("Could not infer route details for {0}" -f $TargetIP)
        }
    }

    Write-Section "4. Layer2 and ARP Neighbor Information"
    if ($TargetIP) {
        Write-Status INFO ("Sending one light probe to {0} to trigger ARP resolution" -f $TargetIP)
        [void](Test-Icmp -Address $TargetIP -Count 1)
    }

    if (Test-Command "Get-NetNeighbor") {
        $neighborTable = @(Get-NetNeighbor -AddressFamily IPv4 -ErrorAction SilentlyContinue | Sort-Object IPAddress)
        if ($neighborTable) {
            foreach ($n in $neighborTable) {
                Write-Log ("- IP={0} MAC={1} State={2} IfIndex={3}" -f $n.IPAddress, $n.LinkLayerAddress, $n.State, $n.ifIndex)
            }
        } else {
            Write-Status WARN "No IPv4 neighbor table entries were collected"
        }
    } else {
        Write-Status WARN "Get-NetNeighbor is unavailable, falling back to arp -a"
    }

    Write-Status INFO "ARP table (arp -a)"
    Write-Log (Invoke-ExternalText -Command "arp" -Arguments @("-a"))

    if ($TargetIP -and $neighborTable) {
        $targetNeighbor = @($neighborTable | Where-Object { $_.IPAddress -eq $TargetIP })
        if ($targetNeighbor) {
            foreach ($n in $targetNeighbor) {
                Write-Log ("Target neighbor: IP={0} MAC={1} State={2} IfIndex={3}" -f $n.IPAddress, $n.LinkLayerAddress, $n.State, $n.ifIndex)
            }

            $distinctMac = @($targetNeighbor | Select-Object -ExpandProperty LinkLayerAddress -Unique | Where-Object { $_ -and $_ -ne "00-00-00-00-00-00" })
            if ($distinctMac.Count -gt 1) {
                Add-Finding WARN ("Target {0} maps to multiple MAC addresses in the neighbor table" -f $TargetIP)
                Add-Recommendation "Check duplicate IP usage, proxy ARP, bridge networks, or path anomalies"
            }
        } else {
            Add-Finding WARN ("Target {0} is absent from the local neighbor table" -f $TargetIP)
        }
    }

    Write-Section "5. Connectivity Tests"
    $loopback = Test-Icmp -Address "127.0.0.1" -Count 2
    Write-Status ($(if ($loopback.Success) { "OK" } else { "FAIL" })) "Loopback test 127.0.0.1"
    if (-not $loopback.Success) {
        Add-Finding FAIL "Loopback test failed, local TCP/IP stack is unhealthy"
        Add-Recommendation "Fix the local network stack before investigating external connectivity"
    }

    foreach ($local in $activeIPv4Configs | Sort-Object IPAddress -Unique) {
        $selfTest = Test-Icmp -Address $local.IPAddress -Count 1
        Write-Status ($(if ($selfTest.Success) { "OK" } else { "WARN" })) ("Self ping {0} ({1})" -f $local.IPAddress, $local.InterfaceAlias)
    }

    $gatewaysToTest = @($activeIPv4Configs | Where-Object { $_.Gateway } | Select-Object -ExpandProperty Gateway -Unique)
    if ($gatewaysToTest) {
        foreach ($gw in $gatewaysToTest) {
            $gwResult = Test-Icmp -Address $gw -Count 2
            $script:GatewayPingOk[$gw] = $gwResult.Success
            Write-Status ($(if ($gwResult.Success) { "OK" } else { "FAIL" })) ("Gateway ping {0}" -f $gw)
            if (-not $gwResult.Success) {
                Add-Finding FAIL ("Default gateway {0} is unreachable" -f $gw)
                Add-Recommendation "Check VLAN, switch, Wi-Fi association, or basic L2 connectivity to the gateway"
            }
        }
    } else {
        Write-Status WARN "No default gateway is available for ping tests"
    }

    if ($TargetIP) {
        $targetPing = Test-Icmp -Address $TargetIP -Count 2
        $script:TargetIPPingOk = $targetPing.Success
        Write-Status ($(if ($targetPing.Success) { "OK" } else { "FAIL" })) ("Target IP ping {0}" -f $TargetIP)
    }

    if ($TargetIP) {
        Write-Status INFO ("tracert to target IP {0}" -f $TargetIP)
        Write-Log (Invoke-ExternalText -Command "tracert" -Arguments @("-d", "-h", "8", "-w", "1000", $TargetIP))
    }

    if ($TargetHost) {
        $resolved = Resolve-TargetHostIPs -HostName $TargetHost
        $script:TargetHostResolvedIPs = $resolved
        if ($resolved) {
            Write-Status OK ("Target host resolved {0} -> {1}" -f $TargetHost, ($resolved -join ", "))
        } else {
            Write-Status FAIL ("Target host resolution failed {0}" -f $TargetHost)
        }

        $hostPing = Test-Icmp -Address $TargetHost -Count 2
        $script:TargetHostPingOk = $hostPing.Success
        Write-Status ($(if ($hostPing.Success) { "OK" } else { "FAIL" })) ("Target host ping {0}" -f $TargetHost)

        Write-Status INFO ("tracert to target host {0}" -f $TargetHost)
        Write-Log (Invoke-ExternalText -Command "tracert" -Arguments @("-h", "8", "-w", "1000", $TargetHost))
    }

    if (($TargetIP -or $TargetHost) -and $Ports) {
        Write-Status INFO ("TCP port probes: {0}" -f ($Ports -join ", "))
    }

    if ($TargetIP -and $Ports) {
        foreach ($port in $Ports) {
            $r = Test-TcpPort -ComputerName $TargetIP -Port $port
            if ($r.Success) { $script:TargetTcpOpenCount++ }
            Write-Status ($(if ($r.Success) { "OK" } else { "WARN" })) ("TCP {0}:{1}" -f $TargetIP, $port)
        }
    }

    if ($TargetHost -and $Ports) {
        foreach ($port in $Ports) {
            $r = Test-TcpPort -ComputerName $TargetHost -Port $port
            if ($r.Success) { $script:TargetHostTcpOpenCount++ }
            Write-Status ($(if ($r.Success) { "OK" } else { "WARN" })) ("TCP {0}:{1}" -f $TargetHost, $port)
        }
    }

    Write-Section "6. DNS Diagnostics"
    Write-Status INFO "Current DNS client configuration"
    foreach ($row in $dnsAddrs | Sort-Object InterfaceIndex) {
        $servers = if ($row.ServerAddresses) { $row.ServerAddresses -join ", " } else { "(none)" }
        Write-Log ("- IfIndex={0} Servers={1}" -f $row.InterfaceIndex, $servers)
    }

    if (Test-Command "ipconfig") {
        Write-Status INFO "ipconfig /all"
        Write-Log (Invoke-ExternalText -Command "ipconfig" -Arguments @("/all"))
    }

    if ($TargetHost) {
        if ($script:TargetHostResolvedIPs.Count -eq 0) {
            Add-Finding FAIL ("DNS resolution failed for {0}" -f $TargetHost)
            Add-Recommendation "Check DNS server configuration, corporate proxy rules, VPN, or split DNS policy"
        } else {
            Add-Finding OK ("DNS resolution succeeded for {0}" -f $TargetHost)
        }

        if ($TargetIP -and $script:TargetHostResolvedIPs.Count -gt 0 -and ($script:TargetHostResolvedIPs -notcontains $TargetIP)) {
            Add-Finding WARN ("Resolved IPs for {0} do not include the requested TargetIP {1}" -f $TargetHost, $TargetIP)
            Add-Recommendation "Check hosts overrides, internal DNS, CDN, proxy, or wrong target selection"
        }

        if ($TargetIP -and $script:TargetIPPingOk -eq $true -and $script:TargetHostResolvedIPs.Count -eq 0) {
            Add-Finding FAIL "The IP is reachable but the hostname cannot be resolved, which indicates a DNS problem"
        }
    }

    Write-Section "7. Firewall, Security Policy, Proxy, and VPN"
    if (Test-Command "Get-NetFirewallProfile") {
        $profiles = @(Get-NetFirewallProfile -ErrorAction SilentlyContinue)
        if ($profiles) {
            foreach ($p in $profiles) {
                Write-Log ("- Profile={0} Enabled={1} DefaultInbound={2} DefaultOutbound={3}" -f $p.Name, $p.Enabled, $p.DefaultInboundAction, $p.DefaultOutboundAction)
            }
            Add-Finding INFO "Windows Firewall profile states were collected"
        } else {
            Write-Status WARN "Failed to read Windows Firewall profiles"
        }
    }

    Write-Status INFO "WinHTTP proxy"
    Write-Log (Invoke-ExternalText -Command "netsh" -Arguments @("winhttp", "show", "proxy"))

    try {
        $ieProxy = Get-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings" -ErrorAction Stop
        Write-Log ("User Proxy Enabled  : {0}" -f $ieProxy.ProxyEnable)
        Write-Log ("User Proxy Server   : {0}" -f $ieProxy.ProxyServer)
        Write-Log ("User Proxy Bypass   : {0}" -f $ieProxy.ProxyOverride)
    }
    catch {
        Write-Status WARN "Failed to read current user proxy settings"
    }

    $vpnLikeAdapters = @($adapters | Where-Object { Get-VirtualAdapterHint -Name $_.Name -Description $_.InterfaceDescription })
    if ($vpnLikeAdapters.Count -gt 0) {
        Add-Finding WARN ("Detected VPN or virtual interface hints: {0}" -f (($vpnLikeAdapters | Select-Object -ExpandProperty Name) -join ", "))
        Add-Recommendation "If ping works only one-way, disconnect VPN or tunnel interfaces and retest"
    }

    Write-Section "8. Wi-Fi and Physical Link Information"
    if (Test-Command "netsh") {
        Write-Status INFO "netsh wlan show interfaces"
        Write-Log (Invoke-ExternalText -Command "netsh" -Arguments @("wlan", "show", "interfaces"))
    } else {
        Write-Status WARN "netsh is unavailable, Wi-Fi details cannot be collected"
    }

    Write-Section "9. Automatic Diagnosis"
    if ($TargetIP) {
        $sameSubnetMatches = @()
        foreach ($item in $activeIPv4Configs) {
            if (Test-SameSubnet -LocalIP $item.IPAddress -PrefixLength $item.PrefixLength -RemoteIP $TargetIP) {
                $sameSubnetMatches += $item
            }
        }

        if ($sameSubnetMatches.Count -gt 0) {
            Add-Finding INFO ("Target {0} is in the same subnet as local interface(s): {1}" -f $TargetIP, (($sameSubnetMatches | Select-Object -ExpandProperty InterfaceAlias -Unique) -join ", "))
        } else {
            Add-Finding INFO ("Target {0} is not in any detected local IPv4 subnet and should rely on routing" -f $TargetIP)
            if (-not $defaultRoutes) {
                Add-Finding FAIL ("Target {0} is off-subnet and no default route was found" -f $TargetIP)
            }
        }

        if ($sameSubnetMatches.Count -gt 0 -and $script:TargetIPPingOk -eq $false) {
            Add-Finding WARN ("Same-subnet target {0} is unreachable by ping" -f $TargetIP)
            Add-Recommendation "Check ARP, client isolation, peer firewall, peer online status, and local egress behavior"
        }

        $gwReachableAny = @($script:GatewayPingOk.Values | Where-Object { $_ -eq $true }).Count -gt 0
        if ($TargetIPPingOk -eq $false -and $gwReachableAny) {
            Add-Finding WARN ("Gateway is reachable but target {0} is not" -f $TargetIP)
        }

        if ($TargetIPPingOk -eq $false -and $script:TargetTcpOpenCount -gt 0) {
            Add-Finding WARN ("Target {0} does not respond to ping but TCP ports are reachable" -f $TargetIP)
        }

        if ($TargetIPPingOk -eq $false) {
            Add-Finding WARN "For the one-way ping case (Windows can ping macOS, but macOS cannot ping Windows), focus on local egress policy, local route choice, adapter binding, peer ICMP response policy, and VPN or virtual adapter interference"
            Add-Recommendation "On Windows, verify inbound ICMPv4 Echo Request rules and confirm the network profile is not overly restrictive"
        }
    }

    Write-Section "10. Summary Findings"
    if ($script:Findings.Count -eq 0) {
        Write-Status INFO "No automatic findings were produced"
    } else {
        foreach ($f in $script:Findings) {
            Write-Status $f.Level $f.Message
        }
    }

    Write-Section "11. Next Step Recommendations"
    if ($script:Recommendations.Count -eq 0) {
        Add-Recommendation "Run the same diagnostics on both macOS and Windows, then compare routes, active adapters, ARP tables, firewall state, and VPN status"
    }

    $recommendationList = $script:Recommendations | Sort-Object -Unique
    foreach ($r in $recommendationList) {
        Write-Log ("- {0}" -f $r)
    }

    Write-Section "12. Report File"
    Write-Log ("Report saved to: {0}" -f $script:ReportFile)
}
catch {
    Write-Error $_.Exception.Message
    exit 1
}

# Optional remediation examples (commented out on purpose):
#
# Disable-NetAdapter -Name "vEthernet (Default Switch)" -Confirm:$false
# Get-NetFirewallRule -DisplayName "*Core Network Diagnostics*" | Format-Table
# Enable-NetFirewallRule -DisplayGroup "File and Printer Sharing"
