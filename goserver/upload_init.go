package beamcore

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// initParams is the validated form of an /upload_init body.
type initParams struct {
	uid      string
	name     string
	size     int64
	pieceLen int64
	needN    int
	hashes   []*string
	wantV2   bool
	noVerify bool
	extract  bool
}

// parseInitParams validates uuid/name/size/piece_len/hashes.
// Returns errKey ("" when ok) for fail().
func parseInitParams(data map[string]interface{}) (*initParams, string) {
	p := &initParams{
		uid:  strings.ToLower(jStr(data, "uuid")),
		name: safeRelPath(jStr(data, "name")),
		size: jInt(data, "size", 0),
	}
	if !uuidRe.MatchString(p.uid) {
		return nil, "up_bad_session"
	}
	if p.name == "" || strings.HasPrefix(p.name, ".") {
		return nil, "up_bad_name"
	}
	if p.size <= 0 || p.size > maxBytes() {
		return nil, "up_too_big"
	}
	p.wantV2 = jHas(data, "piece_len")
	p.noVerify = jBool(data, "noverify")
	p.extract = jBool(data, "extract")
	if p.noVerify {
		p.wantV2 = true
	}
	if raw, ok := data["hashes"]; ok {
		if arr, ok := raw.([]interface{}); ok && len(arr) > 0 {
			p.wantV2 = true
		}
	}
	p.pieceLen = int64(pieceDefault)
	if !p.wantV2 {
		return p, ""
	}
	p.pieceLen = jInt(data, "piece_len", int64(pieceDefault))
	if p.pieceLen < int64(pieceMin) || p.pieceLen > int64(pieceMax) {
		return nil, "up_bad_piece_len"
	}
	p.needN = int((p.size + p.pieceLen - 1) / p.pieceLen)
	if p.needN > maxSessionPieces {
		return nil, "up_too_big"
	}
	raw, ok := data["hashes"]
	if !ok {
		return p, ""
	}
	arr, ok := raw.([]interface{})
	if !ok || len(arr) == 0 {
		return p, ""
	}
	if len(arr) != p.needN {
		return nil, "up_hash_count"
	}
	p.hashes = make([]*string, p.needN)
	for i, h := range arr {
		hs, ok := h.(string)
		if !ok || len(hs) != 64 {
			return nil, "up_bad_hash"
		}
		lower := strings.ToLower(hs)
		p.hashes[i] = &lower
	}
	return p, ""
}

// newSessionMeta builds a fresh v2 meta + sparse data file.
func newSessionMeta(uid string, p *initParams) *sessionMeta {
	m := &sessionMeta{V: 2, Name: p.name, Size: p.size,
		PieceLen: p.pieceLen, Hashes: p.hashes, NoVerify: p.noVerify,
		Extract: p.extract,
		Bitmap:  strings.Repeat("0", p.needN), Ranges: [][2]int64{},
		Created: nowUnix()}
	saveMeta(uid, m)
	if f, err := os.OpenFile(filepath.Join(sessDir(uid), "data.part"),
		os.O_WRONLY|os.O_CREATE, 0644); err == nil {
		_ = f.Truncate(p.size)
		f.Close()
	}
	return m
}

// resumeV2 merges newly offered hashes into a live v2 session.
// Returns (responded, conflict): conflict rebuilds the session from scratch.
func resumeV2(uid string, m *sessionMeta, p *initParams) (conflict bool) {
	// PieceLen is part of the resume identity: re-init with a different
	// piece size must rebuild, never silently reuse the old PieceLen
	// with a resized bitmap.
	if m.PieceLen != 0 && m.PieceLen != p.pieceLen {
		return true
	}
	stored := m.Hashes
	if len(stored) != p.needN {
		stored = make([]*string, p.needN)
	}
	for i := 0; i < p.needN; i++ {
		if stored[i] != nil && p.hashes[i] != nil && *stored[i] != *p.hashes[i] {
			return true
		}
	}
	merged := make([]*string, p.needN)
	changed := false
	for i := 0; i < p.needN; i++ {
		merged[i] = stored[i]
		if merged[i] == nil {
			merged[i] = p.hashes[i]
			if p.hashes[i] != nil {
				changed = true
			}
		}
	}
	if changed {
		m.Hashes = merged
		saveMeta(uid, m)
	}
	return false
}

func v2Status(uid string, m *sessionMeta) map[string]interface{} {
	return map[string]interface{}{
		"id": uid, "offset": contiguousOffset(m),
		"received": receivedRangesBytes(m), "missing": missingPieces(m),
		"piece_len": m.PieceLen}
}

func handleUploadInit(w http.ResponseWriter, r *http.Request) {
	p, errKey := parseInitParams(readJSONBody(r, jsonBigMax))
	if errKey != "" {
		if errKey == "up_too_big" {
			sendJSON(w, r, 413, map[string]interface{}{"error": tr(reqLang(r), errKey)})
			return
		}
		fail(w, r, 400, errKey)
		return
	}
	SweepUploads()

	if m := loadMeta(p.uid); m != nil && m.Name == p.name && m.Size == p.size &&
		m.NoVerify == p.noVerify && (m.V != 2 || !p.wantV2 || m.PieceLen == p.pieceLen) {
		if p.extract && !m.Extract {
			m.Extract = true
			saveMeta(p.uid, m)
		}
		if m.V == 2 {
			if p.hashes != nil {
				lk := sessLock(p.uid)
				lk.Lock()
				if resumeV2(p.uid, m, p) {
					m = newSessionMeta(p.uid, p)
					lk.Unlock()
					sendJSON(w, r, 200, map[string]interface{}{
						"id": p.uid, "offset": 0, "received": 0,
						"missing": seqInts(p.needN), "piece_len": p.pieceLen})
					return
				}
				lk.Unlock()
			}
			sendJSON(w, r, 200, v2Status(p.uid, m))
			return
		}
		sendJSON(w, r, 200, map[string]interface{}{"id": p.uid, "offset": sessReceived(p.uid)})
		return
	}

	sdir := sessDir(p.uid)
	if sdir == "" {
		fail(w, r, 400, "up_bad_session")
		return
	}
	_ = os.MkdirAll(sdir, 0755)
	if p.wantV2 {
		lk := sessLock(p.uid)
		lk.Lock()
		newSessionMeta(p.uid, p)
		lk.Unlock()
		writeLog(clientIP(r), "upload_init",
			p.name+" ("+strconv.FormatInt(p.size, 10)+"b, v2"+openClosed(p.hashes)+")")
		sendJSON(w, r, 200, map[string]interface{}{
			"id": p.uid, "offset": 0, "received": 0,
			"missing": seqInts(p.needN), "piece_len": p.pieceLen})
		return
	}
	saveMeta(p.uid, &sessionMeta{V: 1, Name: p.name, Size: p.size, Created: nowUnix()})
	if f, err := os.OpenFile(filepath.Join(sdir, "data.part"),
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644); err == nil {
		f.Close()
	}
	writeLog(clientIP(r), "upload_init", p.name+" ("+strconv.FormatInt(p.size, 10)+"b)")
	sendJSON(w, r, 200, map[string]interface{}{"id": p.uid, "offset": 0})
}

func openClosed(hashes []*string) string {
	if hashes != nil {
		return " closed"
	}
	return " open"
}

func seqInts(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
