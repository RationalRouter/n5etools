// n5e is the player-facing app: character creation, character sheet, dice.
// It runs as a local web server and opens the result in the user's default
// browser — chosen over a native GUI toolkit for a clean path to two
// planned v2 features (multiplayer, Roll20 integration) that both need a
// client-server shape, and because it's the most direct way to hit a
// modern, Roll20/D&D Beyond-style look using real web tech.
//
// On startup it creates (or opens) characters.db NEXT TO THE EXECUTABLE —
// the portable-folder model, copy the folder and your characters travel
// too — and brings it up to date with schema migrations. The rules
// database (jutsu, clans, classes, ...) ships embedded inside the binary;
// see internal/embedded and rulesdb.go.
//
// This binary is also the antivirus canary: a Windows cross-build goes to
// VirusTotal to confirm that a static Go binary with embedded SQLite doesn't
// trip false positives (a known problem with PyInstaller-style packaging
// this stack was chosen to avoid).
//
// Startup failures (e.g. can't create characters.db) print to stderr AND
// show a native message box (see errdialog_windows.go) — stderr alone would
// be invisible once Windows builds add -H=windowsgui to suppress the
// console window per the "zero CLI surface" requirement.
package main

// Generates cmd/n5e/resource_windows.syso (app icon + version info for the
// Windows build) from versioninfo.json — requires assets/n5e.ico to exist
// first (not committed; supply your own). The .syso is picked up
// automatically by the linker on Windows builds only, per Go's standard
// _windows filename build-constraint convention, and is a no-op/missing-file
// skip on other platforms since go generate isn't run by `go build` itself.
//go:generate go tool goversioninfo -o=resource_windows.syso

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/autoupdate"
	"github.com/sergio/n5e/internal/backup"
	"github.com/sergio/n5e/internal/schema"
	"github.com/sergio/n5e/internal/sources"
)

// backupInterval is how often the running server snapshots characters.db in
// the background, independent of the shutdown-time snapshot in run() below.
// Ten minutes bounds how much play a crash (as opposed to an orderly tab
// close, which the shutdown path already covers) can cost.
const backupInterval = 10 * time.Minute

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		fatalDialog("N5E Toolkit — Startup Failed", err.Error())
		os.Exit(1)
	}
}

func run() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating executable: %w", err)
	}
	charDBPath := filepath.Join(filepath.Dir(exe), "characters.db")
	backupDir := backup.Dir(filepath.Dir(exe))

	// Must run before characters.db is opened at all: a restore staged from
	// the Backups page (see cmd/n5e/backups.go) can't safely replace the
	// live file out from under an already-open *sql.DB — see
	// internal/backup's own doc comment — so it's staged as a marker file
	// and only actually swapped in here, at the one moment nothing has
	// characters.db open yet.
	if err := backup.ApplyPendingRestore(charDBPath, backupDir); err != nil {
		return fmt.Errorf("applying staged restore: %w", err)
	}

	// busy_timeout: this app's own request handlers open short-lived write
	// transactions (internal/charstore) that can occasionally overlap
	// (e.g. two quick successive form submits) — without a busy timeout,
	// SQLite's default is to fail immediately with SQLITE_BUSY on any lock
	// conflict rather than wait, which is exactly what a "database is
	// locked" error in the wild traced back to. modernc.org/sqlite reads
	// _pragma params straight out of the DSN (conn.go's newConn splits on
	// the first "?" whether or not the path is "file:"-prefixed), applied
	// to every pooled connection as it's opened, not just a single Exec
	// against whichever connection happens to be handed out first.
	charDB, err := sql.Open("sqlite", charDBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("opening %s: %w", charDBPath, err)
	}
	defer charDB.Close()
	if err := schema.Apply(charDB, schema.Characters); err != nil {
		return fmt.Errorf("preparing character database: %w", err)
	}

	rulesDBPath := filepath.Join(filepath.Dir(exe), "rules.db")
	rulesDB, err := openRulesDB(rulesDBPath)
	if err != nil {
		return fmt.Errorf("preparing rules database: %w", err)
	}
	defer rulesDB.Close()

	token, err := newToken()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting local server: %w", err)
	}
	addr := listener.Addr().String() // "127.0.0.1:PORT" — port was OS-assigned
	baseURL := "http://" + addr

	srv := &server{
		rulesDB:        rulesDB,
		charDB:         charDB,
		charDBPath:     charDBPath,
		backupDir:      backupDir,
		token:          token,
		expectedOrigin: baseURL,
		shutdown:       make(chan struct{}),
	}
	httpServer := &http.Server{Handler: srv.routes()}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Println("server error:", err)
		}
	}()

	// Backgrounded, never blocks startup: check every official sourcebook
	// for a newer Drive version and re-ingest anything that changed,
	// entirely unattended. cacheDir is OS temp, not this app's own folder —
	// each book's downloaded PDF is scratch space, removed again right
	// after that book's ingest attempt.
	go func() {
		cacheDir := filepath.Join(os.TempDir(), "n5e-autoupdate")
		for _, r := range autoupdate.Run(context.Background(), rulesDB, rulesDBPath, cacheDir, nil, sources.OfficialBooks) {
			switch r.Action {
			case "unchanged":
				// quiet — this is the expected outcome on almost every startup
			case "updated":
				summary := r.Report.Summary
				if i := strings.IndexByte(summary, '\n'); i >= 0 {
					summary = summary[:i]
				}
				log.Printf("auto-update: %s — updated (%s)", r.Title, summary)
			case "no parser yet":
				log.Printf("auto-update: %s — downloaded, no parser yet", r.Title)
			case "error":
				log.Printf("auto-update: %s — %v", r.Title, r.Err)
			}
		}
	}()

	launchURL := baseURL + "/?token=" + token
	if err := openBrowser(launchURL); err != nil {
		fmt.Println("couldn't open a browser automatically — open this URL manually:")
	}
	// Always printed, not just as a failure fallback: if the tab gets
	// closed before the heartbeat watchdog notices (see below) and the
	// session cookie is lost, this is the only way back in short of
	// restarting the app.
	fmt.Println("N5E Toolkit running at", launchURL)
	fmt.Println("character database:", charDBPath)

	// Backgrounded periodic snapshot, independent of the shutdown-time one
	// below: that one covers an orderly tab close, this one covers
	// everything else that ends the process without going through
	// srv.shutdown (a crash, the machine losing power, `kill -9`).
	go func() {
		ticker := time.NewTicker(backupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-srv.shutdown:
				return
			case <-ticker.C:
				if _, err := backup.Create(charDB, backupDir, backup.ReasonPeriodic); err != nil {
					log.Println("periodic backup:", err)
					continue
				}
				if err := backup.Prune(backupDir); err != nil {
					log.Println("pruning backups:", err)
				}
			}
		}
	}()

	// No Quit button — closing the browser tab is the whole UX now.
	// watchHeartbeat notices the tab's /alive connection dropping (or, as
	// a fallback, static/js/heartbeat.js's pings going silent) and closes
	// srv.shutdown; an interrupt signal (Ctrl+C, only reachable when a
	// console is attached — harmless to also listen for it since a
	// -H=windowsgui build has no console to send one from) is the other
	// way out.
	go srv.watchHeartbeat()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-srv.shutdown:
	case <-sigCh:
		// Close the channel on this path too. The /alive handlers block
		// until it closes; Shutdown waits for in-flight handlers and does
		// NOT cancel their contexts, so without this a Ctrl+C would sit
		// out the whole shutdownGrace and then return a deadline error
		// purely because a stream was still open.
		srv.shutdownOnce.Do(func() { close(srv.shutdown) })
	}

	// charDB is still open here (closed by the deferred Close above, which
	// hasn't run yet) — this is the tab-close/Ctrl+C counterpart to the
	// periodic loop above, covering the one path most likely to matter: an
	// ordinary end to a normal session.
	if _, err := backup.Create(charDB, backupDir, backup.ReasonClose); err != nil {
		log.Println("shutdown backup:", err)
	} else if err := backup.Prune(backupDir); err != nil {
		log.Println("pruning backups:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	return httpServer.Shutdown(ctx)
}
