package main

import (
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/puppetupgrades"
)

// This file wires internal/puppetupgrades/foundation.go's curated table for
// the 4 mandatory 2nd-level "build your puppet" picks (Puppeteer Chassis,
// Puppet Frameworks, Puppet Roles, Puppet Weapon Types) into the Puppets
// tab's own existing computed-hint/COALESCE-backfill pipeline
// (loadPuppetsTabData) and sub-choice mechanism (puppetUpgradeSubChoiceSpecs)
// — see foundation.go's own header for what is and isn't modeled and why.
//
// Each of the 17 entries is an untiered class_options row (puppetUpgradeOption's
// own doc explains the shape): picking one writes a real
// character_companion_upgrades row whose upgrade_entry_slug equals the
// class_options.slug directly, which is also the key
// puppetupgrades.FoundationEntries is keyed by — no separate lookup table
// needed to go from "what did this companion pick" to "what does that pick
// mean."

// puppetFoundationCatalogPrefixes: a companion's own picked
// class_options.slug always starts with exactly one of these when it's a
// Foundation catalog pick — used only to group companions.go's Attack list
// merge; foundation.go's own FoundationEntries map is what actually decides
// membership everywhere else.
var puppetFoundationCatalogPrefixes = []string{
	"class/puppet-master/option/puppeteer-chassis/",
	"class/puppet-master/option/puppet-frameworks/",
	"class/puppet-master/option/puppet-roles/",
	"class/puppet-master/option/puppet-weapon-types/",
}

// puppetFoundationPick is one companion's own resolved Chassis/Framework/
// Role/Weapon Type pick — the curated entry it maps to, plus its own
// upgrade row id and ordered sub-choices (ability/size/weapon-damage-type,
// where applicable).
type puppetFoundationPick struct {
	Entry      puppetupgrades.FoundationEntry
	UpgradeID  int64
	SubChoices []charstore.CompanionUpgradeChoice
}

// resolvePuppetFoundationPicks matches a companion's own upgrade picks
// against foundation.go's curated table — in practice at most one match,
// since these 4 catalogs are each gated to a single subclass and a
// character only ever has one Puppet Master subclass chosen, but nothing
// here assumes that; every match found is resolved and folded in.
func resolvePuppetFoundationPicks(upgradePicks []charstore.CompanionUpgrade, choices []charstore.CompanionUpgradeChoice) []puppetFoundationPick {
	choicesByUpgradeID := map[int64][]charstore.CompanionUpgradeChoice{}
	for _, ch := range choices {
		choicesByUpgradeID[ch.CompanionUpgradeID] = append(choicesByUpgradeID[ch.CompanionUpgradeID], ch)
	}
	var out []puppetFoundationPick
	for _, u := range upgradePicks {
		entry, ok := puppetupgrades.FoundationEntries[u.UpgradeEntrySlug]
		if !ok {
			continue
		}
		out = append(out, puppetFoundationPick{Entry: entry, UpgradeID: u.ID, SubChoices: choicesByUpgradeID[u.ID]})
	}
	return out
}

// puppetFoundationMixedChoiceKinds reports whether an entry offers more
// than one KIND of sub-choice under its single pick (Specialized: ability +
// size + weapon damage type; Sentinel: ability + size). Every other entry
// needing a sub-choice at all offers exactly one kind (always ability), so
// its own picks need no prefix to stay unambiguous — see
// puppetFoundationSubChoiceSpec/puppetFoundationSubChoiceValues below, which
// both branch on this.
func puppetFoundationMixedChoiceKinds(e puppetupgrades.FoundationEntry) int {
	kinds := 0
	if len(e.AbilityChoiceGroups) > 0 {
		kinds++
	}
	if len(e.SizeChoices) > 0 {
		kinds++
	}
	if e.NaturalWeapon != nil && len(e.NaturalWeapon.DamageTypeChoices) > 0 {
		kinds++
	}
	if e.SageCreatureFramework {
		kinds += 2 // Bestial Framework only: tribe pick + D-Rank trait picks, on top of ability
	}
	return kinds
}

func puppetFoundationAbilityOptions(choices []string, prefix string) []puppetUpgradeSubChoiceOption {
	out := make([]puppetUpgradeSubChoiceOption, len(choices))
	for i, key := range choices {
		out[i] = puppetUpgradeSubChoiceOption{Slug: prefix + key, Name: abilityLabels[key]}
	}
	return out
}

// capitalizeFirst upper-cases only a value's first rune — values passed
// through here (a size like "Small", a damage type like "piercing") are
// always a single already-lowercase-or-title-case word, so this is
// sufficient without pulling in strings.Title (deprecated) or a full
// title-casing package.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func puppetFoundationValueOptions(values []string, prefix string) []puppetUpgradeSubChoiceOption {
	out := make([]puppetUpgradeSubChoiceOption, len(values))
	for i, v := range values {
		out[i] = puppetUpgradeSubChoiceOption{Slug: prefix + v, Name: capitalizeFirst(v)}
	}
	return out
}

// puppetFoundationSubChoiceSpec builds a puppetUpgradeSubChoiceSpec straight
// from a FoundationEntry's own ability/size/weapon-damage choice fields —
// reused by an init() below to extend the existing, hand-curated
// puppetUpgradeSubChoiceSpecs map, so these 17 entries plug into exactly
// the same picker UI/handler code every other repeatable-upgrade sub-choice
// already uses. Returns ok=false for an entry with no player choice left to
// make at all (11 of the 17 — a fixed ASI/size/weapon needs no sub-choice).
func puppetFoundationSubChoiceSpec(e puppetupgrades.FoundationEntry) (puppetUpgradeSubChoiceSpec, bool) {
	// Bestial Framework needs a tribe pick and 3 level-gated D-Rank trait
	// picks on top of its own 2 ability picks — this generic builder only
	// ever produces the ability half. Registered instead by
	// puppet_bestial_framework.go's own init(), covering all 6 slots in one
	// DynamicOptions spec; skipped here so that registration is never
	// clobbered regardless of which file's init() happens to run first
	// (Go runs same-package init()s in lexical filename order, which is not
	// something either file should have to depend on).
	if e.SageCreatureFramework {
		return puppetUpgradeSubChoiceSpec{}, false
	}
	mixed := puppetFoundationMixedChoiceKinds(e) > 1
	var options []puppetUpgradeSubChoiceOption
	quantity := 0
	if len(e.AbilityChoiceGroups) > 0 {
		prefix := ""
		if mixed {
			prefix = "ability:"
		}
		options = append(options, puppetFoundationAbilityOptions(e.AbilityChoiceGroups[0].Choices, prefix)...)
		quantity += len(e.AbilityChoiceGroups)
	}
	if len(e.SizeChoices) > 0 {
		prefix := ""
		if mixed {
			prefix = "size:"
		}
		options = append(options, puppetFoundationValueOptions(e.SizeChoices, prefix)...)
		quantity++
	}
	if e.NaturalWeapon != nil && len(e.NaturalWeapon.DamageTypeChoices) > 0 {
		prefix := ""
		if mixed {
			prefix = "weapon:"
		}
		options = append(options, puppetFoundationValueOptions(e.NaturalWeapon.DamageTypeChoices, prefix)...)
		quantity++
	}
	if quantity == 0 {
		return puppetUpgradeSubChoiceSpec{}, false
	}
	return puppetUpgradeSubChoiceSpec{Quantity: quantity, FixedOptions: options}, true
}

// puppetFrameworkJutsuRankBySlot/puppetFrameworkJutsuMinLevelBySlot: Puppet
// Frameworks' Shade ("The Shade gains 1 D-Rank Genjutsu of your choice...
// At 10th level, one C-Rank Genjutsu; at 17th, one B-Rank") and Spellblade
// (the identical clause, but Ninjutsu) each grant 3 jutsu picks, one per
// Puppet Master level tier — slot 0/1/2 is D/C/B-Rank, gated at Puppet
// Master level 0/10/17 respectively.
var puppetFrameworkJutsuRankBySlot = [3]string{"D", "C", "B"}
var puppetFrameworkJutsuMinLevelBySlot = [3]int{0, 10, 17}

// puppetFrameworkJutsuOptions resolves one Shade/Spellblade Framework jutsu
// slot's own options — shared by shadeFrameworkJutsuOptions/
// spellbladeFrameworkJutsuOptions below, which only differ in which jutsu
// classification they search. Returns nil, nil (not an error) for a slot
// below its own Puppet Master level gate, so the picker renders no options
// for a still-locked slot rather than an empty-but-technically-open
// dropdown. Queries v_jutsu (not the raw jutsu table) so a rank_override
// row is filtered on its effective rank, same as every other jutsu listing
// in this app (see cmd/n5e/jutsu.go's own loadJutsuList/loadJutsuDetail).
func puppetFrameworkJutsuOptions(s *server, characterID int64, pickIndex int, classification string) ([]puppetUpgradeSubChoiceOption, error) {
	if pickIndex < 0 || pickIndex >= len(puppetFrameworkJutsuRankBySlot) {
		return nil, nil
	}
	level, err := s.puppetMasterClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if level < puppetFrameworkJutsuMinLevelBySlot[pickIndex] {
		return nil, nil
	}
	rows, err := s.rulesDB.Query(
		`SELECT slug, name, COALESCE(description, '') FROM v_jutsu
		 WHERE classification = ? AND rank = ? ORDER BY name`,
		classification, puppetFrameworkJutsuRankBySlot[pickIndex])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []puppetUpgradeSubChoiceOption
	for rows.Next() {
		var o puppetUpgradeSubChoiceOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		o.DetailHref = "/jutsu/" + o.Slug
		out = append(out, o)
	}
	return out, rows.Err()
}

// shadeFrameworkJutsuEntrySlug/spellbladeFrameworkJutsuEntrySlug: the two
// Framework catalog picks whose jutsu-grant clause is modeled via a
// DynamicOptions sub-choice instead of puppetFoundationSubChoiceSpec's
// generic ability/size/weapon-damage-type auto-builder — neither entry has
// any of those 3 fields, so the auto-builder returns ok=false for both and
// leaves them free for this explicit registration below.
const shadeFrameworkJutsuEntrySlug = "class/puppet-master/option/puppet-frameworks/shade-framework"
const spellbladeFrameworkJutsuEntrySlug = "class/puppet-master/option/puppet-frameworks/spellblade-framework"

func shadeFrameworkJutsuOptions(s *server, characterID, companionID int64, pickIndex int) ([]puppetUpgradeSubChoiceOption, error) {
	return puppetFrameworkJutsuOptions(s, characterID, pickIndex, "Genjutsu")
}

func spellbladeFrameworkJutsuOptions(s *server, characterID, companionID int64, pickIndex int) ([]puppetUpgradeSubChoiceOption, error) {
	return puppetFrameworkJutsuOptions(s, characterID, pickIndex, "Ninjutsu")
}

func init() {
	for slug, entry := range puppetupgrades.FoundationEntries {
		if spec, ok := puppetFoundationSubChoiceSpec(entry); ok {
			puppetUpgradeSubChoiceSpecs[slug] = spec
		}
		// Puppet Roles' own "Bonus Skill Proficiency" line (and Drone
		// Weapon's own 4 skills) — the pick itself is a real stored
		// character_companion_upgrades row (see this file's own header
		// comment), so puppetFlatSkillGrants' existing entry-slug lookup
		// already reaches it with no extra plumbing.
		if len(entry.BonusSkills) > 0 {
			puppetFlatSkillGrants[slug] = entry.BonusSkills
		}
	}
	puppetUpgradeSubChoiceSpecs[shadeFrameworkJutsuEntrySlug] = puppetUpgradeSubChoiceSpec{
		Quantity: 3, DynamicOptions: shadeFrameworkJutsuOptions,
	}
	puppetUpgradeSubChoiceSpecs[spellbladeFrameworkJutsuEntrySlug] = puppetUpgradeSubChoiceSpec{
		Quantity: 3, DynamicOptions: spellbladeFrameworkJutsuOptions,
	}
}

// puppetFoundationSubChoiceValues reads back a pick's own already-made
// sub-choices for one kind ("ability", "size", or "weapon"). A non-mixed
// entry's sub-choices carry no prefix at all (there is only ever one kind
// to be ambiguous with), so every recorded choice is returned for that
// entry's own single kind and nothing is returned for the other two.
func puppetFoundationSubChoiceValues(pick puppetFoundationPick, kind string) []string {
	mixed := puppetFoundationMixedChoiceKinds(pick.Entry) > 1
	var out []string
	for _, ch := range pick.SubChoices {
		if !mixed {
			if kind == "ability" && len(pick.Entry.AbilityChoiceGroups) > 0 {
				out = append(out, ch.ChoiceSlug)
			} else if kind == "size" && len(pick.Entry.SizeChoices) > 0 {
				out = append(out, ch.ChoiceSlug)
			} else if kind == "weapon" && pick.Entry.NaturalWeapon != nil && len(pick.Entry.NaturalWeapon.DamageTypeChoices) > 0 {
				out = append(out, ch.ChoiceSlug)
			}
			continue
		}
		prefix := kind + ":"
		if v, ok := strings.CutPrefix(ch.ChoiceSlug, prefix); ok {
			out = append(out, v)
		}
	}
	return out
}

// puppetFoundationAbilityAdjust resolves one pick's AbilitySets/
// AbilityDeltas/AbilityChoiceGroups into a floor-set map and an additive
// delta map — applied in that order (floor, then delta) onto the class's
// own baseline scores. AbilityChoiceGroups are matched POSITIONALLY against
// the pick's own ordered sub-choices (see AbilityChoiceGroup's own doc in
// foundation.go for why that's safe here); a group with no matching pick
// yet (player hasn't chosen it) is simply skipped.
func puppetFoundationAbilityAdjust(pick puppetFoundationPick) (sets, deltas map[string]int) {
	sets = map[string]int{}
	for k, v := range pick.Entry.AbilitySets {
		sets[k] = v
	}
	deltas = map[string]int{}
	for k, v := range pick.Entry.AbilityDeltas {
		deltas[k] += v
	}
	chosen := puppetFoundationSubChoiceValues(pick, "ability")
	for i, g := range pick.Entry.AbilityChoiceGroups {
		if i >= len(chosen) {
			break
		}
		ability := chosen[i]
		switch {
		case g.Set != 0:
			if cur, ok := sets[ability]; !ok || g.Set > cur {
				sets[ability] = g.Set
			}
		case g.Floor != 0:
			if cur, ok := sets[ability]; !ok || g.Floor > cur {
				sets[ability] = g.Floor
			}
			deltas[ability] += g.Delta
		default:
			deltas[ability] += g.Delta
		}
	}
	return sets, deltas
}

// puppetFoundationExpectedAbilityScores resolves every ability score a
// companion's own Foundation pick(s) correct it to — class baseline as the
// floor, a picked ability Set/Floor raising that floor, then every Delta
// added on top — the same "floor, then delta" resolution loadPuppetsTabData
// applies to build its ExpectedStr/Dex/Con/Int/Wis/Cha display hints.
// Factored out so puppetFoundationWeaponAttack (the natural weapon's own
// attack/damage modifier) can use the CORRECTED score directly rather than
// whatever is currently stored on the companion — a Foundation pick's
// ability bonus is real the moment it's picked, whether or not the player
// has since clicked a Sync button to write it into the stored field. nil
// baseline (shouldn't happen for a real puppet, but loadPuppetToolStatBlock
// can return one) yields an empty map, same "nothing to correct" shape.
func puppetFoundationExpectedAbilityScores(baseline *puppetToolStatBlock, foundationPicks []puppetFoundationPick) map[string]int {
	if baseline == nil {
		return nil
	}
	base := map[string]int{
		"str": baseline.Str, "dex": baseline.Dex, "con": baseline.Con,
		"int": baseline.Int, "wis": baseline.Wis, "cha": baseline.Cha,
	}
	sets := map[string]int{}
	deltas := map[string]int{}
	for _, fp := range foundationPicks {
		s, d := puppetFoundationAbilityAdjust(fp)
		for k, v := range s {
			if cur, ok := sets[k]; !ok || v > cur {
				sets[k] = v
			}
		}
		for k, v := range d {
			deltas[k] += v
		}
	}
	out := make(map[string]int, len(base))
	for k, v := range base {
		if s, ok := sets[k]; ok && s > v {
			v = s
		}
		out[k] = v + deltas[k]
	}
	return out
}

// puppetFoundationSize resolves one pick's own size, applying (in order) a
// fixed Size, a player-chosen SizeChoices pick, and then the highest
// SizeAtLevel escalation the character's own Puppet Master level qualifies
// for (Bestial Framework's Large at 10th, Ogre's Huge at 14th).
func puppetFoundationSize(pick puppetFoundationPick, puppetMasterLevel int) string {
	size := pick.Entry.Size
	if len(pick.Entry.SizeChoices) > 0 {
		if chosen := puppetFoundationSubChoiceValues(pick, "size"); len(chosen) > 0 {
			size = chosen[0]
		}
	}
	best := -1
	for lvl, sz := range pick.Entry.SizeAtLevel {
		if puppetMasterLevel >= lvl && lvl > best {
			best, size = lvl, sz
		}
	}
	return size
}

// puppetFoundationWeaponDamageType resolves a natural weapon's own damage
// type — fixed, or the player's own stored choice (Specialized's Slam).
func puppetFoundationWeaponDamageType(pick puppetFoundationPick) string {
	nw := pick.Entry.NaturalWeapon
	if nw == nil {
		return ""
	}
	if nw.DamageType != "" {
		return nw.DamageType
	}
	if chosen := puppetFoundationSubChoiceValues(pick, "weapon"); len(chosen) > 0 {
		return chosen[0]
	}
	return ""
}

// puppetFoundationWeaponAttack computes one Foundation pick's own fixed
// natural weapon as a synthetic attack row — same "never stored, computed
// fresh every render" treatment as puppetIntegratedWeaponAttack, but
// reading the CORRECTED ability score a Foundation pick sets (see
// puppetFoundationExpectedAbilityScores) rather than the character's
// Ninjutsu/Taijutsu modifier, per each entry's own "+ ability Modifier
// [+ your Proficiency Bonus]" text. Deliberately not the companion's own
// stored score: Puppeteer Chassis/Puppet Framework/Puppet Role/Puppet
// Weapon Type ability bonuses apply the moment they're picked, whether or
// not the player has since clicked Sync to write the corrected score into
// the stored field — this is the same reason a raw companionAbilityModifier
// read against the stored field used to under-report the modifier. nil for
// a pick with no natural weapon at all (Bestial/Matryoshka Framework, every
// Role, Sentinel).
func puppetFoundationWeaponAttack(expectedScores map[string]int, ownerProfBonus int, pick puppetFoundationPick) *companionAttackRow {
	nw := pick.Entry.NaturalWeapon
	if nw == nil {
		return nil
	}
	damageType := puppetFoundationWeaponDamageType(pick)
	mod := 0
	if v, ok := expectedScores[nw.DamageAbility]; ok {
		mod = charsheet.AbilityModifier(v)
	}
	bonus := mod
	if nw.AddProficiency {
		bonus += ownerProfBonus
	}
	return &companionAttackRow{
		CompanionAttack: charstore.CompanionAttack{
			Name: nw.Name, Description: nw.CritText,
			DamageCount: nw.DamageCount, DamageSides: nw.DamageSides, DamageType: damageType,
		},
		AttackTotal: mod + ownerProfBonus,
		DamageTotal: bonus,
	}
}

// ghillieCoatingEntrySlug matches puppet_skills.go's own
// puppetFlatSkillGrants map key of the same value — not exported as a
// shared constant there since that map has no other entry needing one; kept
// as a literal here instead of introducing a cross-file dependency for a
// single string.
const ghillieCoatingEntrySlug = "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/ghillie-coating"

// puppetFoundationFixedGrants: the ONE Upgrade entry a Foundation pick
// grants for free, outside the Upgrade cap — same display-only "merge into
// the tier list, no stored row, doesn't count against the cap" treatment
// puppetUpgradeAutoGrants already gives its own 2 entries, just keyed by
// the companion's own CATALOG pick instead of a subclass feature+level.
var puppetFoundationFixedGrants = map[string]string{
	"class/puppet-master/option/puppeteer-chassis/quadrupedal":          puppetupgrades.BulkyBuildEntrySlug,
	"class/puppet-master/option/puppeteer-chassis/warforged":            warfareAugmentationEntrySlug,
	"class/puppet-master/option/puppeteer-chassis/winged":               puppetupgrades.QuickfootedEntrySlug,
	"class/puppet-master/option/puppet-frameworks/shade-framework":      ghillieCoatingEntrySlug,
	"class/puppet-master/option/puppet-frameworks/spellblade-framework": warfareAugmentationEntrySlug,
}

// puppetFoundationSkillAbilityOverride collects every resolved pick's own
// RoleEffect.SkillAbilityOverride into one map — shared by both places that
// call resolvePuppetSkills (the Puppets tab and the companion popup).
func puppetFoundationSkillAbilityOverride(foundationPicks []puppetFoundationPick) map[string]string {
	override := map[string]string{}
	for _, fp := range foundationPicks {
		if fp.Entry.RoleEffect == nil {
			continue
		}
		for skill, ability := range fp.Entry.RoleEffect.SkillAbilityOverride {
			override[skill] = ability
		}
	}
	return override
}

// puppetFoundationBonusTierCapsFromPicks: a genuine material-tier capacity
// increase rather than a fixed entry — Specialized's own "2 free Wood tier
// Upgrades of your choice." Added on top of the class table's own printed
// cap (puppetUpgradeMaterialTierCaps) — same array shape as that function's
// own return, indexed by puppetUpgradeTierRanks, so a caller can just add
// the two arrays together element-wise.
func puppetFoundationBonusTierCapsFromPicks(foundationPicks []puppetFoundationPick) [puppetUpgradeTierCount + 1]int {
	var bonus [puppetUpgradeTierCount + 1]int
	for _, p := range foundationPicks {
		for tierName, n := range p.Entry.BonusTierSlots {
			if rank, ok := puppetUpgradeTierRanks[tierName]; ok {
				bonus[rank] += n
			}
		}
	}
	return bonus
}

// puppetFoundationBonusTierCaps is puppetFoundationBonusTierCapsFromPicks
// for a caller (handlePuppetUpgradeAdd) that doesn't already have the
// companion's picks/choices loaded — loadPuppetUpgradeTiers loads both
// already for its own tier-rendering, so it calls the Picks-taking version
// directly instead of re-querying.
func (s *server) puppetFoundationBonusTierCaps(characterID, companionID int64) ([puppetUpgradeTierCount + 1]int, error) {
	picks, err := charstore.ListCompanionUpgrades(s.charDB, characterID, companionID)
	if err != nil {
		return [puppetUpgradeTierCount + 1]int{}, err
	}
	choices, err := charstore.ListCompanionUpgradeChoices(s.charDB, characterID, companionID)
	if err != nil {
		return [puppetUpgradeTierCount + 1]int{}, err
	}
	return puppetFoundationBonusTierCapsFromPicks(resolvePuppetFoundationPicks(picks, choices)), nil
}
