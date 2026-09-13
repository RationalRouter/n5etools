package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestRulesDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openRulesDB(filepath.Join(t.TempDir(), "rules.db"))
	if err != nil {
		t.Fatalf("openRulesDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTableHasColumn(t *testing.T) {
	db := newTestRulesDB(t)
	if !tableHasColumn(db, "jutsu", "source_book") {
		t.Error("jutsu should have a source_book column")
	}
	if tableHasColumn(db, "jutsu", "not_a_real_column") {
		t.Error("jutsu should not have not_a_real_column")
	}
	if tableHasColumn(db, "not_a_real_table", "source_book") {
		t.Error("a nonexistent table should report no columns, not error out")
	}
}

func TestLoadEnglishValuesBookScoped(t *testing.T) {
	db := newTestRulesDB(t)
	if _, err := db.Exec(`
		INSERT INTO source_books (slug, title, version, file_name, file_sha256)
		VALUES ('book/a', 'A', '1', 'a.pdf', 'x'), ('book/b', 'B', '1', 'b.pdf', 'y')`); err != nil {
		t.Fatalf("seeding source_books: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description, source_book)
		VALUES ('jutsu/one', 'One', 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', '1 Chakra', 'Ninjutsu', 'desc one', 'book/a'),
		       ('jutsu/two', 'Two', 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', '1 Chakra', 'Ninjutsu', 'desc two', 'book/b')`); err != nil {
		t.Fatalf("seeding jutsu: %v", err)
	}

	all, err := loadEnglishValues(db, "jutsu", "")
	if err != nil {
		t.Fatalf("loadEnglishValues (unscoped): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unscoped load = %d rows, want 2", len(all))
	}

	scoped, err := loadEnglishValues(db, "jutsu", "book/a")
	if err != nil {
		t.Fatalf("loadEnglishValues (scoped): %v", err)
	}
	if len(scoped) != 1 || scoped[0].Key != "jutsu/one" {
		t.Fatalf("scoped load = %+v, want exactly jutsu/one", scoped)
	}
}

func TestApplyImportRecords(t *testing.T) {
	db := newTestRulesDB(t)
	if _, err := db.Exec(`
		INSERT INTO source_books (slug, title, version, file_name, file_sha256)
		VALUES ('book/a', 'A', '1', 'a.pdf', 'x')`); err != nil {
		t.Fatalf("seeding source_books: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description, source_book)
		VALUES ('jutsu/one', 'One', 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', '1 Chakra', 'Ninjutsu', 'desc one', 'book/a')`); err != nil {
		t.Fatalf("seeding jutsu: %v", err)
	}

	records := []importRecord{
		{Table: "jutsu", Slug: "jutsu/one", Field: "name", EnHash: "h1", Value: "Uno"},
	}
	n, err := applyImportRecords(db, "es", "machine", records)
	if err != nil {
		t.Fatalf("applyImportRecords: %v", err)
	}
	if n != 1 {
		t.Fatalf("applyImportRecords applied %d, want 1", n)
	}

	var value, status string
	if err := db.QueryRow(`SELECT value, status FROM content_translations WHERE table_name = 'jutsu' AND slug = 'jutsu/one' AND locale = 'es' AND field = 'name'`).Scan(&value, &status); err != nil {
		t.Fatalf("reading applied row: %v", err)
	}
	if value != "Uno" || status != "machine" {
		t.Errorf("applied row = (%q, %q), want (Uno, machine)", value, status)
	}

	if _, err := applyImportRecords(db, "es", "bogus-status", records); err == nil {
		t.Error("applyImportRecords should reject an unrecognized status")
	}
}
