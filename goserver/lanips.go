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

// GetLANIPs returns local IPs ordered hotspot-first, like fileshare.get_lan_ips.
// Cached 8s: every /api/status poll used to fork `ip/hostname` processes.
func GetLANIPs() []string {
	lanIPMu.Lock()
	if nowUnix()-lanIPAt < lanIPTTLSec && lanIPCache != nil {
		out := append([]string{}, lanIPCache...)
		lanIPMu.Unlock()
		return out
	}
	lanIPMu.Unlock()
	ips := discoverLANIPs()
	lanIPMu.Lock()
	lanIPAt = nowUnix()
	lanIPCache = append([]string{}, ips...)
	lanIPMu.Unlock()
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
	if ips, err := net.LookupHost(mustHostname()); err == nil {
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
