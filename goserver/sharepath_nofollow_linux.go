//go:build linux

package beamcore

import (
	"os"
	"syscall"
)

// openSharedNoFollow opens a share file without following a trailing
// symlink (narrows the stat->open swap window; ELOOP surfaces as
// "not exist" upstream).
func openSharedNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}
