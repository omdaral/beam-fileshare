package beamcore

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func handleUploadFind(w http.ResponseWriter, r *http.Request) {
	data := readJSONBody(r, jsonBigMax)
	name := safeRelPath(jStr(data, "name"))
	size := jInt(data, "size", 0)
	if name == "" || size <= 0 {
		fail(w, r, 400, "up_find_invalid")
		return
	}
	if wantFull := strings.ToLower(jStr(data, "full_hash")); wantFull != "" {
		// DoS-hardened: hash ONLY the requested name (never walk+hash up to
		// 2000 files). Symlink-safe stat. Size mismatch short-circuits
		// before any hashing. cachedFileHash reuses size/mtime cache.
		if !hexRe.MatchString(wantFull) || len(wantFull) != 64 {
			fail(w, r, 400, "up_bad_hash")
			return
		}
		if st, fpath, err := sharedFileStat(name); err == nil {
			if st.Size() == size {
				if d, err := cachedFileHash(name, fpath, st.Size(),
					float64(st.ModTime().UnixNano())/1e9); err == nil && d == wantFull {
					sendJSON(w, r, 200, map[string]interface{}{"ok": true, "completed": name})
					return
				}
			}
		}
	}
	type candJSON struct {
		ID         string   `json:"id"`
		V          int      `json:"v"`
		Received   int64    `json:"received"`
		Missing    []int    `json:"missing,omitempty"`
		Offset     int64    `json:"offset,omitempty"`
		PieceLen   int64    `json:"piece_len,omitempty"`
		Hashes     []string `json:"hashes,omitempty"`
		UpdatedAgo int64    `json:"updated_ago"`
	}
	now := nowUnix()
	cands := []candJSON{}
	for _, s := range iterSessions() {
		m := s.Meta
		if m.Name != name || m.Size != size {
			continue
		}
		if m.V == 2 {
			hashes := []string{}
			for _, h := range m.Hashes {
				if h == nil {
					hashes = append(hashes, "")
				} else {
					hashes = append(hashes, *h)
				}
			}
			cands = append(cands, candJSON{ID: s.UID, V: 2,
				Received: receivedRangesBytes(m), Missing: missingPieces(m),
				PieceLen: m.PieceLen, Hashes: hashes,
				UpdatedAgo: int64(now - s.Mtime)})
		} else {
			st, err := os.Stat(filepath.Join(sessDir(s.UID), "data.part"))
			if err != nil {
				continue
			}
			if st.Size() <= 0 {
				continue
			}
			cands = append(cands, candJSON{ID: s.UID, V: 1,
				Received: st.Size(), Offset: st.Size(),
				UpdatedAgo: int64(now - s.Mtime)})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].Received > cands[j].Received })
	if len(cands) > 5 {
		cands = cands[:5]
	}
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "sessions": cands})
}
