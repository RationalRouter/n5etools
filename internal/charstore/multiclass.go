// Multiclassing (core book, page 179, "Legacy Rules" chapter — see
// internal/parse/multiclass.go's own header for why it's kept despite the
// author's own Archetype-system replacement). The book's three tables
// (class_multiclass_rules in rules.db: ability_prereq_text,
// proficiencies_text, jutsu_per_level_text) are free-text, printed prose —
// and genuinely inconsistent prose at that (one row, Puppet Master, is
// missing a number: "Constitution & Intelligence 14" instead of
// "Constitution 14 & Intelligence 14"; several proficiency grants name
// items whose catalog spelling differs from the class's own
// class_proficiencies row, e.g. "Cooking Tools" vs the real "Cooking Kit").
// A regex parser over that prose would either mis-handle these or need as
// much hand-verification as just transcribing the 11 rows directly — so,
// same precedent as equipment_choices.go and skilldescriptions.go, they're
// hand-curated Go tables below, each verified against the book and the live
// rules.db catalog (query trail in the design doc, not repeated per-row
// here). Never touches rulesDB: every fixed value here is already the
// resolved, catalog-exact string a caller can hand straight to
// character_proficiencies.
package charstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// abilityClause is one AND'd term of a multiclass ability-score
// prerequisite. Abilities are OR'd within a clause (Taijutsu Specialist:
// "Strength or Dexterity 14" is one clause with two abilities) — the
// character qualifies for the clause if ANY listed ability meets Min.
type abilityClause struct {
	Abilities []string // 3-letter codes: str/dex/con/int/wis/cha
	Min       int
}

// multiclassPrereqs is the Multiclassing Prerequisites table. The Puppet
// Master row repairs the book's missing "14" after Constitution (see
// package doc) — both abilities clearly share the one printed threshold.
var multiclassPrereqs = map[string][]abilityClause{
	"class/genjutsu-specialist":    {{[]string{"wis"}, 14}, {[]string{"cha"}, 14}},
	"class/hunter-nin":             {{[]string{"dex"}, 14}, {[]string{"int"}, 14}},
	"class/intelligence-operative": {{[]string{"int"}, 14}, {[]string{"cha"}, 14}},
	"class/medical-nin":            {{[]string{"int"}, 14}, {[]string{"wis"}, 14}},
	"class/ninjutsu-specialist":    {{[]string{"con"}, 14}, {[]string{"int"}, 14}},
	"class/scout-nin":              {{[]string{"str"}, 13}, {[]string{"int"}, 13}, {[]string{"wis"}, 13}},
	"class/taijutsu-specialist":    {{[]string{"str", "dex"}, 14}, {[]string{"con"}, 14}},
	"class/weapon-specialist":      {{[]string{"str"}, 14}, {[]string{"dex"}, 14}},
	"class/puppet-master":          {{[]string{"con"}, 14}, {[]string{"int"}, 14}},
	"class/cooking-nin":            {{[]string{"int"}, 14}, {[]string{"cha"}, 14}},
	"class/science-nin":            {{[]string{"int"}, 16}},
}

// MeetsMulticlassPrereq reports whether scores (3-letter ability codes ->
// score) satisfies classSlug's ability-score minimums to multiclass into
// it. An unknown classSlug (a stale slug from a rules update) reports true
// rather than blocking — same "don't fail closed on missing rows" leniency
// loadClasses already applies to a dropped class slug.
func MeetsMulticlassPrereq(scores map[string]int, classSlug string) bool {
	clauses, ok := multiclassPrereqs[classSlug]
	if !ok {
		return true
	}
	for _, clause := range clauses {
		met := false
		for _, ab := range clause.Abilities {
			if scores[ab] >= clause.Min {
				met = true
				break
			}
		}
		if !met {
			return false
		}
	}
	return true
}

// UnmetMulticlassClasses checks every classSlug in classSlugs against
// scores and returns the ones that fail — used both for the new class being
// added and, per the book's own "you must meet the prerequisites for BOTH
// your current class and your new one" rule, every class the character
// already holds (ability scores can change after a class was taken, e.g. an
// ASI moving a score back below a threshold was never possible before
// multiclassing existed to care).
func UnmetMulticlassClasses(scores map[string]int, classSlugs []string) []string {
	var unmet []string
	for _, slug := range classSlugs {
		if !MeetsMulticlassPrereq(scores, slug) {
			unmet = append(unmet, slug)
		}
	}
	return unmet
}

// profGrant is one proficiency a multiclass character gains from a class
// other than their first. Kind/Value match character_proficiencies'
// columns directly for a fixed grant (Choice == ""); otherwise Choice names
// which caller-supplied pick to use instead, and Value is unused.
type profGrant struct {
	Kind   string // "armor", "weapon", "tool", "skill" — never "saving_throw" (multiclassing never grants those) or "language"
	Value  string
	Choice string // "" | "class_skill_pool" | "toolkit" | "martial_weapon"
}

const (
	ChoiceClassSkillPool = "class_skill_pool" // 1 skill chosen from the class's own first-level skill_choice pool
	ChoiceToolkit        = "toolkit"          // 1 toolkit chosen from the general starting-tier catalog
	ChoiceMartialWeapon  = "martial_weapon"   // 1 weapon chosen from the catalog's weapon_category='martial'
)

// multiclassGrants is the Multiclassing Proficiencies table, resolved to
// the exact strings each class's own class_proficiencies rows already use
// (so a native and a multiclass character end up with identically-spelled
// proficiencies) — see the design doc for the row-by-row verification
// against rules.db this table was transcribed from.
var multiclassGrants = map[string][]profGrant{
	"class/genjutsu-specialist": {
		{Kind: "skill", Value: "Illusions"},
		{Kind: "skill", Value: "Deception"},
	},
	"class/hunter-nin": {
		{Kind: "armor", Value: "Light armor"},
		{Choice: ChoiceClassSkillPool},
		{Kind: "tool", Value: "Trackers Kit"},
	},
	"class/intelligence-operative": {
		{Kind: "armor", Value: "Light armor"},
		{Kind: "armor", Value: "medium armor"},
		{Choice: ChoiceToolkit},
	},
	"class/medical-nin": {
		{Kind: "armor", Value: "Light armor"},
		{Kind: "tool", Value: "Medicine Kit"},
		{Kind: "skill", Value: "Medicine"},
	},
	"class/ninjutsu-specialist": {
		{Kind: "skill", Value: "Ninshou"},
		{Kind: "skill", Value: "Chakra Control"},
	},
	"class/scout-nin": {
		{Kind: "armor", Value: "Light armor"},
		{Kind: "armor", Value: "medium armor"},
		{Choice: ChoiceMartialWeapon},
		{Choice: ChoiceClassSkillPool},
	},
	"class/taijutsu-specialist": {
		{Kind: "weapon", Value: "Combat Bracers"},
		{Kind: "weapon", Value: "Iron Claws"},
		{Kind: "skill", Value: "Martial Arts"},
	},
	"class/weapon-specialist": {
		{Kind: "armor", Value: "Heavy Armor"},
		{Kind: "armor", Value: "Light armor"},
		{Kind: "armor", Value: "medium armor"},
		{Kind: "weapon", Value: "All Simple and Martial Weapons"},
	},
	"class/puppet-master": {
		{Kind: "armor", Value: "Light armor"},
		{Choice: ChoiceClassSkillPool},
		{Kind: "tool", Value: "Weaponsmith kit"},
	},
	"class/cooking-nin": {
		{Kind: "armor", Value: "Light armor"},
		{Choice: ChoiceClassSkillPool},
		{Kind: "tool", Value: "Cooking Kit"},
	},
	"class/science-nin": {
		{Kind: "armor", Value: "Light armor"},
		{Kind: "skill", Value: "Ninshou"},
		{Choice: ChoiceToolkit},
	},
}

// multiclassJutsuRate is the Multiclassing Jutsu Known table: an additional
// class beyond the first grants +1 known jutsu every N levels in that
// class, N given here.
var multiclassJutsuRate = map[string]int{
	"class/genjutsu-specialist":    2,
	"class/hunter-nin":             3,
	"class/intelligence-operative": 2,
	"class/medical-nin":            1,
	"class/ninjutsu-specialist":    1,
	"class/scout-nin":              2,
	"class/taijutsu-specialist":    3,
	"class/weapon-specialist":      3,
	"class/puppet-master":          2,
	"class/cooking-nin":            2,
	"class/science-nin":            2,
}

// MulticlassJutsuRate returns classSlug's "+1 every N levels" rate, or 0 if
// unknown (a stale slug — callers should treat that as "grants nothing
// extra" rather than dividing by zero).
func MulticlassJutsuRate(classSlug string) int {
	return multiclassJutsuRate[classSlug]
}

// MulticlassGrantChoices reports which Choice kinds classSlug's multiclass
// grant needs the caller to supply, so a handler can render exactly the
// right picker fields without hand-duplicating the table above.
func MulticlassGrantChoices(classSlug string) []string {
	var choices []string
	for _, g := range multiclassGrants[classSlug] {
		if g.Choice != "" {
			choices = append(choices, g.Choice)
		}
	}
	return choices
}

// ErrLevelCapExceeded is returned by SetClassLevel/AddMulticlass when a
// level change would push the character's TOTAL level (summed across every
// class) above 20 — xp_levels only has rows 1-20, so charsheet.Compute's
// proficiency-bonus lookup would fail past that regardless of how the game
// intends multiclass leveling to feel; this is a hard engine limit, not a
// house rule.
var ErrLevelCapExceeded = errors.New("total character level cannot exceed 20")

// ErrMissingChoice is returned by AddMulticlass when a class's grant table
// requires a choice (skill/toolkit/weapon) the caller didn't supply.
var ErrMissingChoice = errors.New("multiclass grant requires a choice the caller didn't supply")

// TotalClassLevels sums character_classes.levels across every class the
// character has, optionally excluding one class_slug (pass "" to exclude
// nothing) — the shared building block for the total-level-20 cap, used
// both when raising an existing class's level and when adding a new one.
func TotalClassLevels(charDB *sql.DB, characterID int64, excludeClassSlug string) (int, error) {
	var total sql.NullInt64
	err := charDB.QueryRow(
		`SELECT SUM(levels) FROM character_classes WHERE character_id = ? AND class_slug != ?`,
		characterID, excludeClassSlug,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	return int(total.Int64), nil
}

// SetClassLevel sets classSlug's level directly (generalizes the old
// SetLevel, which only ever touched the primary class — SetLevel is now a
// thin wrapper around this). Rejects a level that would push the
// character's total level over 20 (ErrLevelCapExceeded) — this is the fix
// for the crash bug multiclassing exposes in charsheet.Compute, see
// ErrLevelCapExceeded's own doc.
//
// Writes no character_level_gains rows, same reasoning as the old SetLevel:
// a level with no stored row takes the fixed (unrolled) gain at read time
// in charsheet.Compute, so raising or lowering a level needs no stored
// state of its own beyond the number itself. Rows above the new level are
// trimmed so lowering a level doesn't leave a stale roll waiting to be
// counted again on a later re-raise.
func SetClassLevel(charDB *sql.DB, characterID int64, classSlug string, level int) error {
	if level < 1 || level > 20 {
		return fmt.Errorf("level %d out of range 1-20", level)
	}
	othersTotal, err := TotalClassLevels(charDB, characterID, classSlug)
	if err != nil {
		return fmt.Errorf("sum other class levels: %w", err)
	}
	if othersTotal+level > 20 {
		return ErrLevelCapExceeded
	}

	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE character_classes SET levels = ? WHERE character_id = ? AND class_slug = ?`,
		level, characterID, classSlug,
	)
	if err != nil {
		return fmt.Errorf("set class level: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("check class level update: %w", err)
	} else if n == 0 {
		return fmt.Errorf("character %d has no class %q", characterID, classSlug)
	}
	if _, err := tx.Exec(
		`DELETE FROM character_level_gains WHERE character_id = ? AND class_slug = ? AND class_level > ?`,
		characterID, classSlug, level,
	); err != nil {
		return fmt.Errorf("trim level gains: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM character_overrides WHERE character_id = ? AND field = 'level'`, characterID,
	); err != nil {
		return fmt.Errorf("clear legacy level override: %w", err)
	}
	return tx.Commit()
}

// AddMulticlass adds classSlug as a further class beyond the character's
// first (use SetClass for the first class — this never touches an existing
// class's rows). Ability-score prerequisites are the caller's
// responsibility to check (UnmetMulticlassClasses) before calling this,
// same "the handler is where a form value stops being arbitrary input"
// convention SetClass's own chosenSkills/chosenToolkits already follow —
// this function trusts classSlug/level/choices are already valid and only
// re-checks the level-20 cap, which is a hard engine limit rather than a
// UI-level validation.
//
// Writes no level-1 hit/chakra-gain row: the book only grants the
// max-value level-1 gain "when you are a 1st-level CHARACTER", i.e. for
// your very first class ever — charsheet.Compute's existing
// `first := i==0 && lvl==1` already implements that correctly for any
// class with no stored gain row, so a fresh non-primary class needs no
// row at all, at level 1 or any level after it.
func AddMulticlass(charDB *sql.DB, characterID int64, classSlug string, level int, chosenSkill, chosenToolkit, chosenWeapon string) error {
	if level < 1 || level > 20 {
		return fmt.Errorf("level %d out of range 1-20", level)
	}
	grants, ok := multiclassGrants[classSlug]
	if !ok {
		return fmt.Errorf("no multiclass rules for %q", classSlug)
	}
	othersTotal, err := TotalClassLevels(charDB, characterID, "")
	if err != nil {
		return fmt.Errorf("sum existing class levels: %w", err)
	}
	if othersTotal+level > 20 {
		return ErrLevelCapExceeded
	}

	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var nextOrder sql.NullInt64
	if err := tx.QueryRow(
		`SELECT MAX(order_index) FROM character_classes WHERE character_id = ?`, characterID,
	).Scan(&nextOrder); err != nil {
		return fmt.Errorf("load next order_index: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, ?, ?, ?)`,
		characterID, classSlug, level, nextOrder.Int64+1,
	); err != nil {
		return fmt.Errorf("insert multiclass row: %w", err)
	}

	for _, g := range grants {
		kind, value := g.Kind, g.Value
		switch g.Choice {
		case ChoiceClassSkillPool:
			if chosenSkill == "" {
				return ErrMissingChoice
			}
			kind, value = "skill", chosenSkill
		case ChoiceToolkit:
			if chosenToolkit == "" {
				return ErrMissingChoice
			}
			kind, value = "tool", chosenToolkit
		case ChoiceMartialWeapon:
			if chosenWeapon == "" {
				return ErrMissingChoice
			}
			kind, value = "weapon", chosenWeapon
		}
		if _, err := tx.Exec(
			`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
			 VALUES (?, ?, ?, 'class', ?)`,
			characterID, kind, value, classSlug,
		); err != nil {
			return fmt.Errorf("insert multiclass proficiency: %w", err)
		}
	}

	return tx.Commit()
}

// RemoveClass drops classSlug entirely from the character: its
// character_classes row, every character_level_gains row for it, every
// character_proficiencies row tagged source_kind='class' AND
// source_ref=classSlug (both the full first-class grant SetClass writes
// and AddMulticlass's partial grant use this exact tagging, so this cleans
// up either), and any subclass pick among subclassSiblingSlugs — resolved
// by the caller against rules.db first, same convention SetSubclass already
// uses, since this package never queries rules.db.
//
// Remaining classes' order_index is re-sequenced to stay contiguous from 0
// so "the primary class" (order_index 0, which governs the level-1
// max-value HP/Chakra rule in charsheet.Compute) stays well-defined after a
// removal — removing a character's original starting class promotes
// whichever remains to primary, which the book doesn't address for
// "un-multiclassing" but is the only sensible default.
func RemoveClass(charDB *sql.DB, characterID int64, classSlug string, subclassSiblingSlugs []string) error {
	tx, err := charDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var removedOrder int
	if err := tx.QueryRow(
		`SELECT order_index FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, classSlug,
	).Scan(&removedOrder); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("character %d has no class %q", characterID, classSlug)
		}
		return fmt.Errorf("load class order: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM character_classes WHERE character_id = ? AND class_slug = ?`, characterID, classSlug,
	); err != nil {
		return fmt.Errorf("delete class: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE character_classes SET order_index = order_index - 1 WHERE character_id = ? AND order_index > ?`,
		characterID, removedOrder,
	); err != nil {
		return fmt.Errorf("resequence order_index: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM character_level_gains WHERE character_id = ? AND class_slug = ?`, characterID, classSlug,
	); err != nil {
		return fmt.Errorf("delete level gains: %w", err)
	}
	if _, err := tx.Exec(
		`DELETE FROM character_proficiencies WHERE character_id = ? AND source_kind = 'class' AND source_ref = ?`,
		characterID, classSlug,
	); err != nil {
		return fmt.Errorf("delete class proficiencies: %w", err)
	}
	if len(subclassSiblingSlugs) > 0 {
		placeholders := make([]string, len(subclassSiblingSlugs))
		args := make([]any, 0, len(subclassSiblingSlugs)+1)
		args = append(args, characterID)
		for i, slug := range subclassSiblingSlugs {
			placeholders[i] = "?"
			args = append(args, slug)
		}
		query := `DELETE FROM character_subclasses WHERE character_id = ? AND subclass_slug IN (` +
			strings.Join(placeholders, ",") + `)`
		if _, err := tx.Exec(query, args...); err != nil {
			return fmt.Errorf("delete subclass pick: %w", err)
		}
	}
	return tx.Commit()
}
