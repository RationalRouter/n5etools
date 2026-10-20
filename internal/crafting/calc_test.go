package crafting

import (
	"testing"
)

// testCatalog is a small hand-built catalog covering every rule Calculate
// enforces, so the rule tests don't depend on rules.db.
func testCatalog() *Catalog {
	c := &Catalog{
		Bases: []Base{
			{Slug: "weapon/katana", Name: "Katana", Kind: "weapon", CostRyo: 30},
			{Slug: "armor/chunin-jacket", Name: "Chunin Jacket", Kind: "armor", CostRyo: 750},
			{Slug: "armor/hq-plate", Name: "HQ Plate", Kind: "armor", CostRyo: 1000, HighQuality: true},
		},
		Seals: []Seal{
			{Slug: "seal/weapon/minor/martial-seal", Name: "Martial Seal (Minor)", AppliesTo: "weapon", Rank: "D", CostRyo: 250, Family: "martial-seal"},
			{Slug: "seal/weapon/greater/martial-seal", Name: "Martial Seal (Greater)", AppliesTo: "weapon", Rank: "B", CostRyo: 1000, Family: "martial-seal"},
			{Slug: "seal/weapon/greater/elemental-seal", Name: "Elemental Seal (Greater)", AppliesTo: "weapon", Rank: "B", CostRyo: 1200, Family: "elemental-seal"},
			{Slug: "seal/weapon/minor/bane-seal", Name: "Bane Seal (Minor)", AppliesTo: "weapon", Rank: "D", CostRyo: 300, Family: "bane-seal"},
			{Slug: "seal/weapon/minor/fanged-seal", Name: "Fanged Seal (Minor)", AppliesTo: "weapon", Rank: "D", CostRyo: 300, Family: "fanged-seal"},
			{Slug: "seal/weapon/refined/storing-seal", Name: "Storing Seal (Refined)", AppliesTo: "weapon", Rank: "C", CostRyo: 600, Family: "storing-seal"},
			{Slug: "seal/weapon/supreme/doom-seal", Name: "Doom Seal (Supreme)", AppliesTo: "weapon", Rank: "S", CostRyo: 10000, Family: "doom-seal"},
			{Slug: "seal/armor/minor/swift-seal", Name: "Swift Seal (Minor)", AppliesTo: "armor", Rank: "D", CostRyo: 200, Family: "swift-seal"},
			{Slug: "seal/armor/superior/warding-seal", Name: "Warding Seal (Superior)", AppliesTo: "armor", Rank: "A", CostRyo: 3000, Family: "warding-seal"},
		},
		Jutsu: []JutsuOption{
			{Slug: "jutsu/fireball", Name: "Fireball", Rank: "D", Classification: "Ninjutsu"},
			{Slug: "jutsu/big-fireball", Name: "Big Fireball", Rank: "B", Classification: "Ninjutsu"},
		},
		Summons: []SummonOption{
			{Slug: "summon/wolves", Name: "Wolves", SummonType: "Carnivoran"},
			{Slug: "summon/hawks", Name: "Hawks", SummonType: "Avian"},
		},
		baseBySlug: map[string]int{}, sealBySlug: map[string]int{},
		jutsuBySlug: map[string]int{}, summonBySlug: map[string]int{},
	}
	for i, b := range c.Bases {
		c.baseBySlug[b.Slug] = i
	}
	for i := range c.Seals {
		if rule, ok := sealChoices[c.Seals[i].Slug]; ok {
			c.Seals[i].Choice = &rule
		}
		c.sealBySlug[c.Seals[i].Slug] = i
	}
	for i, j := range c.Jutsu {
		c.jutsuBySlug[j.Slug] = i
	}
	for i, s := range c.Summons {
		c.summonBySlug[s.Slug] = i
	}
	return c
}

func recipe(base, quality string, seals ...SealPick) Recipe {
	return Recipe{Name: "Test", BaseSlug: base, Quality: quality, Seals: seals, Crafting: DefaultCrafting()}
}

func hasKey(msgs []Message, key string) bool {
	for _, m := range msgs {
		if m.Key == key {
			return true
		}
	}
	return false
}

func TestCalculateSlotsRankAndPrice(t *testing.T) {
	c := testCatalog()
	r := recipe("weapon/katana", "greater",
		SealPick{Slug: "seal/weapon/minor/martial-seal"},
		SealPick{Slug: "seal/weapon/greater/elemental-seal", Choices: []string{"fire"}})
	r.Crafting.CrafterLevel = 9
	res := Calculate(c, r)
	if !res.Valid() {
		t.Fatalf("unexpected errors: %+v", res.Errors)
	}
	if res.SlotsUsed != 4 || res.SlotsAvailable != 5 {
		t.Errorf("slots = %d/%d, want 4/5", res.SlotsUsed, res.SlotsAvailable)
	}
	if res.Rank != "B" || res.MinLevel != 9 || res.CraftDC != 22 || res.SealWeeks != 8 || res.FailureDowntime != 3 {
		t.Errorf("rank row = %s L%d DC%d %dwk fail %d, want B L9 DC22 8wk fail 3",
			res.Rank, res.MinLevel, res.CraftDC, res.SealWeeks, res.FailureDowntime)
	}
	if res.BaseValue != 150 || res.BaseCost != 75 {
		t.Errorf("base value/cost = %v/%v, want 150/75", res.BaseValue, res.BaseCost)
	}
	if res.SealMaterials != 1450 || res.SealCost != 1450 || res.SealPrepDowntime != 1 {
		t.Errorf("seal materials/cost/prep = %v/%v/%d, want 1450/1450/1", res.SealMaterials, res.SealCost, res.SealPrepDowntime)
	}
	if res.MarketValue != 1600 || res.TotalCost != 1525 {
		t.Errorf("market/total = %v/%v, want 1600/1525", res.MarketValue, res.TotalCost)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("unexpected warnings: %+v", res.Warnings)
	}
}

func TestCalculateNoSeals(t *testing.T) {
	res := Calculate(testCatalog(), recipe("weapon/katana", "standard"))
	if !res.Valid() || res.Rank != "" || res.MinLevel != 1 || res.SealCost != 0 || res.SealPrepDowntime != 0 {
		t.Errorf("plain item: %+v", res)
	}
}

func TestCalculateHardErrors(t *testing.T) {
	c := testCatalog()
	cases := []struct {
		name string
		r    Recipe
		key  string
	}{
		{"name required", Recipe{BaseSlug: "weapon/katana", Quality: "standard"}, "crafting.err.name_required"},
		{"no base", recipe("weapon/nope", "standard"), "crafting.err.no_base"},
		{"bad quality", recipe("weapon/katana", "legendary"), "crafting.err.bad_quality"},
		{"unknown seal", recipe("weapon/katana", "standard", SealPick{Slug: "seal/weapon/minor/nope"}), "crafting.err.unknown_seal"},
		{"armor seal on weapon", recipe("weapon/katana", "standard", SealPick{Slug: "seal/armor/minor/swift-seal"}), "crafting.err.wrong_kind"},
		{"weapon seal on armor", recipe("armor/chunin-jacket", "standard", SealPick{Slug: "seal/weapon/minor/martial-seal"}), "crafting.err.wrong_kind"},
		{"same family across tiers", recipe("weapon/katana", "greater",
			SealPick{Slug: "seal/weapon/minor/martial-seal"}, SealPick{Slug: "seal/weapon/greater/martial-seal"}), "crafting.err.same_family"},
		{"too many slots", recipe("weapon/katana", "standard", SealPick{Slug: "seal/weapon/greater/martial-seal"},
			SealPick{Slug: "seal/weapon/minor/bane-seal", Choices: []string{"beast", "undead"}}), "crafting.err.too_many_slots"},
		{"missing choice", recipe("weapon/katana", "greater", SealPick{Slug: "seal/weapon/greater/elemental-seal"}), "crafting.err.choice"},
		{"choice not in list", recipe("weapon/katana", "greater",
			SealPick{Slug: "seal/weapon/greater/elemental-seal", Choices: []string{"acid"}}), "crafting.err.choice"},
		{"duplicate creature", recipe("weapon/katana", "standard",
			SealPick{Slug: "seal/weapon/minor/bane-seal", Choices: []string{"beast", "beast"}}), "crafting.err.choice"},
		{"too few creatures", recipe("weapon/katana", "standard",
			SealPick{Slug: "seal/weapon/minor/bane-seal", Choices: []string{"beast"}}), "crafting.err.choice"},
		{"summon of wrong type", recipe("weapon/katana", "standard",
			SealPick{Slug: "seal/weapon/minor/fanged-seal", Choices: []string{"summon/hawks"}}), "crafting.err.choice"},
		{"stored jutsu above cap", recipe("weapon/katana", "standard",
			SealPick{Slug: "seal/weapon/refined/storing-seal", Choices: []string{"jutsu/big-fireball"}}), "crafting.err.choice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Calculate(c, tc.r)
			if res.Valid() || !hasKey(res.Errors, tc.key) {
				t.Errorf("errors = %+v, want %s", res.Errors, tc.key)
			}
		})
	}
}

func TestCalculateValidChoicesResolve(t *testing.T) {
	c := testCatalog()
	res := Calculate(c, recipe("weapon/katana", "greater",
		SealPick{Slug: "seal/weapon/minor/fanged-seal", Choices: []string{"summon/wolves"}},
		SealPick{Slug: "seal/weapon/refined/storing-seal", Choices: []string{"jutsu/fireball"}},
		SealPick{Slug: "seal/weapon/minor/bane-seal", Choices: []string{"beast", "undead"}}))
	if !res.Valid() {
		t.Fatalf("unexpected errors: %+v", res.Errors)
	}
	if got := res.Seals[0].Chosen; len(got) != 1 || got[0].Name != "Wolves" || got[0].Href != "" {
		t.Errorf("fanged choice = %+v", got)
	}
	if got := res.Seals[1].Chosen; len(got) != 1 || got[0].Name != "Fireball" || got[0].Href != "/jutsu/jutsu/fireball" {
		t.Errorf("storing choice = %+v", got)
	}
	if got := res.Seals[2].Chosen; len(got) != 2 || got[0].LabelKey != "crafting.creature.beast" {
		t.Errorf("bane choice = %+v", got)
	}
	if !hasKey(res.Warnings, "crafting.warn.storing_check_gm") {
		t.Errorf("missing storing GM warning: %+v", res.Warnings)
	}
}

func TestCalculateHighQualityArmorSlot(t *testing.T) {
	c := testCatalog()
	if res := Calculate(c, recipe("armor/hq-plate", "standard")); res.SlotsAvailable != 4 {
		t.Errorf("High Quality armor slots = %d, want 4", res.SlotsAvailable)
	}
	if res := Calculate(c, recipe("armor/chunin-jacket", "mastercraft")); res.SlotsAvailable != 11 {
		t.Errorf("mastercraft slots = %d, want 11", res.SlotsAvailable)
	}
}

func TestQualityMultipliers(t *testing.T) {
	c := testCatalog()
	for _, tc := range []struct {
		base, quality string
		want          float64
	}{
		{"weapon/katana", "standard", 30},
		{"weapon/katana", "superior", 210},
		{"weapon/katana", "mastercraft", 450},
		{"armor/chunin-jacket", "greater", 1125},
		{"armor/chunin-jacket", "supreme", 1500},
	} {
		if res := Calculate(c, recipe(tc.base, tc.quality)); res.BaseValue != tc.want {
			t.Errorf("%s %s value = %v, want %v", tc.base, tc.quality, res.BaseValue, tc.want)
		}
	}
}

// TestBaseCraftDowntime follows the book's worked example (p.71): a 750 Ryo
// Reinforced Chunin Jacket takes 8 Downtime alone, and 3 Downtime split
// across three crafters.
func TestBaseCraftDowntime(t *testing.T) {
	c := testCatalog()
	cases := []struct {
		name               string
		participants, mast int
		kit                string
		want               int
	}{
		{"solo", 1, 0, "standard", 8},
		{"three crafters", 3, 0, "standard", 3},
		{"solo master", 1, 1, "standard", 5},
		{"solo superior kit", 1, 0, "superior", 4},
		{"solo supreme kit", 1, 0, "supreme", 3},
	}
	for _, tc := range cases {
		r := recipe("armor/chunin-jacket", "standard")
		r.Crafting.Participants, r.Crafting.Masters, r.Crafting.KitTier = tc.participants, tc.mast, tc.kit
		res := Calculate(c, r)
		if res.BaseDowntime != tc.want || res.BaseCost != 375 {
			t.Errorf("%s: downtime %d cost %v, want %d / 375", tc.name, res.BaseDowntime, res.BaseCost, tc.want)
		}
	}
}

func TestBaseSources(t *testing.T) {
	c := testCatalog()
	r := recipe("armor/chunin-jacket", "greater")
	r.Crafting.BaseSource = BaseBuy
	res := Calculate(c, r)
	if res.BaseCost != 1125 || res.BaseDowntime != 0 || !hasKey(res.Warnings, "crafting.warn.rare_quality") {
		t.Errorf("buy: cost %v downtime %d warnings %+v", res.BaseCost, res.BaseDowntime, res.Warnings)
	}
	r.Crafting.BaseSource = BaseOwn
	res = Calculate(c, r)
	if res.BaseCost != 0 || res.BaseDowntime != 0 || res.MarketValue != 1125 {
		t.Errorf("own: cost %v downtime %d market %v", res.BaseCost, res.BaseDowntime, res.MarketValue)
	}
}

func TestSealsmithAndSplitting(t *testing.T) {
	c := testCatalog()
	r := recipe("armor/chunin-jacket", "greater", SealPick{Slug: "seal/armor/superior/warding-seal"})
	r.Crafting.CrafterLevel = 13
	r.Crafting.KitTier = "greater"
	r.Crafting.Participants = 5
	res := Calculate(c, r)
	if res.SealCost != 3000 || res.SealWeeks != 12 || res.SealWeeksPerCrafter != 3 || res.SealPrepDowntime != 1 {
		t.Errorf("crafted: cost %v weeks %d per crafter %d prep %d", res.SealCost, res.SealWeeks, res.SealWeeksPerCrafter, res.SealPrepDowntime)
	}
	r.Crafting.Participants = 20
	if res := Calculate(c, r); res.SealWeeksPerCrafter != 1 {
		t.Errorf("split floor = %d, want 1", res.SealWeeksPerCrafter)
	}
	r.Crafting.HireSealsmith = true
	res = Calculate(c, r)
	if res.SealCost != 4500 || res.SealPrepDowntime != 0 || res.SealWeeksPerCrafter != 0 {
		t.Errorf("hired: cost %v prep %d per crafter %d", res.SealCost, res.SealPrepDowntime, res.SealWeeksPerCrafter)
	}
}

func TestRequirementWarnings(t *testing.T) {
	c := testCatalog()

	r := recipe("weapon/katana", "supreme", SealPick{Slug: "seal/weapon/supreme/doom-seal"})
	r.Crafting.Proficient, r.Crafting.HasForge, r.Crafting.HasSealForge = false, false, false
	res := Calculate(c, r)
	for _, key := range []string{"crafting.warn.no_proficiency", "crafting.warn.no_forge", "crafting.warn.needs_mastery",
		"crafting.warn.no_seal_forge", "crafting.warn.crafter_level"} {
		if !hasKey(res.Warnings, key) {
			t.Errorf("missing %s in %+v", key, res.Warnings)
		}
	}
	if !res.Valid() {
		t.Errorf("warnings must not invalidate: %+v", res.Errors)
	}

	r = recipe("weapon/katana", "mastercraft")
	r.Crafting.Participants, r.Crafting.Masters = 3, 2
	if res := Calculate(c, r); !hasKey(res.Warnings, "crafting.warn.needs_three_masters") {
		t.Errorf("missing needs_three_masters: %+v", res.Warnings)
	}
	r.Crafting.Masters = 3
	if res := Calculate(c, r); hasKey(res.Warnings, "crafting.warn.needs_three_masters") {
		t.Errorf("three masters still warned: %+v", res.Warnings)
	}

	r = recipe("armor/chunin-jacket", "greater", SealPick{Slug: "seal/armor/superior/warding-seal"})
	r.Crafting.CrafterLevel = 13
	if res := Calculate(c, r); !hasKey(res.Warnings, "crafting.warn.armor_kit_rank") {
		t.Errorf("standard armorsmith kit above C-Rank not warned: %+v", res.Warnings)
	}
	r.Crafting.KitTier = "greater"
	if res := Calculate(c, r); hasKey(res.Warnings, "crafting.warn.armor_kit_rank") {
		t.Errorf("greater armorsmith kit warned: %+v", res.Warnings)
	}

	// Buying the base and hiring a Sealsmith means the crafter makes nothing,
	// so none of the crafting requirements apply.
	r = recipe("weapon/katana", "standard", SealPick{Slug: "seal/weapon/supreme/doom-seal"})
	r.Crafting.BaseSource, r.Crafting.HireSealsmith, r.Crafting.Proficient = BaseBuy, true, false
	if res := Calculate(c, r); len(res.Warnings) != 0 {
		t.Errorf("nothing crafted, but warned: %+v", res.Warnings)
	}
}

func TestDiceBonusNonstackingWarning(t *testing.T) {
	c := testCatalog()
	r := recipe("weapon/katana", "greater",
		SealPick{Slug: "seal/weapon/greater/elemental-seal", Choices: []string{"fire"}},
		SealPick{Slug: "seal/weapon/minor/bane-seal", Choices: []string{"beast", "undead"}})
	r.Crafting.CrafterLevel = 9
	if res := Calculate(c, r); !hasKey(res.Warnings, "crafting.warn.dice_bonus_nonstacking") {
		t.Errorf("missing dice_bonus_nonstacking: %+v", res.Warnings)
	}
}

func TestNormalize(t *testing.T) {
	cr := Crafting{BaseSource: "steal", Participants: 0, Masters: 9, KitTier: "mythic", CrafterLevel: 40}
	cr.Normalize()
	if cr.BaseSource != BaseCraft || cr.Participants != 1 || cr.Masters != 1 || cr.KitTier != "standard" || cr.CrafterLevel != 20 {
		t.Errorf("normalized = %+v", cr)
	}
}
