// Package features resolves a character's granted class, subclass, and clan
// features against rules.db, and — this is the reason it exists as its own
// package rather than staying inside cmd/n5e where it used to live — turns a
// curated subset of those features into real numeric effects (proficiency,
// AC, Speed, Mastery) that internal/charsheet.Compute folds into the sheet
// it returns.
//
// cmd/n5e still owns display-only uses of the granted-feature list (the
// Features & Traits panel, the Resistances & Senses panel, free jutsu
// grants) — those keep working exactly as before, just importing the type
// from here instead of defining it locally. What's new is that
// internal/charsheet can now import this package too, so a feature grant
// changes the actual computed Skills/Saves/AC/Speed, not just a side panel.
//
// Feats are deliberately out of scope here (see internal/schema's
// character_proficiencies.source_kind, which reserves 'feat' for later) —
// this package resolves class/subclass/clan features only, matching what
// was actually asked for and audited.
package features

import (
	"database/sql"
	"strconv"
)

// GrantedFeatureRow is one auto-seeded, real class/subclass/clan feature a
// character has — the shape cmd/n5e's Core tab shows alongside custom
// features, and the shape the curated grant tables below are keyed against.
type GrantedFeatureRow struct {
	// Slug identifies the rules-database row (class_features/
	// subclass_features/clan_features) this came from — the join key the
	// curated tables below (and cmd/n5e's passiveTraitGrants) key off of.
	Slug        string
	Name        string
	SourceLabel string
	Description string
	// Level is the level the feature was gained at, 0 for one that is
	// always on (level IS NULL). Only exists to sort/gate a merged list —
	// SourceLabel already says the level in the reader's own words.
	Level int
}

// LoadGrantedFeatures assembles a character's real class + subclass + clan
// features out of rules.db, gated by the level each was actually gained at
// — one query per character_classes row (multiclassing means more than one
// class can each unlock its own features up to its own levels), one per
// chosen subclass (gated by its parent class's own level, resolved through
// subclass_groups.class_slug since character_subclasses only stores the
// subclass slug), plus one for the clan gated by the character's total
// class level (classLevel — the caller's Sheet.Level, so there is no
// display-only level to disagree with it).
func LoadGrantedFeatures(rulesDB, charDB *sql.DB, characterID int64, clanSlug string, classLevel int) ([]GrantedFeatureRow, error) {
	var out []GrantedFeatureRow

	classRows, err := charDB.Query(
		`SELECT class_slug, levels FROM character_classes WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	type classLevelPair struct {
		Slug   string
		Levels int
	}
	var classes []classLevelPair
	for classRows.Next() {
		var c classLevelPair
		if err := classRows.Scan(&c.Slug, &c.Levels); err != nil {
			classRows.Close()
			return nil, err
		}
		classes = append(classes, c)
	}
	classRows.Close()
	if err := classRows.Err(); err != nil {
		return nil, err
	}

	classLevels := make(map[string]int, len(classes))
	for _, c := range classes {
		classLevels[c.Slug] = c.Levels
	}

	for _, c := range classes {
		var className string
		if err := rulesDB.QueryRow(`SELECT name FROM classes WHERE slug = ?`, c.Slug).Scan(&className); err != nil {
			className = c.Slug
		}
		rows, err := rulesDB.Query(`
			SELECT slug, name, level, description FROM v_class_features
			WHERE class_slug = ? AND (level IS NULL OR level <= ?)
			ORDER BY sort_order, level`, c.Slug, c.Levels)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var slug, name, description string
			var level sql.NullInt64
			if err := rows.Scan(&slug, &name, &level, &description); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, GrantedFeatureRow{
				Slug: slug, Name: name, Description: description,
				SourceLabel: FeatureSourceLabel("Class: "+className, level),
				Level:       int(level.Int64),
			})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	subRows, err := charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	var subclassSlugs []string
	for subRows.Next() {
		var slug string
		if err := subRows.Scan(&slug); err != nil {
			subRows.Close()
			return nil, err
		}
		subclassSlugs = append(subclassSlugs, slug)
	}
	subRows.Close()
	if err := subRows.Err(); err != nil {
		return nil, err
	}

	for _, slug := range subclassSlugs {
		var subclassName, classSlug string
		if err := rulesDB.QueryRow(`
			SELECT sc.name, g.class_slug FROM subclasses sc
			JOIN subclass_groups g ON g.slug = sc.group_slug
			WHERE sc.slug = ?`, slug,
		).Scan(&subclassName, &classSlug); err != nil {
			continue // a stale/removed subclass slug just contributes no features
		}
		gateLevel, ok := classLevels[classSlug]
		if !ok {
			continue // subclass chosen for a class the character no longer has
		}
		rows, err := rulesDB.Query(`
			SELECT slug, name, level, description FROM v_subclass_features
			WHERE subclass_slug = ? AND (level IS NULL OR level <= ?)
			ORDER BY sort_order, level`, slug, gateLevel)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var featureSlug, name, description string
			var level sql.NullInt64
			if err := rows.Scan(&featureSlug, &name, &level, &description); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, GrantedFeatureRow{
				Slug: featureSlug, Name: name, Description: description,
				SourceLabel: FeatureSourceLabel("Subclass: "+subclassName, level),
				Level:       int(level.Int64),
			})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	if clanSlug != "" {
		var clanName string
		if err := rulesDB.QueryRow(`SELECT name FROM clans WHERE slug = ?`, clanSlug).Scan(&clanName); err != nil {
			clanName = clanSlug
		}
		rows, err := rulesDB.Query(`
			SELECT slug, name, level, description FROM v_clan_features
			WHERE clan_slug = ? AND (level IS NULL OR level <= ?)
			ORDER BY sort_order, level`, clanSlug, classLevel)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var slug, name, description string
			var level sql.NullInt64
			if err := rows.Scan(&slug, &name, &level, &description); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, GrantedFeatureRow{
				Slug: slug, Name: name, Description: description,
				SourceLabel: FeatureSourceLabel("Racial: "+clanName, level),
				Level:       int(level.Int64),
			})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	return out, nil
}

// FeatureSourceLabel matches the reference screenshots' "Racial: Vedalken/
// 1st Level" style, omitting the level clause for an always-on feature
// (level IS NULL). Exported: cmd/n5e's mergeFeatFeatures reuses it for the
// same "Category/Nth Level" label on a taken feat.
func FeatureSourceLabel(prefix string, level sql.NullInt64) string {
	if !level.Valid {
		return prefix
	}
	return prefix + "/" + ordinal(level.Int64) + " Level"
}

func ordinal(n int64) string {
	switch {
	case n%100 >= 11 && n%100 <= 13:
		return strconv.FormatInt(n, 10) + "th"
	case n%10 == 1:
		return strconv.FormatInt(n, 10) + "st"
	case n%10 == 2:
		return strconv.FormatInt(n, 10) + "nd"
	case n%10 == 3:
		return strconv.FormatInt(n, 10) + "rd"
	default:
		return strconv.FormatInt(n, 10) + "th"
	}
}
