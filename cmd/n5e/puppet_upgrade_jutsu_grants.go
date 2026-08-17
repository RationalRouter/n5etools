package main

import (
	"github.com/sergio/n5e/internal/charstore"
)

// Three Puppet Master upgrades each print a small table granting specific
// named jutsu at set class levels — Elemental Reactor (keyed by the
// element chosen), Weaponized Jutsu Casting (keyed by the companion's own
// Puppet Weapon Type — Drone/Ogre/Sentinel — which is a separate,
// already-existing pick, not a sub-choice of this upgrade itself), and
// Entangling Threads (a single fixed list, no key). See migration
// 0033_puppet_upgrade_jutsu_grants.sql for the hand-transcribed data and
// why it had to be hand-transcribed (same PDF table/image data loss as
// Armor Chassis).
//
// These jutsu belong to the character's own known-jutsu pool, the same
// "computed, not stored" shape as jutsu_grants.go's class/clan feature
// grants (e.g. Chakra Hands) — reusing that file's SourceLabel convention
// so jutsuKnownCount already excludes them from the known-jutsu cap for
// free, with no changes needed there.
var puppetWeaponTypeVariants = map[string]string{
	"class/puppet-master/option/puppet-weapon-types/drone-weapon":    "drone-weapon",
	"class/puppet-master/option/puppet-weapon-types/ogre-weapon":     "ogre-weapon",
	"class/puppet-master/option/puppet-weapon-types/sentinel-weapon": "sentinel-weapon",
}

// tableGrantUpgradeEntrySlugs is every upgrade entry puppet_upgrade_jutsu_grants
// has rows for — the set this file's queries scope against.
var tableGrantUpgradeEntrySlugs = []string{
	elementalReactorEntrySlug,
	"class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting",
	"class/puppet-master/option/interwoven-upgrades/wood-tier/entry/entangling-threads",
}

// companionUpgradeGrantVariant resolves the "variant" key
// puppet_upgrade_jutsu_grants needs for one companion's pick of a
// table-granting upgrade: Elemental Reactor's own sub-choice (the element),
// Weaponized Jutsu Casting's companion-wide Puppet Weapon Type pick, or ""
// for Entangling Threads (no variant — NULL in the table).
func (s *server) companionUpgradeGrantVariant(characterID, companionID int64, upgradeEntrySlug string, choices []charstore.CompanionUpgradeChoice, pickID int64) (string, error) {
	switch upgradeEntrySlug {
	case elementalReactorEntrySlug:
		for _, ch := range choices {
			if ch.CompanionUpgradeID == pickID {
				return ch.ChoiceSlug, nil
			}
		}
		return "", nil // no element chosen yet — nothing to grant
	case "class/puppet-master/option/upgrades-of-war/wood-tier/entry/weaponized-jutsu-casting":
		upgrades, err := charstore.ListCompanionUpgrades(s.charDB, characterID, companionID)
		if err != nil {
			return "", err
		}
		for _, u := range upgrades {
			if variant, ok := puppetWeaponTypeVariants[u.UpgradeEntrySlug]; ok {
				return variant, nil
			}
		}
		return "", nil // no Puppet Weapon Type chosen yet — nothing to grant
	default:
		return "", nil // Entangling Threads and any future no-variant entry
	}
}

// puppetUpgradeTableGrantedJutsu finds every jutsu the character's own
// companions' table-granting upgrade picks unlock at classLevel or lower,
// returning slug -> a short badge label matching loadGrantedJutsuLabels'
// own convention. Deliberately de-duplicates across multiple companions
// (e.g. two puppets both with Elemental Reactor: Fire) by jutsu slug, since
// known-jutsu is one character-wide pool, not per-companion.
func (s *server) puppetUpgradeTableGrantedJutsu(characterID int64, classLevel int) (map[string]string, error) {
	companions, err := charstore.ListCompanions(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{}
	for _, comp := range companions {
		if comp.Kind != "puppet" {
			continue
		}
		upgrades, err := charstore.ListCompanionUpgrades(s.charDB, characterID, comp.ID)
		if err != nil {
			return nil, err
		}
		var relevant []charstore.CompanionUpgrade
		for _, u := range upgrades {
			for _, slug := range tableGrantUpgradeEntrySlugs {
				if u.UpgradeEntrySlug == slug {
					relevant = append(relevant, u)
					break
				}
			}
		}
		if len(relevant) == 0 {
			continue
		}
		choices, err := charstore.ListCompanionUpgradeChoices(s.charDB, characterID, comp.ID)
		if err != nil {
			return nil, err
		}
		for _, u := range relevant {
			variant, err := s.companionUpgradeGrantVariant(characterID, comp.ID, u.UpgradeEntrySlug, choices, u.ID)
			if err != nil {
				return nil, err
			}
			if u.UpgradeEntrySlug != "class/puppet-master/option/interwoven-upgrades/wood-tier/entry/entangling-threads" && variant == "" {
				continue // sub-choice/weapon-type not picked yet — nothing to grant
			}
			grants, err := s.loadPuppetUpgradeJutsuGrants(u.UpgradeEntrySlug, variant, classLevel)
			if err != nil {
				return nil, err
			}
			for _, slug := range grants {
				if _, exists := labels[slug]; !exists {
					labels[slug] = "Puppet Upgrade"
				}
			}
		}
	}
	return labels, nil
}

// loadPuppetUpgradeJutsuGrants queries the hand-transcribed grant table
// directly — variant "" matches the NULL-variant rows (Entangling Threads),
// via SQLite's own "IS ?" NULL-safe comparison rather than a separate query
// shape per case.
func (s *server) loadPuppetUpgradeJutsuGrants(upgradeEntrySlug, variant string, classLevel int) ([]string, error) {
	var variantArg any
	if variant != "" {
		variantArg = variant
	}
	rows, err := s.rulesDB.Query(`
		SELECT jutsu_slug FROM puppet_upgrade_jutsu_grants
		WHERE upgrade_entry_slug = ? AND variant IS ? AND level <= ?`,
		upgradeEntrySlug, variantArg, classLevel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}
