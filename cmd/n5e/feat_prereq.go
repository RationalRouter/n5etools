package main

import (
	"database/sql"
	"regexp"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

// Feat prerequisites are stored as the book's own prose — `feats.prerequisites`
// is a single string like "Hoshigaki Clan, Level 8+" or "Level 5+, Strength or
// Dexterity 15+, You cannot have class levels in Weapon Specialist". There is
// no structured form of them anywhere in rules.db, so deciding whether a
// character can take a feat means reading that sentence.
//
// The parser below is deliberately one-sided: a clause it does not recognise
// is dropped, never treated as unmet. Hiding a feat the player is entitled to
// is a bug they cannot see or work around, while leaving one extra row in the
// list costs a moment's reading — and the source prose is genuinely ragged
// (several rows in the PDF are truncated mid-sentence: "Level 5+, Any two of
// the following three ability scores must be 14+ (Strength, Dexterity,").
// Under this rule those still get their "Level 5+" enforced and the dangling
// remainder ignored, which is the most that can honestly be read out of them.

// featPrereq is one feat's prerequisites, reduced to the parts that can be
// checked against a character. Every field is optional; a feat with no
// recognised clauses parses to a zero value that every character satisfies.
type featPrereq struct {
	MinLevel int // "Level 8+" — 0 for none

	// Clan is the clan whose name the prose names ("Hyūga Clan"), lowercased.
	Clan string

	// ClassLevels is "At least 4+ levels in Cooking-Nin": the class name
	// lowercased, and the levels required in it.
	ClassName  string
	ClassLevel int

	// ForbiddenClass is "You cannot have class levels in Hunter-Nin".
	ForbiddenClass string

	// Abilities is an any-of group: "Strength or Dexterity 15+" is satisfied
	// by either one reaching 15. A plain "Charisma 15+" is a group of one.
	Abilities []abilityRequirement

	// Skills is an any-of group of skill names that must be proficient
	// ("Proficiency in either Ninshou or Martial Arts").
	Skills []string

	// Feats are other feats that must already be taken, by lowercased name.
	Feats []string
}

type abilityRequirement struct {
	Ability string // lowercased full name: "strength"
	Min     int
}

// featAbilityKeys maps the full ability names the book's prose uses onto the
// three-letter keys charsheet.Sheet.Abilities is keyed by.
var featAbilityKeys = map[string]string{
	"strength": "str", "dexterity": "dex", "constitution": "con",
	"intelligence": "int", "wisdom": "wis", "charisma": "cha",
}

var (
	// "Level 8+", "Level 10.", "level 4+" — the trailing punctuation varies
	// row to row in the source.
	reLevel = regexp.MustCompile(`(?i)^level\s+(\d+)\s*\+?\.?$`)
	// "At least 4+ levels in Cooking-Nin.", "At least 10+ Levels in the
	// Puppet Master class"
	reClassLevels = regexp.MustCompile(`(?i)^at least\s+(\d+)\s*\+?\s+levels?\s+in\s+(?:the\s+)?(.+?)(?:\s+class)?\.?$`)
	// "You cannot have class levels in Hunter-Nin", "You cannot have Levels
	// in the Cooking-Nin Class."
	reForbiddenClass = regexp.MustCompile(`(?i)^you cannot have\s+(?:class\s+)?levels?\s+in\s+(?:the\s+)?(.+?)(?:\s+class)?\.?$`)
	// "Strength 15+", "Charisma 20", "Intelligence 15 +"
	reAbility = regexp.MustCompile(`(?i)^(strength|dexterity|constitution|intelligence|wisdom|charisma)\s*(\d+)?\s*\+?\.?$`)
	// "Proficiency in Chakra Control", "Proficiency in Medicine & Chakra
	// Control."
	reProficiency = regexp.MustCompile(`(?i)^proficiency in\s+(.+?)\.?$`)
	// "Aburame Clan", "Hyūga Clan"
	reClan = regexp.MustCompile(`(?i)^(.+?)\s+clan\.?$`)
)

// parseFeatPrereq reads one feats.prerequisites string. knownSkills,
// knownFeats, knownClasses, and knownClans are the vocabularies the rest of
// the app recognises, all lowercased — they're what keeps a truncated
// clause from being read as a requirement nothing can satisfy, and what
// lets a bare name ("Medical-Nin, Level 8+", "Akimichi") be recognised as a
// real class/clan requirement even though it doesn't match either regex
// shape below (reClassLevels needs "at least N+ levels in ...", reClan
// needs a trailing "... Clan" — several rows in the book state the
// requirement as just the bare name instead, meaning "you must belong to
// this class/clan", full stop, no numeric threshold of its own beyond
// whatever separate "Level N+" clause sits alongside it).
func parseFeatPrereq(text string, knownSkills, knownFeats, knownClasses, knownClans map[string]bool) featPrereq {
	var p featPrereq
	for _, clause := range splitPrereqClauses(text) {
		switch {
		case reLevel.MatchString(clause):
			if n, err := strconv.Atoi(reLevel.FindStringSubmatch(clause)[1]); err == nil && n > p.MinLevel {
				p.MinLevel = n
			}

		case reClassLevels.MatchString(clause):
			m := reClassLevels.FindStringSubmatch(clause)
			if n, err := strconv.Atoi(m[1]); err == nil {
				p.ClassLevel, p.ClassName = n, strings.ToLower(strings.TrimSpace(m[2]))
			}

		case reForbiddenClass.MatchString(clause):
			p.ForbiddenClass = strings.ToLower(strings.TrimSpace(reForbiddenClass.FindStringSubmatch(clause)[1]))

		case reProficiency.MatchString(clause):
			// "either Ninshou or Martial" — the second half of that one is
			// cut off in the source, so anything that isn't a skill name is
			// dropped and the clause only applies if something survived.
			var skills []string
			for _, name := range splitAnyOf(reProficiency.FindStringSubmatch(clause)[1]) {
				if knownSkills[name] {
					skills = append(skills, name)
				}
			}
			p.Skills = append(p.Skills, skills...)

		case reClan.MatchString(clause):
			p.Clan = strings.ToLower(strings.TrimSpace(reClan.FindStringSubmatch(clause)[1]))

		default:
			bare := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(clause), "."))
			// A bare class name ("Medical-Nin", "Taijutsu Specialist") means
			// "you must have at least 1 level in this class" — no explicit
			// number is given, so ClassLevel defaults to 1 rather than
			// whatever a co-occurring "At least N+ levels in ..." clause
			// for a DIFFERENT class might otherwise have left behind (no
			// feat in the book currently states two separate class
			// requirements at once, so this can't silently clobber one).
			if knownClasses[bare] {
				p.ClassName, p.ClassLevel = bare, 1
				continue
			}
			// A bare clan name ("Akimichi" — most rows in the book add a
			// trailing "Clan" that reClan above already handles; this one
			// row doesn't) means the same thing reClan's own match means.
			if knownClans[bare] {
				p.Clan = bare
				continue
			}
			// Otherwise: an ability group ("Strength or Dexterity 15+") or
			// a named feat ("Medical Expert", "Chef Feat"). Both are
			// any-of over " or ", and both fall through to being ignored
			// when nothing in them is recognised.
			if abilities := parseAbilityGroup(clause); len(abilities) > 0 {
				p.Abilities = append(p.Abilities, abilities...)
				continue
			}
			name := strings.TrimSuffix(bare, " feat")
			if knownFeats[name] {
				p.Feats = append(p.Feats, name)
			}
		}
	}
	return p
}

// knownClassNames/knownClanNames return every class/clan's own name,
// lowercased (and, for clans, with the book's usual trailing "Clan"
// stripped so it matches featCharacter.Clan's own convention) — the
// vocabulary a bare class/clan-name prerequisite clause is checked
// against, the same role knownSkills/knownFeats already play for their own
// clause shapes.
func (s *server) knownClassNames() (map[string]bool, error) {
	rows, err := s.rulesDB.Query(`SELECT name FROM classes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[strings.ToLower(name)] = true
	}
	return out, rows.Err()
}

func (s *server) knownClanNames() (map[string]bool, error) {
	rows, err := s.rulesDB.Query(`SELECT name FROM v_clans`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		lower := strings.ToLower(strings.TrimSpace(name))
		out[strings.TrimSuffix(lower, " clan")] = true
	}
	return out, rows.Err()
}

// splitPrereqClauses cuts the prose on commas and semicolons. Parentheses are
// left alone: the one shape that uses them ("… must be 14+ (Strength,
// Dexterity, Constitution)") is an unparseable clause either way, and
// splitting inside it would turn one ignored clause into three.
func splitPrereqClauses(text string) []string {
	var out []string
	depth := 0
	start := 0
	for i, r := range text {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',', ';':
			if depth == 0 {
				out = append(out, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(text[start:]))

	kept := out[:0]
	for _, clause := range out {
		if clause != "" && clause != "." {
			kept = append(kept, clause)
		}
	}
	return kept
}

// splitAnyOf breaks "either Ninshou or Martial Arts" and "Medicine & Chakra
// Control" into their parts, lowercased.
func splitAnyOf(text string) []string {
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`(?i)^either\s+`).ReplaceAllString(text, "")
	parts := regexp.MustCompile(`(?i)\s+or\s+|\s*&\s*|\s+and\s+`).Split(text, -1)
	var out []string
	for _, part := range parts {
		if part = strings.ToLower(strings.TrimSpace(part)); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parseAbilityGroup reads "Strength or Dexterity 15+" and "Intelligence 14+ or
// Wisdom 14+". A part with no number of its own borrows the next one that
// appears — the book writes the threshold once, at the end, for the first
// shape.
func parseAbilityGroup(clause string) []abilityRequirement {
	parts := regexp.MustCompile(`(?i)\s+or\s+`).Split(clause, -1)
	var out []abilityRequirement
	for _, part := range parts {
		m := reAbility.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil // not an ability clause at all; leave it to the caller
		}
		req := abilityRequirement{Ability: strings.ToLower(m[1])}
		if m[2] != "" {
			req.Min, _ = strconv.Atoi(m[2])
		}
		out = append(out, req)
	}
	// Backfill "Strength" from the "Dexterity 15+" beside it.
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Min == 0 && i+1 < len(out) {
			out[i].Min = out[i+1].Min
		}
	}
	return out
}

// featCharacter is everything about a character that a prerequisite can ask
// about, in the lowercased form the parser produces.
type featCharacter struct {
	Level       int
	Clan        string          // clan name, lowercased; "" when clanless
	ClassLevels map[string]int  // class name (lowercased) -> levels in it
	Abilities   map[string]int  // charsheet's three-letter keys: "str" -> score
	Skills      map[string]bool // lowercased skill names the character is proficient in
	Feats       map[string]bool // feat names already taken, lowercased

	// AllSkills is every skill name the sheet knows about, lowercased —
	// the vocabulary a "Proficiency in …" clause is checked against, so a
	// clause naming something that isn't a skill at all (the truncated
	// "Proficiency in either Ninshou or Martial") is dropped rather than
	// read as a requirement no character could meet.
	AllSkills map[string]bool
}

// buildFeatCharacter gathers what a prerequisite can ask about into the
// lowercased shape the parser produces: the character's level, clan name,
// levels per class by NAME (the prose names classes, not slugs), ability
// scores, skill proficiencies and the feats they already have.
func (s *server) buildFeatCharacter(characterID int64, sheet *charsheet.Sheet, feats []characterFeat) (featCharacter, error) {
	c := featCharacter{
		Level:       sheet.Level,
		ClassLevels: map[string]int{},
		Abilities:   map[string]int{},
		Skills:      map[string]bool{},
		AllSkills:   map[string]bool{},
		Feats:       map[string]bool{},
	}
	for ability, score := range sheet.Abilities {
		c.Abilities[ability] = score.Score
	}
	for _, skill := range sheet.Skills {
		name := strings.ToLower(skill.Name)
		c.AllSkills[name] = true
		if skill.Proficient {
			c.Skills[name] = true
		}
	}
	for _, feat := range feats {
		c.Feats[strings.ToLower(feat.Name)] = true
	}

	if sheet.ClanSlug != "" {
		var name string
		err := s.rulesDB.QueryRow(`SELECT name FROM v_clans WHERE slug = ?`, sheet.ClanSlug).Scan(&name)
		if err != nil && err != sql.ErrNoRows {
			return c, err
		}
		// The prose writes "Hyūga Clan" and the data stores "Hyūga", so the
		// suffix is trimmed on both sides rather than assumed on either.
		c.Clan = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), " Clan"))
	}

	rows, err := s.charDB.Query(
		`SELECT class_slug, levels FROM character_classes WHERE character_id = ?`, characterID)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var levels int
		if err := rows.Scan(&slug, &levels); err != nil {
			return c, err
		}
		var name string
		if err := s.rulesDB.QueryRow(`SELECT name FROM classes WHERE slug = ?`, slug).Scan(&name); err != nil {
			if err != sql.ErrNoRows {
				return c, err
			}
			name = slug // a class rules.db no longer has still counts as levels taken
		}
		c.ClassLevels[strings.ToLower(strings.TrimSpace(name))] += levels
	}
	return c, rows.Err()
}

// Meets reports whether the character satisfies every clause the parser
// understood. Unrecognised prose never reaches here, so this is only ever
// asked about requirements that really were readable.
func (c featCharacter) Meets(p featPrereq) bool {
	if p.MinLevel > 0 && c.Level < p.MinLevel {
		return false
	}
	if p.Clan != "" && c.Clan != p.Clan {
		return false
	}
	if p.ClassName != "" && c.ClassLevels[p.ClassName] < p.ClassLevel {
		return false
	}
	if p.ForbiddenClass != "" && c.ClassLevels[p.ForbiddenClass] > 0 {
		return false
	}
	if len(p.Abilities) > 0 {
		met := false
		for _, req := range p.Abilities {
			if c.Abilities[featAbilityKeys[req.Ability]] >= req.Min {
				met = true
				break
			}
		}
		if !met {
			return false
		}
	}
	if len(p.Skills) > 0 {
		met := false
		for _, skill := range p.Skills {
			if c.Skills[skill] {
				met = true
				break
			}
		}
		if !met {
			return false
		}
	}
	for _, feat := range p.Feats {
		if !c.Feats[feat] {
			return false
		}
	}
	return true
}
