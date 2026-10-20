package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/RationalRouter/n5e/internal/charsheet"
	"github.com/RationalRouter/n5e/internal/charstore"
	"github.com/RationalRouter/n5e/internal/crafting"
)

func seedCraftingCatalog(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, cost_ryo, damage_dice, damage_type, properties, seal_rank, seal_applies_to, description) VALUES
		('weapon/katana', 'Katana', 'weapon', 30, '1d8', 'Slashing', 'Finesse', NULL, NULL, ''),
		('armor/chunin-jacket', 'Chunin Jacket', 'armor', 750, NULL, NULL, NULL, NULL, NULL, ''),
		('seal/weapon/minor/martial-seal', 'Martial Seal (Minor)', 'enhancement_seal', 250, NULL, NULL, NULL, 'D', 'weapon', 'A +1 bonus to your Taijutsu attack.'),
		('seal/weapon/greater/martial-seal', 'Martial Seal (Greater)', 'enhancement_seal', 1000, NULL, NULL, NULL, 'B', 'weapon', 'A +3 bonus to your Taijutsu attack.'),
		('seal/weapon/greater/elemental-seal', 'Elemental Seal (Greater)', 'enhancement_seal', 1200, NULL, NULL, NULL, 'B', 'weapon', 'Select one damage type.'),
		('seal/armor/minor/swift-seal', 'Swift Seal (Minor)', 'enhancement_seal', 200, NULL, NULL, NULL, 'D', 'armor', '+10 ft speed.')`); err != nil {
		t.Fatal(err)
	}
}

// TestSealCraftingPlannerFlow previews, saves, edits, duplicates and
// archives a crafted item through the planner's routes.
func TestSealCraftingPlannerFlow(t *testing.T) {
	s := testServer(t)
	seedCraftingCatalog(t, s)

	page := getPage(t, s, "/crafting/new")
	for _, want := range []string{`id="crafting-form"`, `name="base_slug"`, `id="crafting-result"`, "Katana"} {
		if !strings.Contains(page, want) {
			t.Errorf("editor lacks %s", want)
		}
	}
	if !strings.Contains(getPage(t, s, "/"), `href="/crafting"`) {
		t.Error("nav has no Seal Crafting link")
	}

	form := url.Values{
		"name": {"Storm Katana"}, "base_slug": {"weapon/katana"}, "quality": {"greater"},
		"seal":                                      {"seal/weapon/minor/martial-seal", "seal/weapon/greater/elemental-seal"},
		"choice:seal/weapon/greater/elemental-seal": {"fire"},
		"base_source": {"craft"}, "participants": {"1"}, "kit_tier": {"standard"}, "crafter_level": {"9"},
		"proficient": {"1"}, "has_forge": {"1"}, "has_seal_forge": {"1"},
	}
	w := postPopupForm(t, s, "/crafting/calculate", form)
	if w.Code != http.StatusOK {
		t.Fatalf("calculate: status %d: %s", w.Code, w.Body.String())
	}
	live := w.Body.String()
	for _, want := range []string{`id="crafting-seals"`, `id="crafting-result"`, "4 / 5", "1,600 Ryo"} {
		if !strings.Contains(live, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
	if strings.Contains(live, `id="crafting-form"`) {
		t.Error("preview answered the whole page")
	}
	// The other Martial Seal is blocked: same family.
	if !strings.Contains(live, `value="seal/weapon/greater/martial-seal" disabled`) {
		t.Error("same-family seal not blocked")
	}

	bad := url.Values{}
	for k, v := range form {
		bad[k] = v
	}
	bad["seal"] = append(bad["seal"], "seal/weapon/greater/martial-seal")
	if w := postPopupForm(t, s, "/crafting", bad); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid save: status %d", w.Code)
	}

	w = postPopupForm(t, s, "/crafting", form)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("save: status %d: %s", w.Code, w.Body.String())
	}
	items, err := charstore.ListCraftedItems(s.charDB, false)
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %+v, %v", items, err)
	}
	it := items[0]
	if w.Header().Get("Location") != "/crafting/1?saved=1" || it.Slug != "crafted/storm-katana-1" ||
		len(it.Recipe.Seals) != 2 || it.Recipe.Seals[1].Choices[0] != "fire" {
		t.Errorf("saved %+v at %q", it, w.Header().Get("Location"))
	}
	if page := getPage(t, s, "/crafting/1?saved=1"); !strings.Contains(page, "Storm Katana") || !strings.Contains(page, `action="/crafting/1/update"`) {
		t.Error("edit page wrong")
	}

	form.Set("name", "Blaze Katana")
	if w := postPopupForm(t, s, "/crafting/1/update", form); w.Code != http.StatusSeeOther {
		t.Fatalf("update: status %d", w.Code)
	}
	if w := postPopupForm(t, s, "/crafting/1/duplicate", url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("duplicate: status %d", w.Code)
	}
	if w := postPopupForm(t, s, "/crafting/1/archive", url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("archive: status %d", w.Code)
	}
	list := getPage(t, s, "/crafting")
	if !strings.Contains(list, "Blaze Katana (copy)") || strings.Contains(list, ">Blaze Katana<") {
		t.Errorf("active list wrong")
	}
	if !strings.Contains(getPage(t, s, "/crafting?archived=1"), ">Blaze Katana<") {
		t.Error("archived list lacks the archived item")
	}
}

// TestCraftedItemPageAndGive renders a crafted item's page and adds it to a
// character, paid and unpaid, including a refusal the purse can't cover.
func TestCraftedItemPageAndGive(t *testing.T) {
	s := testServer(t)
	seedCraftingCatalog(t, s)
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, ryo) VALUES ('Sakura', 2000)`); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"name": {"Storm Katana"}, "base_slug": {"weapon/katana"}, "quality": {"greater"},
		"seal":                                      {"seal/weapon/minor/martial-seal", "seal/weapon/greater/elemental-seal"},
		"choice:seal/weapon/greater/elemental-seal": {"fire"},
		"base_source": {"craft"}, "participants": {"1"}, "kit_tier": {"standard"}, "crafter_level": {"9"},
		"proficient": {"1"}, "has_forge": {"1"}, "has_seal_forge": {"1"},
	}
	if w := postPopupForm(t, s, "/crafting", form); w.Code != http.StatusSeeOther {
		t.Fatalf("save: status %d", w.Code)
	}
	it, err := charstore.GetCraftedItem(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	page := getPage(t, s, "/items/"+it.Slug)
	for _, want := range []string{"Storm Katana", "Elemental Seal (Greater)", `action="/crafting/1/give"`, `href="/crafting/1"`, "Sakura"} {
		if !strings.Contains(page, want) {
			t.Errorf("item page lacks %q", want)
		}
	}

	ryo := func() float64 {
		var v float64
		if err := s.charDB.QueryRow(`SELECT ryo FROM characters WHERE id = 1`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	qty := func() int {
		var n int
		s.charDB.QueryRow(`SELECT COALESCE(SUM(quantity), 0) FROM character_inventory WHERE character_id = 1 AND item_slug = ?`, it.Slug).Scan(&n)
		return n
	}

	w := postPopupForm(t, s, "/crafting/1/give", url.Values{"character_id": {"1"}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/items/"+it.Slug+"?added=1" || qty() != 1 || ryo() != 2000 {
		t.Fatalf("free give: %d %q qty %d ryo %v", w.Code, w.Header().Get("Location"), qty(), ryo())
	}
	if page := getPage(t, s, "/items/"+it.Slug+"?added=1"); !strings.Contains(page, "Added to Sakura") ||
		!strings.Contains(page, `href="/characters/1#tab-inventory"`) {
		t.Error("no added flash with an inventory link")
	}

	w = postPopupForm(t, s, "/crafting/1/give", url.Values{"character_id": {"1"}, "pay": {"cost"}})
	if w.Code != http.StatusSeeOther || qty() != 2 || ryo() >= 2000 {
		t.Fatalf("paid give: %d qty %d ryo %v", w.Code, qty(), ryo())
	}
	left := ryo()

	w = postPopupForm(t, s, "/crafting/1/give", url.Values{"character_id": {"1"}, "pay": {"value"}})
	if w.Header().Get("Location") != "/items/"+it.Slug+"?short=1" || qty() != 2 || ryo() != left {
		t.Fatalf("unaffordable give: %q qty %d ryo %v", w.Header().Get("Location"), qty(), ryo())
	}
	if !strings.Contains(getPage(t, s, "/items/"+it.Slug+"?short=1"), "enough Ryo") {
		t.Error("no short flash")
	}
}

// TestCraftedItemMechanics equips crafted items and checks they play as
// their base items: the weapon's attack row and the armor's AC, under the
// crafted item's own name, plus the Inventory pane's Crafted group.
func TestCraftedItemMechanics(t *testing.T) {
	s := testServer(t)
	seedCraftingCatalog(t, s)
	if _, err := s.rulesDB.Exec(`INSERT INTO equipment (slug, name, kind, cost_ryo, ac_bonus) VALUES ('armor/test-plate', 'Test Plate', 'armor', 500, 7)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO equipment (slug, name, kind, cost_ryo, seal_rank, seal_applies_to, description) VALUES
		('seal/armor/refined/swift-seal', 'Swift Seal (Refined)', 'enhancement_seal', 500, 'C', 'armor', '+10 speed.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Smith', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	katana, err := charstore.AddCraftedItem(s.charDB, charstore.CraftedItem{Recipe: crafting.Recipe{
		Name: "Storm Katana", BaseSlug: "weapon/katana", Quality: "standard", Crafting: crafting.DefaultCrafting(),
		Seals: []crafting.SealPick{{Slug: "seal/weapon/minor/martial-seal"}}}})
	if err != nil {
		t.Fatal(err)
	}
	plate, err := charstore.AddCraftedItem(s.charDB, charstore.CraftedItem{Recipe: crafting.Recipe{
		Name: "Swift Plate", BaseSlug: "armor/test-plate", Quality: "standard", Crafting: crafting.DefaultCrafting(),
		Seals: []crafting.SealPick{{Slug: "seal/armor/refined/swift-seal"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{katana.Slug, plate.Slug} {
		if ok, err := s.carriedItemExists(slug); err != nil || !ok {
			t.Fatalf("carriedItemExists(%s) = %v, %v", slug, ok, err)
		}
		if _, err := s.charDB.Exec(`INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES (1, ?, 1, 1)`, slug); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := s.carriedItemExists("crafted/nothing-9"); ok {
		t.Error("unknown crafted slug accepted")
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.AC == nil || *sheet.AC != 17 {
		t.Errorf("AC = %d, want 17 from the crafted plate's base", *sheet.AC)
	}
	// The katana's D-Rank Martial Seal applies at level 1; the plate's
	// C-Rank Swift Seal needs level 5.
	var taijutsu int
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Taijutsu" {
			taijutsu = a.Modifier
		}
	}
	if want := sheet.Abilities["str"].Modifier + sheet.ProficiencyBonus + 1; taijutsu != want {
		t.Errorf("Taijutsu attack = %d, want %d", taijutsu, want)
	}
	if sheet.Speed != 30 || sheet.Seals.Item(plate.Slug) == nil || !sheet.Seals.Item(plate.Slug).Gated {
		t.Errorf("speed %d, plate %+v", sheet.Speed, sheet.Seals.Item(plate.Slug))
	}
	page := getPage(t, s, "/characters/1")
	if !strings.Contains(page, "Needs level 5") || strings.Count(page, "sheet-squares sheet-skills") < 2 {
		t.Error("sheet lacks the level badge or the crafted equip refresh list")
	}

	inv, err := s.loadCharacterInventory(1, "")
	if err != nil {
		t.Fatal(err)
	}
	if inv[0].Name != "Storm Katana" || inv[0].Kind != "weapon" || inv[1].Kind != "armor" {
		t.Errorf("inventory = %+v", inv)
	}
	attacks, err := s.buildAttacks(1, inv, sheet, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(attacks) != 1 || attacks[0].Name != "Storm Katana" || attacks[0].DamageDice != "1d8" || attacks[0].Slug != katana.Slug {
		t.Errorf("attacks = %+v", attacks)
	}

	groups, err := s.loadSheetItemLibrary(map[string]bool{katana.Slug: true})
	if err != nil {
		t.Fatal(err)
	}
	last := groups[len(groups)-1]
	if last.Label != "Crafted" || len(last.Entries) != 2 || !last.Entries[0].Owned {
		t.Errorf("last library group = %+v", last)
	}
}
