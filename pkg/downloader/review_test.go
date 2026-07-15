package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tierone/harbormaster/pkg/config"
)

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestHTTPDownloader_ChecksumMatch_SkipsDownload(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("served content"))
	}))
	defer server.Close()

	const content = "pinned content"
	dest := filepath.Join(t.TempDir(), "asset.bin")
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultOptions()
	opts.Checksum = sha256Hex(content)
	dl := NewHTTPDownloader(opts)

	hash, err := dl.Download(server.URL, dest)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if hash != opts.Checksum {
		t.Errorf("hash = %s, want %s", hash, opts.Checksum)
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("expected no HTTP requests for an already-matching file, got %d", n)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != content {
		t.Errorf("destination content changed: %q", got)
	}
}

func TestHTTPDownloader_ChecksumMismatch_PreservesDestination(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("tampered content"))
	}))
	defer server.Close()

	// The local file has drifted from the pinned checksum, so a download is
	// attempted — but the server's content doesn't match the pin either.
	const drifted = "locally drifted content"
	dest := filepath.Join(t.TempDir(), "asset.bin")
	if err := os.WriteFile(dest, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultOptions()
	opts.RetryAttempts = 3
	opts.Checksum = sha256Hex("pinned content")
	dl := NewHTTPDownloader(opts)

	_, err := dl.Download(server.URL, dest)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want checksum mismatch", err)
	}
	// The destination must be untouched by the rejected download.
	got, _ := os.ReadFile(dest)
	if string(got) != drifted {
		t.Errorf("destination was overwritten with unverified content: %q", got)
	}
	// A mismatch is definitive; it must not be retried.
	if n := requests.Load(); n != 1 {
		t.Errorf("expected exactly 1 attempt, got %d", n)
	}
	// No temp files may be left behind.
	entries, _ := os.ReadDir(filepath.Dir(dest))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".harbormaster-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestDetectType_ForgeHostInQueryString(t *testing.T) {
	if got := DetectType("https://downloads.example.com/tool.tar.gz?src=github.com"); got != config.RepoTypeHTTP {
		t.Errorf("DetectType = %s, want http", got)
	}
	// Forge host in the path proper is still git.
	if got := DetectType("https://github.com/user/repo"); got != config.RepoTypeGit {
		t.Errorf("DetectType = %s, want git", got)
	}
}

// A defaulted general.timeout must not bound git operations (initial clones
// can exceed any default), but an explicit timeout must; HTTP keeps the
// default as its client timeout either way.
func TestNewFromRepository_TimeoutMapping(t *testing.T) {
	cfg := config.NewDefaultConfig()
	gitRepo := &config.Repository{Name: "g", URL: "https://example.com/r.git", Type: config.RepoTypeGit}
	httpRepo := &config.Repository{Name: "h", URL: "https://example.com/f.bin", Type: config.RepoTypeHTTP}

	dl, err := NewFromRepository(gitRepo, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if timeout := dl.(*GitDownloader).options.Timeout; timeout != 0 {
		t.Errorf("git timeout with defaulted config = %v, want 0 (unbounded)", timeout)
	}

	dl, err = NewFromRepository(httpRepo, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if timeout := dl.(*HTTPDownloader).options.Timeout; timeout != cfg.General.Timeout {
		t.Errorf("http timeout = %v, want default %v", timeout, cfg.General.Timeout)
	}

	cfg.General.TimeoutExplicit = true
	dl, err = NewFromRepository(gitRepo, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if timeout := dl.(*GitDownloader).options.Timeout; timeout != cfg.General.Timeout {
		t.Errorf("git timeout with explicit config = %v, want %v", timeout, cfg.General.Timeout)
	}
}

// A pinned commit that is not reachable from any branch or tag tip must be
// fetched directly on a fresh clone, exactly as the update path does.
func TestGitDownloader_Download_PinnedUnreachableCommit(t *testing.T) {
	source := setupTestGitRepo(t)

	// Create a commit, record it, then move the branch back so the commit
	// is no longer reachable from any ref (as after a force-push).
	pinned := addCommit(t, source, "orphan.txt", "orphaned", "orphaned commit")
	gitRun(t, source, "reset", "--hard", "HEAD~1")
	// Direct SHA fetches need the server's consent; local daemons and major
	// forges allow it.
	gitRun(t, source, "config", "uploadpack.allowAnySHA1InWant", "true")

	opts := DefaultOptions()
	opts.Commit = pinned
	dl := NewGitDownloader(opts)

	dest := filepath.Join(t.TempDir(), "clone")
	sha, err := dl.Download(fileURL(source), dest)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if sha != pinned {
		t.Errorf("HEAD = %s, want pinned %s", sha, pinned)
	}
}
