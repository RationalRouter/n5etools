package encounter

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// The Adversary Builder (Bingo Book p.6, "Build Your Adversary"): ten build
// choices -- Rank, Class, Level, Clan, Affiliation, Role, Traits, Passives,
// Jutsu, Finalize -- turned into a full stat block by Compute. The charts it
// reads are rules migration 0082's tables (the community Adversary Builder
// sheet's ChartsMain, "V3.11 Bingo Book" maths). Class modifiers follow Bingo
// Book Pack 1 (adversary_ranks, adversary_class_hp_mods) where the two
// disagree. The formulas mirror the sheet's own cells, noted per stat below.

// Abilities in the order every [6] array in this file uses.
var Abilities = [6]string{"str", "dex", "con", "int", "wis", "cha"}

const (
	abStr = iota
	abDex
	abCon
	abInt
	abWis
	abCha
)

// AdversaryRanks lists the book's Rank letters, weakest first.
var AdversaryRanks = []string{"E", "D", "C", "B", "A", "S"}

// Armor kinds an adversary can wear (the sheet's L./M./H./C. Armor).
var ArmorKinds = []string{"light", "medium", "heavy", "chakra-int", "chakra-wis", "chakra-cha"}

// Disciplines are the three jutsu disciplines with their own attack bonus
// and save DC.
var Disciplines = []string{"ninjutsu", "genjutsu", "taijutsu"}

// LevelStats is one adversary_level_stats row.
type LevelStats struct {
	Level           int
	Prof            int
	AttackBonus     int
	DamagePerAction int
	StandardMods    [6]int // highest first; Minion and Standard
	EliteMods       [6]int // highest first; Elite and Solo
	XP              int
}

// RankTier is one adversary_rank_tiers row.
type RankTier struct {
	Rank           string
	HitDie, HitAvg int
	ChakraDie      int
	ChakraAvg      int
	Defensive      int
	ToHit          int
	SaveDC         int
	BonusDamage    int
}

// BuilderRole is one adversary_builder_roles row. Tag is the untranslated
// name, which adversary_auto_bonuses and the sheet trait catalog key on;
// Name may be translated.
type BuilderRole struct {
	Slug, Tag, Name string
	AC, Save        int
	HP, CP          float64
	DamageMult      float64
	Speed           int
	TraitRoleSlug   string
	SortOrder       int
}

// ClanMods is one adversary_clan_mods row.
type ClanMods struct {
	Tag    string
	HP, CP float64
	Atk    map[string]int // by discipline
	DC     map[string]int // by discipline
}

// AutoBonus is one adversary_auto_bonuses row.
type AutoBonus struct {
	Tag, Stat string
	Value     int
	Note      string
}

// ClassHPMod is one adversary_class_hp_mods row.
type ClassHPMod struct {
	Base, PerPlayer float64
	MinionFlat      bool
}

// JutsuKnownRow is one adversary_jutsu_known row.
type JutsuKnownRow struct {
	ClassSlug string
	JutsuRank string // "*" = any rank up to the adversary's own
	LevelMin  int
	Count     int
}

// BuilderTables bundles every chart Compute reads.
type BuilderTables struct {
	Levels          map[int]LevelStats
	TraitCounts     map[int][4]int // Minion, Standard, Elite, Solo
	Ranks           map[string]RankTier
	Roles           map[string]BuilderRole
	Clans           map[string]ClanMods
	AutoBonuses     []AutoBonus
	Classes         map[string]Rank // Pack 1 adversary_ranks, by slug
	ClassHP         map[string]ClassHPMod
	JutsuKnown      []JutsuKnownRow
	Classifications []ClassificationModifier
}

// Adjustments are step 10's flat tweaks, added after every formula.
type Adjustments struct {
	HP, CP, AC, Init, Speed, DR int
}

// BuildChoices are the GM's (or the generator's) answers to the ten steps.
type BuildChoices struct {
	ClassSlug      string // adversary-rank/minion..solo
	Rank           string // E..S
	Level          int
	Players        int
	RoleSlug       string
	ClanTag        string
	AffiliationTag string
	ClassModTag    string
	Armor          string
	Abilities      [6]*int  // nil = assigned from the level's array by role
	SaveProfs      []string // ability keys; empty = the two highest
	Classification string   // "", "iconic", "epic"
	Adjust         Adjustments
}

// Term is one labelled part of a stat's formula, for the breakdown tooltip.
// Key is a locale-key suffix under "encounters.builder_term_".
type Term struct {
	Key   string
	Value string
}

// JutsuQuota is how many jutsu of up to Rank the adversary knows.
type JutsuQuota struct {
	Rank  string
	Count int
}

// StatBlock is Compute's result.
type StatBlock struct {
	Level, Prof         int
	HPMax, CPMax        int
	HPPhases            []int // Elite/Solo phase thresholds (60%, 30%)
	AC, Init, Speed, DR int
	Abilities           [6]int
	SaveProf            [6]bool
	Saves               [6]int
	WeaponAttack        int
	Attack, DC          map[string]int // by discipline
	DamagePerAction     int
	TraitSlots          int
	MaxTraitRank        string
	Jutsu               []JutsuQuota
	JutsuTotal          int
	TenacityMax         int
	Notes               []string // text-only automatic benefits
	Breakdown           map[string][]Term
}

// ClassKind returns "minion", "standard", "elite" or "solo" for a class slug.
func ClassKind(classSlug string) string {
	return strings.TrimPrefix(classSlug, "adversary-rank/")
}

// classIndex is a class's column in TraitCounts.
func classIndex(kind string) int {
	switch kind {
	case "minion":
		return 0
	case "elite":
		return 2
	case "solo":
		return 3
	}
	return 1
}

// RankIndex returns a Rank letter's position in AdversaryRanks, or -1.
func RankIndex(rank string) int {
	for i, r := range AdversaryRanks {
		if r == rank {
			return i
		}
	}
	return -1
}

// RankForLevel is the Rank a generated or backfilled adversary gets for its
// level, following the mission-rank level bands.
func RankForLevel(level int) string {
	switch {
	case level <= 4:
		return "D"
	case level <= 8:
		return "C"
	case level <= 12:
		return "B"
	case level <= 16:
		return "A"
	}
	return "S"
}

// rolePriority orders abilities by how much a role wants them; the level's
// highest modifier goes to the first.
var rolePriority = map[string][6]int{
	"adversary-builder-role/caster-ninjutsu":  {abInt, abCon, abDex, abWis, abCha, abStr},
	"adversary-builder-role/caster-genjutsu":  {abWis, abDex, abCon, abCha, abInt, abStr},
	"adversary-builder-role/caster-taijutsu":  {abDex, abCon, abStr, abWis, abInt, abCha},
	"adversary-builder-role/striker-ninjutsu": {abInt, abDex, abCon, abWis, abCha, abStr},
	"adversary-builder-role/striker-genjutsu": {abWis, abDex, abCon, abInt, abCha, abStr},
	"adversary-builder-role/striker-taijutsu": {abStr, abDex, abCon, abWis, abInt, abCha},
	"adversary-builder-role/controller":       {abWis, abInt, abCon, abDex, abCha, abStr},
	"adversary-builder-role/defender":         {abCon, abStr, abDex, abWis, abInt, abCha},
	"adversary-builder-role/generalist":       {abDex, abCon, abStr, abInt, abWis, abCha},
	"adversary-builder-role/lurker":           {abDex, abInt, abCon, abWis, abCha, abStr},
	"adversary-builder-role/supporter":        {abWis, abCon, abInt, abDex, abCha, abStr},
}

var defaultPriority = [6]int{abDex, abCon, abStr, abWis, abInt, abCha}

// DefaultArmor is the armor the generator gives a role.
func DefaultArmor(roleSlug string) string {
	switch strings.TrimPrefix(roleSlug, "adversary-builder-role/") {
	case "defender":
		return "heavy"
	case "caster-ninjutsu":
		return "chakra-int"
	case "caster-genjutsu", "controller", "supporter":
		return "chakra-wis"
	case "caster-taijutsu", "striker-taijutsu":
		return "medium"
	}
	return "light"
}

// RoleDiscipline is the jutsu discipline a Caster or Striker role is built
// around, or "" for the others.
func RoleDiscipline(roleSlug string) string {
	for _, d := range Disciplines {
		if strings.HasSuffix(roleSlug, "-"+d) {
			return d
		}
	}
	return ""
}

// LevelArray returns the six ability modifiers (highest first) a class gets
// at a level.
func (t BuilderTables) LevelArray(classSlug string, level int) [6]int {
	ls := t.level(level)
	switch ClassKind(classSlug) {
	case "elite", "solo":
		return ls.EliteMods
	}
	return ls.StandardMods
}

// level returns the stats row for level, clamped to the table's range.
func (t BuilderTables) level(level int) LevelStats {
	if ls, ok := t.Levels[level]; ok {
		return ls
	}
	lo, hi := math.MaxInt, math.MinInt
	for l := range t.Levels {
		lo, hi = min(lo, l), max(hi, l)
	}
	if level < lo {
		return t.Levels[lo]
	}
	return t.Levels[hi]
}

// AssignAbilities fills every unset ability from the level's array: values
// already pinned by the GM are taken out of the array first, then the rest
// go to the role's priority order, highest first.
func AssignAbilities(set [6]*int, array [6]int, roleSlug string) [6]int {
	remaining := append([]int(nil), array[:]...)
	sort.Sort(sort.Reverse(sort.IntSlice(remaining)))
	var out [6]int
	for i, v := range set {
		if v == nil {
			continue
		}
		out[i] = *v
		for j, r := range remaining {
			if r == *v {
				remaining = append(remaining[:j], remaining[j+1:]...)
				break
			}
		}
	}
	prio, ok := rolePriority[roleSlug]
	if !ok {
		prio = defaultPriority
	}
	for _, ab := range prio {
		if set[ab] != nil {
			continue
		}
		if len(remaining) > 0 {
			out[ab] = remaining[0]
			remaining = remaining[1:]
		}
	}
	return out
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func signed(n int) string {
	if n >= 0 {
		return "+" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// Compute turns build choices into a stat block.
func Compute(c BuildChoices, t BuilderTables) StatBlock {
	kind := ClassKind(c.ClassSlug)
	class := t.Classes[c.ClassSlug]
	classHP := t.ClassHP[c.ClassSlug]
	rank := t.Ranks[c.Rank]
	role := t.Roles[c.RoleSlug]
	clan := t.Clans[c.ClanTag]
	ls := t.level(c.Level)
	cls, _ := classificationModifier(c.Classification, t.Classifications)

	auto := map[string]int{}
	var notes []string
	for _, b := range t.AutoBonuses {
		if b.Tag == "" || (b.Tag != role.Tag && b.Tag != c.ClassModTag) {
			continue
		}
		if b.Stat == "note" {
			notes = append(notes, b.Note)
			continue
		}
		auto[b.Stat] += b.Value
	}

	sb := StatBlock{
		Level:     c.Level,
		Prof:      ls.Prof,
		Attack:    map[string]int{},
		DC:        map[string]int{},
		Notes:     notes,
		Breakdown: map[string][]Term{},
	}
	add := func(stat, key string, v string) {
		sb.Breakdown[stat] = append(sb.Breakdown[stat], Term{Key: key, Value: v})
	}

	// Ability modifiers: the level's array, assigned by the GM or by role,
	// plus any flat automatic bonus (sheet BY5:BY10).
	base := AssignAbilities(c.Abilities, t.LevelArray(c.ClassSlug, c.Level), c.RoleSlug)
	for i, ab := range Abilities {
		sb.Abilities[i] = base[i] + auto[ab]
	}
	mod := func(i int) int { return sb.Abilities[i] }
	str, dex, con := mod(abStr), mod(abDex), mod(abCon)

	// Hit and Chakra Points (sheet U8/AA8, book p.8): 10 + Con x Level +
	// Rank Die average x Level x (1 + Role + Class + Clan + Iconic/Epic).
	// A Minion has 10 + its level instead (Pack 1).
	pool := func(stat string, avg int, roleMod, clanMod float64, autoKey string, adj int) int {
		if classHP.MinionFlat {
			add(stat, "minion", "10 + "+itoa(c.Level))
			v := 10 + c.Level + auto[autoKey] + adj
			if auto[autoKey] != 0 {
				add(stat, "auto", signed(auto[autoKey]))
			}
			if adj != 0 {
				add(stat, "adjust", signed(adj))
			}
			return v
		}
		classMod := classHP.Base + classHP.PerPlayer*float64(c.Players)
		mult := 1 + roleMod + classMod + clanMod + float64(cls.HPChakraModifier)
		v := int(math.Floor(10 + float64(con*c.Level) + float64(avg*c.Level)*mult))
		add(stat, "base", "10")
		add(stat, "con_level", signed(con)+" × "+itoa(c.Level))
		add(stat, "rank_die_level", itoa(avg)+" × "+itoa(c.Level))
		add(stat, "multiplier", "× "+ftoa(mult))
		if roleMod != 0 {
			add(stat, "role", "+"+ftoa(roleMod))
		}
		if classMod != 0 {
			add(stat, "class", "+"+ftoa(classMod))
		}
		if clanMod != 0 {
			add(stat, "clan", "+"+ftoa(clanMod))
		}
		if cls.HPChakraModifier != 0 {
			add(stat, "classification", signed(cls.HPChakraModifier))
		}
		if auto[autoKey] != 0 {
			add(stat, "auto", signed(auto[autoKey]))
		}
		if adj != 0 {
			add(stat, "adjust", signed(adj))
		}
		return v + auto[autoKey] + adj
	}
	sb.HPMax = max(pool("hp", rank.HitAvg, role.HP, clan.HP, "max_hp", c.Adjust.HP), 1)
	sb.CPMax = max(pool("cp", rank.ChakraAvg, role.CP, clan.CP, "max_chakra", c.Adjust.CP), 0)
	if kind == "elite" || kind == "solo" {
		sb.HPPhases = []int{int(0.6 * float64(sb.HPMax)), int(0.3 * float64(sb.HPMax))}
	}

	// Armor Class (sheet N8): 10 + ceil(Level/4) + armor + level bracket +
	// Role + Class + Rank defensive bonus + Iconic/Epic.
	armor := 0
	switch c.Armor {
	case "medium":
		armor = 4 + floorDiv(dex, 2)
	case "heavy":
		armor = 6
	case "chakra-int":
		armor = 1 + mod(abInt)
	case "chakra-wis":
		armor = 1 + mod(abWis)
	case "chakra-cha":
		armor = 1 + mod(abCha)
	default:
		armor = 1 + dex
	}
	bracket := 4
	switch {
	case c.Level < 7:
		bracket = 0
	case c.Level < 11:
		bracket = 1
	case c.Level < 14:
		bracket = 2
	case c.Level < 17:
		bracket = 3
	}
	levelAC := int(math.Ceil(float64(c.Level) / 4))
	sb.AC = 10 + levelAC + armor + bracket + role.AC + class.ACBonus + rank.Defensive + cls.ACBonus + auto["ac"] + c.Adjust.AC
	add("ac", "base", "10")
	add("ac", "level", signed(levelAC+bracket))
	add("ac", "armor", signed(armor))
	for _, p := range []struct {
		key string
		v   int
	}{{"role", role.AC}, {"class", class.ACBonus}, {"rank", rank.Defensive}, {"classification", cls.ACBonus}, {"auto", auto["ac"]}, {"adjust", c.Adjust.AC}} {
		if p.v != 0 {
			add("ac", p.key, signed(p.v))
		}
	}

	// Initiative (sheet N10): half proficiency + Dex + Class.
	sb.Init = floorDiv(ls.Prof, 2) + dex + class.InitBonus + cls.InitiativeBonus + auto["init"] + c.Adjust.Init
	add("init", "half_prof", signed(floorDiv(ls.Prof, 2)))
	add("init", "dex", signed(dex))
	for _, p := range []struct {
		key string
		v   int
	}{{"class", class.InitBonus}, {"classification", cls.InitiativeBonus}, {"auto", auto["init"]}, {"adjust", c.Adjust.Init}} {
		if p.v != 0 {
			add("init", p.key, signed(p.v))
		}
	}

	// Speed (sheet N12): 30 ft + Role; Iconic +20 ft, Epic +30 ft.
	clsSpeed := map[string]int{"iconic": 20, "epic": 30}[c.Classification]
	sb.Speed = 30 + role.Speed + clsSpeed + auto["speed_flat"] + c.Adjust.Speed
	add("speed", "base", "30")
	for _, p := range []struct {
		key string
		v   int
	}{{"role", role.Speed}, {"classification", clsSpeed}, {"auto", auto["speed_flat"]}, {"adjust", c.Adjust.Speed}} {
		if p.v != 0 {
			add("speed", p.key, signed(p.v))
		}
	}

	// Damage Reduction (sheet N13): Rank defensive bonus x armor weight
	// (medium 1, heavy 2) + 2, Defenders +3.
	weight := map[string]int{"medium": 1, "heavy": 2}[c.Armor]
	defender := 0
	if strings.HasSuffix(c.RoleSlug, "/defender") {
		defender = 3
	}
	sb.DR = max(rank.Defensive*weight+2+defender+auto["dr"]+c.Adjust.DR, 0)
	add("dr", "base", "2")
	for _, p := range []struct {
		key string
		v   int
	}{{"armor", rank.Defensive * weight}, {"role", defender}, {"auto", auto["dr"]}, {"adjust", c.Adjust.DR}} {
		if p.v != 0 {
			add("dr", p.key, signed(p.v))
		}
	}

	// Saving throws (sheet F10:F15): modifier + proficiency (half when not
	// proficient) + Role + Class.
	profs := c.SaveProfs
	if len(profs) == 0 {
		order := []int{0, 1, 2, 3, 4, 5}
		sort.SliceStable(order, func(i, j int) bool { return base[order[i]] > base[order[j]] })
		profs = []string{Abilities[order[0]], Abilities[order[1]]}
	}
	for _, p := range profs {
		for i, ab := range Abilities {
			if ab == p {
				sb.SaveProf[i] = true
			}
		}
	}
	for i, ab := range Abilities {
		p := floorDiv(ls.Prof, 2)
		if sb.SaveProf[i] {
			p = ls.Prof
		}
		sb.Saves[i] = mod(i) + p + role.Save + class.SaveBonus + auto["all_saves"] + auto[ab+"_save"]
	}

	// Attacks and DCs (sheet AN9, AM13:AM15, AS13:AS15) plus the Rank's
	// to-hit and save DC bonuses (book p.6, step 1) and Iconic/Epic's
	// attack bonus/save DC bonus (book p.10).
	best := max(str, dex)
	sb.WeaponAttack = ls.Prof + best + rank.ToHit + cls.AttackSaveBonus + auto["weapon_attack"]
	for _, d := range Disciplines {
		m := mod(abInt)
		switch d {
		case "genjutsu":
			m = mod(abWis)
		case "taijutsu":
			m = best
		}
		sb.Attack[d] = ls.Prof + m + rank.ToHit + clan.Atk[d] + cls.AttackSaveBonus + auto[d+"_atk"]
		sb.DC[d] = 10 + ls.Prof + m + class.SaveDCBonus + rank.SaveDC + clan.DC[d] + cls.AttackSaveBonus + auto[d+"_dc"]
	}

	// Damage per action (sheet AL83): the level's figure x Role (x1.1
	// Elite, x1.2 Solo) + the Rank's bonus damage.
	cm := 1.0
	switch kind {
	case "elite":
		cm = 1.1
	case "solo":
		cm = 1.2
	}
	dm := role.DamageMult
	if dm == 0 {
		dm = 1
	}
	sb.DamagePerAction = int(math.Round(float64(ls.DamagePerAction)*dm*cm)) + rank.BonusDamage

	// Traits and jutsu.
	if tc, ok := t.TraitCounts[c.Level]; ok {
		sb.TraitSlots = tc[classIndex(kind)]
	}
	sb.MaxTraitRank = c.Rank
	if c.Rank == "E" {
		sb.MaxTraitRank = "D"
	}
	sb.Jutsu = jutsuQuotas(t.JutsuKnown, c.ClassSlug, c.Rank, c.Level)
	for _, q := range sb.Jutsu {
		sb.JutsuTotal += q.Count
	}
	sb.TenacityMax = tenacityMaxFor(kind, c.Level)
	return sb
}

// jutsuQuotas returns the jutsu an adversary knows, by highest allowed rank.
// Each jutsu rank's count is the row with the greatest LevelMin <= level; a
// rank above the adversary's own counts toward its own Rank instead.
func jutsuQuotas(rows []JutsuKnownRow, classSlug, rank string, level int) []JutsuQuota {
	type pick struct{ levelMin, count int }
	best := map[string]pick{}
	for _, r := range rows {
		if r.ClassSlug != classSlug || r.LevelMin > level {
			continue
		}
		if p, ok := best[r.JutsuRank]; !ok || r.LevelMin > p.levelMin {
			best[r.JutsuRank] = pick{r.LevelMin, r.Count}
		}
	}
	advIdx := RankIndex(rank)
	counts := map[string]int{}
	for jr, p := range best {
		target := rank
		if jr != "*" && RankIndex(jr) < advIdx {
			target = jr
		}
		counts[target] += p.count
	}
	var out []JutsuQuota
	for _, r := range AdversaryRanks {
		if n := counts[r]; n > 0 {
			out = append(out, JutsuQuota{Rank: r, Count: n})
		}
	}
	return out
}
