// Package fetch downloads the official sourcebook PDFs from their canonical
// Google Drive locations (internal/sources.OfficialBooks) for the
// unattended startup auto-updater (internal/autoupdate).
//
// Verified against all 6 real book links before writing this: Drive's
// "uc?export=download&id=<ID>" endpoint serves every one of them directly
// (redirecting to drive.usercontent.google.com/download?...) since all are
// well under Drive's ~100MB virus-scan-interstitial threshold — no
// confirm-token dance needed. A plain HEAD against that same URL returns a
// Last-Modified header without downloading the body, which is what makes
// "check for updates" a cheap per-book request rather than a full download
// on every app startup.
package fetch

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"
)

var driveFileIDRe = regexp.MustCompile(`/file/d/([^/]+)`)

// DriveFileID extracts the file ID from a Drive "view" URL, e.g.
// "https://drive.google.com/file/d/1abcXYZ/view?usp=sharing" -> "1abcXYZ".
// Every internal/sources.OfficialBooks entry uses this exact URL shape.
func DriveFileID(driveURL string) (string, error) {
	m := driveFileIDRe.FindStringSubmatch(driveURL)
	if m == nil {
		return "", fmt.Errorf("no /file/d/<id>/ segment in %q", driveURL)
	}
	return m[1], nil
}

// directDownloadURL builds Drive's direct-download endpoint for a file ID.
func directDownloadURL(fileID string) string {
	return "https://drive.google.com/uc?export=download&id=" + url.QueryEscape(fileID)
}

// defaultClient is used whenever a caller passes a nil *http.Client — same
// nil-means-sane-default convention internal/correct.Options.HTTPClient
// already established in this codebase.
func defaultClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// downloadClient is the nil-client default for full-file downloads. It has
// no total timeout: Drive throttles some files to well under 100 KB/s (the
// 18 MB Bingo Book took about 5 minutes), so any fixed whole-request budget
// either fails slow-but-healthy transfers on every launch or is too long to
// catch a dead one. A dead transfer is caught by stallTimeout instead, and
// the caller's context still bounds the total.
func downloadClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: t}
}

// stallTimeout is how long a download may go without receiving a single
// byte before it is abandoned. A var so tests can shorten it.
var stallTimeout = 60 * time.Second

var errStalled = errors.New("download stalled")

// stallGuard derives a context that is cancelled once no progress has been
// reported for stallTimeout. Call progress after every successful read and
// stop once the transfer is over.
func stallGuard(ctx context.Context) (guarded context.Context, progress func(), stop func()) {
	guarded, cancel := context.WithCancelCause(ctx)
	timer := time.AfterFunc(stallTimeout, func() { cancel(errStalled) })
	return guarded, func() { timer.Reset(stallTimeout) }, func() { timer.Stop(); cancel(nil) }
}

// progressReader calls onRead after every read that returned data.
type progressReader struct {
	r      io.Reader
	onRead func()
}

func (p progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.onRead()
	}
	return n, err
}

// stallError replaces err with a clear message when the transfer was
// cancelled by stallGuard rather than failing on its own.
func stallError(ctx context.Context, err error) error {
	if errors.Is(context.Cause(ctx), errStalled) {
		return fmt.Errorf("no data received for %s: %w", stallTimeout, errStalled)
	}
	return err
}

// HeadLastModified does a cheap HEAD request against a book's canonical
// Drive link and returns the server's Last-Modified header — the basis for
// "check for updates" without downloading the body. client may be nil.
func HeadLastModified(ctx context.Context, client *http.Client, driveURL string) (time.Time, error) {
	id, err := DriveFileID(driveURL)
	if err != nil {
		return time.Time{}, err
	}
	if client == nil {
		client = defaultClient(15 * time.Second)
	}
	return headLastModified(ctx, client, directDownloadURL(id))
}

// headLastModified is the actual HTTP-handling logic, split out from
// HeadLastModified so tests can point it at an httptest server directly
// instead of needing a real drive.google.com URL to substitute in.
func headLastModified(ctx context.Context, client *http.Client, target string) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return time.Time{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("HEAD %s: unexpected status %s", target, resp.Status)
	}
	lm := resp.Header.Get("Last-Modified")
	if lm == "" {
		return time.Time{}, fmt.Errorf("HEAD %s: no Last-Modified header in response", target)
	}
	t, err := http.ParseTime(lm)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing Last-Modified %q: %w", lm, err)
	}
	return t, nil
}

// Download streams a book's PDF to destPath, verifying the response is
// actually a PDF before writing anything to disk — a guard against ever
// silently "ingesting" an HTML virus-scan-warning interstitial page, should
// some future book cross Drive's size threshold — and returns its SHA256,
// computed while streaming rather than buffering the whole file in memory.
// client may be nil.
func Download(ctx context.Context, client *http.Client, driveURL, destPath string) (string, error) {
	id, err := DriveFileID(driveURL)
	if err != nil {
		return "", err
	}
	if client == nil {
		client = downloadClient()
	}
	return downloadTo(ctx, client, directDownloadURL(id), destPath)
}

// downloadTo is Download's actual HTTP-handling logic, split out the same
// way and for the same reason as headLastModified above.
func downloadTo(ctx context.Context, client *http.Client, target, destPath string) (string, error) {
	ctx, progress, stop := stallGuard(ctx)
	defer stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", stallError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: unexpected status %s", target, resp.Status)
	}

	br := bufio.NewReader(progressReader{resp.Body, progress})
	sniff, err := br.Peek(4)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("reading response from %s: %w", target, stallError(ctx, err))
	}
	if string(sniff) != "%PDF" {
		return "", fmt.Errorf("response from %s is not a PDF (got %q) — likely a Drive virus-scan interstitial page", target, sniff)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), br); err != nil {
		return "", fmt.Errorf("writing %s: %w", destPath, stallError(ctx, err))
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
