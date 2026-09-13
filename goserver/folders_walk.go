package beamcore

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type sharedEntry struct {
	Path  string
	Size  int64
	Mtime float64
	Depth int // 1 = top level ("a.txt"), 2 = "dir/a.txt", ...
}
// walkShared lists regular share files up to limit, sorted by path.
// Dotfiles/dotdirs, symlinks, .uploads and non-regular files are skipped.
// Entries deeper than maxRelDepth are skipped and reported via deepUnder
// (ancestor rel paths that lost content to the depth cap).
func walkShared(limit int) (entries []sharedEntry, truncated bool, deepUnder map[string]bool) {
	type item struct {
		dir   string
		rel   string
		depth int
	}
	deepUnder = map[string]bool{}
	stack := []item{{dir: SharedDir, rel: "", depth: 0}}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		rd, err := os.ReadDir(cur.dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(rd))
		byName := map[string]os.DirEntry{}
		for _, e := range rd {
			names = append(names, e.Name())
			byName[e.Name()] = e
		}
		sort.Strings(names)
		for _, nm := range names {
			e := byName[nm]
			if strings.HasPrefix(nm, ".") {
				continue
			}
			if e.Type()&os.ModeSymlink != 0 {
				continue
			}
			rel := nm
			if cur.rel != "" {
				rel = cur.rel + "/" + nm
			}
			if e.IsDir() {
				if cur.depth+1 >= maxRelDepth {
					for d := rel; d != ""; {
						deepUnder[d] = true
						d, _ = splitParent(d)
					}
					continue
				}
				stack = append(stack, item{dir: filepath.Join(cur.dir, nm), rel: rel, depth: cur.depth + 1})
				continue
			}
			info, err := e.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if len(entries) >= limit {
				return entries, true, deepUnder
			}
			entries = append(entries, sharedEntry{
				Path: rel, Size: info.Size(),
				Mtime: float64(info.ModTime().UnixNano()) / 1e9,
				Depth: cur.depth + 1,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, false, deepUnder
}
// walkSharedDir lists regular files under one validated rel dir.
func walkSharedDir(rel string, limit int) ([]sharedEntry, bool) {
	root := filepath.Join(SharedDir, rel)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil, false
	}
	baseDepth := 0
	if rel != "" {
		baseDepth = len(strings.Split(rel, "/"))
	}
	type item struct {
		dir   string
		rel   string
		depth int
	}
	out := []sharedEntry{}
	truncated := false
	stack := []item{{dir: root, rel: rel, depth: baseDepth}}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		rd, err := os.ReadDir(cur.dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(rd))
		byName := map[string]os.DirEntry{}
		for _, e := range rd {
			names = append(names, e.Name())
			byName[e.Name()] = e
		}
		sort.Strings(names)
		for _, nm := range names {
			e := byName[nm]
			if strings.HasPrefix(nm, ".") {
				continue
			}
			if e.Type()&os.ModeSymlink != 0 {
				continue
			}
			sub := nm
			if cur.rel != "" {
				sub = cur.rel + "/" + nm
			}
			if e.IsDir() {
				if cur.depth+1 >= maxRelDepth {
					continue
				}
				stack = append(stack, item{dir: filepath.Join(cur.dir, nm), rel: sub, depth: cur.depth + 1})
				continue
			}
			info, err := e.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if len(out) >= limit {
				return out, true
			}
			out = append(out, sharedEntry{
				Path: sub, Size: info.Size(),
				Mtime: float64(info.ModTime().UnixNano()) / 1e9,
				Depth: cur.depth + 1,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, truncated
}
// dirJSON describes one folder for the tree UI.
// Over is "" (expandable) or "files"/"depth"/"list": over-cap folders must
// be shown as a direct zip row with the reason, never expanded.
type dirJSON struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
	Over  string `json:"over,omitempty"`
}
// buildDirStats aggregates recursive file counts from a listing.
func buildDirStats(entries []sharedEntry, deepUnder map[string]bool, truncated bool) []dirJSON {
	counts := map[string]int{}
	order := []string{}
	seen := map[string]bool{}
	for _, e := range entries {
		d, _ := splitParent(e.Path)
		for d != "" {
			if !seen[d] {
				seen[d] = true
				order = append(order, d)
			}
			counts[d]++
			d, _ = splitParent(d)
		}
	}
	for d := range deepUnder {
		if !seen[d] {
			seen[d] = true
			order = append(order, d)
		}
	}
	sort.Strings(order)
	out := []dirJSON{}
	for _, d := range order {
		over := ""
		switch {
		case counts[d] > maxTreeFiles:
			over = "files"
		case deepUnder[d]:
			over = "depth"
		case truncated && counts[d] > 0:
			over = "list"
		}
		out = append(out, dirJSON{Path: d, Files: counts[d], Over: over})
	}
	return out
}
