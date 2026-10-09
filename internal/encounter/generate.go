package encounter

import (
	"fmt"
	"math/rand"
	"sort"
)

// Rank is one Bingo Book adversary rank template (Minion/Standard/Elite/
// Solo), as read from rules.db's adversary_ranks table.
type Rank struct {
	Slug        string
	Name        string
	HPFormula   string
	ACBonus     int
	SaveBonus   int
	SaveDCBonus int
	InitBonus   int
}

// Role is one of the five named Roles, as read from rules.db's
// adversary_roles table.
type Role struct {
	Slug string
	Name string
}

// Trait is one entry of the step-7 trait pool: an official Pack 1 Role
// Trait (rules.db adversary_role_traits, Source "official", Category
// "role") or a community Adversary Builder sheet trait
// (adversary_builder_sheet_traits, Source "sheet").
type Trait struct {
	Slug        string
	Name        string
	RoleSlug    string // official traits: the Pack 1 role
	Rank        string // D, C, B, A, S; "" for an unranked sheet trait
	Description string
	Source      string // "official" or "sheet"
	Category    string // general, role, clan, affiliation, class_mod, iconic
	Tag         string // sheet traits: the role/clan/affiliation/class mod tag
}

// FreeformSlot is one level-gated Freeform Attack/Jutsu slot-count row, as
// read from rules.db's adversary_freeform_slots table.
type FreeformSlot struct {
	RankSlug string
	LevelMin int
	Slots    int
}

// ClassificationModifier is one Optional Classification's (Iconic/Epic)
// flat stat bumps, as read from rules.db's adversary_classification_modifiers
// table (book p.10). HPChakraModifier and AbilityPoints have no existing
// numeric counterpart on GeneratedAdversary -- HPFormula is carried as
// printed text and never evaluated into a number anywhere in this codebase,
// and adversaries don't track individual ability scores at all -- so those
// two are surfaced as reference numbers for the GM to apply by hand, the
// same "kept as printed text" treatment HPFormula itself already gets.
type ClassificationModifier struct {
	Kind             string // "iconic" or "epic"
	HPChakraModifier int
	ACBonus          int
	AttackSaveBonus  int
	AbilityPoints    int
	InitiativeBonus  int
}

// TransformationExample mirrors one row of rules.db's
// adversary_transformation_examples table (book p.20) -- a GM-facing
// reference write-up a Solo adversary can optionally be tagged with.
type TransformationExample struct {
	Slug string
	Name string
}

// BattlefieldDomainExample mirrors one row of rules.db's
// adversary_battlefield_domains table (book pp.21-23) -- a GM-facing
// reference write-up a Solo adversary can optionally be tagged with.
// Mutually exclusive with a TransformationExample attachment (book p.21:
// "An Adversary should either use Transformations or Battlefield Domains,
// not both").
type BattlefieldDomainExample struct {
	Slug string
	Name string
}

// GeneratedTrait is one trait attached to a generated adversary, with its
// display data snapshotted at generation time.
type GeneratedTrait struct {
	Slug        string
	Name        string
	Rank        string
	Description string
	Source      string
	Category    string
}

// GeneratedAdversary is one adversary: the Adversary Builder choices it was
// built from (Choices turns them into a BuildChoices for Compute) plus the
// Pack 1 class values snapshotted at generation time. The legacy
// HPFormula/ACBonus/SaveBonus/SaveDCBonus/InitBonus fields are still stored
// for older readers; the stat block shown to the GM comes from Compute.
type GeneratedAdversary struct {
	Name          string
	RankSlug      string // the book's Class (adversary_ranks)
	RoleSlug      string // adversary_builder_roles
	Level         int
	HPFormula     string
	ACBonus       int
	SaveBonus     int
	SaveDCBonus   int
	InitBonus     int
	FreeformSlots int
	BudgetCost    int
	Traits        []GeneratedTrait
	Jutsu         []GeneratedJutsu

	// The book's Rank (E..S) and the remaining builder steps.
	AdvRank        string
	ClanTag        string
	AffiliationTag string
	ClassModTag    string
	Armor          string
	AbilityMods    [6]*int  // nil = assigned from the level's array
	SaveProfs      []string // empty = the two highest abilities
	Adjust         Adjustments

	// TenacityMax is an Elite or Solo adversary's Tenacity pool (book p.9);
	// zero for Minion/Standard, which don't use Tenacity at all.
	TenacityMax int

	// Classification is the GM's optional Iconic/Epic tag ("" for none) --
	// see ApplyClassification's own comment for why this is never randomly
	// assigned by the generator itself.
	Classification                 string
	ClassificationHPChakraModifier int
	ClassificationAbilityPoints    int

	// TransformationSlug/BattlefieldDomainSlug are mutually exclusive
	// optional attachments, meaningful only for a Solo adversary -- GM-
	// picked reference text, not something the generator computes a new
	// mechanical effect from (see migration 0077's own tables for the full
	// write-up each slug points to).
	TransformationSlug    string
	TransformationName    string
	BattlefieldDomainSlug string
	BattlefieldDomainName string
}

// Default builder tags for an adversary with no clan or affiliation.
const (
	NoClan        = "Non_Clan"
	NoAffiliation = "Factionless"
)

// legacyRoleSlugs maps Pack 1's five role slugs (rosters generated before
// the Adversary Builder) onto builder roles, matching characters migration
// 0094's backfill.
var legacyRoleSlugs = map[string]string{
	"adversary-role/striker":    "adversary-builder-role/striker-taijutsu",
	"adversary-role/controller": "adversary-builder-role/controller",
	"adversary-role/defender":   "adversary-builder-role/defender",
	"adversary-role/lurker":     "adversary-builder-role/lurker",
	"adversary-role/supporter":  "adversary-builder-role/supporter",
}

// Normalize fills builder fields an older adversary (saved or still in an
// unsaved preview) was created without.
func (a GeneratedAdversary) Normalize() GeneratedAdversary {
	if slug, ok := legacyRoleSlugs[a.RoleSlug]; ok {
		a.RoleSlug = slug
		if a.Armor == "" {
			a.Armor = DefaultArmor(slug)
		}
	}
	if RankIndex(a.AdvRank) < 0 {
		a.AdvRank = RankForLevel(a.Level)
	}
	if a.ClanTag == "" {
		a.ClanTag = NoClan
	}
	if a.AffiliationTag == "" {
		a.AffiliationTag = NoAffiliation
	}
	if a.Armor == "" {
		a.Armor = DefaultArmor(a.RoleSlug)
	}
	return a
}

// Choices returns the adversary's build choices for Compute; players is the
// encounter's party size.
func (a GeneratedAdversary) Choices(players int) BuildChoices {
	a = a.Normalize()
	return BuildChoices{
		ClassSlug:      a.RankSlug,
		Rank:           a.AdvRank,
		Level:          a.Level,
		Players:        players,
		RoleSlug:       a.RoleSlug,
		ClanTag:        a.ClanTag,
		AffiliationTag: a.AffiliationTag,
		ClassModTag:    a.ClassModTag,
		Armor:          a.Armor,
		Abilities:      a.AbilityMods,
		SaveProfs:      a.SaveProfs,
		Classification: a.Classification,
		Adjust:         a.Adjust,
	}
}

// Toolkit is everything the generator draws from, loaded from rules.db.
type Toolkit struct {
	Classes []Rank        // adversary_ranks (the book's Class)
	Roles   []BuilderRole // adversary_builder_roles, in sort order
	Traits  []Trait       // official role traits and sheet traits
	Jutsu   []Jutsu       // every ranked jutsu, for step 9
	Slots   []FreeformSlot
	Tables  BuilderTables
	// Clans and Affiliations are the tags a "random" pick chooses from,
	// without NoClan / NoAffiliation.
	Clans        []string
	Affiliations []string
	// Name builds a generated adversary's display name; nil uses an
	// English fallback.
	Name func(level int, class Rank, role BuilderRole) string
}

// RandomTag is the GenOptions value that picks a clan or affiliation at
// random for each adversary.
const RandomTag = "random"

// GenOptions are the generator form's optional clan and affiliation picks:
// "" for none, RandomTag, or a tag.
type GenOptions struct {
	Clan, Affiliation string
}

// maxRolePoolTraits is how many of an adversary's trait slots the generator
// fills from its role's pool; the rest come from the general, clan,
// affiliation and class-mod pools.
const maxRolePoolTraits = 2

// maxRosterSize caps how many adversaries a single generated roster can
// hold, regardless of how large the budget is.
const maxRosterSize = 12

// GenerateRoster builds a full adversary roster for a party of the given
// size/level/difficulty, greedily spending the computed Budget: a boss-tier
// pick (Elite or Solo) for hard/deadly difficulties, then filling the
// remainder with Standard/Minion adversaries until the budget or the roster
// cap is reached.
func GenerateRoster(rng *rand.Rand, partySize, partyLevel int, difficulty string, kit Toolkit, opt GenOptions) ([]GeneratedAdversary, int, error) {
	if len(kit.Classes) == 0 {
		return nil, 0, fmt.Errorf("encounter: no adversary classes available")
	}
	if len(kit.Roles) == 0 {
		return nil, 0, fmt.Errorf("encounter: no adversary roles available")
	}

	byKind := make(map[string]Rank, len(kit.Classes))
	for _, rk := range kit.Classes {
		byKind[ClassKind(rk.Slug)] = rk
	}

	budget := Budget(partySize, partyLevel, difficulty)
	remaining := budget

	var roster []GeneratedAdversary

	spend := func(kind string) bool {
		rk, ok := byKind[kind]
		if !ok {
			return false
		}
		cost := rankCost(kind, partyLevel)
		if cost > remaining && len(roster) > 0 {
			return false
		}
		adv := generateOne(rng, kit, rk, partyLevel, opt)
		adv.BudgetCost = cost
		roster = append(roster, adv)
		remaining -= cost
		return true
	}

	// A boss-tier pick anchors hard/deadly encounters.
	if difficulty == "hard" || difficulty == "deadly" {
		boss := "elite"
		if difficulty == "deadly" {
			boss = "solo"
		}
		spend(boss)
	}

	fillOrder := []string{"standard", "minion"}
	for len(roster) < maxRosterSize && remaining > 0 {
		spent := false
		for _, kind := range fillOrder {
			if spend(kind) {
				spent = true
				break
			}
		}
		if !spent {
			break
		}
	}

	// A tiny budget (e.g. a level-1 party) can leave remaining < every
	// class's cost before anything gets added at all — always field at
	// least one Minion so the generator never returns an empty roster.
	if len(roster) == 0 {
		spend("minion")
	}

	return roster, budget, nil
}

// GenerateOne builds a single fresh adversary of the given class and level,
// used by the add-adversary forms.
func GenerateOne(rng *rand.Rand, kit Toolkit, rk Rank, level int, opt GenOptions) GeneratedAdversary {
	adv := generateOne(rng, kit, rk, level, opt)
	adv.BudgetCost = rankCost(ClassKind(rk.Slug), level)
	return adv
}

// Reroll rebuilds prev's role, armor, abilities, traits and jutsu, keeping its
// class, level, Rank, clan, affiliation, class mod, step-10 adjustments, a
// name the GM typed, and the GM's classification and narrative attachment.
func Reroll(rng *rand.Rand, kit Toolkit, rk Rank, prev GeneratedAdversary) GeneratedAdversary {
	prev = prev.Normalize()
	adv := generateOne(rng, kit, rk, prev.Level, GenOptions{Clan: prev.ClanTag, Affiliation: prev.AffiliationTag})
	adv.BudgetCost = rankCost(ClassKind(rk.Slug), prev.Level)
	adv.AdvRank = prev.AdvRank
	adv.ClassModTag = prev.ClassModTag
	adv.Adjust = prev.Adjust
	if prev.Name != "" && !IsDefaultName(kit, rk, prev) {
		adv.Name = prev.Name
	}
	adv.Traits = pickTraits(rng, kit, adv)
	adv.Jutsu = pickJutsu(rng, kit, adv)
	adv = ApplyClassification(adv, prev.Classification, kit.Tables.Classifications)
	return WithNarrativeAttachment(adv, prev.TransformationSlug, prev.TransformationName, prev.BattlefieldDomainSlug, prev.BattlefieldDomainName)
}

// resolveTag turns a GenOptions pick into a tag.
func resolveTag(rng *rand.Rand, pick, none string, pool []string) string {
	switch {
	case pick == "":
		return none
	case pick == RandomTag:
		if len(pool) == 0 {
			return none
		}
		return pool[rng.Intn(len(pool))]
	}
	return pick
}

// generateOne makes the builder's choices for one adversary of the given
// class and level: the Rank its level maps to, a random role with that
// role's armor, abilities left to the level's array, and trait and jutsu
// sets that fill the class's slots and quotas without going above its Rank.
func generateOne(rng *rand.Rand, kit Toolkit, rk Rank, level int, opt GenOptions) GeneratedAdversary {
	role := kit.Roles[rng.Intn(len(kit.Roles))]
	adv := GeneratedAdversary{
		RankSlug:       rk.Slug,
		RoleSlug:       role.Slug,
		Level:          level,
		HPFormula:      rk.HPFormula,
		ACBonus:        rk.ACBonus,
		SaveBonus:      rk.SaveBonus,
		SaveDCBonus:    rk.SaveDCBonus,
		InitBonus:      rk.InitBonus,
		FreeformSlots:  freeformSlotsFor(rk.Slug, level, kit.Slots),
		AdvRank:        RankForLevel(level),
		ClanTag:        resolveTag(rng, opt.Clan, NoClan, kit.Clans),
		AffiliationTag: resolveTag(rng, opt.Affiliation, NoAffiliation, kit.Affiliations),
		Armor:          DefaultArmor(role.Slug),
		TenacityMax:    tenacityMaxFor(ClassKind(rk.Slug), level),
	}
	adv.Traits = pickTraits(rng, kit, adv)
	adv.Jutsu = pickJutsu(rng, kit, adv)
	adv.Name = DefaultName(kit, rk, role, level)
	return adv
}

// DefaultName is the name a generated adversary gets.
func DefaultName(kit Toolkit, rk Rank, role BuilderRole, level int) string {
	if kit.Name != nil {
		return kit.Name(level, rk, role)
	}
	return fmt.Sprintf("Level %d %s %s", level, rk.Name, role.Name)
}

// IsDefaultName reports whether adv's name is the one DefaultName gives its
// current choices, rather than one the GM typed. A default made in another
// locale reads as typed.
func IsDefaultName(kit Toolkit, rk Rank, adv GeneratedAdversary) bool {
	return adv.Name == DefaultName(kit, rk, kit.Tables.Roles[adv.RoleSlug], adv.Level)
}

// Assemble completes an adversary whose builder choices were made by hand:
// it snapshots the class's Pack 1 values, Freeform slots, budget cost and
// Tenacity for adv's class and level, reapplies its classification, and
// names it when the name is blank.
func Assemble(kit Toolkit, rk Rank, adv GeneratedAdversary) GeneratedAdversary {
	classification := adv.Classification
	kind := ClassKind(rk.Slug)
	adv.RankSlug = rk.Slug
	adv.HPFormula = rk.HPFormula
	adv.ACBonus, adv.SaveBonus, adv.SaveDCBonus, adv.InitBonus = rk.ACBonus, rk.SaveBonus, rk.SaveDCBonus, rk.InitBonus
	adv.Classification = ""
	adv.FreeformSlots = freeformSlotsFor(rk.Slug, adv.Level, kit.Slots)
	adv.BudgetCost = rankCost(kind, adv.Level)
	adv.TenacityMax = tenacityMaxFor(kind, adv.Level)
	adv = ApplyClassification(adv, classification, kit.Tables.Classifications)
	if kind != "solo" {
		adv = WithNarrativeAttachment(adv, "", "", "", "")
	}
	adv = adv.Normalize()
	if adv.Name == "" {
		adv.Name = DefaultName(kit, rk, kit.Tables.Roles[adv.RoleSlug], adv.Level)
	}
	return adv
}

// TraitEligible reports whether a trait can be given to an adversary of the
// given Rank: unranked traits always can, ranked ones up to MaxTraitRank.
func TraitEligible(t Trait, maxRank string) bool {
	return t.Rank == "" || RankIndex(t.Rank) <= RankIndex(maxRank)
}

// InRolePool reports whether t belongs to role's step-7 pool: the Pack 1
// traits of the role it draws from, or the sheet's traits for its tag.
func InRolePool(t Trait, role BuilderRole) bool {
	if t.Source == "official" {
		return role.TraitRoleSlug != "" && t.RoleSlug == role.TraitRoleSlug
	}
	return t.Category == "role" && t.Tag == role.Tag
}

// InOtherPool reports whether t belongs to the general, clan, affiliation
// or class-mod pools for adv.
func InOtherPool(t Trait, adv GeneratedAdversary) bool {
	if t.Source != "sheet" {
		return false
	}
	switch t.Category {
	case "general":
		return true
	case "clan":
		return t.Tag == adv.ClanTag
	case "affiliation":
		return t.Tag == adv.AffiliationTag
	case "class_mod":
		return adv.ClassModTag != "" && t.Tag == adv.ClassModTag
	}
	return false
}

// pickTraits fills adv's trait slots: up to maxRolePoolTraits from its
// role's pool, the rest from the other pools, topping up from the role pool
// when those run short.
func pickTraits(rng *rand.Rand, kit Toolkit, adv GeneratedAdversary) []GeneratedTrait {
	sb := Compute(adv.Choices(1), kit.Tables)
	role := kit.Tables.Roles[adv.RoleSlug]
	var rolePool, otherPool []Trait
	for _, t := range kit.Traits {
		if !TraitEligible(t, sb.MaxTraitRank) {
			continue
		}
		switch {
		case InRolePool(t, role):
			rolePool = append(rolePool, t)
		case InOtherPool(t, adv):
			otherPool = append(otherPool, t)
		}
	}
	rng.Shuffle(len(rolePool), func(i, j int) { rolePool[i], rolePool[j] = rolePool[j], rolePool[i] })
	rng.Shuffle(len(otherPool), func(i, j int) { otherPool[i], otherPool[j] = otherPool[j], otherPool[i] })

	slots := sb.TraitSlots
	nRole := min(maxRolePoolTraits, slots, len(rolePool))
	nOther := min(slots-nRole, len(otherPool))
	nRole = min(slots-nOther, len(rolePool))
	picked := append(append([]Trait(nil), rolePool[:nRole]...), otherPool[:nOther]...)
	sort.Slice(picked, func(i, j int) bool { return picked[i].Name < picked[j].Name })

	out := make([]GeneratedTrait, len(picked))
	for i, t := range picked {
		out[i] = GeneratedTrait{Slug: t.Slug, Name: t.Name, Rank: t.Rank, Description: t.Description, Source: t.Source, Category: t.Category}
	}
	return out
}

// tenacityMaxFor returns an Elite or Solo adversary's maximum Tenacity pool
// (book p.9): half its level rounded up for an Elite, its full level for a
// Solo. Minions and Standards don't use Tenacity at all. kind is ClassKind's
// result, so a translated class name never changes the outcome.
func tenacityMaxFor(kind string, level int) int {
	switch kind {
	case "elite":
		return (level + 1) / 2
	case "solo":
		return level
	default:
		return 0
	}
}

// ApplyClassification returns adv with an Optional Classification's flat
// stat bumps applied (or removed, for kind == ""). kind must be "iconic",
// "epic", or "" -- never assigned by GenerateOne/GenerateRoster themselves.
// Per the book (p.10): "Use them sparingly. The moment everything is Iconic
// or Epic, nothing is" -- this is a deliberate, GM-driven tag applied after
// the fact, not something the generator ever picks on its own.
//
// Calling this repeatedly (e.g. switching Iconic -> Epic, or -> none) is
// safe: whatever classification is already on adv is undone first, so
// bonuses never stack.
func ApplyClassification(adv GeneratedAdversary, kind string, mods []ClassificationModifier) GeneratedAdversary {
	if m, ok := classificationModifier(adv.Classification, mods); ok {
		adv.ACBonus -= m.ACBonus
		adv.SaveDCBonus -= m.AttackSaveBonus
		adv.InitBonus -= m.InitiativeBonus
	}
	adv.Classification = ""
	adv.ClassificationHPChakraModifier = 0
	adv.ClassificationAbilityPoints = 0

	if m, ok := classificationModifier(kind, mods); ok {
		adv.Classification = kind
		adv.ACBonus += m.ACBonus
		adv.SaveDCBonus += m.AttackSaveBonus
		adv.InitBonus += m.InitiativeBonus
		adv.ClassificationHPChakraModifier = m.HPChakraModifier
		adv.ClassificationAbilityPoints = m.AbilityPoints
	}
	return adv
}

func classificationModifier(kind string, mods []ClassificationModifier) (ClassificationModifier, bool) {
	if kind == "" {
		return ClassificationModifier{}, false
	}
	for _, m := range mods {
		if m.Kind == kind {
			return m, true
		}
	}
	return ClassificationModifier{}, false
}

// WithNarrativeAttachment sets adv's optional Transformation or Battlefield
// Domain attachment -- Solo-only GM-picked reference text (see migration
// 0077's own tables for the full write-up each slug points to). The book
// requires picking one or the other, never both (p.21), so setting a
// Transformation clears any Battlefield Domain and vice versa. Pass all four
// arguments empty to clear both.
func WithNarrativeAttachment(adv GeneratedAdversary, transformationSlug, transformationName, domainSlug, domainName string) GeneratedAdversary {
	adv.TransformationSlug, adv.TransformationName = "", ""
	adv.BattlefieldDomainSlug, adv.BattlefieldDomainName = "", ""
	if transformationSlug != "" {
		adv.TransformationSlug, adv.TransformationName = transformationSlug, transformationName
	} else if domainSlug != "" {
		adv.BattlefieldDomainSlug, adv.BattlefieldDomainName = domainSlug, domainName
	}
	return adv
}

// freeformSlotsFor returns the Freeform Attack/Jutsu slot count for a rank
// at the given level: the slots row with the greatest LevelMin that is
// still <= level.
func freeformSlotsFor(rankSlug string, level int, slots []FreeformSlot) int {
	best := -1
	n := 0
	for _, s := range slots {
		if s.RankSlug != rankSlug || s.LevelMin > level {
			continue
		}
		if s.LevelMin > best {
			best = s.LevelMin
			n = s.Slots
		}
	}
	return n
}
