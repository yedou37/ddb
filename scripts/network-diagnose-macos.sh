#!/usr/bin/env bash
# macOS network diagnostics script
# - Uses only built-in tools.
# - Avoids destructive changes.
# - Produces a structured report with automatic findings.

set +e
umask 022

TARGET_IP=""
TARGET_HOST=""
PORTS=""
OUTPUT_DIR="./network-diagnostics"

usage() {
  cat <<'EOF'
Usage:
  bash network-diagnose-macos.sh [-t target_ip] [-h target_host] [-p ports] [-o output_dir]

Options:
  -t    Target IPv4 address, for example 192.168.1.20
  -h    Target hostname, for example example.com
  -p    Comma-separated port list, for example 22,80,443,3389
  -o    Output directory, default: ./network-diagnostics

Examples:
  bash network-diagnose-macos.sh -t 192.168.1.20
  bash network-diagnose-macos.sh -t 192.168.1.20 -p 2379,2380,8080
  bash network-diagnose-macos.sh -h example.com
  bash network-diagnose-macos.sh -t 192.168.1.20 -h example.com -p 22,80,443 -o ./reports
EOF
}

while getopts ":t:h:p:o:?" opt; do
  case "$opt" in
    t) TARGET_IP="$OPTARG" ;;
    h) TARGET_HOST="$OPTARG" ;;
    p) PORTS="$OPTARG" ;;
    o) OUTPUT_DIR="$OPTARG" ;;
    ?) usage; exit 0 ;;
    :) echo "missing argument for -$OPTARG" >&2; usage; exit 1 ;;
    \?) echo "unknown option -$OPTARG" >&2; usage; exit 1 ;;
  esac
done

mkdir -p "$OUTPUT_DIR" || {
  echo "failed to create output directory: $OUTPUT_DIR" >&2
  exit 1
}

TIMESTAMP="$(date '+%Y%m%d-%H%M%S')"
REPORT_FILE="$OUTPUT_DIR/network-diagnose-macos-$TIMESTAMP.txt"
FINDINGS_FILE="$OUTPUT_DIR/.network-findings-$TIMESTAMP.tmp"
RECOMM_FILE="$OUTPUT_DIR/.network-recommend-$TIMESTAMP.tmp"
IFACE_DB="$OUTPUT_DIR/.network-iface-$TIMESTAMP.tmp"

: > "$REPORT_FILE"
: > "$FINDINGS_FILE"
: > "$RECOMM_FILE"
: > "$IFACE_DB"

cleanup_tmp() {
  rm -f "$FINDINGS_FILE" "$RECOMM_FILE" "$IFACE_DB"
}
trap cleanup_tmp EXIT

if [ -z "$PORTS" ] && { [ -n "$TARGET_IP" ] || [ -n "$TARGET_HOST" ]; }; then
  PORTS="22,80,443,3389"
fi

log() {
  printf '%s\n' "$*" | tee -a "$REPORT_FILE"
}

section() {
  log ""
  log "=============================================================================="
  log "$1"
  log "=============================================================================="
}

status() {
  level="$1"
  shift
  log "[$level] $*"
}

add_finding() {
  level="$1"
  shift
  printf '[%s] %s\n' "$level" "$*" >> "$FINDINGS_FILE"
}

add_recommend() {
  printf '%s\n' "$*" >> "$RECOMM_FILE"
}

have_cmd() {
  command -v "$1" >/dev/null 2>&1
}

is_ipv4() {
  printf '%s\n' "$1" | grep -Eq '^([0-9]{1,3}\.){3}[0-9]{1,3}$'
}

safe_run() {
  title="$1"
  shift
  status INFO "$title"
  output="$("$@" 2>&1)"
  rc=$?
  if [ -n "$output" ]; then
    printf '%s\n' "$output" | tee -a "$REPORT_FILE" >/dev/null
  fi
  if [ $rc -ne 0 ]; then
    status WARN "command exited with code $rc: $*"
  fi
  return 0
}

safe_sh() {
  title="$1"
  cmd="$2"
  status INFO "$title"
  output="$(sh -c "$cmd" 2>&1)"
  rc=$?
  if [ -n "$output" ]; then
    printf '%s\n' "$output" | tee -a "$REPORT_FILE" >/dev/null
  fi
  if [ $rc -ne 0 ]; then
    status WARN "command exited with code $rc: $cmd"
  fi
  return 0
}

hexmask_to_prefix() {
  hexmask="$1"
  hexmask="${hexmask#0x}"
  if [ -z "$hexmask" ]; then
    echo ""
    return
  fi

  dec=$((16#$hexmask))
  prefix=0
  i=0
  while [ $i -lt 32 ]; do
    if [ $(( (dec >> (31 - i)) & 1 )) -eq 1 ]; then
      prefix=$((prefix + 1))
    fi
    i=$((i + 1))
  done
  echo "$prefix"
}

ipv4_to_int() {
  ip="$1"
  awk -F. 'NF==4 {print ($1*16777216)+($2*65536)+($3*256)+$4}' <<EOF
$ip
EOF
}

same_subnet() {
  ip1="$1"
  prefix="$2"
  ip2="$3"

  [ -z "$ip1" ] && return 1
  [ -z "$prefix" ] && return 1
  [ -z "$ip2" ] && return 1

  int1="$(ipv4_to_int "$ip1")"
  int2="$(ipv4_to_int "$ip2")"
  [ -z "$int1" ] && return 1
  [ -z "$int2" ] && return 1

  if [ "$prefix" -eq 0 ] 2>/dev/null; then
    mask=0
  else
    mask=$(( (0xFFFFFFFF << (32 - prefix)) & 0xFFFFFFFF ))
  fi

  if [ $(( int1 & mask )) -eq $(( int2 & mask )) ]; then
    return 0
  fi
  return 1
}

is_virtual_iface() {
  dev="$1"
  case "$dev" in
    lo0|utun*|awdl*|llw*|bridge*|vnic*|vmnet*|vboxnet*|tap*|tun*|gif*|stf*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

get_active_wifi_device() {
  networksetup -listallhardwareports 2>/dev/null | awk '
    /Hardware Port: Wi-Fi/ {getline; sub(/^Device: /,"",$0); print; exit}
    /Hardware Port: AirPort/ {getline; sub(/^Device: /,"",$0); print; exit}
  '
}

resolve_host_ips() {
  host="$1"

  if have_cmd dscacheutil; then
    dscacheutil -q host -a name "$host" 2>/dev/null | awk '/ip_address:/{print $2}' | sort -u
    return
  fi

  if have_cmd nslookup; then
    nslookup "$host" 2>/dev/null | awk '/^Address: /{print $2}' | sort -u
    return
  fi
}

tcp_probe() {
  target="$1"
  port="$2"

  if have_cmd nc; then
    out="$(nc -G 2 -vz "$target" "$port" 2>&1)"
    rc=$?
    printf '%s\n' "$out" | tee -a "$REPORT_FILE" >/dev/null
    return $rc
  fi

  if have_cmd bash; then
    bash -c "exec 3<>/dev/tcp/$target/$port" >/dev/null 2>&1
    return $?
  fi

  return 127
}

ping_once() {
  host="$1"
  ping -c 1 "$host" >/dev/null 2>&1
}

ping_twice() {
  host="$1"
  ping -c 2 "$host" >/dev/null 2>&1
}

ADMIN_MODE="No"
if [ "$(id -u)" -eq 0 ]; then
  ADMIN_MODE="Yes"
fi

if [ -n "$TARGET_IP" ] && ! is_ipv4 "$TARGET_IP"; then
  echo "Target IP is not a valid IPv4 address: $TARGET_IP" >&2
  exit 1
fi

if [ -n "$PORTS" ]; then
  OLD_IFS="$IFS"
  IFS=','
  for p in $PORTS; do
    case "$p" in
      ''|*[!0-9]*)
        echo "invalid port: $p" >&2
        exit 1
        ;;
      *)
        if [ "$p" -lt 1 ] || [ "$p" -gt 65535 ]; then
          echo "port out of range: $p" >&2
          exit 1
        fi
        ;;
    esac
  done
  IFS="$OLD_IFS"
fi

TARGET_IP_PING_OK=""
TARGET_HOST_PING_OK=""
TARGET_TCP_OPEN_COUNT=0
TARGET_HOST_TCP_OPEN_COUNT=0
TARGET_HOST_RESOLVED_IPS=""
DEFAULT_GW=""
DEFAULT_GW_REACHABLE=""
ROUTE_IFACE=""
ROUTE_GW=""
ROUTE_SRC=""
PHYSICAL_ACTIVE_IPV4_COUNT=0
VIRTUAL_ACTIVE_COUNT=0
TARGET_SAME_SUBNET=0

section "1. Basic System Information"
log "Timestamp           : $(date '+%Y-%m-%d %H:%M:%S %z')"
log "Hostname            : $(hostname)"
log "Current User        : $(id -un)"
log "Administrator Mode  : $ADMIN_MODE"
log "OS                  : $(sw_vers -productName 2>/dev/null)"
log "OS Version          : $(sw_vers -productVersion 2>/dev/null)"
log "Build Version       : $(sw_vers -buildVersion 2>/dev/null)"
log "Kernel              : $(uname -a)"

section "2. Adapters and IP Information"
IFACES="$(ifconfig -l 2>/dev/null)"
if [ -z "$IFACES" ]; then
  status FAIL "No network interface information was collected"
  add_finding FAIL "No network interface information was collected"
else
  for dev in $IFACES; do
    info="$(ifconfig "$dev" 2>/dev/null)"
    [ -z "$info" ] && continue

    flags_line="$(printf '%s\n' "$info" | head -n 1)"
    status_line="$(printf '%s\n' "$info" | awk '/status:/{print $2; exit}')"
    [ -z "$status_line" ] && status_line="unknown"
    ether="$(printf '%s\n' "$info" | awk '/ether /{print $2; exit}')"
    media="$(printf '%s\n' "$info" | awk -F': ' '/media:/{print $2; exit}')"
    inet_lines="$(printf '%s\n' "$info" | awk '/inet /{print $2 "|" $4}')"
    inet6_lines="$(printf '%s\n' "$info" | awk '/inet6 /{print $2}')"
    is_virtual_iface "$dev"
    is_virtual=$?
    virtual_text="No"
    [ $is_virtual -eq 0 ] && virtual_text="Yes"

    log "- Interface Name      : $dev"
    log "  Flags               : $flags_line"
    log "  Status              : $status_line"
    log "  MAC                 : ${ether:-"(none)"}"
    log "  Media               : ${media:-"(unknown)"}"
    log "  Virtual/Tunnel Hint : $virtual_text"

    if [ -n "$inet_lines" ]; then
      printf '%s\n' "$inet_lines" | while IFS='|' read -r ip maskhex; do
        prefix="$(hexmask_to_prefix "$maskhex")"
        log "  IPv4                : $ip/${prefix:-unknown}"
      done
    else
      log "  IPv4                : (none)"
    fi

    if [ -n "$inet6_lines" ]; then
      printf '%s\n' "$inet6_lines" | sed 's/^/  IPv6                : /' | tee -a "$REPORT_FILE" >/dev/null
    else
      log "  IPv6                : (none)"
    fi

    if [ "$dev" != "lo0" ] && [ -n "$inet_lines" ]; then
      printf '%s\n' "$inet_lines" | while IFS='|' read -r ip maskhex; do
        prefix="$(hexmask_to_prefix "$maskhex")"
        echo "$dev|$status_line|$ip|$prefix|$is_virtual" >> "$IFACE_DB"
      done
    fi

    if [ "$dev" != "lo0" ] && [ -n "$inet_lines" ]; then
      active_mark=1
      case "$status_line" in
        inactive|down) active_mark=0 ;;
      esac
      if [ $active_mark -eq 1 ]; then
        if [ $is_virtual -eq 0 ]; then
          VIRTUAL_ACTIVE_COUNT=$((VIRTUAL_ACTIVE_COUNT + 1))
        else
          PHYSICAL_ACTIVE_IPV4_COUNT=$((PHYSICAL_ACTIVE_IPV4_COUNT + 1))
        fi
      fi
    fi

    if have_cmd ipconfig; then
      dhcp_raw="$(ipconfig getpacket "$dev" 2>/dev/null)"
      if [ -n "$dhcp_raw" ]; then
        log "  Address Mode        : DHCP"
      elif [ -n "$inet_lines" ]; then
        log "  Address Mode        : Static/Unknown"
      else
        log "  Address Mode        : (none)"
      fi
    fi

    log ""
  done
fi

if [ ! -s "$IFACE_DB" ]; then
  add_finding FAIL "No active interface with IPv4 was detected"
  add_recommend "Verify that the machine is connected and has a valid IPv4 address"
else
  add_finding OK "Detected active local IPv4 interface data"
fi

if [ "$PHYSICAL_ACTIVE_IPV4_COUNT" -gt 1 ]; then
  add_finding WARN "Detected multiple active physical IPv4 interfaces"
  add_recommend "Disconnect unrelated adapters or review service order and routing priority"
fi

if [ "$VIRTUAL_ACTIVE_COUNT" -gt 0 ]; then
  add_finding WARN "Detected active virtual or tunnel interfaces"
  add_recommend "Review utun, bridge, vmnet, Docker, VPN, or VM-related interfaces"
fi

section "3. Route and Egress Decision"
safe_sh "IPv4 route table (netstat -rn -f inet)" "netstat -rn -f inet"
safe_sh "Default route (route -n get default)" "route -n get default"

DEFAULT_GW="$(route -n get default 2>/dev/null | awk '/gateway:/{print $2; exit}')"
DEFAULT_IFACE="$(route -n get default 2>/dev/null | awk '/interface:/{print $2; exit}')"

default_route_count="$(netstat -rn -f inet 2>/dev/null | awk '$1=="default"{count++} END{print count+0}')"
if [ "$default_route_count" -gt 1 ]; then
  add_finding WARN "Detected multiple default routes"
  add_recommend "Review service order, VPN routing, and default route priority"
else
  add_finding OK "Default route count looks normal"
fi

if [ -z "$DEFAULT_GW" ]; then
  add_finding FAIL "No default gateway was found"
  add_recommend "If the target is off-subnet, traffic will fail without a default route or static route"
fi

if [ -n "$TARGET_IP" ]; then
  section "3.1 Route For Target"
  route_out="$(route -n get "$TARGET_IP" 2>&1)"
  route_rc=$?
  printf '%s\n' "$route_out" | tee -a "$REPORT_FILE" >/dev/null

  if [ $route_rc -eq 0 ]; then
    ROUTE_IFACE="$(printf '%s\n' "$route_out" | awk '/interface:/{print $2; exit}')"
    ROUTE_GW="$(printf '%s\n' "$route_out" | awk '/gateway:/{print $2; exit}')"
    ROUTE_SRC="$(printf '%s\n' "$route_out" | awk '/source address:/{print $3; exit}')"
    status INFO "Traffic to $TARGET_IP uses interface ${ROUTE_IFACE:-unknown}, gateway ${ROUTE_GW:-direct}, source ${ROUTE_SRC:-unknown}"

    is_virtual_iface "$ROUTE_IFACE"
    if [ $? -eq 0 ]; then
      add_finding WARN "Traffic to $TARGET_IP appears to use virtual or tunnel interface $ROUTE_IFACE"
      add_recommend "Check whether VPN or tunnel routing is hijacking the target subnet"
    fi
  else
    add_finding WARN "Failed to infer route details for target $TARGET_IP"
  fi
fi

section "4. Layer2 and ARP Neighbor Information"
safe_sh "ARP neighbor table (arp -an)" "arp -an"

if [ -n "$TARGET_IP" ]; then
  status INFO "Sending one light probe to $TARGET_IP to trigger ARP resolution"
  ping_once "$TARGET_IP" >/dev/null 2>&1
  target_arp="$(arp -an 2>/dev/null | grep "($TARGET_IP)")"

  if [ -n "$target_arp" ]; then
    printf '%s\n' "$target_arp" | tee -a "$REPORT_FILE" >/dev/null
    mac_count="$(printf '%s\n' "$target_arp" | awk '{print $4}' | sort -u | wc -l | tr -d ' ')"
    if [ "$mac_count" -gt 1 ]; then
      add_finding WARN "Target $TARGET_IP maps to multiple MAC addresses in ARP"
      add_recommend "Check duplicate IP, proxy ARP, bridge networks, or L2 anomalies"
    fi
  else
    add_finding WARN "Target $TARGET_IP is absent from the local ARP table"
  fi
fi

section "5. Connectivity Tests"
if ping_twice "127.0.0.1"; then
  status OK "Loopback test 127.0.0.1"
else
  status FAIL "Loopback test 127.0.0.1"
  add_finding FAIL "Loopback test failed, local TCP/IP stack is unhealthy"
  add_recommend "Fix the local network stack before investigating external connectivity"
fi

if [ -s "$IFACE_DB" ]; then
  while IFS='|' read -r dev state ip prefix is_virtual; do
    if ping_once "$ip"; then
      status OK "Self ping $ip ($dev)"
    else
      status WARN "Self ping $ip ($dev)"
    fi
  done < "$IFACE_DB"
fi

if [ -n "$DEFAULT_GW" ]; then
  if ping_twice "$DEFAULT_GW"; then
    DEFAULT_GW_REACHABLE="Yes"
    status OK "Gateway ping $DEFAULT_GW"
  else
    DEFAULT_GW_REACHABLE="No"
    status FAIL "Gateway ping $DEFAULT_GW"
    add_finding FAIL "Default gateway is unreachable"
    add_recommend "Check Wi-Fi association, switch, VLAN, cable, or L2 isolation"
  fi
else
  status WARN "No default gateway is available for ping tests"
fi

if [ -n "$TARGET_IP" ]; then
  if ping_twice "$TARGET_IP"; then
    TARGET_IP_PING_OK="Yes"
    status OK "Target IP ping $TARGET_IP"
  else
    TARGET_IP_PING_OK="No"
    status FAIL "Target IP ping $TARGET_IP"
  fi

  if have_cmd traceroute; then
    safe_sh "traceroute to target IP $TARGET_IP" "traceroute -m 8 -w 1 '$TARGET_IP'"
  else
    status WARN "traceroute is unavailable"
  fi
fi

if [ -n "$TARGET_HOST" ]; then
  TARGET_HOST_RESOLVED_IPS="$(resolve_host_ips "$TARGET_HOST")"
  if [ -n "$TARGET_HOST_RESOLVED_IPS" ]; then
    status OK "Target host resolved $TARGET_HOST -> $(printf '%s' "$TARGET_HOST_RESOLVED_IPS" | paste -sd ', ' -)"
  else
    status FAIL "Target host resolution failed $TARGET_HOST"
  fi

  if ping_twice "$TARGET_HOST"; then
    TARGET_HOST_PING_OK="Yes"
    status OK "Target host ping $TARGET_HOST"
  else
    TARGET_HOST_PING_OK="No"
    status FAIL "Target host ping $TARGET_HOST"
  fi

  if have_cmd traceroute; then
    safe_sh "traceroute to target host $TARGET_HOST" "traceroute -m 8 -w 1 '$TARGET_HOST'"
  fi
fi

if [ -n "$PORTS" ] && { [ -n "$TARGET_IP" ] || [ -n "$TARGET_HOST" ]; }; then
  status INFO "TCP port probes: $PORTS"
fi

if [ -n "$TARGET_IP" ] && [ -n "$PORTS" ]; then
  OLD_IFS="$IFS"
  IFS=','
  for port in $PORTS; do
    if tcp_probe "$TARGET_IP" "$port"; then
      TARGET_TCP_OPEN_COUNT=$((TARGET_TCP_OPEN_COUNT + 1))
      status OK "TCP $TARGET_IP:$port"
    else
      status WARN "TCP $TARGET_IP:$port"
    fi
  done
  IFS="$OLD_IFS"
fi

if [ -n "$TARGET_HOST" ] && [ -n "$PORTS" ]; then
  OLD_IFS="$IFS"
  IFS=','
  for port in $PORTS; do
    if tcp_probe "$TARGET_HOST" "$port"; then
      TARGET_HOST_TCP_OPEN_COUNT=$((TARGET_HOST_TCP_OPEN_COUNT + 1))
      status OK "TCP $TARGET_HOST:$port"
    else
      status WARN "TCP $TARGET_HOST:$port"
    fi
  done
  IFS="$OLD_IFS"
fi

section "6. DNS Diagnostics"
if have_cmd scutil; then
  safe_run "System resolver info (scutil --dns)" scutil --dns
else
  status WARN "scutil is unavailable"
fi

if have_cmd networksetup; then
  safe_run "Network service order (networksetup -listnetworkserviceorder)" networksetup -listnetworkserviceorder
  services="$(networksetup -listallnetworkservices 2>/dev/null | sed '1d' | sed '/^\*/d')"
  if [ -n "$services" ]; then
    printf '%s\n' "$services" | while IFS= read -r svc; do
      [ -z "$svc" ] && continue
      dns_info="$(networksetup -getdnsservers "$svc" 2>/dev/null)"
      log "- DNS($svc): ${dns_info:-"(unknown)"}"
    done
  fi
fi

if [ -n "$TARGET_HOST" ]; then
  if [ -z "$TARGET_HOST_RESOLVED_IPS" ]; then
    add_finding FAIL "DNS resolution failed for $TARGET_HOST"
    add_recommend "Check DNS configuration, corporate proxy rules, VPN, or internal DNS reachability"
  else
    add_finding OK "DNS resolution succeeded for $TARGET_HOST"
  fi

  if [ -n "$TARGET_IP" ] && [ "$TARGET_IP_PING_OK" = "Yes" ] && [ -z "$TARGET_HOST_RESOLVED_IPS" ]; then
    add_finding FAIL "The IP is reachable but the hostname cannot be resolved, which indicates a DNS problem"
  fi

  if [ -n "$TARGET_IP" ] && [ -n "$TARGET_HOST_RESOLVED_IPS" ]; then
    echo "$TARGET_HOST_RESOLVED_IPS" | grep -qx "$TARGET_IP"
    if [ $? -ne 0 ]; then
      add_finding WARN "Resolved IPs for $TARGET_HOST do not include the requested TargetIP $TARGET_IP"
      add_recommend "Check hosts overrides, internal DNS, CDN, proxy, or a mismatched target"
    fi
  fi
fi

section "7. Firewall, Security Policy, Proxy, and VPN"
if [ -x /usr/libexec/ApplicationFirewall/socketfilterfw ]; then
  safe_run "Application firewall state" /usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate
  safe_run "Application firewall stealth mode" /usr/libexec/ApplicationFirewall/socketfilterfw --getstealthmode
else
  status WARN "socketfilterfw is unavailable"
fi

if have_cmd pfctl; then
  pf_info="$(pfctl -s info 2>&1)"
  pf_rc=$?
  status INFO "PF state"
  printf '%s\n' "$pf_info" | tee -a "$REPORT_FILE" >/dev/null
  if [ $pf_rc -ne 0 ]; then
    status WARN "Detailed PF output usually requires administrator privileges"
  fi
fi

if have_cmd scutil; then
  safe_run "System proxy config (scutil --proxy)" scutil --proxy
fi

if have_cmd scutil; then
  vpn_list="$(scutil --nc list 2>/dev/null)"
  if [ -n "$vpn_list" ]; then
    status INFO "VPN services (scutil --nc list)"
    printf '%s\n' "$vpn_list" | tee -a "$REPORT_FILE" >/dev/null
  fi
fi

active_virtual_names="$(awk -F'|' '$5==0 {print $1}' "$IFACE_DB" | sort -u | paste -sd ', ' -)"
if [ -n "$active_virtual_names" ]; then
  add_finding WARN "Detected active virtual or tunnel interfaces: $active_virtual_names"
  add_recommend "If ping works only one-way, disconnect VPN or virtual interfaces and retest"
fi

section "8. Wi-Fi and Physical Link Information"
WIFI_DEV="$(get_active_wifi_device)"
if [ -n "$WIFI_DEV" ]; then
  wifi_network="$(networksetup -getairportnetwork "$WIFI_DEV" 2>/dev/null)"
  status INFO "Wi-Fi device: $WIFI_DEV"
  log "$wifi_network"

  AIRPORT_BIN="/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport"
  if [ -x "$AIRPORT_BIN" ]; then
    safe_run "Wi-Fi details (airport -I)" "$AIRPORT_BIN" -I
  else
    status WARN "airport tool is unavailable"
  fi
else
  status INFO "No Wi-Fi hardware port was detected, or the machine is using wired networking"
fi

section "9. Automatic Diagnosis"
if [ -n "$TARGET_IP" ] && [ -s "$IFACE_DB" ]; then
  while IFS='|' read -r dev state ip prefix is_virtual; do
    if same_subnet "$ip" "$prefix" "$TARGET_IP"; then
      TARGET_SAME_SUBNET=1
      break
    fi
  done < "$IFACE_DB"

  if [ "$TARGET_SAME_SUBNET" -eq 1 ]; then
    add_finding INFO "Target $TARGET_IP is in the same subnet as at least one local interface"
  else
    add_finding INFO "Target $TARGET_IP is off-subnet and should rely on routing"
    if [ -z "$DEFAULT_GW" ]; then
      add_finding FAIL "Target is off-subnet and no default gateway was found"
    fi
  fi

  if [ "$TARGET_SAME_SUBNET" -eq 1 ] && [ "$TARGET_IP_PING_OK" = "No" ]; then
    add_finding WARN "Same-subnet target does not respond to ping"
    add_recommend "Check ARP, client isolation, peer firewall, peer online status, and local egress behavior"
  fi

  if [ "$DEFAULT_GW_REACHABLE" = "Yes" ] && [ "$TARGET_IP_PING_OK" = "No" ]; then
    add_finding WARN "Gateway is reachable but the target is not"
  fi

  if [ "$TARGET_IP_PING_OK" = "No" ] && [ "$TARGET_TCP_OPEN_COUNT" -gt 0 ]; then
    add_finding WARN "Target does not respond to ping but TCP ports are reachable"
  fi

  if [ -n "$ROUTE_IFACE" ]; then
    is_virtual_iface "$ROUTE_IFACE"
    if [ $? -eq 0 ]; then
      add_finding WARN "Traffic to $TARGET_IP uses virtual or tunnel interface $ROUTE_IFACE"
    fi
  fi

  if [ "$TARGET_IP_PING_OK" = "No" ]; then
    add_finding WARN "For the one-way ping case (Windows can ping macOS, but macOS cannot ping Windows), focus on local egress policy, local route choice, adapter binding, peer ICMP response policy, and VPN or virtual adapter interference"
    add_recommend "On Windows, verify inbound ICMPv4 Echo Request rules and confirm the network profile is not overly restrictive"
  fi
fi

section "10. Summary Findings"
if [ -s "$FINDINGS_FILE" ]; then
  sort -u "$FINDINGS_FILE" | while IFS= read -r line; do
    log "$line"
  done
else
  status INFO "No automatic findings were produced"
fi

section "11. Next Step Recommendations"
if [ -s "$RECOMM_FILE" ]; then
  sort -u "$RECOMM_FILE" | while IFS= read -r line; do
    log "- $line"
  done
else
  log "- Run the same diagnostics on both macOS and Windows, then compare routes, active interfaces, ARP tables, firewall state, and VPN status."
fi

section "12. Report File"
log "Report saved to: $REPORT_FILE"

# Optional remediation examples (commented out on purpose):
#
# networksetup -setairportpower Wi-Fi off
# networksetup -ordernetworkservices "USB 10/100/1000 LAN" "Wi-Fi"
# sudo pfctl -sr
