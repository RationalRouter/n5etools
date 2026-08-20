package main

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

// featAbilityIncreaseRe matches the common "Increase your/the X [ability]
// [score] by N[, to a/an/the max[imum] [of] M]" clause that opens ~200 of
// this project's 371 feats (validated against the live rules.db corpus —
// see FEAT_AUDIT.md). The "score" word is optional: most feats include it
// ("Increase your Strength Score by 1") but at least 9 omit it entirely
// ("Increase your Strength by 1" — illusionist-research, poisoner, stealthy,
// and others). Ability names not recognized by abilityNameToKey (e.g. the
// "Clash Checks"/"initiative"/damage-die increases some non-ability feats
// also phrase as "Increase ... by N") still match this regex's loose shape
// but yield zero valid keys downstream in parseFeatAbilityIncrease, so they
// parse as not-an-ability-increase exactly as before this word was made
// optional — dropping "score" widens what the regex matches as text, not
// what it treats as a real ability bonus. The ability-list group accepts
// one or more names joined by ", "/"or"/"and" (e.g. "Intelligence or
// Wisdom", "Strength, Dexterity, or Constitution"). The trailing "to a
// maximum of M" clause is optional and its value unused (this mechanism
// doesn't runtime-clamp against it — see the plan doc) because a handful
// of real feats (empathic, the three X
// Release Expert feats) have it malformed in the sourcebook text itself
// ("to a maximum 20" missing "of", "to a maximum of" missing the number
// entirely) — since only the amount (group 2) is actually used, requiring
// the cap clause to parse cleanly would reject real, otherwise-fine feats
// over a decorative value this code never reads.
var featAbilityIncreaseRe = regexp.MustCompile(
	`(?i)Increase (?:your|the) ([A-Za-z, ]+?)(?: ability)?(?: [Ss]core)? by \+?(\d+)(?:,?\s*(?:up )?to (?:a|an|the) [Mm]ax(?:imum)?(?: of)? \d*)?`,
)

// featAbilityIncreaseOverrides lists feats whose ability-increase clause
// reads as a free choice of any ability ("Increase one Ability score by
// 1...") rather than naming 2-3 specific ones — the general regex's
// ability-list group can't express "all six", so these are listed by hand
// instead of trying to generalize the regex to match them.
var featAbilityIncreaseOverrides = map[string][]string{
	"feat/advanced-study":   {"str", "dex", "con", "int", "wis", "cha"},
	"feat/resilient":        {"str", "dex", "con", "int", "wis", "cha"},
	"feat/practiced-adept":  {"str", "dex", "con", "int", "wis", "cha"},
	"feat/practiced-expert": {"str", "dex", "con", "int", "wis", "cha"},
}

// featAbilityIncreaseExcluded lists feats whose description matches the
// regex's loose SQL prefilter ("Increase your...score by...") but are not
// an ability-score increase at all — e.g. raising a summoned Puppet's own
// stat, not the character's. Confirmed by reading each one directly against
// dist/rules.db.
var featAbilityIncreaseExcluded = map[string]bool{
	"feat/class/puppeteer-training": true,
	"feat/class/swifter-response":   true,
	"feat/class/backup-plan-new":    true,
}

// abilityNameToKey normalizes a full ability name, as it appears in feat
// prose ("Intelligence"), to the three-letter key used everywhere else in
// this codebase. Mirrors abilityLabels (classes.go) in reverse.
func abilityNameToKey(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "strength", "str":
		return "str", true
	case "dexterity", "dex":
		return "dex", true
	case "constitution", "con":
		return "con", true
	case "intelligence", "int":
		return "int", true
	case "wisdom", "wis":
		return "wis", true
	case "charisma", "cha":
		return "cha", true
	default:
		return "", false
	}
}

// parseFeatAbilityIncrease extracts a feat's ability-score-increase clause,
// if it has one, returning the eligible ability keys and the amount to
// apply. When len(abilities) == 1 the increase is unconditional and should
// be applied immediately; when >1 it's a player choice.
func parseFeatAbilityIncrease(slug, description string) (abilities []string, amount int, ok bool) {
	if featAbilityIncreaseExcluded[slug] {
		return nil, 0, false
	}
	if override, found := featAbilityIncreaseOverrides[slug]; found {
		return override, 1, true
	}

	m := featAbilityIncreaseRe.FindStringSubmatch(description)
	if m == nil {
		return nil, 0, false
	}

	amount = 1
	if n, err := strconv.Atoi(m[2]); err == nil {
		amount = n
	}

	var keys []string
	seen := make(map[string]bool)
	for _, part := range splitAbilityList(m[1]) {
		key, found := abilityNameToKey(part)
		if !found {
			continue
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, 0, false
	}
	return keys, amount, true
}

// splitAbilityList breaks a comma/or/and-joined ability list ("Strength,
// Dexterity, or Constitution") into individual name fragments. Flushes on
// both a trailing comma and a literal "or"/"and" token, so an Oxford-comma
// three-option list ("Strength, Dexterity or Constitution" — the comma
// stops after the second name, not the third) still splits into three
// fragments rather than gluing the first two names into one unrecognized
// fragment that abilityNameToKey silently drops.
func splitAbilityList(s string) []string {
	fields := strings.Fields(s)
	var parts []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			parts = append(parts, strings.Join(cur, " "))
			cur = nil
		}
	}
	for _, f := range fields {
		trimmed := strings.TrimSuffix(f, ",")
		hadComma := trimmed != f
		low := strings.ToLower(trimmed)
		if low == "or" || low == "and" {
			flush()
			continue
		}
		cur = append(cur, trimmed)
		if hadComma {
			flush()
		}
	}
	flush()
	return parts
}

// pendingFeatAbilityChoiceRow is one taken feat whose ability-increase
// clause names more than one option (Hive Minded's "Intelligence or
// Wisdom") and hasn't been resolved yet — same visual shape as
// pendingFeatureChoiceRow's IsAbilityScore branch (feature_choices.go), but
// rendered from a separate list since this mechanism is deliberately
// isolated from internal/features' own choice-slot engine (see the plan
// doc's architecture rationale).
type pendingFeatAbilityChoiceRow struct {
	FeatSlug string
	Label    string
	Options  []featureChoiceOption
}

// buildPendingFeatAbilityChoiceRows finds every taken feat with an
// unresolved multi-option ability-increase clause. characterFeats is the
// caller's already-loaded s.loadCharacterFeats result — both existing
// sheet-render call sites already have it for the Feats tab, so this avoids
// a second identical query.
func (s *server) buildPendingFeatAbilityChoiceRows(characterID int64, characterFeats []characterFeat) ([]pendingFeatAbilityChoiceRow, error) {
	resolved := make(map[string]bool)
	rows, err := s.charDB.Query(
		`SELECT source_ref FROM character_ability_bonuses WHERE character_id = ? AND source_kind = 'feat'`,
		characterID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			rows.Close()
			return nil, err
		}
		resolved[ref] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []pendingFeatAbilityChoiceRow
	for _, feat := range characterFeats {
		if resolved[feat.Slug] {
			continue
		}
		abilities, amount, ok := parseFeatAbilityIncrease(feat.Slug, feat.Description)
		if !ok || len(abilities) <= 1 {
			continue
		}
		row := pendingFeatAbilityChoiceRow{FeatSlug: feat.Slug, Label: feat.Name}
		for _, ab := range abilities {
			row.Options = append(row.Options, featureChoiceOption{
				Value: ab,
				Label: fmt.Sprintf("%s (+%d)", abilityLabels[ab], amount),
				Description: fmt.Sprintf(
					"Permanently increases your %s score by +%d. This pick cannot be changed later.",
					abilityLabels[ab], amount),
			})
		}
		out = append(out, row)
	}
	return out, nil
}

// handleSheetFeatAbilityChoice resolves one pendingFeatAbilityChoiceRow —
// posted as feat_slug + ability, body fields rather than path segments
// since feat slugs contain slashes (same reasoning as handleSheetFeatAdd/
// handleSheetFeatDelete). A separate handler/route from
// handleSheetFeatureChoice, deliberately: this mechanism never touches
// internal/features' own choice-slot engine, so nothing here can regress it.
func (s *server) handleSheetFeatAbilityChoice(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	featSlug := strings.TrimSpace(r.FormValue("feat_slug"))
	ability := strings.ToLower(strings.TrimSpace(r.FormValue("ability")))
	if featSlug == "" || ability == "" {
		http.Error(w, "missing or invalid choice", http.StatusBadRequest)
		return
	}

	characterFeats, err := s.loadCharacterFeats(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load character feats for feat ability choice:", err)
		return
	}
	pending, err := s.buildPendingFeatAbilityChoiceRows(id, characterFeats)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending feat ability choice rows:", err)
		return
	}
	var options []featureChoiceOption
	found := false
	for _, row := range pending {
		if row.FeatSlug == featSlug {
			found = true
			options = row.Options
			break
		}
	}
	if !found {
		http.Error(w, "not a choice you currently qualify for", http.StatusBadRequest)
		return
	}
	validAbility := false
	for _, o := range options {
		if o.Value == ability {
			validAbility = true
			break
		}
	}
	if !validAbility {
		http.Error(w, "not a valid ability pick", http.StatusBadRequest)
		return
	}

	_, amount, _ := parseFeatAbilityIncrease(featSlug, descriptionFor(characterFeats, featSlug))
	if err := charstore.ApplyFeatAbilityBonus(s.charDB, id, featSlug, ability, amount); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("apply feat ability bonus (picker):", err)
		return
	}
	redirectToSheet(w, r, id)
}

// descriptionFor looks up one feat's rules text from an already-loaded
// characterFeats slice — avoids a second rules.db round trip in
// handleSheetFeatAbilityChoice, which already has the full list from its
// own eligibility check just above.
func descriptionFor(characterFeats []characterFeat, slug string) string {
	for _, f := range characterFeats {
		if f.Slug == slug {
			return f.Description
		}
	}
	return ""
}
