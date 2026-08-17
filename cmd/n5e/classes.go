package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
)

// ---- Class list -------------------------------------------------------

type classListEntry struct {
	Slug      string
	Name      string
	HitDie    sql.NullInt64
	ChakraDie sql.NullInt64
	JutsuTier sql.NullString
}

func (s *server) handleClasses(w http.ResponseWriter, r *http.Request) {
	rows, err := s.rulesDB.Query(`SELECT slug, name, hit_die, chakra_die, jutsu_tier FROM classes`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query classes:", err)
		return
	}
	defer rows.Close()

	var classes []classListEntry
	for rows.Next() {
		var c classListEntry
		if err := rows.Scan(&c.Slug, &c.Name, &c.HitDie, &c.ChakraDie, &c.JutsuTier); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("scan class:", err)
			return
		}
		classes = append(classes, c)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("classes rows:", err)
		return
	}
	sort.Slice(classes, func(i, k int) bool { return sortKey(classes[i].Name) < sortKey(classes[k].Name) })

	s.render(w, "classes.html", map[string]any{"Title": "Classes", "Classes": classes})
}

// ---- Class detail -------------------------------------------------------

var abilityLabels = map[string]string{
	"str": "Strength", "dex": "Dexterity", "con": "Constitution",
	"int": "Intelligence", "wis": "Wisdom", "cha": "Charisma",
}

var classProfLabels = []struct{ kind, label string }{
	{"armor", "Armor"},
	{"weapon", "Weapons"},
	{"tool", "Tools"},
	{"saving_throw", "Saving Throws"},
	{"skill", "Skills"},
	{"skill_choice", "Skill Choices"},
}

type castingRow struct {
	Discipline string
	Ability    string
}

type classLevelRow struct {
	Level            int
	ProficiencyBonus sql.NullInt64
	Features         sql.NullString
	JutsuKnown       sql.NullInt64
	HighestRankKnown sql.NullString
	// ResourceValues is positionally parallel to classDetail.ResourceColumns
	// (empty string where the class has no value for that resource at this
	// level, e.g. before it's unlocked).
	ResourceValues []string
}

type classFeatureRow struct {
	Name        string
	Level       sql.NullInt64
	Description string
}

type subclassFeatureRow struct {
	Name        string
	Level       sql.NullInt64
	Description string
}

type subclassRow struct {
	Slug        string
	Name        string
	Description sql.NullString
	Features    []subclassFeatureRow
	Options     []classOptionGroup // option lists scoped to this subclass (e.g. "Arbiter Maneuvers")
}

type subclassGroupRow struct {
	DisplayName     string
	SelectionLevels sql.NullString
	Description     sql.NullString
	Subclasses      []subclassRow
}

// levelFeatureEntry is one row in the merged, level-ordered feature list:
// either a general class feature (SubclassSlug "") or a specific subclass's
// feature at that level, tagged so client-side JS can show/hide it based on
// which subclass tiles are selected.
type levelFeatureEntry struct {
	Name         string
	Description  string
	Level        sql.NullInt64
	SubclassSlug string
	SubclassName string
}

type levelFeatureGroup struct {
	Level   sql.NullInt64
	Entries []levelFeatureEntry
}

type classOptionRow struct {
	Name          string
	Prerequisites sql.NullString
	Description   string
	SubclassName  sql.NullString
}

type classOptionGroup struct {
	ListName string
	Options  []classOptionRow
}

type equipmentGroup struct {
	Choices []string // multiple entries means an (a)/(b)/(c)-style alternative
}

type classDetail struct {
	Slug            string
	Name            string
	HitDie          sql.NullInt64
	ChakraDie       sql.NullInt64
	JutsuTier       sql.NullString
	Description     sql.NullString
	QuickBuild      sql.NullString
	Casting         []castingRow
	Proficiencies   []profGroup
	Equipment       []equipmentGroup
	Levels          []classLevelRow
	ResourceColumns []string
	Features        []classFeatureRow
	SubclassGroups  []subclassGroupRow
	LevelFeatures   []levelFeatureGroup
	Options         []classOptionGroup
	Feats           []featRow
	MulticlassRules *multiclassRow
}

type multiclassRow struct {
	AbilityPrereqText string
	ProficienciesText string
	JutsuPerLevelText string
}

// loadClassDetail is shared by the standalone /classes/{slug} page, the
// character-creation Class step's two-pane preview, and the AJAX fragment
// swap (see handleClassDetail) — wraps the sequential loadClassX(&c) calls
// in one function so all three callers stay in sync automatically, same
// pattern as jutsu.go's loadJutsuDetail.
func (s *server) loadClassDetail(slug string) (*classDetail, error) {
	var c classDetail
	err := s.rulesDB.QueryRow(`
		SELECT slug, name, hit_die, chakra_die, jutsu_tier, description, quick_build
		FROM classes WHERE slug = ?`, slug,
	).Scan(&c.Slug, &c.Name, &c.HitDie, &c.ChakraDie, &c.JutsuTier, &c.Description, &c.QuickBuild)
	if err != nil {
		return nil, err
	}

	if err := s.loadClassCasting(&c); err != nil {
		return nil, fmt.Errorf("load class casting: %w", err)
	}
	if err := s.loadClassProficiencies(&c); err != nil {
		return nil, fmt.Errorf("load class proficiencies: %w", err)
	}
	if err := s.loadClassEquipment(&c); err != nil {
		return nil, fmt.Errorf("load class equipment: %w", err)
	}
	if err := s.loadClassLevels(&c); err != nil {
		return nil, fmt.Errorf("load class levels: %w", err)
	}
	if err := s.loadClassFeatures(&c); err != nil {
		return nil, fmt.Errorf("load class features: %w", err)
	}
	if err := s.loadSubclassGroups(&c); err != nil {
		return nil, fmt.Errorf("load subclass groups: %w", err)
	}
	c.LevelFeatures = buildLevelFeatures(&c)
	if err := s.loadClassOptions(&c); err != nil {
		return nil, fmt.Errorf("load class options: %w", err)
	}
	if err := s.loadClassFeats(&c); err != nil {
		return nil, fmt.Errorf("load class feats: %w", err)
	}
	if err := s.loadMulticlassRules(&c); err != nil {
		return nil, fmt.Errorf("load multiclass rules: %w", err)
	}

	return &c, nil
}

// handleClassDetail serves the standalone /classes/{slug} page and, with
// ?fragment=1, just the inner detail card for the two-pane view's AJAX
// swap — same content-negotiation-by-query-param pattern as jutsu.go's
// handleJutsuDetail.
func (s *server) handleClassDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	c, err := s.loadClassDetail(slug)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("query class detail:", err)
		return
	}

	if r.URL.Query().Get("fragment") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl, ok := pageTemplates["class_detail.html"]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render class fragment: no template registered")
			return
		}
		if err := tmpl.ExecuteTemplate(w, "class_detail_card", c); err != nil {
			log.Println("render class fragment:", err)
		}
		return
	}

	s.render(w, "class_detail.html", map[string]any{"Title": c.Name, "Class": c})
}

func (s *server) loadClassCasting(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT discipline, ability FROM class_casting WHERE class_slug = ? ORDER BY discipline`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var discipline, ability string
		if err := rows.Scan(&discipline, &ability); err != nil {
			return err
		}
		label := abilityLabels[ability]
		if label == "" {
			label = ability
		}
		c.Casting = append(c.Casting, castingRow{
			Discipline: strings.ToUpper(discipline[:1]) + discipline[1:],
			Ability:    label,
		})
	}
	return rows.Err()
}

func (s *server) loadClassProficiencies(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT kind, value, choose_n FROM class_proficiencies WHERE class_slug = ?`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()
	byKind := map[string][]string{}
	for rows.Next() {
		var kind, value string
		var chooseN sql.NullInt64
		if err := rows.Scan(&kind, &value, &chooseN); err != nil {
			return err
		}
		byKind[kind] = append(byKind[kind], value)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, k := range classProfLabels {
		values := byKind[k.kind]
		if len(values) == 0 {
			continue
		}
		sort.Strings(values)
		c.Proficiencies = append(c.Proficiencies, profGroup{Kind: k.label, Values: values})
	}
	return nil
}

func (s *server) loadClassEquipment(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT group_idx, description FROM class_equipment_options
		WHERE class_slug = ? ORDER BY group_idx, choice_idx`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()
	var lastGroup = -1
	for rows.Next() {
		var groupIdx int
		var description string
		if err := rows.Scan(&groupIdx, &description); err != nil {
			return err
		}
		if groupIdx != lastGroup {
			c.Equipment = append(c.Equipment, equipmentGroup{})
			lastGroup = groupIdx
		}
		g := &c.Equipment[len(c.Equipment)-1]
		g.Choices = append(g.Choices, description)
	}
	return rows.Err()
}

// classResourceColumns gives each class's named resource columns (e.g.
// Genjutsu Specialist's "Malleable Mirages", Puppet Master's five-tier
// system) in the exact left-to-right order printed in its book level table.
// This can't be derived from the data (class_level_resources carries no
// ordering column) and genuinely varies per class, so it's hand-transcribed
// here — verified directly against each class's page in the Orochimaru
// Observation Compendium (2026-07-19). Note book order is not always
// alphabetical: Weapon Specialist prints "Styles Known" before "Flurry Die".
var classResourceColumns = map[string][]string{
	"class/cooking-nin":            {"Cooking Die"},
	"class/genjutsu-specialist":    {"Malleable Mirages"},
	"class/hunter-nin":             {"Lethal Attack"},
	"class/intelligence-operative": {"Plans Known", "Brave Orders"},
	"class/medical-nin":            {"Channeled Healing", "Chakra Scalpel Charges", "Chakra Scalpel damage"},
	"class/ninjutsu-specialist":    {"Refined Ninjutsu", "Efficient Moldings"},
	"class/science-nin":            {"Creation Points"},
	"class/taijutsu-specialist":    {"Martial Die", "Martial Techniques"},
	"class/weapon-specialist":      {"Styles Known", "Flurry Die"},
	"class/puppet-master":          {"Wood Tier", "Bronze Tier", "Silver Tier", "Gold Tier", "Platinum Tier"},
}

func (s *server) loadClassLevels(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT level, proficiency_bonus, features_text, jutsu_known, highest_rank_known
		FROM v_class_levels WHERE class_slug = ? ORDER BY level`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()

	var levels []classLevelRow
	for rows.Next() {
		var lr classLevelRow
		if err := rows.Scan(&lr.Level, &lr.ProficiencyBonus, &lr.Features, &lr.JutsuKnown, &lr.HighestRankKnown); err != nil {
			return err
		}
		levels = append(levels, lr)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	c.ResourceColumns = classResourceColumns[c.Slug]

	// One query per level for resources, same N+1 shape internal/export uses
	// for its ClassChartMaster CSV (20 levels max — not worth batching).
	for i := range levels {
		if len(c.ResourceColumns) == 0 {
			continue
		}
		resRows, err := s.rulesDB.Query(`
			SELECT resource_name, value FROM v_class_level_resources
			WHERE class_slug = ? AND level = ?`, c.Slug, levels[i].Level)
		if err != nil {
			return err
		}
		byName := map[string]string{}
		for resRows.Next() {
			var name, value string
			if err := resRows.Scan(&name, &value); err != nil {
				resRows.Close()
				return err
			}
			byName[name] = value
		}
		resRows.Close()
		if err := resRows.Err(); err != nil {
			return err
		}
		levels[i].ResourceValues = make([]string, len(c.ResourceColumns))
		for k, col := range c.ResourceColumns {
			levels[i].ResourceValues[k] = byName[col]
		}
	}
	c.Levels = levels
	return nil
}

func (s *server) loadClassFeatures(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT name, level, description FROM v_class_features
		WHERE class_slug = ? ORDER BY (level IS NULL), level, sort_order`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var f classFeatureRow
		if err := rows.Scan(&f.Name, &f.Level, &f.Description); err != nil {
			return err
		}
		c.Features = append(c.Features, f)
	}
	return rows.Err()
}

func (s *server) loadSubclassGroups(c *classDetail) error {
	groupRows, err := s.rulesDB.Query(`
		SELECT slug, display_name, selection_levels, description
		FROM subclass_groups WHERE class_slug = ?`, c.Slug)
	if err != nil {
		return err
	}
	type group struct {
		slug            string
		displayName     string
		selectionLevels sql.NullString
		description     sql.NullString
	}
	var groups []group
	for groupRows.Next() {
		var g group
		if err := groupRows.Scan(&g.slug, &g.displayName, &g.selectionLevels, &g.description); err != nil {
			groupRows.Close()
			return err
		}
		groups = append(groups, g)
	}
	groupRows.Close()
	if err := groupRows.Err(); err != nil {
		return err
	}
	sort.Slice(groups, func(i, k int) bool { return sortKey(groups[i].displayName) < sortKey(groups[k].displayName) })

	for _, g := range groups {
		agr := subclassGroupRow{DisplayName: g.displayName, SelectionLevels: g.selectionLevels, Description: g.description}

		archRows, err := s.rulesDB.Query(`
			SELECT slug, name, description FROM subclasses WHERE group_slug = ?`, g.slug)
		if err != nil {
			return err
		}
		var subclasses []subclassRow
		for archRows.Next() {
			var a subclassRow
			if err := archRows.Scan(&a.Slug, &a.Name, &a.Description); err != nil {
				archRows.Close()
				return err
			}
			subclasses = append(subclasses, a)
		}
		archRows.Close()
		if err := archRows.Err(); err != nil {
			return err
		}
		sort.Slice(subclasses, func(i, k int) bool { return sortKey(subclasses[i].Name) < sortKey(subclasses[k].Name) })

		for i := range subclasses {
			featRows, err := s.rulesDB.Query(`
				SELECT name, level, description FROM v_subclass_features
				WHERE subclass_slug = ? ORDER BY (level IS NULL), level, sort_order`, subclasses[i].Slug)
			if err != nil {
				return err
			}
			for featRows.Next() {
				var f subclassFeatureRow
				if err := featRows.Scan(&f.Name, &f.Level, &f.Description); err != nil {
					featRows.Close()
					return err
				}
				subclasses[i].Features = append(subclasses[i].Features, f)
			}
			featRows.Close()
			if err := featRows.Err(); err != nil {
				return err
			}
		}

		agr.Subclasses = subclasses
		c.SubclassGroups = append(c.SubclassGroups, agr)
	}
	return nil
}

// buildLevelFeatures merges c.Features (general, always shown) with every
// subclass's features (tagged, hidden by default and revealed client-side
// via the subclass tiles) into one level-ordered list — e.g. selecting Azure
// Analyst under Intelligence Operative should show the general level 1-2
// features first, then Azure Analyst's own features interleaved by level,
// not a separate always-visible per-subclass panel.
// Every class in the current book data has exactly one subclass group
// (confirmed directly against the DB), so pooling features across all
// groups here is equivalent to using the one group; it also degenerates
// correctly to "general features only" for classes with none.
//
// sort.SliceStable is load-bearing: entries start in "general (already
// level/sort_order-ordered), then each subclass in tile order (each
// already level/sort_order-ordered)" order, and a stable sort on level
// alone preserves that as the tie-break within a level and pushes every
// NULL-level entry (e.g. the class-wide "Ability Score Improvement/Feat"
// placeholder) into one contiguous trailing group.
func buildLevelFeatures(c *classDetail) []levelFeatureGroup {
	// Carry each entry's level alongside it for sorting, since
	// levelFeatureEntry itself doesn't need to expose Level to templates.
	type withLevel struct {
		level sql.NullInt64
		entry levelFeatureEntry
	}
	var tagged []withLevel
	for _, f := range c.Features {
		tagged = append(tagged, withLevel{level: f.Level, entry: levelFeatureEntry{Name: f.Name, Description: f.Description, Level: f.Level}})
	}
	for _, grp := range c.SubclassGroups {
		for _, sub := range grp.Subclasses {
			for _, f := range sub.Features {
				tagged = append(tagged, withLevel{level: f.Level, entry: levelFeatureEntry{
					Name: f.Name, Description: f.Description, Level: f.Level,
					SubclassSlug: sub.Slug, SubclassName: sub.Name,
				}})
			}
		}
	}

	sort.SliceStable(tagged, func(a, b int) bool {
		la, lb := tagged[a].level, tagged[b].level
		if la.Valid != lb.Valid {
			return la.Valid // valid (real level) sorts before NULL
		}
		return la.Valid && la.Int64 < lb.Int64
	})

	var groups []levelFeatureGroup
	for _, t := range tagged {
		if n := len(groups); n == 0 || groups[n-1].Level != t.level {
			groups = append(groups, levelFeatureGroup{Level: t.level})
		}
		g := &groups[len(groups)-1]
		g.Entries = append(g.Entries, t.entry)
	}
	return groups
}

// loadClassOptions must run after loadSubclassGroups: subclass-scoped option
// lists (e.g. "Arbiter Maneuvers") attach directly to their subclass's
// subclassRow.Options, so the tile switcher shows them alongside that
// subclass's features instead of in a separate always-visible section.
// Class-wide lists (subclass_slug NULL) still land in c.Options.
func (s *server) loadClassOptions(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT o.list_name, o.name, o.prerequisites, o.description, a.name
		FROM class_options o LEFT JOIN subclasses a ON a.slug = o.subclass_slug
		WHERE o.class_slug = ? ORDER BY o.list_name, o.sort_order`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rawOption struct {
		listName string
		opt      classOptionRow
	}
	var raw []rawOption
	for rows.Next() {
		var ro rawOption
		if err := rows.Scan(&ro.listName, &ro.opt.Name, &ro.opt.Prerequisites, &ro.opt.Description, &ro.opt.SubclassName); err != nil {
			return err
		}
		raw = append(raw, ro)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	bySubclass := map[string]*subclassRow{}
	for gi := range c.SubclassGroups {
		for si := range c.SubclassGroups[gi].Subclasses {
			bySubclass[c.SubclassGroups[gi].Subclasses[si].Name] = &c.SubclassGroups[gi].Subclasses[si]
		}
	}

	appendOption := func(groups *[]classOptionGroup, listName string, o classOptionRow) {
		if n := len(*groups); n == 0 || (*groups)[n-1].ListName != listName {
			*groups = append(*groups, classOptionGroup{ListName: listName})
		}
		g := &(*groups)[len(*groups)-1]
		g.Options = append(g.Options, o)
	}

	for _, ro := range raw {
		if ro.opt.SubclassName.Valid {
			if sub, ok := bySubclass[ro.opt.SubclassName.String]; ok {
				appendOption(&sub.Options, ro.listName, ro.opt)
				continue
			}
		}
		appendOption(&c.Options, ro.listName, ro.opt)
	}
	return nil
}

func (s *server) loadClassFeats(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, prerequisites, description FROM feats WHERE class_slug = ?`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var f featRow
		if err := rows.Scan(&f.Slug, &f.Name, &f.Prerequisites, &f.Description); err != nil {
			return err
		}
		c.Feats = append(c.Feats, f)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	sort.Slice(c.Feats, func(i, k int) bool { return sortKey(c.Feats[i].Name) < sortKey(c.Feats[k].Name) })
	return nil
}

func (s *server) loadMulticlassRules(c *classDetail) error {
	var m multiclassRow
	err := s.rulesDB.QueryRow(`
		SELECT ability_prereq_text, proficiencies_text, jutsu_per_level_text
		FROM class_multiclass_rules WHERE class_slug = ?`, c.Slug,
	).Scan(&m.AbilityPrereqText, &m.ProficienciesText, &m.JutsuPerLevelText)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query multiclass rules: %w", err)
	}
	c.MulticlassRules = &m
	return nil
}
