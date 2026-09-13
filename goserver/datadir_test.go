package beamcore

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Beam keeps no side files: the share folder is always ~/Downloads/Beam
// (redirected via beamHomeOverride in tests), settings + log are memory.
func TestSharedDefaultDir(t *testing.T) {
	old := beamHomeOverride
	beamHomeOverride = "/home/u"
	defer func() { beamHomeOverride = old }()
	got := SharedDefaultDir()
	if got != filepath.Join("/home/u", "Downloads", "Beam") {
		t.Fatalf("SharedDefaultDir = %q", got)
	}
	// resolveDataPaths shim still points at the same well-known folder.
	s, l, c := resolveDataPaths("/app", "", "Shared")
	if s != got || l != "" || c != "" {
		t.Fatalf("resolve shim = %q %q %q", s, l, c)
	}
}

func TestMemLogRing(t *testing.T) {
	clearLog()
	writeLog("1.2.3.4", "test", "hello")
	if got := readLog(10); len(got) != 1 {
		t.Fatalf("readLog = %d lines, want 1", len(got))
	}
	clearLog()
	if got := readLog(10); len(got) != 0 {
		t.Fatalf("log should be empty after clear: %v", got)
	}
	// Ring cap is respected.
	for i := 0; i < memLogCap+50; i++ {
		writeLog("127.0.0.1", "spam", "x")
	}
	if got := readLog(memLogCap + 1000); len(got) != memLogCap {
		t.Fatalf("ring len = %d, want %d", len(got), memLogCap)
	}
	clearLog()
}

func TestIdleTracking(t *testing.T) {
	old := IdleTimeout
	IdleTimeout = time.Hour
	defer func() { IdleTimeout = old }()
	TouchActivity()
	left := idleLeft()
	if left <= 0 || left > time.Hour {
		t.Fatalf("idleLeft = %v", left)
	}
	if idleSecondsLeft() <= 0 {
		t.Fatalf("idleSecondsLeft = %d", idleSecondsLeft())
	}
	// Disabled timeout never fires.
	IdleTimeout = 0
	if idleLeft() <= 0 || idleSecondsLeft() != -1 {
		t.Fatalf("disabled idle = %v %d", idleLeft(), idleSecondsLeft())
	}
}

func TestBeamHealthy(t *testing.T) {
	beam := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"ok":true}`)) // compact like the real server
			return
		}
		w.WriteHeader(404)
	}))
	defer beam.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not beam"))
	}))
	defer other.Close()

	portOf := func(url string) int {
		parts := strings.Split(url[strings.LastIndex(url, ":")+1:], "/")
		var p int
		for _, c := range parts[0] {
			p = p*10 + int(c-'0')
		}
		return p
	}
	if !BeamHealthy(portOf(beam.URL)) {
		t.Fatal("beam server not detected")
	}
	if BeamHealthy(portOf(other.URL)) {
		t.Fatal("foreign server misdetected as Beam")
	}
	if BeamHealthy(1) { // closed port
		t.Fatal("closed port misdetected as Beam")
	}
}
