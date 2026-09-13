package beamcore

import (
	"net/http"
	"os"
	"strconv"
)

func handleDelete(w http.ResponseWriter, r *http.Request) {
	// NOTE (kept open by owner choice): any LAN device may upload/download/
	// delete — Wi-Fi password is the only gate. Every delete is logged with IP.
	data := readJSONBody(r, jsonBigMax)
	if rawDir, ok := data["dir"]; ok {
		dirStr, _ := rawDir.(string)
		rel := safeRelPath(dirStr)
		if rel == "" {
			fail(w, r, 400, "dir_bad")
			return
		}
		_, fpath, err := sharedDirStat(rel)
		if err != nil {
			fail(w, r, 404, "dir_gone")
			return
		}
		entries, _ := walkSharedDir(rel, maxZipFiles+1)
		n := len(entries)
		if err := os.RemoveAll(fpath); err != nil {
			fail(w, r, 409, "delete_busy")
			return
		}
		invalidateFileHashPrefix(rel + "/")
		pruneEmptyParents(rel)
		writeLog(clientIP(r), "delete_dir", rel+" ("+strconv.FormatInt(int64(n), 10)+" files)")
		sendJSON(w, r, 200, map[string]interface{}{"ok": true, "removed": n})
		return
	}
	name := safeRelPath(jStr(data, "file"))
	if name == "" {
		fail(w, r, 400, "delete_bad_name")
		return
	}
	_, fpath, err := sharedFileStat(name)
	if err != nil {
		fail(w, r, 404, "delete_missing")
		return
	}
	if err := os.Remove(fpath); err != nil {
		fail(w, r, 409, "delete_busy")
		return
	}
	invalidateFileHash(name)
	if dir, _ := splitParent(name); dir != "" {
		pruneEmptyParents(name)
	}
	writeLog(clientIP(r), "delete", name)
	sendJSON(w, r, 200, map[string]interface{}{"ok": true})
}
