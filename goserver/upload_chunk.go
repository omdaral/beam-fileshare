package beamcore

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// streamBodyToFile copies exactly length bytes from r.Body to f (1MB chunks,
// capped by MaxBytesReader). Reports (written, clientGone, diskFail).
func streamBodyToFile(w http.ResponseWriter, r *http.Request, f *os.File, length int64) (int64, bool, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, length)
	defer r.Body.Close()
	var written int64
	buf := make([]byte, copyBufSize)
	for remaining := length; remaining > 0; {
		if r.Context().Err() != nil {
			return written, true, false
		}
		want := remaining
		if want > int64(len(buf)) {
			want = int64(len(buf))
		}
		nr, rerr := r.Body.Read(buf[:want])
		if nr > 0 {
			nw, werr := f.Write(buf[:nr])
			written += int64(nw)
			remaining -= int64(nw)
			if werr != nil {
				return written, false, true
			}
		}
		if rerr != nil {
			break
		}
	}
	return written, false, false
}

func handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	uid := strings.ToLower(q.Get("id"))
	offset, _ := strconv.ParseInt(q.Get("offset"), 10, 64)
	if q.Get("offset") == "" {
		offset = -1
	}
	length := r.ContentLength
	m := loadMeta(uid)
	if m == nil {
		fail(w, r, 404, "up_session_gone")
		return
	}
	if r.ContentLength < 0 {
		sendJSON(w, r, 411, map[string]interface{}{
			"error": tr(reqLang(r), "up_chunk_no_len"), "offset": sessReceived(uid)})
		return
	}
	if m.V == 2 {
		writeV2(w, r, uid, offset, length, nil, "", false)
		return
	}
	// V1: serialize chunks per session (parallel same-offset chunks used to
	// race past the offset check and corrupt data.part via O_APPEND).
	lk := sessLock(uid)
	lk.Lock()
	defer lk.Unlock()
	received := sessReceived(uid)
	if offset != received {
		sendJSON(w, r, 409, map[string]interface{}{"error": "offset", "offset": received})
		return
	}
	if received+length > m.Size {
		fail(w, r, 413, "up_chunk_over")
		return
	}
	// Cap body so a lying Content-Length can't OOM/Fill disk.
	f, err := os.OpenFile(filepath.Join(sessDir(uid), "data.part"),
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fail(w, r, 500, "up_write_fail")
		return
	}
	defer f.Close()
	if r.Body != nil {
		if _, gone, diskFail := streamBodyToFile(w, r, f, length); gone || diskFail {
			fail(w, r, 500, "up_write_fail")
			return
		}
	}
	touchSessDir(uid)
	sendJSON(w, r, 200, map[string]interface{}{"id": uid, "offset": sessReceived(uid)})
}

// resolvePiece validates index/length/claim for a v2 piece request.
// Returns piece start, normalized claim hash, hasClaim, errKey ("" when ok).
func resolvePiece(m *sessionMeta, index int, length int64, claimHash string) (int64, string, bool, string) {
	if index < 0 || index >= pieceCount(m) {
		return 0, "", false, "up_bad_index"
	}
	ps, pe := pieceRange(m, index)
	if length != pe-ps {
		return 0, "", false, "up_piece_len"
	}
	hasClaim := claimHash != ""
	if m.NoVerify && !hasClaim {
		return ps, "", false, "" // turbo lane: verified at complete
	}
	var known *string
	if index < len(m.Hashes) {
		known = m.Hashes[index]
	}
	if known == nil && (len(claimHash) != 64 || !hexRe.MatchString(claimHash)) {
		if len(claimHash) != 64 {
			return 0, "", false, "up_hash_required"
		}
		return 0, "", false, "up_bad_hash"
	}
	return ps, strings.ToLower(claimHash), hasClaim, ""
}

func handleUploadPiece(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	uid := strings.ToLower(q.Get("id"))
	index, _ := strconv.Atoi(q.Get("index"))
	if q.Get("index") == "" {
		index = -1
	}
	length := r.ContentLength
	m := loadMeta(uid)
	if m == nil || m.V != 2 {
		fail(w, r, 404, "up_no_piece_session")
		return
	}
	ps, claim, hasClaim, errKey := resolvePiece(m, index, length, q.Get("hash"))
	if errKey != "" {
		if errKey == "up_piece_len" {
			ps, pe := pieceRange(m, index)
			sendJSON(w, r, 400, map[string]interface{}{
				"error": tr(reqLang(r), errKey, pe-ps)})
			return
		}
		if errKey == "up_hash_required" {
			sendJSON(w, r, 400, map[string]interface{}{"error": tr(reqLang(r), errKey)})
			return
		}
		fail(w, r, 400, errKey)
		return
	}
	writeV2(w, r, uid, ps, length, &index, claim, hasClaim)
}

// mergeClaimHash records a client-claimed piece hash (409 on conflict).
// Returns errKey ("" when ok).
func mergeClaimHash(m *sessionMeta, claimIdx *int, claimHash string) string {
	ci := *claimIdx
	if ci < 0 || ci >= pieceCount(m) {
		return "up_bad_index"
	}
	for len(m.Hashes) <= ci {
		m.Hashes = append(m.Hashes, nil)
	}
	if m.Hashes[ci] != nil && *m.Hashes[ci] != claimHash {
		return "up_conflict"
	}
	m.Hashes[ci] = &claimHash
	return ""
}

// verifySpan checks newly covered pieces, zeroing corrupt ones.
// Returns bad piece indexes.
func verifySpan(uid string, m *sessionMeta, offset, length int64) []int {
	n := pieceCount(m)
	first := offset / m.PieceLen
	if first < 0 {
		first = 0
	}
	last := (offset + length - 1) / m.PieceLen
	bad := []int{}
	bm := getBitmap(m)
	for i := int(first); i <= int(last) && i < n; i++ {
		if i < len(bm) && bm[i] == '1' {
			continue
		}
		ps, pe := pieceRange(m, i)
		if !rangeCover(m, ps, pe) {
			continue
		}
		if m.NoVerify {
			markPiece(uid, m, i, true)
			continue
		}
		if verifyPiece(uid, m, i) {
			markPiece(uid, m, i, true)
		} else {
			zeroPiece(uid, m, i)
			rangeRemove(m, ps, pe)
			saveMeta(uid, m)
			bad = append(bad, i)
		}
	}
	return bad
}

// writeV2 writes v2 pieces at any offset with immediate hash verification.
// Streaming: body is copied directly to disk in 1MB chunks (no
// make([]byte,length) up to 64MB), so parallel pieces can't OOM.
func writeV2(w http.ResponseWriter, r *http.Request, uid string, offset, length int64,
	claimIdx *int, claimHash string, hasClaim bool) {
	if offset < 0 || length <= 0 {
		fail(w, r, 400, "up_bad_pos")
		return
	}
	if length > int64(chunkMaxV2) {
		fail(w, r, 413, "up_piece_too_big")
		return
	}
	if r.ContentLength >= 0 && r.ContentLength != length {
		sendJSON(w, r, 400, map[string]interface{}{
			"error": tr(reqLang(r), "up_short_piece"), "offset": offset})
		return
	}
	if hasClaim {
		if claimIdx == nil || len(claimHash) != 64 || !hexRe.MatchString(claimHash) {
			fail(w, r, 400, "up_bad_hash")
			return
		}
	}
	lk := sessLock(uid)
	lk.Lock()
	defer lk.Unlock()
	m := loadMeta(uid)
	if m == nil || m.V != 2 {
		fail(w, r, 404, "up_session_gone")
		return
	}
	if offset+length > m.Size {
		fail(w, r, 413, "up_chunk_over")
		return
	}
	if hasClaim {
		if errKey := mergeClaimHash(m, claimIdx, claimHash); errKey != "" {
			if errKey == "up_conflict" {
				sendJSON(w, r, 409, map[string]interface{}{"error": tr(reqLang(r), errKey)})
				return
			}
			fail(w, r, 400, errKey)
			return
		}
	}
	f, err := os.OpenFile(filepath.Join(sessDir(uid), "data.part"), os.O_RDWR, 0644)
	if err != nil {
		fail(w, r, 500, "up_write_fail")
		return
	}
	if _, err := f.Seek(offset, 0); err != nil {
		f.Close()
		fail(w, r, 500, "up_write_fail")
		return
	}
	// Stream request body straight to the file (capped at length).
	var written int64
	if r.Body != nil {
		var gone, diskFail bool
		written, gone, diskFail = streamBodyToFile(w, r, f, length)
		if gone {
			f.Close()
			sendJSON(w, r, 499, map[string]interface{}{"error": tr(reqLang(r), "up_write_fail")})
			return
		}
		if diskFail {
			f.Close()
			fail(w, r, 500, "up_write_fail")
			return
		}
	}
	f.Close()
	if written != length {
		sendJSON(w, r, 400, map[string]interface{}{
			"error": tr(reqLang(r), "up_short_piece"), "offset": offset})
		return
	}
	rangeAdd(m, offset, offset+length)
	bad := verifySpan(uid, m, offset, length)
	saveMeta(uid, m)
	touchSessDir(uid)
	received := receivedRangesBytes(m)
	missing := missingPieces(m)
	resp := map[string]interface{}{"id": uid, "received": received,
		"missing": missing, "missing_count": len(missing)}
	if len(bad) > 0 {
		resp["bad"] = bad
		resp["error"] = tr(reqLang(r), "up_bad_pieces")
		sendJSON(w, r, 422, resp)
		return
	}
	sendJSON(w, r, 200, resp)
}
