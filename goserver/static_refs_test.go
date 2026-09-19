package beamcore

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Regression guard for the missing-shares.js outage: every local /app/*
// script or stylesheet referenced by the embedded index.html must actually
// be served (embedded + whitelisted). A 404 on any of them silently kills
// a whole feature (uploads registered nothing while bytes hit the disk).
func TestIndexRefsAllServed(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()
	c := noRedirectClient()

	st, _, body := getRaw(t, c, ts.URL+"/", nil)
	if st != 200 {
		t.Fatalf("GET / = %d", st)
	}
	re := regexp.MustCompile(`(?:src|href)="(/app/[^"]+)"`)
	seen := map[string]bool{}
	var paths []string
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		p := m[1]
		if strings.Contains(p, "..") || strings.Contains(p, "\\") {
			continue
		}
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		t.Fatal("no /app/* refs found in index.html")
	}
	sort.Strings(paths)
	for _, p := range paths {
		st, h, b := getRaw(t, c, ts.URL+p, nil)
		if st != 200 {
			t.Errorf("GET %s = %d, want 200 (referenced by index.html but not served)", p, st)
			continue
		}
		ct := h.Get("Content-Type")
		if strings.HasSuffix(p, ".js") && ct != "application/javascript; charset=utf-8" {
			t.Errorf("%s content-type = %q", p, ct)
		}
		if strings.HasSuffix(p, ".css") && ct != "text/css; charset=utf-8" {
			t.Errorf("%s content-type = %q", p, ct)
		}
		if len(b) == 0 {
			t.Errorf("%s served empty", p)
		}
	}
}
