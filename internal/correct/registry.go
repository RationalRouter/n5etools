package correct

import (
	"reflect"

	"github.com/sergio/n5e/internal/parse"
)

// proseFields lists, per parsed struct type, which string fields hold
// free-form narrative text worth spell/grammar-checking. Deliberately an
// explicit allowlist, not a naming convention: fields like SpeedText,
// CostText, HitPointsText, AbilityRaw, or QuickBuild look like prose by
// name but are short, semi-structured rules text (or mix proper nouns/build
// shorthand) where a grammar tool is more likely to mangle a term of art
// than fix a real mistake. Name/Epithet/Keywords/Prerequisites and similar
// identifier-like fields are excluded for the same reason. Extend one line
// at a time as new entity types gain a genuine prose field.
var proseFields = map[reflect.Type][]string{
	reflect.TypeOf(parse.Clan{}):               {"Overview"},
	reflect.TypeOf(parse.ClanTrait{}):          {"Description"},
	reflect.TypeOf(parse.ClanFeature{}):        {"Description"},
	reflect.TypeOf(parse.Feat{}):               {"Description"},
	reflect.TypeOf(parse.Jutsu{}):              {"Description", "AtHigherRanks"},
	reflect.TypeOf(parse.Class{}):              {"Description"},
	reflect.TypeOf(parse.ClassFeature{}):       {"Description"},
	reflect.TypeOf(parse.ClassOption{}):        {"Description"},
	reflect.TypeOf(parse.OptionList{}):         {"Description"},
	reflect.TypeOf(parse.Subclass{}):           {"Description"},
	reflect.TypeOf(parse.SubclassGroup{}):      {"Description"},
	reflect.TypeOf(parse.BloodlineLatent{}):    {"Description"},
	reflect.TypeOf(parse.Background{}):         {"Description", "FeatureText", "ASIText", "Equipment"},
	reflect.TypeOf(parse.FightingStance{}):     {"Description"},
	reflect.TypeOf(parse.EnhancementSeal{}):    {"Description"},
	reflect.TypeOf(parse.SummonTribe{}):        {"Description", "JutsuSpecialty"},
	reflect.TypeOf(parse.SummonRole{}):         {"Description"},
	reflect.TypeOf(parse.SummonAttack{}):       {"Description"},
	reflect.TypeOf(parse.SummonFeature{}):      {"Description"},
	reflect.TypeOf(parse.AdversaryRole{}):      {"Description"},
	reflect.TypeOf(parse.AdversaryRoleTrait{}): {"Description"},
}
