package beamcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type sessionMeta struct {
	V        int        `json:"v"`
	Name     string     `json:"name"`
	Size     int64      `json:"size"`
	PieceLen int64      `json:"piece_len,omitempty"`
	Hashes   []*string  `json:"hashes"`
	Bitmap   string     `json:"bitmap,omitempty"`
	Ranges   [][2]int64 `json:"ranges,omitempty"`
	Created  float64    `json:"created"`
	NoVerify bool       `json:"noverify,omitempty"`
	Extract  bool       `json:"extract,omitempty"`
}
func uploadsDir() string {
	d := filepath.Join(SharedDir, uploadsDirname)
	_ = os.MkdirAll(d, 0755)
	return d
}
func sessDir(uid string) string {
	if !uuidRe.MatchString(uid) {
		return ""
	}
	return filepath.Join(uploadsDir(), strings.ToLower(uid))
}
func sessLock(uid string) *sync.Mutex {
	sessMu.Lock()
	defer sessMu.Unlock()
	lk, ok := sessLocks[uid]
	if !ok {
		lk = &sync.Mutex{}
		sessLocks[uid] = lk
	}
	return lk
}
func forgetSessLock(uid string) {
	sessMu.Lock()
	defer sessMu.Unlock()
	delete(sessLocks, uid)
}
func loadMeta(uid string) *sessionMeta {
	sdir := sessDir(uid)
	if sdir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(sdir, "meta.json"))
	if err != nil {
		return nil
	}
	var m sessionMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}
func saveMeta(uid string, m *sessionMeta) {
	sdir := sessDir(uid)
	if sdir == "" {
		return
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	tmp := filepath.Join(sdir, "meta.json.tmp")
	_ = os.WriteFile(tmp, data, 0644)
	_ = os.Rename(tmp, filepath.Join(sdir, "meta.json"))
}
func sessReceived(uid string) int64 {
	sdir := sessDir(uid)
	if sdir == "" {
		return 0
	}
	st, err := os.Stat(filepath.Join(sdir, "data.part"))
	if err != nil {
		return 0
	}
	return st.Size()
}
type sessInfo struct {
	UID   string
	Meta  *sessionMeta
	Mtime float64
}
func iterSessions() []sessInfo {
	out := []sessInfo{}
	root := uploadsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := loadMeta(e.Name())
		if m == nil {
			continue
		}
		st, err := os.Stat(filepath.Join(root, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, sessInfo{UID: e.Name(), Meta: m,
			Mtime: float64(st.ModTime().UnixNano()) / 1e9})
	}
	return out
}
func SweepUploads() {
	root := uploadsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	now := nowUnix()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name())
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if now-float64(st.ModTime().UnixNano())/1e9 < float64(uploadTTLSec) {
			continue
		}
		_ = os.RemoveAll(p)
		forgetSessLock(e.Name())
	}
}
func touchSessDir(uid string) {
	if sdir := sessDir(uid); sdir != "" {
		now := time.Now()
		_ = os.Chtimes(sdir, now, now)
	}
}
