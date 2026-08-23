package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

// pendingPackToolkitChoiceRow is one still-open toolkit pick left over from
// unpacking a pack — Crafter's Pack ("3 Toolkits (pick three)"), Captain's
// Pack ("1 Toolkit (pick one)"), and Infiltrator's Pack ("1 Hackers Kit or
// Security Kit (pick one)") are the only three packs that currently grant
// one (see starting_equipment.go's parsePackContents), but this is driven
// by the placeholder row's own notes tag, not a pack slug list, so a rules
// update adding a sixth pack with the same shape needs nothing here either.
//
// RowID identifies the character_inventory placeholder row this resolves
// (handleSheetPackToolkitChoice re-validates it against a freshly built
// list of these rows before trusting it, same as every other pending-choice
// handler in this codebase).
type pendingPackToolkitChoiceRow struct {
	RowID   int64
	Label   string
	Options []featureChoiceOption
}

// buildPendingPackToolkitChoiceRows finds every character_inventory row
// still carrying charstore.PackToolkitChoiceNotesTag — i.e. every toolkit
// pick a pack has granted that the player hasn't resolved yet. Unlike the
// feat skill-or-tool choice's "resolved" check (buildPendingFeatSkillOrToolChoiceRows,
// which has to look for a matching character_proficiencies row elsewhere),
// this needs no separate resolved-state query: ResolvePackToolkitChoice
// deletes or decrements the placeholder row itself, so a row's mere
// existence here already means "still pending".
func (s *server) buildPendingPackToolkitChoiceRows(characterID int64) ([]pendingPackToolkitChoiceRow, error) {
	rows, err := s.charDB.Query(`
		SELECT id, quantity, notes FROM character_inventory
		WHERE character_id = ? AND notes LIKE ? || '%'
		ORDER BY id`, characterID, charstore.PackToolkitChoiceNotesTag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type placeholder struct {
		id       int64
		quantity int
		options  string
		ok       bool
	}
	var placeholders []placeholder
	for rows.Next() {
		var p placeholder
		var notes sql.NullString
		if err := rows.Scan(&p.id, &p.quantity, &notes); err != nil {
			return nil, err
		}
		if !notes.Valid {
			continue
		}
		p.options, p.ok = charstore.DecodePackToolkitChoiceNotes(notes.String)
		if !p.ok {
			continue
		}
		placeholders = append(placeholders, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(placeholders) == 0 {
		return nil, nil
	}

	toolkits, err := s.loadToolkitOptions()
	if err != nil {
		return nil, err
	}
	toolkitByName := make(map[string]toolkitOption, len(toolkits))
	for _, t := range toolkits {
		toolkitByName[normalizeItemName(t.Name)] = t
	}
	// Every toolkit the character already has proficiency with, from ANY
	// source — same exclusion the multiclass Toolkit picker and the
	// equipment step's own kit-choice dropdowns already apply (see
	// excludeHeldOptions' doc), so a pack pick can't hand out a second copy
	// of a toolkit proficiency the player already holds.
	heldTools, err := s.characterProficiencyValues(characterID, "tool", "", "")
	if err != nil {
		return nil, err
	}

	var out []pendingPackToolkitChoiceRow
	for _, p := range placeholders {
		var candidates []toolkitOption
		var scopedNames []string
		if p.options == "" {
			candidates = toolkits
		} else {
			scopedNames = strings.Split(p.options, "|")
			for _, name := range scopedNames {
				if t, ok := toolkitByName[normalizeItemName(name)]; ok {
					candidates = append(candidates, t)
				}
			}
		}
		filtered := excludeHeldOptions(candidates, heldTools)
		if len(filtered) == 0 {
			// Every candidate is already held elsewhere (only realistically
			// reachable for a scoped 2-option pick) — offer the full
			// candidate set anyway rather than leaving the player stuck
			// with a picker that has nothing to pick from; the server-side
			// "already held" check on submit still applies.
			filtered = candidates
		}

		label := "Pick a Toolkit"
		if len(scopedNames) > 0 {
			label = "Pick " + strings.Join(scopedNames, " or ")
		}
		if p.quantity > 1 {
			label += fmt.Sprintf(" (%d remaining)", p.quantity)
		}

		options := make([]featureChoiceOption, 0, len(filtered))
		for _, t := range filtered {
			options = append(options, featureChoiceOption{Value: t.Slug, Label: t.Name, Description: t.Description})
		}
		out = append(out, pendingPackToolkitChoiceRow{RowID: p.id, Label: label, Options: options})
	}
	return out, nil
}

// handleSheetPackToolkitChoice resolves one pendingPackToolkitChoiceRow —
// posted as row_id + toolkit_slug (one of that row's own Option.Value
// strings). Body fields rather than a path segment, same reasoning as
// handleSheetFeatSkillOrToolChoice.
func (s *server) handleSheetPackToolkitChoice(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	rowID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("row_id")), 10, 64)
	if err != nil {
		http.Error(w, "missing or invalid choice", http.StatusBadRequest)
		return
	}
	toolkitSlug := strings.TrimSpace(r.FormValue("toolkit_slug"))
	if toolkitSlug == "" {
		http.Error(w, "missing or invalid choice", http.StatusBadRequest)
		return
	}

	pending, err := s.buildPendingPackToolkitChoiceRows(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("build pending pack toolkit choice rows:", err)
		return
	}
	var options []featureChoiceOption
	found := false
	for _, row := range pending {
		if row.RowID == rowID {
			found = true
			options = row.Options
			break
		}
	}
	if !found {
		http.Error(w, "not a choice you currently qualify for", http.StatusBadRequest)
		return
	}
	var toolkitName string
	valid := false
	for _, o := range options {
		if o.Value == toolkitSlug {
			valid = true
			toolkitName = o.Label
			break
		}
	}
	if !valid {
		http.Error(w, "not a valid pick", http.StatusBadRequest)
		return
	}

	if err := charstore.ResolvePackToolkitChoice(s.charDB, id, rowID, toolkitSlug, toolkitName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("resolve pack toolkit choice:", err)
		return
	}
	redirectToSheet(w, r, id)
}
