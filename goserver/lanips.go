package beamcore

import (
	"context"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ipv4Re = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
var (
	lanIPMu    sync.Mutex
	lanIPAt    float64
	lanIPCache []string
)

var (
	phoneLanMu sync.Mutex
	phoneLanIP string
)

// SetPhoneLanIP records the Android-reported LAN address (from
// WifiManager/ConnectivityManager via the Capacitor bridge). It is the only
// reliable source on Android, where the `ip`/`hostname` tools don't exist.
// Empty clears it.
func SetPhoneLanIP(ip string) {
	phoneLanMu.Lock()
	phoneLanIP = strings.TrimSpace(ip)
	phoneLanMu.Unlock()
}

// PhoneLanIP returns the Android-reported LAN address, if any.
func PhoneLanIP() string {
	phoneLanMu.Lock()
	defer phoneLanMu.Unlock()
	return phoneLanIP
}

// preferPhoneIP moves the Android-reported address to the front so the UI
// always shares the live address even right after a network switch (before
// the 8s discovery cache refreshes). Applied on top of cached results too,
// so a network change pushed from Java takes effect immediately.
func preferPhoneIP(ips []string) []string {
	phoneLanMu.Lock()
	pip := phoneLanIP
	phoneLanMu.Unlock()
	if pip == "" || len(ips) == 0 {
		return ips
	}
	for i, ip := range ips {
		if ip == pip {
			if i == 0 {
				return ips
			}
			out := make([]string, 0, len(ips))
			out = append(out, pip)
			out = append(out, ips[:i]...)
			out = append(out, ips[i+1:]...)
			return out
		}
	}
	return append([]string{pip}, ips...)
}

// dnsLookupTimeout bounds own-hostname resolution. Plain net.LookupHost
// has no timeout and can hang for tens of seconds on phones with broken
// DNS — every /api/status poll would then pile up blocked goroutines and
// starve the whole server (observed as /health flapping on Android).
const dnsLookupTimeout = 1500 * time.Millisecond

// lookupHostBounded resolves host with a hard timeout (nil on failure).
func lookupHostBounded(host string) []string {
	if host == "" {
		return nil
	}
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: dnsLookupTimeout}
			return d.DialContext(ctx, network, address)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), dnsLookupTimeout)
	defer cancel()
	ips, err := r.LookupHost(ctx, host)
	if err != nil {
		return nil
	}
	return ips
}

// GetLANIPs returns local IPs ordered hotspot-first, like fileshare.get_lan_ips.
// Cached 8s: every /api/status poll used to fork `ip/hostname` processes.
func GetLANIPs() []string {
	lanIPMu.Lock()
	if nowUnix()-lanIPAt < lanIPTTLSec && lanIPCache != nil {
		out := append([]string{}, lanIPCache...)
		lanIPMu.Unlock()
		return preferPhoneIP(out)
	}
	lanIPMu.Unlock()
	ips := discoverLANIPs()
	lanIPMu.Lock()
	lanIPAt = nowUnix()
	lanIPCache = append([]string{}, ips...)
	lanIPMu.Unlock()
	return preferPhoneIP(ips)
}

// addPhoneInterfaceIPs enumerates local IPv4s without fork/exec (Android
// ships no iproute2/hostname binaries — net.Interfaces is the only way).
func addPhoneInterfaceIPs(add func(string)) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			if v4 := ip.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsLinkLocalUnicast() {
				add(v4.String())
			}
		}
	}
}

// finalizeIPSet sorts hotspot-first and falls back to loopback when empty.
func finalizeIPSet(set map[string]bool) []string {
	delete(set, "127.0.0.1")
	delete(set, "127.0.1.1")
	ips := []string{}
	for ip := range set {
		if ip != "" && !strings.HasPrefix(ip, "0.") {
			ips = append(ips, ip)
		}
	}
	sort.Slice(ips, func(i, j int) bool {
		gi, gj := ipSortGroup(ips[i]), ipSortGroup(ips[j])
		if gi != gj {
			return gi < gj
		}
		return ips[i] < ips[j]
	})
	if len(ips) == 0 {
		return []string{"127.0.0.1"}
	}
	return ips
}
func discoverLANIPs() []string {
	set := map[string]bool{}
	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" || strings.HasPrefix(ip, "127.") || ip == "::1" {
			return
		}
		if strings.HasPrefix(ip, "0.") || strings.Contains(ip, "/") || strings.Contains(ip, ":") {
			return
		}
		parts := strings.Split(ip, ".")
		if len(parts) != 4 {
			return
		}
		for _, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 || n > 255 {
				return
			}
		}
		set[ip] = true
	}
	runOut := func(name string, args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return string(out)
	}
	if IsPhoneBuild {
		// Android has no `ip`/`hostname` CLIs: skip the fork/exec probes
		// (each costs up to its 2s timeout and they all fail) and use the
		// Java-pushed address + interface enumeration + the UDP-dial trick.
		// This keeps every /api/status poll fast so /health never starves.
		if ip := PhoneLanIP(); ip != "" {
			add(ip)
		}
		addPhoneInterfaceIPs(add)
		if conn, err := net.DialTimeout("udp", "8.8.8.8:80", 500*time.Millisecond); err == nil {
			if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
				add(addr.IP.String())
			}
			conn.Close()
		}
		return finalizeIPSet(set)
	}
	for _, target := range []string{"8.8.8.8", "10.42.0.1", "192.168.137.1"} {
		out := runOut("ip", "route", "get", target)
		if m := regexp.MustCompile(`\bsrc\s+(\d{1,3}(?:\.\d{1,3}){3})`).FindStringSubmatch(out); m != nil {
			add(m[1])
			break
		}
	}
	for _, tok := range strings.Fields(runOut("hostname", "-I")) {
		add(tok)
	}
	for _, m := range regexp.MustCompile(`\binet\s+(\d{1,3}(?:\.\d{1,3}){3})/\d+`).FindAllStringSubmatch(
		runOut("ip", "-4", "-o", "addr", "show", "up"), -1) {
		add(m[1])
	}
	if ips := lookupHostBounded(mustHostname()); ips != nil {
		for _, ip := range ips {
			add(ip)
		}
	}
	if conn, err := net.DialTimeout("udp", "8.8.8.8:80", 500*time.Millisecond); err == nil {
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			add(addr.IP.String())
		}
		conn.Close()
	}
	return finalizeIPSet(set)
}
func ipSortGroup(ip string) int {
	if strings.HasPrefix(ip, "10.42.0.") || strings.HasPrefix(ip, "192.168.137.") {
		return 0
	}
	if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") {
		return 1
	}
	return 2
}
func mustHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}
