package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Weapon Specialist's Weapon Focus feature (class_features, level 1) is the
// one piece of this class with a real numeric bonus reaching an existing
// tracked field: buildAttacks' AttackBonus/DamageBonus. Each of the 8
// Weapon Forms also grants two mechanics at 3rd level: "[Form] Techniques"
// (2-3 bespoke Flurry Techniques, always known, no player choice — see
// weaponFormTechniqueAutoGrants) and "[Form] Styles" (a real pick from a
// 4-6 option list, capped by class_level_resources' "Styles Known" — see
// loadWeaponFormTabData). Everything else this class grants is either
// Group 2 (conditional/activated Flurry Techniques and Styles themselves —
// spend-a-die-for-a-temporary-effect, the same classification
// internal/puppetupgrades established) or Group 3 with no receiving
// field/catalog:
//
//   - Weapon Stance (Chapter 13 "Weapon Stances" catalog) has zero
//     class_options rows in rules.db — confirmed via a direct query, not
//     assumed. Building this picker needs a new ingestion pass; deferred,
//     same "explicit, not silently dropped" treatment CLASS_AUDIT.md gives
//     Puppet Master's Life-Like Puppetry.
//   - The Weapon Specialist Bonus Trait Chart (the trait Weapon Focus also
//     grants, alongside its numeric bonus) maps onto existing weapon
//     PROPERTY names (Blocking, Deadly, Disarm, ...), but this app doesn't
//     mechanically simulate weapon properties anywhere — they're reference
//     text on the item card, not a field an engine reads — so granting one
//     has nothing to change even once picked. Left as reference text.
//   - Extra Attack/Superior Attack (attacks-per-turn) and Critical Focus
//     (crit-range ranks) have no computed field anywhere in this app for
//     ANY class, Weapon Specialist included — not a gap unique to this
//     audit pass, the project-wide boundary already drawn for how many
//     attacks a turn grants and where a crit range is tracked.
const weaponSpecialistSlug = "class/weapon-specialist"

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

// loadWeaponFocusCatalog lists every weapon in the equipment catalog as a
// candidate Weapon Focus type — the book lets a Weapon Specialist choose
// any weapon type "like katana" they'll specialize into, not just ones
// currently in their own inventory.
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
}

// loadWeaponFocusTabData returns nil for a character with no Weapon
// Specialist levels — the template gates the whole box's existence on this
// being non-nil, same treatment MartialTechniques gets.
func (s *server) loadWeaponFocusTabData(characterID int64) (*weaponFocusTabData, error) {
	level, err := s.weaponSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if level == 0 {
		return nil, nil
	}

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
		} else {
			available = append(available, o)
		}
	}

	return &weaponFocusTabData{
		Cap:       charstore.WeaponFocusSlotCap(level),
		Used:      len(known),
		Bonus:     charstore.WeaponFocusBonus(level),
		DieSize:   flurryDieSize(level),
		Known:     known,
		Available: available,
	}, nil
}

// weaponFocusBonusSet returns the character's chosen Weapon Focus weapon
// slugs and the shared bonus to apply to each, for buildAttacks to
// consult — nil set (bonus irrelevant) for a character with no Weapon
// Focus picks, same "empty means untouched" shape opt-in overrides
// elsewhere in buildAttacks already use.
func (s *server) weaponFocusBonusSet(characterID int64) (map[string]bool, int, error) {
	level, err := s.weaponSpecialistClassLevel(characterID)
	if err != nil || level == 0 {
		return nil, 0, err
	}
	picks, err := charstore.ListWeaponFocus(s.charDB, characterID)
	if err != nil || len(picks) == 0 {
		return nil, 0, err
	}
	set := make(map[string]bool, len(picks))
	for _, slug := range picks {
		set[slug] = true
	}
	return set, charstore.WeaponFocusBonus(level), nil
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
	data, err := s.loadWeaponFocusTabData(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon focus for add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no levels in Weapon Specialist", http.StatusBadRequest)
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

	return &weaponFormTabData{
		FormName:   subclassName,
		FlurryDie:  flurryDieSize(level),
		Techniques: techniques,
		Cap:        cap,
		Used:       len(picks),
		Known:      known,
		Available:  available,
	}, nil
}

// handleWeaponFormStyleAdd learns one Style, gated by the character's own
// current cap — server-side, defense in depth regardless of what the UI
// already disables.
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
	if slug == "" {
		http.Error(w, "missing style", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for weapon form style add:", err)
		return
	}
	data, err := s.loadWeaponFormTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon form styles for add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no Weapon Form chosen", http.StatusBadRequest)
		return
	}
	if data.Used >= data.Cap {
		http.Error(w, "no style slots remaining", http.StatusBadRequest)
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
		http.Error(w, "not a valid style to learn", http.StatusBadRequest)
		return
	}
	if err := charstore.AddWeaponFormStyle(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add weapon form style:", err)
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
