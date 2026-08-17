// Bingo Book Pack 1 parser (Adversaries - Class, Freeform, Roles, Role
// Traits). This is a GM adversary-building rules chapter, not a bestiary of
// named creatures: Minion/Standard/Elite/Solo rank templates, a five-Role
// system (Striker, Lurker, Defender, Controller, Supporter — despite the
// book's own prose mostly grouping "Lurker" as a Striker sub-flavor, its
// ROLE DEFINITIONS section defines and describes it exactly like the other
// four, and ROLE TRAITS gives it its own trait catalog, so it's modeled as
// a fifth real Role, not a Striker sub-category), and that Role system's
// catalog of ~150 named, ranked traits.
//
// The surrounding procedural GM-narrative prose (Elite Actions, Tenacity
// spending/recharge, Mob/Collective Turn, Phased Combat/Health Gates/
// Transformations, Optional Classifications) is deliberately NOT parsed
// into structured rows — same boundary already drawn around the core
// book's jutsu-creation-cost chapter: real rules text, but not enumerable
// entities, readable only via the source PDF.
package parse

import (
	"regexp"
	"strconv"
	"strings"
)

// AdversaryRole is one of the five named Roles from ROLE DEFINITIONS.
type AdversaryRole struct {
	Name        string // "Striker", "Lurker", "Defender", "Controller", "Supporter"
	Description string
	SourcePage  int
}

// AdversaryRoleTrait is one named, ranked trait an Adversary built with a
// given Role can be given (ROLE TRAITS).
type AdversaryRoleTrait struct {
	Name        string
	RoleName    string // matches an AdversaryRole.Name
	Rank        string // single letter: D, C, B, A, S
	Description string
	SourcePage  int
}

// AdversaryRank is one of the four Minion/Standard/Elite/Solo templates.
// HPFormula is kept as printed text, not decomposed further: Minion's is a
// flat replacement formula ("10 + Level"), Standard has none at all (it's
// the baseline — the ordinary player-style Hit Point calculation applies
// unmodified, confirmed by there being no "STANDARD ADVERSARY TEMPLATE"
// block in the book at all), and Elite/Solo's are multipliers applied on
// top of that same baseline ("+1.25", "+1.5 + 0.5 x Per Player").
type AdversaryRank struct {
	Name        string
	HPFormula   string
	ACBonus     int
	SaveBonus   int
	SaveDCBonus int
	InitBonus   int
	Notes       string
	SourcePage  int
}

// AdversaryFreeformSlot is one level-gated row of a rank's Freeform Slots
// template (how many Freeform Attacks/Jutsu an adversary of that rank and
// level can use).
type AdversaryFreeformSlot struct {
	RankName string
	LevelMin int
	Slots    int
}

// adversaryRoleNames is the book's fixed, closed set — used both to
// recognize a ROLE DEFINITIONS heading and to know when a ROLE TRAITS
// heading switches which Role subsequent traits belong to. A role name
// appearing anywhere else (there is nowhere else it legitimately would)
// would silently misattribute traits, so this list is deliberately closed
// rather than "any caps line with no rank line after it."
var adversaryRoleNames = map[string]bool{
	"STRIKER": true, "LURKER": true, "DEFENDER": true,
	"CONTROLLER": true, "SUPPORTER": true,
}

var roleTraitRankRe = regexp.MustCompile(`^(D|C|B|A|S)-Rank$`)

// ParseAdversaryRoles scans the Bingo Book's ROLE DEFINITIONS section (each
// Role's own name-then-description paragraph, ending where ROLE TRAITS
// begins).
func ParseAdversaryRoles(lines []Line) ([]AdversaryRole, []Anomaly) {
	var ls []Line
	inSection := false
	for _, ln := range lines {
		if !inSection {
			inSection = ln.Text == "ROLE DEFINITIONS"
			continue
		}
		if ln.Text == "ROLE TRAITS" {
			break
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		return nil, []Anomaly{{Subject: "Adversary Roles", Problem: "ROLE DEFINITIONS section not found"}}
	}

	var roles []AdversaryRole
	var anomalies []Anomaly
	var cur *AdversaryRole
	flush := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(cur.Description)
		if cur.Description == "" {
			anomalies = append(anomalies, Anomaly{Page: cur.SourcePage, Subject: cur.Name,
				Problem: "role with empty description"})
		} else {
			roles = append(roles, *cur)
		}
		cur = nil
	}
	for _, ln := range ls {
		if adversaryRoleNames[ln.Text] {
			flush()
			cur = &AdversaryRole{Name: tidyName(ln.Text), SourcePage: ln.Page}
			continue
		}
		if cur == nil {
			continue // section intro, if any
		}
		cur.Description += " " + ln.Text
	}
	flush()
	return roles, anomalies
}

// ParseAdversaryRoleTraits scans the Bingo Book's ROLE TRAITS section (from
// its own heading to the end of the book — Pack 1 has nothing after it).
// The CLARIFICATIONS glossary block (Auras/Ambush/Barrier/Intercept/Mark/
// Strike/Saving Throws definitions) sits between the section heading and
// the first Role heading; it's never captured as a trait because no trait
// is open yet when its lines are scanned — same "not enumerable data" call
// this parser makes for the GM-narrative sections it never reaches at all.
func ParseAdversaryRoleTraits(lines []Line) ([]AdversaryRoleTrait, []Anomaly) {
	var ls []Line
	inSection := false
	for _, ln := range lines {
		if !inSection {
			inSection = ln.Text == "ROLE TRAITS"
			continue
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		return nil, []Anomaly{{Subject: "Adversary Role Traits", Problem: "ROLE TRAITS section not found"}}
	}

	var traits []AdversaryRoleTrait
	var anomalies []Anomaly
	role := ""
	var cur *AdversaryRoleTrait
	flush := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(cur.Description)
		if cur.Description == "" {
			anomalies = append(anomalies, Anomaly{Page: cur.SourcePage, Subject: cur.Name,
				Problem: "role trait with empty description"})
		} else {
			traits = append(traits, *cur)
		}
		cur = nil
	}
	for i := 0; i < len(ls); i++ {
		text := ls[i].Text
		if adversaryRoleNames[text] {
			flush()
			role = tidyName(text)
			continue
		}
		if capsLineRe.MatchString(text) && i+1 < len(ls) && roleTraitRankRe.MatchString(ls[i+1].Text) {
			flush()
			cur = &AdversaryRoleTrait{
				Name: tidyName(text), RoleName: role,
				Rank: ls[i+1].Text[:1], SourcePage: ls[i].Page,
			}
			i++ // consume the rank line too
			continue
		}
		if cur == nil {
			continue // CLARIFICATIONS glossary / section intro — not enumerable data
		}
		cur.Description += " " + text
	}
	flush()
	return traits, anomalies
}

var (
	minionHPRe     = regexp.MustCompile(`^Hit Points: (.+) Initiative Bonus: ([+-]?\d+)$`)
	freeformRowRe  = regexp.MustCompile(`^(\d+)\+ (\d+)$`)
	eliteSoloRowRe = regexp.MustCompile(`^(\d+\+|-) (\d+|-) (\d+\+|-) (\d+|-)$`)
	hitChakraModRe = regexp.MustCompile(`^Hit/Chakra Point Modifier: (.+)$`)
	acSaveRe       = regexp.MustCompile(`^AC Bonus ([+-]?\d+) Saving Throw Bonus ([+-]?\d+)$`)
	saveDCInitRe   = regexp.MustCompile(`^Save DC Bonus ([+-]?\d+) Initiative ([+-]?\d+)$`)
)

// ParseAdversaryRanks scans the Bingo Book's opening chapter for the four
// rank templates (MINIONS through the Solo Adversary Template, just before
// the unrelated Elite Enhancements narrative section begins) and their
// Freeform Slots tables.
func ParseAdversaryRanks(lines []Line) ([]AdversaryRank, []AdversaryFreeformSlot, []Anomaly) {
	var ls []Line
	inSection := false
	for _, ln := range lines {
		if !inSection {
			inSection = ln.Text == "MINIONS"
			continue
		}
		if ln.Text == "ELITE ENHANCEMENTS" {
			break
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		return nil, nil, []Anomaly{{Subject: "Adversary Ranks", Problem: "rank template section not found"}}
	}

	var anomalies []Anomaly
	var ranks []AdversaryRank
	var slots []AdversaryFreeformSlot
	flag := func(page int, subject, problem string) {
		anomalies = append(anomalies, Anomaly{Page: page, Subject: subject, Problem: problem})
	}

	idx := map[string]int{}
	for i, ln := range ls {
		idx[ln.Text] = i
	}

	// Minion: flat HP replacement + a fixed Initiative penalty, no AC/Save
	// bonuses printed anywhere (there is no such table for Minions).
	if i, ok := idx["MINION ADVERSARY TEMPLATE"]; ok && i+1 < len(ls) {
		if m := minionHPRe.FindStringSubmatch(ls[i+1].Text); m != nil {
			initBonus, _ := strconv.Atoi(m[2])
			ranks = append(ranks, AdversaryRank{
				Name: "Minion", HPFormula: strings.TrimSpace(m[1]), InitBonus: initBonus,
				Notes: "Minions automatically fail saving throws.", SourcePage: ls[i].Page,
			})
		} else {
			flag(ls[i].Page, "Minion", "Hit Points/Initiative Bonus line not found or unrecognized")
		}
	} else {
		flag(0, "Minion", "MINION ADVERSARY TEMPLATE heading not found")
	}

	// Standard has no template block in the book at all — it's the
	// baseline every other rank is defined relative to (ordinary
	// player-style HP calculation, zero bonuses). Not derived from any
	// matched text; recorded here as an explicit, auditable fact rather
	// than left implicit.
	ranks = append(ranks, AdversaryRank{Name: "Standard"})

	parseEliteOrSolo := func(name, heading string) {
		i, ok := idx[heading]
		if !ok || i+2 >= len(ls) {
			flag(0, name, heading+" block not found")
			return
		}
		modM := hitChakraModRe.FindStringSubmatch(ls[i+1].Text)
		acM := acSaveRe.FindStringSubmatch(ls[i+2].Text)
		var dcM []string
		if i+3 < len(ls) {
			dcM = saveDCInitRe.FindStringSubmatch(ls[i+3].Text)
		}
		if modM == nil || acM == nil || dcM == nil {
			flag(ls[i].Page, name, heading+" lines not recognized")
			return
		}
		acBonus, _ := strconv.Atoi(acM[1])
		saveBonus, _ := strconv.Atoi(acM[2])
		saveDCBonus, _ := strconv.Atoi(dcM[1])
		initBonus, _ := strconv.Atoi(dcM[2])
		ranks = append(ranks, AdversaryRank{
			Name: name, HPFormula: strings.TrimSpace(modM[1]),
			ACBonus: acBonus, SaveBonus: saveBonus, SaveDCBonus: saveDCBonus, InitBonus: initBonus,
			SourcePage: ls[i].Page,
		})
	}
	parseEliteOrSolo("Elite", "ELITE ADVERSARY TEMPLATE")
	parseEliteOrSolo("Solo", "SOLO ADVERSARY TEMPLATE")

	// Freeform Slots: Minion and Standard are single-column level->slots
	// tables; Elite and Solo share one two-column table (some level rows
	// exist for only one of the two, printed as "-").
	parseSingleColumn := func(rankName, heading string) {
		i, ok := idx[heading]
		if !ok {
			flag(0, rankName, heading+" heading not found")
			return
		}
		for j := i + 2; j < len(ls); j++ { // i+1 is the "Level Freeform Slots" column header
			m := freeformRowRe.FindStringSubmatch(ls[j].Text)
			if m == nil {
				break
			}
			lvl, _ := strconv.Atoi(m[1])
			n, _ := strconv.Atoi(m[2])
			slots = append(slots, AdversaryFreeformSlot{RankName: rankName, LevelMin: lvl, Slots: n})
		}
	}
	parseSingleColumn("Minion", "MINION FREEFORM TEMPLATE")
	parseSingleColumn("Standard", "STANDARD FREEFORM TEMPLATE")

	if i, ok := idx["ELITE/SOLO FREEFORM TEMPLATE"]; ok {
		for j := i + 2; j < len(ls); j++ { // i+1 is the "Level ... Slots  Level ... Slots" column header
			m := eliteSoloRowRe.FindStringSubmatch(ls[j].Text)
			if m == nil {
				break
			}
			if m[1] != "-" {
				lvl, _ := strconv.Atoi(strings.TrimSuffix(m[1], "+"))
				n, _ := strconv.Atoi(m[2])
				slots = append(slots, AdversaryFreeformSlot{RankName: "Elite", LevelMin: lvl, Slots: n})
			}
			if m[3] != "-" {
				lvl, _ := strconv.Atoi(strings.TrimSuffix(m[3], "+"))
				n, _ := strconv.Atoi(m[4])
				slots = append(slots, AdversaryFreeformSlot{RankName: "Solo", LevelMin: lvl, Slots: n})
			}
		}
	} else {
		flag(0, "Elite/Solo", "ELITE/SOLO FREEFORM TEMPLATE heading not found")
	}

	return ranks, slots, anomalies
}
