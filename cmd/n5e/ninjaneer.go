package main

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Ninjaneer's own weapon designations — WHICH specific owned weapon (a
// character_inventory row) currently fills Enhanced Arsenal's Enhanced
// Weapon, Warrior of Science's Legendary Weapon, and A Weapon to Surpass's
// (single, for this app's purposes) Perfected Weapon. Nothing before this
// file let a player bind any of Ninjaneer's own weapon-shaped features to
// one specific piece of gear — Arsenal Modifications/Perfected Weapons
// (science_nin_subclasses.go's scienceNinNinjaneerData) only ever tracked
// abstract known-upgrade slots, never bound to a real inventory row.
//
// All three designations are single-slot and freely re-picked — the SAME
// "trust the player, clear the current pick before choosing a different
// one" boundary Mixed Studies/Ascended W.o.W/B.I.M Specialist already draw
// for their own single-slot picks (science_nin_subclasses.go) — enforced
// here by the Add handler refusing a second pick outright rather than
// silently replacing one.
//
// scienceNinWarriorOfScienceFeatureSlug is Warrior of Science (9th level):
// "As an Action you can spend 20 CCD chakra and turn an Enhanced Weapon you
// are holding into a Legendary Weapon. You must pay 10 CCD chakra at the
// start of your turn to maintain this benefit." Confirmed level 9 against
// dist/rules.db's v_subclass_features.
const scienceNinWarriorOfScienceFeatureSlug = "class/science-nin/group/scientific-inquiry/ninjaneer/feature/warrior-of-science"

// ninjaneerWeaponOption is one candidate for a Ninjaneer weapon designation
// — either the character's own currently-designated weapon, or one still
// available to pick. Unlike every catalog option in
// science_nin_subclasses.go, Slug here is a character_inventory.id (decimal
// text), not a class_options/class_option_entries reference — WHICH owned
// copy of a weapon is designated matters, since a character can own two
// identical weapons.
type ninjaneerWeaponOption struct {
	InventoryID int64
	Name        string
	// ItemHref links to the weapon's own rules/custom-item detail page
	// (inventoryRow.DetailHref) — "" only for a custom item slug that no
	// longer resolves, the same graceful-degrade lookupCarriedItem already
	// applies.
	ItemHref string
	// Description is the weapon's own printed/custom item description,
	// shown as this picker's rollover tooltip content (formatDescription,
	// same convention every other option_slug picker on this sheet
	// follows) so a player can tell which owned weapon they're designating
	// without leaving the sheet.
	Description string
}

// ninjaneerWeaponDesignationTier bundles what each of the three
// designations needs: which pick category stores it, which granting
// feature gates the picker being offered at all, and whether Enhanced
// Arsenal's own "use Intelligence in place of Dexterity for Enhanced
// Finesse weapons" clause applies to it (Enhanced Weapon only — Warrior of
// Science and A Weapon to Surpass restate no ability override of their
// own).
type ninjaneerWeaponDesignationTier struct {
	Category      charstore.ScienceNinSubclassPickCategory
	FeatureSlug   string
	FinesseIntDex bool
	// DamageBonus computes The Future of Shinobi: Weapons' own flat
	// per-tier damage-roll bonus ("Your Enhanced Weapons add your
	// proficiency bonus to all damage rolls. Legendary Weapons add twice
	// your proficiency bonus to all damage rolls. Perfected Weapons add
	// your Intelligence Modifier to all damage rolls.") from the
	// character's own current Proficiency Bonus/Intelligence modifier —
	// recomputed on every load rather than stored, so a level-up keeps it
	// correct with no re-pick needed.
	DamageBonus func(sheet *charsheet.Sheet) int
}

var (
	ninjaneerEnhancedWeaponTier = ninjaneerWeaponDesignationTier{
		Category: charstore.ScienceNinPickEnhancedWeapon, FeatureSlug: scienceNinArsenalFeatureSlug, FinesseIntDex: true,
		DamageBonus: func(sheet *charsheet.Sheet) int { return sheet.ProficiencyBonus },
	}
	ninjaneerLegendaryWeaponTier = ninjaneerWeaponDesignationTier{
		Category: charstore.ScienceNinPickLegendaryWeapon, FeatureSlug: scienceNinWarriorOfScienceFeatureSlug,
		DamageBonus: func(sheet *charsheet.Sheet) int { return 2 * sheet.ProficiencyBonus },
	}
	ninjaneerPerfectedWeaponMarkTier = ninjaneerWeaponDesignationTier{
		Category: charstore.ScienceNinPickPerfectedWeaponMark, FeatureSlug: scienceNinPerfectedWeaponFeatureSlug,
		DamageBonus: func(sheet *charsheet.Sheet) int { return nonNegativeIntMod(sheet.Abilities["int"].Modifier) },
	}
)

// loadNinjaneerCandidateWeapons returns every weapon-kind row in the
// character's own inventory — regardless of Equipped, since Enhanced
// Arsenal's own text ("During a Long Rest you can work on your weapons and
// make an Enhanced Weapon") doesn't require the weapon to already be worn —
// eligible to be designated an Enhanced/Legendary/Perfected Weapon. A
// custom item qualifies the same way it does for the Attacks & Jutsu table
// itself (buildAttacks): RollableKind == "weapon", not its free-text Kind.
func (s *server) loadNinjaneerCandidateWeapons(characterID int64) ([]ninjaneerWeaponOption, error) {
	inventory, err := s.loadCharacterInventory(characterID)
	if err != nil {
		return nil, err
	}
	var out []ninjaneerWeaponOption
	for _, item := range inventory {
		if item.Slug == "" {
			continue
		}
		var description string
		if strings.HasPrefix(item.Slug, "custom/") {
			ci, err := charstore.GetCustomItemBySlug(s.charDB, item.Slug)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
			if ci.RollableKind != "weapon" {
				continue
			}
			description = ci.Description
		} else {
			if item.Kind != "weapon" {
				continue
			}
			var d sql.NullString
			if err := s.rulesDB.QueryRow(`SELECT description FROM equipment WHERE slug = ?`, item.Slug).Scan(&d); err != nil && err != sql.ErrNoRows {
				return nil, err
			}
			description = d.String
		}
		out = append(out, ninjaneerWeaponOption{InventoryID: item.ID, Name: item.Name, ItemHref: item.DetailHref(), Description: description})
	}
	return out, nil
}

// ninjaneerCandidateFinesse reports whether one candidate weapon (by
// inventory row id) carries the Finesse property — read fresh off
// equipment.properties/custom_items.properties rather than cached on
// ninjaneerWeaponOption, since only the currently-designated Enhanced
// Weapon ever needs this check (recomputeNinjaneerWeaponOverrides below),
// not every candidate in the picker list.
func (s *server) ninjaneerCandidateFinesse(characterID, inventoryID int64) (bool, error) {
	var itemSlug string
	if err := s.charDB.QueryRow(
		`SELECT item_slug FROM character_inventory WHERE id = ? AND character_id = ?`, inventoryID, characterID,
	).Scan(&itemSlug); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	var properties string
	if strings.HasPrefix(itemSlug, "custom/") {
		ci, err := charstore.GetCustomItemBySlug(s.charDB, itemSlug)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		properties = ci.Properties
	} else {
		var p sql.NullString
		if err := s.rulesDB.QueryRow(`SELECT properties FROM equipment WHERE slug = ?`, itemSlug).Scan(&p); err != nil {
			if err == sql.ErrNoRows {
				return false, nil
			}
			return false, err
		}
		properties = p.String
	}
	return strings.Contains(strings.ToLower(properties), "finesse"), nil
}

// ninjaneerDesignatedWeapon splits the character's own weapon-kind
// inventory into the currently-designated pick for one tier (nil if none,
// or if the stored pick points at a weapon that's no longer a candidate —
// same "stale pick just isn't a match" tolerance mergeMixedStudiesFeatures
// already applies) and the remaining candidates still available to pick.
func (s *server) ninjaneerDesignatedWeapon(characterID int64, category charstore.ScienceNinSubclassPickCategory, candidates []ninjaneerWeaponOption) (current *ninjaneerWeaponOption, available []ninjaneerWeaponOption, err error) {
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, category)
	if err != nil {
		return nil, nil, err
	}
	pickedInvID := int64(-1)
	if len(picks) > 0 {
		pickedInvID, _ = strconv.ParseInt(picks[0].OptionSlug, 10, 64)
	}
	for _, c := range candidates {
		if c.InventoryID == pickedInvID {
			pick := c
			current = &pick
			continue
		}
		available = append(available, c)
	}
	return current, available, nil
}

// currentWeaponAttackOptions reads back one inventory row's own stored
// character_weapon_attack_options, or a zero-value (all-derived) row if it
// has none yet — the base recomputeNinjaneerWeaponOverrides merges its own
// AttackAbility/DamageBonus contribution on top of, so a player's own
// manual AttackProf/AttackBonus/DamageAbility (set via the sheet's per-
// weapon Adjust form, handleSheetWeaponAttackOptions) survive a recompute
// untouched.
func currentWeaponAttackOptions(charDB *sql.DB, characterID, inventoryID int64) (charstore.WeaponAttackOptions, error) {
	all, err := charstore.ListWeaponAttackOptions(charDB, characterID)
	if err != nil {
		return charstore.WeaponAttackOptions{}, err
	}
	if o, ok := all[inventoryID]; ok {
		return o, nil
	}
	return charstore.WeaponAttackOptions{InventoryID: inventoryID}, nil
}

// clearNinjaneerWeaponOverride resets whichever of AttackAbility/
// DamageBonus this mechanism owns on one weapon back to "derived" (empty/
// zero), preserving AttackProf/AttackBonus/DamageAbility exactly as stored
// — called when a designation is cleared (handleNinjaneerWeaponDesignationDelete)
// so a stale bonus doesn't linger on a weapon that's no longer designated;
// recomputeNinjaneerWeaponOverrides only ever looks at the CURRENTLY
// designated weapon each load, so once a pick row is gone this is the only
// place that still knows which weapon to clean up.
func (s *server) clearNinjaneerWeaponOverride(characterID, inventoryID int64, clearAttackAbility bool) error {
	opts, err := currentWeaponAttackOptions(s.charDB, characterID, inventoryID)
	if err != nil {
		return err
	}
	if clearAttackAbility {
		opts.AttackAbility = ""
	}
	opts.DamageBonus = 0
	return charstore.SetWeaponAttackOptions(s.charDB, characterID, opts)
}

// recomputeNinjaneerWeaponOverrides keeps the three designations' own
// character_weapon_attack_options contributions in sync with the
// character's own CURRENT level/Proficiency Bonus/Intelligence modifier and
// CURRENT granted-features set — recomputed every time this runs rather
// than written once, the same "grant once, recompute defensively" shape
// ensureAirTrecksGranted uses for Air Trecks' own Intelligence-for-
// attack/damage override, just applied every load instead of only once.
// Handles a level-up (Proficiency Bonus/Intelligence modifier changes) and
// gaining/losing The Future of Shinobi: Weapons automatically; switching
// WHICH weapon is designated is instead handled explicitly by
// handleNinjaneerWeaponDesignationDelete's own clear step, since once a
// pick row is deleted this function has no way to know which weapon to
// clean up.
func (s *server) recomputeNinjaneerWeaponOverrides(characterID int64, sheet *charsheet.Sheet, granted []grantedFeatureRow) error {
	hasCapstone := hasFeature(granted, scienceNinFutureOfShinobiWeaponsFeatureSlug)
	for _, tier := range []ninjaneerWeaponDesignationTier{ninjaneerEnhancedWeaponTier, ninjaneerLegendaryWeaponTier, ninjaneerPerfectedWeaponMarkTier} {
		picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, tier.Category)
		if err != nil {
			return err
		}
		if len(picks) == 0 {
			continue
		}
		inventoryID, err := strconv.ParseInt(picks[0].OptionSlug, 10, 64)
		if err != nil {
			continue // corrupt/stale pick — nothing to recompute against
		}
		opts, err := currentWeaponAttackOptions(s.charDB, characterID, inventoryID)
		if err != nil {
			return err
		}

		if tier.FinesseIntDex {
			finesse := false
			if hasFeature(granted, tier.FeatureSlug) {
				finesse, err = s.ninjaneerCandidateFinesse(characterID, inventoryID)
				if err != nil {
					return err
				}
			}
			if finesse {
				opts.AttackAbility = "int"
			} else {
				opts.AttackAbility = ""
			}
		}

		if hasCapstone && hasFeature(granted, tier.FeatureSlug) {
			opts.DamageBonus = tier.DamageBonus(sheet)
		} else {
			opts.DamageBonus = 0
		}

		if err := charstore.SetWeaponAttackOptions(s.charDB, characterID, opts); err != nil {
			return err
		}
	}
	return nil
}

// handleNinjaneerWeaponDesignationAdd builds one tier's "designate a
// weapon" route — shared by all three designations, differing only in
// which category/feature/Finesse-clause the tier struct carries. Refuses a
// second pick outright rather than silently replacing the first (cap 1,
// same "clear the current pick first" boundary every other single-slot
// pick in this package draws) — the sheet's own markup only ever shows the
// Available list once the current pick has been cleared, matching Mixed
// Studies/Ascended W.o.W's own template shape.
func (s *server) handleNinjaneerWeaponDesignationAdd(tier ninjaneerWeaponDesignationTier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCharacterID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		invIDStr := strings.TrimSpace(r.FormValue("option_slug"))
		if status, msg := s.addNinjaneerWeaponDesignation(tier, id, invIDStr); status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		s.respondSheet(w, r, id, "sheet_science_nin")
	}
}

// ninjaneerWeaponDesignationPopupAdd is addNinjaneerWeaponDesignation's own
// popup-flavored wrapper — redirects to popupPath instead of swapping the
// Core sheet's sheet_science_nin fragment. Shared by the three weapon
// designation sections inside the Ninjaneer subclass tracker popup
// (science_nin_ninjaneer_popup.go).
func (s *server) ninjaneerWeaponDesignationPopupAdd(tier ninjaneerWeaponDesignationTier, popupPath func(int64) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCharacterID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		invIDStr := strings.TrimSpace(r.FormValue("option_slug"))
		if status, msg := s.addNinjaneerWeaponDesignation(tier, id, invIDStr); status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		http.Redirect(w, r, popupPath(id), http.StatusSeeOther)
	}
}

// addNinjaneerWeaponDesignation validates and stores one tier's "designate a
// weapon" pick — shared by handleNinjaneerWeaponDesignationAdd's own
// Core-sheet AJAX route and ninjaneerWeaponDesignationPopupAdd's popup
// route, differing only in which of the tier struct's own
// category/feature/Finesse-clause fields it reads. Refuses a second pick
// outright rather than silently replacing the first (cap 1, same "clear the
// current pick first" boundary every other single-slot pick in this package
// draws).
func (s *server) addNinjaneerWeaponDesignation(tier ninjaneerWeaponDesignationTier, id int64, invIDStr string) (int, string) {
	inventoryID, err := strconv.ParseInt(invIDStr, 10, 64)
	if invIDStr == "" || err != nil {
		return http.StatusBadRequest, "missing weapon"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for ninjaneer weapon designation add:", err)
		return http.StatusInternalServerError, "database error"
	}
	granted, err := s.loadGrantedFeatures(id, sheet.ClanSlug, sheet.Level)
	if err != nil {
		log.Println("load granted features for ninjaneer weapon designation add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if !hasFeature(granted, tier.FeatureSlug) {
		return http.StatusBadRequest, "character does not have this feature"
	}
	existing, err := charstore.ListScienceNinSubclassPicks(s.charDB, id, tier.Category)
	if err != nil {
		log.Println("list ninjaneer weapon designation picks:", err)
		return http.StatusInternalServerError, "database error"
	}
	if len(existing) > 0 {
		return http.StatusBadRequest, "a weapon is already designated — clear it first"
	}
	candidates, err := s.loadNinjaneerCandidateWeapons(id)
	if err != nil {
		log.Println("load ninjaneer candidate weapons:", err)
		return http.StatusInternalServerError, "database error"
	}
	valid := false
	for _, c := range candidates {
		if c.InventoryID == inventoryID {
			valid = true
			break
		}
	}
	if !valid {
		return http.StatusBadRequest, "not a valid weapon to designate"
	}
	if err := charstore.AddScienceNinSubclassPick(s.charDB, id, tier.Category, invIDStr, ""); err != nil {
		log.Println("add ninjaneer weapon designation:", err)
		return http.StatusInternalServerError, "database error"
	}
	if err := s.recomputeNinjaneerWeaponOverrides(id, sheet, granted); err != nil {
		log.Println("recompute ninjaneer weapon overrides:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleNinjaneerWeaponDesignationDelete clears one tier's designation —
// freely, at any time, same "trust the player" boundary every other pick
// removal in this codebase draws — and resets whatever
// character_weapon_attack_options contribution this mechanism had applied
// to the now-former weapon, so no stale bonus lingers on it.
func (s *server) handleNinjaneerWeaponDesignationDelete(tier ninjaneerWeaponDesignationTier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCharacterID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		invIDStr := strings.TrimSpace(r.FormValue("option_slug"))
		if status, msg := s.removeNinjaneerWeaponDesignation(tier, id, invIDStr); status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		s.respondSheet(w, r, id, "sheet_science_nin")
	}
}

// ninjaneerWeaponDesignationPopupDelete is removeNinjaneerWeaponDesignation's
// own popup-flavored wrapper — redirects to popupPath instead of swapping
// the Core sheet's sheet_science_nin fragment. Not built on
// subclassTrackerPopupDelete's shared factory (subclass_tracker_popup.go),
// since a weapon designation's own delete also needs to clear its
// character_weapon_attack_options contribution, unlike every other category
// that factory covers.
func (s *server) ninjaneerWeaponDesignationPopupDelete(tier ninjaneerWeaponDesignationTier, popupPath func(int64) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCharacterID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		invIDStr := strings.TrimSpace(r.FormValue("option_slug"))
		if status, msg := s.removeNinjaneerWeaponDesignation(tier, id, invIDStr); status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		http.Redirect(w, r, popupPath(id), http.StatusSeeOther)
	}
}

// removeNinjaneerWeaponDesignation clears one tier's designation — freely,
// at any time — and resets whatever character_weapon_attack_options
// contribution this mechanism had applied to the now-former weapon, so no
// stale bonus lingers on it. Shared by handleNinjaneerWeaponDesignationDelete
// and ninjaneerWeaponDesignationPopupDelete.
func (s *server) removeNinjaneerWeaponDesignation(tier ninjaneerWeaponDesignationTier, id int64, invIDStr string) (int, string) {
	inventoryID, err := strconv.ParseInt(invIDStr, 10, 64)
	if invIDStr == "" || err != nil {
		return http.StatusBadRequest, "missing weapon"
	}
	if err := s.clearNinjaneerWeaponOverride(id, inventoryID, tier.FinesseIntDex); err != nil {
		log.Println("clear ninjaneer weapon override:", err)
		return http.StatusInternalServerError, "database error"
	}
	if err := charstore.RemoveScienceNinSubclassPick(s.charDB, id, tier.Category, invIDStr); err != nil {
		log.Println("remove ninjaneer weapon designation:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleSheetNinjaneerLegendaryWeaponToggle toggles Warrior of Science's own
// Legendary Weapon activation state (form field "on", "1" or "0") — same
// sheet-toggle-form shape handleSheetExoskeletonToggle uses. Unlike
// Exoskeleton, this state feeds nothing in internal/charsheet.Compute: the
// +2 attack roll, doubled Weapon Property, THP/damage-reduction bypass and
// force-conversion effects all stay manual/narrated (see this file's own
// header doc and the sheet_science_nin template's rules-text hint) — only
// the CCD spend-gate itself (20 to activate, 10/turn upkeep, tracked
// nowhere numerically either) and this on/off state are surfaced.
func (s *server) handleSheetNinjaneerLegendaryWeaponToggle(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := charstore.SetNinjaneerLegendaryWeaponActive(s.charDB, id, r.FormValue("on") == "1"); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set ninjaneer legendary weapon active:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}
