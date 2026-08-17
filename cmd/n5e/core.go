package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"
	"strings"
)

// ---- Backgrounds --------------------------------------------------------

type backgroundRow struct {
	Slug              string
	Name              string
	Description       string
	FeatureName       sql.NullString
	FeatureText       sql.NullString
	ASIText           sql.NullString
	EquipmentText     sql.NullString
	EquipmentPackText sql.NullString
	Proficiencies     []profGroup
}

func (s *server) handleBackgrounds(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, description, feature_name, feature_text, asi_text,
		       equipment_text, equipment_pack_text
		FROM backgrounds`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query backgrounds:", err)
		return
	}
	defer rows.Close()

	var backgrounds []backgroundRow
	for rows.Next() {
		var b backgroundRow
		if err := rows.Scan(&b.Slug, &b.Name, &b.Description, &b.FeatureName, &b.FeatureText,
			&b.ASIText, &b.EquipmentText, &b.EquipmentPackText); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan background:", err)
			return
		}
		backgrounds = append(backgrounds, b)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("backgrounds rows:", err)
		return
	}
	sort.Slice(backgrounds, func(i, k int) bool { return sortKey(backgrounds[i].Name) < sortKey(backgrounds[k].Name) })

	profRows, err := s.rulesDB.Query(`SELECT background_slug, kind, value FROM background_proficiencies`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query background proficiencies:", err)
		return
	}
	byBackground := map[string]map[string][]string{}
	for profRows.Next() {
		var slug, kind, value string
		if err := profRows.Scan(&slug, &kind, &value); err != nil {
			profRows.Close()
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan background proficiency:", err)
			return
		}
		if byBackground[slug] == nil {
			byBackground[slug] = map[string][]string{}
		}
		byBackground[slug][kind] = append(byBackground[slug][kind], value)
	}
	profRows.Close()

	for i := range backgrounds {
		byKind := byBackground[backgrounds[i].Slug]
		for _, k := range profKindLabels {
			values := byKind[k.kind]
			if len(values) == 0 {
				continue
			}
			sort.Strings(values)
			backgrounds[i].Proficiencies = append(backgrounds[i].Proficiencies, profGroup{Kind: k.label, Values: values})
		}
	}

	s.render(w, "backgrounds.html", map[string]any{"Title": "Backgrounds", "Backgrounds": backgrounds})
}

type ambitionPrompt struct {
	Kind string
	Text string
}

type backgroundDetail struct {
	backgroundRow
	AmbitionPrompts []ambitionPrompt
}

// loadBackgroundDetail is shared by the standalone /backgrounds/{slug}
// page, the character-creation Background step's two-pane preview, and the
// AJAX fragment swap (see handleBackgroundDetail) — same pattern as
// jutsu.go's loadJutsuDetail.
func (s *server) loadBackgroundDetail(slug string) (*backgroundDetail, error) {
	var b backgroundDetail
	err := s.rulesDB.QueryRow(`
		SELECT slug, name, description, feature_name, feature_text, asi_text,
		       equipment_text, equipment_pack_text
		FROM backgrounds WHERE slug = ?`, slug,
	).Scan(&b.Slug, &b.Name, &b.Description, &b.FeatureName, &b.FeatureText,
		&b.ASIText, &b.EquipmentText, &b.EquipmentPackText)
	if err != nil {
		return nil, err
	}

	profRows, err := s.rulesDB.Query(`
		SELECT kind, value FROM background_proficiencies WHERE background_slug = ?`, slug)
	if err != nil {
		return nil, err
	}
	byKind := map[string][]string{}
	for profRows.Next() {
		var kind, value string
		if err := profRows.Scan(&kind, &value); err != nil {
			profRows.Close()
			return nil, err
		}
		byKind[kind] = append(byKind[kind], value)
	}
	profRows.Close()
	if err := profRows.Err(); err != nil {
		return nil, err
	}
	for _, k := range profKindLabels {
		values := byKind[k.kind]
		if len(values) == 0 {
			continue
		}
		sort.Strings(values)
		b.Proficiencies = append(b.Proficiencies, profGroup{Kind: k.label, Values: values})
	}

	ambitionRows, err := s.rulesDB.Query(`
		SELECT kind, text FROM ambition_prompts WHERE background_slug = ?`, slug)
	if err != nil {
		return nil, err
	}
	ambitionKindLabels := map[string]string{"drive": "Drive", "goal": "Goal", "fear": "Fear"}
	for ambitionRows.Next() {
		var a ambitionPrompt
		if err := ambitionRows.Scan(&a.Kind, &a.Text); err != nil {
			ambitionRows.Close()
			return nil, err
		}
		if label, ok := ambitionKindLabels[a.Kind]; ok {
			a.Kind = label
		}
		b.AmbitionPrompts = append(b.AmbitionPrompts, a)
	}
	ambitionRows.Close()
	if err := ambitionRows.Err(); err != nil {
		return nil, err
	}

	return &b, nil
}

// handleBackgroundDetail serves the standalone /backgrounds/{slug} page
// and, with ?fragment=1, just the inner detail card for the two-pane
// view's AJAX swap — same content-negotiation-by-query-param pattern as
// jutsu.go's handleJutsuDetail.
func (s *server) handleBackgroundDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	b, err := s.loadBackgroundDetail(slug)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query background detail:", err)
		return
	}

	if r.URL.Query().Get("fragment") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl, ok := pageTemplates["background_detail.html"]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render background fragment: no template registered")
			return
		}
		if err := tmpl.ExecuteTemplate(w, "background_detail_card", b); err != nil {
			log.Println("render background fragment:", err)
		}
		return
	}

	s.render(w, "background_detail.html", map[string]any{"Title": b.Name, "Background": b})
}

// ---- Fighting Stances -----------------------------------------------------

type stanceRow struct {
	Slug          string
	Name          string
	Prerequisites sql.NullString
	Description   string
}

func (s *server) handleStances(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, stance_type, prerequisites, description FROM fighting_stances`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query stances:", err)
		return
	}
	defer rows.Close()

	byType := map[string][]stanceRow{}
	for rows.Next() {
		var st stanceRow
		var stanceType sql.NullString
		if err := rows.Scan(&st.Slug, &st.Name, &stanceType, &st.Prerequisites, &st.Description); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan stance:", err)
			return
		}
		byType[stanceType.String] = append(byType[stanceType.String], st)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("stances rows:", err)
		return
	}

	type stanceGroup struct {
		Label   string
		Stances []stanceRow
	}
	var groups []stanceGroup
	for _, g := range []struct{ kind, label string }{
		{"taijutsu", "Taijutsu Stances"},
		{"bukijutsu", "Bukijutsu Stances"},
	} {
		stances := byType[g.kind]
		if len(stances) == 0 {
			continue
		}
		sort.Slice(stances, func(i, k int) bool { return sortKey(stances[i].Name) < sortKey(stances[k].Name) })
		groups = append(groups, stanceGroup{Label: g.label, Stances: stances})
	}

	s.render(w, "stances.html", map[string]any{"Title": "Fighting Stances", "Groups": groups})
}

func (s *server) handleStanceDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var st stanceRow
	var stanceType sql.NullString
	err := s.rulesDB.QueryRow(`
		SELECT slug, name, stance_type, prerequisites, description
		FROM fighting_stances WHERE slug = ?`, slug,
	).Scan(&st.Slug, &st.Name, &stanceType, &st.Prerequisites, &st.Description)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query stance detail:", err)
		return
	}

	label := "Taijutsu Stance"
	if stanceType.String == "bukijutsu" {
		label = "Bukijutsu Stance"
	}

	s.render(w, "stance_detail.html", map[string]any{"Title": st.Name, "Stance": st, "TypeLabel": label})
}

// ---- Feats ----------------------------------------------------------------

// featCategoryLabels maps the feats.category column's raw values (printed
// verbatim from the PDF's "Category:" line, lowercase, some with a trailing
// ", rare"/", level 4+" qualifier) to a properly capitalized display label.
// "Archetype" is left as the book's own printed term here (a class-feat
// category label the sourcebook prints literally, same as the "Archetype:"
// class-name prefix in internal/parse/clans.go) — not renamed to "Subclass"
// alongside the rest of the app, since this one is quoting the book's own
// text rather than naming our own subclass-system tables.
var featCategoryLabels = map[string]string{
	"archetype":          "Archetype",
	"bukijutsu":          "Bukijutsu",
	"chakra":             "Chakra",
	"chakra, level 4+":   "Chakra (Level 4+)",
	"clan":               "Clan",
	"clan, rare":         "Clan (Rare)",
	"class":              "Class",
	"critical":           "Critical",
	"critical, level 4+": "Critical (Level 4+)",
	"general":            "General",
	"genjutsu":           "Genjutsu",
	"ninjutsu":           "Ninjutsu",
	"skill":              "Skill",
	"taijutsu":           "Taijutsu",
	"taijutsu, rare":     "Taijutsu (Rare)",
}

// featCategoryLabel looks up featCategoryLabels, falling back to
// capitalizing just the first letter for any category value not in the map
// (defensive — the map above is exhaustive against the live data today, but
// a future re-ingest could introduce a new one).
func featCategoryLabel(raw string) string {
	if label, ok := featCategoryLabels[raw]; ok {
		return label
	}
	if raw == "" {
		return raw
	}
	return strings.ToUpper(raw[:1]) + raw[1:]
}

type featRow struct {
	Slug          string
	Name          string
	Prerequisites sql.NullString
	Description   string
}

type featDetail struct {
	Slug          string
	Name          string
	Category      string
	Prerequisites sql.NullString
	Description   string
	ClanSlug      sql.NullString
	ClanName      sql.NullString
	ClassSlug     sql.NullString
	ClassName     sql.NullString
}

func (s *server) handleFeats(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`SELECT slug, name, category, prerequisites, description FROM feats`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query feats:", err)
		return
	}
	defer rows.Close()

	byCategory := map[string][]featRow{}
	for rows.Next() {
		var f featRow
		var category string
		if err := rows.Scan(&f.Slug, &f.Name, &category, &f.Prerequisites, &f.Description); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan feat:", err)
			return
		}
		byCategory[category] = append(byCategory[category], f)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("feats rows:", err)
		return
	}

	type featGroup struct {
		Category string
		Feats    []featRow
	}
	var categories []string
	for c := range byCategory {
		categories = append(categories, c)
	}
	sort.Strings(categories)

	var groups []featGroup
	for _, c := range categories {
		feats := byCategory[c]
		sort.Slice(feats, func(i, k int) bool { return sortKey(feats[i].Name) < sortKey(feats[k].Name) })
		groups = append(groups, featGroup{Category: featCategoryLabel(c), Feats: feats})
	}

	s.render(w, "feats.html", map[string]any{"Title": "Feats", "Groups": groups})
}

func (s *server) handleFeatDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var f featDetail
	err := s.rulesDB.QueryRow(`
		SELECT f.slug, f.name, f.category, f.prerequisites, f.description,
		       f.clan_slug, cl.name, f.class_slug, cs.name
		FROM feats f
		LEFT JOIN clans cl ON cl.slug = f.clan_slug
		LEFT JOIN classes cs ON cs.slug = f.class_slug
		WHERE f.slug = ?`, slug,
	).Scan(&f.Slug, &f.Name, &f.Category, &f.Prerequisites, &f.Description,
		&f.ClanSlug, &f.ClanName, &f.ClassSlug, &f.ClassName)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query feat detail:", err)
		return
	}
	f.Category = featCategoryLabel(f.Category)

	if r.URL.Query().Get("fragment") == "1" {
		s.renderCard(w, "feats.html", "feat_detail_card", f)
		return
	}

	s.render(w, "feat_detail.html", map[string]any{"Title": f.Name, "Feat": f})
}

// ---- Enhancement Seals ------------------------------------------------------

type sealRow struct {
	Slug        string
	Name        string
	SealRank    sql.NullString
	Description sql.NullString
}

func (s *server) handleSeals(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`
		SELECT e.slug, e.name, e.seal_rank, e.seal_applies_to, e.description
		FROM equipment e WHERE e.kind = 'enhancement_seal'`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query seals:", err)
		return
	}
	defer rows.Close()

	type sealWithGroup struct {
		sealRow
		AppliesTo string
	}
	var seals []sealWithGroup
	for rows.Next() {
		var sw sealWithGroup
		var appliesTo sql.NullString
		if err := rows.Scan(&sw.Slug, &sw.Name, &sw.SealRank, &appliesTo, &sw.Description); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan seal:", err)
			return
		}
		sw.AppliesTo = appliesTo.String
		seals = append(seals, sw)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("seals rows:", err)
		return
	}

	sort.Slice(seals, func(i, k int) bool {
		if seals[i].AppliesTo != seals[k].AppliesTo {
			return seals[i].AppliesTo < seals[k].AppliesTo
		}
		if ri, rk := jutsuRankOrder[seals[i].SealRank.String], jutsuRankOrder[seals[k].SealRank.String]; ri != rk {
			return ri < rk
		}
		return sortKey(seals[i].Name) < sortKey(seals[k].Name)
	})

	type sealGroup struct {
		Label string
		Seals []sealRow
	}
	var groups []sealGroup
	for _, sw := range seals {
		label := "Weapon Seals"
		if sw.AppliesTo == "armor" {
			label = "Armor Seals"
		}
		if len(groups) == 0 || groups[len(groups)-1].Label != label {
			groups = append(groups, sealGroup{Label: label})
		}
		groups[len(groups)-1].Seals = append(groups[len(groups)-1].Seals, sw.sealRow)
	}

	s.render(w, "seals.html", map[string]any{"Title": "Enhancement Seals", "Groups": groups})
}

func (s *server) handleSealDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var sw struct {
		sealRow
		AppliesTo sql.NullString
	}
	err := s.rulesDB.QueryRow(`
		SELECT e.slug, e.name, e.seal_rank, e.seal_applies_to, e.description
		FROM equipment e WHERE e.kind = 'enhancement_seal' AND e.slug = ?`, slug,
	).Scan(&sw.Slug, &sw.Name, &sw.SealRank, &sw.AppliesTo, &sw.Description)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query seal detail:", err)
		return
	}

	appliesToLabel := ""
	switch sw.AppliesTo.String {
	case "weapon":
		appliesToLabel = "Weapon"
	case "armor":
		appliesToLabel = "Armor"
	}

	s.render(w, "seal_detail.html", map[string]any{"Title": sw.Name, "Seal": sw, "AppliesTo": appliesToLabel})
}
