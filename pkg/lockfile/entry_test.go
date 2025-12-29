package lockfile

import (
	"testing"
	"time"
)

func TestLockEntry_IsStale(t *testing.T) {
	entry := LockEntry{RequestedRef: "main"}

	tests := []struct {
		name         string
		requestedRef string
		expected     bool
	}{
		{"same ref", "main", false},
		{"different ref", "develop", true},
		{"empty ref", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := entry.IsStale(tt.requestedRef)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLockEntry_Age(t *testing.T) {
	// Entry synced 1 hour ago
	entry := LockEntry{
		LastSyncedAt: time.Now().Add(-1 * time.Hour),
	}

	age := entry.Age()
	if age < 59*time.Minute || age > 61*time.Minute {
		t.Errorf("expected age around 1 hour, got %v", age)
	}
}

func TestSubmoduleLock(t *testing.T) {
	sub := SubmoduleLock{
		Path:        "vendor/lib",
		URL:         "https://github.com/other/lib.git",
		ResolvedSHA: "abc123",
	}

	if sub.Path != "vendor/lib" {
		t.Errorf("unexpected Path: %s", sub.Path)
	}
	if sub.URL != "https://github.com/other/lib.git" {
		t.Errorf("unexpected URL: %s", sub.URL)
	}
	if sub.ResolvedSHA != "abc123" {
		t.Errorf("unexpected ResolvedSHA: %s", sub.ResolvedSHA)
	}
}
