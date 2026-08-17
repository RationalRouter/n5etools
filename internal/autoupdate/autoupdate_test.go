package autoupdate

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/ingest"
	"github.com/sergio/n5e/internal/schema"
	"github.com/sergio/n5e/internal/sources"
	"github.com/sergio/n5e/internal/store"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := schema.Apply(db, schema.Rules); err != nil {
		t.Fatal(err)
	}
	return db
}

// rewriteTransport redirects every outgoing request to target's scheme+host,
// regardless of what host was in the request URL. internal/fetch always
// builds a real drive.google.com URL from a BookSource's DriveURL, so tests
// need this to actually exercise checkOne's network calls against a local
// httptest.Server instead of the real internet.
type rewriteTransport struct{ target *url.URL }

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func clientFor(srv *httptest.Server) *http.Client {
	u, err := url.Parse(srv.URL)
	if err != nil {
		panic(err)
	}
	return &http.Client{Transport: &rewriteTransport{target: u}}
}

const fakeDriveURL = "https://drive.google.com/file/d/1FakeFileIDForTests/view?usp=sharing"

// testVersion returns the current binary's real LatestMigration(Rules)
// value — tests stamp this as the "already up to date" baseline so they
// exercise the Drive-timestamp logic in isolation, without also tripping
// the (separately, explicitly tested) version-staleness gate.
func testVersion(t *testing.T) string {
	t.Helper()
	v, err := schema.LatestMigration(schema.Rules)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCheckOneUnchanged(t *testing.T) {
	db := testDB(t)
	remoteMod := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	downloadCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteMod.Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			downloadCalled = true
		}
	}))
	defer srv.Close()

	book := sources.BookSource{Slug: "book/core", Title: "Test Book", DriveURL: fakeDriveURL}
	if err := store.RecordSourceBookProvenance(db, store.SourceBook{
		Slug: book.Slug, Title: book.Title, Version: "x", FileName: "x.pdf", FileSHA256: "x",
	}); err != nil {
		t.Fatal(err)
	}
	// Stored baseline is AFTER the server's Last-Modified, so nothing changed.
	if err := store.SetSourceBookLastModified(db, book.Slug, remoteMod.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	version := testVersion(t)
	if err := store.SetSourceBookIngestVersion(db, book.Slug, version); err != nil {
		t.Fatal(err)
	}

	res := checkOne(context.Background(), db, t.TempDir()+"/rules.db", t.TempDir(), clientFor(srv), version, book)
	if res.Action != "unchanged" {
		t.Errorf("Action = %q, want %q (err: %v)", res.Action, "unchanged", res.Err)
	}
	if downloadCalled {
		t.Error("Download should never be attempted for an unchanged book")
	}
}

// TestCheckOneVersionStaleForcesReingest pins the actual bug fix: even when
// Drive's own Last-Modified says nothing changed, a book whose stored
// ingest_schema_version doesn't match this binary's current one (here:
// never stamped at all, the real state of every pre-existing install on
// upgrade) must still force a real download+ingest — this is what makes a
// migration needing a content backfill (e.g. jutsu_upcast_rules) actually
// get populated on an existing install, not just structurally applied.
func TestCheckOneVersionStaleForcesReingest(t *testing.T) {
	db := testDB(t)
	remoteMod := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	pdfBody := []byte("%PDF-1.4\nfake\n")
	downloadCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteMod.Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			downloadCalled = true
			w.Write(pdfBody)
		}
	}))
	defer srv.Close()

	const testSlug = "book/fixture-version-stale"
	ingestCalled := false
	ingestDispatch[testSlug] = func(db *sql.DB, dbPath, pdfPath, version string) (*ingest.Report, error) {
		ingestCalled = true
		if err := store.RecordSourceBookProvenance(db, store.SourceBook{
			Slug: testSlug, Title: "Fixture Book", Version: version,
			FileName: "fixture.pdf", FileSHA256: "fixture-sha",
		}); err != nil {
			return nil, err
		}
		return &ingest.Report{Summary: "parsed 1 fixture"}, nil
	}
	t.Cleanup(func() { delete(ingestDispatch, testSlug) })

	book := sources.BookSource{Slug: testSlug, Title: "Fixture Book", DriveURL: fakeDriveURL}
	if err := store.RecordSourceBookProvenance(db, store.SourceBook{
		Slug: book.Slug, Title: book.Title, Version: "x", FileName: "x.pdf", FileSHA256: "x",
	}); err != nil {
		t.Fatal(err)
	}
	// Drive baseline is AFTER the server's Last-Modified — Drive itself says
	// unchanged. ingest_schema_version is deliberately left unstamped
	// (never called SetSourceBookIngestVersion), the real state of an
	// install that predates this migration.
	if err := store.SetSourceBookLastModified(db, book.Slug, remoteMod.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	res := checkOne(context.Background(), db, t.TempDir()+"/rules.db", t.TempDir(), clientFor(srv), testVersion(t), book)
	if res.Action != "updated" {
		t.Fatalf("Action = %q (err: %v), want %q — a stale ingest version must force a real re-ingest "+
			"even though Drive itself is unchanged", res.Action, res.Err, "updated")
	}
	if !downloadCalled || !ingestCalled {
		t.Errorf("downloadCalled=%v ingestCalled=%v, want both true", downloadCalled, ingestCalled)
	}

	gotVersion, found, err := store.GetSourceBookIngestVersion(db, testSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !found || gotVersion != testVersion(t) {
		t.Errorf("stamped ingest version = %q (found %v), want %q", gotVersion, found, testVersion(t))
	}
}

func TestCheckOneHeadError(t *testing.T) {
	db := testDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	book := sources.BookSource{Slug: "book/core", Title: "Test Book", DriveURL: fakeDriveURL}
	res := checkOne(context.Background(), db, t.TempDir()+"/rules.db", t.TempDir(), clientFor(srv), testVersion(t), book)
	if res.Action != "error" || res.Err == nil {
		t.Errorf("Action = %q, Err = %v; want error", res.Action, res.Err)
	}
}

func TestCheckOneDownloadError(t *testing.T) {
	db := testDB(t)
	remoteMod := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteMod.Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			w.Write([]byte("<html>virus scan warning</html>"))
		}
	}))
	defer srv.Close()

	book := sources.BookSource{Slug: "book/core", Title: "Test Book", DriveURL: fakeDriveURL}
	res := checkOne(context.Background(), db, t.TempDir()+"/rules.db", t.TempDir(), clientFor(srv), testVersion(t), book)
	if res.Action != "error" || res.Err == nil {
		t.Errorf("Action = %q, Err = %v; want error (non-PDF download)", res.Action, res.Err)
	}
}

func TestCheckOneNoParserYet(t *testing.T) {
	db := testDB(t)
	remoteMod := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	pdfBody := []byte("%PDF-1.4\nfake\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteMod.Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			w.Write(pdfBody)
		}
	}))
	defer srv.Close()

	// book/kage-guide has no entry in ingestDispatch — real, current state.
	book := sources.BookSource{Slug: "book/kage-guide", Title: "Kage Guide", DriveURL: fakeDriveURL}
	res := checkOne(context.Background(), db, t.TempDir()+"/rules.db", t.TempDir(), clientFor(srv), testVersion(t), book)
	if res.Action != "no parser yet" {
		t.Fatalf("Action = %q (err: %v), want %q", res.Action, res.Err, "no parser yet")
	}
	if res.Report != nil {
		t.Error("Report should be nil for a no-parser book")
	}

	// Provenance + a last-modified baseline must still be recorded, so this
	// book doesn't re-download in full on every future startup.
	var title, sha string
	if err := db.QueryRow(`SELECT title, file_sha256 FROM source_books WHERE slug = ?`, book.Slug).
		Scan(&title, &sha); err != nil {
		t.Fatalf("provenance row not recorded: %v", err)
	}
	if title != book.Title {
		t.Errorf("recorded title = %q, want %q", title, book.Title)
	}
	if sha == "" {
		t.Error("recorded file_sha256 is empty")
	}
	_, found, err := store.GetSourceBookLastModified(db, book.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("drive_last_modified was not stamped for the no-parser book")
	}
}

// TestCheckOneNoParserStaysOnDriveCadence confirms a no-parser book is
// NEVER subject to the version-staleness gate (it has no ingested content
// to go stale, and its ingest_schema_version is never stamped by design —
// see checkOne's hasParser guard) — it must settle into the plain
// Drive-timestamp-only cadence like it always has, not re-download on every
// single startup forever.
func TestCheckOneNoParserStaysOnDriveCadence(t *testing.T) {
	db := testDB(t)
	remoteMod := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	downloadCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteMod.Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			downloadCalled = true
		}
	}))
	defer srv.Close()

	book := sources.BookSource{Slug: "book/kage-guide", Title: "Kage Guide", DriveURL: fakeDriveURL}
	if err := store.RecordSourceBookProvenance(db, store.SourceBook{
		Slug: book.Slug, Title: book.Title, Version: "x", FileName: "x.pdf", FileSHA256: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSourceBookLastModified(db, book.Slug, remoteMod.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	res := checkOne(context.Background(), db, t.TempDir()+"/rules.db", t.TempDir(), clientFor(srv), testVersion(t), book)
	if res.Action != "unchanged" {
		t.Errorf("Action = %q, want %q (err: %v)", res.Action, "unchanged", res.Err)
	}
	if downloadCalled {
		t.Error("a no-parser book with no ingested content must never be forced to re-download by the version gate")
	}
}

func TestCheckOneUpdatedCallsRegisteredParser(t *testing.T) {
	db := testDB(t)
	remoteMod := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	pdfBody := []byte("%PDF-1.4\nfake\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteMod.Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			w.Write(pdfBody)
		}
	}))
	defer srv.Close()

	const testSlug = "book/fixture-for-tests"
	var gotPDFPath, gotVersion string
	ingestDispatch[testSlug] = func(db *sql.DB, dbPath, pdfPath, version string) (*ingest.Report, error) {
		gotPDFPath, gotVersion = pdfPath, version
		// Every real internal/ingest function creates its book's
		// source_books row internally via store.LoadX -> upsertSourceBook
		// before returning — mimicked here so this fake exercises the same
		// "row already exists by the time checkOne stamps last-modified"
		// precondition a real ingest always leaves behind.
		if err := store.RecordSourceBookProvenance(db, store.SourceBook{
			Slug: testSlug, Title: "Fixture Book", Version: version,
			FileName: "fixture.pdf", FileSHA256: "fixture-sha",
		}); err != nil {
			return nil, err
		}
		return &ingest.Report{Summary: "parsed 1 fixture"}, nil
	}
	t.Cleanup(func() { delete(ingestDispatch, testSlug) })

	book := sources.BookSource{Slug: testSlug, Title: "Fixture Book", DriveURL: fakeDriveURL}
	cacheDir := t.TempDir()
	currentVersion := testVersion(t)
	res := checkOne(context.Background(), db, t.TempDir()+"/rules.db", cacheDir, clientFor(srv), currentVersion, book)

	if res.Action != "updated" {
		t.Fatalf("Action = %q (err: %v), want %q", res.Action, res.Err, "updated")
	}
	if res.Report == nil || res.Report.Summary != "parsed 1 fixture" {
		t.Errorf("Report = %+v, want the fixture's canned report", res.Report)
	}
	if filepath.Dir(gotPDFPath) != cacheDir {
		t.Errorf("ingest was called with pdfPath %q, want it inside %q", gotPDFPath, cacheDir)
	}
	if gotVersion != "2025-06-15 (auto, drive)" {
		t.Errorf("version = %q, want the Drive-date-derived provenance string", gotVersion)
	}

	got, found, err := store.GetSourceBookLastModified(db, testSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !got.Equal(remoteMod) {
		t.Errorf("stamped last-modified = %v, found=%v; want %v, true", got, found, remoteMod)
	}

	gotIngestVersion, foundIngestVersion, err := store.GetSourceBookIngestVersion(db, testSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !foundIngestVersion || gotIngestVersion != currentVersion {
		t.Errorf("stamped ingest version = %q (found %v), want %q", gotIngestVersion, foundIngestVersion, currentVersion)
	}
}

func TestCheckOneUpdatedIngestError(t *testing.T) {
	db := testDB(t)
	remoteMod := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	pdfBody := []byte("%PDF-1.4\nfake\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteMod.Format(http.TimeFormat))
		if r.Method == http.MethodGet {
			w.Write(pdfBody)
		}
	}))
	defer srv.Close()

	const testSlug = "book/fixture-for-tests-err"
	ingestDispatch[testSlug] = func(db *sql.DB, dbPath, pdfPath, version string) (*ingest.Report, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { delete(ingestDispatch, testSlug) })

	book := sources.BookSource{Slug: testSlug, Title: "Fixture Book", DriveURL: fakeDriveURL}
	res := checkOne(context.Background(), db, t.TempDir()+"/rules.db", t.TempDir(), clientFor(srv), testVersion(t), book)
	if res.Action != "error" || res.Err == nil {
		t.Errorf("Action = %q, Err = %v; want an error result when the parser itself fails", res.Action, res.Err)
	}

	// A failed ingest must NOT stamp last-modified — a future startup should
	// retry, not treat this version as already handled.
	_, found, err := store.GetSourceBookLastModified(db, testSlug)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("last-modified was stamped despite the ingest failing")
	}
}

func TestRunOneResultPerBook(t *testing.T) {
	db := testDB(t)
	remoteMod := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", remoteMod.Format(http.TimeFormat))
	}))
	defer srv.Close()

	books := []sources.BookSource{
		{Slug: "book/core", Title: "A", DriveURL: fakeDriveURL},
		{Slug: "book/kage-guide", Title: "B", DriveURL: fakeDriveURL},
	}
	results := Run(context.Background(), db, t.TempDir()+"/rules.db", t.TempDir(), clientFor(srv), books)
	if len(results) != len(books) {
		t.Fatalf("got %d results, want %d", len(results), len(books))
	}
	for i, r := range results {
		if r.Slug != books[i].Slug {
			t.Errorf("result[%d].Slug = %q, want %q (order must match input)", i, r.Slug, books[i].Slug)
		}
	}
}
