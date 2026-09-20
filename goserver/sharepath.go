package beamcore

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// sharedBaseCache memoizes EvalSymlinks(SharedDir): one syscall per process
// instead of two per file op. Refreshed when SharedDir changes (tests) or
// when evaluation fails (fallback is never cached).
var (
	sharedBaseMu  sync.Mutex
	sharedBaseFor string
	sharedBaseVal string
)

func cachedSharedBase() string {
	sharedBaseMu.Lock()
	defer sharedBaseMu.Unlock()
	if sharedBaseVal != "" && sharedBaseFor == SharedDir {
		return sharedBaseVal
	}
	realBase, err := filepath.EvalSymlinks(SharedDir)
	if err != nil {
		return SharedDir
	}
	sharedBaseFor = SharedDir
	sharedBaseVal = realBase
	return realBase
}

// sharedFileStat stats one share-relative file WITHOUT following symlinks.
// Rejects symlinks, non-regular files, and targets escaping SharedDir
// (TOCTOU-hardened: Lstat + EvalSymlinks prefix check).
func sharedFileStat(rel string) (os.FileInfo, string, error) {
	fpath := filepath.Join(SharedDir, rel)
	st, err := os.Lstat(fpath)
	if err != nil {
		return nil, "", err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return nil, "", os.ErrNotExist
	}
	if !st.Mode().IsRegular() {
		return nil, "", os.ErrNotExist
	}
	real, err := filepath.EvalSymlinks(fpath)
	if err != nil {
		return nil, "", err
	}
	realBase := cachedSharedBase()
	if real != realBase && !strings.HasPrefix(real, realBase+string(os.PathSeparator)) {
		return nil, "", os.ErrNotExist
	}
	return st, fpath, nil
}

// sharedDirStat stats one share-relative dir WITHOUT following symlinks.
func sharedDirStat(rel string) (os.FileInfo, string, error) {
	fpath := filepath.Join(SharedDir, rel)
	st, err := os.Lstat(fpath)
	if err != nil {
		return nil, "", err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return nil, "", os.ErrNotExist
	}
	if !st.IsDir() {
		return nil, "", os.ErrNotExist
	}
	real, err := filepath.EvalSymlinks(fpath)
	if err != nil {
		return nil, "", err
	}
	realBase := cachedSharedBase()
	if real != realBase && !strings.HasPrefix(real, realBase+string(os.PathSeparator)) {
		return nil, "", os.ErrNotExist
	}
	return st, fpath, nil
}

// openSharedFile opens a share file after sharedFileStat + re-stat of the
// opened fd (narrows symlink-swap TOCTOU window).
func openSharedFile(rel string) (*os.File, os.FileInfo, string, error) {
	st, fpath, err := sharedFileStat(rel)
	if err != nil {
		return nil, nil, "", err
	}
	_ = st
	f, err := openSharedNoFollow(fpath)
	if err != nil {
		return nil, nil, "", err
	}
	fst, err := f.Stat()
	if err != nil || !fst.Mode().IsRegular() {
		f.Close()
		return nil, nil, "", os.ErrNotExist
	}
	// Return the live fd stat (not the pre-open Lstat) so ETag and
	// Content-Length always describe the bytes actually served.
	return f, fst, fpath, nil
}
