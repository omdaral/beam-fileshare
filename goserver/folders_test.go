package beamcore

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSafeRelPath(t *testing.T) {
	valid := map[string]string{
		"a.txt":                 "a.txt",
		"Docs/a.pdf":            "Docs/a.pdf",
		"Docs/2026/deep/b.bin":  "Docs/2026/deep/b.bin",
		"  spaced dir /f.txt ":  "spaced dir/f.txt",
		"a/b (1).txt":           "a/b (1).txt",
		"uni-مجلد/ملف عربي.pdf": "uni-مجلد/ملف عربي.pdf",
	}
	for in, want := range valid {
		if got := safeRelPath(in); got != want {
			t.Errorf("safeRelPath(%q) = %q, want %q", in, got, want)
		}
	}
	invalid := []string{
		"", ".", "..", "/abs.txt", "../up.txt", "a/../../up.txt",
		"a//b.txt", "a/./b.txt", ".hidden", "a/.hidden", ".hdir/a.txt",
		"a/", "/a", "CON/a.txt", "a/COM1.txt", "a\\..\\up.txt",
		strings.Repeat("a", 600) + ".txt",
		strings.Repeat("d/", 11) + "f.txt",
	}
	for _, in := range invalid {
		if got := safeRelPath(in); got != "" {
			t.Errorf("safeRelPath(%q) = %q, want empty", in, got)
		}
	}
}

func getRawBytes(t *testing.T, url string) (int, http.Header, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, body
}

func TestFolderFlow(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	uid := "f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0"
	payload := []byte("folder-file-bytes")
	// init with a relative path
	st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": uid, "name": "Docs/a.txt", "size": len(payload)}, nil)
	if st != 200 {
		t.Fatalf("init = %d %v", st, j)
	}
	// v1 chunk + complete
	if st, _ := postBin(t, ts.URL+"/upload_chunk?id="+uid+"&offset=0", payload); st != 200 {
		t.Fatalf("chunk = %d", st)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete",
		map[string]interface{}{"id": uid}, nil)
	if st != 200 {
		t.Fatalf("complete = %d %v", st, j)
	}
	if saved, _ := j["saved"].([]interface{}); len(saved) != 1 || saved[0] != "Docs/a.txt" {
		t.Fatalf("saved = %v", j)
	}
	if _, err := os.Stat(filepath.Join(SharedDir, "Docs", "a.txt")); err != nil {
		t.Fatalf("file not on disk: %v", err)
	}

	// listing exposes path + dir entry, hides .uploads session residue
	st, j, _ = doJSON(t, "GET", ts.URL+"/files", nil, nil)
	if st != 200 {
		t.Fatalf("files = %d", st)
	}
	raw, _ := json.Marshal(j)
	if strings.Contains(string(raw), ".uploads") {
		t.Errorf("listing leaks .uploads: %s", raw)
	}
	files, _ := j["files"].([]interface{})
	found := false
	for _, f := range files {
		if m, _ := f.(map[string]interface{}); m["path"] == "Docs/a.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("path missing in listing: %v", j)
	}
	dirs, _ := j["dirs"].([]interface{})
	foundDir := false
	for _, d := range dirs {
		if m, _ := d.(map[string]interface{}); m["path"] == "Docs" {
			foundDir = true
			if m["files"].(float64) != 1 {
				t.Errorf("dir files = %v", m)
			}
			if over, _ := m["over"].(string); over != "" {
				t.Errorf("dir over = %q, want empty", over)
			}
		}
	}
	if !foundDir {
		t.Errorf("dir missing in listing: %v", j)
	}

	// download by path + hash by path
	st, _, body := getRawBytes(t, ts.URL+"/download?file=Docs%2Fa.txt")
	if st != 200 || string(body) != string(payload) {
		t.Fatalf("download = %d %q", st, body)
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/file_hash?file=Docs%2Fa.txt", nil, nil)
	if st != 200 || j["sha256"] == nil {
		t.Fatalf("hash = %d %v", st, j)
	}

	// zip the folder
	st, h, zbody := getRawBytes(t, ts.URL+"/download_zip?dir=Docs")
	if st != 200 {
		t.Fatalf("zip = %d", st)
	}
	if ct := h.Get("Content-Type"); ct != "application/zip" {
		t.Errorf("zip content-type = %q", ct)
	}
	if len(zbody) < 4 || string(zbody[:2]) != "PK" {
		t.Errorf("not a zip (%d bytes)", len(zbody))
	}
	if cd := h.Get("Content-Disposition"); !strings.Contains(cd, ".zip") {
		t.Errorf("content-disposition = %q", cd)
	}

	// recursive delete of the folder
	st, j, _ = doJSON(t, "POST", ts.URL+"/delete",
		map[string]interface{}{"dir": "Docs"}, nil)
	if st != 200 || j["ok"] != true {
		t.Fatalf("delete dir = %d %v", st, j)
	}
	if _, err := os.Stat(filepath.Join(SharedDir, "Docs")); !os.IsNotExist(err) {
		t.Errorf("dir still on disk")
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/files", nil, nil)
	if st != 200 {
		t.Fatalf("files after = %d", st)
	}
	if files, _ := j["files"].([]interface{}); len(files) != 0 {
		t.Errorf("files not empty after delete: %v", files)
	}
}

func TestFolderOverCap(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	oldTree, oldList := maxTreeFiles, maxListFiles
	maxTreeFiles, maxListFiles = 5, 5000
	defer func() { maxTreeFiles, maxListFiles = oldTree, oldList }()

	for i := 0; i < 7; i++ {
		p := filepath.Join(SharedDir, "Big", "f"+strconv.FormatInt(int64(i), 10)+".txt")
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		_ = os.WriteFile(p, []byte("x"), 0644)
	}
	// 12 levels deep on disk (uploads would reject it, listing must flag it)
	deep := SharedDir
	for i := 0; i < 12; i++ {
		deep = filepath.Join(deep, "d"+strconv.FormatInt(int64(i), 10))
	}
	_ = os.MkdirAll(deep, 0755)
	_ = os.WriteFile(filepath.Join(deep, "deep.txt"), []byte("x"), 0644)

	st, j, _ := doJSON(t, "GET", ts.URL+"/files", nil, nil)
	if st != 200 {
		t.Fatalf("files = %d", st)
	}
	over := map[string]string{}
	for _, d := range j["dirs"].([]interface{}) {
		if m, _ := d.(map[string]interface{}); m["path"] != nil {
			if o, _ := m["over"].(string); o != "" {
				over[m["path"].(string)] = o
			}
		}
	}
	if over["Big"] != "files" {
		t.Errorf("Big over = %q, want files (all=%v)", over["Big"], over)
	}
	foundDepth := false
	for p, o := range over {
		if strings.HasPrefix(p, "d0") && o == "depth" {
			foundDepth = true
		}
	}
	if !foundDepth {
		t.Errorf("no depth flag under d0 (over=%v)", over)
	}
	// over-cap folder still zips fine
	st, _, zbody := getRawBytes(t, ts.URL+"/download_zip?dir=Big")
	if st != 200 || len(zbody) < 4 || string(zbody[:2]) != "PK" {
		t.Errorf("zip Big = %d (%d bytes)", st, len(zbody))
	}
}

func TestDeleteGuards(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	_ = os.WriteFile(filepath.Join(SharedDir, "top.txt"), []byte("x"), 0644)
	for _, tc := range []struct {
		body map[string]interface{}
		want int
	}{
		{map[string]interface{}{"dir": "../x"}, 400},
		{map[string]interface{}{"dir": ""}, 400},
		{map[string]interface{}{"dir": "nope"}, 404},
		{map[string]interface{}{"dir": "top.txt"}, 404}, // a file is not a dir
		{map[string]interface{}{"file": "top.txt"}, 200},
		{map[string]interface{}{"file": "top.txt"}, 404}, // gone
		{map[string]interface{}{"file": "a/b/../c"}, 400},
	} {
		if st, _, _ := doJSON(t, "POST", ts.URL+"/delete", tc.body, nil); st != tc.want {
			t.Errorf("delete %v = %d, want %d", tc.body, st, tc.want)
		}
	}
	// empty parents are pruned after nested file delete
	nested := filepath.Join(SharedDir, "P", "Q", "n.txt")
	_ = os.MkdirAll(filepath.Dir(nested), 0755)
	_ = os.WriteFile(nested, []byte("x"), 0644)
	if st, _, _ := doJSON(t, "POST", ts.URL+"/delete",
		map[string]interface{}{"file": "P/Q/n.txt"}, nil); st != 200 {
		t.Fatalf("delete nested = %d", st)
	}
	if _, err := os.Stat(filepath.Join(SharedDir, "P")); !os.IsNotExist(err) {
		t.Errorf("empty parents not pruned")
	}
}

func TestNoVerifyFlow(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	uid := "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0"
	payload := []byte("turbo-bytes-0123456789")
	sum := sha256.Sum256(payload)
	full := hex.EncodeToString(sum[:])

	// init a turbo (noverify) session
	st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": uid, "name": "fast.bin",
			"size": len(payload), "piece_len": 262144, "noverify": true}, nil)
	if st != 200 {
		t.Fatalf("init = %d %v", st, j)
	}
	// piece WITHOUT hash must be accepted
	if st, _ := postBin(t, ts.URL+"/upload_piece?id="+uid+"&index=0", payload); st != 200 {
		t.Fatalf("piece without hash = %d, want 200", st)
	}
	// status advertises noverify
	st, j, _ = doJSON(t, "GET", ts.URL+"/upload_status?id="+uid, nil, nil)
	if st != 200 || j["noverify"] != true {
		t.Fatalf("status = %d %v", st, j)
	}
	// complete without full_hash must be rejected
	st, _, _ = doJSON(t, "POST", ts.URL+"/upload_complete",
		map[string]interface{}{"id": uid}, nil)
	if st != 422 {
		t.Fatalf("complete without full_hash = %d, want 422", st)
	}
	// complete with full_hash succeeds
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete",
		map[string]interface{}{"id": uid, "full_hash": full}, nil)
	if st != 200 || j["ok"] != true {
		t.Fatalf("complete = %d %v", st, j)
	}
	got, err := os.ReadFile(filepath.Join(SharedDir, "fast.bin"))
	if err != nil || string(got) != string(payload) {
		t.Fatalf("saved bytes mismatch: %v", err)
	}
}

func TestExtractZip(t *testing.T) {
	setupTestEnv(t)

	zpath := filepath.Join(SharedDir, "Pack.zip")
	f, err := os.Create(zpath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	add := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	add("Pack/a.txt", "AAA")
	add("Pack/sub/b.txt", "BBB")
	add("../evil.txt", "EVIL")
	add("/abs.txt", "ABS")
	add("Pack/.hidden", "HID")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	n, top, err := extractZip(zpath)
	if err != nil {
		t.Fatalf("extract = %v", err)
	}
	if n != 2 || top != "Pack" {
		t.Errorf("extract = %d %q, want 2 Pack", n, top)
	}
	if got, _ := os.ReadFile(filepath.Join(SharedDir, "Pack", "a.txt")); string(got) != "AAA" {
		t.Errorf("a.txt = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(SharedDir, "Pack", "sub", "b.txt")); string(got) != "BBB" {
		t.Errorf("b.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(SharedDir, "evil.txt")); !os.IsNotExist(err) {
		t.Errorf("zip-slip evil.txt escaped!")
	}
	if _, err := os.Stat(filepath.Join(SharedDir, "abs.txt")); !os.IsNotExist(err) {
		t.Errorf("absolute entry escaped!")
	}
	if _, err := os.Stat(filepath.Join(SharedDir, "Pack", ".hidden")); !os.IsNotExist(err) {
		t.Errorf("dotfile extracted!")
	}

	// async wrapper removes the zip on success
	extractZipAsync(zpath, "Pack.zip", "127.0.0.1")
	if _, err := os.Stat(zpath); !os.IsNotExist(err) {
		t.Errorf("zip not removed after successful extract")
	}

	// corrupt zip: error + file kept (transfer untouched)
	bad := filepath.Join(SharedDir, "bad.zip")
	_ = os.WriteFile(bad, []byte("not-a-zip"), 0644)
	extractZipAsync(bad, "bad.zip", "127.0.0.1")
	if _, err := os.Stat(bad); err != nil {
		t.Errorf("corrupt zip should be kept, err=%v", err)
	}
}

func TestExtractViaUpload(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	// build a real zip payload in memory
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("Up/c.txt")
	_, _ = w.Write([]byte("CCC"))
	_ = zw.Close()
	payload := buf.Bytes()

	uid := "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1"
	st, _, _ := doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": uid, "name": "Up.zip",
			"size": len(payload), "extract": true}, nil)
	if st != 200 {
		t.Fatalf("init = %d", st)
	}
	if st, _ := postBin(t, ts.URL+"/upload_chunk?id="+uid+"&offset=0", payload); st != 200 {
		t.Fatalf("chunk = %d", st)
	}
	st, j, _ := doJSON(t, "POST", ts.URL+"/upload_complete",
		map[string]interface{}{"id": uid, "extract": true}, nil)
	if st != 200 || j["ok"] != true {
		t.Fatalf("complete = %d %v", st, j)
	}
	// background extract: poll for the tree (zip removed on success)
	found := false
	for i := 0; i < 100; i++ {
		if got, err := os.ReadFile(filepath.Join(SharedDir, "Up", "c.txt")); err == nil && string(got) == "CCC" {
			found = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Fatalf("extracted tree missing")
	}
	// zip removal happens right after the tree lands (same background job):
	// poll for absence instead of asserting instantly (flaky under load).
	gone := false
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(filepath.Join(SharedDir, "Up.zip")); os.IsNotExist(err) {
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !gone {
		t.Errorf("zip should be removed after successful extract")
	}
}
