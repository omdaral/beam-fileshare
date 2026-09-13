package beamcore

import (
	"os"
	"path/filepath"
	"strings"
)

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
	realBase, err := filepath.EvalSymlinks(SharedDir)
	if err != nil {
		realBase = SharedDir
	}
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
	realBase, err := filepath.EvalSymlinks(SharedDir)
	if err != nil {
		realBase = SharedDir
	}
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
	f, err := os.Open(fpath)
	if err != nil {
		return nil, nil, "", err
	}
	fst, err := f.Stat()
	if err != nil || !fst.Mode().IsRegular() {
		f.Close()
		return nil, nil, "", os.ErrNotExist
	}
	return f, st, fpath, nil
}
