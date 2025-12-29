package config

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands ~ to the home directory and environment variables.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Handle ~ expansion
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = home
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	return filepath.Clean(path), nil
}

// ExpandEnv expands environment variables in a string.
func ExpandEnv(s string) string {
	return os.ExpandEnv(s)
}
