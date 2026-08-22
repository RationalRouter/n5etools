package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

// TestPuppetUpgradeAutoGrantedEntrySlugs locks in Green Technique
// Marionettist's own two free-Upgrade grants ("you gain the Jutsu Channeler
// Upgrade" at 2nd level, "all Puppets you possess gain the Jutsu
// Specialization Upgrade" at 6th) and confirms no other subclass/level
// combination is affected.
func TestPuppetUpgradeAutoGrantedEntrySlugs(t *testing.T) {
	jutsuChanneler := "class/puppet-master/option/puppet-master-upgrades/wood-tier/entry/jutsu-channeler"
	jutsuSpecialization := "class/puppet-master/option/puppet-master-upgrades/bronze-tier/entry/jutsu-specialization"

	if got := puppetUpgradeAutoGrantedEntrySlugs("Green", 1); len(got) != 0 {
		t.Errorf("Green level 1 = %v, want empty (not yet 2nd level)", got)
	}
	got2 := puppetUpgradeAutoGrantedEntrySlugs("Green", 2)
	if !got2[jutsuChanneler] || got2[jutsuSpecialization] {
		t.Errorf("Green level 2 = %v, want only Jutsu Channeler", got2)
	}
	got6 := puppetUpgradeAutoGrantedEntrySlugs("Green", 6)
	if !got6[jutsuChanneler] || !got6[jutsuSpecialization] || len(got6) != 2 {
		t.Errorf("Green level 6 = %v, want both entries", got6)
	}
	if got := puppetUpgradeAutoGrantedEntrySlugs("Purple", 20); len(got) != 0 {
		t.Errorf("Purple level 20 = %v, want empty (Green-only grant)", got)
	}
}

// TestLoadPuppetsTabDataFoundationAbilityScores reproduces the audit's live
// scenario end-to-end through loadPuppetsTabData: a companion with a
// pre-existing, non-null (and now stale) str_score picks Quadrupedal
// (Puppeteer Chassis), which sets STR/CON to 16. Both halves of the fix are
// checked — the ExpectedStr hint that drives the Sync button now resolves
// to a real, non-zero value (previously never forwarded into the template
// at all, so the Sync button never rendered), and the auto-generated Bite
// attack uses the CORRECTED score rather than the stale stored one.
func TestLoadPuppetsTabDataFoundationAbilityScores(t *testing.T) {
	s := testServer(t)
	seedPuppetMasterRules(t, s)
	if _, err := s.rulesDB.Exec(`
		INSERT INTO puppet_tool_stat_block (class_slug, creature_type, proficiency_rule_text,
			hp_base, hp_con_bonus_add, speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
			passive_perception, traits_text)
		VALUES ('class/puppet-master', 'Medium Construct', 'Puppet Master''s Proficiency',
			4, 5, 30, 8, 12, 8, 3, 8, 6, 9, '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kankuro', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/puppet-master', 20, 0)`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Chibi")
	if err != nil {
		t.Fatal(err)
	}
	// The pre-existing, already-non-null (and now stale) score the audit's
	// live reproduction found: COALESCE-only backfill never touches this
	// once it's set, so it stays 15 forever without the fix. str_score has
	// no dedicated setter (companionIntFields only covers AC/HP/jutsu-slots/
	// barrier), so this is set directly.
	if _, err := s.charDB.Exec(`UPDATE character_companions SET str_score = 15 WHERE id = ?`, companionID); err != nil {
		t.Fatal(err)
	}
	quadrupedalSlug := "class/puppet-master/option/puppeteer-chassis/quadrupedal"
	if _, err := charstore.AddCompanionUpgrade(s.charDB, 1, companionID, quadrupedalSlug, quadrupedalSlug); err != nil {
		t.Fatal(err)
	}

	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.loadPuppetsTabData(1, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Companions) != 1 {
		t.Fatalf("got %d companions, want 1", len(data.Companions))
	}
	view := data.Companions[0]

	if view.ExpectedStr != 16 {
		t.Errorf("ExpectedStr = %d, want 16 — without this, the dict-key fix, the Sync STR button never renders at all", view.ExpectedStr)
	}
	if view.ExpectedCon != 16 {
		t.Errorf("ExpectedCon = %d, want 16", view.ExpectedCon)
	}
	if view.ExpectedSize != "Large" {
		t.Errorf("ExpectedSize = %q, want Large", view.ExpectedSize)
	}

	var bite *companionAttackRow
	for i := range view.Attacks {
		if view.Attacks[i].Name == "Bite" {
			bite = &view.Attacks[i]
		}
	}
	if bite == nil {
		t.Fatal("no auto-generated Bite attack from the Quadrupedal Foundation pick")
	}
	// Str 16 (mod +3) + level-20 Puppet Master's own +9 proficiency bonus —
	// NOT the stale stored Str 15's mod +2 (which would give +11).
	if bite.AttackTotal != 12 {
		t.Errorf("Bite AttackTotal = %d, want 12 (corrected Str 16's mod +3, + prof +9) — a modifier of 11 means it's still reading the stale stored Str 15", bite.AttackTotal)
	}
}

// TestPuppetArmorChassisGatedToPurpleTechnique reproduces the audit's live
// finding: a Black Technique character's companion had armor_chassis =
// 'Wooden Suit' set (bypassing the UI, which never should have offered it),
// and loadPuppetsTabData's Sync-AC hint computed AC using Purple Technique
// Juggernaut's own exclusive chassis formula instead of the ordinary
// puppetToolDefaultAC every other subclass's Puppet Tool uses. Covers both
// directions: a non-Purple subclass gets neither the chassis picker options
// nor the chassis AC formula even with a stale armor_chassis value already
// stored; a Purple Technique character gets both.
func TestPuppetArmorChassisGatedToPurpleTechnique(t *testing.T) {
	const purpleSlug = "class/puppet-master/group/puppet-techniques/purple-technique-juggernaut"

	cases := []struct {
		name          string
		subclassSlug  string
		wantOptions   bool
		wantChassisAC bool
	}{
		{"Blue Technique gets no chassis options or formula", "class/puppet-master/group/puppet-techniques/blue-technique-warmaster", false, false},
		{"Purple Technique gets both", purpleSlug, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			seedPuppetMasterRules(t, s)
			if _, err := s.rulesDB.Exec(`
				INSERT INTO subclasses (slug, group_slug, name) VALUES
					(?, 'class/puppet-master/group/puppet-techniques', 'Purple Technique ~ Juggernaut')`,
				purpleSlug); err != nil {
				t.Fatal(err)
			}
			if _, err := s.rulesDB.Exec(`
				INSERT INTO puppet_tool_stat_block (class_slug, creature_type, proficiency_rule_text,
					hp_base, hp_con_bonus_add, speed, str_score, dex_score, con_score, int_score, wis_score, cha_score,
					passive_perception, traits_text)
				VALUES ('class/puppet-master', 'Medium Construct', 'Puppet Master''s Proficiency',
					4, 5, 30, 8, 12, 8, 3, 8, 6, 9, '')`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.charDB.Exec(`
				INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
				VALUES ('Sasori', 10, 10, 10, 10, 10, 10)`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.charDB.Exec(`
				INSERT INTO character_classes (character_id, class_slug, levels, order_index)
				VALUES (1, 'class/puppet-master', 20, 0)`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.charDB.Exec(`
				INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
				VALUES (1, ?, 2)`, c.subclassSlug); err != nil {
				t.Fatal(err)
			}
			companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Hiruko")
			if err != nil {
				t.Fatal(err)
			}
			// Set directly rather than through handleCompanionSave — this
			// models both a stale pre-fix value and a raw POST bypassing the
			// popup's own gate, either of which the AC computation itself
			// must still not trust for a non-Purple character.
			if _, err := s.charDB.Exec(`UPDATE character_companions SET armor_chassis = 'Steel Fortress' WHERE id = ?`, companionID); err != nil {
				t.Fatal(err)
			}

			sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
			if err != nil {
				t.Fatal(err)
			}
			data, err := s.loadPuppetsTabData(1, sheet)
			if err != nil {
				t.Fatal(err)
			}
			if gotOptions := len(data.ArmorChassisOptions) > 0; gotOptions != c.wantOptions {
				t.Errorf("ArmorChassisOptions non-empty = %v, want %v", gotOptions, c.wantOptions)
			}
			if len(data.Companions) != 1 {
				t.Fatalf("got %d companions, want 1", len(data.Companions))
			}
			view := data.Companions[0]
			// Steel Fortress: 10 + ac_bonus 8 + Dex mod ('none' mode, so +0) = 18.
			// Default formula: ac_base 10 (no ac_base seeded, COALESCEd) + level
			// 20's own +9 proficiency bonus = 19. The two formulas deliberately
			// land on different numbers so a formula mix-up shows up as a wrong
			// value, not a coincidentally-matching one.
			wantAC := 19
			if c.wantChassisAC {
				wantAC = 18
			}
			if view.ExpectedAC != wantAC {
				t.Errorf("ExpectedAC = %d, want %d (%s formula)", view.ExpectedAC, wantAC,
					map[bool]string{true: "Steel Fortress chassis", false: "default puppetToolDefaultAC"}[c.wantChassisAC])
			}
		})
	}
}

// TestPuppetToolDefaultAC locks in the book's own Puppet Tool card formula
// ("Armor Class 13 + your Proficiency Bonus (Natural Armor)") — the
// Puppet MASTER's own proficiency bonus, not the puppet's Dex modifier
// (confirmed against the source page image directly; this text isn't in
// the PDF's extracted text stream at all, see migration 0032).
func TestPuppetToolDefaultAC(t *testing.T) {
	cases := []struct {
		acBase, profBonus, want int
	}{
		{13, 2, 15},
		{13, 6, 19},
	}
	for _, c := range cases {
		if got := puppetToolDefaultAC(c.acBase, c.profBonus); got != c.want {
			t.Errorf("puppetToolDefaultAC(%d, %d) = %d, want %d", c.acBase, c.profBonus, got, c.want)
		}
	}
}

// TestPuppetMendingHealDie: "2d6 + half your Puppet Master level... This
// die increases to 2d8 at 9th level and 2d10 at 15th level."
func TestPuppetMendingHealDie(t *testing.T) {
	cases := []struct {
		level int
		want  string
	}{
		{1, "2d6"},
		{8, "2d6"},
		{9, "2d8"},
		{14, "2d8"},
		{15, "2d10"},
		{20, "2d10"},
	}
	for _, c := range cases {
		if got := puppetMendingHealDie(c.level); got != c.want {
			t.Errorf("puppetMendingHealDie(%d) = %q, want %q", c.level, got, c.want)
		}
	}
}

// Enhanced Durability's own per-technique formulas moved to
// internal/puppetupgrades together with every other modeled upgrade bonus;
// their tests live beside them there (bonuses_test.go).

// TestPuppetIntegratedWeaponAttack: "these weapons use your Ninjutsu/
// Taijutsu modifier" — the higher of the two, not a fixed one, and each
// choice's own damage die/type from the book ("Arm Blades... 1d6...
// slashing", "Axe Tail... 1d8... piercing").
func TestPuppetIntegratedWeaponAttack(t *testing.T) {
	sheet := &charsheet.Sheet{
		JutsuAttacks: []charsheet.JutsuAttack{
			{Kind: "Ninjutsu", Modifier: 3},
			{Kind: "Genjutsu", Modifier: 10}, // must be ignored — not Ninjutsu/Taijutsu
			{Kind: "Taijutsu", Modifier: 6},
			{Kind: "Bukijutsu", Modifier: 1},
		},
	}
	armBlades := puppetIntegratedWeaponAttack(sheet, "arm-blades")
	if armBlades == nil {
		t.Fatal("arm-blades returned nil")
	}
	if armBlades.Name != "Arm Blades" || armBlades.DamageSides != 6 || armBlades.DamageType != "slashing" {
		t.Errorf("Arm Blades = %+v, want Name=Arm Blades DamageSides=6 DamageType=slashing", armBlades)
	}
	if armBlades.AttackTotal != 6 || armBlades.DamageTotal != 6 {
		t.Errorf("Arm Blades AttackTotal/DamageTotal = %d/%d, want 6/6 (higher of Ninjutsu 3, Taijutsu 6)", armBlades.AttackTotal, armBlades.DamageTotal)
	}

	axeTail := puppetIntegratedWeaponAttack(sheet, "axe-tail")
	if axeTail == nil {
		t.Fatal("axe-tail returned nil")
	}
	if axeTail.Name != "Axe Tail" || axeTail.DamageSides != 8 || axeTail.DamageType != "piercing" {
		t.Errorf("Axe Tail = %+v, want Name=Axe Tail DamageSides=8 DamageType=piercing", axeTail)
	}

	if got := puppetIntegratedWeaponAttack(sheet, "not-a-real-choice"); got != nil {
		t.Errorf("unknown choice slug = %+v, want nil", got)
	}
	if got := puppetIntegratedWeaponAttack(nil, "arm-blades"); got != nil {
		t.Errorf("nil sheet = %+v, want nil", got)
	}
}

// TestPuppetUpgradeEffectiveCap locks in the worked example from Puppet
// Upgrades' own closing rule: "at 6th level, you choose to either take five
// Wood tier Upgrades, and one Bronze tier Upgrade, or simply take six Wood
// tier Upgrades" — level 6's real class table caps are Wood=4, Bronze=2 (see
// v_class_level_resources), so both of those end states require converting
// at least one Bronze slot down into a Wood pick.
func TestPuppetUpgradeEffectiveCap(t *testing.T) {
	caps := [puppetUpgradeTierCount + 1]int{0: 0, 1: 4, 2: 2} // Wood=4, Bronze=2, level 6

	t.Run("wood cap with nothing taken yet allows full conversion", func(t *testing.T) {
		used := [puppetUpgradeTierCount + 1]int{}
		if got := puppetUpgradeEffectiveCap(1, caps, used); got != 6 {
			t.Errorf("Wood effective cap = %d, want 6 (4 own + 2 converted from Bronze)", got)
		}
	})

	t.Run("bronze cap never grows from unused wood slots", func(t *testing.T) {
		used := [puppetUpgradeTierCount + 1]int{}
		if got := puppetUpgradeEffectiveCap(2, caps, used); got != 2 {
			t.Errorf("Bronze effective cap = %d, want 2 (conversion only flows downward)", got)
		}
	})

	t.Run("five Wood plus one Bronze is a legal end state", func(t *testing.T) {
		used := [puppetUpgradeTierCount + 1]int{1: 5, 2: 1}
		if woodCap := puppetUpgradeEffectiveCap(1, caps, used); used[1] > woodCap {
			t.Errorf("5 Wood picks exceeds effective Wood cap %d", woodCap)
		}
		if bronzeCap := puppetUpgradeEffectiveCap(2, caps, used); used[2] > bronzeCap {
			t.Errorf("1 Bronze pick exceeds effective Bronze cap %d", bronzeCap)
		}
	})

	t.Run("six Wood and zero Bronze is a legal end state", func(t *testing.T) {
		used := [puppetUpgradeTierCount + 1]int{1: 6, 2: 0}
		if woodCap := puppetUpgradeEffectiveCap(1, caps, used); used[1] > woodCap {
			t.Errorf("6 Wood picks exceeds effective Wood cap %d", woodCap)
		}
	})

	t.Run("a seventh total pick is never reachable", func(t *testing.T) {
		// 4 Wood already at their own cap, 2 Bronze already at their own
		// cap — every slot in the ladder is spent, so a 5th Wood pick
		// (via a would-be 3rd Bronze-turned-Wood conversion) must be
		// rejected: there is no more capacity anywhere to convert.
		used := [puppetUpgradeTierCount + 1]int{1: 4, 2: 2}
		if woodCap := puppetUpgradeEffectiveCap(1, caps, used); used[1] < woodCap {
			t.Errorf("Wood effective cap = %d, want exactly %d once every slot in the ladder is spent", woodCap, used[1])
		}
	})
}

// TestPuppetUpgradeNonMaterialTierUsagePoolsAcrossUntieredSiblings locks in
// the mutual-exclusivity fix: three untiered class_options rows sharing one
// list_name (the real shape of Puppet Weapon Types/Puppeteer Chassis/Puppet
// Frameworks/Puppet Roles — one row per option, no class_option_entries of
// their own) must share ONE real "pick one" cap, not three independent
// cap-1 tiers. Before this fix, picking Drone Weapon left Ogre Weapon's own
// usage count at 0, so nothing stopped also picking it.
func TestPuppetUpgradeNonMaterialTierUsagePoolsAcrossUntieredSiblings(t *testing.T) {
	s := testServer(t)

	if _, err := s.rulesDB.Exec(`INSERT INTO classes (slug, name) VALUES ('class/puppet-master', 'Puppet Master')`); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"drone-weapon", "ogre-weapon", "sentinel-weapon"} {
		full := "class/puppet-master/option/puppet-weapon-types/" + slug
		if _, err := s.rulesDB.Exec(`
			INSERT INTO class_options (slug, class_slug, list_name, name, description)
			VALUES (?, 'class/puppet-master', 'Puppet Weapon Types', ?, 'test')`, full, slug); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Warmaster')`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Karasu")
	if err != nil {
		t.Fatal(err)
	}
	droneSlug := "class/puppet-master/option/puppet-weapon-types/drone-weapon"
	ogreSlug := "class/puppet-master/option/puppet-weapon-types/ogre-weapon"

	usedBefore, err := s.puppetUpgradeNonMaterialTierUsage(1, companionID, droneSlug, "Drone Weapon")
	if err != nil {
		t.Fatal(err)
	}
	if usedBefore != 0 {
		t.Fatalf("usage before any pick = %d, want 0", usedBefore)
	}

	if _, err := charstore.AddCompanionUpgrade(s.charDB, 1, companionID, droneSlug, droneSlug); err != nil {
		t.Fatal(err)
	}

	// Checking the SIBLING row's own usage (Ogre Weapon) must ALSO reflect
	// the Drone Weapon pick — that's the whole fix.
	usedForSibling, err := s.puppetUpgradeNonMaterialTierUsage(1, companionID, ogreSlug, "Ogre Weapon")
	if err != nil {
		t.Fatal(err)
	}
	if usedForSibling != 1 {
		t.Fatalf("usage for sibling Ogre Weapon after picking Drone Weapon = %d, want 1 (shared cap already spent)", usedForSibling)
	}
}

// TestPuppetUpgradePickIsPermanent locks in which entries are permanent,
// no-take-backs choices (Elemental Reactor, Integrated Weapon, Mastered
// Armament — every entry whose own sub-choice must be unique across picks)
// versus an ordinary, freely-removable upgrade pick.
func TestPuppetUpgradePickIsPermanent(t *testing.T) {
	for _, slug := range []string{elementalReactorEntrySlug, integratedWeaponEntrySlug, masteredArmamentEntrySlug} {
		if !puppetUpgradePickIsPermanent(slug) {
			t.Errorf("%s: want permanent", slug)
		}
	}
	if puppetUpgradePickIsPermanent("class/puppet-master/option/black-iron-upgrades/wood-tier/entry/poison-mist-hell") {
		t.Error("Poison Mist Hell: want NOT permanent (its own tags are meant to be swapped)")
	}
}

// TestHandlePuppetUpgradeDeleteRejectsPermanentPick: the server-side gate,
// not just the template hiding the button — a POST to delete an already-
// taken Integrated Weapon pick must be rejected outright, since the book's
// own text ("select one... take this upgrade a second time to gain the
// OTHER") reads as a standing, permanent choice.
func TestHandlePuppetUpgradeDeleteRejectsPermanentPick(t *testing.T) {
	s := testServer(t)
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Kankuro')`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Chibi")
	if err != nil {
		t.Fatal(err)
	}
	upgradeID, err := charstore.AddCompanionUpgrade(s.charDB, 1, companionID, integratedWeaponEntrySlug, integratedWeaponEntrySlug)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/characters/1/companions/1/upgrades/1/delete", nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", strconv.FormatInt(companionID, 10))
	req.SetPathValue("uid", strconv.FormatInt(upgradeID, 10))
	rr := httptest.NewRecorder()
	s.handlePuppetUpgradeDelete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	picks, err := charstore.ListCompanionUpgrades(s.charDB, 1, companionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 {
		t.Fatalf("picks after rejected delete = %d, want 1 (still there)", len(picks))
	}
}

// TestPuppetUpgradeAvailableToColor locks in the "Techniques: X, Y, Z"
// parser against real book shapes: a plain All entry, a subset naming
// every color except one (Undying Form's real "Techniques: Black, Blue,
// Green, Red, White" — every technique except Purple), a single-color
// entry, and the "Perfect" qualifier (not a 7th subclass) trailing a real
// color list.
func TestPuppetUpgradeAvailableToColor(t *testing.T) {
	cases := []struct {
		name, description, color string
		want                     bool
	}{
		{"All available to Purple", "Techniques: All A weapon first created by...", "Purple", true},
		{"All available to Black", "Techniques: All A weapon first created by...", "Black", true},
		{"subset excludes Purple", "Techniques: Black, Blue, Green, Red, White Your force of will continues...", "Purple", false},
		{"subset includes Black", "Techniques: Black, Blue, Green, Red, White Your force of will continues...", "Black", true},
		{"single color match", "Techniques: Purple, Perfect You integrate physical...", "Purple", true},
		{"single color mismatch", "Techniques: Purple, Perfect You integrate physical...", "Blue", false},
		{"no Techniques line always available", "ASI: Wisdom score becomes 14...", "Blue", true},
		{"no subclass chosen yet excludes a color-scoped entry", "Techniques: Purple You integrate...", "", false},
		{"no subclass chosen yet still allows All", "Techniques: All A weapon first created by...", "", true},
	}
	for _, c := range cases {
		if got := puppetUpgradeAvailableToColor(c.description, c.color); got != c.want {
			t.Errorf("%s: puppetUpgradeAvailableToColor(%q, %q) = %v, want %v", c.name, c.description, c.color, got, c.want)
		}
	}
}

// TestBattleReadyArmorSealOptions: only Armor Seals (not Weapon Seals) at
// or below the character's own qualifying rank are offered.
func TestBattleReadyArmorSealOptions(t *testing.T) {
	s := testServer(t)
	seed := []struct{ slug, name, kind, appliesTo, rank string }{
		{"seal/armor/basic/strength-seal", "Strength Seal (Basic)", "enhancement_seal", "armor", "D"},
		{"seal/armor/refined/strength-seal", "Strength Seal (Refined)", "enhancement_seal", "armor", "C"},
		{"seal/armor/greater/strength-seal", "Strength Seal (Greater)", "enhancement_seal", "armor", "B"},
		{"seal/armor/superior/strength-seal", "Strength Seal (Superior)", "enhancement_seal", "armor", "A"},
		{"seal/weapon/basic/sharpness-seal", "Sharpness Seal (Basic)", "enhancement_seal", "weapon", "D"},
	}
	for _, r := range seed {
		if _, err := s.rulesDB.Exec(
			`INSERT INTO equipment (slug, name, kind, seal_applies_to, seal_rank) VALUES (?, ?, ?, ?, ?)`,
			r.slug, r.name, r.kind, r.appliesTo, r.rank); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Kankuro')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(
		`INSERT INTO character_classes (character_id, class_slug, levels, order_index) VALUES (1, 'class/puppet-master', 6, 0)`); err != nil {
		t.Fatal(err)
	}

	got, err := battleReadyArmorSealOptions(s, 1, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, o := range got {
		names = append(names, o.Name)
	}
	want := []string{"Strength Seal (Basic)", "Strength Seal (Refined)"} // level 6 -> C-Rank qualifying
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v (level 6 qualifies up to C-Rank, and no weapon seals)", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("got %v, want %v", names, want)
		}
	}
}

// TestPuppetChassisPropertyBulkBonus covers Item 4's bulk half: a Steel
// Fortress chassis worn as armor grants the character's max bulk a +10
// bonus from its Powerful Build property; a chassis without that property,
// or no puppet worn as armor at all, grants nothing.
func TestPuppetChassisPropertyBulkBonus(t *testing.T) {
	cases := []struct {
		name      string
		chassis   string
		wornArmor bool
		wantBonus float64
	}{
		{"Steel Fortress worn grants +10", "Steel Fortress", true, 10},
		{"Weaved Mail worn grants nothing (no Powerful Build)", "Weaved Mail", true, 0},
		{"Steel Fortress not worn grants nothing", "Steel Fortress", false, 0},
		{"no chassis picked yet grants nothing", "", true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Chassis Bulk Test')`); err != nil {
				t.Fatal(err)
			}
			isArmorForm := 0
			if c.wornArmor {
				isArmorForm = 1
			}
			if _, err := s.charDB.Exec(
				`INSERT INTO character_companions (character_id, kind, name, armor_chassis, is_armor_form, sort_order)
				 VALUES (1, 'puppet', 'Juggernaut Armor', ?, ?, 0)`, c.chassis, isArmorForm,
			); err != nil {
				t.Fatal(err)
			}
			got, err := s.puppetChassisPropertyBulkBonus(1)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.wantBonus {
				t.Errorf("puppetChassisPropertyBulkBonus = %v, want %v", got, c.wantBonus)
			}
		})
	}
}

// TestPuppetWarfareAugmentationMaxTakes locks in Red Technique's own
// worked example: "this upgrade is given to 2 Puppet Tools you possess
// when acquired... You can select this upgrade again if you acquire 2
// more Puppets" — floor(companionCount/2), never negative or fractional.
func TestPuppetWarfareAugmentationMaxTakes(t *testing.T) {
	cases := []struct {
		companionCount, want int
	}{
		{0, 0}, {1, 0}, {2, 1}, {3, 1}, {4, 2}, {5, 2}, {6, 3},
	}
	for _, c := range cases {
		if got := puppetWarfareAugmentationMaxTakes(c.companionCount); got != c.want {
			t.Errorf("puppetWarfareAugmentationMaxTakes(%d) = %d, want %d", c.companionCount, got, c.want)
		}
	}
}
