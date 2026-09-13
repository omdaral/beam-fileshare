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
	core.BaseDir = dataDir
	core.SharedDir = filepath.Join(dataDir, "Beam")
	_ = os.MkdirAll(core.SharedDir, 0755)
	core.Cfg = core.LoadConfig()
	core.Cfg.Port = port
	core.ServerPort = port
	core.AppVersion = version
	core.IdleTimeout = 5 * time.Hour // auto-off saves phone battery too
	core.TouchActivity()
	core.SweepUploads()
	core.StartNameDiscovery(core.GetLANIPs)
	core.NSMu.Lock()
	core.Net.SSID = "Beam"
	core.Net.LanMode = true
	core.Net.Security = "wpa"
	core.Net.WifiPassword = ""
	core.NSMu.Unlock()
}

// Start boots the server (idempotent). Returns "" on success or an error message.
func Start(dataDir string, port int, version string) string {
	if running.Load() {
		return ""
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
	return "down"
}

// ShareDir returns the on-phone share folder.
func ShareDir() string {
	return core.SharedDir
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
