package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Puppet Master's "Generalized Skill" (class_features, granted at 5th
// level): "select a number of skills equal to 2 + your Intelligence
// Modifier. Your Puppet Tools may use your skill bonuses for these chosen
// skills (though they may calculate these bonuses using their statistics,
// if the result is higher)." Character-level, not per-companion — the
// feature's own text applies the same chosen list to every Puppet Tool the
// character has.
//
// Deliberately NOT modeled: the "if you do not have a Puppet Tool, select
// two skills [and] gain proficiency in these skills [yourself]" fallback —
// that grants the CHARACTER their own proficiency directly rather than
// sharing it with a companion, a different subsystem (character_proficiencies,
// see internal/features' fixedProficiencyGrants) than this file's own
// per-companion resolution. Also not modeled: the feature's alternate "trade
// this whole feature for one permanent Mastery rank" mode, since Mastery
// doesn't exist anywhere in this app yet (see the n5e-feature-grants-audit
// project notes).

// puppetGeneralizedSkillFreeGrants: a Puppet Upgrade's own entry slug ->
// skill(s) it adds to the Generalized Skill list for free, not counted
// against the character's own pick cap — hand-verified against the two
// upgrades whose own text actually says this (dist/rules.db, Chakra
// Regulators and Decepticon), same "curated table, not a text parser"
// precedent passiveTraitGrants (passive_traits.go) already established.
var puppetGeneralizedSkillFreeGrants = map[string][]string{
	"class/puppet-master/option/puppet-master-upgrades/bronze-tier/entry/chakra-regulators": {"Chakra Control"},
	"class/puppet-master/option/upgrades-of-war/silver-tier/entry/decepticon":               {"Deception", "Stealth"},
}

// puppetFlatSkillGrants: a Puppet Upgrade's own entry slug -> skill(s) the
// Puppet Tool that took it is directly proficient in, independent of
// Generalized Skill entirely. Each of these also prints a Mastery-rank
// escalation clause ("or +1 rank of Mastery, if already proficient...") that
// is deliberately not modeled here — see this file's own header comment on
// why Mastery isn't implemented anywhere yet.
var puppetFlatSkillGrants = map[string][]string{
	"class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/ghillie-coating": {"Stealth"},
	"class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/quickfooted":     {"Athletics", "Acrobatics"},
	"class/puppet-master/option/magus-upgrades/silver-tier/entry/overflowing-chakra":    {"Chakra Control"},
}

// generalizedSkillPickRow is one of the character's own chosen Generalized
// Skill names, plus the character's OWN skill modifier for it — shown on
// the character-wide picker so it's rollable directly (previously just a
// bare name with nothing to roll: clicking it did nothing, since a name
// alone carries no modifier). This is the character's own number, not any
// specific companion's resolved one (the picker isn't scoped to one
// companion) — resolvePuppetSkills is still what each companion card's own
// list uses to show its own, possibly-higher, per-companion number.
type generalizedSkillPickRow struct {
	Name     string
	Modifier int
}

// puppetGeneralizedSkillCap returns how many skills a character can have
// chosen for Generalized Skill right now: 0 before level 5 (not unlocked
// yet), 2 + Intelligence modifier from level 5 on, floored at 0 (a very low
// Int score should never produce a negative cap).
func puppetGeneralizedSkillCap(sheet *charsheet.Sheet, level int) int {
	if level < 5 || sheet == nil {
		return 0
	}
	cap := 2 + sheet.Abilities["int"].Modifier
	if cap < 0 {
		return 0
	}
	return cap
}

// puppetSkillRow is one resolved skill check a specific Puppet Tool can
// make, alongside its Puppet Master — same shape as charsheet.SkillEntry
// (Name/Ability/Proficient/Modifier) so the Puppet Skills box can be laid
// out exactly like the Core tab's own Skills box, grouped by ability with
// the same proficiency-dot styling.
type puppetSkillRow struct {
	Name       string
	Ability    string
	Proficient bool
	Modifier   int
}

// puppetSkillGroupRow mirrors characters.go's own skillGroupRow, grouping
// an already ability-then-name-sorted []puppetSkillRow (see
// groupPuppetSkillsByAbility) the same way groupSkillsByAbility does for
// the Core tab.
type puppetSkillGroupRow struct {
	Ability string
	Skills  []puppetSkillRow
}

// groupPuppetSkillsByAbility is characters.go's groupSkillsByAbility,
// specialized to puppetSkillRow — kept as a small parallel function rather
// than a generic one, since the two element types differ only in which
// extra fields they carry and a generic version would buy nothing here.
func groupPuppetSkillsByAbility(skills []puppetSkillRow) []puppetSkillGroupRow {
	var groups []puppetSkillGroupRow
	for _, sk := range skills {
		if n := len(groups); n == 0 || groups[n-1].Ability != sk.Ability {
			groups = append(groups, puppetSkillGroupRow{Ability: sk.Ability})
		}
		g := &groups[len(groups)-1]
		g.Skills = append(g.Skills, sk)
	}
	return groups
}

// resolvePuppetSkills computes companion c's full 21-skill list — every
// named skill, not just the ones some grant or pick touches, per "Generally,
// a puppet will take its skill statistics from the Puppet Master himself,
// unless the puppet has an upgrade installed that says otherwise": the
// DEFAULT for every skill is simply the character's OWN already-computed
// SkillEntry (sheet.Skills), copied as-is — same ability score, same
// proficiency, same modifier, since that default IS "the Puppet Master's
// own statistics."
//
// Two things can override that default, layered in this order:
//  1. puppetFlatSkillGrants — an installed upgrade (Ghillie Coating, etc.)
//     making the companion proficient in a specific skill it wasn't already
//     proficient in via the character. Recomputed from the CHARACTER's own
//     ability score (the default's own ability, not the companion's) plus
//     the character's Proficiency Bonus — the grant is read as "you're now
//     proficient using the same stats you'd otherwise use," not as also
//     swapping to the companion's own ability score; the Mastery-rank
//     escalation clause every one of these grants also prints ("...or +1
//     rank of Mastery, if already proficient") is deliberately not modeled,
//     same reason as this file's own header comment (Mastery doesn't exist
//     anywhere in this app yet).
//  2. Generalized Skill's own chosen names (globalPicks, character-wide) —
//     "may use your skill bonus... though they may calculate these bonuses
//     using their own statistics, if the result is higher." Only the
//     NUMBER can improve here, using the companion's own ability score with
//     the SAME proficiency state resolved by step 1 (RAW grants no NEW
//     proficiency from being chosen — only permission to compare against
//     the companion's own stats).
//
// abilityOverride swaps which ability score a named skill's modifier reads
// for THIS companion — Puppet Roles' Surveyor: "may use Wisdom in place of
// Intelligence for Investigation." The skill still displays and sorts under
// its own normal governing ability (Investigation stays grouped under INT,
// matching sheet.Skills' own ability-then-name order groupPuppetSkillsByAbility
// assumes); only the modifier itself is recomputed from the overridden
// ability. nil/empty is the common case (no override at all).
func resolvePuppetSkills(sheet *charsheet.Sheet, companion charstore.Companion, globalPicks, upgradeSlugs []string, abilityOverride map[string]string) []puppetSkillGroupRow {
	if sheet == nil || len(sheet.Skills) == 0 {
		return nil
	}
	generalizedNames := map[string]bool{}
	for _, n := range globalPicks {
		generalizedNames[n] = true
	}
	flatGrantedNames := map[string]bool{}
	for _, slug := range upgradeSlugs {
		for _, n := range puppetGeneralizedSkillFreeGrants[slug] {
			generalizedNames[n] = true
		}
		for _, n := range puppetFlatSkillGrants[slug] {
			flatGrantedNames[n] = true
		}
	}

	rows := make([]puppetSkillRow, 0, len(sheet.Skills))
	for _, sk := range sheet.Skills {
		readAbility, modifier := sk.Ability, sk.Modifier
		if override, ok := abilityOverride[sk.Name]; ok && override != readAbility {
			readAbility = override
			modifier = sheet.Abilities[readAbility].Modifier
			if sk.Proficient {
				modifier += sheet.ProficiencyBonus
			}
		}
		row := puppetSkillRow{Name: sk.Name, Ability: sk.Ability, Proficient: sk.Proficient, Modifier: modifier}
		if flatGrantedNames[sk.Name] && !row.Proficient {
			row.Proficient = true
			row.Modifier = charsheet.CheckModifier(sheet.Abilities[readAbility].Modifier, sheet.ProficiencyBonus, true)
		}
		if generalizedNames[sk.Name] {
			ownMod := companionAbilityModifier(companion, readAbility)
			if row.Proficient {
				ownMod += sheet.ProficiencyBonus
			}
			if ownMod > row.Modifier {
				row.Modifier = ownMod
			}
		}
		rows = append(rows, row)
	}
	return groupPuppetSkillsByAbility(rows)
}

// handleGeneralizedSkillAdd chooses one skill for Generalized Skill, gated
// by the character's own current cap server-side (the same defense-in-depth
// every other cap check on this sheet gets).
func (s *server) handleGeneralizedSkillAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("skill_name"))
	if _, ok := charsheet.SkillAbility[name]; !ok {
		http.Error(w, "unknown skill", http.StatusBadRequest)
		return
	}

	level, err := s.puppetMasterClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master level for generalized skill add:", err)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for generalized skill add:", err)
		return
	}
	picks, err := charstore.ListGeneralizedSkills(s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load generalized skills for add:", err)
		return
	}
	if len(picks) >= puppetGeneralizedSkillCap(sheet, level) {
		http.Error(w, "no generalized skill slots remaining", http.StatusBadRequest)
		return
	}

	if err := charstore.AddGeneralizedSkill(s.charDB, id, name); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add generalized skill:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handleGeneralizedSkillDelete drops one chosen Generalized Skill.
func (s *server) handleGeneralizedSkillDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("skill")
	if err := charstore.RemoveGeneralizedSkill(s.charDB, id, name); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove generalized skill:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}
