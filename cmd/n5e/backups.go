// The Backups page: manual snapshots on top of the automatic ones
// (main.go's periodic loop and shutdown hook), and restoring one back.
//
// Restoring can't safely swap files under the live *sql.DB the running
// server already has open (see internal/backup's doc comment), so
// handleBackupRestore only stages the choice and then shuts the server down
// — the same "closing the tab is the whole UX" shape the rest of the app
// already uses, just triggered from the server side instead of the browser
// closing the connection. internal/backup.ApplyPendingRestore finishes the
// job the next time the app launches.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/sergio/n5e/internal/backup"
)

type backupRow struct {
	Filename    string
	ReasonLabel string
	WhenLabel   string
	SizeLabel   string
}

var backupReasonLabels = map[string]string{
	backup.ReasonPeriodic:   "Automatic (10-minute)",
	backup.ReasonClose:      "Automatic (app closed)",
	backup.ReasonManual:     "Manual",
	backup.ReasonPrerestore: "Safety copy (before a restore)",
}

func backupReasonLabel(reason string) string {
	if label, ok := backupReasonLabels[reason]; ok {
		return label
	}
	return reason
}

// humanSize renders a byte count the way a player picking a restore point
// cares about — one or two significant figures, not exact bytes.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (s *server) loadBackupRows() ([]backupRow, error) {
	entries, err := backup.List(s.backupDir)
	if err != nil {
		return nil, err
	}
	rows := make([]backupRow, len(entries))
	for i, e := range entries {
		rows[i] = backupRow{
			Filename:    e.Filename,
			ReasonLabel: backupReasonLabel(e.Reason),
			WhenLabel:   e.When.Local().Format("Jan 2, 2006 3:04 PM"),
			SizeLabel:   humanSize(e.Size),
		}
	}
	return rows, nil
}

func (s *server) handleBackups(w http.ResponseWriter, r *http.Request) {
	rows, err := s.loadBackupRows()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("list backups:", err)
		return
	}
	s.render(w, "backups.html", map[string]any{"Title": "Backups", "Backups": rows, "MaxKept": backup.MaxKept})
}

// handleBackupCreate is the "Back Up Now" button — a snapshot on demand,
// same mechanism and same retention policy as the automatic ones.
func (s *server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	if _, err := backup.Create(s.charDB, s.backupDir, backup.ReasonManual); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("manual backup:", err)
		return
	}
	if err := backup.Prune(s.backupDir); err != nil {
		log.Println("pruning backups:", err)
	}
	http.Redirect(w, r, "/backups", http.StatusSeeOther)
}

// handleBackupRestore stages filename as the next launch's database and
// shuts the server down — see the package doc comment above for why it
// can't just swap the file in live. Guarded client-side by confirm-submit.js
// (data-confirm), same convention handleDeleteCharacter uses.
func (s *server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if err := backup.StageRestore(s.backupDir, filename, s.charDBPath); err != nil {
		rows, listErr := s.loadBackupRows()
		if listErr != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("list backups after failed restore:", listErr)
			return
		}
		log.Println("stage restore:", err)
		s.render(w, "backups.html", map[string]any{
			"Title": "Backups", "Backups": rows, "MaxKept": backup.MaxKept,
			"Error": "Couldn't stage that backup for restore: " + err.Error(),
		})
		return
	}

	// Same shutdown path watchHeartbeat uses. The response below still
	// reaches the browser: main.go's httpServer.Shutdown waits for this
	// in-flight handler to return before the listener actually closes.
	s.shutdownOnce.Do(func() { close(s.shutdown) })

	s.render(w, "backups.html", map[string]any{
		"Title": "Backups", "RestoreStaged": true,
	})
}
