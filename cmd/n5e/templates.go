package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
)

//go:embed templates/*.html templates/partials/*.html
var templatesFS embed.FS

// pageTemplates holds, for each page name, a template set combining that
// page's own "content" definition with the shared layout and every shared
// partial.
//
// Each page is parsed separately (layout.html + exactly one page file +
// all partials), not all glob-parsed together, because html/template shares
// {{define}} names across every file in a single parse: if home.html and
// clans.html both defined a template named "content", parsing them together
// would let whichever file parses last silently win for BOTH pages. Parsing
// pairs keeps each page's "content" unambiguous. Partials are safe to glob
// in alongside every page because each one uses its own unique {{define}}
// name (never "content") — see templates/partials/tooltip.html for the
// pattern new shared fragments should follow.
var pageTemplates = map[string]*template.Template{}

// dict lets templates build an ad-hoc map to pass multiple named values into
// a {{template}} call, e.g. {{template "tooltip" (dict "Trigger" .Name "Body" .Desc)}}.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", pairs[i])
		}
		m[key] = pairs[i+1]
	}
	return m, nil
}

// seq returns [0, 1, ..., n-1] so a template can {{range}} a fixed count
// (e.g. one <select> per pick-slot) without a backing slice to range over.
func seq(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// sub is plain integer subtraction — the character sheet's Hit Dice
// remaining display (Level - HitDiceSpent) is the only spot in this app
// that needs arithmetic inside a template rather than in Go.
func sub(a, b int) int {
	return a - b
}

// add is sub's counterpart, for the one place a template needs a total of
// two counts it already has (the Features & Traits collapsible's headline
// number, which spans two separate slices).
func add(a, b int) int {
	return a + b
}

// signed renders a modifier with an explicit sign ("+3", "-1", "+0") —
// several sheet numbers (ability/skill/save modifiers) can go negative, so
// the manual "+{{.}}"-in-template pattern used elsewhere for always-
// non-negative numbers (e.g. proficiency bonus) doesn't apply here.
func signed(n int) string {
	if n >= 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprintf("%d", n)
}

// signedFloat is signed for a float64 — the bulk readout's Strength
// contribution, which is the only signed number on the sheet that isn't an
// int (bulk is a REAL column throughout). 'f' with precision -1 keeps whole
// values whole ("+4", not "+4.0") and never switches to exponent form, the
// same reasoning comma's doc spells out.
func signedFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if v >= 0 {
		return "+" + s
	}
	return s
}

// comma renders a float64 as a thousands-separated decimal string, never
// scientific notation. Ryo is stored as a REAL, and Go's default template
// rendering of a float64 switches to %g-style exponent form once the value
// gets large — a million ryo printed as "1.0018e+06" instead of
// "1,001,800". strconv.FormatFloat with 'f' and precision -1 is the fix:
// 'f' has no exponent form at all, and -1 asks for the shortest digit
// string that round-trips, so whole numbers stay whole and a fractional
// value keeps exactly the digits it needs.
func comma(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i := 0; i < len(intPart); i++ {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(intPart[i])
	}
	b.WriteString(frac)
	return b.String()
}

// imageDataURL vouches for a stored portrait so it can be used as an
// <img src>. html/template's URL filter only trusts http, https, mailto
// and relative URLs — everything else is rewritten to "#ZgotmplZ", so a
// data: URL interpolated normally renders as a broken image with no
// obvious cause. The vouching is narrow on purpose: only the exact
// "data:image/<raster>;base64," prefixes handleSheetPortrait itself
// writes are returned as trusted, and anything else (including a data:
// URL claiming to be SVG, which can carry script) comes back empty so the
// template falls through to its no-portrait branch.
func imageDataURL(s string) template.URL {
	for _, prefix := range []string{
		"data:image/png;base64,",
		"data:image/jpeg;base64,",
		"data:image/gif;base64,",
		"data:image/webp;base64,",
	} {
		if strings.HasPrefix(s, prefix) {
			return template.URL(s)
		}
	}
	return ""
}

// abilityLabel expands a 3-letter ability code ("str") to its full name
// ("Strength"), reusing classes.go's existing abilityLabels map rather than
// a second copy of the same 6 names.
func abilityLabel(ab string) string {
	if label, ok := abilityLabels[ab]; ok {
		return label
	}
	return ab
}

// slugID turns a slug into something safe to use as an HTML id, by replacing
// the path separators: "jutsu/hyuga/gentle-fist" -> "jutsu-hyuga-gentle-fist".
//
// getElementById (which is what sheet-inventory.js's ✎ handler uses) would cope
// with the slashes, but "#jutsu/hyuga/..." is not a valid CSS selector, so a
// raw slug id is a trap waiting for the first querySelector to touch it.
func slugID(slug string) string {
	return strings.ReplaceAll(slug, "/", "-")
}

// jsonify marshals a value for embedding inside a <script type="application/
// json"> block (e.g. the Point Buy cost table, so create-abilities.js can
// compute a live remaining-points counter without re-deriving the 30-point/
// 8-15 rule a second time in JS). Returns template.JS rather than a plain
// string so html/template's contextual auto-escaper treats it as already-
// safe script content instead of JS-escaping the JSON's own quotes.
func jsonify(v any) (template.JS, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return template.JS(b), nil
}

// templateFuncs is shared by every page's template set, including
// companion_sheet.html's (see below) which uses layout_bare.html instead of
// layout.html but still needs the same helpers (formatDescription in
// particular, for its rules-reference panels).
var templateFuncs = template.FuncMap{
	"formatDescription": formatDescription, "formatWoWDescription": formatWoWDescription, "dict": dict, "diceAvg": diceAvg,
	"jsonify": jsonify, "skillOptionPairs": skillOptionPairs, "seq": seq,
	"jutsuKnownCount": jutsuKnownCount,
	"abilityLabel":    abilityLabel, "signed": signed, "signedFloat": signedFloat,
	"sub": sub, "add": add,
	"comma": comma, "imageDataURL": imageDataURL,
	// The six abilities in canonical order, for fieldsets nested
	// too deep for the page's own AbilityOrder to reach without
	// threading it through four layers of dict.
	"abilities": func() []string { return charsheet.Abilities },
	"slugID":    slugID,
	// Ability codes are stored lowercase and shown uppercase.
	// A real uppercase string rather than CSS text-transform,
	// because <option> text is rendered by the OS on some
	// platforms and ignores the transform there.
	"upper":                        strings.ToUpper,
	"assetVersion":                 func() string { return assetVersion },
	"puppetUpgradePickIsPermanent": puppetUpgradePickIsPermanent,
	// pathEscape: for a URL PATH segment built from a free-text name (e.g.
	// Mastery's own skill/toolkit name, "Sleight of Hand"/"Armorsmith
	// kit") — html/template's builtin "urlquery" uses query-string
	// escaping (space -> "+"), which is wrong for a path segment (net/url
	// only treats "+" as space inside a query string, not a path), so a
	// name with a space would round-trip back as a literal "+" instead of
	// a space via r.PathValue. url.PathEscape encodes space as %20
	// instead, which decodes correctly either way.
	"pathEscape": url.PathEscape,
	// masteryBonus/masteryRankName: the two Mastery lookups a row needs to
	// label itself, so the rank stored on a skill/proficiency row is enough
	// on its own and neither the bonus nor the book's Shinobi Rank name has
	// to be threaded through as extra fields on every row type that shows
	// one (see charsheet.MasteryBonus/MasteryRankLabel).
	"masteryBonus": charsheet.MasteryBonus,
	"masteryRankName": func(rank int) string {
		return charsheet.MasteryRankLabel(rank, true)
	},
	"hasOptionDescription":               hasOptionDescription,
	"ninDogReferenceOrZero":              ninDogReferenceOrZero,
	"titanReferenceOrZero":               titanReferenceOrZero,
	"snbReferenceOrZero":                 snbReferenceOrZero,
	"puppetCompanionViewOrZero":          puppetCompanionViewOrZero,
	"companionKindLabel":                 companionKindLabel,
	"companionSupportsStructuredAttacks": companionSupportsStructuredAttacks,
	// firstNonEmpty: picks whichever companion-kind reference's own
	// Expected* string field is actually set — the Companions tab's
	// sheet_summon_tab and the companion popup both build one shared
	// Expected* dict (companion_stat_fields' own doc) from EVERY kind's
	// reference at once (ninDogReferenceOrZero/titanReferenceOrZero/
	// snbReferenceOrZero), even though a single companion is only ever one
	// kind — so exactly one of the two args passed to any single call is
	// ever non-empty for a given companion (nested calls handle a third
	// source), same reasoning "add" already relies on for the numeric
	// Expected* fields.
	"firstNonEmpty": func(a, b string) string {
		if a != "" {
			return a
		}
		return b
	},
}

func init() {
	for _, page := range []string{
		"home.html",
		"characters.html", "character_new.html", "creation_hub.html", "backups.html",
		"create_clan.html", "create_class.html", "create_abilities.html",
		"create_background.html", "create_equipment.html", "create_jutsu.html", "create_ambitions.html",
		"character_sheet.html", "sheet_class.html",
		"clans.html", "clan_detail.html",
		"jutsu.html", "jutsu_detail.html",
		"classes.html", "class_detail.html",
		"feats.html", "feat_detail.html",
		"backgrounds.html", "background_detail.html",
		"stances.html", "stance_detail.html",
		"seals.html", "seal_detail.html",
		"items.html", "item_detail.html", "properties.html", "property_detail.html",
		"traps.html", "trap_detail.html", "poisons.html", "poison_detail.html",
		"about.html", "bugs.html", "faq.html", "technical.html",
	} {
		pageTemplates[page] = template.Must(
			template.New("layout").
				Funcs(templateFuncs).
				ParseFS(templatesFS, "templates/layout.html", "templates/"+page, "templates/partials/*.html"))
	}

	// companion_sheet.html is the one page that opens in its own small
	// popup window rather than the main app tab (see companions.go's
	// handleCompanionSheet) — it uses layout_bare.html (no nav, no dice
	// roller, none of the main sheet's box-grid/library JS) instead of
	// layout.html, so it stays genuinely lightweight rather than just
	// visually smaller.
	pageTemplates["companion_sheet.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/companion_sheet.html", "templates/partials/*.html"))

	// character_reference.html: the "Class Reference" popup — same bare,
	// no-nav layout as companion_sheet.html, for the same reason (a small
	// separate window, not a scaled-down copy of the main page).
	pageTemplates["character_reference.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_reference.html", "templates/partials/*.html"))

	// character_reference_picker.html: the multiclass Class Reference
	// popup's "which class?" first stop — see handleCharacterReference.
	pageTemplates["character_reference_picker.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_reference_picker.html", "templates/partials/*.html"))

	// character_clan_reference.html: the "Clan Reference" popup — same
	// bare layout again, reusing the standalone /clans/{slug} page's own
	// clan_detail_card partial (see clan_reference.go's
	// handleCharacterClanReference).
	pageTemplates["character_clan_reference.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_clan_reference.html", "templates/partials/*.html"))

	// character_custom_features.html: the "Custom Features" popup — same
	// bare layout again, for homebrew/DM-granted features (see
	// characters.go's handleCharacterCustomFeatures).
	pageTemplates["character_custom_features.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_custom_features.html", "templates/partials/*.html"))

	// character_snb_upgrades.html / character_titan_slots.html: the first
	// two "subclass tracker popup" pages (see cmd/n5e/subclass_tracker_
	// popup.go's own header doc for the pattern) — same bare layout again,
	// same partials glob so both pick up partials/subclass_tracker_
	// popup.html's shared blocks alongside every other partial.
	pageTemplates["character_snb_upgrades.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_snb_upgrades.html", "templates/partials/*.html"))
	pageTemplates["character_titan_slots.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_titan_slots.html", "templates/partials/*.html"))

	// Five more subclass tracker popups, same bare layout/partials glob:
	// Elemental Innovationist, Grenadier, Mad Scientist, Ninjaneer,
	// Shinobi-Ware.
	pageTemplates["character_science_nin_elemental_innovationist.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_science_nin_elemental_innovationist.html", "templates/partials/*.html"))
	pageTemplates["character_science_nin_grenadier.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_science_nin_grenadier.html", "templates/partials/*.html"))
	pageTemplates["character_science_nin_mad_scientist.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_science_nin_mad_scientist.html", "templates/partials/*.html"))
	pageTemplates["character_science_nin_ninjaneer.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_science_nin_ninjaneer.html", "templates/partials/*.html"))
	pageTemplates["character_science_nin_shinobi_ware.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_science_nin_shinobi_ware.html", "templates/partials/*.html"))

	// Three more subclass tracker popups, same bare layout/partials glob:
	// Spyware, Storm Rider, Technobi — the last of Science-Nin's nine
	// subclasses to move off the Core sheet's own inline trackers.
	pageTemplates["character_science_nin_spyware.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_science_nin_spyware.html", "templates/partials/*.html"))
	pageTemplates["character_science_nin_storm_rider.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_science_nin_storm_rider.html", "templates/partials/*.html"))
	pageTemplates["character_science_nin_technobi.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_science_nin_technobi.html", "templates/partials/*.html"))

	// character_weapon_form.html / character_taijutsu_stancer.html /
	// character_taijutsu_passionate_flame.html / character_taijutsu_ruin.
	// html: four more subclass tracker popups, same bare layout/partials
	// glob — Weapon Specialist's Weapon Form (all 8 Forms share one popup,
	// see weapon_form_popup.go) and Taijutsu Specialist's Stancer,
	// Passionate Flame, and Ruin.
	pageTemplates["character_weapon_form.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_weapon_form.html", "templates/partials/*.html"))
	pageTemplates["character_taijutsu_stancer.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_taijutsu_stancer.html", "templates/partials/*.html"))
	pageTemplates["character_taijutsu_passionate_flame.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_taijutsu_passionate_flame.html", "templates/partials/*.html"))
	pageTemplates["character_taijutsu_ruin.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_taijutsu_ruin.html", "templates/partials/*.html"))

	// character_hunter_creed.html: one more subclass tracker popup, same
	// bare layout/partials glob — Hunter-Nin's 8 Hunter's Creeds, all
	// sharing one popup the same way Weapon Form's 8 Forms already do (see
	// hunter_creed_popup.go's own header doc).
	pageTemplates["character_hunter_creed.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_hunter_creed.html", "templates/partials/*.html"))

	// character_intelligence_operative_operative_traps.html /
	// character_ninjutsu_specialist_awakened_scroll.html / character_
	// medical_nin_combat_medic.html: three more subclass tracker popups,
	// same bare layout/partials glob — Intelligence Operative's Tactical
	// Strategist (Operative Traps), Ninjutsu Specialist's Scribe Master
	// (Awakened Scroll), and Medical-Nin's Combat Medic (Fighting
	// Stance + Expert Combatant).
	pageTemplates["character_intelligence_operative_operative_traps.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_intelligence_operative_operative_traps.html", "templates/partials/*.html"))
	pageTemplates["character_ninjutsu_specialist_awakened_scroll.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_ninjutsu_specialist_awakened_scroll.html", "templates/partials/*.html"))
	pageTemplates["character_medical_nin_combat_medic.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_medical_nin_combat_medic.html", "templates/partials/*.html"))

	// character_scout_nin_scouting_technique.html: one more subclass
	// tracker popup, same bare layout/partials glob — Scout-Nin's 9
	// Scouting Techniques, all sharing one popup the same way Hunter's
	// Creed's 8 Creeds already do (see scout_nin_scouting_technique_
	// popup.go's own header doc).
	pageTemplates["character_scout_nin_scouting_technique.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_scout_nin_scouting_technique.html", "templates/partials/*.html"))

	// character_genjutsu_twisted_casting.html / character_genjutsu_psyche_
	// breaker.html: two more subclass tracker popups, same bare layout/
	// partials glob — Genjutsu Specialist's Beguiler (Twisted Casting) and
	// Corrupt Thoughts (Psyche Breaker).
	pageTemplates["character_genjutsu_twisted_casting.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_genjutsu_twisted_casting.html", "templates/partials/*.html"))
	pageTemplates["character_genjutsu_psyche_breaker.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_genjutsu_psyche_breaker.html", "templates/partials/*.html"))

	// character_cooking_nin_pipe.html / character_cooking_nin_expert_
	// combatant.html / character_cooking_nin_fast_and_furious.html /
	// character_cooking_nin_blend_enhancements.html: four more subclass
	// tracker popups, same bare layout/partials glob — Cooking-Nin's
	// Herbalist (Bonus Tool Infusion: Pipe), Battle Cook (Expert Combatant),
	// Entremetier Chef (Fast and Furious), and Gastrochemist (Nature's
	// Blend Enhancements).
	pageTemplates["character_cooking_nin_pipe.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_cooking_nin_pipe.html", "templates/partials/*.html"))
	pageTemplates["character_cooking_nin_expert_combatant.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_cooking_nin_expert_combatant.html", "templates/partials/*.html"))
	pageTemplates["character_cooking_nin_fast_and_furious.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_cooking_nin_fast_and_furious.html", "templates/partials/*.html"))
	pageTemplates["character_cooking_nin_blend_enhancements.html"] = template.Must(
		template.New("layout").
			Funcs(templateFuncs).
			ParseFS(templatesFS, "templates/layout_bare.html", "templates/character_cooking_nin_blend_enhancements.html", "templates/partials/*.html"))
}
