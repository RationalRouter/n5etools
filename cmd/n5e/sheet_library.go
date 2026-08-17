package main

import (
	"database/sql"
	"sort"
	"strings"
)

// The reference libraries embedded in the character sheet's own tabs.
//
// The sheet integrates the standalone reference libraries — jutsu, feats,
// items, traps, and poisons — directly into its own tabs. Each pane shows a
// fully filterable list, the same filter UI already used by the standalone
// library pages, and supports dragging an entry from the right-hand list
// into the left-hand side to add it to the character's inventory/jutsu list
// or known feats.
//
// Three panes cover the categories: Jutsu, Feats, and one Inventory pane
// covering items + traps + poisons together.
//
// Every row is server-rendered up front and filtered client-side, exactly as
// /items, /jutsu and /feats already do — the whole catalogue is a few hundred
// kilobytes of markup for a local offline app, and it keeps the filters
// instant with no round trip. The jutsu pane reuses the /jutsu page's filter
// implementation wholesale (see jutsu-filter.js's factory), which is why its
// rows carry the same data- attributes.

// libraryEntry is one draggable row in a sheet library pane.
type libraryEntry struct {
	Slug string
	Name string
	// Kind is the label shown beside the name and the value the pane's
	// kind checkboxes filter on ("Weapons", "Poison", "Clan Feats", …).
	Kind string
	// Meta is a short right-aligned detail — a rank, a cost, a category —
	// so a row is identifiable without opening it.
	Meta string
	// Search is the extra text the pane's search box matches on beyond the
	// name (a description), so searching finds a feat by what it does.
	Search string
	// Owned marks a row the character already has, so the pane can dim it
	// rather than letting the player drag a duplicate.
	Owned bool
	// Href is the row's rules page. Carried per row rather than built
	// from a per-pane prefix because the inventory pane mixes three URL
	// spaces — /items, /poisons and /traps — in one list.
	Href string
	// Ineligible marks a row the character does not currently qualify for —
	// a feat whose prerequisites they fail (see feat_prereq.go). The pane
	// hides these by default and offers a "Show unavailable" toggle rather
	// than dropping them server-side, because a GM waiving a prerequisite is
	// normal play and the parse is a best effort over the book's prose.
	// Always false for panes with no prerequisites to check, i.e. items.
	Ineligible bool
	// WhyIneligible is the prerequisite clause that failed, for the row's
	// tooltip. Empty when the row is eligible.
	WhyIneligible string
	// WeaponCategory and ArmorCategory drive the inventory pane's
	// Simple/Martial/Exotic and Light/Medium/Heavy sub-filters — see
	// equipmentFilterCategory, which fills them. Exactly one is set on an
	// equipment row and both are empty everywhere else (feats, poisons,
	// traps), which is what keeps those rows out of the sub-filters' reach.
	WeaponCategory string
	ArmorCategory  string
	// Highlight marks a row that should read in the gold/blue split's blue
	// (see app.css's .subclass-scope doc comment) — set only for feats
	// whose category ties them to a specific Clan or Class (feats.category
	// "clan"/"clan, rare"/"class", see featIsClanOrClassCategory); always
	// false for panes with no such notion, i.e. items.
	Highlight bool
}

// featIsClanOrClassCategory reports whether a feat's raw category value
// (feats.category — "clan", "clan, rare", "class", "archetype", ...) ties
// it to a specific Clan or Class. Deliberately excludes "archetype" —
// featCategoryLabels' own doc comment already establishes Archetype as the
// book's own printed term, not this app's Subclass concept, and no feat is
// actually locked to one specific named subclass (confirmed against every
// feat's own prerequisites), so there is nothing "subclass" to highlight.
func featIsClanOrClassCategory(category string) bool {
	switch category {
	case "clan", "clan, rare", "class":
		return true
	default:
		return false
	}
}

// libraryGroup is one heading in a pane.
type libraryGroup struct {
	Label   string
	Entries []libraryEntry
}

// loadSheetItemLibrary builds the Inventory tab's right-hand pane: every
// carriable thing in the rules, grouped by kind.
//
// Poisons and traps are folded in beside equipment since items, traps, and
// poisons all belong to one inventory category. They live in their own
// tables with their own columns, so they are read separately and normalised
// into libraryEntry here — the same normalisation lookupCarriedItem does on
// the way back out.
func (s *server) loadSheetItemLibrary(owned map[string]bool) ([]libraryGroup, error) {
	byKind := map[string][]libraryEntry{}

	rows, err := s.rulesDB.Query(`
		SELECT slug, name, kind, cost_ryo, bulk, COALESCE(description, ''),
		       COALESCE(weapon_category, ''), COALESCE(armor_category, '')
		FROM v_equipment ORDER BY name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var slug, name, kind, description, weaponCategory, armorCategory string
		var cost, bulk sql.NullFloat64
		if err := rows.Scan(&slug, &name, &kind, &cost, &bulk, &description,
			&weaponCategory, &armorCategory); err != nil {
			rows.Close()
			return nil, err
		}
		label := equipmentKindLabels[kind]
		if label == "" {
			label = kind
		}
		byKind[label] = append(byKind[label], libraryEntry{
			Slug: slug, Name: name, Kind: label,
			Meta:   itemMetaLabel(cost, bulk),
			Search: description,
			Owned:  owned[slug],
			Href:   "/items/" + slug,

			WeaponCategory: equipmentFilterCategory(kind, "weapon", weaponCategory),
			ArmorCategory:  equipmentFilterCategory(kind, "armor", armorCategory),
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	poisonRows, err := s.rulesDB.Query(`
		SELECT slug, name, COALESCE(poison_rank, ''), cost_ryo, bulk, COALESCE(description, '')
		FROM poisons ORDER BY name`)
	if err != nil {
		return nil, err
	}
	for poisonRows.Next() {
		var slug, name, rank, description string
		var cost, bulk sql.NullFloat64
		if err := poisonRows.Scan(&slug, &name, &rank, &cost, &bulk, &description); err != nil {
			poisonRows.Close()
			return nil, err
		}
		meta := itemMetaLabel(cost, bulk)
		if rank != "" {
			meta = rank + "-Rank · " + meta
		}
		byKind["Poisons"] = append(byKind["Poisons"], libraryEntry{
			Slug: slug, Name: name, Kind: "Poisons", Meta: meta,
			Search: description, Owned: owned[slug], Href: "/poisons/" + slug,
		})
	}
	poisonRows.Close()
	if err := poisonRows.Err(); err != nil {
		return nil, err
	}

	trapRows, err := s.rulesDB.Query(`
		SELECT slug, name, COALESCE(toolkit_required, ''), COALESCE(description, '')
		FROM trap_templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	for trapRows.Next() {
		var slug, name, toolkit, description string
		if err := trapRows.Scan(&slug, &name, &toolkit, &description); err != nil {
			trapRows.Close()
			return nil, err
		}
		byKind["Traps"] = append(byKind["Traps"], libraryEntry{
			Slug: slug, Name: name, Kind: "Traps", Meta: toolkit,
			Search: description, Owned: owned[slug], Href: "/traps/" + slug,
		})
	}
	trapRows.Close()
	if err := trapRows.Err(); err != nil {
		return nil, err
	}

	// Weapons and armor first for the same reason the add-item dropdown
	// puts them there — they're the ones that change a character's numbers
	// — then the rest of the equipment kinds in their established order,
	// then the two new categories.
	order := append(append([]string{}, equipmentKindOrder...), "poison", "trap")
	labelFor := func(kind string) string {
		switch kind {
		case "poison":
			return "Poisons"
		case "trap":
			return "Traps"
		}
		if label := equipmentKindLabels[kind]; label != "" {
			return label
		}
		return kind
	}
	var groups []libraryGroup
	seen := map[string]bool{}
	for _, kind := range order {
		label := labelFor(kind)
		if len(byKind[label]) == 0 || seen[label] {
			continue
		}
		seen[label] = true
		groups = append(groups, libraryGroup{Label: label, Entries: byKind[label]})
	}
	// Any kind a future rules update introduces still appears rather than
	// silently vanishing from the pane.
	var extra []string
	for label := range byKind {
		if !seen[label] {
			extra = append(extra, label)
		}
	}
	sort.Strings(extra)
	for _, label := range extra {
		groups = append(groups, libraryGroup{Label: label, Entries: byKind[label]})
	}
	return groups, nil
}

// itemMetaLabel renders the one-line "cost · bulk" summary shown on a library
// row. Either half is omitted when the rules don't record it, rather than
// printed as a zero.
func itemMetaLabel(cost, bulk sql.NullFloat64) string {
	var parts []string
	if cost.Valid {
		parts = append(parts, comma(cost.Float64)+" Ryo")
	}
	if bulk.Valid {
		parts = append(parts, "Bulk "+comma(bulk.Float64))
	}
	return strings.Join(parts, " · ")
}

// loadSheetFeatLibrary builds the Feats tab's right-hand pane, grouped by the
// same categories /feats uses.
func (s *server) loadSheetFeatLibrary(owned map[string]bool, character featCharacter) ([]libraryGroup, error) {
	knownClasses, err := s.knownClassNames()
	if err != nil {
		return nil, err
	}
	knownClans, err := s.knownClanNames()
	if err != nil {
		return nil, err
	}

	rows, err := s.rulesDB.Query(`
		SELECT slug, name, category, COALESCE(prerequisites, ''), COALESCE(description, '')
		FROM feats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type featRow struct {
		slug, name, category, prerequisites, description string
	}
	var featRows []featRow
	// Feat names are needed as a set before any prerequisite can be parsed:
	// a clause like "Medical Expert" is only a requirement if a feat by that
	// name exists, and that is what tells it apart from the prose the parser
	// has to ignore. So the rows are collected first and judged second.
	knownFeats := map[string]bool{}
	for rows.Next() {
		var f featRow
		if err := rows.Scan(&f.slug, &f.name, &f.category, &f.prerequisites, &f.description); err != nil {
			return nil, err
		}
		featRows = append(featRows, f)
		knownFeats[strings.ToLower(f.name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	byCategory := map[string][]libraryEntry{}
	for _, f := range featRows {
		label := featCategoryLabel(f.category)
		entry := libraryEntry{
			Slug: f.slug, Name: f.name, Kind: label,
			Meta:      f.prerequisites,
			Search:    f.prerequisites + " " + f.description,
			Owned:     owned[f.slug],
			Href:      "/feats/" + f.slug,
			Highlight: featIsClanOrClassCategory(f.category),
		}
		if f.prerequisites != "" {
			if !character.Meets(parseFeatPrereq(f.prerequisites, character.AllSkills, knownFeats, knownClasses, knownClans)) {
				entry.Ineligible = true
				entry.WhyIneligible = f.prerequisites
			}
		}
		byCategory[label] = append(byCategory[label], entry)
	}

	labels := make([]string, 0, len(byCategory))
	for label := range byCategory {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	groups := make([]libraryGroup, 0, len(labels))
	for _, label := range labels {
		entries := byCategory[label]
		sort.Slice(entries, func(i, k int) bool { return sortKey(entries[i].Name) < sortKey(entries[k].Name) })
		groups = append(groups, libraryGroup{Label: label, Entries: entries})
	}
	return groups, nil
}

// characterFeat is one feat the character has taken, with its rules text so
// the sheet can show what it actually does rather than just its name.
type characterFeat struct {
	Slug          string
	Name          string
	Category      string
	Prerequisites string
	Description   string
	// ChosenAtLevel is the character's level when the feat was taken, which
	// is what puts it in the right place in the Core tab's level-ordered
	// Features & Traits list. NULL on rows written before the sheet started
	// recording it, and those simply list without a level clause.
	ChosenAtLevel sql.NullInt64
}

// loadCharacterFeats reads the character's taken feats and joins each back to
// its rules entry. A slug with no matching feat (a rules update that removed
// or renamed one) still lists, showing the slug — the same graceful degrade
// the inventory uses, so a character never silently loses something they
// chose.
func (s *server) loadCharacterFeats(characterID int64) ([]characterFeat, error) {
	rows, err := s.charDB.Query(
		`SELECT feat_slug, chosen_at_level FROM character_feats WHERE character_id = ?`, characterID)
	if err != nil {
		return nil, err
	}
	var taken []characterFeat
	for rows.Next() {
		var f characterFeat
		if err := rows.Scan(&f.Slug, &f.ChosenAtLevel); err != nil {
			rows.Close()
			return nil, err
		}
		taken = append(taken, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]characterFeat, 0, len(taken))
	for _, row := range taken {
		slug := row.Slug
		feat := characterFeat{Slug: slug, Name: slug, ChosenAtLevel: row.ChosenAtLevel}
		var category, prerequisites, description sql.NullString
		err := s.rulesDB.QueryRow(
			`SELECT name, category, prerequisites, description FROM feats WHERE slug = ?`, slug,
		).Scan(&feat.Name, &category, &prerequisites, &description)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		feat.Category = featCategoryLabel(category.String)
		feat.Prerequisites = prerequisites.String
		feat.Description = description.String
		out = append(out, feat)
	}
	sort.Slice(out, func(i, k int) bool { return sortKey(out[i].Name) < sortKey(out[k].Name) })
	return out, nil
}
