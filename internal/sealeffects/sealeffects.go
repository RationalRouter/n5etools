// Package sealeffects applies the enhancement seals on a character's
// equipped crafted items (characters.db crafted_items) to the numbers on
// their sheet.
//
// Every seal is classified as either auto or reference. An auto seal has a
// clear-cut, unconditional numeric effect (a flat attack, save, AC or speed
// bonus, a proficiency) that Totals carries into charsheet.Compute. A
// reference seal is conditional (against a creature type, a condition, a
// time of day, once per rest) or has no number the sheet tracks; its text
// is shown on the item page and the sheet leaves the rest to the player.
//
// The book's level rule (p.72): a creature below an item's level gains no
// proficiency with it, and armor grants none of its seal effects. Items
// whose minimum level is above the character's get no auto effects here and
// are flagged Gated.
package sealeffects

import (
	"cmp"
	"database/sql"
	"fmt"
	"strings"

	"github.com/RationalRouter/n5e/internal/crafting"
)

// Effect is what one auto seal adds to its bearer.
type Effect struct {
	// JutsuAttack is keyed by charsheet.AttackKinds' Kind ("Taijutsu").
	JutsuAttack map[string]int
	// Saves and Checks are keyed by ability code ("str"); Checks applies to
	// every skill check of that ability.
	Saves  map[string]int
	Checks map[string]int
	AC     int
	Speed  int
	// ClimbSwim grants climbing and swimming speeds equal to walking speed.
	ClimbSwim bool
	// Skill is a skill proficiency granted, by its SkillAbility name.
	Skill string
	// MeleeRange and RangedRange extend this one weapon's reach and range,
	// in feet; they stay on the Item rather than in Totals.
	MeleeRange, RangedRange int
}

func attack(kind string, n int) Effect { return Effect{JutsuAttack: map[string]int{kind: n}} }

func saves(n int, abilities ...string) Effect {
	e := Effect{Saves: map[string]int{}}
	for _, a := range abilities {
		e.Saves[a] = n
	}
	return e
}

func saveAndChecks(ability string, n int) Effect {
	return Effect{Saves: map[string]int{ability: n}, Checks: map[string]int{ability: n}}
}

// auto is keyed "<applies>/<tier>/<family>", from the seal slug
// seal/<applies>/<tier>/<family>-seal.
var auto = map[string]Effect{
	"weapon/minor/martial":        attack("Taijutsu", 1),
	"weapon/greater/martial":      attack("Taijutsu", 2),
	"weapon/mastercraft/martial":  attack("Taijutsu", 3),
	"weapon/minor/ninshou":        attack("Ninjutsu", 1),
	"weapon/greater/ninshou":      attack("Ninjutsu", 2),
	"weapon/mastercraft/ninshou":  attack("Ninjutsu", 3),
	"weapon/minor/illusory":       attack("Genjutsu", 1),
	"weapon/greater/illusory":     attack("Genjutsu", 2),
	"weapon/mastercraft/illusory": attack("Genjutsu", 3),

	"weapon/refined/fearsome":  {Skill: "Intimidation"},
	"weapon/minor/extending":   {MeleeRange: 5, RangedRange: 20},
	"weapon/greater/extending": {MeleeRange: 10, RangedRange: 30},

	"armor/minor/cold-steel":           saves(1, "dex", "int"),
	"armor/greater/cold-steel":         saves(2, "dex", "int"),
	"armor/mastercraft/cold-steel":     saves(3, "dex", "int"),
	"armor/minor/obsidian-steel":       saves(1, "str", "con"),
	"armor/greater/obsidian-steel":     saves(2, "str", "con"),
	"armor/mastercraft/obsidian-steel": saves(3, "str", "con"),
	"armor/minor/star-metal":           saves(1, "wis", "cha"),
	"armor/greater/star-metal":         saves(2, "wis", "cha"),
	"armor/mastercraft/star-metal":     saves(3, "wis", "cha"),
	"armor/refined/strength":           saveAndChecks("str", 1),
	"armor/superior/strength":          saveAndChecks("str", 3),
	"armor/refined/dexterity":          saveAndChecks("dex", 1),
	"armor/superior/dexterity":         saveAndChecks("dex", 3),

	"armor/greater/elemental":     {AC: 1},
	"armor/mastercraft/elemental": {AC: 2},
	"armor/greater/medical":       {AC: 1},
	"armor/mastercraft/medical":   {AC: 2},

	"armor/refined/swift":    {Speed: 10},
	"armor/superior/swift":   {Speed: 20},
	"armor/refined/mobility": {ClimbSwim: true},
}

// reference lists, per "<applies>/<family>", the seals left as text: every
// tier of the family unless that tier is in auto. Elemental and Medical
// armor seals below Greater carry only damage reduction, which the sheet
// does not track; weapon Elemental/Medical change damage type at will.
var reference = map[string]bool{
	"armor/abyssal": true, "armor/blackened": true, "armor/bloody": true, "armor/bracing": true,
	"armor/bravery": true, "armor/celestial": true, "armor/chaotic": true, "armor/chilling": true,
	"armor/corrosive": true, "armor/crushing": true, "armor/deathless": true, "armor/disguised": true,
	"armor/dispelling": true, "armor/disruptive": true, "armor/elemental": true, "armor/enhanced": true,
	"armor/hot": true, "armor/lunar": true, "armor/medical": true, "armor/occult": true,
	"armor/primal": true, "armor/shocking": true, "armor/sickening": true, "armor/sinister": true,
	"armor/slick": true, "armor/solar": true, "armor/stellar": true, "armor/vanishing": true,

	"weapon/bane": true, "weapon/barrier": true, "weapon/bloody": true, "weapon/chilling": true,
	"weapon/corrosive": true, "weapon/crushing": true, "weapon/disruptive": true, "weapon/elemental": true,
	"weapon/fanged": true, "weapon/frightening": true, "weapon/grievous": true, "weapon/hot": true,
	"weapon/hunter": true, "weapon/laceration": true, "weapon/luminary": true, "weapon/lunar": true,
	"weapon/medical": true, "weapon/occult": true, "weapon/phantom": true, "weapon/primal": true,
	"weapon/shocking": true, "weapon/sickening": true, "weapon/solar": true, "weapon/speed": true,
	"weapon/stellar": true, "weapon/storing": true,
}

// parts splits seal/<applies>/<tier>/<family>-seal.
func parts(slug string) (applies, tier, family string, ok bool) {
	p := strings.Split(slug, "/")
	if len(p) != 4 || p[0] != "seal" || !strings.HasSuffix(p[3], "-seal") {
		return "", "", "", false
	}
	return p[1], p[2], strings.TrimSuffix(p[3], "-seal"), true
}

// Classify reports a seal's auto effect (isAuto), and whether the seal is
// classified at all; an unclassified seal has no effect applied.
func Classify(slug string) (e Effect, isAuto, known bool) {
	applies, tier, family, ok := parts(slug)
	if !ok {
		return Effect{}, false, false
	}
	if e, ok := auto[applies+"/"+tier+"/"+family]; ok {
		return e, true, true
	}
	return Effect{}, false, reference[applies+"/"+family]
}

// Seal is one seal on an equipped crafted item.
type Seal struct {
	Slug string
	Auto bool
}

// Item is one equipped crafted item.
type Item struct {
	Slug     string
	Name     string
	Rank     string
	MinLevel int
	// Gated: the character is below MinLevel, so none of the item's seals
	// apply.
	Gated bool
	Seals []Seal
	// MeleeRange and RangedRange: the Extending Seal's added reach/range,
	// 0 when gated.
	MeleeRange, RangedRange int
}

// Totals is every equipped crafted item's applied seals, summed. Seals of
// one family don't stack across items (a second Martial Seal on another
// weapon adds nothing): each family contributes its best value per number.
type Totals struct {
	Effect
	Items []Item
}

// JutsuAttackBonus returns the seal bonus to one attack kind.
func (t *Totals) JutsuAttackBonus(kind string) int {
	if t == nil {
		return 0
	}
	return t.JutsuAttack[kind]
}

// Item returns the equipped crafted item with this inventory slug, or nil.
func (t *Totals) Item(slug string) *Item {
	if t == nil {
		return nil
	}
	for i := range t.Items {
		if t.Items[i].Slug == slug {
			return &t.Items[i]
		}
	}
	return nil
}

// Load reads a character's equipped crafted items and totals their auto
// seals. level is the character's total level, for the level gate.
func Load(rulesDB, charDB *sql.DB, characterID int64, level int) (*Totals, error) {
	rows, err := charDB.Query(`
		SELECT c.slug, c.name, c.base_slug, COALESCE(s.seal_slug, '')
		FROM character_inventory i
		JOIN crafted_items c ON c.slug = i.item_slug
		LEFT JOIN crafted_item_seals s ON s.crafted_item_id = c.id
		WHERE i.character_id = ? AND i.equipped = 1
		ORDER BY i.id, s.sort_order, s.id`, characterID)
	if err != nil {
		return nil, fmt.Errorf("load equipped crafted items: %w", err)
	}
	type row struct{ slug, name, base, seal string }
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.slug, &r.name, &r.base, &r.seal); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	t := &Totals{Effect: Effect{
		JutsuAttack: map[string]int{}, Saves: map[string]int{}, Checks: map[string]int{},
	}}
	best := map[string]Effect{} // family → best per number across items
	seen := map[string]int{}    // item slug → index in t.Items
	baseKind := map[string]string{}
	for _, r := range all {
		i, ok := seen[r.slug]
		if !ok {
			kind, err := equipmentKind(rulesDB, r.base)
			if err != nil {
				return nil, err
			}
			baseKind[r.slug] = kind
			t.Items = append(t.Items, Item{Slug: r.slug, Name: r.name})
			i = len(t.Items) - 1
			seen[r.slug] = i
		}
		if r.seal == "" {
			continue
		}
		applies, _, family, ok := parts(r.seal)
		if !ok || applies != baseKind[r.slug] {
			continue
		}
		var rank sql.NullString
		if err := rulesDB.QueryRow(`SELECT seal_rank FROM equipment WHERE slug = ?`, r.seal).Scan(&rank); err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		it := &t.Items[i]
		if rank.String != "" && (it.Rank == "" || strings.Index("DCBAS", rank.String) > strings.Index("DCBAS", it.Rank)) {
			it.Rank = rank.String
		}
		e, isAuto, _ := Classify(r.seal)
		it.Seals = append(it.Seals, Seal{Slug: r.seal, Auto: isAuto})
		if isAuto {
			it.MeleeRange += e.MeleeRange
			it.RangedRange += e.RangedRange
			best[family+"\x00"+r.slug] = e
		}
	}

	// A character with no class levels yet (mid-creation) counts as 1st
	// level, which every D-Rank item allows.
	level = max(level, 1)
	gated := map[string]bool{}
	for i := range t.Items {
		it := &t.Items[i]
		it.MinLevel = 1
		if rule, ok := crafting.RankRuleFor(it.Rank); ok {
			it.MinLevel = rule.MinLevel
		}
		it.Gated = level < it.MinLevel
		if it.Gated {
			it.MeleeRange, it.RangedRange = 0, 0
		}
		gated[it.Slug] = it.Gated
	}
	families := map[string]Effect{}
	for k, e := range best {
		family, item, _ := strings.Cut(k, "\x00")
		if gated[item] {
			continue
		}
		families[family] = maxEffect(families[family], e)
	}
	for _, e := range families {
		t.add(e)
	}
	return t, nil
}

func equipmentKind(rulesDB *sql.DB, slug string) (string, error) {
	var kind sql.NullString
	if err := rulesDB.QueryRow(`SELECT kind FROM equipment WHERE slug = ?`, slug).Scan(&kind); err != nil && err != sql.ErrNoRows {
		return "", err
	}
	return kind.String, nil
}

func maxMap(a, b map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = max(out[k], v)
	}
	return out
}

func maxEffect(a, b Effect) Effect {
	return Effect{
		JutsuAttack: maxMap(a.JutsuAttack, b.JutsuAttack),
		Saves:       maxMap(a.Saves, b.Saves),
		Checks:      maxMap(a.Checks, b.Checks),
		AC:          max(a.AC, b.AC),
		Speed:       max(a.Speed, b.Speed),
		ClimbSwim:   a.ClimbSwim || b.ClimbSwim,
		Skill:       cmp.Or(a.Skill, b.Skill),
	}
}

func (t *Totals) add(e Effect) {
	for k, v := range e.JutsuAttack {
		t.JutsuAttack[k] += v
	}
	for k, v := range e.Saves {
		t.Saves[k] += v
	}
	for k, v := range e.Checks {
		t.Checks[k] += v
	}
	t.AC += e.AC
	t.Speed += e.Speed
	t.ClimbSwim = t.ClimbSwim || e.ClimbSwim
	t.Skill = cmp.Or(t.Skill, e.Skill)
}
