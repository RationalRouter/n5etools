// Clan compendium loader. Same two rules as the jutsu loader (see store.go):
// stable slugs, overrides never touched.
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sergio/n5e/internal/parse"
)

// ClanSlug derives a clan's stable key: "Aburame Clan" → "clan/aburame",
// "Non-Clan" → "clan/non-clan".
func ClanSlug(name string) string {
	return "clan/" + Slugify(strings.TrimSuffix(name, " Clan"))
}

// LoadClanBook upserts one parsed clan compendium in a single transaction:
// clans, traits, ability increases, proficiencies, features, clan jutsu
// (into the shared jutsu table, slug-scoped per clan), and clan feats.
func LoadClanBook(db *sql.DB, book SourceBook, clans []parse.Clan, anomalies []parse.Anomaly) (*LoadReport, error) {
	report := &LoadReport{}

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

	seenClans := map[string]string{}
	seenJutsu := map[string]string{}
	seenFeatures := map[string]string{}
	seenFeats := map[string]string{}

	for _, c := range clans {
		clanSlug := ClanSlug(c.Name)
		if other, dup := seenClans[clanSlug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", clanSlug, other, c.Name))
			continue
		}
		seenClans[clanSlug] = c.Name

		status := "auto"
		if flagged[c.Name] {
			status = "needs_review"
		}
		outcome, err := upsertClan(tx, book, clanSlug, c, status)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", clanSlug, err)
		}
		report.count(outcome, clanSlug)

		// Derived rows: rebuilt wholesale from the parse each run. No
		// overrides live in these tables.
		if err := rebuildClanDetail(tx, clanSlug, c); err != nil {
			return nil, fmt.Errorf("%s detail: %w", clanSlug, err)
		}

		for order, f := range c.Features {
			featSlug := clanSlug + "/feature/" + Slugify(f.Name)
			if other, dup := seenFeatures[featSlug]; dup {
				report.Duplicates = append(report.Duplicates,
					fmt.Sprintf("%s: %q and %q collide", featSlug, other, f.Name))
				continue
			}
			seenFeatures[featSlug] = f.Name
			outcome, err := upsertClanFeature(tx, book, featSlug, clanSlug, f, order)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", featSlug, err)
			}
			report.count(outcome, featSlug)
		}

		for _, j := range c.Jutsu {
			// Clan jutsu are scoped under the clan so a clan technique can
			// never collide with (or overwrite) a main-compendium jutsu of
			// the same name: "jutsu/aburame/human-cocoon".
			jutsuSlug := "jutsu/" + Slugify(strings.TrimSuffix(c.Name, " Clan")) + "/" + Slugify(j.Name)
			if other, dup := seenJutsu[jutsuSlug]; dup {
				report.Duplicates = append(report.Duplicates,
					fmt.Sprintf("%s: %q and %q collide", jutsuSlug, other, j.Name))
				continue
			}
			seenJutsu[jutsuSlug] = j.Name
			if j.Rank == "" {
				report.Skipped = append(report.Skipped,
					fmt.Sprintf("%s: no rank detected, cannot load", jutsuSlug))
				continue
			}
			status := "auto"
			if flagged[j.Name] {
				status = "needs_review"
			}
			outcome, err := upsertJutsu(tx, book, jutsuSlug, j, status)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", jutsuSlug, err)
			}
			report.count(outcome, jutsuSlug)
			if err := replaceKeywords(tx, jutsuSlug, j.Keywords); err != nil {
				return nil, fmt.Errorf("%s keywords: %w", jutsuSlug, err)
			}
			if _, err := tx.Exec(
				`UPDATE jutsu SET clan_slug = ? WHERE slug = ?`, clanSlug, jutsuSlug); err != nil {
				return nil, err
			}
		}
		if err := rebuildClanJutsuLinks(tx, clanSlug); err != nil {
			return nil, err
		}

		for _, f := range c.Feats {
			featSlug := "feat/" + Slugify(strings.TrimSuffix(c.Name, " Clan")) + "/" + Slugify(f.Name)
			if other, dup := seenFeats[featSlug]; dup {
				report.Duplicates = append(report.Duplicates,
					fmt.Sprintf("%s: %q and %q collide", featSlug, other, f.Name))
				continue
			}
			seenFeats[featSlug] = f.Name
			outcome, err := upsertFeat(tx, book, featSlug, clanSlug, "", f)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", featSlug, err)
			}
			report.count(outcome, featSlug)
		}
	}

	// Vanished detection per table, scoped to this book.
	for _, q := range []struct {
		table string
		cond  string
		seen  map[string]string
	}{
		{"clans", "", seenClans},
		{"clan_features", "", seenFeatures},
		// This loader only writes clan-scoped feats; the unscoped
		// BLOODLINE, LATENT feat is loaded (and tracked) elsewhere.
		{"feats", "clan_slug IS NOT NULL", seenFeats},
	} {
		if err := findVanished(tx, q.table, q.cond, book.Slug, q.seen, report); err != nil {
			return nil, err
		}
	}
	// Jutsu vanished: only this book's rows.
	if err := findVanished(tx, "jutsu", "", book.Slug, seenJutsu, report); err != nil {
		return nil, err
	}

	if err := tx.QueryRow(`
		SELECT (SELECT COUNT(*) FROM clans WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM clan_features WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM jutsu WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM feats WHERE detection_status = 'needs_review')`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}

	return report, tx.Commit()
}

// count folds one row outcome into the report totals.
func (r *LoadReport) count(o rowOutcome, slug string) {
	switch o {
	case rowCreated:
		r.Created++
	case rowUpdated:
		r.Updated++
	case rowDemoted:
		r.Updated++
		r.Demoted = append(r.Demoted, slug)
	case rowUnchanged:
		r.Unchanged++
	}
}

// findVanished reports rows of table from this book whose slug was not in
// this batch. Reported, never deleted. extraCond narrows the scan when two
// loaders share a table (e.g. LoadClanBook owns only clan-scoped feats; the
// unscoped BLOODLINE, LATENT feat belongs to LoadBloodlineLatents); pass ""
// to scan the book's whole table.
func findVanished(tx *sql.Tx, table, extraCond, bookSlug string, seen map[string]string, report *LoadReport) error {
	q := `SELECT slug FROM ` + table + ` WHERE source_book = ?`
	if extraCond != "" {
		q += ` AND ` + extraCond
	}
	rows, err := tx.Query(q+` ORDER BY slug`, bookSlug)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return err
		}
		if _, ok := seen[slug]; !ok {
			report.Vanished = append(report.Vanished, slug)
		}
	}
	return rows.Err()
}

// decideStatus applies the shared demote rule: content change on a
// human-blessed row sends it back to review; otherwise the new status wins
// on change and the old one survives no-ops.
func decideStatus(oldStatus, newStatus string, changed bool) (string, rowOutcome) {
	if !changed {
		return oldStatus, rowUnchanged
	}
	if oldStatus == "verified" || oldStatus == "manual" {
		return "needs_review", rowDemoted
	}
	return newStatus, rowUpdated
}

func upsertClan(tx *sql.Tx, book SourceBook, slug string, c parse.Clan, status string) (rowOutcome, error) {
	var speedFeet any
	if c.SpeedFeet != nil {
		speedFeet = *c.SpeedFeet
	}

	var old struct {
		name, epithet, overview, abilityText, extraLanguage sql.NullString
		speedFeet                                           sql.NullInt64
		status                                              string
	}
	err := tx.QueryRow(`
		SELECT name, epithet, overview, ability_increase_text, extra_language,
		       speed_feet, detection_status
		FROM clans WHERE slug = ?`, slug).Scan(
		&old.name, &old.epithet, &old.overview, &old.abilityText,
		&old.extraLanguage, &old.speedFeet, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO clans (slug, name, epithet, overview, speed_feet,
			                   extra_language, ability_increase_text,
			                   source_book, source_version, source_page,
			                   detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			slug, c.Name, c.Epithet, c.Overview, speedFeet,
			c.ExtraLanguage, c.AbilityRaw,
			book.Slug, book.Version, c.SourcePage, status)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != c.Name ||
		old.epithet.String != c.Epithet ||
		old.overview.String != c.Overview ||
		old.abilityText.String != c.AbilityRaw ||
		old.extraLanguage.String != c.ExtraLanguage ||
		!nullableIntEq(old.speedFeet, c.SpeedFeet)
	newStatus, outcome := decideStatus(old.status, status, changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE clans
		SET name = ?, epithet = ?, overview = ?, speed_feet = ?,
		    extra_language = ?, ability_increase_text = ?,
		    source_book = ?, source_version = ?, source_page = ?,
		    detection_status = ?
		WHERE slug = ?`,
		c.Name, c.Epithet, c.Overview, speedFeet,
		c.ExtraLanguage, c.AbilityRaw,
		book.Slug, book.Version, c.SourcePage, newStatus, slug)
	return outcome, err
}

// rebuildClanDetail replaces the purely-derived clan rows: ability
// increases, skill proficiencies, named traits.
func rebuildClanDetail(tx *sql.Tx, clanSlug string, c parse.Clan) error {
	for _, del := range []string{
		`DELETE FROM clan_ability_increases WHERE clan_slug = ?`,
		`DELETE FROM clan_proficiencies WHERE clan_slug = ?`,
		`DELETE FROM clan_traits WHERE clan_slug = ?`,
	} {
		if _, err := tx.Exec(del, clanSlug); err != nil {
			return err
		}
	}
	for _, ai := range c.AbilityIncreases {
		if _, err := tx.Exec(`
			INSERT INTO clan_ability_increases (clan_slug, ability, amount)
			VALUES (?, ?, ?)`, clanSlug, ai.Ability, ai.Amount); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, skill := range c.SkillProfs {
		if seen[skill] {
			continue
		}
		seen[skill] = true
		if _, err := tx.Exec(`
			INSERT INTO clan_proficiencies (clan_slug, kind, value)
			VALUES (?, 'skill', ?)`, clanSlug, skill); err != nil {
			return err
		}
	}
	for order, tr := range c.Traits {
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO clan_traits (clan_slug, name, description, sort_order)
			VALUES (?, ?, ?, ?)`, clanSlug, tr.Name, tr.Description, order); err != nil {
			return err
		}
	}
	return nil
}

// rebuildClanJutsuLinks rebuilds the clan↔jutsu junction from the jutsu
// table's clan_slug column (set during the jutsu upserts above).
func rebuildClanJutsuLinks(tx *sql.Tx, clanSlug string) error {
	if _, err := tx.Exec(`DELETE FROM clan_jutsu WHERE clan_slug = ?`, clanSlug); err != nil {
		return err
	}
	_, err := tx.Exec(`
		INSERT INTO clan_jutsu (clan_slug, jutsu_slug)
		SELECT clan_slug, slug FROM jutsu WHERE clan_slug = ?`, clanSlug)
	return err
}

func upsertClanFeature(tx *sql.Tx, book SourceBook, slug, clanSlug string, f parse.ClanFeature, order int) (rowOutcome, error) {
	var level any
	if f.Level != nil {
		level = *f.Level
	}

	var old struct {
		name, description sql.NullString
		level             sql.NullInt64
		status            string
	}
	err := tx.QueryRow(`
		SELECT name, description, level, detection_status
		FROM clan_features WHERE slug = ?`, slug).Scan(
		&old.name, &old.description, &old.level, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO clan_features (slug, clan_slug, name, level, description,
			                           sort_order, source_book, source_version,
			                           source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, clanSlug, f.Name, level, f.Description, order,
			book.Slug, book.Version, f.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != f.Name ||
		old.description.String != f.Description ||
		!nullableIntEq(old.level, f.Level)
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE clan_features
		SET name = ?, level = ?, description = ?, sort_order = ?,
		    source_book = ?, source_version = ?, source_page = ?,
		    detection_status = ?
		WHERE slug = ?`,
		f.Name, level, f.Description, order,
		book.Slug, book.Version, f.SourcePage, newStatus, slug)
	return outcome, err
}

func upsertFeat(tx *sql.Tx, book SourceBook, slug, clanSlug, classSlug string, f parse.Feat) (rowOutcome, error) {
	category := strings.ToLower(strings.TrimSpace(f.Category))
	if category == "" {
		category = "unknown"
	}
	// Feats without a clan/class scope (general/bloodline) store NULL, not "".
	var clanRef, classRef any
	if clanSlug != "" {
		clanRef = clanSlug
	}
	if classSlug != "" {
		classRef = classSlug
	}

	var old struct {
		name, category, prerequisites, description sql.NullString
		status                                     string
	}
	err := tx.QueryRow(`
		SELECT name, category, prerequisites, description, detection_status
		FROM feats WHERE slug = ?`, slug).Scan(
		&old.name, &old.category, &old.prerequisites, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO feats (slug, name, category, prerequisites, description,
			                   clan_slug, class_slug, source_book, source_version,
			                   source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, f.Name, category, f.Prerequisites, f.Description,
			clanRef, classRef, book.Slug, book.Version, f.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != f.Name ||
		old.category.String != category ||
		old.prerequisites.String != f.Prerequisites ||
		old.description.String != f.Description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE feats
		SET name = ?, category = ?, prerequisites = ?, description = ?,
		    clan_slug = ?, class_slug = ?, source_book = ?, source_version = ?,
		    source_page = ?, detection_status = ?
		WHERE slug = ?`,
		f.Name, category, f.Prerequisites, f.Description,
		clanRef, classRef, book.Slug, book.Version, f.SourcePage, newStatus, slug)
	return outcome, err
}
