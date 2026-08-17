package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/backup"
	"github.com/sergio/n5e/internal/schema"
)

// backupTestServer is testServer's file-backed counterpart: the backups
// handlers exercise real file operations (VACUUM INTO, staging a restore
// marker next to characters.db) that an in-memory DSN can't stand in for.
func backupTestServer(t *testing.T) *server {
	t.Helper()
	dir := t.TempDir()
	charDBPath := filepath.Join(dir, "characters.db")

	charDB, err := sql.Open("sqlite", charDBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { charDB.Close() })
	if err := schema.Apply(charDB, schema.Characters); err != nil {
		t.Fatal(err)
	}

	return &server{
		charDB:     charDB,
		charDBPath: charDBPath,
		backupDir:  backup.Dir(dir),
		shutdown:   make(chan struct{}),
	}
}

func TestHandleBackupsListsAndBacksUpNow(t *testing.T) {
	s := backupTestServer(t)

	// Empty state renders without a backup ever having been taken.
	req := httptest.NewRequest(http.MethodGet, "/backups", nil)
	w := httptest.NewRecorder()
	s.handleBackups(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /backups status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "No backups yet") {
		t.Error("expected empty-state message")
	}

	// "Back Up Now" creates one and redirects back.
	req = httptest.NewRequest(http.MethodPost, "/backups", nil)
	w = httptest.NewRecorder()
	s.handleBackupCreate(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST /backups status = %d, body:\n%s", w.Code, w.Body.String())
	}

	entries, err := backup.List(s.backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Reason != backup.ReasonManual {
		t.Fatalf("entries = %v", entries)
	}

	req = httptest.NewRequest(http.MethodGet, "/backups", nil)
	w = httptest.NewRecorder()
	s.handleBackups(w, req)
	if !strings.Contains(w.Body.String(), "Manual") {
		t.Errorf("expected the manual backup to be listed, body:\n%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Restore this backup") {
		t.Error("expected a restore button")
	}
}

func TestHandleBackupRestoreStagesAndShutsDown(t *testing.T) {
	s := backupTestServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Sakura')`); err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Create(s.charDB, s.backupDir, backup.ReasonManual); err != nil {
		t.Fatal(err)
	}
	entries, err := backup.List(s.backupDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %v, err = %v", entries, err)
	}
	filename := entries[0].Filename

	req := httptest.NewRequest(http.MethodPost, "/backups/"+filename+"/restore", nil)
	req.SetPathValue("filename", filename)
	w := httptest.NewRecorder()
	s.handleBackupRestore(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "relaunch") {
		t.Errorf("expected a relaunch notice, body:\n%s", w.Body.String())
	}

	select {
	case <-s.shutdown:
	default:
		t.Error("expected s.shutdown to be closed after staging a restore")
	}

	if _, err := os.Stat(s.charDBPath + ".pending-restore"); err != nil {
		t.Errorf("pending restore marker not written: %v", err)
	}
}

func TestHandleBackupRestoreRejectsUnknownFilename(t *testing.T) {
	s := backupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/backups/does-not-exist.db/restore", nil)
	req.SetPathValue("filename", "does-not-exist.db")
	w := httptest.NewRecorder()
	s.handleBackupRestore(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no such backup") {
		t.Errorf("expected an error message, body:\n%s", w.Body.String())
	}
	select {
	case <-s.shutdown:
		t.Error("s.shutdown should not close on a rejected restore")
	default:
	}
}
