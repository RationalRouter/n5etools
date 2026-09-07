package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"

	"github.com/sergio/n5e/internal/i18n"
)

type poisonRow struct {
	Slug        string
	Name        string
	PoisonRank  sql.NullString
	CraftDC     sql.NullInt64
	Uses        sql.NullInt64
	Bulk        sql.NullFloat64
	CostRyo     sql.NullFloat64
	Description string
}

const poisonSelectColumns = `slug, name, poison_rank, craft_dc, uses, bulk, cost_ryo, description`

func scanPoisonRow(scan func(dest ...any) error) (poisonRow, error) {
	var p poisonRow
	err := scan(&p.Slug, &p.Name, &p.PoisonRank, &p.CraftDC, &p.Uses, &p.Bulk, &p.CostRyo, &p.Description)
	return p, err
}

func overlayPoisonTranslation(p *poisonRow, tr map[string]string) {
	if v, ok := tr["name"]; ok {
		p.Name = v
	}
	if v, ok := tr["description"]; ok {
		p.Description = v
	}
}

// poisonRankOrder mirrors jutsuRankOrder (jutsu.go) — the same fixed D..S
// enum, just missing E-Rank since no poison in the book is that weak.
var poisonRankOrder = map[string]int{"D": 0, "C": 1, "B": 2, "A": 3, "S": 4}

type poisonGroup struct {
	Rank    string
	Poisons []poisonRow
}

func (s *server) handlePoisons(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`SELECT ` + poisonSelectColumns + ` FROM poisons`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query poisons:", err)
		return
	}
	defer rows.Close()

	var poisons []poisonRow
	for rows.Next() {
		p, err := scanPoisonRow(rows.Scan)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan poison:", err)
			return
		}
		poisons = append(poisons, p)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("poisons rows:", err)
		return
	}

	locale := localeFromContext(r.Context())
	slugs := make([]string, len(poisons))
	for i, p := range poisons {
		slugs[i] = p.Slug
	}
	translations, err := i18n.Batch(s.rulesDB, locale, "poisons", slugs)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("translate poisons:", err)
		return
	}
	for i := range poisons {
		overlayPoisonTranslation(&poisons[i], translations[poisons[i].Slug])
	}

	sort.Slice(poisons, func(i, j int) bool {
		ri, rj := poisonRankOrder[poisons[i].PoisonRank.String], poisonRankOrder[poisons[j].PoisonRank.String]
		if ri != rj {
			return ri < rj
		}
		return sortKey(poisons[i].Name) < sortKey(poisons[j].Name)
	})

	var groups []poisonGroup
	for _, p := range poisons {
		rank := p.PoisonRank.String
		if len(groups) == 0 || groups[len(groups)-1].Rank != rank {
			groups = append(groups, poisonGroup{Rank: rank})
		}
		g := &groups[len(groups)-1]
		g.Poisons = append(g.Poisons, p)
	}

	s.render(w, r, "poisons.html", map[string]any{"Title": s.t(r, "poisons.title"), "Groups": groups})
}

func (s *server) handlePoisonDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	row := s.rulesDB.QueryRow(`SELECT `+poisonSelectColumns+` FROM poisons WHERE slug = ?`, slug)
	p, err := scanPoisonRow(row.Scan)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query poison detail:", err)
		return
	}
	if tr, err := i18n.Row(s.rulesDB, localeFromContext(r.Context()), "poisons", p.Slug); err == nil {
		overlayPoisonTranslation(&p, tr)
	}

	if r.URL.Query().Get("fragment") == "1" {
		s.renderCard(w, r, "poisons.html", "poison_detail_card", p)
		return
	}

	s.render(w, r, "poison_detail.html", map[string]any{"Title": p.Name, "Poison": p})
}
