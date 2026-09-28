package charstore

import (
	"database/sql"
)

// subclass_picks.go: the per-subclass catalog-pick tables — one section per
// class. They all share the same shape: an Add that upserts a pick, a
// Remove, and a List the sheet reads back. The picker's own budget rules
// live on the cmd/n5e side, not here.

// AddScienceNinTool records one Scientific Ninja Tools catalog pick (see
// cmd/n5e/science_nin.go). A duplicate add is a silent no-op — same shape
// AddNinjutsuMoldingPick/AddMedicalDoctrinePick already use, since the
// picker's own Creation Points budget check is what actually decides
// whether a NEW pick can be added, not this.
func AddScienceNinTool(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_science_nin_tools (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveScienceNinTool drops one known tool — freely, at any time. The book
// states no rest/level-up gate on this pick at all (unlike Efficient
// Molding or Hunters Exploits), so this is the least restrictive of every
// pick-removal function in this package, not just following their same
// "trust the player" boundary.
func RemoveScienceNinTool(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_science_nin_tools WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListScienceNinTools returns the class_option_entries slugs a character has
// picked.
func ListScienceNinTools(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_science_nin_tools WHERE character_id = ?`,
		characterID)
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

// ScienceNinSubclassPickCategory is one of eight subclasses' own
// cap+catalog picks handled by cmd/n5e/science_nin_subclasses.go — a
// second, more heterogeneous cousin of character_science_nin_tools (the
// base-class Scientific Ninja Tools catalog, which needs no category
// discriminator since it's the only Creation-Points-budget pick in this
// schema).
type ScienceNinSubclassPickCategory string

const (
	ScienceNinPickEIP             ScienceNinSubclassPickCategory = "eip"
	ScienceNinPickWOW             ScienceNinSubclassPickCategory = "wow"
	ScienceNinPickPermaPerk       ScienceNinSubclassPickCategory = "perma_perk"
	ScienceNinPickBIM             ScienceNinSubclassPickCategory = "bim"
	ScienceNinPickInversionSerum  ScienceNinSubclassPickCategory = "inversion_serum"
	ScienceNinPickArsenalMod      ScienceNinSubclassPickCategory = "arsenal_mod"
	ScienceNinPickPerfectedWeapon ScienceNinSubclassPickCategory = "perfected_weapon"

	// The final four subclasses' own categories — see
	// 0046_science_nin_subclass_picks_more_categories.sql for what each one
	// backs.
	ScienceNinPickShinobiWareUpgrade    ScienceNinSubclassPickCategory = "shinobi_ware_upgrade"
	ScienceNinPickEvolvedUpgrade        ScienceNinSubclassPickCategory = "evolved_upgrade"
	ScienceNinPickSpywareProgram        ScienceNinSubclassPickCategory = "spyware_program"
	ScienceNinPickQuickHack             ScienceNinSubclassPickCategory = "quick_hack"
	ScienceNinPickAirTreckEnhancement   ScienceNinSubclassPickCategory = "air_treck_enhancement"
	ScienceNinPickRegalia               ScienceNinSubclassPickCategory = "regalia"
	ScienceNinPickTechnobiMechanization ScienceNinSubclassPickCategory = "technobi_mechanization"
	ScienceNinPickSNBUpgrade            ScienceNinSubclassPickCategory = "snb_upgrade"

	// ScienceNinPickMixedStudiesInquiry is base-class Mixed Studies (18th
	// level) — a single-slot pick, added by
	// 0051_science_nin_mixed_studies_pick.sql. option_slug holds the chosen
	// OTHER Scientific Inquiry's own subclass slug rather than a
	// class_options/class_option_entries reference like every category
	// above — see cmd/n5e/science_nin.go's handleMixedStudiesPickAdd.
	ScienceNinPickMixedStudiesInquiry ScienceNinSubclassPickCategory = "mixed_studies_inquiry"

	// ScienceNinPickInfusedTool is Infused Genius (11th level, base class) —
	// added by 0053_science_nin_infused_genius.sql. Unlike every category
	// above, its option_slug is shared with the base Scientific Ninja Tools
	// catalog (character_science_nin_tools) rather than pointing at a
	// catalog of its own — see cmd/n5e/science_nin.go's InfusedGenius field.
	ScienceNinPickInfusedTool ScienceNinSubclassPickCategory = "infused_tool"

	// The four categories below — added by
	// 0055_science_nin_more_pick_categories.sql — each restrict a cap-gated
	// pick to a subset of a character's own already-known picks in a
	// sibling category above (bim/inversion_serum/shinobi_ware_upgrade/
	// snb_upgrade), the same "restricted to already-known subset via
	// cross-reference" shape perma_perk/quick_hack/infused_tool already
	// use. See each field's own doc in
	// cmd/n5e/science_nin_subclasses.go's scienceNinGrenadierData/
	// scienceNinMadScientistData/scienceNinShinobiWareData/
	// scienceNinSNBSpecialistData.
	ScienceNinPickBIMSpecialist         ScienceNinSubclassPickCategory = "bim_specialist"
	ScienceNinPickSheepAndShepherdSerum ScienceNinSubclassPickCategory = "sheep_and_shepherd_serum"
	// ScienceNinPickShinjutsuUpgrade is PERMANENT once made ("You can not
	// change this later") — unlike every other category in this table, no
	// delete route is wired for it (see server.go).
	ScienceNinPickShinjutsuUpgrade    ScienceNinSubclassPickCategory = "shinjutsu_upgrade"
	ScienceNinPickSNBUpgradePermanent ScienceNinSubclassPickCategory = "snb_upgrade_permanent"

	// ScienceNinPickAscendedWoW is Elemental Innovationist's own Elemental
	// Innovation (17th level) — added by
	// 0058_science_nin_ascended_wow.sql. Same "restricted to already-known
	// subset via cross-reference" shape as ScienceNinPickBIMSpecialist
	// above (any known 'wow' pick qualifies, no exclusion clause): one
	// designated Ascended W.o.W, cap 1. See
	// cmd/n5e/science_nin_subclasses.go's
	// scienceNinElementalInnovationistData.DesignatedWoW.
	ScienceNinPickAscendedWoW ScienceNinSubclassPickCategory = "ascended_wow"

	// The three Mech Crafter (Titan) categories below — added by
	// 0067_titan_subclass_picks.sql. Unlike every category above, all
	// three are character-scoped rather than tied to a specific companion
	// row: Ordnance Training's own text ("You can only have 1 Titan
	// created at a time") means a character never has more than one Titan
	// to distinguish between, so no companion_id column exists on this
	// table for these categories either. See cmd/n5e/titan.go.
	//
	// ScienceNinPickTitanUpgrade is Ordnance Training's own Titan Slots
	// (cap = Proficiency Bonus) — a Mech- or Weapon-keyword Titan Upgrade
	// installed on a long rest. option_slug is the picked Titan Upgrades
	// class_option_entries.slug (or the Mastercraft tier's own
	// class_options slug for Bijuu Slayer, which has no entries row, never
	// split since it's a single named item bundled straight into its own
	// tier). Spends from the SAME Creation Points budget as the base
	// Scientific Ninja Tools catalog — see titanEffectiveUpgradeCost.
	ScienceNinPickTitanUpgrade ScienceNinSubclassPickCategory = "titan_upgrade"

	// ScienceNinPickTitanExosuitUpgrade is Endless Work's (6th level) own
	// separate 1 (2 at 14th level) Mech-keyword-only slot, restricted to
	// upgrades of Cost 8 or lower ("Greater or lower") — see
	// cmd/n5e/titan.go's own header doc on the Refined/Greater tier merge
	// this restriction is read against. Tracked as its own category (not
	// ScienceNinPickTitanUpgrade above) so the same upgrade can occupy an
	// ordinary Titan Slot and the Exo-Suit slot independently, without the
	// two colliding over the same option_slug — same "separate category
	// per independent slot pool" precedent
	// ScienceNinPickPerfectedWeapon already sets against ScienceNinPickArsenalMod.
	ScienceNinPickTitanExosuitUpgrade ScienceNinSubclassPickCategory = "titan_exosuit_upgrade"

	// ScienceNinPickTitanSpecialistCraftingKeyword is Specialist Crafting's
	// (14th level) own single-slot "Mech or Weapon" keyword designation —
	// option_slug is the literal string "mech" or "weapon", cap 1, freely
	// re-picked (the current pick is dropped, then the other is added), the
	// same "trust the player" boundary ScienceNinPickMixedStudiesInquiry's
	// own single-slot pick already draws.
	ScienceNinPickTitanSpecialistCraftingKeyword ScienceNinSubclassPickCategory = "titan_specialist_crafting_keyword"

	// The three categories below — added by
	// 0073_ninjaneer_weapon_designations.sql — are Ninjaneer's own
	// single-slot "which owned weapon" designations. Unlike every category
	// above, option_slug is a character_inventory.id (cast to text), not a
	// class_options/class_option_entries slug or a hand-assigned string —
	// WHICH specific owned copy of a weapon is designated matters here,
	// since two owned copies of the same weapon type must stay
	// distinguishable. See cmd/n5e/ninjaneer.go for what each backs and how
	// the designation feeds character_weapon_attack_options.
	ScienceNinPickEnhancedWeapon      ScienceNinSubclassPickCategory = "enhanced_weapon"
	ScienceNinPickLegendaryWeapon     ScienceNinSubclassPickCategory = "legendary_weapon"
	ScienceNinPickPerfectedWeaponMark ScienceNinSubclassPickCategory = "perfected_weapon_mark"

	// ScienceNinPickEverEvolvingSeal is Shinobi-Ware's own 14th-level Ever
	// Evolving — added by 0074_science_nin_ever_evolving_seal.sql. Single
	// slot (cap 1: "apply IT to your Full Metal Shinobi armor," singular),
	// gated by the SAME shared Creation Points budget as the base
	// Scientific Ninja Tools catalog rather than a flat slot-count cap —
	// the same CP-BUDGET shape ScienceNinPickTitanUpgrade already uses,
	// unlike every category above. option_slug is the picked equipment
	// slug (kind='enhancement_seal', seal_applies_to='armor'), the same
	// catalog shape Martial Defense's own guard-slot picks draw from
	// (character_martial_defense_seals, a separate dedicated table) — Ever
	// Evolving reuses this generic table instead since it's a single CP-
	// budget pick, not a multi-slot count. See cmd/n5e/ever_evolving.go.
	ScienceNinPickEverEvolvingSeal ScienceNinSubclassPickCategory = "ever_evolving_seal"
)

// ScienceNinSubclassPick is one stored pick, along with its own pool
// ("mending"/"maiming"/"") when the category records one — every category
// except inversion_serum leaves this "" — and its own held Quantity (always
// 1 outside 'bim'; see AddScienceNinSubclassPick/RemoveScienceNinSubclassPick
// for the one category where it can climb above 1 or fall while the row
// still exists).
type ScienceNinSubclassPick struct {
	OptionSlug string
	Pool       string
	Quantity   int
}

// AddScienceNinSubclassPick records one catalog pick within its own
// category, optionally noting which CCD pool paid for it (Inversion Serums
// only; every other category passes pool="").
//
// For 'bim' (Grenadier's bandolier — "you can pick any modification with
// 'B.I.M' in the name more than once, other than the Barrier B.I.M"), a
// duplicate add INCREMENTS the stored quantity instead of no-opping; the
// Barrier B.I.M exception itself is enforced by AvailableBIM never
// re-offering an already-known Barrier B.I.M (cmd/n5e/
// science_nin_subclasses.go), not by anything here — a direct repeat call
// against Barrier B.I.M's own option_slug would still increment it, same as
// any other bim slug.
//
// For every other category, a duplicate add UPDATES the stored pool rather
// than silently no-opping — unlike every other pick table in this package,
// Inversion Serums' own pool choice is meant to be changeable after the
// fact (the book allows "created during a Long Rest", this app doesn't
// enforce that timing, same "trust the player" boundary every other pick
// already draws), so re-submitting the same serum with a different pool is
// how a player switches it, not a no-op. quantity is left untouched by this
// path (stays at its DEFAULT 1), preserving the existing "a character
// either holds a given pick or doesn't" semantics for every non-bim
// category.
func AddScienceNinSubclassPick(charDB *sql.DB, characterID int64, category ScienceNinSubclassPickCategory, optionSlug, pool string) error {
	onConflict := `pool = excluded.pool`
	if category == ScienceNinPickBIM {
		onConflict = `quantity = quantity + 1`
	}
	_, err := charDB.Exec(
		`INSERT INTO character_science_nin_subclass_picks (character_id, category, option_slug, pool) VALUES (?, ?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO UPDATE SET `+onConflict,
		characterID, category, optionSlug, pool)
	return err
}

// RemoveScienceNinSubclassPick drops one pick — freely, at any time, same
// "trust the player" boundary every other pick removal in this package
// draws.
//
// For 'bim', this decrements the held quantity by one instead of always
// discarding the whole row, so removing one held copy of a duplicate-held
// B.I.M type leaves the rest in place — the row itself is only deleted once
// quantity would drop to (or was already at) zero or below. Every other
// category still deletes the row outright, since quantity never leaves 1
// there.
func RemoveScienceNinSubclassPick(charDB *sql.DB, characterID int64, category ScienceNinSubclassPickCategory, optionSlug string) error {
	if category == ScienceNinPickBIM {
		tx, err := charDB.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		// Delete first, matching only a row already at quantity <= 1 (a
		// single remaining copy, decremented away entirely) — done before
		// the UPDATE below so that UPDATE never has a chance to sink this
		// same row to 0 and leave a zero-quantity row behind instead of
		// deleting it.
		if _, err := tx.Exec(
			`DELETE FROM character_science_nin_subclass_picks WHERE character_id = ? AND category = ? AND option_slug = ? AND quantity <= 1`,
			characterID, category, optionSlug,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE character_science_nin_subclass_picks SET quantity = quantity - 1 WHERE character_id = ? AND category = ? AND option_slug = ?`,
			characterID, category, optionSlug,
		); err != nil {
			return err
		}
		return tx.Commit()
	}
	_, err := charDB.Exec(
		`DELETE FROM character_science_nin_subclass_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListScienceNinSubclassPicks returns every pick a character has stored
// within one category, each carrying its own pool (empty for every
// category but inversion_serum) and Quantity (always 1 outside 'bim').
func ListScienceNinSubclassPicks(charDB *sql.DB, characterID int64, category ScienceNinSubclassPickCategory) ([]ScienceNinSubclassPick, error) {
	rows, err := charDB.Query(
		`SELECT option_slug, pool, quantity FROM character_science_nin_subclass_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScienceNinSubclassPick
	for rows.Next() {
		var p ScienceNinSubclassPick
		if err := rows.Scan(&p.OptionSlug, &p.Pool, &p.Quantity); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CookingNinBlendEnhancementPick is one Gastrochemist Nature's Blend
// Enhancement pick — a known jutsu (character_jutsu.id) paired with which
// of the 4 Enhancement types was chosen for it (see cmd/n5e/cooking_nin.go).
type CookingNinBlendEnhancementPick struct {
	JutsuID         int64
	EnhancementType string
}

// AddCookingNinBlendEnhancementPick records one (jutsu, Enhancement type)
// pick. A duplicate add (the jutsu already has an Enhancement) is a silent
// no-op — same shape AddNinjutsuJutsuPick already uses, since the picker's
// own cap check is what actually decides whether a NEW pick can be added,
// not this.
func AddCookingNinBlendEnhancementPick(charDB *sql.DB, characterID int64, jutsuID int64, enhancementType string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_cooking_nin_blend_enhancement_picks (character_id, jutsu_id, enhancement_type) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, jutsu_id) DO NOTHING`,
		characterID, jutsuID, enhancementType)
	return err
}

// RemoveCookingNinBlendEnhancementPick drops one pick — freely, at any
// time, same "trust the player" boundary every other pick removal on this
// sheet draws.
func RemoveCookingNinBlendEnhancementPick(charDB *sql.DB, characterID int64, jutsuID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_cooking_nin_blend_enhancement_picks WHERE character_id = ? AND jutsu_id = ?`,
		characterID, jutsuID)
	return err
}

// ListCookingNinBlendEnhancementPicks returns every (jutsu, Enhancement
// type) pick a character has chosen. A row disappears from this list on its
// own once the underlying character_jutsu row is deleted (ON DELETE
// CASCADE), so forgetting a jutsu automatically clears any Enhancement pick
// that pointed at it.
func ListCookingNinBlendEnhancementPicks(charDB *sql.DB, characterID int64) ([]CookingNinBlendEnhancementPick, error) {
	rows, err := charDB.Query(
		`SELECT jutsu_id, enhancement_type FROM character_cooking_nin_blend_enhancement_picks WHERE character_id = ?`,
		characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CookingNinBlendEnhancementPick
	for rows.Next() {
		var p CookingNinBlendEnhancementPick
		if err := rows.Scan(&p.JutsuID, &p.EnhancementType); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GenjutsuPickCategory is one of the four cap-gated catalogs Genjutsu
// Specialist's base class grants — see cmd/n5e/genjutsu.go.
type GenjutsuPickCategory string

const (
	GenjutsuPickMirage          GenjutsuPickCategory = "mirage"
	GenjutsuPickInception       GenjutsuPickCategory = "inception"
	GenjutsuPickConversion      GenjutsuPickCategory = "conversion"
	GenjutsuPickIllusionMastery GenjutsuPickCategory = "illusion_mastery"
)

// AddGenjutsuPick records one catalog pick within its own category. A
// duplicate add is a silent no-op — same shape AddHunterNinPick already
// uses, since the picker's own cap check is what actually decides whether a
// NEW pick can be added, not this.
func AddGenjutsuPick(charDB *sql.DB, characterID int64, category GenjutsuPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_genjutsu_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveGenjutsuPick drops one pick — freely, at any time. Genjutsu
// Inception's own text implies a permanent choice and Malleable Mirages
// gates re-picking to a class level-up, but this app doesn't enforce
// re-pick timing anywhere, same boundary Hunter-Nin's own picks draw.
func RemoveGenjutsuPick(charDB *sql.DB, characterID int64, category GenjutsuPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_genjutsu_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListGenjutsuPicks returns the option_slugs a character has picked within
// one category.
func ListGenjutsuPicks(charDB *sql.DB, characterID int64, category GenjutsuPickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_genjutsu_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
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

// GenjutsuJutsuPickCategory is one of Beguiler's Twisted Casting / Corrupt
// Thoughts' Psyche Breaker — both "select two Genjutsu that you know" picks
// sourced from the character's own known jutsu (character_jutsu) rather
// than a rules-database catalog, the same shape
// NinjutsuJutsuPickCategory/AddNinjutsuJutsuPick already establish for
// Ninjutsu Specialist's Refined Ninjutsu/Ninjutsu Master — see
// cmd/n5e/genjutsu.go's loadKnownGenjutsu. A distinct type from
// GenjutsuPickCategory (genjutsu_picks.go), which keys the class's other
// four picks by rules-database option_slug instead.
type GenjutsuJutsuPickCategory string

const (
	GenjutsuPickTwistedCasting GenjutsuJutsuPickCategory = "twisted_casting"
	GenjutsuPickPsycheBreaker  GenjutsuJutsuPickCategory = "psyche_breaker"
)

// AddGenjutsuJutsuPick records one category's pick of a specific known-jutsu
// row. A duplicate add is a silent no-op — same shape AddNinjutsuJutsuPick
// already uses, since the picker's own cap check is what actually decides
// whether a NEW pick can be added, not this.
func AddGenjutsuJutsuPick(charDB *sql.DB, characterID int64, category GenjutsuJutsuPickCategory, jutsuID int64) error {
	_, err := charDB.Exec(
		`INSERT INTO character_genjutsu_jutsu_picks (character_id, category, jutsu_id) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, jutsu_id) DO NOTHING`,
		characterID, category, jutsuID)
	return err
}

// RemoveGenjutsuJutsuPick drops one pick — freely, at any time. Twisted
// Casting's own text allows changing on a Long Rest; this app doesn't
// enforce that timing, same "trust the player" boundary
// RemoveNinjutsuJutsuPick already draws.
func RemoveGenjutsuJutsuPick(charDB *sql.DB, characterID int64, category GenjutsuJutsuPickCategory, jutsuID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_genjutsu_jutsu_picks WHERE character_id = ? AND category = ? AND jutsu_id = ?`,
		characterID, category, jutsuID)
	return err
}

// ListGenjutsuJutsuPicks returns the character_jutsu row ids a character
// has picked within one category. A row id disappears from this list on its
// own once the underlying character_jutsu row is deleted (ON DELETE
// CASCADE), so forgetting a jutsu automatically clears any Twisted Casting/
// Psyche Breaker pick that pointed at it.
func ListGenjutsuJutsuPicks(charDB *sql.DB, characterID int64, category GenjutsuJutsuPickCategory) ([]int64, error) {
	rows, err := charDB.Query(
		`SELECT jutsu_id FROM character_genjutsu_jutsu_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// HunterNinPickCategory is one of the four cap-gated catalogs Hunter-Nin's
// base class grants — see cmd/n5e/hunter_nin.go.
type HunterNinPickCategory string

const (
	// HunterPickLethalPrecision: Lethal Precision's own 1st-level Taijutsu/
	// Bukijutsu pick ("Select one between Taijutsu & Bukijutsu. You can
	// cast the chosen Jutsu type using Dexterity in place of Strength for
	// all calculations. You cannot switch this choice later.") — two fixed
	// option slugs ('taijutsu'/'bukijutsu'), not class_options rows, same
	// hand-curated shape as HunterPickDefensiveTactic. cap 1, no re-cap:
	// the book's "cannot switch this choice later" is enforced by
	// handleHunterPickAdd rejecting any add once the category is already
	// at its cap, same as HunterPickWardenWeapon's own no-re-cap pick.
	HunterPickLethalPrecision HunterNinPickCategory = "lethal_precision" // base class, cap 1, no re-cap, 1st level
	HunterPickPattern         HunterNinPickCategory = "pattern"
	HunterPickExploit         HunterNinPickCategory = "exploit"
	HunterPickDefensiveTactic HunterNinPickCategory = "defensive_tactic"

	// The remaining 6 subclasses' own 3rd-level (cap 1, ->2@10th unless
	// noted) technique/property/item picks — see cmd/n5e/hunter_nin.go's
	// own option-catalog doc comments for where each one's real text
	// actually lives in rules.db (every one of them is mistagged onto an
	// unrelated 7th/17th-level feature's own description row, not the
	// granting feature's own row).
	HunterPickWardenWeapon         HunterNinPickCategory = "warden_weapon"          // Blade Warden, cap 1, no re-cap
	HunterPickWardenWeaponProperty HunterNinPickCategory = "warden_weapon_property" // Blade Warden, cap 1->2@10th
	HunterPickMedicalTechnique     HunterNinPickCategory = "medical_technique"      // Necrotic Hand, cap 1->2@10th
	HunterPickShadowTechnique      HunterNinPickCategory = "shadow_technique"       // Grave Stalker, cap 1->2@10th
	HunterPickArsenalItem          HunterNinPickCategory = "arsenal_item"           // Arsenalist, cap 4->6@10th
	HunterPickToxicTechnique       HunterNinPickCategory = "toxic_technique"        // Undertaker, cap 1->2@10th
	HunterPickViceTechnique        HunterNinPickCategory = "vice_technique"         // Vice Agent, cap 1->2@10th
	HunterPickVoidTechnique        HunterNinPickCategory = "void_technique"         // Void Walker, cap 1->2@10th
	HunterPickProstheticAttachment HunterNinPickCategory = "prosthetic_attachment"  // Wolves Legacy, cap 2->4@10th

	// HunterPickWolfTechnique: Wolves Legacy's own 10th-level Wolf
	// Techniques pick — a 6-option named-jutsu table (cap 1, no re-cap),
	// unlike the 6 tables above which all live at 3rd level. See
	// cmd/n5e/hunter_nin.go's wolfTechniqueOptions doc comment.
	HunterPickWolfTechnique HunterNinPickCategory = "wolf_technique" // Wolves Legacy, cap 1 @10th
)

// AddHunterNinPick records one catalog pick within its own category. A
// duplicate add is a silent no-op — same shape AddMartialTechnique already
// uses, since the picker's own cap check is what actually decides whether a
// NEW pick can be added, not this.
func AddHunterNinPick(charDB *sql.DB, characterID int64, category HunterNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_hunter_nin_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveHunterNinPick drops one pick — freely, at any time. The book gates
// re-picking Hunters Exploits to a long rest; this app doesn't enforce rest
// timing, same boundary Martial Adept/Mastery/Puppet Tactics already draw.
func RemoveHunterNinPick(charDB *sql.DB, characterID int64, category HunterNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_hunter_nin_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListHunterNinPicks returns the option_slugs a character has picked within
// one category.
func ListHunterNinPicks(charDB *sql.DB, characterID int64, category HunterNinPickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_hunter_nin_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
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

// ScoutNinPickCategory is one of Scout-Nin's cap-gated catalog picks — see
// cmd/n5e/scout_nin.go. Mirrors HunterNinPickCategory's shape exactly.
type ScoutNinPickCategory string

const (
	ScoutNinPickShinobiAdept ScoutNinPickCategory = "shinobi_adept"
	ScoutNinPickJackOfAll    ScoutNinPickCategory = "jack_of_all"
	// ScoutNinPickManeuvers is subclass-scoped (each subclass's own
	// Maneuvers catalog uses different option_slugs), unlike the other two
	// categories — cmd/n5e/scout_nin.go's own loadScoutNinTabData handles
	// the "picks from a different, no-longer-current subclass stay stored
	// but don't count against the cap" case, not this package.
	ScoutNinPickManeuvers ScoutNinPickCategory = "maneuvers"
	// ScoutNinPickSignatureTechnique is base-class-wide (Signature
	// Technique, 10th/18th level) over its own 3-option catalog (Hidden/
	// Aggressive/Tactical Technique).
	ScoutNinPickSignatureTechnique ScoutNinPickCategory = "signature_technique"
	// ScoutNinPickMobileSavant is Pathfinder Scout's own 3rd/6th/9th/14th
	// level pick over its 4-jutsu catalog — option_slugs are jutsu slugs,
	// not class_features slugs, unlike every other category here.
	ScoutNinPickMobileSavant ScoutNinPickCategory = "mobile_savant"
	// ScoutNinPickTacticalSuperiority is Tactical Scout's own 6th/14th/20th
	// level cross-subclass Maneuver pick — option_slugs are drawn from
	// several OTHER subclasses' own Maneuvers catalogs, not the
	// character's own chosen subclass.
	ScoutNinPickTacticalSuperiority ScoutNinPickCategory = "tactical_superiority"
	// ScoutNinPickSignatureManeuver is Tactical Scout's own 3rd/9th level
	// pick of an already-known Tactical-keyword Maneuver — option_slugs
	// always overlap with a subset of that character's own ScoutNinPickManeuvers
	// picks.
	ScoutNinPickSignatureManeuver ScoutNinPickCategory = "signature_maneuver"
	// ScoutNinPickSupremeClones is Cloning Scout's own flat 20th-level
	// pick of an already-known Maneuver — like ScoutNinPickSignatureManeuver,
	// option_slugs always overlap with a subset of that character's own
	// ScoutNinPickManeuvers picks, but with no keyword restriction and no
	// escalating cap (always exactly 1 slot).
	ScoutNinPickSupremeClones ScoutNinPickCategory = "supreme_clones"
	// ScoutNinPickVoidSoulJutsu is Trickster Scout's own 3rd-level Void Soul
	// Awakening companion-scoped known-jutsu pick (cmd/n5e/void_soul.go) —
	// option_slugs are jutsu slugs, like ScoutNinPickMobileSavant, but these
	// are jutsu the Void Soul itself knows and casts, not the player, so
	// they are deliberately never written into character_jutsu (see
	// void_soul.go's own header doc).
	ScoutNinPickVoidSoulJutsu ScoutNinPickCategory = "void_soul_jutsu"
)

// AddScoutNinPick records one catalog pick within its own category. A
// duplicate add is a silent no-op — same shape AddHunterNinPick already
// uses, since the picker's own cap check is what actually decides whether a
// NEW pick can be added, not this.
func AddScoutNinPick(charDB *sql.DB, characterID int64, category ScoutNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_scout_nin_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveScoutNinPick drops one pick — freely, at any time, same "trust the
// player" boundary Hunters Exploits/Martial Techniques already draw.
func RemoveScoutNinPick(charDB *sql.DB, characterID int64, category ScoutNinPickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_scout_nin_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListScoutNinPicks returns the option_slugs a character has picked within
// one category.
func ListScoutNinPicks(charDB *sql.DB, characterID int64, category ScoutNinPickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_scout_nin_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
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

// AddMedicalDoctrinePick records one of Medical Doctrine's 4 catalog picks
// (see cmd/n5e/medical_nin.go). A duplicate add is a silent no-op — same
// shape AddHunterNinPick/AddGenjutsuPick already use, since the picker's
// own cap check is what actually decides whether a NEW pick can be added,
// not this.
func AddMedicalDoctrinePick(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_medical_doctrine_picks (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveMedicalDoctrinePick drops one pick — freely, at any time. RAW says
// "You cannot change this choice once made," but this app doesn't enforce
// pick-timing anywhere else either (Hunters Exploits, Malleable Mirages,
// Martial Techniques all allow free re-picks), same "trust the player"
// boundary.
func RemoveMedicalDoctrinePick(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_medical_doctrine_picks WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListMedicalDoctrinePicks returns the option_slugs a character has chosen.
func ListMedicalDoctrinePicks(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_medical_doctrine_picks WHERE character_id = ?`,
		characterID)
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

// AddMedicalNinExpertCombatantPick records one of Combat Medic's Expert
// Combatant picks — a specific known-jutsu row (see
// cmd/n5e/medical_nin.go), not a rules-database slug. A duplicate add is a
// silent no-op, same boundary AddMedicalDoctrinePick draws.
func AddMedicalNinExpertCombatantPick(charDB *sql.DB, characterID int64, jutsuID int64) error {
	_, err := charDB.Exec(
		`INSERT INTO character_medical_nin_expert_combatant_picks (character_id, jutsu_id) VALUES (?, ?)
		 ON CONFLICT (character_id, jutsu_id) DO NOTHING`,
		characterID, jutsuID)
	return err
}

// RemoveMedicalNinExpertCombatantPick drops one pick — freely, at any time.
// RAW allows switching which jutsu this feature affects on a Long Rest; this
// app doesn't enforce that timing anywhere else either, same "trust the
// player" boundary as RemoveMedicalDoctrinePick.
func RemoveMedicalNinExpertCombatantPick(charDB *sql.DB, characterID int64, jutsuID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_medical_nin_expert_combatant_picks WHERE character_id = ? AND jutsu_id = ?`,
		characterID, jutsuID)
	return err
}

// ListMedicalNinExpertCombatantPicks returns the character_jutsu row ids a
// character has picked for Expert Combatant. A row id disappears from this
// list on its own once the underlying character_jutsu row is deleted (ON
// DELETE CASCADE), so forgetting a jutsu automatically clears any pick that
// pointed at it.
func ListMedicalNinExpertCombatantPicks(charDB *sql.DB, characterID int64) ([]int64, error) {
	rows, err := charDB.Query(
		`SELECT jutsu_id FROM character_medical_nin_expert_combatant_picks WHERE character_id = ?`,
		characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AddNinjutsuMoldingPick records one Efficient Molding catalog pick (see
// cmd/n5e/ninjutsu_specialist.go). A duplicate add is a silent no-op — same
// shape AddMedicalDoctrinePick/AddMartialTechnique already use, since the
// picker's own cap check is what actually decides whether a NEW pick can be
// added, not this.
func AddNinjutsuMoldingPick(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_ninjutsu_molding_picks (character_id, option_slug) VALUES (?, ?)
		 ON CONFLICT (character_id, option_slug) DO NOTHING`,
		characterID, optionSlug)
	return err
}

// RemoveNinjutsuMoldingPick drops one pick — freely, at any time. RAW gates
// re-picking to a class level-up ("When you would level up in this class,
// you may swap your current Moldings"); this app doesn't enforce that
// timing anywhere else either (Hunters Exploits, Martial Techniques, Plans
// all allow free re-picks), same "trust the player" boundary.
func RemoveNinjutsuMoldingPick(charDB *sql.DB, characterID int64, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_ninjutsu_molding_picks WHERE character_id = ? AND option_slug = ?`,
		characterID, optionSlug)
	return err
}

// ListNinjutsuMoldingPicks returns the option_slugs a character has chosen.
func ListNinjutsuMoldingPicks(charDB *sql.DB, characterID int64) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_ninjutsu_molding_picks WHERE character_id = ?`,
		characterID)
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

// NinjutsuJutsuPickCategory is one of Ninjutsu Specialist's picks sourced
// from the character's own known jutsu (character_jutsu) rather than a
// rules-database catalog — see cmd/n5e/ninjutsu_specialist.go.
type NinjutsuJutsuPickCategory string

const (
	NinjutsuPickRefined        NinjutsuJutsuPickCategory = "refined_ninjutsu"
	NinjutsuPickMaster         NinjutsuJutsuPickCategory = "ninjutsu_master"
	NinjutsuPickAwakenedScroll NinjutsuJutsuPickCategory = "awakened_scroll"
)

// AddNinjutsuJutsuPick records one category's pick of a specific known-jutsu
// row. A duplicate add is a silent no-op, same boundary every other pick
// table in this file draws.
func AddNinjutsuJutsuPick(charDB *sql.DB, characterID int64, category NinjutsuJutsuPickCategory, jutsuID int64) error {
	_, err := charDB.Exec(
		`INSERT INTO character_ninjutsu_jutsu_picks (character_id, category, jutsu_id) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, jutsu_id) DO NOTHING`,
		characterID, category, jutsuID)
	return err
}

// RemoveNinjutsuJutsuPick drops one pick — freely, at any time. Refined
// Ninjutsu's own text allows changing on a long rest; this app doesn't
// enforce that timing, same "trust the player" boundary as above.
func RemoveNinjutsuJutsuPick(charDB *sql.DB, characterID int64, category NinjutsuJutsuPickCategory, jutsuID int64) error {
	_, err := charDB.Exec(
		`DELETE FROM character_ninjutsu_jutsu_picks WHERE character_id = ? AND category = ? AND jutsu_id = ?`,
		characterID, category, jutsuID)
	return err
}

// ListNinjutsuJutsuPicks returns the character_jutsu row ids a character has
// picked within one category. A row id disappears from this list on its own
// once the underlying character_jutsu row is deleted (ON DELETE CASCADE),
// so forgetting a jutsu automatically clears any Refined Ninjutsu/Ninjutsu
// Master pick that pointed at it.
func ListNinjutsuJutsuPicks(charDB *sql.DB, characterID int64, category NinjutsuJutsuPickCategory) ([]int64, error) {
	rows, err := charDB.Query(
		`SELECT jutsu_id FROM character_ninjutsu_jutsu_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// IntelligenceOperativePickCategory is one of the two cap-gated catalogs
// Intelligence Operative grants — see cmd/n5e/intelligence_operative.go.
type IntelligenceOperativePickCategory string

const (
	IntelligenceOperativePickPlan          IntelligenceOperativePickCategory = "plan"
	IntelligenceOperativePickOperativeTrap IntelligenceOperativePickCategory = "operative_trap"
)

// AddIntelligenceOperativePick records one catalog pick within its own
// category. A duplicate add is a silent no-op — same shape
// AddHunterNinPick/AddGenjutsuPick already use, since the picker's own cap
// check is what actually decides whether a NEW pick can be added, not this.
func AddIntelligenceOperativePick(charDB *sql.DB, characterID int64, category IntelligenceOperativePickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`INSERT INTO character_intelligence_operative_picks (character_id, category, option_slug) VALUES (?, ?, ?)
		 ON CONFLICT (character_id, category, option_slug) DO NOTHING`,
		characterID, category, optionSlug)
	return err
}

// RemoveIntelligenceOperativePick drops one pick — freely, at any time.
// Plans' own text allows switching on a long rest ("When you would gain the
// benefits of a long rest you may switch the plans you know with another
// plan"); this app doesn't enforce that timing anywhere, same boundary
// Hunter-Nin/Genjutsu Specialist's own picks already draw.
func RemoveIntelligenceOperativePick(charDB *sql.DB, characterID int64, category IntelligenceOperativePickCategory, optionSlug string) error {
	_, err := charDB.Exec(
		`DELETE FROM character_intelligence_operative_picks WHERE character_id = ? AND category = ? AND option_slug = ?`,
		characterID, category, optionSlug)
	return err
}

// ListIntelligenceOperativePicks returns the option_slugs a character has
// picked within one category.
func ListIntelligenceOperativePicks(charDB *sql.DB, characterID int64, category IntelligenceOperativePickCategory) ([]string, error) {
	rows, err := charDB.Query(
		`SELECT option_slug FROM character_intelligence_operative_picks WHERE character_id = ? AND category = ?`,
		characterID, category)
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
