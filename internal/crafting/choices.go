package crafting

// ChoiceKind is what a seal asks the crafter to pick when it is imbued.
type ChoiceKind string

const (
	// ChoiceDamageType: one damage type (Elemental/Medical seals).
	ChoiceDamageType ChoiceKind = "damage_type"
	// ChoiceCreatureTypes: creature types the seal targets (Bane/Hunter).
	ChoiceCreatureTypes ChoiceKind = "creature_types"
	// ChoiceSummonTribe: a summon tribe of an allowed summon type (Fanged).
	ChoiceSummonTribe ChoiceKind = "summon_tribe"
	// ChoiceJutsu: a jutsu stored in the weapon (Storing).
	ChoiceJutsu ChoiceKind = "jutsu"
)

// ChoiceRule describes the pick a seal requires. Options holds option IDs
// for damage and creature types (i18n keys crafting.damage.<id> and
// crafting.creature.<id>); summon and jutsu picks are slugs, validated
// against the catalog.
type ChoiceRule struct {
	Kind         ChoiceKind
	Count        int
	Options      []string
	SummonTypes  []string // ChoiceSummonTribe: allowed summon_tribes.summon_type values
	MaxJutsuRank string   // ChoiceJutsu: highest storable rank
}

var (
	natureReleaseDamage = []string{"earth", "wind", "fire", "cold", "lightning"}
	medicalReleaseDamage = []string{"acid", "necrotic", "poison"}
	baneCreatures        = []string{"aberration", "beast", "celestial", "monstrosity", "mutant", "plant", "sage_creature", "undead"}
	hunterCreatures      = []string{"aberration", "beast", "celestial", "construct", "demon", "monstrosity", "mutant", "plant", "sage_creature", "undead"}
	hunterSuperiorCreatures = []string{"aberration", "beast", "celestial", "construct", "demon", "monstrosity", "mutant", "otsutsuki", "plant", "sage_creature", "undead"}
)

// sealChoices lists every seal whose text asks for a choice when it is added,
// transcribed from the seal descriptions (pp. 73–82). TestSealChoicesCoverage
// fails if a seal's description asks to "select" something without an entry
// here.
var sealChoices = map[string]ChoiceRule{
	"seal/weapon/minor/elemental-seal":       {Kind: ChoiceDamageType, Count: 1, Options: natureReleaseDamage},
	"seal/weapon/greater/elemental-seal":     {Kind: ChoiceDamageType, Count: 1, Options: natureReleaseDamage},
	"seal/weapon/mastercraft/elemental-seal": {Kind: ChoiceDamageType, Count: 1, Options: natureReleaseDamage},
	"seal/armor/minor/elemental-seal":        {Kind: ChoiceDamageType, Count: 1, Options: natureReleaseDamage},
	"seal/armor/greater/elemental-seal":      {Kind: ChoiceDamageType, Count: 1, Options: natureReleaseDamage},
	"seal/armor/mastercraft/elemental-seal":  {Kind: ChoiceDamageType, Count: 1, Options: natureReleaseDamage},

	"seal/weapon/minor/medical-seal":       {Kind: ChoiceDamageType, Count: 1, Options: medicalReleaseDamage},
	"seal/weapon/greater/medical-seal":     {Kind: ChoiceDamageType, Count: 1, Options: medicalReleaseDamage},
	"seal/weapon/mastercraft/medical-seal": {Kind: ChoiceDamageType, Count: 1, Options: medicalReleaseDamage},
	"seal/armor/minor/medical-seal":        {Kind: ChoiceDamageType, Count: 1, Options: medicalReleaseDamage},
	"seal/armor/greater/medical-seal":      {Kind: ChoiceDamageType, Count: 1, Options: medicalReleaseDamage},
	"seal/armor/mastercraft/medical-seal":  {Kind: ChoiceDamageType, Count: 1, Options: medicalReleaseDamage},

	"seal/weapon/minor/bane-seal":       {Kind: ChoiceCreatureTypes, Count: 2, Options: baneCreatures},
	"seal/weapon/refined/hunter-seal":   {Kind: ChoiceCreatureTypes, Count: 2, Options: hunterCreatures},
	"seal/weapon/superior/hunter-seal":  {Kind: ChoiceCreatureTypes, Count: 2, Options: hunterSuperiorCreatures},
	"seal/weapon/minor/fanged-seal":     {Kind: ChoiceSummonTribe, Count: 1, SummonTypes: []string{"Carnivoran"}},
	"seal/weapon/greater/fanged-seal":   {Kind: ChoiceSummonTribe, Count: 1, SummonTypes: []string{"Avian", "Carnivoran"}},
	"seal/weapon/refined/storing-seal":  {Kind: ChoiceJutsu, Count: 1, MaxJutsuRank: "D"},
	"seal/weapon/superior/storing-seal": {Kind: ChoiceJutsu, Count: 1, MaxJutsuRank: "B"},
}

// storableClassifications are the jutsu classifications a Storing Seal
// accepts: "Select one Ninjutsu, Genjutsu or Bukijutsu".
var storableClassifications = []string{"Ninjutsu", "Genjutsu", "Bukijutsu"}

// dieDamageFamilies are weapon seal families whose bonus is extra damage
// dice. Greater and Mastercraft Elemental/Medical weapon seals say their own
// dice bonus "does not stack with other Seals that grant dice-based bonuses
// to damage rolls".
var dieDamageFamilies = map[string]bool{
	"bane-seal": true, "hunter-seal": true, "bloody-seal": true, "chilling-seal": true,
	"corrosive-seal": true, "crushing-seal": true, "hot-seal": true, "shocking-seal": true,
	"sickening-seal": true, "laceration-seal": true,
}
