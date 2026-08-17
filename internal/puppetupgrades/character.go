package puppetupgrades

import "database/sql"

// ClassSlug is the Puppet Master class's own slug in rules.db — the class
// whose level every formula in this package is scaled by ("for each level
// you have in this class").
const ClassSlug = "class/puppet-master"

// SubclassColorBySlug maps each Puppet Technique subclass to the short
// color name its own upgrade text uses in a "Techniques: X, Y" line (e.g.
// "Techniques: Black, Blue, Green, Red, White" on Undying Form, naming
// every technique except Purple) — hand-curated against the real six
// subclass names rather than derived from the slug, since deriving "Purple"
// out of ".../purple-technique-juggernaut" by string-splitting is exactly
// as much code as just listing six known values, and listing them is
// self-documenting against a rules change (a 7th subclass, a renamed one)
// instead of silently mis-deriving.
var SubclassColorBySlug = map[string]string{
	"class/puppet-master/group/puppet-techniques/black-technique-puppeteer":    "Black",
	"class/puppet-master/group/puppet-techniques/blue-technique-warmaster":     "Blue",
	"class/puppet-master/group/puppet-techniques/green-technique-marionettist": "Green",
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut":  "Purple",
	"class/puppet-master/group/puppet-techniques/red-technique-performer":      "Red",
	"class/puppet-master/group/puppet-techniques/white-technique-weaver":       "White",
}

// CharacterTotals is what a character's own sheet gains from their Puppet
// Upgrades — the bonuses that land on the Puppet Master rather than on a
// companion (see CharacterBonuses).
type CharacterTotals struct {
	Speed int
	MaxHP int
	// Sources names the upgrades that contributed, so the sheet can say
	// where a raised number came from instead of showing an unexplained
	// total.
	Sources []string
}

// LoadCharacterTotals resolves the character-side upgrade bonuses for one
// character. puppetMasterLevel is their level in the Puppet Master class
// (0 for anyone who has none), and a zero level short-circuits the whole
// thing — every query below would come back empty for a character with no
// Puppet Master levels, and the overwhelming majority of characters have
// none, so the cheap check is worth making before touching the database at
// all on every sheet render.
func LoadCharacterTotals(charDB *sql.DB, characterID int64, puppetMasterLevel int) (CharacterTotals, error) {
	var totals CharacterTotals
	if puppetMasterLevel <= 0 {
		return totals, nil
	}

	color, err := subclassColor(charDB, characterID)
	if err != nil {
		return totals, err
	}

	picks, err := entryPickCounts(charDB, characterID)
	if err != nil {
		return totals, err
	}
	for _, b := range CharacterBonuses {
		if picks[b.EntrySlug] == 0 || !b.AppliesTo(color) {
			continue
		}
		contributed := false
		if b.Speed != nil {
			if v := b.Speed(puppetMasterLevel); v != 0 {
				totals.Speed += v
				contributed = true
			}
		}
		if b.MaxHP != nil {
			if v := b.MaxHP(puppetMasterLevel); v != 0 {
				totals.MaxHP += v
				contributed = true
			}
		}
		if contributed {
			totals.Sources = append(totals.Sources, b.Name)
		}
	}
	return totals, nil
}

// subclassColor returns the character's Puppet Technique color, or "" if
// they have not chosen one. A character can hold subclasses from several
// classes at once (multiclassing), so the rows are filtered through the
// known Puppet Technique slugs rather than assuming the only subclass row
// is the relevant one.
func subclassColor(charDB *sql.DB, characterID int64) (string, error) {
	rows, err := charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return "", err
		}
		if color, ok := SubclassColorBySlug[slug]; ok {
			return color, nil
		}
	}
	return "", rows.Err()
}

// entryPickCounts counts every upgrade pick across all of a character's
// companions, keyed by upgrade entry slug. Entry slug rather than
// class_option_slug so a pick taken from a higher tier's slot (the
// tier-escalation mechanic) still counts as the same upgrade.
func entryPickCounts(charDB *sql.DB, characterID int64) (map[string]int, error) {
	rows, err := charDB.Query(`
		SELECT u.upgrade_entry_slug
		FROM character_companion_upgrades u
		JOIN character_companions c ON c.id = u.companion_id
		WHERE c.character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		counts[slug]++
	}
	return counts, rows.Err()
}
