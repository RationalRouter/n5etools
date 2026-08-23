package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// seedNinjaneerCatalog seeds enough rules.db content for a Ninjaneer's own
// weapon designations to unlock: the class itself, a Creation Points chart
// row (loadScienceNinTabData's own early-exit gate), Ninjaneer's subclass
// chain, and all four of its own weapon-shaped features — Enhanced Arsenal
// and A Weapon to Surpass at 3rd level, Warrior of Science at 9th, The
// Future of Shinobi: Weapons at 20th (levels confirmed against dist/
// rules.db's own v_subclass_features) — plus a Finesse and a non-Finesse
// weapon to designate.
func seedNinjaneerCatalog(t *testing.T, s *server, level int) {
	t.Helper()
	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/science-nin', 'Science-Nin', 8, 6)`)
	mustExecRules(`INSERT INTO class_levels (class_slug, level) VALUES ('class/science-nin', ?)`, level)
	mustExecRules(`INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/science-nin', ?, 'Creation Points', '20')`, level)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/ninjaneer', 'class/science-nin/group/scientific-inquiry', 'Ninjaneer')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/ninjaneer', 'Enhanced Arsenal', 3, 'Test.', 1),
		(?, 'class/science-nin/group/scientific-inquiry/ninjaneer', 'A Weapon to Surpass', 3, 'Test.', 2),
		(?, 'class/science-nin/group/scientific-inquiry/ninjaneer', 'Warrior of Science', 9, 'Test.', 3),
		(?, 'class/science-nin/group/scientific-inquiry/ninjaneer', 'The Future of Shinobi: Weapons', 20, 'Test.', 4)`,
		scienceNinArsenalFeatureSlug, scienceNinPerfectedWeaponFeatureSlug, scienceNinWarriorOfScienceFeatureSlug, scienceNinFutureOfShinobiWeaponsFeatureSlug)
	mustExecRules(`INSERT INTO equipment (slug, name, kind, damage_dice, damage_type, properties, description) VALUES
		('weapon/test-finesse', 'Test Finesse Blade', 'weapon', '1d4', 'Slashing', 'Finesse', 'A lightweight test blade favoring finesse strikes.'),
		('weapon/test-heavy', 'Test Heavy Blade', 'weapon', '2d6', 'Slashing', 'Heavy, Two-Handed', 'A cumbersome test blade with no Finesse property.')`)
}

// seedNinjaneerCharacter inserts a character with the given Science-Nin
// level, already 3rd-level-subclassed into Ninjaneer.
func seedNinjaneerCharacter(t *testing.T, s *server, name string, level, baseInt int) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, ?, 10, 10)`, name, baseInt)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/science-nin', ?, 0)`,
		id, level,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/science-nin/group/scientific-inquiry/ninjaneer', 3)`,
		id,
	); err != nil {
		t.Fatal(err)
	}
	return id
}

// addInventoryWeapon inserts an equipped weapon row and returns its
// character_inventory id.
func addInventoryWeapon(t *testing.T, s *server, characterID int64, itemSlug string) int64 {
	t.Helper()
	res, err := s.charDB.Exec(
		`INSERT INTO character_inventory (character_id, item_slug, quantity, equipped) VALUES (?, ?, 1, 1)`,
		characterID, itemSlug)
	if err != nil {
		t.Fatal(err)
	}
	invID, _ := res.LastInsertId()
	return invID
}

// ninjaneerDesignationRequest posts to one of Ninjaneer's own weapon-
// designation routes (mirrors bimAddRequest in science_nin_bim_test.go) —
// real, server.go-registered routes run through s.routes() end to end, not
// a hand-copied stand-in for it.
func ninjaneerDesignationRequest(t *testing.T, s *server, characterID int64, path string, inventoryID int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/characters/"+strconv.FormatInt(characterID, 10)+"/sheet/"+path,
		strings.NewReader(url.Values{"option_slug": {strconv.FormatInt(inventoryID, 10)}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	return w
}

func weaponAttackOptionsFor(t *testing.T, s *server, characterID, inventoryID int64) charstore.WeaponAttackOptions {
	t.Helper()
	all, err := charstore.ListWeaponAttackOptions(s.charDB, characterID)
	if err != nil {
		t.Fatal(err)
	}
	return all[inventoryID]
}

// TestNinjaneerEnhancedWeaponDesignationFinesseOverride covers Enhanced
// Arsenal's own clause ("You can use your Intelligence Modifier in place of
// Dexterity for weapon attacks and Bukijutsu using Enhanced Finesse
// weapons"): designating a Finesse weapon Enhanced must pin its attack
// ability to Intelligence, and designating a non-Finesse weapon must NOT.
func TestNinjaneerEnhancedWeaponDesignationFinesseOverride(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 3)
	id := seedNinjaneerCharacter(t, s, "Test Ninjaneer", 3, 16)
	finesseInv := addInventoryWeapon(t, s, id, "weapon/test-finesse")
	heavyInv := addInventoryWeapon(t, s, id, "weapon/test-heavy")

	if w := ninjaneerDesignationRequest(t, s, id, "science-nin-enhanced-weapon", finesseInv); w.Code != http.StatusOK {
		t.Fatalf("designate Finesse weapon: status %d, body %q", w.Code, w.Body.String())
	}
	if opts := weaponAttackOptionsFor(t, s, id, finesseInv); opts.AttackAbility != "int" {
		t.Fatalf("Finesse Enhanced Weapon AttackAbility = %q, want \"int\"", opts.AttackAbility)
	}

	if w := ninjaneerDesignationRequest(t, s, id, "science-nin-enhanced-weapon/delete", finesseInv); w.Code != http.StatusOK {
		t.Fatalf("clear Finesse weapon: status %d, body %q", w.Code, w.Body.String())
	}
	if opts := weaponAttackOptionsFor(t, s, id, finesseInv); opts.AttackAbility != "" {
		t.Fatalf("Finesse weapon AttackAbility after clearing designation = %q, want unset", opts.AttackAbility)
	}

	if w := ninjaneerDesignationRequest(t, s, id, "science-nin-enhanced-weapon", heavyInv); w.Code != http.StatusOK {
		t.Fatalf("designate non-Finesse weapon: status %d, body %q", w.Code, w.Body.String())
	}
	if opts := weaponAttackOptionsFor(t, s, id, heavyInv); opts.AttackAbility != "" {
		t.Fatalf("non-Finesse Enhanced Weapon AttackAbility = %q, want unset (Enhanced Arsenal's clause is Finesse-only)", opts.AttackAbility)
	}
}

// TestNinjaneerWeaponDesignationSingleSlotEnforced confirms a second
// designation is refused outright while one is already set — the same
// "clear the current pick first" boundary every other single-slot pick in
// this codebase draws (Mixed Studies, Ascended W.o.W, ...).
func TestNinjaneerWeaponDesignationSingleSlotEnforced(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 3)
	id := seedNinjaneerCharacter(t, s, "Test Ninjaneer", 3, 16)
	first := addInventoryWeapon(t, s, id, "weapon/test-finesse")
	second := addInventoryWeapon(t, s, id, "weapon/test-heavy")

	if w := ninjaneerDesignationRequest(t, s, id, "science-nin-enhanced-weapon", first); w.Code != http.StatusOK {
		t.Fatalf("first designation: status %d, body %q", w.Code, w.Body.String())
	}
	w := ninjaneerDesignationRequest(t, s, id, "science-nin-enhanced-weapon", second)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("second designation while one already set: status %d, body %q, want 400", w.Code, w.Body.String())
	}
}

// TestNinjaneerLegendaryWeaponRequiresWarriorOfScience confirms the
// designation route refuses to run below 9th level, before Warrior of
// Science is granted — the same feature-gate every other pick-add handler
// in this package enforces.
func TestNinjaneerLegendaryWeaponRequiresWarriorOfScience(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 3) // below 9th level: no Warrior of Science yet
	id := seedNinjaneerCharacter(t, s, "Test Ninjaneer", 3, 16)
	inv := addInventoryWeapon(t, s, id, "weapon/test-finesse")

	w := ninjaneerDesignationRequest(t, s, id, "science-nin-legendary-weapon", inv)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Legendary Weapon designation below 9th level: status %d, body %q, want 400", w.Code, w.Body.String())
	}
}

// TestNinjaneerLegendaryWeaponActiveToggle covers
// handleSheetNinjaneerLegendaryWeaponToggle's own round trip — same
// on/off-form shape TestSheetExoskeletonToggle would cover if one existed,
// mirrored here since Warrior of Science's own toggle follows that pattern
// exactly (see cmd/n5e/ninjaneer.go's own header doc).
func TestNinjaneerLegendaryWeaponActiveToggle(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 9)
	id := seedNinjaneerCharacter(t, s, "Test Ninjaneer", 9, 16)

	if active, err := charstore.NinjaneerLegendaryWeaponActive(s.charDB, id); err != nil {
		t.Fatal(err)
	} else if active {
		t.Fatalf("Legendary Weapon Active before any toggle = true, want false")
	}

	req := httptest.NewRequest(http.MethodPost,
		"/characters/"+strconv.FormatInt(id, 10)+"/sheet/science-nin-legendary-weapon-active",
		strings.NewReader(url.Values{"on": {"1"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("toggle on: status %d, body %q", w.Code, w.Body.String())
	}
	if active, err := charstore.NinjaneerLegendaryWeaponActive(s.charDB, id); err != nil {
		t.Fatal(err)
	} else if !active {
		t.Fatalf("Legendary Weapon Active after toggling on = false, want true")
	}
}

// TestNinjaneerFutureOfShinobiWeaponsDamageBonus covers the 20th-level
// capstone's own flat per-tier damage-roll bonus (+Proficiency Bonus on the
// Enhanced Weapon, +2x on the Legendary Weapon, +Intelligence Modifier on
// the Perfected Weapon), applied automatically the moment each designation
// is made, and cleared the moment a designation is removed — no stale bonus
// lingering on a weapon that's no longer designated.
func TestNinjaneerFutureOfShinobiWeaponsDamageBonus(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 20)
	id := seedNinjaneerCharacter(t, s, "Test Ninjaneer", 20, 16) // Intelligence modifier +3
	enhInv := addInventoryWeapon(t, s, id, "weapon/test-finesse")
	legInv := addInventoryWeapon(t, s, id, "weapon/test-heavy")
	perfInv := addInventoryWeapon(t, s, id, "weapon/test-finesse") // a second, distinct owned copy

	for _, r := range []struct {
		path string
		inv  int64
	}{
		{"science-nin-enhanced-weapon", enhInv},
		{"science-nin-legendary-weapon", legInv},
		{"science-nin-perfected-weapon-mark", perfInv},
	} {
		if w := ninjaneerDesignationRequest(t, s, id, r.path, r.inv); w.Code != http.StatusOK {
			t.Fatalf("designate via %s: status %d, body %q", r.path, w.Code, w.Body.String())
		}
	}

	if opts := weaponAttackOptionsFor(t, s, id, enhInv); opts.DamageBonus != 9 {
		t.Fatalf("Enhanced Weapon DamageBonus = %d, want 9 (Proficiency Bonus at level 20)", opts.DamageBonus)
	}
	if opts := weaponAttackOptionsFor(t, s, id, legInv); opts.DamageBonus != 18 {
		t.Fatalf("Legendary Weapon DamageBonus = %d, want 18 (2x Proficiency Bonus)", opts.DamageBonus)
	}
	if opts := weaponAttackOptionsFor(t, s, id, perfInv); opts.DamageBonus != 3 {
		t.Fatalf("Perfected Weapon DamageBonus = %d, want 3 (Intelligence Modifier)", opts.DamageBonus)
	}

	if w := ninjaneerDesignationRequest(t, s, id, "science-nin-enhanced-weapon/delete", enhInv); w.Code != http.StatusOK {
		t.Fatalf("clear Enhanced Weapon: status %d, body %q", w.Code, w.Body.String())
	}
	if opts := weaponAttackOptionsFor(t, s, id, enhInv); opts.DamageBonus != 0 {
		t.Fatalf("Enhanced Weapon DamageBonus after clearing its designation = %d, want 0", opts.DamageBonus)
	}
}

// TestNinjaneerFutureOfShinobiWeaponsDamageBonusAbsentBeforeCapstone
// confirms the flat damage bonus stays at zero for a designated weapon
// before The Future of Shinobi: Weapons is granted — only the Finesse/
// Intelligence attack-ability override is live before 20th level.
func TestNinjaneerFutureOfShinobiWeaponsDamageBonusAbsentBeforeCapstone(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 3)
	id := seedNinjaneerCharacter(t, s, "Test Ninjaneer", 3, 16)
	inv := addInventoryWeapon(t, s, id, "weapon/test-finesse")

	if w := ninjaneerDesignationRequest(t, s, id, "science-nin-enhanced-weapon", inv); w.Code != http.StatusOK {
		t.Fatalf("designate Enhanced Weapon: status %d, body %q", w.Code, w.Body.String())
	}
	if opts := weaponAttackOptionsFor(t, s, id, inv); opts.DamageBonus != 0 {
		t.Fatalf("Enhanced Weapon DamageBonus before the 20th-level capstone = %d, want 0", opts.DamageBonus)
	}
}

// TestNinjaneerWeaponDesignationTemplateRendersTooltip renders the Ninjaneer
// subclass tracker popup end to end and confirms the picker markup is
// present, including the candidate weapon's own rollover-tooltip
// description text — catching both "the picker never got added to the
// template" and "the tooltip content field was left empty" regressions, the
// latter a real bug found live (loadNinjaneerCandidateWeapons never
// populated ninjaneerWeaponOption.Description before this test existed).
// Also confirms the Core sheet itself only leaves a pointer to the popup,
// not a duplicate inline picker (science_nin_ninjaneer_popup.go/
// character_science_nin_ninjaneer.html moved this off the Core sheet).
func TestNinjaneerWeaponDesignationTemplateRendersTooltip(t *testing.T) {
	s := testServer(t)
	seedNinjaneerCatalog(t, s, 20)
	id := seedNinjaneerCharacter(t, s, "Test Ninjaneer", 20, 16)
	addInventoryWeapon(t, s, id, "weapon/test-finesse")

	req := httptest.NewRequest(http.MethodGet, "/characters/"+strconv.FormatInt(id, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	s.handleCharacterSheet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("render sheet: status %d, body %q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "Designate Enhanced Weapon") {
		t.Errorf("Core sheet still has the inline Enhanced Weapon designation picker — should only point to the Ninjaneer popup")
	}
	if !strings.Contains(body, `tracked in the "Ninjaneer" popup`) {
		t.Errorf("Core sheet missing its pointer to the Ninjaneer popup")
	}
	if !strings.Contains(body, `href="/characters/`+strconv.FormatInt(id, 10)+`/science-nin/ninjaneer"`) {
		t.Errorf("sidebar missing the Ninjaneer popup button")
	}

	popup := getPopup(t, s, ninjaneerPopupPath(id))
	if popup.Code != http.StatusOK {
		t.Fatalf("render ninjaneer popup: status %d, body %q", popup.Code, popup.Body.String())
	}
	popupBody := popup.Body.String()
	if !strings.Contains(popupBody, "Designate Enhanced Weapon") {
		t.Errorf("popup missing the Enhanced Weapon designation picker")
	}
	if !strings.Contains(popupBody, "Warrior of Science") {
		t.Errorf("popup missing the Warrior of Science section")
	}
	if !strings.Contains(popupBody, "A Weapon to Surpass: Perfected Weapon") {
		t.Errorf("popup missing the Perfected Weapon designation section")
	}
	if !strings.Contains(popupBody, "lightweight test blade") {
		t.Errorf("popup missing the candidate weapon's own tooltip description text")
	}
}
