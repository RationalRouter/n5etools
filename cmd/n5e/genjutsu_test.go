package main

import (
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

func TestGenjutsuInceptionCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {2, 0}, {3, 1}, {20, 1},
	}
	for _, c := range cases {
		if got := genjutsuInceptionCap(c.level); got != c.want {
			t.Errorf("genjutsuInceptionCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestRealWorldConversionCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {4, 0}, {5, 1}, {8, 1}, {9, 2}, {14, 2}, {15, 3}, {20, 3},
	}
	for _, c := range cases {
		if got := realWorldConversionCap(c.level); got != c.want {
			t.Errorf("realWorldConversionCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestMasterOfIllusionCap(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 0}, {12, 0}, {13, 1}, {19, 1}, {20, 2},
	}
	for _, c := range cases {
		if got := masterOfIllusionCap(c.level); got != c.want {
			t.Errorf("masterOfIllusionCap(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestActualizationDieSize(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{1, "d4"}, {8, "d4"}, {9, "d6"}, {16, "d6"}, {17, "d8"}, {20, "d8"},
	}
	for _, c := range cases {
		if got := actualizationDieSize(c.level); got != c.want {
			t.Errorf("actualizationDieSize(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestViciousMockeryDice(t *testing.T) {
	cases := []struct {
		level       int
		wantDamage  string
		wantPenalty string
	}{
		{2, "2d4", "1d4"}, {6, "2d4", "1d4"}, {7, "4d4", "2d4"}, {12, "4d4", "2d4"},
		{13, "6d4", "3d4"}, {15, "6d4", "3d4"}, {16, "8d4", "4d4"}, {20, "8d4", "4d4"},
	}
	for _, c := range cases {
		damage, penalty := viciousMockeryDice(c.level)
		if damage != c.wantDamage || penalty != c.wantPenalty {
			t.Errorf("viciousMockeryDice(%d) = (%q, %q), want (%q, %q)", c.level, damage, penalty, c.wantDamage, c.wantPenalty)
		}
	}
}

func TestVisceralLanguageDie(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{2, "1d6"}, {5, "1d6"}, {6, "2d6"}, {9, "2d6"}, {10, "3d6"}, {13, "3d6"}, {14, "4d6"}, {17, "4d6"}, {18, "5d6"}, {20, "5d6"},
	}
	for _, c := range cases {
		if got := visceralLanguageDie(c.level); got != c.want {
			t.Errorf("visceralLanguageDie(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

// seedGenjutsuLevelResources inserts Malleable Mirages' own known-cap chart
// (the real values at 2nd and 20th level, confirmed against the shipped
// rules.db) plus a minimal Genjutsu Pledge subclass pair (Corrupt Thoughts,
// Siren — for the readout-gating test) and a small Inception/Mirage/
// Conversion catalog exercising the auto-grant and prerequisite paths:
// Reality Marble (Inception) auto-grants Persistent Genjutsu (Mirage,
// prereq "Reality Marble") at 7th level; Mental Placebo shares that same
// prereq; Fast Forward Fate needs BOTH Temporal Stopwatch (a different,
// unpicked Inception) AND 10th level — the real combined-clause shape; Basic
// Mirage carries no prerequisite at all.
func seedGenjutsuLevelResources(t *testing.T, s *server) {
	t.Helper()
	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES
		('class/genjutsu-specialist', 'Genjutsu Specialist', 8, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO class_levels (class_slug, level) VALUES
		('class/genjutsu-specialist', 2), ('class/genjutsu-specialist', 20)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_level_resources (class_slug, level, resource_name, value) VALUES
		('class/genjutsu-specialist', 2, 'Malleable Mirages', '2'),
		('class/genjutsu-specialist', 20, 'Malleable Mirages', '11')`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/genjutsu-specialist/group/genjutsu-pledges', 'class/genjutsu-specialist', 'Genjutsu Pledges')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/genjutsu-specialist/group/genjutsu-pledges/corrupt-thoughts', 'class/genjutsu-specialist/group/genjutsu-pledges', 'Corrupt Thoughts'),
		('class/genjutsu-specialist/group/genjutsu-pledges/siren', 'class/genjutsu-specialist/group/genjutsu-pledges', 'Siren')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_options (slug, class_slug, subclass_slug, list_name, name, description, prerequisites, sort_order) VALUES
		('class/genjutsu-specialist/option/genjutsu-inception/reality-marble', 'class/genjutsu-specialist', NULL,
		 'Genjutsu Inception', 'Reality Marble', 'Warp the battlefield.', NULL, 1),
		('class/genjutsu-specialist/option/genjutsu-inception/temporal-stopwatch', 'class/genjutsu-specialist', NULL,
		 'Genjutsu Inception', 'Temporal Stopwatch', 'Bend time.', NULL, 2),
		('class/genjutsu-specialist/option/malleable-mirages/persistent-genjutsu', 'class/genjutsu-specialist', NULL,
		 'Malleable Mirages', 'Persistent Genjutsu', 'Your Genjutsu lingers.', 'Reality Marble', 1),
		('class/genjutsu-specialist/option/malleable-mirages/mental-placebo', 'class/genjutsu-specialist', NULL,
		 'Malleable Mirages', 'Mental Placebo', 'A soothing fake.', 'Reality Marble', 2),
		('class/genjutsu-specialist/option/malleable-mirages/fast-forward-fate', 'class/genjutsu-specialist', NULL,
		 'Malleable Mirages', 'Fast Forward Fate', 'Skip ahead.', 'Temporal Stopwatch, 10th level', 3),
		('class/genjutsu-specialist/option/malleable-mirages/basic-mirage', 'class/genjutsu-specialist', NULL,
		 'Malleable Mirages', 'Basic Mirage', 'No strings attached.', NULL, 4)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(`
		INSERT INTO class_features (slug, class_slug, name, description) VALUES
		('class/genjutsu-specialist/feature/actualized-power', 'class/genjutsu-specialist', 'Actualized Power', 'Hit harder.'),
		('class/genjutsu-specialist/feature/actualized-perception', 'class/genjutsu-specialist', 'Actualized Perception', 'See more.')`,
	); err != nil {
		t.Fatal(err)
	}
}

func TestMalleableMiragesCap(t *testing.T) {
	s := testServer(t)
	seedGenjutsuLevelResources(t, s)

	if got, err := s.malleableMiragesCap(2); err != nil || got != 2 {
		t.Errorf("malleableMiragesCap(2) = %d, %v, want 2, nil", got, err)
	}
	if got, err := s.malleableMiragesCap(20); err != nil || got != 11 {
		t.Errorf("malleableMiragesCap(20) = %d, %v, want 11, nil", got, err)
	}
}

func TestLoadGenjutsuTabData(t *testing.T) {
	s := testServer(t)
	seedGenjutsuLevelResources(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Sakura', 8, 12, 12, 10, 15, 16)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/genjutsu-specialist', 20, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/genjutsu-specialist/group/genjutsu-pledges/corrupt-thoughts', 2)`); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	data, err := s.loadGenjutsuTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("loadGenjutsuTabData returned nil for a real Genjutsu Specialist")
	}
	if data.ActualizationDieSize != "d8" {
		t.Errorf("ActualizationDieSize = %q, want d8 (level 20)", data.ActualizationDieSize)
	}
	if data.ViciousMockeryDamageDie != "8d4" || data.ViciousMockeryPenaltyDie != "4d4" {
		t.Errorf("Vicious Mockery = %q/%q, want 8d4/4d4 (Corrupt Thoughts, level 20)", data.ViciousMockeryDamageDie, data.ViciousMockeryPenaltyDie)
	}
	if data.VisceralLanguageDie != "" {
		t.Errorf("VisceralLanguageDie = %q, want blank — this character is Corrupt Thoughts, not Siren", data.VisceralLanguageDie)
	}
	if data.MiragesCap != 11 {
		t.Errorf("MiragesCap = %d, want 11 (level 20)", data.MiragesCap)
	}
	if data.InceptionCap != 1 {
		t.Errorf("InceptionCap = %d, want 1 (level 20)", data.InceptionCap)
	}
	if data.ConversionCap != 3 {
		t.Errorf("ConversionCap = %d, want 3 (level 20)", data.ConversionCap)
	}
	if data.IllusionMasteryCap != 2 {
		t.Errorf("IllusionMasteryCap = %d, want 2 (level 20)", data.IllusionMasteryCap)
	}
	if len(data.AvailableConversions) != 2 {
		t.Errorf("AvailableConversions = %+v, want both seeded Conversions before any pick", data.AvailableConversions)
	}
	if len(data.AvailableIllusionMastery) != len(masterOfIllusionOptions) {
		t.Errorf("AvailableIllusionMastery = %+v, want all %d hand-curated options before any pick", data.AvailableIllusionMastery, len(masterOfIllusionOptions))
	}

	// Before any Inception is picked: Persistent Genjutsu and Mental Placebo
	// (both prereq "Reality Marble") must be blocked; Fast Forward Fate
	// (prereq "Temporal Stopwatch, 10th level") must also be blocked; Basic
	// Mirage (no prereq) must be available.
	availableNames := map[string]bool{}
	for _, o := range data.AvailableMirages {
		availableNames[o.Name] = true
	}
	if availableNames["Persistent Genjutsu"] || availableNames["Mental Placebo"] || availableNames["Fast Forward Fate"] {
		t.Errorf("AvailableMirages = %+v, want Reality-Marble/Temporal-Stopwatch-gated Mirages blocked before any Inception pick", data.AvailableMirages)
	}
	if !availableNames["Basic Mirage"] {
		t.Errorf("AvailableMirages = %+v, want Basic Mirage (no prerequisite) always available", data.AvailableMirages)
	}

	// Pick Reality Marble as this character's Genjutsu Inception.
	if err := charstore.AddGenjutsuPick(s.charDB, 1, charstore.GenjutsuPickInception, "class/genjutsu-specialist/option/genjutsu-inception/reality-marble"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadGenjutsuTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}

	// At level 20 (>= 7), Persistent Genjutsu is auto-granted for free.
	if len(data.KnownMirages) != 1 || data.KnownMirages[0].Name != "Persistent Genjutsu" || !data.KnownMirages[0].Granted {
		t.Fatalf("KnownMirages = %+v, want exactly Persistent Genjutsu, Granted", data.KnownMirages)
	}
	if data.MiragesUsed != 0 {
		t.Errorf("MiragesUsed = %d, want 0 — the free grant must not count against the cap", data.MiragesUsed)
	}
	availableNames = map[string]bool{}
	for _, o := range data.AvailableMirages {
		availableNames[o.Name] = true
	}
	if availableNames["Persistent Genjutsu"] {
		t.Error("Persistent Genjutsu should not appear in AvailableMirages once auto-granted")
	}
	if !availableNames["Mental Placebo"] {
		t.Error("Mental Placebo (prereq Reality Marble, now known) should be available")
	}
	if availableNames["Fast Forward Fate"] {
		t.Error("Fast Forward Fate (prereq also needs Temporal Stopwatch, never picked) should still be blocked")
	}

	// Manually learn Mental Placebo and confirm the known/available split
	// updates, same shape hunter_nin_test.go's own pattern-pick check uses.
	if err := charstore.AddGenjutsuPick(s.charDB, 1, charstore.GenjutsuPickMirage, "class/genjutsu-specialist/option/malleable-mirages/mental-placebo"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadGenjutsuTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.MiragesUsed != 1 {
		t.Errorf("MiragesUsed = %d, want 1 after learning Mental Placebo", data.MiragesUsed)
	}
	foundKnown := false
	for _, k := range data.KnownMirages {
		if k.Name == "Mental Placebo" && !k.Granted {
			foundKnown = true
		}
	}
	if !foundKnown {
		t.Errorf("KnownMirages = %+v, want a non-Granted Mental Placebo entry", data.KnownMirages)
	}
}

// TestLoadGenjutsuTabDataArchetypeFeats covers Illusionist Training/Expert/
// Specialist letting a character with zero real Genjutsu Specialist levels
// see and use a small slice of the class's own mechanics. No feats rows
// exist in the seeded rules.db — loadCharacterFeats degrades to using the
// slug alone (feat.Name = slug) when a feats-table row is missing, which is
// all archFeats.* keys off, so that's not seeded here.
func TestLoadGenjutsuTabDataArchetypeFeats(t *testing.T) {
	s := testServer(t)
	seedGenjutsuLevelResources(t, s)

	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Konohamaru', 8, 12, 12, 10, 15, 16)`); err != nil {
		t.Fatal(err)
	}
	addFeat := func(slug string) {
		t.Helper()
		if _, err := s.charDB.Exec(
			`INSERT INTO character_feats (character_id, feat_slug) VALUES (1, ?)`, slug,
		); err != nil {
			t.Fatal(err)
		}
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}

	// No class levels, no feats: the whole tab must stay nil, same as any
	// other non-Genjutsu-Specialist character.
	if data, err := s.loadGenjutsuTabData(1, sheet); err != nil || data != nil {
		t.Fatalf("loadGenjutsuTabData with no levels/feats = %+v, %v, want nil, nil", data, err)
	}

	// Illusionist Training alone: 1 Mirage slot (qualifying as if 2nd
	// level — Reality-Marble/Temporal-Stopwatch-gated Mirages stay blocked,
	// Basic Mirage is open), no Inception, 1 Conversion slot, d4 die.
	addFeat(illusionistTrainingFeatSlug)
	data, err := s.loadGenjutsuTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("loadGenjutsuTabData returned nil for a character holding Illusionist Training")
	}
	if data.ActualizationDieSize != "d4" {
		t.Errorf("ActualizationDieSize = %q, want d4 (Illusionist Training, no real levels)", data.ActualizationDieSize)
	}
	if data.MiragesCap != 1 {
		t.Errorf("MiragesCap = %d, want 1 (Illusionist Training's own single Mirage slot)", data.MiragesCap)
	}
	if data.InceptionCap != 0 {
		t.Errorf("InceptionCap = %d, want 0 — Illusionist Training grants no Inception access", data.InceptionCap)
	}
	if data.ConversionCap != 1 {
		t.Errorf("ConversionCap = %d, want 1 (Illusionist Training's own single Conversion slot)", data.ConversionCap)
	}
	availableNames := map[string]bool{}
	for _, o := range data.AvailableMirages {
		availableNames[o.Name] = true
	}
	if !availableNames["Basic Mirage"] {
		t.Errorf("AvailableMirages = %+v, want Basic Mirage (no prerequisite) available", data.AvailableMirages)
	}
	if availableNames["Persistent Genjutsu"] || availableNames["Mental Placebo"] || availableNames["Fast Forward Fate"] {
		t.Errorf("AvailableMirages = %+v, want Inception/level-gated Mirages still blocked", data.AvailableMirages)
	}

	// Add Illusionist Expert: a second Mirage slot, a second Conversion
	// slot, still no Inception (Expert never grants one).
	addFeat(illusionistExpertFeatSlug)
	data, err = s.loadGenjutsuTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.MiragesCap != 2 {
		t.Errorf("MiragesCap = %d, want 2 (Training + Expert)", data.MiragesCap)
	}
	if data.ConversionCap != 2 {
		t.Errorf("ConversionCap = %d, want 2 (Training + Expert)", data.ConversionCap)
	}
	if data.InceptionCap != 0 {
		t.Errorf("InceptionCap = %d, want 0 — Illusionist Expert grants no Inception access either", data.InceptionCap)
	}

	// Add Illusionist Specialist: a third Mirage slot (qualifying as if
	// 5th level now), ConversionCap unchanged (Specialist's third benefit
	// is an Inception pick, not a second Conversion), and exactly 1
	// Inception slot, qualifying as if 9th level.
	addFeat(illusionistSpecialistFeatSlug)
	data, err = s.loadGenjutsuTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if data.MiragesCap != 3 {
		t.Errorf("MiragesCap = %d, want 3 (Training + Expert + Specialist)", data.MiragesCap)
	}
	if data.ConversionCap != 2 {
		t.Errorf("ConversionCap = %d, want 2 — Specialist doesn't add a third Conversion slot", data.ConversionCap)
	}
	if data.InceptionCap != 1 {
		t.Errorf("InceptionCap = %d, want 1 (Illusionist Specialist's own Inception pick)", data.InceptionCap)
	}
	if len(data.AvailableInception) != 2 {
		t.Errorf("AvailableInception = %+v, want both seeded Inceptions available", data.AvailableInception)
	}

	// Fast Forward Fate needs Temporal Stopwatch AND 10th level — even at
	// Specialist's own effective 5th level (from mirageLevel), 10th level
	// is still out of reach.
	availableNames = map[string]bool{}
	for _, o := range data.AvailableMirages {
		availableNames[o.Name] = true
	}
	if availableNames["Fast Forward Fate"] {
		t.Error("Fast Forward Fate should still be blocked — Specialist's own Mirage qualification tops out at 5th level, not 10th")
	}

	// Picking Reality Marble as this Specialist's one Genjutsu Inception
	// must auto-grant Persistent Genjutsu for free, exactly like a real 9th
	// level (or higher) Genjutsu Specialist gets — Specialist's own text
	// states "as if you were a 9th level Genjutsu Specialist".
	if err := charstore.AddGenjutsuPick(s.charDB, 1, charstore.GenjutsuPickInception, "class/genjutsu-specialist/option/genjutsu-inception/reality-marble"); err != nil {
		t.Fatal(err)
	}
	data, err = s.loadGenjutsuTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	foundGranted := false
	for _, k := range data.KnownMirages {
		if k.Name == "Persistent Genjutsu" && k.Granted {
			foundGranted = true
		}
	}
	if !foundGranted {
		t.Errorf("KnownMirages = %+v, want Persistent Genjutsu auto-granted for a Specialist-feat character with Reality Marble picked", data.KnownMirages)
	}
	if data.MiragesUsed != 0 {
		t.Errorf("MiragesUsed = %d, want 0 — the free grant must not count against the feat-derived cap", data.MiragesUsed)
	}
}

// TestMirageExhibitionBonus covers Mirage Exhibition's own text: 1
// unconditional bonus Mirage slot, plus 1 more each at 5th and 17th
// Genjutsu Specialist level — only ever alongside real levels, since the
// feat's own prerequisite requires at least 4.
func TestMirageExhibitionBonus(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{4, 1}, {5, 2}, {16, 2}, {17, 3}, {20, 3},
	}
	for _, c := range cases {
		if got := mirageExhibitionMirageBonus(c.level); got != c.want {
			t.Errorf("mirageExhibitionMirageBonus(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

// TestPledgeJutsuGrantsFreeCost covers Beguiler's Inspired Appearance and
// Illusionist's Shaping Your World: both are always-on 2nd-level base
// features (not a player pick) that grant a specific E-Rank Genjutsu at 0
// Chakra unconditionally, with no rest-scoped qualifier — same
// genjutsuGrantFreeUnlimited shape as Malleable Mirages' own Beast Speech/
// Myriad Forms/Piece of Mind entries, just always-on rather than
// pick-gated. Confirms the printed nonzero cost_chakra is overridden to 0
// once the granting feature is reached, and stays at the printed cost
// before then.
func TestPledgeJutsuGrantsFreeCost(t *testing.T) {
	s := testServer(t)

	mustExecRules := func(query string, args ...any) {
		t.Helper()
		if _, err := s.rulesDB.Exec(query, args...); err != nil {
			t.Fatalf("seed rules: %v (%s)", err, query)
		}
	}
	mustExecRules(`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/genjutsu-specialist', 'Genjutsu Specialist', 8, 8)`)
	mustExecRules(`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
		('class/genjutsu-specialist/group/genjutsu-pledges', 'class/genjutsu-specialist', 'Genjutsu Pledges')`)
	mustExecRules(`INSERT INTO subclasses (slug, group_slug, name) VALUES
		('class/genjutsu-specialist/group/genjutsu-pledges/beguiler', 'class/genjutsu-specialist/group/genjutsu-pledges', 'Beguiler')`)
	mustExecRules(`INSERT INTO subclass_features (slug, subclass_slug, name, level, description, sort_order) VALUES
		('class/genjutsu-specialist/group/genjutsu-pledges/beguiler/feature/inspired-appearance',
		 'class/genjutsu-specialist/group/genjutsu-pledges/beguiler', 'Inspired Appearance', 2,
		 'When you choose this path at 2nd level, you gain the E-Rank Genjutsu Transform. If you already know this Genjutsu, you gain another E-Rank Genjutsu you qualify for. You can cast Transform at 0 Cost, as a Bonus Action.', 1)`)
	mustExecRules(`INSERT INTO jutsu (slug, name, classification, rank, casting_time, range, duration, components, cost_text, cost_chakra, keywords, description)
		VALUES ('jutsu/transform', 'Transform', 'Genjutsu', 'E', '1 Action', 'Self', 'Instant', 'HS', 'Cost: 1 Chakra', 1, 'Genjutsu', 'test jutsu')`)

	res, err := s.charDB.Exec(`INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Ino', 10, 10, 10, 10, 10, 16)`)
	if err != nil {
		t.Fatal(err)
	}
	characterID, _ := res.LastInsertId()
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (?, 'class/genjutsu-specialist', 1, 0)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level) VALUES (?, 'class/genjutsu-specialist/group/genjutsu-pledges/beguiler', 2)`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}

	// Below level 2: Inspired Appearance not yet reached, Transform (not
	// otherwise known) doesn't even appear on the sheet.
	rows, err := s.loadCharacterJutsuSheet(characterID, &charsheet.Sheet{Level: 1})
	if err != nil {
		t.Fatalf("loadCharacterJutsuSheet at level 1: %v", err)
	}
	for _, j := range rows {
		if j.Slug == "jutsu/transform" {
			t.Errorf("Transform present at level 1, want absent (Inspired Appearance not yet reached)")
		}
	}

	if _, err := s.charDB.Exec(
		`UPDATE character_classes SET levels = 2 WHERE character_id = ? AND class_slug = 'class/genjutsu-specialist'`,
		characterID,
	); err != nil {
		t.Fatal(err)
	}

	// At level 2: Transform lands on the sheet at 0 Chakra, not its printed
	// cost of 1.
	rows, err = s.loadCharacterJutsuSheet(characterID, &charsheet.Sheet{Level: 2})
	if err != nil {
		t.Fatalf("loadCharacterJutsuSheet at level 2: %v", err)
	}
	var transform *jutsuSheetRow
	for i := range rows {
		if rows[i].Slug == "jutsu/transform" {
			transform = &rows[i]
		}
	}
	if transform == nil {
		t.Fatalf("Transform absent at level 2, want present (Inspired Appearance grants it)")
	}
	if transform.CostChakra == nil || *transform.CostChakra != 0 {
		t.Errorf("Transform CostChakra at level 2 = %v, want *0 (Inspired Appearance's unconditional free-cast override)", transform.CostChakra)
	}
	if transform.SourceLabel != "Subclass Feature" {
		t.Errorf("Transform SourceLabel = %q, want %q", transform.SourceLabel, "Subclass Feature")
	}
}
