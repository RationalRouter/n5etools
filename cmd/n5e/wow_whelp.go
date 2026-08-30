package main

import (
	"database/sql"
	"log"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// scienceNinDraconicGauntletWoWSlug is the Draconic Gauntlet W.o.W's own
// class_options slug — the same key wowWeaponSpecs (wow_weapons.go) uses.
// ASCENSION: DRACONIC GAUNTLET reads: "If the Draconic Gauntlet is Ascended,
// you use your element to create and give life to a small Whelp, a tiny
// youngling dragon" — a fielded, statted-out entity, the same "the book
// describes a mechanical companion, not a passive/self-targeted clause"
// shape Angel E.I.P's own Spectre already gets a companion for (see
// scienceNinAngelEIPSlug's own doc, science_nin_subclasses.go). Unlike
// Angel E.I.P, becoming Ascended isn't itself a pick with its own slug —
// it's a designation (charstore.ScienceNinPickAscendedWoW) that can point at
// ANY known W.o.W, so the auto-add/auto-remove hook lives in
// syncDraconicGauntletWhelp below, called after every mutation of that
// designation, rather than inline in one add/remove pair the way Angel
// E.I.P's is.
const scienceNinDraconicGauntletWoWSlug = "class/science-nin/option/w-o-w-weapons-of-wonder/draconic-gauntlet"

// scienceNinWhelpCompanionName is the exact display name ensureWhelpCompanion
// gives the companion it auto-adds — "Whelp" is the book's own name for the
// entity throughout its stat block ("you use your element to create and
// give life to a small Whelp"; "The Whelp rests perched on your Draconic
// Gauntlet"), with the same "(<source>)" suffix scienceNinAngelEIPCompanionName
// uses so a kind="custom" companion (which otherwise carries no back-
// reference to whatever created it) still tells the player at a glance
// which pick produced it, and so removeWhelpCompanion/syncDraconicGauntletWhelp
// can find it again by exact name.
const scienceNinWhelpCompanionName = "Whelp (Draconic Gauntlet)"

// ensureWhelpCompanion auto-adds the Draconic Gauntlet's own Whelp as a
// kind="custom" companion — called by syncDraconicGauntletWhelp the moment
// Draconic Gauntlet becomes the character's designated Ascended W.o.W.
// Reuses kind="custom" (cmd/n5e/companions.go's kind whitelist) rather than
// inventing a new companion kind with its own migration/prefill/template
// plumbing, the same "one-off, no ongoing formula behind its OWN growth"
// precedent scienceNinAngelEIPSlug's own Spectre companion already sets —
// unlike a Puppet Tool/Nin-Dog/Titan, a Whelp has no upgrade tree or
// level-scaling stat block of its own to auto-calculate against on every
// render. AC/HP ARE one-time formulas against the OWNER's own current stats
// (18 + their Ninjutsu ability modifier; their Intelligence modifier times
// their Science-Nin level) rather than fixed constants the way Spectre's 10
// HP is, but they're written ONCE here, at creation time, and are ordinary
// player-editable fields afterward (kind="custom" renders every stat as an
// editable input) — not recomputed on every render the way a Titan's own AC
// is. A player whose Intelligence or Science-Nin level later changes can
// correct the Whelp's card by hand, the same "trust the player" boundary
// every other one-time companion prefill in this codebase already draws
// (prefillPuppetStatDefaults's own baseline included, before a Sync button
// existed for it).
//
// The book's own stat block also lists a flat Damage Resistance-adjacent
// "DR 5" (initially misread as a bare "DR" — see this file's own regression
// test / the rules.db ingestion note below) and a "Treat negative modifiers
// as +0" clause on its saving throws — neither concept exists anywhere else
// in this app's companion model (every companion's AC is a single number,
// and no companion save-proficiency ever floors a negative ability modifier
// at +0), so both stay narrated in the companion-reference block
// (companion_fields.html's own "Whelp (Draconic Gauntlet)" name-matched
// section) rather than becoming new mechanical fields for a single, rarely
// -used stat no other companion in the game needs — the same "narrate the
// one-off, model the standing attack" boundary wow_weapons.go's own header
// doc already draws for a W.o.W's own conditional/triggered effects.
//
// Guarded by scienceNinWhelpCompanionName's own exact-name match against the
// character's existing companions, so re-designating Draconic Gauntlet as
// Ascended after it was forgotten and re-picked never creates a second,
// duplicate Whelp.
func (s *server) ensureWhelpCompanion(characterID int64) error {
	existing, err := charstore.ListCompanions(s.charDB, characterID)
	if err != nil {
		return err
	}
	for _, c := range existing {
		if c.Name == scienceNinWhelpCompanionName {
			return nil
		}
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		return err
	}
	scienceNinLevel, err := s.scienceNinClassLevel(characterID)
	if err != nil {
		return err
	}
	intMod := sheet.Abilities["int"].Modifier
	ac := int64(18 + ninjutsuAbilityModifier(sheet))
	hpMax := int64(intMod * scienceNinLevel)

	companionID, err := charstore.AddCompanion(s.charDB, characterID, "custom", scienceNinWhelpCompanionName)
	if err != nil {
		return err
	}
	if err := charstore.SetCompanionStatDefaults(s.charDB, characterID, companionID,
		ac, hpMax, 60, sql.NullInt64{Int64: 60, Valid: true},
		18, 20, 16, 20, 5, 5, "Small",
	); err != nil {
		return err
	}
	if err := charstore.SetCompanionHP(s.charDB, characterID, companionID, sql.NullInt64{Int64: hpMax, Valid: true}); err != nil {
		return err
	}
	if err := charstore.SetCompanionSaveProficiencies(s.charDB, characterID, companionID, "str,dex,con,int,wis,cha"); err != nil {
		return err
	}
	if err := charstore.SetCompanionFields(s.charDB, characterID, companionID,
		scienceNinWhelpCompanionName, "",
		"", "", "",
		"", false, "", "",
		"Psychic, Force", "", "All Sensory",
		false,
	); err != nil {
		return err
	}

	attacks := []charstore.CompanionAttack{
		{
			Name: "Aether Fireball", AttackAbility: "", AttackProf: "none", AttackBonus: ninjutsuAttackBonus(sheet),
			Description:   "Ranged Weapon Attack: Range 90 feet, one target. On a roll of 17-20, the target gains 1 rank of Burned.",
			DamageCount:   4, DamageSides: 10, DamageAbility: "", DamageBonus: 4, DamageType: "fire",
		},
		{
			Name: "Aether Breath", NoAttackRoll: true,
			Description: "Cost: 10 Chakra. The Whelp breathes a large cone of alchemical fire — Area Weapon Attack: Dexterity saving throw, reach 15-foot cone, all targets. Success: half damage, no effects. Failure: this damage, and Burned. May pay increments of 10 chakra to increase the damage by 2d6+2 and the cone's area by +10ft, up to 40 chakra total.",
			DamageCount: 5, DamageSides: 6, DamageAbility: "", DamageBonus: 5, DamageType: "fire",
		},
		{
			Name: "Aether Meteor", NoAttackRoll: true,
			Description: "Cost: 25 Chakra. Recharge 5-6. The Whelp breathes a draconic sigil of flame on the ground at a space within 120 feet. Large fireballs of alchemical flame begin to manifest in the sky above, treating the area as being targeted by the Fire Storm Fire Ninjutsu at A-Rank.",
		},
	}
	for _, a := range attacks {
		if _, err := charstore.AddCompanionAttack(s.charDB, characterID, companionID, a); err != nil {
			return err
		}
	}
	return nil
}

// ninjutsuAttackBonus reads the owner's already-resolved Ninjutsu attack
// bonus (ability modifier + full proficiency, plus any Expertise/override
// already folded in by charsheet.Compute) — the same "trust the player's own
// resolved numbers" precedent titanNinjutsuAttackStats (titan.go) documents
// for an identical need. Returns 0 for a sheet with no Ninjutsu discipline at
// all; in practice a Draconic Gauntlet owner always has one (Science-Nin
// always grants Ninjutsu).
func ninjutsuAttackBonus(sheet *charsheet.Sheet) int {
	if sheet == nil {
		return 0
	}
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Ninjutsu" {
			return a.Modifier
		}
	}
	return 0
}

// whelpReference is the read-only/interactive panel for the Draconic
// Gauntlet's own Whelp companion (kind="custom", matched by exact Name —
// see scienceNinWhelpCompanionName) — brings AC/Max HP onto the same
// "computed hint, never silently overwritten, Sync-pinnable" treatment
// every other formula-backed companion kind already gets (titanReference/
// ninDogReference/snbReference), replacing the one-time creation-only
// prefill ensureWhelpCompanion used to leave this companion stuck with (see
// that function's own doc, now partially superseded by this). Ability
// scores and Speed are NOT included: the book gives the Whelp no formula
// for those (18/20/16/20/5/5, 60ft flying are flat constants set once at
// creation), unlike AC and Max HP, so they stay ordinary player-editable
// fields, same as any other kind="custom" companion.
type whelpReference struct {
	ExpectedAC    int
	ExpectedMaxHP int
}

// loadWhelpReference recomputes AC ("18 + your Ninjutsu ability modifier")
// and Max HP ("your Intelligence modifier x your Science-Nin Level") fresh
// from the owner's CURRENT stats every render, honoring any manual pin
// (character_companion_overrides) the same way loadSNBReference/
// loadTitanReference/loadNinDogReference already do for their own
// formula-backed fields, and writes the resolved values straight to the
// companion row via charstore.SetWhelpStatDefaultsLive — the identical
// "auto-then-pin, re-read after write" contract those three loaders already
// establish, so companion_stat_fields' shared AC/Max HP inputs need no
// Whelp-specific logic beyond knowing to LABEL them as auto-computed (see
// companion_fields.html's own $isWhelp gate).
func (s *server) loadWhelpReference(characterID int64, companion charstore.Companion, sheet *charsheet.Sheet) (*whelpReference, error) {
	scienceNinLevel, err := s.scienceNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	ac := 18 + ninjutsuAbilityModifier(sheet)
	intMod := sheet.Abilities["int"].Modifier
	hpMax := intMod * scienceNinLevel
	if hpMax < 1 {
		hpMax = 1
	}

	overrides, err := charstore.GetCompanionOverrides(s.charDB, companion.ID)
	if err != nil {
		return nil, err
	}
	if v, ok := companionOverrideInt(overrides, "ac"); ok {
		ac = int(v)
	}
	if v, ok := companionOverrideInt(overrides, "hp_max"); ok {
		hpMax = int(v)
	}

	if err := charstore.SetWhelpStatDefaultsLive(s.charDB, characterID, companion.ID, int64(ac), int64(hpMax)); err != nil {
		return nil, err
	}

	return &whelpReference{ExpectedAC: ac, ExpectedMaxHP: hpMax}, nil
}

// ninjutsuAbilityModifier resolves the player's own Ninjutsu ability
// modifier (the ability score their Ninjutsu attacks key off), 0 for a
// sheet with no Ninjutsu discipline at all — factored out of
// ensureWhelpCompanion so loadWhelpReference's own live recompute can never
// drift from the identical formula the one-time creation prefill uses.
func ninjutsuAbilityModifier(sheet *charsheet.Sheet) int {
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Ninjutsu" {
			return sheet.Abilities[a.Ability].Modifier
		}
	}
	return 0
}

// removeWhelpCompanion deletes the Whelp ensureWhelpCompanion auto-added,
// called by syncDraconicGauntletWhelp once Draconic Gauntlet is no longer
// the character's designated Ascended W.o.W — matched by
// scienceNinWhelpCompanionName's exact name, same lookup ensureWhelpCompanion
// itself uses. A no-op if no such companion exists (already deleted by the
// player, or Draconic Gauntlet was never actually Ascended).
func (s *server) removeWhelpCompanion(characterID int64) error {
	existing, err := charstore.ListCompanions(s.charDB, characterID)
	if err != nil {
		return err
	}
	for _, c := range existing {
		if c.Name == scienceNinWhelpCompanionName {
			return charstore.DeleteCompanion(s.charDB, characterID, c.ID)
		}
	}
	return nil
}

// syncDraconicGauntletWhelp resyncs the Whelp companion to whatever the
// character's CURRENT Ascended W.o.W designation actually is, rather than
// threading an old-value/new-value diff through every route that can change
// it. There are four such routes — the Core sheet's own compact add/delete
// (scienceNinSubclassPickAddCore/handleScienceNinSubclassPickDelete) and the
// Elemental Innovationist popup's matching pair
// (scienceNinTrackerPopupAdd/subclassTrackerPopupDelete) — plus a fifth,
// indirect one: forgetting the underlying W.o.W pick itself while it happens
// to be the Ascended designee (removeWoWPick, science_nin_elemental_
// innovationist_popup.go), which clears the dangling Ascended row without
// going through either delete route above. A single idempotent "recompute
// from current truth" call, made from all five, can never drift out of sync
// with whichever surface the player actually used — the exact class of bug
// Task #12's per-row Demon Foe checkbox had to work around by finding every
// rendering path a computed value could reach; this sidesteps it entirely by
// never trusting a call site's own "before" state.
func (s *server) syncDraconicGauntletWhelp(characterID int64) error {
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickAscendedWoW)
	if err != nil {
		return err
	}
	for _, p := range picks {
		if p.OptionSlug == scienceNinDraconicGauntletWoWSlug {
			return s.ensureWhelpCompanion(characterID)
		}
	}
	return s.removeWhelpCompanion(characterID)
}

// logWhelpSyncErr is syncDraconicGauntletWhelp's own call-site error
// handling — same "don't fail the primary action over a secondary
// companion-sync" judgment call addEIPPick's own ensureAngelEIPSpectreCompanion
// error handling already makes: the Ascended W.o.W designation itself is
// already stored (or cleared) successfully by the time this runs, so a
// database error here just leaves the player to add/remove the Whelp by hand
// rather than failing the whole pick.
func logWhelpSyncErr(err error) {
	if err != nil {
		log.Println("sync draconic gauntlet whelp companion:", err)
	}
}
