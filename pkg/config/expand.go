package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands ~ to the home directory and environment variables.
//
// Referencing an environment variable that is not set is an error rather
// than silently expanding to an empty string (which would turn a path like
// "$UNSET/ws" into "/ws"). Paths of the form "~user" are rejected because
// per-user home directory lookup is not supported.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Handle ~ expansion
	switch {
	case strings.HasPrefix(path, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	case path == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = home
	case strings.HasPrefix(path, "~"):
		return "", fmt.Errorf("cannot expand path %q: ~user home directories are not supported", path)
	}

	// Expand environment variables, tracking references to unset variables.
	var missing []string
	expanded := os.Expand(path, func(name string) string {
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		missing = append(missing, name)
		return ""
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("cannot expand path %q: undefined environment variable(s): %s",
			path, strings.Join(missing, ", "))
	}

	return filepath.Clean(expanded), nil
}

// ExpandEnv expands environment variables in a string.
func ExpandEnv(s string) string {
	return os.ExpandEnv(s)
}
