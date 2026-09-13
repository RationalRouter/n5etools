// Centralized translation bundle fetch/apply — the client half of the
// design that keeps a live DeepL API key out of every end user's binary
// (see internal/i18n's cmd/n5e-i18n translate-book pipeline for the
// maintainer half). A maintainer runs DeepL by hand, once, on their own
// machine, and publishes the resulting content_translations rows as a
// small SQLite file attached to a rolling "translations-latest" GitHub
// Release tag. Every end user's own autoupdate run then just downloads
// that file — a plain, unauthenticated CDN fetch, identical in spirit to
// how the app binary itself is already distributed — and merges it into
// their local rules.db. No DeepL calls, and no DeepL credentials, ever run
// inside an end user's install.
package autoupdate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sergio/n5e/internal/fetch"
	"github.com/sergio/n5e/internal/i18n"
	"github.com/sergio/n5e/internal/schema"
	"github.com/sergio/n5e/internal/store"
)

const (
	translationsRepoOwner  = "RationalRouter"
	translationsRepoName   = "n5etools"
	translationsReleaseTag = "translations-latest"
)

// TranslationResult is what happened when checking one locale's translation
// bundle. Action is one of "unchanged", "updated", "no bundle yet" (the
// maintainer hasn't published a bundle for this binary's current schema
// version), or "error".
type TranslationResult struct {
	Locale string
	Action string
	Err    error
}

// translationAssetName matches cmd/n5e-i18n's own publish-side naming
// exactly — see internal/i18n/translation_methodology.md's bundle-naming
// note. Versioned by ingest_schema_version (not a separate scheme) so both
// sides derive the same expected filename from a value they each already
// compute for their own staleness checks.
func translationAssetName(locale, ingestSchemaVersion string) string {
	return fmt.Sprintf("n5e-translations-%s-%s.db", locale, ingestSchemaVersion)
}

// CheckTranslations checks every non-English locale's translation bundle
// against the rolling translations-latest release and applies whichever
// ones are newer than what's already stored. db must already be
// schema.Apply'd. client may be nil (see fetch's own nil-means-defaults
// convention).
func CheckTranslations(ctx context.Context, db *sql.DB, cacheDir string, client *http.Client) []TranslationResult {
	currentVersion, err := schema.LatestMigration(schema.Rules)
	if err != nil {
		return []TranslationResult{{Action: "error", Err: fmt.Errorf("determining current schema version: %w", err)}}
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return []TranslationResult{{Action: "error", Err: fmt.Errorf("creating cache dir %s: %w", cacheDir, err)}}
	}

	var results []TranslationResult
	for _, locale := range i18n.SupportedLocales {
		if locale == "en" {
			// English is the source language, stored directly in each content
			// table's own columns — content_translations never carries an
			// 'en' row, so there is nothing to fetch a bundle for.
			continue
		}
		results = append(results, checkOneTranslation(ctx, db, cacheDir, client, currentVersion, locale))
	}
	return results
}

func checkOneTranslation(ctx context.Context, db *sql.DB, cacheDir string, client *http.Client, currentVersion, locale string) TranslationResult {
	res := TranslationResult{Locale: locale}
	assetName := translationAssetName(locale, currentVersion)
	assetURL := fetch.GitHubReleaseAssetURL(translationsRepoOwner, translationsRepoName, translationsReleaseTag, assetName)

	headCtx, cancel := context.WithTimeout(ctx, headTimeout)
	remoteETag, err := fetch.HeadETag(headCtx, client, assetURL)
	cancel()
	if err != nil {
		// A bundle simply not existing yet for this binary's current schema
		// version (the maintainer hasn't published one) is expected and
		// common, not a failure — every other HEAD error (network, a real
		// server error) is reported as one.
		if errors.Is(err, fetch.ErrAssetNotFound) {
			res.Action = "no bundle yet"
			return res
		}
		res.Action, res.Err = "error", fmt.Errorf("checking %s translation bundle: %w", locale, err)
		return res
	}

	_, storedETag, found, err := store.GetTranslationBundleState(db, locale)
	if err != nil {
		res.Action, res.Err = "error", fmt.Errorf("reading stored translation state: %w", err)
		return res
	}
	if found && storedETag == remoteETag {
		res.Action = "unchanged"
		return res
	}

	destPath := filepath.Join(cacheDir, "translations-"+locale+".db")
	dlCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	etag, err := fetch.DownloadAsset(dlCtx, client, assetURL, destPath)
	cancel()
	if err != nil {
		res.Action, res.Err = "error", fmt.Errorf("downloading %s translation bundle: %w", locale, err)
		return res
	}
	defer os.Remove(destPath)

	if err := applyTranslationBundle(db, destPath, locale); err != nil {
		res.Action, res.Err = "error", fmt.Errorf("applying %s translation bundle: %w", locale, err)
		return res
	}
	if err := store.SetTranslationBundleState(db, locale, currentVersion, etag); err != nil {
		res.Action, res.Err = "error", fmt.Errorf("stamping %s translation state: %w", locale, err)
		return res
	}

	res.Action = "updated"
	return res
}

// applyTranslationBundle merges a downloaded bundle's content_translations
// rows into db via ATTACH DATABASE, confirmed to work cleanly through this
// project's modernc.org/sqlite driver. Pinned to one *sql.Conn (not plain
// db.Exec, and not a database/sql transaction) for two reasons: ATTACH is
// per-connection state, so pool-assigned connections could otherwise split
// the ATTACH/INSERT/DETACH sequence across different underlying SQLite
// connections; and SQLite itself refuses ATTACH/DETACH while an explicit
// transaction is open ("cannot ATTACH/DETACH database within transaction",
// confirmed by hitting exactly that error via db.Begin() during
// development). The INSERT OR REPLACE is naturally idempotent, so a failure
// between ATTACH and DETACH just leaves the merge partially applied to be
// retried next launch, not corrupted.
func applyTranslationBundle(db *sql.DB, bundlePath, locale string) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS bundle`, bundlePath); err != nil {
		return fmt.Errorf("attaching bundle: %w", err)
	}
	defer conn.ExecContext(ctx, `DETACH DATABASE bundle`)

	// Explicit column list, not SELECT * — a bundle built by an older or
	// newer n5e-i18n than this binary's own schema could otherwise
	// silently misalign columns positionally instead of failing loudly.
	if _, err := conn.ExecContext(ctx, `
		INSERT OR REPLACE INTO content_translations
			(table_name, slug, locale, field, value, status, en_hash, updated_at)
		SELECT table_name, slug, locale, field, value, status, en_hash, updated_at
		FROM bundle.content_translations WHERE locale = ?`, locale); err != nil {
		return fmt.Errorf("merging content_translations: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `DETACH DATABASE bundle`); err != nil {
		return fmt.Errorf("detaching bundle: %w", err)
	}
	return nil
}
