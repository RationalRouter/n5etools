package charstore

import "testing"

func TestDeleteCharacterRemovesCompanionChildRows(t *testing.T) {
	db := testCharDB(t)

	companionID, err := AddCompanion(db, 1, "puppet", "Test Puppet")
	if err != nil {
		t.Fatal(err)
	}
	upgradeID, err := AddCompanionUpgrade(db, 1, companionID,
		"class/puppet-master/option/magus-upgrades/wood-tier", "class/puppet-master/option/magus-upgrades/wood-tier/entry/piercing-chakra")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO character_companion_upgrade_choices (companion_upgrade_id, choice_slug) VALUES (?, 'k')`, upgradeID); err != nil {
		t.Fatal(err)
	}
	if _, err := AddCompanionAttack(db, 1, companionID, CompanionAttack{Name: "Bash"}); err != nil {
		t.Fatal(err)
	}

	if err := DeleteCharacter(db, 1); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{
		"character_companions", "character_companion_upgrades",
		"character_companion_upgrade_choices", "character_companion_attacks",
	} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s: %d rows left after DeleteCharacter, want 0", table, n)
		}
	}
	var characters int
	if err := db.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&characters); err != nil {
		t.Fatal(err)
	}
	if characters != 0 {
		t.Errorf("characters: %d rows left, want 0", characters)
	}
}

func TestResetCharacterCreationKeepsIdentityWipesEverythingElse(t *testing.T) {
	db := testCharDB(t)

	if _, err := db.Exec(`
		UPDATE characters SET
			clan_slug = 'clan/uzumaki', background_slug = 'background/trouble-maker',
			base_str = 16, xp = 500, current_hp = 12, ryo = 300,
			portrait = 'data:image/png;base64,abc', notes = 'has a scar',
			appearance = 'tall', backstory = 'orphan', creation_status = 'complete'
		WHERE id = 1`); err != nil {
		t.Fatal(err)
	}

	companionID, err := AddCompanion(db, 1, "puppet", "Test Puppet")
	if err != nil {
		t.Fatal(err)
	}
	upgradeID, err := AddCompanionUpgrade(db, 1, companionID,
		"class/puppet-master/option/magus-upgrades/wood-tier", "class/puppet-master/option/magus-upgrades/wood-tier/entry/piercing-chakra")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO character_companion_upgrade_choices (companion_upgrade_id, choice_slug) VALUES (?, 'k')`, upgradeID); err != nil {
		t.Fatal(err)
	}
	if err := AddGeneralizedSkill(db, 1, "Stealth"); err != nil {
		t.Fatal(err)
	}

	if err := ResetCharacterCreation(db, 1); err != nil {
		t.Fatal(err)
	}

	var clanSlug, bgSlug any
	var baseStr, xp, currentHP int
	var ryo float64
	var portrait, notes, appearance, backstory, status string
	if err := db.QueryRow(`
		SELECT clan_slug, background_slug, base_str, xp, current_hp, ryo, portrait, notes, appearance, backstory, creation_status
		FROM characters WHERE id = 1`).Scan(&clanSlug, &bgSlug, &baseStr, &xp, &currentHP, &ryo, &portrait, &notes, &appearance, &backstory, &status); err != nil {
		t.Fatal(err)
	}
	if clanSlug != nil {
		t.Errorf("clan_slug = %v, want nil", clanSlug)
	}
	if bgSlug != nil {
		t.Errorf("background_slug = %v, want nil", bgSlug)
	}
	if baseStr != 10 || xp != 0 || currentHP != 0 || ryo != 0 {
		t.Errorf("base_str=%d xp=%d current_hp=%d ryo=%v, want all reset to zero/10", baseStr, xp, currentHP, ryo)
	}
	if status != "draft" {
		t.Errorf("creation_status = %q, want draft", status)
	}
	if portrait != "data:image/png;base64,abc" || notes != "has a scar" || appearance != "tall" || backstory != "orphan" {
		t.Errorf("identity/narrative fields were wiped, want kept: portrait=%q notes=%q appearance=%q backstory=%q", portrait, notes, appearance, backstory)
	}

	for _, table := range []string{
		"character_companions", "character_companion_upgrades",
		"character_companion_upgrade_choices", "character_generalized_skills",
	} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s: %d rows left after ResetCharacterCreation, want 0", table, n)
		}
	}
}

func TestResetCharacterCreationUnknownCharacter(t *testing.T) {
	db := testCharDB(t)
	if err := ResetCharacterCreation(db, 999); err == nil {
		t.Error("expected error resetting a nonexistent character, got nil")
	}
}
