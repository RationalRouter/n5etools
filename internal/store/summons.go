// Loading the jutsu compendium's Summoning chapter (summon_tribes and its
// four related tables). New entity type: creature templates a summoner
// fills in at cast time, not characters or jutsu.
package store

import (
	"database/sql"
	"fmt"

	"github.com/sergio/n5e/internal/parse"
)

// LoadSummonTribes upserts every tribe and rebuilds its derived detail rows
// in one transaction.
func LoadSummonTribes(db *sql.DB, book SourceBook, tribes []parse.SummonTribe) (*LoadReport, error) {
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
	for _, tr := range tribes {
		slug := "summon/" + Slugify(tr.Name)
		if other, dup := seen[slug]; dup {
			report.Duplicates = append(report.Duplicates,
				fmt.Sprintf("%s: %q and %q collide", slug, other, tr.Name))
			continue
		}
		seen[slug] = tr.Name

		outcome, err := upsertSummonTribe(tx, book, slug, tr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", slug, err)
		}
		report.count(outcome, slug)
		if err := rebuildSummonTribeDetail(tx, slug, tr); err != nil {
			return nil, fmt.Errorf("%s detail: %w", slug, err)
		}

		for i, f := range tr.Features {
			featSlug := slug + "/feature/" + Slugify(f.Name)
			fOutcome, err := upsertSummonFeature(tx, book, featSlug, slug, f, i)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", featSlug, err)
			}
			report.count(fOutcome, featSlug)
		}
	}

	if err := findVanished(tx, "summon_tribes", "", book.Slug, seen, report); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(`
		SELECT (SELECT COUNT(*) FROM summon_tribes WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM summon_tribe_features WHERE detection_status = 'needs_review')`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}
	return report, tx.Commit()
}

func upsertSummonTribe(tx *sql.Tx, book SourceBook, slug string, tr parse.SummonTribe) (rowOutcome, error) {
	var old struct {
		name, summonType, description, defensiveAbility, savingThrows sql.NullString
		skills, senses, saveDC, attackBonus, specialty                sql.NullString
		toughness                                                     sql.NullInt64
		status                                                        string
	}
	err := tx.QueryRow(`
		SELECT name, summon_type, description, toughness, defensive_ability,
		       saving_throws, skills, senses, jutsu_save_dc_text,
		       jutsu_attack_bonus_text, jutsu_specialty_text, detection_status
		FROM summon_tribes WHERE slug = ?`, slug).Scan(
		&old.name, &old.summonType, &old.description, &old.toughness, &old.defensiveAbility,
		&old.savingThrows, &old.skills, &old.senses, &old.saveDC, &old.attackBonus,
		&old.specialty, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO summon_tribes (slug, name, summon_type, description, toughness,
			                           defensive_ability, saving_throws, skills, senses,
			                           jutsu_save_dc_text, jutsu_attack_bonus_text,
			                           jutsu_specialty_text, source_book, source_version,
			                           source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, tr.Name, tr.SummonType, tr.Description, tr.Toughness, tr.DefensiveAbility,
			tr.SavingThrows, tr.Skills, tr.Senses, tr.JutsuSaveDCText, tr.JutsuAttackText,
			tr.JutsuSpecialty, book.Slug, book.Version, tr.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != tr.Name ||
		old.summonType.String != tr.SummonType ||
		old.description.String != tr.Description ||
		old.toughness.Int64 != int64(tr.Toughness) ||
		old.defensiveAbility.String != tr.DefensiveAbility ||
		old.savingThrows.String != tr.SavingThrows ||
		old.skills.String != tr.Skills ||
		old.senses.String != tr.Senses ||
		old.saveDC.String != tr.JutsuSaveDCText ||
		old.attackBonus.String != tr.JutsuAttackText ||
		old.specialty.String != tr.JutsuSpecialty
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE summon_tribes
		SET name = ?, summon_type = ?, description = ?, toughness = ?,
		    defensive_ability = ?, saving_throws = ?, skills = ?, senses = ?,
		    jutsu_save_dc_text = ?, jutsu_attack_bonus_text = ?, jutsu_specialty_text = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		tr.Name, tr.SummonType, tr.Description, tr.Toughness, tr.DefensiveAbility,
		tr.SavingThrows, tr.Skills, tr.Senses, tr.JutsuSaveDCText, tr.JutsuAttackText,
		tr.JutsuSpecialty, book.Slug, book.Version, tr.SourcePage, newStatus, slug)
	return outcome, err
}

// rebuildSummonTribeDetail replaces the roles/attacks/progression rows —
// pure parser output with no override columns, so delete + insert.
func rebuildSummonTribeDetail(tx *sql.Tx, tribeSlug string, tr parse.SummonTribe) error {
	for _, table := range []string{"summon_tribe_roles", "summon_tribe_attacks", "summon_tribe_progression"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE tribe_slug = ?`, tribeSlug); err != nil {
			return err
		}
	}
	for _, r := range tr.Roles {
		if _, err := tx.Exec(`
			INSERT INTO summon_tribe_roles (tribe_slug, role_name, description)
			VALUES (?, ?, ?)`, tribeSlug, r.Name, r.Description); err != nil {
			return err
		}
	}
	for _, a := range tr.Attacks {
		if _, err := tx.Exec(`
			INSERT INTO summon_tribe_attacks (tribe_slug, name, description)
			VALUES (?, ?, ?)`, tribeSlug, a.Name, a.Description); err != nil {
			return err
		}
	}
	for _, p := range tr.Progression {
		if _, err := tx.Exec(`
			INSERT INTO summon_tribe_progression (tribe_slug, rank, level, size_text, stats_text)
			VALUES (?, ?, ?, ?, ?)`, tribeSlug, p.Rank, p.Level, p.SizeText, p.StatsText); err != nil {
			return err
		}
	}
	return nil
}

func upsertSummonFeature(tx *sql.Tx, book SourceBook, slug, tribeSlug string, f parse.SummonFeature, order int) (rowOutcome, error) {
	var old struct {
		name, rank, description sql.NullString
		status                  string
	}
	err := tx.QueryRow(`
		SELECT name, rank, description, detection_status
		FROM summon_tribe_features WHERE slug = ?`, slug).Scan(
		&old.name, &old.rank, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO summon_tribe_features (slug, tribe_slug, name, rank, description,
			                                   sort_order, source_book, source_version,
			                                   source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, tribeSlug, f.Name, f.Rank, f.Description, order,
			book.Slug, book.Version, f.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != f.Name ||
		old.rank.String != f.Rank ||
		old.description.String != f.Description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE summon_tribe_features
		SET name = ?, rank = ?, description = ?, sort_order = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		f.Name, f.Rank, f.Description, order, book.Slug, book.Version,
		f.SourcePage, newStatus, slug)
	return outcome, err
}
