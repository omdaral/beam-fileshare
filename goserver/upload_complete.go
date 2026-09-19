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
	// Guest-share sessions retain their bytes in the temp dir instead of
	// the legacy share-folder placement below.
	if shareID, ok := guestShareForSession(uid); ok {
		guestComplete(w, r, uid, shareID, data)
		return
	}
	m := loadMeta(uid)
	if m == nil {
		// Idempotent retry after a completed (cleaned-up) session: never
		// trust size alone — verify the saved bytes by hash. Resolved via
		// the symlink-safe stat (saved path), not a raw join.
		name := safeRelPath(jStr(data, "name"))
		size := jInt(data, "size", 0)
		if name != "" && size > 0 {
			if st, fpath, serr := sharedFileStat(name); serr == nil && st.Size() == size {
				wantFull := strings.ToLower(strings.TrimSpace(jStr(data, "full_hash")))
				mtime := float64(st.ModTime().UnixNano()) / 1e9
				if wantFull != "" {
					if len(wantFull) == 64 && hexRe.MatchString(wantFull) {
						if d, herr := cachedFileHash(name, fpath, st.Size(), mtime); herr == nil && d == wantFull {
							sendJSON(w, r, 200, map[string]interface{}{"ok": true, "saved": []string{name}})
							return
						}
					}
				} else if d, herr := cachedFileHash(name, fpath, st.Size(), mtime); herr == nil && d != "" {
					// Legacy retry without full_hash: still hash the file —
					// a size-match alone never suffices.
					sendJSON(w, r, 200, map[string]interface{}{"ok": true, "saved": []string{name}})
					return
				}
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
	// The full-file hash is a slow disk re-read: run it WITHOUT holding
	// sessLock, then re-acquire and re-validate (the session may have
	// completed or vanished concurrently while hashing).
	wantFull := strings.ToLower(jStr(data, "full_hash"))
	partPath := filepath.Join(sessDir(uid), "data.part")
	lk.Unlock()
	if wantFull != "" {
		gotFull, herr := fullSHA256(partPath)
		if herr != nil {
			fail(w, r, 500, "up_verify_fail")
			return
		}
		if gotFull != wantFull {
			sendJSON(w, r, 422, map[string]interface{}{"error": tr(reqLang(r), "up_hash_mismatch")})
			return
		}
	}
	lk.Lock()
	m = loadMeta(uid)
	if m == nil {
		lk.Unlock()
		fail(w, r, 404, "up_session_gone")
		return
	}
	if m.V == 2 {
		if missing := missingPieces(m); len(missing) > 0 {
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
	secs := nowUnix() - m.Created
	if secs < 0.001 {
		secs = 0.001
	}
	name := m.Name
	src := filepath.Join(sessDir(uid), "data.part")
	// Collision-safe placement: reserve each candidate under uploadLock
	// with O_EXCL so a raced name retries with a fresh unique name
	// instead of Rename silently overwriting an existing file.
	uploadLock.Lock()
	dest := ""
	var rerr error
	for tries := 0; tries < 10; tries++ {
		dest = uniquePath(SharedDir, name)
		_ = os.MkdirAll(filepath.Dir(dest), 0755)
		rsv, cerr := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if cerr != nil {
			rerr = cerr
			if os.IsExist(cerr) {
				continue
			}
			break
		}
		rsv.Close()
		_ = os.Remove(dest)
		rerr = os.Rename(src, dest)
		break
	}
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
	invalidateListCaches()
	writeLog(clientIP(r), "upload",
		saved+" ("+strconv.FormatInt(received, 10)+"b, "+ftoa1(secs)+"s, "+fmtSpeed(received, secs)+")")
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "saved": []string{saved}})
	if jBool(data, "extract") || m.Extract {
		// Folder-zip: unpack in background AFTER the response. Transfer done.
		cip := clientIP(r)
		go extractZipAsync(dest, saved, cip)
	}
}
