package beamcore

import (
	"net/http"
)

type fileItem struct {
	Name  string  `json:"name"`
	Path  string  `json:"path"`
	Size  int64   `json:"size"`
	Mtime float64 `json:"mtime"`
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	entries, truncated, deepUnder := walkShared(maxListFiles)
	items := []fileItem{}
	for _, e := range entries {
		_, base := splitParent(e.Path)
		items = append(items, fileItem{
			Name: base, Path: e.Path, Size: e.Size, Mtime: e.Mtime,
		})
	}
	dirs := buildDirStats(entries, deepUnder, truncated)
	if dirs == nil {
		dirs = []dirJSON{}
	}
	sendJSON(w, r, 200, map[string]interface{}{
		"files": items, "dirs": dirs, "truncated": truncated,
	})
}
