package main

import (
	"database/sql"
	"log"
	"net/http"
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
// each entry individually prerequisite-gated, see genjutsu_prereq.go),
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
// and Siren's Visceral Language's damage die (its own numeric use-pool is
// deferred, see CLASS_AUDIT.md — it needs a Genjutsu-Ability-Modifier input
// customResourceGrant's Max signature doesn't carry).
//
// Everything else (the base class's own conditional/triggered features —
// Chakra Disruption's and Actualization's own spend effects, Keen Awareness,
// The Turn, The Prestige — plus all 7 subclasses' own Reaction/Bonus-Action
// features beyond the 2 readouts above) is documented, not modeled — see
// CLASS_AUDIT.md's Genjutsu Specialist detail entry.
const genjutsuSpecialistSlug = "class/genjutsu-specialist"

const (
	corruptThoughtsSubclassSlug = "class/genjutsu-specialist/group/genjutsu-pledges/corrupt-thoughts"
	sirenSubclassSlug           = "class/genjutsu-specialist/group/genjutsu-pledges/siren"
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
}

// loadGenjutsuTabData returns nil for a character with no Genjutsu
// Specialist levels — the template gates the whole box's existence on this
// being non-nil, same treatment HunterTechniques gets. Each sub-section
// independently gates its own DISPLAY on its own Cap being > 0 (a level 2
// Genjutsu Specialist has Mirages but not yet Inception/Conversion/Illusion
// Mastery) — Inception's own picks are still loaded even below level 3,
// since Mirages' auto-grant and prerequisite check both need them.
func (s *server) loadGenjutsuTabData(characterID int64, sheet *charsheet.Sheet) (*genjutsuTabData, error) {
	genjutsuLevel, err := s.genjutsuSpecialistClassLevel(characterID)
	if err != nil {
		return nil, err
	}
	if genjutsuLevel == 0 {
		return nil, nil
	}

	data := &genjutsuTabData{ActualizationDieSize: actualizationDieSize(genjutsuLevel)}

	if genjutsuLevel >= 2 {
		subclassSlug, _, err := s.genjutsuSpecialistSubclassSlug(characterID)
		if err != nil {
			return nil, err
		}
		switch subclassSlug {
		case corruptThoughtsSubclassSlug:
			data.ViciousMockeryDamageDie, data.ViciousMockeryPenaltyDie = viciousMockeryDice(genjutsuLevel)
		case sirenSubclassSlug:
			data.VisceralLanguageDie = visceralLanguageDie(genjutsuLevel)
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
	if data.InceptionCap > 0 {
		data.InceptionUsed = len(inceptionPicks)
		data.KnownInception, data.AvailableInception = splitGenjutsuPicks(inceptionCatalog, inceptionPickedSet)
	}

	data.MiragesCap, err = s.malleableMiragesCap(genjutsuLevel)
	if err != nil {
		return nil, err
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
		knownMirageNames := make(map[string]bool, len(miragePicks))
		for _, slug := range miragePicks {
			if n, ok := mirageNameBySlug[slug]; ok {
				knownMirageNames[strings.ToLower(n)] = true
			}
		}

		var autoGrantSlug string
		if len(inceptionPicks) == 1 && genjutsuLevel >= 7 {
			autoGrantSlug = genjutsuInceptionMirageAutoGrants[inceptionPicks[0]]
		}

		data.MiragesUsed = len(miragePicks)
		for _, o := range mirageCatalog {
			switch {
			case o.Slug == autoGrantSlug:
				data.KnownMirages = append(data.KnownMirages, knownGenjutsuPick{Slug: o.Slug, Name: o.Name, Granted: true})
			case miragePickedSet[o.Slug]:
				data.KnownMirages = append(data.KnownMirages, knownGenjutsuPick{Slug: o.Slug, Name: o.Name})
			default:
				if genjutsuMiragePrereqMet(o.Prerequisites, genjutsuLevel, allInceptionNames, allMirageNames, knownInceptionNames, knownMirageNames) {
					data.AvailableMirages = append(data.AvailableMirages, o)
				}
			}
		}
	}

	data.ConversionCap = realWorldConversionCap(genjutsuLevel)
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
