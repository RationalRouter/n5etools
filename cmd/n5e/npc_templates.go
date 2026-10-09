// Browsable GM reference pages for the Bingo Book's (book/bingo-book) NPC
// template stat blocks, General Traits glossary and Specialist Jutsu Lists
// (migration 0080, loaded from internal/bingobookdata's hand transcription).
// Read-only reference material: nothing here is wired into the Encounter
// Generator's procedural generation.
package main

import (
	"database/sql"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/charsheet"
	"github.com/RationalRouter/n5e/internal/i18n"
)

const npcTemplateSlugPrefix = "adversary-template/"

type npcTemplateListRow struct {
	Slug     string
	Path     string // Slug without its "adversary-template/" prefix, as used in the detail URL
	Name     string
	RankTier string
	Class    string
	Role     string
	Level    int
	clanName string
}

type npcTemplateGroup struct {
	Clan      string
	Templates []npcTemplateListRow
}

func (s *server) handleNPCTemplates(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, clan_name, rank_tier, class, role, level
		FROM adversary_template_statblocks
		ORDER BY source_page, rowid`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query adversary_template_statblocks:", err)
		return
	}
	defer rows.Close()

	var list []npcTemplateListRow
	var slugs []string
	for rows.Next() {
		var t npcTemplateListRow
		if err := rows.Scan(&t.Slug, &t.Name, &t.clanName, &t.RankTier, &t.Class, &t.Role, &t.Level); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan adversary_template_statblocks:", err)
			return
		}
		t.Path = strings.TrimPrefix(t.Slug, npcTemplateSlugPrefix)
		list = append(list, t)
		slugs = append(slugs, t.Slug)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("adversary_template_statblocks rows:", err)
		return
	}

	locale := localeFromContext(r.Context())
	translations, err := i18n.Batch(s.rulesDB, locale, "adversary_template_statblocks", slugs)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("translate adversary_template_statblocks:", err)
		return
	}

	// Groups follow book order: a clan's group sits where its first block
	// is printed.
	var groups []npcTemplateGroup
	index := map[string]int{}
	for _, t := range list {
		tr := translations[t.Slug]
		if v, ok := tr["name"]; ok {
			t.Name = v
		}
		if v, ok := tr["role"]; ok {
			t.Role = v
		}
		clan := t.clanName
		if v, ok := tr["clan_name"]; ok {
			clan = v
		}
		t.Class = s.t(r, "npc_templates.class_"+t.Class)
		i, ok := index[t.clanName]
		if !ok {
			i = len(groups)
			index[t.clanName] = i
			groups = append(groups, npcTemplateGroup{Clan: clan})
		}
		groups[i].Templates = append(groups[i].Templates, t)
	}

	s.render(w, r, "npc_templates.html", map[string]any{
		"Title":  s.t(r, "npc_templates.title"),
		"Groups": groups,
	})
}

type npcAbility struct {
	Label            string
	Score, Mod, Save int
}

type npcSkill struct {
	Name  string
	Mod   int
	Stars string
}

type npcEntry struct {
	Name, Tags, Description string
}

type npcSection struct {
	Title   string
	Entries []npcEntry
}

type npcJutsu struct {
	Name string
	Slug string // "" when the name matches no jutsu in the rules database
}

type npcJutsuRank struct {
	Rank  string
	Cost  int
	Jutsu []npcJutsu
}

type npcJutsuDiscipline struct {
	Discipline string
	Hit, DC    int
	Ranks      []npcJutsuRank
}

type npcTemplateDetail struct {
	Slug, Name, ClanName, RankTier, Class, Role, Affiliation, Size string
	Level, XP, ProfBonus                                           int
	TenacityDie                                                    string
	TenacityCount                                                  int
	AC                                                             int
	ACNote                                                         string
	DR, Initiative                                                 int
	Speed, HPText                                                  string
	CP                                                             int
	DamageResistances, DamageImmunities, DamageVulnerabilities     string
	ConditionResistances, ConditionImmunities, Senses              string
	PassivePerception, SourcePage                                  int
	Notes                                                          string

	Abilities []npcAbility
	Skills    []npcSkill
	Sections  []npcSection
	Jutsu     []npcJutsuDiscipline
}

func (s *server) handleNPCTemplateDetail(w http.ResponseWriter, r *http.Request) {
	slug := npcTemplateSlugPrefix + r.PathValue("slug")
	locale := localeFromContext(r.Context())

	var d npcTemplateDetail
	var abil [6][3]int
	err := s.rulesDB.QueryRow(`
		SELECT slug, name, clan_name, rank_tier, class, role, affiliation, size, level, xp,
		       prof_bonus, tenacity_die, tenacity_count, ac, ac_note, dr, initiative_bonus,
		       speed, hp_text, cp,
		       str_score, str_mod, str_save, dex_score, dex_mod, dex_save,
		       con_score, con_mod, con_save, int_score, int_mod, int_save,
		       wis_score, wis_mod, wis_save, cha_score, cha_mod, cha_save,
		       damage_resistances, damage_immunities, damage_vulnerabilities,
		       condition_resistances, condition_immunities, senses, passive_perception,
		       coalesce(source_page, 0), notes
		FROM adversary_template_statblocks WHERE slug = ?`, slug,
	).Scan(&d.Slug, &d.Name, &d.ClanName, &d.RankTier, &d.Class, &d.Role, &d.Affiliation, &d.Size,
		&d.Level, &d.XP, &d.ProfBonus, &d.TenacityDie, &d.TenacityCount, &d.AC, &d.ACNote, &d.DR,
		&d.Initiative, &d.Speed, &d.HPText, &d.CP,
		&abil[0][0], &abil[0][1], &abil[0][2], &abil[1][0], &abil[1][1], &abil[1][2],
		&abil[2][0], &abil[2][1], &abil[2][2], &abil[3][0], &abil[3][1], &abil[3][2],
		&abil[4][0], &abil[4][1], &abil[4][2], &abil[5][0], &abil[5][1], &abil[5][2],
		&d.DamageResistances, &d.DamageImmunities, &d.DamageVulnerabilities,
		&d.ConditionResistances, &d.ConditionImmunities, &d.Senses, &d.PassivePerception,
		&d.SourcePage, &d.Notes)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query adversary template:", err)
		return
	}

	overlay, err := i18n.Row(s.rulesDB, locale, "adversary_template_statblocks", d.Slug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("translate adversary template:", err)
		return
	}
	for field, dst := range map[string]*string{
		"name": &d.Name, "clan_name": &d.ClanName, "role": &d.Role, "affiliation": &d.Affiliation,
		"size": &d.Size, "ac_note": &d.ACNote, "speed": &d.Speed,
		"damage_resistances": &d.DamageResistances, "damage_immunities": &d.DamageImmunities,
		"damage_vulnerabilities": &d.DamageVulnerabilities,
		"condition_resistances":  &d.ConditionResistances, "condition_immunities": &d.ConditionImmunities,
		"senses": &d.Senses, "notes": &d.Notes,
	} {
		if v, ok := overlay[field]; ok {
			*dst = v
		}
	}
	d.Class = s.t(r, "npc_templates.class_"+d.Class)
	tr := i18n.Translator(locale)
	for i, key := range charsheet.Abilities {
		d.Abilities = append(d.Abilities, npcAbility{
			Label: abilityLabelTr(tr, key),
			Score: abil[i][0], Mod: abil[i][1], Save: abil[i][2],
		})
	}

	if d.Skills, err = s.loadNPCTemplateSkills(locale, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query adversary template skills:", err)
		return
	}
	if d.Sections, err = s.loadNPCTemplateSections(locale, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query adversary template entries:", err)
		return
	}
	if d.Jutsu, err = s.loadNPCTemplateJutsu(locale, slug); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query adversary template jutsu:", err)
		return
	}

	s.render(w, r, "npc_template_detail.html", map[string]any{"Title": d.Name, "Template": d})
}

func (s *server) loadNPCTemplateSkills(locale, slug string) ([]npcSkill, error) {
	rows, err := s.rulesDB.Query(`
		SELECT skill_name, modifier, stars FROM adversary_template_skills
		WHERE statblock_slug = ? ORDER BY sort_order`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []npcSkill
	for rows.Next() {
		var sk npcSkill
		var stars int
		if err := rows.Scan(&sk.Name, &sk.Mod, &stars); err != nil {
			return nil, err
		}
		sk.Name = skillDisplayName(locale, sk.Name)
		sk.Stars = strings.Repeat("★", stars)
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (s *server) loadNPCTemplateSections(locale, slug string) ([]npcSection, error) {
	rows, err := s.rulesDB.Query(`
		SELECT section_order, section_title, sort_order, name, tags, description
		FROM adversary_template_entries
		WHERE statblock_slug = ? ORDER BY section_order, sort_order`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type keyed struct {
		section int
		key     string
		entry   npcEntry
		title   string
	}
	var all []keyed
	var keys []string
	for rows.Next() {
		var k keyed
		var sortOrder int
		if err := rows.Scan(&k.section, &k.title, &sortOrder, &k.entry.Name, &k.entry.Tags, &k.entry.Description); err != nil {
			return nil, err
		}
		k.key = slug + i18n.KeySeparator + strconv.Itoa(k.section) + i18n.KeySeparator + strconv.Itoa(sortOrder)
		all = append(all, k)
		keys = append(keys, k.key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	translations, err := i18n.Batch(s.rulesDB, locale, "adversary_template_entries", keys)
	if err != nil {
		return nil, err
	}
	var out []npcSection
	last := -1
	for _, k := range all {
		tr := translations[k.key]
		if v, ok := tr["section_title"]; ok {
			k.title = v
		}
		if v, ok := tr["name"]; ok {
			k.entry.Name = v
		}
		if v, ok := tr["tags"]; ok {
			k.entry.Tags = v
		}
		if v, ok := tr["description"]; ok {
			k.entry.Description = v
		}
		if k.section != last {
			out = append(out, npcSection{Title: k.title})
			last = k.section
		}
		out[len(out)-1].Entries = append(out[len(out)-1].Entries, k.entry)
	}
	return out, nil
}

func (s *server) loadNPCTemplateJutsu(locale, slug string) ([]npcJutsuDiscipline, error) {
	rows, err := s.rulesDB.Query(`
		SELECT discipline, hit_bonus, save_dc, rank, cost, jutsu_name
		FROM adversary_template_jutsu
		WHERE statblock_slug = ? ORDER BY sort_order`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []npcJutsuDiscipline
	for rows.Next() {
		var disc, rank, name string
		var hit, dc, cost int
		if err := rows.Scan(&disc, &hit, &dc, &rank, &cost, &name); err != nil {
			return nil, err
		}
		if n := len(out); n == 0 || out[n-1].Discipline != disc || out[n-1].Hit != hit || out[n-1].DC != dc {
			out = append(out, npcJutsuDiscipline{Discipline: disc, Hit: hit, DC: dc})
		}
		d := &out[len(out)-1]
		if n := len(d.Ranks); n == 0 || d.Ranks[n-1].Rank != rank {
			d.Ranks = append(d.Ranks, npcJutsuRank{Rank: rank, Cost: cost})
		}
		rk := &d.Ranks[len(d.Ranks)-1]
		rk.Jutsu = append(rk.Jutsu, npcJutsu{Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.linkNPCTemplateJutsu(locale, out)
}

// linkNPCTemplateJutsu resolves each printed jutsu name to its rules
// database row, so the stat block can link to it and show its translated
// name. The book prints names without the release prefix the jutsu table
// carries ("Hidden Mist" for "Water Release: Hidden Mist") and sometimes
// with a scaled-damage note ("Feather Burst (2d8)"); both forms are matched.
// A name that matches nothing, or matches more than one prefixed jutsu, is
// left as plain text.
func (s *server) linkNPCTemplateJutsu(locale string, disciplines []npcJutsuDiscipline) error {
	rows, err := s.rulesDB.Query(`SELECT slug, name FROM jutsu`)
	if err != nil {
		return err
	}
	defer rows.Close()
	exact := map[string]string{}
	bySuffix := map[string][]string{}
	for rows.Next() {
		var slug, name string
		if err := rows.Scan(&slug, &name); err != nil {
			return err
		}
		exact[npcJutsuKey(name)] = slug
		if _, rest, ok := strings.Cut(name, ": "); ok {
			k := npcJutsuKey(rest)
			bySuffix[k] = append(bySuffix[k], slug)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	resolve := func(name string) string {
		for _, candidate := range []string{name, npcTrailingParen.ReplaceAllString(name, "")} {
			k := npcJutsuKey(candidate)
			if slug, ok := exact[k]; ok {
				return slug
			}
			if slugs := bySuffix[k]; len(slugs) == 1 {
				return slugs[0]
			}
		}
		return ""
	}

	var slugs []string
	for di := range disciplines {
		for ri := range disciplines[di].Ranks {
			for ji := range disciplines[di].Ranks[ri].Jutsu {
				j := &disciplines[di].Ranks[ri].Jutsu[ji]
				j.Slug = resolve(j.Name)
				if j.Slug != "" {
					slugs = append(slugs, j.Slug)
				}
			}
		}
	}
	translations, err := i18n.Batch(s.rulesDB, locale, "jutsu", slugs)
	if err != nil {
		return err
	}
	for di := range disciplines {
		for ri := range disciplines[di].Ranks {
			for ji := range disciplines[di].Ranks[ri].Jutsu {
				j := &disciplines[di].Ranks[ri].Jutsu[ji]
				if v, ok := translations[j.Slug]["name"]; ok {
					j.Name = v
				}
			}
		}
	}
	return nil
}

var npcTrailingParen = regexp.MustCompile(`\s*\([^)]*\)$`)

// npcJutsuKey normalizes a jutsu name for matching: case, curly vs straight
// apostrophes, punctuation and diacritics differ between the book's spelling
// and the jutsu table's.
func npcJutsuKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == 'ū':
			b.WriteRune('u')
		case r == 'ō':
			b.WriteRune('o')
		}
	}
	return b.String()
}

type generalTraitRow struct {
	Slug, Name, Rank, Keywords, Description, Notes, FilterKey string
}

type generalTraitGroup struct {
	Label  string
	Traits []generalTraitRow
}

type specialistListView struct {
	Name  string
	Jutsu []npcJutsu
}

func (s *server) handleBingoBookGeneralTraits(w http.ResponseWriter, r *http.Request) {
	locale := localeFromContext(r.Context())
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, rank, keywords, description, notes
		FROM adversary_beta_general_traits ORDER BY sort_order`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query adversary_beta_general_traits:", err)
		return
	}
	defer rows.Close()
	var traits []generalTraitRow
	var slugs []string
	for rows.Next() {
		var t generalTraitRow
		if err := rows.Scan(&t.Slug, &t.Name, &t.Rank, &t.Keywords, &t.Description, &t.Notes); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan adversary_beta_general_traits:", err)
			return
		}
		traits = append(traits, t)
		slugs = append(slugs, t.Slug)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("adversary_beta_general_traits rows:", err)
		return
	}
	translations, err := i18n.Batch(s.rulesDB, locale, "adversary_beta_general_traits", slugs)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("translate adversary_beta_general_traits:", err)
		return
	}

	byRank := map[string][]generalTraitRow{}
	for _, t := range traits {
		tr := translations[t.Slug]
		if v, ok := tr["name"]; ok {
			t.Name = v
		}
		if v, ok := tr["keywords"]; ok {
			t.Keywords = v
		}
		if v, ok := tr["description"]; ok {
			t.Description = v
		}
		if v, ok := tr["notes"]; ok {
			t.Notes = v
		}
		t.FilterKey = strings.ToLower(t.Name + " " + t.Keywords)
		byRank[t.Rank] = append(byRank[t.Rank], t)
	}
	var groups []generalTraitGroup
	for _, rank := range []string{"D", "C", "B", "A", "S", ""} {
		if len(byRank[rank]) == 0 {
			continue
		}
		label := s.t(r, "npc_templates.rank_traits", rank)
		if rank == "" {
			label = s.t(r, "npc_templates.unranked_traits")
		}
		groups = append(groups, generalTraitGroup{Label: label, Traits: byRank[rank]})
	}

	lists, err := s.loadSpecialistLists(locale)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query adversary_specialist_jutsu_lists:", err)
		return
	}

	s.render(w, r, "bingo_book_general_traits.html", map[string]any{
		"Title":           s.t(r, "npc_templates.general_traits_title"),
		"Groups":          groups,
		"SpecialistLists": lists,
	})
}

func (s *server) loadSpecialistLists(locale string) ([]specialistListView, error) {
	rows, err := s.rulesDB.Query(`
		SELECT list_name, jutsu_name FROM adversary_specialist_jutsu_lists
		ORDER BY list_order, sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []specialistListView
	for rows.Next() {
		var list, name string
		if err := rows.Scan(&list, &name); err != nil {
			return nil, err
		}
		if n := len(out); n == 0 || out[n-1].Name != list {
			out = append(out, specialistListView{Name: list})
		}
		out[len(out)-1].Jutsu = append(out[len(out)-1].Jutsu, npcJutsu{Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reuse the stat-block linker: one pseudo-discipline per list.
	disciplines := make([]npcJutsuDiscipline, len(out))
	for i, l := range out {
		disciplines[i].Ranks = []npcJutsuRank{{Jutsu: l.Jutsu}}
	}
	if err := s.linkNPCTemplateJutsu(locale, disciplines); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Name = specialistListLabel(locale, out[i].Name)
	}
	return out, nil
}

// specialistListLabel translates a Specialist Jutsu List's name ("Fire",
// "Genjutsu", ...) through the UI catalog; list_name itself stays English
// because it is half of the table's primary key.
func specialistListLabel(locale, name string) string {
	return i18n.Translator(locale)("npc_templates.specialist_list_" + strings.ToLower(name))
}
