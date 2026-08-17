// Loading the core book's parsed slices into the rules database.
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/parse"
)

// LoadFightingStances upserts Chapter 13's stance options.
func LoadFightingStances(db *sql.DB, book SourceBook, stances []parse.FightingStance) (*LoadReport, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	report := &LoadReport{}
	seen := map[string]string{}
	for _, s := range stances {
		slug := "stance/" + Slugify(s.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, s.Name))
			continue
		}
		seen[slug] = s.Name
		outcome, err := upsertStance(tx, book, slug, s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "fighting_stances", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM fighting_stances WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertStance(tx *sql.Tx, book SourceBook, slug string, s parse.FightingStance) (rowOutcome, error) {
	var old struct {
		name, stanceType, prerequisites, description sql.NullString
		status                                       string
	}
	err := tx.QueryRow(`
		SELECT name, stance_type, prerequisites, description, detection_status
		FROM fighting_stances WHERE slug = ?`, slug).Scan(
		&old.name, &old.stanceType, &old.prerequisites, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO fighting_stances (slug, name, stance_type, prerequisites,
			                              description, source_book, source_version,
			                              source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, s.Name, s.StanceType, s.Prerequisites, s.Description,
			book.Slug, book.Version, s.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != s.Name ||
		old.stanceType.String != s.StanceType ||
		old.prerequisites.String != s.Prerequisites ||
		old.description.String != s.Description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE fighting_stances
		SET name = ?, stance_type = ?, prerequisites = ?, description = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		s.Name, s.StanceType, s.Prerequisites, s.Description,
		book.Slug, book.Version, s.SourcePage, newStatus, slug)
	return outcome, err
}

// LoadCoreFeats upserts Chapter 13's feats. Core feats are unscoped —
// no clan, no class — and live at "feat/<name>".
func LoadCoreFeats(db *sql.DB, book SourceBook, feats []parse.Feat) (*LoadReport, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	report := &LoadReport{}
	seen := map[string]string{}
	for _, f := range feats {
		slug := "feat/" + Slugify(f.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, f.Name))
			continue
		}
		seen[slug] = f.Name
		outcome, err := upsertFeat(tx, book, slug, "", "", f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "feats", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM feats WHERE detection_status = 'needs_review'
		 AND source_book = ?`, book.Slug,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}
