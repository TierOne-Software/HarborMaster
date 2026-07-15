package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRelaxed_DemotesValidationToWarnings(t *testing.T) {
	path := writeConfigFile(t, t.TempDir(), `
[general]
work_dir = "."
bogus_key = true

[[repository]]
name = "a"
url = "https://example.com/a.git"
type = "git"
path = "shared"

[[repository]]
name = "b"
url = "https://example.com/b.git"
type = "git"
path = "shared"
`)

	// Strict load must fail.
	if _, err := Load(path); err == nil {
		t.Fatal("Load should fail on unknown key + duplicate paths")
	}

	// Relaxed load returns the config plus warnings.
	cfg, warnings, err := LoadRelaxed(path)
	if err != nil {
		t.Fatalf("LoadRelaxed failed: %v", err)
	}
	if len(cfg.Repositories) != 2 {
		t.Errorf("repositories = %d, want 2", len(cfg.Repositories))
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %d (%v), want 2", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "bogus_key") {
		t.Errorf("first warning should name the unknown key: %s", warnings[0])
	}
	if !strings.Contains(warnings[1], "validation") {
		t.Errorf("second warning should be the validation failure: %s", warnings[1])
	}
}

func TestLoadRelaxed_ParseErrorsStillFatal(t *testing.T) {
	path := writeConfigFile(t, t.TempDir(), "not [valid toml")
	if _, _, err := LoadRelaxed(path); err == nil {
		t.Fatal("LoadRelaxed should fail on broken TOML")
	}
}

func TestSetWorkDirOverride_NotPersisted(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, `
[general]
work_dir = "./ws"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg.SetWorkDirOverride("/tmp/elsewhere")
	if cfg.General.WorkDir != "/tmp/elsewhere" {
		t.Fatalf("override not applied at runtime: %s", cfg.General.WorkDir)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "ws")
	if reloaded.General.WorkDir != want {
		t.Errorf("work_dir after save = %s, want original %s", reloaded.General.WorkDir, want)
	}
	if reloaded.General.WorkDirOriginal != "./ws" {
		t.Errorf("work_dir original = %q, want %q", reloaded.General.WorkDirOriginal, "./ws")
	}
}

func TestTimeoutExplicit_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	// No timeout in the file: defaulted, not explicit, and not written back.
	path := writeConfigFile(t, dir, "[general]\nwork_dir = \".\"\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.General.TimeoutExplicit {
		t.Error("defaulted timeout should not be explicit")
	}
	if cfg.General.Timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want default %v", cfg.General.Timeout, DefaultTimeout)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "timeout") {
		t.Errorf("defaulted timeout was persisted:\n%s", raw)
	}

	// Explicit timeout: flagged and round-tripped.
	path2 := writeConfigFile(t, t.TempDir(), "[general]\nwork_dir = \".\"\ntimeout = \"5m\"\n")
	cfg2, err := Load(path2)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg2.General.TimeoutExplicit {
		t.Error("explicit timeout should be flagged")
	}
	if err := cfg2.Save(); err != nil {
		t.Fatal(err)
	}
	raw2, _ := os.ReadFile(path2)
	if !strings.Contains(string(raw2), "timeout = \"5m0s\"") {
		t.Errorf("explicit timeout lost on save:\n%s", raw2)
	}
}
