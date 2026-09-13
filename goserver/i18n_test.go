package beamcore

import (
	"net/http"
	"testing"
)

func TestTrTable(t *testing.T) {
	if got := tr("ar", "delete_missing"); got != "غير موجود" {
		t.Errorf("ar = %q", got)
	}
	if got := tr("en", "delete_missing"); got != "Not found" {
		t.Errorf("en = %q", got)
	}
	if got := tr("fr", "delete_missing"); got != "غير موجود" {
		t.Errorf("unknown lang should fall back to ar, got %q", got)
	}
	if got := tr("ar", "no_such_key"); got != "no_such_key" {
		t.Errorf("unknown key should echo, got %q", got)
	}
	if got := tr("en", "hs_short_pw", 3, 8, 8); got !=
		"Network password too short (3/8 chars). Use at least 8 chars." {
		t.Errorf("format args broken: %q", got)
	}
}

func reqWithHeaders(headers map[string]string) *http.Request {
	req, _ := http.NewRequest("GET", "http://x/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func TestReqLang(t *testing.T) {
	cases := []struct {
		headers map[string]string
		want    string
	}{
		{nil, "ar"},
		{map[string]string{"X-Lang": "en"}, "en"},
		{map[string]string{"X-Lang": "EN"}, "en"},
		{map[string]string{"Accept-Language": "en-US,en;q=0.9"}, "en"},
		{map[string]string{"Accept-Language": "ar-EG,ar;q=0.9"}, "ar"},
		{map[string]string{"Accept-Language": "fr-FR"}, "ar"},
		{map[string]string{"X-Lang": "ar", "Accept-Language": "en-US"}, "ar"},
	}
	for i, c := range cases {
		if got := reqLang(reqWithHeaders(c.headers)); got != c.want {
			t.Errorf("case %d = %q, want %q", i, got, c.want)
		}
	}
}

func TestDeleteSpeaksClientLanguage(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	st, j, _ := doJSON(t, "POST", ts.URL+"/delete",
		map[string]interface{}{"file": "nope-missing.txt"},
		map[string]string{"X-Lang": "en"})
	if st != 404 || j["error"] != "Not found" {
		t.Fatalf("en delete = %d %v", st, j)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/delete",
		map[string]interface{}{"file": "nope-missing.txt"}, nil)
	if st != 404 || j["error"] != "غير موجود" {
		t.Fatalf("ar delete = %d %v", st, j)
	}
}
