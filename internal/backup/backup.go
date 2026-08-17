// Package backup manages point-in-time snapshots of characters.db and the
// staged-restore handoff between a running server and the next launch.
//
// characters.db lives next to the executable and is never anything other
// than the player's real, live data — see cmd/n5e/main.go's own doc comment.
// This package exists so a mistake (an errant DELETE, a bad migration, a
// stray `rm` during development — see git history around 2026-08-01 for
// exactly that) costs at most a few minutes of play, not the character.
package backup

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Reason tags why a snapshot was taken, folded into its filename so List can
// label entries without a sidecar file.
const (
	ReasonPeriodic   = "periodic"   // the 10-minute autosave loop
	ReasonClose      = "close"      // the app shutting down (tab closed, Ctrl+C)
	ReasonManual     = "manual"     // the player's own "Back Up Now" button
	ReasonPrerestore = "prerestore" // safety copy of whatever restore is about to overwrite
	ReasonPreReset   = "prereset"   // safety copy taken right before a Reset Levels retcon
)

// fileTimeFormat is second-resolution: two snapshots requested in the same
// second (only realistically a test or a rapid manual click) collide on
// filename, and the second Create call fails rather than silently
// overwriting the first — see Create's own comment.
const fileTimeFormat = "20060102-150405"

// MaxKept caps how many snapshots Prune leaves behind after a new one is
// written. Ten-minute periodic snapshots plus one on every tab close adds up
// over a long session; capping by count rather than age keeps the policy
// simple, and the on-disk cost of even 50 of them is trivial next to the
// value of not losing a character.
const MaxKept = 50

// pendingSuffix names the staged-restore marker written next to
// characters.db by StageRestore and consumed by ApplyPendingRestore.
const pendingSuffix = ".pending-restore"

// Dir returns the backups folder for an app rooted at exeDir (the directory
// characters.db itself lives in) — same portable-folder model as
// characters.db and rules.db: copy the app folder and its backups travel
// with it.
func Dir(exeDir string) string {
	return filepath.Join(exeDir, "backups")
}

// Entry describes one snapshot on disk, as listed by List.
type Entry struct {
	Filename string
	Reason   string
	When     time.Time
	Size     int64
}

// Create writes a consistent point-in-time snapshot of charDB into dir and
// returns its path. Callers that want the retention policy enforced call
// Prune afterward — kept as a separate step so a snapshot that succeeds
// still counts as a success even if pruning has a problem.
//
// Uses SQLite's own VACUUM INTO rather than copying the file on disk: a raw
// copy racing a live writer, or reading a database that isn't in
// rollback-journal mode's quiescent state, can produce a torn, unopenable
// snapshot. VACUUM INTO always sees one consistent transaction and refuses
// to overwrite an existing destination file, which is exactly the collision
// behavior wanted here.
func Create(charDB *sql.DB, dir, reason string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}
	name := fmt.Sprintf("characters-%s-%s.db", time.Now().UTC().Format(fileTimeFormat), reason)
	path := filepath.Join(dir, name)
	if _, err := charDB.Exec(`VACUUM INTO ?`, path); err != nil {
		return "", fmt.Errorf("vacuum into %s: %w", path, err)
	}
	return path, nil
}

// Prune deletes the oldest snapshots in dir beyond MaxKept.
func Prune(dir string) error {
	entries, err := List(dir)
	if err != nil {
		return err
	}
	if len(entries) <= MaxKept {
		return nil
	}
	var firstErr error
	for _, e := range entries[MaxKept:] {
		if err := os.Remove(filepath.Join(dir, e.Filename)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// List returns every snapshot in dir, newest first. A missing dir (no
// backup has ever been taken) returns an empty list, not an error.
// Filenames Create didn't write are skipped rather than failing the whole
// listing — a user poking around in the folder shouldn't be able to break
// the restore page.
func List(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading backup dir: %w", err)
	}
	var out []Entry
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		when, reason, ok := parseFilename(de.Name())
		if !ok {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{Filename: de.Name(), Reason: reason, When: when, Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	return out, nil
}

// parseFilename recovers the timestamp and reason Create encoded into a
// snapshot's name (e.g. "characters-20260801-153000-periodic.db").
func parseFilename(name string) (time.Time, string, bool) {
	base := strings.TrimSuffix(name, ".db")
	rest, ok := strings.CutPrefix(base, "characters-")
	if !ok {
		return time.Time{}, "", false
	}
	parts := strings.SplitN(rest, "-", 3)
	if len(parts) != 3 {
		return time.Time{}, "", false
	}
	when, err := time.ParseInLocation(fileTimeFormat, parts[0]+"-"+parts[1], time.UTC)
	if err != nil {
		return time.Time{}, "", false
	}
	return when, parts[2], true
}

// StageRestore validates filename (resolved by matching it against what List
// actually finds in dir, which also rules out any path traversal from a
// user-supplied form value — a bare filename that isn't in the listing is
// rejected outright) and copies it next to charDBPath under pendingSuffix,
// ready for ApplyPendingRestore on the next launch.
//
// Restoring can't safely replace the live database file out from under the
// *sql.DB the running server already has open — on Windows in particular,
// an open file can't be overwritten at all — so restore is a two-step
// handoff: stage now, apply at the very start of the next launch, before
// anything has opened characters.db.
func StageRestore(dir, filename, charDBPath string) error {
	entries, err := List(dir)
	if err != nil {
		return err
	}
	found := false
	for _, e := range entries {
		if e.Filename == filename {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no such backup: %s", filename)
	}

	src := filepath.Join(dir, filename)
	if err := validateSnapshot(src); err != nil {
		return fmt.Errorf("backup failed validation: %w", err)
	}
	return copyFile(src, charDBPath+pendingSuffix)
}

// validateSnapshot opens path read-only and checks it's an intact
// characters.db, so a truncated or corrupted backup is rejected at
// stage-time — while the player can still pick a different one — rather
// than discovered on the next launch, when it's too late.
func validateSnapshot(path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()

	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&count); err != nil {
		return fmt.Errorf("not a characters database: %w", err)
	}
	return nil
}

// ApplyPendingRestore must run before anything opens charDBPath. If
// StageRestore left a marker there, it snapshots whatever's currently at
// charDBPath (reason ReasonPrerestore — the safety net for a restore chosen
// by mistake) and swaps the staged file into place.
//
// Uses a plain file copy for the pre-restore safety snapshot rather than
// Create's VACUUM INTO: this runs before charDB is opened, so there is no
// live *sql.DB to VACUUM FROM, and no concurrent writer to race either —
// the file is exactly as safe to copy directly as it will ever be.
func ApplyPendingRestore(charDBPath, dir string) error {
	pendingPath := charDBPath + pendingSuffix
	if _, err := os.Stat(pendingPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checking pending restore: %w", err)
	}

	if _, err := os.Stat(charDBPath); err == nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating backup dir: %w", err)
		}
		name := fmt.Sprintf("characters-%s-%s.db", time.Now().UTC().Format(fileTimeFormat), ReasonPrerestore)
		if err := copyFile(charDBPath, filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("backing up current database before restore: %w", err)
		}
		if err := Prune(dir); err != nil {
			return fmt.Errorf("pruning backups after pre-restore snapshot: %w", err)
		}
	}

	if err := os.Rename(pendingPath, charDBPath); err != nil {
		return fmt.Errorf("applying staged restore: %w", err)
	}
	return nil
}

// copyFile writes src to dst via a temp file in dst's own directory plus a
// rename, so a reader can never observe a partially-written dst — either the
// old file is still there, or the whole new one is.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := out.Name()
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
