package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/schema"
)

// testCharDB opens a real file-backed characters.db (not :memory: — Create
// exercises VACUUM INTO, a real file-to-file operation, and
// ApplyPendingRestore exercises os.Rename, neither of which an in-memory
// database can stand in for) with the real migrations applied, plus one
// character so a restored snapshot has something to assert against.
func testCharDB(t *testing.T, dir, name string) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := schema.Apply(db, schema.Characters); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO characters (name) VALUES ('Sakura')`); err != nil {
		t.Fatal(err)
	}
	return db, path
}

func TestCreateAndList(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	db, _ := testCharDB(t, dir, "characters.db")

	path, err := Create(db, backupDir, ReasonManual)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}

	entries, err := List(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Reason != ReasonManual {
		t.Errorf("reason = %q, want %q", entries[0].Reason, ReasonManual)
	}
	if entries[0].Size == 0 {
		t.Error("expected nonzero size")
	}

	// The snapshot is a real, independent, openable database.
	snap, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	var name string
	if err := snap.QueryRow(`SELECT name FROM characters`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Sakura" {
		t.Errorf("restored name = %q, want Sakura", name)
	}
}

func TestListSkipsUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"notes.txt", "characters.db", "characters-bad-format.db"} {
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := List(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected malformed filenames to be skipped, got %v", entries)
	}
}

func TestListOnMissingDirIsEmptyNotError(t *testing.T) {
	entries, err := List(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Errorf("expected nil, got %v", entries)
	}
}

func TestPruneKeepsOnlyMostRecent(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write MaxKept+3 snapshots directly (bypassing Create, which refuses to
	// collide on a same-second timestamp) with strictly increasing
	// timestamps baked into the filename.
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	total := MaxKept + 3
	for i := 0; i < total; i++ {
		ts := base.Add(time.Duration(i) * time.Minute).Format(fileTimeFormat)
		name := "characters-" + ts + "-periodic.db"
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := Prune(backupDir); err != nil {
		t.Fatal(err)
	}
	entries, err := List(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != MaxKept {
		t.Fatalf("expected %d entries after prune, got %d", MaxKept, len(entries))
	}
	// The newest MaxKept survive — the very last one written (i == total-1)
	// must still be there, and the earliest ones must be gone.
	newest := base.Add(time.Duration(total-1) * time.Minute)
	if !entries[0].When.Equal(newest) {
		t.Errorf("newest surviving entry = %v, want %v", entries[0].When, newest)
	}
	oldestSurviving := base.Add(time.Duration(total-MaxKept) * time.Minute)
	for _, e := range entries {
		if e.When.Before(oldestSurviving) {
			t.Errorf("entry %v should have been pruned (cutoff %v)", e.When, oldestSurviving)
		}
	}
}

func TestStageAndApplyRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	db, charDBPath := testCharDB(t, dir, "characters.db")

	if _, err := Create(db, backupDir, ReasonManual); err != nil {
		t.Fatal(err)
	}
	entries, err := List(backupDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %v, err = %v", entries, err)
	}
	backupName := entries[0].Filename

	// Change the live character after the snapshot, so restoring is
	// observably different from doing nothing.
	if _, err := db.Exec(`UPDATE characters SET name = 'Sasuke' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}

	if err := StageRestore(backupDir, backupName, charDBPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(charDBPath + pendingSuffix); err != nil {
		t.Fatalf("pending marker not written: %v", err)
	}

	// The live DB is untouched until the next launch applies it — this
	// mirrors the running server, which still has charDBPath open via db.
	var stillLive string
	if err := db.QueryRow(`SELECT name FROM characters WHERE id = 1`).Scan(&stillLive); err != nil {
		t.Fatal(err)
	}
	if stillLive != "Sasuke" {
		t.Fatalf("live db changed before restart: %q", stillLive)
	}

	// Simulate the next launch: nothing has charDBPath open at this point.
	db.Close()
	if err := ApplyPendingRestore(charDBPath, backupDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(charDBPath + pendingSuffix); !os.IsNotExist(err) {
		t.Errorf("pending marker should be gone after apply, stat err = %v", err)
	}

	restored, err := sql.Open("sqlite", charDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var name string
	if err := restored.QueryRow(`SELECT name FROM characters WHERE id = 1`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Sakura" {
		t.Errorf("restored name = %q, want Sakura", name)
	}

	// A pre-restore safety snapshot of the pre-restore ("Sasuke") state must
	// exist so the restore itself is undoable.
	entries, err = List(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	foundPrerestore := false
	for _, e := range entries {
		if e.Reason == ReasonPrerestore {
			foundPrerestore = true
		}
	}
	if !foundPrerestore {
		t.Error("expected a prerestore safety snapshot after applying a restore")
	}
}

func TestStageRestoreRejectsUnknownFilename(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	_, charDBPath := testCharDB(t, dir, "characters.db")

	// Neither a nonexistent name nor a path-traversal attempt should be
	// accepted — both fail the same way, by not being found in List.
	for _, bad := range []string{"characters-20260101-000000-manual.db", "../../etc/passwd", "characters.db"} {
		if err := StageRestore(backupDir, bad, charDBPath); err == nil {
			t.Errorf("StageRestore(%q) should have failed", bad)
		}
	}
}

func TestApplyPendingRestoreNoopWhenNothingStaged(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	_, charDBPath := testCharDB(t, dir, "characters.db")

	if err := ApplyPendingRestore(charDBPath, backupDir); err != nil {
		t.Fatal(err)
	}
	entries, err := List(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no snapshots taken, got %v", entries)
	}
}
