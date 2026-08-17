package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// searchEntry is one row of the site-wide search index: enough to render a
// dropdown result, show a rollover preview, and link to it. Fetched once by
// the browser and filtered entirely client-side (see static/js/search.js) —
// names-only search over ~1800 rows is small enough that a per-keystroke
// server round trip would just add latency for no benefit.
type searchEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Preview string `json:"preview,omitempty"`
}

// searchSources lists every content type that should be findable, in the
// order results are built (and therefore roughly the order same-name
// matches appear in the dropdown). Adding a new browsable content type
// later just means adding one row here. descCol is whichever column holds
// that table's summary/flavor prose, used for the search-result rollover
// preview (see previewSnippet).
var searchSources = []struct {
	typeLabel string
	urlPrefix string
	query     string
}{
	{"Clan", "/clans/", "SELECT slug, name, COALESCE(overview, '') FROM clans"},
	{"Jutsu", "/jutsu/", "SELECT slug, name, COALESCE(description, '') FROM v_jutsu"},
	{"Class", "/classes/", "SELECT slug, name, COALESCE(description, '') FROM classes"},
	{"Feat", "/feats/", "SELECT slug, name, COALESCE(description, '') FROM feats"},
	{"Background", "/backgrounds/", "SELECT slug, name, COALESCE(description, '') FROM backgrounds"},
	{"Fighting Stance", "/stances/", "SELECT slug, name, COALESCE(description, '') FROM fighting_stances"},
	{"Enhancement Seal", "/seals/", "SELECT slug, name, COALESCE(description, '') FROM equipment WHERE kind = 'enhancement_seal'"},
	{"Property", "/properties/", "SELECT slug, name, description FROM weapon_properties"},
	{"Item", "/items/", "SELECT slug, name, COALESCE(description, '') FROM equipment WHERE kind IN ('weapon','armor','gear','tool','toolkit','scroll')"},
	{"Trap", "/traps/", "SELECT slug, name, COALESCE(description, '') FROM trap_templates"},
	{"Poison", "/poisons/", "SELECT slug, name, COALESCE(description, '') FROM poisons"},
}

// previewSnippet collapses whitespace (the PDF-extracted prose has no line
// breaks, so a raw substring can start/end mid-word-wrap oddly) and truncates
// to roughly max runes, cutting at the last word boundary so the rollover
// preview doesn't end mid-word.
func previewSnippet(raw string, max int) string {
	s := strings.Join(strings.Fields(raw), " ")
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndexByte(cut, ' '); i > max/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

func (s *server) handleSearchIndex(w http.ResponseWriter, r *http.Request) {
	var entries []searchEntry
	for _, src := range searchSources {
		rows, err := s.rulesDB.Query(src.query)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("query search index", src.typeLabel, ":", err)
			return
		}
		for rows.Next() {
			var slug, name, desc string
			if err := rows.Scan(&slug, &name, &desc); err != nil {
				rows.Close()
				http.Error(w, "database error", http.StatusInternalServerError)
				log.Println("scan search index", src.typeLabel, ":", err)
				return
			}
			// Every browsable type now has a real per-entity detail route and
			// links straight to its slug.
			link := src.urlPrefix + slug
			entries = append(entries, searchEntry{
				Name: name, Type: src.typeLabel, URL: link,
				Preview: previewSnippet(desc, 180),
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("search index rows", src.typeLabel, ":", err)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(entries); err != nil {
		log.Println("encode search index:", err)
	}
}
