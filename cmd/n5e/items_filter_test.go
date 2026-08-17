package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// seedFilterEquipment inserts one row per case the weapon/armor sub-filters
// have to tell apart: a categorised weapon, a weapon the rules leave
// uncategorised (Net and Unarmed Strike are the two real ones), a piece of
// armor, and a gear item that neither sub-filter may touch.
func seedFilterEquipment(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`
		INSERT INTO equipment (slug, name, kind, weapon_category, armor_category, bulk) VALUES
			('weapon/kunai', 'Kunai', 'weapon', 'simple', NULL, 1),
			('weapon/katana', 'Katana', 'weapon', 'martial', NULL, 1),
			('weapon/net', 'Net', 'weapon', NULL, NULL, 1),
			('armor/flak-jacket', 'Flak Jacket', 'armor', NULL, 'medium', 2),
			('item/rope', 'Rope', 'gear', NULL, NULL, 1)`); err != nil {
		t.Fatal(err)
	}
}

// tagWithSlug returns the single HTML tag carrying data-slug="{slug}", so an
// assertion can be about one row rather than about counts over the whole page
// (the rules schema seeds equipment of its own, so page-wide totals aren't
// stable).
func tagWithSlug(t *testing.T, body, slug string) string {
	t.Helper()
	at := strings.Index(body, `data-slug="`+slug+`"`)
	if at < 0 {
		t.Fatalf("no row for %s in the rendered page", slug)
	}
	open := strings.LastIndex(body[:at], "<")
	end := strings.Index(body[at:], ">")
	if open < 0 || end < 0 {
		t.Fatalf("row for %s isn't inside a tag", slug)
	}
	return body[open : at+end]
}

// assertSubfilterAttrs checks what both lists must stamp on their rows: the
// bucket on rows of the matching kind, "other" for the weapon whose category
// the rules don't record, and no attribute at all on gear — a row with no
// attribute is the only way the filter knows to leave it alone (see
// equipmentFilterCategory).
func assertSubfilterAttrs(t *testing.T, body string) {
	t.Helper()
	for _, tc := range []struct{ slug, want, absent string }{
		{"weapon/kunai", `data-weapon-category="simple"`, "data-armor-category"},
		{"weapon/katana", `data-weapon-category="martial"`, "data-armor-category"},
		{"weapon/net", `data-weapon-category="other"`, "data-armor-category"},
		{"armor/flak-jacket", `data-armor-category="medium"`, "data-weapon-category"},
		{"item/rope", "", "data-weapon-category"},
	} {
		tag := tagWithSlug(t, body, tc.slug)
		if tc.want != "" && !strings.Contains(tag, tc.want) {
			t.Errorf("%s row is missing %s: %s", tc.slug, tc.want, tag)
		}
		if strings.Contains(tag, tc.absent) {
			t.Errorf("%s row carries %s and shouldn't: %s", tc.slug, tc.absent, tag)
		}
	}
	// Gear must be untouched by both, not just by the weapon filter.
	if tag := tagWithSlug(t, body, "item/rope"); strings.Contains(tag, "data-armor-category") {
		t.Errorf("gear row carries data-armor-category: %s", tag)
	}
}

func TestItemsPageSubfilterAttributes(t *testing.T) {
	s := testServer(t)
	seedFilterEquipment(t, s)

	req := httptest.NewRequest(http.MethodGet, "/items", nil)
	w := httptest.NewRecorder()
	s.handleItems(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	// The container equipment-subfilter.js fills in. Without it the rows'
	// attributes are inert.
	if !strings.Contains(body, `id="items-subfilters"`) {
		t.Error("items page is missing the sub-filter container")
	}
	assertSubfilterAttrs(t, body)
}

func TestSheetItemLibrarySubfilterAttributes(t *testing.T) {
	s := testServer(t)
	seedFilterEquipment(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha, current_hp)
		VALUES ('Filter Test', 10, 10, 10, 10, 10, 10, 5)`); err != nil {
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

	if !strings.Contains(body, `id="sheet-item-subfilters"`) {
		t.Error("inventory library pane is missing the sub-filter container")
	}
	// The feats pane shares sheet_library_pane, so it gets a container too —
	// equipment-subfilter.js hides it, but it must not be missing outright or
	// list.dataset.subfilters would point at nothing.
	if !strings.Contains(body, `id="sheet-feat-subfilters"`) {
		t.Error("feat library pane is missing the sub-filter container")
	}
	assertSubfilterAttrs(t, body)
}
