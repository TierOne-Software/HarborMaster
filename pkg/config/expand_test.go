package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home directory")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde with path",
			input:    "~/projects/test",
			expected: filepath.Join(home, "projects/test"),
		},
		{
			name:     "tilde only",
			input:    "~",
			expected: home,
		},
		{
			name:     "no tilde",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "relative path",
			input:    "relative/path",
			expected: "relative/path",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandPath(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExpandPath_EnvVar(t *testing.T) {
	// Set test environment variable
	t.Setenv("TEST_HARBORMASTER_VAR", "testvalue")

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "env var expansion",
			input:    "/path/$TEST_HARBORMASTER_VAR/dir",
			contains: "testvalue",
		},
		{
			name:     "env var with braces",
			input:    "/path/${TEST_HARBORMASTER_VAR}/dir",
			contains: "testvalue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandPath(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected result to contain '%s', got '%s'", tt.contains, result)
			}
		})
	}
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_VAR", "hello")

	result := ExpandEnv("$TEST_VAR world")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result)
	}
}
