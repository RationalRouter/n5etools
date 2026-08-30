package parse

import (
	"strings"
	"testing"
)

// Real text confirmed against dist/rules.db (2026-08-30), quoted verbatim.

const puppetToolGluedText = `Beginning at 1st level, you craft a Puppet Tool to carry out your orders and protect you. Your Puppet Tool starts with the statistics below; You learn the Mending E-Rank Ninjutsu, which does not count against your known. Medium Construct, Proficiency = Puppet Master’s Proficiency Hit Points 4 + [(5+Constitution Modifier) x Puppet Master level] Speed 30 ft. 15 (+2) 13 (+1) 13 (+1) 5 (-3) 5 (-3) 5 (-3) Senses Passive Perception 7 Bound. A Puppet is bound to its user via chakra threads and can be no more than 500ft. from you. Hollow Shell. Puppet Tools cannot be affected by Genjutsu. Mechanical Limits. Puppets cannot cast jutsu (or have such jutsu cast through them), or use effects, that increase its ranks of exhaustion or make Clones.`

const titanGluedText = `The Ronin Titans, also sometimes referred to as Stryder Titans, focus on mobility, able to move around the battlefield at breakneck speeds. X Construct, Proficiency bonus = your proficiency bonus, unaligned Hit Points 20+[2*Titan’s Constitution Modifer x Science Nin level] Speed 30 ft. 15 (+2) 13 (+1) 13 (+1) 5 (-3) 5 (-3) 5 (-3) Senses Darkvision(30 feet), Passive Perception( Yours + Intelligence Modifer) Battery Powered Barrier: All Titans are fitted with a barrier that protects the titan from damage. Extra Attack. Your Titan can attack twice with the attack action.`

// The Draconic Gauntlet's real, still-unfixed description (confirmed present
// in dist/rules.db as of this bug report) — the reported bug this whole file
// exists to catch.
const draconicGauntletGluedText = `You forge a mighty gauntlet using your element, granting your fists the strength of dragons. Lastly, once per round when you make an Unarmed attack with this weapon, you may spend increments of 5 chakras from your CCD to move up to 5 feet per 5 chakras spent. ASCENSION: DRACONIC GAUNTLET If the Draconic Gauntlet is Ascended, you use your element to create and give life to a small Whelp, a tiny youngling dragon. The Whelp rests perched on your Draconic Gauntlet and uses the following statistics; Small Beast, Proficiency = Equal to yours Armor ClassDR Hit Points Your Intelligence modifier x your Science-Nin Level Speed 60ft (Flying) (Can hover) STRDEX CON INT WIS CHA 18 (+4) 20 (+5) 16 (+3) 20 (+5) 5 (-3) 5 (-3) Saving Throws Proficient in All (Treat negative modifiers as +0) Damage Resistance Psychic, Force Condition Immunities All Sensory Senses Passive Perception 10 Elemental Existence. The Whelp rests perched on the Draconic Gauntlet. Aether Fireball (Cost: 5 Chakra). Ranged Weapon Attack: Range 90 feet, one target. Hit: 4d10+4 fire damage.`

// The S.N.B Specialist's real, still-unfixed "Combat Programming" feature
// description (confirmed present in dist/rules.db) — an instance of this bug
// class documented in CLASS_AUDIT.md but never given a dedicated fix, unlike
// Puppet Tool/Titan above. A generic detector should catch it without any
// S.N.B-specific code.
const snbCombatProgrammingGluedText = `Also at 6th level, you have improved upon your Scientific Ninja Beast’s programing giving it a definitive roll in combat. Choose one of the following: • Striker: Your S.N.B gains the Multiattack trait. Medium Construct Armor Class 10 + Your Proficiency Bonus (Natural Armor) Hit Points Your Intelligence modifier + your Science-Nin level times 5 Speed 30 ft. STR DEX CON INT WIS CHA 12 (+1) 14 (+2) 12 (+1) 8 (-1) 5 (-3) 5 (-3) Damage Immunity Psychic, Poison Damage Resistances Acid Damage Resistances Charmed, Exhaustion, Frightened, Poisoned Senses Dark Vision (30 feet) passive Perception 7 Artificial Intelligence. The Scientific Ninja Beast is programmed to respond only to its creator.`

func TestSplitStatBlockPuppetTool(t *testing.T) {
	m := SplitStatBlock(puppetToolGluedText)
	if !m.Found {
		t.Fatal("expected a stat block to be found")
	}
	if strings.Contains(m.Prose, "Medium Construct") {
		t.Errorf("Prose still contains the stat block: %q", m.Prose)
	}
	if !strings.HasPrefix(m.RawStatBlock, "Medium Construct") {
		t.Errorf("RawStatBlock = %q, want it to start with \"Medium Construct\"", m.RawStatBlock)
	}
	f := m.Fields
	if f.CreatureType != "Medium Construct" {
		t.Errorf("CreatureType = %q", f.CreatureType)
	}
	if f.Str != 15 || f.Dex != 13 || f.Con != 13 || f.Int != 5 || f.Wis != 5 || f.Cha != 5 {
		t.Errorf("ability scores = %+v", f)
	}
	if f.Speed != 30 {
		t.Errorf("Speed = %d, want 30", f.Speed)
	}
	if f.HPFormulaText != "4 + [(5+Constitution Modifier) x Puppet Master level]" {
		t.Errorf("HPFormulaText = %q", f.HPFormulaText)
	}
	if f.Senses != "Passive Perception 7" {
		t.Errorf("Senses = %q", f.Senses)
	}
	if !strings.HasPrefix(f.TraitsAndAttacksText, "Bound. A Puppet is bound") {
		t.Errorf("TraitsAndAttacksText = %q", f.TraitsAndAttacksText)
	}
	// AC is never printed in this stat block — confirmed absent, not merely
	// unparsed (PuppetToolStatBlock's own doc comment).
	if f.AC != nil {
		t.Errorf("AC = %v, want nil (no AC is printed for Puppet Tool)", *f.AC)
	}
}

func TestSplitStatBlockTitan(t *testing.T) {
	m := SplitStatBlock(titanGluedText)
	if !m.Found {
		t.Fatal("expected a stat block to be found")
	}
	if strings.Contains(m.Prose, "Construct") {
		t.Errorf("Prose still contains the stat block: %q", m.Prose)
	}
	if !strings.HasPrefix(m.RawStatBlock, "X Construct") {
		t.Errorf("RawStatBlock = %q, want it to start with \"X Construct\"", m.RawStatBlock)
	}
	f := m.Fields
	if f.CreatureType != "X Construct" {
		t.Errorf("CreatureType = %q, want the sourcebook's own broken \"X Construct\"", f.CreatureType)
	}
	if f.Str != 15 || f.Cha != 5 {
		t.Errorf("ability scores = %+v", f)
	}
	// Titan's own Passive Perception clause has no digit in it ("Yours +
	// Intelligence Modifer") — the Senses/traits split can't anchor on a
	// number here, so this is a known, accepted limitation: the whole tail
	// stays in Senses rather than being further split. RawStatBlock still
	// has everything verbatim regardless.
	if !strings.Contains(f.Senses, "Battery Powered Barrier") {
		t.Errorf("Senses = %q, want the unsplit tail (no digit to anchor on)", f.Senses)
	}
}

func TestSplitStatBlockDraconicGauntletWhelp(t *testing.T) {
	m := SplitStatBlock(draconicGauntletGluedText)
	if !m.Found {
		t.Fatal("expected a stat block to be found — this is the reported bug")
	}
	if strings.Contains(m.Prose, "Armor Class") || strings.Contains(m.Prose, "STRDEX") {
		t.Errorf("Prose still contains the stat block: %q", m.Prose)
	}
	if !strings.HasSuffix(m.Prose, "uses the following statistics;") {
		t.Errorf("Prose = %q, want it to end cleanly at \"uses the following statistics;\"", m.Prose)
	}
	f := m.Fields
	if f.CreatureType != "Small Beast" {
		t.Errorf("CreatureType = %q", f.CreatureType)
	}
	if f.Str != 18 || f.Dex != 20 || f.Con != 16 || f.Int != 20 || f.Wis != 5 || f.Cha != 5 {
		t.Errorf("ability scores = %+v", f)
	}
	if f.Speed != 60 {
		t.Errorf("Speed = %d, want 60", f.Speed)
	}
	// The AC value itself is lost at the extraction layer (confirmed: the
	// raw text reads "Armor ClassDR Hit Points" with no digit between them),
	// so AC should stay nil rather than a wrong guess.
	if f.AC != nil {
		t.Errorf("AC = %v, want nil (no recoverable AC digit in the extracted text)", *f.AC)
	}
	if f.Resistances != "Psychic, Force" {
		t.Errorf("Resistances = %q", f.Resistances)
	}
	if f.ConditionImmunities != "All Sensory" {
		t.Errorf("ConditionImmunities = %q", f.ConditionImmunities)
	}
	if f.Senses != "Passive Perception 10" {
		t.Errorf("Senses = %q", f.Senses)
	}
	if !strings.HasPrefix(f.TraitsAndAttacksText, "Elemental Existence.") {
		t.Errorf("TraitsAndAttacksText = %q", f.TraitsAndAttacksText)
	}
}

func TestSplitStatBlockSNBCombatProgramming(t *testing.T) {
	m := SplitStatBlock(snbCombatProgrammingGluedText)
	if !m.Found {
		t.Fatal("expected a stat block to be found — this is a previously-undetected instance of the bug")
	}
	if !strings.Contains(m.Prose, "Multiattack trait") {
		t.Errorf("Prose = %q, want the real Combat Programming prose preserved", m.Prose)
	}
	if strings.Contains(m.Prose, "Armor Class") {
		t.Errorf("Prose still contains the stat block: %q", m.Prose)
	}
	f := m.Fields
	if f.CreatureType != "Medium Construct" {
		t.Errorf("CreatureType = %q", f.CreatureType)
	}
	if f.AC == nil || *f.AC != 10 {
		t.Errorf("AC = %v, want 10", f.AC)
	}
	if f.Str != 12 || f.Dex != 14 || f.Con != 12 || f.Int != 8 || f.Wis != 5 || f.Cha != 5 {
		t.Errorf("ability scores = %+v", f)
	}
	if f.Immunities != "Psychic, Poison" {
		t.Errorf("Immunities = %q", f.Immunities)
	}
}

func TestSplitStatBlockNoFalsePositive(t *testing.T) {
	prose := "This is an ordinary feature description with no stat block in it at all. It mentions Hit Points and Saving Throws in passing, and even a Medium Construct in a metaphor, but never six ability scores in a row."
	m := SplitStatBlock(prose)
	if m.Found {
		t.Fatalf("expected no stat block match, got RawStatBlock = %q", m.RawStatBlock)
	}
	if m.Prose != prose {
		t.Errorf("Prose = %q, want the input returned untouched", m.Prose)
	}
}
