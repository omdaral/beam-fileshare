package beamcore

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func getRaw(t *testing.T, client *http.Client, url string, headers map[string]string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(body)
}

func TestSinglePageServedEverywhere(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()
	c := noRedirectClient()

	// One page for everyone: /, /index.html, and the legacy /admin + /guest
	// aliases all serve the same page (owner sections gated client-side).
	var first string
	for _, p := range []string{"/", "/index.html", "/admin", "/guest"} {
		st, _, body := getRaw(t, c, ts.URL+p, nil)
		if st != 200 {
			t.Fatalf("GET %s = %d, want 200", p, st)
		}
		if first == "" {
			first = body
		} else if body != first {
			t.Errorf("GET %s serves different bytes than /", p)
		}
	}
	for _, want := range []string{"ownerZone", "Beam", "themeBtn", "langBtn", "logo-svg",
		"beam-theme", "data-i18n", "/vendor/qrcode.js", "/app/app.js", "/app/style.css",
		"heroUrl", "netShared"} {
		if !strings.Contains(first, want) {
			t.Errorf("single page missing %q", want)
		}
	}
	// Split UI (no bundler): interactive logic lives in /app/app.js.
	// The shell must reference it; content checks live in TestAppSplit.
	for _, want := range []string{"/app/app.js", "/app/style.css"} {
		if !strings.Contains(first, want) {
			t.Errorf("split shell missing %q", want)
		}
	}
	if strings.Contains(first, "QRmini") {
		t.Error("page must not contain the hand-rolled QRmini")
	}
}

func TestOwnerZoneGatedClientSide(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	// Owner sees admin APIs open; guest gets 403 everywhere. The page hides
	// #ownerZone via is_admin, the server enforces it.
	st, j, _ := doJSON(t, "GET", ts.URL+"/api/status", nil, nil)
	if st != 200 || j["is_admin"] != true {
		t.Fatalf("localhost must be admin: %d %v", st, j)
	}

	oldLoopback := isLoopback
	isLoopback = func(string) bool { return false }
	defer func() { isLoopback = oldLoopback }()

	st, j, _ = doJSON(t, "GET", ts.URL+"/api/status", nil, nil)
	if st != 200 || j["is_admin"] != false {
		t.Fatalf("guest must not be admin: %d %v", st, j)
	}
	for _, tc := range []struct {
		method, path string
		body         map[string]interface{}
		want         int
	}{
		{"POST", "/api/net/start", map[string]interface{}{"mode": "lan"}, 403},
		{"POST", "/api/net/stop", map[string]interface{}{}, 403},
		{"POST", "/api/logs/clear", map[string]interface{}{}, 403},
		{"GET", "/api/logs?tail=1", nil, 403},
		{"GET", "/api/config", nil, 403},
		{"POST", "/api/config", map[string]interface{}{"port": 8001}, 403},
		{"POST", "/api/server/stop", map[string]interface{}{}, 403},
	} {
		st, _, _ := doJSON(t, tc.method, ts.URL+tc.path, tc.body, nil)
		if st != tc.want {
			t.Errorf("guest %s %s = %d, want %d", tc.method, tc.path, st, tc.want)
		}
	}
}

func TestVendorQRScript(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	st, h, body := getRaw(t, noRedirectClient(), ts.URL+"/vendor/qrcode.js", nil)
	if st != 200 {
		t.Fatalf("vendor js = %d", st)
	}
	if ct := h.Get("Content-Type"); ct != "application/javascript; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	for _, want := range []string{"qrcode", "addData", "isDark", "getModuleCount", "Kazuhiko Arase"} {
		if !strings.Contains(body, want) {
			t.Errorf("vendor lib missing %q", want)
		}
	}
}

func TestVendorFonts(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()
	c := noRedirectClient()

	for _, f := range []string{"cairo-400.woff2", "cairo-700.woff2", "cairo-900.woff2"} {
		st, h, body := getRaw(t, c, ts.URL+"/vendor/"+f, nil)
		if st != 200 {
			t.Fatalf("%s = %d", f, st)
		}
		if ct := h.Get("Content-Type"); ct != "font/woff2" {
			t.Errorf("%s content-type = %q", f, ct)
		}
		if len(body) < 10000 || body[:4] != "wOF2" {
			t.Errorf("%s not a real woff2 (%d bytes)", f, len(body))
		}
	}
	st, _, _ := getRaw(t, c, ts.URL+"/vendor/evil.js", nil)
	if st != 404 {
		t.Errorf("unknown vendor file = %d, want 404", st)
	}
}

func TestAppSplit(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()
	c := noRedirectClient()
	// Split UI without bundler: shell references /app/*, logic served separately.
	for _, tc := range []struct {
		path, ctype, want string
	}{
		{"/app/config.js", "application/javascript; charset=utf-8", "var CONFIG={"},
		{"/app/i18n.js", "application/javascript; charset=utf-8", "var STR={"},
		{"/app/i18n.js", "application/javascript; charset=utf-8", "setLang"},
		{"/app/api.js", "application/javascript; charset=utf-8", "postJSON"},
		{"/app/ui.js", "application/javascript; charset=utf-8", "setBtn"},
		{"/app/files.js", "application/javascript; charset=utf-8", "renderFiles"},
		{"/app/upload.js", "application/javascript; charset=utf-8", "uploadOne"},
		{"/app/upload_queue.js", "application/javascript; charset=utf-8", "pumpQueue"},
		{"/app/upload_zip.js", "application/javascript; charset=utf-8", "buildFolderZip"},
		{"/app/download.js", "application/javascript; charset=utf-8", "fastDownload"},
		{"/app/net.js", "application/javascript; charset=utf-8", "fetchStatus"},
		{"/app/app.js", "application/javascript; charset=utf-8", "resetPolling"},
		{"/app/style.css", "text/css; charset=utf-8", "--bg0"},
	} {
		st, h, body := getRaw(t, c, ts.URL+tc.path, nil)
		if st != 200 {
			t.Fatalf("GET %s = %d, want 200", tc.path, st)
		}
		if ct := h.Get("Content-Type"); ct != tc.ctype {
			t.Errorf("%s content-type = %q, want %q", tc.path, ct, tc.ctype)
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s missing %q", tc.path, tc.want)
		}
	}
	st, _, _ := getRaw(t, c, ts.URL+"/app/evil.js", nil)
	if st != 404 {
		t.Errorf("unknown app file = %d, want 404", st)
	}
}

func TestSTRDictHasNoCode(t *testing.T) {
	// The STR dictionary must be pure data: any call inside it kills the
	// whole page script at load time (zero backend connectivity).
	// Split UI: dictionary lives in /app/i18n.js (embedded appFS).
	pageBytes, err := appFS.ReadFile("web/app/i18n.js")
	if err != nil {
		pageBytes = indexHTML // fallback: old monolith dev override
	}
	page := string(pageBytes)
	start := strings.Index(page, "var STR={")
	end := strings.Index(page[start:], "\n};")
	if start < 0 || end < 0 {
		t.Fatal("STR dictionary block not found")
	}
	dict := page[start : start+end]
	for _, banned := range []string{`T("`, "setBtn(", `$("`, "ICONS.", "fetch(", "=>"} {
		if strings.Contains(dict, banned) {
			t.Errorf("STR dictionary must not contain code %q", banned)
		}
	}
}
