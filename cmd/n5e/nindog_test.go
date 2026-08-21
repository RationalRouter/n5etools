package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// TestNinDogRankForLevel pins Beast Master's own explicit level
// breakpoints ("levels 8, 12, 16 and 20"), distinct from the generic
// summon rank-band formula (highestRankForLevel) an ordinary kind="summon"
// companion uses.
func TestNinDogRankForLevel(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{1, "D"}, {7, "D"}, {8, "C"}, {11, "C"}, {12, "B"}, {15, "B"}, {16, "A"}, {19, "A"}, {20, "S"},
	}
	for _, c := range cases {
		if got := ninDogRankForLevel(c.level); got != c.want {
			t.Errorf("ninDogRankForLevel(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

// TestNinDogSizeForLevel pins Beast Master's own explicit size progression
// ("begins as small, grows to medium when it reaches 8th level and grows
// to Large at 12th" — no stated growth past 12th).
func TestNinDogSizeForLevel(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{1, "Small"}, {7, "Small"}, {8, "Medium"}, {11, "Medium"}, {12, "Large"}, {20, "Large"},
	}
	for _, c := range cases {
		if got := ninDogSizeForLevel(c.level); got != c.want {
			t.Errorf("ninDogSizeForLevel(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

// TestNinDogJutsuSlotsMaxForRank pins summon_tribe_progression's own Jutsu
// Slots column (5/7/10/13/16 at D/C/B/A/S) — the one column from that
// table still meaningful for a Nin-Dog.
func TestNinDogJutsuSlotsMaxForRank(t *testing.T) {
	cases := []struct {
		rank string
		want int
	}{
		{"D", 5}, {"C", 7}, {"B", 10}, {"A", 13}, {"S", 16},
	}
	for _, c := range cases {
		if got := ninDogJutsuSlotsMaxForRank(c.rank); got != c.want {
			t.Errorf("ninDogJutsuSlotsMaxForRank(%q) = %d, want %d", c.rank, got, c.want)
		}
	}
}

// TestNinDogBaseSpeedForRank pins summon_tribe_progression's own Speed
// column (30/45/50/60/75ft at D/C/B/A/S).
func TestNinDogBaseSpeedForRank(t *testing.T) {
	cases := []struct {
		rank string
		want int
	}{
		{"D", 30}, {"C", 45}, {"B", 50}, {"A", 60}, {"S", 75},
	}
	for _, c := range cases {
		if got := ninDogBaseSpeedForRank(c.rank); got != c.want {
			t.Errorf("ninDogBaseSpeedForRank(%q) = %d, want %d", c.rank, got, c.want)
		}
	}
}

// TestNinDogJutsuSlotShortRestRegen pins Beast Master's own "regains 1
// Jutsu Slot on a Short Rest... Starting at 11th level... regains 2".
func TestNinDogJutsuSlotShortRestRegen(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 1}, {10, 1}, {11, 2}, {20, 2},
	}
	for _, c := range cases {
		if got := ninDogJutsuSlotShortRestRegen(c.level); got != c.want {
			t.Errorf("ninDogJutsuSlotShortRestRegen(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

// TestNinDogMaxHP pins the (Toughness + CON modifier) x level formula with
// Toughness fixed at 10 (Beast Master's own override of the generic
// Dog/Wolf tribe's printed 8), and the floor of 1 for a pathological
// negative-modifier case.
func TestNinDogMaxHP(t *testing.T) {
	cases := []struct {
		level, conMod int
		want          int
	}{
		{1, 2, 12},   // (10+2)*1
		{12, 2, 144}, // (10+2)*12 — the level-12 case exercised live
		{1, -5, 5},   // (10-5)*1
		{1, -11, 1},  // would go negative without the floor
	}
	for _, c := range cases {
		if got := ninDogMaxHP(c.level, c.conMod); got != c.want {
			t.Errorf("ninDogMaxHP(%d, %d) = %d, want %d", c.level, c.conMod, got, c.want)
		}
	}
}

// TestNinDogAC pins "12 + Half Nin-Dogs Level + Nin-Dogs Defensive Ability
// score" read as ability MODIFIER (see ninDogAC's own doc comment for why).
func TestNinDogAC(t *testing.T) {
	cases := []struct {
		level, dexMod int
		want          int
	}{
		{1, 2, 14},  // 12 + 0 + 2
		{12, 2, 20}, // 12 + 6 + 2 — the level-12 case exercised live
		{20, 3, 25}, // 12 + 10 + 3
	}
	for _, c := range cases {
		if got := ninDogAC(c.level, c.dexMod); got != c.want {
			t.Errorf("ninDogAC(%d, %d) = %d, want %d", c.level, c.dexMod, got, c.want)
		}
	}
}

// TestNinDogEffectiveAbilityModifier covers the fallback-to-D-Rank-baseline
// behavior a brand-new Nin-Dog (every ability score field blank) needs for
// its very first Sync AC/Max HP hint to be sane, and confirms an
// explicitly-set companion score always wins over the baseline.
func TestNinDogEffectiveAbilityModifier(t *testing.T) {
	blank := charstore.Companion{}
	if got := ninDogEffectiveAbilityModifier(blank, "dex"); got != 2 {
		t.Errorf("blank companion dex modifier = %d, want 2 (from the 14 D-Rank baseline)", got)
	}
	if got := ninDogEffectiveAbilityModifier(blank, "int"); got != 0 {
		t.Errorf("blank companion int modifier = %d, want 0 (from the 10 D-Rank baseline)", got)
	}

	set := charstore.Companion{Dex: sql.NullInt64{Int64: 20, Valid: true}}
	if got := ninDogEffectiveAbilityModifier(set, "dex"); got != 5 {
		t.Errorf("explicit dex=20 modifier = %d, want 5 (explicit score must win over baseline)", got)
	}
}

// TestNinDogFeaturePickSlots covers the four base rank-up slots every
// breed gets plus Young Kugsha's own two bonus slots, and that a non-Kugsha
// breed never sees those bonus slots at all.
func TestNinDogFeaturePickSlots(t *testing.T) {
	base := ninDogFeaturePickSlots("young-inuit", 20, nil)
	if len(base) != 4 {
		t.Fatalf("young-inuit at level 20 = %d slots, want 4 (no Kugsha bonus slots)", len(base))
	}
	for _, s := range base {
		if !s.Unlocked {
			t.Errorf("slot %q locked at level 20, want unlocked", s.Label)
		}
	}

	kugsha := ninDogFeaturePickSlots("young-kugsha", 1, nil)
	if len(kugsha) != 6 {
		t.Fatalf("young-kugsha at level 1 = %d slots, want 6 (4 base + 2 Kugsha bonus)", len(kugsha))
	}
	// ChoiceIndex 4: the immediate bonus D-Rank pick, unlocked from level 1.
	if !kugsha[4].Unlocked {
		t.Error("young-kugsha's immediate bonus D-Rank slot (index 4) should be unlocked at level 1")
	}
	// ChoiceIndex 5: the 11th-level D-or-C-Rank bonus pick, locked below 11.
	if kugsha[5].Unlocked {
		t.Error("young-kugsha's 11th-level bonus slot (index 5) should be locked at level 1")
	}
	kugsha11 := ninDogFeaturePickSlots("young-kugsha", 11, nil)
	if !kugsha11[5].Unlocked {
		t.Error("young-kugsha's 11th-level bonus slot (index 5) should be unlocked at level 11")
	}

	// A pick already made should be reflected back on its own slot.
	picked := ninDogFeaturePickSlots("young-inuit", 8, map[int]string{0: "Inu Fangs"})
	if picked[0].Picked != "Inu Fangs" {
		t.Errorf("slot 0 Picked = %q, want %q", picked[0].Picked, "Inu Fangs")
	}
}

// TestNinDogFilterFeaturesByRank covers the rank-subset filter that
// populates each pick slot's own Options, including Young Kugsha's
// 11th-level slot spanning two ranks (D or C).
func TestNinDogFilterFeaturesByRank(t *testing.T) {
	features := []companionFeatureRef{
		{Name: "Inu Growl", Rank: "D"},
		{Name: "Inu Tactics", Rank: "C"},
		{Name: "Inu Distraction", Rank: "B"},
	}
	if got := ninDogFilterFeaturesByRank(features, []string{"C"}); len(got) != 1 || got[0].Name != "Inu Tactics" {
		t.Errorf("filter by [C] = %+v, want just Inu Tactics", got)
	}
	if got := ninDogFilterFeaturesByRank(features, []string{"D", "C"}); len(got) != 2 {
		t.Errorf("filter by [D,C] = %d results, want 2", len(got))
	}
	if got := ninDogFilterFeaturesByRank(features, []string{"S"}); len(got) != 0 {
		t.Errorf("filter by [S] = %d results, want 0 (no S-Rank features in this fixture)", len(got))
	}
}

// TestPrefillNinDogStatDefaultsOnCreation covers the actual bug this fix
// closes: a brand-new Nin-Dog must reach its first render already carrying
// AC/HP/Speed/Jutsu-Slots-Max/ability scores computed from Beast Master's
// own baseline (mirroring prefillPuppetStatDefaults for kind="puppet"),
// not sitting blank until the player clicks every Sync button by hand.
// Exercises the real HTTP handler (handleSheetCompanionAdd), not
// prefillNinDogStatDefaults directly, so this also pins that the creation
// handler's own kind=="nin-dog" branch actually calls it.
func TestPrefillNinDogStatDefaultsOnCreation(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kiba', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	// Level 8 (rank C): exercises the same level/rank breakpoint confirmed
	// live during this fix's own manual testing (a level-1 character stays
	// at Jutsu Slots Max default of 5/Speed 30, too flat a case to catch a
	// wrong rank lookup).
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/hunter-nin', 8, 0)`,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/characters/1/sheet/companions",
		strings.NewReader(url.Values{"name": {"Akamaru"}, "kind": {"nin-dog"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleSheetCompanionAdd(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("add nin-dog companion: status %d, body %s", w.Code, w.Body.String())
	}

	companions, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(companions) != 1 {
		t.Fatalf("ListCompanions = %d rows, want 1", len(companions))
	}
	c := companions[0]

	// D-Rank baseline ability scores (14/14/14/10/12/10) give a +2 Dex mod
	// and a +2 Con mod — the same fallback ninDogEffectiveAbilityModifier
	// uses for a companion whose own ability score fields are still blank
	// at the moment prefillNinDogStatDefaults runs (right after AddCompanion,
	// before this call ever writes them).
	const rank = "C"
	wantAC := int64(ninDogAC(8, 2))
	wantHP := int64(ninDogMaxHP(8, 2))
	wantSpeed := int64(ninDogBaseSpeedForRank(rank))
	wantSlots := int64(ninDogJutsuSlotsMaxForRank(rank))

	if !c.AC.Valid || c.AC.Int64 != wantAC {
		t.Errorf("AC = %+v, want %d", c.AC, wantAC)
	}
	if !c.HPCurrent.Valid || c.HPCurrent.Int64 != wantHP {
		t.Errorf("HPCurrent = %+v, want %d", c.HPCurrent, wantHP)
	}
	if !c.HPMax.Valid || c.HPMax.Int64 != wantHP {
		t.Errorf("HPMax = %+v, want %d", c.HPMax, wantHP)
	}
	if !c.Speed.Valid || c.Speed.Int64 != wantSpeed {
		t.Errorf("Speed = %+v, want %d", c.Speed, wantSpeed)
	}
	if !c.JutsuSlotsMax.Valid || c.JutsuSlotsMax.Int64 != wantSlots {
		t.Errorf("JutsuSlotsMax = %+v, want %d", c.JutsuSlotsMax, wantSlots)
	}
	for key, dest := range map[string]sql.NullInt64{
		"str": c.Str, "dex": c.Dex, "con": c.Con, "int": c.Int, "wis": c.Wis, "cha": c.Cha,
	} {
		want := int64(ninDogBaseAbilityScores[key])
		if !dest.Valid || dest.Int64 != want {
			t.Errorf("%s = %+v, want %d (D-Rank baseline)", key, dest, want)
		}
	}

	// The same COALESCE-based safety prefillPuppetStatDefaults already
	// relies on: re-running the prefill (as a future per-render backfill
	// would) must never clobber a field the player has since edited by
	// hand.
	if err := charstore.SetCompanionIntField(s.charDB, 1, c.ID, "ac", sql.NullInt64{Int64: 99, Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.prefillNinDogStatDefaults(1, c.ID); err != nil {
		t.Fatal(err)
	}
	after, err := charstore.GetCompanion(s.charDB, 1, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.AC.Valid || after.AC.Int64 != 99 {
		t.Errorf("re-running the prefill overwrote a manually-set AC: got %+v, want 99 preserved", after.AC)
	}
}
