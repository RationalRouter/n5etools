package main

import (
	"database/sql"

	"github.com/sergio/n5e/internal/charstore"
)

// wow_weapons.go: turns an Elemental Innovationist's W.O.W (Weapons of
// Wonder) pick into a real, equipped weapon — the mechanical half of
// formatWoWDescription's own purely cosmetic base/Ascension split
// (format.go). class_options has no structured damage/property columns (it
// is pure prose — see that catalog's own rows), so wowWeaponSpecs is a
// hand-curated table, the same "no structured source data, so hand-curate
// it once" precedent scienceNinRegaliaOptions (science_nin_subclasses.go)
// already sets.
//
// Scope, deliberately bounded to match titanWeaponUpgradeAttacks' own
// "standing attack only" boundary (titan.go): each spec models the W.o.W's
// own printed base attack line (and, where the book gives a single,
// unconditional Ascension replacement for it, the Ascended line too) — not
// every triggered/resource-spending special ability a W.o.W's full prose
// describes (Charged Shot, Blast Away, the Terra Spikes barrier, Primeval
// Bow's own per-Awakening branch effects, ...). Those stay exactly what
// they already are after formatWoWDescription: read from the popup's own
// tooltip and the Known-pick detail card, not modeled as separate
// calculator logic. Three of the ten (Cymbal Grenades, Terra Spikes,
// Sinister Skull) have no unconditional "deals X on a hit" line at all —
// their own use is entirely the save-based special ability prose, so their
// spec leaves DamageDice empty, same "damage roll omitted" shape Flash Tag
// (equipment, rules.db) already ships with for the identical reason. A
// W.o.W whose Ascension branches into a player choice with no picker of its
// own (Blunderbuss's 3 properties, Ray Gun's Industrial/Otherworldly split,
// Primeval Bow's own 4 Awakenings) is left without an AscendedDamageDice/
// Type override entirely, rather than guessing which branch to apply —
// unless the book also gives a single, unconditional numeric replacement
// that applies regardless of which branch is picked, which gets modeled
// even though the branch itself doesn't (Alchemical Staff, Ghost of
// Konoha, Thunder Cannon, and Primeval Bow's own unconditional 2d10 bump
// ahead of its Awakening choice).
type wowWeaponSpec struct {
	Name       string
	DamageDice string
	DamageType string
	Properties string

	// AbilityOverride, when set, unconditionally replaces buildAttacks' own
	// Finesse/Range/Ammunition-derived ability pick — the same "always-on
	// once equipped, still overridable by a manual per-weapon Adjust"
	// treatment Cooking Tool Infusion and S.E.N.Ts already get for an
	// identically-worded "you may use X in place of Y" clause. Only
	// Draconic Gauntlet's own unconditional "you may calculate your Unarmed
	// attack and damage rolls with Intelligence instead of Strength"
	// qualifies — Ghost of Konoha's, Alchemical Staff's, and Primeval Bow's
	// own INT clauses are about casting a Jutsu (Bukijutsu) with the weapon
	// as a component, not the weapon's own attack roll, so they are left
	// alone here.
	AbilityOverride string

	// NoDamageAbilityMod mirrors Ray Gun's own unconditional "You do not add
	// your ability modifier to the damage dealt with the Raygun" — applied
	// as a default, same "still overridable" boundary as AbilityOverride.
	NoDamageAbilityMod bool

	// AscendedDamageDice/AscendedDamageType, when set, replace DamageDice/
	// DamageType once this specific W.o.W is the character's designated
	// Ascended W.o.W (see wowAscendedOverride) — "" leaves the corresponding
	// base value untouched.
	AscendedDamageDice string
	AscendedDamageType string
}

// wowWeaponSpecs is keyed by the W.o.W's own class_options slug (the same
// value AddScienceNinSubclassPick stores as option_slug, and
// ascendedWowSection's own AvailableDesignatedWoW options carry) —
// hand-verified against dist/rules.db's W.O.W (Weapons of Wonder) list_name
// rows in full, not a sample. Primeval Bow is a 10th entry recovered from a
// PDF-column-bleed bug that had scattered its text across Terra Spikes' and
// Ray Gun's own rows (see internal/schema/migrations/rules/0066) — it now
// has its own class_options slug to key a spec off of.
var wowWeaponSpecs = map[string]wowWeaponSpec{
	"class/science-nin/option/w-o-w-weapons-of-wonder/alchemical-staff": {
		Name: "Alchemical Staff", DamageDice: "4d8", DamageType: "Elemental",
		Properties:         "Two-Handed, Range (60/120), Ammo",
		AscendedDamageDice: "5d12",
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/blunderbuss": {
		Name: "Blunderbuss", DamageDice: "5d8", DamageType: "Piercing",
		Properties: "Critical, Two-Handed, Heavy, Range (15/45)",
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/cymbal-grenades": {
		Name: "Cymbal Grenades",
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/draconic-gauntlet": {
		Name: "Draconic Gauntlet", DamageDice: "1d6", DamageType: "Force",
		Properties:      "Deadly, Reach, Trip, Unarmed",
		AbilityOverride: "int",
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/ghost-of-konoha": {
		Name: "Ghost of Konoha", DamageDice: "2d6", DamageType: "Slashing",
		Properties:         "Critical, Deadly, Finesse, Versatile (2d8)",
		AscendedDamageDice: "2d8",
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/terra-spikes": {
		Name: "Terra Spikes",
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/primeval-bow": {
		Name: "Primeval Bow", DamageDice: "1d10", DamageType: "Piercing/Fire",
		Properties:         "Range (200/500), Tactical, Two-Handed",
		AscendedDamageDice: "2d10",
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/ray-gun": {
		Name: "Ray Gun", DamageDice: "3d4", DamageType: "Force",
		Properties:         "Light, Range (30/60)",
		NoDamageAbilityMod: true,
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/sinister-skull": {
		Name: "Sinister Skull",
	},
	"class/science-nin/option/w-o-w-weapons-of-wonder/thunder-cannon": {
		Name: "Thunder Cannon", DamageDice: "3d10", DamageType: "Wind",
		Properties:         "Heavy, Two-Handed",
		AscendedDamageDice: "3d12",
	},
}

// wowWeaponSpecsByName mirrors wowWeaponSpecs, keyed by the weapon's own
// display Name instead of its catalog slug — buildAttacks only has the
// granted custom_items row's Name to key off of (see wowCustomItemKind),
// not the class_options slug that granted it.
var wowWeaponSpecsByName = func() map[string]wowWeaponSpec {
	m := make(map[string]wowWeaponSpec, len(wowWeaponSpecs))
	for _, spec := range wowWeaponSpecs {
		m[spec.Name] = spec
	}
	return m
}()

// wowCustomItemKind marks a custom_items row as one grantWoWWeapon minted —
// both the free-text "Type" shown in Inventory and, since custom_items is a
// shared library with no character_id of its own, the lookup key
// revokeWoWWeapon uses (alongside the character's own inventory rows) to
// find a specific character's granted W.o.W again. buildAttacks checks it
// too, to show the Weapons-table "Weapon of Wonder" source badge.
const wowCustomItemKind = "Weapon of Wonder"

// grantWoWWeapon forges wowSlug's own weapon into characterID's equipped
// Inventory — a real custom_items row (so it renders, edits, and rolls
// exactly like any other library weapon; see items.go's handleItemDetail)
// plus the character_inventory row that equips it. wowSlug with no curated
// spec (a rules update adding a 10th W.o.W before this table catches up)
// still grants a bare, statless weapon under its own catalog name rather
// than failing outright — the player can fill in numbers by hand via the
// same "Edit this item" UI every other custom item already has.
func (s *server) grantWoWWeapon(characterID int64, wowSlug, wowName, wowDescription string) error {
	spec, ok := wowWeaponSpecs[wowSlug]
	if !ok {
		spec = wowWeaponSpec{Name: wowName}
	}
	item, err := charstore.AddCustomItem(s.charDB, charstore.CustomItem{
		Name:         spec.Name,
		Kind:         wowCustomItemKind,
		RollableKind: "weapon",
		DamageDice:   spec.DamageDice,
		DamageType:   spec.DamageType,
		Properties:   spec.Properties,
		// The item library's generic /items/{slug} detail page (items.go's
		// handleItemDetail) runs this through the ordinary formatDescription,
		// not formatWoWDescription's own base/Ascension split — a plainer
		// rendering than the Known-pick popup card gets (see
		// renderScienceNinSubclassPickDetailWoW), but still readable rather
		// than the blank description every other freshly-created custom item
		// starts with.
		Description: wowDescription,
	})
	if err != nil {
		return err
	}
	return charstore.AddInventoryItemWithEquipped(s.charDB, characterID, item.Slug, 1, true)
}

// revokeWoWWeapon removes every equipped inventory row this character holds
// for the named W.o.W — called when the pick itself is forgotten, so the
// weapon doesn't linger in Inventory pointing at a subclass pick that no
// longer exists. Leaves the underlying custom_items library row itself in
// place, same as every other inventory deletion in this codebase
// (handleSheetInventoryDelete, DeleteInventoryItem's own doc) — an orphaned
// library entry is the accepted cost of never needing a second, harder
// "is anyone else still using this row" check.
func (s *server) revokeWoWWeapon(characterID int64, wowName string) error {
	rows, err := s.charDB.Query(`
		SELECT inv.id FROM character_inventory inv
		JOIN custom_items ci ON ci.slug = inv.item_slug
		WHERE inv.character_id = ? AND ci.kind = ? AND ci.name = ?`,
		characterID, wowCustomItemKind, wowName)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for _, id := range ids {
		if err := charstore.DeleteInventoryItem(s.charDB, characterID, id); err != nil {
			return err
		}
	}
	return nil
}

// ensureWoWWeaponsGranted backfills a granted weapon for every Known W.o.W
// pick that doesn't already have one — both the migration path for a pick
// made before this mechanism existed (grantWoWWeapon only ever runs from
// addWoWPick going forward, so any character who picked a W.o.W before this
// feature shipped has a pick with nothing behind it) and a safety net if a
// grant ever silently failed. Idempotent per pick, same "check first, no-op
// if already there" boundary ensureAirTrecksGranted (science_nin.go) already
// draws for its own single fixed weapon — call this from
// ensureScienceNinAutoGrants, before a request's own inventory load and
// buildAttacks call, so a freshly-backfilled weapon is visible the same
// request that discovers it's missing.
func (s *server) ensureWoWWeaponsGranted(characterID int64) error {
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickWOW)
	if err != nil || len(picks) == 0 {
		return err
	}
	for _, p := range picks {
		name, description, err := s.scienceNinOptionNameAndDescription(p.OptionSlug)
		if err != nil {
			return err
		}
		if name == "" {
			continue // stale slug after a rules update — nothing to backfill against
		}
		var exists bool
		if err := s.charDB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM character_inventory inv
				JOIN custom_items ci ON ci.slug = inv.item_slug
				WHERE inv.character_id = ? AND ci.kind = ? AND ci.name = ?)`,
			characterID, wowCustomItemKind, name,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := s.grantWoWWeapon(characterID, p.OptionSlug, name, description); err != nil {
			return err
		}
	}
	return nil
}

// wowAscendedOverride reports the current Ascended W.o.W designee's own
// display Name plus the damage dice/type its Ascension swaps in — "" name
// means no designee, or that designee's own spec has nothing to swap (see
// wowWeaponSpec's own AscendedDamageDice doc for why most don't). buildAttacks
// calls this once per request and compares its own Name against each
// granted W.o.W row's Name, the same "resolve once, match by name in the
// loop" shape sentAttackOverrides/cookingToolInfusionAttackOverrides already
// use for their own single always-relevant slug.
func (s *server) wowAscendedOverride(characterID int64) (name, dice, dtype string, err error) {
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickAscendedWoW)
	if err != nil || len(picks) == 0 {
		return "", "", "", err
	}
	spec, ok := wowWeaponSpecs[picks[0].OptionSlug]
	if !ok || spec.AscendedDamageDice == "" {
		return "", "", "", nil
	}
	return spec.Name, spec.AscendedDamageDice, spec.AscendedDamageType, nil
}

// scienceNinOptionName looks up one class_options row's own printed Name by
// slug — removeWoWPick's own "what did the player just forget" lookup, same
// inline query classOptionNamesBySlug (puppets.go) already runs for the
// identical purpose against a different picker.
func (s *server) scienceNinOptionName(slug string) (string, error) {
	var name string
	err := s.rulesDB.QueryRow(`SELECT name FROM class_options WHERE slug = ?`, slug).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return name, err
}

// scienceNinOptionNameAndDescription is scienceNinOptionName's own twin,
// additionally returning the raw catalog description — addWoWPick's own
// "what did the player just pick" lookup, since grantWoWWeapon wants both.
func (s *server) scienceNinOptionNameAndDescription(slug string) (name, description string, err error) {
	err = s.rulesDB.QueryRow(`SELECT name, description FROM class_options WHERE slug = ?`, slug).Scan(&name, &description)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return name, description, err
}
