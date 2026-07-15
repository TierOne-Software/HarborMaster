package work

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic writes to path atomically: content is written to a
// temporary file in the same directory which is then renamed over path, so
// a crash or write error can never truncate or corrupt an existing file.
// If path already exists its permissions are preserved; otherwise the file
// is created with mode 0600.
func writeFileAtomic(path string, write func(f *os.File) error) (err error) {
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(mode); err != nil {
		return fmt.Errorf("failed to set permissions on temporary file: %w", err)
	}
	if err = write(tmp); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	return nil
}
