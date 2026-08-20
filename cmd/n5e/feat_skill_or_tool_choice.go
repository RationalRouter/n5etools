package main

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

// featSkillOrToolChoiceRe matches the recurring "You gain proficiency
// in/with [a/an/the] X or [a/an/the] Y (Pick one)." clause — a genuine
// player choice between two named proficiencies, at least one of which is
// always a real skill in the live corpus (feat/burglar: Security Kit or
// Sleight of Hand; feat/chef: Cooking Kit or Survival; feat/chemist:
// Alchemist kit or Investigation; feat/investigator: Investigation or
// Forensics Kit — verified against dist/rules.db). Deliberately distinct
// from featSkillProficiencyRe (feat_skill_proficiency.go): that regex's
// non-greedy single-capture-to-period shape already fails closed on this
// exact clause (the "or"/parenthetical breaks it before any terminator), so
// the two parsers are mutually exclusive by construction — a feat's
// description is never read by both.
var featSkillOrToolChoiceRe = regexp.MustCompile(
	`(?i)you gain proficiency (?:in|with) (?:a |an |the )?([A-Za-z ]+?) or (?:a |an |the )?([A-Za-z ]+?)\.?\s*\(pick one\)`,
)

// featSkillOrToolChoiceExcluded lists feats that match
// featSkillOrToolChoiceRe's loose shape but aren't a genuine two-option
// choice. feat/crafter's own clause is "proficiency in Crafting and either
// Weaponsmiths or Armorsmith Kit (Pick one)" — an unconditional Crafting
// grant (handled separately by featSkillProficiencyOverrides in
// feat_skill_proficiency.go) followed by a kit-vs-kit choice with no skill
// on either side. This mechanism is built for a skill-vs-kit choice and has
// no clean way to present that: the regex's own non-greedy capture reads
// "Crafting and either Weaponsmiths" as one nonsense first option, not
// "Crafting" and "Weaponsmiths" separately. Left undone here rather than
// surfacing that fragment as a player-facing pick — the Weaponsmith-vs-
// Armorsmith kit choice itself stays a documented Group 2/3 gap (a
// tool-only proficiency choice, the same standing boundary every other
// tool/kit clause in this codebase respects).
var featSkillOrToolChoiceExcluded = map[string]bool{
	"feat/crafter": true,
}

// featChoiceCandidate is one side of a parsed skill-or-tool choice. Kind is
// "skill" (resolved against skillNameLookup, feat_skill_proficiency.go) or
// "tool" (the captured text, title-cased, taken as-is — this app doesn't
// validate free-text tool/kit proficiencies against a fixed catalog
// anywhere else either; see charstore.AddCustomProficiency).
type featChoiceCandidate struct {
	Kind string
	Name string
}

// parseFeatSkillOrToolChoice extracts a feat's two-option "Kit or Skill
// (Pick one)" clause, if it has one. At least one side must resolve to a
// real skill (skillNameLookup) — a hypothetical tool-vs-tool "X or Y (Pick
// one)" clause is out of scope for this mechanism (nothing in the live
// corpus is shaped that way once feat/crafter is excluded above) and is
// left unparsed rather than surfaced as a picker with no skill-writer path
// on either side.
func parseFeatSkillOrToolChoice(slug, description string) ([2]featChoiceCandidate, bool) {
	var zero [2]featChoiceCandidate
	if featSkillOrToolChoiceExcluded[slug] {
		return zero, false
	}
	m := featSkillOrToolChoiceRe.FindStringSubmatch(description)
	if m == nil {
		return zero, false
	}
	a := resolveChoiceCandidate(m[1])
	b := resolveChoiceCandidate(m[2])
	if a.Kind != "skill" && b.Kind != "skill" {
		return zero, false
	}
	return [2]featChoiceCandidate{a, b}, true
}

// resolveChoiceCandidate classifies one captured option as a real skill
// (canonical charsheet.SkillAbility spelling) or, failing that, a tool/kit
// name taken from the feat's own text.
func resolveChoiceCandidate(raw string) featChoiceCandidate {
	trimmed := strings.TrimSpace(raw)
	if canonical, found := skillNameLookup[strings.ToLower(trimmed)]; found {
		return featChoiceCandidate{Kind: "skill", Name: canonical}
	}
	return featChoiceCandidate{Kind: "tool", Name: titleCaseWords(trimmed)}
}

// titleCaseWords capitalizes the first letter of each word, leaving the
// rest alone — normalizes casing drift in the book's own prose (feat/
// chemist writes "Alchemist kit", feat/alchemist writes "Alchemist Kit" for
// the same real item) without botching a name this function has never seen.
func titleCaseWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// pendingFeatSkillOrToolChoiceRow is one taken feat whose skill-or-tool
// clause hasn't been resolved yet — same shape as pendingFeatAbilityChoiceRow
// (feat_ability_bonus.go), rendered from its own list for the same
// "isolated from internal/features' choice-slot engine" reasoning
// documented there. Each Option's Value is "skill:Name" or "tool:Name" —
// encodes which writer handleSheetFeatSkillOrToolChoice should use for that
// pick without a second form field.
type pendingFeatSkillOrToolChoiceRow struct {
	FeatSlug string
	Label    string
	Options  []featureChoiceOption
}

// buildPendingFeatSkillOrToolChoiceRows finds every taken feat with an
// unresolved skill-or-tool choice clause. A feat counts as resolved once a
// character_proficiencies row of kind 'skill' or 'tool' is tagged
// source_kind='feat', source_ref=<its slug> — written by whichever branch
// handleSheetFeatSkillOrToolChoice took — same "check the row that would
// exist" pattern buildPendingFeatAbilityChoiceRows (feat_ability_bonus.go)
// uses against character_ability_bonuses. Restricted to kind IN
// ('skill','tool') rather than every source_kind='feat' row for this slug:
// a feat that also grants a fixed weapon-category proficiency (kind=
// 'weapon', see feat_weapon_proficiency.go) writes its own row under the
// same source_ref, which must not be mistaken for this picker having
// already been resolved.
func (s *server) buildPendingFeatSkillOrToolChoiceRows(characterID int64, characterFeats []characterFeat) ([]pendingFeatSkillOrToolChoiceRow, error) {
	resolved := make(map[string]bool)
	rows, err := s.charDB.Query(
		`SELECT DISTINCT source_ref FROM character_proficiencies
		 WHERE character_id = ? AND source_kind = 'feat' AND kind IN ('skill', 'tool')`,
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

	var out []pendingFeatSkillOrToolChoiceRow
	for _, feat := range characterFeats {
		if resolved[feat.Slug] {
			continue
		}
		options, ok := parseFeatSkillOrToolChoice(feat.Slug, feat.Description)
		if !ok {
			continue
		}
		row := pendingFeatSkillOrToolChoiceRow{FeatSlug: feat.Slug, Label: feat.Name}
		for _, opt := range options {
			row.Options = append(row.Options, featureChoiceOption{
				Value: opt.Kind + ":" + opt.Name,
				Label: opt.Name,
				Description: fmt.Sprintf(
					"Gain proficiency %s the %s. If you're already proficient, this instead adds a rank of Mastery.",
					choicePreposition(opt.Kind), opt.Name),
			})
		}
		out = append(out, row)
	}
	return out, nil
}

// choicePreposition matches the book's own wording for each proficiency
// kind ("proficiency in a skill", "proficiency with a kit") in the option
// tooltip text above.
func choicePreposition(kind string) string {
	if kind == "skill" {
		return "in"
	}
	return "with"
}

// handleSheetFeatSkillOrToolChoice resolves one
// pendingFeatSkillOrToolChoiceRow — posted as feat_slug + choice (one of
// that row's own Option.Value strings), body fields rather than path
// segments since feat slugs contain slashes (same reasoning as
// handleSheetFeatAdd/handleSheetFeatAbilityChoice).
func (s *server) handleSheetFeatSkillOrToolChoice(w http.ResponseWriter, r *http.Request) {
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
	choice := strings.TrimSpace(r.FormValue("choice"))
	if featSlug == "" || choice == "" {
		http.Error(w, "missing or invalid choice", http.StatusBadRequest)
		return
	}

	characterFeats, err := s.loadCharacterFeats(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load character feats for feat skill-or-tool choice:", err)
		return
	}
	pending, err := s.buildPendingFeatSkillOrToolChoiceRows(id, characterFeats)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending feat skill-or-tool choice rows:", err)
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
	valid := false
	for _, o := range options {
		if o.Value == choice {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}

	kind, value, _ := strings.Cut(choice, ":")
	if err := s.applyFeatProficiencyGrant(id, featSlug, kind, value, descriptionFor(characterFeats, featSlug)); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("apply feat skill-or-tool choice:", err)
		return
	}
	redirectToSheet(w, r, id)
}
