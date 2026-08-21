package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
	"github.com/sergio/n5e/internal/schema"
)

// testServer builds an in-memory server{} with both schemas applied but no
// seeded rules content — html/template.Must only catches parse errors at
// package init, not execution errors (a bad field reference, a nil map
// index), so this exercises handleCharacterSheet end-to-end the same way
// TestCompute* in internal/charsheet does for Compute itself.
func testServer(t *testing.T) *server {
	t.Helper()
	// A bare ":memory:" DSN gives every new connection its own private,
	// empty database — fine as long as nothing ever needs two connections
	// to the same *sql.DB at once, but a query issued while another Rows
	// cursor from the same *sql.DB is still open (e.g. lookupCarriedItem's
	// custom_items lookup, run per-row inside loadCharacterInventory's own
	// open Rows loop) checks out a second connection, which then can't see
	// any of the schema the first connection migrated. "cache=shared" keeps
	// every connection opened against the same DSN pointed at the same
	// underlying database; t.Name() keeps that DSN — and so the shared
	// database it names — unique per test.
	rulesDB, err := sql.Open("sqlite", "file:"+t.Name()+"-rules?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rulesDB.Close() })
	if err := schema.Apply(rulesDB, schema.Rules); err != nil {
		t.Fatal(err)
	}

	charDB, err := sql.Open("sqlite", "file:"+t.Name()+"-characters?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { charDB.Close() })
	if err := schema.Apply(charDB, schema.Characters); err != nil {
		t.Fatal(err)
	}

	return &server{rulesDB: rulesDB, charDB: charDB}
}

// TestHandleCharacterSheetRenders exercises the redesigned character sheet
// template end-to-end for a bare draft character (no class/clan/items/
// jutsu/chat log yet) — the emptiest, most panic-prone case for a template
// full of {{range}}/{{if}} over possibly-nil slices.
func TestHandleCharacterSheetRenders(t *testing.T) {
	s := testServer(t)
	res, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Test Genin', 10, 14, 12, 8, 13, 15)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	req := httptest.NewRequest(http.MethodGet, "/characters/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCharacterSheet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	_ = id
}

// TestHandleCharacterSheetRendersWithData covers the non-empty path:
// inventory, jutsu, granted class/clan features, a custom feature, and a
// chat log line all present at once — the fields the empty-character test
// above can't reach.
func TestHandleCharacterSheetRendersWithData(t *testing.T) {
	s := testServer(t)

	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/genin', 'Genin', 8, 6)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, level, description, sort_order)
		VALUES ('class/genin/feature/focus', 'class/genin', 'Focus', 1, 'A focused mind.', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO clans (slug, name, speed_feet) VALUES ('clan/test', 'Test Clan', 35)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO clan_features (slug, clan_slug, name, level, description, sort_order)
		VALUES ('clan/test/feature/trait', 'clan/test', 'Trait', NULL, 'Always on.', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, damage_dice, damage_type, properties)
		VALUES ('item/kunai', 'Kunai', 'weapon', '1d4', 'Piercing', 'Finesse, Thrown (20/60)')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/test', 'Test Jutsu', 'Ninjutsu', 'D', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 'Ninjutsu', 'A test jutsu.')`); err != nil {
		t.Fatal(err)
	}

	res, err := s.charDB.Exec(`
		INSERT INTO characters (name, clan_slug, base_str, base_dex, base_con, base_int, base_wis, base_cha, current_hp, temp_hp, base_temp_hp)
		VALUES ('Test Genin', 'clan/test', 10, 14, 12, 8, 13, 15, 5, 2, 2)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/genin', 1, 0)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES (?, 'item/kunai', 1, 1)`, id); err != nil {
		t.Fatal(err)
	}
	// A custom item (part 11) — exercises loadCharacterInventory's custom/
	// slug branch (lookupCarriedItem reading name/kind/bulk from the local
	// item library, custom_items, instead of rules.db) and the Inventory
	// tab rendering it as a real link like any catalogue row.
	customItem, err := charstore.AddCustomItem(s.charDB, charstore.CustomItem{
		Name: "Sealed Scroll", Kind: "Scroll", Bulk: sql.NullFloat64{Float64: 0.5, Valid: true},
		Description: "A mystery.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (?, ?, 1, 1)`, id, customItem.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_jutsu (character_id, jutsu_slug, learned_at_level, source) VALUES (?, 'jutsu/test', 1, 'learned')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_custom_features (character_id, name, source_label, description, sort_order)
		VALUES (?, 'Amethyst Gemstone', 'Other: Magic Item', 'Glows faintly.', 0)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_chat_log (character_id, kind, text, crit) VALUES (?, 'roll', '1d20(20) + 2 = 22', 'nat20')`, id); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/characters/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCharacterSheet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}

	// The equipped Kunai is Finesse, so with STR 10 (+0) and DEX 14 (+2)
	// it uses DEX. N5E's proficiency bonus starts at +3 at level 1 (not
	// 5e's +2 — see the xp_levels seed), so the attack is +5 and the
	// damage is 1d4+2. This pins the ability-selection rule, the fact that
	// the bonus comes from the rules table rather than a 5e assumption,
	// and that the row renders as a rollable button at all.
	body := w.Body.String()
	for _, want := range []string{
		`data-modifier="5" data-label="Kunai Attack"`,
		`data-label="Kunai Damage"`,
		`data-sides="4" data-count="1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("attack table missing %s", want)
		}
	}

	// The four jutsu attack modifiers. INT 8 (-1) + prof 3 = +2 Ninjutsu,
	// WIS 13 (+1) + 3 = +4 Genjutsu, STR 10 (+0) + 3 = +3 Taijutsu, and
	// (part 10: Bukijutsu is its own independent kind now, no longer a
	// mirror of Taijutsu) STR 10 (+0) + 3 = +3 Bukijutsu too, same default
	// ability as Taijutsu but its own row.
	for _, want := range []string{
		`data-modifier="2" data-label="Ninjutsu Attack"`,
		`data-modifier="4" data-label="Genjutsu Attack"`,
		`data-modifier="3" data-label="Taijutsu Attack"`,
		`data-modifier="3" data-label="Bukijutsu Attack"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("attack modifiers missing %s", want)
		}
	}

	// Speed and hit dice sit in the square row now, not in the mid column's
	// AC box and the vitals block respectively.
	if !strings.Contains(body, `id="sheet-squares"`) {
		t.Error("square row fragment is missing from the full page render")
	}
	if strings.Contains(body, `id="sheet-vitals"`) && strings.Contains(body, ">Hit Dice<") {
		t.Error("hit dice still rendered in the vitals block as well as the squares")
	}

	// Part 11: the custom "Sealed Scroll" row renders as a real link to its
	// own detail page (/items/custom/...), same as any catalogue row, with
	// its Type read from the local item library (custom_items) — not a
	// name-toggle reopening the creation form, and not "from creation"/"—"
	// the way an untouched custom row used to show.
	for _, want := range []string{
		`href="/items/` + customItem.Slug + `"`,
		`>Sealed Scroll<`,
		`>Scroll<`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("custom inventory row/edit-form missing %s", want)
		}
	}
}

// TestCharacterSheetLibraryPanes covers the part-7 sheet restructure: the
// three library tabs, their drop targets, the custom attack rows, and the
// per-table trash cans.
//
// The href assertions are the point of the poison row: everything the bag
// holds used to link to /items/{slug}, which 404s for a poison or a trap
// now that those can be carried.
func TestCharacterSheetLibraryPanes(t *testing.T) {
	s := testServer(t)

	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/genin', 'Genin', 8, 6)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, bulk) VALUES ('item/rope', 'Rope', 'gear', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO poisons (slug, name, craft_dc, poison_rank, cost_ryo, bulk, description)
		VALUES ('poison/wolfsbane', 'Wolfsbane', 13, 'D', 50, 0.5, 'Nasty.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO trap_templates (slug, name, build_dc, toolkit_required, description)
		VALUES ('trap/pit', 'Pit Trap', 12, 'Trapper''s Kit', 'A hole.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO feats (slug, name, category, prerequisites, description)
		VALUES ('feat/tough', 'Tough', 'general', NULL, 'More hit points.')`); err != nil {
		t.Fatal(err)
	}

	res, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha, current_hp)
		VALUES ('Pane Test', 10, 10, 10, 10, 10, 10, 5)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/genin', 1, 0)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES (?, 'poison/wolfsbane', 1, 0)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_feats (character_id, feat_slug, chosen_at_level, source) VALUES (?, 'feat/tough', 1, 'other')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCustomAttack(s.charDB, id, charstore.CustomAttack{
		Kind: "jutsu", Name: "Overcharged Fireball", AttackBonus: 7,
		DamageCount: 3, DamageSides: 6, DamageBonus: 2, DamageType: "Fire",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/characters/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCharacterSheet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	for _, want := range []string{
		`data-tab="feats"`,                          // the new third library tab
		`data-tab-panel="feats"`,                    //
		`data-drop-into="sheet-jutsu-add"`,          // each pane's drop target…
		`data-drop-into="sheet-feat-add"`,           //
		`data-drop-into="sheet-item-add"`,           //
		`id="sheet-jutsu-library-list"`,             // …and its library list
		`id="sheet-feat-list"`,                      //
		`id="sheet-item-list"`,                      //
		`name="item_slug" value="poison/wolfsbane"`, // the library's own add button
		`href="/poisons/poison/wolfsbane"`,          // routed to the right rules page
		`href="/traps/trap/pit"`,                    //
		`name="slug" value="feat/tough"`,            //
		`data-label="Overcharged Fireball Attack"`,  // the custom attack rolls
		`data-sides="6" data-count="3"`,             //
		`/sheet/attacks/1/delete`,                   // and can be deleted
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sheet is missing %s", want)
		}
	}

	// The standalone plain-list jutsu tab and the "type it in yourself"
	// inventory tab were replaced, not kept alongside the new panes.
	// Summons is always present regardless of class (unlike Puppets, which
	// only renders for a character with Puppet Master levels — this test
	// character has none, see TestCharacterSheetHasPuppetsTabWhenGated for
	// the gated-on case), so the expected count is 6, not 5.
	if strings.Count(body, `data-tab-panel=`) != 6 {
		t.Errorf("expected exactly 6 tab panels (core, bio, jutsu, feats, inventory, summons), got %d",
			strings.Count(body, `data-tab-panel=`))
	}
}

// TestComma pins the exact bug this function exists to prevent: a ryo
// total in the millions rendering as "1.0018e+06". Go's default float64
// formatting (%v, and therefore a bare {{.Sheet.Ryo}}) switches to
// exponent notation above ~1e21 for %v but well earlier for the shortest
// representation of many values, and the sheet must never show one.
func TestComma(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{1001800, "1,001,800"},
		{1234567890, "1,234,567,890"},
		{-2500, "-2,500"},
		{1500.5, "1,500.5"},
	}
	for _, c := range cases {
		if got := comma(c.in); got != c.want {
			t.Errorf("comma(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHandleSheetRyo covers the "the leading character carries the intent"
// rule: a bare number sets the total outright, a signed one adjusts it.
// TestHandleSheetSubclass covers the sheet's inline subclass picker — the
// only place in the app a subclass can be chosen at all (see
// handleSheetSubclass's doc comment). Set, change, clear, and the
// server-side validation that rejects a subclass/class pairing that
// doesn't actually match.
func TestHandleSheetSubclass(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		('class/puppet-master', 'Puppet Master', 8, 8), ('class/hunter-nin', 'Hunter-Nin', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/puppet-master/group/puppet-techniques', 'class/puppet-master', 'Puppet Techniques')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/puppet-master/group/puppet-techniques/blue-technique-warmaster',
		 'class/puppet-master/group/puppet-techniques', 'Blue Technique ~ Warmaster'),
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut',
		 'class/puppet-master/group/puppet-techniques', 'Purple Technique ~ Juggernaut')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kankuro', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/puppet-master', 2, 0)`,
	); err != nil {
		t.Fatal(err)
	}

	post := func(classSlug, subclassSlug string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/subclass",
			strings.NewReader(url.Values{"class_slug": {classSlug}, "subclass_slug": {subclassSlug}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetSubclass(w, req)
		return w.Code
	}
	current := func() string {
		t.Helper()
		var slug string
		err := s.charDB.QueryRow(`SELECT subclass_slug FROM character_subclasses WHERE character_id = 1`).Scan(&slug)
		if err == sql.ErrNoRows {
			return ""
		}
		if err != nil {
			t.Fatal(err)
		}
		return slug
	}

	if code := post("class/puppet-master", "class/puppet-master/group/puppet-techniques/blue-technique-warmaster"); code != http.StatusOK {
		t.Fatalf("pick Warmaster: status %d", code)
	}
	if got := current(); got != "class/puppet-master/group/puppet-techniques/blue-technique-warmaster" {
		t.Errorf("after picking Warmaster, current subclass = %q", got)
	}

	// Changing the pick must remove the old row, not add a second one.
	if code := post("class/puppet-master", "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut"); code != http.StatusOK {
		t.Fatalf("change to Juggernaut: status %d", code)
	}
	var count int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_subclasses WHERE character_id = 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("after changing subclass, character_subclasses has %d rows, want 1", count)
	}
	if got := current(); got != "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut" {
		t.Errorf("after changing to Juggernaut, current subclass = %q", got)
	}

	// Clearing removes it entirely.
	if code := post("class/puppet-master", ""); code != http.StatusOK {
		t.Fatalf("clear subclass: status %d", code)
	}
	if got := current(); got != "" {
		t.Errorf("after clearing, current subclass = %q, want none", got)
	}

	// A class the character doesn't have.
	if code := post("class/hunter-nin", "class/puppet-master/group/puppet-techniques/blue-technique-warmaster"); code != http.StatusBadRequest {
		t.Errorf("class the character doesn't have: status %d, want 400", code)
	}
	// A subclass that doesn't belong to the given class's group.
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES ('class/hunter-nin/group/hunters-creeds', 'class/hunter-nin', 'Hunters Creeds')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES ('class/hunter-nin/group/hunters-creeds/wolves-legacy', 'class/hunter-nin/group/hunters-creeds', "Wolf's Legacy")`,
	); err != nil {
		t.Fatal(err)
	}
	if code := post("class/puppet-master", "class/hunter-nin/group/hunters-creeds/wolves-legacy"); code != http.StatusBadRequest {
		t.Errorf("mismatched class/subclass pair: status %d, want 400", code)
	}
}

func TestHandleSheetRyo(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha, ryo)
		VALUES ('Rich Genin', 10, 10, 10, 10, 10, 10, 500)`); err != nil {
		t.Fatal(err)
	}

	post := func(value string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/ryo",
			strings.NewReader(url.Values{"value": {value}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetRyo(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("POST value=%q: status %d, body %s", value, w.Code, w.Body.String())
		}
		return w.Body.String()
	}

	var total float64
	read := func() float64 {
		t.Helper()
		if err := s.charDB.QueryRow(`SELECT ryo FROM characters WHERE id = 1`).Scan(&total); err != nil {
			t.Fatal(err)
		}
		return total
	}

	post("+200")
	if got := read(); got != 700 {
		t.Errorf("after +200: ryo = %v, want 700", got)
	}
	post("-50")
	if got := read(); got != 650 {
		t.Errorf("after -50: ryo = %v, want 650", got)
	}
	post("2000")
	if got := read(); got != 2000 {
		t.Errorf("after bare 2000: ryo = %v, want 2000 (bare numbers set, not add)", got)
	}
	// Typed straight back in from the comma-formatted display.
	body := post("1,001,800")
	if got := read(); got != 1001800 {
		t.Errorf("after 1,001,800: ryo = %v, want 1001800", got)
	}
	if !strings.Contains(body, "1,001,800") {
		t.Errorf("fragment should render the comma-formatted total, got: %s", body)
	}
	if strings.Contains(body, "e+") {
		t.Errorf("fragment fell back to scientific notation: %s", body)
	}
}

// TestHandleSheetPortrait covers the upload round trip and, specifically,
// the html/template gotcha it depends on: a data: URL interpolated into an
// <img src> without imageDataURL's vouching gets rewritten to "#ZgotmplZ"
// by the template package's URL filter, which would render as a silently
// broken image rather than any kind of error.
func TestHandleSheetPortrait(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Portrait Genin', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	// A real 1x1 PNG — http.DetectContentType sniffs the actual signature,
	// so a placeholder byte slice would be rejected as it should be.
	png, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("portrait", "face.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/portrait", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetPortrait(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upload: status %d, body %s", w.Code, w.Body.String())
	}

	var stored string
	if err := s.charDB.QueryRow(`SELECT portrait FROM characters WHERE id = 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "data:image/png;base64,") {
		t.Errorf("stored portrait = %.40q, want a data:image/png URL", stored)
	}

	fragment := w.Body.String()
	if strings.Contains(fragment, "ZgotmplZ") {
		t.Error("template URL filter stripped the data: URL — imageDataURL is not vouching for it")
	}
	if !strings.Contains(fragment, "data:image/png;base64,") {
		t.Errorf("fragment should embed the portrait, got: %s", fragment)
	}

	// A non-image must be refused rather than stored and later rendered as
	// a broken <img>.
	var textBody bytes.Buffer
	tw := multipart.NewWriter(&textBody)
	textPart, _ := tw.CreateFormFile("portrait", "notes.txt")
	textPart.Write([]byte("this is plainly not an image"))
	tw.Close()
	req = httptest.NewRequest(http.MethodPost, "/characters/1/sheet/portrait", &textBody)
	req.Header.Set("Content-Type", tw.FormDataContentType())
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w = httptest.NewRecorder()
	s.handleSheetPortrait(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("non-image upload: status %d, want 400", w.Code)
	}

	// Removing puts the column back to NULL, not to an empty string.
	req = httptest.NewRequest(http.MethodPost, "/characters/1/sheet/portrait/delete", nil)
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w = httptest.NewRecorder()
	s.handleSheetPortraitDelete(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: status %d, body %s", w.Code, w.Body.String())
	}
	var after sql.NullString
	if err := s.charDB.QueryRow(`SELECT portrait FROM characters WHERE id = 1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after.Valid {
		t.Errorf("portrait after delete = %q, want NULL", after.String)
	}
}

// TestSheetJavaScriptHooks asserts the contract between the sheet template
// and the scripts that wire it up. Every selector below is one a file in
// static/js queries by name; if a template edit renames or drops one, the
// feature silently stops working with no build error, no test failure
// anywhere else, and nothing in the page to suggest anything is wrong —
// which is exactly how a broken sheet control has shipped here before.
//
// This is not a substitute for clicking through the real app, but it does
// catch the whole class of "the handler is looking for an element that
// isn't there any more".
func TestSheetJavaScriptHooks(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Hook Test', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/characters/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCharacterSheet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	hooks := map[string]string{
		"sheet-vitals.js":                `id="sheet-vitals"`,
		"sheet-vitals.js hp":             `class="sheet-hp-box sheet-edit-box"`,
		"sheet-vitals.js edit role":      `data-role="display"`,
		"sheet-ryo (fragment target)":    `id="sheet-ryo"`,
		"sheet-ryo (absolute-set field)": `data-field="value"`,
		"mon coin icon":                  `/static/img/mon-coin.svg`,
		"sheet-portrait.js":              `class="sheet-portrait-form"`,
		"sheet-portrait.js target":       `id="sheet-portrait"`,
		"sheet-calculator.js display":    `id="sheet-calc-display"`,
		"sheet-calculator.js keys":       `data-calc="="`,
		"sheet-abilities.js endpoint":    `data-ability-endpoint=`,
		"sheet-abilities.js input":       `class="sheet-ability-input"`,
		"sheet-rest.js":                  `sheet-short-rest-form`,
		"sheet-rest.js hit die":          `data-hit-die=`,
		"sheet-toggles.js":               `class="sheet-inline-form sheet-toggle-form"`,
		"sheet-bio.js":                   `id="sheet-bio-form"`,
		"sheet-chat.js":                  `id="sheet-chat-log"`,
		"sheet-tabs.js":                  `data-tab-panel="core"`,
		"sheet-tabs.js inventory":        `data-tab-panel="inventory"`,
		"sheet-inventory.js add":         `class="sheet-inventory-add-form"`,
		"sheet-toggles.js skills target": `data-target="sheet-skills"`,
		"sheet_skills fragment target":   `id="sheet-skills"`,
		"sheet-popup.js":                 `id="sheet-item-popup"`,
		"confirm-submit.js delete":       `data-confirm="Delete Hook Test?`,
		"initiative square":              `data-label="Initiative"`,
	}
	for name, hook := range hooks {
		if !strings.Contains(body, hook) {
			t.Errorf("%s: template is missing %s", name, hook)
		}
	}

	// Initiative used to be in the mid column's stat trio as well. Two
	// Initiative boxes means one of them isn't the clickable one, and
	// which one is anybody's guess from the player's side.
	if n := strings.Count(body, `data-label="Initiative"`); n != 1 {
		t.Errorf("found %d Initiative boxes, want exactly 1", n)
	}

	// Every <select> of abilities has to actually contain them. This shipped
	// empty once: "sheet_squares" is a {{define}} invoked with only {ID, Sheet},
	// so the $.AbilityOrder the picker ranged over was nil and the dropdown
	// rendered with no options at all — a template range over a missing field
	// is silent, and nothing else on the page looked wrong. Codes are shown
	// uppercase.
	// Checked per-select, not across the whole page: the codes also appear in
	// the attack composer, so a page-wide search would have passed while the
	// initiative dropdown was empty.
	for _, sel := range []string{
		`<select name="ability">`,        // initiative picker
		`<select name="attack_ability">`, // attack modifier composer
		`<select name="damage_ability">`, // damage modifier composer
	} {
		start := strings.Index(body, sel)
		if start < 0 {
			t.Errorf("template is missing %s", sel)
			continue
		}
		rest := body[start:]
		options := rest[:strings.Index(rest, "</select>")]
		for _, ability := range charsheet.Abilities {
			if !strings.Contains(options, `>`+strings.ToUpper(ability)+`</option>`) {
				t.Errorf("%s has no %s option", sel, strings.ToUpper(ability))
			}
		}
	}
	if !strings.Contains(body, `<select name="attack_prof">`) {
		t.Error("template is missing the attack proficiency picker")
	}
	// Lowercase codes would mean a picker slipped through without {{upper}}.
	if strings.Contains(body, `>str</option>`) || strings.Contains(body, `>dex</option>`) {
		t.Error("an ability picker is rendering lowercase codes; use {{upper .}}")
	}

	// The coin has to actually be in the embedded filesystem too — the
	// go:embed directive lists directories explicitly, so a new one that
	// isn't added there produces a 404 at runtime and nothing at build.
	if _, err := staticFS.ReadFile("static/img/mon-coin.svg"); err != nil {
		t.Errorf("mon-coin.svg is not embedded: %v", err)
	}
}

// TestSheetNativeSubmitRedirects pins the other half of the fragment
// negotiation: a form that JavaScript did not intercept must come back as
// a redirect to the sheet, not as a bare HTML fragment. Answering a native
// submission with a fragment is what put the player on a stray, unstyled
// "backend page" with no way back and no sign the change had saved.
func TestSheetNativeSubmitRedirects(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('No JS', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	// No X-Requested-With — exactly what a browser sends for a plain form.
	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/base-temp-hp",
		strings.NewReader(url.Values{"value": {"7"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetBaseTempHP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("native submit: status %d, want 303 redirect; body: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/characters/1" {
		t.Errorf("redirect target = %q, want /characters/1", loc)
	}
	// The change still has to have been saved before redirecting.
	var baseTempHP int
	if err := s.charDB.QueryRow(`SELECT base_temp_hp FROM characters WHERE id = 1`).Scan(&baseTempHP); err != nil {
		t.Fatal(err)
	}
	if baseTempHP != 7 {
		t.Errorf("base_temp_hp = %d, want 7 — a redirect must not mean the write was skipped", baseTempHP)
	}
}

// TestPackChoices covers the background pack line becoming a real choice.
// The text is prose from the book, complete with a typographic apostrophe,
// and the parsing has to survive that.
func TestPackChoices(t *testing.T) {
	s := testServer(t)
	// INSERT OR IGNORE because the schema's own seed migrations may already
	// carry these rows; the test only needs them to exist, not to own them.
	for _, pack := range []struct{ slug, name string }{
		{"gear/captains-pack", "Captain's Pack"},
		{"gear/crafters-pack", "Crafter's Pack"},
		{"gear/explorers-pack", "Explorer's Pack"},
		{"gear/infiltrators-pack", "Infiltrator's Pack"},
		{"gear/travelers-pack", "Traveler's Pack"},
		{"gear/shinobi-backpack", "Shinobi Backpack"},
	} {
		if _, err := s.rulesDB.Exec(
			`INSERT OR IGNORE INTO equipment (slug, name, kind) VALUES (?, ?, 'gear')`, pack.slug, pack.name,
		); err != nil {
			t.Fatal(err)
		}
	}

	// Curly apostrophes, exactly as the book text stores them.
	got, err := s.packChoices("Infiltrator’s or Crafter’s Pack (Choose one).")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d choices, want 2: %+v", len(got), got)
	}
	// Ordered as the sentence names them, not alphabetically.
	if got[0].Slug != "gear/infiltrators-pack" || got[1].Slug != "gear/crafters-pack" {
		t.Errorf("choices = %+v, want infiltrator's then crafter's", got)
	}

	// A line naming nothing resolvable returns nothing, so the caller
	// falls back to showing the prose.
	if got, err := s.packChoices("A bag of holding, probably"); err != nil {
		t.Fatal(err)
	} else if len(got) != 0 {
		t.Errorf("unresolvable line produced %+v, want no choices", got)
	}
}

// TestSheetInventoryEquipDrivesAC walks the chain the Inventory tab exists
// to make work: adding armor, equipping it, and seeing AC actually move.
// Before the tab existed nothing could ever be marked equipped, so AC
// rendered as "—" for every character in the app.
// Weapon attack rows are derived from the weapon's printed properties, and
// character_weapon_attack_options overrides any part of that. The point of
// storing parts rather than a total is that the total then tracks the
// character, which is what the last assertion checks.
func TestSheetWeaponAttackOptions(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, damage_dice, damage_type, properties)
		VALUES ('weapon/kunai', 'Kunai', 'weapon', '1d4', 'Piercing', 'Finesse, Thrown')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Blade', 10, 18, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped)
		VALUES (1, 'weapon/kunai', 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	invID, _ := res.LastInsertId()

	rows := func() attackRow {
		t.Helper()
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
		if err != nil {
			t.Fatal(err)
		}
		inv, err := s.loadCharacterInventory(1)
		if err != nil {
			t.Fatal(err)
		}
		attacks, err := s.buildAttacks(1, inv, sheet)
		if err != nil {
			t.Fatal(err)
		}
		if len(attacks) != 1 {
			t.Fatalf("got %d attack rows, want 1", len(attacks))
		}
		return attacks[0]
	}

	// Derived: Finesse with Dex 18 (+4) beating Str 10 (+0), full proficiency.
	base := rows()
	if base.AttackAbility != "dex" || !base.Derived {
		t.Fatalf("derived row = %s (derived %v), want dex/true", base.AttackAbility, base.Derived)
	}

	post := func(v url.Values) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/weapon-attack/1", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		req.SetPathValue("rowID", strconv.FormatInt(invID, 10))
		w := httptest.NewRecorder()
		s.handleSheetWeaponAttackOptions(w, req)
		return w.Code
	}

	if code := post(url.Values{
		"attack_ability": {"str"}, "attack_prof": {"half"}, "attack_bonus": {"1"},
		"damage_ability": {"dex"}, "damage_bonus": {"2"},
	}); code != http.StatusSeeOther {
		t.Fatalf("set options: status %d", code)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := rows()
	wantAttack := sheet.Abilities["str"].Modifier + sheet.ProficiencyBonus/2 + 1
	wantDamage := sheet.Abilities["dex"].Modifier + 2
	if got.AttackBonus != wantAttack || got.DamageBonus != wantDamage {
		t.Errorf("attack/damage = %+d/%+d, want %+d/%+d",
			got.AttackBonus, got.DamageBonus, wantAttack, wantDamage)
	}
	if got.Derived {
		t.Error("row still reports itself as derived after an override")
	}

	// Clearing back to the defaults drops the row entirely, so the weapon goes
	// back to being fully derived rather than keeping a frozen copy.
	if code := post(url.Values{
		"attack_ability": {""}, "attack_prof": {"full"}, "attack_bonus": {"0"},
		"damage_ability": {""}, "damage_bonus": {"0"},
	}); code != http.StatusSeeOther {
		t.Fatalf("clear options: status %d", code)
	}
	cleared := rows()
	if !cleared.Derived || cleared.AttackBonus != base.AttackBonus {
		t.Errorf("cleared row = %+d (derived %v), want the derived %+d/true",
			cleared.AttackBonus, cleared.Derived, base.AttackBonus)
	}
}

// TestBuildAttacksIncludesExplosiveTagsAndBombs covers the equipment.kind
// gate in buildAttacks: Paper Bombs, Flash Tags, and the rest of that family
// are catalogued as kind='tool', not kind='weapon' (the book prints them as
// a tool-slot item), but they carry a printed save_dc same as a real
// explosive, so they belong in the Attacks & Jutsu table like any other
// equipped rollable item. An ordinary tool (no save_dc — a lockpick set,
// a kit) must stay excluded, same as before this fix.
func TestBuildAttacksIncludesExplosiveTagsAndBombs(t *testing.T) {
	s := testServer(t)
	// tool/paper-bombs and tool/flash-tag already exist in the seeded rules
	// schema (migration 0017) — testServer applies the real migrations, not
	// a hand-rolled empty schema, so the real catalog rows are used as-is
	// rather than re-declared here. Only the negative-case fixture (an
	// ordinary tool with no save_dc) needs inserting.
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, damage_dice, damage_type, save_dc)
		VALUES ('tool/lockpick-set', 'Lockpick Set', 'tool', NULL, NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Demolitionist', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES
			(1, 'tool/paper-bombs', 1, 1),
			(1, 'tool/flash-tag', 1, 1),
			(1, 'tool/lockpick-set', 1, 1)`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := s.loadCharacterInventory(1)
	if err != nil {
		t.Fatal(err)
	}
	attacks, err := s.buildAttacks(1, inv, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if len(attacks) != 2 {
		t.Fatalf("got %d attack rows, want 2 (Paper Bombs, Flash Tag) — lockpick set must stay excluded: %+v", len(attacks), attacks)
	}
	bySlug := map[string]attackRow{}
	for _, a := range attacks {
		bySlug[a.Slug] = a
	}
	bombs, ok := bySlug["tool/paper-bombs"]
	if !ok {
		t.Fatal("Paper Bombs missing from attack rows")
	}
	if bombs.DamageDice != "5d4" || bombs.DamageType != "Fire" || bombs.DamageCount != 5 || bombs.DamageSides != 4 {
		t.Errorf("Paper Bombs damage = %q %q (%dd%d), want 5d4 Fire (5d4)",
			bombs.DamageDice, bombs.DamageType, bombs.DamageCount, bombs.DamageSides)
	}
	flash, ok := bySlug["tool/flash-tag"]
	if !ok {
		t.Fatal("Flash Tag missing from attack rows")
	}
	if flash.DamageSides != 0 {
		t.Errorf("Flash Tag DamageSides = %d, want 0 (no printed damage — its effect is a save, not a damage roll)", flash.DamageSides)
	}
}

// The whole reason modifiers are stored as parts: raising an ability score has
// to move every attack that uses it, with nothing re-entered by hand.
func TestComposedAttackTracksAbilityScore(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Brawler', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := charstore.AddCustomAttack(s.charDB, 1, charstore.CustomAttack{
		Kind: "weapon", Name: "Summon Bite",
		AttackAbility: "str", AttackProf: charsheet.ProfFull, AttackBonus: 1,
		DamageCount: 2, DamageSides: 6, DamageAbility: "str", DamageBonus: 0,
	}); err != nil {
		t.Fatal(err)
	}

	totals := func() (int, int) {
		t.Helper()
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
		if err != nil {
			t.Fatal(err)
		}
		list, err := charstore.ListCustomAttacks(s.charDB, 1, "weapon")
		if err != nil {
			t.Fatal(err)
		}
		composed := composeCustomAttacks(list, sheet, 0)
		if len(composed) != 1 {
			t.Fatalf("got %d custom attacks, want 1", len(composed))
		}
		return composed[0].AttackTotal, composed[0].DamageTotal
	}

	atk0, dmg0 := totals()
	if _, err := s.charDB.Exec(`UPDATE characters SET base_str = 18 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	atk1, dmg1 := totals()
	// Str 10 (+0) -> 18 (+4) is a four-point swing on both.
	if atk1-atk0 != 4 || dmg1-dmg0 != 4 {
		t.Errorf("after raising Str: attack %+d->%+d, damage %+d->%+d, want both +4",
			atk0, atk1, dmg0, dmg1)
	}
}

// A row written before modifiers were composable has no ability and no
// proficiency mode, and must still total exactly the number that was typed.
func TestComposedAttackKeepsLegacyFlatNumbers(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Veteran', 18, 18, 18, 18, 18, 18)`); err != nil {
		t.Fatal(err)
	}
	// Exactly what the pre-0009 columns hold: flat numbers, defaults elsewhere.
	if _, err := s.charDB.Exec(`
		INSERT INTO character_custom_attacks (character_id, kind, name, attack_bonus, damage_bonus)
		VALUES (1, 'weapon', 'Old Row', 7, 3)`); err != nil {
		t.Fatal(err)
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	list, err := charstore.ListCustomAttacks(s.charDB, 1, "weapon")
	if err != nil {
		t.Fatal(err)
	}
	composed := composeCustomAttacks(list, sheet, 0)
	if len(composed) != 1 {
		t.Fatalf("got %d custom attacks, want 1", len(composed))
	}
	if composed[0].AttackTotal != 7 || composed[0].DamageTotal != 3 {
		t.Errorf("legacy row totals = %+d/%+d, want the stored +7/+3 unchanged",
			composed[0].AttackTotal, composed[0].DamageTotal)
	}
}

func TestSheetInventoryEquipDrivesAC(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, ac_bonus, armor_ability_1, armor_max_mod)
		VALUES ('armor/flak-jacket', 'Flak Jacket', 'armor', 4, 'dex', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Armored', 10, 14, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	post := func(path string, form url.Values, handler http.HandlerFunc, rowID string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		if rowID != "" {
			req.SetPathValue("rowID", rowID)
		}
		w := httptest.NewRecorder()
		handler(w, req)
		// A fragment back (200), not the bare 204 these handlers used to
		// answer with before the screen-flash-bug fix gave inventory
		// add/update real fragments instead of a client-side reload.
		if w.Code != http.StatusOK {
			t.Fatalf("POST %s: status %d, body %s", path, w.Code, w.Body.String())
		}
	}

	// Unarmored to start: 10 + Dex 14's +2.
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.AC == nil || *sheet.AC != 12 {
		t.Fatalf("starting AC = %v, want 12", sheet.AC)
	}

	post("/characters/1/sheet/inventory",
		url.Values{"item_slug": {"armor/flak-jacket"}, "quantity": {"1"}},
		s.handleSheetInventoryAdd, "")

	var rowID int64
	if err := s.charDB.QueryRow(
		`SELECT id FROM character_inventory WHERE character_id = 1`).Scan(&rowID); err != nil {
		t.Fatal(err)
	}

	// Carried but not worn changes nothing.
	sheet, err = charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.AC == nil || *sheet.AC != 12 {
		t.Errorf("AC after adding unequipped armor = %v, want 12", sheet.AC)
	}

	post("/characters/1/sheet/inventory/"+strconv.FormatInt(rowID, 10)+"/update",
		url.Values{"quantity": {"1"}, "equipped": {"1"}},
		s.handleSheetInventoryUpdate, strconv.FormatInt(rowID, 10))

	sheet, err = charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.AC == nil || *sheet.AC != 16 {
		t.Errorf("AC after equipping = %v, want 16 (4 base + Dex +2 + half prof 3)", sheet.AC)
	}

	// Adding the same item again merges into the existing row rather than
	// making a second one.
	post("/characters/1/sheet/inventory",
		url.Values{"item_slug": {"armor/flak-jacket"}, "quantity": {"2"}},
		s.handleSheetInventoryAdd, "")
	var rows, quantity int
	if err := s.charDB.QueryRow(
		`SELECT COUNT(*), SUM(quantity) FROM character_inventory WHERE character_id = 1`,
	).Scan(&rows, &quantity); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || quantity != 3 {
		t.Errorf("after re-adding: %d rows totalling %d, want 1 row totalling 3", rows, quantity)
	}

	post("/characters/1/sheet/inventory/"+strconv.FormatInt(rowID, 10)+"/delete",
		nil, s.handleSheetInventoryDelete, strconv.FormatInt(rowID, 10))
	if err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_inventory WHERE character_id = 1`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d rows left after delete, want 0", rows)
	}
}

// TestToolkitChoiceCount pins the classification that decides whether a
// class's tool proficiency is a grant or an instruction. The inputs are the
// real strings in rules.db — the book writes these as prose and the ingest
// keeps them verbatim, so a regression here silently turns "pick two
// toolkits" back into a proficiency literally named "Pick two".
func TestToolkitChoiceCount(t *testing.T) {
	cases := []struct {
		value string
		want  int
	}{
		{"Select Any two Toolkits", 2},
		{"Pick four", 4},
		{"3 of your choice", 3},
		{"One Tool Kit of your choice", 1},
		// Real grants, which must stay grants.
		{"Disguise Kit", 0},
		{"Forensics Kit", 0},
		{"Weaponsmith Kit", 0},
		{"Armorsmith kit", 0},
		{"Medicine Kit", 0},
	}
	for _, c := range cases {
		if got := charstore.ToolkitChoiceCount(c.value); got != c.want {
			t.Errorf("ToolkitChoiceCount(%q) = %d, want %d", c.value, got, c.want)
		}
	}
}

// TestClassStepToolkitDropdowns walks the whole path that was previously
// missing: a class whose tool proficiency is an instruction renders that
// many real dropdowns, and submitting them stores actual toolkit names as
// proficiencies instead of the instruction text.
func TestClassStepToolkitDropdowns(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/scout-nin', 'Scout-nin', 8, 6)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_proficiencies (class_slug, kind, value) VALUES
		 ('class/scout-nin', 'tool', 'Select Any two Toolkits'),
		 ('class/scout-nin', 'armor', 'Light armor')`); err != nil {
		t.Fatal(err)
	}
	// INSERT OR IGNORE: the seed migrations may already carry these.
	if _, err := s.rulesDB.Exec(`
		INSERT OR IGNORE INTO equipment (slug, name, kind) VALUES
		 ('toolkit/disguise-kit', 'Disguise Kit', 'toolkit'),
		 ('toolkit/forgery-kit', 'Forgery Kit', 'toolkit')`); err != nil {
		t.Fatal(err)
	}

	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Scout')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	// GET renders one dropdown per pick, not the instruction as a label.
	req := httptest.NewRequest(http.MethodGet, "/characters/"+idStr+"/create/class?class=class/scout-nin", nil)
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleCreateClass(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, name := range []string{`name="toolkit_0"`, `name="toolkit_1"`} {
		if !strings.Contains(body, name) {
			t.Errorf("class step is missing a %s dropdown", name)
		}
	}
	if strings.Contains(body, `name="toolkit_2"`) {
		t.Error("class step rendered more toolkit dropdowns than the class asks for")
	}

	// Submitting with a slot left blank must not save a half-made choice.
	form := url.Values{"class_slug": {"class/scout-nin"}, "toolkit_0": {"Disguise Kit"}}
	req = httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w = httptest.NewRecorder()
	s.handleCreateClass(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("incomplete POST status = %d, want 200 re-render", w.Code)
	}
	var saved int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_classes WHERE character_id = ?`, id).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if saved != 0 {
		t.Errorf("incomplete toolkit picks saved the class anyway (%d rows)", saved)
	}

	// A complete submission stores the toolkits by name, and never the
	// instruction that produced the slots.
	form.Set("toolkit_1", "Forgery Kit")
	req = httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/create/class", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", idStr)
	w = httptest.NewRecorder()
	s.handleCreateClass(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("complete POST status = %d, body:\n%s", w.Code, w.Body.String())
	}

	rows, err := s.charDB.Query(`
		SELECT value FROM character_proficiencies WHERE character_id = ? AND kind = 'tool' ORDER BY rowid`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tools []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		tools = append(tools, v)
	}
	if len(tools) != 2 || tools[0] != "Disguise Kit" || tools[1] != "Forgery Kit" {
		t.Fatalf("stored tool proficiencies = %v, want [Disguise Kit Forgery Kit]", tools)
	}

	// And revisiting the step shows those picks selected again.
	req = httptest.NewRequest(http.MethodGet, "/characters/"+idStr+"/create/class?class=class/scout-nin", nil)
	req.SetPathValue("id", idStr)
	w = httptest.NewRecorder()
	s.handleCreateClass(w, req)
	if !strings.Contains(w.Body.String(), `value="Disguise Kit" data-slug="toolkit/disguise-kit" selected`) {
		t.Error("revisiting the class step did not re-select the saved toolkit")
	}
}

// TestDeleteCharacter checks that deleting takes the child rows with it.
// The cascade declared in the schema is NOT what does this — PRAGMA
// foreign_keys is per-connection and database/sql pools connections — so
// this is really a test that charstore.DeleteCharacter's own explicit
// deletes reach every table.
func TestDeleteCharacter(t *testing.T) {
	s := testServer(t)
	res, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Doomed')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	idStr := strconv.FormatInt(id, 10)

	if _, err := s.charDB.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, quantity) VALUES (?, 'weapon/kunai', 1)`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref)
		 VALUES (?, 'tool', 'Disguise Kit', 'class', 'class/scout-nin')`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_chat_log (character_id, kind, text) VALUES (?, 'message', 'hello')`, id,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/delete", nil)
	req.SetPathValue("id", idStr)
	w := httptest.NewRecorder()
	s.handleDeleteCharacter(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}

	var n int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM characters WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("characters still has %d rows for the deleted character", n)
	}

	// Every table that references a character must be empty afterwards.
	// Enumerated from the live schema rather than hard-coded, so a table
	// added later and forgotten in DeleteCharacter's list fails here
	// instead of silently leaking rows that a recycled id would inherit.
	tables, err := s.charDB.Query(`
		SELECT m.name FROM sqlite_master m
		WHERE m.type = 'table'
		  AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'character_id')`)
	if err != nil {
		t.Fatal(err)
	}
	defer tables.Close()
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		t.Fatal("found no character_id tables to check — the enumeration query is broken")
	}
	for _, table := range names {
		var left int
		if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE character_id = ?`, id).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left != 0 {
			t.Errorf("%s still has %d rows for the deleted character — missing from DeleteCharacter's list?", table, left)
		}
	}

	// Deleting again is a no-op, not a 500.
	req = httptest.NewRequest(http.MethodPost, "/characters/"+idStr+"/delete", nil)
	req.SetPathValue("id", idStr)
	w = httptest.NewRecorder()
	s.handleDeleteCharacter(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("second delete status = %d, want a plain redirect", w.Code)
	}
}

// TestProficiencyBulletStaysRound guards a CSS specificity trap that cost
// three rounds of "still squares" bug reports.
//
// The bullets are <button>s inside .sheet-inline-form. A descendant
// selector like `.sheet-inline-form button` scores (0,1,1) and outranks
// `.sheet-prof-bullet` at (0,1,0), so the generic form-button styling —
// border-radius: 6px, plus padding — silently won over the circle. The
// circle CSS was correct the entire time and looked broken anyway.
//
// This is a text assertion, not a rendering one; nothing here can lay out a
// page. It checks the one structural property that made the bug possible:
// no generic button selector inside these forms may match the bullet.
func TestProficiencyBulletStaysRound(t *testing.T) {
	cssBytes, err := staticFS.ReadFile("static/css/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)

	if !strings.Contains(css, "border-radius: 50%") {
		t.Error("app.css no longer makes .sheet-prof-bullet a circle at all")
	}
	for _, form := range []string{".sheet-inline-form", ".sheet-panel-form"} {
		if strings.Contains(css, form+" button,") || strings.Contains(css, form+" button {") {
			t.Errorf("%s has an unqualified `button` rule again — it outranks "+
				".sheet-prof-bullet and will square off the proficiency bullets. "+
				"Add :not(.sheet-prof-bullet).", form)
		}
	}
}

func TestJutsuAttackKind(t *testing.T) {
	cases := []struct {
		description string
		want        string
	}{
		{"Make a Ranged Ninjutsu Attack against a creature within range.", "Ninjutsu"},
		{"Make one Melee Taijutsu Attack. Compare the result against all creatures.", "Taijutsu"},
		{"You make two Genjutsu Attacks against the same target.", "Genjutsu"},
		{"Make a Melee Ninjutsu attack against a creature.", "Ninjutsu"}, // lowercase "attack" as printed
		// A buff jutsu that modifies attacks you make LATER is not itself an
		// attack; a bare "taijutsu attack" match would give it a to-hit
		// button that rolls nothing real.
		{"Weapon and Taijutsu attacks made with this weapon deal an additional 2d4 Poison Damage.", ""},
		{"Their Unarmed, Weapon, and Taijutsu Attacks deal an additional 1d6 Earth Damage.", ""},
		// The overwhelmingly common case: resolved with a saving throw.
		{"Each creature in the area must make a Strength Saving Throw.", ""},
		// Bukijutsu is one of charsheet.AttackKinds, so it does get a button.
		// Hijutsu still has no rule and gets none.
		{"Make a Melee Bukijutsu Attack against the target.", "Bukijutsu"},
		{"Make a Ranged Hijutsu Attack against the target.", ""},
	}
	for _, c := range cases {
		if got := jutsuAttackKind(c.description); got != c.want {
			t.Errorf("jutsuAttackKind(%q) = %q, want %q", c.description, got, c.want)
		}
	}
}

// TestBuildUpcastOptions pins the rank-walk cost math: every rank from a
// jutsu's own base rank through S, cost scaling linearly by the parsed
// per-rank delta, with NO cap at any particular rank (see buildUpcastOptions'
// own comment — upcasting is confirmed to not be bounded by Highest Rank
// Known the way learning is).
func TestBuildUpcastOptions(t *testing.T) {
	got := buildUpcastOptions("D", 3, 3)
	want := []jutsuUpcastOption{
		{Rank: "D", Cost: 3},
		{Rank: "C", Cost: 6},
		{Rank: "B", Cost: 9},
		{Rank: "A", Cost: 12},
		{Rank: "S", Cost: 15},
	}
	if len(got) != len(want) {
		t.Fatalf("buildUpcastOptions(D, 3, 3) = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d: got %+v, want %+v", i, got[i], want[i])
		}
	}

	// A jutsu already at S-Rank has nowhere to upcast to — one entry, itself.
	got = buildUpcastOptions("S", 20, 3)
	if len(got) != 1 || got[0] != (jutsuUpcastOption{Rank: "S", Cost: 20}) {
		t.Errorf("buildUpcastOptions(S, 20, 3) = %+v, want a single S-Rank entry at cost 20", got)
	}

	// An unrecognized base rank (shouldn't happen — jutsu.rank is always one
	// of E/D/C/B/A/S — but a nil/empty result is the safe fallback rather
	// than a panic) yields no options at all.
	if got := buildUpcastOptions("", 3, 3); got != nil {
		t.Errorf("buildUpcastOptions('', 3, 3) = %+v, want nil", got)
	}
}

// TestComputeInventoryBulk pins the things about the bulk calculation that
// are not obvious from the formula: bulk is per unit and multiplies by the
// stack size, the four Shinobi storage tools' equipment.bulk value is a
// capacity they GRANT, not space they take, and (part 10) that capacity only
// applies while the storage tool is actually equipped — previously,
// equipping/unequipping a Shinobi Waist Bag made no difference at all,
// because the old code granted the bonus unconditionally.
func TestComputeInventoryBulk(t *testing.T) {
	bulk := func(v float64) sql.NullFloat64 { return sql.NullFloat64{Float64: v, Valid: true} }

	inv := []inventoryRow{
		{Slug: "weapon/katana", Bulk: bulk(2), Quantity: 1},
		{Slug: "weapon/kunai", Bulk: bulk(2), Quantity: 3},                            // stacks: 6, not 2
		{Slug: "gear/shinobi-backpack", Bulk: bulk(10), Quantity: 1, Equipped: true},  // +10 capacity, 0 carried
		{Slug: "gear/shinobi-waist-bag", Bulk: bulk(5), Quantity: 1, Equipped: false}, // NOT equipped: no bonus at all
		{Slug: "weapon/net", Quantity: 1},                                             // no bulk in the rules
		{Name: "Grandpa's letter", Quantity: 1},                                       // free-text row, no slug
	}
	sheet := &charsheet.Sheet{Abilities: map[string]charsheet.AbilityScore{"str": {Score: 14, Modifier: 2}}}

	got := computeInventoryBulk(inv, sheet, 0)

	if got.Carried != 8 {
		t.Errorf("Carried = %v, want 8 (katana 2 + three kunai at 2)", got.Carried)
	}
	if got.Base != 14 {
		t.Errorf("Base = %v, want 14 (10 + 2 per point of a +2 Strength modifier)", got.Base)
	}
	if got.Storage != 10 {
		t.Errorf("Storage = %v, want 10 (equipped Shinobi Backpack only — the unequipped waist bag contributes nothing)", got.Storage)
	}
	if got.Capacity != 24 {
		t.Errorf("Capacity = %v, want 24", got.Capacity)
	}
	if got.Unknown != 2 {
		t.Errorf("Unknown = %d, want 2 (the net and the free-text row)", got.Unknown)
	}
	if got.Encumbered {
		t.Error("Encumbered = true, want false — 8 carried against 24 capacity")
	}
	if inv[1].BulkTotal != 6 {
		t.Errorf("kunai row BulkTotal = %v, want 6", inv[1].BulkTotal)
	}
	if inv[2].BulkTotal != 0 || inv[2].StorageBonus != 10 {
		t.Errorf("backpack row = {BulkTotal %v, StorageBonus %v}, want {0, 10}", inv[2].BulkTotal, inv[2].StorageBonus)
	}
	if inv[3].BulkTotal != 0 || inv[3].StorageBonus != 0 {
		t.Errorf("unequipped waist bag row = {BulkTotal %v, StorageBonus %v}, want {0, 0}", inv[3].BulkTotal, inv[3].StorageBonus)
	}

	// Over capacity: a character with no Strength bonus and no storage tool
	// carrying 11 bulk is Encumbered against the flat base of 10.
	over := []inventoryRow{{Slug: "weapon/great-axe", Bulk: bulk(3), Quantity: 4}}
	if got := computeInventoryBulk(over, &charsheet.Sheet{Abilities: map[string]charsheet.AbilityScore{"str": {Score: 10}}}, 0); !got.Encumbered {
		t.Errorf("Encumbered = false for %v carried against %v capacity, want true", got.Carried, got.Capacity)
	}

	// Always Prepared (Puppet Master, L15): the caller-supplied
	// featureBulkBonus folds into Base/Capacity the same way the Strength
	// bonus does.
	withFeature := computeInventoryBulk(nil, &charsheet.Sheet{Abilities: map[string]charsheet.AbilityScore{"str": {Score: 10}}}, 10)
	if withFeature.FeatureBonus != 10 || withFeature.Base != 20 || withFeature.Capacity != 20 {
		t.Errorf("with featureBulkBonus=10: FeatureBonus=%v Base=%v Capacity=%v, want 10/20/20",
			withFeature.FeatureBonus, withFeature.Base, withFeature.Capacity)
	}
}

// TestLoadGrantedJutsuLabels covers a class/clan feature that grants a
// specific jutsu for free ("Beginning at 1st level, as a Puppet Master you
// learn the Chakra Hands E-Rank Ninjutsu for free") never actually reaching
// the character's known jutsu — the feature text was never parsed for a
// jutsu grant at all, so a fresh Puppet Master had neither Chakra Hands nor
// Mending on their sheet despite both features saying so.
//
// Also pins the case a naive "grant everything the feature's own level
// unlocks" implementation would get wrong: Hoshigaki's Commander of the
// Deep is itself a 1st-level feature, but its Summoning Technique grant
// explicitly starts at 7th level — a jutsu grant's own stated level must
// win over the feature's row-level fallback.
func TestLoadGrantedJutsuLabels(t *testing.T) {
	s := testServer(t)

	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 6)`)
	mustExecRules(`INSERT INTO class_features (slug, class_slug, name, level, description, sort_order) VALUES
		('class/puppet-master/feature/chakra-threads', 'class/puppet-master', 'Chakra Threads', 1,
		 'Beginning at 1st level, as a Puppet Master you learn the Chakra Hands E-Rank Ninjutsu for free. When you cast this jutsu, it takes the form of strings.', 1)`)
	mustExecRules(`INSERT INTO class_features (slug, class_slug, name, level, description, sort_order) VALUES
		('class/puppet-master/feature/puppet-tool', 'class/puppet-master', 'Puppet Tool', 1,
		 'Beginning at 1st level, you craft a Puppet Tool to carry out your orders. You learn the Mending E-Rank Ninjutsu, which does not count against your known.', 2)`)
	mustExecRules(`INSERT INTO clans (slug, name) VALUES ('clan/hoshigaki', 'Hoshigaki Clan')`)
	mustExecRules(`INSERT INTO clan_features (slug, clan_slug, name, level, description, sort_order) VALUES
		('clan/hoshigaki/feature/commander-of-the-deep', 'clan/hoshigaki', 'Commander of the Deep', 1,
		 'Beginning at 1st level, aquatic creatures have an affinity with people of your clan. Starting at 7th level you learn the Summoning Technique D-Rank ninjutsu with a contract with the Shark tribe.', 1)`)
	for _, j := range [][2]string{
		{"jutsu/chakra-hands", "Chakra Hands"},
		{"jutsu/mending", "Mending"},
		{"jutsu/summoning-technique", "Summoning Technique"},
	} {
		mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
			VALUES (?, ?, 'Ninjutsu', 'E', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 'Ninjutsu', 'test jutsu')`,
			j[0], j[1])
	}

	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha, clan_slug)
		VALUES ('Kankuro', 10, 10, 10, 10, 10, 10, 'clan/hoshigaki')`)
	if err != nil {
		t.Fatal(err)
	}
	characterID, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/puppet-master', 7, 0)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}

	// At level 1: both level-1 grants present, Summoning Technique not yet.
	labels, err := s.loadGrantedJutsuLabels(characterID, &charsheet.Sheet{ClanSlug: "clan/hoshigaki", Level: 1})
	if err != nil {
		t.Fatalf("loadGrantedJutsuLabels at level 1: %v", err)
	}
	want := map[string]string{
		"jutsu/chakra-hands": "Class Feature",
		"jutsu/mending":      "Class Feature",
	}
	if len(labels) != len(want) {
		t.Fatalf("labels at level 1 = %v, want %v", labels, want)
	}
	for slug, label := range want {
		if labels[slug] != label {
			t.Errorf("labels[%q] at level 1 = %q, want %q", slug, labels[slug], label)
		}
	}

	// At level 7: Commander of the Deep's own jutsu grant (stated at 7th
	// level in the text) also lands, even though the feature row itself is
	// gained at level 1.
	labels, err = s.loadGrantedJutsuLabels(characterID, &charsheet.Sheet{ClanSlug: "clan/hoshigaki", Level: 7})
	if err != nil {
		t.Fatalf("loadGrantedJutsuLabels at level 7: %v", err)
	}
	if labels["jutsu/summoning-technique"] != "Clan" {
		t.Errorf("labels[jutsu/summoning-technique] at level 7 = %q, want %q", labels["jutsu/summoning-technique"], "Clan")
	}
	if len(labels) != 3 {
		t.Errorf("labels at level 7 = %v, want 3 entries", labels)
	}

	// The granted jutsu show up on the sheet itself and don't count against
	// JutsuKnownCap.
	sheet := &charsheet.Sheet{ClanSlug: "clan/hoshigaki", Level: 7}
	rows, err := s.loadCharacterJutsuSheet(characterID, sheet)
	if err != nil {
		t.Fatalf("loadCharacterJutsuSheet: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("loadCharacterJutsuSheet returned %d rows, want 3", len(rows))
	}
	if got := jutsuKnownCount(rows); got != 0 {
		t.Errorf("jutsuKnownCount = %d, want 0 (all three rows are free grants)", got)
	}
}

// TestControlledChakraFlowGrantedJutsu covers Puppet Master's Controlled
// Chakra Flow (Green Technique Marionettist, 6th level) — a choice between
// 2 named jutsu (features.ChoiceNamedJutsuGrant), unlike every other grant
// loadGrantedJutsuLabels resolves by parsing a fixed sentence. Confirms
// neither jutsu shows up until the player actually picks one, and that the
// resolved pick lands with the same "Subclass Feature" label an
// auto-parsed subclass grant would get.
func TestControlledChakraFlowGrantedJutsu(t *testing.T) {
	s := testServer(t)

	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 6)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/puppet-master/group/puppet-techniques', 'class/puppet-master', 'Puppet Techniques')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/puppet-master/group/puppet-techniques/green-technique-marionettist', 'class/puppet-master/group/puppet-techniques', 'Green Technique Marionettist')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/controlled-chakra-flow',
		 'class/puppet-master/group/puppet-techniques/green-technique-marionettist', 'Controlled Chakra Flow', 6,
		 'Also at 6th level, you learn one of the following E-Rank jutsu. Depending on your choice, you gain an extra effect; Firecracker Flash (Ninjutsu)... Feather Burst (Genjutsu)...', 1)`)
	for _, j := range [][2]string{
		{"jutsu/firecracker-flash", "Firecracker Flash"},
		{"jutsu/feather-burst", "Feather Burst"},
	} {
		mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
			VALUES (?, ?, 'Ninjutsu', 'E', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 'Ninjutsu', 'test jutsu')`,
			j[0], j[1])
	}

	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Sasori', 10, 10, 10, 10, 10, 10)`)
	if err != nil {
		t.Fatal(err)
	}
	characterID, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/puppet-master', 6, 0)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/puppet-master/group/puppet-techniques/green-technique-marionettist', 2)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}

	sheet := &charsheet.Sheet{Level: 6}
	labels, err := s.loadGrantedJutsuLabels(characterID, sheet)
	if err != nil {
		t.Fatalf("loadGrantedJutsuLabels before a pick: %v", err)
	}
	if _, ok := labels["jutsu/firecracker-flash"]; ok {
		t.Errorf("labels before a pick = %v, want neither named jutsu present yet", labels)
	}
	if _, ok := labels["jutsu/feather-burst"]; ok {
		t.Errorf("labels before a pick = %v, want neither named jutsu present yet", labels)
	}

	if err := charstore.SetFeatureChoice(s.charDB, characterID,
		"class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/controlled-chakra-flow",
		0, "jutsu/feather-burst"); err != nil {
		t.Fatalf("SetFeatureChoice: %v", err)
	}

	labels, err = s.loadGrantedJutsuLabels(characterID, sheet)
	if err != nil {
		t.Fatalf("loadGrantedJutsuLabels after a pick: %v", err)
	}
	if labels["jutsu/feather-burst"] != "Subclass Feature" {
		t.Errorf("labels[jutsu/feather-burst] = %q, want %q", labels["jutsu/feather-burst"], "Subclass Feature")
	}
	if _, ok := labels["jutsu/firecracker-flash"]; ok {
		t.Errorf("labels = %v, want the UNPICKED option absent", labels)
	}
}

// TestLoadGrantedFeaturesIncludesSubclass covers subclass_features never
// being queried at all — loadGrantedFeatures only ever looked at
// class_features and clan_features, so a chosen subclass's own features
// (and anything they grant, like a free jutsu or a passive resistance)
// never reached the sheet regardless of level.
//
// Also pins that a subclass feature is gated by its PARENT CLASS's level,
// not the character's clan level or some subclass-only counter —
// character_subclasses only stores the subclass slug, so the parent class
// has to be resolved through subclass_groups.class_slug.
func TestLoadGrantedFeaturesIncludesSubclass(t *testing.T) {
	s := testServer(t)

	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 6)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/puppet-master/group/puppet-techniques', 'class/puppet-master', 'Puppet Techniques')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 'class/puppet-master/group/puppet-techniques', 'Purple Technique Juggernaut')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/enhanced-vision',
		 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 'Enhanced Vision', 6,
		 'Starting at 6th level, you have fit your armor with a special chakra visor that grants you 60 feet of Darkvision. If you already have Darkvision, it is increased by 60 feet instead.', 1)`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/puppet-master/group/puppet-techniques/purple-technique-juggernaut/feature/intelligent-design',
		 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 'Intelligent Design', 10,
		 'Starting at 10th level, you become immune to airborne toxins and poisons while in your armor.', 2)`)
	mustExecRules(`INSERT INTO clans (slug, name) VALUES ('clan/vesper', 'Vesper Clan')`)
	mustExecRules(`INSERT INTO clan_features (slug, clan_slug, name, level, description, sort_order) VALUES
		('clan/vesper/feature/supreme-nightvision', 'clan/vesper', 'Supreme Nightvision', 1,
		 'Also at 1st level, Vesper Clan members gain 60 feet of Darkvision.', 1)`)

	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha, clan_slug)
		VALUES ('Kuroari', 10, 10, 10, 10, 10, 10, 'clan/vesper')`)
	if err != nil {
		t.Fatal(err)
	}
	characterID, _ := res.LastInsertId()
	// The class's real level starts at 5 — below both subclass features'
	// own gate levels (6 and 10) — and is raised to 10 further down.
	// loadGrantedFeatures resolves a subclass feature's gate from the
	// PARENT CLASS's actual character_classes.levels row, not from the
	// classLevel parameter (that parameter only gates the clan half), so
	// the level has to move in the database, not just in a call argument.
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/puppet-master', 5, 0)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/puppet-master/group/puppet-techniques/purple-technique-juggernaut', 3)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}

	// Below the subclass's own feature levels: only the clan feature shows.
	features, err := s.loadGrantedFeatures(characterID, "clan/vesper", 5)
	if err != nil {
		t.Fatalf("loadGrantedFeatures at level 5: %v", err)
	}
	if len(features) != 1 || features[0].Name != "Supreme Nightvision" {
		t.Fatalf("loadGrantedFeatures at level 5 = %+v, want just Supreme Nightvision", features)
	}

	// Below level 10, the subclass features table wasn't even in the
	// features list (loadGrantedFeatures gates that), so only the clan's
	// base 60ft Darkvision applies — no stacking, no immunity.
	belowTraits := computePassiveTraits(features, 5)
	if len(belowTraits.Senses) != 1 || belowTraits.Senses[0].Feet != 60 {
		t.Errorf("Senses below level 6 = %+v, want just 60ft Darkvision", belowTraits.Senses)
	}
	if len(belowTraits.Immunities) != 0 {
		t.Errorf("Immunities below level 10 = %+v, want none", belowTraits.Immunities)
	}

	// Raise the class to level 10: both subclass features show, gated by
	// the parent class's level (resolved through subclass_groups), with the
	// "Subclass: <name>" label and the slug loadGrantedJutsuLabels/
	// computePassiveTraits key off of.
	if _, err := s.charDB.Exec(
		`UPDATE character_classes SET levels = 10 WHERE character_id = ?`, characterID,
	); err != nil {
		t.Fatal(err)
	}
	features, err = s.loadGrantedFeatures(characterID, "clan/vesper", 10)
	if err != nil {
		t.Fatalf("loadGrantedFeatures at level 10: %v", err)
	}
	if len(features) != 3 {
		t.Fatalf("loadGrantedFeatures at level 10 = %d rows, want 3: %+v", len(features), features)
	}
	var sawSubclass int
	for _, f := range features {
		if f.SourceLabel == "Subclass: Purple Technique Juggernaut/6th Level" || f.SourceLabel == "Subclass: Purple Technique Juggernaut/10th Level" {
			sawSubclass++
			if f.Slug == "" {
				t.Errorf("feature %q has no Slug — passive-trait lookup keys off this", f.Name)
			}
		}
	}
	if sawSubclass != 2 {
		t.Errorf("saw %d subclass-labeled features, want 2: %+v", sawSubclass, features)
	}

	// The subclass's Darkvision stacks with the clan's (Enhanced Vision:
	// "if you already have Darkvision, it is increased by 60 feet instead"),
	// and Intelligent Design's immunity is present once level 10 is reached.
	traits := computePassiveTraits(features, 10)
	var darkvisionFeet int
	for _, sense := range traits.Senses {
		if sense.Sense == "Darkvision" {
			darkvisionFeet = sense.Feet
		}
	}
	if darkvisionFeet != 120 {
		t.Errorf("Darkvision = %d ft, want 120 (60 clan + 60 stacking subclass)", darkvisionFeet)
	}
	foundPoisonImmunity := false
	for _, imm := range traits.Immunities {
		if imm.Target == "Poison" {
			foundPoisonImmunity = true
		}
	}
	if !foundPoisonImmunity {
		t.Errorf("Immunities = %+v, want Poison damage immunity from Intelligent Design", traits.Immunities)
	}
}

// TestComputePassiveTraitsEscalation pins Heavenly Flame's "Resistance to
// Fire damage, or Immunity if you already have Resistance" wording: the
// escalation has to check every OTHER source before deciding, never itself,
// and a character with no other Fire resistance source just gets Resistance.
func TestComputePassiveTraitsEscalation(t *testing.T) {
	heavenlyFlame := grantedFeatureRow{
		Slug: "class/cooking-nin/group/cooking-focus/heat-master/feature/heavenly-flame",
		Name: "Heavenly Flame",
	}
	ashenResilience := grantedFeatureRow{
		Slug: "clan/iburi/feature/ashen-resilience",
		Name: "Ashen Resilience",
	}

	// Heavenly Flame alone: no other Fire resistance source, so it grants
	// plain Resistance.
	traits := computePassiveTraits([]grantedFeatureRow{heavenlyFlame}, 17)
	if len(traits.Immunities) != 0 {
		t.Errorf("Immunities = %+v, want none (nothing else grants Fire resistance)", traits.Immunities)
	}
	if len(traits.Resistances) != 1 || traits.Resistances[0].Target != "Fire" {
		t.Errorf("Resistances = %+v, want just Fire", traits.Resistances)
	}

	// Alongside Ashen Resilience (a plain Fire resistance grant, gated at
	// its own MinLevel 7), Heavenly Flame escalates to Immunity instead.
	// Level 9 rather than 17 on purpose: Ashen Resilience's OWN text also
	// grants Burned condition immunity, but only at its MinLevel 11 — level
	// 9 isolates the Fire-damage escalation this test is actually about
	// from that unrelated second grant on the same feature.
	traits = computePassiveTraits([]grantedFeatureRow{heavenlyFlame, ashenResilience}, 9)
	if len(traits.Resistances) != 0 {
		t.Errorf("Resistances = %+v, want none (escalated to Immunity)", traits.Resistances)
	}
	if len(traits.Immunities) != 1 || traits.Immunities[0].Target != "Fire" {
		t.Errorf("Immunities = %+v, want just Fire", traits.Immunities)
	}

	// Below Ashen Resilience's own MinLevel 7 gate, its Fire resistance
	// isn't active, so Heavenly Flame has nothing to escalate against.
	traits = computePassiveTraits([]grantedFeatureRow{heavenlyFlame, ashenResilience}, 6)
	if len(traits.Immunities) != 0 {
		t.Errorf("Immunities at level 6 = %+v, want none (Ashen Resilience's Fire resistance needs level 7)", traits.Immunities)
	}
	if len(traits.Resistances) != 1 || traits.Resistances[0].Target != "Fire" {
		t.Errorf("Resistances at level 6 = %+v, want just Fire (plain Resistance)", traits.Resistances)
	}
}

// TestWaterAndOilBonusJutsuSlots covers Fry Cook's "Water and Oil, Do Mix":
// "If you already have access to Water or Medical release Jutsu, you
// instead learn a combined number of Jutsu of these releases equal to half
// your Proficiency Bonus." The bonus only applies when the character
// already has that release access from another source (here, their clan's
// jutsu list) — a character with neither gets nothing extra.
func TestWaterAndOilBonusJutsuSlots(t *testing.T) {
	s := testServer(t)

	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/cooking-nin', 'Cooking-Nin', 8, 6)`)
	mustExecRules(`INSERT INTO clans (slug, name) VALUES ('clan/hanami', 'Hanami Clan')`)
	mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/mystical-palm', 'Mystical Palm', 'Ninjutsu', 'E', '1 Action', 'Touch', 'Instant', 'HS', 'Cost: 1 Chakra', 'Ninjutsu, Medical', 'test jutsu')`)
	mustExecRules(`INSERT INTO jutsu_keywords (jutsu_slug, keyword) VALUES ('jutsu/mystical-palm', 'Medical')`)
	mustExecRules(`INSERT INTO clan_jutsu (clan_slug, jutsu_slug) VALUES ('clan/hanami', 'jutsu/mystical-palm')`)

	features := []grantedFeatureRow{{Slug: waterAndOilDoMixSlug, Name: "Water and Oil, Do Mix"}}

	bonus, err := s.waterAndOilBonusJutsuSlots(1, features, "class/cooking-nin", "clan/hanami", 5)
	if err != nil {
		t.Fatalf("waterAndOilBonusJutsuSlots (has access): %v", err)
	}
	if bonus != 2 {
		t.Errorf("bonus = %d, want 2 (half of proficiency bonus 5, rounded down)", bonus)
	}

	bonus, err = s.waterAndOilBonusJutsuSlots(1, features, "class/cooking-nin", "", 5)
	if err != nil {
		t.Fatalf("waterAndOilBonusJutsuSlots (no clan): %v", err)
	}
	if bonus != 0 {
		t.Errorf("bonus = %d, want 0 (no Water or Medical release access from any source)", bonus)
	}

	noFeature, err := s.waterAndOilBonusJutsuSlots(1, nil, "class/cooking-nin", "clan/hanami", 5)
	if err != nil {
		t.Fatalf("waterAndOilBonusJutsuSlots (no feature): %v", err)
	}
	if noFeature != 0 {
		t.Errorf("bonus = %d, want 0 (character doesn't have the feature)", noFeature)
	}

	// Regression: a Ninjutsu-casting class's own broad discipline list
	// includes plenty of jutsu tagged an elemental release keyword — that
	// must NOT by itself count as "already has access" for an ELEMENTAL
	// keyword (unlike "Medical" above, which has no affinity system and so
	// stays gated on the broad class/clan check). Seeding class_casting so
	// class/cooking-nin casts Ninjutsu, plus a Ninjutsu-classified,
	// Fire-Release-tagged jutsu reachable by that discipline, reproduces
	// the exact shape confirmed live against dist/rules.db where Heat
	// Master's own bonus was always paying out — this must stay 0 without
	// a real elemental affinity (clan trait, Nature Release feat, or
	// Professor slot) granting Fire from somewhere else.
	mustExecRules(`INSERT INTO class_casting (class_slug, discipline, ability) VALUES ('class/cooking-nin', 'ninjutsu', 'int')`)
	mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/fireball', 'Fireball', 'Ninjutsu', 'E', '1 Action', '30 ft', 'Instant', 'HS', 'Cost: 1 Chakra', 'Ninjutsu, Fire Release', 'test jutsu')`)
	mustExecRules(`INSERT INTO jutsu_keywords (jutsu_slug, keyword) VALUES ('jutsu/fireball', 'Fire Release')`)

	heatMasterFeatures := []grantedFeatureRow{{Slug: heatMasterFireAccessSlug, Name: "If You Can't Handle the Heat"}}
	fireBonus, err := s.heatMasterBonusJutsuSlots(1, heatMasterFeatures, "class/cooking-nin", "", 5)
	if err != nil {
		t.Fatalf("heatMasterBonusJutsuSlots (no elemental affinity): %v", err)
	}
	if fireBonus != 0 {
		t.Errorf("bonus = %d, want 0 (class discipline alone isn't real Fire release access, only an elemental affinity is)", fireBonus)
	}

	// A real Fire affinity (an Uchiha clan trait) DOES trigger the bonus.
	mustExecRules(`INSERT INTO clans (slug, name) VALUES ('clan/uchiha', 'Uchiha Clan')`)
	fireBonus, err = s.heatMasterBonusJutsuSlots(1, heatMasterFeatures, "class/cooking-nin", "clan/uchiha", 5)
	if err != nil {
		t.Fatalf("heatMasterBonusJutsuSlots (Uchiha Fire affinity): %v", err)
	}
	if fireBonus != 2 {
		t.Errorf("bonus = %d, want 2 (half of proficiency bonus 5, rounded down, via a real clan Fire affinity)", fireBonus)
	}
}

// TestHandleSheetLevel covers the part-6 level progression bug: setting a
// level has to move the real class level, the hit dice pool, and the HP and
// chakra maxima — the old handler wrote a display-only override that moved
// none of them.
func TestHandleSheetLevel(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/genin', 'Genin', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Climbing Genin', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/genin', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_level_gains (character_id, class_slug, class_level, hp_gain, chakra_gain, method)
		VALUES (1, 'class/genin', 1, 10, 8, 'fixed')`); err != nil {
		t.Fatal(err)
	}

	before, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	post := func(level string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/level",
			strings.NewReader(url.Values{"level": {level}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetLevel(w, req)
		return w.Code
	}

	if code := post("7"); code != http.StatusSeeOther {
		t.Fatalf("POST level=7: status %d, want %d", code, http.StatusSeeOther)
	}

	var levels int
	if err := s.charDB.QueryRow(
		`SELECT levels FROM character_classes WHERE character_id = 1`).Scan(&levels); err != nil {
		t.Fatal(err)
	}
	if levels != 7 {
		t.Errorf("stored class level = %d, want 7 (the level must be real, not an override)", levels)
	}

	after, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if after.Level != 7 {
		t.Errorf("Level = %d, want 7", after.Level)
	}
	// The hit dice pool is level-many dice; this is the reported bug.
	if after.Level-after.HitDiceSpent != 7 {
		t.Errorf("hit dice available = %d, want 7", after.Level-after.HitDiceSpent)
	}
	if after.MaxHP <= before.MaxHP {
		t.Errorf("MaxHP did not grow on level up: %d -> %d", before.MaxHP, after.MaxHP)
	}
	if after.MaxChakra <= before.MaxChakra {
		t.Errorf("MaxChakra did not grow on level up: %d -> %d", before.MaxChakra, after.MaxChakra)
	}
	if after.ProficiencyBonus < before.ProficiencyBonus {
		t.Errorf("ProficiencyBonus went backwards: %d -> %d", before.ProficiencyBonus, after.ProficiencyBonus)
	}

	// Levelling back down must not leave gain rows above the new level
	// behind, or a second level-up would count them twice.
	if _, err := s.charDB.Exec(`
		INSERT INTO character_level_gains (character_id, class_slug, class_level, hp_gain, chakra_gain, method)
		VALUES (1, 'class/genin', 6, 9, 7, 'rolled')`); err != nil {
		t.Fatal(err)
	}
	if code := post("3"); code != http.StatusSeeOther {
		t.Fatalf("POST level=3: status %d", code)
	}
	var above int
	if err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_level_gains WHERE character_id = 1 AND class_level > 3`).Scan(&above); err != nil {
		t.Fatal(err)
	}
	if above != 0 {
		t.Errorf("%d gain rows left above level 3", above)
	}

	if code := post("0"); code != http.StatusBadRequest {
		t.Errorf("POST level=0: status %d, want 400", code)
	}
	if code := post("21"); code != http.StatusBadRequest {
		t.Errorf("POST level=21: status %d, want 400", code)
	}
	if code := post(""); code != http.StatusBadRequest {
		t.Errorf("POST level=\"\": status %d, want 400", code)
	}
}

// TestHandleSheetLevelWithoutClass pins the one case SetLevel refuses:
// level is a class level, so there is nothing to raise before a class is
// picked, and the player gets told rather than getting a 500.
func TestHandleSheetLevelWithoutClass(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Draft', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/level",
		strings.NewReader(url.Values{"level": {"5"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetLevel(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleSheetMaxima(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/genin', 'Genin', 10, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Roller', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/genin', 3, 0)`); err != nil {
		t.Fatal(err)
	}

	post := func(values url.Values) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/maxima", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetMaxima(w, req)
		return w.Code
	}

	if code := post(url.Values{"maxhp": {"42"}, "maxchakra": {"31"}}); code != http.StatusOK {
		t.Fatalf("pinning maxima: status %d", code)
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.MaxHP != 42 || !sheet.MaxHPPinned || sheet.MaxChakra != 31 || !sheet.MaxChakraPinned {
		t.Errorf("maxima = %d/%d (pinned %v/%v), want 42/31 both pinned",
			sheet.MaxHP, sheet.MaxChakra, sheet.MaxHPPinned, sheet.MaxChakraPinned)
	}

	// Clearing a field hands the number back to the automatic calculation.
	if code := post(url.Values{"maxhp": {""}, "maxchakra": {"31"}}); code != http.StatusOK {
		t.Fatalf("clearing maxhp: status %d", code)
	}
	sheet, err = charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.MaxHPPinned || sheet.MaxHP != sheet.MaxHPAuto {
		t.Errorf("MaxHP = %d (pinned %v), want the computed %d", sheet.MaxHP, sheet.MaxHPPinned, sheet.MaxHPAuto)
	}

	if code := post(url.Values{"maxhp": {"-3"}}); code != http.StatusBadRequest {
		t.Errorf("negative maxhp: status %d, want 400", code)
	}
}

// The initiative picker's endpoint: three parts, one post, and the composed
// total has to move as a result.
func TestHandleSheetInitiative(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Scout', 10, 14, 10, 10, 18, 10)`); err != nil {
		t.Fatal(err)
	}

	post := func(v url.Values) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/initiative", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetInitiative(w, req)
		return w.Code
	}

	before, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if code := post(url.Values{"ability": {"wis"}, "prof": {"full"}, "bonus": {"2"}}); code != http.StatusSeeOther {
		t.Fatalf("set initiative: status %d", code)
	}
	after, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := after.Abilities["wis"].Modifier + after.ProficiencyBonus + 2
	if after.Initiative != want {
		t.Errorf("Initiative = %+d, want %+d", after.Initiative, want)
	}
	if after.Initiative == before.Initiative {
		t.Errorf("Initiative unchanged at %+d after an override", after.Initiative)
	}

	// Clearing every field goes back to the Dex + half proficiency default.
	if code := post(url.Values{"ability": {""}, "prof": {""}, "bonus": {""}}); code != http.StatusSeeOther {
		t.Fatalf("clear initiative: status %d", code)
	}
	cleared, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Initiative != before.Initiative {
		t.Errorf("Initiative = %+d after clearing, want the default %+d", cleared.Initiative, before.Initiative)
	}

	if code := post(url.Values{"ability": {"luck"}}); code != http.StatusBadRequest {
		t.Errorf("unknown ability: status %d, want 400", code)
	}
	if code := post(url.Values{"prof": {"double"}}); code != http.StatusBadRequest {
		t.Errorf("unknown proficiency mode: status %d, want 400", code)
	}
	if code := post(url.Values{"bonus": {"lots"}}); code != http.StatusBadRequest {
		t.Errorf("non-numeric bonus: status %d, want 400", code)
	}
}

func TestHandleSheetAttackAbility(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Illusionist', 10, 18, 10, 10, 10, 20)`); err != nil {
		t.Fatal(err)
	}

	post := func(kind, ability string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/attack-ability",
			strings.NewReader(url.Values{"kind": {kind}, "ability": {ability}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetAttackAbility(w, req)
		return w.Code
	}

	if code := post("Genjutsu", "cha"); code != http.StatusSeeOther {
		t.Fatalf("Genjutsu -> cha: status %d", code)
	}
	if code := post("Taijutsu", "dex"); code != http.StatusSeeOther {
		t.Fatalf("Taijutsu -> dex: status %d", code)
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, a := range sheet.JutsuAttacks {
		got[a.Kind] = a.Ability
	}
	if got["Genjutsu"] != "cha" || got["Taijutsu"] != "dex" || got["Ninjutsu"] != "int" {
		t.Errorf("abilities = %v, want Genjutsu cha, Taijutsu dex, Ninjutsu int (untouched)", got)
	}

	// Part 10: Bukijutsu is now a fully independent attack kind (it used to
	// mirror Taijutsu outright) — setting Taijutsu's ability must NOT move
	// Bukijutsu's, and Bukijutsu gets its own default (str) until overridden.
	var bukijutsu charsheet.JutsuAttack
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Bukijutsu" {
			bukijutsu = a
		}
	}
	if bukijutsu.Kind == "" {
		t.Fatal("no Bukijutsu attack on the sheet")
	}
	if bukijutsu.Ability != "str" {
		t.Errorf("Bukijutsu = %s, want the str default, untouched by Taijutsu's dex override", bukijutsu.Ability)
	}

	// Bukijutsu takes its own override same as the other three now.
	if code := post("Bukijutsu", "wis"); code != http.StatusSeeOther {
		t.Fatalf("Bukijutsu -> wis: status %d", code)
	}
	sheet, err = charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Bukijutsu" && a.Ability != "wis" {
			t.Errorf("Bukijutsu = %s, want wis", a.Ability)
		}
		if a.Kind == "Taijutsu" && a.Ability != "dex" {
			t.Errorf("Taijutsu = %s, want dex — Bukijutsu's override must not affect it", a.Ability)
		}
	}

	// Clearing goes back to the default.
	if code := post("Genjutsu", ""); code != http.StatusSeeOther {
		t.Fatalf("clearing Genjutsu: status %d", code)
	}
	sheet, err = charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range sheet.JutsuAttacks {
		if a.Kind == "Genjutsu" && a.Ability != "wis" {
			t.Errorf("Genjutsu = %s, want the wis default", a.Ability)
		}
	}

	// An unlisted field name must not become an override row: the column is
	// a free-text key, so an unchecked one would let a request invent rows
	// nothing ever reads or cleans up.
	if code := post("Hijutsu", "str"); code != http.StatusBadRequest {
		t.Errorf("unknown attack kind: status %d, want 400", code)
	}
	if code := post("Ninjutsu", "luck"); code != http.StatusBadRequest {
		t.Errorf("unknown ability: status %d, want 400", code)
	}
}

// TestHandleSheetFragment covers the on-demand fragment endpoint that lets a
// rest refresh the hit-dice squares, which sit outside the fragment the rest
// itself replies with.
func TestHandleSheetFragment(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Rester', 10, 14, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	get := func(name string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/characters/1/sheet/fragment/"+name, nil)
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		req.SetPathValue("name", name)
		w := httptest.NewRecorder()
		s.handleSheetFragment(w, req)
		return w
	}

	w := get("sheet_squares")
	if w.Code != http.StatusOK {
		t.Fatalf("sheet_squares: status %d, body %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// The id and data-fragment are what sheet-refresh.js round-trips through;
	// losing either would silently stop the squares refreshing.
	if !strings.Contains(body, `id="sheet-squares"`) || !strings.Contains(body, `data-fragment="sheet_squares"`) {
		t.Errorf("fragment lost its id/data-fragment hooks: %s", body)
	}
	for _, label := range []string{"Initiative", "Speed", "Hit", "Prof"} {
		if !strings.Contains(body, label) {
			t.Errorf("square row is missing %q: %s", label, body)
		}
	}

	if code := get("layout").Code; code != http.StatusNotFound {
		t.Errorf("unlisted template name: status %d, want 404", code)
	}
}

// TestCreateClanStepAbilityPicker covers the clan-bonus gap from part 7: a
// clan whose book text offers a CHOICE of ability ("+2 Str or Dex, +1 Con")
// had no clan_ability_increases rows, so the step never asked and the
// character got no bonus at all. The step must now render a dropdown, and
// the POST must store what was picked.
func TestCreateClanStepAbilityPicker(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO clans (slug, name, epithet, overview, speed_feet, extra_language, ability_increase_text)
		VALUES ('clan/hebi', 'Hebi', 'Serpent', 'Snake handlers.', 30, '', '+2 Str or Dex, +1 Con')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Test Genin')`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/characters/1/create/clan?clan=clan/hebi", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCreateClan(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// The +2 slot is a real choice and gets a <select>; the +1 slot is
	// fixed and posts a hidden field instead.
	for _, want := range []string{`name="asi_0_0"`, `value="str"`, `value="dex"`, `name="asi_0_1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("ability picker missing %s", want)
		}
	}

	post := func(form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/characters/1/create/clan", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleCreateClan(w, req)
		return w
	}

	res := post(url.Values{
		"clan_slug": {"clan/hebi"}, "asi_variant": {"0"},
		"asi_0_0": {"dex"}, "asi_0_1": {"con"},
	})
	if res.Code != http.StatusSeeOther {
		t.Fatalf("valid pick: status %d, body:\n%s", res.Code, res.Body.String())
	}
	got := map[string]int{}
	rows, err := s.charDB.Query(`SELECT ability, amount FROM character_ability_bonuses WHERE character_id = 1 AND source_kind = 'clan'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var ability string
		var amount int
		if err := rows.Scan(&ability, &amount); err != nil {
			t.Fatal(err)
		}
		got[ability] = amount
	}
	rows.Close()
	if got["dex"] != 2 || got["con"] != 1 || len(got) != 2 {
		t.Errorf("stored bonuses = %v, want dex +2 and con +1", got)
	}

	// An ability the +2 slot never offered is the player's error, not a
	// server fault: the step re-renders with a message, it does not 500.
	res = post(url.Values{
		"clan_slug": {"clan/hebi"}, "asi_variant": {"0"},
		"asi_0_0": {"cha"}, "asi_0_1": {"con"},
	})
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "creation-form-error") {
		t.Errorf("off-menu pick: status %d, want 200 with an error message", res.Code)
	}
}

// A known jutsu's row is overridable the same way weapons and custom attacks
// are: its to-hit is derived from the attack kind its description names, and
// its damage is nothing at all until the player pins it, because the book
// prints jutsu damage in prose rather than as dice.
func TestSheetJutsuOptions(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/fireball', 'Fireball', 'Ninjutsu', 'D', '1 Action', '60 feet', 'Instant', 'HS', 'Cost: 2', 'Ninjutsu',
		        'Make a Ranged Ninjutsu Attack against a creature within range.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Caster', 10, 10, 10, 18, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_jutsu (character_id, jutsu_slug) VALUES (1, 'jutsu/fireball')`); err != nil {
		t.Fatal(err)
	}

	row := func() jutsuSheetRow {
		t.Helper()
		sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := s.loadCharacterJutsuSheet(1, sheet)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d jutsu rows, want 1", len(rows))
		}
		return rows[0]
	}

	// Derived: Ninjutsu off Intelligence, full proficiency, no damage.
	base := row()
	if base.AttackKind != "Ninjutsu" || !base.Derived {
		t.Fatalf("derived row = %q (derived %v), want Ninjutsu/true", base.AttackKind, base.Derived)
	}
	if base.DamageDice != "" {
		t.Errorf("derived row has damage %q, want none", base.DamageDice)
	}

	post := func(v url.Values) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/jutsu/options", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetJutsuOptions(w, req)
		return w.Code
	}

	if code := post(url.Values{
		"slug": {"jutsu/fireball"}, "attack_ability": {"wis"}, "attack_prof": {"half"},
		"attack_bonus": {"1"}, "damage_count": {"3"}, "damage_sides": {"6"},
		"damage_ability": {"int"}, "damage_bonus": {"2"}, "damage_type": {"Fire"},
	}); code != http.StatusSeeOther {
		t.Fatalf("set jutsu options: status %d", code)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := row()
	wantAttack := sheet.Abilities["wis"].Modifier + sheet.ProficiencyBonus/2 + 1
	wantDamage := sheet.Abilities["int"].Modifier + 2
	if got.AttackBonus != wantAttack {
		t.Errorf("attack = %+d, want %+d", got.AttackBonus, wantAttack)
	}
	if got.DamageDice != "3d6" || got.DamageBonus != wantDamage || got.DamageType != "Fire" {
		t.Errorf("damage = %s %+d %s, want 3d6 %+d Fire", got.DamageDice, got.DamageBonus, got.DamageType, wantDamage)
	}
	if got.Derived {
		t.Error("row still reports itself as derived after an override")
	}

	// Forgetting the jutsu has to take its override with it, or relearning it
	// would come back silently pre-tuned. Nothing cascades: the override table
	// is keyed by slug, not by a character_jutsu row id.
	form := url.Values{"slug": {"jutsu/fireball"}}
	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/jutsu/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetJutsuDelete(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("forget jutsu: status %d", w.Code)
	}
	var n int
	if err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_jutsu_options WHERE character_id = 1 AND jutsu_slug = 'jutsu/fireball'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("override rows left after forgetting the jutsu = %d, want 0", n)
	}
}

// The sheet's Cast button only renders for a jutsu with a fixed numeric
// chakra cost — one with only a prose cost ("Special") must come back with
// CostChakra == nil rather than a false zero, or the template would render
// a "Cast 0⚡" button that spends nothing.
func TestSheetJutsuCostChakra(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, cost_chakra, keywords, description)
		VALUES ('jutsu/fireball', 'Fireball', 'Ninjutsu', 'D', '1 Action', '60 feet', 'Instant', 'HS', 'Cost: 2', 2, 'Ninjutsu',
		        'Make a Ranged Ninjutsu Attack against a creature within range.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		VALUES ('jutsu/mystery', 'Mystery Technique', 'Ninjutsu', 'D', '1 Action', '60 feet', 'Instant', 'HS', 'Special', 'Ninjutsu',
		        'A jutsu whose cost is not a flat chakra amount.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Caster', 10, 10, 10, 18, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_jutsu (character_id, jutsu_slug) VALUES (1, 'jutsu/fireball'), (1, 'jutsu/mystery')`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.loadCharacterJutsuSheet(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d jutsu rows, want 2", len(rows))
	}

	byName := map[string]jutsuSheetRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	fireball := byName["Fireball"]
	if fireball.CostChakra == nil || *fireball.CostChakra != 2 {
		t.Errorf("Fireball CostChakra = %v, want *2", fireball.CostChakra)
	}
	mystery := byName["Mystery Technique"]
	if mystery.CostChakra != nil {
		t.Errorf("Mystery Technique CostChakra = %v, want nil", *mystery.CostChakra)
	}
}

// Bukijutsu has to be selectable at creation, and has to reach the category
// filter with it. class_casting lists Ninjutsu, Genjutsu and Taijutsu for all
// eleven classes and Bukijutsu for none — not even the Weapon Specialist — so
// gating on that table alone hid every Bukijutsu jutsu in the book from every
// character. The one classified "Hijutsu, Bukijutsu" counts too; plain Hijutsu
// still does not, since those arrive via a clan list instead.
func TestCreateJutsuStepIncludesBukijutsu(t *testing.T) {
	s := testServer(t)
	for _, stmt := range []string{
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/weapon-specialist', 'Weapon Specialist', 10, 6)`,
		// Exactly what the book gives every class: no Bukijutsu row anywhere.
		`INSERT INTO class_casting (class_slug, discipline, ability) VALUES
			('class/weapon-specialist', 'ninjutsu', 'int'),
			('class/weapon-specialist', 'genjutsu', 'wis'),
			('class/weapon-specialist', 'taijutsu', 'str')`,
		`INSERT INTO class_levels (class_slug, level, jutsu_known, highest_rank_known) VALUES ('class/weapon-specialist', 1, 4, 'D')`,
		`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		 VALUES ('jutsu/blade-dance', 'Blade Dance', 'Bukijutsu', 'D', '1 Action', 'Self', 'Instant', 'CM', 'Cost: 2', 'Bukijutsu', 'A flurry of cuts.')`,
		`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		 VALUES ('jutsu/viper-stance', 'Striking Stance Viper', 'Hijutsu, Bukijutsu', 'D', '1 Action', 'Self', 'Instant', 'CM', 'Cost: 2', 'Bukijutsu', 'A coiled stance.')`,
		`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		 VALUES ('jutsu/secret-art', 'Secret Art', 'Hijutsu', 'D', '1 Action', 'Self', 'Instant', 'CM', 'Cost: 2', 'Hijutsu', 'A hidden technique.')`,
	} {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if _, err := s.charDB.Exec(`INSERT INTO characters (name) VALUES ('Bladed')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/weapon-specialist', 1, 0)`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/characters/1/create/jutsu", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCreateJutsu(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	if !strings.Contains(body, `value="jutsu/blade-dance"`) {
		t.Error("a plain Bukijutsu jutsu is not selectable at creation")
	}
	if !strings.Contains(body, `value="jutsu/viper-stance"`) {
		t.Error(`the "Hijutsu, Bukijutsu" jutsu is not selectable at creation`)
	}
	if strings.Contains(body, `value="jutsu/secret-art"`) {
		t.Error("a plain Hijutsu jutsu leaked into the class list; those come from a clan")
	}

	// jutsu-filter.js builds the Categories dropdown from the rendered rows'
	// data-classification, so the filter only offers Bukijutsu if the rows
	// carry it — this is the "make sure it's in the filters too" half.
	if !strings.Contains(body, `data-classification="Bukijutsu"`) {
		t.Error("no row carries data-classification=Bukijutsu, so the category filter can't offer it")
	}

	// And it can actually be saved, not just displayed.
	form := url.Values{"jutsu": {"jutsu/blade-dance"}}
	req = httptest.NewRequest(http.MethodPost, "/characters/1/create/jutsu", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w = httptest.NewRecorder()
	s.handleCreateJutsu(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("saving a Bukijutsu jutsu: status %d, body:\n%s", w.Code, w.Body.String())
	}
	var n int
	if err := s.charDB.QueryRow(
		`SELECT COUNT(*) FROM character_jutsu WHERE character_id = 1 AND jutsu_slug = 'jutsu/blade-dance'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("saved Bukijutsu rows = %d, want 1", n)
	}
}

// TestCreateJutsuStepIncludesClanJutsu covers the other half of part 7's
// jutsu ask: the character's own clan's jutsu have to be selectable on the
// creation step (they were completely absent), tagged as clan jutsu so the
// clan counter can pick them out, and the step has to carry the same filter
// controls the /jutsu library page has.
func TestCreateJutsuStepIncludesClanJutsu(t *testing.T) {
	s := testServer(t)
	for _, stmt := range []string{
		`INSERT INTO clans (slug, name, speed_feet) VALUES ('clan/hyuga', 'Hyuga', 30)`,
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/ninjutsu-specialist', 'Ninjutsu Specialist', 8, 8)`,
		`INSERT INTO class_casting (class_slug, discipline, ability) VALUES ('class/ninjutsu-specialist', 'ninjutsu', 'int')`,
		`INSERT INTO class_levels (class_slug, level, jutsu_known, highest_rank_known) VALUES ('class/ninjutsu-specialist', 1, 4, 'D')`,
		`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description)
		 VALUES ('jutsu/fireball', 'Fireball', 'Ninjutsu', 'D', '1 Action', '60 feet', 'Instant', 'HS', 'Cost: 2', 'Ninjutsu', 'A ball of fire.')`,
		`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, keywords, description, clan_slug)
		 VALUES ('jutsu/hyuga/gentle-fist', 'Gentle Fist', 'Hijutsu', 'D', '1 Action', 'Touch', 'Instant', 'CM', 'Cost: 2', 'Hijutsu', 'A palm strike.', 'clan/hyuga')`,
		`INSERT INTO clan_jutsu (clan_slug, jutsu_slug) VALUES ('clan/hyuga', 'jutsu/hyuga/gentle-fist')`,
	} {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if _, err := s.charDB.Exec(`INSERT INTO characters (name, clan_slug) VALUES ('Test Genin', 'clan/hyuga')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/ninjutsu-specialist', 1, 0)`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/characters/1/create/jutsu", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleCreateJutsu(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	// Both sources present, the clan one tagged.
	for _, want := range []string{
		`value="jutsu/fireball"`,
		`value="jutsu/hyuga/gentle-fist"`,
		`class="jutsu-choice-row" data-source="clan"`,
		`id="create-jutsu-clan-count"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("jutsu step missing %s", want)
		}
	}

	// The filter controls jutsu-filter.js binds to. These are ids, not
	// classes: a typo in one silently disables that filter with no error
	// anywhere, which is exactly what a test is for.
	for _, want := range []string{
		`id="create-jutsu-category-panel"`, `id="create-jutsu-details-panel"`,
		`id="create-jutsu-rank-filters"`, `id="create-jutsu-action-filters"`,
		`id="create-jutsu-duration-filters"`, `id="create-jutsu-component-filters"`,
		`id="create-jutsu-range-min"`, `id="create-jutsu-range-max"`,
		`id="create-jutsu-source-tiles"`,
		`data-casting-action="Action"`, `data-duration=`, `data-components=`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("jutsu step filter markup missing %s", want)
		}
	}

	// A clan jutsu can actually be saved.
	form := url.Values{"jutsu": {"jutsu/hyuga/gentle-fist"}}
	req = httptest.NewRequest(http.MethodPost, "/characters/1/create/jutsu", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	w = httptest.NewRecorder()
	s.handleCreateJutsu(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("saving a clan jutsu: status %d, body:\n%s", w.Code, w.Body.String())
	}
	var n int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_jutsu WHERE character_id = 1 AND jutsu_slug = 'jutsu/hyuga/gentle-fist'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("stored clan jutsu rows = %d, want 1", n)
	}
}

// A taken feat is a feature of the character, so it lists on the Core tab
// among the class and clan rows, sorted by the level it was taken at rather
// than tacked onto either end.
func TestMergeFeatFeatures(t *testing.T) {
	granted := []grantedFeatureRow{
		{Name: "Always On", SourceLabel: "Class: Taijutsu Specialist", Level: 0},
		{Name: "Star Chakra", SourceLabel: "Racial: Hoshi Clan/1st Level", Level: 1},
		{Name: "Extra Attack", SourceLabel: "Class: Taijutsu Specialist/5th Level", Level: 5},
	}
	feats := []characterFeat{
		{Name: "Bloodline, Latent", Category: "Clan (Rare)", Description: "Blood of a famous clan.",
			ChosenAtLevel: sql.NullInt64{Int64: 4, Valid: true}},
		{Name: "Chakra Guidance", Category: "Chakra",
			ChosenAtLevel: sql.NullInt64{Int64: 1, Valid: true}},
		// Written before the sheet recorded a level: still lists, at the top,
		// with no level clause to invent.
		{Name: "Old Pick", Category: "General"},
	}

	got := mergeFeatFeatures(granted, feats)
	wantOrder := []string{"Always On", "Old Pick", "Star Chakra", "Chakra Guidance", "Bloodline, Latent", "Extra Attack"}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(wantOrder), got)
	}
	for i, want := range wantOrder {
		if got[i].Name != want {
			t.Errorf("row %d = %q, want %q (full order: %v)", i, got[i].Name, want, featureNames(got))
		}
	}

	byName := map[string]grantedFeatureRow{}
	for _, row := range got {
		byName[row.Name] = row
	}
	if label := byName["Bloodline, Latent"].SourceLabel; label != "Feat: Clan (Rare)/4th Level" {
		t.Errorf("feat source label = %q, want %q", label, "Feat: Clan (Rare)/4th Level")
	}
	if label := byName["Old Pick"].SourceLabel; label != "Feat: General" {
		t.Errorf("level-less feat label = %q, want %q", label, "Feat: General")
	}
	if desc := byName["Bloodline, Latent"].Description; desc != "Blood of a famous clan." {
		t.Errorf("feat description = %q, want the rules text", desc)
	}
}

func featureNames(rows []grantedFeatureRow) []string {
	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.Name
	}
	return names
}

// TestHandleSheetAC covers part 9's "same adjust menu as Initiative" ask for
// AC — the same ability/proficiency/bonus composer stacked on top of
// whatever computeEquippedAC already produced.
func TestHandleSheetAC(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Guard', 10, 14, 10, 10, 18, 10)`); err != nil {
		t.Fatal(err)
	}

	post := func(v url.Values) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/ac", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetAC(w, req)
		return w.Code
	}

	before, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before.AC == nil || *before.AC != 12 {
		t.Fatalf("starting AC = %v, want 12 (10 + Dex 14's +2)", before.AC)
	}

	if code := post(url.Values{"ability": {"wis"}, "prof": {"full"}, "bonus": {"1"}}); code != http.StatusSeeOther {
		t.Fatalf("set AC adjustment: status %d", code)
	}
	after, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := *before.AC + after.Abilities["wis"].Modifier + after.ProficiencyBonus + 1
	if after.AC == nil || *after.AC != want {
		t.Errorf("AC = %v, want %d", after.AC, want)
	}
	if after.ACAbility != "wis" || after.ACProf != "full" || after.ACBonus != 1 {
		t.Errorf("AC adjustment fields = %q/%q/%d, want wis/full/1", after.ACAbility, after.ACProf, after.ACBonus)
	}

	// Clearing every field goes back to the plain computed AC.
	if code := post(url.Values{"ability": {""}, "prof": {""}, "bonus": {""}}); code != http.StatusSeeOther {
		t.Fatalf("clear AC adjustment: status %d", code)
	}
	cleared, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.AC == nil || *cleared.AC != *before.AC {
		t.Errorf("AC = %v after clearing, want the default %v", cleared.AC, before.AC)
	}

	if code := post(url.Values{"ability": {"luck"}}); code != http.StatusBadRequest {
		t.Errorf("unknown ability: status %d, want 400", code)
	}
	if code := post(url.Values{"prof": {"double"}}); code != http.StatusBadRequest {
		t.Errorf("unknown proficiency mode: status %d, want 400", code)
	}
	if code := post(url.Values{"bonus": {"lots"}}); code != http.StatusBadRequest {
		t.Errorf("non-numeric bonus: status %d, want 400", code)
	}
}

// TestHandleSheetSpeed covers part 10's "make Speed editable like Ryo" ask:
// an absolute number pins an override, a signed one adjusts the CURRENT
// effective speed (clan default or an earlier override, not whatever the
// client last rendered), and it can never go negative.
func TestHandleSheetSpeed(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`INSERT INTO clans (slug, name, speed_feet) VALUES ('clan/fast', 'Fast Clan', 35)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, clan_slug, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Runner', 'clan/fast', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	post := func(v url.Values) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/speed", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetSpeed(w, req)
		return w.Code
	}

	before, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before.Speed != 35 {
		t.Fatalf("starting Speed = %d, want the clan's 35", before.Speed)
	}

	if code := post(url.Values{"value": {"+5"}}); code != http.StatusSeeOther {
		t.Fatalf("relative adjust: status %d", code)
	}
	after, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if after.Speed != 40 {
		t.Errorf("Speed = %d after +5, want 40", after.Speed)
	}

	if code := post(url.Values{"value": {"10"}}); code != http.StatusSeeOther {
		t.Fatalf("absolute set: status %d", code)
	}
	set, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if set.Speed != 10 {
		t.Errorf("Speed = %d after setting to 10, want 10", set.Speed)
	}

	if code := post(url.Values{"value": {"-50"}}); code != http.StatusBadRequest {
		t.Errorf("dropping below 0: status %d, want 400", code)
	}
	if code := post(url.Values{"value": {"fast"}}); code != http.StatusBadRequest {
		t.Errorf("non-numeric value: status %d, want 400", code)
	}
	if code := post(url.Values{"value": {""}}); code != http.StatusBadRequest {
		t.Errorf("blank value: status %d, want 400", code)
	}
}

// TestHandleSheetInventoryAddCustom covers the "+ Add custom item" escape
// hatch for anything the equipment catalogue has no row for. Since part 11,
// this creates a shared, slug-addressable custom_items row (the local item
// library) and points a normal character_inventory row at it by slug,
// rather than storing the item inline — same shape a catalogue item's row
// has, so it gets a real detail page and can be reused on other characters.
func TestHandleSheetInventoryAddCustom(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Packrat', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	post := func(v url.Values, wantFragment bool) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/inventory/custom", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if wantFragment {
			req.Header.Set("X-Requested-With", "fetch")
		}
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetInventoryAddCustom(w, req)
		return w.Code
	}

	if code := post(url.Values{"name": {"Lucky Coin"}, "quantity": {"3"}, "notes": {"Found it"}}, false); code != http.StatusSeeOther {
		t.Fatalf("plain POST: status %d, want 303", code)
	}
	var slug string
	var quantity int
	if err := s.charDB.QueryRow(
		`SELECT item_slug, quantity FROM character_inventory WHERE character_id = 1`,
	).Scan(&slug, &quantity); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(slug, "custom/") || quantity != 3 {
		t.Errorf("stored item = slug %q x%d, want a custom/ slug x3", slug, quantity)
	}
	var libName, libDescription string
	if err := s.charDB.QueryRow(`SELECT name, description FROM custom_items WHERE slug = ?`, slug).
		Scan(&libName, &libDescription); err != nil {
		t.Fatal(err)
	}
	if libName != "Lucky Coin" || libDescription != "Found it" {
		t.Errorf("library entry = {%q, %q}, want {Lucky Coin, Found it}", libName, libDescription)
	}

	// A fetch POST (the JS-handled path) gets the sheet_inventory_full
	// fragment back (200), not a redirect — this endpoint is only ever
	// posted from the Inventory tab, so that's always the right fragment
	// (see screen-flash-bug fix: this used to be a bare 204 before custom
	// item add swapped a fragment instead of reloading the whole page).
	if code := post(url.Values{"name": {"Spare Kunai"}}, true); code != http.StatusOK {
		t.Fatalf("fetch POST: status %d, want 200", code)
	}
	var count int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_inventory WHERE character_id = 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("inventory rows = %d, want 2", count)
	}

	// A blank name is rejected outright rather than storing an unnamed row.
	if code := post(url.Values{"name": {"  "}}, false); code != http.StatusBadRequest {
		t.Errorf("blank name: status %d, want 400", code)
	}

	// A non-numeric or sub-1 quantity silently falls back to 1 rather than
	// rejecting the whole item.
	if code := post(url.Values{"name": {"Odd Rock"}, "quantity": {"0"}}, false); code != http.StatusSeeOther {
		t.Fatalf("zero quantity: status %d", code)
	}
	var oddQty int
	if err := s.charDB.QueryRow(`
		SELECT ci.quantity FROM character_inventory ci JOIN custom_items on ci.item_slug = custom_items.slug
		WHERE ci.character_id = 1 AND custom_items.name = 'Odd Rock'`,
	).Scan(&oddQty); err != nil {
		t.Fatal(err)
	}
	if oddQty != 1 {
		t.Errorf("zero quantity stored as %d, want fallback to 1", oddQty)
	}

	// Part 10: type, bulk, equipped and description are all optional but
	// all storable — the whole point of replacing "Add by name" with a real
	// custom-item form.
	if code := post(url.Values{
		"name": {"Shinobi Cloak"}, "kind": {"Armor"}, "bulk": {"1.5"}, "equipped": {"1"}, "notes": {"Handmade"},
	}, false); code != http.StatusSeeOther {
		t.Fatalf("full custom item POST: status %d, want 303", code)
	}
	var kind, itemNotes string
	var bulk float64
	var equipped bool
	if err := s.charDB.QueryRow(`
		SELECT custom_items.kind, custom_items.bulk, ci.equipped, custom_items.description
		FROM character_inventory ci JOIN custom_items on ci.item_slug = custom_items.slug
		WHERE ci.character_id = 1 AND custom_items.name = 'Shinobi Cloak'`,
	).Scan(&kind, &bulk, &equipped, &itemNotes); err != nil {
		t.Fatal(err)
	}
	if kind != "Armor" || bulk != 1.5 || !equipped || itemNotes != "Handmade" {
		t.Errorf("custom item = {kind %q, bulk %v, equipped %v, notes %q}, want {Armor, 1.5, true, Handmade}",
			kind, bulk, equipped, itemNotes)
	}

	// A malformed bulk is rejected outright rather than silently stored as 0
	// or dropped — a typo shouldn't quietly zero out a carried weight.
	if code := post(url.Values{"name": {"Bad Bulk"}, "bulk": {"heavy"}}, false); code != http.StatusBadRequest {
		t.Errorf("bad bulk: status %d, want 400", code)
	}

	// Part 11: rollable_kind='weapon' stores its damage/properties on the
	// library entry — buildAttacks reads these directly (see
	// GetCustomItemBySlug's use in characters.go), no override table needed.
	if code := post(url.Values{
		"name": {"Rusty Cleaver"}, "rollable_kind": {"weapon"},
		"damage_count": {"1"}, "damage_sides": {"8"}, "damage_type": {"slashing"}, "properties": {"finesse"},
	}, false); code != http.StatusSeeOther {
		t.Fatalf("weapon custom item POST: status %d, want 303", code)
	}
	var rollableKind, damageDice, damageType, properties string
	if err := s.charDB.QueryRow(`
		SELECT rollable_kind, damage_dice, damage_type, properties FROM custom_items WHERE name = 'Rusty Cleaver'`,
	).Scan(&rollableKind, &damageDice, &damageType, &properties); err != nil {
		t.Fatal(err)
	}
	if rollableKind != "weapon" || damageDice != "1d8" || damageType != "slashing" || properties != "finesse" {
		t.Errorf("weapon rollable = {%q, %q, %q, %q}, want {weapon, 1d8, slashing, finesse}",
			rollableKind, damageDice, damageType, properties)
	}

	// rollable_kind='toolkit' immediately grants a tool proficiency under
	// the item's name (same as picking a toolkit from a choice slot).
	if code := post(url.Values{"name": {"Lockpick Set"}, "rollable_kind": {"toolkit"}}, false); code != http.StatusSeeOther {
		t.Fatalf("toolkit custom item POST: status %d, want 303", code)
	}
	var toolCount int
	if err := s.charDB.QueryRow(`
		SELECT COUNT(*) FROM character_proficiencies WHERE character_id = 1 AND kind = 'tool' AND value = 'Lockpick Set'`,
	).Scan(&toolCount); err != nil {
		t.Fatal(err)
	}
	if toolCount != 1 {
		t.Errorf("tool proficiency rows for Lockpick Set = %d, want 1", toolCount)
	}

	// rollable_kind='other' gets a flat-bonus custom attack (kind='item'),
	// the "Other Rollables" section of the Attacks & Jutsu box.
	if code := post(url.Values{"name": {"Lucky Charm"}, "rollable_kind": {"other"}}, false); code != http.StatusSeeOther {
		t.Fatalf("other-rollable custom item POST: status %d, want 303", code)
	}
	var attackCount int
	if err := s.charDB.QueryRow(`
		SELECT COUNT(*) FROM character_custom_attacks WHERE character_id = 1 AND kind = 'item' AND name = 'Lucky Charm'`,
	).Scan(&attackCount); err != nil {
		t.Fatal(err)
	}
	if attackCount != 1 {
		t.Errorf("custom attack rows for Lucky Charm = %d, want 1", attackCount)
	}
}

// TestHandleCustomItemUpdate covers the library entry's own edit form (its
// detail page's "Edit this item" disclosure) — part 11 replaced the old
// per-character "explode into a detail view" edit row with this, since a
// custom item can now be shared by more than one character and editing it
// has to update the one shared entry, not a character-scoped copy.
func TestHandleCustomItemUpdate(t *testing.T) {
	s := testServer(t)
	created, err := charstore.AddCustomItem(s.charDB, charstore.CustomItem{Name: "Sealed Scroll"})
	if err != nil {
		t.Fatal(err)
	}

	post := func(v url.Values) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost,
			fmt.Sprintf("/custom-items/%d/update", created.ID), strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", strconv.FormatInt(created.ID, 10))
		w := httptest.NewRecorder()
		s.handleCustomItemUpdate(w, req)
		return w
	}

	w := post(url.Values{
		"name": {"Sealed Scroll (opened)"}, "kind": {"Scroll"}, "bulk": {"0.5"}, "notes": {"Reads blank now"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("update: status %d, want 303", w.Code)
	}
	// Slug is stable across an edit — every inventory row pointing at it,
	// on every character, must not go stale.
	wantLocation := "/items/" + created.Slug
	if loc := w.Header().Get("Location"); loc != wantLocation {
		t.Errorf("redirect Location = %q, want %q", loc, wantLocation)
	}
	updated, err := charstore.GetCustomItemBySlug(s.charDB, created.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Sealed Scroll (opened)" || updated.Kind != "Scroll" ||
		!updated.Bulk.Valid || updated.Bulk.Float64 != 0.5 || updated.Description != "Reads blank now" {
		t.Errorf("updated item = %+v, want {Sealed Scroll (opened), Scroll, 0.5, Reads blank now}", updated)
	}

	// A blank name is rejected outright, same as the add form.
	if code := post(url.Values{"name": {"  "}}).Code; code != http.StatusBadRequest {
		t.Errorf("blank name: status %d, want 400", code)
	}

	// An id with no matching row 404s rather than silently no-op'ing.
	req := httptest.NewRequest(http.MethodPost, "/custom-items/99999/update", strings.NewReader(url.Values{"name": {"X"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "99999")
	w2 := httptest.NewRecorder()
	s.handleCustomItemUpdate(w2, req)
	if w2.Code != http.StatusNotFound {
		t.Errorf("unknown id: status %d, want 404", w2.Code)
	}
}

// TestSheetProficiencyFragmentDispatch covers part 10's scroll-reset fix:
// handleSheetCustomProf and handleSheetProficiency now answer with a live
// fragment (instead of a full-page redirect) on a fetch request, and which
// fragment comes back has to follow which box the row actually lives in —
// tool/custom-skill rows in Tool Proficiencies & Custom Skills, language
// rows in Languages, and a BOOK skill's own bullet toggle still in Skills.
func TestSheetProficiencyFragmentDispatch(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Linguist', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	fetchPost := func(action string, v url.Values) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, action, strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		if strings.HasSuffix(action, "/custom-prof") {
			s.handleSheetCustomProf(w, req)
		} else {
			s.handleSheetProficiency(w, req)
		}
		return w.Code, w.Body.String()
	}

	// Adding a tool answers with the Tool Proficiencies & Custom Skills box.
	if code, body := fetchPost("/characters/1/sheet/custom-prof", url.Values{"kind": {"tool"}, "value": {"Smithing Tools"}}); code != http.StatusOK || !strings.Contains(body, `id="sheet-tools-skills"`) {
		t.Errorf("add tool: status %d, body contains sheet-tools-skills = %v", code, strings.Contains(body, `id="sheet-tools-skills"`))
	}
	// Adding a language answers with the Languages box.
	if code, body := fetchPost("/characters/1/sheet/custom-prof", url.Values{"kind": {"language"}, "value": {"Elvish"}}); code != http.StatusOK || !strings.Contains(body, `id="sheet-languages"`) {
		t.Errorf("add language: status %d, body contains sheet-languages = %v", code, strings.Contains(body, `id="sheet-languages"`))
	}
	// A custom skill (not one of the rules' own) also answers with T&CS.
	if code, body := fetchPost("/characters/1/sheet/custom-prof", url.Values{"kind": {"skill"}, "value": {"Calligraphy"}}); code != http.StatusOK || !strings.Contains(body, `id="sheet-tools-skills"`) {
		t.Errorf("add custom skill: status %d, body contains sheet-tools-skills = %v", code, strings.Contains(body, `id="sheet-tools-skills"`))
	}
	// Removing the tool (the T&CS remove button) also targets T&CS.
	if code, body := fetchPost("/characters/1/sheet/proficiency", url.Values{"kind": {"tool"}, "value": {"Smithing Tools"}, "on": {"0"}}); code != http.StatusOK || !strings.Contains(body, `id="sheet-tools-skills"`) {
		t.Errorf("remove tool: status %d, body contains sheet-tools-skills = %v", code, strings.Contains(body, `id="sheet-tools-skills"`))
	}
	// Removing the language (the Languages remove button) targets Languages.
	if code, body := fetchPost("/characters/1/sheet/proficiency", url.Values{"kind": {"language"}, "value": {"Elvish"}, "on": {"0"}}); code != http.StatusOK || !strings.Contains(body, `id="sheet-languages"`) {
		t.Errorf("remove language: status %d, body contains sheet-languages = %v", code, strings.Contains(body, `id="sheet-languages"`))
	}
	// A book skill's own proficiency bullet (kind=skill, a name SkillAbility
	// knows) still targets the Skills box, unchanged from before part 10.
	if code, body := fetchPost("/characters/1/sheet/proficiency", url.Values{"kind": {"skill"}, "value": {"Acrobatics"}, "on": {"1"}}); code != http.StatusOK || !strings.Contains(body, `id="sheet-skills"`) {
		t.Errorf("toggle book skill: status %d, body contains sheet-skills = %v", code, strings.Contains(body, `id="sheet-skills"`))
	}
}

// TestHandleSheetProficiencyMod covers the per-line roll tweak that makes
// every Tool Proficiencies & Custom Skills row "fully customizable" —
// setting it, clearing it back out, and buildProficiencyRows folding stored
// mods together with the sheet's plain proficiency list.
func TestHandleSheetProficiencyMod(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Tinkerer', 10, 10, 10, 16, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref) VALUES
			(1, 'tool', 'Smithing Tools', 'class', 'class/weapon-master'),
			(1, 'tool', 'Cook''s Utensils', 'background', 'background/cook')`,
	); err != nil {
		t.Fatal(err)
	}

	post := func(v url.Values) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/proficiency-mod", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetProficiencyMod(w, req)
		return w.Code
	}

	if code := post(url.Values{"kind": {"tool"}, "value": {"Smithing Tools"}, "ability": {"int"}, "prof": {"full"}, "bonus": {"1"}}); code != http.StatusSeeOther {
		t.Fatalf("set mod: status %d", code)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	mods, err := s.loadProficiencyMods(1, "tool")
	if err != nil {
		t.Fatal(err)
	}
	rows := buildProficiencyRows([]string{"Smithing Tools", "Cook's Utensils"}, "tool", mods, sheet)
	byName := map[string]profRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	wantMod := sheet.Abilities["int"].Modifier + sheet.ProficiencyBonus + 1
	if got := byName["Smithing Tools"].Modifier; got != wantMod {
		t.Errorf("Smithing Tools modifier = %+d, want %+d", got, wantMod)
	}
	if got := byName["Cook's Utensils"].Modifier; got != 0 {
		t.Errorf("Cook's Utensils (never adjusted) modifier = %+d, want 0", got)
	}

	// Clearing every field back to blank/none/0 deletes the row rather than
	// leaving a neutral one behind — SetProficiencyMod's own contract.
	if code := post(url.Values{"kind": {"tool"}, "value": {"Smithing Tools"}, "ability": {""}, "prof": {""}, "bonus": {"0"}}); code != http.StatusSeeOther {
		t.Fatalf("clear mod: status %d", code)
	}
	var count int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_proficiency_mods WHERE character_id = 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("proficiency_mods rows after clearing = %d, want 0", count)
	}

	if code := post(url.Values{"kind": {"weapon"}, "value": {"x"}}); code != http.StatusBadRequest {
		t.Errorf("unknown kind: status %d, want 400", code)
	}
	if code := post(url.Values{"kind": {"tool"}, "value": {""}}); code != http.StatusBadRequest {
		t.Errorf("blank value: status %d, want 400", code)
	}
	if code := post(url.Values{"kind": {"tool"}, "value": {"x"}, "ability": {"luck"}}); code != http.StatusBadRequest {
		t.Errorf("unknown ability: status %d, want 400", code)
	}
}

// TestMasteryAppliesToToolkitRow covers the half of Mastery that lives
// outside charsheet.Compute: the book grants Mastery "with a given skill or
// toolkit", but a toolkit's roll is composed by buildProficiencyRows from
// character_proficiency_mods, so the rank has to travel via
// Sheet.MasteryRanks to reach it. It also checks Mastery lands on top of a
// row's own hand-tuned bonus instead of replacing it.
func TestMasteryAppliesToToolkitRow(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Tinkerer', 10, 10, 10, 16, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/test', 7, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_proficiencies (character_id, kind, value, source_kind, source_ref) VALUES
			(1, 'tool', 'Smithing Tools', 'class', 'class/test'),
			(1, 'skill', 'Calligraphy', 'other', 'custom')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_mastery (character_id, skill_name, rank) VALUES (1, 'Smithing Tools', 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_proficiency_mods (character_id, kind, value, ability, prof_mode, bonus)
		VALUES (1, 'tool', 'Smithing Tools', 'int', 'full', 1)`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	mods, err := s.loadProficiencyMods(1, "tool")
	if err != nil {
		t.Fatal(err)
	}
	rows := buildProficiencyRows([]string{"Smithing Tools"}, "tool", mods, sheet)
	want := sheet.Abilities["int"].Modifier + sheet.ProficiencyBonus + 1 + 4
	if rows[0].Modifier != want {
		t.Errorf("Smithing Tools modifier = %+d, want %+d (int + proficiency + flat 1 + Rank 2 Mastery)", rows[0].Modifier, want)
	}
	if rows[0].MasteryRank != 2 {
		t.Errorf("Smithing Tools MasteryRank = %d, want 2", rows[0].MasteryRank)
	}

	// A custom skill is a skill for Mastery's purposes, and both it and the
	// toolkit have to be offerable in the picker.
	names, err := s.masteryEligibleNames(1)
	if err != nil {
		t.Fatal(err)
	}
	has := map[string]bool{}
	for _, n := range names {
		has[n] = true
	}
	for _, want := range []string{"Acrobatics", "Smithing Tools", "Calligraphy"} {
		if !has[want] {
			t.Errorf("masteryEligibleNames missing %q", want)
		}
	}
}

// TestHandleSheetMasteryAddRejectsOverCapRank covers the server-side half of
// the level gate: the rank <select> only offers what the character's level
// allows, but that is a UI convenience — a posted rank above the cap has to
// be refused outright, and an eligible one at or below it accepted.
func TestHandleSheetMasteryAddRejectsOverCapRank(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Novice', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/test', 6, 0)`); err != nil {
		t.Fatal(err)
	}

	post := func(v url.Values) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/mastery", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleSheetMasteryAdd(w, req)
		return w.Code, w.Body.String()
	}

	if code, _ := post(url.Values{"skill_name": {"Acrobatics"}, "rank": {"2"}}); code != http.StatusBadRequest {
		t.Errorf("Rank 2 at level 6: status %d, want 400", code)
	}
	if code, _ := post(url.Values{"skill_name": {"Nonexistent Kit"}, "rank": {"1"}}); code != http.StatusBadRequest {
		t.Errorf("unowned toolkit: status %d, want 400", code)
	}
	code, body := post(url.Values{"skill_name": {"Acrobatics"}, "rank": {"1"}})
	if code != http.StatusOK || !strings.Contains(body, `id="sheet-mastery"`) {
		t.Errorf("Rank 1 at level 6: status %d, body has the Mastery fragment = %v", code, strings.Contains(body, `id="sheet-mastery"`))
	}
	var rank int
	if err := s.charDB.QueryRow(
		`SELECT rank FROM character_mastery WHERE character_id = 1 AND skill_name = 'Acrobatics'`).Scan(&rank); err != nil {
		t.Fatal(err)
	}
	if rank != 1 {
		t.Errorf("stored rank = %d, want 1", rank)
	}
}

// TestLoadClassSummary covers the header's "under the name" class/subclass
// line, including the multiclass case where two classes each carry their
// own, independently-chosen subclass.
func TestLoadClassSummary(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO classes (slug, name) VALUES
			('class/genjutsu-specialist', 'Genjutsu Specialist'),
			('class/weapon-master', 'Weapon Master')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
			('class/genjutsu-specialist/group/pledge', 'class/genjutsu-specialist', 'Genjutsu Pledge')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO subclasses (slug, group_slug, name) VALUES
			('subclass/beguiler', 'class/genjutsu-specialist/group/pledge', 'Beguiler')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Multiclasser', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES
			(1, 'class/genjutsu-specialist', 5, 0),
			(1, 'class/weapon-master', 2, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (1, 'subclass/beguiler', 3)`,
	); err != nil {
		t.Fatal(err)
	}

	rows, err := s.loadClassSummary(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0].ClassName != "Genjutsu Specialist" || rows[0].Levels != 5 || rows[0].Subclass != "Beguiler" {
		t.Errorf("row 0 = %+v, want Genjutsu Specialist 5 (Beguiler)", rows[0])
	}
	if rows[1].ClassName != "Weapon Master" || rows[1].Levels != 2 || rows[1].Subclass != "" {
		t.Errorf("row 1 = %+v, want Weapon Master 2 (no subclass)", rows[1])
	}
}

// TestHandleSheetFeatAddDelete pins the screen-flash-bug fix for the Feats
// tab: taking or dropping a feat used to always answer with a full-page
// reload/redirect (respondSheetReload/redirectToSheet), unlike every other
// sheet-shaped handler. Both now answer a fetch POST with the sheet_feats
// fragment instead.
func TestHandleSheetFeatAddDelete(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO feats (slug, name, category, prerequisites, description)
		VALUES ('feat/tough', 'Tough', 'general', NULL, 'More hit points.')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Featured', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}

	post := func(path string, v url.Values, handler http.HandlerFunc) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "fetch")
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		handler(w, req)
		return w.Code
	}

	code := post("/characters/1/sheet/feats", url.Values{"slug": {"feat/tough"}}, s.handleSheetFeatAdd)
	if code != http.StatusOK {
		t.Fatalf("add: status %d, want 200 (sheet_feats fragment, not a reload)", code)
	}
	var count int
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_feats WHERE character_id = 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("character_feats rows after add = %d, want 1", count)
	}

	code = post("/characters/1/sheet/feats/delete", url.Values{"slug": {"feat/tough"}}, s.handleSheetFeatDelete)
	if code != http.StatusOK {
		t.Fatalf("delete: status %d, want 200 (sheet_feats fragment, not a reload)", code)
	}
	if err := s.charDB.QueryRow(`SELECT COUNT(*) FROM character_feats WHERE character_id = 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("character_feats rows after delete = %d, want 0", count)
	}
}

// TestCharacterSheetHasPuppetsTabWhenGated confirms the Puppets tab's real
// DOM-removal gating: absent entirely (button and panel both) for a
// character with no Puppet Master levels, present once they have at least
// one — the exact condition TestCharacterSheetLibraryPanes' fixed panel
// count relies on staying at 0 for its own (non-Puppet-Master) test
// character.
func TestCharacterSheetHasPuppetsTabWhenGated(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 10)`); err != nil {
		t.Fatal(err)
	}
	res, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Puppeteer', 10, 14, 12, 8, 13, 15)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	render := func() string {
		req := httptest.NewRequest(http.MethodGet, "/characters/1", nil)
		req.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		s.handleCharacterSheet(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}

	if body := render(); strings.Contains(body, `data-tab="puppets"`) || strings.Contains(body, `data-tab-panel="puppets"`) {
		t.Errorf("Puppets tab present for a character with no Puppet Master levels")
	}

	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (?, 'class/puppet-master', 2, 0)`, id); err != nil {
		t.Fatal(err)
	}

	body := render()
	if !strings.Contains(body, `data-tab="puppets"`) {
		t.Errorf("Puppets tab button missing for a Puppet Master character")
	}
	if !strings.Contains(body, `data-tab-panel="puppets"`) {
		t.Errorf("Puppets tab panel missing for a Puppet Master character")
	}
}
