// Package beamapp exposes the Beam server engine to Android via gomobile.
//
// Only gomobile-safe signatures are exported (string/int/bool, no structs).
// The desktop CLI (cmd/beam) uses the same beamcore engine directly.
package beamapp

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	core "fileshare"
)

var running atomic.Bool

// configure wires the engine to an app-private directory.
func configure(dataDir string, port int, version string) {
	core.ApplyEnvOverrides()
	core.IsPhoneBuild = true
	core.BaseDir = dataDir
	// The only auto-created dir is the temp dir (Beam-Temp).
	core.TempDir = filepath.Join(dataDir, "Beam-Temp")
	core.SharedDir = core.TempDir // deprecated alias stays pointed at temp
	_ = os.MkdirAll(core.TempDir, 0755)
	core.Cfg = core.LoadConfig()
	core.Cfg.TempDir = core.TempDir
	core.Cfg.Port = port
	core.ServerPort = port
	core.AppVersion = version
	core.IdleTimeout = 5 * time.Hour // auto-off saves phone battery too
	core.TouchActivity()
	core.RestoreTempShares() // before the sweeps so adopted files count as live
	core.SweepUploads()
	core.SweepTempOrphans()
	core.StartNameDiscovery(core.GetLANIPs)
	core.RestartIdleMonitor() // fresh monitor per Start (restartable)
	core.NSMu.Lock()
	core.Net.SSID = "Beam"
	core.Net.LanMode = true
	core.Net.Security = "wpa"
	core.Net.WifiPassword = ""
	core.NSMu.Unlock()
}

// Start boots the server (idempotent). Returns "" on success or an error message.
func Start(dataDir string, port int, version string) string {
	// Restart-safe: the web UI's own stop button (POST /api/server/stop)
	// shuts the listener without clearing this flag (no Capacitor bridge
	// inside the served page to call Stop()). Trust the flag only when the
	// port really answers as Beam; otherwise fall through and reboot.
	if running.Load() {
		if core.BeamHealthy(port) {
			return ""
		}
		running.Store(false)
		core.Shutdown()
	}
	if dataDir == "" || port < 1 || port > 65535 {
		return "bad data dir or port"
	}
	configure(dataDir, port, version)
	go func() {
		_ = core.Run(port) // Shutdown() unblocks this with ErrServerClosed
	}()
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if core.IsPortOpen(port) {
			running.Store(true)
			return ""
		}
	}
	return "server did not come up"
}

// Stop shuts the server down. Returns "" on success.
func Stop() string {
	core.StopIdleMonitor()   // a stopped server must not kill its successor
	core.StopNameDiscovery() // stop advertising + free sockets for next Start
	core.Shutdown()
	running.Store(false)
	return ""
}

// Health reports "ok", "down" or "stopped".
func Health(port int) string {
	if !running.Load() {
		return "stopped"
	}
	if core.BeamHealthy(port) {
		return "ok"
	}
	// The self-HTTP probe flakes under Doze/GC while the listener is fine
	// (WebView fetch still answers — seen in production diagnostics) — a
	// live TCP accept means the server is up; only a closed port is "down".
	if core.IsPortOpen(port) {
		return "ok"
	}
	return "down"
}

// SetLanIp records the Android-reported LAN address (the only reliable
// source on the phone) so /api/status and QR codes share the live address
// immediately, even before the discovery cache refreshes.
func SetLanIp(ip string) string {
	core.SetPhoneLanIP(ip)
	return ""
}

// ShareDir returns the on-phone share folder.
func ShareDir() string {
	return core.SharedDir
}

// OwnerToken returns the session owner token (the app bridge is an
// owner context, like loopback). Lets native-side flows prove
// ownership without depending on the peer address.
func OwnerToken() string {
	return core.OwnerTokenHex()
}

// Version returns the engine version.
func Version() string {
	return core.AppVersion
}

// SetHotspotCreds advertises phone-hotspot credentials (from the LOHS plugin)
// so the existing QR UI shares them automatically. Empty values keep current.
func SetHotspotCreds(ssid, pass, security string) string {
	core.NSMu.Lock()
	defer core.NSMu.Unlock()
	if ssid != "" {
		core.Net.SSID = ssid
	}
	if pass != "" {
		core.Net.WifiPassword = pass
	}
	if security == "open" || security == "wpa" {
		core.Net.Security = security
	}
	core.Net.LanMode = false
	core.Net.HotspotRunning = true
	return ""
}

// ClearHotspot reverts the network state to LAN mode.
func ClearHotspot() string {
	core.NSMu.Lock()
	defer core.NSMu.Unlock()
	core.Net.LanMode = true
	core.Net.HotspotRunning = false
	core.Net.Security = "wpa"
	core.Net.WifiPassword = ""
	return ""
}
