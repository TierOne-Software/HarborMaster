//go:build !unix

package lockfile

import "os"

// Advisory file locking is not supported on this platform; locking degrades
// gracefully to a no-op. Concurrent harbormaster processes on such platforms
// still get atomic saves, but read-modify-write cycles are not serialized.

func flockExclusive(_ *os.File) error { return nil }

func flockUnlock(_ *os.File) error { return nil }
