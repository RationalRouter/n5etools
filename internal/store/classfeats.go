// Loading the class-feat sections of the class compendium.
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/parse"
)

// LoadClassFeats upserts the class compendium's closing feat sections.
// Feats whose "Archetype:" label names a loaded class get class_slug set;
// caster/martial feats stay unscoped. Runs after LoadClassBook so the class
// rows exist to link against.
func LoadClassFeats(db *sql.DB, book SourceBook, feats []parse.Feat) (*LoadReport, error) {
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
		slug := "feat/class/" + Slugify(f.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, f.Name))
			continue
		}
		seen[slug] = f.Name

		classSlug := ""
		if f.ClassName != "" {
			// The book prints "Cooking-Ninja" where the class is Cooking-Nin.
			name := f.ClassName
			if name == "Cooking-Ninja" {
				name = "Cooking-Nin"
			}
			classSlug = "class/" + Slugify(name)
			var exists int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM classes WHERE slug = ?`,
				classSlug).Scan(&exists); err != nil {
				return nil, err
			}
			if exists == 0 {
				// Witch/Knight feats reference subclasses with no class in
				// any loaded book — keep the feat, unlinked, for review.
				report.Skipped = append(report.Skipped, fmt.Sprintf(
					"%s: loaded WITHOUT class link — no class %q in the rules DB", slug, f.ClassName))
				classSlug = ""
			}
		}
		outcome, err := upsertFeat(tx, book, slug, "", classSlug, f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	// This loader owns only the book's feat/class/ rows; LoadClassBook's
	// vanished scan never touches the feats table.
	if err := findVanished(tx, "feats", "slug LIKE 'feat/class/%'", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM feats WHERE detection_status = 'needs_review'
		 AND slug LIKE 'feat/class/%'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}
