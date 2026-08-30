// Loading the parsed class compendium into the rules database.
//
// Same contract as the clan loader: stable slugs, parsed columns only,
// overrides and human statuses preserved, vanished slugs reported never
// deleted. Detail rows without human-editable content (casting abilities,
// proficiencies, equipment bullets) are rebuilt wholesale on every load.
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sergio/n5e/internal/parse"
)

// statBlockColumnList is the 18 stat_block_* columns' names, in the exact
// order migration 0067_generic_stat_block_columns.sql adds them to both
// class_features and class_options — shared by upsertLeveledFeature and
// upsertClassOption so the two INSERT/UPDATE statements can't drift apart.
const statBlockColumnList = `raw_stat_block_text, stat_block_creature_type, stat_block_ac,
	stat_block_ac_formula_text, stat_block_hp_formula_text, stat_block_speed,
	stat_block_str, stat_block_dex, stat_block_con, stat_block_int, stat_block_wis, stat_block_cha,
	stat_block_saving_throws_text, stat_block_resistances, stat_block_immunities,
	stat_block_condition_immunities, stat_block_senses, stat_block_traits_attacks_text`

// statBlockSetClause is statBlockColumnList's columns rendered as "col = ?"
// pairs, for use in an UPDATE statement's SET clause (statBlockColumnList
// itself is bare names, correct for an INSERT's column list but not valid
// SQL inside SET).
const statBlockSetClause = `raw_stat_block_text = ?, stat_block_creature_type = ?, stat_block_ac = ?,
	stat_block_ac_formula_text = ?, stat_block_hp_formula_text = ?, stat_block_speed = ?,
	stat_block_str = ?, stat_block_dex = ?, stat_block_con = ?, stat_block_int = ?, stat_block_wis = ?, stat_block_cha = ?,
	stat_block_saving_throws_text = ?, stat_block_resistances = ?, stat_block_immunities = ?,
	stat_block_condition_immunities = ?, stat_block_senses = ?, stat_block_traits_attacks_text = ?`

// statBlockArgs returns the 18 stat_block_* column values, in statBlockColumnList's
// own order, for one parse.StatBlockMatch — all NULL when m.Found is false
// (no companion/summon stat card was glued into this row's text). Reference
// data only, see StatBlockFields' own doc comment (internal/parse/statblock.go)
// — not gameplay-critical, so unlike name/description this is simply
// overwritten on every load rather than tracked through decideStatus's own
// auto/manual/needs_review machinery.
func statBlockArgs(m parse.StatBlockMatch) []any {
	if !m.Found {
		return make([]any, 18)
	}
	f := m.Fields
	var ac any
	if f.AC != nil {
		ac = *f.AC
	}
	return []any{
		m.RawStatBlock, f.CreatureType, ac,
		f.ACFormulaText, f.HPFormulaText, f.Speed,
		f.Str, f.Dex, f.Con, f.Int, f.Wis, f.Cha,
		f.SavingThrowsText, f.Resistances, f.Immunities,
		f.ConditionImmunities, f.Senses, f.TraitsAndAttacksText,
	}
}

// LoadClassBook upserts the parsed classes (with their subclass groups,
// subclasses, features and option lists) inside one transaction.
func LoadClassBook(db *sql.DB, book SourceBook, classes []parse.Class, anomalies []parse.Anomaly) (*LoadReport, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	report := &LoadReport{}
	seen := map[string]map[string]string{ // table → slug → display name
		"classes": {}, "class_features": {}, "subclass_groups": {},
		"subclasses": {}, "subclass_features": {}, "class_options": {},
	}
	// remember claims a slug within its table. The book reuses names for
	// DISTINCT features (Puppet Master has two different "Puppeteer Chassis"
	// features), so a collision gets a -2/-3 suffix instead of being dropped
	// — and is still reported for human review. Book order is stable, so
	// re-runs assign the same suffixes.
	remember := func(table, slug, name string) string {
		if _, dup := seen[table][slug]; !dup {
			seen[table][slug] = name
			return slug
		}
		base := slug
		for n := 2; ; n++ {
			slug = fmt.Sprintf("%s-%d", base, n)
			if _, dup := seen[table][slug]; !dup {
				break
			}
		}
		report.Duplicates = append(report.Duplicates,
			fmt.Sprintf("%s: reused name %q stored as %s", base, name, slug))
		seen[table][slug] = name
		return slug
	}

	for _, c := range classes {
		classSlug := remember("classes", "class/"+Slugify(c.Name), c.Name)
		outcome, err := upsertClass(tx, book, classSlug, c)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", classSlug, err)
		}
		report.count(outcome, classSlug)
		if err := rebuildClassDetail(tx, classSlug, c); err != nil {
			return nil, fmt.Errorf("%s detail: %w", classSlug, err)
		}

		for i, f := range c.Features {
			slug := remember("class_features", classSlug+"/feature/"+Slugify(f.Name), f.Name)
			outcome, err := upsertLeveledFeature(tx, book, "class_features", "class_slug",
				slug, classSlug, f, i)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", slug, err)
			}
			report.count(outcome, slug)
		}

		if c.PuppetToolStatBlock != nil {
			outcome, err := upsertPuppetToolStatBlock(tx, book, classSlug, c.PuppetToolStatBlock)
			if err != nil {
				return nil, fmt.Errorf("%s puppet tool stat block: %w", classSlug, err)
			}
			report.count(outcome, classSlug+"/puppet-tool-stat-block")
		}

		if c.TitanBaseText != "" {
			outcome, err := upsertTitanUnitCard(tx, book, classSlug, c.TitanBaseText)
			if err != nil {
				return nil, fmt.Errorf("%s titan unit card: %w", classSlug, err)
			}
			report.count(outcome, classSlug+"/titan-unit-card")
		}

		// Subclass slugs by name, for scoping option lists below.
		subclassSlugs := map[string]string{}
		if c.Group != nil {
			groupSlug := remember("subclass_groups", classSlug+"/group/"+Slugify(c.Group.DisplayName), c.Group.DisplayName)
			outcome, err := upsertSubclassGroup(tx, book, groupSlug, classSlug, c.Group)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", groupSlug, err)
			}
			report.count(outcome, groupSlug)
			for _, a := range c.Group.Subclasses {
				subclassSlug := remember("subclasses", groupSlug+"/"+Slugify(a.Name), a.Name)
				subclassSlugs[a.Name] = subclassSlug
				outcome, err := upsertSubclass(tx, book, subclassSlug, groupSlug, a)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", subclassSlug, err)
				}
				report.count(outcome, subclassSlug)
				for i, f := range a.Features {
					slug := remember("subclass_features", subclassSlug+"/feature/"+Slugify(f.Name), f.Name)
					cf := parse.ClassFeature{Name: f.Name, Level: f.Level,
						Description: f.Description, SourcePage: f.SourcePage}
					outcome, err := upsertLeveledFeature(tx, book, "subclass_features",
						"subclass_slug", slug, subclassSlug, cf, i)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", slug, err)
					}
					report.count(outcome, slug)
				}
			}
		}

		for _, list := range c.OptionLists {
			var subclassRef any // NULL for class-wide lists
			if s, ok := subclassSlugs[list.SubclassName]; ok && list.SubclassName != "" {
				subclassRef = s
			}
			listSlug := Slugify(list.Name)
			for i, o := range list.Options {
				slug := remember("class_options", classSlug+"/option/"+listSlug+"/"+Slugify(o.Name), o.Name)
				outcome, err := upsertClassOption(tx, book, slug, classSlug, subclassRef, list.Name, o, i)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", slug, err)
				}
				report.count(outcome, slug)
			}
		}
	}

	for _, table := range []string{"classes", "class_features", "subclass_groups",
		"subclasses", "subclass_features", "class_options"} {
		if err := findVanished(tx, table, "", book.Slug, seen[table], report); err != nil {
			return nil, err
		}
	}

	if err := tx.QueryRow(`
		SELECT (SELECT COUNT(*) FROM classes WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM class_features WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM subclass_groups WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM subclasses WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM subclass_features WHERE detection_status = 'needs_review')
		     + (SELECT COUNT(*) FROM class_options WHERE detection_status = 'needs_review')`,
	).Scan(&report.NeedsReview); err != nil {
		return nil, err
	}

	return report, tx.Commit()
}

func upsertClass(tx *sql.Tx, book SourceBook, slug string, c parse.Class) (rowOutcome, error) {
	var old struct {
		name, description, quickBuild sql.NullString
		hitDie, chakraDie             sql.NullInt64
		status                        string
	}
	err := tx.QueryRow(`
		SELECT name, description, quick_build, hit_die, chakra_die, detection_status
		FROM classes WHERE slug = ?`, slug).Scan(
		&old.name, &old.description, &old.quickBuild, &old.hitDie, &old.chakraDie, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO classes (slug, name, hit_die, chakra_die, description,
			                     quick_build, source_book, source_version,
			                     source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, c.Name, c.HitDie, c.ChakraDie, c.Description, c.QuickBuild,
			book.Slug, book.Version, c.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != c.Name ||
		old.description.String != c.Description ||
		old.quickBuild.String != c.QuickBuild ||
		old.hitDie.Int64 != int64(c.HitDie) ||
		old.chakraDie.Int64 != int64(c.ChakraDie)
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE classes
		SET name = ?, hit_die = ?, chakra_die = ?, description = ?, quick_build = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		c.Name, c.HitDie, c.ChakraDie, c.Description, c.QuickBuild,
		book.Slug, book.Version, c.SourcePage, newStatus, slug)
	return outcome, err
}

// rebuildClassDetail replaces the class's casting, proficiency and equipment
// rows — pure parser output with no override columns, so delete + insert.
func rebuildClassDetail(tx *sql.Tx, classSlug string, c parse.Class) error {
	for _, table := range []string{"class_casting", "class_proficiencies", "class_equipment_options"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE class_slug = ?`, classSlug); err != nil {
			return err
		}
	}
	for _, cast := range c.Casting {
		if _, err := tx.Exec(`
			INSERT INTO class_casting (class_slug, discipline, ability)
			VALUES (?, ?, ?)`, classSlug, cast.Discipline, cast.Ability); err != nil {
			return err
		}
	}
	for _, p := range c.Proficiencies {
		var chooseN any
		if p.ChooseN > 0 {
			chooseN = p.ChooseN
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO class_proficiencies (class_slug, kind, value, choose_n)
			VALUES (?, ?, ?, ?)`, classSlug, p.Kind, p.Value, chooseN); err != nil {
			return err
		}
	}
	// Each printed bullet is one option group; "(a) … or (b) …" alternatives
	// are split into real per-choice rows (splitEquipmentChoice) rather than
	// kept as one bundled description string, so each alternative can carry
	// its own resolved item_slug where the text names one specific,
	// purchasable item (equipmentNameLookup) — bundles and category/
	// free-choice text ("Simple Weapon", "Toolkit of your choice") stay
	// unresolved on purpose, see that map's doc comment.
	for i, e := range c.Equipment {
		for choiceIdx, opt := range splitEquipmentChoice(e) {
			var itemSlug any
			if opt.ItemSlug != "" {
				itemSlug = opt.ItemSlug
			}
			if _, err := tx.Exec(`
				INSERT INTO class_equipment_options (class_slug, group_idx, choice_idx, description, item_slug, quantity)
				VALUES (?, ?, ?, ?, ?, ?)`, classSlug, i, choiceIdx, opt.Description, itemSlug, opt.Quantity); err != nil {
				return err
			}
		}
	}
	return nil
}

// upsertLeveledFeature covers class_features and subclass_features — the
// two tables share the same shape apart from the owner column. Migration
// 0067 only added the stat_block_* columns to class_features (the one table
// with proven instances of the bug so far), so subclass_features rows never
// get those columns in their INSERT/UPDATE — hasStatBlockCols gates that.
func upsertLeveledFeature(tx *sql.Tx, book SourceBook, table, ownerCol, slug, owner string, f parse.ClassFeature, order int) (rowOutcome, error) {
	hasStatBlockCols := table == "class_features"
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
		FROM `+table+` WHERE slug = ?`, slug).Scan(
		&old.name, &old.description, &old.level, &old.status)

	if err == sql.ErrNoRows {
		args := []any{slug, owner, f.Name, level, f.Description, order,
			book.Slug, book.Version, f.SourcePage}
		columns := ""
		placeholders := "?, ?, ?, ?, ?, ?, ?, ?, ?"
		if hasStatBlockCols {
			args = append(args, statBlockArgs(f.StatBlock)...)
			columns = ", " + statBlockColumnList
			placeholders += strings.Repeat(", ?", 18)
		}
		_, err := tx.Exec(`
			INSERT INTO `+table+` (slug, `+ownerCol+`, name, level, description,
			                       sort_order, source_book, source_version,
			                       source_page`+columns+`, detection_status)
			VALUES (`+placeholders+`, 'auto')`,
			args...)
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
	args := []any{f.Name, level, f.Description, order,
		book.Slug, book.Version, f.SourcePage}
	setClause := ""
	if hasStatBlockCols {
		args = append(args, statBlockArgs(f.StatBlock)...)
		setClause = statBlockSetClause + ", "
	}
	args = append(args, newStatus, slug)
	_, err = tx.Exec(`
		UPDATE `+table+`
		SET name = ?, level = ?, description = ?, sort_order = ?,
		    source_book = ?, source_version = ?, source_page = ?,
		    `+setClause+`detection_status = ?
		WHERE slug = ?`,
		args...)
	return outcome, err
}

func upsertSubclassGroup(tx *sql.Tx, book SourceBook, slug, classSlug string, g *parse.SubclassGroup) (rowOutcome, error) {
	levels := make([]string, len(g.SelectionLevels))
	for i, l := range g.SelectionLevels {
		levels[i] = fmt.Sprint(l)
	}
	selection := strings.Join(levels, ",")

	var old struct {
		displayName, selection, description sql.NullString
		status                              string
	}
	err := tx.QueryRow(`
		SELECT display_name, selection_levels, description, detection_status
		FROM subclass_groups WHERE slug = ?`, slug).Scan(
		&old.displayName, &old.selection, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO subclass_groups (slug, class_slug, display_name,
			                              selection_levels, description, source_book,
			                              source_version, source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, classSlug, g.DisplayName, selection, g.Description,
			book.Slug, book.Version, g.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.displayName.String != g.DisplayName ||
		old.selection.String != selection ||
		old.description.String != g.Description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE subclass_groups
		SET display_name = ?, selection_levels = ?, description = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		g.DisplayName, selection, g.Description,
		book.Slug, book.Version, g.SourcePage, newStatus, slug)
	return outcome, err
}

func upsertSubclass(tx *sql.Tx, book SourceBook, slug, groupSlug string, a parse.Subclass) (rowOutcome, error) {
	var old struct {
		name, description sql.NullString
		status            string
	}
	err := tx.QueryRow(`
		SELECT name, description, detection_status
		FROM subclasses WHERE slug = ?`, slug).Scan(&old.name, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO subclasses (slug, group_slug, name, description, source_book,
			                        source_version, source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, groupSlug, a.Name, a.Description, book.Slug, book.Version, a.SourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != a.Name || old.description.String != a.Description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE subclasses
		SET name = ?, description = ?, source_book = ?, source_version = ?,
		    source_page = ?, detection_status = ?
		WHERE slug = ?`,
		a.Name, a.Description, book.Slug, book.Version, a.SourcePage, newStatus, slug)
	return outcome, err
}

func upsertClassOption(tx *sql.Tx, book SourceBook, slug, classSlug string, subclassRef any, listName string, o parse.ClassOption, order int) (rowOutcome, error) {
	var old struct {
		name, prereq, description, subclassSlug sql.NullString
		status                                  string
	}
	err := tx.QueryRow(`
		SELECT name, prerequisites, description, subclass_slug, detection_status
		FROM class_options WHERE slug = ?`, slug).Scan(
		&old.name, &old.prereq, &old.description, &old.subclassSlug, &old.status)

	if err == sql.ErrNoRows {
		args := append([]any{slug, classSlug, subclassRef, listName, o.Name, o.Prerequisites, o.Description,
			order, book.Slug, book.Version, o.SourcePage}, statBlockArgs(o.StatBlock)...)
		_, err := tx.Exec(`
			INSERT INTO class_options (slug, class_slug, subclass_slug, list_name,
			                           name, prerequisites, description, sort_order,
			                           source_book, source_version, source_page,
			                           `+statBlockColumnList+`, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			args...)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	newSubclassSlug, _ := subclassRef.(string) // "" for the NULL/class-wide case
	changed := old.name.String != o.Name ||
		old.prereq.String != o.Prerequisites ||
		old.description.String != o.Description ||
		old.subclassSlug.String != newSubclassSlug
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	args := append([]any{o.Name, o.Prerequisites, o.Description, order, subclassRef, listName,
		book.Slug, book.Version, o.SourcePage}, statBlockArgs(o.StatBlock)...)
	args = append(args, newStatus, slug)
	_, err = tx.Exec(`
		UPDATE class_options
		SET name = ?, prerequisites = ?, description = ?, sort_order = ?,
		    subclass_slug = ?, list_name = ?, source_book = ?, source_version = ?,
		    source_page = ?, `+statBlockSetClause+`, detection_status = ?
		WHERE slug = ?`,
		args...)
	return outcome, err
}

// upsertPuppetToolStatBlock covers puppet_tool_stat_block — a singleton row
// per class_slug (only Puppet Master has one), same upsert shape as every
// other table in this file. Migration 0028 seeds the already-shipped bad
// class_features row's replacement as detection_status='manual'; a real
// re-ingest with the parser fix in place produces the identical values
// (confirmed against the live book text) and leaves that status alone
// unless something actually changed.
func upsertPuppetToolStatBlock(tx *sql.Tx, book SourceBook, classSlug string, sb *parse.PuppetToolStatBlock) (rowOutcome, error) {
	var old struct {
		creatureType, profText, traits sql.NullString
		hpBase, hpConBonus, speed      sql.NullInt64
		str, dex, con, intl, wis, cha  sql.NullInt64
		passivePerception              sql.NullInt64
		status                         string
	}
	err := tx.QueryRow(`
		SELECT creature_type, proficiency_rule_text, hp_base, hp_con_bonus_add, speed,
		       str_score, dex_score, con_score, int_score, wis_score, cha_score,
		       passive_perception, traits_text, detection_status
		FROM puppet_tool_stat_block WHERE class_slug = ?`, classSlug).Scan(
		&old.creatureType, &old.profText, &old.hpBase, &old.hpConBonus, &old.speed,
		&old.str, &old.dex, &old.con, &old.intl, &old.wis, &old.cha,
		&old.passivePerception, &old.traits, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO puppet_tool_stat_block (class_slug, creature_type, proficiency_rule_text,
			    hp_base, hp_con_bonus_add, speed, str_score, dex_score, con_score,
			    int_score, wis_score, cha_score, passive_perception, traits_text,
			    source_book, source_version, source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			classSlug, sb.CreatureType, sb.ProficiencyRuleText, sb.HPBase, sb.HPConBonusAdd,
			sb.Speed, sb.Str, sb.Dex, sb.Con, sb.Int, sb.Wis, sb.Cha, sb.PassivePerception,
			sb.TraitsText, book.Slug, book.Version, nil)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.creatureType.String != sb.CreatureType ||
		old.profText.String != sb.ProficiencyRuleText ||
		old.hpBase.Int64 != int64(sb.HPBase) ||
		old.hpConBonus.Int64 != int64(sb.HPConBonusAdd) ||
		old.speed.Int64 != int64(sb.Speed) ||
		old.str.Int64 != int64(sb.Str) || old.dex.Int64 != int64(sb.Dex) || old.con.Int64 != int64(sb.Con) ||
		old.intl.Int64 != int64(sb.Int) || old.wis.Int64 != int64(sb.Wis) || old.cha.Int64 != int64(sb.Cha) ||
		old.passivePerception.Int64 != int64(sb.PassivePerception) ||
		old.traits.String != sb.TraitsText
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE puppet_tool_stat_block
		SET creature_type = ?, proficiency_rule_text = ?, hp_base = ?, hp_con_bonus_add = ?,
		    speed = ?, str_score = ?, dex_score = ?, con_score = ?, int_score = ?,
		    wis_score = ?, cha_score = ?, passive_perception = ?, traits_text = ?,
		    source_book = ?, source_version = ?, detection_status = ?
		WHERE class_slug = ?`,
		sb.CreatureType, sb.ProficiencyRuleText, sb.HPBase, sb.HPConBonusAdd, sb.Speed,
		sb.Str, sb.Dex, sb.Con, sb.Int, sb.Wis, sb.Cha, sb.PassivePerception, sb.TraitsText,
		book.Slug, book.Version, newStatus, classSlug)
	return outcome, err
}

// upsertTitanUnitCard covers titan_unit_card — a singleton row per
// class_slug (only Science-Nin has one), same upsert shape as
// upsertPuppetToolStatBlock just above, for the raw (unparsed — see
// Class.TitanBaseText's doc comment) text field.
func upsertTitanUnitCard(tx *sql.Tx, book SourceBook, classSlug, rawText string) (rowOutcome, error) {
	var old struct {
		rawText sql.NullString
		status  string
	}
	err := tx.QueryRow(`
		SELECT raw_text, detection_status FROM titan_unit_card WHERE class_slug = ?`,
		classSlug).Scan(&old.rawText, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO titan_unit_card (class_slug, raw_text, source_book, source_version, source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, 'auto')`,
			classSlug, rawText, book.Slug, book.Version, nil)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.rawText.String != rawText
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE titan_unit_card
		SET raw_text = ?, source_book = ?, source_version = ?, detection_status = ?
		WHERE class_slug = ?`,
		rawText, book.Slug, book.Version, newStatus, classSlug)
	return outcome, err
}
