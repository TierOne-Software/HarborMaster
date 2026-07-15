//go:build unix

package lockfile

import (
	"errors"
	"os"
	"syscall"
)

// flockExclusive acquires an exclusive advisory lock on f, blocking until
// the lock is available.
func flockExclusive(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

// flockUnlock releases the advisory lock on f.
func flockUnlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
