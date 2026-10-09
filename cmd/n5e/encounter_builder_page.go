// Adversary Builder page (Bingo Book p.6, "Build Your Adversary"): one form
// with the book's ten steps in order and a live stat block. Every change
// posts the form to /encounters/builder/preview, which answers with the
// parts of the page that depend on the choices (encounter-builder.js swaps
// them in place); Save creates or replaces the saved adversary.
package main

import (
	"database/sql"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/encounter"
	"github.com/RationalRouter/n5e/internal/i18n"
)

// builderOption is one <option> of a builder picker.
type builderOption struct {
	Value, Name, Description string
	Selected                 bool
}

type builderOptions []builderOption

// Current is the selected option's description, for the picker's tooltip
// before any change event.
func (o builderOptions) Current() string {
	for _, opt := range o {
		if opt.Selected {
			return opt.Description
		}
	}
	if len(o) > 0 {
		return o[0].Description
	}
	return ""
}

// builderAbilityRow is one ability in step 3: the GM's pinned modifier
// ("" when assigned automatically), the values it can be pinned to, and the
// result.
type builderAbilityRow struct {
	Key         string
	Pinned      string
	Options     []int
	Value       int
	SaveChecked bool // proficiency picked by the GM
	SaveProf    bool // proficiency in effect
	Save        int
}

// builderTraitOption is one checkbox of the step-7 checklist.
type builderTraitOption struct {
	Slug, Name, Rank, Description string
	Checked                       bool
}

// builderTraitGroup is one category of the step-7 checklist. Key "outside"
// holds chosen traits the current choices no longer put in the pool.
type builderTraitGroup struct {
	Key     string
	Traits  []builderTraitOption
	Checked int
}

// builderPassive is one read-only step-8 passive.
type builderPassive struct {
	Source, Name, Description string
}

// builderJutsuOption is one checkbox of the step-9 list.
type builderJutsuOption struct {
	Slug, Name, Rank, Classification, CostText, Description string
	Checked                                                 bool
}

// builderJutsuFilter narrows step 9's list of jutsu to add: a class from
// encounter.JutsuClasses, a rank and a name search, each "" for any.
type builderJutsuFilter struct {
	Class, Rank, Query string
}

// builderJutsuShown caps step 9's list of jutsu to add; the filter narrows
// the rest.
const builderJutsuShown = 40

// builderJutsuDescriptionLen caps a jutsu's description in its tooltip.
const builderJutsuDescriptionLen = 400

// builderView is what encounters_builder.html renders.
type builderView struct {
	EncounterID   int64
	EncounterName string
	AdversaryID   int64 // 0 for a new adversary
	Players       int

	Row encounterAdversaryView
	// DisplayName is the typed name, or the name a blank one saves as.
	DisplayName string

	RankOptions, ClassOptions, ClanOptions, AffiliationOptions builderOptions
	ClassModOptions, RoleOptions, ArmorOptions                 builderOptions
	ClassificationOptions                                      builderOptions
	TransformationOptions, DomainOptions                       builderOptions

	LevelMin, LevelMax int
	Array              []int
	ArrayText          string
	AbilityRows        []builderAbilityRow
	PinsOffArray       bool

	TraitGroups  []builderTraitGroup
	TraitsChosen int
	Passives     []builderPassive
	Notes        []string
	IsSolo       bool

	JutsuChosen       []builderJutsuOption
	JutsuCandidates   []builderJutsuOption
	JutsuMatches      int // jutsu matching the filter, before the cap
	JutsuFilter       builderJutsuFilter
	JutsuClassOptions builderOptions
	JutsuRankOptions  builderOptions
	JutsuMaxRank      string // the highest rank the quotas allow; "" for none
	JutsuRankOver     bool   // the chosen jutsu don't fit the per-rank quotas
}

// builderEncounter is the saved encounter an adversary is built for.
type builderEncounter struct {
	ID         int64
	Name       string
	PartySize  int
	PartyLevel int
}

func loadBuilderEncounter(charDB *sql.DB, id int64) (builderEncounter, error) {
	e := builderEncounter{ID: id}
	err := charDB.QueryRow(`SELECT name, party_size, party_level FROM encounters WHERE id = ?`, id).
		Scan(&e.Name, &e.PartySize, &e.PartyLevel)
	return e, err
}

// builderEncounterOr404 reads an encounter by its id, answering 404 or
// 500 itself.
func (s *server) builderEncounterOr404(w http.ResponseWriter, r *http.Request, raw string) (builderEncounter, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return builderEncounter{}, false
	}
	e, err := loadBuilderEncounter(s.charDB, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return e, false
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load encounter for builder:", err)
		return e, false
	}
	return e, true
}

// levelRange is the lowest and highest level the builder charts cover.
func (k encounterKit) levelRange() (lo, hi int) {
	lo, hi = 1, 20
	first := true
	for l := range k.Tables.Levels {
		if first {
			lo, hi, first = l, l, false
			continue
		}
		lo, hi = min(lo, l), max(hi, l)
	}
	return lo, hi
}

// defaultClass is the class a new adversary starts as: Standard, or the
// first class when there is none.
func (k encounterKit) defaultClass() encounter.Rank {
	for _, c := range k.Classes {
		if encounter.ClassKind(c.Slug) == "standard" {
			return c
		}
	}
	if len(k.Classes) > 0 {
		return k.Classes[0]
	}
	return encounter.Rank{}
}

// newBuilderAdversary is a new adversary's starting choices.
func (k encounterKit) newBuilderAdversary(level int) encounter.GeneratedAdversary {
	adv := encounter.GeneratedAdversary{
		RankSlug: k.defaultClass().Slug,
		Level:    level,
		AdvRank:  encounter.RankForLevel(level),
	}
	if len(k.Roles) > 0 {
		adv.RoleSlug = k.Roles[0].Slug
	}
	return adv.Normalize()
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// parseBuilderForm reads the builder's choices from the form. Unknown
// values fall back to defaults; prev supplies snapshots for chosen traits
// and jutsu the catalog no longer has (the zero value for a new adversary).
func parseBuilderForm(r *http.Request, kit encounterKit, partyLevel int, prev encounter.GeneratedAdversary) encounter.GeneratedAdversary {
	lo, hi := kit.levelRange()
	level, err := strconv.Atoi(r.FormValue("level"))
	if err != nil {
		level = partyLevel
	}
	level = min(max(level, lo), hi)

	adv := encounter.GeneratedAdversary{
		Name:           strings.TrimSpace(r.FormValue("name")),
		RankSlug:       r.FormValue("class_slug"),
		Level:          level,
		AdvRank:        r.FormValue("adv_rank"),
		RoleSlug:       r.FormValue("role_slug"),
		ClanTag:        r.FormValue("clan_tag"),
		AffiliationTag: r.FormValue("affiliation_tag"),
		ClassModTag:    r.FormValue("class_mod_tag"),
		Armor:          r.FormValue("armor"),
		Classification: r.FormValue("classification"),
	}
	if _, ok := kit.class(adv.RankSlug); !ok {
		adv.RankSlug = kit.defaultClass().Slug
	}
	if encounter.RankIndex(adv.AdvRank) < 0 {
		adv.AdvRank = encounter.RankForLevel(level)
	}
	if _, ok := kit.Tables.Roles[adv.RoleSlug]; !ok && len(kit.Roles) > 0 {
		adv.RoleSlug = kit.Roles[0].Slug
	}
	if !containsString(kit.Clans, adv.ClanTag) {
		adv.ClanTag = encounter.NoClan
	}
	if !containsString(kit.Affiliations, adv.AffiliationTag) {
		adv.AffiliationTag = encounter.NoAffiliation
	}
	if !containsString(kit.ClassMods, adv.ClassModTag) {
		adv.ClassModTag = ""
	}
	if !containsString(encounter.ArmorKinds, adv.Armor) {
		adv.Armor = encounter.DefaultArmor(adv.RoleSlug)
	}
	if adv.Classification != "iconic" && adv.Classification != "epic" {
		adv.Classification = ""
	}

	for i, ab := range encounter.Abilities {
		if v, err := strconv.Atoi(r.FormValue("ab_" + ab)); err == nil {
			adv.AbilityMods[i] = &v
		}
		if r.FormValue("save_"+ab) != "" {
			adv.SaveProfs = append(adv.SaveProfs, ab)
		}
	}

	adj := func(name string) int {
		v, _ := strconv.Atoi(r.FormValue(name))
		return v
	}
	adv.Adjust = encounter.Adjustments{
		HP: adj("adj_hp"), CP: adj("adj_cp"), AC: adj("adj_ac"),
		Init: adj("adj_init"), Speed: adj("adj_speed"), DR: adj("adj_dr"),
	}

	if encounter.ClassKind(adv.RankSlug) == "solo" {
		tSlug, dSlug := r.FormValue("transformation_slug"), r.FormValue("battlefield_domain_slug")
		tName, dName := "", ""
		for _, t := range kit.Transformations {
			if t.Slug == tSlug {
				tName = t.Name
			}
		}
		for _, d := range kit.Domains {
			if d.Slug == dSlug {
				dName = d.Name
			}
		}
		if tName == "" {
			tSlug = ""
		}
		if dName == "" {
			dSlug = ""
		}
		adv = encounter.WithNarrativeAttachment(adv, tSlug, tName, dSlug, dName)
	}

	bySlug := make(map[string]encounter.Trait, len(kit.Traits))
	for _, t := range kit.Traits {
		bySlug[t.Slug] = t
	}
	prevBySlug := map[string]encounter.GeneratedTrait{}
	for _, t := range prev.Traits {
		prevBySlug[t.Slug] = t
	}
	seen := map[string]bool{}
	for _, slug := range r.Form["trait"] {
		if seen[slug] {
			continue
		}
		seen[slug] = true
		if t, ok := bySlug[slug]; ok {
			adv.Traits = append(adv.Traits, encounter.GeneratedTrait{
				Slug: t.Slug, Name: t.Name, Rank: t.Rank, Description: t.Description, Source: t.Source, Category: t.Category,
			})
		} else if t, ok := prevBySlug[slug]; ok {
			adv.Traits = append(adv.Traits, t)
		}
	}

	prevJutsu := map[string]encounter.GeneratedJutsu{}
	for _, j := range prev.Jutsu {
		prevJutsu[j.Slug] = j
	}
	var jutsu []encounter.Jutsu
	for _, slug := range r.Form["jutsu"] {
		if seen["jutsu "+slug] {
			continue
		}
		seen["jutsu "+slug] = true
		if j, ok := kit.jutsuBySlug[slug]; ok {
			jutsu = append(jutsu, j)
		} else if j, ok := prevJutsu[slug]; ok {
			jutsu = append(jutsu, encounter.Jutsu{Slug: j.Slug, Name: j.Name, Rank: j.Rank, Classification: j.Classification, CostText: j.CostText})
		}
	}
	encounter.SortJutsu(jutsu)
	for _, j := range jutsu {
		adv.Jutsu = append(adv.Jutsu, encounter.GeneratedJutsu{Slug: j.Slug, Name: j.Name, Rank: j.Rank, Classification: j.Classification, CostText: j.CostText})
	}
	return adv.Normalize()
}

// parseJutsuFilter reads step 9's filter fields.
func parseJutsuFilter(r *http.Request) builderJutsuFilter {
	f := builderJutsuFilter{
		Class: r.FormValue("jutsu_class"),
		Rank:  r.FormValue("jutsu_rank"),
		Query: strings.TrimSpace(r.FormValue("jutsu_q")),
	}
	if !containsString(encounter.JutsuClasses, f.Class) {
		f.Class = ""
	}
	if encounter.RankIndex(f.Rank) < 0 {
		f.Rank = ""
	}
	return f
}

// defaultJutsuFilter is step 9's filter when the page opens: a Caster's or
// Striker's own discipline, otherwise everything.
func defaultJutsuFilter(adv encounter.GeneratedAdversary) builderJutsuFilter {
	return builderJutsuFilter{Class: encounter.RoleDiscipline(adv.RoleSlug)}
}

// jutsuStep fills step 9: adv's chosen jutsu, and the eligible jutsu that
// match f, highest rank first, up to builderJutsuShown. The pool stops at
// the highest rank adv's quotas allow, which the level sets and which can
// sit below adv's own Rank.
func (k encounterKit) jutsuStep(v *builderView, adv encounter.GeneratedAdversary, f builderJutsuFilter, tr i18n.TranslateFunc) {
	v.JutsuMaxRank = encounter.MaxJutsuRank(v.Row.Stats.Jutsu)
	maxIdx := encounter.RankIndex(v.JutsuMaxRank)
	if encounter.RankIndex(f.Rank) > maxIdx {
		f.Rank = ""
	}
	v.JutsuFilter = f
	for _, c := range encounter.JutsuClasses {
		v.JutsuClassOptions = append(v.JutsuClassOptions, builderOption{Value: c, Name: tr("encounters.builder_jutsu_class_" + c), Selected: c == f.Class})
	}
	for _, letter := range encounter.AdversaryRanks[:maxIdx+1] {
		v.JutsuRankOptions = append(v.JutsuRankOptions, builderOption{Value: letter, Name: tr("encounters.builder_rank_option", letter), Selected: letter == f.Rank})
	}

	chosen := map[string]bool{}
	var ranks []string
	for _, j := range adv.Jutsu {
		chosen[j.Slug] = true
		ranks = append(ranks, j.Rank)
		v.JutsuChosen = append(v.JutsuChosen, builderJutsuOption{
			Slug: j.Slug, Name: j.Name, Rank: j.Rank, Classification: j.Classification, CostText: j.CostText, Checked: true,
		})
	}
	v.JutsuRankOver = !encounter.JutsuWithinQuotas(v.Row.Stats.Jutsu, ranks)
	q := strings.ToLower(f.Query)
	for ri := maxIdx; ri >= 0; ri-- {
		k.jutsuCandidates(v, adv, f, q, chosen, encounter.AdversaryRanks[ri])
	}
}

// jutsuCandidates adds the eligible jutsu of one rank that match f to v.
func (k encounterKit) jutsuCandidates(v *builderView, adv encounter.GeneratedAdversary, f builderJutsuFilter, q string, chosen map[string]bool, rank string) {
	for _, j := range k.Jutsu {
		switch {
		case j.Rank != rank:
			continue
		case chosen[j.Slug], !encounter.JutsuEligible(j, adv):
			continue
		case f.Class != "" && !encounter.JutsuInClass(j, f.Class):
			continue
		case f.Rank != "" && j.Rank != f.Rank:
			continue
		case q != "" && !strings.Contains(strings.ToLower(j.Name), q):
			continue
		}
		v.JutsuMatches++
		if len(v.JutsuCandidates) < builderJutsuShown {
			v.JutsuCandidates = append(v.JutsuCandidates, builderJutsuOption{
				Slug: j.Slug, Name: j.Name, Rank: j.Rank, Classification: j.Classification, CostText: j.CostText,
			})
		}
	}
}

// loadEncounterJutsu reads every ranked jutsu, with its name and cost in
// locale. Descriptions are left out: only the few the builder shows are
// read, by builderJutsuDescriptions.
func loadEncounterJutsu(rulesDB *sql.DB, locale string) ([]encounter.Jutsu, error) {
	rows, err := rulesDB.Query(`
		SELECT j.slug, COALESCE(n.value, j.name), j.rank, j.classification,
		       COALESCE(c.value, j.cost_text, ''), COALESCE(j.clan_slug, '')
		FROM jutsu j
		LEFT JOIN content_translations n
		       ON n.table_name = 'jutsu' AND n.slug = j.slug AND n.locale = ? AND n.field = 'name'
		LEFT JOIN content_translations c
		       ON c.table_name = 'jutsu' AND c.slug = j.slug AND c.locale = ? AND c.field = 'cost_text'
		WHERE j.rank IN ('E', 'D', 'C', 'B', 'A', 'S')`, locale, locale)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []encounter.Jutsu
	for rows.Next() {
		var j encounter.Jutsu
		if err := rows.Scan(&j.Slug, &j.Name, &j.Rank, &j.Classification, &j.CostText, &j.ClanSlug); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	encounter.SortJutsu(out)
	return out, nil
}

// fillJutsuDescriptions reads the descriptions of the jutsu step 9 shows,
// in locale, shortened for their tooltips.
func fillJutsuDescriptions(rulesDB *sql.DB, locale string, v *builderView) error {
	var slugs []string
	for _, list := range [][]builderJutsuOption{v.JutsuChosen, v.JutsuCandidates} {
		for _, j := range list {
			slugs = append(slugs, j.Slug)
		}
	}
	if len(slugs) == 0 {
		return nil
	}
	args := make([]any, len(slugs))
	for i, slug := range slugs {
		args[i] = slug
	}
	rows, err := rulesDB.Query(`SELECT slug, COALESCE(description, '') FROM jutsu WHERE slug IN (?`+strings.Repeat(", ?", len(slugs)-1)+`)`, args...)
	if err != nil {
		return err
	}
	desc := map[string]string{}
	for rows.Next() {
		var slug, d string
		if err := rows.Scan(&slug, &d); err != nil {
			rows.Close()
			return err
		}
		desc[slug] = d
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if tr, err := i18n.Batch(rulesDB, locale, "jutsu", slugs); err == nil {
		for slug, fields := range tr {
			if d, ok := fields["description"]; ok {
				desc[slug] = d
			}
		}
	}
	for _, list := range [][]builderJutsuOption{v.JutsuChosen, v.JutsuCandidates} {
		for i := range list {
			list[i].Description = shortenText(desc[list[i].Slug], builderJutsuDescriptionLen)
		}
	}
	return nil
}

// shortenText cuts s to at most n bytes at a word boundary, adding an
// ellipsis when it cut anything.
func shortenText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndexByte(s[:n], ' ')
	if cut <= 0 {
		cut = n
	}
	return strings.TrimRight(s[:cut], " ,.;:") + "…"
}

// builderView builds the page for adv, an adversary of encounter enc.
func (k encounterKit) builderView(enc builderEncounter, advID int64, adv encounter.GeneratedAdversary, jf builderJutsuFilter, tr i18n.TranslateFunc) builderView {
	v := builderView{
		EncounterID:   enc.ID,
		EncounterName: enc.Name,
		AdversaryID:   advID,
		Players:       enc.PartySize,
		IsSolo:        encounter.ClassKind(adv.RankSlug) == "solo",
	}
	v.Row = encounterAdversaryView{GeneratedAdversary: adv, ID: advID}
	k.fillRow(&v.Row, enc.PartySize, tr)
	adv = v.Row.GeneratedAdversary
	stats := v.Row.Stats
	v.DisplayName = adv.Name
	if v.DisplayName == "" {
		rk, _ := k.class(adv.RankSlug)
		v.DisplayName = encounter.DefaultName(k.Toolkit, rk, k.Tables.Roles[adv.RoleSlug], adv.Level)
	}
	v.LevelMin, v.LevelMax = k.levelRange()

	// Step 1: Rank.
	for _, letter := range encounter.AdversaryRanks {
		rt := k.Tables.Ranks[letter]
		v.RankOptions = append(v.RankOptions, builderOption{
			Value: letter, Name: tr("encounters.builder_rank_option", letter), Selected: letter == adv.AdvRank,
			Description: tr("encounters.builder_rank_hint", rt.HitDie, rt.ChakraDie,
				signedInt(rt.Defensive), signedInt(rt.ToHit), signedInt(rt.SaveDC), signedInt(rt.BonusDamage)),
		})
	}

	// Step 2: Class.
	for _, c := range k.Classes {
		hp := k.Tables.ClassHP[c.Slug]
		var hpText string
		switch {
		case hp.MinionFlat:
			hpText = tr("encounters.builder_class_hp_minion")
		case hp.PerPlayer != 0:
			hpText = tr("encounters.builder_class_hp_players", "+"+ftoa(hp.Base), "+"+ftoa(hp.PerPlayer))
		default:
			hpText = tr("encounters.builder_class_hp", "+"+ftoa(hp.Base))
		}
		v.ClassOptions = append(v.ClassOptions, builderOption{
			Value: c.Slug, Name: c.Name, Selected: c.Slug == adv.RankSlug,
			Description: tr("encounters.builder_class_hint", hpText,
				signedInt(c.ACBonus), signedInt(c.SaveBonus), signedInt(c.SaveDCBonus), signedInt(c.InitBonus)),
		})
	}

	// Step 3: the level's modifier array and save proficiencies.
	array := k.Tables.LevelArray(adv.RankSlug, adv.Level)
	v.Array = append([]int(nil), array[:]...)
	parts := make([]string, len(v.Array))
	for i, n := range v.Array {
		parts[i] = signedInt(n)
	}
	v.ArrayText = strings.Join(parts, ", ")
	distinct := []int{}
	for _, n := range v.Array {
		if !containsInt(distinct, n) {
			distinct = append(distinct, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(distinct)))
	remaining := append([]int(nil), v.Array...)
	for i, ab := range encounter.Abilities {
		row := builderAbilityRow{
			Key:         ab,
			Value:       stats.Abilities[i],
			SaveChecked: containsString(adv.SaveProfs, ab),
			SaveProf:    stats.SaveProf[i],
			Save:        stats.Saves[i],
			Options:     distinct,
		}
		if p := adv.AbilityMods[i]; p != nil {
			row.Pinned = strconv.Itoa(*p)
			if !containsInt(distinct, *p) {
				row.Options = append(append([]int(nil), distinct...), *p)
			}
			if j := indexInt(remaining, *p); j >= 0 {
				remaining = append(remaining[:j], remaining[j+1:]...)
			} else {
				v.PinsOffArray = true
			}
		}
		v.AbilityRows = append(v.AbilityRows, row)
	}

	// Steps 4-6: Clan and Class Mod, Affiliation, Role and armor.
	for _, o := range k.clanOptions(tr) {
		if o.Tag == encounter.RandomTag {
			continue
		}
		if o.Tag == "" {
			o.Tag = encounter.NoClan
		}
		v.ClanOptions = append(v.ClanOptions, builderOption{Value: o.Tag, Name: o.Name, Description: o.Description, Selected: o.Tag == adv.ClanTag})
	}
	for _, o := range k.affiliationOptions(tr) {
		if o.Tag == encounter.RandomTag {
			continue
		}
		if o.Tag == "" {
			o.Tag = encounter.NoAffiliation
		}
		v.AffiliationOptions = append(v.AffiliationOptions, builderOption{Value: o.Tag, Name: o.Name, Description: o.Description, Selected: o.Tag == adv.AffiliationTag})
	}
	v.ClassModOptions = builderOptions{{Value: "", Name: tr("encounters.builder_class_mod_none"), Description: tr("encounters.builder_class_mod_none_hint"), Selected: adv.ClassModTag == ""}}
	for _, tag := range k.ClassMods {
		var names []string
		for _, t := range k.Traits {
			if t.Category == "class_mod" && t.Tag == tag {
				names = append(names, t.Name)
			}
		}
		v.ClassModOptions = append(v.ClassModOptions, builderOption{
			Value: tag, Name: classModLabel(tr, tag), Selected: tag == adv.ClassModTag,
			Description: tr("encounters.affiliation_traits_hint", strings.Join(names, ", ")),
		})
	}
	for _, ro := range k.Roles {
		v.RoleOptions = append(v.RoleOptions, builderOption{
			Value: ro.Slug, Name: ro.Name, Selected: ro.Slug == adv.RoleSlug,
			Description: tr("encounters.builder_role_hint", signedInt(ro.AC), signedInt(ro.Save),
				signedFloat(ro.HP), signedFloat(ro.CP), "×"+ftoa(roleDamageMult(ro)), signedInt(ro.Speed)),
		})
	}
	for _, a := range encounter.ArmorKinds {
		v.ArmorOptions = append(v.ArmorOptions, builderOption{
			Value: a, Name: tr("encounters.builder_armor_" + a), Selected: a == adv.Armor,
			Description: tr("encounters.builder_armor_" + a + "_hint"),
		})
	}

	// Step 7: the trait checklist.
	v.TraitGroups, v.TraitsChosen = k.traitGroups(adv, stats.MaxTraitRank)

	// Step 8: passives.
	v.Passives = k.builderPassives(adv, tr)
	for _, n := range stats.Notes {
		v.Notes = append(v.Notes, tagLabel(tr, "adversary_note:", n))
	}

	// Step 9: jutsu.
	k.jutsuStep(&v, adv, jf, tr)

	// Step 10: classification and the Solo narrative attachment.
	for _, c := range []string{"", "iconic", "epic"} {
		key := c
		if key == "" {
			key = "none"
		}
		v.ClassificationOptions = append(v.ClassificationOptions, builderOption{
			Value: c, Name: tr("encounters.classification_" + key), Selected: c == adv.Classification,
			Description: tr("encounters.classification_hint_" + key),
		})
	}
	v.TransformationOptions = builderOptions{{Value: "", Name: tr("encounters.transformation_none"), Description: tr("encounters.narrative_hint"), Selected: adv.TransformationSlug == ""}}
	for _, t := range k.Transformations {
		v.TransformationOptions = append(v.TransformationOptions, builderOption{Value: t.Slug, Name: t.Name, Description: tr("encounters.narrative_hint"), Selected: t.Slug == adv.TransformationSlug})
	}
	v.DomainOptions = builderOptions{{Value: "", Name: tr("encounters.battlefield_domain_none"), Description: tr("encounters.narrative_hint"), Selected: adv.BattlefieldDomainSlug == ""}}
	for _, d := range k.Domains {
		v.DomainOptions = append(v.DomainOptions, builderOption{Value: d.Slug, Name: d.Name, Description: tr("encounters.narrative_hint"), Selected: d.Slug == adv.BattlefieldDomainSlug})
	}
	return v
}

func containsInt(list []int, v int) bool { return indexInt(list, v) >= 0 }

func indexInt(list []int, v int) int {
	for i, n := range list {
		if n == v {
			return i
		}
	}
	return -1
}

func roleDamageMult(ro encounter.BuilderRole) float64 {
	if ro.DamageMult == 0 {
		return 1
	}
	return ro.DamageMult
}

func classModLabel(tr i18n.TranslateFunc, tag string) string {
	return tagLabel(tr, "adversary_class_mod:", tag)
}

// traitCategoryOrder is the step-7 checklist's group order.
var traitCategoryOrder = []string{"role", "general", "clan", "affiliation", "class_mod", "iconic", "outside"}

// traitGroups sorts the trait catalog into the step-7 checklist for adv:
// its role's pool, the general pool, its clan, affiliation and class-mod
// pools, and the Iconic traits once it has a classification, all up to
// maxRank. Chosen traits outside every pool are kept in their own group.
func (k encounterKit) traitGroups(adv encounter.GeneratedAdversary, maxRank string) ([]builderTraitGroup, int) {
	chosen := map[string]bool{}
	for _, t := range adv.Traits {
		chosen[t.Slug] = true
	}
	role := k.Tables.Roles[adv.RoleSlug]
	groups := map[string][]builderTraitOption{}
	inCatalog := map[string]bool{}
	for _, t := range k.Traits {
		inCatalog[t.Slug] = true
		group := ""
		switch {
		case !encounter.TraitEligible(t, maxRank):
		case encounter.InRolePool(t, role):
			group = "role"
		case encounter.InOtherPool(t, adv):
			group = t.Category
		case t.Source == "sheet" && t.Category == "iconic" && adv.Classification != "":
			group = "iconic"
		}
		if group == "" {
			if !chosen[t.Slug] {
				continue
			}
			group = "outside"
		}
		groups[group] = append(groups[group], builderTraitOption{
			Slug: t.Slug, Name: t.Name, Rank: t.Rank, Description: t.Description, Checked: chosen[t.Slug],
		})
	}
	for _, t := range adv.Traits {
		if !inCatalog[t.Slug] {
			groups["outside"] = append(groups["outside"], builderTraitOption{
				Slug: t.Slug, Name: t.Name, Rank: t.Rank, Description: t.Description, Checked: true,
			})
		}
	}

	var out []builderTraitGroup
	for _, key := range traitCategoryOrder {
		list := groups[key]
		if len(list) == 0 {
			continue
		}
		sort.SliceStable(list, func(i, j int) bool {
			ri, rj := encounter.RankIndex(list[i].Rank), encounter.RankIndex(list[j].Rank)
			if ri != rj {
				return ri < rj
			}
			return list[i].Name < list[j].Name
		})
		g := builderTraitGroup{Key: key, Traits: list}
		for _, t := range list {
			if t.Checked {
				g.Checked++
			}
		}
		out = append(out, g)
	}
	return out, len(adv.Traits)
}

// passiveTagKey compares a passive's tag with a clan tag: the sheet writes
// "Cerealian (T7)" on passives and "Cerealian T7" on clans.
func passiveTagKey(tag string) string {
	return strings.TrimSpace(strings.NewReplacer("(", "", ")", "").Replace(tag))
}

// builderPassives lists adv's read-only passives (step 8): its role's
// passives, the Role Ability for an Elite or Solo, and its clan,
// affiliation and class-mod passives.
func (k encounterKit) builderPassives(adv encounter.GeneratedAdversary, tr i18n.TranslateFunc) []builderPassive {
	role := k.Tables.Roles[adv.RoleSlug]
	kind := encounter.ClassKind(adv.RankSlug)
	var out []builderPassive
	for _, p := range k.Passives {
		var source string
		switch {
		case p.Tag == role.Tag:
			if strings.HasSuffix(p.Slug, "/role-ability") && kind != "elite" && kind != "solo" {
				continue
			}
			source = role.Name
		case adv.ClanTag != encounter.NoClan && passiveTagKey(p.Tag) == passiveTagKey(adv.ClanTag):
			source = clanLabel(tr, adv.ClanTag)
		case adv.AffiliationTag != encounter.NoAffiliation && p.Tag == adv.AffiliationTag:
			source = affiliationLabel(tr, adv.AffiliationTag)
		case adv.ClassModTag != "" && p.Tag == adv.ClassModTag:
			source = classModLabel(tr, adv.ClassModTag)
		default:
			continue
		}
		out = append(out, builderPassive{Source: source, Name: p.Name, Description: p.Description})
	}
	return out
}

// builderPage builds the page or its live parts, with the shown jutsu's
// descriptions.
func (s *server) builderPage(r *http.Request, kit encounterKit, enc builderEncounter, advID int64, adv encounter.GeneratedAdversary, jf builderJutsuFilter) builderView {
	locale := localeFromContext(r.Context())
	view := kit.builderView(enc, advID, adv, jf, i18n.Translator(locale))
	if err := fillJutsuDescriptions(s.rulesDB, locale, &view); err != nil {
		log.Println("load builder jutsu descriptions:", err)
	}
	return view
}

func (s *server) renderBuilderPage(w http.ResponseWriter, r *http.Request, kit encounterKit, enc builderEncounter, advID int64, adv encounter.GeneratedAdversary) {
	tr := i18n.Translator(localeFromContext(r.Context()))
	view := s.builderPage(r, kit, enc, advID, adv, defaultJutsuFilter(adv))
	title := tr("encounters.builder_title")
	s.render(w, r, "encounters_builder.html", map[string]any{"Title": title, "Builder": view})
}

// handleBuilderNew opens the builder for a new adversary in a saved
// encounter, at the party's level.
func (s *server) handleBuilderNew(w http.ResponseWriter, r *http.Request) {
	enc, ok := s.builderEncounterOr404(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	s.renderBuilderPage(w, r, kit, enc, 0, kit.newBuilderAdversary(enc.PartyLevel))
}

// handleBuilderEdit opens the builder on a saved adversary.
func (s *server) handleBuilderEdit(w http.ResponseWriter, r *http.Request) {
	enc, ok := s.builderEncounterOr404(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	advID, err := strconv.ParseInt(r.PathValue("advId"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	adv, err := loadSavedAdversary(s.charDB, enc.ID, advID)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load adversary for builder:", err)
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	// A name still matching its generated default follows the choices
	// rather than keeping the old class and role in it.
	adv = adv.Normalize()
	if rk, ok := kit.class(adv.RankSlug); ok && encounter.IsDefaultName(kit.Toolkit, rk, adv) {
		adv.Name = ""
	}
	s.renderBuilderPage(w, r, kit, enc, advID, adv)
}

// handleBuilderName answers a random adversary name as plain text, for
// the builder's Generate name button. ?clan= is the chosen clan tag, whose
// family name the adversary carries when the clan is a family.
func (s *server) handleBuilderName(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, encounter.RandomName(newRand(), r.URL.Query().Get("clan")))
}

// handleBuilderPreview answers the builder's live parts for the posted
// choices, without saving anything.
func (s *server) handleBuilderPreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	enc, ok := s.builderEncounterOr404(w, r, r.FormValue("encounter_id"))
	if !ok {
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}
	advID, _ := strconv.ParseInt(r.FormValue("adversary_id"), 10, 64)
	// The saved adversary keeps chosen traits and jutsu the catalog no
	// longer has in the form across previews.
	var prev encounter.GeneratedAdversary
	if advID != 0 {
		if saved, err := loadSavedAdversary(s.charDB, enc.ID, advID); err == nil {
			prev = saved
		}
	}
	adv := parseBuilderForm(r, kit, enc.PartyLevel, prev)
	s.renderCard(w, r, "encounters_builder.html", "encounter_builder_live", s.builderPage(r, kit, enc, advID, adv, parseJutsuFilter(r)))
}

// handleBuilderSave saves the builder's choices as a new adversary at the
// end of the roster, or over the adversary being edited, and returns to the
// encounter.
func (s *server) handleBuilderSave(w http.ResponseWriter, r *http.Request) {
	enc, ok := s.builderEncounterOr404(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	kit, ok := s.encounterKitOr500(w, r)
	if !ok {
		return
	}

	var advID int64
	var prev encounter.GeneratedAdversary
	if raw := r.FormValue("adversary_id"); raw != "" && raw != "0" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "bad adversary id", http.StatusBadRequest)
			return
		}
		saved, err := loadSavedAdversary(s.charDB, enc.ID, id)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("load adversary for builder save:", err)
			return
		}
		advID, prev = id, saved
	}

	adv := parseBuilderForm(r, kit, enc.PartyLevel, prev)
	if adv.Name == "" {
		adv.Name = encounter.RandomName(newRand(), adv.ClanTag)
	}
	rk, _ := kit.class(adv.RankSlug)
	adv = encounter.Assemble(kit.Toolkit, rk, adv)

	tx, err := s.charDB.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("begin builder save tx:", err)
		return
	}
	defer tx.Rollback()
	if advID != 0 {
		err = replaceAdversary(tx, advID, adv, kit)
	} else {
		_, err = appendAdversary(tx, enc.ID, adv, kit)
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("save built adversary:", err)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("commit builder save:", err)
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(enc.ID, 10), http.StatusSeeOther)
}
