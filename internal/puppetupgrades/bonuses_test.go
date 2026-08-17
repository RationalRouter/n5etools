package puppetupgrades

import "testing"

// TestEnhancedDurabilityBonus locks in Bronze Tier's own per-technique
// split: "Purple: Your armor gains a +1 bonus to its AC calculation";
// "Black/Blue/Green: Increase the hit points of your Puppet Tool by +10.
// For each level after 2nd level... by +2" (only the companion holding the
// pick); "Red: Increase the hit points of all of your Puppet Tools by +5...
// by +1" (every companion, from a single take); White's own bonus lands on
// the character, not here (see CharacterBonuses).
func TestEnhancedDurabilityBonus(t *testing.T) {
	cases := []struct {
		name      string
		ctx       BonusContext
		wantAC    int
		wantMaxHP int
	}{
		{"Purple gets +1 AC", BonusContext{SubclassColor: "Purple", MasterLevel: 10, PicksOnThisCompanion: 1, PicksOnAnyCompanion: 1}, 1, 0},
		{"Purple without the pick", BonusContext{SubclassColor: "Purple", MasterLevel: 10}, 0, 0},
		{"Black at 2nd level", BonusContext{SubclassColor: "Black", MasterLevel: 2, PicksOnThisCompanion: 1, PicksOnAnyCompanion: 1}, 0, 10},
		{"Blue at 6th level", BonusContext{SubclassColor: "Blue", MasterLevel: 6, PicksOnThisCompanion: 1, PicksOnAnyCompanion: 1}, 0, 10 + 2*4},
		{"Green not on this companion", BonusContext{SubclassColor: "Green", MasterLevel: 10, PicksOnAnyCompanion: 1}, 0, 0},
		{"Red at 2nd level, held elsewhere", BonusContext{SubclassColor: "Red", MasterLevel: 2, PicksOnAnyCompanion: 1}, 0, 5},
		{"Red at 9th level", BonusContext{SubclassColor: "Red", MasterLevel: 9, PicksOnAnyCompanion: 1}, 0, 5 + 1*7},
		{"Red with nobody holding it", BonusContext{SubclassColor: "Red", MasterLevel: 9}, 0, 0},
		{"White has no companion term", BonusContext{SubclassColor: "White", MasterLevel: 10, PicksOnThisCompanion: 1, PicksOnAnyCompanion: 1}, 0, 0},
		{"level 1 never goes negative", BonusContext{SubclassColor: "Black", MasterLevel: 1, PicksOnThisCompanion: 1, PicksOnAnyCompanion: 1}, 0, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := enhancedDurabilityBonus(c.ctx)
			if got.AC != c.wantAC || got.MaxHP != c.wantMaxHP {
				t.Errorf("enhancedDurabilityBonus = %+v, want AC %d / MaxHP %d", got, c.wantAC, c.wantMaxHP)
			}
		})
	}
}

// TestBulkyBuildBonus: "Your Puppet Tool gains a +1 to its AC... You can
// take this upgrade up to 3 times, per Puppet Tool" — stacking, capped at
// the book's own 3 takes even if more rows somehow exist.
func TestBulkyBuildBonus(t *testing.T) {
	for _, c := range []struct{ takes, want int }{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {5, 3}} {
		got := bulkyBuildBonus(BonusContext{PicksOnThisCompanion: c.takes})
		if got.AC != c.want {
			t.Errorf("bulkyBuildBonus with %d takes = %+d AC, want %+d", c.takes, got.AC, c.want)
		}
	}
}

// TestQuickfootedBonus: "Your Puppet increases its movement speed by 15
// feet" — once per Puppet Tool, so a second row on the same companion must
// not double it.
func TestQuickfootedBonus(t *testing.T) {
	if got := quickfootedBonus(BonusContext{}); got.Speed != 0 {
		t.Errorf("without the pick = %d, want 0", got.Speed)
	}
	if got := quickfootedBonus(BonusContext{PicksOnThisCompanion: 1}); got.Speed != 15 {
		t.Errorf("with the pick = %d, want 15", got.Speed)
	}
	if got := quickfootedBonus(BonusContext{PicksOnThisCompanion: 2}); got.Speed != 15 {
		t.Errorf("with a duplicate row = %d, want 15 (not doubled)", got.Speed)
	}
}

// TestHoveringMechanismBonus: "Your Puppet Tool gains 30 feet of flying
// speed... For those who practice the Red Technique, two Puppet Tools gain
// the benefits... For those who practice the Purple technique, if your
// Juggernaut Armor is Weaved Mail or a Wooden Suit, increase the flying
// speed by +10 feet. If it is a Steel Fortress, decrease the flying speed
// by -10ft."
func TestHoveringMechanismBonus(t *testing.T) {
	cases := []struct {
		name string
		ctx  BonusContext
		want int // 0 means no flying speed granted at all
	}{
		{"not taken", BonusContext{SubclassColor: "Black"}, 0},
		{"taken", BonusContext{SubclassColor: "Black", PicksOnThisCompanion: 1}, 30},
		{"taken twice raises by 20", BonusContext{SubclassColor: "Black", PicksOnThisCompanion: 2}, 50},
		{"Red reaches a second puppet", BonusContext{SubclassColor: "Red", PicksOnAnyCompanion: 1}, 30},
		{"Black does not reach a second puppet", BonusContext{SubclassColor: "Black", PicksOnAnyCompanion: 1}, 0},
		{"Purple in Weaved Mail", BonusContext{SubclassColor: "Purple", PicksOnThisCompanion: 1, ArmorChassis: "Weaved Mail"}, 40},
		{"Purple in a Wooden Suit", BonusContext{SubclassColor: "Purple", PicksOnThisCompanion: 1, ArmorChassis: "Wooden Suit"}, 40},
		{"Purple in a Steel Fortress", BonusContext{SubclassColor: "Purple", PicksOnThisCompanion: 1, ArmorChassis: "Steel Fortress"}, 20},
		{"Purple in an Iron Shell", BonusContext{SubclassColor: "Purple", PicksOnThisCompanion: 1, ArmorChassis: "Iron Shell"}, 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hoveringMechanismBonus(c.ctx)
			if c.want == 0 {
				if got.GrantsFlight {
					t.Errorf("granted %d ft. of flight, want none", got.FlySpeed)
				}
				return
			}
			if !got.GrantsFlight || got.FlySpeed != c.want {
				t.Errorf("flying speed = %d (granted %v), want %d", got.FlySpeed, got.GrantsFlight, c.want)
			}
		})
	}
}

// TestChassisReinforcedBonus: Enhanced Durability's Purple reading also
// "increases the Reinforced property by 3" — only for Purple, and only
// when the upgrade is actually held.
func TestChassisReinforcedBonus(t *testing.T) {
	if got := ChassisReinforcedBonus("Purple", true); got != 3 {
		t.Errorf("Purple with the upgrade = %d, want 3", got)
	}
	if got := ChassisReinforcedBonus("Purple", false); got != 0 {
		t.Errorf("Purple without the upgrade = %d, want 0", got)
	}
	if got := ChassisReinforcedBonus("Black", true); got != 0 {
		t.Errorf("Black with the upgrade = %d, want 0", got)
	}
}

// TestCharacterBonusesApplyTo covers the two upgrades that modify the
// Puppet Master's own sheet: Enhanced Durability's White reading is
// technique-gated, Accelerated Movement's text is not.
func TestCharacterBonusesApplyTo(t *testing.T) {
	byName := map[string]CharacterBonus{}
	for _, b := range CharacterBonuses {
		byName[b.Name] = b
	}

	ed := byName["Enhanced Durability"]
	if !ed.AppliesTo("White") {
		t.Error("Enhanced Durability should apply to White")
	}
	if ed.AppliesTo("Purple") {
		t.Error("Enhanced Durability's character-side HP term is White only")
	}
	if got := ed.MaxHP(7); got != 7 {
		t.Errorf("White Max HP at Puppet Master level 7 = %d, want 7 (+1 per class level)", got)
	}

	am := byName["Accelerated Movement"]
	for _, color := range []string{"Purple", "White", "Black"} {
		if !am.AppliesTo(color) {
			t.Errorf("Accelerated Movement should apply to %s (no per-technique split in its text)", color)
		}
	}
	if got := am.Speed(1); got != 10 {
		t.Errorf("Accelerated Movement speed = %d, want 10", got)
	}
}

// TestStatBonusParts is the wording shown on a puppet's card: signed deltas
// for a modifier, a plain total for a flying speed the puppet did not have.
func TestStatBonusParts(t *testing.T) {
	got := StatBonusParts(StatBonus{AC: 1, MaxHP: 18, Speed: 15, FlySpeed: 30, GrantsFlight: true})
	want := []string{"+1 AC", "+18 Max HP", "+15 ft. Speed", "30 ft. flying speed"}
	if len(got) != len(want) {
		t.Fatalf("parts = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d = %q, want %q", i, got[i], want[i])
		}
	}
}
