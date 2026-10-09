// GitHub Release asset downloads, used by internal/autoupdate to fetch a
// centrally-translated bundle (see internal/i18n's translation-bundle
// design) — the SAME per-book "cheap check, then conditional download"
// shape as the Drive functions above, but keyed on the standard HTTP ETag
// every GitHub Release asset response carries, rather than Drive's
// Last-Modified header.
//
// Deliberately never calls the GitHub REST API (api.github.com): the direct
// ".../releases/download/<tag>/<asset>" URL is a plain, unauthenticated CDN
// redirect (github.com -> objects.githubusercontent.com) that works
// identically for a public repo's release assets as it does for the app's
// own binary downloads (see README.md), and carries none of
// api.github.com's 60-requests/hour unauthenticated rate limit — a real
// concern for a URL every end user's install would otherwise hit on every
// launch.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// ErrAssetNotFound wraps the error returned by HeadETag/DownloadAsset when
// the release asset doesn't exist (HTTP 404) — the routine, expected case
// of "no bundle published yet for this exact filename", as opposed to a
// real network/server failure. Callers check for it with errors.Is.
var ErrAssetNotFound = errors.New("release asset not found")

// GitHubReleaseAssetURL builds the direct download URL for one named asset
// on one release tag of a public GitHub repo.
func GitHubReleaseAssetURL(owner, repo, tag, assetName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag), url.PathEscape(assetName))
}

// HeadETag does a cheap HEAD request against a release asset URL and
// returns its ETag — the basis for "has this asset changed" without
// downloading the body, mirroring HeadLastModified's role for Drive. client
// may be nil.
func HeadETag(ctx context.Context, client *http.Client, assetURL string) (string, error) {
	if client == nil {
		client = defaultClient(15 * time.Second)
	}
	return headETag(ctx, client, assetURL)
}

func headETag(ctx context.Context, client *http.Client, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("HEAD %s: %w", target, ErrAssetNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HEAD %s: unexpected status %s", target, resp.Status)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", fmt.Errorf("HEAD %s: no ETag header in response", target)
	}
	return etag, nil
}

// DownloadAsset streams a release asset to destPath and returns its ETag,
// with no content-type sniffing (unlike Download's PDF-magic-bytes check —
// a translation bundle is a SQLite file, not a PDF, and its own consumer
// validates it by opening it as a database, not by a byte-sniff here).
// client may be nil.
func DownloadAsset(ctx context.Context, client *http.Client, assetURL, destPath string) (etag string, err error) {
	if client == nil {
		client = downloadClient()
	}
	return downloadAssetTo(ctx, client, assetURL, destPath)
}

func downloadAssetTo(ctx context.Context, client *http.Client, target, destPath string) (string, error) {
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

	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, progressReader{resp.Body, progress}); err != nil {
		return "", fmt.Errorf("writing %s: %w", destPath, stallError(ctx, err))
	}
	return resp.Header.Get("ETag"), nil
}
