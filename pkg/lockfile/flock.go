package lockfile

import (
	"fmt"
	"os"
)

// FileLock is an inter-process advisory lock guarding a lock file. It is
// implemented with flock(2) on Unix systems; on platforms without advisory
// lock support it degrades gracefully to a no-op.
//
// The advisory lock is taken on a sidecar file (path + ".flock") rather
// than on the lock file itself, because Save replaces the lock file via
// rename and a lock on the replaced inode would be useless.
type FileLock struct {
	f *os.File
}

// lockSidecarPath returns the path of the sidecar file used for advisory
// locking of the lock file at path.
func lockSidecarPath(path string) string {
	return path + ".flock"
}

// Lock blocks until the exclusive inter-process advisory lock for the lock
// file at path is acquired. Callers must Unlock the returned FileLock when
// done, typically with defer.
func Lock(path string) (*FileLock, error) {
	f, err := os.OpenFile(lockSidecarPath(path), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock sidecar: %w", err)
	}
	if err := flockExclusive(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to acquire lock on %s: %w", path, err)
	}
	return &FileLock{f: f}, nil
}

// Unlock releases the advisory lock. It is safe to call on a nil FileLock
// and safe to call more than once.
func (l *FileLock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	unlockErr := flockUnlock(l.f)
	closeErr := l.f.Close()
	l.f = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

// Mutate performs an atomic read-modify-write cycle on the lock file at
// path, holding the inter-process advisory lock for its duration so that
// concurrent harbormaster processes cannot lose each other's updates.
// If fn returns an error the lock file is left unchanged.
func Mutate(path string, fn func(*LockFile) error) error {
	lock, err := Lock(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Unlock()
	}()

	lf, err := Load(path)
	if err != nil {
		return err
	}
	if err := fn(lf); err != nil {
		return err
	}
	return lf.Save(path)
}
