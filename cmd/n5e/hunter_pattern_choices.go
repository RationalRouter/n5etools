package main

import (
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// kleptomaniacOptionSlug and habitualResearcherOptionSlug identify the two
// Hunters Patterns (class_options, list_name "Hunters Patterns" — a mid-tier
// optional pick catalog, not a class_features row) whose own text is a
// proficiency choice rather than a fixed grant: "You gain proficiency in
// Sleight of Hand or Security Kits" (Kleptomaniac) and "Select two skills.
// You gain proficiency in the given skills" (Habitual Researcher).
// Kleptomaniac's own "mark a lock as your Primary Target" clause is
// action-economy/DC text with no field to attach to and stays narrated, the
// same boundary gaseousHazeFeatureSlug's own DC half draws (internal/
// charsheet/charsheet.go). Habitual Researcher's "You can select this
// Pattern multiple times, selecting different skills" repeat-pick clause
// stays narrated too: character_hunter_nin_picks has a UNIQUE(character_id,
// category, option_slug) constraint (internal/charstore/hunter_nin_picks.go),
// so the same catalog slug can only ever be picked once — supporting true
// repetition would mean reworking that shared table's key shape across
// every Hunter-Nin pick category, not just this one Pattern, for a single
// Pattern's own repeat clause.
const (
	kleptomaniacOptionSlug       = "class/hunter-nin/option/hunters-patterns/kleptomaniac"
	habitualResearcherOptionSlug = "class/hunter-nin/option/hunters-patterns/habitual-researcher"
)

// pendingHunterPatternChoiceField is one <select> in a pending Hunters
// Pattern choice row. Kleptomaniac's row has one field (skill vs. tool);
// Habitual Researcher's has two (one per skill it grants) sharing the same
// full skill-name option list.
type pendingHunterPatternChoiceField struct {
	Name    string // POST form field name: "choice", or "choice_1"/"choice_2"
	Options []featureChoiceOption
}

// pendingHunterPatternChoiceRow is one taken Hunters Pattern whose own
// proficiency-choice clause hasn't been resolved yet — same "Pending
// Choices" banner shape as pendingFeatSkillOrToolChoiceRow
// (feat_skill_or_tool_choice.go), generalized to more than one <select>
// since Habitual Researcher needs two.
type pendingHunterPatternChoiceRow struct {
	PatternSlug string
	Label       string
	Fields      []pendingHunterPatternChoiceField
}

// kleptomaniacChoiceOptions is Kleptomaniac's own fixed two-option choice —
// hardcoded rather than parsed from its rules-text the way
// featSkillOrToolChoiceRe parses feats, since there is exactly one Hunters
// Pattern shaped this way (no reuse benefit to a general parser here).
func kleptomaniacChoiceOptions() []featureChoiceOption {
	return []featureChoiceOption{
		{Value: "skill:Sleight of Hand", Label: "Sleight of Hand", Description: "Gain proficiency in Sleight of Hand."},
		{Value: "tool:Security Kits", Label: "Security Kits", Description: "Gain proficiency with Security Kits."},
	}
}

// sortedSkillChoiceOptions is every skill the rules know about, alphabetical
// — same source and ordering as buildPendingFeatureChoiceRows' own
// skillNames list (feature_choices.go), duplicated here rather than shared
// since that function builds a plain []string while this needs
// []featureChoiceOption with a "skill:" — prefixed Value each caller here
// resolves the same way Kleptomaniac's tool option does.
func sortedSkillChoiceOptions() []featureChoiceOption {
	names := make([]string, 0, len(charsheet.SkillAbility))
	for name := range charsheet.SkillAbility {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]featureChoiceOption, 0, len(names))
	for _, name := range names {
		out = append(out, featureChoiceOption{Value: "skill:" + name, Label: name})
	}
	return out
}

// buildPendingHunterPatternChoiceRows finds every taken Hunters Pattern with
// an unresolved proficiency-choice clause. A pattern counts as resolved once
// a character_proficiencies row is tagged source_kind='hunter_pattern',
// source_ref=<its slug> — written by handleSheetHunterPatternChoice, same
// "check the row that would exist" pattern
// buildPendingFeatSkillOrToolChoiceRows uses against source_kind='feat'.
func (s *server) buildPendingHunterPatternChoiceRows(characterID int64) ([]pendingHunterPatternChoiceRow, error) {
	picks, err := charstore.ListHunterNinPicks(s.charDB, characterID, charstore.HunterPickPattern)
	if err != nil {
		return nil, err
	}

	resolved := make(map[string]bool)
	rows, err := s.charDB.Query(
		`SELECT DISTINCT source_ref FROM character_proficiencies WHERE character_id = ? AND source_kind = 'hunter_pattern'`,
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

	var out []pendingHunterPatternChoiceRow
	for _, slug := range picks {
		if resolved[slug] {
			continue
		}
		switch slug {
		case kleptomaniacOptionSlug:
			out = append(out, pendingHunterPatternChoiceRow{
				PatternSlug: slug, Label: "Kleptomaniac",
				Fields: []pendingHunterPatternChoiceField{{Name: "choice", Options: kleptomaniacChoiceOptions()}},
			})
		case habitualResearcherOptionSlug:
			skillOptions := sortedSkillChoiceOptions()
			out = append(out, pendingHunterPatternChoiceRow{
				PatternSlug: slug, Label: "Habitual Researcher",
				Fields: []pendingHunterPatternChoiceField{
					{Name: "choice_1", Options: skillOptions},
					{Name: "choice_2", Options: skillOptions},
				},
			})
		}
	}
	return out, nil
}

// handleSheetHunterPatternChoice resolves one pendingHunterPatternChoiceRow
// — posted as pattern_slug + one form value per that row's own Fields, body
// fields rather than path segments since pattern slugs contain slashes (same
// reasoning as handleSheetFeatSkillOrToolChoice). Every submitted field must
// resolve to a distinct option: rejecting a repeat catches Habitual
// Researcher's "two skills" clause picking the same skill twice for both of
// its own fields.
func (s *server) handleSheetHunterPatternChoice(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	patternSlug := strings.TrimSpace(r.FormValue("pattern_slug"))
	if patternSlug == "" {
		http.Error(w, "missing or invalid choice", http.StatusBadRequest)
		return
	}

	pending, err := s.buildPendingHunterPatternChoiceRows(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending hunter pattern choice rows:", err)
		return
	}
	var row *pendingHunterPatternChoiceRow
	for i := range pending {
		if pending[i].PatternSlug == patternSlug {
			row = &pending[i]
			break
		}
	}
	if row == nil {
		http.Error(w, "not a choice you currently qualify for", http.StatusBadRequest)
		return
	}

	profs := make([]charstore.HunterPatternProficiency, 0, len(row.Fields))
	seen := make(map[string]bool, len(row.Fields))
	for _, field := range row.Fields {
		value := strings.TrimSpace(r.FormValue(field.Name))
		valid := false
		for _, o := range field.Options {
			if o.Value == value {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "not a valid pick", http.StatusBadRequest)
			return
		}
		if seen[value] {
			http.Error(w, "pick two different skills", http.StatusBadRequest)
			return
		}
		seen[value] = true
		kind, name, _ := strings.Cut(value, ":")
		profs = append(profs, charstore.HunterPatternProficiency{Kind: kind, Value: name})
	}

	if err := charstore.ApplyHunterPatternProficiencies(s.charDB, id, patternSlug, profs); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("apply hunter pattern choice:", err)
		return
	}
	redirectToSheet(w, r, id)
}
