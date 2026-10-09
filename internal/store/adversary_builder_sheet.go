// Loading internal/parse/adversary_builder_sheet.go's rows into the rules
// database. This is bulk community reference content re-generated wholesale
// from a spreadsheet a GM occasionally re-exports — there's no player edit
// history on any individual row worth preserving across a re-ingest, unlike
// adversary_role_traits' upsert-with-diff pattern, so this follows
// bingobook_freeform.go's simpler delete+reload convention instead.
package store

import (
	"database/sql"
	"fmt"

	"github.com/RationalRouter/n5e/internal/parse"
)

// LoadAdversaryBuilderTraits fully replaces adversary_builder_sheet_traits.
func LoadAdversaryBuilderTraits(db *sql.DB, book SourceBook, traits []parse.AdversaryBuilderTrait) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM adversary_builder_sheet_traits`); err != nil {
		return err
	}

	seen := map[string]bool{}
	for _, t := range traits {
		slug := fmt.Sprintf("adversary-builder-trait/%s/%s", Slugify(t.CategoryTag), Slugify(t.Name))
		if seen[slug] {
			slug = fmt.Sprintf("%s-row%d", slug, t.RowNumber)
		}
		seen[slug] = true

		if _, err := tx.Exec(
			`INSERT INTO adversary_builder_sheet_traits
			 (slug, category, category_tag, name, rank, description, keywords, sort_order,
			  source_book, source_version, detection_status)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, t.Category, t.CategoryTag, t.Name, t.Rank, t.Description, t.Keywords, t.RowNumber,
			book.Slug, book.Version); err != nil {
			return fmt.Errorf("%s: %w", slug, err)
		}
	}
	return tx.Commit()
}
