// Bloodline Latents loader. The BLOODLINE, LATENT feat goes into feats
// (unscoped); each priced ability goes into bloodline_latents keyed
// "latent/<clan>/<ability>".
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/parse"
)

// LoadBloodlineLatents upserts the latent tables in one transaction.
func LoadBloodlineLatents(db *sql.DB, book SourceBook, feat *parse.Feat, latents []parse.BloodlineLatent) (*LoadReport, error) {
	report := &LoadReport{}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	if feat != nil {
		outcome, err := upsertFeat(tx, book, "feat/bloodline-latent", "", "", *feat)
		if err != nil {
			return nil, fmt.Errorf("feat/bloodline-latent: %w", err)
		}
		report.count(outcome, "feat/bloodline-latent")
	}

	seen := map[string]string{}
	for _, l := range latents {
		clanSlug := ClanSlug(l.ClanName)
		var clanExists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM clans WHERE slug = ?`, clanSlug).Scan(&clanExists); err != nil {
			return nil, err
		}
		if clanExists == 0 {
			report.Skipped = append(report.Skipped,
				fmt.Sprintf("latent %q: no clan row %s (load clans first?)", l.Name, clanSlug))
			continue
		}

		slug := "latent/" + Slugify(l.ClanName) + "/" + Slugify(l.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, l.Name))
			continue
		}
		seen[slug] = l.Name

		outcome, err := upsertLatent(tx, book, slug, clanSlug, l)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "bloodline_latents", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM bloodline_latents WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertLatent(tx *sql.Tx, book SourceBook, slug, clanSlug string, l parse.BloodlineLatent) (rowOutcome, error) {
	var old struct {
		name, prerequisites, description sql.NullString
		pointCost                        sql.NullInt64
		status                           string
	}
	err := tx.QueryRow(`
		SELECT name, prerequisites, description, point_cost, detection_status
		FROM bloodline_latents WHERE slug = ?`, slug).Scan(
		&old.name, &old.prerequisites, &old.description, &old.pointCost, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO bloodline_latents (slug, name, stage, clan_slug,
			                               prerequisites, description, point_cost,
			                               source_book, source_version, source_page,
			                               detection_status)
			VALUES (?, ?, 'latent', ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, l.Name, clanSlug, l.Prerequisites, l.Description, l.PointCost,
			book.Slug, book.Version, l.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != l.Name ||
		old.prerequisites.String != l.Prerequisites ||
		old.description.String != l.Description ||
		!old.pointCost.Valid || old.pointCost.Int64 != int64(l.PointCost)
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE bloodline_latents
		SET name = ?, stage = 'latent', clan_slug = ?, prerequisites = ?,
		    description = ?, point_cost = ?,
		    source_book = ?, source_version = ?, source_page = ?,
		    detection_status = ?
		WHERE slug = ?`,
		l.Name, clanSlug, l.Prerequisites, l.Description, l.PointCost,
		book.Slug, book.Version, l.SourcePage, newStatus, slug)
	return outcome, err
}
