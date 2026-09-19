//go:build !linux

package beamcore

import "os"

// openSharedNoFollow falls back to a plain open where O_NOFOLLOW is
// unavailable; the Lstat + re-stat checks in the caller still apply.
func openSharedNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
