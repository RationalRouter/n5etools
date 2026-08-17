package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"
)

// jutsuRankOrder is a small fixed enum (E..S never changes book to book),
// hardcoded the same way profKindLabels is in server.go rather than joined
// against jutsu_ranks on every request.
var jutsuRankOrder = map[string]int{"E": 0, "D": 1, "C": 2, "B": 3, "A": 4, "S": 5}

type jutsuListEntry struct {
	Slug           string
	Name           string
	Rank           sql.NullString
	Classification string
	CategoryGroup  sql.NullString
	CastingTime    string
	Range          string
	Duration       string
	SourceBook     string
	Keywords       string // printed keywords, e.g. "Fire Release, Ninjutsu" — see elemental_affinity.go's jutsuElement
	Preview        string // truncated description, for the tile's rollover preview
	Description    string // full raw description, for client-side description search

	// Filter-panel fields derived from the raw text columns above — see
	// jutsu_filters.go for the parsing/bucketing rules and why each one
	// is shaped the way it is.
	CastingAction string
	DurationLabel string
	DurationOrder int
	RangeFeet     int
	RangeNumeric  bool
	RangeSpecial  string
	Components    []string
}

type jutsuSubgroup struct {
	Label   string // category_group, blank when it's redundant with the classification heading above it
	Entries []jutsuListEntry
}

type jutsuGroup struct {
	Classification string
	Subgroups      []jutsuSubgroup
}

// jutsuSourceTile is one sourcebook filter tile — only books that actually
// have at least one jutsu (confirmed via a real query: only
// book/jutsu-compendium and book/clan-compendium ever appear in
// jutsu.source_book; book/core and book/class-compendium have zero, and
// showing them as filter tiles that always produce an empty list is
// confusing dead weight, not a harmless no-op).
type jutsuSourceTile struct {
	Slug  string
	Title string
}

func loadJutsuSourceTiles(rulesDB *sql.DB) ([]jutsuSourceTile, error) {
	rows, err := rulesDB.Query(`
		SELECT DISTINCT sb.slug, sb.title
		FROM source_books sb JOIN jutsu j ON j.source_book = sb.slug
		ORDER BY sb.title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tiles []jutsuSourceTile
	for rows.Next() {
		var t jutsuSourceTile
		if err := rows.Scan(&t.Slug, &t.Title); err != nil {
			return nil, err
		}
		tiles = append(tiles, t)
	}
	return tiles, rows.Err()
}

// otherClansJutsu returns every jutsu belonging to some clan's own list
// EXCEPT ownClanSlug — there is no rule anywhere in this game letting a
// character learn or benefit from a second clan's jutsu, so unlike class
// jutsu (where multiclassing is a real path to a class you don't have),
// another clan's jutsu should never appear in a character's own Jutsu
// Library at all, not just be filterable out via the origin checkboxes.
func otherClansJutsu(rulesDB *sql.DB, ownClanSlug string) (map[string]bool, error) {
	rows, err := rulesDB.Query(`SELECT DISTINCT jutsu_slug FROM clan_jutsu WHERE clan_slug != ?`, ownClanSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out[slug] = true
	}
	return out, rows.Err()
}

func loadJutsuList(rulesDB *sql.DB) ([]jutsuListEntry, error) {
	// Joined back to the raw jutsu table for source_book, which v_jutsu
	// doesn't project (it only carries columns a character-facing page
	// would ever need to override) — a small, scoped join rather than
	// widening the view for one filter-only field.
	rows, err := rulesDB.Query(`
		SELECT v.slug, v.name, v.rank, v.classification, v.category_group,
		       v.casting_time, v.range, v.duration, v.components, v.description, v.keywords,
		       COALESCE(j.source_book, '')
		FROM v_jutsu v JOIN jutsu j ON j.slug = v.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []jutsuListEntry
	for rows.Next() {
		var j jutsuListEntry
		var components string
		if err := rows.Scan(&j.Slug, &j.Name, &j.Rank, &j.Classification, &j.CategoryGroup,
			&j.CastingTime, &j.Range, &j.Duration, &components, &j.Description, &j.Keywords, &j.SourceBook); err != nil {
			return nil, err
		}
		j.Preview = previewSnippet(j.Description, 140)

		j.CastingAction = castingActionBucket(j.CastingTime)
		j.DurationLabel = durationBucket(j.Duration)
		j.DurationOrder = durationOrder[j.DurationLabel]
		rng := parseJutsuRange(j.Range)
		j.RangeFeet, j.RangeNumeric, j.RangeSpecial = rng.Feet, rng.Numeric, rng.Special
		j.Components = componentCodes(components)

		entries = append(entries, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, k int) bool {
		if entries[i].Classification != entries[k].Classification {
			return entries[i].Classification < entries[k].Classification
		}
		if entries[i].CategoryGroup.String != entries[k].CategoryGroup.String {
			return entries[i].CategoryGroup.String < entries[k].CategoryGroup.String
		}
		if ri, rk := jutsuRankOrder[entries[i].Rank.String], jutsuRankOrder[entries[k].Rank.String]; ri != rk {
			return ri < rk
		}
		return sortKey(entries[i].Name) < sortKey(entries[k].Name)
	})
	return entries, nil
}

// groupJutsu folds the flat, pre-sorted list into Classification ->
// category_group buckets. Relies on entries already being in bucket order,
// so this is a single linear pass, not a re-sort.
func groupJutsu(entries []jutsuListEntry) []jutsuGroup {
	var groups []jutsuGroup
	for _, e := range entries {
		if len(groups) == 0 || groups[len(groups)-1].Classification != e.Classification {
			groups = append(groups, jutsuGroup{Classification: e.Classification})
		}
		g := &groups[len(groups)-1]

		label := e.CategoryGroup.String
		if label == e.Classification {
			label = "" // e.g. "Bukijutsu" classification with category_group "Bukijutsu" too — no subheading needed
		}
		if len(g.Subgroups) == 0 || g.Subgroups[len(g.Subgroups)-1].Label != label {
			g.Subgroups = append(g.Subgroups, jutsuSubgroup{Label: label})
		}
		sg := &g.Subgroups[len(g.Subgroups)-1]
		sg.Entries = append(sg.Entries, e)
	}
	return groups
}

func (s *server) handleJutsuList(w http.ResponseWriter, r *http.Request) {
	entries, err := loadJutsuList(s.rulesDB)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load jutsu list:", err)
		return
	}
	tiles, err := loadJutsuSourceTiles(s.rulesDB)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load jutsu source tiles:", err)
		return
	}

	var selected *jutsuDetail
	if len(entries) > 0 {
		selected, err = loadJutsuDetail(s.rulesDB, entries[0].Slug)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load initial jutsu detail:", err)
			return
		}
	}

	s.render(w, "jutsu.html", map[string]any{
		"Title": "Jutsu", "Groups": groupJutsu(entries), "SourceTiles": tiles, "Selected": selected,
	})
}

type jutsuCastableClass struct {
	Slug string
	Name string
}

type jutsuDetail struct {
	Slug            string
	Name            string
	Classification  string
	Rank            sql.NullString
	CastingTime     string
	Range           string
	Duration        string
	Components      string
	CostText        string
	Keywords        string
	Description     string
	AtHigherRanks   sql.NullString
	CategoryGroup   sql.NullString
	ClanSlug        sql.NullString
	ClanName        sql.NullString
	CastableClasses []jutsuCastableClass
}

// loadJutsuDetail is shared by the standalone /jutsu/{slug} page, the
// two-pane view's initial server-rendered selection, and the AJAX
// fragment swap (see handleJutsuDetail) — one query/struct/template
// reused three ways, same pattern as items.go's loadItem.
func loadJutsuDetail(rulesDB *sql.DB, slug string) (*jutsuDetail, error) {
	var j jutsuDetail
	err := rulesDB.QueryRow(`
		SELECT j.slug, j.name, j.classification, j.rank, j.casting_time, j.range, j.duration,
		       j.components, j.cost_text, j.keywords, j.description, j.at_higher_ranks,
		       j.category_group, j.clan_slug, c.name
		FROM v_jutsu j LEFT JOIN clans c ON c.slug = j.clan_slug
		WHERE j.slug = ?`, slug,
	).Scan(&j.Slug, &j.Name, &j.Classification, &j.Rank, &j.CastingTime, &j.Range, &j.Duration,
		&j.Components, &j.CostText, &j.Keywords, &j.Description, &j.AtHigherRanks,
		&j.CategoryGroup, &j.ClanSlug, &j.ClanName)
	if err != nil {
		return nil, err
	}

	// Classes that CAN cast this jutsu's discipline, derived indirectly
	// via class_casting — not a literal "this class's spell list" (no such
	// table exists), so the template labels this as "Castable by:" rather
	// than implying a curated per-jutsu class list. classification is
	// matched as a substring since it's sometimes compound ("Hijutsu,
	// Bukijutsu") — LOWER(?) LIKE '%'||discipline||'%' is safe here since
	// discipline values are our own fixed enum with no wildcard characters.
	rows, err := rulesDB.Query(`
		SELECT DISTINCT c.slug, c.name
		FROM class_casting cc JOIN classes c ON c.slug = cc.class_slug
		WHERE LOWER(?) LIKE '%' || cc.discipline || '%'
		ORDER BY c.name`, j.Classification)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cc jutsuCastableClass
		if err := rows.Scan(&cc.Slug, &cc.Name); err != nil {
			return nil, err
		}
		j.CastableClasses = append(j.CastableClasses, cc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &j, nil
}

// handleJutsuDetail serves the standalone /jutsu/{slug} page and, with
// ?fragment=1, just the inner detail card for the two-pane view's AJAX
// swap — same content-negotiation-by-query-param pattern as items.go's
// handleItemDetail.
func (s *server) handleJutsuDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	j, err := loadJutsuDetail(s.rulesDB, slug)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query jutsu detail:", err)
		return
	}

	if r.URL.Query().Get("fragment") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl, ok := pageTemplates["jutsu.html"]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render jutsu fragment: no template registered")
			return
		}
		if err := tmpl.ExecuteTemplate(w, "jutsu_detail_card", j); err != nil {
			log.Println("render jutsu fragment:", err)
		}
		return
	}

	s.render(w, "jutsu_detail.html", map[string]any{"Title": j.Name, "Jutsu": j})
}
