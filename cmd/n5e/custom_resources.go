package main

import (
	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// customResourceGrants is the curated table backing the sheet's "Special
// Resources" box — the second spendable point pool several classes/clans
// layer on top of the shared HP/Chakra pair (Science-Nin's Chakra
// Containment Device, Hatake's White Chakra, Hoshi's Star Chakra, and a
// handful more found while sweeping the book for this feature: Akimichi's
// Calories, Uzumaki's Reserve Cells, Scout-Nin's Chakra Barrier, and
// Hatake's feat-derived Purple Lightning, plus Hunter-Nin's Hunters
// Exploits "uses" pool below — the only entry that needs Proficiency
// Bonus in its own Max, which is why that parameter exists at all).
//
// Same "curated table keyed by rules-database feature slug" shape
// passiveTraitGrants (passive_traits.go) already established, for the same
// reason: a blind text scan over "pool"/"chakra"/"per rest" language would
// false-positive constantly (Martial Dice, Bloodline Points, and Chakra
// Barrier's own per-turn regen all use similar words but don't fit this
// tracker's rest-tier shape at all) — every entry below was read against
// the real book text instead.
//
// Deliberately NOT modeled here (see plan doc / memory for the full case
// for each): bloodline-latent variants of these same resources (no
// character-side "I took this latent" selection exists anywhere in the app
// yet), Martial Dice (resets every turn, not on a rest), Bloodline Points
// (a one-time allocation, not a spendable pool), CCD's Mending/Maiming
// split (one narrow subclass option), and Chakra Barrier's per-turn regen
// (a manual click on the resource's own edit box, same boundary
// Concentration Check's DC entry already uses).
type restRegen int

const (
	regenNone      restRegen = iota // no automatic gain; player adjusts the resource box by hand
	regenConMod                     // gains CON modifier (min 1), capped at max — Star Chakra, Calories
	regenHalfSpent                  // regains half of whatever was spent (floor), capped at max — White Chakra
	regenHalfMax                    // resets to half of Max outright — CCD's "charges to half-full"
	regenFull                       // resets straight to max
)

// customResourceGrant is one hand-verified custom resource a class
// feature, subclass feature, clan feature, or feat grants — keyed by the
// rules-database slug of the feature that grants it, exactly like
// passiveTraitGrant.
type customResourceGrant struct {
	// Key groups multiple granting slugs into one resource (e.g. the base
	// Hatake clan feature and the White Chakra Surge feat both feed
	// "white_chakra") and is the column stored in
	// character_custom_resources.resource_key.
	Key  string
	Name string
	// MinLevel is an independent character-level gate beyond the
	// feature's own row-level, same purpose as passiveTraitGrant.MinLevel.
	MinLevel int
	// Max computes the resource's maximum from the character's per-class
	// level map (keyed by class slug, e.g. "class/science-nin" — CCD is
	// the one entry that needs one specific class's level rather than the
	// total, which is why this takes the whole map instead of a single
	// int), CON modifier, and Proficiency Bonus (Hunters Exploits' own
	// "uses" pool — see class/hunter-nin/feature/hunters-exploits — is the
	// one entry that needs it; every other grant ignores the parameter).
	// When more than one grant shares a Key, the higher computed Max wins
	// (White Chakra Surge stacks onto the base Hatake grant this way).
	Max func(classLevels map[string]int, conMod, profBonus int) int
	// ShortRegen/LongRegen/FullRegen: how much the resource recovers on
	// each rest tier. FullRegen is always >= LongRegen (Full Rest is a
	// superset of Long Rest, confirmed against the book).
	ShortRegen  restRegen
	LongRegen   restRegen
	FullRegen   restRegen
	Restriction string // informational only, shown on the sheet
}

func characterLevel(classLevels map[string]int) int {
	total := 0
	for _, v := range classLevels {
		total += v
	}
	return total
}

var customResourceGrants = map[string]customResourceGrant{
	"clan/hatake/feature/white-chakra": {
		Key:  "white_chakra",
		Name: "White Chakra",
		Max: func(cl map[string]int, con, prof int) int {
			return 5 + characterLevel(cl)
		},
		ShortRegen:  regenHalfSpent,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Lightning Release jutsu only",
	},
	// Raises the White Chakra max further (+character level, +1/level
	// after) — additive onto the same "white_chakra" key via the
	// higher-Max-wins combination rule.
	"feat/hatake/white-chakra-surge": {
		Key:  "white_chakra",
		Name: "White Chakra",
		Max: func(cl map[string]int, con, prof int) int {
			return 5 + 2*characterLevel(cl)
		},
		ShortRegen:  regenHalfSpent,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Lightning Release jutsu only",
	},
	// A derived pool, converted from White Chakra by hand at the end of
	// any rest (2 White Chakra -> 1 Purple Lightning, up to 10/rest) — no
	// automatic regen of its own; the player moves points into it via the
	// resource's own edit box.
	"feat/hatake/purple-lightning": {
		Key:  "purple_lightning",
		Name: "Purple Lightning",
		Max: func(map[string]int, int, int) int {
			return 10
		},
		ShortRegen: regenNone,
		LongRegen:  regenNone,
		FullRegen:  regenNone,
	},
	"clan/hoshi/feature/star-chakra": {
		Key:  "star_chakra",
		Name: "Star Chakra",
		Max: func(cl map[string]int, con, prof int) int {
			if con < 1 {
				con = 1
			}
			return con + characterLevel(cl)
		},
		ShortRegen: regenConMod,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"clan/akimichi/feature/calories": {
		Key:  "calories",
		Name: "Calories",
		Max: func(cl map[string]int, con, prof int) int {
			if con < 1 {
				con = 1
			}
			return characterLevel(cl) + con
		},
		ShortRegen: regenConMod,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"clan/uzumaki/feature/chakra-reserves": {
		Key:  "reserve_cells",
		Name: "Reserve Cells",
		Max: func(cl map[string]int, con, prof int) int {
			return characterLevel(cl)
		},
		// The book only ever says "per long rest" — no short rest gain.
		ShortRegen: regenNone,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/scout-nin/group/scouting-technique/barrier-scout/feature/chakra-barrier": {
		Key:      "chakra_barrier",
		Name:     "Chakra Barrier",
		MinLevel: 3,
		Max: func(cl map[string]int, con, prof int) int {
			return 2 * characterLevel(cl)
		},
		// "Whenever you complete a rest of any type" — recreated fresh on
		// every tier, including Short Rest.
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	"class/science-nin/feature/chakra-containment-device": {
		Key:      "ccd",
		Name:     "Chakra Containment Device",
		MinLevel: 2,
		Max: func(cl map[string]int, con, prof int) int {
			return cl["class/science-nin"] * 15
		},
		// No automatic Short Rest gain in the book text at all — sealing
		// chakra into the CCD on a short rest is a player choice tied to
		// a Chakra Dice roll (same "player rolls, types the result in"
		// shape the main HP/Chakra short rest already uses), covered by
		// the resource's own manual edit box rather than an automatic
		// regen here.
		ShortRegen:  regenNone,
		LongRegen:   regenHalfMax,
		FullRegen:   regenFull,
		Restriction: "Powers Scientific Ninja Tools and Science-Nin features",
	},
	// "You can use these features a number of times equal to your
	// Proficiency Bonus per Short Rest" — the spend pool behind whichever
	// Hunters Exploits the character knows (the known-list itself, cap
	// 2->3@10th->4@17th, is a separate cap+catalog picker in
	// cmd/n5e/hunter_nin.go, the same "pool here, known-list there" split
	// Martial Dice/Known Martial Techniques already established).
	"class/hunter-nin/feature/hunters-exploits": {
		Key:  "hunter_exploits",
		Name: "Hunters Exploits",
		Max: func(cl map[string]int, con, prof int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
	// "This Scroll can be used once per long rest." — a single-charge
	// resource, not a pool the Puppet Swarm's own AC/HP/Commands (all
	// computed live in puppetSwarmStats, not tracked here) has any part in.
	"class/puppet-master/group/puppet-techniques/red-technique-performer/feature/performance-of-10-puppets": {
		Key:      "performance_scroll",
		Name:     "Performance Scroll",
		MinLevel: 10,
		Max: func(cl map[string]int, con, prof int) int {
			return 1
		},
		ShortRegen:  regenNone,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Summons the Puppet Swarm",
	},
	// "Once per rest, when a creature is affected by a Genjutsu that you
	// cast, you can disrupt their chakra..." Gains an additional use of
	// either of the feature's two effects at 7th and 14th level (1 -> 2 ->
	// 3 total). The book never names a rest tier ("per rest" alone) — same
	// unscoped phrasing Chakra Barrier's own "whenever you complete a rest
	// of any type" already resolves by regenerating fully on every tier,
	// applied identically here rather than guessing at one tier.
	"class/genjutsu-specialist/feature/chakra-disruption": {
		Key:  "chakra_disruption",
		Name: "Chakra Disruption",
		Max: func(cl map[string]int, con, prof int) int {
			n := 1
			gl := cl["class/genjutsu-specialist"]
			if gl >= 7 {
				n++
			}
			if gl >= 14 {
				n++
			}
			return n
		},
		ShortRegen:  regenFull,
		LongRegen:   regenFull,
		FullRegen:   regenFull,
		Restriction: "Disrupt a Genjutsu-affected creature's chakra, or (5th+) disable their chakra-molding until the end of their next turn",
	},
	// "You have a number of Actualization Die equal to your Proficiency
	// Bonus. You recover spent die on a short or long rest." Die SIZE
	// (d4->d6@9th->d8@17th) is a separate, level-only lookup
	// (actualizationDieSize, genjutsu.go) — this pool only tracks the
	// COUNT, same split Hunter-Nin's Lethal Attack die-size readout draws
	// against its own unrelated resource pools.
	"class/genjutsu-specialist/feature/actualization": {
		Key:  "actualization_die",
		Name: "Actualization Die",
		Max: func(cl map[string]int, con, prof int) int {
			return prof
		},
		ShortRegen: regenFull,
		LongRegen:  regenFull,
		FullRegen:  regenFull,
	},
}

// validCustomResourceKey reports whether key matches some grant's Key —
// used by handleSheetCustomResource to reject a crafted request naming an
// arbitrary resource_key before it ever reaches charstore.
func validCustomResourceKey(key string) bool {
	for _, g := range customResourceGrants {
		if g.Key == key {
			return true
		}
	}
	return false
}

// CustomResourceEntry is one resolved custom resource, ready for the
// sheet's Special Resources box.
type CustomResourceEntry struct {
	Key         string
	Name        string
	Current     int
	Max         int
	Restriction string
	// ShortRegen/LongRegen/FullRegen let handleSheetRest apply this
	// entry's own regen rule for whichever tier of rest was just taken,
	// without a second lookup back into customResourceGrants.
	ShortRegen restRegen
	LongRegen  restRegen
	FullRegen  restRegen
}

// computeCustomResources resolves customResourceGrants against a
// character's granted features (the same loadMergedGrantedFeatures result
// the Core tab and passive traits already use — feats included, since the
// White Chakra Surge and Purple Lightning feats must reach this table
// too), the character's per-class level map, and CON modifier.
//
// stored holds whatever's actually saved in character_custom_resources,
// keyed by resource Key (charstore.GetCustomResources) — a resource with
// no stored row yet starts at its own Max, the same implicit-seed
// convention Sheet.MaxChakraAuto already establishes for current_chakra.
//
// When more than one grant shares a Key (White Chakra Surge stacking onto
// the base Hatake grant), the higher computed Max wins, same "take the
// stronger" shape computePassiveTraits already uses for escalating grants.
func computeCustomResources(features []grantedFeatureRow, classLevels map[string]int, conMod, profBonus int, characterLevel int, stored map[string]int) []CustomResourceEntry {
	type resolved struct {
		grant customResourceGrant
		max   int
	}
	byKey := map[string]resolved{}
	var order []string

	for _, f := range features {
		grant, ok := customResourceGrants[f.Slug]
		if !ok {
			continue
		}
		if grant.MinLevel > 0 && characterLevel < grant.MinLevel {
			continue
		}
		max := grant.Max(classLevels, conMod, profBonus)
		if existing, seen := byKey[grant.Key]; !seen {
			byKey[grant.Key] = resolved{grant: grant, max: max}
			order = append(order, grant.Key)
		} else if max > existing.max {
			// The winning (higher-Max) grant also supplies the regen
			// kind/Restriction that governs this key from here on.
			byKey[grant.Key] = resolved{grant: grant, max: max}
		}
	}

	out := make([]CustomResourceEntry, 0, len(order))
	for _, key := range order {
		r := byKey[key]
		current, ok := stored[key]
		if !ok {
			current = r.max
		}
		if current > r.max {
			current = r.max
		}
		out = append(out, CustomResourceEntry{
			Key:         key,
			Name:        r.grant.Name,
			Current:     current,
			Max:         r.max,
			Restriction: r.grant.Restriction,
			ShortRegen:  r.grant.ShortRegen,
			LongRegen:   r.grant.LongRegen,
			FullRegen:   r.grant.FullRegen,
		})
	}
	return out
}

// applyRestRegen computes a custom resource's new current value for one
// rest tier. Returns the unchanged current value for regenNone.
func applyRestRegen(kind restRegen, current, max, conMod int) int {
	clamp := func(v int) int {
		if v > max {
			return max
		}
		if v < 0 {
			return 0
		}
		return v
	}
	switch kind {
	case regenConMod:
		gain := conMod
		if gain < 0 {
			gain = 0
		}
		return clamp(current + gain)
	case regenHalfSpent:
		spent := max - current
		return clamp(current + spent/2)
	case regenHalfMax:
		v := max / 2
		if v < current {
			return current
		}
		return v
	case regenFull:
		return max
	default:
		return current
	}
}

// loadCustomResources gathers everything computeCustomResources needs for
// one character: its granted features (class/subclass/clan/feats, same
// merged list the Core tab and passive traits use), per-class level map,
// and CON modifier, then resolves the curated grant table against them.
func (s *server) loadCustomResources(characterID int64, sheet *charsheet.Sheet) ([]CustomResourceEntry, error) {
	features, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	classes, err := s.loadCharacterClassLevels(characterID)
	if err != nil {
		return nil, err
	}
	classLevels := make(map[string]int, len(classes))
	for _, c := range classes {
		classLevels[c.Slug] = c.Levels
	}
	stored, err := charstore.GetCustomResources(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	conMod := sheet.Abilities["con"].Modifier
	return computeCustomResources(features, classLevels, conMod, sheet.ProficiencyBonus, sheet.Level, stored), nil
}

// applyCustomResourceRest applies one rest tier's regen to every custom
// resource a character has, via SetCustomResourceValue. tier selects which
// of ShortRegen/LongRegen/FullRegen governs each entry.
func (s *server) applyCustomResourceRest(characterID int64, sheet *charsheet.Sheet, tier string) error {
	entries, err := s.loadCustomResources(characterID, sheet)
	if err != nil {
		return err
	}
	conMod := sheet.Abilities["con"].Modifier
	for _, e := range entries {
		var kind restRegen
		switch tier {
		case "full":
			kind = e.FullRegen
		case "long":
			kind = e.LongRegen
		default:
			kind = e.ShortRegen
		}
		if kind == regenNone {
			continue
		}
		newValue := applyRestRegen(kind, e.Current, e.Max, conMod)
		if newValue == e.Current {
			continue
		}
		if err := charstore.SetCustomResourceValue(s.charDB, characterID, e.Key, newValue); err != nil {
			return err
		}
	}
	return nil
}
