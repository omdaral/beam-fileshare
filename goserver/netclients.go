package beamcore

import (
	"net"
	"runtime"
	"strings"
	"time"
)

func currentLANIP() string {
	if conn, err := net.DialTimeout("udp", "8.8.8.8:80", time.Second); err == nil {
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			ip := addr.IP.String()
			conn.Close()
			if ip != "" && !strings.HasPrefix(ip, "127.") {
				return ip
			}
		} else {
			conn.Close()
		}
	}
	if h := mustHostname(); h != "" {
		if ips, err := net.LookupHost(h); err == nil {
			for _, ip := range ips {
				if ip != "" && !strings.HasPrefix(ip, "127.") && !strings.Contains(ip, ":") {
					return ip
				}
			}
		}
	}
	return "127.0.0.1"
}
func detectClients() []string {
	code, out, _ := runCmd([]string{"arp", "-a"}, 5*time.Second, "ar")
	if code != 0 || out == "" {
		return []string{}
	}
	return parseArpOutput(out)
}
func parseArpOutput(out string) []string {
	seen := map[string]bool{}
	list := []string{}
	for _, ip := range ipv4Re.FindAllString(out, -1) {
		if strings.HasPrefix(ip, "224.") || strings.HasPrefix(ip, "239.") ||
			strings.HasPrefix(ip, "255.") {
			continue
		}
		if !seen[ip] {
			seen[ip] = true
			list = append(list, ip)
		}
		if len(list) >= 32 {
			break
		}
	}
	return list
}
func gatewayIP() string {
	hsMu.Lock()
	running, ip := hsRunning, hsIP
	hsMu.Unlock()
	if running && ip != "" {
		return ip
	}
	if runtime.GOOS == "windows" {
		if code, out, _ := runCmd([]string{"ipconfig"}, 5*time.Second, "ar"); code == 0 && strings.Contains(out, windowsGateway) {
			return windowsGateway
		}
		if running {
			return windowsGateway
		}
	} else {
		if hasCommand("ip") {
			if code, out, _ := runCmd([]string{"ip", "-4", "addr", "show"}, 5*time.Second, "ar"); code == 0 && strings.Contains(out, linuxGateway) {
				return linuxGateway
			}
		} else if hasCommand("nmcli") {
			if code, out, _ := runCmd([]string{"nmcli", "-t", "-f", "IP4.ADDRESS", "connection", "show", "Hotspot"}, 5*time.Second, "ar"); code == 0 && strings.Contains(out, linuxGateway) {
				return linuxGateway
			}
		}
		if running {
			return linuxGateway
		}
	}
	return currentLANIP()
}

// hotspotStatus returns live hotspot status (never fails).
func HotspotStatus() map[string]interface{} {
	hsMu.Lock()
	running := hsRunning
	ssid, ip, port, security := hsSSID, hsIP, hsPort, hsSecurity
	hsMu.Unlock()
	if running {
		if runtime.GOOS == "windows" {
			st := winTetherState()
			if st == "On" {
				// still running
			} else if st == "Off" || st == "InTransition" {
				running = false
			} else {
				if code, out, _ := runCmd([]string{"netsh", "wlan", "show", "hostednetwork"}, 5*time.Second, "ar"); code == 0 && netshShowsStopped(out) {
					running = false
				}
			}
		} else if hasCommand("nmcli") {
			if code, out, _ := runCmd([]string{"nmcli", "-t", "-f", "NAME", "connection", "show", "--active"}, 5*time.Second, "ar"); code == 0 && !strings.Contains(out, "Hotspot") {
				running = false
			}
		}
	}
	clients := []string{}
	if running {
		clients = detectClients()
	}
	displayIP := ip
	if displayIP == "" && running {
		displayIP = gatewayIP()
	}
	return map[string]interface{}{
		"running": running, "ssid": ssid, "ip": displayIP, "port": port,
		"clients": clients, "security": security,
	}
}

// netCapabilities reports (hotspotAvailable, adminCapable) cached 30s.
func netCapabilities() (bool, bool) {
	netcapMu.Lock()
	defer netcapMu.Unlock()
	if nowUnix()-netcapAt < netcapTTLSec {
		return netcapAvail, netcapCapable
	}
	capable := hotspotIsAdmin()
	avail, _ := hotspotSupportsAP("ar")
	netcapAt = nowUnix()
	netcapAvail = avail
	netcapCapable = capable
	return avail, capable
}
func hotspotClients() []string {
	NSMu.RLock()
	running := Net.HotspotRunning
	NSMu.RUnlock()
	if !running {
		return []string{}
	}
	st := HotspotStatus()
	if cl, ok := st["clients"].([]string); ok {
		return cl
	}
	return []string{}
}

// hotspotStartRequest wraps HotspotStart with pkexec retry on localhost.
// Race-free: pkexec is threaded as a param, never via global env.
func hotspotStartRequest(ssid, password string, port int, openNet bool, clientAddr string, lang string) (bool, string, map[string]interface{}) {
	ok, msg, info := HotspotStart(ssid, password, port, openNet, lang)
	if ok {
		return ok, msg, info
	}
	if !hotspotIsAdmin() && isLoopback(clientIPFromAddr(clientAddr)) &&
		pkexecAvailable() && runtime.GOOS != "windows" {
		ok2, msg2, info2 := HotspotStartWithPkexec(ssid, password, port, openNet, lang, true)
		if ok2 {
			return ok2, msg2, info2
		}
		if msg2 != "" {
			msg = msg2
		}
	}
	return ok, msg, info
}
func hotspotStopRequest(clientAddr string, lang string) (bool, string) {
	ok, msg := HotspotStop(lang)
	if ok {
		return ok, msg
	}
	if isLoopback(clientIPFromAddr(clientAddr)) && pkexecAvailable() && runtime.GOOS != "windows" {
		ok2, msg2 := HotspotStopWithPkexec(lang, true)
		if ok2 {
			return ok2, msg2
		}
		if msg2 != "" {
			msg = msg2
		}
	}
	return ok, msg
}

// clientIPFromAddr extracts IP from a "host:port" RemoteAddr.
func clientIPFromAddr(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
