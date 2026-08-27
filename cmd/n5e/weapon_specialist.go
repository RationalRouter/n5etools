package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// Weapon Specialist's Weapon Focus feature (class_features, level 1) is the
// one piece of this class with a real numeric bonus reaching an existing
// tracked field: buildAttacks' AttackBonus/DamageBonus. Two feats
// (feat/class/expanded-focus, feat/class/weapon-arts-expert) add bonus
// Weapon Focus slots on top of the class-level schedule — see
// weaponFocusBonusSlots below for both feats' exact text and the
// repeatable-take limitation on Expanded Focus. Each of the 8
// Weapon Forms also grants two mechanics at 3rd level: "[Form] Techniques"
// (2-3 bespoke Flurry Techniques, always known, no player choice — see
// weaponFormTechniqueAutoGrants) and "[Form] Styles" (a real pick from a
// 4-6 option list, capped by class_level_resources' "Styles Known" — see
// loadWeaponFormTabData). Slayer Form's own 6th-level Stalking Predator
// (a freely re-selectable pick between its two named Predator Exploits —
// see stalkingPredatorOptions) and the base class's own 14th/18th-level
// Superior Weapon Flurry (a cap-gated 2-of-4-then-3-of-4 benefit pick —
// see superiorWeaponFlurryCatalog) are both tracked the same way: WHICH
// option is selected is a real Group 1 pick, even though the option's own
// triggered effect stays manual/narrated. Everything else this class
// grants is either Group 2 (conditional/activated Flurry Techniques and
// Styles themselves — spend-a-die-for-a-temporary-effect, the same
// classification internal/puppetupgrades established) or Group 3 with no
// receiving field/catalog:
//
//   - Weapon Stance (2nd level, base class) IS implemented, alongside
//     Weapon Focus in the same sheet_weapon_focus box below — it picks
//     from fighting_stances WHERE stance_type='bukijutsu' (12 rows, the
//     same table Taijutsu Stance already consumes for stance_type=
//     'taijutsu'), reusing taijutsu.go's own stanceOption/
//     fightingStanceView/storeFightingStanceChoice machinery via
//     loadStanceOptionsByType("bukijutsu"). An earlier pass's claim that
//     this had "zero structured data" was checking class_options, which is
//     the wrong table for Chapter 13 reference catalogs — fighting_stances
//     is class-agnostic and had the 12 Weapon Stance rows the whole time.
//   - The Weapon Specialist Bonus Trait Chart (the trait Weapon Focus also
//     grants, alongside its numeric bonus) maps onto existing weapon
//     PROPERTY names (Blocking, Deadly, Disarm, ...), but this app doesn't
//     mechanically simulate weapon properties anywhere — they're reference
//     text on the item card, not a field an engine reads — so granting one
//     has nothing to change even once picked. Left as reference text.
//   - Extra Attack/Superior Attack (attacks-per-turn) has no computed field
//     anywhere in this app for ANY class, Weapon Specialist included — not a
//     gap unique to this audit pass, the project-wide boundary already
//     drawn for how many attacks a turn grants. Critical Focus IS tracked
//     (weaponSpecialistCritRangeThreshold, characters.go) and reaches both
//     weapon attacks (attackRow.CritRangeThreshold) and Bukijutsu-classified
//     jutsu casts (jutsuSheetRow.CritRangeThreshold).
const weaponSpecialistSlug = "class/weapon-specialist"

// weaponStanceFeatureSlug identifies Weapon Stance (class_features, 2nd
// level, base class — not subclass-gated) as the granting feature for
// storeFightingStanceChoice's own ChoiceKey, same role
// unarmedTechniqueFeatureSlug plays for Taijutsu Stance.
const weaponStanceFeatureSlug = "class/weapon-specialist/feature/weapon-stance"

// weaponSpecialistClassLevel returns the character's own Weapon Specialist
// class level, or 0 if they have none — mirrors taijutsuSpecialistClassLevel.
func (s *server) weaponSpecialistClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, weaponSpecialistSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// weaponSpecialistSubclassSlug resolves the character's own chosen Weapon
// Form, if any — mirrors taijutsuSpecialistSubclassSlug exactly (a
// character can have subclasses from more than one class under
// multiclassing, so this scans character_subclasses for the one whose
// parent class is Weapon Specialist rather than assuming there's only one
// row).
func (s *server) weaponSpecialistSubclassSlug(characterID int64) (slug, name string, err error) {
	subRows, err := s.charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return "", "", err
	}
	var subclassSlugs []string
	for subRows.Next() {
		var sc string
		if err := subRows.Scan(&sc); err != nil {
			subRows.Close()
			return "", "", err
		}
		subclassSlugs = append(subclassSlugs, sc)
	}
	subRows.Close()
	if err := subRows.Err(); err != nil {
		return "", "", err
	}

	for _, sc := range subclassSlugs {
		var n, classSlug string
		if err := s.rulesDB.QueryRow(`
			SELECT sc.name, g.class_slug FROM subclasses sc
			JOIN subclass_groups g ON g.slug = sc.group_slug
			WHERE sc.slug = ?`, sc,
		).Scan(&n, &classSlug); err != nil {
			continue // a stale/removed subclass slug just isn't a match
		}
		if classSlug == weaponSpecialistSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// weaponFormTechniqueAutoGrants: feature slug -> the bespoke Flurry
// Technique names it grants for free. Every one of the 8 Weapon Forms has
// an identical 3rd-level "[Form] Techniques" feature (Samurai's own is
// named "Kenjutsu Technique") reading "you learn additional Flurry
// Techniques" followed by 2-3 named techniques with no player choice —
// these have no class_options catalog row of their own; their full text
// already renders wherever the granting feature's own description is
// shown, so this map only needs names, for listing them on the sheet as
// Granted: auto-known, no cap, no delete button — same SourceLabel
// boundary loadGrantedJutsuLabels/martialTechniqueAutoGrants already
// established.
var weaponFormTechniqueAutoGrants = map[string][]string{
	"class/weapon-specialist/group/weapon-forms/battle-dancer-form/feature/battle-techniques-changed":            {"Disastrous Strike", "Forced Regression"},
	"class/weapon-specialist/group/weapon-forms/gungnir-piercer-form/feature/gungnir-piercer-techniques-changed": {"Fenrir’s Claw", "Heimdallr Vision"},
	"class/weapon-specialist/group/weapon-forms/obsidian-hammer-form/feature/obsidian-hammer-techniques-changed": {"Syphon Strike", "Unmend"},
	"class/weapon-specialist/group/weapon-forms/phantom-blade-form/feature/phantom-blade-techniques-changed":     {"Phantom’s Edge", "Phantoms Eclipse"},
	"class/weapon-specialist/group/weapon-forms/primal-weapon-form/feature/primal-weapon-techniques-changed":     {"Primal Reverb", "Primal Strike", "Primal Pulse"},
	"class/weapon-specialist/group/weapon-forms/ranger-form/feature/ranger-techniques-changed":                   {"Blinding Shot", "Brutal Shot", "Crippling Shot"},
	"class/weapon-specialist/group/weapon-forms/samurai-form/feature/kenjutsu-technique-changed":                 {"Frenetic Draw", "Riposte"},
	"class/weapon-specialist/group/weapon-forms/slayer-form/feature/slayer-techniques-changed":                   {"Studied Crippling", "Studied Strike"},
}

// flurryDieSize is Weapon Flurry's own die progression (class_level_
// resources, "Flurry Die": d4 through 4th, d6 through 8th, d8 through
// 12th, d10 through 16th, d12 from 17th). Unlike Martial Dice, the book
// gives Flurry Die no separate charge-count column — Flurry Techniques are
// limited "once per turn," not by a spendable pool — so this is
// informational only, same boundary unarmedDamageDieSize/martialDieSize
// already draw for a die-size-only progression.
func flurryDieSize(level int) string {
	switch {
	case level >= 17:
		return "d12"
	case level >= 13:
		return "d10"
	case level >= 9:
		return "d8"
	case level >= 5:
		return "d6"
	default:
		return "d4"
	}
}

// weaponFocusOption is one weapon catalog entry offered (or already chosen)
// as a Weapon Focus type.
type weaponFocusOption struct {
	Slug       string
	Name       string
	DamageDice string
	DamageType string
}

// weaponFocusGripPairs maps the four Versatile weapons whose grips print as
// two separate equipment rows (migration 0021: Quarterstaff, Spear, Taichi,
// Tetsubo — e.g. "weapon/tetsubo" and "weapon/tetsubo-two-hands") between
// each pair's two slugs, in both directions. A Weapon Focus pick names one
// specific slug, but "chooses a weapon type (like katana)" (the feature's
// own text) means a pick against either grip covers the same physical
// weapon in the other. Built as an explicit lookup table rather than a bare
// "-two-hands" suffix trim so an unrelated future weapon slug can never
// accidentally match.
var weaponFocusGripPairs = func() map[string]string {
	pairs := map[string]string{
		"weapon/quarterstaff": "weapon/quarterstaff-two-hands",
		"weapon/spear":        "weapon/spear-two-hands",
		"weapon/taichi":       "weapon/taichi-two-hands",
		"weapon/tetsubo":      "weapon/tetsubo-two-hands",
	}
	out := make(map[string]string, len(pairs)*2)
	for a, b := range pairs {
		out[a] = b
		out[b] = a
	}
	return out
}()

// loadWeaponFocusCatalog lists every weapon in the equipment catalog as a
// candidate Weapon Focus type — the book lets a Weapon Specialist choose
// any weapon type "like katana" they'll specialize into, not just ones
// currently in their own inventory.
//
// This deliberately does NOT exclude the "(Two Hands)" grip-duplicate rows
// loadWeaponOptions (characters.go) drops for the same reason — a character
// who already has one of the four pairs' two-handed grip slug recorded as
// their pick (from before this dedup existed, or from data entered before
// this list existed at all) still needs that row present here to resolve
// its name for display. loadWeaponFocusTabData is what keeps a NEW pick
// from ever choosing a duplicate grip row; see its own Available filter.
func (s *server) loadWeaponFocusCatalog() ([]weaponFocusOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, COALESCE(damage_dice, ''), COALESCE(damage_type, '')
		FROM equipment WHERE kind = 'weapon' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []weaponFocusOption
	for rows.Next() {
		var o weaponFocusOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.DamageDice, &o.DamageType); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// weaponFocusTabData is the sheet_weapon_focus box's full data.
type weaponFocusTabData struct {
	Cap       int
	Used      int
	Bonus     int
	DieSize   string
	Known     []weaponFocusOption
	Available []weaponFocusOption
	// Stance is Weapon Stance's own pick (2nd level, base class — every
	// Weapon Specialist at that level, not subclass-gated), nil below
	// level 2 same as every other level-gated sub-section elsewhere on the
	// sheet.
	Stance *fightingStanceView
}

// expandedFocusFeatSlug and weaponArtsExpertFeatSlug are the two feats that
// grant a bonus Weapon Focus slot on top of WeaponFocusSlotCap's
// class-level schedule — see weaponFocusBonusSlots.
const (
	expandedFocusFeatSlug    = "feat/class/expanded-focus"
	weaponArtsExpertFeatSlug = "feat/class/weapon-arts-expert"
)

// weaponFocusBonusSlots returns the bonus Weapon Focus slots granted by
// feats, on top of charstore.WeaponFocusSlotCap's class-level schedule.
// This app doesn't enforce feat prerequisites anywhere (prerequisites are
// shown on the feat's card as reference text only — see characters.go), so
// both grants below apply purely off whether the feat is taken, regardless
// of whether the character's build actually satisfies the book's stated
// prerequisite for it.
//
//   - feat/class/expanded-focus ("Select one weapon type... This chosen
//     weapon becomes known as your Weapon Focus... You can take this Feat
//     more than once, selecting either a new weapon type, or the same
//     weapon you previously selected") grants one slot per copy taken —
//     but character_feats' primary key is (character_id, feat_slug), so
//     this app has no mechanism to record the same feat slug more than
//     once per character. Only a single copy is ever reflected here; a
//     character who narratively took this feat twice still only gets +1,
//     not +2. This is a real, documented limitation, not a bug — tracking
//     repeatable feats is a bigger change than this slot count itself and
//     is out of scope here.
//   - feat/class/weapon-arts-expert ("Select one weapon type you have
//     proficiency with... The chosen weapon becomes your Weapon Focus")
//     grants one slot outright. Its prerequisite chain (Weapon Arts
//     Training, itself gated on the character NOT having Weapon Specialist
//     levels) means this is frequently a non-Weapon-Specialist's ONLY
//     Weapon Focus slot — WeaponFocusSlotCap alone returns 0 for a
//     level-0 Weapon Specialist, so both callers below add this bonus on
//     top of the class-level cap rather than gating on class level alone.
//     This feat's other two picks (a Flurry Technique, a Weapon Trait) are
//     not modeled — no catalog/field exists for either pick's own
//     mechanical effect yet.
func weaponFocusBonusSlots(grantedFeatures []grantedFeatureRow) int {
	bonus := 0
	if hasGrantedFeature(grantedFeatures, expandedFocusFeatSlug) {
		bonus++
	}
	if hasGrantedFeature(grantedFeatures, weaponArtsExpertFeatSlug) {
		bonus++
	}
	return bonus
}

// weaponFocusTotalBonus mirrors charstore.WeaponFocusBonus's own "the
// shared bonus equals the number of Weapon Focus slots" rule, once bonus
// slots from feats are folded into the slot count too — capped at +3 per
// Expanded Focus's own text ("A Weapon Focus can only have a +3 bonus to
// Attack and Damage rolls as a result of being a Weapon Focus"), even when
// the total slot count itself climbs past 3.
func weaponFocusTotalBonus(totalSlots int) int {
	if totalSlots > 3 {
		return 3
	}
	return totalSlots
}

// loadWeaponFocusTabData returns nil for a character with no Weapon
// Specialist levels AND no feat-granted Weapon Focus slot — the template
// gates the whole box's existence on this being non-nil, same treatment
// MartialTechniques gets.
func (s *server) loadWeaponFocusTabData(characterID int64, sheet *charsheet.Sheet) (*weaponFocusTabData, error) {
	level, err := s.weaponSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	bonusSlots := weaponFocusBonusSlots(grantedFeatures)
	if level == 0 && bonusSlots == 0 {
		return nil, nil
	}
	totalSlots := charstore.WeaponFocusSlotCap(level) + bonusSlots

	catalog, err := s.loadWeaponFocusCatalog()
	if err != nil {
		return nil, err
	}
	picks, err := charstore.ListWeaponFocus(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	pickedSet := make(map[string]bool, len(picks))
	for _, slug := range picks {
		pickedSet[slug] = true
	}

	var known, available []weaponFocusOption
	for _, o := range catalog {
		if pickedSet[o.Slug] {
			known = append(known, o)
			continue
		}
		// A "(Two Hands)" row is never offered as a fresh pick — it is the
		// same physical weapon as its one-handed sibling row (already in
		// this same catalog), so offering both would read as two different
		// weapons and would let a player burn a second slot on one they
		// already have covered. weaponFocusGripPairs is what makes a pick
		// against either grip's slug apply the shared bonus to both.
		if strings.HasSuffix(o.Name, "(Two Hands)") {
			continue
		}
		available = append(available, o)
	}

	data := &weaponFocusTabData{
		Cap:       totalSlots,
		Used:      len(known),
		Bonus:     weaponFocusTotalBonus(totalSlots),
		DieSize:   flurryDieSize(level),
		Known:     known,
		Available: available,
	}

	if level >= 2 {
		choices, err := features.LoadFeatureChoices(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		stanceOptions, err := s.loadStanceOptionsByType("bukijutsu")
		if err != nil {
			return nil, err
		}
		data.Stance = buildFightingStanceView(choices, stanceOptions, weaponStanceFeatureSlug)
	}

	return data, nil
}

// weaponFocusBonusSet returns the character's chosen Weapon Focus weapon
// slugs and the shared bonus to apply to each, for buildAttacks to
// consult — nil set (bonus irrelevant) for a character with no Weapon
// Focus picks, same "empty means untouched" shape opt-in overrides
// elsewhere in buildAttacks already use. sheet is the caller's own
// already-computed sheet (buildAttacks already has one), consulted only
// for ClanSlug/Level to resolve feat-granted bonus slots via
// weaponFocusBonusSlots.
func (s *server) weaponFocusBonusSet(characterID int64, sheet *charsheet.Sheet) (map[string]bool, int, error) {
	level, err := s.weaponSpecialistClassLevel(characterID)
	if err != nil {
		return nil, 0, err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, 0, err
	}
	bonusSlots := weaponFocusBonusSlots(grantedFeatures)
	if level == 0 && bonusSlots == 0 {
		return nil, 0, nil
	}
	picks, err := charstore.ListWeaponFocus(s.charDB, characterID)
	if err != nil || len(picks) == 0 {
		return nil, 0, err
	}
	set := make(map[string]bool, len(picks)*2)
	for _, slug := range picks {
		set[slug] = true
		// A pick recorded against either grip's slug (see
		// weaponFocusGripPairs) covers the same physical weapon in the
		// other, so buildAttacks' exact-slug match against the equipped
		// item's own slug finds it regardless of which grip is equipped.
		if sibling, ok := weaponFocusGripPairs[slug]; ok {
			set[sibling] = true
		}
	}
	totalSlots := charstore.WeaponFocusSlotCap(level) + bonusSlots
	return set, weaponFocusTotalBonus(totalSlots), nil
}

// enhancedStrikesFeatureSlug identifies Samurai Form's Enhanced Strikes
// (subclass_features, 13th level, confirmed verbatim against rules.db
// 2026-08-26): "Starting at 13th Level, your weapon is constantly being
// fed your chakra, enhancing its offensive potential. Whenever you hit a
// creature with a weapon attack using a weapon you have as your Weapon
// Focus, the creature takes extra damage equal [to] 1 flurry die."
//
// Master of Focus (20th level, same subclass) separately states "Enhanced
// Strikes now counts for Bukijutsu that use chosen weapon" — extending
// this same bonus die to Bukijutsu jutsu casts using the Weapon Focus
// weapon. That extension is NOT modeled here: loadCharacterJutsuSheet's
// jutsu rows have no equivalent of buildAttacks' per-item Weapon-Focus-slug
// match to gate it against, and Master of Focus's own capstone payload
// (customResourceGrants' master_of_the_primal_uses-shaped pool, resistance,
// and the rest of its many clauses) is already documented as manual/Group 3
// elsewhere in this class's audit — left as a follow-up rather than folded
// into this pass.
const enhancedStrikesFeatureSlug = "class/weapon-specialist/group/weapon-forms/samurai-form/feature/enhanced-strikes"

// weaponSpecialistEnhancedStrikesDice returns the dice-notation bonus damage
// ("1d10") buildAttacks should add on top of a Weapon-Focus-weapon attack's
// own damage roll once the character has Enhanced Strikes — "" for a
// character without it (no Weapon Specialist levels, or Weapon Specialist
// levels but not this Samurai Form feature). The die itself is Weapon
// Flurry's own die-size progression (flurryDieSize) at the character's own
// Weapon Specialist class level, matching the feature's "equal [to] 1
// flurry die" text — the same die-size chart weaponFocusTabData.DieSize
// already surfaces for display. sheet is the caller's own already-computed
// sheet, consulted only for ClanSlug/Level to resolve the merged granted-
// features list, the same role it plays in weaponFocusBonusSet above.
func (s *server) weaponSpecialistEnhancedStrikesDice(characterID int64, sheet *charsheet.Sheet) (string, error) {
	level, err := s.weaponSpecialistClassLevel(characterID)
	if err != nil || level == 0 {
		return "", err
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return "", err
	}
	if !hasGrantedFeature(grantedFeatures, enhancedStrikesFeatureSlug) {
		return "", nil
	}
	return "1" + flurryDieSize(level), nil
}

// handleWeaponFocusAdd learns one Weapon Focus type, gated by the
// character's own current slot cap — server-side, defense in depth
// regardless of what the UI already disables.
func (s *server) handleWeaponFocusAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("weapon_slug"))
	if slug == "" {
		http.Error(w, "missing weapon", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for weapon focus add:", err)
		return
	}
	data, err := s.loadWeaponFocusTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon focus for add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no Weapon Focus slots", http.StatusBadRequest)
		return
	}
	if data.Used >= data.Cap {
		http.Error(w, "no weapon focus slots remaining", http.StatusBadRequest)
		return
	}
	valid := false
	for _, o := range data.Available {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid weapon type to focus", http.StatusBadRequest)
		return
	}
	if err := charstore.AddWeaponFocus(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add weapon focus:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_focus")
}

// handleWeaponFocusDelete drops one chosen Weapon Focus type. The slug is a
// form field, not a URL path segment — equipment slugs contain slashes
// (e.g. "weapon/katana"), same reason handleMartialTechniqueDelete takes
// its slug as a form field instead of a {slug} path wildcard.
func (s *server) handleWeaponFocusDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("weapon_slug"))
	if slug == "" {
		http.Error(w, "missing weapon", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveWeaponFocus(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove weapon focus:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_focus")
}

// handleWeaponStance records Weapon Stance's own pick — 2nd level, base
// class (not subclass-gated), freely re-editable same boundary as Unarmed
// Technique's Taijutsu Stance in taijutsu.go. Reuses
// storeFightingStanceChoice/loadStanceOptionsByType exactly the way
// medical_nin.go's handleMedicalNinFightingStance and scout_nin.go's
// handleScoutNinFightingStance already do for their own granting features.
func (s *server) handleWeaponStance(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("stance_slug"))
	level, err := s.weaponSpecialistClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon specialist level for weapon stance:", err)
		return
	}
	if level < 2 {
		http.Error(w, "character has not reached Weapon Stance yet", http.StatusBadRequest)
		return
	}
	options, err := s.loadStanceOptionsByType("bukijutsu")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon stance options:", err)
		return
	}
	if err := s.storeFightingStanceChoice(id, weaponStanceFeatureSlug, options, slug); err != nil {
		if err == errInvalidStance {
			http.Error(w, "not a valid stance", http.StatusBadRequest)
		} else {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set weapon stance:", err)
		}
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_focus")
}

// weaponFormTechnique is one of the 2-3 bespoke Flurry Techniques a Weapon
// Form auto-grants for free at 3rd level — always known, no cap, no
// picker. Description is the granting feature's own full text (both
// techniques' names and bodies together); formatDescription's named-entry
// auto-detection splits it into readable pieces at render time, same as
// Martial Techniques' own Granted entries.
type weaponFormTechnique struct {
	Name        string
	Description string
}

// weaponFormStyleOption is one Style from the chosen Weapon Form's own
// list — either a player's own pick (on the Known list, has a remove
// button, counts against the cap) or a not-yet-known catalog entry (on the
// Available list).
type weaponFormStyleOption struct {
	Slug        string
	Name        string
	Description string
}

// slayerFormSubclassSlug is Slayer Form's own subclass slug — Stalking
// Predator (below) is gated on this specific Form, unlike Weapon Stance
// (base class, every Form shares it) or Weapon Focus/Styles (per-Form, but
// structurally identical across all 8 Forms).
const slayerFormSubclassSlug = "class/weapon-specialist/group/weapon-forms/slayer-form"

// stalkingPredatorFeatureSlug identifies Stalking Predator (subclass_
// features, Slayer Form, 6th level: "you can select one of two Predator
// Exploits you gain access to. You can switch which exploit you are using
// when you would complete a short rest.") as the granting feature for its
// own re-selectable pick — same ChoiceKey role foodForTheSoulFeatureSlug
// plays for Cooking-Nin's Food For the Soul.
const stalkingPredatorFeatureSlug = "class/weapon-specialist/group/weapon-forms/slayer-form/feature/stalking-predator"

// stalkingPredatorOptions: freely re-editable "when you would complete a
// short rest" (RAW; this app doesn't enforce rest timing, same "trust the
// player" boundary Food For the Soul already draws). Both Exploits' own
// triggered effect (adding a Flurry die to the named skill check, and
// Vipers Tongue's/Apex Glare's own follow-on effects on a good roll) stays
// manual/Group 2-3 — nothing in this app adds a die to a skill check or
// tracks Fear/Concussed ranks — so this only records WHICH Exploit is
// currently active.
var stalkingPredatorOptions = []featureChoiceOption{
	{
		Value: "vipers-tongue", Label: "Vipers Tongue",
		Description: "Skill: Deception. When you would attempt the Lie or Impersonate skill action, you can add 1 Flurry die to the check. On a success, your lie is passed off as genuine based on the context of the lie itself. If you beat the DC by 5 or more, the lie is accepted as the absolute truth and contesting thoughts or opposing truths are treated as the lie, and they must attempt to convince affected individuals of the truth.",
	},
	{
		Value: "apex-glare", Label: "Apex Glare",
		Description: "Skill: Intimidation. When you would take the Coerce or Demoralize skill action, you can add 1 Flurry die to the check. If a creature gained a rank of Fear as a result of a skill action you take, they also gain 1 rank of Concussed.",
	},
}

// stalkingPredatorView holds Slayer Form's own Stalking Predator pick —
// nil unless the character is 6th level or higher in Weapon Specialist
// with Slayer Form as their chosen Weapon Form.
type stalkingPredatorView struct {
	Current string // "vipers-tongue"/"apex-glare"/"" if unpicked
	Options []featureChoiceOption
}

// superiorWeaponFlurryOption is one of Superior Weapon Flurry's own 4
// named benefits.
type superiorWeaponFlurryOption struct {
	Slug        string
	Name        string
	Description string
}

// superiorWeaponFlurryCatalog: Superior Weapon Flurry's own 4 benefits
// (base class, 14th level: "Select two of the following benefits. You gain
// an additional benefit at 18th level.") — hardcoded here rather than
// sourced from class_options, since no rows exist there for these 4
// benefits (confirmed by direct query), the same "hardcode a small
// catalog in Go" precedent weaponFormTechniqueAutoGrants already sets for
// this class. Each benefit modifies an existing Flurry Technique's own
// duration or effect; none of that reaches a computed field anywhere in
// this app (no duration tracker, no AC-swap toggle, no Dash-effect flag),
// so only WHICH benefits are selected is tracked here — applying a
// selected benefit's effect stays manual/narrated.
var superiorWeaponFlurryCatalog = []superiorWeaponFlurryOption{
	{
		Slug: "enhanced-deflection-duration", Name: "Enhanced Deflection: Extended Duration",
		Description: "Your Enhanced Deflection Flurry Technique now lasts until the beginning of your next turn.",
	},
	{
		Slug: "enhanced-deflection-ac-swap", Name: "Enhanced Deflection: AC Option",
		Description: "Your Enhanced Deflection Flurry Technique allows you to choose to add a third of your maximum Flurry Die to your AC instead.",
	},
	{
		Slug: "perceptive-augmentation-duration", Name: "Perceptive Augmentation: Extended Duration",
		Description: "Your Perceptive Augmentation Flurry Technique now lasts until the end of your next turn.",
	},
	{
		Slug: "perceptive-augmentation-dash", Name: "Perceptive Augmentation: Dash Effect",
		Description: "Your Perceptive Augmentation Flurry Technique now also grants you the effects of the Dash action.",
	},
}

// superiorWeaponFlurryCap is Superior Weapon Flurry's own 2-of-4-then-
// 3-of-4 pick cap. Not in class_level_resources (confirmed no such row for
// this feature), so hardcoded directly rather than a rules.db lookup, same
// "hardcode a small cap in Go" precedent the catalog above sets. 0 below
// 14th level.
func superiorWeaponFlurryCap(level int) int {
	switch {
	case level >= 18:
		return 3
	case level >= 14:
		return 2
	default:
		return 0
	}
}

// superiorWeaponFlurryTabData is the sheet_weapon_form box's Superior
// Weapon Flurry sub-section — nil below 14th level in Weapon Specialist,
// same "no empty state" treatment every other cap-gated pick in this file
// gets.
type superiorWeaponFlurryTabData struct {
	Cap       int
	Used      int
	Known     []superiorWeaponFlurryOption
	Available []superiorWeaponFlurryOption
}

// loadSuperiorWeaponFlurryTabData returns nil below 14th level in Weapon
// Specialist.
func (s *server) loadSuperiorWeaponFlurryTabData(characterID int64, level int) (*superiorWeaponFlurryTabData, error) {
	cap := superiorWeaponFlurryCap(level)
	if cap == 0 {
		return nil, nil
	}
	picks, err := charstore.ListSuperiorWeaponFlurryBenefits(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	pickedSet := make(map[string]bool, len(picks))
	for _, slug := range picks {
		pickedSet[slug] = true
	}
	var known, available []superiorWeaponFlurryOption
	for _, o := range superiorWeaponFlurryCatalog {
		if pickedSet[o.Slug] {
			known = append(known, o)
		} else {
			available = append(available, o)
		}
	}
	return &superiorWeaponFlurryTabData{
		Cap:       cap,
		Used:      len(known),
		Known:     known,
		Available: available,
	}, nil
}

// weaponFormTabData is the sheet_weapon_form box's full data — nil for a
// character with no Weapon Specialist levels, or who hasn't chosen a
// Weapon Form yet (both Techniques and Styles are tied to a specific Form,
// not the base class).
type weaponFormTabData struct {
	FormName   string
	FlurryDie  string
	Techniques []weaponFormTechnique
	Cap        int
	Used       int
	Known      []weaponFormStyleOption
	Available  []weaponFormStyleOption
	// StalkingPredator is Slayer Form's own 6th-level re-selectable
	// Predator Exploit pick — nil for any other Form, or below 6th level.
	StalkingPredator *stalkingPredatorView
	// SuperiorWeaponFlurry is the base class's own 14th/18th-level
	// cap-gated benefit pick — nil below 14th level. Not Form-specific
	// (unlike Techniques/Styles/StalkingPredator above), but rendered in
	// this same box since a character can't reach 14th level without
	// having chosen a Weapon Form first.
	SuperiorWeaponFlurry *superiorWeaponFlurryTabData
}

// loadWeaponFormTabData returns nil for a character with no Weapon
// Specialist levels or no Weapon Form chosen — the template gates the
// whole box's existence on this being non-nil, same treatment
// MartialTechniques/WeaponFocus get.
func (s *server) loadWeaponFormTabData(characterID int64, sheet *charsheet.Sheet) (*weaponFormTabData, error) {
	level, err := s.weaponSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if level == 0 {
		return nil, nil
	}
	subclassSlug, subclassName, err := s.weaponSpecialistSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	if subclassSlug == "" {
		return nil, nil
	}

	cap, err := s.classLevelResourceInt(weaponSpecialistSlug, "Styles Known", level)
	if err != nil {
		return nil, err
	}

	picks, err := charstore.ListWeaponFormStyles(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	pickedSet := make(map[string]bool, len(picks))
	for _, slug := range picks {
		pickedSet[slug] = true
	}

	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_options
		WHERE class_slug = ? AND subclass_slug = ?
		ORDER BY name`, weaponSpecialistSlug, subclassSlug)
	if err != nil {
		return nil, err
	}
	var known, available []weaponFormStyleOption
	for rows.Next() {
		var o weaponFormStyleOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			rows.Close()
			return nil, err
		}
		if pickedSet[o.Slug] {
			known = append(known, o)
		} else {
			available = append(available, o)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	var techniques []weaponFormTechnique
	for _, f := range grantedFeatures {
		if names, ok := weaponFormTechniqueAutoGrants[f.Slug]; ok {
			for _, name := range names {
				techniques = append(techniques, weaponFormTechnique{Name: name, Description: f.Description})
			}
		}
	}
	sort.Slice(techniques, func(i, j int) bool { return techniques[i].Name < techniques[j].Name })

	var stalkingPredator *stalkingPredatorView
	if subclassSlug == slayerFormSubclassSlug && level >= 6 {
		choices, err := features.LoadFeatureChoices(s.charDB, characterID)
		if err != nil {
			return nil, err
		}
		stalkingPredator = &stalkingPredatorView{
			Current: choices[features.ChoiceKey{FeatureSlug: stalkingPredatorFeatureSlug, ChoiceIndex: 0}],
			Options: stalkingPredatorOptions,
		}
	}

	superiorWeaponFlurry, err := s.loadSuperiorWeaponFlurryTabData(characterID, level)
	if err != nil {
		return nil, err
	}

	return &weaponFormTabData{
		FormName:             subclassName,
		FlurryDie:            flurryDieSize(level),
		Techniques:           techniques,
		Cap:                  cap,
		Used:                 len(picks),
		Known:                known,
		Available:            available,
		StalkingPredator:     stalkingPredator,
		SuperiorWeaponFlurry: superiorWeaponFlurry,
	}, nil
}

// addWeaponFormStyle validates and stores one Style pick, gated by the
// character's own current cap — server-side, defense in depth regardless of
// what the UI already disables. Shared by handleWeaponFormStyleAdd's own
// Core-sheet AJAX route and the Weapon Form popup's own route
// (weapon_form_popup.go), so both share one validation path.
func (s *server) addWeaponFormStyle(id int64, slug string) (int, string) {
	if slug == "" {
		return http.StatusBadRequest, "missing style"
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		log.Println("compute sheet for weapon form style add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadWeaponFormTabData(id, sheet)
	if err != nil {
		log.Println("load weapon form styles for add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil {
		return http.StatusBadRequest, "character has no Weapon Form chosen"
	}
	if data.Used >= data.Cap {
		return http.StatusBadRequest, "no style slots remaining"
	}
	valid := false
	for _, o := range data.Available {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		return http.StatusBadRequest, "not a valid style to learn"
	}
	if err := charstore.AddWeaponFormStyle(s.charDB, id, slug); err != nil {
		log.Println("add weapon form style:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleWeaponFormStyleAdd is addWeaponFormStyle's own Core-sheet AJAX
// wrapper.
func (s *server) handleWeaponFormStyleAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if status, msg := s.addWeaponFormStyle(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_form")
}

// handleWeaponFormStyleDelete drops one known Style. The slug is a form
// field, not a URL path segment — class_options slugs contain slashes,
// same reason handleMartialTechniqueDelete takes its slug as a form field
// instead of a {slug} path wildcard.
func (s *server) handleWeaponFormStyleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if slug == "" {
		http.Error(w, "missing style", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveWeaponFormStyle(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove weapon form style:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_form")
}

// setStalkingPredator validates and stores Slayer Form's own Stalking
// Predator pick (one of stalkingPredatorOptions) — reuses
// charstore.SetFeatureChoice exactly the way handleFoodForTheSoul does for
// Cooking-Nin's own re-selectable pick. Shared by handleStalkingPredator's
// own Core-sheet AJAX route and the Weapon Form popup's own route
// (weapon_form_popup.go).
func (s *server) setStalkingPredator(id int64, value string) (int, string) {
	valid := false
	for _, o := range stalkingPredatorOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		return http.StatusBadRequest, "not a valid pick"
	}
	level, err := s.weaponSpecialistClassLevel(id)
	if err != nil {
		log.Println("load weapon specialist level for stalking predator:", err)
		return http.StatusInternalServerError, "database error"
	}
	subclassSlug, _, err := s.weaponSpecialistSubclassSlug(id)
	if err != nil {
		log.Println("load weapon specialist subclass for stalking predator:", err)
		return http.StatusInternalServerError, "database error"
	}
	if level < 6 || subclassSlug != slayerFormSubclassSlug {
		return http.StatusBadRequest, "character has not reached Stalking Predator"
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, stalkingPredatorFeatureSlug, 0, value); err != nil {
		log.Println("set stalking predator:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleStalkingPredator is setStalkingPredator's own Core-sheet AJAX
// wrapper.
func (s *server) handleStalkingPredator(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if status, msg := s.setStalkingPredator(id, value); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_form")
}

// addSuperiorWeaponFlurryBenefit validates and selects one Superior Weapon
// Flurry benefit, gated by the character's own current cap — server-side,
// defense in depth regardless of what the UI already disables. Shared by
// handleSuperiorWeaponFlurryAdd's own Core-sheet AJAX route and the Weapon
// Form popup's own route (weapon_form_popup.go).
func (s *server) addSuperiorWeaponFlurryBenefit(id int64, slug string) (int, string) {
	if slug == "" {
		return http.StatusBadRequest, "missing benefit"
	}
	level, err := s.weaponSpecialistClassLevel(id)
	if err != nil {
		log.Println("load weapon specialist level for superior weapon flurry add:", err)
		return http.StatusInternalServerError, "database error"
	}
	data, err := s.loadSuperiorWeaponFlurryTabData(id, level)
	if err != nil {
		log.Println("load superior weapon flurry for add:", err)
		return http.StatusInternalServerError, "database error"
	}
	if data == nil {
		return http.StatusBadRequest, "character has not reached Superior Weapon Flurry"
	}
	if data.Used >= data.Cap {
		return http.StatusBadRequest, "no superior weapon flurry slots remaining"
	}
	valid := false
	for _, o := range data.Available {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		return http.StatusBadRequest, "not a valid benefit to select"
	}
	if err := charstore.AddSuperiorWeaponFlurryBenefit(s.charDB, id, slug); err != nil {
		log.Println("add superior weapon flurry benefit:", err)
		return http.StatusInternalServerError, "database error"
	}
	return http.StatusOK, ""
}

// handleSuperiorWeaponFlurryAdd is addSuperiorWeaponFlurryBenefit's own
// Core-sheet AJAX wrapper.
func (s *server) handleSuperiorWeaponFlurryAdd(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if status, msg := s.addSuperiorWeaponFlurryBenefit(id, slug); status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_form")
}

// handleSuperiorWeaponFlurryDelete drops one selected benefit. The slug is
// a form field, not a URL path segment, same reason
// handleWeaponFormStyleDelete's own slug is.
func (s *server) handleSuperiorWeaponFlurryDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("option_slug"))
	if slug == "" {
		http.Error(w, "missing benefit", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveSuperiorWeaponFlurryBenefit(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove superior weapon flurry benefit:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_weapon_form")
}
