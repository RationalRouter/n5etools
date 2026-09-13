package autoupdate

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/store"
)

// buildFixtureBundle creates a small SQLite file at path containing one
// content_translations row for locale, mirroring the shape a real
// cmd/n5e-i18n-published bundle would have.
func buildFixtureBundle(t *testing.T, path, locale, value string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE content_translations (
			table_name TEXT, slug TEXT, locale TEXT, field TEXT, value TEXT,
			status TEXT, en_hash TEXT, updated_at TEXT,
			PRIMARY KEY (table_name, slug, locale, field)
		)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO content_translations (table_name, slug, locale, field, value, status, en_hash, updated_at)
		VALUES ('jutsu', 'jutsu/fireball', ?, 'description', ?, 'machine', 'deadbeef', datetime('now'))`,
		locale, value); err != nil {
		t.Fatal(err)
	}
}

func TestCheckOneTranslationDownloadsAndApplies(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "fixture-bundle.db")
	buildFixtureBundle(t, bundlePath, "es", "bola de fuego")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		http.ServeFile(w, r, bundlePath)
	}))
	defer srv.Close()

	cacheDir := filepath.Join(dir, "cache")
	res := checkOneTranslationAt(t, db, cacheDir, srv, "es", "test-version-1")

	if res.Action != "updated" {
		t.Fatalf("Action = %q, want %q (err: %v)", res.Action, "updated", res.Err)
	}

	var got string
	if err := db.QueryRow(`SELECT value FROM content_translations WHERE slug = 'jutsu/fireball' AND locale = 'es'`).Scan(&got); err != nil {
		t.Fatalf("reading applied row: %v", err)
	}
	if got != "bola de fuego" {
		t.Errorf("applied value = %q, want %q", got, "bola de fuego")
	}

	_, etag, found, err := store.GetTranslationBundleState(db, "es")
	if err != nil {
		t.Fatalf("GetTranslationBundleState: %v", err)
	}
	if !found || etag != `"v1"` {
		t.Errorf("stored state = (found=%v, etag=%q), want (true, %q)", found, etag, `"v1"`)
	}
}

func TestCheckOneTranslationUnchangedSkipsReapply(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "fixture-bundle.db")
	buildFixtureBundle(t, bundlePath, "es", "bola de fuego")

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		if r.Method == http.MethodGet {
			hits++
		}
		http.ServeFile(w, r, bundlePath)
	}))
	defer srv.Close()

	cacheDir := filepath.Join(dir, "cache")
	first := checkOneTranslationAt(t, db, cacheDir, srv, "es", "test-version-1")
	if first.Action != "updated" {
		t.Fatalf("first Action = %q, want updated (err: %v)", first.Action, first.Err)
	}
	second := checkOneTranslationAt(t, db, cacheDir, srv, "es", "test-version-1")
	if second.Action != "unchanged" {
		t.Fatalf("second Action = %q, want unchanged (err: %v)", second.Action, second.Err)
	}
	if hits != 1 {
		t.Errorf("bundle GET happened %d times, want 1 (unchanged run must not re-download)", hits)
	}
}

func TestCheckOneTranslationNoBundleYet(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	res := checkOneTranslationAt(t, db, filepath.Join(dir, "cache"), srv, "es", "test-version-1")
	if res.Action != "no bundle yet" {
		t.Fatalf("Action = %q, want %q (err: %v)", res.Action, "no bundle yet", res.Err)
	}
}

// checkOneTranslationAt runs checkOneTranslation against srv, substituting
// its URL for the real GitHub asset URL via a rewriteTransport, matching
// autoupdate_test.go's own established pattern for redirecting a hardcoded
// external hostname to a local httptest server.
func checkOneTranslationAt(t *testing.T, db *sql.DB, cacheDir string, srv *httptest.Server, locale, version string) TranslationResult {
	t.Helper()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	client := clientFor(srv)
	return checkOneTranslation(context.Background(), db, cacheDir, client, version, locale)
}
