package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/features"
)

// Superiority Dice is base-class-wide (all 9 subclasses grant their own
// copy, each keyed to their own 3rd-level introducing feature — see
// customResourceGrants in custom_resources.go for the pool itself and the
// Cloning Scout granting-slug exception). This file hand-transcribes the
// count and die-size progressions each subclass's own "Superior <Name>
// Table" states, since class_level_resources has zero rows for
// class/scout-nin — every table was independently re-read from the real
// book text (five of the nine have their own numeric table glued onto a
// later, unrelated feature by the same PDF-extraction column-wrap quirk
// documented elsewhere in this package: Assault's onto Relentless Assault
// [20th], Barrier's onto Rallying Barrier [9th], Cloning's onto Superior
// Cloning [glued cleanly onto Cloning Tactics itself], Tactical's onto
// Unmatched Tactics [20th], Trickster's onto Void Soul Awakening [3rd] —
// none of that affects this hand-transcription, since it reads the
// resolved table values directly rather than relying on which feature row
// the table text happens to sit under).
//
// Every subclass's own count table reduces to one of four shapes once the
// per-level values are laid out side by side:
//   - "a" (Arbiter, Cloning, Trickster — base d8/d8/d6): 3 dice at 3rd,
//     +1 every 3 levels, capping at 8 by 18th.
//   - "b" (Barrier, Elemental, Phantom — base d8/d6/d6): 3 dice at 3rd,
//     +1 every 5 levels, capping at 6 by 18th.
//   - "c" (Assault, Pathfinder — base d4/d6): 3 dice at 3rd, +1 every 4
//     levels, capping at 7 by 19th.
//   - "tactical" (base d10, its own unique table): 3 dice at 3rd, +1 every
//     3 levels through 12th, then +1 every 2 levels, capping at 10 by 20th.
func scoutNinSuperiorityDice(shape string, level int) int {
	switch shape {
	case "a":
		switch {
		case level >= 18:
			return 8
		case level >= 15:
			return 7
		case level >= 12:
			return 6
		case level >= 9:
			return 5
		case level >= 6:
			return 4
		case level >= 3:
			return 3
		}
	case "b":
		switch {
		case level >= 18:
			return 6
		case level >= 13:
			return 5
		case level >= 8:
			return 4
		case level >= 3:
			return 3
		}
	case "c":
		switch {
		case level >= 19:
			return 7
		case level >= 15:
			return 6
		case level >= 11:
			return 5
		case level >= 7:
			return 4
		case level >= 3:
			return 3
		}
	case "tactical":
		switch {
		case level >= 20:
			return 10
		case level >= 18:
			return 9
		case level >= 16:
			return 8
		case level >= 14:
			return 7
		case level >= 12:
			return 6
		case level >= 9:
			return 5
		case level >= 6:
			return 4
		case level >= 3:
			return 3
		}
	}
	return 0
}

// scoutNinSuperiorityDieSize is each subclass's own base Superiority Die
// size (Assault Scout's own "Superior Assault" text: d4; Elemental/
// Pathfinder/Phantom/Trickster: d6; Arbiter/Barrier/Cloning: d8; Tactical:
// d10 — every value independently re-read against the book, not inferred).
// Assault Scout's own Untapped Potential (6th level) grows this die as the
// character levels: "Your Superiority Die grows 1 step, into a D6. It
// repeats this process at 9th (d8), 14th (d10) and 17th levels (d12)." No
// other subclass's die size changes with level. Untapped Potential's own
// second clause (regaining 1 Superiority Die on a critical hit or a
// natural 20 saving throw) is a combat-trigger effect this app has no
// mechanism to automate and is not modeled here — only the die-size
// readout is.
func scoutNinSuperiorityDieSize(subclassSlug string, level int) string {
	switch subclassSlug {
	case "assault-scout":
		switch {
		case level >= 17:
			return "d12"
		case level >= 14:
			return "d10"
		case level >= 9:
			return "d8"
		case level >= 6:
			return "d6"
		default:
			return "d4"
		}
	case "arbiter-scout", "barrier-scout", "cloning-scout":
		return "d8"
	case "tactical-scout":
		return "d10"
	default: // elemental-scout, pathfinder-scout, phantom-scout, trickster-scout
		return "d6"
	}
}

// scoutNinPhantasmicPowerDieSize is Phantom Scout's own Phantasmic Power
// die-size readout — the base die nearly every Phantom Scout feature rolls
// (Ghastly Leech's own uses-count pool converts one of these rolls into HP
// or Chakra, see custom_resources.go). Book text: "represented as 1d6...
// becoming 2d6 at 6th, 3d6 at 9th, 4d6 at 14th, 5d6 at 17th and 6d6 at 20th
// Level." Scales off the character's own Scout-Nin CLASS level (not
// Phantom Scout subclass level — this game has no separate subclass
// leveling, and the feature's own text states plain "Level"), mirroring
// scoutNinSuperiorityDieSize's own shape. Rolling Phantasmic Power itself
// and every downstream effect that consumes it (Phantom's Power's own
// advantage-attack bonus, Phantasm's Grip, Ethereal Snap) stay fully
// manual/narrated — only this readout of what the pool converts is
// automated.
func scoutNinPhantasmicPowerDieSize(level int) string {
	switch {
	case level >= 20:
		return "6d6"
	case level >= 17:
		return "5d6"
	case level >= 14:
		return "4d6"
	case level >= 9:
		return "3d6"
	case level >= 6:
		return "2d6"
	default:
		return "1d6"
	}
}

// Group 1 base-class picks closing further Scout-Nin gaps beyond
// Superiority Dice itself: Shinobi Adept and Jack of All, Master of None
// (both cap+catalog picks over a hand-curated, live-read class_features
// list — neither has a class_options catalog of its own, same "book states
// the options directly as their own feature rows" shape
// medical_nin.go's medicalDoctrineSlugs already established), Fighting
// Stance (base class, level 1 — generalizes taijutsu.go's
// buildFightingStanceView/storeFightingStanceChoice to a Taijutsu-OR-Weapon
// stance choice instead of Taijutsu-only), and Absolute Authority (Arbiter
// Scout's flat AC bonus — see internal/features/grants.go's
// flatACBonusGrants).
//
// Deft Explorer's own Mobile sub-tier (6th level, automatically granted —
// not a player pick) is rendered as a computed informational readout
// (deftExplorerMobileFeatureSlug below) rather than new persistent
// swim/climb/wall/water-walk speed fields: every one of those speeds is
// simply set equal to the character's own Speed, so a stored field would
// only ever duplicate a value already computed elsewhere on the sheet, the
// same "no bonus-composition mechanism to wire a new value into" boundary
// unarmedDamageDieSize (taijutsu.go) already draws for a similarly
// display-only value. Tireless (11th level, spend a full turn Action once
// per long rest to recover all spent Superiority Dice) now has its
// once-per-long-rest use-count tracked as a customResourceGrants entry
// (custom_resources.go, key "class/scout-nin/feature/tireless") — actually
// spending the Action and restoring the Superiority Dice pool by that
// amount both stay a manual player step, only the availability gate is
// automated.
//
// Combat/Control/Mobility/Skill/Support (Jack of All's 5 Generalizations)
// are, like Medical Doctrine's 4 options, blanket-granted class_features
// rows once the character reaches 5th level — this picker tracks which
// ones the player has actually chosen (for display, and because the cap
// itself is meaningful), but does not suppress the other, unpicked
// options' own text from Features & Traits, matching the precedent
// medicalDoctrineSlugs/loadMedicalDoctrineCatalog already established
// rather than inventing a new "hide the unpicked options" mechanism here.
// Combat's attack/damage bonus, Control's save-DC bonus, and Skill's flat
// check bonus are correspondingly NOT wired into the attack table/save-DC
// computation — same informational-readout-only boundary as Mobile above;
// each Generalization's own current-level bonus is already legible in its
// class_features description, shown via this picker's own tooltip. Mobility
// is the one exception: a real speedGrants entry already existed pre-dating
// this stage (internal/features/grants.go). Its level tiers were corrected
// to 5/11 to match the class_features level-tagging fix here, but the bonus
// itself stayed blanket-applied regardless of this picker's own pick until
// a later fix (internal/charsheet/charsheet.go's own ResolveSpeedBonus call
// site, gated on charstore.ScoutNinPickJackOfAll) — the blanket grant was
// fine for the other 4 Generalizations precisely because none of them feed
// a real computed field the way Mobility feeds Speed.
//
// Stage 6 closes 3 further gaps in the same "the pick/use-count is
// trackable, the triggered effect stays manual" shape: Paragon's Presence
// (Arbiter Scout, 14th — a freely re-editable 3-option condition-immunity
// pick, same shape as Change of Heart), Supreme Clones (Cloning Scout,
// 20th — a flat 1-slot pick of an already-known Maneuver, drawn from the
// same Maneuvers Known list Signature Maneuver already reads), and
// Signature Jutsu (base class, 7th/15th, a 3rd slot on Tactical Scout only
// — a genuinely compound jutsu+effect pick per slot, built on
// puppets.go's Thread Savant "known jutsu" catalog pattern for the jutsu
// half and this file's own loadScoutNinFeatureCatalog for the effect
// half). None of the 3 needed a new storage table — Paragon's
// Presence/Supreme Clones reuse existing infra (charstore.SetFeatureChoice/
// character_scout_nin_picks) as-is, and Signature Jutsu's 2 halves per
// slot are packed into signatureJutsuFeatureSlug's own ChoiceIndex range
// (signatureJutsuChoiceIndex) rather than adding a second table or
// category.
const (
	scoutNinSlug                      = "class/scout-nin"
	scoutNinFightingStanceFeatureSlug = "class/scout-nin/feature/fighting-stance"
	jackOfAllFeatureSlug              = "class/scout-nin/feature/jack-of-all-master-of-none"
	deftExplorerMobileFeatureSlug     = "class/scout-nin/feature/mobile"

	// expertOfAllFeatureSlug/unmatchedTacticsFeatureSlug: Tactical Scout's
	// own two Jack of All extensions — "Expert of All, Jack of None" (9th
	// level) lets the character gain the benefit of 2 Generalizations at
	// once; "Unmatched Tactics" (20th level) adds one more on top of that
	// (its own text: "you may gain the benefit of an additional
	// Generalization" — a +1, not the unlimited count its OWN separate
	// "any number of maneuvers... per attack" clause grants for Maneuvers,
	// a different mechanic entirely). Read directly off the character's
	// merged granted features (hasGrantedFeature), the same reliable
	// per-subclass signal ironcladFeaturePrefix/unnaturalTalentFeatureSlug
	// already use elsewhere, rather than a separate subclass-slug check.
	expertOfAllFeatureSlug      = "class/scout-nin/group/scouting-technique/tactical-scout/feature/expert-of-all-jack-of-none"
	unmatchedTacticsFeatureSlug = "class/scout-nin/group/scouting-technique/tactical-scout/feature/unmatched-tactics"

	// scoutNinElementalKnowledgeFeatureSlug/scoutNinElementalResistanceFeatureSlug:
	// Elemental Scout's own 3rd-level Nature Release pick and the 6th-level
	// resistance it feeds — see elemental_affinity.go's
	// elementalScoutAffinitySlots for the pick itself and
	// scoutNinElementalResistanceEntry below for the resistance grant.
	scoutNinElementalKnowledgeFeatureSlug  = "class/scout-nin/group/scouting-technique/elemental-scout/feature/elemental-knowledge"
	scoutNinElementalResistanceFeatureSlug = "class/scout-nin/group/scouting-technique/elemental-scout/feature/elemental-resistance"

	// masteredSignatureTechniqueFeatureSlug: Mastered Signature Technique
	// (Tactical Scout, 14th level: "you may gain the benefit of an
	// additional feature, granted by the Signature Technique class
	// feature") — the +1 cap bonus signatureTechniqueCap applies on top of
	// Signature Technique's own base-class 10th/18th level progression (see
	// signatureTechniqueSlugs above for that feature's 3 options).
	masteredSignatureTechniqueFeatureSlug = "class/scout-nin/group/scouting-technique/tactical-scout/feature/mastered-signature-technique"

	// tacticalSuperiorityFeatureSlug: Tactical Scout's own 6th/14th/20th
	// level cross-subclass Maneuver pick — "Select one Maneuver from one of
	// the Following Scout-Nin Subclasses... If the Maneuver has the
	// Specialized keyword, you cannot take it." See
	// tacticalSuperiorityCatalogSubclassSlugs below for exactly which
	// subclasses the book's own bullet list names.
	tacticalSuperiorityFeatureSlug = "class/scout-nin/group/scouting-technique/tactical-scout/feature/tactical-superiority"

	// signatureManeuverFeatureSlug: Tactical Scout's own 3rd/9th level pick
	// of a known Maneuver with the Tactical keyword to swap its Superiority
	// Die cost for a flat d4.
	signatureManeuverFeatureSlug = "class/scout-nin/group/scouting-technique/tactical-scout/feature/signature-maneuver"

	// changeOfHeartFeatureSlug: Trickster Scout's own 17th-level 3-option
	// benefit pick (Messiah/Izanagi/Satanael), gained while Tricksters Soul
	// Binding is active.
	changeOfHeartFeatureSlug = "class/scout-nin/group/scouting-technique/trickster-scout/feature/change-of-heart"

	// paragonsPresenceFeatureSlug: Arbiter Scout's own 14th-level 3-option
	// condition-immunity pick — "all allied creatures within 30 feet of you
	// are immune to the Berserk, Charmed or Frightened Conditions. You
	// decide which condition at the end of a Short rest." Same freely
	// re-editable single-slot shape as Change of Heart.
	paragonsPresenceFeatureSlug = "class/scout-nin/group/scouting-technique/arbiter-scout/feature/paragons-presence"

	// supremeClonesFeatureSlug: Cloning Scout's own 20th-level capstone —
	// "select one clone maneuver. Your clones can perform this maneuver
	// without expending a Superiority Die once per turn." A flat 1-slot
	// pick drawn from the character's own already-known Maneuvers
	// (ScoutNinPickManeuvers), not a separate catalog of its own.
	supremeClonesFeatureSlug = "class/scout-nin/group/scouting-technique/cloning-scout/feature/supreme-clones"

	// signatureJutsuFeatureSlug: Signature Jutsu (base class, 7th/15th
	// level — a 3rd slot on Tactical Scout only, see
	// masteredSignatureJutsuFeatureSlug) — "Select one Jutsu you know of
	// B-Rank or Lower. You gain one of the following benefits of your
	// choice when using the chosen Jutsu." Each slot is a COMPOUND pick (a
	// jutsu + one of 3 named effects), stored as 2 charstore.SetFeatureChoice
	// ChoiceIndex values per slot under this one real granting feature slug
	// (see signatureJutsuChoiceIndex) — the same "several independent picks
	// share one featureSlug, distinguished only by ChoiceIndex" shape
	// taijutsu.go's own Anti-Chakra Wavelength Keyword1/Keyword2 pair
	// already establishes, generalized from 2 same-typed slots to 2
	// differently-typed halves (a jutsu pick, an effect pick) per slot.
	signatureJutsuFeatureSlug = "class/scout-nin/feature/signature-jutsu"

	// masteredSignatureJutsuFeatureSlug: Tactical Scout's own 17th-level
	// extension — "you can choose the final benefit granted by the
	// Signature Jutsu class feature" — unlocks Signature Jutsu's 3rd slot.
	masteredSignatureJutsuFeatureSlug = "class/scout-nin/group/scouting-technique/tactical-scout/feature/mastered-signature-jutsu"
)

// signatureJutsuEffectSlugs: Signature Jutsu's own 3 named effects —
// Signature Power/Ramping/Control — real class_features rows (NULL level,
// sort_order 24-26) rather than a class_options catalog, same "book states
// the options directly as their own feature rows" shape shinobiAdeptSlugs/
// jackOfAllSlugs already established.
var signatureJutsuEffectSlugs = []string{
	"class/scout-nin/feature/signature-power",
	"class/scout-nin/feature/signature-ramping",
	"class/scout-nin/feature/signature-control",
}

// signatureJutsuChoiceIndex packs Signature Jutsu's own 2-dimensional
// compound pick (a jutsu + an effect, for each of up to 3 slots) into
// signatureJutsuFeatureSlug's own ChoiceIndex range: slot*2 holds the jutsu
// half, slot*2+1 holds the paired effect half.
func signatureJutsuChoiceIndex(slot int, effect bool) int {
	idx := slot * 2
	if effect {
		idx++
	}
	return idx
}

// signatureJutsuCap: 0 below 7th level, 1 at 7th, 2 at 15th ("You can
// select a second Signature effect and Jutsu... beginning at 15th level"),
// plus a 3rd slot once Mastered Signature Jutsu (Tactical Scout, 17th
// level) is granted — mirrors jackOfAllCap/signatureTechniqueCap's own
// "granted-feature-gated extra slot" shape.
func signatureJutsuCap(level int, grantedFeatures []grantedFeatureRow) int {
	if level < 7 {
		return 0
	}
	cap := 1
	if level >= 15 {
		cap = 2
	}
	if hasGrantedFeature(grantedFeatures, masteredSignatureJutsuFeatureSlug) {
		cap = 3
	}
	return cap
}

// The 9 Scouting Technique subclass slugs, named once here since Maneuvers
// Known's cap (below) needs to switch on them and no such constant list
// existed anywhere in this file yet — every other per-subclass lookup in
// this package so far only needed a granting FEATURE slug, not the bare
// subclass slug itself.
const (
	arbiterScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/arbiter-scout"
	assaultScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/assault-scout"
	barrierScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/barrier-scout"
	cloningScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/cloning-scout"
	elementalScoutSubclassSlug  = "class/scout-nin/group/scouting-technique/elemental-scout"
	pathfinderScoutSubclassSlug = "class/scout-nin/group/scouting-technique/pathfinder-scout"
	phantomScoutSubclassSlug    = "class/scout-nin/group/scouting-technique/phantom-scout"
	tacticalScoutSubclassSlug   = "class/scout-nin/group/scouting-technique/tactical-scout"
	tricksterScoutSubclassSlug  = "class/scout-nin/group/scouting-technique/trickster-scout"
)

// scoutNinManeuversKnownCap: each subclass's own "Maneuvers Known" column,
// read directly off the same 9 "Superior <Name> Table"s
// scoutNinSuperiorityDice already hand-transcribes the "Superiority Dice"
// column from (5 of the 9 tables were found glued onto a later, unrelated
// feature by a PDF-extraction column-wrap quirk — see scoutNinSuperiorityDice's
// own doc comment for exactly which; every one of the 9 was independently
// re-read here too, not inferred from the Dice column's own shape).
//
// A pattern falls out once all 9 tables are laid side by side: for every
// subclass whose Dice column follows shape "a" (+1 every 3 levels, cap 8 by
// 18th) or "b" (+1 every 5 levels, cap 6 by 18th), the Maneuvers Known
// column follows the OTHER of that pair — Arbiter/Cloning/Trickster's own
// Dice use shape "a" but their Maneuvers Known column is shape "b"
// (verified against Arbiter's, Cloning's, and Trickster's own printed
// tables); Barrier/Elemental/Phantom's Dice use shape "b" but their
// Maneuvers Known column is shape "a" (verified against all 3). Assault and
// Pathfinder's Dice already use shape "c", and their own Maneuvers Known
// columns are IDENTICAL to their Dice columns, also shape "c" (verified
// against both). Tactical's Maneuvers Known column is likewise identical to
// its own unique "tactical" Dice column (verified against its own table).
// This isn't assumed from the pattern — every one of the 9 values above was
// independently read off its own subclass's printed table text before this
// function was written, and the pattern is simply what the values happen to
// reduce to.
func scoutNinManeuversKnownCap(subclassSlug string, level int) int {
	switch subclassSlug {
	case arbiterScoutSubclassSlug, cloningScoutSubclassSlug, tricksterScoutSubclassSlug:
		return scoutNinSuperiorityDice("b", level)
	case barrierScoutSubclassSlug, elementalScoutSubclassSlug, phantomScoutSubclassSlug:
		return scoutNinSuperiorityDice("a", level)
	case assaultScoutSubclassSlug, pathfinderScoutSubclassSlug:
		return scoutNinSuperiorityDice("c", level)
	case tacticalScoutSubclassSlug:
		return scoutNinSuperiorityDice("tactical", level)
	default:
		return 0
	}
}

// scoutManeuversCapFeatGrants: feat slug -> the flat Maneuvers Known cap
// bonus its own text grants. Scouting Adept ("You learn two maneuvers that
// you qualify for. You gain 1 Superiority Die.") and Scouting Veteran ("You
// gain 2 Superiority Die. You learn two maneuvers that you qualify for.")
// are two independent feats, not tiers of one chain, so each contributes
// its own flat +2 rather than a running total — a character could plausibly
// have taken only one of the two. Both feats' own prerequisites (4+ and 8+
// levels in Scout-Nin respectively) already guarantee a real Scout-Nin
// class level and a chosen subclass by the time either could be taken, so
// no additional subclassSlug-empty guard is needed beyond the one
// loadScoutNinTabData already applies around scoutNinManeuversKnownCap.
//
// Each feat's own "+1/+2 Superiority Die" clause is a separate, smaller gap
// not modeled here — it would need to stack onto the existing per-subclass
// superiority_dice customResourceGrants entries (custom_resources.go),
// which use a non-additive "higher Max wins" merge.
var scoutManeuversCapFeatGrants = map[string]int{
	"feat/class/scouting-adept":   2,
	"feat/class/scouting-veteran": 2,
}

// scoutManeuversCapBonus sums any archetype feats' own flat Maneuvers Known
// cap bonus on top of scoutNinManeuversKnownCap's base table value.
func scoutManeuversCapBonus(grantedFeatures []grantedFeatureRow) int {
	bonus := 0
	for slug, grant := range scoutManeuversCapFeatGrants {
		if hasGrantedFeature(grantedFeatures, slug) {
			bonus += grant
		}
	}
	return bonus
}

// shinobiAdeptSlugs: Shinobi Adept's 7 named options exist as separate
// class_features rows (sort_order 7-13 — scattered around Deft Explorer's
// own sub-tiers and Shinobi's Training, not grouped together) rather than a
// class_options catalog list — read live here rather than hand-transcribed,
// same "read the chart, don't retype it" discipline loadMedicalDoctrineCatalog
// already established.
//
// Hidden Technique/Aggressive Technique/Tactical Technique (sort_order
// 28-30) used to be folded into this same list, but they're actually
// Signature Technique's own 3 options (class/scout-nin/feature/signature-
// technique, 10th level), not Shinobi Adept's — migration 0051 mistakenly
// leveled all 10 rows together (level 2) since they happened to be caught
// by the same NULL-level sweep; see signatureTechniqueSlugs below and
// migration 0056_scout_nin_signature_technique_level.sql, which re-levels
// just those 3 to 10.
var shinobiAdeptSlugs = []string{
	"class/scout-nin/feature/shinobis-tactics",
	"class/scout-nin/feature/shinobis-general-literacy",
	"class/scout-nin/feature/shinobis-tool-competency",
	"class/scout-nin/feature/shinobis-precision",
	"class/scout-nin/feature/shinobis-edge",
	"class/scout-nin/feature/shinobis-drive",
	"class/scout-nin/feature/shinobis-focus",
}

// signatureTechniqueSlugs: Signature Technique's (base class, 10th/18th
// level) own 3 named options — "select one of the following which helps in
// building out your already broad skillset... Beginning at 18th level, you
// can instead gain the benefit of two of these options." Each option is a
// separate, fully-populated class_features row (Hidden Technique's own
// ambush-invisibility text, Aggressive Technique's Hit-Die/Chakra-Die-for-HP
// breathing, Tactical Technique's opportunity-attack immunity) immediately
// following Signature Technique's own introducing row in sort_order — not
// missing/truncated data, just wrongly folded into shinobiAdeptSlugs above
// until this fix.
var signatureTechniqueSlugs = []string{
	"class/scout-nin/feature/hidden-technique",
	"class/scout-nin/feature/aggressive-technique",
	"class/scout-nin/feature/tactical-technique",
}

// jackOfAllSlugs: Jack of All, Master of None's 5 named Generalizations —
// same "live class_features rows, no class_options catalog" shape as
// Shinobi Adept above. "mobility" (this list) and "mobile"
// (deftExplorerMobileFeatureSlug above) are two entirely independent
// features that happen to share a near-identical name and speed-related
// text: one is Deft Explorer's automatic 6th-level grant, the other is one
// of these 5 player-picked Generalizations, gained starting 5th level —
// never conflate the two slugs.
var jackOfAllSlugs = []string{
	"class/scout-nin/feature/combat",
	"class/scout-nin/feature/control",
	"class/scout-nin/feature/mobility",
	"class/scout-nin/feature/skill",
	"class/scout-nin/feature/support",
}

// scoutNinClassLevel returns the character's own Scout-Nin class level, or
// 0 if they have none — mirrors taijutsuSpecialistClassLevel.
func (s *server) scoutNinClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, scoutNinSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// scoutNinSubclassSlug resolves the character's own chosen Scouting
// Technique subclass, if any — mirrors hunterNinSubclassSlug exactly.
func (s *server) scoutNinSubclassSlug(characterID int64) (slug, name string, err error) {
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
		if classSlug == scoutNinSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// shinobiAdeptCap: 2 at 2nd level, 4 at 13th ("select two additional
// features").
func shinobiAdeptCap(level int) int {
	switch {
	case level >= 13:
		return 4
	case level >= 2:
		return 2
	default:
		return 0
	}
}

// jackOfAllCap: 1 by default from 5th level, extended for Tactical Scout
// only — 2 once Expert of All, Jack of None (9th) is granted, 3 once
// Unmatched Tactics (20th) is granted on top of that.
func jackOfAllCap(level int, grantedFeatures []grantedFeatureRow) int {
	if level < 5 {
		return 0
	}
	cap := 1
	if hasGrantedFeature(grantedFeatures, expertOfAllFeatureSlug) {
		cap = 2
	}
	if hasGrantedFeature(grantedFeatures, unmatchedTacticsFeatureSlug) {
		cap = 3
	}
	return cap
}

// signatureTechniqueCap: 0 below 10th level, 1 at 10th, 2 at 18th ("select
// one... beginning at 18th level, you can instead gain the benefit of two
// of these options"), plus +1 more once Mastered Signature Technique
// (Tactical Scout, 14th level) is granted on top of that — mirrors
// jackOfAllCap's shape exactly.
func signatureTechniqueCap(level int, grantedFeatures []grantedFeatureRow) int {
	if level < 10 {
		return 0
	}
	cap := 1
	if level >= 18 {
		cap = 2
	}
	if hasGrantedFeature(grantedFeatures, masteredSignatureTechniqueFeatureSlug) {
		cap++
	}
	return cap
}

// mobileSavantCap: 0 below 3rd level, 1 at 3rd, 2 at 6th, 3 at 9th, 4 at
// 14th ("You learn another jutsu from this list when you would reach 6th,
// 9th and 14th levels"). Pathfinder Scout only — the caller gates this on
// the character's own chosen subclass.
func mobileSavantCap(level int) int {
	switch {
	case level >= 14:
		return 4
	case level >= 9:
		return 3
	case level >= 6:
		return 2
	case level >= 3:
		return 1
	default:
		return 0
	}
}

// tacticalSuperiorityCap: 0 below 6th level, 1 at 6th, 2 at 14th, 3 at 20th
// ("You can select one more Maneuver from the aforementioned subclasses at
// 14th and 20th levels"). Tactical Scout only — the caller gates this on
// the character's own chosen subclass.
func tacticalSuperiorityCap(level int) int {
	switch {
	case level >= 20:
		return 3
	case level >= 14:
		return 2
	case level >= 6:
		return 1
	default:
		return 0
	}
}

// signatureManeuverCap: 0 below 3rd level, 1 at 3rd, 2 at 9th ("You can
// select a second maneuver beginning at 9th level"). Tactical Scout only —
// the caller gates this on the character's own chosen subclass.
func signatureManeuverCap(level int) int {
	switch {
	case level >= 9:
		return 2
	case level >= 3:
		return 1
	default:
		return 0
	}
}

// tacticalSuperiorityCatalogSubclassSlugs: the subclasses Tactical
// Superiority's own text names as eligible sources — "• Arbiter Scout •
// Assault Scout • Cloning Scout • Defensive Scout • Pathfinder Scout •
// Phantom Scout". No "Defensive Scout" subclass exists anywhere in this
// book (confirmed against the 9 real Scout-Nin subclass slugs); Barrier
// Scout is the only one of those 9 not already named elsewhere in the
// bullet list, so it's used here as the intended match pending a direct
// re-check against the printed PDF. Tactical Scout's own catalog is
// deliberately excluded (nothing gained by importing your own subclass's
// Maneuvers into itself), as are Elemental and Trickster Scout, which the
// book's own bullet list never names.
var tacticalSuperiorityCatalogSubclassSlugs = []string{
	arbiterScoutSubclassSlug,
	assaultScoutSubclassSlug,
	barrierScoutSubclassSlug,
	cloningScoutSubclassSlug,
	pathfinderScoutSubclassSlug,
	phantomScoutSubclassSlug,
}

// mobileSavantJutsuSlugs: Mobile Savant's own 4-option progressive jutsu
// pick (Pathfinder Scout, 3rd/6th/9th/14th level). These are real jutsu
// table rows, not class_features rows, so this is a small hand-curated slug
// list read against the jutsu table directly, unlike shinobiAdeptSlugs/
// jackOfAllSlugs/signatureTechniqueSlugs above (all class_features rows).
// The feature's OTHER grant, the Chakra Movement Ninjutsu, is not listed
// here — it's a single flat "you learn the X" clause already picked up
// automatically by jutsu_grants.go's generic class-feature text-scanner
// (parseJutsuGrants), independently confirmed against this exact
// description text before this picker was written.
var mobileSavantJutsuSlugs = []string{
	"jutsu/wind-release-zephyr-strike",
	"jutsu/lightning-release-lightning-speed",
	"jutsu/graceful-cat",
	"jutsu/flower-petal-escape",
}

// scoutNinPickOption is one entry in one of Scout-Nin's hand-curated (no
// class_options row) catalog picks.
type scoutNinPickOption struct {
	Slug        string
	Name        string
	Description string
}

// knownScoutNinPick is one chosen entry on a Known list — no Granted
// variant exists (nothing auto-grants a Shinobi Adept option or a Jack of
// All Generalization for free outside the pick), unlike Hunter-Nin's
// knownHunterPick. Carries its own Description so the Known list can show
// a plain rollover tooltip, same treatment knownMedicalDoctrine gets.
type knownScoutNinPick struct {
	Slug        string
	Name        string
	Description string
}

// loadScoutNinFeatureCatalog reads one hand-curated slug list's rows live
// from class_features, in sort_order — mirrors
// loadMedicalDoctrineCatalog's identical shape, generalized to take any
// slug list since Scout-Nin has two of these (Shinobi Adept, Jack of All)
// rather than Medical-Nin's one.
func (s *server) loadScoutNinFeatureCatalog(slugs []string) ([]scoutNinPickOption, error) {
	placeholders := make([]string, len(slugs))
	args := make([]any, 0, len(slugs)+1)
	args = append(args, scoutNinSlug)
	for i, slug := range slugs {
		placeholders[i] = "?"
		args = append(args, slug)
	}
	rows, err := s.rulesDB.Query(fmt.Sprintf(`
		SELECT slug, name, description FROM class_features
		WHERE class_slug = ? AND slug IN (%s)
		ORDER BY sort_order`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scoutNinPickOption
	for rows.Next() {
		var o scoutNinPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// loadScoutNinManeuverCatalog reads one subclass's own Maneuvers catalog —
// unlike Shinobi Adept/Jack of All (hand-curated slug lists, since the book
// states those options directly in their own class_features rows), the 92
// Maneuvers rows genuinely exist as class_options rows, scoped by their own
// subclass_slug column, so this reads that table directly instead —
// mirrors loadHunterOptionCatalog, generalized to filter by subclass_slug
// rather than list_name since every subclass shares class_slug
// "class/scout-nin" but each only ever sees its own Maneuvers. Also
// returns the catalog's own list_name (e.g. "Arbiter Maneuvers") for the
// sheet's own section heading, read from the data rather than a hand-typed
// per-subclass label map.
func (s *server) loadScoutNinManeuverCatalog(subclassSlug string) ([]scoutNinPickOption, string, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description, list_name FROM class_options
		WHERE class_slug = ? AND subclass_slug = ? ORDER BY name`, scoutNinSlug, subclassSlug)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []scoutNinPickOption
	var listName string
	for rows.Next() {
		var o scoutNinPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description, &listName); err != nil {
			return nil, "", err
		}
		out = append(out, o)
	}
	return out, listName, rows.Err()
}

// loadScoutNinManeuverCatalogForSubclasses reads the combined Maneuvers
// catalog across several subclasses at once — Tactical Superiority's own
// "select one Maneuver from one of the Following Scout-Nin Subclasses"
// grant. Generalizes loadScoutNinManeuverCatalog's single-subclass_slug
// query into a subclass_slug IN (...) form, and drops any row carrying the
// "Keyword: Specialized" marker per that feature's own "If the Maneuver has
// the Specialized keyword, you cannot take it" restriction. No list_name is
// returned since results span more than one subclass's own named catalog.
func (s *server) loadScoutNinManeuverCatalogForSubclasses(subclassSlugs []string) ([]scoutNinPickOption, error) {
	placeholders := make([]string, len(subclassSlugs))
	args := make([]any, 0, len(subclassSlugs)+1)
	args = append(args, scoutNinSlug)
	for i, slug := range subclassSlugs {
		placeholders[i] = "?"
		args = append(args, slug)
	}
	rows, err := s.rulesDB.Query(fmt.Sprintf(`
		SELECT slug, name, description FROM class_options
		WHERE class_slug = ? AND subclass_slug IN (%s) ORDER BY name`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scoutNinPickOption
	for rows.Next() {
		var o scoutNinPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		if strings.Contains(o.Description, "Keyword: Specialized") {
			continue // "If the Maneuver has the Specialized keyword, you cannot take it"
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// filterScoutNinManeuversByKeyword returns the subset of a Maneuvers
// catalog whose own description carries the given "Keyword: X" marker —
// used by Signature Maneuver to narrow Tactical Scout's own Maneuvers
// catalog down to the "Keyword: Tactical" options its text requires.
func filterScoutNinManeuversByKeyword(catalog []scoutNinPickOption, keyword string) []scoutNinPickOption {
	var out []scoutNinPickOption
	for _, o := range catalog {
		if strings.Contains(o.Description, keyword) {
			out = append(out, o)
		}
	}
	return out
}

// loadScoutNinMobileSavantCatalog reads Mobile Savant's own 4 jutsu options
// live from the jutsu table — mirrors loadScoutNinFeatureCatalog's shape,
// against a different table since these options are jutsu, not
// class_features rows.
func (s *server) loadScoutNinMobileSavantCatalog() ([]scoutNinPickOption, error) {
	placeholders := make([]string, len(mobileSavantJutsuSlugs))
	args := make([]any, len(mobileSavantJutsuSlugs))
	for i, slug := range mobileSavantJutsuSlugs {
		placeholders[i] = "?"
		args[i] = slug
	}
	rows, err := s.rulesDB.Query(fmt.Sprintf(`
		SELECT slug, name, description FROM jutsu WHERE slug IN (%s) ORDER BY name`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scoutNinPickOption
	for rows.Next() {
		var o scoutNinPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// scoutNinMobileSavantGrantedJutsu returns the character's own Mobile
// Savant pick as slug -> a short badge label, reusing loadGrantedJutsuLabels/
// puppetUpgradeTableGrantedJutsu's own SourceLabel convention
// (jutsu_grants.go) so jutsuKnownCount excludes it from the known-jutsu cap
// for free — Mobile Savant's own text states the picked jutsu "also does
// not count against your known jutsu limit," same as the base Chakra
// Movement grant the generic class-feature text-scanner already picks up
// automatically. Casting the picked jutsu using a Superiority Die instead
// of chakra, and its concentration-cost waiver, are not modeled — a manual
// cross-pool trade the player self-tracks, same boundary Willpower Surge/
// Projected Barrier's own Superiority-Die substitutions already draw.
func (s *server) scoutNinMobileSavantGrantedJutsu(characterID int64) (map[string]string, error) {
	picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickMobileSavant)
	if err != nil {
		return nil, err
	}
	if len(picks) == 0 {
		return nil, nil
	}
	labels := make(map[string]string, len(picks))
	for _, slug := range picks {
		labels[slug] = "Mobile Savant"
	}
	return labels, nil
}

// loadSignatureJutsuOptions catalogs every jutsu the character already
// knows, of B-Rank or lower ("Select one Jutsu you know of B-Rank or
// Lower") — same known-jutsu source as puppets.go's loadThreadSavantJutsuOptions
// (published or custom jutsu alike, via character_jutsu), filtered further
// by masterOfGreenTechniqueMaxRank's own "B-Rank or lower" ceiling rather
// than duplicating that query here.
func (s *server) loadSignatureJutsuOptions(characterID int64) ([]puppetJutsuChoiceOption, error) {
	options, err := s.loadThreadSavantJutsuOptions(characterID)
	if err != nil {
		return nil, err
	}
	var out []puppetJutsuChoiceOption
	for _, o := range options {
		if jutsuRankOrder[o.Rank] > masterOfGreenTechniqueMaxRank {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

// signatureJutsuSlotView holds one Signature Jutsu slot's own compound
// pick — JutsuValue/EffectValue empty until that half has been set. Both
// JutsuOptions and EffectOptions are the same full catalogs on every slot
// (the book text never restricts a later slot's candidates by what an
// earlier slot already picked), carried per-slot only so the sheet's own
// per-slot forms can each render their own <select> without a second
// lookup.
type signatureJutsuSlotView struct {
	Index int

	JutsuValue       string // character_jutsu row id (decimal text), "" if unpicked
	JutsuName        string
	JutsuRank        string
	JutsuDescription string
	JutsuOptions     []puppetJutsuChoiceOption

	EffectValue       string // signature-power/signature-ramping/signature-control slug, "" if unpicked
	EffectName        string
	EffectDescription string
	EffectOptions     []scoutNinPickOption
}

// buildSignatureJutsuSlots resolves Signature Jutsu's own up-to-3 stored
// slots against the character's shared jutsu/effect catalogs.
func buildSignatureJutsuSlots(choices map[features.ChoiceKey]string, jutsuOptions []puppetJutsuChoiceOption, effectOptions []scoutNinPickOption, cap int) []signatureJutsuSlotView {
	slots := make([]signatureJutsuSlotView, cap)
	for i := range slots {
		slot := signatureJutsuSlotView{
			Index:         i,
			JutsuValue:    choices[features.ChoiceKey{FeatureSlug: signatureJutsuFeatureSlug, ChoiceIndex: signatureJutsuChoiceIndex(i, false)}],
			EffectValue:   choices[features.ChoiceKey{FeatureSlug: signatureJutsuFeatureSlug, ChoiceIndex: signatureJutsuChoiceIndex(i, true)}],
			JutsuOptions:  jutsuOptions,
			EffectOptions: effectOptions,
		}
		for _, o := range jutsuOptions {
			if o.Value == slot.JutsuValue {
				slot.JutsuName, slot.JutsuRank, slot.JutsuDescription = o.Name, o.Rank, o.Description
			}
		}
		for _, o := range effectOptions {
			if o.Slug == slot.EffectValue {
				slot.EffectName, slot.EffectDescription = o.Name, o.Description
			}
		}
		slots[i] = slot
	}
	return slots
}

// splitScoutNinPicks classifies a catalog against a character's stored
// picks — mirrors splitMedicalDoctrinePicks/splitHunterPicks.
func splitScoutNinPicks(catalog []scoutNinPickOption, picked map[string]bool) (known []knownScoutNinPick, available []scoutNinPickOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownScoutNinPick{Slug: o.Slug, Name: o.Name, Description: o.Description})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// scoutNinTabData is the sheet_scout_nin box's full data — nil for a
// character with no Scout-Nin levels, same "no empty state" treatment
// every other class box establishes.
type scoutNinTabData struct {
	ShinobiAdeptCap       int
	ShinobiAdeptUsed      int
	KnownShinobiAdept     []knownScoutNinPick
	AvailableShinobiAdept []scoutNinPickOption

	JackOfAllCap       int
	JackOfAllUsed      int
	KnownJackOfAll     []knownScoutNinPick
	AvailableJackOfAll []scoutNinPickOption

	// ManeuversListName is the character's own subclass Maneuvers list's
	// book name (e.g. "Arbiter Maneuvers"), read live off class_options —
	// empty (and Cap 0) for a character with no chosen subclass yet.
	ManeuversListName  string
	ManeuversCap       int
	ManeuversUsed      int
	KnownManeuvers     []knownScoutNinPick
	AvailableManeuvers []scoutNinPickOption

	// SignatureTechniqueCap/etc: base-class 10th/18th level cap+catalog pick
	// over Signature Technique's own 3 options (Hidden/Aggressive/Tactical
	// Technique) — see signatureTechniqueSlugs/signatureTechniqueCap.
	SignatureTechniqueCap       int
	SignatureTechniqueUsed      int
	KnownSignatureTechnique     []knownScoutNinPick
	AvailableSignatureTechnique []scoutNinPickOption

	// MobileSavantCap/etc: Pathfinder Scout's own 3rd/6th/9th/14th level
	// cap+catalog pick over Mobile Savant's 4 alternative jutsu — see
	// mobileSavantJutsuSlugs/mobileSavantCap. The base Chakra Movement
	// grant is not part of this picker (see mobileSavantJutsuSlugs' own
	// comment) — it's already automatic.
	MobileSavantCap       int
	MobileSavantUsed      int
	KnownMobileSavant     []knownScoutNinPick
	AvailableMobileSavant []scoutNinPickOption

	// TacticalSuperiorityCap/etc: Tactical Scout's own 6th/14th/20th level
	// cross-subclass Maneuver pick — see
	// tacticalSuperiorityCatalogSubclassSlugs/tacticalSuperiorityCap.
	TacticalSuperiorityCap       int
	TacticalSuperiorityUsed      int
	KnownTacticalSuperiority     []knownScoutNinPick
	AvailableTacticalSuperiority []scoutNinPickOption

	// SignatureManeuverCap/etc: Tactical Scout's own 3rd/9th level pick of
	// an already-known Tactical-keyword Maneuver — see
	// signatureManeuverCap. Available/Known are drawn from the
	// intersection of the character's own known Maneuvers (KnownManeuvers
	// above) and Tactical Scout's own "Keyword: Tactical" options, not the
	// full Maneuvers catalog.
	SignatureManeuverCap       int
	SignatureManeuverUsed      int
	KnownSignatureManeuver     []knownScoutNinPick
	AvailableSignatureManeuver []scoutNinPickOption

	// SupremeClonesCap/etc: Cloning Scout's own flat 20th-level pick of an
	// already-known Maneuver — see supremeClonesFeatureSlug. Unlike
	// Signature Maneuver, the book text applies no "Keyword: X" filter, so
	// every already-known Maneuver (KnownManeuvers above) is a candidate.
	// Actually letting a clone perform the marked Maneuver for free stays
	// a manual player call; only which Maneuver is currently marked is
	// tracked.
	SupremeClonesCap       int
	SupremeClonesUsed      int
	KnownSupremeClones     []knownScoutNinPick
	AvailableSupremeClones []scoutNinPickOption

	// ChangeOfHeart is Trickster Scout's own 17th-level 3-option benefit
	// pick (Messiah/Izanagi/Satanael) — nil until the character has
	// reached Change of Heart.
	ChangeOfHeart *changeOfHeartView

	// ParagonsPresence is Arbiter Scout's own 14th-level 3-option
	// condition-immunity pick (Berserk/Charmed/Frightened) — nil until the
	// character has reached Paragon's Presence.
	ParagonsPresence *paragonsPresenceView

	// SignatureJutsuCap/SignatureJutsu: base-class 7th/15th level compound
	// jutsu+effect pick (a 3rd slot on Tactical Scout only, once Mastered
	// Signature Jutsu is granted) — see signatureJutsuCap/
	// buildSignatureJutsuSlots. Each slot's own effect governs a
	// per-cast damage/upcast/cost bonus that stays fully manual/narrated
	// (applying it when the marked jutsu is cast); only which jutsu+effect
	// combo currently occupies each slot is tracked.
	SignatureJutsuCap int
	SignatureJutsu    []signatureJutsuSlotView

	// MobileActive is Deft Explorer's own automatic 6th-level grant — see
	// this file's own doc comment for why this is a computed readout
	// (swim/climb/wall/water-walk speed all equal Speed) rather than new
	// persistent fields.
	MobileActive bool
	Speed        int

	// Stance is Scout-Nin's own base-class Fighting Stance pick (1st
	// level) — every Scout-Nin has one, unlike Combat Medic's subclass-
	// gated Martial Competency pick.
	Stance *fightingStanceView
}

// loadScoutNinTabData returns nil for a character with no Scout-Nin
// levels — the template gates the whole box's existence on this being
// non-nil, same treatment HunterTechniques/MedicalNin already establish.
func (s *server) loadScoutNinTabData(characterID int64, sheet *charsheet.Sheet) (*scoutNinTabData, error) {
	scoutNinLevel, err := s.scoutNinClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if scoutNinLevel == 0 {
		return nil, nil
	}

	grantedFeatures, err := s.loadMergedGrantedFeatures(characterID, sheet.ClanSlug, sheet.Level)
	if err != nil {
		return nil, err
	}

	data := &scoutNinTabData{
		MobileActive: hasGrantedFeature(grantedFeatures, deftExplorerMobileFeatureSlug),
		Speed:        sheet.Speed,
	}

	data.ShinobiAdeptCap = shinobiAdeptCap(scoutNinLevel)
	if data.ShinobiAdeptCap > 0 {
		catalog, err := s.loadScoutNinFeatureCatalog(shinobiAdeptSlugs)
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickShinobiAdept)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.ShinobiAdeptUsed = len(picks)
		data.KnownShinobiAdept, data.AvailableShinobiAdept = splitScoutNinPicks(catalog, pickedSet)
	}

	data.JackOfAllCap = jackOfAllCap(scoutNinLevel, grantedFeatures)
	if data.JackOfAllCap > 0 {
		catalog, err := s.loadScoutNinFeatureCatalog(jackOfAllSlugs)
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickJackOfAll)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.JackOfAllUsed = len(picks)
		data.KnownJackOfAll, data.AvailableJackOfAll = splitScoutNinPicks(catalog, pickedSet)
	}

	data.SignatureTechniqueCap = signatureTechniqueCap(scoutNinLevel, grantedFeatures)
	if data.SignatureTechniqueCap > 0 {
		catalog, err := s.loadScoutNinFeatureCatalog(signatureTechniqueSlugs)
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickSignatureTechnique)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.SignatureTechniqueUsed = len(picks)
		data.KnownSignatureTechnique, data.AvailableSignatureTechnique = splitScoutNinPicks(catalog, pickedSet)
	}

	subclassSlug, _, err := s.scoutNinSubclassSlug(characterID)
	if err != nil {
		return nil, err
	}
	if subclassSlug != "" {
		data.ManeuversCap = scoutNinManeuversKnownCap(subclassSlug, scoutNinLevel)
		data.ManeuversCap += scoutManeuversCapBonus(grantedFeatures)
		if data.ManeuversCap > 0 {
			catalog, listName, err := s.loadScoutNinManeuverCatalog(subclassSlug)
			if err != nil {
				return nil, err
			}
			data.ManeuversListName = listName
			picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickManeuvers)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[string]bool, len(picks))
			for _, slug := range picks {
				pickedSet[slug] = true
			}
			data.KnownManeuvers, data.AvailableManeuvers = splitScoutNinPicks(catalog, pickedSet)
			// Used counts only picks that are actually IN the current
			// subclass's own catalog, not every stored row in this
			// category — unlike Shinobi Adept/Jack of All (base-class-wide,
			// their catalog never shrinks), Maneuvers picks are
			// subclass-scoped, so a character who picked Maneuvers under
			// one subclass and later switched to another would otherwise
			// have orphaned, invisible picks silently eating their new
			// subclass's cap. They're left in storage (never deleted here)
			// rather than lost, so switching back restores them.
			data.ManeuversUsed = len(data.KnownManeuvers)

			// Signature Maneuver (Tactical Scout only): its own available
			// list is the intersection of what the character already knows
			// (pickedSet, above) and Tactical Scout's own "Keyword:
			// Tactical" options — reusing this same already-loaded catalog
			// rather than querying it twice.
			if subclassSlug == tacticalScoutSubclassSlug {
				data.SignatureManeuverCap = signatureManeuverCap(scoutNinLevel)
				if data.SignatureManeuverCap > 0 {
					tacticalCandidates := filterScoutNinManeuversByKeyword(catalog, "Keyword: Tactical")
					var known []scoutNinPickOption
					for _, o := range tacticalCandidates {
						if pickedSet[o.Slug] {
							known = append(known, o)
						}
					}
					sigPicks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickSignatureManeuver)
					if err != nil {
						return nil, err
					}
					sigPickedSet := make(map[string]bool, len(sigPicks))
					for _, slug := range sigPicks {
						sigPickedSet[slug] = true
					}
					data.KnownSignatureManeuver, data.AvailableSignatureManeuver = splitScoutNinPicks(known, sigPickedSet)
					data.SignatureManeuverUsed = len(data.KnownSignatureManeuver)
				}
			}

			// Supreme Clones (Cloning Scout only, 20th level): unlike
			// Signature Maneuver there's no "Keyword: X" filter in the book
			// text, so every already-known Maneuver (pickedSet, above) is a
			// candidate.
			if subclassSlug == cloningScoutSubclassSlug && hasGrantedFeature(grantedFeatures, supremeClonesFeatureSlug) {
				data.SupremeClonesCap = 1
				var known []scoutNinPickOption
				for _, o := range catalog {
					if pickedSet[o.Slug] {
						known = append(known, o)
					}
				}
				scPicks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickSupremeClones)
				if err != nil {
					return nil, err
				}
				scPickedSet := make(map[string]bool, len(scPicks))
				for _, slug := range scPicks {
					scPickedSet[slug] = true
				}
				data.KnownSupremeClones, data.AvailableSupremeClones = splitScoutNinPicks(known, scPickedSet)
				data.SupremeClonesUsed = len(data.KnownSupremeClones)
			}
		}

		// Tactical Superiority (Tactical Scout only): a cross-subclass pick
		// independent of the character's own Maneuvers Known cap above —
		// its catalog spans 6 OTHER subclasses' own Maneuvers lists, not
		// this character's chosen one.
		if subclassSlug == tacticalScoutSubclassSlug {
			data.TacticalSuperiorityCap = tacticalSuperiorityCap(scoutNinLevel)
			if data.TacticalSuperiorityCap > 0 {
				catalog, err := s.loadScoutNinManeuverCatalogForSubclasses(tacticalSuperiorityCatalogSubclassSlugs)
				if err != nil {
					return nil, err
				}
				picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickTacticalSuperiority)
				if err != nil {
					return nil, err
				}
				pickedSet := make(map[string]bool, len(picks))
				for _, slug := range picks {
					pickedSet[slug] = true
				}
				data.KnownTacticalSuperiority, data.AvailableTacticalSuperiority = splitScoutNinPicks(catalog, pickedSet)
				data.TacticalSuperiorityUsed = len(data.KnownTacticalSuperiority)
			}
		}

		// Mobile Savant (Pathfinder Scout only).
		if subclassSlug == pathfinderScoutSubclassSlug {
			data.MobileSavantCap = mobileSavantCap(scoutNinLevel)
			if data.MobileSavantCap > 0 {
				catalog, err := s.loadScoutNinMobileSavantCatalog()
				if err != nil {
					return nil, err
				}
				picks, err := charstore.ListScoutNinPicks(s.charDB, characterID, charstore.ScoutNinPickMobileSavant)
				if err != nil {
					return nil, err
				}
				pickedSet := make(map[string]bool, len(picks))
				for _, slug := range picks {
					pickedSet[slug] = true
				}
				data.KnownMobileSavant, data.AvailableMobileSavant = splitScoutNinPicks(catalog, pickedSet)
				data.MobileSavantUsed = len(data.KnownMobileSavant)
			}
		}
	}

	choices, err := features.LoadFeatureChoices(s.charDB, characterID)
	if err != nil {
		return nil, err
	}
	stanceOptions, err := s.loadStanceOptionsByType("taijutsu", "bukijutsu")
	if err != nil {
		return nil, err
	}
	data.Stance = buildFightingStanceView(choices, stanceOptions, scoutNinFightingStanceFeatureSlug)

	if hasGrantedFeature(grantedFeatures, changeOfHeartFeatureSlug) {
		data.ChangeOfHeart = &changeOfHeartView{
			Current: choices[features.ChoiceKey{FeatureSlug: changeOfHeartFeatureSlug, ChoiceIndex: 0}],
			Options: changeOfHeartOptions,
		}
	}

	if hasGrantedFeature(grantedFeatures, paragonsPresenceFeatureSlug) {
		data.ParagonsPresence = &paragonsPresenceView{
			Current: choices[features.ChoiceKey{FeatureSlug: paragonsPresenceFeatureSlug, ChoiceIndex: 0}],
			Options: paragonsPresenceOptions,
		}
	}

	data.SignatureJutsuCap = signatureJutsuCap(scoutNinLevel, grantedFeatures)
	if data.SignatureJutsuCap > 0 {
		jutsuOptions, err := s.loadSignatureJutsuOptions(characterID)
		if err != nil {
			return nil, err
		}
		effectOptions, err := s.loadScoutNinFeatureCatalog(signatureJutsuEffectSlugs)
		if err != nil {
			return nil, err
		}
		data.SignatureJutsu = buildSignatureJutsuSlots(choices, jutsuOptions, effectOptions, data.SignatureJutsuCap)
	}

	return data, nil
}

// handleScoutNinPickAdd builds one category's "learn a pick" route — shared
// since Shinobi Adept and Jack of All validate/store identically, differing
// only in which of scoutNinTabData's own fields govern the cap and the
// currently-pickable list. Mirrors handleHunterPickAdd exactly.
func (s *server) handleScoutNinPickAdd(
	category charstore.ScoutNinPickCategory,
	used func(*scoutNinTabData) int,
	cap func(*scoutNinTabData) int,
	available func(*scoutNinTabData) []scoutNinPickOption,
) http.HandlerFunc {
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
		slug := strings.TrimSpace(r.FormValue("option_slug"))
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for scout-nin pick add:", err)
			return
		}
		data, err := s.loadScoutNinTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load scout-nin for add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Scout-Nin", http.StatusBadRequest)
			return
		}
		if used(data) >= cap(data) {
			http.Error(w, "no slots remaining", http.StatusBadRequest)
			return
		}
		valid := false
		for _, o := range available(data) {
			if o.Slug == slug {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid pick", http.StatusBadRequest)
			return
		}
		if err := charstore.AddScoutNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add scout-nin pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_scout_nin")
	}
}

// handleScoutNinPickDelete builds one category's "forget a pick" route —
// freely, at any time, same "trust the player" boundary every other cap+
// catalog pick in this codebase already draws.
func (s *server) handleScoutNinPickDelete(category charstore.ScoutNinPickCategory) http.HandlerFunc {
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
		slug := strings.TrimSpace(r.FormValue("option_slug"))
		if slug == "" {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		if err := charstore.RemoveScoutNinPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove scout-nin pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_scout_nin")
	}
}

// handleScoutNinFightingStance records Scout-Nin's own base-class Fighting
// Stance pick — generalizes taijutsu.go's storeFightingStanceChoice to a
// Taijutsu-OR-Weapon stance choice (its own text: "between Taijutsu Stance
// and Weapon Stance") instead of the Taijutsu-only catalog Unarmed
// Technique/Martial Competency offer.
func (s *server) handleScoutNinFightingStance(w http.ResponseWriter, r *http.Request) {
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
	scoutNinLevel, err := s.scoutNinClassLevel(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load scout-nin level for fighting stance:", err)
		return
	}
	if scoutNinLevel == 0 {
		http.Error(w, "character has no levels in Scout-Nin", http.StatusBadRequest)
		return
	}
	options, err := s.loadStanceOptionsByType("taijutsu", "bukijutsu")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load scout-nin stance options:", err)
		return
	}
	if err := s.storeFightingStanceChoice(id, scoutNinFightingStanceFeatureSlug, options, slug); err != nil {
		if err == errInvalidStance {
			http.Error(w, "not a valid stance", http.StatusBadRequest)
		} else {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("set scout-nin fighting stance:", err)
		}
		return
	}
	s.respondSheet(w, r, id, "sheet_scout_nin")
}

// changeOfHeartView holds Change of Heart's current pick — nil unless the
// character has reached Trickster Scout's own 17th-level Change of Heart
// feature.
type changeOfHeartView struct {
	Current string // "messiah"/"izanagi"/"satanael", "" if unpicked
	Options []featureChoiceOption
}

// changeOfHeartOptions: freely re-editable any time — the book's own "you
// must complete a long rest to switch which benefit you gain" restriction
// is not enforced anywhere else in this app either, same "trust the player"
// boundary Food For the Soul/Hand Wraps of Passion already draw. Every one
// of the 3 benefits' own triggered effects stays fully manual/reactive,
// including Satanael's own die-size change (not fed back into the
// Superiority Dice/Tricksters Words DieSize readouts) — only which of the 3
// is currently selected is tracked here.
var changeOfHeartOptions = []featureChoiceOption{
	{Value: "messiah", Label: "Messiah", Description: "When a creature within 60 feet of you would be reduced to 0 hit points or less, you can, as a Reaction, spend 1 Superiority Die. They instead fall to 1 hit point, then gain temporary hit points equal to 5 times the result."},
	{Value: "izanagi", Label: "Izanagi", Description: "When you would spend a Superiority Die targeting an allied creature, if the combined total of their d20 and your Superiority Die is 20 or greater, they treat it as rolling a natural 20."},
	{Value: "satanael", Label: "Satanael", Description: "Your Superiority Dice become d8s, and the die used for Tricksters Words becomes a d8 as well."},
}

// handleChangeOfHeart records Change of Heart's re-selectable benefit pick.
func (s *server) handleChangeOfHeart(w http.ResponseWriter, r *http.Request) {
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
	valid := false
	for _, o := range changeOfHeartOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for change of heart:", err)
		return
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(id, sheet.ClanSlug, sheet.Level)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load granted features for change of heart:", err)
		return
	}
	if !hasGrantedFeature(grantedFeatures, changeOfHeartFeatureSlug) {
		http.Error(w, "character has not reached Change of Heart", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, changeOfHeartFeatureSlug, 0, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set change of heart:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_scout_nin")
}

// paragonsPresenceView holds Paragon's Presence's current pick — nil
// unless the character has reached Arbiter Scout's own 14th-level
// Paragon's Presence feature.
type paragonsPresenceView struct {
	Current string // "berserk"/"charmed"/"frightened", "" if unpicked
	Options []featureChoiceOption
}

// paragonsPresenceOptions: freely re-editable any time — the book's own
// "you decide which condition at the end of a Short rest" cadence isn't
// enforced here either, same "trust the player" boundary Change of Heart
// already draws. Actually granting the immunity to nearby allies has no
// ally-targeting mechanism in this app (same boundary Absolute Authority's
// own ally-AC clause draws) — only which of the 3 conditions is currently
// selected is tracked here.
var paragonsPresenceOptions = []featureChoiceOption{
	{Value: "berserk", Label: "Berserk", Description: "Allied creatures within 30 feet of you are immune to the Berserk condition."},
	{Value: "charmed", Label: "Charmed", Description: "Allied creatures within 30 feet of you are immune to the Charmed condition."},
	{Value: "frightened", Label: "Frightened", Description: "Allied creatures within 30 feet of you are immune to the Frightened condition."},
}

// handleParagonsPresence records Paragon's Presence's re-selectable
// condition-immunity pick — mirrors handleChangeOfHeart exactly.
func (s *server) handleParagonsPresence(w http.ResponseWriter, r *http.Request) {
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
	valid := false
	for _, o := range paragonsPresenceOptions {
		if o.Value == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for paragons presence:", err)
		return
	}
	grantedFeatures, err := s.loadMergedGrantedFeatures(id, sheet.ClanSlug, sheet.Level)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load granted features for paragons presence:", err)
		return
	}
	if !hasGrantedFeature(grantedFeatures, paragonsPresenceFeatureSlug) {
		http.Error(w, "character has not reached Paragon's Presence", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, paragonsPresenceFeatureSlug, 0, value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set paragons presence:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_scout_nin")
}

// handleSignatureJutsuSlot validates slotIndex against the character's
// current Signature Jutsu cap and value against validValues(slot), then
// stores it at signatureJutsuFeatureSlug's own ChoiceIndex for
// (slotIndex, effect) — shared by handleSignatureJutsuJutsu/
// handleSignatureJutsuEffect below, since the slot-count validation is
// identical between the two and only the option catalog differs. Reuses
// loadScoutNinTabData (rather than re-deriving cap/catalog inline) so the
// same per-slot Options this handler validates against are exactly what
// the sheet itself just rendered, mirroring handleScoutNinPickAdd's own
// shape.
func (s *server) handleSignatureJutsuSlot(w http.ResponseWriter, r *http.Request, valueField string, effect bool, validValues func(signatureJutsuSlotView) []string) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	slotIndex, convErr := strconv.Atoi(strings.TrimSpace(r.FormValue("slot_index")))
	value := strings.TrimSpace(r.FormValue(valueField))
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for signature jutsu:", err)
		return
	}
	data, err := s.loadScoutNinTabData(id, sheet)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load scout-nin for signature jutsu:", err)
		return
	}
	if data == nil {
		http.Error(w, "character has no levels in Scout-Nin", http.StatusBadRequest)
		return
	}
	if convErr != nil || slotIndex < 0 || slotIndex >= len(data.SignatureJutsu) {
		http.Error(w, "slot not available", http.StatusBadRequest)
		return
	}
	valid := false
	for _, v := range validValues(data.SignatureJutsu[slotIndex]) {
		if v == value {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}
	if err := charstore.SetFeatureChoice(s.charDB, id, signatureJutsuFeatureSlug, signatureJutsuChoiceIndex(slotIndex, effect), value); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("set signature jutsu slot:", err)
		return
	}
	s.respondSheet(w, r, id, "sheet_scout_nin")
}

// handleSignatureJutsuJutsu records one Signature Jutsu slot's jutsu half.
// The paired effect half is set independently via handleSignatureJutsuEffect
// below — the sheet presents the two as a matched pair of forms, but
// nothing server-side requires both halves to be set together.
func (s *server) handleSignatureJutsuJutsu(w http.ResponseWriter, r *http.Request) {
	s.handleSignatureJutsuSlot(w, r, "jutsu_id", false, func(slot signatureJutsuSlotView) []string {
		values := make([]string, len(slot.JutsuOptions))
		for i, o := range slot.JutsuOptions {
			values[i] = o.Value
		}
		return values
	})
}

// handleSignatureJutsuEffect records one Signature Jutsu slot's effect
// half (Signature Power/Ramping/Control).
func (s *server) handleSignatureJutsuEffect(w http.ResponseWriter, r *http.Request) {
	s.handleSignatureJutsuSlot(w, r, "effect_slug", true, func(slot signatureJutsuSlotView) []string {
		values := make([]string, len(slot.EffectOptions))
		for i, o := range slot.EffectOptions {
			values[i] = o.Slug
		}
		return values
	})
}

// scoutNinElementalResistanceEntry resolves Elemental Resistance's own
// "you gain resistance to the damage of your chosen nature release" grant
// (Elemental Scout, 6th level) — not modeled via the static
// passiveTraitGrants table (passive_traits.go) since its Target is the
// player's own Elemental Knowledge pick, not a fixed value that table's
// shape assumes every entry has. Returns nil if the character hasn't
// reached Elemental Resistance yet or hasn't picked a Nature Release.
//
// The same feature's second clause ("add half your Intelligence Modifier"
// to an ally's saving throw against a Nature Release jutsu) is a
// per-roll/reactive bonus, not a standing trait — same boundary this
// codebase already draws elsewhere for combat-trigger effects (e.g.
// Untapped Potential's own "regain a die on a crit" clause) — and isn't
// modeled here.
func scoutNinElementalResistanceEntry(grantedFeatures []grantedFeatureRow, element string) *PassiveTraitEntry {
	if element == "" {
		return nil
	}
	if !hasGrantedFeature(grantedFeatures, scoutNinElementalResistanceFeatureSlug) {
		return nil
	}
	return &PassiveTraitEntry{Target: element, Sources: []string{"Elemental Resistance"}}
}
