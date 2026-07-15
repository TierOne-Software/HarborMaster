package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
		source:  opts.SourceURL,
	}
}

// Type returns the downloader type.
func (h *HTTPDownloader) Type() string {
	return "http"
}

// Download downloads a file from HTTP/HTTPS.
func (h *HTTPDownloader) Download(source, destination string) (string, error) {
	return h.download(source, destination, nil)
}

// DownloadWithProgress downloads with progress reporting.
func (h *HTTPDownloader) DownloadWithProgress(source, destination string) (string, <-chan types.ProgressUpdate, error) {
	progress := make(chan types.ProgressUpdate, 10)

	go func() {
		defer close(progress)

		hash, err := h.download(source, destination, progress)
		if err != nil {
			sendTerminal(progress, types.ProgressUpdate{
				Phase: types.PhaseFailed,
				Error: err,
			})
			return
		}

		sendTerminal(progress, types.ProgressUpdate{
			Phase:   types.PhaseComplete,
			Message: hash,
		})
	}()

	return "", progress, nil
}

// download is the shared implementation for Download and DownloadWithProgress.
// progress may be nil for the blocking variant.
func (h *HTTPDownloader) download(source, destination string, progress chan types.ProgressUpdate) (string, error) {
	h.source = source

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// If the destination already matches the expected checksum there is
	// nothing to do; this also keeps locked syncs from ever touching a
	// pinned artifact.
	if h.options.Checksum != "" {
		if hash, err := hashFile(destination); err == nil && hash == h.options.Checksum {
			return hash, nil
		}
	}

	// Bound the whole operation (all attempts and retry sleeps) by the
	// configured timeout so retries cannot outlive it.
	ctx, cancel := h.newContext()
	defer cancel()

	sendUpdate(progress, types.ProgressUpdate{
		Phase:   types.PhaseConnecting,
		Message: "Connecting...",
	})

	var lastErr error
	attempts := 0
	for attempt := 0; attempt <= h.options.RetryAttempts; attempt++ {
		if attempt > 0 {
			sendUpdate(progress, types.ProgressUpdate{
				Phase:   types.PhaseConnecting,
				Message: fmt.Sprintf("Retrying (%d/%d)...", attempt, h.options.RetryAttempts),
			})
			select {
			case <-time.After(h.options.RetryDelay):
			case <-ctx.Done():
				return "", fmt.Errorf("download failed after %d attempt(s): %w (%w)", attempts, lastErr, ctx.Err())
			}
		}

		attempts++
		hash, err := h.downloadFile(ctx, source, destination, progress)
		if err == nil {
			return hash, nil
		}
		lastErr = err

		if !isRetryableError(err) {
			break
		}
	}

	return "", fmt.Errorf("download failed after %d attempt(s): %w", attempts, lastErr)
}

// newContext returns a context honoring Options.Timeout.
func (h *HTTPDownloader) newContext() (context.Context, context.CancelFunc) {
	if h.options.Timeout > 0 {
		return context.WithTimeout(context.Background(), h.options.Timeout)
	}
	return context.WithCancel(context.Background())
}

// httpStatusError is an HTTP error response status.
type httpStatusError struct {
	code   int
	status string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.code, e.status)
}

// checksumError reports downloaded content not matching Options.Checksum.
type checksumError struct {
	expected, got string
}

func (e *checksumError) Error() string {
	return fmt.Sprintf("checksum mismatch: expected %s, got %s", e.expected, e.got)
}

// isRetryableError reports whether a download error is worth retrying.
// Client errors such as 404 are definitive and are not retried; server
// errors, throttling, and network errors are.
func isRetryableError(err error) bool {
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.code >= 500 ||
			statusErr.code == http.StatusTooManyRequests ||
			statusErr.code == http.StatusRequestTimeout
	}
	// A checksum mismatch is definitive: the server sent the wrong content.
	var ckErr *checksumError
	if errors.As(err, &ckErr) {
		return false
	}
	// Network-level errors are considered transient.
	return true
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

// downloadFile performs a single download attempt. The file is written to a
// temporary file and atomically renamed into place only on success, so a
// failed or interrupted download never destroys a previous good file.
func (h *HTTPDownloader) downloadFile(ctx context.Context, source, destination string, progress chan types.ProgressUpdate) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", err
	}

	if h.options.UserAgent != "" {
		req.Header.Set("User-Agent", h.options.UserAgent)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		// Request errors embed the URL, which may contain credentials.
		return "", errors.New(scrubCredentials(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", &httpStatusError{code: resp.StatusCode, status: resp.Status}
	}

	sendUpdate(progress, types.ProgressUpdate{
		Phase:   types.PhaseFetching,
		Message: "Downloading...",
	})

	tmp, err := os.CreateTemp(filepath.Dir(destination), ".harbormaster-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(tmp, hasher)

	total := resp.ContentLength
	var done int64

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := writer.Write(buf[:n]); werr != nil {
				cleanup()
				return "", werr
			}
			done += int64(n)

			if total > 0 {
				sendUpdate(progress, types.ProgressUpdate{
					Phase:      types.PhaseFetching,
					BytesDone:  done,
					BytesTotal: total,
				})
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup()
			return "", err
		}
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Verify the expected checksum before the file can replace the
	// destination, so a changed or tampered upstream never clobbers a
	// pinned artifact.
	hash := hex.EncodeToString(hasher.Sum(nil))
	if h.options.Checksum != "" && hash != h.options.Checksum {
		_ = os.Remove(tmpPath)
		return "", &checksumError{expected: h.options.Checksum, got: hash}
	}

	// os.CreateTemp creates the file with 0600; match os.Create's default.
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to set file permissions: %w", err)
	}

	if err := os.Rename(tmpPath, destination); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to move file into place: %w", err)
	}

	return hash, nil
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
