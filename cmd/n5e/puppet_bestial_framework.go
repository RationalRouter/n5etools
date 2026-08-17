package main

import (
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// This file wires Green Technique's own Bestial Framework catalog pick
// ("Select one Sage Creature from the Summoning Technique... Gains 1 D-Rank
// trait from the chosen Sage Creature at 2nd level, a 2nd at 10th, the final
// at 17th... Gains one Natural Weapon from the chosen Sage Creature") into
// the existing Summoning Technique catalog (summon_tribes/
// summon_tribe_features/summon_tribe_attacks, see internal/schema/
// migrations/rules/0008_summon_tribes.sql) — a picker + reference-text job
// reusing an already-ingested catalog, not a new subsystem.
//
// Bestial Framework's own sub-choice sequence is 6 slots, not the 2 the
// generic AbilityChoiceGroups auto-builder (puppetFoundationSubChoiceSpec)
// alone would produce: slot 0/1 are the entry's own ability picks (+2, +1),
// slot 2 is the Sage Creature tribe itself, slots 3-5 are its 3 level-gated
// D-Rank traits. All 6 share ONE character_companion_upgrades row (the
// single Bestial Framework pick), same as every other Foundation entry's
// sub-choices — see puppetFoundationSubChoiceSpec's own doc for why a
// single spec, not per-kind specs, is this app's existing shape for an
// entry offering more than one kind of choice.
//
// Each option's own slug carries one of 3 kind prefixes — "ability:" for
// slots 0/1 (puppetFoundationSubChoiceValues' "ability" kind reader, used by
// every Foundation entry's own ability-score computation, expects exactly
// this prefix), "tribe:" for slot 2, "trait:" for slots 3-5. Because slots
// 0/1 share "ability:" and slots 3-5 share "trait:", handlePuppetUpgradeChoiceAdd's
// own "one pick per kind" guard (written for Specialized/Sentinel, where
// every kind really is capped at 1 pick) would otherwise block Bestial
// Framework's own SECOND ability pick and 2nd/3rd trait picks — see
// puppetFoundationRepeatableSubChoiceKinds below, which is the one place
// that guard is relaxed for a kind needing more than one pick.
const bestialFrameworkEntrySlug = "class/puppet-master/option/puppet-frameworks/bestial-framework"

// bestialFrameworkTraitMinLevelBySlot: pick index 3/4/5 (D-Rank trait 1/2/3)
// each gate at the Puppet Master level the entry's own text names ("at 2nd
// level, a 2nd at 10th level, and the final at 17th level").
var bestialFrameworkTraitMinLevelBySlot = [3]int{2, 10, 17}

// puppetFoundationRepeatableSubChoiceKinds: kinds whose "already chosen for
// this upgrade" collision check (handlePuppetUpgradeChoiceAdd) doesn't
// apply — see this file's own header comment for why Bestial Framework's
// "ability" (2 picks) and "trait" (3 picks) kinds both need it relaxed.
var puppetFoundationRepeatableSubChoiceKinds = map[string]map[string]bool{
	bestialFrameworkEntrySlug: {"ability": true, "trait": true},
}

// bestialFrameworkSubChoiceOptions is Bestial Framework's own DynamicOptions
// implementation, covering all 6 of its sub-choice slots (see this file's
// header comment for why one entry needs one combined function rather than
// the generic per-field auto-builder).
func bestialFrameworkSubChoiceOptions(s *server, characterID, companionID int64, pickIndex int) ([]puppetUpgradeSubChoiceOption, error) {
	switch pickIndex {
	case 0, 1:
		return puppetFoundationAbilityOptions(charsheet.Abilities, "ability:"), nil
	case 2:
		rows, err := s.rulesDB.Query(`SELECT slug, name, COALESCE(description, '') FROM summon_tribes ORDER BY name`)
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
			o.Slug = "tribe:" + o.Slug
			out = append(out, o)
		}
		return out, rows.Err()
	case 3, 4, 5:
		slot := pickIndex - 3
		level, err := s.puppetMasterClassLevel(characterID)
		if err != nil {
			return nil, err
		}
		if level < bestialFrameworkTraitMinLevelBySlot[slot] {
			return nil, nil
		}
		tribeSlug, err := s.bestialFrameworkTribeSlug(characterID, companionID)
		if err != nil {
			return nil, err
		}
		if tribeSlug == "" {
			return nil, nil // no Sage Creature chosen yet — nothing to offer
		}
		rows, err := s.rulesDB.Query(
			`SELECT slug, name, description FROM summon_tribe_features
			 WHERE tribe_slug = ? AND rank = 'D' ORDER BY sort_order, name`, tribeSlug)
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
			o.Slug = "trait:" + o.Slug
			out = append(out, o)
		}
		return out, rows.Err()
	default:
		return nil, nil
	}
}

// bestialFrameworkTribeSlug reads back THIS companion's own already-chosen
// Sage Creature tribe (slot 2's "tribe:<slug>" sub-choice), or "" if none
// has been picked yet. Queried fresh (not threaded through
// puppetFoundationPick) since DynamicOptions only carries characterID/
// companionID/pickIndex, not the caller's already-loaded picks.
func (s *server) bestialFrameworkTribeSlug(characterID, companionID int64) (string, error) {
	upgrades, err := charstore.ListCompanionUpgrades(s.charDB, characterID, companionID)
	if err != nil {
		return "", err
	}
	var upgradeID int64
	found := false
	for _, u := range upgrades {
		if u.UpgradeEntrySlug == bestialFrameworkEntrySlug {
			upgradeID, found = u.ID, true
			break
		}
	}
	if !found {
		return "", nil
	}
	choices, err := charstore.ListCompanionUpgradeChoices(s.charDB, characterID, companionID)
	if err != nil {
		return "", err
	}
	for _, ch := range choices {
		if ch.CompanionUpgradeID != upgradeID {
			continue
		}
		if slug, ok := strings.CutPrefix(ch.ChoiceSlug, "tribe:"); ok {
			return slug, nil
		}
	}
	return "", nil
}

// bestialFrameworkReferenceTraits resolves one companion's own Bestial
// Framework pick into reference text — the chosen Sage Creature's own
// Natural Weapon(s) (summon_tribe_attacks has no structured dice columns at
// all, only printed prose like "Str + Prof to hit, +Str Slashing Damage.",
// so unlike every other Foundation entry's NaturalWeapon this can't compute
// a real attack row) and its chosen D-Rank traits (resolved to their own
// name/description, not just the stored slug). Folded into the SAME
// FoundationTraits list every other entry's ReferenceTraits/RoleEffect text
// already renders through — no new template block needed.
func (s *server) bestialFrameworkReferenceTraits(fp puppetFoundationPick) ([]string, error) {
	var tribeSlug string
	var traitSlugs []string
	for _, ch := range fp.SubChoices {
		if slug, ok := strings.CutPrefix(ch.ChoiceSlug, "tribe:"); ok {
			tribeSlug = slug
		} else if slug, ok := strings.CutPrefix(ch.ChoiceSlug, "trait:"); ok {
			traitSlugs = append(traitSlugs, slug)
		}
	}
	if tribeSlug == "" {
		return []string{"Sage Creature: not yet chosen."}, nil
	}

	var tribeName string
	if err := s.rulesDB.QueryRow(`SELECT name FROM summon_tribes WHERE slug = ?`, tribeSlug).Scan(&tribeName); err != nil {
		return nil, err
	}
	out := []string{"Sage Creature: " + tribeName}

	attackRows, err := s.rulesDB.Query(
		`SELECT name, description FROM summon_tribe_attacks WHERE tribe_slug = ? ORDER BY name`, tribeSlug)
	if err != nil {
		return nil, err
	}
	for attackRows.Next() {
		var name, desc string
		if err := attackRows.Scan(&name, &desc); err != nil {
			attackRows.Close()
			return nil, err
		}
		out = append(out, "Natural Weapon — "+name+": "+desc)
	}
	attackRows.Close()
	if err := attackRows.Err(); err != nil {
		return nil, err
	}

	for _, slug := range traitSlugs {
		var name, desc string
		err := s.rulesDB.QueryRow(`SELECT name, description FROM summon_tribe_features WHERE slug = ?`, slug).Scan(&name, &desc)
		if err != nil {
			continue // a stale/removed trait slug just isn't shown
		}
		out = append(out, "D-Rank Trait — "+name+": "+desc)
	}
	return out, nil
}

func init() {
	// Overrides the generic auto-builder's own ability-only spec (built by
	// puppet_foundation.go's init(), which runs first — both are plain
	// sequential statements within package init, not concurrent) with the
	// full 6-slot version.
	puppetUpgradeSubChoiceSpecs[bestialFrameworkEntrySlug] = puppetUpgradeSubChoiceSpec{
		Quantity: 6, DynamicOptions: bestialFrameworkSubChoiceOptions,
	}
}
