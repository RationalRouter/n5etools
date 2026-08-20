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

// Genjutsu Specialist's own mechanics, closing the base-class-wide gaps
// CLASS_AUDIT.md's Genjutsu Specialist entry documents: two custom-resource
// pools (Chakra Disruption's rest-gated use count, Actualization Die's
// Proficiency-Bonus-sized pool — both handled by customResourceGrants, see
// custom_resources.go; the die's own SIZE, d4->d6@9th->d8@17th, isn't a
// count and lives here instead, see actualizationDieSize) and four
// simultaneous cap-gated catalog picks — Malleable Mirages (level 2,
// class_options list_name "Malleable Mirages", known-cap read live off
// v_class_level_resources via classLevelResourceInt, 2@2nd up to 11@20th,
// each entry individually prerequisite-gated, see genjutsu_prereq.go; ~31
// of the 79 entries also carry their own printed rest-scoped use-limit on
// top of the known-Mirage pick itself — see genjutsuMiragePickedRows and
// the matching customResourceGrants entries, custom_resources.go),
// Genjutsu Inception (level 3, "Genjutsu Inception", a single pick from 6
// named entries, each granting one specific Mirage for free at 7th level —
// see genjutsuInceptionMirageAutoGrants), Real World Conversion (level 5, 5
// named class_features rows read live rather than hand-transcribed, cap
// 1->2@9th->3@15th), and Master of Illusion (level 13, a hand-curated
// 4-option catalog with no class_options/class_features rows of its own —
// the book embeds its options directly in the granting feature's own text,
// same shape Hunter-Nin's defensiveTacticsOptions already uses — cap
// 1->2@20th). Also built: the 2 subclasses (of 7) whose own features reduce
// to a cheap level-scaled dice readout — Corrupt Thoughts' Vicious Mockery
// and Siren's Visceral Language's damage die (its own combined use-pool,
// sized by Genjutsu Ability Modifier, is genjutsuVisceralLanguagePool,
// custom_resources.go).
//
// Every one of the 7 subclasses' own per-rest use-count gates found on top
// of the 2 damage-die readouts above is now a customResourceGrants entry
// (custom_resources.go): Beguiler's Beguiling Presence/Beyond Sight/
// Beguiling Influence, Corrupt Thoughts' Nightmare Incarnates, Illusionist's
// Illusionary Fury/Illusionary Rage, Layered Reality's Split Focus/Master of
// Reality, Time Slipper's Misread Clocks/Temporal Mastery, and Siren's Words
// of Detriment. Beguiler's Twisted Casting and Corrupt Thoughts' Psyche
// Breaker (both 10th level, "select two Genjutsu you know") are their own
// jutsu-ID-keyed picks instead — TwistedCastingCap/PsycheBreakerCap below,
// backed by charstore.AddGenjutsuJutsuPick — since neither sources its
// options from a rules-database catalog.
//
// Everything else (the base class's own conditional/triggered features —
// Chakra Disruption's and Actualization's own spend effects, Keen Awareness,
// The Turn, The Prestige, and every subclass's own effect once cast/
// triggered — which creature is targeted, what condition is inflicted,
// which Genjutsu is selected) is documented, not modeled: only the use-count
// gate in front of each is tracked, never the triggered effect itself. See
// CLASS_AUDIT.md's Genjutsu Specialist detail entry's "Documented,
// deliberately not automated this pass" table for the current, per-subclass
// breakdown.
const genjutsuSpecialistSlug = "class/genjutsu-specialist"

const (
	corruptThoughtsSubclassSlug    = "class/genjutsu-specialist/group/genjutsu-pledges/corrupt-thoughts"
	sirenSubclassSlug              = "class/genjutsu-specialist/group/genjutsu-pledges/siren"
	timeSlipperSubclassSlug        = "class/genjutsu-specialist/group/genjutsu-pledges/time-slipper"
	illusionistSubclassSlug        = "class/genjutsu-specialist/group/genjutsu-pledges/illusionist"
	beguilerSubclassSlug           = "class/genjutsu-specialist/group/genjutsu-pledges/beguiler"
	mistyDistortionistSubclassSlug = "class/genjutsu-specialist/group/genjutsu-pledges/misty-distortionist"
)

// genjutsuInceptionMirageAutoGrants: the Genjutsu Inception option_slug ->
// the one named Malleable Mirage it grants for free at 7th level, outside
// the known-Mirage cap — same shape huntersExploitAutoGrants already
// establishes for Hunter-Nin's 8 subclass-exclusive Exploits, just keyed off
// a player's own pick (Inception) rather than a granted subclass feature.
var genjutsuInceptionMirageAutoGrants = map[string]string{
	"class/genjutsu-specialist/option/genjutsu-inception/elemental-manifestation":  "class/genjutsu-specialist/option/malleable-mirages/illusionary-chronicle",
	"class/genjutsu-specialist/option/genjutsu-inception/hallucinatory-instrument": "class/genjutsu-specialist/option/malleable-mirages/song-of-the-end",
	"class/genjutsu-specialist/option/genjutsu-inception/illusionary-weapon":       "class/genjutsu-specialist/option/malleable-mirages/vicious-illusion",
	"class/genjutsu-specialist/option/genjutsu-inception/phantasmal-force":         "class/genjutsu-specialist/option/malleable-mirages/agonizing-thoughts",
	"class/genjutsu-specialist/option/genjutsu-inception/reality-marble":           "class/genjutsu-specialist/option/malleable-mirages/persistent-genjutsu",
	"class/genjutsu-specialist/option/genjutsu-inception/temporal-stopwatch":       "class/genjutsu-specialist/option/malleable-mirages/pause",
}

// masterOfIllusionOptions: Master of Illusion's own 4-option catalog,
// hand-curated straight from class_features (the book embeds these options
// directly in the granting feature's text, not as class_options rows) —
// same reasoning as defensiveTacticsOptions.
var masterOfIllusionOptions = []genjutsuPickOption{
	{Slug: "greater-mastery", Name: "Greater Mastery", Description: "Creatures whose saving throw result is 2 or more lower than your Save DC are treated as if they Critically Failed."},
	{Slug: "higher-understanding", Name: "Higher Understanding", Description: "A creature must beat your Save DC by 10 or more to be treated as if it Critically Succeeded on a Genjutsu you cast."},
	{Slug: "subdued-illusion", Name: "Subdued Illusion", Description: "You can increase the Illusion check DC to identify your Genjutsu by 5."},
	{Slug: "genjutsu-flow", Name: "Genjutsu Flow", Description: "You double the range of Genjutsu you cast. Genjutsu with a Touch range instead have a range of 30 feet."},
}

// genjutsuSpecialistClassLevel returns the character's own Genjutsu
// Specialist class level, or 0 if they have none — mirrors
// hunterNinClassLevel.
func (s *server) genjutsuSpecialistClassLevel(characterID int64) (int, error) {
	var level int
	err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = ? AND class_slug = ?`,
		characterID, genjutsuSpecialistSlug,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return level, err
}

// genjutsuSpecialistSubclassSlug resolves the character's own chosen
// Genjutsu Pledge, if any — mirrors hunterNinSubclassSlug exactly.
func (s *server) genjutsuSpecialistSubclassSlug(characterID int64) (slug, name string, err error) {
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
		if classSlug == genjutsuSpecialistSlug {
			return sc, n, nil
		}
	}
	return "", "", nil
}

// genjutsuActualizedAlterationPickedRows returns a synthetic
// grantedFeatureRow under a key distinct from the real rules-database slug
// when the character has actually picked Actualized Alteration among their
// Real World Conversions. Real World Conversion's 5 options (Actualized
// Alteration/Duplicity/Perception/Perfection/Power) are all NULL-level
// class_features rows, so the real slug is already unconditionally present
// in loadGrantedFeatures' output for every Genjutsu Specialist 5+
// regardless of which Conversion (if any) was picked — a customResourceGrants
// entry keyed to the real slug directly would therefore grant Actualized
// Alteration's own extra pool to every such character. See the
// "genjutsu-pick/actualized-alteration" entry in customResourceGrants
// (custom_resources.go) for the pool this feeds. Mirrors
// medicalDoctrinePickedRows' exact shape (medical_nin.go), which solves the
// identical NULL-level-catalog-row problem for Medical Doctrine.
func (s *server) genjutsuActualizedAlterationPickedRows(characterID int64) ([]grantedFeatureRow, error) {
	picks, err := charstore.ListGenjutsuPicks(s.charDB, characterID, charstore.GenjutsuPickConversion)
	if err != nil {
		return nil, err
	}
	for _, slug := range picks {
		if slug == "class/genjutsu-specialist/feature/actualized-alteration" {
			return []grantedFeatureRow{{Slug: "genjutsu-pick/actualized-alteration", Name: "Actualized Alteration"}}, nil
		}
	}
	return nil, nil
}

// genjutsuMirageOptionSlugPrefix is the common class_options slug prefix
// every Malleable Mirage's real rules-database slug shares — trimmed to get
// the bare suffix genjutsuMirageRestLimitedSlugs and
// genjutsuMirageDeceitfulDuplicateSlug key off.
const genjutsuMirageOptionSlugPrefix = "class/genjutsu-specialist/option/malleable-mirages/"

// genjutsuMirageDeceitfulDuplicateSlug is the synthetic key
// genjutsuMiragePickedRows emits when the character has picked Deceitful
// Duplicate specifically — loadCustomResources checks for its presence to
// call genjutsuDeceitfulDuplicatePool (custom_resources.go) instead of
// resolving a flat customResourceGrants entry, since that Mirage's own
// use-pool is sized by Genjutsu Ability Modifier ("a number of times equal
// to your Genjutsu ability modifier... recover when you finish a long
// rest") rather than a flat printed integer — the same reason Visceral
// Language (sirenVisceralLanguageFeatureSlug) gets a dedicated function
// instead of a flat grant.
const genjutsuMirageDeceitfulDuplicateSlug = "genjutsu-mirage/deceitful-duplicate"

// genjutsuMirageRestLimitedSlugs: the Malleable Mirages (of 79 total) whose
// own printed text carries a rest-scoped use-limit on top of the base
// known-Mirage pick MiragesCap/KnownMirages/AvailableMirages (below)
// already track — e.g. "you can cast X at no cost... once per rest," on
// top of the shared fallback clause every one of these also prints, "if
// you have already reached this Mirage's use limit, you can choose to cast
// the jutsu this Mirage provides by spending N Chakra die" (that fallback
// needs no separate mechanism here — it already spends from the existing,
// separately-tracked Chakra Die pool by hand). Devilish Vigor's own "once
// every 10 minutes" is a real-time cooldown, not rest-scoped, and is
// deliberately excluded. Deceitful Duplicate is also excluded from this
// flat map — see genjutsuMirageDeceitfulDuplicateSlug above. Every Mirage
// without an entry here or there has no printed use-limit of its own at
// all (a permanent always-on effect, or a plain Chakra spend with no
// separate cap).
//
// Value is the display Name for the synthetic grantedFeatureRow
// genjutsuMiragePickedRows emits under "genjutsu-mirage/<key>" — see the
// matching customResourceGrants entries (custom_resources.go), one per key
// here, for the actual Max/regen-tier reading off each Mirage's own text.
var genjutsuMirageRestLimitedSlugs = map[string]string{
	"accelerate":             "Accelerate",
	"ascendant-image":        "Ascendant Image",
	"battle-ready-minds":     "Battle Ready Minds",
	"beneath-the-boot":       "Beneath the Boot",
	"berserk-thoughts":       "Berserk Thoughts",
	"bewitching-whispers":    "Bewitching Whispers",
	"black-fangs":            "Black Fangs",
	"blade-song":             "Blade Song",
	"blinding-lights":        "Blinding Lights",
	"book-of-stolen-secrets": "Book of Stolen Secrets",
	"chained-mind":           "Chained Mind",
	"chains-of-madness":      "Chains of Madness",
	"critical-confusion":     "Critical Confusion",
	"dreadful-word":          "Dreadful Word",
	"dulled-mind":            "Dulled Mind",
	"evil-thoughts":          "Evil Thoughts",
	"freedom-of-frenzy":      "Freedom of Frenzy",
	"ghastly-mist":           "Ghastly Mist",
	"illusionary-brawler":    "Illusionary Brawler",
	"magnum-opus":            "Magnum Opus",
	"misty-thoughts":         "Misty Thoughts",
	"misty-visions":          "Misty Visions",
	"no-light-at-the-end":    "No Light At the End",
	"pause":                  "Pause",
	"psyche-drinker":         "Psyche Drinker",
	"shaken-up":              "Shaken Up",
	"shroud-of-shadows":      "Shroud of Shadows",
	"song-of-the-end":        "Song of the End",
	"superior-thoughts":      "Superior Thoughts",
	"tentative-escape":       "Tentative Escape",
	"thieving-confidence":    "Thieving Confidence",
}

// genjutsuMiragePickedRows returns one synthetic grantedFeatureRow per
// Malleable Mirage the character has actually picked
// (charstore.ListGenjutsuPicks) that carries its own rest-scoped use-limit
// (genjutsuMirageRestLimitedSlugs) or is Deceitful Duplicate specifically
// (genjutsuMirageDeceitfulDuplicateSlug) — same shape
// genjutsuActualizedAlterationPickedRows establishes just above,
// generalized from checking one fixed slug to checking every picked slug
// against a lookup table. A synthetic "genjutsu-mirage/<slug>" key is used
// rather than the real class_options slug for a stronger reason than
// Actualized Alteration's own collision-avoidance: Malleable Mirages' own
// class_options rows never appear in loadMergedGrantedFeatures' output at
// all (that function only ever reads class_features/subclass_features/
// clan_features plus feats, never class_options picks), so without this
// injection customResourceGrants could never match on a Mirage pick no
// matter which slug it was keyed to.
func (s *server) genjutsuMiragePickedRows(characterID int64) ([]grantedFeatureRow, error) {
	picks, err := charstore.ListGenjutsuPicks(s.charDB, characterID, charstore.GenjutsuPickMirage)
	if err != nil {
		return nil, err
	}
	var out []grantedFeatureRow
	for _, slug := range picks {
		suffix := strings.TrimPrefix(slug, genjutsuMirageOptionSlugPrefix)
		if suffix == "deceitful-duplicate" {
			out = append(out, grantedFeatureRow{Slug: genjutsuMirageDeceitfulDuplicateSlug, Name: "Deceitful Duplicate"})
			continue
		}
		if name, ok := genjutsuMirageRestLimitedSlugs[suffix]; ok {
			out = append(out, grantedFeatureRow{Slug: "genjutsu-mirage/" + suffix, Name: name})
		}
	}
	return out, nil
}

// actualizationDieSize: d4 base, d6 at 9th, d8 at 17th — the die's own
// COUNT (equal to Proficiency Bonus, recovers on short or long rest) is a
// customResourceGrants entry (custom_resources.go); this is only the size.
func actualizationDieSize(genjutsuLevel int) string {
	switch {
	case genjutsuLevel >= 17:
		return "d8"
	case genjutsuLevel >= 9:
		return "d6"
	default:
		return "d4"
	}
}

// viciousMockeryDice: Corrupt Thoughts' own damage/penalty-die chart —
// 2d4/1d4 base, 4d4/2d4 at 7th, 6d4/3d4 at 13th, 8d4/4d4 at 16th.
func viciousMockeryDice(genjutsuLevel int) (damage, penalty string) {
	switch {
	case genjutsuLevel >= 16:
		return "8d4", "4d4"
	case genjutsuLevel >= 13:
		return "6d4", "3d4"
	case genjutsuLevel >= 7:
		return "4d4", "2d4"
	default:
		return "2d4", "1d4"
	}
}

// visceralLanguageDie: Siren's own damage/temp-HP die chart — 1d6 base,
// 2d6@6th, 3d6@10th, 4d6@14th, 5d6@18th. The feature's own numeric use-pool
// ("a combined number of times equal to your Genjutsu Ability Modifier per
// long rest") is explicitly deferred — see the doc comment above.
func visceralLanguageDie(genjutsuLevel int) string {
	switch {
	case genjutsuLevel >= 18:
		return "5d6"
	case genjutsuLevel >= 14:
		return "4d6"
	case genjutsuLevel >= 10:
		return "3d6"
	case genjutsuLevel >= 6:
		return "2d6"
	default:
		return "1d6"
	}
}

// genjutsuKeywordDCBonus: the shared +1/+2/+3-at-2nd/10th/19th Genjutsu Save
// DC keyword bonus four subclasses (Beguiler, Illusionist, Misty
// Distortionist, Siren) each grant against their own specific keyword —
// only ever called from inside the genjutsuLevel >= 2 gate those subclasses'
// features first unlock at, so the default +1 case is always safe.
func genjutsuKeywordDCBonus(genjutsuLevel int) int {
	switch {
	case genjutsuLevel >= 19:
		return 3
	case genjutsuLevel >= 10:
		return 2
	default:
		return 1
	}
}

// malleableMiragesCap reads Malleable Mirages' own live chart
// (v_class_level_resources) rather than hand-transcribing it — same
// "read the chart, don't retype it" precedent lethalAttackDice established;
// unlike Lethal Attack's dice-string chart this one is a plain int, so it
// goes through classLevelResourceInt (taijutsu.go) instead.
func (s *server) malleableMiragesCap(level int) (int, error) {
	return s.classLevelResourceInt(genjutsuSpecialistSlug, "Malleable Mirages", level)
}

// genjutsuInceptionCap: a single permanent pick, unlocked at 3rd level.
func genjutsuInceptionCap(level int) int {
	if level >= 3 {
		return 1
	}
	return 0
}

// realWorldConversionCap: 1 at 5th, 2 at 9th, 3 at 15th.
func realWorldConversionCap(level int) int {
	switch {
	case level >= 15:
		return 3
	case level >= 9:
		return 2
	case level >= 5:
		return 1
	default:
		return 0
	}
}

// masterOfIllusionCap: 1 at 13th, 2 at 20th.
func masterOfIllusionCap(level int) int {
	switch {
	case level >= 20:
		return 2
	case level >= 13:
		return 1
	default:
		return 0
	}
}

// Illusionist Training/Expert/Specialist's own feat slugs — three
// archetype-training feats that let a character with no real Genjutsu
// Specialist class levels still pick from a small slice of Malleable
// Mirages/Genjutsu Inception/Real World Conversion/Actualization Die,
// "as if" they held real levels. Illusionist Enthusiast (its own effect —
// picking a Genjutsu Pledge and gaining that Pledge's 6th-level feature —
// isn't modeled anywhere in this file; see the doc comment on
// loadGenjutsuTabData) and Mirage Exhibition (only ever taken alongside
// real levels; see mirageExhibitionMirageBonus below) are read directly by
// slug where needed instead of joining this struct.
const (
	illusionistTrainingFeatSlug   = "feat/class/illusionist-training"
	illusionistExpertFeatSlug     = "feat/class/illusionist-expert"
	illusionistSpecialistFeatSlug = "feat/class/illusionist-specialist"
	mirageExhibitionFeatSlug      = "feat/class/mirage-exhibition"
)

// genjutsuArchetypeFeats reports which of Illusionist Training/Expert/
// Specialist a character holds. Specialist requires Expert as its own
// prerequisite, which requires Training — a character with a deeper feat
// is assumed to also carry every shallower one's own benefits (this app
// doesn't enforce feat prerequisites in code, same "trust the player"
// boundary every other pick in this file already draws).
type genjutsuArchetypeFeats struct {
	Training, Expert, Specialist bool
}

func loadGenjutsuArchetypeFeats(feats []characterFeat) genjutsuArchetypeFeats {
	var g genjutsuArchetypeFeats
	for _, f := range feats {
		switch f.Slug {
		case illusionistTrainingFeatSlug:
			g.Training = true
		case illusionistExpertFeatSlug:
			g.Expert = true
		case illusionistSpecialistFeatSlug:
			g.Specialist = true
		}
	}
	return g
}

func (g genjutsuArchetypeFeats) any() bool {
	return g.Training || g.Expert || g.Specialist
}

// mirageLevel is the "as if you were an Nth level Genjutsu Specialist"
// qualification these feats grant for Malleable Mirages specifically —
// Training says 2nd level, Expert and Specialist each separately say 5th.
// The caller combines this with the character's own real Genjutsu
// Specialist level via max(), and only ever feeds the Mirage catalog's own
// per-entry prerequisite gate (genjutsuMiragePrereqMet) — the known-Mirage
// COUNT is driven by mirageSlotBonus below instead, since each feat grants
// one fixed extra slot rather than scaling the class's own level-keyed
// table (malleableMiragesCap), which would overshoot what the feat text
// actually grants (e.g. Training's "as if 2nd level" would otherwise pull
// in the class table's own 2-Mirage cap at 2nd level, not the single
// Mirage the feat names).
func (g genjutsuArchetypeFeats) mirageLevel() int {
	level := 0
	if g.Training && level < 2 {
		level = 2
	}
	if (g.Expert || g.Specialist) && level < 5 {
		level = 5
	}
	return level
}

// mirageSlotBonus: Training, Expert, and Specialist each separately say
// "You learn one Malleable Mirage" — one extra known-Mirage slot per feat
// held, stacking up to 3.
func (g genjutsuArchetypeFeats) mirageSlotBonus() int {
	n := 0
	if g.Training {
		n++
	}
	if g.Expert {
		n++
	}
	if g.Specialist {
		n++
	}
	return n
}

// conversionSlotBonus: Training and Expert each separately say "Select one
// effect granted by Real World Conversion" — one extra known-Conversion
// slot per feat held. Specialist's own third benefit is a Genjutsu
// Inception pick instead (see inceptionUnlockLevel), not a second
// Conversion.
func (g genjutsuArchetypeFeats) conversionSlotBonus() int {
	n := 0
	if g.Training {
		n++
	}
	if g.Expert {
		n++
	}
	return n
}

// inceptionUnlockLevel: only Specialist grants a Genjutsu Inception pick
// ("Select one Genjutsu Inception. You gain it as if you were a 9th level
// Genjutsu Specialist") — Training and Expert grant no Inception access at
// all, so this deliberately does NOT reuse mirageLevel's own 5. The
// caller feeds this into genjutsuInceptionCap (which only cares whether it
// clears its own single 3rd-level threshold — it never scales past 1) and
// into the 7th-level "Inception auto-grants a free Mirage" check
// (genjutsuInceptionMirageAutoGrants), both of which a real 9th-level
// Genjutsu Specialist would already pass.
func (g genjutsuArchetypeFeats) inceptionUnlockLevel() int {
	if g.Specialist {
		return 9
	}
	return 0
}

// mirageExhibitionMirageBonus: Mirage Exhibition's own text — "You gain 1
// Additional Malleable Mirage that you qualify for. You gain an additional
// Malleable Mirage at 5th and 17th Genjutsu Specialist levels" — 1 base,
// +1 at 5th, +1 at 17th (up to 3 total). The feat's own prerequisite ("At
// least 4+ levels in Genjutsu Specialist") means this only ever applies
// alongside real class levels, unlike Training/Expert/Specialist above —
// no effective-level substitution needed, genjutsuLevel is always the
// character's real level here.
func mirageExhibitionMirageBonus(genjutsuLevel int) int {
	n := 1
	if genjutsuLevel >= 5 {
		n++
	}
	if genjutsuLevel >= 17 {
		n++
	}
	return n
}

func featSlugPresent(feats []characterFeat, slug string) bool {
	for _, f := range feats {
		if f.Slug == slug {
			return true
		}
	}
	return false
}

// genjutsuPickOption is one entry in any of Genjutsu Specialist's four
// catalog picks. Prerequisites is only ever populated for Malleable
// Mirages — blank for the other three.
type genjutsuPickOption struct {
	Slug          string
	Name          string
	Description   string
	Prerequisites string
}

// knownGenjutsuPick is one entry on a Known list — either a player pick
// (has a remove button, counts against its own cap) or Granted true (a
// Mirage auto-granted by the character's own Genjutsu Inception pick, no
// remove button, doesn't count against the cap) — same shape
// knownHunterPick already established.
type knownGenjutsuPick struct {
	Slug    string
	Name    string
	Granted bool
}

// loadGenjutsuOptionCatalog reads one of Genjutsu Specialist's two
// class_options-backed catalogs (list_name "Malleable Mirages" or
// "Genjutsu Inception") in full.
func (s *server) loadGenjutsuOptionCatalog(listName string) ([]genjutsuPickOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description, COALESCE(prerequisites, '') FROM class_options
		WHERE class_slug = ? AND list_name = ? ORDER BY name`, genjutsuSpecialistSlug, listName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []genjutsuPickOption
	for rows.Next() {
		var o genjutsuPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description, &o.Prerequisites); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// loadGenjutsuConversionCatalog reads Real World Conversion's 5 named
// class_features rows live (blank level column — these aren't gated by
// their own level, the base feature's own cap functions handle that) rather
// than hand-transcribing their names/text.
func (s *server) loadGenjutsuConversionCatalog() ([]genjutsuPickOption, error) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description FROM class_features
		WHERE class_slug = ? AND slug LIKE 'class/genjutsu-specialist/feature/actualized-%'
		ORDER BY name`, genjutsuSpecialistSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []genjutsuPickOption
	for rows.Next() {
		var o genjutsuPickOption
		if err := rows.Scan(&o.Slug, &o.Name, &o.Description); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// loadKnownGenjutsu reads every jutsu of classification "Genjutsu" the
// character currently knows (character_jutsu, both a published jutsu_slug
// resolved against rules.db's v_jutsu and a player-created custom_jutsu
// resolved entirely within charDB), for Twisted Casting/Psyche Breaker's own
// pickers to filter and offer. A sibling of ninjutsu_specialist.go's
// loadKnownNinjutsu (same body, different classification filter —
// "Genjutsu" here instead of "Ninjutsu") rather than a shared/generalized
// helper, kept in this file since both picks are Genjutsu Specialist's own,
// the same "one sibling function per class" precedent
// medical_nin.go's loadKnownExpertCombatantJutsu already establishes. A
// stale published jutsu_slug (removed in a rules update) is silently
// skipped, same tolerance loadKnownNinjutsu already extends.
func (s *server) loadKnownGenjutsu(characterID int64) ([]knownJutsuOption, error) {
	rows, err := s.charDB.Query(`
		SELECT id, jutsu_slug, custom_jutsu_id FROM character_jutsu WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	type row struct {
		id        int64
		jutsuSlug sql.NullString
		customID  sql.NullInt64
	}
	var stored []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.jutsuSlug, &r.customID); err != nil {
			rows.Close()
			return nil, err
		}
		stored = append(stored, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []knownJutsuOption
	for _, r := range stored {
		var name, classification, rank, keywords, description string
		if r.jutsuSlug.Valid {
			err := s.rulesDB.QueryRow(
				`SELECT name, classification, rank, keywords, description FROM v_jutsu WHERE slug = ?`, r.jutsuSlug.String,
			).Scan(&name, &classification, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue // stale slug (rules update) — skip rather than break the picker
			}
			if err != nil {
				return nil, err
			}
		} else if r.customID.Valid {
			err := s.charDB.QueryRow(
				`SELECT name, classification, rank, keywords, description FROM custom_jutsu WHERE id = ?`, r.customID.Int64,
			).Scan(&name, &classification, &rank, &keywords, &description)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
		} else {
			continue
		}
		if classification != "Genjutsu" {
			continue
		}
		out = append(out, knownJutsuOption{
			JutsuID:        r.id,
			Name:           name,
			Rank:           rank,
			Description:    description,
			HasCombination: strings.Contains(keywords, "Combination"),
		})
	}
	return out, nil
}

// twistedCastingCap: Beguiler's Twisted Casting, 10th level, "select two
// Genjutsu that you know" — a flat cap of 2, no scaling chart of its own.
func twistedCastingCap(subclassSlug string, genjutsuLevel int) int {
	if subclassSlug == beguilerSubclassSlug && genjutsuLevel >= 10 {
		return 2
	}
	return 0
}

// psycheBreakerCap: Corrupt Thoughts' Psyche Breaker, 10th level, "select
// two Genjutsu that you know that deal damage" — a flat cap of 2, no
// scaling chart of its own. The "that deal damage" qualifier is not
// enforced: jutsu carry no structured damage field anywhere in this schema
// to filter on, so the picker offers every known Genjutsu — the same
// "trust the player" boundary Malleable Mirages' own 3 unenforceable
// Prerequisite-column-free rows already draw.
func psycheBreakerCap(subclassSlug string, genjutsuLevel int) int {
	if subclassSlug == corruptThoughtsSubclassSlug && genjutsuLevel >= 10 {
		return 2
	}
	return 0
}

// splitGenjutsuPicks classifies a catalog against a character's stored picks
// for one category — shared by Inception, Real World Conversion, and Master
// of Illusion; Mirages needs its own version (loadGenjutsuTabData) since it
// also merges the Inception auto-grant and filters by prerequisite.
func splitGenjutsuPicks(catalog []genjutsuPickOption, picked map[string]bool) (known []knownGenjutsuPick, available []genjutsuPickOption) {
	for _, o := range catalog {
		if picked[o.Slug] {
			known = append(known, knownGenjutsuPick{Slug: o.Slug, Name: o.Name})
		} else {
			available = append(available, o)
		}
	}
	return known, available
}

// genjutsuTabData is the sheet_genjutsu box's full data.
type genjutsuTabData struct {
	ActualizationDieSize string

	ViciousMockeryDamageDie  string
	ViciousMockeryPenaltyDie string

	VisceralLanguageDie string

	PassiveGenjutsuDetection int

	KeywordDCBonus      int
	KeywordDCBonusLabel string

	MiragesCap       int
	MiragesUsed      int
	KnownMirages     []knownGenjutsuPick
	AvailableMirages []genjutsuPickOption

	InceptionCap       int
	InceptionUsed      int
	KnownInception     []knownGenjutsuPick
	AvailableInception []genjutsuPickOption

	ConversionCap        int
	ConversionUsed       int
	KnownConversions     []knownGenjutsuPick
	AvailableConversions []genjutsuPickOption

	IllusionMasteryCap       int
	IllusionMasteryUsed      int
	KnownIllusionMastery     []knownGenjutsuPick
	AvailableIllusionMastery []genjutsuPickOption

	// TwistedCasting/PsycheBreaker source their Known/Available lists from
	// the character's own known jutsu (loadKnownGenjutsu) rather than a
	// rules-database catalog — same "Known"/"Available" markup as
	// sheet_ninjutsu_specialist's Refined Ninjutsu/Ninjutsu Master, reusing
	// their knownNinjutsuJutsuPick/knownJutsuOption types as-is.
	TwistedCastingCap       int
	TwistedCastingUsed      int
	KnownTwistedCasting     []knownNinjutsuJutsuPick
	AvailableTwistedCasting []knownJutsuOption

	PsycheBreakerCap       int
	PsycheBreakerUsed      int
	KnownPsycheBreaker     []knownNinjutsuJutsuPick
	AvailablePsycheBreaker []knownJutsuOption
}

// loadGenjutsuTabData returns nil for a character with no Genjutsu
// Specialist levels AND none of Illusionist Training/Expert/Specialist (the
// three archetype-training feats that let a non-Genjutsu-Specialist taste
// these mechanics — see genjutsuArchetypeFeats) — the template gates the
// whole box's existence on this being non-nil, same treatment
// HunterTechniques gets. Each sub-section independently gates its own
// DISPLAY on its own Cap being > 0 (a level 2 Genjutsu Specialist has
// Mirages but not yet Inception/Conversion/Illusion Mastery) — Inception's
// own picks are still loaded even below level 3, since Mirages' auto-grant
// and prerequisite check both need them. TwistedCastingCap/PsycheBreakerCap
// only ever both gate on genjutsuLevel >= 10 (subclassSlug is already
// resolved by the genjutsuLevel >= 2 block above by the time either is
// checked), so loadKnownGenjutsu is skipped entirely for a character who
// qualifies for neither.
//
// Two Genjutsu Specialist feats are deliberately NOT modeled here:
//   - Illusionist Enthusiast ("Select one Genjutsu Specialist Class,
//     Genjutsu Pledge (Subclass). You gain the 6th Level feature it has.")
//     would need a brand-new picker (a 7-option catalog of subclass_features
//     rows, its own charstore.GenjutsuPickCategory, add/delete routes, and a
//     template section) whose payoff, once picked, still couldn't compute
//     anything: none of the 7 Genjutsu Pledges' own 6th-level features are
//     modeled anywhere in this codebase yet (this file's own header comment
//     already documents subclass Reaction/Bonus-Action features beyond the
//     2 existing damage-die readouts as "documented, not modeled") — the
//     pick would only ever render as inert flavor text, not a real gap this
//     change can close.
//   - Real World Conversion's own effect choices (both Training's and
//     Expert's) reuse the class's own existing Conversion picker as-is
//     (ConversionCap/KnownConversions/AvailableConversions below) — no new
//     lookup needed, just an extra known-slot per feat.
func (s *server) loadGenjutsuTabData(characterID int64, sheet *charsheet.Sheet) (*genjutsuTabData, error) {
	genjutsuLevel, err := s.genjutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	feats, err := s.loadCharacterFeats(characterID)
	if err != nil {
		return nil, err
	}
	archFeats := loadGenjutsuArchetypeFeats(feats)
	if genjutsuLevel == 0 && !archFeats.any() {
		return nil, nil
	}

	data := &genjutsuTabData{}
	switch {
	case genjutsuLevel > 0:
		data.ActualizationDieSize = actualizationDieSize(genjutsuLevel)
	case archFeats.Training:
		// Illusionist Training: "represented as a D4" — fixed, no real
		// levels to scale the die size against (actualizationDieSize's own
		// d6@9th/d8@17th tiers are Genjutsu Specialist class-level scaling
		// that Training/Expert/Specialist never grant).
		data.ActualizationDieSize = "d4"
	}

	var subclassSlug string
	if genjutsuLevel >= 2 {
		subclassSlug, _, err = s.genjutsuSpecialistSubclassSlug(characterID)
		if err != nil {
			return nil, err
		}
		switch subclassSlug {
		case corruptThoughtsSubclassSlug:
			data.ViciousMockeryDamageDie, data.ViciousMockeryPenaltyDie = viciousMockeryDice(genjutsuLevel)
		case sirenSubclassSlug:
			data.VisceralLanguageDie = visceralLanguageDie(genjutsuLevel)
			data.KeywordDCBonus, data.KeywordDCBonusLabel = genjutsuKeywordDCBonus(genjutsuLevel), "Auditory-keyword Genjutsu DC"
		case illusionistSubclassSlug:
			illusionsMod, _ := charsheet.SkillModifier(sheet, "Illusions")
			data.PassiveGenjutsuDetection = 10 + illusionsMod
			data.KeywordDCBonus, data.KeywordDCBonusLabel = genjutsuKeywordDCBonus(genjutsuLevel), "Non-damaging Genjutsu DC"
		case beguilerSubclassSlug:
			data.KeywordDCBonus, data.KeywordDCBonusLabel = genjutsuKeywordDCBonus(genjutsuLevel), "Visual-keyword Genjutsu DC"
		case mistyDistortionistSubclassSlug:
			data.KeywordDCBonus, data.KeywordDCBonusLabel = genjutsuKeywordDCBonus(genjutsuLevel), "Inhaled-keyword Genjutsu DC"
		}
	}

	inceptionCatalog, err := s.loadGenjutsuOptionCatalog("Genjutsu Inception")
	if err != nil {
		return nil, err
	}
	inceptionPicks, err := charstore.ListGenjutsuPicks(s.charDB, characterID, charstore.GenjutsuPickInception)
	if err != nil {
		return nil, err
	}
	inceptionPickedSet := make(map[string]bool, len(inceptionPicks))
	for _, slug := range inceptionPicks {
		inceptionPickedSet[slug] = true
	}
	data.InceptionCap = genjutsuInceptionCap(genjutsuLevel)
	if featCap := genjutsuInceptionCap(archFeats.inceptionUnlockLevel()); featCap > data.InceptionCap {
		data.InceptionCap = featCap
	}
	if data.InceptionCap > 0 {
		data.InceptionUsed = len(inceptionPicks)
		data.KnownInception, data.AvailableInception = splitGenjutsuPicks(inceptionCatalog, inceptionPickedSet)
	}

	data.MiragesCap, err = s.malleableMiragesCap(genjutsuLevel)
	if err != nil {
		return nil, err
	}
	if subclassSlug == timeSlipperSubclassSlug && genjutsuLevel >= 2 {
		data.MiragesCap++
	}
	data.MiragesCap += archFeats.mirageSlotBonus()
	if featSlugPresent(feats, mirageExhibitionFeatSlug) {
		data.MiragesCap += mirageExhibitionMirageBonus(genjutsuLevel)
	}
	if data.MiragesCap > 0 {
		mirageCatalog, err := s.loadGenjutsuOptionCatalog("Malleable Mirages")
		if err != nil {
			return nil, err
		}
		miragePicks, err := charstore.ListGenjutsuPicks(s.charDB, characterID, charstore.GenjutsuPickMirage)
		if err != nil {
			return nil, err
		}
		miragePickedSet := make(map[string]bool, len(miragePicks))
		for _, slug := range miragePicks {
			miragePickedSet[slug] = true
		}

		allMirageNames := make(map[string]bool, len(mirageCatalog))
		mirageNameBySlug := make(map[string]string, len(mirageCatalog))
		for _, o := range mirageCatalog {
			allMirageNames[strings.ToLower(o.Name)] = true
			mirageNameBySlug[o.Slug] = o.Name
		}
		allInceptionNames := make(map[string]bool, len(inceptionCatalog))
		inceptionNameBySlug := make(map[string]string, len(inceptionCatalog))
		for _, o := range inceptionCatalog {
			allInceptionNames[strings.ToLower(o.Name)] = true
			inceptionNameBySlug[o.Slug] = o.Name
		}
		knownInceptionNames := make(map[string]bool, len(inceptionPicks))
		for _, slug := range inceptionPicks {
			if n, ok := inceptionNameBySlug[slug]; ok {
				knownInceptionNames[strings.ToLower(n)] = true
			}
		}
		if subclassSlug == timeSlipperSubclassSlug && genjutsuLevel >= 2 {
			// Temporal Technique's bonus Mirage slot unlocks every
			// Temporal-Stopwatch-prerequisite Mirage even without the
			// player actually having picked Temporal Stopwatch as their
			// own Genjutsu Inception — level-based restrictions on those
			// entries (e.g. "10th level") still apply via
			// genjutsuMiragePrereqMet's own level check.
			knownInceptionNames["temporal stopwatch"] = true
		}
		knownMirageNames := make(map[string]bool, len(miragePicks))
		for _, slug := range miragePicks {
			if n, ok := mirageNameBySlug[slug]; ok {
				knownMirageNames[strings.ToLower(n)] = true
			}
		}

		// inceptionAutoGrantLevel folds in Specialist's own "as if 9th
		// level" Inception qualification — a real 9th-level Genjutsu
		// Specialist gets their Inception pick's auto-granted Mirage, so a
		// Specialist-feat character with an Inception pick should too.
		inceptionAutoGrantLevel := genjutsuLevel
		if lvl := archFeats.inceptionUnlockLevel(); lvl > inceptionAutoGrantLevel {
			inceptionAutoGrantLevel = lvl
		}
		var autoGrantSlug string
		if len(inceptionPicks) == 1 && inceptionAutoGrantLevel >= 7 {
			autoGrantSlug = genjutsuInceptionMirageAutoGrants[inceptionPicks[0]]
		}

		// mirageQualifyLevel folds in Training/Expert/Specialist's own "as
		// if Nth level" Mirage qualification (mirageLevel) into the
		// prerequisite gate every Mirage's own "Nth level" clause checks —
		// this is the ONLY thing archFeats.mirageLevel feeds; the known-
		// Mirage COUNT above already comes from mirageSlotBonus instead.
		mirageQualifyLevel := genjutsuLevel
		if lvl := archFeats.mirageLevel(); lvl > mirageQualifyLevel {
			mirageQualifyLevel = lvl
		}

		data.MiragesUsed = len(miragePicks)
		for _, o := range mirageCatalog {
			switch {
			case o.Slug == autoGrantSlug:
				data.KnownMirages = append(data.KnownMirages, knownGenjutsuPick{Slug: o.Slug, Name: o.Name, Granted: true})
			case miragePickedSet[o.Slug]:
				data.KnownMirages = append(data.KnownMirages, knownGenjutsuPick{Slug: o.Slug, Name: o.Name})
			default:
				if genjutsuMiragePrereqMet(o.Prerequisites, mirageQualifyLevel, allInceptionNames, allMirageNames, knownInceptionNames, knownMirageNames) {
					data.AvailableMirages = append(data.AvailableMirages, o)
				}
			}
		}
	}

	data.ConversionCap = realWorldConversionCap(genjutsuLevel) + archFeats.conversionSlotBonus()
	if data.ConversionCap > 0 {
		catalog, err := s.loadGenjutsuConversionCatalog()
		if err != nil {
			return nil, err
		}
		picks, err := charstore.ListGenjutsuPicks(s.charDB, characterID, charstore.GenjutsuPickConversion)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.ConversionUsed = len(picks)
		data.KnownConversions, data.AvailableConversions = splitGenjutsuPicks(catalog, pickedSet)
	}

	data.IllusionMasteryCap = masterOfIllusionCap(genjutsuLevel)
	if data.IllusionMasteryCap > 0 {
		picks, err := charstore.ListGenjutsuPicks(s.charDB, characterID, charstore.GenjutsuPickIllusionMastery)
		if err != nil {
			return nil, err
		}
		pickedSet := make(map[string]bool, len(picks))
		for _, slug := range picks {
			pickedSet[slug] = true
		}
		data.IllusionMasteryUsed = len(picks)
		data.KnownIllusionMastery, data.AvailableIllusionMastery = splitGenjutsuPicks(masterOfIllusionOptions, pickedSet)
	}

	data.TwistedCastingCap = twistedCastingCap(subclassSlug, genjutsuLevel)
	data.PsycheBreakerCap = psycheBreakerCap(subclassSlug, genjutsuLevel)
	if data.TwistedCastingCap > 0 || data.PsycheBreakerCap > 0 {
		known, err := s.loadKnownGenjutsu(characterID)
		if err != nil {
			return nil, err
		}
		if data.TwistedCastingCap > 0 {
			picks, err := charstore.ListGenjutsuJutsuPicks(s.charDB, characterID, charstore.GenjutsuPickTwistedCasting)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[int64]bool, len(picks))
			for _, id := range picks {
				pickedSet[id] = true
			}
			data.TwistedCastingUsed = len(picks)
			for _, o := range known {
				if pickedSet[o.JutsuID] {
					data.KnownTwistedCasting = append(data.KnownTwistedCasting, knownNinjutsuJutsuPick{JutsuID: o.JutsuID, Name: o.Name, Rank: o.Rank})
				} else {
					data.AvailableTwistedCasting = append(data.AvailableTwistedCasting, o)
				}
			}
		}
		if data.PsycheBreakerCap > 0 {
			picks, err := charstore.ListGenjutsuJutsuPicks(s.charDB, characterID, charstore.GenjutsuPickPsycheBreaker)
			if err != nil {
				return nil, err
			}
			pickedSet := make(map[int64]bool, len(picks))
			for _, id := range picks {
				pickedSet[id] = true
			}
			data.PsycheBreakerUsed = len(picks)
			for _, o := range known {
				if pickedSet[o.JutsuID] {
					data.KnownPsycheBreaker = append(data.KnownPsycheBreaker, knownNinjutsuJutsuPick{JutsuID: o.JutsuID, Name: o.Name, Rank: o.Rank})
				} else {
					data.AvailablePsycheBreaker = append(data.AvailablePsycheBreaker, o)
				}
			}
		}
	}

	return data, nil
}

// handleGenjutsuPickAdd builds one category's "learn a pick" route — shared
// since all four catalogs validate/store identically, differing only in
// which of genjutsuTabData's own fields govern the cap and the currently-
// pickable list. Re-running loadGenjutsuTabData on every request (rather
// than trusting the client saw a filtered list) is also what re-enforces
// Malleable Mirages' own prerequisites server-side on add — Available
// already excludes prereq-unmet entries.
func (s *server) handleGenjutsuPickAdd(
	category charstore.GenjutsuPickCategory,
	used func(*genjutsuTabData) int,
	cap func(*genjutsuTabData) int,
	available func(*genjutsuTabData) []genjutsuPickOption,
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
			log.Println("compute sheet for genjutsu pick add:", err)
			return
		}
		data, err := s.loadGenjutsuTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load genjutsu for add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Genjutsu Specialist", http.StatusBadRequest)
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
		if err := charstore.AddGenjutsuPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add genjutsu pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_genjutsu")
	}
}

// handleGenjutsuPickDelete builds one category's "forget a pick" route —
// freely, at any time, same "trust the player" boundary every other pick
// removal in this app already draws.
func (s *server) handleGenjutsuPickDelete(category charstore.GenjutsuPickCategory) http.HandlerFunc {
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
		if err := charstore.RemoveGenjutsuPick(s.charDB, id, category, slug); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove genjutsu pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_genjutsu")
	}
}

// handleGenjutsuJutsuPickAdd builds one of Twisted Casting/Psyche Breaker's
// own "pick a known Genjutsu" routes — shared since both validate/store
// identically, differing only in which of genjutsuTabData's own fields
// govern the cap and the currently-pickable list. Same shape
// handleNinjutsuJutsuPickAdd (ninjutsu_specialist.go) already establishes,
// keyed by a character_jutsu row id (form field "jutsu_id") instead of a
// rules-database option_slug.
func (s *server) handleGenjutsuJutsuPickAdd(
	category charstore.GenjutsuJutsuPickCategory,
	used func(*genjutsuTabData) int,
	cap func(*genjutsuTabData) int,
	available func(*genjutsuTabData) []knownJutsuOption,
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
		jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
		if err != nil {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("compute sheet for genjutsu jutsu pick add:", err)
			return
		}
		data, err := s.loadGenjutsuTabData(id, sheet)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load genjutsu for jutsu pick add:", err)
			return
		}
		if data == nil {
			http.Error(w, "character has no levels in Genjutsu Specialist", http.StatusBadRequest)
			return
		}
		if used(data) >= cap(data) {
			http.Error(w, "no slots remaining", http.StatusBadRequest)
			return
		}
		valid := false
		for _, o := range available(data) {
			if o.JutsuID == jutsuID {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid pick", http.StatusBadRequest)
			return
		}
		if err := charstore.AddGenjutsuJutsuPick(s.charDB, id, category, jutsuID); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add genjutsu jutsu pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_genjutsu")
	}
}

// handleGenjutsuJutsuPickDelete builds one category's "forget a pick"
// route — freely, at any time, same "trust the player" boundary every other
// pick removal in this app already draws.
func (s *server) handleGenjutsuJutsuPickDelete(category charstore.GenjutsuJutsuPickCategory) http.HandlerFunc {
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
		jutsuID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("jutsu_id")), 10, 64)
		if err != nil {
			http.Error(w, "missing pick", http.StatusBadRequest)
			return
		}
		if err := charstore.RemoveGenjutsuJutsuPick(s.charDB, id, category, jutsuID); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("remove genjutsu jutsu pick:", err)
			return
		}
		s.respondSheet(w, r, id, "sheet_genjutsu")
	}
}

// handleGenjutsuPickDetail serves the click-to-open popup for a Known
// Mirage/Inception/Conversion/Illusion Mastery pick (both player-picked and
// the auto-granted Mirage carry a real Slug, so all four link here) — same
// mechanism handleHunterPickDetail already uses. Not character-scoped — all
// four catalogs are static rules content.
func (s *server) handleGenjutsuPickDetail(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	slug := r.PathValue("slug")

	var name, description string
	switch category {
	case "mirage":
		err := s.rulesDB.QueryRow(`
			SELECT name, description FROM class_options
			WHERE slug = ? AND class_slug = ? AND list_name = 'Malleable Mirages'`,
			slug, genjutsuSpecialistSlug,
		).Scan(&name, &description)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load genjutsu mirage detail:", err)
			return
		}
	case "inception":
		err := s.rulesDB.QueryRow(`
			SELECT name, description FROM class_options
			WHERE slug = ? AND class_slug = ? AND list_name = 'Genjutsu Inception'`,
			slug, genjutsuSpecialistSlug,
		).Scan(&name, &description)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load genjutsu inception detail:", err)
			return
		}
	case "conversion":
		err := s.rulesDB.QueryRow(`
			SELECT name, description FROM class_features
			WHERE slug = ? AND class_slug = ? AND slug LIKE 'class/genjutsu-specialist/feature/actualized-%'`,
			slug, genjutsuSpecialistSlug,
		).Scan(&name, &description)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load genjutsu conversion detail:", err)
			return
		}
	case "illusion-mastery":
		found := false
		for _, o := range masterOfIllusionOptions {
			if o.Slug == slug {
				name, description, found = o.Name, o.Description, true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}

	tmpl, ok := pageTemplates["character_sheet.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		log.Println("render genjutsu pick detail: no template registered")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "hunter_pick_detail_card", map[string]any{"Name": name, "Description": description}); err != nil {
		log.Println("render genjutsu pick detail:", err)
	}
}
