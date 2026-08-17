package store

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/parse"
	"github.com/sergio/n5e/internal/schema"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Fire Release: Ember Toss":   "fire-release-ember-toss",
		"Summoner's Art: Bear!":      "summoners-art-bear",
		"8-Inner Gates: Seimon":      "8-inner-gates-seimon",
		"Ammo Heart [Name/ Changed]": "ammo-heart-name-changed",
		"  Padded  Name  ":           "padded-name",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

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

func sampleJutsu() parse.Jutsu {
	cost := 4
	return parse.Jutsu{
		Name:           "Fire Release: Ember Toss",
		Classification: "Ninjutsu",
		Rank:           "D",
		RankRaw:        "D-Rank",
		CastingTime:    "1 Action",
		Range:          "30 Feet",
		Duration:       "Instant",
		Components:     "HS, CM",
		CostChakra:     &cost,
		CostText:       "4 Chakra",
		Keywords:       "Fire Release, Ninjutsu",
		Description:    "You toss a mote of flame.",
		CategoryGroup:  "Ninjutsu / Fire Release",
		SourcePage:     84,
	}
}

var testBook = SourceBook{
	Slug: "book/jutsu-compendium", Title: "Jiraiya's Jutsu Compendium",
	Version: "3.1", FileName: "test.pdf", FileSHA256: "abc",
}

func TestLoadJutsuCreateThenUnchanged(t *testing.T) {
	db := testDB(t)
	batch := []parse.Jutsu{sampleJutsu()}

	r, err := LoadJutsu(db, testBook, batch, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 1 || r.Updated != 0 || r.Unchanged != 0 {
		t.Errorf("first load: %+v", r)
	}

	r, err = LoadJutsu(db, testBook, batch, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 0 || r.Updated != 0 || r.Unchanged != 1 {
		t.Errorf("second load must be a no-op: %+v", r)
	}

	var kw int
	if err := db.QueryRow(`SELECT COUNT(*) FROM jutsu_keywords WHERE jutsu_slug = 'jutsu/fire-release-ember-toss'`).Scan(&kw); err != nil {
		t.Fatal(err)
	}
	if kw != 2 {
		t.Errorf("got %d keyword rows, want 2", kw)
	}
}

// The load must never touch overrides or notes, and must demote a
// human-'verified' row back to needs_review when its parsed content changes.
func TestLoadJutsuPreservesOverridesAndDemotesVerified(t *testing.T) {
	db := testDB(t)
	if _, err := LoadJutsu(db, testBook, []parse.Jutsu{sampleJutsu()}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE jutsu SET rank_override = 'C', detection_status = 'verified',
		                 notes = 'checked against print'
		WHERE slug = 'jutsu/fire-release-ember-toss'`); err != nil {
		t.Fatal(err)
	}

	// Re-ingest with unchanged content: verified stands.
	if _, err := LoadJutsu(db, testBook, []parse.Jutsu{sampleJutsu()}, nil); err != nil {
		t.Fatal(err)
	}
	var status, override, notes string
	row := db.QueryRow(`SELECT detection_status, rank_override, notes FROM jutsu WHERE slug = 'jutsu/fire-release-ember-toss'`)
	if err := row.Scan(&status, &override, &notes); err != nil {
		t.Fatal(err)
	}
	if status != "verified" || override != "C" || notes != "checked against print" {
		t.Errorf("unchanged re-ingest disturbed human data: status=%q override=%q notes=%q", status, override, notes)
	}

	// Re-ingest with changed content: parsed column updates, verified demotes,
	// override and notes still untouched.
	changed := sampleJutsu()
	changed.Description = "You toss a bigger mote of flame."
	r, err := LoadJutsu(db, testBook, []parse.Jutsu{changed}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Demoted) != 1 {
		t.Errorf("Demoted = %v, want the one changed slug", r.Demoted)
	}
	row = db.QueryRow(`SELECT detection_status, rank_override, notes, description FROM jutsu WHERE slug = 'jutsu/fire-release-ember-toss'`)
	var desc string
	if err := row.Scan(&status, &override, &notes, &desc); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" {
		t.Errorf("changed verified row must demote to needs_review, got %q", status)
	}
	if override != "C" || notes != "checked against print" {
		t.Errorf("override/notes disturbed: %q %q", override, notes)
	}
	if desc != changed.Description {
		t.Errorf("description not updated: %q", desc)
	}
}

func TestLoadJutsuFlagsAnomalousEntries(t *testing.T) {
	db := testDB(t)
	j := sampleJutsu()
	anoms := []parse.Anomaly{{Page: 84, Subject: j.Name, Problem: "missing field Description"}}
	r, err := LoadJutsu(db, testBook, []parse.Jutsu{j}, anoms)
	if err != nil {
		t.Fatal(err)
	}
	if r.NeedsReview != 1 {
		t.Errorf("NeedsReview = %d, want 1", r.NeedsReview)
	}
	var status string
	if err := db.QueryRow(`SELECT detection_status FROM jutsu WHERE slug = 'jutsu/fire-release-ember-toss'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "needs_review" {
		t.Errorf("status = %q, want needs_review", status)
	}
}

func TestLoadJutsuReportsVanishedAndDuplicates(t *testing.T) {
	db := testDB(t)
	first := sampleJutsu()
	second := sampleJutsu()
	second.Name = "Fire Release: Great Ember Toss"
	if _, err := LoadJutsu(db, testBook, []parse.Jutsu{first, second}, nil); err != nil {
		t.Fatal(err)
	}

	// Next batch drops the second entry and adds a slug collision.
	collides := sampleJutsu()
	collides.Name = "FIRE RELEASE: EMBER TOSS" // different text, same slug
	r, err := LoadJutsu(db, testBook, []parse.Jutsu{first, collides}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Duplicates) != 1 {
		t.Errorf("Duplicates = %v, want 1 collision", r.Duplicates)
	}
	if len(r.Vanished) != 1 || r.Vanished[0] != "jutsu/fire-release-great-ember-toss" {
		t.Errorf("Vanished = %v, want the dropped slug", r.Vanished)
	}

	// Vanished rows are reported, never deleted.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM jutsu`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("vanished row was deleted: %d rows, want 2", count)
	}
}

func TestSourceBookLastModifiedUnknownBeforeSet(t *testing.T) {
	db := testDB(t)
	if _, err := LoadJutsu(db, testBook, []parse.Jutsu{sampleJutsu()}, nil); err != nil {
		t.Fatal(err)
	}
	// A row exists (from the ingest above) but drive_last_modified is NULL —
	// this is the state every hand-run n5e-ingest leaves a book in.
	_, found, err := GetSourceBookLastModified(db, testBook.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("found = true for a book with no drive_last_modified stamp yet, want false")
	}

	// A slug with no source_books row at all (never ingested) must behave
	// the same way, not error.
	_, found, err = GetSourceBookLastModified(db, "book/never-ingested")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("found = true for a slug with no source_books row, want false")
	}
}

func TestSetThenGetSourceBookLastModified(t *testing.T) {
	db := testDB(t)
	if _, err := LoadJutsu(db, testBook, []parse.Jutsu{sampleJutsu()}, nil); err != nil {
		t.Fatal(err)
	}

	want := time.Date(2025, 5, 4, 8, 30, 0, 0, time.UTC)
	if err := SetSourceBookLastModified(db, testBook.Slug, want); err != nil {
		t.Fatal(err)
	}

	got, found, err := GetSourceBookLastModified(db, testBook.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("found = false after SetSourceBookLastModified, want true")
	}
	if !got.Equal(want) {
		t.Errorf("GetSourceBookLastModified = %v, want %v", got, want)
	}
}

func TestSourceBookIngestVersionUnknownBeforeSet(t *testing.T) {
	db := testDB(t)
	if _, err := LoadJutsu(db, testBook, []parse.Jutsu{sampleJutsu()}, nil); err != nil {
		t.Fatal(err)
	}
	// A row exists (from the ingest above) but ingest_schema_version is
	// NULL — the state every book ingested before this stamp existed (or by
	// a hand-run n5e-ingest) is left in.
	_, found, err := GetSourceBookIngestVersion(db, testBook.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("found = true for a book with no ingest_schema_version stamp yet, want false")
	}

	_, found, err = GetSourceBookIngestVersion(db, "book/never-ingested")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("found = true for a slug with no source_books row, want false")
	}
}

func TestSetThenGetSourceBookIngestVersion(t *testing.T) {
	db := testDB(t)
	if _, err := LoadJutsu(db, testBook, []parse.Jutsu{sampleJutsu()}, nil); err != nil {
		t.Fatal(err)
	}

	const want = "0026_source_books_ingest_version.sql"
	if err := SetSourceBookIngestVersion(db, testBook.Slug, want); err != nil {
		t.Fatal(err)
	}

	got, found, err := GetSourceBookIngestVersion(db, testBook.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("found = false after SetSourceBookIngestVersion, want true")
	}
	if got != want {
		t.Errorf("GetSourceBookIngestVersion = %q, want %q", got, want)
	}
}

func TestLoadJutsuSkipsRanklessEntry(t *testing.T) {
	db := testDB(t)
	j := sampleJutsu()
	j.Rank = ""
	r, err := LoadJutsu(db, testBook, []parse.Jutsu{j}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Skipped) != 1 || r.Created != 0 {
		t.Errorf("rankless entry must be skipped and reported: %+v", r)
	}
}
