package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/i18n"
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

	s.render(w, r, "classes.html", map[string]any{"Title": s.t(r, "classes.title"), "Classes": classes})
}

// ---- Class detail -------------------------------------------------------

var abilityLabels = map[string]string{
	"str": "Strength", "dex": "Dexterity", "con": "Constitution",
	"int": "Intelligence", "wis": "Wisdom", "cha": "Charisma",
}

// abilityCatalogKeys maps a 3-letter ability code to its UI-catalog key —
// the same full names already translated for create_abilities.html's own
// ability-score labels, reused here rather than a second copy.
var abilityCatalogKeys = map[string]string{
	"str": "create_abilities.strength", "dex": "create_abilities.dexterity", "con": "create_abilities.constitution",
	"int": "create_abilities.intelligence", "wis": "create_abilities.wisdom", "cha": "create_abilities.charisma",
}

// abilityLabelTr expands a 3-letter ability code to its full name in tr's
// locale, falling back to the raw code for anything not in
// abilityCatalogKeys (e.g. already-invalid input).
func abilityLabelTr(tr i18n.TranslateFunc, ab string) string {
	if key, ok := abilityCatalogKeys[ab]; ok {
		return tr(key)
	}
	return ab
}

// tierNameTr translates one raw Tier/TierName value (Titan Upgrades' and
// Scientific Ninja Tools' own class_options.name — "Minor", "Refined",
// "Superior", "Supreme", "Mastercraft", "Greater") via the shared
// "tier_name:" UI-catalog namespace — see templates.go's own tierName
// template func doc for why this happens at render time rather than at the
// Go loader (several call sites match the raw English value first).
func tierNameTr(tr i18n.TranslateFunc, tier string) string {
	if tier == "" {
		return ""
	}
	return tr("tier_name:" + tier)
}

// classProfLabels' catalogKey reuses the items page's item_kind.* labels for
// armor/weapon/tool (same English words, already translated) and the
// character sheet's own saving_throws/skills_heading keys, rather than a
// second copy of any of them.
var classProfLabels = []struct{ kind, catalogKey string }{
	{"armor", "item_kind.armor"},
	{"weapon", "item_kind.weapon"},
	{"tool", "item_kind.tool"},
	{"saving_throw", "sheet.saving_throws"},
	{"skill", "sheet.skills_heading"},
	{"skill_choice", "partial_class_detail.skill_choices_kind"},
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
	Slug        string
	Name        string
	Level       sql.NullInt64
	Description string
}

type subclassFeatureRow struct {
	Slug        string
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
	Slug            string
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
	Slug          string
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
	// Locale is internal plumbing, not rendered by any template -- it lets
	// the sequential loadClassX(&c) helpers below overlay content
	// translations without each one needing its own locale parameter. Set
	// once, at the top of loadClassDetail.
	Locale          string
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
func (s *server) loadClassDetail(locale, slug string) (*classDetail, error) {
	var c classDetail
	c.Locale = locale
	err := s.rulesDB.QueryRow(`
		SELECT slug, name, hit_die, chakra_die, jutsu_tier, description, quick_build
		FROM classes WHERE slug = ?`, slug,
	).Scan(&c.Slug, &c.Name, &c.HitDie, &c.ChakraDie, &c.JutsuTier, &c.Description, &c.QuickBuild)
	if err != nil {
		return nil, err
	}
	if tr, err := i18n.Row(s.rulesDB, locale, "classes", c.Slug); err == nil {
		if v, ok := tr["name"]; ok {
			c.Name = v
		}
		if v, ok := tr["description"]; ok {
			c.Description = sql.NullString{String: v, Valid: true}
		}
		if v, ok := tr["quick_build"]; ok {
			c.QuickBuild = sql.NullString{String: v, Valid: true}
		}
	}

	if err := s.loadClassCasting(&c, locale); err != nil {
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

	c, err := s.loadClassDetail(localeFromContext(r.Context()), slug)
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
		s.renderCard(w, r, "class_detail.html", "class_detail_card", c)
		return
	}

	s.render(w, r, "class_detail.html", map[string]any{"Title": c.Name, "Class": c})
}

func (s *server) loadClassCasting(c *classDetail, locale string) error {
	tr := i18n.Translator(locale)
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
		label := abilityLabelTr(tr, ability)
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
	type profRow struct{ kind, value string }
	var raw []profRow
	for rows.Next() {
		var kind, value string
		var chooseN sql.NullInt64
		if err := rows.Scan(&kind, &value, &chooseN); err != nil {
			return err
		}
		raw = append(raw, profRow{kind, value})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	keys := make([]string, len(raw))
	for i, r := range raw {
		keys[i] = c.Slug + i18n.KeySeparator + r.kind + i18n.KeySeparator + r.value
	}
	translations, err := i18n.Batch(s.rulesDB, c.Locale, "class_proficiencies", keys)
	if err != nil {
		return err
	}
	byKind := map[string][]string{}
	for i, r := range raw {
		value := r.value
		if tr, ok := translations[keys[i]]; ok {
			if v, ok := tr["value"]; ok {
				value = v
			}
		}
		byKind[r.kind] = append(byKind[r.kind], value)
	}
	tr := i18n.Translator(c.Locale)
	for _, k := range classProfLabels {
		values := byKind[k.kind]
		if len(values) == 0 {
			continue
		}
		sort.Strings(values)
		c.Proficiencies = append(c.Proficiencies, profGroup{Kind: tr(k.catalogKey), Values: values})
	}
	return nil
}

func (s *server) loadClassEquipment(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT group_idx, choice_idx, description FROM class_equipment_options
		WHERE class_slug = ? ORDER BY group_idx, choice_idx`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()
	type equipRow struct {
		groupIdx, choiceIdx int
		description         string
	}
	var raw []equipRow
	for rows.Next() {
		var r equipRow
		if err := rows.Scan(&r.groupIdx, &r.choiceIdx, &r.description); err != nil {
			return err
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	keys := make([]string, len(raw))
	for i, r := range raw {
		keys[i] = c.Slug + i18n.KeySeparator + strconv.Itoa(r.groupIdx) + i18n.KeySeparator + strconv.Itoa(r.choiceIdx)
	}
	translations, err := i18n.Batch(s.rulesDB, c.Locale, "class_equipment_options", keys)
	if err != nil {
		return err
	}

	var lastGroup = -1
	for i, r := range raw {
		description := r.description
		if tr, ok := translations[keys[i]]; ok {
			if v, ok := tr["description"]; ok {
				description = v
			}
		}
		if r.groupIdx != lastGroup {
			c.Equipment = append(c.Equipment, equipmentGroup{})
			lastGroup = r.groupIdx
		}
		g := &c.Equipment[len(c.Equipment)-1]
		g.Choices = append(g.Choices, description)
	}
	return nil
}

// classResourceCol pairs a v_class_level_resources.resource_name value (name
// -- unavoidably English, since it's the DB lookup key, and this table isn't
// itself registered as translatable content) with the UI-catalog key its
// column header displays as. Some reuse a label already translated
// elsewhere (Creation Points, Malleable Mirages, Martial Techniques, Refined
// Ninjutsu all have their own catalog key from the character sheet/subclass
// popups already), the rest live under the class_resource_col.* namespace
// added specifically for this table.
type classResourceCol struct{ name, catalogKey string }

// classResourceColumns gives each class's named resource columns (e.g.
// Genjutsu Specialist's "Malleable Mirages", Puppet Master's five-tier
// system) in the exact left-to-right order printed in its book level table.
// This can't be derived from the data (class_level_resources carries no
// ordering column) and genuinely varies per class, so it's hand-transcribed
// here — verified directly against each class's page in the Orochimaru
// Observation Compendium (2026-07-19). Note book order is not always
// alphabetical: Weapon Specialist prints "Styles Known" before "Flurry Die".
var classResourceColumns = map[string][]classResourceCol{
	"class/cooking-nin":            {{"Cooking Die", "class_resource_col.cooking_die"}},
	"class/genjutsu-specialist":    {{"Malleable Mirages", "sheet.malleable_mirages"}},
	"class/hunter-nin":             {{"Lethal Attack", "class_resource_col.lethal_attack"}},
	"class/intelligence-operative": {{"Plans Known", "class_resource_col.plans_known"}, {"Brave Orders", "class_resource_col.brave_orders"}},
	"class/medical-nin": {
		{"Channeled Healing", "class_resource_col.channeled_healing"},
		{"Chakra Scalpel Charges", "class_resource_col.chakra_scalpel_charges"},
		{"Chakra Scalpel damage", "class_resource_col.chakra_scalpel_damage"},
	},
	"class/ninjutsu-specialist": {{"Refined Ninjutsu", "sheet.refined_ninjutsu"}, {"Efficient Moldings", "class_resource_col.efficient_moldings"}},
	"class/science-nin":         {{"Creation Points", "science_nin_creation_points.label"}},
	"class/taijutsu-specialist": {{"Martial Die", "class_resource_col.martial_die"}, {"Martial Techniques", "sheet.martial_techniques"}},
	"class/weapon-specialist":   {{"Styles Known", "class_resource_col.styles_known"}, {"Flurry Die", "class_resource_col.flurry_die"}},
	"class/puppet-master": {
		{"Wood Tier", "class_resource_col.wood_tier"}, {"Bronze Tier", "class_resource_col.bronze_tier"},
		{"Silver Tier", "class_resource_col.silver_tier"}, {"Gold Tier", "class_resource_col.gold_tier"},
		{"Platinum Tier", "class_resource_col.platinum_tier"},
	},
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

	levelKeys := make([]string, len(levels))
	for i, lr := range levels {
		levelKeys[i] = c.Slug + i18n.KeySeparator + strconv.Itoa(lr.Level)
	}
	levelTranslations, err := i18n.Batch(s.rulesDB, c.Locale, "class_levels", levelKeys)
	if err != nil {
		return err
	}
	for i := range levels {
		if tr, ok := levelTranslations[levelKeys[i]]; ok {
			if v, ok := tr["features_text"]; ok {
				levels[i].Features = sql.NullString{String: v, Valid: true}
			}
		}
	}

	resCols := classResourceColumns[c.Slug]
	resTr := i18n.Translator(c.Locale)
	c.ResourceColumns = make([]string, len(resCols))
	for i, col := range resCols {
		c.ResourceColumns[i] = resTr(col.catalogKey)
	}

	// One query per level for resources, same N+1 shape internal/export uses
	// for its ClassChartMaster CSV (20 levels max — not worth batching).
	for i := range levels {
		if len(resCols) == 0 {
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
		levels[i].ResourceValues = make([]string, len(resCols))
		for k, col := range resCols {
			levels[i].ResourceValues[k] = byName[col.name]
		}
	}
	c.Levels = levels
	return nil
}

func (s *server) loadClassFeatures(c *classDetail) error {
	rows, err := s.rulesDB.Query(`
		SELECT slug, name, level, description FROM v_class_features
		WHERE class_slug = ? ORDER BY (level IS NULL), level, sort_order`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var f classFeatureRow
		if err := rows.Scan(&f.Slug, &f.Name, &f.Level, &f.Description); err != nil {
			return err
		}
		c.Features = append(c.Features, f)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	slugs := make([]string, len(c.Features))
	for i, f := range c.Features {
		slugs[i] = f.Slug
	}
	translations, err := i18n.Batch(s.rulesDB, c.Locale, "class_features", slugs)
	if err != nil {
		return err
	}
	for i := range c.Features {
		tr := translations[c.Features[i].Slug]
		if v, ok := tr["name"]; ok {
			c.Features[i].Name = v
		}
		if v, ok := tr["description"]; ok {
			c.Features[i].Description = v
		}
	}
	return nil
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
		agr := subclassGroupRow{Slug: g.slug, DisplayName: g.displayName, SelectionLevels: g.selectionLevels, Description: g.description}
		if tr, err := i18n.Row(s.rulesDB, c.Locale, "subclass_groups", g.slug); err == nil {
			if v, ok := tr["display_name"]; ok {
				agr.DisplayName = v
			}
			if v, ok := tr["description"]; ok {
				agr.Description = sql.NullString{String: v, Valid: true}
			}
		}

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

		subSlugs := make([]string, len(subclasses))
		for i, a := range subclasses {
			subSlugs[i] = a.Slug
		}
		subTranslations, err := i18n.Batch(s.rulesDB, c.Locale, "subclasses", subSlugs)
		if err != nil {
			return err
		}
		for i := range subclasses {
			tr := subTranslations[subclasses[i].Slug]
			if v, ok := tr["name"]; ok {
				subclasses[i].Name = v
			}
			if v, ok := tr["description"]; ok {
				subclasses[i].Description = sql.NullString{String: v, Valid: true}
			}
		}
		sort.Slice(subclasses, func(i, k int) bool { return sortKey(subclasses[i].Name) < sortKey(subclasses[k].Name) })

		for i := range subclasses {
			featRows, err := s.rulesDB.Query(`
				SELECT slug, name, level, description FROM v_subclass_features
				WHERE subclass_slug = ? ORDER BY (level IS NULL), level, sort_order`, subclasses[i].Slug)
			if err != nil {
				return err
			}
			for featRows.Next() {
				var f subclassFeatureRow
				if err := featRows.Scan(&f.Slug, &f.Name, &f.Level, &f.Description); err != nil {
					featRows.Close()
					return err
				}
				subclasses[i].Features = append(subclasses[i].Features, f)
			}
			featRows.Close()
			if err := featRows.Err(); err != nil {
				return err
			}

			featSlugs := make([]string, len(subclasses[i].Features))
			for k, f := range subclasses[i].Features {
				featSlugs[k] = f.Slug
			}
			featTranslations, err := i18n.Batch(s.rulesDB, c.Locale, "subclass_features", featSlugs)
			if err != nil {
				return err
			}
			for k := range subclasses[i].Features {
				tr := featTranslations[subclasses[i].Features[k].Slug]
				if v, ok := tr["name"]; ok {
					subclasses[i].Features[k].Name = v
				}
				if v, ok := tr["description"]; ok {
					subclasses[i].Features[k].Description = v
				}
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
		SELECT o.slug, o.list_name, o.name, o.prerequisites, o.description, o.subclass_slug, a.name
		FROM class_options o LEFT JOIN subclasses a ON a.slug = o.subclass_slug
		WHERE o.class_slug = ? ORDER BY o.list_name, o.sort_order`, c.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rawOption struct {
		listName     string
		subclassSlug sql.NullString
		opt          classOptionRow
	}
	var raw []rawOption
	for rows.Next() {
		var ro rawOption
		if err := rows.Scan(&ro.opt.Slug, &ro.listName, &ro.opt.Name, &ro.opt.Prerequisites, &ro.opt.Description, &ro.subclassSlug, &ro.opt.SubclassName); err != nil {
			return err
		}
		raw = append(raw, ro)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	slugs := make([]string, len(raw))
	for i, ro := range raw {
		slugs[i] = ro.opt.Slug
	}
	translations, err := i18n.Batch(s.rulesDB, c.Locale, "class_options", slugs)
	if err != nil {
		return err
	}
	// listNameTr canonicalizes each list's translated heading by its raw
	// English name: DeepL translated list_name per option row rather than
	// once per list, so rows sharing one English list name can carry
	// slightly different Spanish wording. Using the first translation seen
	// for a given English name keeps every row in that list under one
	// heading instead of splintering it by wording variance.
	listNameTr := map[string]string{}
	for i := range raw {
		tr := translations[raw[i].opt.Slug]
		if v, ok := tr["name"]; ok {
			raw[i].opt.Name = v
		}
		if v, ok := tr["prerequisites"]; ok {
			raw[i].opt.Prerequisites = sql.NullString{String: v, Valid: true}
		}
		if v, ok := tr["description"]; ok {
			raw[i].opt.Description = v
		}
		if v, ok := tr["list_name"]; ok {
			if canon, ok := listNameTr[raw[i].listName]; ok {
				v = canon
			} else {
				listNameTr[raw[i].listName] = v
			}
			raw[i].listName = v
		}
	}

	bySubclass := map[string]*subclassRow{}
	for gi := range c.SubclassGroups {
		for si := range c.SubclassGroups[gi].Subclasses {
			bySubclass[c.SubclassGroups[gi].Subclasses[si].Slug] = &c.SubclassGroups[gi].Subclasses[si]
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
		if ro.subclassSlug.Valid {
			if sub, ok := bySubclass[ro.subclassSlug.String]; ok {
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

	slugs := make([]string, len(c.Feats))
	for i, f := range c.Feats {
		slugs[i] = f.Slug
	}
	translations, err := i18n.Batch(s.rulesDB, c.Locale, "feats", slugs)
	if err != nil {
		return err
	}
	for i := range c.Feats {
		tr := translations[c.Feats[i].Slug]
		if v, ok := tr["name"]; ok {
			c.Feats[i].Name = v
		}
		if v, ok := tr["prerequisites"]; ok {
			c.Feats[i].Prerequisites = sql.NullString{String: v, Valid: true}
		}
		if v, ok := tr["description"]; ok {
			c.Feats[i].Description = v
		}
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
	if tr, err := i18n.Row(s.rulesDB, c.Locale, "class_multiclass_rules", c.Slug); err == nil {
		if v, ok := tr["ability_prereq_text"]; ok {
			m.AbilityPrereqText = v
		}
		if v, ok := tr["proficiencies_text"]; ok {
			m.ProficienciesText = v
		}
		if v, ok := tr["jutsu_per_level_text"]; ok {
			m.JutsuPerLevelText = v
		}
	}
	c.MulticlassRules = &m
	return nil
}
