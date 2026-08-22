package main

import (
	"database/sql"
	"log"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
	"github.com/sergio/n5e/internal/puppetupgrades"
)

// This file builds on companions.go's Puppet/Summon feature with the parts
// specific to Puppet Master's Puppets sheet tab: the Puppet Tool's baseline
// "unit card" (see internal/parse/classes.go's puppetToolUnitCardRe and
// migration 0028_puppet_tool_stat_block.sql) used to compute a fresh
// puppet's starting stats, and the Puppet Upgrade catalog/pick-tracking
// system (class_option_entries, migration 0029; character_companion_upgrades,
// migration 0019) that lets a player choose specific named upgrades instead
// of just reading a wall of bundled tier text.

// puppetToolStatBlock mirrors rules.db's puppet_tool_stat_block row — the
// parsed baseline every puppet starts from. ACBase and the five *Text
// fields (see migration 0032) are populated only by that migration, not by
// any ingest code path — the book's sidebar card prints this text as
// non-extractable graphics, confirmed absent from the PDF's own linear text
// stream, so there is nothing for a parser to read no matter how it's
// written. They're universal to every Puppet Master subclass (this table
// has exactly one row, keyed by class, not by subclass).
type puppetToolStatBlock struct {
	CreatureType        string
	ProficiencyRuleText string
	HPBase              int
	HPConBonusAdd       int
	Speed               int
	Str, Dex, Con       int
	Int, Wis, Cha       int
	PassivePerception   int
	TraitsText          string
	ACBase              int
	SavingThrowsText    string
	DamageResistance    string
	DamageImmunity      string
	ConditionImmunity   string
	WeaponProficiency   string
}

func (s *server) loadPuppetToolStatBlock() (*puppetToolStatBlock, error) {
	var sb puppetToolStatBlock
	err := s.rulesDB.QueryRow(`
		SELECT creature_type, proficiency_rule_text, hp_base, hp_con_bonus_add, speed,
		       str_score, dex_score, con_score, int_score, wis_score, cha_score,
		       passive_perception, traits_text,
		       COALESCE(ac_base, 10), COALESCE(saving_throws_text, ''),
		       COALESCE(damage_resistance_text, ''), COALESCE(damage_immunity_text, ''),
		       COALESCE(condition_immunity_text, ''), COALESCE(weapon_proficiency_text, '')
		FROM puppet_tool_stat_block WHERE class_slug = 'class/puppet-master'`,
	).Scan(&sb.CreatureType, &sb.ProficiencyRuleText, &sb.HPBase, &sb.HPConBonusAdd, &sb.Speed,
		&sb.Str, &sb.Dex, &sb.Con, &sb.Int, &sb.Wis, &sb.Cha, &sb.PassivePerception, &sb.TraitsText,
		&sb.ACBase, &sb.SavingThrowsText, &sb.DamageResistance, &sb.DamageImmunity,
		&sb.ConditionImmunity, &sb.WeaponProficiency)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sb, nil
}

// puppetToolMaxHP: baseline.HPBase + (baseline.HPConBonusAdd + the Puppet
// Master's OWN Constitution modifier) × the Puppet Master's OWN class
// level — confirmed source text uses the player character's own stats
// here, not the puppet's.
func puppetToolMaxHP(baseline puppetToolStatBlock, masterLevel, masterConMod int) int {
	return baseline.HPBase + (baseline.HPConBonusAdd+masterConMod)*masterLevel
}

// puppetToolDefaultAC: baseline.ACBase (13, "Natural Armor") + the Puppet
// MASTER's own proficiency bonus — confirmed against the book's own card
// text ("Armor Class 13 + your Proficiency Bonus (Natural Armor)"), the
// same "uses the player character's own stats" shape as puppetToolMaxHP
// above, not the puppet's own ability scores. A toolkit-computed default,
// always shown as an editable/overridable hint, never silently overwritten
// on an existing companion.
func puppetToolDefaultAC(acBase, masterProficiencyBonus int) int {
	return acBase + masterProficiencyBonus
}

// puppetMendingHealDie returns Puppet Tool's own Mending-casting die size at
// the given Puppet Master level — "restore a number of hit points equal to
// 2d6 + half your Puppet Master level. This die increases to 2d8 at 9th
// level and 2d10 at 15th level" (base class, Puppet Tool). No
// class_level_resources chart row backs this (only the 5 material-tier rows
// exist for this class), so it's a hardcoded level-threshold helper, the
// same shape puppetAlwaysPreparedBulkBonus below already uses. Reference-
// only: actually rolling and applying the heal, and the alternative "cast
// for the full duration within 5ft to heal half max HP" branch, both stay
// entirely manual — only the current die size is surfaced, as a readout on
// the Puppet Tool Traits card.
func puppetMendingHealDie(puppetMasterLevel int) string {
	switch {
	case puppetMasterLevel >= 15:
		return "2d10"
	case puppetMasterLevel >= 9:
		return "2d8"
	default:
		return "2d6"
	}
}

// Numeric-bonus autocompute audit: every Puppet Master upgrade's own
// description across all six technique lists was swept for flat numeric
// benefits (AC, HP, speed, damage, or any other "+X"), and the outcome —
// which ones are computed, which are deliberately left to the player, and
// which have no field to land in — lives in puppet_upgrade_bonuses.go
// together with the formulas themselves. Two OTHER shapes of upgrade
// benefit are wired up elsewhere and are not part of that table: a granted
// Attack row (Integrated Weapon/Mastered Armament,
// puppetIntegratedWeaponAttacks) and a granted named jutsu (Elemental
// Reactor/Weaponized Jutsu Casting/Entangling Threads,
// puppet_upgrade_jutsu_grants.go).

// puppetArmorChassis mirrors rules.db's puppet_armor_chassis row — one of
// the 4 named chassis Purple Technique's Armor Chassis feature lets a
// Juggernaut Armor be built around (see migration 0031, hand-transcribed
// from the source PDF's image-only chassis table).
type puppetArmorChassis struct {
	Slug             string
	Name             string
	Description      string
	ArmorType        string
	ACBonus          int
	DexBonusMode     string // "full", "max2", "none"
	Bulk             int
	PropertiesText   string
	PropertyTooltips []puppetArmorChassisProperty
}

// puppetArmorChassisProperty is one named property shown as a tooltip next
// to a chassis's own PropertiesText — resolved from either the
// puppet-specific puppet_armor_chassis_property table (Athletic, Mobile,
// Powerful Build, Smart, Sturdy (Renamed)) or, when not found there, the
// core book's own general armor_properties catalog (Bulky, Fashionable,
// Reinforced, Threatening all already exist there) — a chassis's property
// list mixes both sources, so both get checked rather than only seeding a
// second, duplicate copy of the general ones.
type puppetArmorChassisProperty struct {
	Name        string
	Description string
	// Value/HasValue carry the parenthetical number some properties are
	// written with ("Reinforced (4)" — the DR that property grants). It is
	// kept as a real number, not left inside the name, because Enhanced
	// Durability's Purple Technique reading raises it ("increases the
	// Reinforced property by 3"), which needs arithmetic rather than string
	// display. Properties written without one (Bulky, Mobile, Smart) have
	// HasValue false.
	Value    int
	HasValue bool
}

// Label is how the property reads on the card: its name, plus its value in
// parentheses when it has one — the same shape the source table writes.
func (p puppetArmorChassisProperty) Label() string {
	if !p.HasValue {
		return p.Name
	}
	return p.Name + " (" + strconv.Itoa(p.Value) + ")"
}

// reinforcedPropertyName is the one property with a numeric value that a
// modeled upgrade adjusts (see puppetupgrades.ChassisReinforcedBonus).
const reinforcedPropertyName = "Reinforced"

// adjustChassisReinforced returns the property list with Reinforced's own
// value raised by bonus. A chassis without the Reinforced property is
// returned untouched — the upgrade increases a property the armor has, it
// does not grant one it doesn't (Weaved Mail and Wooden Suit have no
// Reinforced value at all).
func adjustChassisReinforced(props []puppetArmorChassisProperty, bonus int) []puppetArmorChassisProperty {
	if bonus == 0 {
		return props
	}
	out := make([]puppetArmorChassisProperty, len(props))
	copy(out, props)
	for i := range out {
		if out[i].Name == reinforcedPropertyName && out[i].HasValue {
			out[i].Value += bonus
		}
	}
	return out
}

// puppetArmorChassisTokenRe splits one chassis's comma-separated
// PropertiesText into a property name and its optional trailing
// parenthetical value ("Reinforced (4)" -> "Reinforced", "4"). The name
// alone is what the property catalogs are keyed by; the value is real
// per-chassis content that belongs to this armor, not to the property, and
// is captured rather than discarded so an upgrade that raises it has a
// number to work with (see adjustChassisReinforced).
var puppetArmorChassisTokenRe = regexp.MustCompile(`^(.*?)\s*\(([^)]*)\)\s*$`)

// loadPuppetArmorChassisOptions loads every chassis option, each with its
// own properties resolved to tooltip text — shared by every puppet card on
// the tab, so loaded once rather than per-companion.
func (s *server) loadPuppetArmorChassisOptions() ([]puppetArmorChassis, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description, armor_type, ac_bonus, dex_bonus_mode, bulk, properties_text
		FROM puppet_armor_chassis ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []puppetArmorChassis
	for rows.Next() {
		var c puppetArmorChassis
		if err := rows.Scan(&c.Slug, &c.Name, &c.Description, &c.ArmorType, &c.ACBonus, &c.DexBonusMode, &c.Bulk, &c.PropertiesText); err != nil {
			return nil, err
		}
		for _, token := range strings.Split(c.PropertiesText, ",") {
			name := strings.TrimSpace(token)
			prop := puppetArmorChassisProperty{}
			if m := puppetArmorChassisTokenRe.FindStringSubmatch(name); m != nil {
				name = strings.TrimSpace(m[1])
				// A non-numeric parenthetical ("Sturdy (Renamed)") is a note
				// on the property, not a value — the name still resolves
				// without it, so it is dropped the same way it always was.
				if v, err := strconv.Atoi(strings.TrimSpace(m[2])); err == nil {
					prop.Value, prop.HasValue = v, true
				}
			}
			if name == "" {
				continue
			}
			desc, err := s.lookupArmorPropertyDescription(name)
			if err != nil {
				return nil, err
			}
			if desc != "" {
				prop.Name, prop.Description = name, desc
				c.PropertyTooltips = append(c.PropertyTooltips, prop)
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// lookupArmorPropertyDescription checks the puppet-specific property table
// first, then falls back to the core book's general armor_properties
// catalog by name — see puppetArmorChassisProperty's doc for why both.
// Returns "" (not an error) if neither table has this name, so an
// unexpected future property still renders, just without a tooltip.
func (s *server) lookupArmorPropertyDescription(name string) (string, error) {
	var desc string
	err := s.rulesDB.QueryRow(
		`SELECT description FROM puppet_armor_chassis_property WHERE name = ?`, name,
	).Scan(&desc)
	if err == nil {
		return desc, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	err = s.rulesDB.QueryRow(
		`SELECT description FROM armor_properties WHERE name = ?`, name,
	).Scan(&desc)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return desc, err
}

// puppetArmorChassisAC computes the AC a chassis's own formula gives at a
// given Dexterity score: 10 + the chassis's own AC bonus + a Dexterity
// modifier capped per its own dex_bonus_mode — the same "light/medium/heavy
// armor" shape the core book's own equipment already uses, confirmed by the
// Armor Type column each chassis prints (see migration 0031's doc comment).
func puppetArmorChassisAC(chassis puppetArmorChassis, dexScore int) int {
	dexMod := charsheet.AbilityModifier(dexScore)
	switch chassis.DexBonusMode {
	case "max2":
		if dexMod > 2 {
			dexMod = 2
		}
	case "none":
		dexMod = 0
	}
	return 10 + chassis.ACBonus + dexMod
}

// puppetMasterClassLevel returns the character's own Puppet Master class
// level, or 0 if they have none — the single query every puppet-specific
// handler in this file needs before doing anything else.
func (s *server) puppetMasterClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = 'class/puppet-master'`,
		characterID,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// puppetSubclassColorBySlug is the shared Puppet Technique color map (see
// puppetupgrades.SubclassColorBySlug) — aliased rather than duplicated, so
// the upgrade formulas in that package and every eligibility check in this
// one can never disagree about which subclass is which color.
var puppetSubclassColorBySlug = puppetupgrades.SubclassColorBySlug

// puppetUpgradeTechniquesToken is the complete closed vocabulary a
// "Techniques: X, Y, Z" line (every class_option_entries row for this class
// starts with one — the same keyword capsEntryPattern anchors the whole
// entry split on) can name: the 6 subclass colors, "All" (every subclass),
// and "Perfect" (a separate, currently-unmodeled material-tier gate, not a
// 7th subclass). Parsing against this fixed set — stop at the first token
// NOT in it — is far more robust than trying to guess where the list ends
// from the following prose's own shape (which varies: "Prerequisite:", a
// name restart, plain body text with no consistent lead-in word).
var puppetUpgradeTechniquesToken = map[string]bool{
	"All": true, "Black": true, "Blue": true, "Green": true,
	"Purple": true, "Red": true, "White": true, "Perfect": true,
}

// puppetUpgradeAvailableToColor reports whether a not-yet-taken option's own
// "Techniques:" line permits the given subclass color — "All" or the color
// itself. A description with no "Techniques:" prefix at all (every untiered
// single-option row — Puppet Weapon Types, Puppeteer Chassis, Puppet
// Frameworks, Puppet Roles — already scoped to one subclass structurally,
// via class_options.subclass_slug, with no per-option restriction of its
// own printed in the book) is always available; there is nothing to gate.
func puppetUpgradeAvailableToColor(description, color string) bool {
	rest, ok := strings.CutPrefix(description, "Techniques:")
	if !ok {
		return true
	}
	for _, field := range strings.Fields(rest) {
		// TrimRight, not TrimSuffix(",") alone: Expanded Frame's own printed
		// text has a real book typo, "Techniques: Black. Blue, Green, Red"
		// (a period instead of a comma after the first color) — confirmed
		// against the source page, not an extraction artifact. Without also
		// stripping a trailing period, "Black." fails the token check
		// immediately and the whole entry reads as available to nobody.
		name := strings.TrimRight(field, ".,")
		if !puppetUpgradeTechniquesToken[name] {
			break // end of the technique list — whatever this word starts next
		}
		if name == "All" || (color != "" && name == color) {
			return true
		}
	}
	return false
}

// puppetUpgradeOption is one pickable upgrade — either a real
// class_option_entries row (the common, tiered case) or, when a
// class_options row has no bundled entries (Blue Technique's untiered
// Puppet Weapon Types, one row per weapon type), the class_options row
// itself standing in as its own single option.
type puppetUpgradeOption struct {
	Slug        string
	Name        string
	Description string
	// PrereqMet/PrereqHint are only meaningful for a not-yet-added option
	// (see loadPuppetUpgradeTiers) — an already-added pick embeds this same
	// struct but never reads either field, since a pick that's already been
	// taken has nothing to gate. PrereqHint is empty whenever PrereqMet is
	// true OR the upgrade has no prerequisite this parser could confidently
	// resolve at all (see puppet_upgrade_prereq.go's own doc on why an
	// unparseable clause is silently dropped rather than guessed at).
	PrereqMet  bool
	PrereqHint string
	// AlreadyTaken is also only meaningful for a not-yet-added option: this
	// companion has already picked this exact entry as many times as
	// puppetUpgradeMaxTakes allows (1, for every upgrade except Piercing
	// Chakra), so selecting it again in a tier with slots still open would
	// silently double up on the same named upgrade instead of offering a
	// genuinely different choice.
	AlreadyTaken bool
}

// puppetUpgradePick is one taken upgrade, carrying its own
// character_companion_upgrades row id so the sheet's delete button has
// something to post to, plus its own repeatable sub-choices if
// puppetUpgradeSubChoiceSpecs has an entry for it (SubChoiceQuantity == 0
// when it doesn't — nothing to render).
type puppetUpgradePick struct {
	ID int64
	puppetUpgradeOption
	SubChoiceQuantity int
	SubChoiceOptions  []puppetUpgradeSubChoiceOption
	SubChoicePicks    []puppetUpgradeSubChoicePick
	// Granted marks a display-only synthetic pick from puppetUpgradeAutoGrants
	// — ID is meaningless (0, no character_companion_upgrade row backs it),
	// same "auto-known, don't count against the cap, no delete button"
	// treatment martialTechniqueAutoGrants/huntersExploitAutoGrants give
	// their own free grants elsewhere in this app.
	Granted bool
}

// puppetUpgradeAutoGrants: the granting feature slug -> the ONE Puppet
// Upgrade entry slug it grants for free, outside the character's known-
// Upgrade cap. Display-only merge (see loadPuppetUpgradeTiers' own use of
// this map) — no character_companion_upgrade row gets written, since a
// full-corpus search (2026-08-05) confirmed neither entry here is
// referenced by any other entry's own "Prerequisite:" text or by
// puppet_upgrade_jutsu_grants, so nothing else needs to see a real stored
// pick for these two to work correctly.
//
// NOT a general mechanism for every "free Upgrade" grant this class has —
// several others are a meaningfully harder shape and are deliberately left
// out: Purple Technique Proficiency and Chakra String Augments each grant
// a CHOICE among several named Upgrades (not a fixed single one); Nearly
// Perfected Architecture and Elevated Design grant a free pick of a given
// TIER rather than a fixed entry; Symphony of Puppetry branches into two
// entirely different options first. See CLASS_AUDIT.md's Puppet Master
// entry for the full breakdown of what's deferred and why.
var puppetUpgradeAutoGrants = map[string]string{
	"class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/green-technique-proficiency":    "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/jutsu-channeler",
	"class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/craftsman-of-the-ninshou-creed": "class/puppet-master/option/puppet-master-upgrades/bronze-tier/entry/jutsu-specialization",
}

// puppetUpgradeFeatureChoiceGrants: the set of granted subclass feature
// slugs whose free-Upgrade grant is a CHOICE among several named entries
// (Purple Technique Proficiency, Chakra String Augments) rather than one
// fixed entry — resolved via internal/features' generic ChoiceUpgradeEntry
// slot mechanism (choiceSlotDefs, cmd/n5e/feature_choices.go) instead of
// this file's own puppetUpgradeAutoGrants. Values are unused (a set, keyed
// by the same feature slugs choiceSlotDefs itself uses) — the real option
// lists live there, not duplicated here.
var puppetUpgradeFeatureChoiceGrants = map[string]bool{
	"class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/purple-technique-proficiency": true,
	"class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/chakra-string-augments":            true,
}

// puppetUpgradeAutoGrantedEntrySlugs resolves which of puppetUpgradeAutoGrants'
// free Upgrades this character currently has. Both of today's two entries
// are Green Technique Marionettist features gated by a fixed level (Green
// Technique Proficiency, L2; Craftsman of the Ninshou Creed, L6) — read
// directly off their own printed level rather than a granted-features scan,
// the same shortcut puppetTacticCap already takes for Black Technique's own
// Tactics-cap bonus.
func puppetUpgradeAutoGrantedEntrySlugs(subclassColor string, puppetMasterLevel int) map[string]bool {
	granted := map[string]bool{}
	if subclassColor != "Green" {
		return granted
	}
	if puppetMasterLevel >= 2 {
		granted[puppetUpgradeAutoGrants["class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/green-technique-proficiency"]] = true
	}
	if puppetMasterLevel >= 6 {
		granted[puppetUpgradeAutoGrants["class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/craftsman-of-the-ninshou-creed"]] = true
	}
	return granted
}

// puppetUpgradeSubChoiceOption is one pickable value in a repeatable
// upgrade's own sub-choice list (e.g. one Poison Tag quality), resolved
// from rules.db's equipment table.
type puppetUpgradeSubChoiceOption struct {
	Slug        string
	Name        string
	Description string
	// DetailHref is this option's own browsable rules page, when it has
	// one — a Fighting Stance and an Armor Seal are both real catalog
	// entries with their own detail pages, while an Integrated Weapon's
	// "Axe Tail" or an elemental affinity is only ever a line inside the
	// upgrade that offers it. Blank means "no page to link to", and the
	// chip renders as plain text.
	DetailHref string
}

// puppetUpgradeSubChoicePick is one taken sub-choice, carrying its own
// character_companion_upgrade_choices row id for the delete button.
type puppetUpgradeSubChoicePick struct {
	ID int64
	puppetUpgradeSubChoiceOption
}

// puppetUpgradeSubChoiceSpec describes one upgrade's own repeatable
// sub-choice mechanic and where its choice list comes from — either
// EquipmentSlugs (resolved against rules.db's equipment table, e.g. "...you
// can install up to 3 Poison Tags of any quality into your Puppet Tool" —
// Black Iron Upgrades / Wood Tier's Poison Mist Hell) or FixedOptions, a
// small closed list that isn't equipment at all (Elemental Reactor's 5
// elements). Exactly one of the two is set.
type puppetUpgradeSubChoiceSpec struct {
	Quantity       int
	EquipmentSlugs []string
	FixedOptions   []puppetUpgradeSubChoiceOption
	// DynamicOptions is for a sub-choice whose available list depends on
	// character state that can change over time (Battle Ready Armor: which
	// Armor Seals the character currently qualifies for by level) — unlike
	// EquipmentSlugs' fixed list, resolved fresh on every render/add rather
	// than baked into this map. Exactly one of EquipmentSlugs/FixedOptions/
	// DynamicOptions is set per entry. pickIndex is this sub-choice's own
	// 0-based slot number (how many of this pick's sub-choices already
	// exist) — most implementations ignore it (every slot offers the same
	// list); Shade/Spellblade Framework's jutsu picks use it to gate each
	// slot to its own D/C/B-Rank tier. companionID is the specific companion
	// this pick belongs to — most implementations ignore it too (the option
	// list is the same for every companion); Bestial Framework's own D-Rank
	// trait slots use it to read back which Sage Creature THIS companion
	// already picked, since that (unlike everything else DynamicOptions
	// backs so far) varies per companion, not just per character.
	DynamicOptions func(s *server, characterID, companionID int64, pickIndex int) ([]puppetUpgradeSubChoiceOption, error)
}

// elementalReactorEntrySlug: Purple Technique's Wood Tier "Elemental
// Reactor" upgrade ("You fit your Puppet Tool with a special elemental
// reactor which enables you to cast jutsu of a specific element... You can
// take this upgrade multiple times, each time selecting a different
// element"). Modeled as a repeatable entry, one character_companion_upgrades
// row per element taken, each carrying exactly one sub-choice (this map,
// Quantity: 1) recording which element. The element grants the CHARACTER
// the table's own specific named jutsu directly at levels 2/6/10/14 (see
// puppet_upgrade_jutsu_grants.go) — NOT a standing elemental affinity
// (the earlier reading of "gaining the jutsu associated with it" as "now
// eligible to learn them" was wrong; the book's own table, hand-transcribed
// since it isn't PDF-extractable text, names specific jutsu outright).
const elementalReactorEntrySlug = "class/puppet-master/option/armorers-upgrades/wood-tier/entry/elemental-reactor"

// integratedWeaponEntrySlug: Purple Technique's Wood Tier "Integrated
// Weapon" upgrade ("Select one of the following options [Arm Blades, Axe
// Tail]... You can take this upgrade a second time, to gain the other").
// Same repeatable-entry-with-a-unique-sub-choice-per-pick shape as
// Elemental Reactor above (puppetUpgradeUniqueSubChoiceEntries), capped at
// 2 (one of each weapon) instead of 5. Each weapon it grants is computed
// straight into the companion's Attacks list, never stored as its own row —
// see puppetIntegratedWeaponAttacks — since which weapon(s) a pick grants
// is fully determined by its own sub-choice already on file.
const integratedWeaponEntrySlug = "class/puppet-master/option/armorers-upgrades/wood-tier/entry/integrated-weapon"

// puppetUpgradeUniqueSubChoiceEntries: repeatable entries whose sub-choice
// must be unique across ALL of a companion's own picks of that entry
// ("each time selecting a different element" / "to gain the OTHER") — as
// opposed to Poison Mist Hell's shape, where up to 3 sub-choices are picked
// under the SAME single pick and repeats are fine (any quality of Poison
// Tag, as many times as wanted).
//
// This same set doubles as "permanent, no delete button" — see
// puppetUpgradePickIsPermanent's own doc for why the two properties always
// travel together for these two entries specifically.
var puppetUpgradeUniqueSubChoiceEntries = map[string]bool{
	elementalReactorEntrySlug: true,
	integratedWeaponEntrySlug: true,
	masteredArmamentEntrySlug: true,
}

// puppetUpgradePickIsPermanent reports whether a taken pick of this entry
// can never be undone — Elemental Reactor ("each time selecting a
// different element") and Integrated Weapon ("You can take this upgrade a
// second time, to gain the OTHER") both read as a standing, permanent
// choice with no book text allowing it to be swapped back out, unlike an
// ordinary upgrade pick (freely removable — the player may have simply
// misclicked, or wants to reallocate the slot). Deliberately the SAME set as
// puppetUpgradeUniqueSubChoiceEntries: both entries need a NEW, DIFFERENT
// value each time specifically because the old one is never given back —
// a re-deletable pick would make "each time selecting a different X"
// unenforceable (delete Fire, re-add Fire, no real uniqueness at all).
// Checked both in the template (hides the delete button) and server-side in
// handlePuppetUpgradeDelete/handlePuppetUpgradeChoiceDelete (the button
// being hidden is a convenience, not the real gate, same as every other
// control on this sheet).
func puppetUpgradePickIsPermanent(entrySlug string) bool {
	return puppetUpgradeUniqueSubChoiceEntries[entrySlug]
}

// integratedWeaponOptions is Integrated Weapon's own fixed 2-item choice —
// slugs match the ones puppetIntegratedWeaponAttacks reads back.
func integratedWeaponOptions() []puppetUpgradeSubChoiceOption {
	return []puppetUpgradeSubChoiceOption{
		{Slug: "arm-blades", Name: "Arm Blades", Description: "1d6 slashing damage (counts as Tonfa and Knuckle Blades). Double Strike: once per round, an extra attack with it as part of the same action."},
		{Slug: "axe-tail", Name: "Axe Tail", Description: "1d8 piercing damage (counts as Whip, Weighted Chain, and Scythe). Deflection: as a Reaction to being targeted within 10ft, increase AC by 1d8 for that attack; afterward, until end of turn, any attack against you can be answered with an attack using the tail, ignoring DR."},
	}
}

// masteredArmamentEntrySlug: Purple Technique's Bronze Tier "Mastered
// Armament" upgrade — an upgraded version of whichever Integrated Weapon
// the Puppet Tool already has ("You install an improved version of the
// special weapon you made previously... Select one of the following
// options; You can take this upgrade a second time to gain the other
// option"). Same repeatable-entry-with-a-unique-sub-choice-per-pick shape
// as Integrated Weapon itself — found in a sweep for every "take this
// upgrade a second time"/"multiple times" clause in the class, which turned
// up this entry with NO repeat-limit entry at all (silently capped at 1,
// blocking a legal second take) and no sub-choice tracking.
const masteredArmamentEntrySlug = "class/puppet-master/option/armorers-upgrades/bronze-tier/entry/mastered-armament"

// masteredArmamentOptions is Mastered Armament's own fixed 2-item choice —
// which prior Integrated Weapon pick it upgrades, own bonus ability text
// from the book (distinct from Integrated Weapon's own base weapon text).
func masteredArmamentOptions() []puppetUpgradeSubChoiceOption {
	return []puppetUpgradeSubChoiceOption{
		{Slug: "arm-blades", Name: "Arm Blades", Description: "Twice per round when you hit a creature with your arm blades you can pull them out violently. A creature must make a Constitution saving throw or gain a rank of bleeding. Also, you gain additional uses of your Arm Blades, Double Strike special ability equal to half your level."},
		{Slug: "axe-tail", Name: "Axe Tail", Description: "When you would use Deflection you can instead react if an attack hits you, and the AC bonus rerolls 1s and 2s and lasts until the start of your next turn. Also, you gain additional uses of your Axe Tail, Deflection special ability equal to half your level."},
	}
}

// warfareAugmentationEntrySlug: Wood Tier "Warfare Augmentation" (class-wide
// list, "Techniques: All") — "Select one Fighting Stance from Chapter 13...
// Your Puppet Tool gains this Fighting Stance. Puppets are able to use
// Bonus Action stance abilities." Its own repeat cap is entirely different
// per Puppet Technique, unlike every other Puppet Upgrade's fixed
// max-takes count (puppetUpgradeRepeatLimit) — see
// puppetWarfareAugmentationMaxTakes for the per-technique formula this
// implements. Deliberately NOT in puppetUpgradeUniqueSubChoiceEntries: the
// book places no "different each time" constraint on repeated takes here
// (unlike Elemental Reactor/Integrated Weapon), so the same Fighting Stance
// can legally be chosen more than once across a character's own Puppet
// Tools. Deliberately NOT in puppetUpgradePickIsPermanent either — nothing
// in the text reads as a standing, unswappable choice, so it follows the
// class's ordinary "swap Upgrades on a long/full rest via kit charges"
// rule like everything else.
const warfareAugmentationEntrySlug = "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/warfare-augmentation"

// puppetWarfareAugmentationMaxTakes: Red Technique is the ONLY reading that
// needs an override here — "this upgrade is given to 2 Puppet Tools you
// possess when acquired... You can select this upgrade again if you
// acquire 2 more Puppets" pools across the character's WHOLE roster (one
// take grants the stance to 2 companions at once), capped at
// floor(companionCount / 2). Every other technique's reading needs no
// special code at all: Black/Green's "select this upgrade multiple times,
// once for each Puppet Tool you have" and Purple/White's "you yourself
// gain a fighting stance" are both already exactly what the ORDINARY
// default per-companion cap of 1 gives for free — each companion already
// has its own fully independent Wood Tier slot budget
// (puppetUpgradeMaterialTierUsage is scoped by companionID), so "once per
// Puppet Tool" falls out of that for Black/Green automatically, and
// Purple/White effectively have only the one companion (their armor/
// gloves) for a cap of 1 to apply to anyway. Only called (see both call
// sites below) when subclassColor == "Red" — the ordinary
// puppetUpgradeMaxTakes(warfareAugmentationEntrySlug) default of 1 is used
// unchanged for every other technique.
func puppetWarfareAugmentationMaxTakes(puppetCompanionCount int) int {
	return puppetCompanionCount / 2
}

// puppetAlwaysPreparedBulkBonus returns the max-bulk bonus from Always
// Prepared (base class, L15): "you and your Puppet Tool gain an additional
// +5 to your maximum bulk (Or you gain a +10 to your max bulk if you do not
// have a Puppet)." — 0 below 15th level, +5 with at least one active Puppet
// Tool companion, +10 with none.
func (s *server) puppetAlwaysPreparedBulkBonus(characterID int64) (float64, error) {
	level, err := s.puppetMasterClassLevel(characterID)
	if err != nil || level < 15 {
		return 0, err
	}
	count, err := s.puppetCompanionCount(characterID)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 5, nil
	}
	return 10, nil
}

// puppetChassisPropertyBulkBonus returns the max-bulk bonus from a currently
// worn Armor Chassis's own Powerful Build property ("increase your maximum
// bulk by +10") — 0 if no puppet is worn as armor, or the worn one's chassis
// doesn't carry Powerful Build (Weaved Mail, Wooden Suit, Iron Shell).
func (s *server) puppetChassisPropertyBulkBonus(characterID int64) (float64, error) {
	slug, err := charsheet.PuppetWornArmorChassisSlug(s.rulesDB, s.charDB, characterID)
	if err != nil {
		return 0, err
	}
	if puppetupgrades.ChassisHasPowerfulBuild(slug) {
		return puppetupgrades.PowerfulBuildBulkBonus, nil
	}
	return 0, nil
}

// backupPlanBulkBonus returns the max-bulk bonus from the Backup Plan (New)
// feat (feat/class/backup-plan-new, prerequisite 10+ Puppet Master levels):
// "Increase your maximum bulk by half your Puppet Master class level." — 0
// if the character doesn't have the feat.
func (s *server) backupPlanBulkBonus(characterID int64) (float64, error) {
	has, err := s.characterHasFeat(characterID, "feat/class/backup-plan-new")
	if err != nil || !has {
		return 0, err
	}
	level, err := s.puppetMasterClassLevel(characterID)
	if err != nil {
		return 0, err
	}
	return float64(level / 2), nil
}

// puppetCompanionCount counts a character's own kind="puppet" companions —
// Red Technique's own Warfare Augmentation cap needs this; queried fresh
// rather than threaded through every caller since it's a single cheap
// COUNT(*).
func (s *server) puppetCompanionCount(characterID int64) (int, error) {
	var n int
	err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_companions WHERE character_id = ? AND kind = 'puppet'`, characterID,
	).Scan(&n)
	return n, err
}

// puppetEntryTotalPicksAcrossCompanions counts how many of a character's
// OWN puppet companions (any of them, not just the one currently being
// viewed/edited) have taken the given upgrade entry, summed together —
// needed only for Red Technique's own Warfare Augmentation cap, which
// pools across the character's whole roster rather than staying scoped to
// one companion the way every other upgrade cap on this sheet does (see
// puppetWarfareAugmentationMaxTakes's own doc for why).
func (s *server) puppetEntryTotalPicksAcrossCompanions(characterID int64, entrySlug string) (int, error) {
	var n int
	err := s.charDB.QueryRow(`
		SELECT COUNT(*) FROM character_companion_upgrades u
		JOIN character_companions c ON c.id = u.companion_id
		WHERE c.character_id = ? AND c.kind = 'puppet' AND u.upgrade_entry_slug = ?`,
		characterID, entrySlug,
	).Scan(&n)
	return n, err
}

// fightingStanceOptions resolves every Fighting Stance from rules.db as
// Warfare Augmentation's own sub-choice list — a DynamicOptions func (like
// battleReadyArmorSealOptions) rather than a static FixedOptions slice
// purely because that field can't hold something requiring a DB query;
// unlike Battle Ready Armor's own list, nothing here is actually
// character-state-gated, every one of the 21 stances is always offered.
// Every stance is offered at every slot, so pickIndex/companionID (see
// DynamicOptions' own doc) are unused here.
func fightingStanceOptions(s *server, characterID, companionID int64, pickIndex int) ([]puppetUpgradeSubChoiceOption, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name, description FROM fighting_stances ORDER BY name`)
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
		// A stance's own full rules page — its effects are activated
		// abilities this app does not compute (see internal/puppetupgrades'
		// own classification of conditional benefits), so reaching the real
		// text quickly is what actually helps at the table.
		o.DetailHref = "/stances/" + o.Slug
		out = append(out, o)
	}
	return out, rows.Err()
}

// battleReadyArmorEntrySlug: Purple Technique's Silver Tier "Battle Ready
// Armor" upgrade ("Choose a number of armor seals that would take up to 3
// armor slots that you qualify for. You gain these armor seals and you can
// swap out which seals you are benefitting from on a full rest."). Unlike
// Elemental Reactor/Integrated Weapon/Mastered Armament, this is explicitly
// NOT permanent — the book says outright you can swap seals on a full rest
// — so it's a single ordinary (non-repeatable, freely deletable) upgrade
// pick whose own sub-choices (up to 3 Armor Seals) can each be added or
// removed at will, the same shape as Poison Mist Hell's poison tags, just
// with a level-gated rather than fixed option list (see
// battleReadyArmorSealOptions).
const battleReadyArmorEntrySlug = "class/puppet-master/option/armorers-upgrades/silver-tier/entry/battle-ready-armor"

// battleReadyArmorSealOptions resolves every Armor Seal (equipment.kind =
// 'enhancement_seal', seal_applies_to = 'armor') the character currently
// qualifies for by rank — "up to 3 armor slots that you qualify for" reads
// as one slot per seal (equipment carries no separate per-seal slot-cost
// column; every seal on this table costs the same 1 slot, same convention
// as the /seals reference page's own listing), gated by the same universal
// D/C/B/A/S rank-by-level formula every other rank-gated pick in this app
// already uses (highestRankForLevel) rather than the Puppet Master class
// level specifically — enhancement seals are a core-book mechanic, not
// puppet-specific, so they follow the core book's own level convention.
// Every slot offers the same rank-gated list, so pickIndex/companionID (see
// DynamicOptions' own doc) are unused here.
func battleReadyArmorSealOptions(s *server, characterID, companionID int64, pickIndex int) ([]puppetUpgradeSubChoiceOption, error) {
	qualifyingRank := jutsuRankOrder[highestRankForLevel(s.characterLevel(characterID))]
	rows, err := s.rulesDB.Query(
		`SELECT slug, name, COALESCE(description, ''), seal_rank FROM equipment
		 WHERE kind = 'enhancement_seal' AND seal_applies_to = 'armor' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []puppetUpgradeSubChoiceOption
	for rows.Next() {
		var o puppetUpgradeSubChoiceOption
		var rank sql.NullString
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description, &rank); err != nil {
			return nil, err
		}
		if jutsuRankOrder[rank.String] <= qualifyingRank {
			o.DetailHref = "/seals/" + o.Slug
			out = append(out, o)
		}
	}
	return out, rows.Err()
}

// puppetSealOption is one Armor or Weapon seal a puppet's own Chakra
// Enhanced Retrofit slots can hold, resolved from rules.db's equipment
// table (kind = 'enhancement_seal') the same way battleReadyArmorSealOptions
// already resolves the Armor-only subset.
type puppetSealOption struct {
	Slug, Name, Description string
	AppliesTo               string // "armor" or "weapon"
}

// puppetSealPick is one seal actually equipped on a companion, carrying its
// own character_companion_seals row id for the sheet's delete button.
type puppetSealPick struct {
	ID int64
	puppetSealOption
}

// puppetSealSlotCap: Chakra Enhanced Retrofit (Class: Puppet Master/3rd
// Level) — "Your Puppet Tool gains a number of seal slots equal to 2 + your
// Proficiency Bonus." One shared pool covering both Armor and Weapon seals
// ("You can place both Armor and Weapon seals on your puppet"), not two
// separate caps.
func puppetSealSlotCap(proficiencyBonus int) int {
	return 2 + proficiencyBonus
}

// loadPuppetSealOptions resolves every enhancement seal (both Armor and
// Weapon) the character currently qualifies for by rank — same
// highestRankForLevel gate battleReadyArmorSealOptions already uses, since
// this is the same core-book mechanic, just no longer restricted to Armor
// seals or gated behind taking a specific Upgrade first.
func (s *server) loadPuppetSealOptions(characterID int64) ([]puppetSealOption, error) {
	qualifyingRank := jutsuRankOrder[highestRankForLevel(s.characterLevel(characterID))]
	rows, err := s.rulesDB.Query(
		`SELECT slug, name, COALESCE(description, ''), seal_rank, seal_applies_to FROM equipment
		 WHERE kind = 'enhancement_seal' ORDER BY seal_applies_to, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []puppetSealOption
	for rows.Next() {
		var o puppetSealOption
		var rank sql.NullString
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description, &rank, &o.AppliesTo); err != nil {
			return nil, err
		}
		if jutsuRankOrder[rank.String] <= qualifyingRank {
			out = append(out, o)
		}
	}
	return out, rows.Err()
}

// loadCompanionSeals resolves a companion's own equipped seals to their
// full rules.db rows, dropping any whose slug no longer resolves (a
// removed/renamed seal) rather than erroring the whole sheet render over
// one stale row.
func (s *server) loadCompanionSeals(characterID, companionID int64) ([]puppetSealPick, error) {
	picks, err := charstore.ListCompanionSeals(s.charDB, characterID, companionID)
	if err != nil {
		return nil, err
	}
	if len(picks) == 0 {
		return nil, nil
	}
	options, err := s.loadPuppetSealOptions(characterID)
	if err != nil {
		return nil, err
	}
	bySlug := map[string]puppetSealOption{}
	for _, o := range options {
		bySlug[o.Slug] = o
	}
	var out []puppetSealPick
	for _, p := range picks {
		opt, ok := bySlug[p.SealSlug]
		if !ok {
			continue
		}
		out = append(out, puppetSealPick{ID: p.ID, puppetSealOption: opt})
	}
	return out, nil
}

// handlePuppetSealAdd records one equipped seal (form field "seal_slug"),
// hard-rejecting once Chakra Enhanced Retrofit's own slot cap is used up or
// the character doesn't have the feature yet (level 3+).
func (s *server) handlePuppetSealAdd(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for seal add:", err)
		return
	}
	if companion.Kind != "puppet" {
		http.Error(w, "not a puppet", http.StatusBadRequest)
		return
	}
	level, err := s.puppetMasterClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master level for seal add:", err)
		return
	}
	if level < 3 {
		http.Error(w, "Chakra Enhanced Retrofit isn't unlocked yet", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for seal add:", err)
		return
	}
	existing, err := charstore.ListCompanionSeals(s.charDB, id, cid)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load existing seals:", err)
		return
	}
	if len(existing) >= puppetSealSlotCap(sheet.ProficiencyBonus) {
		http.Error(w, "no seal slots remaining", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	sealSlug := strings.TrimSpace(r.FormValue("seal_slug"))
	if sealSlug == "" {
		http.Error(w, "missing seal selection", http.StatusBadRequest)
		return
	}
	options, err := s.loadPuppetSealOptions(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load seal options for seal add:", err)
		return
	}
	valid := false
	for _, o := range options {
		if o.Slug == sealSlug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "this seal isn't available at your current rank", http.StatusBadRequest)
		return
	}
	if _, err := charstore.AddCompanionSeal(s.charDB, id, cid, sealSlug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add companion seal:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handlePuppetSealDelete removes one equipped seal — "This Weapon or Armor
// can be resealed without destroying it" reads as freely swappable, unlike
// the permanent Elemental Reactor/Integrated Weapon/Mastered Armament picks.
func (s *server) handlePuppetSealDelete(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	sid, err := strconv.ParseInt(r.PathValue("sid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := charstore.DeleteCompanionSeal(s.charDB, id, cid, sid); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete companion seal:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// puppetUpgradeSubChoiceSpecs is a small, explicit, hand-curated map from
// class_option_entries slug to its own sub-choice spec — same "small and
// targeted, not a generic engine" precedent as puppetToolUnitCardRe.
var puppetUpgradeSubChoiceSpecs = map[string]puppetUpgradeSubChoiceSpec{
	"class/puppet-master/option/black-iron-upgrades/wood-tier/entry/poison-mist-hell": {
		Quantity: 3,
		EquipmentSlugs: []string{
			"tool/poison-tag", "tool/greater-poison-tag", "tool/superior-poison-tag", "tool/supreme-poison-tag",
		},
	},
	elementalReactorEntrySlug: {
		Quantity:     1,
		FixedOptions: elementReactorOptions(),
	},
	integratedWeaponEntrySlug: {
		Quantity:     1,
		FixedOptions: integratedWeaponOptions(),
	},
	masteredArmamentEntrySlug: {
		Quantity:     1,
		FixedOptions: masteredArmamentOptions(),
	},
	battleReadyArmorEntrySlug: {
		Quantity:       3,
		DynamicOptions: battleReadyArmorSealOptions,
	},
	warfareAugmentationEntrySlug: {
		Quantity:       1,
		DynamicOptions: fightingStanceOptions,
	},
}

// elementReactorOptions turns elementNames (elemental_affinity.go) into the
// Elemental Reactor sub-choice list — same 5 elements a jutsu's own
// keywords column can name, kept as the single source of truth rather than
// a second hand-typed list that could drift from it.
func elementReactorOptions() []puppetUpgradeSubChoiceOption {
	out := make([]puppetUpgradeSubChoiceOption, len(elementNames))
	for i, el := range elementNames {
		out[i] = puppetUpgradeSubChoiceOption{Slug: el, Name: el, Description: "Grants eligibility to learn " + el + " Release jutsu (subject to level requirements), the same as any other elemental affinity source."}
	}
	return out
}

// puppetUpgradeTier is one class_options row (a slot-consuming tier, or an
// untiered one-time-pick list like a single Puppet Weapon Type) with its
// resolved slot cap, current usage, full option catalog, and the
// companion's own picks drawn from it.
type puppetUpgradeTier struct {
	ClassOptionSlug string
	ListName        string
	TierName        string // class_options.name — "Wood Tier", "Drone Weapon", ...
	Prerequisites   string
	Cap             int
	Used            int
	Options         []puppetUpgradeOption
	Picks           []puppetUpgradePick
}

// puppetUpgradeList groups every tier sharing the same ListName under one
// heading ("Puppet Master Upgrades", "Black Iron Upgrades", ...) — without
// this grouping, the class-wide list and a subclass-exclusive list both
// printing "Wood Tier"/"Bronze Tier"/... side by side with no label reads
// as the same tiers shown twice, not two different lists that happen to
// share tier names (see puppetUpgradeSlotCap's own doc for why they really
// do pool the same slot count).
type puppetUpgradeList struct {
	ListName string
	Tiers    []puppetUpgradeTier
}

// groupPuppetUpgradeTiers groups an already list_name-ordered tier slice
// (see loadPuppetUpgradeTiers' own ORDER BY) into named lists, preserving
// that order rather than re-sorting — a map would lose it.
func groupPuppetUpgradeTiers(tiers []puppetUpgradeTier) []puppetUpgradeList {
	var lists []puppetUpgradeList
	for _, t := range tiers {
		if len(lists) == 0 || lists[len(lists)-1].ListName != t.ListName {
			lists = append(lists, puppetUpgradeList{ListName: t.ListName})
		}
		last := &lists[len(lists)-1]
		last.Tiers = append(last.Tiers, t)
	}
	return lists
}

// loadPuppetUpgradeSubChoiceOptions resolves a repeatable upgrade's own
// backing equipment slugs (see puppetUpgradeSubChoiceSpecs) to real
// name/description rows from rules.db's equipment table.
func (s *server) loadPuppetUpgradeSubChoiceOptions(equipmentSlugs []string) ([]puppetUpgradeSubChoiceOption, error) {
	if len(equipmentSlugs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(equipmentSlugs)), ",")
	args := make([]any, len(equipmentSlugs))
	for i, slug := range equipmentSlugs {
		args[i] = slug
	}
	rows, err := s.rulesDB.Query(
		`SELECT slug, name, COALESCE(description, '') FROM equipment WHERE slug IN (`+placeholders+`) ORDER BY name`, args...)
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
		out = append(out, o)
	}
	return out, rows.Err()
}

// handlePuppetUpgradeChoiceAdd records one sub-pick under an already-taken
// repeatable upgrade (form field "choice_slug"), hard-rejecting once the
// upgrade's own quantity is used up.
func (s *server) handlePuppetUpgradeChoiceAdd(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for upgrade choice add:", err)
		return
	}
	if companion.Kind != "puppet" {
		http.Error(w, "not a puppet", http.StatusBadRequest)
		return
	}
	upgrades, err := charstore.ListCompanionUpgrades(s.charDB, id, cid)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion upgrades for choice add:", err)
		return
	}
	var upgradeEntrySlug string
	for _, u := range upgrades {
		if u.ID == uid {
			upgradeEntrySlug = u.UpgradeEntrySlug
			break
		}
	}
	spec, ok := puppetUpgradeSubChoiceSpecs[upgradeEntrySlug]
	if !ok {
		http.Error(w, "this upgrade has no sub-choices", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	choiceSlug := strings.TrimSpace(r.FormValue("choice_slug"))
	if choiceSlug == "" {
		http.Error(w, "missing choice selection", http.StatusBadRequest)
		return
	}
	existing, err := charstore.ListCompanionUpgradeChoices(s.charDB, id, cid)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load existing upgrade choices:", err)
		return
	}
	used := 0
	for _, ch := range existing {
		if ch.CompanionUpgradeID == uid {
			used++
		}
	}
	if used >= spec.Quantity {
		http.Error(w, "no sub-choice slots remaining for this upgrade", http.StatusBadRequest)
		return
	}
	// A Foundation entry offering more than one KIND of sub-choice under one
	// pick (Specialized: ability/size/weapon damage type; Sentinel:
	// ability/size) prefixes each option's own slug with its kind ("ability:
	// str") specifically so this check can cap each kind at 1 pick — spec's
	// own flat Quantity above only bounds the TOTAL, which would otherwise
	// let a player pick, say, 2 abilities and skip size entirely. A plain
	// (unprefixed) choiceSlug — every other sub-choice entry in this app —
	// makes kind == choiceSlug, so this loop just falls through to the
	// existing used-count check for those, unaffected.
	//
	// puppetFoundationRepeatableSubChoiceKinds (Bestial Framework's own
	// "ability" and "trait" kinds) opts a kind out of this check entirely —
	// unlike Specialized/Sentinel, those two kinds legitimately need more
	// than one pick under the same prefix, and spec.Quantity's own flat cap
	// (checked just above) already bounds the total.
	if kind, _, ok := strings.Cut(choiceSlug, ":"); ok && !puppetFoundationRepeatableSubChoiceKinds[upgradeEntrySlug][kind] {
		for _, ch := range existing {
			if ch.CompanionUpgradeID == uid && strings.HasPrefix(ch.ChoiceSlug, kind+":") {
				http.Error(w, "this category has already been chosen for this upgrade", http.StatusBadRequest)
				return
			}
		}
	}
	// Elemental Reactor ("each time selecting a different element") and
	// Integrated Weapon ("to gain the OTHER") both need their sub-choice
	// unique across every OTHER row of the same repeatable entry — the
	// per-row Quantity: 1 check above only guards THIS row, so every other
	// row's already-chosen value is also checked here (mirrors
	// loadPuppetUpgradeTiers' own filter, which only hides the option
	// client-side).
	if puppetUpgradeUniqueSubChoiceEntries[upgradeEntrySlug] {
		upgrades, err := charstore.ListCompanionUpgrades(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load companion upgrades for sub-choice uniqueness check:", err)
			return
		}
		sameEntryUpgradeIDs := map[int64]bool{}
		for _, u := range upgrades {
			if u.UpgradeEntrySlug == upgradeEntrySlug {
				sameEntryUpgradeIDs[u.ID] = true
			}
		}
		for _, ch := range existing {
			if sameEntryUpgradeIDs[ch.CompanionUpgradeID] && ch.ChoiceSlug == choiceSlug {
				http.Error(w, "this option has already been chosen for another pick of this upgrade", http.StatusBadRequest)
				return
			}
		}
	}
	if _, err := charstore.AddCompanionUpgradeChoice(s.charDB, id, cid, uid, choiceSlug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add companion upgrade choice:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handlePuppetUpgradeChoiceDelete removes one sub-pick.
// handlePuppetUpgradeChoiceDelete removes one sub-pick — rejected
// server-side for a permanent entry (Elemental Reactor/Integrated Weapon,
// see puppetUpgradePickIsPermanent) the same way handlePuppetUpgradeDelete
// re-checks its own button's hidden/shown state.
func (s *server) handlePuppetUpgradeChoiceDelete(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	chid, err := strconv.ParseInt(r.PathValue("chid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	picks, err := charstore.ListCompanionUpgrades(s.charDB, id, cid)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion upgrades for choice delete:", err)
		return
	}
	for _, p := range picks {
		if p.ID == uid && puppetUpgradePickIsPermanent(p.UpgradeEntrySlug) {
			http.Error(w, "this upgrade is a permanent choice and cannot be removed", http.StatusBadRequest)
			return
		}
	}
	if err := charstore.DeleteCompanionUpgradeChoice(s.charDB, id, cid, chid); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete companion upgrade choice:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// puppetUpgradeEntryClassOptionSlug resolves one upgrade_entry_slug back to
// the class_options row it belongs to — needed since a merged tier's "+Add"
// form (see groupPuppetUpgradeTiers) can no longer carry one static
// class_option_slug covering every option it shows. Returns sql.ErrNoRows
// only if upgradeEntrySlug isn't a real class_option_entries row AND isn't a
// real untiered class_options row either (an untiered one-time pick, e.g. a
// Puppet Weapon Type, has no class_option_entries row of its own at all —
// see loadPuppetUpgradeTiers's own fallback — so its "entry slug" IS its
// class_options slug).
func (s *server) puppetUpgradeEntryClassOptionSlug(upgradeEntrySlug string) (string, error) {
	var classOptionSlug string
	err := s.rulesDB.QueryRow(
		`SELECT class_option_slug FROM class_option_entries WHERE slug = ?`, upgradeEntrySlug,
	).Scan(&classOptionSlug)
	if err == nil {
		return classOptionSlug, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	err = s.rulesDB.QueryRow(
		`SELECT slug FROM class_options WHERE slug = ? AND class_slug = 'class/puppet-master'`, upgradeEntrySlug,
	).Scan(&classOptionSlug)
	return classOptionSlug, err
}

// puppetUpgradeTierName resolves one class_options row's own name — the
// same value class_level_resources.resource_name uses as its key — so
// slot-cap lookups and usage pooling both key off it consistently.
func (s *server) puppetUpgradeTierName(classOptionSlug string) (string, error) {
	var name string
	err := s.rulesDB.QueryRow(`SELECT name FROM class_options WHERE slug = ?`, classOptionSlug).Scan(&name)
	return name, err
}

// puppetUpgradeTierClassOptionSlug resolves which REAL class_options row
// backs a given tier name for a character's own subclass — needed only for
// puppetUpgradeTierEscalation, where an entry escalating "to Platinum Tier"
// has to be recorded against an actual row, and which one that is depends
// on the character's own subclass: the class-wide "Puppet Master Upgrades"
// list has no Platinum Tier row at all (confirmed directly against
// rules.db — it only goes Wood/Bronze/Silver/Gold), so a Purple Technique
// character's Platinum escalation resolves to armorers-upgrades'
// own platinum-tier row, a Black Technique character's to
// black-iron-upgrades', and so on — the subclass-specific row for this
// tier name if one exists, else the class-wide one.
func (s *server) puppetUpgradeTierClassOptionSlug(tierName, subclassSlug string) (string, error) {
	var slug string
	if subclassSlug != "" {
		err := s.rulesDB.QueryRow(`
			SELECT slug FROM class_options
			WHERE class_slug = 'class/puppet-master' AND name = ? AND subclass_slug = ?`,
			tierName, subclassSlug).Scan(&slug)
		if err == nil {
			return slug, nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
	}
	err := s.rulesDB.QueryRow(`
		SELECT slug FROM class_options
		WHERE class_slug = 'class/puppet-master' AND name = ? AND subclass_slug IS NULL`,
		tierName).Scan(&slug)
	return slug, err
}

// puppetUpgradeSlotCap resolves how many picks a tier allows at the given
// Puppet Master level. A class_options row with zero class_option_entries
// (nothing to bundle-split — e.g. each Puppet Weapon Type row) is a single
// one-time pick, cap 1, not looked up in class_level_resources at all —
// confirmed the only such rows for this class are exactly this shape.
// Everything else looks up class_level_resources by (class/puppet-master,
// level, tierName): class_level_resources carries exactly one row per
// (class, level, tier name), so a tier name shared by the class-wide list
// and a subclass-exclusive list (e.g. "Wood Tier" appears in both) resolves
// to the SAME cap, by construction — not a guess, confirmed directly
// against the live rules.db (single PRIMARY KEY row per tier name).
func (s *server) puppetUpgradeSlotCap(classOptionSlug, tierName string, puppetMasterLevel int) (int, error) {
	var entryCount int
	if err := s.rulesDB.QueryRow(
		`SELECT COUNT(*) FROM class_option_entries WHERE class_option_slug = ?`, classOptionSlug,
	).Scan(&entryCount); err != nil {
		return 0, err
	}
	if entryCount == 0 {
		return 1, nil
	}

	var valueText sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT value FROM v_class_level_resources
		WHERE class_slug = 'class/puppet-master' AND level = ? AND resource_name = ?`,
		puppetMasterLevel, tierName,
	).Scan(&valueText)
	if err == sql.ErrNoRows {
		return 0, nil // tier not unlocked yet at this level
	}
	if err != nil {
		return 0, err
	}
	n, convErr := strconv.Atoi(valueText.String)
	if convErr != nil {
		return 0, nil
	}
	return n, nil
}

// puppetUpgradeTierRanks orders the five material-tier Upgrade slots from
// weakest/most-common (Wood) to strongest/rarest (Platinum) — the ladder
// Puppet Upgrades' own closing rule converts along: "You always choose to
// expend slots of higher tiers to gain more lower tier upgrades. For
// example, at 6th level, you choose to either take five Wood tier
// Upgrades, and one Bronze tier Upgrade, or simply take six Wood tier
// Upgrades." A tier name absent here (the untiered one-time picks, e.g. a
// Puppet Weapon Type) isn't part of this ladder at all and keeps the flat
// cap=1 gate puppetUpgradeSlotCap already gives it.
var puppetUpgradeTierRanks = map[string]int{
	"Wood Tier":     1,
	"Bronze Tier":   2,
	"Silver Tier":   3,
	"Gold Tier":     4,
	"Platinum Tier": 5,
}

const puppetUpgradeTierCount = 5

// puppetUpgradeMaterialTierCaps resolves the class table's own printed cap
// for every material tier at once, at the given Puppet Master level —
// index 0 is unused (ranks start at 1) so callers can index directly by
// puppetUpgradeTierRanks' own values. A tier not yet unlocked at this level
// simply stays 0, the same "not an error" case puppetUpgradeSlotCap treats
// a missing row as.
func (s *server) puppetUpgradeMaterialTierCaps(level int) ([puppetUpgradeTierCount + 1]int, error) {
	var caps [puppetUpgradeTierCount + 1]int
	for name, rank := range puppetUpgradeTierRanks {
		var valueText sql.NullString
		err := s.rulesDB.QueryRow(`
			SELECT value FROM v_class_level_resources
			WHERE class_slug = 'class/puppet-master' AND level = ? AND resource_name = ?`,
			level, name,
		).Scan(&valueText)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return caps, err
		}
		if n, convErr := strconv.Atoi(valueText.String); convErr == nil {
			caps[rank] = n
		}
	}
	return caps, nil
}

// puppetUpgradeMaterialTierUsage counts a companion's existing picks by
// material tier rank, pooled across every class_options row sharing that
// tier name exactly like puppetUpgradeTierUsage — this is the batch
// equivalent, computing all five ranks in one pass over the picks instead
// of one query per tier.
func (s *server) puppetUpgradeMaterialTierUsage(characterID, companionID int64) ([puppetUpgradeTierCount + 1]int, error) {
	var used [puppetUpgradeTierCount + 1]int
	picks, err := charstore.ListCompanionUpgrades(s.charDB, characterID, companionID)
	if err != nil {
		return used, err
	}
	if len(picks) == 0 {
		return used, nil
	}
	nameBySlug, err := s.classOptionNamesBySlug(picks)
	if err != nil {
		return used, err
	}
	for _, p := range picks {
		if rank, ok := puppetUpgradeTierRanks[nameBySlug[p.ClassOptionSlug]]; ok {
			used[rank]++
		}
	}
	return used, nil
}

// puppetUpgradeEffectiveCap returns how many total picks material tier
// `rank` can actually hold once every unused slot in a HIGHER tier is
// converted down as far as it will go. Conversion only ever moves capacity
// DOWN the ladder (a Bronze slot can fund a Wood upgrade, never the
// reverse), so for any threshold k <= rank, the combined slots of rank
// k-or-higher can never fund more than that many total picks of rank
// k-or-higher, no matter how a player individually allocates them — the
// true ceiling for `rank` is the tightest (smallest slack) of those
// thresholds, not just its own row in the class table.
func puppetUpgradeEffectiveCap(rank int, caps, used [puppetUpgradeTierCount + 1]int) int {
	cumCap, cumUsed := 0, 0
	slack := -1
	for k := puppetUpgradeTierCount; k >= 1; k-- {
		cumCap += caps[k]
		cumUsed += used[k]
		if k > rank {
			continue
		}
		if s := cumCap - cumUsed; slack == -1 || s < slack {
			slack = s
		}
	}
	if slack < 0 {
		slack = 0
	}
	return used[rank] + slack
}

// puppetUpgradeTierUsage counts how many of a companion's existing picks
// were drawn from any class_options row sharing tierName — pooled across
// lists the same way puppetUpgradeSlotCap's cap already is (see its doc),
// so a class-wide "Wood Tier" pick and a subclass-exclusive "Wood Tier"
// pick correctly count against the same slot budget.
func (s *server) puppetUpgradeTierUsage(characterID, companionID int64, tierName string) (int, error) {
	picks, err := charstore.ListCompanionUpgrades(s.charDB, characterID, companionID)
	if err != nil {
		return 0, err
	}
	if len(picks) == 0 {
		return 0, nil
	}
	nameBySlug, err := s.classOptionNamesBySlug(picks)
	if err != nil {
		return 0, err
	}
	used := 0
	for _, p := range picks {
		if nameBySlug[p.ClassOptionSlug] == tierName {
			used++
		}
	}
	return used, nil
}

// puppetUpgradeNonMaterialTierUsage resolves how many picks already count
// against a non-material-tier row's cap — see loadPuppetUpgradeTiers' own
// "untiered" merge comment for why this can't just be
// puppetUpgradeTierUsage(tierName): an untiered single-option row (one
// Puppet Weapon Type, one Puppeteer Chassis, ...) has no class_option_entries
// of its own, and each option is its OWN class_options row with its OWN
// name, so per-row-name scoping alone never sees a pick of a SIBLING option
// as using the same real-world cap. Usage pools across every sibling row
// sharing the same list_name in that case; every other (tiered) row keeps
// the existing per-tier-name scoping unchanged.
func (s *server) puppetUpgradeNonMaterialTierUsage(characterID, companionID int64, classOptionSlug, tierName string) (int, error) {
	var entryCount int
	if err := s.rulesDB.QueryRow(
		`SELECT COUNT(*) FROM class_option_entries WHERE class_option_slug = ?`, classOptionSlug,
	).Scan(&entryCount); err != nil {
		return 0, err
	}
	if entryCount > 0 {
		return s.puppetUpgradeTierUsage(characterID, companionID, tierName)
	}

	var listName string
	if err := s.rulesDB.QueryRow(`SELECT list_name FROM class_options WHERE slug = ?`, classOptionSlug).Scan(&listName); err != nil {
		return 0, err
	}
	siblingRows, err := s.rulesDB.Query(`
		SELECT co.slug FROM class_options co
		WHERE co.list_name = ?
		  AND NOT EXISTS (SELECT 1 FROM class_option_entries e WHERE e.class_option_slug = co.slug)`,
		listName)
	if err != nil {
		return 0, err
	}
	defer siblingRows.Close()
	siblings := map[string]bool{}
	for siblingRows.Next() {
		var slug string
		if err := siblingRows.Scan(&slug); err != nil {
			return 0, err
		}
		siblings[slug] = true
	}
	if err := siblingRows.Err(); err != nil {
		return 0, err
	}

	picks, err := charstore.ListCompanionUpgrades(s.charDB, characterID, companionID)
	if err != nil {
		return 0, err
	}
	used := 0
	for _, p := range picks {
		if siblings[p.ClassOptionSlug] {
			used++
		}
	}
	return used, nil
}

// classOptionNamesBySlug batch-resolves every distinct class_option_slug
// referenced by picks to its own class_options.name, one query instead of
// one per pick.
func (s *server) classOptionNamesBySlug(picks []charstore.CompanionUpgrade) (map[string]string, error) {
	seen := map[string]bool{}
	var slugs []string
	for _, p := range picks {
		if !seen[p.ClassOptionSlug] {
			seen[p.ClassOptionSlug] = true
			slugs = append(slugs, p.ClassOptionSlug)
		}
	}
	if len(slugs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(slugs)), ",")
	args := make([]any, len(slugs))
	for i, sl := range slugs {
		args[i] = sl
	}
	rows, err := s.rulesDB.Query(`SELECT slug, name FROM class_options WHERE slug IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var slug, name string
		if err := rows.Scan(&slug, &name); err != nil {
			return nil, err
		}
		out[slug] = name
	}
	return out, rows.Err()
}

// loadPuppetUpgradeTiers builds the full upgrade catalog for one puppet
// companion: every class_options row under class/puppet-master that's
// either class-wide (subclass_slug NULL) or belongs to the character's own
// chosen Puppet Master subclass, each resolved to its slot cap, current
// pooled usage, full pickable option list, and this companion's own picks.
func (s *server) loadPuppetUpgradeTiers(characterID, companionID int64, puppetMasterLevel int, subclassSlug, armorChassis string) ([]puppetUpgradeTier, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, list_name, name, COALESCE(prerequisites, ''), subclass_slug
		FROM class_options
		WHERE class_slug = 'class/puppet-master' AND (subclass_slug IS NULL OR subclass_slug = ?)
		ORDER BY list_name, sort_order`, subclassSlug)
	if err != nil {
		return nil, err
	}
	type optRow struct {
		slug, listName, name, prereq string
		classWide                    bool // subclass_slug IS NULL — see the merge below
	}
	var subclassOptRows, classWideOptRows []optRow
	for rows.Next() {
		var o optRow
		var rowSubclassSlug sql.NullString
		if err := rows.Scan(&o.slug, &o.listName, &o.name, &o.prereq, &rowSubclassSlug); err != nil {
			rows.Close()
			return nil, err
		}
		o.classWide = !rowSubclassSlug.Valid
		if o.classWide {
			classWideOptRows = append(classWideOptRows, o)
		} else {
			subclassOptRows = append(subclassOptRows, o)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	// Subclass-specific rows are processed first so the class-wide merge
	// below always has a same-named tier already built to fold into,
	// regardless of which list happens to sort alphabetically first
	// ("Upgrades of War"/"Upgrades of the Theatre" sort AFTER "Puppet
	// Master Upgrades", the other four subclass lists sort before it).
	optRows := append(subclassOptRows, classWideOptRows...)

	picks, err := charstore.ListCompanionUpgrades(s.charDB, characterID, companionID)
	if err != nil {
		return nil, err
	}
	allChoices, err := charstore.ListCompanionUpgradeChoices(s.charDB, characterID, companionID)
	if err != nil {
		return nil, err
	}
	resolvedFeatureChoices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	foundationPicks := resolvePuppetFoundationPicks(picks, allChoices)
	choicesByUpgradeID := map[int64][]charstore.CompanionUpgradeChoice{}
	for _, ch := range allChoices {
		choicesByUpgradeID[ch.CompanionUpgradeID] = append(choicesByUpgradeID[ch.CompanionUpgradeID], ch)
	}
	picksByOption := map[string][]charstore.CompanionUpgrade{}
	for _, p := range picks {
		picksByOption[p.ClassOptionSlug] = append(picksByOption[p.ClassOptionSlug], p)
	}
	nameBySlug, err := s.classOptionNamesBySlug(picks)
	if err != nil {
		return nil, err
	}
	usedByTierName := map[string]int{}
	for _, p := range picks {
		usedByTierName[nameBySlug[p.ClassOptionSlug]]++
	}

	// Every entry across every list in optRows, fetched once instead of one
	// query per row — this is also the vocabulary
	// parsePuppetUpgradePrereq's greedy scanner matches a prerequisite
	// clause's referenced name(s) against (see that file's own doc), so it
	// has to cover every list up front rather than being built one row at a
	// time as the tier loop below goes.
	entriesByOption := map[string][]puppetUpgradeOption{}
	entryBySlug := map[string]puppetUpgradeOption{}
	entryNameBySlug := map[string]string{}
	var allEntryNames []string
	if len(optRows) > 0 {
		slugs := make([]string, len(optRows))
		for i, o := range optRows {
			slugs[i] = o.slug
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(slugs)), ",")
		args := make([]any, len(slugs))
		for i, sl := range slugs {
			args[i] = sl
		}
		entryRows, err := s.rulesDB.Query(
			`SELECT class_option_slug, slug, name, description FROM class_option_entries
			 WHERE class_option_slug IN (`+placeholders+`) ORDER BY class_option_slug, sort_order`, args...)
		if err != nil {
			return nil, err
		}
		for entryRows.Next() {
			var optionSlug string
			var e puppetUpgradeOption
			if err := entryRows.Scan(&optionSlug, &e.Slug, &e.Name, &e.Description); err != nil {
				entryRows.Close()
				return nil, err
			}
			entriesByOption[optionSlug] = append(entriesByOption[optionSlug], e)
			entryBySlug[e.Slug] = e
			entryNameBySlug[e.Slug] = e.Name
			allEntryNames = append(allEntryNames, e.Name)
		}
		entryRows.Close()
		if err := entryRows.Err(); err != nil {
			return nil, err
		}
	}
	sortedUpgradeNames := puppetUpgradeSortedNames(allEntryNames)

	chassisOptions, err := s.loadPuppetArmorChassisOptions()
	if err != nil {
		return nil, err
	}
	chassisNameStrings := make([]string, len(chassisOptions))
	for i, c := range chassisOptions {
		chassisNameStrings[i] = c.Name
	}
	sortedChassisNames := puppetUpgradeSortedNames(chassisNameStrings)

	// What this companion already has, by entry name (lowercased) —
	// puppetUpgradePrereqMet's own vocabulary — resolved through
	// entryNameBySlug since picks are stored by slug.
	haveUpgradeName := map[string]bool{}
	pickedCountByEntrySlug := map[string]int{}
	subclassColor := puppetSubclassColorBySlug[subclassSlug]
	for _, p := range picks {
		if name, ok := entryNameBySlug[p.UpgradeEntrySlug]; ok {
			haveUpgradeName[strings.ToLower(name)] = true
		}
		pickedCountByEntrySlug[p.UpgradeEntrySlug]++
	}
	// Symphony of Puppetry's Expansion branch: "You gain the Expanded
	// Puppetry upgrade for free. If you have already taken this upgrade
	// twice, you gain an upgrade of bronze tier or lower." Computed once
	// here (needs pickedCountByEntrySlug, just built above) — merged into
	// the granted-display list and materialCaps below.
	symphonyExpansionEntrySlug, symphonyExpansionBronzeOrLower := puppetSymphonyExpansionGrant(resolvedFeatureChoices, pickedCountByEntrySlug)
	// Warfare Augmentation, Red Technique only: its own max-takes and
	// taken-count both pool across the character's WHOLE roster instead of
	// staying scoped to this one companion — see
	// puppetWarfareAugmentationMaxTakes's own doc. Queried once here,
	// rather than per-option below, and only for Red (every other
	// technique needs neither query at all).
	var puppetCompanionCount, warfareAugmentationTotalPicks int
	if subclassColor == "Red" {
		puppetCompanionCount, err = s.puppetCompanionCount(characterID)
		if err != nil {
			return nil, err
		}
		warfareAugmentationTotalPicks, err = s.puppetEntryTotalPicksAcrossCompanions(characterID, warfareAugmentationEntrySlug)
		if err != nil {
			return nil, err
		}
	}

	// puppetUpgradeUniqueSubChoiceEntries (Elemental Reactor, Integrated
	// Weapon, Mastered Armament): a fresh pick of one of these is useless
	// on its own — it only means something once its own sub-choice (which
	// element, which weapon) is assigned. Nothing previously stopped adding
	// a SECOND base pick before the first one's sub-choice was ever chosen,
	// which let both takes of a 2-take entry (e.g. Integrated Weapon)
	// exist at once with only one of them ever getting a sub-choice — the
	// other permanently stuck with nothing to delete (see
	// puppetUpgradePickIsPermanent). Gate a new pick on every existing pick
	// of the same entry already having its sub-choice filled in.
	incompleteUniqueSubChoicePick := map[string]bool{}
	for _, p := range picks {
		if !puppetUpgradeUniqueSubChoiceEntries[p.UpgradeEntrySlug] {
			continue
		}
		if len(choicesByUpgradeID[p.ID]) == 0 {
			incompleteUniqueSubChoicePick[p.UpgradeEntrySlug] = true
		}
	}

	// puppetUpgradeTierEscalation entries (Faraday Faceplate, Stonecold
	// Stronghold, Chakra Draining Mechanism, ...) can be taken more than
	// once, once per tier it's eligible for — each take is its own real
	// slot-consuming pick, stored under that TIER's own class_option_slug.
	// Showing every one of those picks in its own tier section reads as
	// the same upgrade appearing duplicated across the sheet; only the
	// HIGHEST tier actually taken is shown (the pick's own slot usage
	// still counts wherever it's really stored — this only affects
	// display). Tiers with no rank in puppetUpgradeTierRanks (untiered
	// one-time picks) are never affected: every pick of theirs is at rank
	// 0, so highest-tier-taken always resolves to their one and only tier.
	highestTierRankByEntrySlug := map[string]int{}
	for _, p := range picks {
		rank := puppetUpgradeTierRanks[nameBySlug[p.ClassOptionSlug]]
		if rank > highestTierRankByEntrySlug[p.UpgradeEntrySlug] {
			highestTierRankByEntrySlug[p.UpgradeEntrySlug] = rank
		}
	}

	// Computed once, not per class_options row: a material tier's real cap
	// (see puppetUpgradeEffectiveCap) depends on every OTHER tier's own
	// cap/usage too, not just its own.
	materialCaps, err := s.puppetUpgradeMaterialTierCaps(puppetMasterLevel)
	if err != nil {
		return nil, err
	}
	foundationBonusCaps := puppetFoundationBonusTierCapsFromPicks(foundationPicks)
	featureBonusCaps, err := s.puppetUpgradeFeatureBonusTierCapsForCharacter(characterID, subclassColor, puppetMasterLevel)
	if err != nil {
		return nil, err
	}
	for i := range materialCaps {
		materialCaps[i] += foundationBonusCaps[i] + featureBonusCaps[i]
	}
	if symphonyExpansionBronzeOrLower {
		materialCaps[puppetUpgradeTierRanks["Bronze Tier"]]++
	}
	materialUsed, err := s.puppetUpgradeMaterialTierUsage(characterID, companionID)
	if err != nil {
		return nil, err
	}

	// "All"-technique upgrades (class_options.subclass_slug IS NULL, e.g.
	// the "Puppet Master Upgrades" list) apply to every Puppet Technique —
	// merged here into the matching-named tier of whichever subclass-
	// specific list the character actually has, by TierName, rather than
	// kept as their own separate list/section (which is what made them easy
	// to miss: a Purple Technique puppet master would see one "Wood Tier"
	// under "Armorer's Upgrades" and an entirely separate "Wood Tier" under
	// "Puppet Master Upgrades" several scrolls down). tierIndexByName is
	// only populated by the subclass-specific pass (optRows puts those
	// first — see above), so a class-wide row always finds its subclass
	// counterpart already built by the time its own turn comes; if the
	// character has no subclass chosen yet, there is nothing to merge into
	// and the class-wide tier just appends as its own entry, unchanged from
	// today's behavior.
	tierIndexByName := map[string]int{}
	untieredListIndexByListName := map[string]int{}
	var tiers []puppetUpgradeTier
	for _, o := range optRows {
		options := entriesByOption[o.slug]
		untiered := len(options) == 0

		if untiered {
			// Untiered single-option row (e.g. one Puppet Weapon Type, one
			// Puppeteer Chassis, one Puppet Framework, one Puppet Role):
			// look up its own description to stand in as the one option.
			var desc string
			if err := s.rulesDB.QueryRow(`SELECT description FROM class_options WHERE slug = ?`, o.slug).Scan(&desc); err != nil {
				return nil, err
			}
			options = []puppetUpgradeOption{{Slug: o.slug, Name: o.name, Description: desc}}
		}

		// Restrict to options this character's own subclass can actually
		// take — a class-wide list (e.g. "Puppet Master Upgrades") bundles
		// entries that each individually restrict themselves to a subset of
		// subclasses ("Undying Form... Techniques: Black, Blue, Green, Red,
		// White" — every technique except Purple), which nothing enforced
		// before. An already-taken entry is kept regardless (never hide a
		// pick the player already committed to, even in the edge case where
		// a later subclass change would otherwise un-qualify it).
		var filtered []puppetUpgradeOption
		for _, opt := range options {
			if pickedCountByEntrySlug[opt.Slug] > 0 || puppetUpgradeAvailableToColor(opt.Description, subclassColor) {
				filtered = append(filtered, opt)
			}
		}
		options = filtered

		// puppetUpgradeTierEscalation entries also show up in each of their
		// OWN higher tiers' option lists, not just their home tier — rules.db
		// never duplicates the row itself, so this is the only place that
		// happens. The same struct (same Slug) is reused rather than copied,
		// so PrereqMet/AlreadyTaken below and pickedCountByEntrySlug (built
		// from ALL of this companion's picks up front, not scoped to one
		// tier) already treat every tier's copy as the same named upgrade.
		//
		// Injected only into the SUBCLASS-SPECIFIC row for a tier name, never
		// the class-wide one, even though a class-wide entry's higher tiers
		// (Chakra Draining Mechanism, Tag-Team) often have BOTH a class-wide
		// AND a subclass-specific row sharing that same tier name: those two
		// get merged into one visual section below (see the "All"-technique
		// merge comment above), so injecting into both would show the same
		// upgrade twice in that merged list. Every subclass's own list has a
		// full Wood-Platinum ladder (confirmed against rules.db), so the
		// subclass-specific row always exists once a subclass is chosen —
		// which every Puppet Master with any upgrades at all already has,
		// Puppet Technique being granted at the same level as Puppet
		// Upgrades itself.
		if !o.classWide {
			for entrySlug, spec := range puppetUpgradeTierEscalation {
				if !slices.Contains(spec.HigherTiers, o.name) {
					continue
				}
				e, ok := entryBySlug[entrySlug]
				if !ok {
					continue
				}
				// Injected after the eligibility filter above, so it needs
				// the same subclass-color check applied here — otherwise a
				// class-wide entry restricted to a subset of Techniques
				// (Chakra Draining Mechanism, Tag-Team) shows up in every
				// higher tier for every subclass, including ones its own
				// "Techniques:" line excludes.
				if pickedCountByEntrySlug[e.Slug] > 0 || puppetUpgradeAvailableToColor(e.Description, subclassColor) {
					options = append(options, e)
				}
			}
		}

		// Gate each not-yet-added option on its own embedded "Prerequisite:"
		// clause, if this can confidently resolve one — see
		// puppet_upgrade_prereq.go's own doc for what "confidently" means
		// here. PrereqHint is a plain "Requires: X" string for the tile's
		// own display; empty whenever there's nothing to show.
		for i := range options {
			groups := parsePuppetUpgradePrereq(options[i].Description, sortedUpgradeNames, sortedChassisNames)
			options[i].PrereqMet = puppetUpgradePrereqMet(groups, haveUpgradeName, armorChassis)
			if !options[i].PrereqMet {
				options[i].PrereqHint = "Requires " + puppetUpgradePrereqHintText(groups)
			}
			maxTakes := puppetUpgradeMaxTakes(options[i].Slug)
			takenCount := pickedCountByEntrySlug[options[i].Slug]
			if options[i].Slug == warfareAugmentationEntrySlug && subclassColor == "Red" {
				maxTakes = puppetWarfareAugmentationMaxTakes(puppetCompanionCount)
				takenCount = warfareAugmentationTotalPicks
			}
			options[i].AlreadyTaken = takenCount >= maxTakes ||
				incompleteUniqueSubChoicePick[options[i].Slug]
		}

		cap, err := s.puppetUpgradeSlotCap(o.slug, o.name, puppetMasterLevel)
		if err != nil {
			return nil, err
		}
		// A material tier's displayed cap is its EFFECTIVE cap (its own
		// table row plus whatever's convertible down from a higher tier),
		// so "X/Y used" and the "+Add" form's own visibility match exactly
		// what handlePuppetUpgradeAdd will actually accept — see
		// puppetUpgradeEffectiveCap's doc for the rule.
		if rank, ok := puppetUpgradeTierRanks[o.name]; ok {
			cap = puppetUpgradeEffectiveCap(rank, materialCaps, materialUsed)
		}

		byOptionSlug := map[string]puppetUpgradeOption{}
		for _, opt := range options {
			byOptionSlug[opt.Slug] = opt
		}
		// puppetUpgradeUniqueSubChoiceEntries (Elemental Reactor, Integrated
		// Weapon): each pick of one of these carries its own single sub-
		// choice, and that value must be unique across every OTHER pick of
		// the SAME entry this companion already has — the per-row
		// Quantity: 1 check alone doesn't stop a DIFFERENT row of the same
		// entry from offering an already-claimed value, so anything already
		// picked by this tier's other rows of the same entry is excluded
		// from every still-open row's own dropdown ("each time selecting a
		// different element" / "to gain the OTHER").
		usedSubChoiceValues := map[string]map[string]bool{} // entry slug -> value -> true
		for _, p := range picksByOption[o.slug] {
			if !puppetUpgradeUniqueSubChoiceEntries[p.UpgradeEntrySlug] {
				continue
			}
			for _, ch := range choicesByUpgradeID[p.ID] {
				if usedSubChoiceValues[p.UpgradeEntrySlug] == nil {
					usedSubChoiceValues[p.UpgradeEntrySlug] = map[string]bool{}
				}
				usedSubChoiceValues[p.UpgradeEntrySlug][ch.ChoiceSlug] = true
			}
		}

		var pickViews []puppetUpgradePick
		for _, p := range picksByOption[o.slug] {
			v, ok := byOptionSlug[p.UpgradeEntrySlug]
			if !ok {
				continue
			}
			// See highestTierRankByEntrySlug's own doc: a tier-escalation
			// entry taken at more than one tier only displays under the
			// highest one actually taken.
			if puppetUpgradeTierRanks[o.name] < highestTierRankByEntrySlug[p.UpgradeEntrySlug] {
				continue
			}
			pick := puppetUpgradePick{ID: p.ID, puppetUpgradeOption: v}
			if spec, ok := puppetUpgradeSubChoiceSpecs[p.UpgradeEntrySlug]; ok {
				// subOptions is what the "+Add" dropdown offers for the NEXT
				// slot only (pickIndex-gated for entries like Shade/
				// Spellblade Framework, where each slot has its own D/C/B-
				// Rank tier). lookupOptions is the superset used to resolve
				// ALREADY-chosen picks' display labels — those can span
				// every earlier, differently-gated slot, so a single
				// pickIndex-specific list would silently drop older picks
				// from display the moment a later slot's own gate takes
				// over (e.g. a D-Rank jutsu chosen at level 2 vanishing
				// once level 10 opens the C-Rank slot). For every entry
				// whose gate doesn't vary by slot (every DynamicOptions
				// implementation but the jutsu pickers, and every
				// FixedOptions/EquipmentSlugs spec), the two lists are
				// identical and this union is just redundant work, not a
				// behavior change.
				pickIndex := len(choicesByUpgradeID[p.ID])
				var subOptions, lookupOptions []puppetUpgradeSubChoiceOption
				switch {
				case spec.DynamicOptions != nil:
					var err error
					subOptions, err = spec.DynamicOptions(s, characterID, companionID, pickIndex)
					if err != nil {
						return nil, err
					}
					lookupOptions = subOptions
					for i := 0; i < spec.Quantity; i++ {
						if i == pickIndex {
							continue
						}
						opts, err := spec.DynamicOptions(s, characterID, companionID, i)
						if err != nil {
							return nil, err
						}
						lookupOptions = append(lookupOptions, opts...)
					}
				case spec.FixedOptions != nil:
					subOptions = spec.FixedOptions
					lookupOptions = subOptions
				default:
					var err error
					subOptions, err = s.loadPuppetUpgradeSubChoiceOptions(spec.EquipmentSlugs)
					if err != nil {
						return nil, err
					}
					lookupOptions = subOptions
				}
				byChoiceSlug := map[string]puppetUpgradeSubChoiceOption{}
				for _, opt := range lookupOptions {
					byChoiceSlug[opt.Slug] = opt
				}
				for _, ch := range choicesByUpgradeID[p.ID] {
					if opt, ok := byChoiceSlug[ch.ChoiceSlug]; ok {
						pick.SubChoicePicks = append(pick.SubChoicePicks, puppetUpgradeSubChoicePick{ID: ch.ID, puppetUpgradeSubChoiceOption: opt})
					}
				}
				if puppetUpgradeUniqueSubChoiceEntries[p.UpgradeEntrySlug] {
					used := usedSubChoiceValues[p.UpgradeEntrySlug]
					var open []puppetUpgradeSubChoiceOption
					for _, opt := range subOptions {
						if !used[opt.Slug] {
							open = append(open, opt)
						}
					}
					subOptions = open
				}
				pick.SubChoiceQuantity = spec.Quantity
				pick.SubChoiceOptions = subOptions
			}
			pickViews = append(pickViews, pick)
		}

		// Merge in any puppetUpgradeAutoGrants entry that belongs to THIS
		// tier and isn't already a real pick (defensive — it never should
		// be, since it's free and never offered in the normal Add form, but
		// this avoids a double listing if that ever changes). Appended
		// after the real picks loop above so it never disturbs Used/Cap,
		// which are computed independently from usedByTierName.
		for entrySlug := range puppetUpgradeAutoGrantedEntrySlugs(subclassColor, puppetMasterLevel) {
			if v, ok := byOptionSlug[entrySlug]; ok && pickedCountByEntrySlug[entrySlug] == 0 {
				pickViews = append(pickViews, puppetUpgradePick{puppetUpgradeOption: v, Granted: true})
			}
		}
		// Same merge, for this companion's own Foundation catalog pick's
		// fixed free-Upgrade grant (Quadrupedal -> Bulky Build, etc.) — see
		// puppetFoundationFixedGrants' own doc.
		for _, fp := range foundationPicks {
			entrySlug, ok := puppetFoundationFixedGrants[fp.Entry.EntrySlug]
			if !ok {
				continue
			}
			if v, ok := byOptionSlug[entrySlug]; ok && pickedCountByEntrySlug[entrySlug] == 0 {
				pickViews = append(pickViews, puppetUpgradePick{puppetUpgradeOption: v, Granted: true})
			}
		}

		// Same merge, for a resolved ChoiceUpgradeEntry pick (Purple
		// Technique Proficiency, Chakra String Augments) — the character's
		// own choice among several named free Upgrades, rather than a
		// single fixed grant like puppetUpgradeAutoGrants above.
		for featureSlug := range puppetUpgradeFeatureChoiceGrants {
			entrySlug, ok := resolvedFeatureChoices[features.ChoiceKey{FeatureSlug: featureSlug, ChoiceIndex: 0}]
			if !ok {
				continue
			}
			if v, ok := byOptionSlug[entrySlug]; ok && pickedCountByEntrySlug[entrySlug] == 0 {
				pickViews = append(pickViews, puppetUpgradePick{puppetUpgradeOption: v, Granted: true})
			}
		}
		// Same merge, for Symphony of Puppetry's Expansion branch's own free
		// Expanded Puppetry grant (computed just above, not a fixed map
		// lookup — see puppetSymphonyExpansionGrant).
		if symphonyExpansionEntrySlug != "" {
			if v, ok := byOptionSlug[symphonyExpansionEntrySlug]; ok && pickedCountByEntrySlug[symphonyExpansionEntrySlug] == 0 {
				pickViews = append(pickViews, puppetUpgradePick{puppetUpgradeOption: v, Granted: true})
			}
		}

		// An untiered list (Puppet Weapon Types, Puppeteer Chassis, Puppet
		// Frameworks, Puppet Roles) prints one class_options ROW per pickable
		// option, all sharing one list_name — without this merge, each row
		// became its own independent tier with its own independent cap=1,
		// so nothing stopped picking Drone Weapon AND Ogre Weapon AND
		// Sentinel Weapon at once (three separately-satisfied 1/1 caps
		// instead of one real "pick one of the three" cap). Merged here into
		// ONE tier per list_name instead, TierName set to the list's own
		// name so the sheet shows one "Puppet Weapon Types" section with all
		// options and one real 1/1 cap — mirrors the classWide merge just
		// below, keyed on list_name instead of tier name since these rows
		// have no shared tier name to merge on at all.
		if untiered {
			if idx, ok := untieredListIndexByListName[o.listName]; ok {
				tiers[idx].Options = append(tiers[idx].Options, options...)
				tiers[idx].Picks = append(tiers[idx].Picks, pickViews...)
				tiers[idx].Used += usedByTierName[o.name]
				continue
			}
			tiers = append(tiers, puppetUpgradeTier{
				ClassOptionSlug: o.slug, ListName: o.listName, TierName: o.listName, Prerequisites: o.prereq,
				Cap: cap, Used: usedByTierName[o.name], Options: options, Picks: pickViews,
			})
			untieredListIndexByListName[o.listName] = len(tiers) - 1
			continue
		}

		if o.classWide {
			if idx, ok := tierIndexByName[o.name]; ok {
				tiers[idx].Options = append(tiers[idx].Options, options...)
				tiers[idx].Picks = append(tiers[idx].Picks, pickViews...)
				continue
			}
		}

		tiers = append(tiers, puppetUpgradeTier{
			ClassOptionSlug: o.slug, ListName: o.listName, TierName: o.name, Prerequisites: o.prereq,
			Cap: cap, Used: usedByTierName[o.name], Options: options, Picks: pickViews,
		})
		if !o.classWide {
			tierIndexByName[o.name] = len(tiers) - 1
		}
	}
	return tiers, nil
}

// companionAbilityModifier resolves a three-letter ability key ("str",
// "dex", ...) against the COMPANION's own ability scores — unlike a custom
// attack on the player character's own sheet (composeCustomAttacks), a
// companion attack's ability term is never the player's own score, so this
// cannot reuse charsheet.Sheet.Abilities. An unrecognized/empty key (no
// ability chosen) resolves to a flat 0, same as a custom attack row with no
// ability picked.
func companionAbilityModifier(c charstore.Companion, key string) int {
	var score sql.NullInt64
	switch key {
	case "str":
		score = c.Str
	case "dex":
		score = c.Dex
	case "con":
		score = c.Con
	case "int":
		score = c.Int
	case "wis":
		score = c.Wis
	case "cha":
		score = c.Cha
	default:
		return 0
	}
	return charsheet.AbilityModifier(int(score.Int64))
}

// companionAttackRow is one stored companion attack plus the totals its
// parts add up to right now — same shape as characters.go's own
// customAttackRow, since the roll buttons need the composition done at
// render time.
type companionAttackRow struct {
	charstore.CompanionAttack
	AttackTotal int
	DamageTotal int
	// CritRangeThreshold: the lowest d20 result that counts as a critical
	// hit for THIS attack roll — 20 for every ordinary attack, 19 for a
	// companion holding Puppet Roles' Lurker (its own Role Effect widens
	// the crit range by 1). Read by the roll button's own data-crit-range
	// attribute (companion_fields.html/sheet-chat.js) rather than assumed
	// client-side, since it varies per companion, not per attack.
	CritRangeThreshold int
	// CritDamageBonus: the flat bonus to add to a Damage roll immediately
	// following a crit on this row's own Attack roll — Puppet Roles'
	// Lurker's own Role Effect. 0 for every companion without it. Read by
	// the roll button's own data-crit-damage-bonus attribute
	// (companion_fields.html/character-sheet.js), which folds it in only
	// when the immediately-preceding Attack roll on the SAME row crit (see
	// dice-roller.js's onResult).
	CritDamageBonus int
}

// composeCompanionAttacks resolves each row's ability/proficiency/flat
// parts against the companion's OWN ability scores and the OWNING
// character's proficiency bonus — per the confirmed "Proficiency = Puppet
// Master's Proficiency" rule on the baseline stat block, a Puppet Tool's
// attacks borrow the Puppet Master's own proficiency bonus, not a
// proficiency bonus of its own.
func composeCompanionAttacks(list []charstore.CompanionAttack, companion charstore.Companion, ownerProfBonus int) []companionAttackRow {
	out := make([]companionAttackRow, 0, len(list))
	for _, a := range list {
		out = append(out, companionAttackRow{
			CompanionAttack: a,
			AttackTotal: charsheet.ComposeModifier(
				companionAbilityModifier(companion, a.AttackAbility), ownerProfBonus, a.AttackProf, a.AttackBonus),
			DamageTotal: companionAbilityModifier(companion, a.DamageAbility) + a.DamageBonus,
		})
	}
	return out
}

// puppetIntegratedWeaponAttack computes the ONE weapon attack a given
// Integrated Weapon sub-choice (Arm Blades or Axe Tail) grants — "these
// weapons use your Ninjutsu/Taijutsu modifier for attack and damage rolls,"
// the higher of the character's own two rather than picking one
// arbitrarily. Never stored as a charstore.CompanionAttack row (see
// integratedWeaponEntrySlug's own doc) — computed fresh from the sheet on
// every render instead, the same "never store derived math" rule the rest
// of this app follows; nil for a choice slug this doesn't recognize.
func puppetIntegratedWeaponAttack(sheet *charsheet.Sheet, choiceSlug string) *companionAttackRow {
	var name, damageType string
	var damageSides int
	switch choiceSlug {
	case "arm-blades":
		name, damageSides, damageType = "Arm Blades", 6, "slashing"
	case "axe-tail":
		name, damageSides, damageType = "Axe Tail", 8, "piercing"
	default:
		return nil
	}
	if sheet == nil {
		return nil
	}
	modifier, found := 0, false
	for _, a := range sheet.JutsuAttacks {
		if a.Kind != "Ninjutsu" && a.Kind != "Taijutsu" {
			continue
		}
		if !found || a.Modifier > modifier {
			modifier = a.Modifier
			found = true
		}
	}
	return &companionAttackRow{
		CompanionAttack: charstore.CompanionAttack{
			Name: name, DamageCount: 1, DamageSides: damageSides, DamageType: damageType,
		},
		AttackTotal: modifier,
		DamageTotal: modifier,
	}
}

// puppetUpgradeGrantedAttacks resolves every ATTACK an already-taken
// upgrade grants beyond charstore's own stored companion attacks — a small
// hand-curated set, same "one confirmed instance, not a claim of full
// catalog coverage" precedent as puppetUpgradeSubChoiceSpecs. Integrated
// Weapon is the only one confirmed so far; a future one (any subclass, not
// just Purple Technique) gets its own case here.
func puppetUpgradeGrantedAttacks(sheet *charsheet.Sheet, upgradePicks []charstore.CompanionUpgrade, choices []charstore.CompanionUpgradeChoice) []companionAttackRow {
	integratedWeaponUpgradeIDs := map[int64]bool{}
	for _, u := range upgradePicks {
		if u.UpgradeEntrySlug == integratedWeaponEntrySlug {
			integratedWeaponUpgradeIDs[u.ID] = true
		}
	}
	if len(integratedWeaponUpgradeIDs) == 0 {
		return nil
	}
	var out []companionAttackRow
	for _, ch := range choices {
		if !integratedWeaponUpgradeIDs[ch.CompanionUpgradeID] {
			continue
		}
		if row := puppetIntegratedWeaponAttack(sheet, ch.ChoiceSlug); row != nil {
			out = append(out, *row)
		}
	}
	return out
}

// puppetCompanionView is one kind="puppet" companion plus everything the
// Puppets tab shows alongside its own stat fields: the computed baseline
// hints (never silently overwriting the stored, editable values), its own
// upgrade catalog/picks, and its own structured attack list.
type puppetCompanionView struct {
	charstore.Companion
	ExpectedMaxHP    int
	ExpectedAC       int
	ExpectedSpeed    int
	ExpectedFlySpeed int
	// ExpectedStr..ExpectedCha/ExpectedSize are the same "computed hint,
	// never silently overwritten, Sync button available" treatment as
	// ExpectedAC etc. above — sourced from the class baseline PLUS whichever
	// Puppeteer Chassis/Puppet Framework/Puppet Role/Puppet Weapon Type this
	// companion has picked (see puppet_foundation.go).
	ExpectedStr, ExpectedDex, ExpectedCon int
	ExpectedInt, ExpectedWis, ExpectedCha int
	ExpectedSize                          string
	UpgradeLists                          []puppetUpgradeList
	Attacks                               []companionAttackRow
	PuppetSkills                          []puppetSkillGroupRow
	// FoundationCatalogLabel/FoundationEntryName/FoundationTraits: this
	// companion's own resolved Foundation catalog pick (at most one in
	// practice — these 4 catalogs are each gated to a single subclass) —
	// every clause its own text grants with no computed field to land in,
	// plus any Role Effect that resolves to a spelled-out number rather
	// than a live field. Blank/nil when no Foundation pick has been made
	// yet, or the pick has no such clauses at all.
	FoundationCatalogLabel string
	FoundationEntryName    string
	FoundationTraits       []string
	// Seals/SealCap/SealOptions: Chakra Enhanced Retrofit (Class: Puppet
	// Master/3rd Level) — nil/0 gate, not shown at all, when the character
	// hasn't reached level 3 yet (see loadPuppetsTabData).
	Seals       []puppetSealPick
	SealCap     int
	SealOptions []puppetSealOption
	// UpgradeBonuses is every modeled always-on numeric upgrade bonus this
	// companion currently has (see puppet_upgrade_bonuses.go). Each is
	// folded into the very first computed AC/Max HP/Speed/flying speed a
	// brand-new companion backfills to (see the SetCompanionStatDefaults
	// call below), but — same as every other computed default on this tab —
	// never silently rewritten into an EXISTING companion's own already-set
	// value, so they are also surfaced as a hint with a Sync button per
	// affected field.
	UpgradeBonuses []puppetUpgradeBonusNote
	// ChassisProperties is the worn chassis's own property list, with any
	// Reinforced value already adjusted for Enhanced Durability (Purple).
	ChassisProperties []puppetArmorChassisProperty
	// ElevatedDesignBonus is Black Technique's own +2-per-ability bonus
	// (map[ability]amount), applied to every Puppet Tool once resolved —
	// nil if not resolved yet or not applicable. Read-only annotation
	// only; see puppetElevatedDesignAbilityBonus's own doc for why it is
	// never folded into the editable ability score fields.
	ElevatedDesignBonus map[string]int
	// PuppetToolASIBonus is Puppet Tool's own base-class per-ASI-breakpoint
	// ability bonus (map[ability]amount), applied to every Puppet Tool the
	// character possesses — nil if no breakpoint has been resolved yet.
	// Same display-only annotation treatment as ElevatedDesignBonus; see
	// puppetToolASIAbilityBonus's own doc for the bonus math.
	PuppetToolASIBonus map[string]int
	// SymphonyEnhancement is set only for the (at most two) Puppet Tools
	// Symphony of Puppetry's Enhancement branch applies to, once that
	// branch has been chosen — nil otherwise.
	SymphonyEnhancement *puppetSymphonyEnhancementView
	// IsMatryoshkaFramework marks a companion that has picked Matryoshka
	// Framework (puppetupgrades.FoundationEntry.SplitBodyFramework) —
	// gates the Split-into-bodies control on the Puppets tab. A split
	// sibling body has no Foundation pick of its own (only the original
	// companion row does), so the tab also shows the Matryoshka block on
	// any companion whose MatryoshkaGroupID is set, regardless of this
	// flag — see character_sheet.html's own gate.
	IsMatryoshkaFramework bool
}

// puppetUpgradeBonusNote is one modeled upgrade's contribution, as shown on
// the card: the upgrade's name and the fields it moved.
type puppetUpgradeBonusNote struct {
	Name  string
	Parts []string // "+1 AC", "+15 ft. Speed", "30 ft. flying speed"
}

// puppetsTabData is everything the Puppets tab (and its "sheet_puppet_tab"
// fragment) needs, assembled once and shared by both the full-page render
// and the fragment re-render after an add/delete/upgrade-pick action.
// blueTechniqueProficiencyFeatureSlug is Blue Technique Warmaster's own L2
// feature ("You also gain 1 Fighting Stance from Chapter 13: Customization
// options") — unlike Taijutsu's Unarmed Technique, the book's text here
// doesn't say "Taijutsu Stance," so the catalog offered is every row in
// fighting_stances (both stance_type values), not just the 9 taijutsu ones.
const blueTechniqueProficiencyFeatureSlug = "class/puppet-master/group/puppet-techniques/blue-technique-warmaster/feature/blue-technique-proficiency"

// loadFightingStanceOptions catalogs every Chapter 13 Fighting Stance,
// taijutsu and bukijutsu alike — see blueTechniqueProficiencyFeatureSlug's
// own doc for why this differs from taijutsu.go's loadTaijutsuStanceOptions
// (which deliberately filters to stance_type='taijutsu' for Unarmed
// Technique's own narrower grant).
func (s *server) loadFightingStanceOptions() ([]stanceOption, error) {
	rows, err := s.rulesDB.Query(`SELECT slug, name, description FROM fighting_stances ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []stanceOption
	for rows.Next() {
		var o stanceOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		options = append(options, o)
	}
	return options, rows.Err()
}

// transformerWeaponTypeFeatureSlug is Blue Technique Warmaster's own L14
// Transformer feature — "on a long rest select a second Puppet Weapon Type
// for your Puppet Tool." A distinct feature slug from the character's
// PRIMARY Weapon Type pick, which isn't a feature choice at all (it's a
// Foundation catalog pick — a real character_companion_upgrades row, see
// puppet_foundation.go), so the two can never collide in storage.
const transformerWeaponTypeFeatureSlug = "class/puppet-master/group/puppet-techniques/blue-technique-warmaster/feature/transformer"

// puppetWeaponTypeOption is one Puppet Weapon Type offered by Transformer's
// own second-Weapon-Type pick, read straight from class_options (the same 3
// rows internal/puppetupgrades/foundation.go's FoundationEntries table
// curates for the PRIMARY pick's own ability-score/size/natural-weapon
// auto-builder) rather than through that curated table — this pick is
// display-only reference text, deliberately NOT run through the Foundation
// auto-builder: the puppet alternates between its two Weapon Types rather
// than having both active at once, so auto-applying the second one's
// bonuses on top of the first would double-stack them.
type puppetWeaponTypeOption struct {
	Slug        string
	Name        string
	Description string
}

// puppetWeaponTypeChoiceView holds Transformer's own second-Weapon-Type
// pick — freely re-editable ("on a long rest select…"), same boundary as
// Blue Technique's own Fighting Stance pick (fightingStanceView) above.
type puppetWeaponTypeChoiceView struct {
	Current            string // weapon-type slug, "" if unpicked
	CurrentName        string
	CurrentDescription string
	Options            []puppetWeaponTypeOption
}

// loadPuppetWeaponTypeOptions catalogs the 3 Puppet Weapon Types (Drone/
// Ogre/Sentinel Weapon) directly from class_options, for Transformer's own
// display-only second pick.
func (s *server) loadPuppetWeaponTypeOptions() ([]puppetWeaponTypeOption, error) {
	rows, err := s.rulesDB.Query(
		`SELECT slug, name, description FROM class_options WHERE list_name = 'Puppet Weapon Types' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []puppetWeaponTypeOption
	for rows.Next() {
		var o puppetWeaponTypeOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		options = append(options, o)
	}
	return options, rows.Err()
}

// masterOfGreenTechniqueFeatureSlug is Green Technique Marionettist's own
// L20 capstone — "Each long rest, select one Ninjutsu or Genjutsu of B-Rank
// or lower that you could learn… Your Puppets are now always under the
// effects of your chosen jutsu as long as you are conscious."
const masterOfGreenTechniqueFeatureSlug = "class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/master-of-the-green-technique"

// threadSavantFeatureSlug is White Technique Weaver's own L14 feature —
// "Select one jutsu that you know… you may switch your chosen jutsu at the
// conclusion of a full rest."
const threadSavantFeatureSlug = "class/puppet-master/group/puppet-techniques/white-technique-weaver/feature/thread-savant"

// puppetJutsuChoiceOption is one jutsu candidate offered by a long-rest/
// full-rest re-pickable single jutsu choice (Master of the Green Technique,
// Thread Savant) — both freely re-editable via charstore.SetFeatureChoice
// rather than a capped catalog pick (choices.go's own ChoiceKind/
// choiceSlotDefs engine locks a resolved pick permanently, wrong for a
// feature the player must reselect every rest), so both share this same
// small view shape despite sourcing from different catalogs. Value is the
// string actually stored as the ChoiceKey's value: a rules-db jutsu slug
// for Master of the Green Technique, or a character_jutsu row id (as
// decimal text) for Thread Savant, whose own candidates can include a
// player-created custom jutsu with no slug of its own.
type puppetJutsuChoiceOption struct {
	Value       string
	Name        string
	Rank        string
	Description string
}

// puppetJutsuChoiceView holds one of these single re-pickable jutsu
// choices' current pick — same shape fightingStanceView already
// establishes for a freely re-editable single choice.
type puppetJutsuChoiceView struct {
	Current            string
	CurrentName        string
	CurrentRank        string
	CurrentDescription string
	Options            []puppetJutsuChoiceOption
}

// buildPuppetJutsuChoiceView resolves featureSlug's own stored pick against
// options — shared by Master of the Green Technique and Thread Savant, same
// role buildFightingStanceView (taijutsu.go) plays for stance picks.
func buildPuppetJutsuChoiceView(choices map[features.ChoiceKey]string, options []puppetJutsuChoiceOption, featureSlug string) *puppetJutsuChoiceView {
	view := &puppetJutsuChoiceView{
		Current: choices[features.ChoiceKey{FeatureSlug: featureSlug, ChoiceIndex: 0}],
		Options: options,
	}
	for _, o := range options {
		if o.Value == view.Current {
			view.CurrentName = o.Name
			view.CurrentRank = o.Rank
			view.CurrentDescription = o.Description
		}
	}
	return view
}

// masterOfGreenTechniqueMaxRank is Master of the Green Technique's own
// fixed "B-Rank or lower" cap — unlike the ordinary highestRankForLevel
// gate most rank-limited picks in this app use, this feature's own text
// never scales the cap by character level.
var masterOfGreenTechniqueMaxRank = jutsuRankOrder["B"]

// loadGreenTechniqueJutsuOptions catalogs every Ninjutsu/Genjutsu of B-Rank
// or lower the character could learn (jutsuEligible: class/clan origin +
// elemental affinity, the same gate handleSheetJutsuAdd enforces), for
// Master of the Green Technique's own long-rest jutsu pick. Eligibility is
// computed once via loadJutsuEligibilityContext and reused for every
// candidate — jutsuEligibleForCharacter's own per-slug query cost would
// otherwise be paid once per candidate jutsu (several hundred rows even
// after the rank filter below). The book's further restrictions — "does
// not deal damage or affect any other creature," "casting time of 1 action
// or Bonus Action" — have no queryable rules-db column to filter on, so
// they're left as player-enforced text in the picker's own label rather
// than silently mis-filtering the list.
func (s *server) loadGreenTechniqueJutsuOptions(characterID int64) ([]puppetJutsuChoiceOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, rank, keywords, description FROM v_jutsu
		WHERE classification IN ('Ninjutsu', 'Genjutsu') ORDER BY name`)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		slug, name, keywords, description string
		rank                              sql.NullString
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.slug, &c.name, &c.rank, &c.keywords, &c.description); err != nil {
			rows.Close()
			return nil, err
		}
		if !c.rank.Valid || jutsuRankOrder[c.rank.String] > masterOfGreenTechniqueMaxRank {
			continue
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ctx, err := s.loadJutsuEligibilityContext(characterID)
	if err != nil {
		return nil, err
	}
	var out []puppetJutsuChoiceOption
	for _, c := range candidates {
		if !ctx.eligible(c.slug, c.keywords, c.rank.String) {
			continue
		}
		out = append(out, puppetJutsuChoiceOption{Value: c.slug, Name: c.name, Rank: c.rank.String, Description: c.description})
	}
	return out, nil
}

// loadThreadSavantJutsuOptions catalogs every jutsu the character already
// knows (character_jutsu, published and custom alike) as candidates for
// Thread Savant's own full-rest re-pickable jutsu choice — unlike Master of
// the Green Technique, the book's text scopes this to jutsu "that you
// know," not the full catalog. A stale published jutsu_slug (removed in a
// rules update) is silently skipped, same tolerance ninjutsu_specialist.go's
// loadKnownNinjutsu already extends. The book's further restrictions — "does
// not deal damage, does not inflict a condition," "casting time of 1 action
// or Bonus Action" — have no queryable rules-db column to filter on (custom
// jutsu doubly so), so they're left as player-enforced text in the picker's
// own label rather than silently mis-filtering the list.
func (s *server) loadThreadSavantJutsuOptions(characterID int64) ([]puppetJutsuChoiceOption, error) {
	rows, err := s.charDB.Query(
		`SELECT id, jutsu_slug, custom_jutsu_id FROM character_jutsu WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	type stored struct {
		id        int64
		jutsuSlug sql.NullString
		customID  sql.NullInt64
	}
	var picks []stored
	for rows.Next() {
		var p stored
		if err := rows.Scan(&p.id, &p.jutsuSlug, &p.customID); err != nil {
			rows.Close()
			return nil, err
		}
		picks = append(picks, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []puppetJutsuChoiceOption
	for _, p := range picks {
		var name, rank, description string
		switch {
		case p.jutsuSlug.Valid:
			err := s.rulesDB.QueryRow(
				`SELECT name, rank, description FROM v_jutsu WHERE slug = ?`, p.jutsuSlug.String,
			).Scan(&name, &rank, &description)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
		case p.customID.Valid:
			err := s.charDB.QueryRow(
				`SELECT name, rank, description FROM custom_jutsu WHERE id = ?`, p.customID.Int64,
			).Scan(&name, &rank, &description)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
		default:
			continue
		}
		out = append(out, puppetJutsuChoiceOption{Value: strconv.FormatInt(p.id, 10), Name: name, Rank: rank, Description: description})
	}
	return out, nil
}

// alwaysPreparedTemporaryUpgradeFeatureSlug is Always Prepared's own
// 18th-level clause — "after a rest of any type, you can select and create
// a temporary version of an Upgrade from your Puppet Technique you are
// qualified to take but do not have. This Upgrade must be of Silver tier or
// lower. You have this Upgrade until you complete a rest." A base-class
// feature (available to every Puppet Technique, not gated to one subclass),
// distinct from this same feature's own twice-per-full-rest kit/pill
// conversion clause, which is tracked separately via customResourceGrants'
// "always_prepared_conversion" key (custom_resources.go) — a completely
// different table (character_feature_choices via charstore.SetFeatureChoice,
// here, vs. character_custom_resources there), so the two can never collide
// despite sharing this same class_features slug as their common key.
const alwaysPreparedTemporaryUpgradeFeatureSlug = "class/puppet-master/feature/always-prepared"

// alwaysPreparedMaxTierRank is Always Prepared's own fixed "Silver tier or
// lower" cap on its 18th-level temporary-Upgrade pick — the same fixed-cap
// simplification masterOfGreenTechniqueMaxRank uses above, just resolved
// against puppetUpgradeTierRanks instead of jutsuRankOrder.
var alwaysPreparedMaxTierRank = puppetUpgradeTierRanks["Silver Tier"]

// puppetUpgradeChoiceOption is one not-yet-held Puppet Upgrade entry offered
// by Always Prepared's own temporary-Upgrade pick — same role
// puppetJutsuChoiceOption plays for the jutsu-flavored single-pick features
// above, just sourced from the Upgrade catalog (class_option_entries)
// instead of v_jutsu/character_jutsu.
type puppetUpgradeChoiceOption struct {
	Slug        string
	Name        string
	Tier        string
	Description string
}

// puppetUpgradeChoiceView holds Always Prepared's own current temporary-
// Upgrade pick — same shape puppetJutsuChoiceView already establishes for a
// freely re-editable single choice.
type puppetUpgradeChoiceView struct {
	Current            string
	CurrentName        string
	CurrentTier        string
	CurrentDescription string
	Options            []puppetUpgradeChoiceOption
}

// buildPuppetUpgradeChoiceView resolves alwaysPreparedTemporaryUpgradeFeatureSlug's
// own stored pick against options — same role buildPuppetJutsuChoiceView
// plays for the jutsu-flavored single-pick features above.
func buildPuppetUpgradeChoiceView(choices map[features.ChoiceKey]string, options []puppetUpgradeChoiceOption, featureSlug string) *puppetUpgradeChoiceView {
	view := &puppetUpgradeChoiceView{
		Current: choices[features.ChoiceKey{FeatureSlug: featureSlug, ChoiceIndex: 0}],
		Options: options,
	}
	for _, o := range options {
		if o.Slug == view.Current {
			view.CurrentName = o.Name
			view.CurrentTier = o.Tier
			view.CurrentDescription = o.Description
		}
	}
	return view
}

// loadAlwaysPreparedUpgradeOptions catalogs every Puppet Upgrade of Silver
// tier or lower the character is qualified to take but doesn't already
// have, for Always Prepared's own 18th-level temporary-Upgrade pick.
// Eligibility runs through puppetUpgradeEntryPrereqMet — the exact same
// server-side gate handlePuppetUpgradeAdd itself uses for a REAL Upgrade
// pick — checked against the character's first Puppet Tool companion, since
// (like Transformer/Master of the Green Technique/Thread Savant above) this
// pick is stored character-wide via charstore.SetFeatureChoice, not scoped
// to one companion. An "already held" entry is excluded if ANY of the
// character's puppet companions has it for real (puppetEntryTotalPicksAcrossCompanions'
// own "pool across the whole roster" treatment, not just the one companion
// used for the prerequisite check), so a second Puppet Tool that already
// carries an Upgrade for real is never re-offered here as if it were still
// available. Untiered rows (Puppet Weapon Types, Puppeteer Chassis, Puppet
// Frameworks, Puppet Roles — none of them a puppetUpgradeTierRanks tier)
// are naturally excluded by the same rank filter that caps out at Silver:
// the book's own "Upgrade... of Silver tier or lower" phrase only ever
// refers to the tiered Upgrade catalog, never those untiered Foundation-style
// picks.
func (s *server) loadAlwaysPreparedUpgradeOptions(characterID int64) ([]puppetUpgradeChoiceOption, error) {
	companions, err := charstore.ListCompanions(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	var companionID int64
	var armorChassis string
	haveEntry := map[string]bool{}
	for _, c := range companions {
		if c.Kind != "puppet" {
			continue
		}
		if companionID == 0 {
			companionID = c.ID
			armorChassis = c.ArmorChassis
		}
		picks, err := charstore.ListCompanionUpgrades(s.charDB, characterID, c.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range picks {
			haveEntry[p.UpgradeEntrySlug] = true
		}
	}
	if companionID == 0 {
		return nil, nil // no Puppet Tool yet — nothing to check eligibility against
	}

	subclassSlug, _, err := s.puppetMasterSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	subclassColor := puppetSubclassColorBySlug[subclassSlug]

	rows, err := s.rulesDB.Query(`
		SELECT e.slug, e.name, e.description, o.name
		FROM class_option_entries e
		JOIN class_options o ON o.slug = e.class_option_slug
		WHERE o.class_slug = 'class/puppet-master' AND (o.subclass_slug IS NULL OR o.subclass_slug = ?)
		ORDER BY o.list_name, o.sort_order, e.sort_order`, subclassSlug)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		slug, name, description, tierName string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.slug, &c.name, &c.description, &c.tierName); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []puppetUpgradeChoiceOption
	for _, c := range candidates {
		if haveEntry[c.slug] {
			continue
		}
		rank, ok := puppetUpgradeTierRanks[c.tierName]
		if !ok || rank > alwaysPreparedMaxTierRank {
			continue
		}
		if !puppetUpgradeAvailableToColor(c.description, subclassColor) {
			continue
		}
		met, err := s.puppetUpgradeEntryPrereqMet(characterID, companionID, c.slug, armorChassis)
		if err != nil {
			return nil, err
		}
		if !met {
			continue
		}
		out = append(out, puppetUpgradeChoiceOption{Slug: c.slug, Name: c.name, Tier: c.tierName, Description: c.description})
	}
	return out, nil
}

type puppetsTabData struct {
	PuppetMasterLevel     int
	Companions            []puppetCompanionView
	ArmorChassisOptions   []puppetArmorChassis
	GeneralizedSkillCap   int
	GeneralizedSkillUsed  int
	GeneralizedSkillPicks []generalizedSkillPickRow
	// AllSkillNames feeds the Generalized Skill picker's own <select> —
	// every named skill not already chosen, in charsheet.SkillAbility's own
	// grouping order (matches the Core tab's own Skills box layout).
	AllSkillNames []string
	// BaseTraits: the Puppet Tool's baseline Saving Throws/Damage
	// Resistance/Damage Immunity/Condition Immunities/Weapon Proficiencies/
	// Senses/traits — universal to every Puppet Master subclass (see
	// puppetToolStatBlock's own doc), shown once per companion card rather
	// than computed per-companion since none of it varies by companion.
	BaseTraits *puppetToolStatBlock
	// FightingStance is set only for Blue Technique Warmaster — its own L2
	// feature grants a Fighting Stance pick that, before this, had nowhere
	// to record a choice: taijutsu.go's own fighting-stance box only
	// renders for a character with Taijutsu Specialist levels.
	FightingStance *fightingStanceView
	// PuppetSwarm is set only for Red Technique Performer at L10+
	// (Performance of 10 Puppets) — see puppet_swarm.go.
	PuppetSwarm *puppetSwarmStatsView
	// TransformerWeaponType is set only for Blue Technique Warmaster at
	// L14+ (Transformer) — its own long-rest second Puppet Weapon Type
	// pick, display-only (see transformerWeaponTypeFeatureSlug's own doc).
	TransformerWeaponType *puppetWeaponTypeChoiceView
	// GreenTechniqueJutsu is set only for Green Technique Marionettist at
	// L20+ (Master of the Green Technique) — its own long-rest jutsu pick.
	GreenTechniqueJutsu *puppetJutsuChoiceView
	// ThreadSavantJutsu is set only for White Technique Weaver at L14+
	// (Thread Savant) — its own full-rest re-pickable jutsu choice.
	ThreadSavantJutsu *puppetJutsuChoiceView
	// AlwaysPreparedUpgrade is set for every Puppet Technique at L18+ (base
	// class Always Prepared's own 18th-level clause) — its own rest-
	// re-pickable temporary Upgrade choice, unlike the three subclass-gated
	// pickers above. See alwaysPreparedTemporaryUpgradeFeatureSlug's own doc
	// for the exact automation boundary.
	AlwaysPreparedUpgrade *puppetUpgradeChoiceView
	// MendingHealDie is Puppet Tool's own current Mending-casting die size
	// (puppetMendingHealDie) — a readout on the Puppet Tool Traits card,
	// not a spendable resource (Mending itself is unlimited, gated only by
	// action economy/chakra which are already tracked elsewhere).
	MendingHealDie string
}

func (s *server) loadPuppetsTabData(characterID int64, sheet *charsheet.Sheet) (puppetsTabData, error) {
	var data puppetsTabData

	level, err := s.puppetMasterClassLevel(characterID)
	if err != nil {
		return data, err
	}
	data.PuppetMasterLevel = level
	if level == 0 {
		return data, nil
	}
	data.MendingHealDie = puppetMendingHealDie(level)

	// Resolved once, up front, and reused everywhere below a subclass-color
	// gate is needed: Generalized Skill's own Green Technique Genjutsu-
	// modifier substitution (puppetGeneralizedSkillCap), the Blue/Green/
	// White-only upgrade pickers, and the roster-wide upgrade bonuses noted
	// further down.
	subclassSlug, _, err := s.puppetMasterSubclassSlug(characterID)
	if err != nil {
		return data, err
	}
	subclassColor := puppetSubclassColorBySlug[subclassSlug]

	generalizedPicks, err := charstore.ListGeneralizedSkills(s.charDB, characterID)
	if err != nil {
		return data, err
	}
	sort.Strings(generalizedPicks)
	data.GeneralizedSkillPicks = make([]generalizedSkillPickRow, len(generalizedPicks))
	for i, name := range generalizedPicks {
		var mod int
		if sheet != nil {
			mod, _ = charsheet.SkillModifier(sheet, name)
		}
		data.GeneralizedSkillPicks[i] = generalizedSkillPickRow{Name: name, Modifier: mod}
	}
	data.GeneralizedSkillUsed = len(generalizedPicks)
	data.GeneralizedSkillCap = puppetGeneralizedSkillCap(sheet, level, subclassColor)
	picked := map[string]bool{}
	for _, n := range generalizedPicks {
		picked[n] = true
	}
	for _, sk := range sheet.Skills {
		if !picked[sk.Name] {
			data.AllSkillNames = append(data.AllSkillNames, sk.Name)
		}
	}

	allCompanions, err := charstore.ListCompanions(s.charDB, characterID)
	if err != nil {
		return data, err
	}
	baseline, err := s.loadPuppetToolStatBlock()
	if err != nil {
		return data, err
	}
	data.BaseTraits = baseline
	masterConMod := 0
	masterProfBonus := 0
	if sheet != nil {
		masterConMod = sheet.Abilities["con"].Modifier
		masterProfBonus = sheet.ProficiencyBonus
	}

	// Armor Chassis (the Juggernaut Armor transformation, and its own AC
	// formula below) is Purple Technique Juggernaut's own 2nd-level
	// subclass feature — every other subclass's Puppet Tool keeps using
	// puppetToolDefaultAC instead. chassisByName stays empty for a non-
	// Purple character, so the ExpectedAC lookup against it below falls
	// through to that default formula even if a companion has a stale
	// ArmorChassis value from before this gate existed (e.g. a subclass
	// change after it was set).
	var chassisOptions []puppetArmorChassis
	chassisByName := map[string]puppetArmorChassis{}
	if subclassColor == "Purple" {
		chassisOptions, err = s.loadPuppetArmorChassisOptions()
		if err != nil {
			return data, err
		}
		for _, ch := range chassisOptions {
			chassisByName[ch.Name] = ch
		}
	}
	data.ArmorChassisOptions = chassisOptions

	// Several modeled upgrade bonuses read the character's WHOLE roster, not
	// just the companion being computed (Red Technique's Enhanced Durability
	// raises every Puppet Tool's HP from a single take; its Hovering
	// Mechanism reaches two puppets), so every puppet's picks are counted
	// up front rather than per companion inside the loop below. subclassColor
	// itself was already resolved near the top of this function.

	// Loaded once and reused below by the Blue Technique fighting-stance
	// picker, Elevated Design's ability bonus, and Symphony of Puppetry's
	// own branch resolution — all three read from the same resolved-choices
	// map rather than each re-querying it.
	resolvedFeatureChoices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return data, err
	}

	if subclassColor == "Blue" {
		stanceOptions, err := s.loadFightingStanceOptions()
		if err != nil {
			return data, err
		}
		stance := &fightingStanceView{
			Current: resolvedFeatureChoices[features.ChoiceKey{FeatureSlug: blueTechniqueProficiencyFeatureSlug, ChoiceIndex: 0}],
			Options: stanceOptions,
		}
		for _, o := range stanceOptions {
			if o.Slug == stance.Current {
				stance.CurrentName = o.Name
				stance.CurrentDescription = o.Description
			}
		}
		data.FightingStance = stance
	}

	// Transformer (Blue Technique Warmaster, L14): "on a long rest select a
	// second Puppet Weapon Type for your Puppet Tool." Freely re-editable,
	// same boundary as Blue Technique's own Fighting Stance pick just
	// above (a distinct feature slug, so the two can never collide), but
	// display-only: NOT run through the Foundation catalog's ability-
	// score/size/natural-weapon auto-builder the PRIMARY Weapon Type pick
	// uses — see puppetWeaponTypeOption's own doc for why. Transforming
	// in/out mid-combat, the object-interaction HP-split mechanic, and the
	// Bonus-Action Tactic swap in the same feature's last sentence all
	// stay fully manual/narrated — only WHICH second Weapon Type is
	// currently selected is tracked.
	if subclassColor == "Blue" && level >= 14 {
		weaponTypeOptions, err := s.loadPuppetWeaponTypeOptions()
		if err != nil {
			return data, err
		}
		choice := &puppetWeaponTypeChoiceView{
			Current: resolvedFeatureChoices[features.ChoiceKey{FeatureSlug: transformerWeaponTypeFeatureSlug, ChoiceIndex: 0}],
			Options: weaponTypeOptions,
		}
		for _, o := range weaponTypeOptions {
			if o.Slug == choice.Current {
				choice.CurrentName = o.Name
				choice.CurrentDescription = o.Description
			}
		}
		data.TransformerWeaponType = choice
	}

	// Master of the Green Technique (Green Technique Marionettist, L20):
	// "Each long rest, select one Ninjutsu or Genjutsu of B-Rank or lower
	// that you could learn… Your Puppets are now always under the effects
	// of your chosen jutsu as long as you are conscious." Freely
	// re-editable, sourced from the full rank/eligibility-filtered rules-db
	// catalog (loadGreenTechniqueJutsuOptions), not the character's own
	// known jutsu — the book's own text is "that you could learn," not
	// "that you know." The continuous "Puppets are always under its
	// effects" application stays entirely manual — only which jutsu is
	// currently selected is tracked.
	if subclassColor == "Green" && level >= 20 {
		jutsuOptions, err := s.loadGreenTechniqueJutsuOptions(characterID)
		if err != nil {
			return data, err
		}
		data.GreenTechniqueJutsu = buildPuppetJutsuChoiceView(resolvedFeatureChoices, jutsuOptions, masterOfGreenTechniqueFeatureSlug)
	}

	// Thread Savant (White Technique Weaver, L14): "Select one jutsu that
	// you know… you may switch your chosen jutsu at the conclusion of a
	// full rest." Freely re-editable, sourced from the character's own
	// already-known jutsu (loadThreadSavantJutsuOptions) rather than the
	// full catalog — the book's text is scoped to jutsu the character
	// already knows. The simultaneous-cast-on-thread-connect application,
	// and the "cannot benefit from Grandmaster Manipulation" interaction,
	// stay fully manual/narrated — only which known jutsu is currently
	// selected (and its full-rest re-selectability) is tracked.
	if subclassColor == "White" && level >= 14 {
		jutsuOptions, err := s.loadThreadSavantJutsuOptions(characterID)
		if err != nil {
			return data, err
		}
		data.ThreadSavantJutsu = buildPuppetJutsuChoiceView(resolvedFeatureChoices, jutsuOptions, threadSavantFeatureSlug)
	}

	// Always Prepared (base class, L18): "...select and create a temporary
	// version of an Upgrade from your Puppet Technique you are qualified to
	// take but do not have. This Upgrade must be of Silver tier or lower.
	// You have this Upgrade until you complete a rest." Available to every
	// Puppet Technique, unlike the three subclass-gated pickers above.
	// Freely re-editable every rest — only WHICH temporary Upgrade is
	// currently selected is tracked. The selected entry's own name/tier/
	// description resolve for display, and its eligibility is checked
	// through the same puppetUpgradeEntryPrereqMet gate a real Upgrade pick
	// uses (see loadAlwaysPreparedUpgradeOptions' own doc), but the entry's
	// own mechanical bonus is NOT applied to the companion stat block the
	// way a permanently-held Upgrade of the same entry would be — creating
	// and un-creating it as a temporary Upgrade every rest stays the
	// player's own bookkeeping, same as Transformer's second Weapon Type
	// above.
	if level >= 18 {
		upgradeOptions, err := s.loadAlwaysPreparedUpgradeOptions(characterID)
		if err != nil {
			return data, err
		}
		data.AlwaysPreparedUpgrade = buildPuppetUpgradeChoiceView(resolvedFeatureChoices, upgradeOptions, alwaysPreparedTemporaryUpgradeFeatureSlug)
	}

	if subclassColor == "Red" && level >= 10 {
		swarm := puppetSwarmStats(allCompanions, level)
		refText, err := s.loadPuppetSwarmReferenceText()
		if err != nil {
			return data, err
		}
		swarm.ReferenceText = refText
		data.PuppetSwarm = &swarm
	}

	elevatedDesignBonus := puppetElevatedDesignAbilityBonus(resolvedFeatureChoices)
	puppetToolASIBonus := puppetToolASIAbilityBonus(resolvedFeatureChoices)

	symphonyEnhancementViews, err := s.puppetSymphonyEnhancementViews(characterID, resolvedFeatureChoices, allCompanions)
	if err != nil {
		return data, err
	}

	picksByCompanion := map[int64]map[string]int{}
	picksAnyCompanion := map[string]int{}
	upgradesByCompanion := map[int64][]charstore.CompanionUpgrade{}
	upgradeChoicesByCompanion := map[int64][]charstore.CompanionUpgradeChoice{}
	foundationPicksByCompanion := map[int64][]puppetFoundationPick{}
	for _, c := range allCompanions {
		if c.Kind != "puppet" {
			continue
		}
		picks, err := charstore.ListCompanionUpgrades(s.charDB, characterID, c.ID)
		if err != nil {
			return data, err
		}
		choices, err := charstore.ListCompanionUpgradeChoices(s.charDB, characterID, c.ID)
		if err != nil {
			return data, err
		}
		upgradesByCompanion[c.ID] = picks
		upgradeChoicesByCompanion[c.ID] = choices
		foundationPicks := resolvePuppetFoundationPicks(picks, choices)
		foundationPicksByCompanion[c.ID] = foundationPicks

		counts := map[string]int{}
		for _, p := range picks {
			counts[p.UpgradeEntrySlug]++
			picksAnyCompanion[p.UpgradeEntrySlug]++
		}
		// A Foundation catalog pick's own fixed free-Upgrade grant (Bulky
		// Build, Quickfooted, ...) is display-only — no
		// character_companion_upgrades row of its own (see
		// puppetFoundationFixedGrants) — but if the granted entry itself
		// has a modeled always-on stat bonus (puppetupgrades.StatBonuses)
		// or flat skill grant (puppetFlatSkillGrants), that should still
		// apply for free, the same as an ordinary take of the same entry
		// would. Counted in here purely for that purpose — cap/tier
		// accounting never reads this map for anything but a real
		// class_options row.
		for _, fp := range foundationPicks {
			if grantedSlug, ok := puppetFoundationFixedGrants[fp.Entry.EntrySlug]; ok {
				counts[grantedSlug]++
				picksAnyCompanion[grantedSlug]++
			}
		}
		picksByCompanion[c.ID] = counts
	}

	for _, c := range allCompanions {
		if c.Kind != "puppet" {
			continue
		}
		upgradePicks := upgradesByCompanion[c.ID]
		upgradeChoices := upgradeChoicesByCompanion[c.ID]
		foundationPicks := foundationPicksByCompanion[c.ID]
		ownerProfBonus := 0
		if sheet != nil {
			ownerProfBonus = sheet.ProficiencyBonus
		}

		// A puppet with any stat field still NULL — created before this
		// feature computed any baseline at all, or one whose fields were
		// only ever partially set (e.g. AC/HP typed in by hand during play
		// before Speed/ability scores existed as tracked fields) — gets
		// those still-missing fields backfilled here. Safe to call
		// unconditionally on every render: SetCompanionStatDefaults writes
		// every column via COALESCE(column, ?), so a field that already has
		// a real value (including one the player customized) is never
		// touched, only a genuinely-NULL one gets the computed default.
		bonusCtx := puppetupgrades.BonusContext{
			SubclassColor: subclassColor,
			MasterLevel:   level,
			ArmorChassis:  c.ArmorChassis,
		}
		var total puppetupgrades.StatBonus
		var notes []puppetUpgradeBonusNote
		for _, ub := range puppetupgrades.StatBonuses {
			ctx := bonusCtx
			ctx.PicksOnThisCompanion = picksByCompanion[c.ID][ub.EntrySlug]
			ctx.PicksOnAnyCompanion = picksAnyCompanion[ub.EntrySlug]
			b := ub.Bonus(ctx)
			if b.IsZero() {
				continue
			}
			total.AC += b.AC
			total.MaxHP += b.MaxHP
			total.Speed += b.Speed
			if b.GrantsFlight && b.FlySpeed > total.FlySpeed {
				total.FlySpeed = b.FlySpeed
				total.GrantsFlight = true
			}
			notes = append(notes, puppetUpgradeBonusNote{Name: ub.Name, Parts: puppetupgrades.StatBonusParts(b)})
		}

		// Fold in this companion's own Foundation catalog pick (Puppeteer
		// Chassis/Puppet Framework/Puppet Role/Puppet Weapon Type) — see
		// puppet_foundation.go. abilitySets/abilityDeltas are applied to
		// the class baseline below (floor, then delta); speedSet/
		// flySpeedEqualsSpeed are resolved once the rest of total.Speed is
		// known; attackRollBonus/critRangeThreshold are applied to the
		// Attacks list once it's fully assembled; catalogLabel/entryName/
		// foundationTraits feed the Foundation Traits reference block.
		abilitySets := map[string]int{}
		abilityDeltas := map[string]int{}
		size := ""
		speedSet := 0
		flySpeedEqualsSpeed := false
		attackRollBonus := 0
		critRangeThreshold := 20
		critDamageBonus := 0
		skillAbilityOverride := map[string]string{}
		var catalogLabel, entryName string
		var foundationTraits []string
		isMatryoshkaFramework := false
		for _, fp := range foundationPicks {
			entry := fp.Entry
			sets, deltas := puppetFoundationAbilityAdjust(fp)
			for k, v := range sets {
				if cur, ok := abilitySets[k]; !ok || v > cur {
					abilitySets[k] = v
				}
			}
			for k, v := range deltas {
				abilityDeltas[k] += v
			}
			if sz := puppetFoundationSize(fp, level); sz != "" {
				size = sz
			}
			total.AC += entry.FlatACBonus
			total.Speed += entry.SpeedDelta
			if entry.SpeedSet != 0 {
				speedSet = entry.SpeedSet
			}
			if entry.FlySpeedEqualsSpeed {
				flySpeedEqualsSpeed = true
			}
			attackRollBonus += entry.FlatAttackRollBonus
			if entry.RoleEffect != nil {
				if entry.RoleEffect.ACBonus != nil {
					total.AC += entry.RoleEffect.ACBonus(ownerProfBonus)
				}
				if entry.RoleEffect.AttackRollBonus != nil {
					attackRollBonus += entry.RoleEffect.AttackRollBonus(ownerProfBonus)
				}
				if entry.RoleEffect.CritRangeBonus > 0 && 20-entry.RoleEffect.CritRangeBonus < critRangeThreshold {
					critRangeThreshold = 20 - entry.RoleEffect.CritRangeBonus
				}
				if entry.RoleEffect.CritDamageBonus != nil {
					critDamageBonus += entry.RoleEffect.CritDamageBonus(ownerProfBonus)
				}
				for skill, ability := range entry.RoleEffect.SkillAbilityOverride {
					skillAbilityOverride[skill] = ability
				}
				if entry.RoleEffect.TextBonus != nil {
					foundationTraits = append(foundationTraits, entry.RoleEffect.TextBonus(ownerProfBonus))
				}
			}
			foundationTraits = append(foundationTraits, entry.ReferenceTraits...)
			if entry.NaturalWeapon != nil {
				foundationTraits = append(foundationTraits, entry.NaturalWeapon.CritText)
			}
			if entry.SageCreatureFramework {
				bestialTraits, err := s.bestialFrameworkReferenceTraits(fp)
				if err != nil {
					return data, err
				}
				foundationTraits = append(foundationTraits, bestialTraits...)
			}
			if entry.SplitBodyFramework {
				isMatryoshkaFramework = true
			}
			catalogLabel, entryName = entry.Catalog, entry.Name
		}
		finalSpeed := total.Speed
		if baseline != nil {
			finalSpeed += baseline.Speed
		}
		if speedSet != 0 {
			finalSpeed = speedSet
		}
		if flySpeedEqualsSpeed && finalSpeed > total.FlySpeed {
			total.FlySpeed = finalSpeed
			total.GrantsFlight = true
		}

		if baseline != nil {
			ac := int64(puppetToolDefaultAC(baseline.ACBase, masterProfBonus) + total.AC)
			hpMax := int64(puppetToolMaxHP(*baseline, level, masterConMod) + total.MaxHP)
			flySpeed := sql.NullInt64{}
			if total.GrantsFlight {
				flySpeed = sql.NullInt64{Int64: int64(total.FlySpeed), Valid: true}
			}
			abilityScore := func(key string, baseValue int) int64 {
				v := baseValue
				if s, ok := abilitySets[key]; ok && s > v {
					v = s
				}
				return int64(v + abilityDeltas[key])
			}
			if err := charstore.SetCompanionStatDefaults(s.charDB, characterID, c.ID,
				ac, hpMax, hpMax, int64(finalSpeed), flySpeed,
				abilityScore("str", baseline.Str), abilityScore("dex", baseline.Dex), abilityScore("con", baseline.Con),
				abilityScore("int", baseline.Int), abilityScore("wis", baseline.Wis), abilityScore("cha", baseline.Cha),
				size,
			); err != nil {
				return data, err
			}
			// Re-read rather than hand-assemble the updated fields locally —
			// keeps this one call the single source of truth for what a
			// backfilled companion looks like, matching the row LoadCompanions
			// would return on the very next request.
			c, err = charstore.GetCompanion(s.charDB, characterID, c.ID)
			if err != nil {
				return data, err
			}
		}

		view := puppetCompanionView{Companion: c}
		var expectedAbilityScores map[string]int
		view.UpgradeBonuses = notes
		view.FoundationCatalogLabel = catalogLabel
		view.FoundationEntryName = entryName
		view.FoundationTraits = foundationTraits
		view.IsMatryoshkaFramework = isMatryoshkaFramework
		if len(elevatedDesignBonus) > 0 {
			view.ElevatedDesignBonus = elevatedDesignBonus
		}
		if len(puppetToolASIBonus) > 0 {
			view.PuppetToolASIBonus = puppetToolASIBonus
		}
		view.SymphonyEnhancement = symphonyEnhancementViews[c.ID]
		if baseline != nil {
			view.ExpectedMaxHP = puppetToolMaxHP(*baseline, level, masterConMod) + total.MaxHP
			view.ExpectedSpeed = finalSpeed
			expected := func(key string, baseValue int) int {
				v := baseValue
				if s, ok := abilitySets[key]; ok && s > v {
					v = s
				}
				return v + abilityDeltas[key]
			}
			view.ExpectedStr = expected("str", baseline.Str)
			view.ExpectedDex = expected("dex", baseline.Dex)
			view.ExpectedCon = expected("con", baseline.Con)
			view.ExpectedInt = expected("int", baseline.Int)
			view.ExpectedWis = expected("wis", baseline.Wis)
			view.ExpectedCha = expected("cha", baseline.Cha)
			// Same values the Sync STR/DEX/.../CHA buttons above show — reused
			// below so the natural weapon's own attack/damage modifier always
			// matches what a fully-synced companion's stored score would give,
			// whether or not the player has actually clicked Sync yet.
			expectedAbilityScores = map[string]int{
				"str": view.ExpectedStr, "dex": view.ExpectedDex, "con": view.ExpectedCon,
				"int": view.ExpectedInt, "wis": view.ExpectedWis, "cha": view.ExpectedCha,
			}
		}
		view.ExpectedSize = size
		view.ExpectedFlySpeed = total.FlySpeed
		// A worn chassis's own formula supersedes the natural-armor baseline
		// (see puppetArmorChassisAC's doc) — falls back to the baseline
		// whenever ArmorChassis is blank or doesn't match a known chassis
		// name (free text predating this feature, or the player's own
		// note). Enhanced Durability's own +1 AC (Purple Technique) applies
		// on top of either formula — its own text reads as a flat bonus to
		// "its AC calculation", not specific to which one is in use.
		if ch, ok := chassisByName[c.ArmorChassis]; ok {
			view.ExpectedAC = puppetArmorChassisAC(ch, int(c.Dex.Int64)) + total.AC
			view.ChassisProperties = adjustChassisReinforced(ch.PropertyTooltips,
				puppetupgrades.ChassisReinforcedBonus(subclassColor, picksByCompanion[c.ID][puppetupgrades.EnhancedDurabilityEntrySlug] > 0))
		} else if baseline != nil {
			view.ExpectedAC = puppetToolDefaultAC(baseline.ACBase, masterProfBonus) + total.AC
		}
		tiers, err := s.loadPuppetUpgradeTiers(characterID, c.ID, level, subclassSlug, c.ArmorChassis)
		if err != nil {
			return data, err
		}
		view.UpgradeLists = groupPuppetUpgradeTiers(tiers)

		attacks, err := charstore.ListCompanionAttacks(s.charDB, characterID, c.ID)
		if err != nil {
			return data, err
		}
		view.Attacks = composeCompanionAttacks(attacks, c, ownerProfBonus)

		upgradeSlugs := make([]string, len(upgradePicks))
		for i, u := range upgradePicks {
			upgradeSlugs[i] = u.UpgradeEntrySlug
		}
		// A Foundation pick's own fixed free-Upgrade grant (Shade
		// Framework's free Ghillie Coating, e.g.) isn't a real stored pick
		// (see puppetFoundationFixedGrants), so it's added here purely so
		// resolvePuppetSkills' own puppetFlatSkillGrants lookup sees it —
		// same treatment picksByCompanion already gives it for
		// puppetupgrades.StatBonuses above.
		for _, fp := range foundationPicks {
			if grantedSlug, ok := puppetFoundationFixedGrants[fp.Entry.EntrySlug]; ok {
				upgradeSlugs = append(upgradeSlugs, grantedSlug)
			}
		}
		view.PuppetSkills = resolvePuppetSkills(sheet, c, generalizedPicks, upgradeSlugs, skillAbilityOverride)

		view.Attacks = append(view.Attacks, puppetUpgradeGrantedAttacks(sheet, upgradePicks, upgradeChoices)...)
		for _, fp := range foundationPicks {
			if row := puppetFoundationWeaponAttack(expectedAbilityScores, ownerProfBonus, fp); row != nil {
				view.Attacks = append(view.Attacks, *row)
			}
		}
		// Striker's/Sentinel's own flat/Proficiency-scaled attack-roll bonus
		// and Lurker's own widened crit range both apply to every attack
		// this companion has — the structured Attacks list, any granted
		// attack (Integrated Weapon), and its own Foundation natural
		// weapon alike — so this runs once, after every source above has
		// been appended, rather than being threaded through each of those
		// three separate assembly paths individually.
		for i := range view.Attacks {
			view.Attacks[i].AttackTotal += attackRollBonus
			view.Attacks[i].CritRangeThreshold = critRangeThreshold
			view.Attacks[i].CritDamageBonus = critDamageBonus
		}

		// Chakra Enhanced Retrofit (Class: Puppet Master/3rd Level) — not
		// shown at all below that level, same "real DOM removal" gate every
		// other level-locked block on this sheet uses. The book's own "If
		// you do not have a Puppet Tool, select 1 weapon or armor you have
		// to gain the benefits of this feature" clause is deliberately not
		// modeled — every Puppet Master character in this app already has
		// a Puppet Tool (its own kind="puppet" companion is how the class
		// is represented here at all), so that branch never applies.
		if level >= 3 {
			view.SealCap = puppetSealSlotCap(ownerProfBonus)
			seals, err := s.loadCompanionSeals(characterID, c.ID)
			if err != nil {
				return data, err
			}
			view.Seals = seals
			if len(seals) < view.SealCap {
				options, err := s.loadPuppetSealOptions(characterID)
				if err != nil {
					return data, err
				}
				view.SealOptions = options
			}
		}

		data.Companions = append(data.Companions, view)
	}
	return data, nil
}

// formCompanionAttack reads the shared sheet_modifier_fields form fields
// (attack_ability/attack_prof/attack_bonus/damage_ability/damage_bonus)
// plus name/description/damage dice/damage type into a CompanionAttack —
// the companion-scoped equivalent of characters.go's own attack-form
// parsing, reused by both add and (future) edit handlers.
func formCompanionAttack(r *http.Request) charstore.CompanionAttack {
	damageCount, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("damage_count")))
	damageSides, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("damage_sides")))
	attackBonus, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("attack_bonus")))
	damageBonus, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("damage_bonus")))
	return charstore.CompanionAttack{
		Name:          strings.TrimSpace(r.FormValue("name")),
		Description:   strings.TrimSpace(r.FormValue("description")),
		AttackAbility: r.FormValue("attack_ability"),
		AttackProf:    r.FormValue("attack_prof"),
		AttackBonus:   attackBonus,
		DamageCount:   damageCount,
		DamageSides:   damageSides,
		DamageAbility: r.FormValue("damage_ability"),
		DamageBonus:   damageBonus,
		DamageType:    strings.TrimSpace(r.FormValue("damage_type")),
	}
}

// handlePuppetAttackAdd records one structured attack for a companion whose
// kind has reached the structured-attacks presentation (see
// companionSupportsStructuredAttacks) — puppet originally, nin-dog as of
// the "Attacks section should be rollable, not typed" fix, kept under its
// original puppet-era name since the route itself
// (/characters/{id}/companions/{cid}/attacks) was never puppet-specific.
func (s *server) handlePuppetAttackAdd(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for attack add:", err)
		return
	}
	if !companionSupportsStructuredAttacks(companion.Kind) {
		http.Error(w, "this companion kind doesn't support structured attacks", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	attack := formCompanionAttack(r)
	if attack.Name == "" {
		http.Error(w, "attack needs a name", http.StatusBadRequest)
		return
	}
	if _, err := charstore.AddCompanionAttack(s.charDB, id, cid, attack); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add companion attack:", err)
		return
	}
	s.respondSheet(w, r, id, companionAttacksFragment(companion.Kind))
}

// handlePuppetAttackDelete removes one structured attack — see
// handlePuppetAttackAdd's own doc for why this isn't puppet-exclusive
// either. Loads the companion (unlike the puppet-only version this
// replaced, which trusted the URL's own cid without checking kind at all)
// both to reject a kind that never reached structured attacks and to know
// which fragment actually contains the row that just changed.
func (s *server) handlePuppetAttackDelete(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	aid, err := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for attack delete:", err)
		return
	}
	if !companionSupportsStructuredAttacks(companion.Kind) {
		http.Error(w, "this companion kind doesn't support structured attacks", http.StatusBadRequest)
		return
	}
	if err := charstore.DeleteCompanionAttack(s.charDB, id, cid, aid); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete companion attack:", err)
		return
	}
	s.respondSheet(w, r, id, companionAttacksFragment(companion.Kind))
}

// handlePuppetUpgradeAdd records one upgrade pick (form fields
// "upgrade_entry_slug", "at_tier" — the latter only consulted for a
// puppetUpgradeTierEscalation entry, see below), hard-rejecting if the
// tier's slot cap is already used up.
func (s *server) handlePuppetUpgradeAdd(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	companion, err := charstore.GetCompanion(s.charDB, id, cid)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion for upgrade add:", err)
		return
	}
	if companion.Kind != "puppet" {
		http.Error(w, "not a puppet", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	upgradeEntrySlug := strings.TrimSpace(r.FormValue("upgrade_entry_slug"))
	if upgradeEntrySlug == "" {
		http.Error(w, "missing upgrade selection", http.StatusBadRequest)
		return
	}
	atTier := strings.TrimSpace(r.FormValue("at_tier"))

	// Resolved from the entry itself rather than trusted from a form field —
	// the class-wide "Puppet Master Upgrades" list and a subclass's own list
	// are now merged into one combined tier section per tier name (see
	// groupPuppetUpgradeTiers/loadPuppetUpgradeTiers's own doc on "All"-
	// technique upgrades), so a single "+Add" form's options can come from
	// either underlying class_options row — there is no one static slug a
	// hidden field could carry for the whole tier anymore.
	//
	// puppetUpgradeTierEscalation is the one exception: one of those entries
	// can legitimately be added from a tier OTHER than its own home (e.g.
	// Stonecold Stronghold from the Gold Tier section, not just Bronze), so
	// at_tier is trusted there instead — but only after checking it's
	// actually one of that entry's own valid tiers, the same "server re-
	// derives, never trusts the form outright" rule every other gate here
	// follows.
	var classOptionSlug string
	if spec, ok := puppetUpgradeTierEscalation[upgradeEntrySlug]; ok && atTier != "" {
		homeSlug, err := s.puppetUpgradeEntryClassOptionSlug(upgradeEntrySlug)
		if err == sql.ErrNoRows {
			http.Error(w, "unknown upgrade", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("resolve upgrade entry's class option:", err)
			return
		}
		homeTierName, err := s.puppetUpgradeTierName(homeSlug)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("resolve upgrade entry's home tier name:", err)
			return
		}
		if atTier == homeTierName {
			classOptionSlug = homeSlug
		} else if slices.Contains(spec.HigherTiers, atTier) {
			_, subclassSlug, err := s.puppetMasterSubclassSlug(id)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("resolve subclass for tier escalation:", err)
				return
			}
			classOptionSlug, err = s.puppetUpgradeTierClassOptionSlug(atTier, subclassSlug)
			if err != nil {
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("resolve escalation tier's class option:", err)
				return
			}
		} else {
			http.Error(w, "not a valid tier for this upgrade", http.StatusBadRequest)
			return
		}
	} else {
		var err error
		classOptionSlug, err = s.puppetUpgradeEntryClassOptionSlug(upgradeEntrySlug)
		if err == sql.ErrNoRows {
			http.Error(w, "unknown upgrade", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("resolve upgrade entry's class option:", err)
			return
		}
	}

	level, err := s.puppetMasterClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master level for upgrade add:", err)
		return
	}
	if level == 0 {
		http.Error(w, "character has no levels in Puppet Master", http.StatusBadRequest)
		return
	}

	tierName, err := s.puppetUpgradeTierName(classOptionSlug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load upgrade tier name:", err)
		return
	}

	// A material tier (Wood/Bronze/Silver/Gold/Platinum) can borrow spare
	// capacity converted down from any higher tier — see
	// puppetUpgradeEffectiveCap's own doc for the rule this implements —
	// so its real ceiling isn't just its own row in the class table. An
	// untiered one-time pick (rank not found) keeps the old flat cap=1
	// gate, since it isn't part of that ladder at all.
	if rank, ok := puppetUpgradeTierRanks[tierName]; ok {
		caps, err := s.puppetUpgradeMaterialTierCaps(level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load upgrade material tier caps:", err)
			return
		}
		foundationBonusCaps, err := s.puppetFoundationBonusTierCaps(id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load foundation bonus tier caps:", err)
			return
		}
		_, subclassSlug, err := s.puppetMasterSubclassSlug(id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("resolve subclass for upgrade tier caps:", err)
			return
		}
		featureBonusCaps, err := s.puppetUpgradeFeatureBonusTierCapsForCharacter(id, puppetSubclassColorBySlug[subclassSlug], level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load feature bonus tier caps:", err)
			return
		}
		for i := range caps {
			caps[i] += foundationBonusCaps[i] + featureBonusCaps[i]
		}
		used, err := s.puppetUpgradeMaterialTierUsage(id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load upgrade material tier usage:", err)
			return
		}
		if used[rank] >= puppetUpgradeEffectiveCap(rank, caps, used) {
			http.Error(w, "no upgrade slots remaining for this tier", http.StatusBadRequest)
			return
		}
	} else {
		cap, err := s.puppetUpgradeSlotCap(classOptionSlug, tierName, level)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load upgrade slot cap:", err)
			return
		}
		used, err := s.puppetUpgradeNonMaterialTierUsage(id, cid, classOptionSlug, tierName)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load upgrade tier usage:", err)
			return
		}
		if used >= cap {
			http.Error(w, "no upgrade slots remaining for this tier", http.StatusBadRequest)
			return
		}
	}

	met, err := s.puppetUpgradeEntryPrereqMet(id, cid, upgradeEntrySlug, companion.ArmorChassis)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("check upgrade prerequisite:", err)
		return
	}
	if !met {
		http.Error(w, "prerequisite not met for this upgrade", http.StatusBadRequest)
		return
	}

	var entryDescription string
	if err := s.rulesDB.QueryRow(`SELECT description FROM class_option_entries WHERE slug = ?`, upgradeEntrySlug).Scan(&entryDescription); err != nil && err != sql.ErrNoRows {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load upgrade entry description for technique check:", err)
		return
	}
	subclassSlug, _, err := s.puppetMasterSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load subclass for technique check:", err)
		return
	}
	if !puppetUpgradeAvailableToColor(entryDescription, puppetSubclassColorBySlug[subclassSlug]) {
		http.Error(w, "this upgrade isn't available to your subclass", http.StatusBadRequest)
		return
	}

	existingPicks, err := charstore.ListCompanionUpgrades(s.charDB, id, cid)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load existing upgrade picks:", err)
		return
	}
	takenCount := 0
	for _, p := range existingPicks {
		if p.UpgradeEntrySlug == upgradeEntrySlug {
			takenCount++
		}
	}
	maxTakes := puppetUpgradeMaxTakes(upgradeEntrySlug)
	// Warfare Augmentation, Red Technique only: pools its own cap and
	// taken-count across the character's WHOLE roster instead of staying
	// scoped to this one companion — see puppetWarfareAugmentationMaxTakes's
	// own doc for why every other technique needs no override here.
	if upgradeEntrySlug == warfareAugmentationEntrySlug && puppetSubclassColorBySlug[subclassSlug] == "Red" {
		companionCount, err := s.puppetCompanionCount(id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("count puppet companions for warfare augmentation cap:", err)
			return
		}
		maxTakes = puppetWarfareAugmentationMaxTakes(companionCount)
		takenCount, err = s.puppetEntryTotalPicksAcrossCompanions(id, warfareAugmentationEntrySlug)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("count warfare augmentation picks across companions:", err)
			return
		}
	}
	if takenCount >= maxTakes {
		http.Error(w, "this upgrade has already been taken", http.StatusBadRequest)
		return
	}
	// See loadPuppetUpgradeTiers' incompleteUniqueSubChoicePick doc: a
	// repeatable entry with its own unique-per-pick sub-choice (Elemental
	// Reactor/Integrated Weapon/Mastered Armament) can't be taken again
	// until every existing pick of it already has that sub-choice filled
	// in, or a second take could sit permanently stuck with nothing
	// assigned and no way to delete it.
	if puppetUpgradeUniqueSubChoiceEntries[upgradeEntrySlug] {
		existingChoices, err := charstore.ListCompanionUpgradeChoices(s.charDB, id, cid)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load existing upgrade choices for uniqueness gate:", err)
			return
		}
		choiceCountByUpgradeID := map[int64]int{}
		for _, ch := range existingChoices {
			choiceCountByUpgradeID[ch.CompanionUpgradeID]++
		}
		for _, p := range existingPicks {
			if p.UpgradeEntrySlug == upgradeEntrySlug && choiceCountByUpgradeID[p.ID] == 0 {
				http.Error(w, "finish choosing this upgrade's own option before taking it again", http.StatusBadRequest)
				return
			}
		}
	}

	if _, err := charstore.AddCompanionUpgrade(s.charDB, id, cid, classOptionSlug, upgradeEntrySlug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("add companion upgrade:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// puppetUpgradeEntryPrereqMet is handlePuppetUpgradeAdd's own server-side
// gate — the tile's disabled state (loadPuppetUpgradeTiers) is what a
// player actually sees, but a POST reaching this handler is trusted no
// further than any other form submission on this app, so the exact same
// check runs again here against the database's own current state.
func (s *server) puppetUpgradeEntryPrereqMet(characterID, companionID int64, upgradeEntrySlug, armorChassis string) (bool, error) {
	var description string
	err := s.rulesDB.QueryRow(`SELECT description FROM class_option_entries WHERE slug = ?`, upgradeEntrySlug).Scan(&description)
	if err == sql.ErrNoRows {
		return true, nil // an untiered one-time pick (e.g. Weapon Type) — nothing to gate
	}
	if err != nil {
		return false, err
	}

	nameRows, err := s.rulesDB.Query(`SELECT DISTINCT name FROM class_option_entries WHERE class_option_slug LIKE 'class/puppet-master/%'`)
	if err != nil {
		return false, err
	}
	var allNames []string
	for nameRows.Next() {
		var n string
		if err := nameRows.Scan(&n); err != nil {
			nameRows.Close()
			return false, err
		}
		allNames = append(allNames, n)
	}
	nameRows.Close()
	if err := nameRows.Err(); err != nil {
		return false, err
	}

	chassisOptions, err := s.loadPuppetArmorChassisOptions()
	if err != nil {
		return false, err
	}
	chassisNames := make([]string, len(chassisOptions))
	for i, c := range chassisOptions {
		chassisNames[i] = c.Name
	}

	picks, err := charstore.ListCompanionUpgrades(s.charDB, characterID, companionID)
	if err != nil {
		return false, err
	}
	haveUpgradeName := map[string]bool{}
	for _, p := range picks {
		var name string
		if err := s.rulesDB.QueryRow(`SELECT name FROM class_option_entries WHERE slug = ?`, p.UpgradeEntrySlug).Scan(&name); err == nil {
			haveUpgradeName[strings.ToLower(name)] = true
		} else if err != sql.ErrNoRows {
			return false, err
		}
	}

	groups := parsePuppetUpgradePrereq(description, puppetUpgradeSortedNames(allNames), puppetUpgradeSortedNames(chassisNames))
	return puppetUpgradePrereqMet(groups, haveUpgradeName, armorChassis), nil
}

// handlePuppetUpgradeDelete removes one upgrade pick — rejected server-side
// for a permanent entry (see puppetUpgradePickIsPermanent) the same way
// every other gate on this sheet re-checks what the button's hidden/shown
// state only suggests to the player.
func (s *server) handlePuppetUpgradeDelete(w http.ResponseWriter, r *http.Request) {
	id, cid, ok := parseCharacterAndCompanionID(w, r)
	if !ok {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	picks, err := charstore.ListCompanionUpgrades(s.charDB, id, cid)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load companion upgrades for delete:", err)
		return
	}
	for _, p := range picks {
		if p.ID == uid && puppetUpgradePickIsPermanent(p.UpgradeEntrySlug) {
			http.Error(w, "this upgrade is a permanent choice and cannot be removed", http.StatusBadRequest)
			return
		}
	}
	if err := charstore.DeleteCompanionUpgrade(s.charDB, id, cid, uid); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("delete companion upgrade:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handlePuppetFightingStance records Blue Technique Warmaster's own Fighting
// Stance pick — see blueTechniqueProficiencyFeatureSlug's doc for why this
// is a separate handler/catalog from taijutsu.go's handleFightingStance
// rather than a shared one: different feature slug, different (unfiltered)
// stance catalog, different response fragment. Freely re-editable, same
// boundary as every other subclass-choice pick on this sheet.
func (s *server) handlePuppetFightingStance(w http.ResponseWriter, r *http.Request) {
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
	subclassSlug, _, err := s.puppetMasterSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master subclass for fighting stance:", err)
		return
	}
	if puppetSubclassColorBySlug[subclassSlug] != "Blue" {
		http.Error(w, "character is not a Blue Technique Warmaster", http.StatusBadRequest)
		return
	}
	options, err := s.loadFightingStanceOptions()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load stance options for puppet fighting stance:", err)
		return
	}
	valid := false
	for _, o := range options {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid stance", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, blueTechniqueProficiencyFeatureSlug, 0, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set puppet fighting stance:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handlePuppetTransformerWeaponType records Blue Technique Warmaster's own
// L14 Transformer feature — "on a long rest select a second Puppet Weapon
// Type for your Puppet Tool." See transformerWeaponTypeFeatureSlug's own
// doc for why this is a display-only reference pick, never routed through
// the Foundation catalog's auto-builder. Freely re-editable, same boundary
// as handlePuppetFightingStance above.
func (s *server) handlePuppetTransformerWeaponType(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("weapon_type_slug"))
	subclassSlug, _, err := s.puppetMasterSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master subclass for transformer weapon type:", err)
		return
	}
	if puppetSubclassColorBySlug[subclassSlug] != "Blue" {
		http.Error(w, "character is not a Blue Technique Warmaster", http.StatusBadRequest)
		return
	}
	level, err := s.puppetMasterClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master level for transformer weapon type:", err)
		return
	}
	if level < 14 {
		http.Error(w, "character has not reached Transformer (14th level) yet", http.StatusBadRequest)
		return
	}
	options, err := s.loadPuppetWeaponTypeOptions()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load weapon type options for transformer:", err)
		return
	}
	valid := false
	for _, o := range options {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid Puppet Weapon Type", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, transformerWeaponTypeFeatureSlug, 0, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set transformer weapon type:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handlePuppetGreenTechniqueJutsu records Green Technique Marionettist's own
// L20 Master of the Green Technique feature — "Each long rest, select one
// Ninjutsu or Genjutsu of B-Rank or lower that you could learn…" Freely
// re-editable, sourced from loadGreenTechniqueJutsuOptions (the full
// rank/eligibility-filtered catalog, not just known jutsu).
func (s *server) handlePuppetGreenTechniqueJutsu(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("jutsu_slug"))
	subclassSlug, _, err := s.puppetMasterSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master subclass for green technique jutsu:", err)
		return
	}
	if puppetSubclassColorBySlug[subclassSlug] != "Green" {
		http.Error(w, "character is not a Green Technique Marionettist", http.StatusBadRequest)
		return
	}
	level, err := s.puppetMasterClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master level for green technique jutsu:", err)
		return
	}
	if level < 20 {
		http.Error(w, "character has not reached Master of the Green Technique (20th level) yet", http.StatusBadRequest)
		return
	}
	options, err := s.loadGreenTechniqueJutsuOptions(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load jutsu options for green technique:", err)
		return
	}
	valid := false
	for _, o := range options {
		if o.Value == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid jutsu choice", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, masterOfGreenTechniqueFeatureSlug, 0, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set green technique jutsu:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handlePuppetThreadSavantJutsu records White Technique Weaver's own L14
// Thread Savant feature — "Select one jutsu that you know… you may switch
// your chosen jutsu at the conclusion of a full rest." Freely re-editable,
// sourced from loadThreadSavantJutsuOptions (the character's own known
// jutsu only) — the stored value is a character_jutsu row id (decimal
// text), not a rules-db slug, so a player-created custom jutsu can be
// picked too.
func (s *server) handlePuppetThreadSavantJutsu(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("jutsu_id"))
	subclassSlug, _, err := s.puppetMasterSubclassSlug(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master subclass for thread savant:", err)
		return
	}
	if puppetSubclassColorBySlug[subclassSlug] != "White" {
		http.Error(w, "character is not a White Technique Weaver", http.StatusBadRequest)
		return
	}
	level, err := s.puppetMasterClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master level for thread savant:", err)
		return
	}
	if level < 14 {
		http.Error(w, "character has not reached Thread Savant (14th level) yet", http.StatusBadRequest)
		return
	}
	options, err := s.loadThreadSavantJutsuOptions(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load jutsu options for thread savant:", err)
		return
	}
	valid := false
	for _, o := range options {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid jutsu choice", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, threadSavantFeatureSlug, 0, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set thread savant jutsu:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}

// handlePuppetAlwaysPreparedUpgrade records Always Prepared's own 18th-level
// temporary-Upgrade pick — "after a rest of any type, select and create a
// temporary version of an Upgrade... of Silver tier or lower... until you
// complete a rest." A base-class feature available to every Puppet
// Technique, unlike handlePuppetTransformerWeaponType/
// handlePuppetGreenTechniqueJutsu/handlePuppetThreadSavantJutsu above, which
// each also check the character's subclass.
func (s *server) handlePuppetAlwaysPreparedUpgrade(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("upgrade_entry_slug"))
	level, err := s.puppetMasterClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load puppet master level for always prepared upgrade:", err)
		return
	}
	if level < 18 {
		http.Error(w, "character has not reached Always Prepared's temporary Upgrade clause (18th level) yet", http.StatusBadRequest)
		return
	}
	options, err := s.loadAlwaysPreparedUpgradeOptions(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load always prepared upgrade options:", err)
		return
	}
	valid := false
	for _, o := range options {
		if o.Slug == slug {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid upgrade choice", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, alwaysPreparedTemporaryUpgradeFeatureSlug, 0, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set always prepared temporary upgrade:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_puppet_tab")
}
