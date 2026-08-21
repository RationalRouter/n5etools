package main

import (
	"database/sql"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// Science-Nin's own mechanics, starting with the base class's Scientific
// Ninja Tools catalog — the Creation Points budget (class_level_resources,
// 4 @2nd -> 40 @20th) every subclass's own tiered upgrade catalog (S.N.B
// Upgrades, Shinobi-Ware Upgrades, Arsenal Modifications, ...) will
// eventually spend from too, so this file builds the shared plumbing
// (catalog loading, Cost/Prerequisite parsing, the spend-budget picker
// shape) those later subclass-specific catalogs are expected to reuse
// rather than re-derive.
//
// Scientific Ninja Tools (class_options list_name of the same name, 6
// tiers: Minor/Refined/Greater/Superior/Supreme/Mastercraft) bundles
// several individually named tools into each tier's own description field
// — split into class_option_entries rows at ingest time (see
// internal/store/classoptionentries.go), the same treatment Puppet
// Master's own Upgrade tiers get. Each entry's own text opens with a
// "Cost: N Creation Points" line (occasionally preceded by "Prerequisite:
// <name>", occasionally followed by "Drain: N CCD Chakra") — parsed here,
// not hand-transcribed, so a future rules update to any entry's own text
// is picked up by the next ingest rather than needing a matching manual
// edit to a hardcoded table.
//
// A Prerequisite is enforced (gates the entry out of Available until the
// referenced tool is already known) only when its text exactly matches
// another Scientific Ninja Tools entry's own name — Ricocheting Weapon's
// "Prerequisite: Thrown Weapon (consumed on creation)" references an
// equipment item this catalog has no row for at all, and Enhanced Aero
// Amplifier's own "Prerequisite: Pyro Amplifier" is a source-book naming
// slip (no tool named "Pyro Amplifier" exists anywhere in this catalog —
// the entry's own body text confirms it means "your Aero Amplifier").
// Both are left informational-only (shown in the entry's own description,
// never silently corrected or dropped) rather than guessed at, the same
// "don't force an ill-fitting pattern" restraint namedEntryPattern's own
// doc argues for.
//
// Everything else this class's own base-class/subclass features grant —
// Full-Metal Shinobi's unarmored AC override, Yhprum's Law, Chakra Cell
// Enhancement's bonus jutsu slots, Calculated Response/The Right Tool's own
// spend pools, every subclass's own companion (S.N.B, Titan) and tiered
// upgrade catalog — is documented, not modeled here; see CLASS_AUDIT.md's
// Science-Nin detail entry for the full per-item breakdown and which stage
// of this build each is scoped to.
const scienceNinSlug = "class/science-nin"

// scienceNinExoskeletonFeatureSlug is Elemental Innovationist's 3rd-level
// Exoskeleton feature. Mirrors internal/charsheet's own unexported
// elementalInnovationistExoskeletonFeatureSlug const — that package can't
// be imported back from here (charsheet doesn't import cmd/n5e, but the
// slug string itself is duplicated rather than exported solely for this,
// the same "small string, not worth a cross-package API" call
// scienceNinSlug/charsheet.scienceNinClassSlug already made).
const scienceNinExoskeletonFeatureSlug = "class/science-nin/group/scientific-inquiry/elemental-innovationist/feature/exoskeleton"

// airTrecksWeaponSlug is the equipment row Storm Rider's own 3rd-level Air
// Trecks feature grants for free (migration 0050_air_trecks_weapon.sql) —
// no starting-equipment catalog entry exists for it anywhere, since the
// Trecks are built by the character, not bought.
const airTrecksWeaponSlug = "weapon/air-trecks"

// scienceNinInfusedGeniusFeatureSlug is Infused Genius, an 11th-level BASE
// class feature (unlike the ElementalInnovationist/Grenadier/... fields
// further down this file, none of which apply here — every Science-Nin
// subclass reaches this one on the same base-class progression). See
// loadScienceNinTabData for where it's populated.
const scienceNinInfusedGeniusFeatureSlug = "class/science-nin/feature/infused-genius"

// scienceNinMixedStudiesFeatureSlug is Mixed Studies, an 18th-level BASE
// class feature: "You gain the 3rd Level features of another Scientific
// Inquiry. You cannot select the one you chose at 3rd Level." Like Infused
// Genius above, every subclass reaches it on the same base-class
// progression.
const scienceNinMixedStudiesFeatureSlug = "class/science-nin/feature/mixed-studies"

// Mixed Studies second-3rd-level-feature slugs. Every one of Science-Nin's
// ten real Scientific Inquiries grants exactly two 3rd-level features
// (confirmed against rules.db's own v_subclass_features) — one of each
// pair already has its own named constant, in this file or
// science_nin_subclasses.go/custom_resources.go, because something else in
// this codebase keys off it (a catalog picker, an AC override, a custom-
// resource pool). These eight are the pair's OTHER half: nothing else in
// this app reads them, they exist purely so mixedStudiesInquiryOptions
// (below) doesn't embed bare literal strings.
const (
	scienceNinExplosiveTendenciesFeatureSlug  = "class/science-nin/group/scientific-inquiry/grenadier/feature/explosive-tendencies"
	scienceNinOrdnanceTrainingFeatureSlug     = "class/science-nin/group/scientific-inquiry/mech-crafter/feature/ordnance-training"
	scienceNinAdaptiveMovementFeatureSlug     = "class/science-nin/group/scientific-inquiry/mech-crafter/feature/adaptive-movement"
	scienceNinScientificNinjaBeastFeatureSlug = "class/science-nin/group/scientific-inquiry/s-n-b-specialist/feature/scientific-ninja-beast"
	// scienceNinFullMetalShinobiFeatureSlug mirrors internal/charsheet's own
	// unexported shinobiWareFullMetalShinobiFeatureSlug — same cross-package
	// duplication scienceNinExoskeletonFeatureSlug above already established
	// (that package can't be imported back into this one).
	scienceNinFullMetalShinobiFeatureSlug = "class/science-nin/group/scientific-inquiry/shinobi-ware/feature/full-metal-shinobi"
	scienceNinGhostInTheShellFeatureSlug  = "class/science-nin/group/scientific-inquiry/spyware/feature/ghost-in-the-shell"
	scienceNinWingRoadFeatureSlug         = "class/science-nin/group/scientific-inquiry/storm-rider/feature/wing-road"
	scienceNinSENTsFeatureSlug            = "class/science-nin/group/scientific-inquiry/technobi/feature/s-e-n-ts"
)

// mixedStudiesInquiryOption is one of the ten real Scientific Inquiries
// Mixed Studies can copy from, paired with the two feature slugs
// v_subclass_features grants that Inquiry's own holders at 3rd level.
type mixedStudiesInquiryOption struct {
	SubclassSlug string
	Name         string
	FeatureSlugs [2]string
}

// mixedStudiesInquiryOptions: Science-Nin's ten real Scientific Inquiries.
// "S.N.B Upgrades" is a phantom 11th subclasses row left over from a
// since-fixed ingest bug (0048_snb_upgrades_phantom_subclass.sql) and is
// never a legal Mixed Studies pick, even against a rules.db copy still
// carrying the stale row — this list is hand-curated rather than read live
// from the subclasses table specifically to exclude it unconditionally,
// the same "known static list, no DB round trip needed" precedent
// scienceNinRegaliaOptions (science_nin_subclasses.go) already sets.
//
// Mech Crafter is included despite its own Titan companion staying
// unbuilt: Mixed Studies only ever grants a subclass's PRINTED 3rd-level
// feature rows into the granted-features list — the same bare display a
// native 3rd-level Mech Crafter already gets, with no further automation
// either way. mergeMixedStudiesFeatures (below) is the only reader of
// FeatureSlugs; loadScienceNinTabData's own Mixed Studies picker only
// needs SubclassSlug/Name.
var mixedStudiesInquiryOptions = []mixedStudiesInquiryOption{
	{"class/science-nin/group/scientific-inquiry/elemental-innovationist", "Elemental Innovationist", [2]string{scienceNinExoskeletonFeatureSlug, scienceNinEIPFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/grenadier", "Grenadier", [2]string{scienceNinExplosiveTendenciesFeatureSlug, scienceNinBIMFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/mad-scientist", "Mad Scientist", [2]string{madScientistBioticMasteryFeatureSlug, scienceNinInversionSerumsFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/mech-crafter", "Mech Crafter", [2]string{scienceNinOrdnanceTrainingFeatureSlug, scienceNinAdaptiveMovementFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/ninjaneer", "Ninjaneer", [2]string{scienceNinArsenalFeatureSlug, scienceNinPerfectedWeaponFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/s-n-b-specialist", "S.N.B Specialist", [2]string{scienceNinScientificNinjaBeastFeatureSlug, scienceNinSNBUpgradesFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/shinobi-ware", "Shinobi-Ware", [2]string{scienceNinFullMetalShinobiFeatureSlug, scienceNinEdgeRunnerFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/spyware", "Spyware", [2]string{scienceNinGhostInTheShellFeatureSlug, scienceNinCruelAngelsThesisFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/storm-rider", "Storm Rider", [2]string{scienceNinAirTrecksFeatureSlug, scienceNinWingRoadFeatureSlug}},
	{"class/science-nin/group/scientific-inquiry/technobi", "Technobi", [2]string{scienceNinBestLaidTrapFeatureSlug, scienceNinSENTsFeatureSlug}},
}

// scienceNinSubclassSlug resolves the character's own chosen Scientific
// Inquiry, if any — mirrors medicalNinSubclassSlug/hunterNinSubclassSlug
// exactly. Used by Mixed Studies to exclude the character's own native
// 3rd-level subclass from the Available list ("You cannot select the one
// you chose at 3rd Level").
func (s *server) scienceNinSubclassSlug(characterID int64) (slug, name string, err error) {
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
		if classSlug == scienceNinSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// mergeMixedStudiesFeatures appends synthetic granted-feature rows for a
// Mixed Studies pick (base class, 18th level) onto an already-loaded
// granted-features list — the chosen OTHER Scientific Inquiry's own two
// 3rd-level feature rows, looked up live against rules.db for Name/
// Description rather than hand-transcribed, same restraint this file's own
// header doc already applies to the Scientific Ninja Tools catalog.
// SourceLabel reads "Mixed Studies: <Inquiry>" rather than "Subclass:
// <Inquiry>" so the Core tab's Features & Traits panel doesn't read as
// though the character actually took that subclass.
//
// Called from BOTH loadGrantedFeatures choke points — this one
// (loadGrantedFeatures, characters.go) and internal/charsheet.Compute's own
// independent copy of this same function — rather than appended only at
// display call sites the way hunterNinPatternPassiveRows is: these ARE
// real subclass_features rows, and every one of science_nin_subclasses.go's
// own catalog pickers, internal/charsheet's AC overrides, and
// custom_resources.go's Biotic Mastery CCD split all gate purely on a
// slug's PRESENCE in the granted-features list (see
// science_nin_subclasses.go's own header doc) — folding the injection into
// loadGrantedFeatures itself is what reaches every one of those consumers
// without having to touch each call site individually.
//
// A no-op when the character has no Mixed Studies pick stored, or doesn't
// currently qualify for the real Mixed Studies feature itself (a delevel
// below 18 drops the bonus the same way every other subclass section here
// already disappears once its own gating feature stops matching). Whatever
// the picked Inquiry's own 3rd-level mechanics still depend on (Mech
// Crafter's Titan, S.N.B Specialist's own companion) stays exactly as
// unbuilt as it already is for a native holder — this only widens who can
// reach the mechanics that already exist, it builds nothing new.
func (s *server) mergeMixedStudiesFeatures(characterID int64, granted []grantedFeatureRow) ([]grantedFeatureRow, error) {
	hasMixedStudies := false
	for _, f := range granted {
		if f.Slug == scienceNinMixedStudiesFeatureSlug {
			hasMixedStudies = true
			break
		}
	}
	if !hasMixedStudies {
		return granted, nil
	}
	picks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickMixedStudiesInquiry)
	if err != nil {
		return nil, err
	}
	if len(picks) == 0 {
		return granted, nil
	}
	var opt *mixedStudiesInquiryOption
	for i, o := range mixedStudiesInquiryOptions {
		if o.SubclassSlug == picks[0].OptionSlug {
			opt = &mixedStudiesInquiryOptions[i]
			break
		}
	}
	if opt == nil {
		return granted, nil // stale pick pointing at a since-renamed/removed subclass slug
	}
	for _, slug := range opt.FeatureSlugs {
		var name, description string
		if err := s.rulesDB.QueryRow(`SELECT name, description FROM v_subclass_features WHERE slug = ?`, slug).Scan(&name, &description); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}
		granted = append(granted, grantedFeatureRow{
			Slug:        slug,
			Name:        name,
			Description: description,
			SourceLabel: "Mixed Studies: " + opt.Name,
			Level:       18,
		})
	}
	return granted, nil
}

// mixedStudiesInquiryDescription builds the Mixed Studies picker's own
// rollover-tooltip text for one Inquiry option — both of its own 3rd Level
// features' names and full body text, read live from rules.db rather than
// hand-transcribed (same restraint this file's own header doc already
// applies to the Scientific Ninja Tools catalog). Also reused by
// handleScienceNinSubclassPickDetail's "mixed-studies" branch
// (science_nin_subclasses.go) for the click-to-open popup, so the tooltip
// and the popup never drift apart.
func (s *server) mixedStudiesInquiryDescription(opt mixedStudiesInquiryOption) string {
	var parts []string
	for _, slug := range opt.FeatureSlugs {
		var name, description string
		if err := s.rulesDB.QueryRow(`SELECT name, description FROM v_subclass_features WHERE slug = ?`, slug).Scan(&name, &description); err != nil {
			continue
		}
		parts = append(parts, name+": "+description)
	}
	return strings.Join(parts, "\n\n")
}

// ensureAirTrecksGranted grants the Air Trecks weapon into a Storm Rider's
// own inventory the first time their sheet is loaded after gaining the
// feature — equipped from the start, and with its attack/damage ability
// pinned to Intelligence via a stored WeaponAttackOptions override (Air
// Trecks' own text: "you use your Intelligence modifier for the attack and
// damage roll instead of Strength or Dexterity" — overriding the Finesse
// property's own usual "better of Strength/Dexterity" default that
// buildAttacks, characters.go, would otherwise apply). A no-op once the
// character already has any "weapon/air-trecks" row, however it got there
// (granted once, then manually deleted and re-added, or duplicated by
// hand from the item library) — the same "grant once, then trust the
// player" boundary character creation's own one-shot equipment grant
// (charstore.SetEquipment) already draws.
func (s *server) ensureAirTrecksGranted(characterID int64) error {
	var exists bool
	if err := s.charDB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM character_inventory WHERE character_id = ? AND item_slug = ?)`,
		characterID, airTrecksWeaponSlug,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	res, err := s.charDB.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES (?, ?, 1, 1)`,
		characterID, airTrecksWeaponSlug)
	if err != nil {
		return err
	}
	invID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	return charstore.SetWeaponAttackOptions(s.charDB, characterID, charstore.WeaponAttackOptions{
		InventoryID:   invID,
		AttackAbility: "int",
	})
}

// ensureScienceNinAutoGrants runs every one-time, side-effecting grant this
// class hands out purely for having a feature (currently just Storm Rider's
// Air Trecks weapon) BEFORE a caller loads inventory/builds the attack
// table for the same request. loadScienceNinSubclassData (called later, by
// loadScienceNinTabData, while assembling the Science-Nin sheet box's own
// data) performs the identical grant itself — harmless to run twice
// (ensureAirTrecksGranted's own existence check makes it a no-op the
// second time), but by then the SAME request's own inventory read and
// buildAttacks call have already run, so a character's very first page
// load right after gaining Storm Rider would otherwise show the newly
// granted Air Trecks nowhere in the Attacks & Jutsu table until a second
// reload. Calling this once, early, alongside the sheet's own
// charsheet.Compute call, closes that window — every caller that builds
// the attack table from a freshly loaded inventory should call this first.
func (s *server) ensureScienceNinAutoGrants(characterID int64, sheet *charsheet.Sheet) error {
	granted, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return err
	}
	for _, f := range granted {
		if f.Slug == scienceNinAirTrecksFeatureSlug {
			return s.ensureAirTrecksGranted(characterID)
		}
	}
	return nil
}

// scienceNinCCDMax is the Chakra Containment Device's own base formula (15
// chakra per Science-Nin level) — extracted from custom_resources.go's own
// "class/science-nin/feature/chakra-containment-device" grant so Mad
// Scientist's Biotic Mastery split (custom_resources.go's
// madScientistCCDSplit) can recompute the identical total without
// duplicating the formula.
func scienceNinCCDMax(scienceNinLevel int) int {
	return scienceNinLevel * 15
}

// scienceNinClassLevel returns the character's own Science-Nin class level,
// or 0 if they have none — mirrors hunterNinClassLevel/
// ninjutsuSpecialistClassLevel exactly.
func (s *server) scienceNinClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, scienceNinSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// scienceNinAdaptiveMovementJutsuSlugs: the two named jutsu Mech Crafter's
// Adaptive Movement (3rd level) both grants as known (jutsu_grants.go's
// loadGrantedJutsuLabels) and lets its owner cast off CCD chakra instead of
// the normal Chakra pool, at the jutsu's own full printed cost — "You can
// cast these jutsu using chakra from your CCD." Confirmed against
// dist/rules.db's v_jutsu: both resolve to real jutsu rows with a numeric
// cost_chakra (3 each).
var scienceNinAdaptiveMovementJutsuSlugs = map[string]bool{
	"jutsu/body-flicker":   true,
	"jutsu/chakra-leaping": true,
}

// scienceNinAdaptiveMovementResourceKey must match custom_resources.go's own
// Key for the base "class/science-nin/feature/chakra-containment-device"
// grant (Chakra Containment Device).
const scienceNinAdaptiveMovementResourceKey = "ccd"

// scienceNinAdaptiveMovementJutsuGrantsForCharacter is Adaptive Movement's
// own version of genjutsuMirageJutsuGrantsForCharacter (genjutsu.go)/
// hunterNinJutsuGrantsForCharacter (hunter_nin.go) — a jutsu whose cast can
// be paid from a rest-scoped pool instead of Chakra. Unlike either of those,
// the pool spent (CCD) is itself measured in Chakra and the feature's own
// text pays the jutsu's FULL printed cost out of it rather than a fixed
// per-cast use count or a half/free reduction, so this returns a plain set
// of qualifying jutsu slugs rather than a struct carrying its own Cost/
// UsesPerCast — loadCharacterJutsuSheet (characters.go) computes both
// directly off each jutsu's own printed cost_chakra, already in scope there
// for the identical reason Malleable Mirages' own half-cost mode is.
//
// Unconditional once granted (not a player pick), so this reads
// loadGrantedFeatures directly rather than a stored pick — same "always-on
// feature" shape genjutsuPledgeJutsuGrantsForCharacter (genjutsu.go) already
// establishes.
func (s *server) scienceNinAdaptiveMovementJutsuGrantsForCharacter(characterID int64, clanSlug string, classLevel int) (map[string]bool, error) {
	granted, err := s.loadGrantedFeatures(characterID, clanSlug, classLevel)
	if err != nil {
		return nil, err
	}
	if !hasFeature(granted, scienceNinAdaptiveMovementFeatureSlug) {
		return nil, nil
	}
	out := make(map[string]bool, len(scienceNinAdaptiveMovementJutsuSlugs))
	for slug := range scienceNinAdaptiveMovementJutsuSlugs {
		out[slug] = true
	}
	return out, nil
}

// scienceNinToolStatLinePattern peels a Scientific Ninja Tools entry's own
// leading "(Prerequisite: <name> )?Cost: <n> Creation Points?(Drain: <n>
// CCD Chakra)?" stat line off the front of its class_option_entries
// description — the same PDF text-extraction shape splitLeadingStatLine
// (format.go) already parses for RENDER-time display, but resolved here
// into real Cost/Drain integers and a Prerequisite string a picker can
// gate on, not just a display string. Confirmed against every one of the
// 33 real Scientific Ninja Tools entries in rules.db before writing this
// pattern — "Cost: 2 Creation Point" (singular, Minor tier only) vs "Cost:
// 4 Creation Points" (plural, every other tier) is why the unit word is
// optional-plural rather than assumed always-plural, and Superior tier's
// own "Secondary Chakra Containment Device" has no Drain line at all,
// which is why that whole group is optional too.
var scienceNinToolStatLinePattern = regexp.MustCompile(
	`^(?:Prerequisite: (.+?) )?Cost: (\d+)(?:-\d+)? Creation Points?(?: Drain: (\d+) CCD Chakra)?\s*`)

// parseScienceNinToolStatLine splits one entry's raw description into its
// parsed Cost/Drain/Prerequisite plus the remaining prose (everything after
// the stat line) — cost is 0 and rest is the untouched original string when
// the description doesn't start with a recognized stat line at all (not
// observed in the live catalog, but a parse failure should degrade to
// "always affordable, no gate" rather than silently vanishing the entry).
func parseScienceNinToolStatLine(description string) (cost, drain int, prereq, rest string) {
	m := scienceNinToolStatLinePattern.FindStringSubmatch(description)
	if m == nil {
		return 0, 0, "", description
	}
	prereq = strings.TrimSpace(m[1])
	cost, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		drain, _ = strconv.Atoi(m[3])
	}
	rest = strings.TrimSpace(description[len(m[0]):])
	return cost, drain, prereq, rest
}

// scienceNinToolOption is one individually named Scientific Ninja Tools
// entry.
type scienceNinToolOption struct {
	Slug        string // class_option_entries.slug
	Name        string
	Tier        string // "Minor" | "Refined" | "Greater" | "Superior" | "Supreme" | "Mastercraft"
	Cost        int    // Creation Points
	Drain       int    // CCD Chakra to activate; 0 = no drain stated
	Prereq      string // raw "Prerequisite:" text, "" if none — see this file's header doc on when this is enforced vs. informational-only
	PrereqSlug  string // resolved sibling entry slug when Prereq exactly names one, "" otherwise
	Description string // body text with the leading stat line already stripped
}

// knownScienceNinTool is one entry on the Known Tools list.
type knownScienceNinTool struct {
	Slug string
	Name string
	Tier string
	Cost int
}

// loadScientificNinjaToolCatalog reads every Scientific Ninja Tools entry
// (all 6 tiers) in catalog (tier, then in-tier) order, parsing each one's
// own Cost/Drain/Prerequisite out of its class_option_entries description.
func (s *server) loadScientificNinjaToolCatalog() ([]scienceNinToolOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT e.slug, e.name, e.description, o.name
		FROM class_option_entries e
		JOIN class_options o ON o.slug = e.class_option_slug
		WHERE o.class_slug = ? AND o.list_name = 'Scientific Ninja Tools'
		ORDER BY o.sort_order, e.sort_order`, scienceNinSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []scienceNinToolOption
	byName := map[string]int{} // lowercased name -> index into out, for prereq resolution below
	for rows.Next() {
		var o scienceNinToolOption
		var rawDescription string
		if err := rows.Scan(&o.Slug, &o.Name, &rawDescription, &o.Tier); err != nil {
			return nil, err
		}
		o.Cost, o.Drain, o.Prereq, o.Description = parseScienceNinToolStatLine(rawDescription)
		byName[strings.ToLower(o.Name)] = len(out)
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		if out[i].Prereq == "" {
			continue
		}
		if j, ok := byName[strings.ToLower(out[i].Prereq)]; ok {
			out[i].PrereqSlug = out[j].Slug
		}
	}
	return out, nil
}

// scienceNinToolsTabData is the sheet_science_nin box's full data.
type scienceNinToolsTabData struct {
	CreationPointsCap  int
	CreationPointsUsed int
	KnownTools         []knownScienceNinTool
	AvailableTools     []scienceNinToolOption
	// Exoskeleton is non-nil only for a character with Elemental
	// Innovationist's own Exoskeleton feature granted — nil hides the
	// donned/doffed toggle entirely rather than showing it disabled, same
	// "real DOM removal" treatment every other subclass-gated box on this
	// sheet gets.
	Exoskeleton *scienceNinExoskeletonData

	// InfusedGenius is non-nil only once the character has the 11th-level
	// Infused Genius feature — a BASE-class pick (every subclass reaches
	// it), unlike every field below which is gated on one specific
	// subclass's own granting feature.
	InfusedGenius *scienceNinInfusedGeniusData

	// MixedStudies is non-nil only once the character has the 18th-level
	// Mixed Studies feature — another BASE-class pick, same shape as
	// InfusedGenius above. See mergeMixedStudiesFeatures for how the picked
	// Inquiry's own 3rd Level feature rows actually reach the rest of the
	// sheet (AC overrides, subclass catalog pickers, ...) once chosen here.
	MixedStudies *scienceNinMixedStudiesData

	// ElementalInnovationist/Grenadier/MadScientist/Ninjaneer/ShinobiWare/
	// Spyware/StormRider/Technobi/SNBSpecialist are each non-nil only for a
	// character with that subclass's own 3rd-level granting feature
	// present — same "real DOM removal" treatment Exoskeleton gets above,
	// extended to all eight subclass catalogs built in
	// cmd/n5e/science_nin_subclasses.go. Mech Crafter (Titan) has no field
	// here — its own Titan companion + Titan Upgrades catalog is deferred
	// alongside the Scientific Ninja Beast companion, see CLASS_AUDIT.md's
	// Science-Nin detail entry.
	ElementalInnovationist *scienceNinElementalInnovationistData
	Grenadier              *scienceNinGrenadierData
	MadScientist           *scienceNinMadScientistData
	Ninjaneer              *scienceNinNinjaneerData
	ShinobiWare            *scienceNinShinobiWareData
	Spyware                *scienceNinSpywareData
	StormRider             *scienceNinStormRiderData
	Technobi               *scienceNinTechnobiData
	SNBSpecialist          *scienceNinSNBSpecialistData
}

// scienceNinExoskeletonData backs the Exoskeleton donned/doffed toggle —
// see internal/charsheet.Sheet.ExoskeletonDonned and migration
// 0043_exoskeleton_donned.sql.
type scienceNinExoskeletonData struct {
	Donned bool
}

// scienceNinInfusedGeniusData backs Infused Genius (11th level, base
// class): "You can select one Scientific Ninja Tool of Creation Point Cost
// 8 or lower... Twice per long rest the holder of this weapon or wearer of
// the armor can use the tool... at no chakra cost. You can have a number
// of Infused Tools equal to your Intelligence modifier." Available/Known
// are restricted to the character's own already-known Scientific Ninja
// Tools (scienceNinToolsTabData.KnownTools, filtered to Cost <= 8) — the
// same "restricted to already-known subset via cross-reference" shape
// Perma Perk/Quick Hack (science_nin_subclasses.go) already use for their
// own overlays. Known/Available both use scienceNinSubclassOption's shape
// (Slug/Name/Tier) purely so this reuses handleScienceNinSubclassPickAdd/
// Delete, the same generic factory every subclass catalog in
// science_nin_subclasses.go shares — no new picker mechanism.
//
// WHICH weapon/armor a given Infused Tool is attached to, and "the
// holder/wearer (not just you) can use it," both stay fully manual — no
// item-slot binding or other-creature action economy exists anywhere in
// this app. Only the cap-gated pick itself (which tools are Infused) is
// tracked here; the twice-per-long-rest USE budget is a separate
// customResourceGrants pool (custom_resources.go, Key "infused_tools").
type scienceNinInfusedGeniusData struct {
	Cap       int
	Used      int
	Known     []knownScienceNinPick
	Available []scienceNinSubclassOption
}

// scienceNinMixedStudiesData backs Mixed Studies (18th level, base class):
// a single-slot, freely re-picked choice of another Scientific Inquiry
// (see mixedStudiesInquiryOptions), reusing knownScienceNinPick/
// scienceNinSubclassOption's shape — same "cap 1, freely re-picked" shape
// Regalia/Perma Perk/Quick Hack already use — purely so this reuses
// handleScienceNinSubclassPickAdd/Delete, the same generic factory every
// other catalog in this file/science_nin_subclasses.go shares. Picking a
// different Inquiry means deleting the current pick first, same "trust the
// player" boundary every other pick in this package draws.
//
// Available excludes the character's own native 3rd-level subclass (the
// book's own "You cannot select the one you chose at 3rd Level"). What the
// picked Inquiry's own 3rd-level feature rows actually DO once chosen is
// resolved by mergeMixedStudiesFeatures, not here — this struct only backs
// the picker UI itself.
type scienceNinMixedStudiesData struct {
	Picked    *knownScienceNinPick
	Available []scienceNinSubclassOption
}

// loadScienceNinTabData returns nil for a character with no Science-Nin
// levels, or none yet at 2nd (Scientific Ninja Tools' own unlock level, the
// first level class_level_resources charts a Creation Points value) — the
// template gates the whole box's existence on this being non-nil, same
// treatment MartialDefense gets.
func (s *server) loadScienceNinTabData(characterID int64, sheet *charsheet.Sheet) (*scienceNinToolsTabData, error) {
	level, err := s.scienceNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if level == 0 {
		return nil, nil
	}
	cap, err := s.classLevelResourceInt(scienceNinSlug, "Creation Points", level)
	if err != nil {
		return nil, err
	}
	if cap == 0 {
		return nil, nil
	}

	data := &scienceNinToolsTabData{CreationPointsCap: cap}

	granted, err := s.loadGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}
	for _, f := range granted {
		if f.Slug == scienceNinExoskeletonFeatureSlug {
			data.Exoskeleton = &scienceNinExoskeletonData{Donned: sheet.ExoskeletonDonned}
			break
		}
	}
	// Future of Shinobi: Shinobi-Ware (20th level, Shinobi-Ware subclass):
	// "You also double your maximum creation points." Applied here, before
	// anything below reads data.CreationPointsCap, so both the sheet's own
	// display and handleScienceNinToolAdd's server-side spend-budget check
	// (which calls this same function) see the doubled cap from the one
	// field both read — no second, separate doubling exists anywhere else.
	// The capstone's own extra upgrade-slot bonus (+Intelligence modifier,
	// Shinobi-Ware Upgrades only) is applied in
	// loadScienceNinSubclassData/science_nin_subclasses.go instead.
	for _, f := range granted {
		if f.Slug == scienceNinFutureOfShinobiShinobiWareFeatureSlug {
			data.CreationPointsCap *= 2
			break
		}
	}
	// Elemental Innovationist/Grenadier/Mad Scientist/Ninjaneer's own
	// subclass catalogs — see cmd/n5e/science_nin_subclasses.go.
	if err := s.loadScienceNinSubclassData(characterID, sheet, level, granted, data); err != nil {
		return nil, err
	}

	catalog, err := s.loadScientificNinjaToolCatalog()
	if err != nil {
		return nil, err
	}
	picks, err := charstore.ListScienceNinTools(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	pickedSet := make(map[string]bool, len(picks))
	for _, slug := range picks {
		pickedSet[slug] = true
	}

	for _, o := range catalog {
		switch {
		case pickedSet[o.Slug]:
			data.KnownTools = append(data.KnownTools, knownScienceNinTool{Slug: o.Slug, Name: o.Name, Tier: o.Tier, Cost: o.Cost})
			data.CreationPointsUsed += o.Cost
		case o.PrereqSlug != "" && !pickedSet[o.PrereqSlug]:
			// Prerequisite tool not yet known — stays out of Available until
			// it is, same "gate on owning the prerequisite" rule the entry's
			// own text states.
		default:
			data.AvailableTools = append(data.AvailableTools, o)
		}
	}

	for _, f := range granted {
		if f.Slug != scienceNinInfusedGeniusFeatureSlug {
			continue
		}
		ig := &scienceNinInfusedGeniusData{}
		if intMod := sheet.Abilities["int"].Modifier; intMod > 0 {
			ig.Cap = intMod
		}
		infusedPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickInfusedTool)
		if err != nil {
			return nil, err
		}
		infusedSet := make(map[string]bool, len(infusedPicks))
		for _, p := range infusedPicks {
			infusedSet[p.OptionSlug] = true
		}
		ig.Used = len(infusedPicks)
		for _, t := range data.KnownTools {
			if t.Cost > 8 {
				continue // Infused Genius: "Cost 8 or lower" only
			}
			if infusedSet[t.Slug] {
				ig.Known = append(ig.Known, knownScienceNinPick{Slug: t.Slug, Name: t.Name, Tier: t.Tier})
				continue
			}
			ig.Available = append(ig.Available, scienceNinSubclassOption{Slug: t.Slug, Name: t.Name, Tier: t.Tier})
		}
		data.InfusedGenius = ig
		break
	}

	for _, f := range granted {
		if f.Slug != scienceNinMixedStudiesFeatureSlug {
			continue
		}
		ms := &scienceNinMixedStudiesData{}
		nativeSlug, _, err := s.scienceNinSubclassSlug(characterID)
		if err != nil {
			return nil, err
		}
		msPicks, err := charstore.ListScienceNinSubclassPicks(s.charDB, characterID, charstore.ScienceNinPickMixedStudiesInquiry)
		if err != nil {
			return nil, err
		}
		pickedSlug := ""
		if len(msPicks) > 0 {
			pickedSlug = msPicks[0].OptionSlug
		}
		for _, opt := range mixedStudiesInquiryOptions {
			if opt.SubclassSlug == nativeSlug {
				continue // "You cannot select the one you chose at 3rd Level"
			}
			if opt.SubclassSlug == pickedSlug {
				pick := knownScienceNinPick{Slug: opt.SubclassSlug, Name: opt.Name}
				ms.Picked = &pick
				continue
			}
			ms.Available = append(ms.Available, scienceNinSubclassOption{
				Slug:        opt.SubclassSlug,
				Name:        opt.Name,
				Description: s.mixedStudiesInquiryDescription(opt),
			})
		}
		data.MixedStudies = ms
		break
	}

	return data, nil
}

// handleScienceNinToolAdd buys one Scientific Ninja Tools entry, gated by
// the character's own remaining Creation Points budget (not a flat slot
// count — every other cap+catalog picker in this codebase gates on count,
// this is the first one gated on a spent/budget total instead).
func (s *server) handleScienceNinToolAdd(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing tool", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for science-nin tool add:", err)
		return
	}
	data, err := s.loadScienceNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin for tool add:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no Scientific Ninja Tools budget yet", http.StatusBadRequest)
		return
	}
	var picked *scienceNinToolOption
	for i, o := range data.AvailableTools {
		if o.Slug == slug {
			picked = &data.AvailableTools[i]
			break
		}
	}
	if picked == nil {
		http.Error(w, "not a valid tool to learn", http.StatusBadRequest)
		return
	}
	if data.CreationPointsUsed+picked.Cost > data.CreationPointsCap {
		http.Error(w, "not enough Creation Points remaining", http.StatusBadRequest)
		return
	}
	if err := charstore.AddScienceNinTool(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add science-nin tool:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// handleScienceNinToolDelete drops one known tool — freely, at any time,
// same "trust the player" boundary every other pick removal on this sheet
// draws.
func (s *server) handleScienceNinToolDelete(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "missing tool", http.StatusBadRequest)
		return
	}
	if err := charstore.RemoveScienceNinTool(s.charDB, id, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("remove science-nin tool:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}

// handleScienceNinToolDetail serves the click-to-open popup for a Known
// tool — same mechanism handleHunterPickDetail/handleNinjutsuMoldingDetail
// already use, reusing the same hunter_pick_detail_card template. Not
// character-scoped — the catalog is static rules content.
func (s *server) handleScienceNinToolDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var name, description string
	err := s.rulesDB.QueryRow(`
		SELECT e.name, e.description FROM class_option_entries e
		JOIN class_options o ON o.slug = e.class_option_slug
		WHERE e.slug = ? AND o.class_slug = ? AND o.list_name = 'Scientific Ninja Tools'`,
		slug, scienceNinSlug,
	).Scan(&name, &description)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load science-nin tool detail:", err)
		return
	}

	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render science-nin tool detail: no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "hunter_pick_detail_card", map[string]any{"Name": name, "Description": description}); err != nil {
		log.Println("render science-nin tool detail:", err)
	}
}

// handleSheetExoskeletonToggle toggles Elemental Innovationist's Exoskeleton
// donned/doffed state (form field "on", "1" or "0") — same sheet-toggle-form
// shape handleSheetInspiration uses, responding with sheet_science_nin (the
// fragment the toggle button itself lives in, so its own hidden "on" value
// and label text land in sync) rather than sheet_ac. AC — the number
// donning/doffing actually changes (internal/charsheet.Compute swaps
// Intelligence in for Dexterity while donned and light/medium armor is
// equipped) — lives in a different box entirely, so the template's own
// data-also-refresh="sheet-ac" is what keeps that number correct; returning
// sheet_ac here instead would leave this toggle's own hidden field stale,
// silently breaking every click after the first (see sheet-toggles.js: the
// hidden "on" value only self-corrects from the swapped-in HTML of
// data-target, never from data-also-refresh).
func (s *server) handleSheetExoskeletonToggle(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := charstore.SetExoskeletonDonned(s.charDB, id, r.FormValue("on") == "1"); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set exoskeleton donned:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_science_nin")
}
