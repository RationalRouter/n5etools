package main

import (
	"database/sql"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/charstore"
)

type itemPropertyRow struct {
	Slug        string
	Name        string
	Detail      sql.NullString
	Description string
}

type itemRow struct {
	Slug           string
	Name           string
	Kind           string
	KindLabel      string
	CostRyo        sql.NullFloat64
	WeightLb       sql.NullFloat64
	Description    sql.NullString
	DamageDice     sql.NullString
	DamageType     sql.NullString
	WeaponCategory sql.NullString
	AmmoDie        sql.NullString
	ACBonus        sql.NullInt64
	ArmorCategory  sql.NullString
	ArmorAbility1  sql.NullString
	ArmorAbility2  sql.NullString
	ArmorMaxMod    sql.NullFloat64
	StrengthReq    sql.NullInt64
	StealthDisadv  sql.NullInt64
	Bulk           sql.NullFloat64
	Charges        sql.NullInt64
	CraftDC        sql.NullInt64
	HealAmount     sql.NullString
	RegainType     sql.NullString
	Uses           sql.NullInt64
	BonusEffect    sql.NullString
	SaveDC         sql.NullInt64
	Properties     []itemPropertyRow

	// Custom-only fields, from the local item library (custom_items in
	// characters.db — see charstore.CustomItem and loadCustomItem). Zero for
	// every catalogue item; item_detail_card's edit block gates on IsCustom.
	IsCustom       bool
	CustomID       int64
	RollableKind   string
	PropertiesText string // free text, unlike the catalogue's structured Properties slice
	DamageCount    int
	DamageSides    int

	// Set by expandKitReference for the Greater/Superior/Supreme items
	// whose printed description is only a cross-reference ("See Security
	// Kit."): the base item's name and its full description, so the card
	// can show what the item actually does instead of a pointer. Empty for
	// every other item.
	BaseName        string
	BaseDescription string
}

// HealText renders heal_amount/regain_type as one plain-English string —
// e.g. "2d6+5 HP", "2d6+2 Chakra", "2d10+5 Temp HP & Chakra" — so the
// template can pass it straight through diceAvg like every other
// dice-notation field. Returns "" (falsy in a template {{if}}) when the
// item doesn't grant a heal/regain at all (most gear/tools don't).
func (it itemRow) HealText() string {
	if !it.HealAmount.Valid {
		return ""
	}
	label := "HP"
	switch it.RegainType.String {
	case "chakra":
		label = "Chakra"
	case "temp_hp_chakra":
		label = "Temp HP & Chakra"
	}
	return it.HealAmount.String + " " + label
}

// ArmorAbilityText renders the armor's ability-derived AC term(s) in plain
// English for the detail card — e.g. "DEX" or "DEX + half proficiency" —
// skipping any slot storing the DB's "NONE" sentinel (see
// internal/charsheet.ArmorClass's doc for why "NONE" isn't the same as
// absent/NULL). Returns "" when neither slot applies, so the template's
// {{if .ArmorAbilityText}} guard can stay a plain truthiness check.
func (it itemRow) ArmorAbilityText() string {
	var parts []string
	for _, ab := range []sql.NullString{it.ArmorAbility1, it.ArmorAbility2} {
		if !ab.Valid || ab.String == "" || ab.String == "NONE" {
			continue
		}
		if ab.String == "PROF" {
			parts = append(parts, "half proficiency")
		} else {
			parts = append(parts, ab.String)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	joined := parts[0]
	for _, p := range parts[1:] {
		joined += " + " + p
	}
	return joined
}

// equipmentFilterCategory buckets one row for the browse lists' weapon/armor
// sub-filters (Simple/Martial/Exotic, Light/Medium/Heavy).
//
// Returns "" for anything that isn't of wantKind, which is what tells the
// templates to omit the data- attribute entirely — a row with no attribute is
// left alone by that sub-filter rather than being judged by it, so ticking
// "Simple" never hides gear or toolkits.
//
// A row of the right kind whose category the rules don't record buckets as
// "other" instead of dropping out of the attribute: two weapons (Net, Unarmed
// Strike) have no category, and without a bucket of their own they'd ignore the
// weapon filter completely and sit in a list filtered to "Simple" looking like
// a bug.
func equipmentFilterCategory(kind, wantKind, category string) string {
	if kind != wantKind {
		return ""
	}
	if c := strings.ToLower(strings.TrimSpace(category)); c != "" {
		return c
	}
	return "other"
}

// WeaponFilterCategory and ArmorFilterCategory expose that bucketing to
// items.html, which stamps them onto each row for static/js/equipment-
// subfilter.js.
func (it itemRow) WeaponFilterCategory() string {
	return equipmentFilterCategory(it.Kind, "weapon", it.WeaponCategory.String)
}

func (it itemRow) ArmorFilterCategory() string {
	return equipmentFilterCategory(it.Kind, "armor", it.ArmorCategory.String)
}

// itemKindLabels fixes the section order (Weapons before Armor) and display
// name — enhancement seals keep their own dedicated /seals page (bespoke
// rank/applies-to grouping) and aren't folded in here.
var itemKindLabels = []struct{ kind, label string }{
	{"weapon", "Weapons"},
	{"armor", "Armor"},
	{"gear", "Gear"},
	{"tool", "Tools"},
	{"toolkit", "Toolkits"},
	{"scroll", "Scrolls"},
}

func itemKindLabel(kind string) string {
	for _, k := range itemKindLabels {
		if k.kind == kind {
			return k.label
		}
	}
	return kind
}

func kindSortOrder(kind string) int {
	for i, k := range itemKindLabels {
		if k.kind == kind {
			return i
		}
	}
	return len(itemKindLabels)
}

// loadItemProperties fills in it.Properties for a single already-scanned
// row — weapon and armor properties are normalized into separate
// tables/junctions (equipment_properties/weapon_properties vs.
// equipment_armor_properties/armor_properties), so which pair to query
// depends on the item's own kind. Armor's junction currently has zero rows
// (which named property applies to which specific armor is only printed in
// the book's Armor table, a pure image — see V2_ROADMAP.md's OCR-debt
// note), but querying it anyway costs one cheap lookup and means armor
// properties appear here automatically the moment that gap is closed, no
// code change needed — same "no code needed once data exists" pattern
// properties.go already established for the two property kinds.
func loadItemProperties(rulesDB *sql.DB, it *itemRow) error {
	var query string
	switch it.Kind {
	case "weapon":
		query = `SELECT wp.slug, wp.name, ep.detail, wp.description
			FROM equipment_properties ep JOIN weapon_properties wp ON wp.slug = ep.property_slug
			WHERE ep.equipment_slug = ? ORDER BY wp.name`
	case "armor":
		query = `SELECT ap.slug, ap.name, eap.detail, ap.description
			FROM equipment_armor_properties eap JOIN armor_properties ap ON ap.slug = eap.property_slug
			WHERE eap.equipment_slug = ? ORDER BY ap.name`
	default:
		return nil
	}
	rows, err := rulesDB.Query(query, it.Slug)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p itemPropertyRow
		if err := rows.Scan(&p.Slug, &p.Name, &p.Detail, &p.Description); err != nil {
			return err
		}
		it.Properties = append(it.Properties, p)
	}
	return rows.Err()
}

const itemSelectColumns = `slug, name, kind, cost_ryo, weight_lb, description,
	       damage_dice, damage_type, ammo_die, weapon_category,
	       ac_bonus, armor_category, armor_ability_1, armor_ability_2, armor_max_mod,
	       strength_req, stealth_disadv, bulk,
	       charges, craft_dc, heal_amount, regain_type, uses, bonus_effect, save_dc`

func scanItemRow(scan func(dest ...any) error) (itemRow, error) {
	var it itemRow
	err := scan(&it.Slug, &it.Name, &it.Kind, &it.CostRyo, &it.WeightLb, &it.Description,
		&it.DamageDice, &it.DamageType, &it.AmmoDie, &it.WeaponCategory,
		&it.ACBonus, &it.ArmorCategory, &it.ArmorAbility1, &it.ArmorAbility2, &it.ArmorMaxMod,
		&it.StrengthReq, &it.StealthDisadv, &it.Bulk,
		&it.Charges, &it.CraftDC, &it.HealAmount, &it.RegainType, &it.Uses, &it.BonusEffect, &it.SaveDC)
	it.KindLabel = itemKindLabel(it.Kind)
	return it, err
}

// loadItems queries every browsable item for the left-pane list. Properties
// are loaded per-row (71 weapons + 32 armor today — consistent with the
// app's existing N+1-at-this-scale tolerance, see e.g. classes.go's
// per-level resource loop).
func loadItems(rulesDB *sql.DB) ([]itemRow, error) {
	rows, err := rulesDB.Query(`SELECT ` + itemSelectColumns + ` FROM v_equipment
		WHERE kind IN ('weapon','armor','gear','tool','toolkit','scroll')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []itemRow
	for rows.Next() {
		it, err := scanItemRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range items {
		if err := loadItemProperties(rulesDB, &items[i]); err != nil {
			return nil, err
		}
		if err := expandKitReference(rulesDB, &items[i]); err != nil {
			return nil, err
		}
	}

	sort.Slice(items, func(i, j int) bool {
		ki, kj := kindSortOrder(items[i].Kind), kindSortOrder(items[j].Kind)
		if ki != kj {
			return ki < kj
		}
		return sortKey(items[i].Name) < sortKey(items[j].Name)
	})
	return items, nil
}

// seeAlsoPattern matches a description that defers to another item's entry:
// an optional tier-specific note, then "See <Item Name>." at the very end.
//
// Deliberately strict about the capital letter after "See": two base kits
// end with a sentence that also contains the word — Disguise Kit's "...to
// see through it." (lowercase) and Poison Kit's "See the Poisons reference
// for what can be crafted with this kit." (lowercase "the") — and neither
// is a cross-reference to an item. Requiring an initial capital rules both
// out, and expandKitReference additionally refuses to rewrite anything
// whose captured name isn't a real equipment row.
var seeAlsoPattern = regexp.MustCompile(`(?s)^(.*?)\s*\bSee ([A-Z][^.]*)\.\s*$`)

// expandKitReference resolves the "See Security Kit."-style descriptions the
// book prints for every Greater/Superior/Supreme item.
//
// 69 rows across toolkits, tools and gear carry one. Shown verbatim they
// were useless: the Supreme Security Kit's whole description was the words
// "See Security Kit." The book means "everything the base item does, plus
// this tier's bonus effect", so this splits the row into the
// tier's own note (whatever preceded the reference, e.g. "Swift Craft DC
// 22.") and the base item's full description, which the card then shows in
// that order. The tier's bonus_effect column is already a separate field on
// the card and is left alone.
//
// A reference that doesn't resolve to a real item is left exactly as it was,
// so a future rules edit that changes the wording degrades to the old
// behaviour rather than blanking a description.
func expandKitReference(rulesDB *sql.DB, it *itemRow) error {
	if !it.Description.Valid {
		return nil
	}
	m := seeAlsoPattern.FindStringSubmatch(it.Description.String)
	if m == nil {
		return nil
	}
	note, baseName := strings.TrimSpace(m[1]), strings.TrimSpace(m[2])

	var baseDesc sql.NullString
	err := rulesDB.QueryRow(
		`SELECT description FROM v_equipment WHERE name = ? AND slug <> ?`, baseName, it.Slug,
	).Scan(&baseDesc)
	if err == sql.ErrNoRows || !baseDesc.Valid {
		return nil
	}
	if err != nil {
		return err
	}

	it.BaseName = baseName
	it.BaseDescription = baseDesc.String
	it.Description = sql.NullString{String: note, Valid: note != ""}
	return nil
}

// loadItem fetches one item by slug for the detail pane/page — a direct
// lookup rather than filtering loadItems' full result, since
// items-filter.js calls this on every list click and there's no reason to
// re-scan and re-sort ~250 rows just to find one.
func loadItem(rulesDB *sql.DB, slug string) (*itemRow, error) {
	row := rulesDB.QueryRow(`SELECT `+itemSelectColumns+` FROM v_equipment
		WHERE slug = ? AND kind IN ('weapon','armor','gear','tool','toolkit','scroll')`, slug)
	it, err := scanItemRow(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := loadItemProperties(rulesDB, &it); err != nil {
		return nil, err
	}
	if err := expandKitReference(rulesDB, &it); err != nil {
		return nil, err
	}
	return &it, nil
}

// loadCustomItem loads one custom/-prefixed slug from the local item
// library (custom_items, characters.db) instead of rules.db — the custom/
// half of handleItemDetail's dispatch. Mapped into an itemRow so the shared
// item_detail_card template renders it exactly like a catalogue item, plus
// the handful of custom-only fields the card's own "Edit this item" block
// uses.
func (s *server) loadCustomItem(slug string) (*itemRow, error) {
	item, err := charstore.GetCustomItemBySlug(s.charDB, slug)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	it := itemRow{
		Slug: item.Slug, Name: item.Name, Kind: item.Kind, KindLabel: itemKindLabel(item.Kind),
		Bulk:           item.Bulk,
		DamageType:     sql.NullString{String: item.DamageType, Valid: item.DamageType != ""},
		Description:    sql.NullString{String: item.Description, Valid: item.Description != ""},
		IsCustom:       true,
		CustomID:       item.ID,
		RollableKind:   item.RollableKind,
		PropertiesText: item.Properties,
	}
	if m := damageDicePattern.FindStringSubmatch(strings.TrimSpace(item.DamageDice)); m != nil {
		count := 1
		if m[1] != "" {
			count, _ = strconv.Atoi(m[1])
		}
		sides, _ := strconv.Atoi(m[2])
		it.DamageCount, it.DamageSides = count, sides
		it.DamageDice = sql.NullString{String: item.DamageDice, Valid: true}
	}
	return &it, nil
}

type itemGroup struct {
	Label string
	Items []itemRow
}

func groupItems(items []itemRow) []itemGroup {
	var groups []itemGroup
	for _, k := range itemKindLabels {
		var inKind []itemRow
		for _, it := range items {
			if it.Kind == k.kind {
				inKind = append(inKind, it)
			}
		}
		if len(inKind) == 0 {
			continue
		}
		groups = append(groups, itemGroup{Label: k.label, Items: inKind})
	}
	return groups
}

// handleItems renders the two-pane browse view: a filterable list on the
// left (static/js/items-filter.js), and a live detail pane on the right
// that's swapped via a fetch to /items/{slug}?fragment=1 (see
// handleItemDetail) rather than a full page reload — this app has no
// client-side templating anywhere, so a fetched HTML fragment reuses the
// exact same Go template the standalone detail page and this initial
// render both use, instead of duplicating the card markup in JS.
func (s *server) handleItems(w http.ResponseWriter, r *http.Request) {
	items, err := loadItems(s.rulesDB)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load items:", err)
		return
	}
	groups := groupItems(items)

	var selected *itemRow
	if len(groups) > 0 && len(groups[0].Items) > 0 {
		selected = &groups[0].Items[0]
	}

	s.render(w, "items.html", map[string]any{"Title": "Items", "Groups": groups, "Selected": selected})
}

// handleItemDetail serves both the standalone /items/{slug} page (used by
// site-wide search and direct links, matching every other browsable type's
// "real per-entity detail route" convention) and, when called with
// ?fragment=1, just the inner card markup with no layout — items-filter.js
// fetches that fragment to swap the two-pane view's right side without a
// full page reload.
func (s *server) handleItemDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	// A custom/ slug is a local library entry (custom_items, characters.db),
	// never a rules.db row — see loadCustomItem and CustomItem's doc
	// comment. Same "slug prefix picks the table" dispatch inventoryRow.
	// DetailHref and lookupCarriedItem already use for this.
	var item *itemRow
	var err error
	if strings.HasPrefix(slug, "custom/") {
		item, err = s.loadCustomItem(slug)
	} else {
		item, err = loadItem(s.rulesDB, slug)
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load item:", err)
		return
	}
	if item == nil {
		http.NotFound(w, r)
		return
	}

	if r.URL.Query().Get("fragment") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl, ok := pageTemplates["items.html"]
		if !ok {
			http.Error(w, "template not found", http.StatusInternalServerError)
			log.Println("render item fragment: no template registered")
			return
		}
		if err := tmpl.ExecuteTemplate(w, "item_detail_card", item); err != nil {
			log.Println("render item fragment:", err)
		}
		return
	}

	s.render(w, "item_detail.html", map[string]any{"Title": item.Name, "Item": item})
}

// handleCustomItemUpdate saves every editable field of a library entry —
// the custom item detail page's own "Edit this item" disclosure
// (item_detail_card's IsCustom block). Not character-scoped: every
// character carrying this item by slug sees the change, same as editing a
// shared rules-book row would if that were possible. parseCustomItemForm is
// the same parser handleSheetInventoryAddCustom uses in characters.go.
func (s *server) handleCustomItemUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	item, err := parseCustomItemForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing, err := charstore.GetCustomItemByID(s.charDB, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load custom item for update:", err)
		return
	}
	if err := charstore.UpdateCustomItem(s.charDB, id, item); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("update custom item:", err)
		return
	}
	http.Redirect(w, r, "/items/"+existing.Slug, http.StatusSeeOther)
}
