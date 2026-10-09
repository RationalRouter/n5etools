// Adversary Builder (Bingo Book p.6): loading rules migration 0082's charts
// into internal/encounter's BuilderTables, the step-7 trait pool, and the
// per-row stat blocks the roster and turn order show.
package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/encounter"
	"github.com/RationalRouter/n5e/internal/i18n"
)

// encounterKit is everything the encounter pages read from rules.db.
type encounterKit struct {
	encounter.Toolkit
	Transformations []encounter.TransformationExample
	Domains         []encounter.BattlefieldDomainExample
	// ClassMods are the sheet's class-mod tags (Cursed Seal, Sage Mode...),
	// sorted; Passives are its read-only step-8 passives.
	ClassMods []string
	Passives  []encounter.Trait
	// jutsuBySlug indexes Toolkit.Jutsu.
	jutsuBySlug map[string]encounter.Jutsu
}

// tagOption is one clan or affiliation choice on a picker.
type tagOption struct {
	Tag, Name, Description string
}

// tagLabel returns the catalog's display name for a builder tag, or the tag
// itself when it is a proper name with no catalog entry (most clans).
func tagLabel(tr i18n.TranslateFunc, prefix, tag string) string {
	key := prefix + tag
	if v := tr(key); v != key {
		return v
	}
	return tag
}

// clanLabel and affiliationLabel name a clan or affiliation tag for display.
func clanLabel(tr i18n.TranslateFunc, tag string) string {
	return tagLabel(tr, "adversary_clan:", tag)
}

func affiliationLabel(tr i18n.TranslateFunc, tag string) string {
	return tagLabel(tr, "adversary_affiliation:", tag)
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func signedInt(n int) string {
	if n >= 0 {
		return "+" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// loadBuilderTables reads rules migration 0082's charts, with Pack 1's
// class rows and the classification modifiers already loaded by the caller.
func loadBuilderTables(rulesDB *sql.DB, locale string, classes []encounter.Rank, mods []encounter.ClassificationModifier) (encounter.BuilderTables, []encounter.BuilderRole, error) {
	t := encounter.BuilderTables{
		Levels:          map[int]encounter.LevelStats{},
		TraitCounts:     map[int][4]int{},
		Ranks:           map[string]encounter.RankTier{},
		Roles:           map[string]encounter.BuilderRole{},
		Clans:           map[string]encounter.ClanMods{},
		Classes:         map[string]encounter.Rank{},
		ClassHP:         map[string]encounter.ClassHPMod{},
		Classifications: mods,
	}
	for _, c := range classes {
		t.Classes[c.Slug] = c
	}
	parseMods := func(s string) (out [6]int) {
		for i, p := range strings.Split(s, ",") {
			if i < 6 {
				out[i], _ = strconv.Atoi(strings.TrimSpace(p))
			}
		}
		return out
	}

	each := func(query string, scan func(*sql.Rows) error) error {
		rows, err := rulesDB.Query(query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}

	var roles []encounter.BuilderRole
	steps := []struct {
		query string
		scan  func(*sql.Rows) error
	}{
		{`SELECT level, prof, attack_bonus, damage_per_action, standard_mods, elite_mods, xp FROM adversary_level_stats`, func(r *sql.Rows) error {
			var ls encounter.LevelStats
			var std, elite string
			if err := r.Scan(&ls.Level, &ls.Prof, &ls.AttackBonus, &ls.DamagePerAction, &std, &elite, &ls.XP); err != nil {
				return err
			}
			ls.StandardMods, ls.EliteMods = parseMods(std), parseMods(elite)
			t.Levels[ls.Level] = ls
			return nil
		}},
		{`SELECT level, minion, standard, elite, solo FROM adversary_trait_counts`, func(r *sql.Rows) error {
			var level int
			var c [4]int
			if err := r.Scan(&level, &c[0], &c[1], &c[2], &c[3]); err != nil {
				return err
			}
			t.TraitCounts[level] = c
			return nil
		}},
		{`SELECT rank, hit_die, hit_avg, chakra_die, chakra_avg, defensive_bonus, to_hit_bonus, save_dc_bonus, bonus_damage FROM adversary_rank_tiers`, func(r *sql.Rows) error {
			var rt encounter.RankTier
			if err := r.Scan(&rt.Rank, &rt.HitDie, &rt.HitAvg, &rt.ChakraDie, &rt.ChakraAvg, &rt.Defensive, &rt.ToHit, &rt.SaveDC, &rt.BonusDamage); err != nil {
				return err
			}
			t.Ranks[rt.Rank] = rt
			return nil
		}},
		{`SELECT slug, name, ac_mod, save_mod, hp_mod, cp_mod, damage_mult, speed_bonus, trait_role_slug, sort_order FROM adversary_builder_roles ORDER BY sort_order`, func(r *sql.Rows) error {
			var ro encounter.BuilderRole
			if err := r.Scan(&ro.Slug, &ro.Tag, &ro.AC, &ro.Save, &ro.HP, &ro.CP, &ro.DamageMult, &ro.Speed, &ro.TraitRoleSlug, &ro.SortOrder); err != nil {
				return err
			}
			ro.Name = ro.Tag
			roles = append(roles, ro)
			return nil
		}},
		{`SELECT clan_tag, hp_mod, cp_mod, ninjutsu_atk, ninjutsu_dc, genjutsu_atk, genjutsu_dc, taijutsu_atk, taijutsu_dc FROM adversary_clan_mods`, func(r *sql.Rows) error {
			c := encounter.ClanMods{Atk: map[string]int{}, DC: map[string]int{}}
			var na, nd, ga, gd, ta, td int
			if err := r.Scan(&c.Tag, &c.HP, &c.CP, &na, &nd, &ga, &gd, &ta, &td); err != nil {
				return err
			}
			c.Atk["ninjutsu"], c.DC["ninjutsu"] = na, nd
			c.Atk["genjutsu"], c.DC["genjutsu"] = ga, gd
			c.Atk["taijutsu"], c.DC["taijutsu"] = ta, td
			t.Clans[c.Tag] = c
			return nil
		}},
		{`SELECT tag, stat, value, note_text FROM adversary_auto_bonuses ORDER BY tag, stat`, func(r *sql.Rows) error {
			var b encounter.AutoBonus
			if err := r.Scan(&b.Tag, &b.Stat, &b.Value, &b.Note); err != nil {
				return err
			}
			t.AutoBonuses = append(t.AutoBonuses, b)
			return nil
		}},
		{`SELECT class_slug, base, per_player, minion_flat FROM adversary_class_hp_mods`, func(r *sql.Rows) error {
			var slug string
			var m encounter.ClassHPMod
			if err := r.Scan(&slug, &m.Base, &m.PerPlayer, &m.MinionFlat); err != nil {
				return err
			}
			t.ClassHP[slug] = m
			return nil
		}},
		{`SELECT class_slug, jutsu_rank, level_min, count FROM adversary_jutsu_known`, func(r *sql.Rows) error {
			var j encounter.JutsuKnownRow
			if err := r.Scan(&j.ClassSlug, &j.JutsuRank, &j.LevelMin, &j.Count); err != nil {
				return err
			}
			t.JutsuKnown = append(t.JutsuKnown, j)
			return nil
		}},
	}
	for _, st := range steps {
		if err := each(st.query, st.scan); err != nil {
			return t, nil, err
		}
	}

	slugs := make([]string, len(roles))
	for i, ro := range roles {
		slugs[i] = ro.Slug
	}
	if tr, err := i18n.Batch(rulesDB, locale, "adversary_builder_roles", slugs); err == nil {
		for i := range roles {
			if v, ok := tr[roles[i].Slug]["name"]; ok {
				roles[i].Name = v
			}
		}
	}
	for _, ro := range roles {
		t.Roles[ro.Slug] = ro
	}
	return t, roles, nil
}

// loadSheetTraits reads every community sheet trait, translated: the
// step-7 traits and the read-only passives alike.
func loadSheetTraits(rulesDB *sql.DB, locale string) ([]encounter.Trait, error) {
	rows, err := rulesDB.Query(`
		SELECT slug, category, category_tag, name, rank, description
		FROM adversary_builder_sheet_traits ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var traits []encounter.Trait
	var slugs []string
	for rows.Next() {
		t := encounter.Trait{Source: "sheet"}
		if err := rows.Scan(&t.Slug, &t.Category, &t.Tag, &t.Name, &t.Rank, &t.Description); err != nil {
			return nil, err
		}
		traits = append(traits, t)
		slugs = append(slugs, t.Slug)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if tr, err := i18n.Batch(rulesDB, locale, "adversary_builder_sheet_traits", slugs); err == nil {
		for i := range traits {
			if v, ok := tr[traits[i].Slug]["name"]; ok {
				traits[i].Name = v
			}
			if v, ok := tr[traits[i].Slug]["description"]; ok {
				traits[i].Description = v
			}
		}
	}
	return traits, nil
}

// loadEncounterKit loads every rules.db table the encounter pages use, for
// the request's locale.
func (s *server) loadEncounterKit(r *http.Request) (encounterKit, error) {
	locale := localeFromContext(r.Context())
	var kit encounterKit
	classes, traits, slots, err := loadBingoBookToolkit(s.rulesDB, locale)
	if err != nil {
		return kit, err
	}
	mods, transformations, domains, err := loadClassificationToolkit(s.rulesDB, locale)
	if err != nil {
		return kit, err
	}
	tables, roles, err := loadBuilderTables(s.rulesDB, locale, classes, mods)
	if err != nil {
		return kit, err
	}
	allSheetTraits, err := loadSheetTraits(s.rulesDB, locale)
	if err != nil {
		return kit, err
	}
	jutsu, err := loadEncounterJutsu(s.rulesDB, locale)
	if err != nil {
		return kit, err
	}

	// The sheet restates some Pack 1 role traits in its own wording; where
	// a role's pool would hold both, the Pack 1 version is kept.
	official := map[string]bool{}
	for _, t := range traits {
		official[t.RoleSlug+"|"+slugLeaf(t.Slug)] = true
	}
	roleByTag := map[string]string{}
	for _, ro := range roles {
		roleByTag[ro.Tag] = ro.TraitRoleSlug
	}

	var sheetTraits []encounter.Trait
	affiliations, classMods := map[string]bool{}, map[string]bool{}
	for _, t := range allSheetTraits {
		if t.Category == "passive" {
			kit.Passives = append(kit.Passives, t)
			continue
		}
		if t.Category == "role" && official[roleByTag[t.Tag]+"|"+slugLeaf(t.Slug)] {
			continue
		}
		sheetTraits = append(sheetTraits, t)
		switch {
		case t.Category == "affiliation" && t.Tag != encounter.NoAffiliation:
			affiliations[t.Tag] = true
		case t.Category == "class_mod":
			classMods[t.Tag] = true
		}
	}
	for tag := range classMods {
		kit.ClassMods = append(kit.ClassMods, tag)
	}
	sort.Strings(kit.ClassMods)
	kit.Affiliations = make([]string, 0, len(affiliations))
	for tag := range affiliations {
		kit.Affiliations = append(kit.Affiliations, tag)
	}
	sort.Strings(kit.Affiliations)
	for tag := range tables.Clans {
		if tag != encounter.NoClan {
			kit.Clans = append(kit.Clans, tag)
		}
	}
	sort.Strings(kit.Clans)

	tr := i18n.Translator(locale)
	kit.Classes = classes
	kit.Roles = roles
	kit.Traits = append(traits, sheetTraits...)
	kit.Jutsu = jutsu
	kit.jutsuBySlug = make(map[string]encounter.Jutsu, len(jutsu))
	for _, j := range jutsu {
		kit.jutsuBySlug[j.Slug] = j
	}
	kit.Slots = slots
	kit.Tables = tables
	kit.Name = func(level int, class encounter.Rank, role encounter.BuilderRole) string {
		return tr("encounters.generated_name", level, class.Name, role.Name)
	}
	kit.Transformations = transformations
	kit.Domains = domains
	return kit, nil
}

// slugLeaf returns the last path segment of a slug.
func slugLeaf(slug string) string {
	return slug[strings.LastIndexByte(slug, '/')+1:]
}

// encounterKitOr500 loads the kit, answering 500 itself on failure.
func (s *server) encounterKitOr500(w http.ResponseWriter, r *http.Request) (encounterKit, bool) {
	kit, err := s.loadEncounterKit(r)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load encounter kit:", err)
		return kit, false
	}
	return kit, true
}

func (k encounterKit) class(slug string) (encounter.Rank, bool) {
	return rankBySlug(k.Classes, slug)
}

func (k encounterKit) className(slug string) string {
	if rk, ok := k.class(slug); ok {
		return rk.Name
	}
	return slug
}

func (k encounterKit) roleName(slug string) string {
	if ro, ok := k.Tables.Roles[slug]; ok {
		return ro.Name
	}
	return slug
}

// clanOptions lists the generator's clan picks: none, random, then every
// clan with a description of its modifiers.
func (k encounterKit) clanOptions(tr i18n.TranslateFunc) []tagOption {
	out := []tagOption{
		{Tag: "", Name: clanLabel(tr, encounter.NoClan), Description: tr("encounters.clan_none_hint")},
		{Tag: encounter.RandomTag, Name: tr("encounters.tag_random"), Description: tr("encounters.clan_random_hint")},
	}
	for _, tag := range k.Clans {
		c := k.Tables.Clans[tag]
		out = append(out, tagOption{Tag: tag, Name: clanLabel(tr, tag), Description: tr("encounters.clan_mods_hint",
			signedFloat(c.HP), signedFloat(c.CP),
			signedInt(c.Atk["ninjutsu"]), signedInt(c.DC["ninjutsu"]),
			signedInt(c.Atk["genjutsu"]), signedInt(c.DC["genjutsu"]),
			signedInt(c.Atk["taijutsu"]), signedInt(c.DC["taijutsu"]))})
	}
	sort.SliceStable(out[2:], func(i, j int) bool { return out[2+i].Name < out[2+j].Name })
	return out
}

// affiliationOptions lists the generator's affiliation picks, each
// described by the traits it adds to the pool.
func (k encounterKit) affiliationOptions(tr i18n.TranslateFunc) []tagOption {
	out := []tagOption{
		{Tag: "", Name: affiliationLabel(tr, encounter.NoAffiliation), Description: tr("encounters.affiliation_none_hint")},
		{Tag: encounter.RandomTag, Name: tr("encounters.tag_random"), Description: tr("encounters.affiliation_random_hint")},
	}
	for _, tag := range k.Affiliations {
		var names []string
		for _, t := range k.Traits {
			if t.Category == "affiliation" && t.Tag == tag {
				names = append(names, t.Name)
			}
		}
		out = append(out, tagOption{Tag: tag, Name: affiliationLabel(tr, tag), Description: tr("encounters.affiliation_traits_hint", strings.Join(names, ", "))})
	}
	sort.SliceStable(out[2:], func(i, j int) bool { return out[2+i].Name < out[2+j].Name })
	return out
}

// fill completes a roster view for rendering: the pickers' rows, and each
// adversary's display names and computed stat block for the encounter's
// party size.
func (k encounterKit) fill(view *encounterRosterView, tr i18n.TranslateFunc) {
	view.Ranks = k.Classes
	view.Transformations = k.Transformations
	view.BattlefieldDomains = k.Domains
	for i := range view.Adversaries {
		k.fillRow(&view.Adversaries[i], view.PartySize, tr)
	}
}

// fillRow sets one roster row's display names and computed stat block.
func (k encounterKit) fillRow(a *encounterAdversaryView, players int, tr i18n.TranslateFunc) {
	a.GeneratedAdversary = a.GeneratedAdversary.Normalize()
	a.Stats = encounter.Compute(a.Choices(players), k.Tables)
	a.ClassName = k.className(a.RankSlug)
	a.RoleName = k.roleName(a.RoleSlug)
	a.ClanName = clanLabel(tr, a.ClanTag)
	a.AffiliationName = affiliationLabel(tr, a.AffiliationTag)
	a.Discipline = encounter.RoleDiscipline(a.RoleSlug)
	// Stored jutsu names are in the locale they were saved in; show the
	// current locale's where the jutsu still exists.
	jutsu := make([]encounter.GeneratedJutsu, len(a.Jutsu))
	for i, j := range a.Jutsu {
		if cur, ok := k.jutsuBySlug[j.Slug]; ok {
			j.Name, j.CostText = cur.Name, cur.CostText
		}
		jutsu[i] = j
	}
	a.Jutsu = jutsu
}

// filledRoster reads a saved encounter's roster with every row filled
// (encounterKit.fill), and the kit that filled it.
func (s *server) filledRoster(r *http.Request, encID int64) (encounterRosterView, encounterKit, error) {
	view, err := loadEncounterRosterView(s.charDB, encID)
	if err != nil {
		return view, encounterKit{}, err
	}
	kit, err := s.loadEncounterKit(r)
	if err != nil {
		return view, kit, err
	}
	kit.fill(&view, i18n.Translator(localeFromContext(r.Context())))
	return view, kit, nil
}

// encounterStats returns every saved adversary's computed stat block in an
// encounter, by adversary id.
func (s *server) encounterStats(r *http.Request, encID int64) (map[int64]encounter.StatBlock, error) {
	view, _, err := s.filledRoster(r, encID)
	if err != nil {
		return nil, err
	}
	return rosterStats(view), nil
}

// abilityStatRow is one ability's modifier and save in a stat block.
type abilityStatRow struct {
	Key         string // str..cha
	Value, Save int
	SaveProf    bool
}

// StatAbilityRows lists the six abilities' modifiers and saves.
func (a encounterAdversaryView) StatAbilityRows() []abilityStatRow {
	out := make([]abilityStatRow, len(encounter.Abilities))
	for i, ab := range encounter.Abilities {
		out[i] = abilityStatRow{Key: ab, Value: a.Stats.Abilities[i], Save: a.Stats.Saves[i], SaveProf: a.Stats.SaveProf[i]}
	}
	return out
}

// abilityStat is one ability's save (or modifier) on a roster row.
type abilityStat struct {
	Ability string // str..cha
	Value   int
	Prof    bool
}

// SaveRows lists the six saving throws, for the roster's saves tooltip.
func (a encounterAdversaryView) SaveRows() []abilityStat {
	out := make([]abilityStat, len(encounter.Abilities))
	for i, ab := range encounter.Abilities {
		out[i] = abilityStat{Ability: ab, Value: a.Stats.Saves[i], Prof: a.Stats.SaveProf[i]}
	}
	return out
}

// advAttackRow is one attack bonus and save DC on a roster row; Key is
// "weapon" or a discipline.
type advAttackRow struct {
	Key        string
	Attack, DC int
}

// AttackRows lists the weapon attack and each discipline's attack and DC.
func (a encounterAdversaryView) AttackRows() []advAttackRow {
	out := []advAttackRow{{Key: "weapon", Attack: a.Stats.WeaponAttack}}
	for _, d := range encounter.Disciplines {
		out = append(out, advAttackRow{Key: d, Attack: a.Stats.Attack[d], DC: a.Stats.DC[d]})
	}
	return out
}

// MainAttack is the attack the roster shows up front: the role's
// discipline for a Caster or Striker, the weapon attack otherwise.
func (a encounterAdversaryView) MainAttack() advAttackRow {
	for _, row := range a.AttackRows() {
		if row.Key == a.Discipline {
			return row
		}
	}
	return a.AttackRows()[0]
}
