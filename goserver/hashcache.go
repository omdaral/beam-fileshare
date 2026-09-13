package beamcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

func fullSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, copyBufSize)
	for {
		nr, err := f.Read(buf)
		if nr > 0 {
			h.Write(buf[:nr])
		}
		if err != nil {
			break
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type hashEntry struct {
	Size   int64   `json:"size"`
	Mtime  float64 `json:"mtime"`
	Sha256 string  `json:"sha256"`
}

// cachedFileHash returns cached sha256, invalidated by size/mtime change.
func cachedFileHash(name, fpath string, size int64, mtime float64) (string, error) {
	cachePath := filepath.Join(SharedDir, hashcacheName)
	uploadLock.Lock()
	cache := map[string]hashEntry{}
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &cache)
	}
	if ent, ok := cache[name]; ok && ent.Size == size && ent.Mtime == mtime && ent.Sha256 != "" {
		uploadLock.Unlock()
		return ent.Sha256, nil
	}
	uploadLock.Unlock()

	digest, err := fullSHA256(fpath)
	if err != nil {
		return "", err
	}
	uploadLock.Lock()
	defer uploadLock.Unlock()
	cache2 := map[string]hashEntry{}
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &cache2)
	}
	cache2[name] = hashEntry{Size: size, Mtime: mtime, Sha256: digest}
	if len(cache2) > hashCacheMax {
		// Deterministic eviction (oldest mtime first), not random map order.
		type kv struct {
			k string
			m float64
		}
		all := make([]kv, 0, len(cache2))
		for k, v := range cache2 {
			all = append(all, kv{k, v.Mtime})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].m < all[j].m })
		for i := 0; i < len(all)-hashCacheMax; i++ {
			delete(cache2, all[i].k)
		}
	}
	if data, err := json.Marshal(cache2); err == nil {
		_ = os.WriteFile(cachePath, data, 0644)
	}
	return digest, nil
}
func invalidateFileHash(name string) {
	invalidateFileHashIn(SharedDir, name)
}
func invalidateFileHashIn(sharedDir, name string) {
	cachePath := filepath.Join(sharedDir, hashcacheName)
	uploadLock.Lock()
	defer uploadLock.Unlock()
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return
	}
	cache := map[string]hashEntry{}
	if err := json.Unmarshal(data, &cache); err != nil {
		return
	}
	if _, ok := cache[name]; ok {
		delete(cache, name)
		if out, err := json.Marshal(cache); err == nil {
			_ = os.WriteFile(cachePath, out, 0644)
		}
	}
}
func handleFileHash(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("file")
	name, ok := mustRel(raw)
	if !ok {
		fail(w, r, 400, "up_bad_name")
		return
	}
	st, fpath, err := sharedFileStat(name)
	if err != nil {
		fail(w, r, 404, "delete_missing")
		return
	}
	digest, err := cachedFileHash(name, fpath, st.Size(), float64(st.ModTime().UnixNano())/1e9)
	if err != nil {
		fail(w, r, 500, "up_verify_fail")
		return
	}
	sendJSON(w, r, 200, map[string]interface{}{
		"name": name, "size": st.Size(), "sha256": digest,
	})
}
