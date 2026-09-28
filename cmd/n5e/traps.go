package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"

	"github.com/RationalRouter/n5e/internal/i18n"
)

type trapRow struct {
	Slug            string
	Name            string
	BuildDC         sql.NullInt64
	SaveDC          sql.NullInt64
	NoticeDisableDC sql.NullInt64
	VsAbility       sql.NullString
	TimeToBuild     sql.NullString
	ToolkitRequired string
	Description     string
}

const trapSelectColumns = `slug, name, build_dc, save_dc, notice_disable_dc, vs_ability, time_to_build, toolkit_required, description`

func scanTrapRow(scan func(dest ...any) error) (trapRow, error) {
	var t trapRow
	err := scan(&t.Slug, &t.Name, &t.BuildDC, &t.SaveDC, &t.NoticeDisableDC, &t.VsAbility, &t.TimeToBuild, &t.ToolkitRequired, &t.Description)
	return t, err
}

func overlayTrapTranslation(t *trapRow, tr map[string]string) {
	if v, ok := tr["name"]; ok {
		t.Name = v
	}
	if v, ok := tr["description"]; ok {
		t.Description = v
	}
	if v, ok := tr["vs_ability"]; ok {
		t.VsAbility = sql.NullString{String: v, Valid: true}
	}
	if v, ok := tr["time_to_build"]; ok {
		t.TimeToBuild = sql.NullString{String: v, Valid: true}
	}
	if v, ok := tr["toolkit_required"]; ok {
		t.ToolkitRequired = v
	}
}

func (s *server) handleTraps(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`SELECT ` + trapSelectColumns + ` FROM trap_templates`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query traps:", err)
		return
	}
	defer rows.Close()

	var traps []trapRow
	for rows.Next() {
		t, err := scanTrapRow(rows.Scan)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan trap:", err)
			return
		}
		traps = append(traps, t)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("traps rows:", err)
		return
	}

	locale := localeFromContext(r.Context())
	slugs := make([]string, len(traps))
	for i, t := range traps {
		slugs[i] = t.Slug
	}
	translations, err := i18n.Batch(s.rulesDB, locale, "trap_templates", slugs)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("translate traps:", err)
		return
	}
	for i := range traps {
		overlayTrapTranslation(&traps[i], translations[traps[i].Slug])
	}

	sort.Slice(traps, func(i, j int) bool { return sortKey(traps[i].Name) < sortKey(traps[j].Name) })

	s.render(w, r, "traps.html", map[string]any{"Title": s.t(r, "traps.title"), "Traps": traps})
}

func (s *server) handleTrapDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	row := s.rulesDB.QueryRow(`SELECT `+trapSelectColumns+` FROM trap_templates WHERE slug = ?`, slug)
	t, err := scanTrapRow(row.Scan)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query trap detail:", err)
		return
	}
	if tr, err := i18n.Row(s.rulesDB, localeFromContext(r.Context()), "trap_templates", t.Slug); err == nil {
		overlayTrapTranslation(&t, tr)
	}

	if r.URL.Query().Get("fragment") == "1" {
		s.renderCard(w, r, "traps.html", "trap_detail_card", t)
		return
	}

	s.render(w, r, "trap_detail.html", map[string]any{"Title": t.Name, "Trap": t})
}
