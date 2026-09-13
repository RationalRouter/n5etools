package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGitHubReleaseAssetURL(t *testing.T) {
	got := GitHubReleaseAssetURL("RationalRouter", "n5etools", "translations-latest", "n5e-translations-es-0071_ocr_anomalies.sql.db")
	want := "https://github.com/RationalRouter/n5etools/releases/download/translations-latest/n5e-translations-es-0071_ocr_anomalies.sql.db"
	if got != want {
		t.Errorf("GitHubReleaseAssetURL() = %q, want %q", got, want)
	}
}

func TestHeadETag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("got method %s, want HEAD", r.Method)
		}
		w.Header().Set("ETag", `"abc123"`)
	}))
	defer srv.Close()

	got, err := headETag(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("headETag: %v", err)
	}
	if got != `"abc123"` {
		t.Errorf("headETag = %q, want %q", got, `"abc123"`)
	}
}

func TestHeadETagMissingHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	if _, err := headETag(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected an error for a response with no ETag header, got nil")
	}
}

func TestDownloadAssetTo(t *testing.T) {
	const body = "fake sqlite bundle contents"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"def456"`)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "bundle.db")
	etag, err := downloadAssetTo(context.Background(), srv.Client(), srv.URL, destPath)
	if err != nil {
		t.Fatalf("downloadAssetTo: %v", err)
	}
	if etag != `"def456"` {
		t.Errorf("etag = %q, want %q", etag, `"def456"`)
	}
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != body {
		t.Errorf("downloaded content = %q, want %q", got, body)
	}
}

func TestDownloadAssetToBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if _, err := downloadAssetTo(context.Background(), srv.Client(), srv.URL, filepath.Join(dir, "bundle.db")); err == nil {
		t.Fatal("expected an error for a 404 response, got nil")
	}
}
