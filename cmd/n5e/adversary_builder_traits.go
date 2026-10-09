// Browsable reference page for the community "N5e Adversary Builder" trait
// catalog (internal/parse/adversary_builder_sheet.go, migration
// 0078_adversary_builder_sheet_traits.sql) — a GM's clan/affiliation/class-
// mod/role/passive flavor-trait catalog plus the small "Iconic" named-
// ability catalog, none of it wired into procedural generation (see that
// migration's own doc comment for why: unverified fan content, never
// auto-assigned to a generated adversary).
package main

import (
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/RationalRouter/n5e/internal/i18n"
)

type adversaryBuilderTraitRow struct {
	Slug        string
	CategoryTag string
	Name        string
	Rank        string
	Description string
	FilterKey   string // lowercase "name categorytag", precomputed for the client-side search box
}

type adversaryBuilderTraitGroup struct {
	Category string
	Label    string
	Traits   []adversaryBuilderTraitRow
}

// adversaryBuilderCategoryOrder fixes display order: Iconic first (the one
// subset with a real in-app connection, via the Classification feature),
// then the broader flavor-trait families roughly big-to-small.
var adversaryBuilderCategoryOrder = []string{"iconic", "clan", "role", "general", "affiliation", "passive", "class_mod"}

func (s *server) handleAdversaryBuilderTraits(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, category, category_tag, name, rank, description
		FROM adversary_builder_sheet_traits
		ORDER BY category, category_tag, name`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query adversary_builder_sheet_traits:", err)
		return
	}
	defer rows.Close()

	byCategory := map[string][]adversaryBuilderTraitRow{}
	var slugs []string
	for rows.Next() {
		var t adversaryBuilderTraitRow
		var category string
		if err := rows.Scan(&t.Slug, &category, &t.CategoryTag, &t.Name, &t.Rank, &t.Description); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan adversary_builder_sheet_traits:", err)
			return
		}
		byCategory[category] = append(byCategory[category], t)
		slugs = append(slugs, t.Slug)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("adversary_builder_sheet_traits rows:", err)
		return
	}

	locale := localeFromContext(r.Context())
	translations, err := i18n.Batch(s.rulesDB, locale, "adversary_builder_sheet_traits", slugs)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("translate adversary_builder_sheet_traits:", err)
		return
	}
	for category, traits := range byCategory {
		for i := range traits {
			tr := translations[traits[i].Slug]
			if v, ok := tr["name"]; ok {
				traits[i].Name = v
			}
			if v, ok := tr["description"]; ok {
				traits[i].Description = v
			}
		}
		byCategory[category] = traits
	}

	var groups []adversaryBuilderTraitGroup
	for _, cat := range adversaryBuilderCategoryOrder {
		traits := byCategory[cat]
		if len(traits) == 0 {
			continue
		}
		sort.Slice(traits, func(i, j int) bool { return sortKey(traits[i].Name) < sortKey(traits[j].Name) })
		for i := range traits {
			traits[i].FilterKey = strings.ToLower(traits[i].Name + " " + traits[i].CategoryTag)
		}
		groups = append(groups, adversaryBuilderTraitGroup{
			Category: cat,
			Label:    s.t(r, "adversary_traits.category_"+cat),
			Traits:   traits,
		})
	}

	s.render(w, r, "adversary_builder_traits.html", map[string]any{
		"Title":  s.t(r, "adversary_traits.title"),
		"Groups": groups,
	})
}
