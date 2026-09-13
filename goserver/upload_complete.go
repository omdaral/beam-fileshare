package beamcore

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func handleUploadComplete(w http.ResponseWriter, r *http.Request) {
	data := readJSONBody(r, jsonBigMax)
	uid := strings.ToLower(jStr(data, "id"))
	m := loadMeta(uid)
	if m == nil {
		name := safeRelPath(jStr(data, "name"))
		size := jInt(data, "size", 0)
		if name != "" && size > 0 {
			if st, err := os.Stat(filepath.Join(SharedDir, name)); err == nil && st.Size() == size {
				sendJSON(w, r, 200, map[string]interface{}{"ok": true, "saved": []string{name}})
				return
			}
		}
		fail(w, r, 404, "up_session_gone")
		return
	}
	lk := sessLock(uid)
	lk.Lock()
	m = loadMeta(uid)
	if m == nil {
		lk.Unlock()
		fail(w, r, 404, "up_session_gone")
		return
	}
	var received int64
	if m.V == 2 {
		missing := missingPieces(m)
		if len(missing) > 0 {
			lk.Unlock()
			sendJSON(w, r, 400, map[string]interface{}{
				"error":   tr(reqLang(r), "up_incomplete_n", len(missing)),
				"missing": missing})
			return
		}
		received = m.Size
	} else {
		received = sessReceived(uid)
		if received != m.Size {
			lk.Unlock()
			sendJSON(w, r, 400, map[string]interface{}{
				"error": tr(reqLang(r), "up_incomplete_bytes", strconv.FormatInt(received, 10), strconv.FormatInt(m.Size, 10))})
			return
		}
	}
	if m.NoVerify && strings.TrimSpace(jStr(data, "full_hash")) == "" {
		lk.Unlock()
		sendJSON(w, r, 422, map[string]interface{}{"error": tr(reqLang(r), "up_fullhash_required")})
		return
	}
	if wantFull := strings.ToLower(jStr(data, "full_hash")); wantFull != "" {
		gotFull, err := fullSHA256(filepath.Join(sessDir(uid), "data.part"))
		if err != nil {
			lk.Unlock()
			fail(w, r, 500, "up_verify_fail")
			return
		}
		if gotFull != wantFull {
			lk.Unlock()
			sendJSON(w, r, 422, map[string]interface{}{"error": tr(reqLang(r), "up_hash_mismatch")})
			return
		}
	}
	secs := nowUnix() - m.Created
	if secs < 0.001 {
		secs = 0.001
	}
	name := m.Name
	uploadLock.Lock()
	dest := uniquePath(SharedDir, name)
	_ = os.MkdirAll(filepath.Dir(dest), 0755)
	rerr := os.Rename(filepath.Join(sessDir(uid), "data.part"), dest)
	uploadLock.Unlock()
	if rerr != nil {
		lk.Unlock()
		sendJSON(w, r, 500, map[string]interface{}{"error": tr(reqLang(r), "up_save_fail", name)})
		return
	}
	_ = os.RemoveAll(sessDir(uid))
	lk.Unlock()
	forgetSessLock(uid)
	saved := name
	if rel, err := filepath.Rel(SharedDir, dest); err == nil {
		saved = filepath.ToSlash(rel)
	}
	writeLog(clientIP(r), "upload",
		saved+" ("+strconv.FormatInt(received, 10)+"b, "+ftoa1(secs)+"s, "+fmtSpeed(received, secs)+")")
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "saved": []string{saved}})
	if jBool(data, "extract") || m.Extract {
		// Folder-zip: unpack in background AFTER the response. Transfer done.
		cip := clientIP(r)
		go extractZipAsync(dest, saved, cip)
	}
}
