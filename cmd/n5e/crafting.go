// Seal Crafting planner (Tools menu): plan a chakra-enhanced weapon or armor
// — a base item, a quality and enhancement seals — and save it to the shared
// crafted-items library (characters migration 0095), from which any
// character can carry it. Every change posts the form to
// /crafting/calculate, which answers with the parts of the page that depend
// on the choices (crafting.js swaps them in place by id). Rank, cost and
// downtime are never stored; internal/crafting.Calculate derives them from
// rules.db on every read.
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/charstore"
	"github.com/RationalRouter/n5e/internal/crafting"
	"github.com/RationalRouter/n5e/internal/i18n"
)

// craftingCheck is one checkbox or radio button of a choice sub-picker or
// the crafter inputs.
type craftingCheck struct {
	Value, Name, Description string
	Checked                  bool
}

// craftingChoiceView is the sub-picker a chosen seal needs (see
// crafting.ChoiceRule): a select for one pick from a list, checkboxes for
// several.
type craftingChoiceView struct {
	Field   string // form field holding the picks
	Count   int
	Multi   bool
	Select  builderOptions
	Checks  []craftingCheck
	Invalid bool
}

// craftingSealRow is one seal in the picker.
type craftingSealRow struct {
	Slug, Name, Rank, Description string
	Slots                         int
	CostRyo                       float64
	Checked                       bool
	// Blocked: unchecked and unavailable (its family is already on the item,
	// or it needs more slots than are left); BlockedReason says which.
	Blocked       bool
	BlockedReason string
	Choice        *craftingChoiceView
}

// craftingRankGroup is the picker's seals of one rank.
type craftingRankGroup struct {
	Rank    string
	Seals   []craftingSealRow
	Checked int
}

// craftingResultView is the results panel: the calculation plus its
// localized labels and messages.
type craftingResultView struct {
	crafting.Result
	Crafting      crafting.Crafting
	QualityName   string
	BaseSource    string
	CraftsBase    bool
	CraftsSeals   bool
	KitCheckBonus bool
	TotalDowntime int
	Errors        []string
	Warnings      []string
}

// craftingView is what crafting_editor.html renders.
type craftingView struct {
	ID    int64 // 0 for a new item
	Slug  string
	Name  string
	Notes string
	Saved bool
	// Archived items stay editable; the editor says they are hidden from
	// the pickers.
	Archived bool

	WeaponOptions, ArmorOptions builderOptions
	BaseDescription             string
	QualityOptions, KitOptions  builderOptions
	BaseSources                 []craftingCheck
	Crafting                    crafting.Crafting

	HasBase    bool
	SealGroups []craftingRankGroup
	// Outside holds chosen seals that don't apply to the current base's
	// kind, so they can still be unchecked.
	Outside []craftingSealRow
	Result  craftingResultView
}

// craftingListRow is one saved item in the library table.
type craftingListRow struct {
	ID                                int64
	Slug, Name, BaseName, QualityName string
	Rank                              string
	MinLevel, SealCount               int
	MarketValue, TotalCost            float64
	Valid                             bool
}

func (s *server) craftingCatalog(w http.ResponseWriter, r *http.Request) (*crafting.Catalog, bool) {
	c, err := crafting.LoadCatalog(s.rulesDB, localeFromContext(r.Context()))
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load crafting catalog:", err)
		return nil, false
	}
	return c, true
}

// craftingMessages translates Calculate's messages. A string argument that
// is itself a crafting.* key (a toolkit name) is translated too.
func craftingMessages(tr i18n.TranslateFunc, msgs []crafting.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		args := make([]any, len(m.Args))
		for i, a := range m.Args {
			if s, ok := a.(string); ok && strings.HasPrefix(s, "crafting.") {
				a = tr(s)
			}
			args[i] = a
		}
		out = append(out, tr(m.Key, args...))
	}
	return out
}

// craftingFloat renders a multiplier or bonus without trailing zeros.
func craftingFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func craftingBaseDescription(tr i18n.TranslateFunc, b crafting.Base) string {
	var parts []string
	if b.Kind == "weapon" {
		if b.DamageDice != "" {
			parts = append(parts, strings.TrimSpace(b.DamageDice+" "+translateEnumDisplay(tr, damageTypeCatalogKey, b.DamageType)))
		}
		if b.Properties != "" {
			parts = append(parts, b.Properties)
		}
	} else {
		if b.ArmorCategory != "" {
			parts = append(parts, translateEnumDisplay(tr, armorCategoryCatalogKey, b.ArmorCategory))
		}
		parts = append(parts, tr("crafting.base_ac", b.ACBonus))
		if b.HighQuality {
			parts = append(parts, tr("crafting.base_high_quality"))
		}
	}
	parts = append(parts, tr("crafting.ryo", comma(b.CostRyo)))
	return strings.Join(parts, " · ")
}

// craftingView builds the editor for recipe.
func craftingEditorView(c *crafting.Catalog, tr i18n.TranslateFunc, recipe crafting.Recipe) craftingView {
	res := crafting.Calculate(c, recipe)
	cr := recipe.Crafting
	cr.Normalize()
	v := craftingView{Name: recipe.Name, Crafting: cr, HasBase: res.HasBase}

	for _, b := range c.Bases {
		opt := builderOption{Value: b.Slug, Name: b.Name, Description: craftingBaseDescription(tr, b), Selected: b.Slug == recipe.BaseSlug}
		if opt.Selected {
			v.BaseDescription = opt.Description
		}
		if b.Kind == "weapon" {
			v.WeaponOptions = append(v.WeaponOptions, opt)
		} else {
			v.ArmorOptions = append(v.ArmorOptions, opt)
		}
	}
	if v.BaseDescription == "" {
		v.BaseDescription = tr("crafting.base_pick_hint")
	}
	for _, q := range crafting.Qualities {
		desc := tr("crafting.quality_tip", q.Slots, craftingFloat(q.WeaponMultiplier), craftingFloat(q.ArmorMultiplier))
		switch q.Requirement {
		case crafting.RequireKitMastery:
			desc += " " + tr("crafting.quality_req_mastery")
		case crafting.RequireThreeMasters:
			desc += " " + tr("crafting.quality_req_three_masters")
		}
		v.QualityOptions = append(v.QualityOptions, builderOption{
			Value: q.ID, Name: tr("crafting.quality." + q.ID), Description: desc, Selected: q.ID == res.Quality.ID,
		})
	}
	for _, k := range crafting.KitTiers {
		desc := tr("crafting.kit_tip." + k.ID)
		v.KitOptions = append(v.KitOptions, builderOption{
			Value: k.ID, Name: tr("crafting.kit." + k.ID), Description: desc, Selected: k.ID == cr.KitTier,
		})
	}
	for _, src := range []crafting.BaseSource{crafting.BaseCraft, crafting.BaseBuy, crafting.BaseOwn} {
		v.BaseSources = append(v.BaseSources, craftingCheck{
			Value: string(src), Name: tr("crafting.source." + string(src)),
			Description: tr("crafting.source_tip." + string(src)), Checked: src == cr.BaseSource,
		})
	}

	v.SealGroups, v.Outside = craftingSealPicker(c, tr, recipe, res)
	v.Result = craftingResult(tr, res, cr)
	return v
}

func craftingResult(tr i18n.TranslateFunc, res crafting.Result, cr crafting.Crafting) craftingResultView {
	out := craftingResultView{
		Result: res, Crafting: cr,
		QualityName:   tr("crafting.quality." + res.Quality.ID),
		BaseSource:    tr("crafting.source." + string(cr.BaseSource)),
		CraftsBase:    res.HasBase && cr.BaseSource == crafting.BaseCraft,
		CraftsSeals:   res.CraftsSeals(cr),
		KitCheckBonus: crafting.KitTierByID(cr.KitTier).CheckBonus,
		Errors:        craftingMessages(tr, res.Errors),
		Warnings:      craftingMessages(tr, res.Warnings),
	}
	out.TotalDowntime = res.BaseDowntime + res.SealPrepDowntime + res.SealWeeksPerCrafter
	return out
}

// craftingSealPicker lists the seals for the base's kind, grouped by rank,
// marking what is chosen, what can no longer be added, and each chosen
// seal's sub-picker.
func craftingSealPicker(c *crafting.Catalog, tr i18n.TranslateFunc, recipe crafting.Recipe, res crafting.Result) ([]craftingRankGroup, []craftingSealRow) {
	picks := map[string]crafting.SealPick{}
	families := map[string]bool{}
	for _, p := range recipe.Seals {
		picks[p.Slug] = p
		if s, ok := c.Seal(p.Slug); ok {
			families[s.Family] = true
		}
	}
	invalidChoice := map[string]bool{}
	for _, m := range res.Errors {
		if m.Key == "crafting.err.choice" && len(m.Args) > 0 {
			invalidChoice[fmt.Sprint(m.Args[0])] = true
		}
	}
	free := res.SlotsAvailable - res.SlotsUsed

	row := func(seal crafting.Seal) craftingSealRow {
		rule := seal.RankRule()
		pick, checked := picks[seal.Slug]
		r := craftingSealRow{
			Slug: seal.Slug, Name: seal.Name, Rank: seal.Rank, Description: seal.Description,
			Slots: rule.Slots, CostRyo: seal.CostRyo, Checked: checked,
		}
		switch {
		case checked:
			if seal.Choice != nil {
				r.Choice = craftingChoice(c, tr, seal, pick.Choices, invalidChoice[seal.Name])
			}
		case families[seal.Family]:
			r.Blocked, r.BlockedReason = true, tr("crafting.blocked_family")
		case rule.Slots > free:
			r.Blocked, r.BlockedReason = true, tr("crafting.blocked_slots")
		}
		return r
	}

	var groups []craftingRankGroup
	shown := map[string]bool{}
	if res.HasBase {
		byRank := map[string]*craftingRankGroup{}
		for _, rank := range []string{"D", "C", "B", "A", "S"} {
			groups = append(groups, craftingRankGroup{Rank: rank})
		}
		for i := range groups {
			byRank[groups[i].Rank] = &groups[i]
		}
		for _, seal := range c.SealsFor(res.Base.Kind) {
			g := byRank[seal.Rank]
			if g == nil {
				continue
			}
			r := row(seal)
			if r.Checked {
				g.Checked++
			}
			shown[seal.Slug] = true
			g.Seals = append(g.Seals, r)
		}
	}
	var outside []craftingSealRow
	for _, p := range recipe.Seals {
		if shown[p.Slug] {
			continue
		}
		if seal, ok := c.Seal(p.Slug); ok {
			outside = append(outside, row(seal))
		}
	}
	return groups, outside
}

func craftingChoice(c *crafting.Catalog, tr i18n.TranslateFunc, seal crafting.Seal, picks []string, invalid bool) *craftingChoiceView {
	rule := *seal.Choice
	picked := map[string]bool{}
	for _, p := range picks {
		picked[p] = true
	}
	v := &craftingChoiceView{Field: "choice:" + seal.Slug, Count: rule.Count, Multi: rule.Count > 1, Invalid: invalid}
	add := func(value, name, desc string) {
		if v.Multi {
			v.Checks = append(v.Checks, craftingCheck{Value: value, Name: name, Description: desc, Checked: picked[value]})
			return
		}
		v.Select = append(v.Select, builderOption{Value: value, Name: name, Description: desc, Selected: picked[value]})
	}
	if !v.Multi {
		v.Select = append(v.Select, builderOption{Value: "", Name: tr("crafting.choose"), Description: tr("crafting.choice_hint." + string(rule.Kind))})
	}
	switch rule.Kind {
	case crafting.ChoiceDamageType:
		for _, o := range rule.Options {
			add(o, tr("crafting.damage."+o), tr("crafting.choice_hint."+string(rule.Kind)))
		}
	case crafting.ChoiceCreatureTypes:
		for _, o := range rule.Options {
			add(o, tr("crafting.creature."+o), "")
		}
	case crafting.ChoiceSummonTribe:
		for _, s := range c.SummonsFor(rule) {
			add(s.Slug, s.Name, tr("crafting.summon_type."+strings.ToLower(s.SummonType)))
		}
	case crafting.ChoiceJutsu:
		for _, j := range c.JutsuFor(rule) {
			add(j.Slug, j.Name+" ("+j.Rank+")", tr("crafting.jutsu_tip", j.Rank, j.Classification))
		}
	}
	return v
}

// parseCraftingForm reads a recipe from the editor form. Seals keep the
// order they appear in the form.
func parseCraftingForm(r *http.Request) (crafting.Recipe, string) {
	recipe := crafting.Recipe{
		Name:     strings.TrimSpace(r.FormValue("name")),
		BaseSlug: r.FormValue("base_slug"),
		Quality:  r.FormValue("quality"),
	}
	seen := map[string]bool{}
	for _, slug := range r.Form["seal"] {
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		var choices []string
		for _, c := range r.Form["choice:"+slug] {
			if c != "" {
				choices = append(choices, c)
			}
		}
		recipe.Seals = append(recipe.Seals, crafting.SealPick{Slug: slug, Choices: choices})
	}
	atoi := func(name string) int {
		n, _ := strconv.Atoi(r.FormValue(name))
		return n
	}
	on := func(name string) bool { return r.FormValue(name) != "" }
	recipe.Crafting = crafting.Crafting{
		BaseSource:    crafting.BaseSource(r.FormValue("base_source")),
		HireSealsmith: on("hire_sealsmith"),
		Participants:  atoi("participants"),
		Masters:       atoi("masters"),
		KitTier:       r.FormValue("kit_tier"),
		Proficient:    on("proficient"),
		HasForge:      on("has_forge"),
		HasSealForge:  on("has_seal_forge"),
		CrafterLevel:  atoi("crafter_level"),
	}
	recipe.Crafting.Normalize()
	return recipe, strings.TrimSpace(r.FormValue("notes"))
}

func (s *server) renderCraftingEditor(w http.ResponseWriter, r *http.Request, c *crafting.Catalog, item charstore.CraftedItem, saved bool) {
	tr := i18n.Translator(localeFromContext(r.Context()))
	v := craftingEditorView(c, tr, item.Recipe)
	v.ID, v.Slug, v.Notes, v.Saved, v.Archived = item.ID, item.Slug, item.Notes, saved, item.Archived
	title := tr("crafting.new_title")
	if item.ID != 0 {
		title = item.Recipe.Name
	}
	s.render(w, r, "crafting_editor.html", map[string]any{"Title": title, "Crafting": v})
}

// handleCraftingList is the library of saved items.
func (s *server) handleCraftingList(w http.ResponseWriter, r *http.Request) {
	archived := r.URL.Query().Get("archived") == "1"
	items, err := charstore.ListCraftedItems(s.charDB, archived)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("list crafted items:", err)
		return
	}
	c, ok := s.craftingCatalog(w, r)
	if !ok {
		return
	}
	tr := i18n.Translator(localeFromContext(r.Context()))
	rows := make([]craftingListRow, 0, len(items))
	for _, it := range items {
		res := crafting.Calculate(c, it.Recipe)
		row := craftingListRow{
			ID: it.ID, Slug: it.Slug, Name: it.Recipe.Name,
			QualityName: tr("crafting.quality." + res.Quality.ID),
			Rank:        res.Rank, MinLevel: res.MinLevel, SealCount: len(it.Recipe.Seals),
			MarketValue: res.MarketValue, TotalCost: res.TotalCost, Valid: res.Valid(),
		}
		if res.HasBase {
			row.BaseName = res.Base.Name
		} else {
			row.BaseName = it.Recipe.BaseSlug
		}
		rows = append(rows, row)
	}
	s.render(w, r, "crafting.html", map[string]any{
		"Title": tr("crafting.title"), "Items": rows, "Archived": archived,
	})
}

func (s *server) handleCraftingNew(w http.ResponseWriter, r *http.Request) {
	c, ok := s.craftingCatalog(w, r)
	if !ok {
		return
	}
	recipe := crafting.Recipe{Quality: "standard", Crafting: crafting.DefaultCrafting()}
	if base := r.URL.Query().Get("base"); base != "" {
		recipe.BaseSlug = base
	}
	s.renderCraftingEditor(w, r, c, charstore.CraftedItem{Recipe: recipe}, false)
}

// craftedItemOr404 loads the item named by the {id} path value.
func (s *server) craftedItemOr404(w http.ResponseWriter, r *http.Request) (charstore.CraftedItem, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return charstore.CraftedItem{}, false
	}
	it, err := charstore.GetCraftedItem(s.charDB, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return charstore.CraftedItem{}, false
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load crafted item:", err)
		return charstore.CraftedItem{}, false
	}
	return it, true
}

func (s *server) handleCraftingEdit(w http.ResponseWriter, r *http.Request) {
	it, ok := s.craftedItemOr404(w, r)
	if !ok {
		return
	}
	c, ok := s.craftingCatalog(w, r)
	if !ok {
		return
	}
	s.renderCraftingEditor(w, r, c, it, r.URL.Query().Get("saved") == "1")
}

// handleCraftingCalculate answers the editor's live parts for the posted
// form.
func (s *server) handleCraftingCalculate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	c, ok := s.craftingCatalog(w, r)
	if !ok {
		return
	}
	recipe, _ := parseCraftingForm(r)
	tr := i18n.Translator(localeFromContext(r.Context()))
	s.renderCard(w, r, "crafting_editor.html", "crafting_live", craftingEditorView(c, tr, recipe))
}

// handleCraftingSave saves the form as a new item (POST /crafting) or over
// an existing one (POST /crafting/{id}/update). An invalid recipe re-renders
// the editor with its errors instead of saving.
func (s *server) handleCraftingSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	var existing charstore.CraftedItem
	if r.PathValue("id") != "" {
		it, ok := s.craftedItemOr404(w, r)
		if !ok {
			return
		}
		existing = it
	}
	c, ok := s.craftingCatalog(w, r)
	if !ok {
		return
	}
	recipe, notes := parseCraftingForm(r)
	item := charstore.CraftedItem{ID: existing.ID, Slug: existing.Slug, Archived: existing.Archived, Notes: notes, Recipe: recipe}
	if !crafting.Calculate(c, recipe).Valid() {
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderCraftingEditor(w, r, c, item, false)
		return
	}
	if existing.ID != 0 {
		if err := charstore.UpdateCraftedItem(s.charDB, existing.ID, item); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("update crafted item:", err)
			return
		}
	} else {
		saved, err := charstore.AddCraftedItem(s.charDB, item)
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("add crafted item:", err)
			return
		}
		item = saved
	}
	http.Redirect(w, r, fmt.Sprintf("/crafting/%d?saved=1", item.ID), http.StatusSeeOther)
}

func (s *server) handleCraftingDuplicate(w http.ResponseWriter, r *http.Request) {
	it, ok := s.craftedItemOr404(w, r)
	if !ok {
		return
	}
	tr := i18n.Translator(localeFromContext(r.Context()))
	dup, err := charstore.DuplicateCraftedItem(s.charDB, it.ID, tr("crafting.copy_name", it.Recipe.Name))
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("duplicate crafted item:", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/crafting/%d", dup.ID), http.StatusSeeOther)
}

// handleCraftingArchive archives (archived=true) or restores an item and
// returns to the library view it was in.
func (s *server) handleCraftingArchive(archived bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		it, ok := s.craftedItemOr404(w, r)
		if !ok {
			return
		}
		if err := charstore.SetCraftedItemArchived(s.charDB, it.ID, archived); err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			log.Println("archive crafted item:", err)
			return
		}
		back := "/crafting"
		if !archived {
			back += "?" + url.Values{"archived": {"1"}}.Encode()
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// craftedChoiceView is one resolved seal choice on the item page.
type craftedChoiceView struct {
	Name, Href string
}

// craftedSealView is one seal on the item page.
type craftedSealView struct {
	Name, Rank, Description string
	Chosen                  []craftedChoiceView
}

// craftedDetail is the crafted-only part of a crafted item's page.
type craftedDetail struct {
	ID                        int64
	Slug                      string
	QualityName               string
	Rank                      string
	MinLevel                  int
	SlotsUsed, SlotsAvailable int
	MarketValue, TotalCost    float64
	Archived                  bool
	Errors                    []string
	Seals                     []craftedSealView
	Characters                []charstore.CharacterPurse
	// Flash after "Add to character": the character it went to, or the one
	// who couldn't pay. AddedToID links the flash to that character's
	// Inventory tab.
	AddedTo, ShortOf string
	AddedToID        int64
}

func craftedChoices(tr i18n.TranslateFunc, chosen []crafting.ChosenOption) []craftedChoiceView {
	out := make([]craftedChoiceView, 0, len(chosen))
	for _, c := range chosen {
		name := c.Name
		if c.LabelKey != "" {
			name = tr(c.LabelKey)
		}
		out = append(out, craftedChoiceView{Name: name, Href: c.Href})
	}
	return out
}

// loadCraftedItem loads a crafted/ slug for the item page: the base item's
// card (its stats are the crafted item's stats) renamed and repriced, plus
// the quality, rank and seals. nil when no such item exists.
func (s *server) loadCraftedItem(r *http.Request, slug string) (*itemRow, error) {
	saved, err := charstore.GetCraftedItemBySlug(s.charDB, slug)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	locale := localeFromContext(r.Context())
	tr := i18n.Translator(locale)
	c, err := crafting.LoadCatalog(s.rulesDB, locale)
	if err != nil {
		return nil, err
	}
	res := crafting.Calculate(c, saved.Recipe)

	it := &itemRow{}
	if res.HasBase {
		base, err := loadItem(s.rulesDB, res.Base.Slug, locale)
		if err != nil {
			return nil, err
		}
		if base != nil {
			it = base
		}
		it.BaseName, it.BaseDescription = res.Base.Name, it.Description.String
	}
	it.Slug, it.Name = saved.Slug, saved.Recipe.Name
	it.CostRyo = sql.NullFloat64{Float64: res.MarketValue, Valid: true}
	it.Description = sql.NullString{String: saved.Notes, Valid: saved.Notes != ""}

	d := &craftedDetail{
		ID: saved.ID, Slug: saved.Slug, QualityName: tr("crafting.quality." + res.Quality.ID),
		Rank: res.Rank, MinLevel: res.MinLevel, SlotsUsed: res.SlotsUsed, SlotsAvailable: res.SlotsAvailable,
		MarketValue: res.MarketValue, TotalCost: res.TotalCost, Archived: saved.Archived,
		Errors: craftingMessages(tr, res.Errors),
	}
	for _, a := range res.Seals {
		d.Seals = append(d.Seals, craftedSealView{
			Name: a.Seal.Name, Rank: a.Seal.Rank, Description: a.Seal.Description,
			Chosen: craftedChoices(tr, a.Chosen),
		})
	}
	if d.Characters, err = charstore.ListCharacterPurses(s.charDB); err != nil {
		return nil, err
	}
	q := r.URL.Query()
	for _, p := range d.Characters {
		id := strconv.FormatInt(p.ID, 10)
		if q.Get("added") == id {
			d.AddedTo, d.AddedToID = p.Name, p.ID
		}
		if q.Get("short") == id {
			d.ShortOf = p.Name
		}
	}
	it.Crafted = d
	return it, nil
}

// handleCraftingGive adds one of a crafted item to a character's
// inventory. pay=cost takes the crafting cost from the character's Ryo,
// pay=value the market value; anything else adds it for free.
func (s *server) handleCraftingGive(w http.ResponseWriter, r *http.Request) {
	it, ok := s.craftedItemOr404(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	charID, err := strconv.ParseInt(r.FormValue("character_id"), 10, 64)
	if err != nil {
		http.Error(w, "pick a character", http.StatusBadRequest)
		return
	}
	var price float64
	if pay := r.FormValue("pay"); pay == "cost" || pay == "value" {
		c, ok := s.craftingCatalog(w, r)
		if !ok {
			return
		}
		res := crafting.Calculate(c, it.Recipe)
		price = res.TotalCost
		if pay == "value" {
			price = res.MarketValue
		}
	}
	back := "/items/" + it.Slug + "?"
	switch err := charstore.GiveCraftedItem(s.charDB, charID, it.Slug, price); {
	case err == nil:
		back += url.Values{"added": {strconv.FormatInt(charID, 10)}}.Encode()
	case errors.Is(err, charstore.ErrNotEnoughRyo):
		back += url.Values{"short": {strconv.FormatInt(charID, 10)}}.Encode()
	case errors.Is(err, sql.ErrNoRows):
		http.NotFound(w, r)
		return
	default:
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("give crafted item:", err)
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
