// Loading core-book enhancement seals into the equipment table.
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/parse"
)

// LoadEnhancementSeals upserts the weapon/armor seal entries. Two seals can
// share a name across tiers (a weapon and armor version, or a mislabeled
// duplicate — see the parser's tier/heading mismatch anomaly), so the slug
// includes both applies-to and tier to stay unique.
func LoadEnhancementSeals(db *sql.DB, book SourceBook, seals []parse.EnhancementSeal) (*LoadReport, error) {
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
	for _, s := range seals {
		slug := fmt.Sprintf("seal/%s/%s/%s", s.AppliesTo, Slugify(s.Tier), Slugify(s.Name))
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, s.Name))
			continue
		}
		seen[slug] = s.Name
		outcome, err := upsertSeal(tx, book, slug, s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "equipment",
		"kind = 'enhancement_seal'", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM equipment WHERE detection_status = 'needs_review'
		 AND kind = 'enhancement_seal'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertSeal(tx *sql.Tx, book SourceBook, slug string, s parse.EnhancementSeal) (rowOutcome, error) {
	var old struct {
		name, description, appliesTo, rank sql.NullString
		cost                               sql.NullFloat64
		status                             string
	}
	err := tx.QueryRow(`
		SELECT name, description, seal_applies_to, seal_rank, cost_ryo, detection_status
		FROM equipment WHERE slug = ?`, slug).Scan(
		&old.name, &old.description, &old.appliesTo, &old.rank, &old.cost, &old.status)

	displayName := s.Name + " (" + s.Tier + ")"
	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO equipment (slug, name, kind, cost_ryo, description,
			                       seal_rank, seal_applies_to, source_book,
			                       source_version, source_page, detection_status)
			VALUES (?, ?, 'enhancement_seal', ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, displayName, s.CostRyo, s.Description, s.Rank, s.AppliesTo,
			book.Slug, book.Version, s.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != displayName ||
		old.description.String != s.Description ||
		old.appliesTo.String != s.AppliesTo ||
		old.rank.String != s.Rank ||
		int(old.cost.Float64) != s.CostRyo
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE equipment
		SET name = ?, cost_ryo = ?, description = ?, seal_rank = ?, seal_applies_to = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		displayName, s.CostRyo, s.Description, s.Rank, s.AppliesTo,
		book.Slug, book.Version, s.SourcePage, newStatus, slug)
	return outcome, err
}
