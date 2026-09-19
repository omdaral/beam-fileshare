package beamcore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func testServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(Route))
}

func doJSON(t *testing.T, method, url string, body interface{}, headers map[string]string) (int, map[string]interface{}, http.Header) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]interface{}{}
	if len(bytes.TrimSpace(raw)) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out, resp.Header
}

func postBin(t *testing.T, url string, data []byte) (int, map[string]interface{}) {
	t.Helper()
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]interface{}{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func shaHex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestUploadV1Cycle(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := bytes.Repeat([]byte("abcdef0123456789"), 30000) // ~480KB
	uid := "a1b2c3d4e5f60718"
	st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": uid, "name": "big.bin", "size": len(payload)}, nil)
	if st != 200 || int(j["offset"].(float64)) != 0 {
		t.Fatalf("init = %d %v", st, j)
	}
	// First chunk, then resume with same uuid.
	mid := len(payload) / 2
	if st, j := postBin(t, ts.URL+"/upload_chunk?id="+uid+"&offset=0", payload[:mid]); st != 200 {
		t.Fatalf("chunk1 = %d %v", st, j)
	} else if int(j["offset"].(float64)) != mid {
		t.Fatalf("offset after chunk1 = %v", j)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": uid, "name": "big.bin", "size": len(payload)}, nil)
	if st != 200 || int(j["offset"].(float64)) != mid {
		t.Fatalf("re-init = %d %v", st, j)
	}
	// Wrong offset -> 409 with correct offset.
	if st, j := postBin(t, ts.URL+"/upload_chunk?id="+uid+"&offset=0", payload[:10]); st != 409 {
		t.Fatalf("wrong offset = %d %v", st, j)
	} else if int(j["offset"].(float64)) != mid {
		t.Fatalf("409 offset = %v", j)
	}
	if st, j := postBin(t, ts.URL+"/upload_chunk?id="+uid+"&offset="+strconv.FormatInt(int64(mid), 10), payload[mid:]); st != 200 {
		t.Fatalf("chunk2 = %d %v", st, j)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete", map[string]interface{}{"id": uid}, nil)
	if st != 200 {
		t.Fatalf("complete = %d %v", st, j)
	}
	saved := j["saved"].([]interface{})[0].(string)
	data, err := os.ReadFile(filepath.Join(SharedDir, saved))
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("saved file mismatch: %v", err)
	}
	// Idempotent repeat: session gone but file matches.
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete",
		map[string]interface{}{"id": uid, "name": "big.bin", "size": len(payload)}, nil)
	if st != 200 {
		t.Fatalf("repeat complete = %d %v", st, j)
	}
}

func TestUploadV1Errors(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	st, _, _ := doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": "xx", "name": "a.bin", "size": 10}, nil)
	if st != 400 {
		t.Errorf("bad uuid = %d", st)
	}
	st, _, _ = doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": "bb11cc22dd334455", "name": ".hidden", "size": 10}, nil)
	if st != 400 {
		t.Errorf("dotfile = %d", st)
	}
	// Unlimited by default (MaxFileMB == 0): a very large size is accepted
	// at init time (only disk space / OS limits apply).
	st, _, _ = doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": "ee11cc22dd334488", "name": "huge.bin", "size": int64(50) * 1024 * 1024 * 1024}, nil)
	if st != 200 {
		t.Errorf("unlimited oversize = %d, want 200", st)
	}
	// When an admin sets a cap, it is enforced again.
	CfgMu.Lock()
	Cfg.MaxFileMB = 1
	CfgMu.Unlock()
	st, _, _ = doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": "ee11cc22dd334499", "name": "huge2.bin", "size": 2 * 1024 * 1024}, nil)
	if st != 413 {
		t.Errorf("capped oversize = %d, want 413", st)
	}
	CfgMu.Lock()
	Cfg.MaxFileMB = 0
	CfgMu.Unlock()
	// Incomplete complete -> 400.
	uid := "cc11cc22dd334466"
	doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": uid, "name": "half.bin", "size": 100}, nil)
	st, _, _ = doJSON(t, "POST", ts.URL+"/upload_complete", map[string]interface{}{"id": uid}, nil)
	if st != 400 {
		t.Errorf("incomplete complete = %d", st)
	}
	st, _, _ = doJSON(t, "GET", ts.URL+"/upload_status?id=noid0000", nil, nil)
	if st != 404 {
		t.Errorf("unknown status = %d", st)
	}
}

func pieceHashes(payload []byte, plen int) []string {
	out := []string{}
	for i := 0; i < len(payload); i += plen {
		end := i + plen
		if end > len(payload) {
			end = len(payload)
		}
		out = append(out, shaHex(payload[i:end]))
	}
	return out
}

func TestUploadV2Cycle(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := bytes.Repeat([]byte("0123456789abcdef0123456789abcdef"), 70000) // ~2.2MB
	plen := 1024 * 1024
	hashes := pieceHashes(payload, plen)
	n := len(hashes)
	uid := "aabbccddeeff00112233445566778899"
	st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init", map[string]interface{}{
		"uuid": uid, "name": "v2.bin", "size": len(payload),
		"piece_len": plen, "hashes": hashes}, nil)
	if st != 200 {
		t.Fatalf("init = %d %v", st, j)
	}
	if missing, ok := j["missing"].([]interface{}); !ok || len(missing) != n {
		t.Fatalf("missing = %v", j)
	}
	// Out-of-order parallel-style pieces (sequential here, any order).
	order := []int{n - 1, 0}
	for i := 1; i < n-1; i++ {
		order = append(order, i)
	}
	for _, idx := range order {
		s, e := idx*plen, (idx+1)*plen
		if e > len(payload) {
			e = len(payload)
		}
		st, j := postBin(t, ts.URL+"/upload_piece?id="+uid+"&index="+strconv.FormatInt(int64(idx), 10)+"&hash="+hashes[idx],
			payload[s:e])
		if st != 200 {
			t.Fatalf("piece %d = %d %v", idx, st, j)
		}
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/upload_status?id="+uid, nil, nil)
	if st != 200 || len(j["missing"].([]interface{})) != 0 {
		t.Fatalf("status = %d %v", st, j)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete", map[string]interface{}{"id": uid}, nil)
	if st != 200 {
		t.Fatalf("complete = %d %v", st, j)
	}
	saved := j["saved"].([]interface{})[0].(string)
	data, err := os.ReadFile(filepath.Join(SharedDir, saved))
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("saved file mismatch: %v", err)
	}
}

func TestUploadV2BadPieceHeals(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := bytes.Repeat([]byte("Z"), 900000)
	plen := 300000
	hashes := pieceHashes(payload, plen)
	uid := "bb22cc33dd44556677889900aabbccdd"
	doJSON(t, "POST", ts.URL+"/upload_init", map[string]interface{}{
		"uuid": uid, "name": "heal.bin", "size": len(payload),
		"piece_len": plen, "hashes": hashes}, nil)
	// Corrupt piece 0 but claim the right hash -> 422 with bad list.
	bad := bytes.Repeat([]byte("Q"), plen)
	st, j := postBin(t, ts.URL+"/upload_piece?id="+uid+"&index=0&hash="+hashes[0], bad)
	if st != 422 {
		t.Fatalf("corrupt piece = %d %v", st, j)
	}
	if badList, ok := j["bad"].([]interface{}); !ok || len(badList) != 1 {
		t.Fatalf("bad list = %v", j)
	}
	// Resend correct piece 0 + rest -> completes.
	for i := 0; i < len(hashes); i++ {
		s, e := i*plen, (i+1)*plen
		if e > len(payload) {
			e = len(payload)
		}
		if st, j := postBin(t, ts.URL+"/upload_piece?id="+uid+"&index="+strconv.FormatInt(int64(i), 10)+"&hash="+hashes[i],
			payload[s:e]); st != 200 {
			t.Fatalf("piece %d resend = %d %v", i, st, j)
		}
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete", map[string]interface{}{"id": uid}, nil)
	if st != 200 {
		t.Fatalf("complete after heal = %d %v", st, j)
	}
}

func TestUploadFindAndSessions(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := bytes.Repeat([]byte("F"), 600000)
	plen := 300000
	uid := "cc33dd44ee55667788990011aabbccdd"
	doJSON(t, "POST", ts.URL+"/upload_init", map[string]interface{}{
		"uuid": uid, "name": "resume.bin", "size": len(payload), "piece_len": plen}, nil)
	postBin(t, ts.URL+"/upload_piece?id="+uid+"&index=0&hash="+shaHex(payload[:plen]), payload[:plen])

	st, j, _ := doJSON(t, "POST", ts.URL+"/upload_find",
		map[string]interface{}{"name": "resume.bin", "size": len(payload)}, nil)
	if st != 200 || len(j["sessions"].([]interface{})) == 0 {
		t.Fatalf("find = %d %v", st, j)
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/upload_sessions", nil, nil)
	if st != 200 || len(j["sessions"].([]interface{})) == 0 {
		t.Fatalf("sessions = %d %v", st, j)
	}
	// Full-hash dedup: finish upload, then find by full hash returns completed.
	for i := 1; i < 2; i++ {
		s, e := i*plen, (i+1)*plen
		postBin(t, ts.URL+"/upload_piece?id="+uid+"&index="+strconv.FormatInt(int64(i), 10)+"&hash="+shaHex(payload[s:e]), payload[s:e])
	}
	doJSON(t, "POST", ts.URL+"/upload_complete", map[string]interface{}{"id": uid}, nil)
	full := shaHex(payload)
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_find",
		map[string]interface{}{"name": "resume.bin", "size": len(payload), "full_hash": full}, nil)
	if st != 200 || j["completed"] == nil {
		t.Fatalf("dedup find = %d %v", st, j)
	}
}

func TestDeleteAndMultipart(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	if err := os.WriteFile(filepath.Join(SharedDir, "gone.txt"), []byte("bye"), 0644); err != nil {
		t.Fatal(err)
	}
	st, _, _ := doJSON(t, "POST", ts.URL+"/delete", map[string]interface{}{"file": "gone.txt"}, nil)
	if st != 200 {
		t.Fatalf("delete = %d", st)
	}
	st, _, _ = doJSON(t, "POST", ts.URL+"/delete", map[string]interface{}{"file": "gone.txt"}, nil)
	if st != 404 {
		t.Fatalf("re-delete = %d", st)
	}
	// Legacy multipart upload.
	boundary := "TESTBOUNDARY1234"
	var buf bytes.Buffer
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"mp.txt\"\r\n")
	buf.WriteString("Content-Type: text/plain\r\n\r\n")
	buf.WriteString("hello-multipart")
	buf.WriteString("\r\n--" + boundary + "--\r\n")
	req, _ := http.NewRequest("POST", ts.URL+"/upload", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.ContentLength = int64(buf.Len())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("multipart = %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(SharedDir, "mp.txt")); err != nil {
		t.Fatalf("multipart file missing: %v", err)
	}
}

func TestDownloadRange(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	content := bytes.Repeat([]byte("0123456789"), 1000)
	if err := os.WriteFile(filepath.Join(SharedDir, "r.bin"), content, 0644); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/download?file=r.bin", nil)
	req.Header.Set("Range", "bytes=10-19")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 206 || !bytes.Equal(body, content[10:20]) {
		t.Fatalf("range = %d len=%d", resp.StatusCode, len(body))
	}
	if cr := resp.Header.Get("Content-Range"); cr != "bytes 10-19/10000" {
		t.Fatalf("content-range = %q", cr)
	}
	// Full download + hash endpoint.
	resp2, err := http.Get(ts.URL + "/download?file=r.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	full, _ := io.ReadAll(resp2.Body)
	if !bytes.Equal(full, content) {
		t.Fatal("full download mismatch")
	}
	st, j, _ := doJSON(t, "GET", ts.URL+"/file_hash?file=r.bin", nil, nil)
	if st != 200 || j["sha256"] != shaHex(content) {
		t.Fatalf("file_hash = %d %v", st, j)
	}
	st, _, _ = doJSON(t, "GET", ts.URL+"/download?file=..%2Fsecret", nil, nil)
	if st != 400 && st != 404 {
		t.Fatalf("traversal = %d", st)
	}
	// Unsatisfiable ranges -> 416 with Content-Range */size, not 200.
	for _, rh := range []string{"bytes=99999-100000", "bytes=999-1", "garbage-range"} {
		req, _ := http.NewRequest("GET", ts.URL+"/download?file=r.bin", nil)
		req.Header.Set("Range", rh)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 416 {
			t.Fatalf("range %q = %d, want 416", rh, resp.StatusCode)
		}
		if cr := resp.Header.Get("Content-Range"); cr != "bytes */10000" {
			t.Fatalf("range %q content-range = %q, want bytes */10000", rh, cr)
		}
	}
	// GET carries the same ETag as HEAD, plus Last-Modified.
	headResp, err := http.Head(ts.URL + "/download?file=r.bin")
	if err != nil {
		t.Fatal(err)
	}
	headResp.Body.Close()
	headETag := headResp.Header.Get("ETag")
	if headETag == "" {
		t.Fatal("HEAD missing ETag")
	}
	getResp, err := http.Get(ts.URL + "/download?file=r.bin")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if getResp.Header.Get("ETag") != headETag {
		t.Fatalf("GET ETag = %q, HEAD ETag = %q", getResp.Header.Get("ETag"), headETag)
	}
	if getResp.Header.Get("Last-Modified") == "" {
		t.Fatal("GET missing Last-Modified")
	}
	req206, _ := http.NewRequest("GET", ts.URL+"/download?file=r.bin", nil)
	req206.Header.Set("Range", "bytes=0-9")
	resp206, err := http.DefaultClient.Do(req206)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp206.Body)
	resp206.Body.Close()
	if resp206.Header.Get("ETag") != headETag {
		t.Fatalf("206 ETag = %q, HEAD ETag = %q", resp206.Header.Get("ETag"), headETag)
	}
	if resp206.Header.Get("Last-Modified") == "" {
		t.Fatal("206 missing Last-Modified")
	}
}

func TestUploadV1ShortBody(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	uid := "d4d4d4d4d4d4d4d4"
	payload := bytes.Repeat([]byte("S"), 1000)
	if st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init",
		map[string]interface{}{"uuid": uid, "name": "short.bin", "size": len(payload)}, nil); st != 200 {
		t.Fatalf("init = %d %v", st, j)
	}
	// Declare 1000 bytes but deliver only 400: 400, not a silent 200.
	req := httptest.NewRequest("POST", "/upload_chunk?id="+uid+"&offset=0", bytes.NewReader(payload[:400]))
	req.ContentLength = int64(len(payload))
	rec := httptest.NewRecorder()
	Route(rec, req)
	if rec.Code != 400 {
		t.Fatalf("short body = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestUploadV2LegacyRouteRejected(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := bytes.Repeat([]byte("V"), 600000)
	plen := 300000
	hashes := pieceHashes(payload, plen)
	uid := "e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5"
	if st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init", map[string]interface{}{
		"uuid": uid, "name": "v2leg.bin", "size": len(payload),
		"piece_len": plen, "hashes": hashes}, nil); st != 200 {
		t.Fatalf("init = %d %v", st, j)
	}
	// Verified V2 sessions must use /upload_piece, not legacy /upload_chunk.
	if st, _ := postBin(t, ts.URL+"/upload_chunk?id="+uid+"&offset=0", payload[:plen]); st != 400 {
		t.Fatalf("legacy chunk on V2 = %d, want 400", st)
	}
	// NoVerify (turbo) sessions stay accepted on the legacy route.
	uid2 := "f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6"
	if st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init", map[string]interface{}{
		"uuid": uid2, "name": "turboleg.bin", "size": len(payload),
		"piece_len": plen, "noverify": true}, nil); st != 200 {
		t.Fatalf("turbo init = %d %v", st, j)
	}
	if st, _ := postBin(t, ts.URL+"/upload_chunk?id="+uid2+"&offset=0", payload[:plen]); st != 200 {
		t.Fatalf("legacy chunk on turbo = %d, want 200", st)
	}
}

func TestUploadReinitPieceLenMismatch(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := bytes.Repeat([]byte("R"), 600000)
	plenA := 300000
	hashesA := pieceHashes(payload, plenA)
	uid := "a7a7a7a7a7a7a7a7a7a7a7a7a7a7a7a7"
	if st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init", map[string]interface{}{
		"uuid": uid, "name": "repie.bin", "size": len(payload),
		"piece_len": plenA, "hashes": hashesA}, nil); st != 200 {
		t.Fatalf("init A = %d %v", st, j)
	}
	if st, _ := postBin(t, ts.URL+"/upload_piece?id="+uid+"&index=0&hash="+hashesA[0], payload[:plenA]); st != 200 {
		t.Fatalf("piece 0 = %d", st)
	}
	// Re-init same uid/name/size with a different piece_len must rebuild
	// (fresh bitmap), never reuse the old PieceLen.
	plenB := 262144
	hashesB := pieceHashes(payload, plenB)
	ifaceB := make([]interface{}, len(hashesB))
	for i, h := range hashesB {
		ifaceB[i] = h
	}
	st, j, _ := doJSON(t, "POST", ts.URL+"/upload_init", map[string]interface{}{
		"uuid": uid, "name": "repie.bin", "size": len(payload),
		"piece_len": plenB, "hashes": ifaceB}, nil)
	if st != 200 {
		t.Fatalf("re-init B = %d %v", st, j)
	}
	if int(j["piece_len"].(float64)) != plenB {
		t.Fatalf("piece_len = %v, want %d", j["piece_len"], plenB)
	}
	if len(j["missing"].([]interface{})) != len(hashesB) {
		t.Fatalf("missing = %v, want all %d", j["missing"], len(hashesB))
	}
}

func TestUploadInitPieceCap(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	// needN beyond maxSessionPieces must be rejected with 413.
	huge := int64(200001) * int64(256*1024)
	st, _, _ := doJSON(t, "POST", ts.URL+"/upload_init", map[string]interface{}{
		"uuid": "b8b8b8b8b8b8b8b8b8b8b8b8b8b8b8b8", "name": "giant.bin",
		"size": huge, "piece_len": 256 * 1024}, nil)
	if st != 413 {
		t.Fatalf("giant init = %d, want 413", st)
	}
}
