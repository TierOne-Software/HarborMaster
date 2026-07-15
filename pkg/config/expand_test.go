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

func TestExpandPath_UndefinedEnvVar(t *testing.T) {
	_ = os.Unsetenv("HARBORMASTER_TEST_UNSET_VAR")

	tests := []string{
		"$HARBORMASTER_TEST_UNSET_VAR/ws",
		"${HARBORMASTER_TEST_UNSET_VAR}/ws",
		"/prefix/$HARBORMASTER_TEST_UNSET_VAR/suffix",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ExpandPath(input)
			if err == nil {
				t.Fatalf("expected error for undefined env var in %q", input)
			}
			if !strings.Contains(err.Error(), "HARBORMASTER_TEST_UNSET_VAR") {
				t.Errorf("expected error to name the missing variable, got: %v", err)
			}
		})
	}
}

func TestExpandPath_EmptyButDefinedEnvVar(t *testing.T) {
	// A variable that is set (even to empty) is not an error.
	t.Setenv("HARBORMASTER_TEST_EMPTY_VAR", "")

	result, err := ExpandPath("/path/$HARBORMASTER_TEST_EMPTY_VAR/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "/path/dir" {
		t.Errorf("expected '/path/dir', got '%s'", result)
	}
}

func TestExpandPath_TildeUser(t *testing.T) {
	_, err := ExpandPath("~otheruser/projects")
	if err == nil {
		t.Fatal("expected error for ~user path")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'not supported' error, got: %v", err)
	}
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_VAR", "hello")

	result := ExpandEnv("$TEST_VAR world")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result)
	}
}
