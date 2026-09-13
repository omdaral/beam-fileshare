package beamcore

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func handleUploadStatus(w http.ResponseWriter, r *http.Request) {
	uid := strings.ToLower(r.URL.Query().Get("id"))
	m := loadMeta(uid)
	if m == nil {
		fail(w, r, 404, "up_no_session")
		return
	}
	if m.V == 2 {
		sendJSON(w, r, 200, map[string]interface{}{
			"id": uid, "offset": contiguousOffset(m),
			"received": receivedRangesBytes(m), "missing": missingPieces(m),
			"piece_len": m.PieceLen, "size": m.Size, "name": m.Name,
			"noverify": m.NoVerify,
		})
		return
	}
	sendJSON(w, r, 200, map[string]interface{}{
		"id": uid, "offset": sessReceived(uid), "size": m.Size, "name": m.Name,
	})
}
func handleUploadSessions(w http.ResponseWriter, r *http.Request) {
	type sessJSON struct {
		ID         string `json:"id"`
		V          int    `json:"v"`
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		Received   int64  `json:"received"`
		Missing    int    `json:"missing"`
		UpdatedAgo int64  `json:"updated_ago"`
		NoVerify   bool   `json:"noverify,omitempty"`
	}
	now := nowUnix()
	out := []sessJSON{}
	for _, s := range iterSessions() {
		m := s.Meta
		var received int64
		missing := -1
		if m.V == 2 {
			received = receivedRangesBytes(m)
			missing = len(missingPieces(m))
		} else {
			st, err := os.Stat(filepath.Join(sessDir(s.UID), "data.part"))
			if err != nil {
				continue
			}
			received = st.Size()
		}
		if m.Size > 0 && received >= m.Size {
			continue
		}
		out = append(out, sessJSON{ID: s.UID, V: m.V, Name: m.Name,
			Size: m.Size, Received: received, Missing: missing,
			UpdatedAgo: int64(now - s.Mtime), NoVerify: m.NoVerify})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Received > out[j].Received })
	if len(out) > 50 {
		out = out[:50]
	}
	sendJSON(w, r, 200, map[string]interface{}{"sessions": out})
}
