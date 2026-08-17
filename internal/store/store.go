// Package store writes parsed sourcebook content into the rules database.
//
// The two rules that make re-ingestion safe live here:
//
//  1. STABLE SLUGS. Every entity's primary key is derived from its printed
//     name once ("Fire Release: Ember Toss" → "jutsu/fire-release-ember-toss")
//     and never re-typed by a human. A re-ingest of the same book matches
//     existing rows by slug, so cross-references and manual overrides stay
//     attached.
//
//  2. OVERRIDES ARE NEVER TOUCHED. A load updates parsed columns only.
//     The *_override columns, notes, and a human's 'verified'/'manual'
//     detection_status survive every re-run — except that when the parsed
//     content of a 'verified' row actually changes, the row is demoted to
//     'needs_review' so the change gets human eyes again.
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sergio/n5e/internal/parse"
)

// SourceBook identifies one ingested PDF for provenance rows.
type SourceBook struct {
	Slug       string // 'book/jutsu-compendium'
	Title      string // "Jiraiya's Jutsu Compendium"
	Version    string // "3.1" — passed in by the maintainer; books differ
	FileName   string
	FileSHA256 string
}

// LoadReport says what a load actually did — the numbers the maintainer
// reads after every re-ingest.
type LoadReport struct {
	Created     int
	Updated     int // parsed content changed on an existing row
	Unchanged   int
	NeedsReview int      // rows currently flagged for human review
	Demoted     []string // slugs that were 'verified' but whose content changed
	Duplicates  []string // batch contained two entries slugifying identically
	Skipped     []string // rows that could not be loaded (reason included)
	Vanished    []string // rows from this book absent from this batch —
	// reported, never deleted automatically
}

// slugTransliterations folds the accented letters that appear in the books
// to plain ASCII, so "Hyūga" and the book's own unaccented "Hyuga" spelling
// produce the SAME slug. Extend as new letters show up.
var slugTransliterations = map[rune]rune{
	'ā': 'a', 'á': 'a', 'à': 'a', 'â': 'a',
	'ē': 'e', 'é': 'e', 'è': 'e', 'ê': 'e',
	'ī': 'i', 'í': 'i', 'ì': 'i', 'î': 'i',
	'ō': 'o', 'ó': 'o', 'ò': 'o', 'ô': 'o',
	'ū': 'u', 'ú': 'u', 'ù': 'u', 'û': 'u',
}

// Slugify turns a printed name into its stable key form: lowercase,
// accents folded ("Hyūga" → "hyuga"), apostrophes dropped, every other
// non-alphanumeric run becomes one hyphen.
// "Summoner's Art: Bear!" → "summoners-art-bear".
func Slugify(name string) string {
	var b strings.Builder
	pendingHyphen := false
	for _, r := range strings.ToLower(name) {
		if t, ok := slugTransliterations[r]; ok {
			r = t
		}
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingHyphen = false
			b.WriteRune(r)
		case r == '\'' || r == '’':
			// drop, no hyphen: "summoner's" → "summoners"
		default:
			pendingHyphen = true
		}
	}
	return b.String()
}

// LoadJutsu upserts one parsed jutsu batch into the rules DB inside a single
// transaction. Anomalous entries (by parser anomaly subject) enter as
// detection_status='needs_review'; clean entries as 'auto'.
func LoadJutsu(db *sql.DB, book SourceBook, jutsu []parse.Jutsu, anomalies []parse.Anomaly) (*LoadReport, error) {
	report := &LoadReport{}

	// Entries the parser flagged get needs_review status on load.
	flagged := map[string]bool{}
	for _, a := range anomalies {
		flagged[a.Subject] = true
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	seen := map[string]string{} // slug → name, for duplicate detection
	for _, j := range jutsu {
		slug := "jutsu/" + Slugify(j.Name)

		if j.Rank == "" {
			report.Skipped = append(report.Skipped,
				fmt.Sprintf("%s: no rank detected, cannot load", slug))
			continue
		}
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, j.Name))
			continue
		}
		seen[slug] = j.Name

		status := "auto"
		if flagged[j.Name] {
			status = "needs_review"
		}

		outcome, err := upsertJutsu(tx, book, slug, j, status)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		switch outcome {
		case rowCreated:
			report.Created++
		case rowUpdated:
			report.Updated++
		case rowDemoted:
			report.Updated++
			report.Demoted = append(report.Demoted, slug)
		case rowUnchanged:
			report.Unchanged++
		}

		if err := replaceKeywords(tx, slug, j.Keywords); err != nil {
			return nil, fmt.Errorf("%s keywords: %w", slug, err)
		}
	}

	// Rows this book loaded before that this batch no longer contains:
	// renamed or removed in the new book version. Surfaced for a human;
	// never deleted here, because character saves may reference them.
	rows, err := tx.Query(
		`SELECT slug FROM jutsu WHERE source_book = ? ORDER BY slug`, book.Slug)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := seen[slug]; !ok {
			report.Vanished = append(report.Vanished, slug)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM jutsu WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}

	return report, tx.Commit()
}

type rowOutcome int

const (
	rowCreated rowOutcome = iota
	rowUpdated
	rowUnchanged
	rowDemoted // was human-'verified', parsed content changed → needs_review
)

// upsertJutsu writes one jutsu row. Parsed columns only — rank_override and
// notes are never in any statement here, so they survive every re-ingest.
func upsertJutsu(tx *sql.Tx, book SourceBook, slug string, j parse.Jutsu, status string) (rowOutcome, error) {
	// Read what's there now to decide created/updated/unchanged and to
	// protect a human 'verified'/'manual' status when nothing changed.
	var old struct {
		name, classification, rank, castingTime, rng, duration string
		components, costText, keywords, description            string
		atHigherRanks, categoryGroup                           sql.NullString
		costChakra                                             sql.NullInt64
		status                                                 string
	}
	err := tx.QueryRow(`
		SELECT name, classification, rank, casting_time, range, duration,
		       components, cost_text, keywords, description,
		       at_higher_ranks, category_group, cost_chakra, detection_status
		FROM jutsu WHERE slug = ?`, slug).Scan(
		&old.name, &old.classification, &old.rank, &old.castingTime, &old.rng,
		&old.duration, &old.components, &old.costText, &old.keywords,
		&old.description, &old.atHigherRanks, &old.categoryGroup,
		&old.costChakra, &old.status)

	var costChakra any
	if j.CostChakra != nil {
		costChakra = *j.CostChakra
	}
	var atHigherRanks any
	if j.AtHigherRanks != "" {
		atHigherRanks = j.AtHigherRanks
	}

	if err == sql.ErrNoRows {
		if _, err := tx.Exec(`
			INSERT INTO jutsu (slug, name, classification, rank, casting_time,
			                   range, duration, components, cost_chakra,
			                   cost_text, keywords, description,
			                   at_higher_ranks, category_group,
			                   source_book, source_version, source_page,
			                   detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			slug, j.Name, j.Classification, j.Rank, j.CastingTime,
			j.Range, j.Duration, j.Components, costChakra,
			j.CostText, j.Keywords, j.Description,
			atHigherRanks, j.CategoryGroup,
			book.Slug, book.Version, j.SourcePage, status); err != nil {
			return 0, err
		}
		return rowCreated, upsertJutsuUpcastRule(tx, slug, j.AtHigherRanks)
	}
	if err != nil {
		return 0, err
	}

	unchanged := old.name == j.Name &&
		old.classification == j.Classification &&
		old.rank == j.Rank &&
		old.castingTime == j.CastingTime &&
		old.rng == j.Range &&
		old.duration == j.Duration &&
		old.components == j.Components &&
		old.costText == j.CostText &&
		old.keywords == j.Keywords &&
		old.description == j.Description &&
		old.atHigherRanks.String == j.AtHigherRanks &&
		old.categoryGroup.String == j.CategoryGroup &&
		nullableIntEq(old.costChakra, j.CostChakra)
	if unchanged {
		// jutsu_upcast_rules is synced unconditionally, even when the jutsu
		// row itself is unchanged — it's a separate table with its own
		// idempotent upsert, and skipping it here would leave existing
		// jutsu without a row the first time this check exists at all.
		return rowUnchanged, upsertJutsuUpcastRule(tx, slug, j.AtHigherRanks)
	}

	// Content changed. A human-reviewed row loses its blessing and goes back
	// in the review queue; otherwise the new parse's status stands.
	outcome := rowUpdated
	newStatus := status
	switch old.status {
	case "verified", "manual":
		newStatus = "needs_review"
		outcome = rowDemoted
	}

	_, err = tx.Exec(`
		UPDATE jutsu
		SET name = ?, classification = ?, rank = ?, casting_time = ?,
		    range = ?, duration = ?, components = ?, cost_chakra = ?,
		    cost_text = ?, keywords = ?, description = ?,
		    at_higher_ranks = ?, category_group = ?,
		    source_book = ?, source_version = ?, source_page = ?,
		    detection_status = ?
		WHERE slug = ?`,
		j.Name, j.Classification, j.Rank, j.CastingTime,
		j.Range, j.Duration, j.Components, costChakra,
		j.CostText, j.Keywords, j.Description,
		atHigherRanks, j.CategoryGroup,
		book.Slug, book.Version, j.SourcePage,
		newStatus, slug)
	if err != nil {
		return 0, err
	}
	return outcome, upsertJutsuUpcastRule(tx, slug, j.AtHigherRanks)
}

func nullableIntEq(a sql.NullInt64, b *int) bool {
	if !a.Valid {
		return b == nil
	}
	return b != nil && a.Int64 == int64(*b)
}

func nullableFloatEq(a sql.NullFloat64, b *float64) bool {
	if !a.Valid {
		return b == nil
	}
	return b != nil && a.Float64 == *b
}

// replaceKeywords rebuilds the per-keyword index rows from the printed
// keyword line. Pure derived data — no overrides live here.
func replaceKeywords(tx *sql.Tx, slug, printed string) error {
	if _, err := tx.Exec(`DELETE FROM jutsu_keywords WHERE jutsu_slug = ?`, slug); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, kw := range strings.Split(printed, ",") {
		kw = strings.TrimSpace(kw)
		if kw == "" || seen[kw] {
			continue
		}
		seen[kw] = true
		if _, err := tx.Exec(
			`INSERT INTO jutsu_keywords (jutsu_slug, keyword) VALUES (?, ?)`,
			slug, kw); err != nil {
			return err
		}
	}
	return nil
}

func upsertSourceBook(tx *sql.Tx, book SourceBook) error {
	_, err := tx.Exec(`
		INSERT INTO source_books (slug, title, version, file_name, file_sha256)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(slug) DO UPDATE SET
		    title = excluded.title,
		    version = excluded.version,
		    file_name = excluded.file_name,
		    file_sha256 = excluded.file_sha256,
		    ingested_at = datetime('now')`,
		book.Slug, book.Title, book.Version, book.FileName, book.FileSHA256)
	return err
}

// RecordSourceBookProvenance upserts a source_books row directly, the same
// way every LoadX call already does internally via upsertSourceBook, for a
// book that has no registered ingest parser yet (the Kage Guide, the Bingo
// Book until its parser ships). internal/autoupdate calls this so a book it
// downloaded and hashed but couldn't parse still gets a real provenance
// record and a drive_last_modified baseline — without one, a no-parser book
// would look "never checked" forever and re-download in full on every
// single app startup instead of settling into the normal cheap-HEAD-check
// cadence once ingested books get.
func RecordSourceBookProvenance(db *sql.DB, book SourceBook) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertSourceBook(tx, book); err != nil {
		return err
	}
	return tx.Commit()
}

// GetSourceBookLastModified reads the cheap-check baseline
// (internal/autoupdate's "has this book's Drive copy changed" timestamp,
// migration 0023) for one book slug. found is false when the slug has no
// source_books row yet (never ingested) or its drive_last_modified is NULL
// (ingested by hand, never auto-updated) — both cases the caller should
// treat as "unknown, check anyway."
func GetSourceBookLastModified(db *sql.DB, slug string) (t time.Time, found bool, err error) {
	var raw sql.NullString
	err = db.QueryRow(`SELECT drive_last_modified FROM source_books WHERE slug = ?`, slug).Scan(&raw)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if !raw.Valid {
		return time.Time{}, false, nil
	}
	t, err = time.Parse(time.RFC1123, raw.String)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parsing stored drive_last_modified %q for %s: %w", raw.String, slug, err)
	}
	return t, true, nil
}

// SetSourceBookLastModified stamps the cheap-check baseline after a
// successful auto-update download+ingest. It only updates an existing
// source_books row — a book with no row yet (never ingested at all) has
// nothing to stamp, since every row is otherwise created by upsertSourceBook
// as part of a real ingest.
func SetSourceBookLastModified(db *sql.DB, slug string, t time.Time) error {
	_, err := db.Exec(`UPDATE source_books SET drive_last_modified = ? WHERE slug = ?`,
		t.UTC().Format(time.RFC1123), slug)
	return err
}

// GetSourceBookIngestVersion reads the "which schema/parser version last
// actually ingested this book" stamp (internal/autoupdate's other freshness
// signal alongside drive_last_modified, migration 0026) for one book slug.
// found is false when the slug has no source_books row yet, or its
// ingest_schema_version is NULL (ingested before this stamp existed, or by
// hand) — both cases the caller should treat as "stale, force a real
// re-ingest regardless of Drive's own timestamp."
func GetSourceBookIngestVersion(db *sql.DB, slug string) (version string, found bool, err error) {
	var raw sql.NullString
	err = db.QueryRow(`SELECT ingest_schema_version FROM source_books WHERE slug = ?`, slug).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !raw.Valid {
		return "", false, nil
	}
	return raw.String, true, nil
}

// SetSourceBookIngestVersion stamps the schema/parser-version baseline after
// a successful auto-update ingest, same "existing row only" reasoning as
// SetSourceBookLastModified.
func SetSourceBookIngestVersion(db *sql.DB, slug, version string) error {
	_, err := db.Exec(`UPDATE source_books SET ingest_schema_version = ? WHERE slug = ?`, version, slug)
	return err
}
