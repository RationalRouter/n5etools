package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
)

// Test catalog slugs reused across this file's Creation Points sharing
// tests — two cheap, otherwise-identical Scientific Ninja Tools entries (so
// a test can spend a known amount and then attempt a second purchase
// against a deliberately tight remaining budget), one cheap Titan Upgrade,
// and one cheap S.N.B Upgrade, all costing 6 Creation Points.
const (
	cpTestSNTAlphaSlug     = "class/science-nin/option/scientific-ninja-tools/minor/entry/cp-test-tool-alpha"
	cpTestSNTBetaSlug      = "class/science-nin/option/scientific-ninja-tools/minor/entry/cp-test-tool-beta"
	cpTestTitanUpgradeSlug = "class/science-nin/option/titan-upgrades/minor/entry/cp-test-titan-plating"
	cpTestSNBUpgradeSlug   = "class/science-nin/option/s-n-b-upgrades/minor/entry/cp-test-beast-plating"
)

// seedScienceNinCPRules seeds the Science-Nin class, a class_level_resources
// "Creation Points" row at level, both the Mech Crafter (Ordnance
// Training/Titan Upgrades) and S.N.B Specialist (S.N.B Upgrades) subclass
// chains, and one cheap catalog entry each for Scientific Ninja Tools (two,
// see above), Titan Upgrades, and S.N.B Upgrades — enough for a test to
// spend real Creation Points from any of the three consumers that draw from
// the same shared budget (loadScienceNinTabData's own CreationPointsCap/
// Used).
func seedScienceNinCPRules(t *testing.T, s *server, level, creationPointsCap int) {
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
		('class/science-nin', ?, 'Creation Points', ?)`, level, strconv.Itoa(creationPointsCap))
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/science-nin/group/scientific-inquiry', 'class/science-nin', 'Scientific Inquiry')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/science-nin/group/scientific-inquiry/mech-crafter', 'class/science-nin/group/scientific-inquiry', 'Mech Crafter'),
		('class/science-nin/group/scientific-inquiry/s-n-b-specialist', 'class/science-nin/group/scientific-inquiry', 'S.N.B Specialist')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/mech-crafter', 'Ordnance Training', 3,
		 'Craft a Titan with a number of Titan Slots equal to your Proficiency Bonus.', 1)`,
		scienceNinOrdnanceTrainingFeatureSlug)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		(?, 'class/science-nin/group/scientific-inquiry/s-n-b-specialist', 'S.N.B Upgrades', 3,
		 'Your Scientific Ninja Beast has a number of Upgrade Slots equal to your Proficiency Bonus.', 1)`,
		scienceNinSNBUpgradesFeatureSlug)

	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/scientific-ninja-tools/minor', 'class/science-nin', NULL, 'Scientific Ninja Tools', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/scientific-ninja-tools/minor', 'CP Test Tool Alpha', 'Cost: 6 Creation Points A cheap test tool.', 1),
		(?, 'class/science-nin/option/scientific-ninja-tools/minor', 'CP Test Tool Beta', 'Cost: 6 Creation Points Another cheap test tool.', 2)`,
		cpTestSNTAlphaSlug, cpTestSNTBetaSlug)

	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/titan-upgrades/minor', 'class/science-nin', NULL, 'Titan Upgrades', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/titan-upgrades/minor', 'CP Test Titan Plating', 'Cost: 6 Creation Points Keyword: Mech A sturdy test plate.', 1)`,
		cpTestTitanUpgradeSlug)

	mustExecRules(`INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, sort_order) VALUES
		('class/science-nin/option/s-n-b-upgrades/minor', 'class/science-nin', NULL, 'S.N.B Upgrades', 'Minor', '', 1)`)
	mustExecRules(`INSERT INTO class_option_entries (slug, class_option_slug, name, description, sort_order) VALUES
		(?, 'class/science-nin/option/s-n-b-upgrades/minor', 'CP Test Beast Plating', 'Cost: 6 Creation Points A sturdy test upgrade.', 1)`,
		cpTestSNBUpgradeSlug)
}

// seedScienceNinCPCharacter inserts a character with the given Science-Nin
// level, subclassed into subclassSlug at 3rd level ("" for no subclass —
// used by the feat-bonus tests below, which apply regardless of subclass).
func seedScienceNinCPCharacter(t *testing.T, s *server, name string, level int, subclassSlug string) int64 {
	t.Helper()
	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES (?, 10, 10, 10, 14, 10, 10)`, name)
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
	if subclassSlug != "" {
		if _, err := s.charDB.Exec(
			`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, ?, 3)`,
			id, subclassSlug,
		); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func loadScienceNinCPData(t *testing.T, s *server, characterID int64) *scienceNinToolsTabData {
	t.Helper()
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadScienceNinTabData(characterID, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("loadScienceNinTabData: want non-nil data")
	}
	return data
}

func loadTitanCPData(t *testing.T, s *server, characterID int64) *titanUpgradesData {
	t.Helper()
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, characterID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadTitanUpgradesData(characterID, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("loadTitanUpgradesData: want non-nil data")
	}
	return data
}

// scienceNinToolAddRequest/scienceNinToolDeleteRequest, titanUpgradeAddRequest/
// titanUpgradeDeleteRequest, and snbUpgradeAddRequest/snbUpgradeDeleteRequest
// build requests against the real, server.go-registered routes and run them
// through s.routes() end to end — the same dispatch a live browser POST goes
// through, matching ever_evolving_test.go/science_nin_bim_test.go's own
// request-helper shape rather than calling the handlers or the underlying
// charstore functions directly.
func scienceNinToolAddRequest(t *testing.T, s *server, characterID int64, slug string) *httptest.ResponseRecorder {
	t.Helper()
	return postSheetForm(t, s, characterID, "/sheet/science-nin-tools", url.Values{"option_slug": {slug}})
}

func scienceNinToolDeleteRequest(t *testing.T, s *server, characterID int64, slug string) *httptest.ResponseRecorder {
	t.Helper()
	return postSheetForm(t, s, characterID, "/sheet/science-nin-tools/delete", url.Values{"option_slug": {slug}})
}

func titanUpgradeAddRequest(t *testing.T, s *server, characterID int64, slug string) *httptest.ResponseRecorder {
	t.Helper()
	return postSheetForm(t, s, characterID, "/sheet/titan-upgrade", url.Values{"option_slug": {slug}})
}

func titanUpgradeDeleteRequest(t *testing.T, s *server, characterID int64, slug string) *httptest.ResponseRecorder {
	t.Helper()
	return postSheetForm(t, s, characterID, "/sheet/titan-upgrade/delete", url.Values{"option_slug": {slug}})
}

func snbUpgradeAddRequest(t *testing.T, s *server, characterID int64, slug string) *httptest.ResponseRecorder {
	t.Helper()
	return postSheetForm(t, s, characterID, "/sheet/science-nin-snb-upgrade", url.Values{"option_slug": {slug}})
}

func snbUpgradeDeleteRequest(t *testing.T, s *server, characterID int64, slug string) *httptest.ResponseRecorder {
	t.Helper()
	return postSheetForm(t, s, characterID, "/sheet/science-nin-snb-upgrade/delete", url.Values{"option_slug": {slug}})
}

func postSheetForm(t *testing.T, s *server, characterID int64, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/characters/"+strconv.FormatInt(characterID, 10)+path,
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	return w
}

// TestScienceNinCreationPointsFeatBonuses covers
// scienceNinCreationPointsFeatBonus: each of Secondary Storage Device
// ("+8 creation Points"), Scientist Training/Expert/Specialist ("+10
// [more] Creation Points" each) must add its own flat bonus onto
// CreationPointsCap, and multiple held at once must sum rather than only
// the first match counting — before this map existed, every one of these
// feats' own text silently never reached CreationPointsCap at all (Bug 1),
// with only Future of Shinobi: Shinobi-Ware's doubling ever touching the
// cap.
func TestScienceNinCreationPointsFeatBonuses(t *testing.T) {
	cases := []struct {
		name      string
		feats     []string
		wantExtra int
	}{
		{"no feats", nil, 0},
		{"Secondary Storage Device", []string{"feat/class/secondary-storage-device"}, 8},
		{"Scientist Training", []string{"feat/class/scientist-training"}, 10},
		{"Scientist Expert", []string{"feat/class/scientist-expert"}, 10},
		{"Scientist Specialist", []string{"feat/class/scientist-specialist"}, 10},
		{"Secondary Storage Device stacks with Scientist Training",
			[]string{"feat/class/secondary-storage-device", "feat/class/scientist-training"}, 18},
		{"all four stack",
			[]string{
				"feat/class/secondary-storage-device",
				"feat/class/scientist-training",
				"feat/class/scientist-expert",
				"feat/class/scientist-specialist",
			}, 38},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			seedScienceNinCPRules(t, s, 6, 20)
			id := seedScienceNinCPCharacter(t, s, "Kabuto", 6, "")
			for _, slug := range c.feats {
				if _, err := s.charDB.Exec(
					`INSERT INTO character_feats (character_id, feat_slug) VALUES (?, ?)`, id, slug,
				); err != nil {
					t.Fatal(err)
				}
			}

			data := loadScienceNinCPData(t, s, id)
			want := 20 + c.wantExtra
			if data.CreationPointsCap != want {
				t.Errorf("CreationPointsCap = %d, want %d (base 20 + %d feat bonus)", data.CreationPointsCap, want, c.wantExtra)
			}
		})
	}
}

// TestScienceNinToolAddReflectsSecondaryStorageDeviceBonus covers Bug 1
// end to end through the real handler (not just the internal data
// function): a purchase that the base Creation Points budget cannot afford
// must be rejected, and must succeed once Secondary Storage Device's own
// "+8 creation Points" is granted — pinning that both the sheet's own
// CreationPointsCap display and handleScienceNinToolAdd's server-side
// budget check see the same, correct, feat-inclusive total.
func TestScienceNinToolAddReflectsSecondaryStorageDeviceBonus(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 6) // cap exactly fits one 6-cost tool
	id := seedScienceNinCPCharacter(t, s, "Kabuto", 6, "")

	if w := scienceNinToolAddRequest(t, s, id, cpTestSNTAlphaSlug); w.Code != http.StatusOK {
		t.Fatalf("buy first tool (within base 6-CP budget): status %d, body %q", w.Code, w.Body.String())
	}
	if w := scienceNinToolAddRequest(t, s, id, cpTestSNTBetaSlug); w.Code != http.StatusBadRequest {
		t.Fatalf("buy second tool (over base budget): status %d, body %q, want 400", w.Code, w.Body.String())
	}

	if _, err := s.charDB.Exec(
		`INSERT INTO character_feats (character_id, feat_slug) VALUES (?, 'feat/class/secondary-storage-device')`, id,
	); err != nil {
		t.Fatal(err)
	}

	data := loadScienceNinCPData(t, s, id)
	if data.CreationPointsCap != 14 {
		t.Fatalf("CreationPointsCap after Secondary Storage Device = %d, want 14 (6 base + 8 feat)", data.CreationPointsCap)
	}
	if data.CreationPointsUsed != 6 {
		t.Fatalf("CreationPointsUsed = %d, want 6 (only the first tool bought so far)", data.CreationPointsUsed)
	}

	if w := scienceNinToolAddRequest(t, s, id, cpTestSNTBetaSlug); w.Code != http.StatusOK {
		t.Fatalf("buy second tool with Secondary Storage Device's +8 CP: status %d, body %q, want 200", w.Code, w.Body.String())
	}
	data = loadScienceNinCPData(t, s, id)
	if data.CreationPointsUsed != 12 {
		t.Errorf("CreationPointsUsed after both tools = %d, want 12", data.CreationPointsUsed)
	}
}

// TestMechCrafterCreationPointsSharedBudget covers the asymmetry fix: a
// Mech Crafter's Scientific Ninja Tools spend and Titan Upgrade spend draw
// from the SAME Creation Points budget, enforced symmetrically from BOTH
// sides. Before titanUpgradesCreationPointsSpend was folded into
// loadScienceNinTabData's own CreationPointsUsed, handleScienceNinToolAdd's
// budget check never saw Titan spend at all, so a character could spend the
// full cap on SNTs and, independently, the full cap again on Titan
// Upgrades.
func TestMechCrafterCreationPointsSharedBudget(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 10)
	id := seedScienceNinCPCharacter(t, s, "Kankuro", 6, "class/science-nin/group/scientific-inquiry/mech-crafter")

	if w := scienceNinToolAddRequest(t, s, id, cpTestSNTAlphaSlug); w.Code != http.StatusOK {
		t.Fatalf("buy SNT tool (6/10): status %d, body %q", w.Code, w.Body.String())
	}

	// A 6-cost Titan Upgrade would push combined spend to 12, over the
	// shared 10-CP cap.
	if w := titanUpgradeAddRequest(t, s, id, cpTestTitanUpgradeSlug); w.Code != http.StatusBadRequest {
		t.Fatalf("buy Titan Upgrade over the shared budget (SNT already spent 6/10): status %d, body %q, want 400", w.Code, w.Body.String())
	}
	titanData := loadTitanCPData(t, s, id)
	if len(titanData.KnownUpgrades) != 0 {
		t.Fatalf("KnownUpgrades after rejected over-budget add = %+v, want none", titanData.KnownUpgrades)
	}
	if titanData.CreationPointsUsed != 6 {
		t.Fatalf("titanData.CreationPointsUsed = %d, want 6 (matches the SNT-side spend)", titanData.CreationPointsUsed)
	}

	// Free the SNT tool's 6 CP back up; the identical Titan Upgrade
	// purchase must now succeed.
	if w := scienceNinToolDeleteRequest(t, s, id, cpTestSNTAlphaSlug); w.Code != http.StatusOK {
		t.Fatalf("delete SNT tool: status %d, body %q", w.Code, w.Body.String())
	}
	if w := titanUpgradeAddRequest(t, s, id, cpTestTitanUpgradeSlug); w.Code != http.StatusOK {
		t.Fatalf("buy Titan Upgrade after freeing budget: status %d, body %q, want 200", w.Code, w.Body.String())
	}

	// Reverse direction: with the Titan Upgrade now installed (6/10
	// spent), a second SNT tool that would push combined spend to 12 must
	// also be rejected.
	if w := scienceNinToolAddRequest(t, s, id, cpTestSNTBetaSlug); w.Code != http.StatusBadRequest {
		t.Fatalf("buy SNT tool over the shared budget (Titan Upgrade already spent 6/10): status %d, body %q, want 400", w.Code, w.Body.String())
	}
	sntData := loadScienceNinCPData(t, s, id)
	if len(sntData.KnownTools) != 0 {
		t.Fatalf("KnownTools after rejected over-budget add = %+v, want none", sntData.KnownTools)
	}
	if sntData.CreationPointsUsed != 6 {
		t.Fatalf("sntData.CreationPointsUsed = %d, want 6 (matches the Titan-side spend)", sntData.CreationPointsUsed)
	}

	// Deleting the Titan Upgrade must also free the shared budget back up.
	if w := titanUpgradeDeleteRequest(t, s, id, cpTestTitanUpgradeSlug); w.Code != http.StatusOK {
		t.Fatalf("delete Titan Upgrade: status %d, body %q", w.Code, w.Body.String())
	}
	sntData = loadScienceNinCPData(t, s, id)
	if sntData.CreationPointsUsed != 0 {
		t.Fatalf("CreationPointsUsed after deleting the Titan Upgrade = %d, want 0", sntData.CreationPointsUsed)
	}
}

// TestSNBSpecialistCreationPointsSharedBudget covers the same shared-pool
// symmetry as TestMechCrafterCreationPointsSharedBudget, generalized to
// S.N.B Specialist's own S.N.B Upgrades — a gap the investigation flagged
// as confirmed missing (loadScienceNinSubclassData's S.N.B Upgrades block
// previously never added its own Cost into data.CreationPointsUsed at
// all), not merely a copy of the Mech Crafter fix.
func TestSNBSpecialistCreationPointsSharedBudget(t *testing.T) {
	s := testServer(t)
	seedScienceNinCPRules(t, s, 6, 10)
	id := seedScienceNinCPCharacter(t, s, "Shino", 6, "class/science-nin/group/scientific-inquiry/s-n-b-specialist")

	if w := scienceNinToolAddRequest(t, s, id, cpTestSNTAlphaSlug); w.Code != http.StatusOK {
		t.Fatalf("buy SNT tool (6/10): status %d, body %q", w.Code, w.Body.String())
	}

	if w := snbUpgradeAddRequest(t, s, id, cpTestSNBUpgradeSlug); w.Code != http.StatusBadRequest {
		t.Fatalf("buy S.N.B Upgrade over the shared budget (SNT already spent 6/10): status %d, body %q, want 400", w.Code, w.Body.String())
	}
	data := loadScienceNinCPData(t, s, id)
	if data.SNBSpecialist == nil {
		t.Fatal("loadScienceNinTabData: want a rendered S.N.B Specialist box")
	}
	if data.SNBSpecialist.UpgradeUsed != 0 {
		t.Fatalf("SNBSpecialist.UpgradeUsed after rejected over-budget add = %d, want 0", data.SNBSpecialist.UpgradeUsed)
	}
	if data.CreationPointsUsed != 6 {
		t.Fatalf("CreationPointsUsed after rejected S.N.B Upgrade add = %d, want 6 (unchanged)", data.CreationPointsUsed)
	}

	if w := scienceNinToolDeleteRequest(t, s, id, cpTestSNTAlphaSlug); w.Code != http.StatusOK {
		t.Fatalf("delete SNT tool: status %d, body %q", w.Code, w.Body.String())
	}
	if w := snbUpgradeAddRequest(t, s, id, cpTestSNBUpgradeSlug); w.Code != http.StatusOK {
		t.Fatalf("buy S.N.B Upgrade after freeing budget: status %d, body %q, want 200", w.Code, w.Body.String())
	}

	// Reverse direction: with the S.N.B Upgrade now installed (6/10
	// spent), a second SNT tool that would push combined spend to 12 must
	// also be rejected.
	if w := scienceNinToolAddRequest(t, s, id, cpTestSNTBetaSlug); w.Code != http.StatusBadRequest {
		t.Fatalf("buy SNT tool over the shared budget (S.N.B Upgrade already spent 6/10): status %d, body %q, want 400", w.Code, w.Body.String())
	}
	data = loadScienceNinCPData(t, s, id)
	if len(data.KnownTools) != 0 {
		t.Fatalf("KnownTools after rejected over-budget add = %+v, want none", data.KnownTools)
	}
	if data.CreationPointsUsed != 6 {
		t.Fatalf("CreationPointsUsed = %d, want 6 (matches the S.N.B Upgrade-side spend)", data.CreationPointsUsed)
	}

	// Deleting the S.N.B Upgrade must also free the budget back up, same
	// "delete frees the shared pool" behavior every other consumer gets.
	if w := snbUpgradeDeleteRequest(t, s, id, cpTestSNBUpgradeSlug); w.Code != http.StatusOK {
		t.Fatalf("delete S.N.B Upgrade: status %d, body %q", w.Code, w.Body.String())
	}
	data = loadScienceNinCPData(t, s, id)
	if data.CreationPointsUsed != 0 {
		t.Fatalf("CreationPointsUsed after deleting the S.N.B Upgrade = %d, want 0", data.CreationPointsUsed)
	}
}
