package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tierone/harbormaster/pkg/types"
)

// HTTPDownloader implements Downloader for HTTP/HTTPS file downloads.
type HTTPDownloader struct {
	options Options
	client  *http.Client
	source  string
}

// NewHTTPDownloader creates a new HTTPDownloader with the given options.
func NewHTTPDownloader(opts Options) *HTTPDownloader {
	client := &http.Client{
		Timeout: opts.Timeout,
	}

	return &HTTPDownloader{
		options: opts,
		client:  client,
	}
}

// Type returns the downloader type.
func (h *HTTPDownloader) Type() string {
	return "http"
}

// Download downloads a file from HTTP/HTTPS.
func (h *HTTPDownloader) Download(source, destination string) (string, error) {
	h.source = source

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= h.options.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(h.options.RetryDelay)
		}

		hash, err := h.downloadFile(source, destination)
		if err == nil {
			return hash, nil
		}
		lastErr = err
	}

	return "", fmt.Errorf("download failed after %d attempts: %w", h.options.RetryAttempts+1, lastErr)
}

// DownloadWithProgress downloads with progress reporting.
func (h *HTTPDownloader) DownloadWithProgress(source, destination string) (string, <-chan types.ProgressUpdate, error) {
	h.source = source
	progress := make(chan types.ProgressUpdate, 10)

	go func() {
		defer close(progress)

		progress <- types.ProgressUpdate{
			Phase:   types.PhaseConnecting,
			Message: "Connecting...",
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			progress <- types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: fmt.Errorf("failed to create directory: %w", err),
			}
			return
		}

		var lastErr error
		for attempt := 0; attempt <= h.options.RetryAttempts; attempt++ {
			if attempt > 0 {
				progress <- types.ProgressUpdate{
					Phase:   types.PhaseConnecting,
					Message: fmt.Sprintf("Retrying (%d/%d)...", attempt, h.options.RetryAttempts),
				}
				time.Sleep(h.options.RetryDelay)
			}

			hash, err := h.downloadFileWithProgress(source, destination, progress)
			if err == nil {
				progress <- types.ProgressUpdate{
					Phase:   types.PhaseComplete,
					Message: hash,
				}
				return
			}
			lastErr = err
		}

		progress <- types.ProgressUpdate{
			Phase: types.PhaseFailed,
			Error: fmt.Errorf("download failed: %w", lastErr),
		}
	}()

	return "", progress, nil
}

// Update re-downloads the file.
func (h *HTTPDownloader) Update(destination string) (string, error) {
	if h.source == "" {
		return "", fmt.Errorf("source URL not set")
	}
	return h.Download(h.source, destination)
}

// UpdateWithProgress re-downloads with progress reporting.
func (h *HTTPDownloader) UpdateWithProgress(destination string) (string, <-chan types.ProgressUpdate, error) {
	if h.source == "" {
		return "", nil, fmt.Errorf("source URL not set")
	}
	return h.DownloadWithProgress(h.source, destination)
}

// GetCurrentRef returns the SHA256 hash of the current file.
func (h *HTTPDownloader) GetCurrentRef(destination string) (string, error) {
	return hashFile(destination)
}

func (h *HTTPDownloader) downloadFile(source, destination string) (string, error) {
	req, err := http.NewRequest("GET", source, nil)
	if err != nil {
		return "", err
	}

	if h.options.UserAgent != "" {
		req.Header.Set("User-Agent", h.options.UserAgent)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	f, err := os.Create(destination)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(destination)
		return "", err
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(destination)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (h *HTTPDownloader) downloadFileWithProgress(source, destination string, progress chan<- types.ProgressUpdate) (string, error) {
	req, err := http.NewRequest("GET", source, nil)
	if err != nil {
		return "", err
	}

	if h.options.UserAgent != "" {
		req.Header.Set("User-Agent", h.options.UserAgent)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	progress <- types.ProgressUpdate{
		Phase:   types.PhaseFetching,
		Message: "Downloading...",
	}

	f, err := os.Create(destination)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	total := resp.ContentLength
	var done int64

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := writer.Write(buf[:n]); werr != nil {
				_ = f.Close()
				_ = os.Remove(destination)
				return "", werr
			}
			done += int64(n)

			if total > 0 {
				select {
				case progress <- types.ProgressUpdate{
					Phase:      types.PhaseFetching,
					BytesDone:  done,
					BytesTotal: total,
				}:
				default:
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = f.Close()
			_ = os.Remove(destination)
			return "", err
		}
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(destination)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
