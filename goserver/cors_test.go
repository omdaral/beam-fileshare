package beamcore

import (
	"net/http/httptest"
	"testing"
)

func TestTrustedAppOrigin(t *testing.T) {
	for _, ok := range []string{
		"capacitor://localhost",
		"ionic://localhost",
		"https://localhost", // Capacitor 8 default androidScheme serves the app here
		"https://localhost:3000", // dev + iOS WebView variations
		"https://127.0.0.1:2004", // loopback over self-signed HTTPS
		"http://localhost:8100",
		"http://127.0.0.1:2004",
		"http://beam.local:2004",
		"https://beam.local:2004",
		"http://myphone.local:2004",
	} {
		if !trustedAppOrigin(ok) {
			t.Fatalf("expected trusted origin %q", ok)
		}
	}
	for _, bad := range []string{
		"https://evil.example.com",
		"http://192.168.1.5:2004",
		"null",
		"",
		"::::",
	} {
		if trustedAppOrigin(bad) {
			t.Fatalf("expected untrusted origin %q", bad)
		}
	}
}

func TestPreflightAndCORSHeaders(t *testing.T) {
	// Preflight from the app must succeed (was 404 before the fix,
	// which broke every Capacitor POST behind a preflight).
	req := httptest.NewRequest("OPTIONS", "/api/config", nil)
	req.Header.Set("Origin", "capacitor://localhost")
	rec := httptest.NewRecorder()
	Route(rec, req)
	if rec.Code != 204 {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "capacitor://localhost" {
		t.Fatalf("missing ACAO header: %v", rec.Header())
	}

	// Untrusted origins get no ACAO reflection.
	req2 := httptest.NewRequest("GET", "/health", nil)
	req2.Header.Set("Origin", "https://evil.example.com")
	rec2 := httptest.NewRecorder()
	Route(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("health status = %d, want 200", rec2.Code)
	}
	if h := rec2.Header().Get("Access-Control-Allow-Origin"); h != "" {
		t.Fatalf("evil origin reflected: %q", h)
	}
}

func TestCheckOriginAcceptsCapacitor(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/config", nil)
	req.Host = "127.0.0.1:2004"
	req.Header.Set("Origin", "capacitor://localhost")
	req.Header.Set("Content-Type", "application/json")
	if !checkOrigin(req) {
		t.Fatal("capacitor origin must pass checkOrigin")
	}
}
