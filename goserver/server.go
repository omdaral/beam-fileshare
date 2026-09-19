package beamcore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTP server lifecycle shared by the desktop CLI and the mobile wrapper.
var httpServer *http.Server

var (
	stopMu sync.Mutex
	// stopClosed guards StopCh: the desktop CLI waits on it once, while the
	// Android wrapper restarts the engine in-process (ResetShutdown re-arms
	// it — a sync.Once could never do that, which broke every 2nd Start).
	stopClosed bool
	// StopCh closes when the server is ordered to stop (via /api/server/stop).
	StopCh = make(chan struct{})
)

// shutdownTimeout bounds Shutdown: generous on desktop (slow disks), short
// on the phone so the Capacitor bridge call never ANRs the app.
func shutdownTimeout() time.Duration {
	if IsPhoneBuild {
		return 3 * time.Second
	}
	return 10 * time.Second
}

// Shutdown stops the HTTP listener and signals StopCh (idempotent per
// generation — ResetShutdown re-arms it for the next Start).
func Shutdown() {
	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout())
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}
	stopMu.Lock()
	defer stopMu.Unlock()
	if !stopClosed {
		stopClosed = true
		close(StopCh)
	}
}

// ResetShutdown re-arms the stop signal for a fresh Run (the Android
// wrapper restarts the engine in-process; the old sync.Once stayed spent
// after the first Stop and poisoned every later generation).
func ResetShutdown() {
	stopMu.Lock()
	defer stopMu.Unlock()
	if stopClosed {
		StopCh = make(chan struct{})
		stopClosed = false
	}
}

// Run serves HTTP on the port (blocking). Use Shutdown to stop it.
func Run(port int) error {
	ResetShutdown()    // a previous Stop must not poison this generation
	rotateOwnerToken() // fresh session owner token per start
	httpServer = &http.Server{
		Addr:              "0.0.0.0:" + strconv.Itoa(port),
		Handler:           http.HandlerFunc(Route),
		ReadHeaderTimeout: httpReadHeaderTimeout,
		// No ReadTimeout (0): slow uploads stream bodies for minutes /
		// hours and a fixed read deadline would kill them mid-body.
		// Headers stay bounded by ReadHeaderTimeout above.
		ReadTimeout: 0,
		IdleTimeout: httpIdleTimeout,
		// No WriteTimeout: 20GB downloads/uploads over slow LAN need
		// hours; per-request ctx cancellation still aborts gone clients.
		MaxHeaderBytes: httpMaxHeaderBytes,
	}
	if TLSEnabled {
		certFile, keyFile := tlsCertFiles()
		if certFile == "" {
			if cf, kf, err := EnsureTLSCert(); err == nil {
				certFile, keyFile = cf, kf
			}
		}
		if certFile != "" {
			return httpServer.ListenAndServeTLS(certFile, keyFile)
		}
		// Cert unavailable: fall back to plain HTTP rather than not serving.
	}
	return httpServer.ListenAndServe()
}

// BaseURL builds the scheme-aware URL for an IP (https when TLS is on).
func BaseURL(ip string, port int) string {
	return TLSScheme() + "://" + ip + ":" + strconv.Itoa(port)
}

// Route dispatches every request (exported for the CLI and mobile).
func Route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// CORS for the Android wrapper must precede any WriteHeader.
	setCORS(w, r)
	// Preflight for Capacitor fetch POSTs (admin APIs with JSON bodies).
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}
	// Any real request means a human/device is using Beam: reset the
	// idle auto-shutdown timer. /health probes are excluded so monitoring
	// loops never keep the server awake by themselves.
	if path != "/health" {
		TouchActivity()
	}
	if r.Method == "GET" || r.Method == "HEAD" {
		if strings.HasPrefix(path, "/vendor/") {
			handleVendor(w, r)
			return
		}
		if strings.HasPrefix(path, "/app/") {
			handleApp(w, r)
			return
		}
		if strings.HasPrefix(path, "/r/") {
			handleRegistryGet(w, r)
			return
		}
		switch path {
		case "/", "/index.html", "/admin", "/guest":
			handleIndex(w, r)
			return
		case "/health":
			handleHealth(w, r)
			return
		case "/manifest.json":
			handleManifest(w, r)
			return
		case "/files":
			handleFiles(w, r)
			return
		case "/download":
			handleDownload(w, r)
			return
		case "/download_zip":
			handleDownloadZip(w, r)
			return
		case "/api/status":
			handleAPIStatus(w, r)
			return
		case "/api/net/status":
			handleNetStatus(w, r)
			return
		case "/api/logs":
			handleLogs(w, r)
			return
		case "/api/net/clients":
			handleNetClients(w, r)
			return
		case "/api/wifi/detect":
			handleWifiDetect(w, r)
			return
		case "/api/config":
			handleConfigGet(w, r)
			return
		case "/upload_status":
			handleUploadStatus(w, r)
			return
		case "/upload_sessions":
			handleUploadSessions(w, r)
			return
		case "/file_hash":
			handleFileHash(w, r)
			return
		case "/api/shares":
			handleAPIShares(w, r)
			return
		case "/api/browse":
			handleAPIBrowse(w, r)
			return
		case "/api/relay/need":
			handleAPIRelayNeed(w, r)
			return
		case "/api/temp":
			handleAPITemp(w, r)
			return
		case "/api/folder.zip":
			handleAPIFolderZip(w, r)
			return
		}
		sendEmpty(w, r, 404)
		return
	}
	if r.Method == "POST" {
		switch path {
		case "/api/logs/clear":
			handleLogsClear(w, r)
			return
		case "/api/net/start":
			handleNetStart(w, r)
			return
		case "/api/net/stop":
			handleNetStop(w, r)
			return
		case "/api/config":
			handleConfigPost(w, r)
			return
		case "/api/server/stop":
			handleServerStop(w, r)
			return
		case "/upload_init":
			handleUploadInit(w, r)
			return
		case "/upload_chunk":
			handleUploadChunk(w, r)
			return
		case "/upload_piece":
			handleUploadPiece(w, r)
			return
		case "/upload_complete":
			handleUploadComplete(w, r)
			return
		case "/upload_find":
			handleUploadFind(w, r)
			return
		case "/delete":
			handleDelete(w, r)
			return
		case "/upload":
			handleUploadMultipart(w, r)
			return
		case "/api/share":
			handleAPIShare(w, r)
			return
		case "/api/share/guest":
			handleAPIShareGuest(w, r)
			return
		case "/api/unshare":
			handleAPIUnshare(w, r)
			return
		case "/api/relay/piece":
			handleAPIRelayPiece(w, r)
			return
		case "/api/presence":
			handleAPIPresence(w, r)
			return
		case "/api/temp/clean":
			handleAPITempClean(w, r)
			return
		}
		sendEmpty(w, r, 404)
		return
	}
	sendEmpty(w, r, 404)
}

// ExeDir returns the directory holding the running binary.
func ExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		dir, _ := os.Getwd()
		return dir
	}
	if rp, err := filepath.EvalSymlinks(exe); err == nil {
		exe = rp
	}
	return filepath.Dir(exe)
}

// resolveDataPaths is kept as a tiny helper for tests: Beam stores
// nothing next to the exe anymore (no config.json, no logs/) — only the
// well-known temp dir is used.
func resolveDataPaths(baseDir, _dataDir, _sharedCfg string) (shared, logs, cfgPath string) {
	return TempDefaultDir(), "", ""
}

// IsPortOpen probes a local TCP port (v4 + v6 localhost).
func IsPortOpen(port int) bool {
	for _, host := range []string{"127.0.0.1", "::1"} {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

// beamHealthyTimeout bounds the self-probe: the phone's 2s budget flaked
// under Doze/GC pauses while the listener was fine (WebView fetch with a
// 4s budget still answered) — observed as "native health -> down" flapping
// next to "http probe -> ok" in production diagnostics.
func beamHealthyTimeout() time.Duration {
	if IsPhoneBuild {
		return 4 * time.Second
	}
	return 2 * time.Second
}

// BeamHealthy reports whether a Beam server (not just any program)
// answers on the port.
func BeamHealthy(port int) bool {
	probes := []http.Client{{Timeout: beamHealthyTimeout()}}
	if TLSEnabled {
		// Self-signed LAN cert: skip verification for the loopback self-probe.
		// #nosec G402 -- loopback health check against our own cert.
		probes = append(probes, http.Client{
			Timeout: beamHealthyTimeout(),
			Transport: &http.Transport{
				TLSClientConfig: insecureTLSConfig(),
			},
		})
	}
	schemes := []string{"http://"}
	if TLSEnabled {
		schemes = []string{"https://", "http://"}
	}
	for _, scheme := range schemes {
		for _, ci := range probes {
			c := ci
			for _, host := range []string{"127.0.0.1", "::1"} {
				resp, err := c.Get(scheme + net.JoinHostPort(host, strconv.Itoa(port)) + "/health")
				if err != nil {
					continue
				}
				body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
				resp.Body.Close()
				if err != nil || resp.StatusCode != 200 {
					continue
				}
				// Structured check (not substring): {"ok":true}
				var v struct {
					OK bool `json:"ok"`
				}
				if err := json.Unmarshal(body, &v); err == nil && v.OK {
					return true
				}
			}
		}
	}
	return false
}

// JoinRunningInstance opens the already-running Beam web UI instead of
// failing on a busy port. Returns true when it handled the situation
// (the caller must then exit 0).
func JoinRunningInstance(port int, noBrowser bool, localURL string) bool {
	if !BeamHealthy(port) {
		return false
	}
	fmt.Println("السيرفر شغال أصلاً — فتح الواجهة في المتصفح بدل تشغيل نسخة ثانية.")
	fmt.Printf("  الواجهة: %s\n", localURL)
	if !noBrowser {
		OpenBrowser(localURL)
	}
	return true
}
