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
	stopOnce sync.Once
	// StopCh closes when the server is ordered to stop (via /api/server/stop).
	StopCh = make(chan struct{})
)

// Shutdown stops the HTTP listener and signals StopCh (idempotent).
func Shutdown() {
	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}
	stopOnce.Do(func() { close(StopCh) })
}

// Run serves HTTP on the port (blocking). Use Shutdown to stop it.
func Run(port int) error {
	httpServer = &http.Server{
		Addr:              "0.0.0.0:" + strconv.Itoa(port),
		Handler:           http.HandlerFunc(Route),
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		IdleTimeout:       httpIdleTimeout,
		// No WriteTimeout: 20GB downloads/uploads over slow LAN need
		// hours; per-request ctx cancellation still aborts gone clients.
		MaxHeaderBytes: httpMaxHeaderBytes,
	}
	return httpServer.ListenAndServe()
}

// Route dispatches every request (exported for the CLI and mobile).
func Route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
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
// well-known ~/Downloads/Beam share folder is used.
func resolveDataPaths(baseDir, _dataDir, _sharedCfg string) (shared, logs, cfgPath string) {
	return SharedDefaultDir(), "", ""
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

// BeamHealthy reports whether a Beam server (not just any program)
// answers on the port.
func BeamHealthy(port int) bool {
	client := http.Client{Timeout: 2 * time.Second}
	for _, host := range []string{"127.0.0.1", "::1"} {
		resp, err := client.Get("http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/health")
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
