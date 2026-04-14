package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ifaceIPv4 struct {
	name string
	cidr string
	ip   net.IP
	net  *net.IPNet
}

func main() {
	var (
		peerRaw   string
		portsRaw  string
		timeout   time.Duration
		pingCount int
		showPing  bool
	)

	flag.StringVar(&peerRaw, "peer", "", "Peer IPv4 address to test, for example 192.168.1.35")
	flag.StringVar(&portsRaw, "ports", "", "Comma-separated TCP ports to test, for example 2379,20082,21002")
	flag.DurationVar(&timeout, "timeout", 2*time.Second, "Timeout per network check")
	flag.IntVar(&pingCount, "ping-count", 2, "Number of ping probes")
	flag.BoolVar(&showPing, "show-ping-output", false, "Print raw ping output")
	flag.Parse()

	if peerRaw == "" {
		exitf("missing required --peer")
	}
	if pingCount < 1 {
		exitf("--ping-count must be at least 1")
	}

	peerIP := net.ParseIP(peerRaw)
	if peerIP == nil || peerIP.To4() == nil {
		exitf("--peer must be a valid IPv4 address")
	}
	peerIP = peerIP.To4()

	ports, err := parsePorts(portsRaw)
	if err != nil {
		exitf("invalid --ports: %v", err)
	}

	ifaces, err := listIPv4Interfaces()
	if err != nil {
		exitf("list local interfaces: %v", err)
	}

	fmt.Printf("Peer IP: %s\n", peerIP)
	fmt.Println()
	fmt.Println("Local IPv4 Interfaces:")
	if len(ifaces) == 0 {
		fmt.Println("- no active non-loopback IPv4 interfaces found")
	} else {
		for _, item := range ifaces {
			fmt.Printf("- %s: %s\n", item.name, item.cidr)
		}
	}

	fmt.Println()
	routedIP, routedErr := detectOutboundIPv4(peerIP)
	if routedErr != nil {
		fmt.Printf("Route Check: unable to infer outbound interface: %v\n", routedErr)
	} else {
		fmt.Printf("Route Check: local outbound IP to reach %s is %s\n", peerIP, routedIP)
	}

	matches := subnetMatches(ifaces, peerIP)
	fmt.Println()
	if len(matches) == 0 {
		fmt.Println("Same Subnet: no")
		fmt.Println("Reason: peer IP does not fall into any detected local IPv4 CIDR")
	} else {
		fmt.Println("Same Subnet: yes")
		for _, item := range matches {
			fmt.Printf("- matched %s (%s)\n", item.name, item.cidr)
		}
	}

	fmt.Println()
	pingResult := runPing(peerIP.String(), pingCount, timeout)
	fmt.Printf("Ping: %s\n", pingResult.summary)
	if showPing && pingResult.output != "" {
		fmt.Println()
		fmt.Println("Ping Output:")
		fmt.Println(pingResult.output)
	}

	var failures []string
	if len(matches) == 0 {
		failures = append(failures, "peer is not in any detected local subnet")
	}
	if !pingResult.success {
		failures = append(failures, "ping failed")
	}

	if len(ports) > 0 {
		fmt.Println()
		fmt.Println("TCP Checks:")
		for _, port := range ports {
			addr := net.JoinHostPort(peerIP.String(), strconv.Itoa(port))
			start := time.Now()
			conn, err := net.DialTimeout("tcp", addr, timeout)
			elapsed := time.Since(start).Round(time.Millisecond)
			if err != nil {
				fmt.Printf("- %s: fail (%v, %s)\n", addr, err, elapsed)
				failures = append(failures, fmt.Sprintf("tcp %s failed", addr))
				continue
			}
			_ = conn.Close()
			fmt.Printf("- %s: ok (%s)\n", addr, elapsed)
		}
	}

	fmt.Println()
	if len(failures) == 0 {
		fmt.Println("Summary: PASS")
		return
	}

	fmt.Println("Summary: FAIL")
	for _, item := range dedupeStrings(failures) {
		fmt.Printf("- %s\n", item)
	}
	os.Exit(1)
}

type pingResult struct {
	success bool
	summary string
	output  string
}

func runPing(peer string, count int, timeout time.Duration) pingResult {
	args := pingArgs(peer, count, timeout)
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return pingResult{
			success: false,
			summary: fmt.Sprintf("fail (%v)", err),
			output:  output,
		}
	}
	return pingResult{
		success: true,
		summary: fmt.Sprintf("ok (%d probes)", count),
		output:  output,
	}
}

func pingArgs(peer string, count int, timeout time.Duration) []string {
	timeoutMs := strconv.Itoa(int(timeout.Milliseconds()))
	if runtime.GOOS == "windows" {
		return []string{"ping", "-n", strconv.Itoa(count), "-w", timeoutMs, peer}
	}

	seconds := max(1, int(timeout.Seconds()))
	return []string{"ping", "-c", strconv.Itoa(count), "-W", strconv.Itoa(seconds), peer}
}

func parsePorts(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	items := strings.Split(raw, ",")
	ports := make([]int, 0, len(items))
	seen := map[int]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, errors.New("empty port entry")
		}
		port, err := strconv.Atoi(item)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", item, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("port %d out of range", port)
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports, nil
}

func listIPv4Interfaces() ([]ifaceIPv4, error) {
	netIfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []ifaceIPv4
	for _, iface := range netIfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			result = append(result, ifaceIPv4{
				name: iface.Name,
				cidr: ipNet.String(),
				ip:   ip4,
				net:  &net.IPNet{IP: ip4.Mask(ipNet.Mask), Mask: ipNet.Mask},
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].name == result[j].name {
			return result[i].cidr < result[j].cidr
		}
		return result[i].name < result[j].name
	})
	return result, nil
}

func subnetMatches(ifaces []ifaceIPv4, peerIP net.IP) []ifaceIPv4 {
	var matches []ifaceIPv4
	for _, item := range ifaces {
		if item.net != nil && item.net.Contains(peerIP) {
			matches = append(matches, item)
		}
	}
	return matches
}

func detectOutboundIPv4(peerIP net.IP) (net.IP, error) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(peerIP.String(), "9"), 500*time.Millisecond)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || localAddr.IP == nil {
		return nil, errors.New("unexpected local address type")
	}
	return localAddr.IP.To4(), nil
}

func dedupeStrings(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
