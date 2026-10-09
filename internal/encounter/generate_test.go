package encounter

import (
	"math/rand"
	"testing"
)

func fixtureToolkit() Toolkit {
	tables := builderFixture()
	classes := []Rank{
		{Slug: "adversary-rank/minion", Name: "Minion", HPFormula: "10 + Level", InitBonus: -2},
		{Slug: "adversary-rank/standard", Name: "Standard"},
		{Slug: "adversary-rank/elite", Name: "Elite", ACBonus: 1, SaveBonus: 1, SaveDCBonus: 1, InitBonus: 2},
		{Slug: "adversary-rank/solo", Name: "Solo", ACBonus: 2, SaveBonus: 2, SaveDCBonus: 2, InitBonus: 3},
	}
	roles := []BuilderRole{
		tables.Roles["adversary-builder-role/defender"],
		tables.Roles["adversary-builder-role/lurker"],
	}
	official := func(slug, rank string) Trait {
		return Trait{Slug: "adversary-role-trait/" + slug, Name: slug, RoleSlug: "adversary-role/defender", Rank: rank, Description: "Test description.", Source: "official", Category: "role"}
	}
	sheet := func(slug, category, tag, rank string) Trait {
		return Trait{Slug: "adversary-builder-sheet/" + slug, Name: slug, Rank: rank, Description: "Test description.", Source: "sheet", Category: category, Tag: tag}
	}
	traits := []Trait{
		official("sturdy", "D"), official("bulwark", "C"), official("immovable", "B"),
		official("aegis", "A"), official("fortress", "S"),
		sheet("shadow-step", "role", "Lurker", "C"),
		sheet("ambusher", "role", "Lurker", "D"),
		sheet("tough", "general", "Generic", "D"),
		sheet("quick", "general", "Generic", ""),
		sheet("legend", "general", "Generic", "S"),
		sheet("sharingan", "clan", "Uchiha", "B"),
		sheet("will-of-fire", "affiliation", "Leaf Shinobi", "C"),
		sheet("cursed", "class_mod", "Cursed Seal", "D"),
	}
	slots := []FreeformSlot{
		{RankSlug: "adversary-rank/minion", LevelMin: 1, Slots: 1},
		{RankSlug: "adversary-rank/standard", LevelMin: 1, Slots: 2},
		{RankSlug: "adversary-rank/standard", LevelMin: 5, Slots: 3},
		{RankSlug: "adversary-rank/elite", LevelMin: 1, Slots: 3},
		{RankSlug: "adversary-rank/solo", LevelMin: 1, Slots: 4},
	}
	return Toolkit{
		Classes: classes, Roles: roles, Traits: traits, Slots: slots, Tables: tables,
		Clans: []string{"Uchiha"}, Affiliations: []string{"Leaf Shinobi"},
	}
}

func classOf(kit Toolkit, kind string) Rank {
	for _, rk := range kit.Classes {
		if ClassKind(rk.Slug) == kind {
			return rk
		}
	}
	panic("no class " + kind)
}

func TestGenerateRosterDeterministicForSeededRNG(t *testing.T) {
	kit := fixtureToolkit()
	roster1, budget1, err := GenerateRoster(rand.New(rand.NewSource(42)), 4, 8, "standard", kit, GenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	roster2, budget2, err := GenerateRoster(rand.New(rand.NewSource(42)), 4, 8, "standard", kit, GenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if budget1 != budget2 || len(roster1) != len(roster2) {
		t.Fatalf("seeded rosters differ: budget %d/%d, length %d/%d", budget1, budget2, len(roster1), len(roster2))
	}
	for i := range roster1 {
		if roster1[i].Name != roster2[i].Name || len(roster1[i].Traits) != len(roster2[i].Traits) {
			t.Errorf("roster[%d] differs: %+v vs %+v", i, roster1[i], roster2[i])
		}
	}
}

func TestGenerateRosterNeverEmpty(t *testing.T) {
	roster, _, err := GenerateRoster(rand.New(rand.NewSource(1)), 1, 1, "easy", fixtureToolkit(), GenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) == 0 {
		t.Fatal("expected a non-empty roster even for a tiny budget")
	}
}

func TestGenerateRosterDeadlyIncludesASoloBoss(t *testing.T) {
	roster, _, err := GenerateRoster(rand.New(rand.NewSource(7)), 4, 12, "deadly", fixtureToolkit(), GenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if roster[0].RankSlug != "adversary-rank/solo" {
		t.Errorf("expected a Solo boss first in a deadly roster, got class %q", roster[0].RankSlug)
	}
}

// Classes are found by slug, so translated class names change nothing.
func TestGenerateRosterIgnoresTranslatedClassNames(t *testing.T) {
	kit := fixtureToolkit()
	for i := range kit.Classes {
		kit.Classes[i].Name = "Traducido " + kit.Classes[i].Name
	}
	roster, _, err := GenerateRoster(rand.New(rand.NewSource(7)), 4, 12, "deadly", kit, GenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(roster) < 2 || roster[0].RankSlug != "adversary-rank/solo" || roster[0].BudgetCost != rankCost("solo", 12) {
		t.Errorf("translated names broke class lookup: %+v", roster)
	}
}

func TestGenerateOneMakesBuilderChoices(t *testing.T) {
	kit := fixtureToolkit()
	kit.Name = func(level int, class Rank, role BuilderRole) string { return class.Slug + "|" + role.Slug }
	adv := GenerateOne(rand.New(rand.NewSource(3)), kit, classOf(kit, "elite"), 8, GenOptions{})
	if adv.AdvRank != "C" || adv.ClanTag != NoClan || adv.AffiliationTag != NoAffiliation {
		t.Errorf("level 8 defaults: rank %q clan %q affiliation %q", adv.AdvRank, adv.ClanTag, adv.AffiliationTag)
	}
	if adv.Armor != DefaultArmor(adv.RoleSlug) {
		t.Errorf("armor %q, want the role's %q", adv.Armor, DefaultArmor(adv.RoleSlug))
	}
	if adv.Name != adv.RankSlug+"|"+adv.RoleSlug {
		t.Errorf("Name func not used: %q", adv.Name)
	}
	if adv.TenacityMax != 4 {
		t.Errorf("Elite level 8 tenacity = %d, want 4", adv.TenacityMax)
	}
}

func TestGenerateOneHonorsTraitSlotsAndRank(t *testing.T) {
	kit := fixtureToolkit()
	rng := rand.New(rand.NewSource(5))
	for i := 0; i < 50; i++ {
		// Level 3 is D-Rank; a Solo has 6 trait slots there.
		adv := GenerateOne(rng, kit, classOf(kit, "solo"), 3, GenOptions{Clan: RandomTag, Affiliation: "Leaf Shinobi"})
		sb := Compute(adv.Choices(4), kit.Tables)
		if len(adv.Traits) > sb.TraitSlots {
			t.Fatalf("%d traits for %d slots", len(adv.Traits), sb.TraitSlots)
		}
		role := kit.Tables.Roles[adv.RoleSlug]
		fromRole := 0
		for _, tr := range adv.Traits {
			if tr.Rank != "" && RankIndex(tr.Rank) > RankIndex("D") {
				t.Fatalf("D-Rank adversary got a %s trait %q", tr.Rank, tr.Name)
			}
			for _, kt := range kit.Traits {
				if kt.Slug != tr.Slug {
					continue
				}
				if InRolePool(kt, role) {
					fromRole++
				} else if !InOtherPool(kt, adv) {
					t.Fatalf("trait %q is outside this adversary's pools", tr.Name)
				}
			}
		}
		// The other pools hold only two D-or-unranked traits here, so the
		// role pool tops the set up past its usual two.
		if fromRole == 0 {
			t.Fatalf("no role trait picked: %+v", adv.Traits)
		}
	}
}

func TestGenerateOptionsPickClanAndAffiliation(t *testing.T) {
	kit := fixtureToolkit()
	rng := rand.New(rand.NewSource(9))
	adv := GenerateOne(rng, kit, classOf(kit, "standard"), 3, GenOptions{Clan: RandomTag, Affiliation: RandomTag})
	if adv.ClanTag != "Uchiha" || adv.AffiliationTag != "Leaf Shinobi" {
		t.Errorf("random picks: clan %q affiliation %q", adv.ClanTag, adv.AffiliationTag)
	}
	adv = GenerateOne(rng, kit, classOf(kit, "standard"), 3, GenOptions{Clan: "Aburame"})
	if adv.ClanTag != "Aburame" {
		t.Errorf("fixed clan pick: %q", adv.ClanTag)
	}
}

func TestRerollKeepsGMChoices(t *testing.T) {
	kit := fixtureToolkit()
	rng := rand.New(rand.NewSource(11))
	solo := classOf(kit, "solo")
	prev := GenerateOne(rng, kit, solo, 12, GenOptions{Clan: "Uchiha"})
	prev.AdvRank = "S"
	prev.ClassModTag = "Cursed Seal"
	prev.Adjust = Adjustments{HP: 10, AC: 1}
	prev = ApplyClassification(prev, "iconic", kit.Tables.Classifications)
	prev = WithNarrativeAttachment(prev, "transformation/x", "X", "", "")

	got := Reroll(rng, kit, solo, prev)
	if got.AdvRank != "S" || got.ClanTag != "Uchiha" || got.ClassModTag != "Cursed Seal" || got.Adjust != prev.Adjust {
		t.Errorf("reroll dropped builder choices: %+v", got)
	}
	if got.Classification != "iconic" || got.TransformationSlug != "transformation/x" {
		t.Errorf("reroll dropped the GM's tags: %+v", got)
	}
	if got.Name != DefaultName(kit, solo, kit.Tables.Roles[got.RoleSlug], 12) {
		t.Errorf("reroll kept a generated name: %q", got.Name)
	}

	prev.Name = "Bridge Warden"
	if got := Reroll(rng, kit, solo, prev); got.Name != "Bridge Warden" {
		t.Errorf("reroll dropped the GM's name: %q", got.Name)
	}
}

func TestNormalizeMapsLegacyRoles(t *testing.T) {
	adv := GeneratedAdversary{RoleSlug: "adversary-role/striker", Level: 10}.Normalize()
	if adv.RoleSlug != "adversary-builder-role/striker-taijutsu" || adv.Armor != "medium" || adv.AdvRank != "B" {
		t.Errorf("legacy adversary normalized to %+v", adv)
	}
}

func TestFreeformSlotsForPicksNearestLevelMin(t *testing.T) {
	slots := fixtureToolkit().Slots
	if got := freeformSlotsFor("adversary-rank/standard", 3, slots); got != 2 {
		t.Errorf("level 3 Standard slots = %d, want 2", got)
	}
	if got := freeformSlotsFor("adversary-rank/standard", 5, slots); got != 3 {
		t.Errorf("level 5 Standard slots = %d, want 3", got)
	}
	if got := freeformSlotsFor("adversary-rank/standard", 100, slots); got != 3 {
		t.Errorf("level 100 Standard slots = %d, want 3 (last known row)", got)
	}
}

func TestAssembleSnapshotsClassValues(t *testing.T) {
	kit := fixtureToolkit()
	elite := classOf(kit, "elite")
	adv := GeneratedAdversary{
		Level:              6,
		RoleSlug:           "adversary-builder-role/defender",
		AdvRank:            "C",
		ACBonus:            40, // stale value from a previous class
		Classification:     "iconic",
		TransformationSlug: "transformation/x",
	}
	got := Assemble(kit, elite, adv)
	iconic, _ := classificationModifier("iconic", kit.Tables.Classifications)
	if got.RankSlug != elite.Slug || got.ACBonus != elite.ACBonus+iconic.ACBonus {
		t.Errorf("class values = %s AC %d, want %s AC %d", got.RankSlug, got.ACBonus, elite.Slug, elite.ACBonus+iconic.ACBonus)
	}
	if got.FreeformSlots != 3 || got.TenacityMax != 3 || got.BudgetCost != rankCost("elite", 6) {
		t.Errorf("slots/tenacity/cost = %d/%d/%d", got.FreeformSlots, got.TenacityMax, got.BudgetCost)
	}
	if got.TransformationSlug != "" {
		t.Errorf("a non-Solo adversary kept its Transformation")
	}
	if got.Name == "" || got.Armor != "heavy" || got.ClanTag != NoClan {
		t.Errorf("not normalized/named: %+v", got)
	}

	again := Assemble(kit, elite, got)
	if again.ACBonus != got.ACBonus {
		t.Errorf("reassembling stacked the classification: AC %d then %d", got.ACBonus, again.ACBonus)
	}
}
