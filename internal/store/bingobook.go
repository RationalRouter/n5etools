// Loading the Bingo Book Pack 1 parser's slices (internal/parse/bingobook.go)
// into the rules database.
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/parse"
)

// LoadAdversaryRoles upserts the five named Roles.
func LoadAdversaryRoles(db *sql.DB, book SourceBook, roles []parse.AdversaryRole) (*LoadReport, error) {
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
	for _, r := range roles {
		slug := "adversary-role/" + Slugify(r.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, r.Name))
			continue
		}
		seen[slug] = r.Name
		outcome, err := upsertAdversaryRole(tx, book, slug, r)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "adversary_roles", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM adversary_roles WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertAdversaryRole(tx *sql.Tx, book SourceBook, slug string, r parse.AdversaryRole) (rowOutcome, error) {
	var old struct {
		name, description sql.NullString
		status            string
	}
	err := tx.QueryRow(`
		SELECT name, description, detection_status
		FROM adversary_roles WHERE slug = ?`, slug).Scan(&old.name, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO adversary_roles (slug, name, description, source_book, source_version,
			                             source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, 'auto')`,
			slug, r.Name, r.Description, book.Slug, book.Version, r.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != r.Name || old.description.String != r.Description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE adversary_roles
		SET name = ?, description = ?, source_book = ?, source_version = ?,
		    source_page = ?, detection_status = ?
		WHERE slug = ?`,
		r.Name, r.Description, book.Slug, book.Version, r.SourcePage, newStatus, slug)
	return outcome, err
}

// LoadAdversaryRoleTraits upserts the Role Traits catalog. role_slug is
// derived from each trait's RoleName the same deterministic way every other
// loader in this package derives a parent reference (e.g. class_slug) —
// ParseAdversaryRoleTraits only ever sets RoleName to one of the five names
// ParseAdversaryRoles also produces, so no runtime lookup is needed.
func LoadAdversaryRoleTraits(db *sql.DB, book SourceBook, traits []parse.AdversaryRoleTrait) (*LoadReport, error) {
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
	for _, t := range traits {
		slug := "adversary-role-trait/" + Slugify(t.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, t.Name))
			continue
		}
		seen[slug] = t.Name
		roleSlug := "adversary-role/" + Slugify(t.RoleName)
		outcome, err := upsertAdversaryRoleTrait(tx, book, slug, roleSlug, t)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "adversary_role_traits", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM adversary_role_traits WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertAdversaryRoleTrait(tx *sql.Tx, book SourceBook, slug, roleSlug string, t parse.AdversaryRoleTrait) (rowOutcome, error) {
	var old struct {
		name, roleSlug, rank, description sql.NullString
		status                            string
	}
	err := tx.QueryRow(`
		SELECT name, role_slug, rank, description, detection_status
		FROM adversary_role_traits WHERE slug = ?`, slug).Scan(
		&old.name, &old.roleSlug, &old.rank, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO adversary_role_traits (slug, name, role_slug, rank, description,
			                                   source_book, source_version, source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, t.Name, roleSlug, t.Rank, t.Description, book.Slug, book.Version, t.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != t.Name || old.roleSlug.String != roleSlug ||
		old.rank.String != t.Rank || old.description.String != t.Description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE adversary_role_traits
		SET name = ?, role_slug = ?, rank = ?, description = ?, source_book = ?,
		    source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		t.Name, roleSlug, t.Rank, t.Description, book.Slug, book.Version, t.SourcePage, newStatus, slug)
	return outcome, err
}

// LoadAdversaryRanks upserts the four Minion/Standard/Elite/Solo templates
// and fully replaces adversary_freeform_slots (a pure derived table, no
// detection_status/overrides — owned entirely by this one loader, never
// merged with any other book).
func LoadAdversaryRanks(db *sql.DB, book SourceBook, ranks []parse.AdversaryRank, freeformSlots []parse.AdversaryFreeformSlot) (*LoadReport, error) {
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
	for _, r := range ranks {
		slug := "adversary-rank/" + Slugify(r.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, r.Name))
			continue
		}
		seen[slug] = r.Name
		outcome, err := upsertAdversaryRank(tx, book, slug, r)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
	}

	if err := findVanished(tx, "adversary_ranks", "", book.Slug, seen, report); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`DELETE FROM adversary_freeform_slots`); err != nil {
		return nil, err
	}
	for _, s := range freeformSlots {
		rankSlug := "adversary-rank/" + Slugify(s.RankName)
		if _, err := tx.Exec(
			`INSERT INTO adversary_freeform_slots (rank_slug, level_min, slots) VALUES (?, ?, ?)`,
			rankSlug, s.LevelMin, s.Slots); err != nil {
			return nil, err
		}
	}

	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM adversary_ranks WHERE detection_status = 'needs_review'`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertAdversaryRank(tx *sql.Tx, book SourceBook, slug string, r parse.AdversaryRank) (rowOutcome, error) {
	var old struct {
		name, hpFormula, notes                     sql.NullString
		acBonus, saveBonus, saveDCBonus, initBonus sql.NullInt64
		status                                     string
	}
	err := tx.QueryRow(`
		SELECT name, hp_formula, ac_bonus, save_bonus, save_dc_bonus, init_bonus, notes, detection_status
		FROM adversary_ranks WHERE slug = ?`, slug).Scan(
		&old.name, &old.hpFormula, &old.acBonus, &old.saveBonus, &old.saveDCBonus,
		&old.initBonus, &old.notes, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO adversary_ranks (slug, name, hp_formula, ac_bonus, save_bonus,
			                             save_dc_bonus, init_bonus, notes, source_book,
			                             source_version, source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, r.Name, r.HPFormula, r.ACBonus, r.SaveBonus, r.SaveDCBonus, r.InitBonus,
			r.Notes, book.Slug, book.Version, r.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != r.Name || old.hpFormula.String != r.HPFormula ||
		old.acBonus.Int64 != int64(r.ACBonus) || old.saveBonus.Int64 != int64(r.SaveBonus) ||
		old.saveDCBonus.Int64 != int64(r.SaveDCBonus) || old.initBonus.Int64 != int64(r.InitBonus) ||
		old.notes.String != r.Notes
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE adversary_ranks
		SET name = ?, hp_formula = ?, ac_bonus = ?, save_bonus = ?, save_dc_bonus = ?,
		    init_bonus = ?, notes = ?, source_book = ?, source_version = ?, source_page = ?,
		    detection_status = ?
		WHERE slug = ?`,
		r.Name, r.HPFormula, r.ACBonus, r.SaveBonus, r.SaveDCBonus, r.InitBonus, r.Notes,
		book.Slug, book.Version, r.SourcePage, newStatus, slug)
	return outcome, err
}
