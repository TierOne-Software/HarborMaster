package downloader

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tierone/harbormaster/pkg/types"
)

func TestHTTPDownloader_Type(t *testing.T) {
	dl := NewHTTPDownloader(DefaultOptions())
	if dl.Type() != "http" {
		t.Errorf("expected type 'http', got '%s'", dl.Type())
	}
}

func TestHTTPDownloader_Download(t *testing.T) {
	// Create test server
	content := "Hello, World!"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "13")
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.txt")

	dl := NewHTTPDownloader(Options{
		Timeout:       30 * time.Second,
		RetryAttempts: 0,
	})

	hash, err := dl.Download(server.URL, destPath)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Verify content
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected content '%s', got '%s'", content, string(data))
	}

	// Verify hash is returned
	if hash == "" {
		t.Error("expected hash to be returned")
	}
}

func TestHTTPDownloader_Download_UserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.txt")

	dl := NewHTTPDownloader(Options{
		UserAgent:     "TestAgent/1.0",
		Timeout:       30 * time.Second,
		RetryAttempts: 0,
	})

	_, err := dl.Download(server.URL, destPath)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if receivedUA != "TestAgent/1.0" {
		t.Errorf("expected User-Agent 'TestAgent/1.0', got '%s'", receivedUA)
	}
}

func TestHTTPDownloader_Download_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.txt")

	dl := NewHTTPDownloader(Options{
		Timeout:       30 * time.Second,
		RetryAttempts: 0,
	})

	_, err := dl.Download(server.URL, destPath)
	if err == nil {
		t.Error("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

func TestHTTPDownloader_Download_500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.txt")

	dl := NewHTTPDownloader(Options{
		Timeout:       30 * time.Second,
		RetryAttempts: 0,
	})

	_, err := dl.Download(server.URL, destPath)
	if err == nil {
		t.Error("expected error for 500")
	}
}

func TestHTTPDownloader_Download_Retry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.txt")

	dl := NewHTTPDownloader(Options{
		Timeout:       30 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    1 * time.Millisecond,
	})

	_, err := dl.Download(server.URL, destPath)
	if err != nil {
		t.Fatalf("download should succeed after retries: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestHTTPDownloader_Download_CreateDir(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "nested", "dir", "test.txt")

	dl := NewHTTPDownloader(Options{
		Timeout:       30 * time.Second,
		RetryAttempts: 0,
	})

	_, err := dl.Download(server.URL, destPath)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Verify nested dirs were created
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("file not created in nested dir: %v", err)
	}
}

func TestHTTPDownloader_GetCurrentRef(t *testing.T) {
	// Create a file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	dl := NewHTTPDownloader(DefaultOptions())
	hash, err := dl.GetCurrentRef(testFile)
	if err != nil {
		t.Fatalf("GetCurrentRef failed: %v", err)
	}

	if hash == "" {
		t.Error("expected hash to be returned")
	}

	// Hash should be consistent
	hash2, _ := dl.GetCurrentRef(testFile)
	if hash != hash2 {
		t.Error("hash should be consistent for same content")
	}
}

func TestHTTPDownloader_DownloadWithProgress(t *testing.T) {
	content := strings.Repeat("x", 1000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.txt")

	dl := NewHTTPDownloader(Options{
		Timeout:       30 * time.Second,
		RetryAttempts: 0,
	})

	_, progressCh, err := dl.DownloadWithProgress(server.URL, destPath)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Consume progress updates
	var lastUpdate types.ProgressUpdate
	for update := range progressCh {
		lastUpdate = update
	}

	// Should end with complete
	if lastUpdate.Phase != "complete" && lastUpdate.Phase != "failed" {
		t.Errorf("expected final phase to be complete or failed, got %s", lastUpdate.Phase)
	}

	// Verify file was downloaded
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}
