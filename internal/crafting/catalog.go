package crafting

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/RationalRouter/n5e/internal/i18n"
)

// Base is a weapon or armor a recipe can start from.
type Base struct {
	Slug          string
	Name          string
	Kind          string // "weapon" or "armor"
	CostRyo       float64
	Description   string
	DamageDice    string
	DamageType    string
	Properties    string
	ACBonus       int
	ArmorCategory string
	// HighQuality: the armor's High Quality property grants +1 Seal Slot
	// (armor-property/high-quality).
	HighQuality bool
}

// Seal is one enhancement seal.
type Seal struct {
	Slug        string
	Name        string
	AppliesTo   string // "weapon" or "armor"
	Rank        string // "D"…"S"
	CostRyo     float64
	Description string
	// Family is the seal's name without its tier, taken from the slug's last
	// segment ("elemental-seal"). "Seals of the same name, do not stack…
	// classifications such as (Minor), (Refined) or (Mastercraft) are Not
	// treated as a part of the seals name" (p.72).
	Family string
	Choice *ChoiceRule
}

// RankRule is the seal's row of the per-rank tables.
func (s Seal) RankRule() RankRule {
	r, _ := RankRuleFor(s.Rank)
	return r
}

// JutsuOption is a jutsu a Storing Seal can hold.
type JutsuOption struct {
	Slug           string
	Name           string
	Rank           string
	Classification string
}

// SummonOption is a summon tribe a Fanged Seal can draw on.
type SummonOption struct {
	Slug       string
	Name       string
	SummonType string
}

// Catalog is everything a recipe is validated and priced against, with
// display text already in the requested locale.
type Catalog struct {
	Bases   []Base
	Seals   []Seal
	Jutsu   []JutsuOption
	Summons []SummonOption

	baseBySlug   map[string]int
	sealBySlug   map[string]int
	jutsuBySlug  map[string]int
	summonBySlug map[string]int
}

// Base looks up a base item by slug.
func (c *Catalog) Base(slug string) (Base, bool) {
	i, ok := c.baseBySlug[slug]
	if !ok {
		return Base{}, false
	}
	return c.Bases[i], true
}

// Seal looks up a seal by slug.
func (c *Catalog) Seal(slug string) (Seal, bool) {
	i, ok := c.sealBySlug[slug]
	if !ok {
		return Seal{}, false
	}
	return c.Seals[i], true
}

// JutsuOption looks up a storable jutsu by slug.
func (c *Catalog) JutsuOption(slug string) (JutsuOption, bool) {
	i, ok := c.jutsuBySlug[slug]
	if !ok {
		return JutsuOption{}, false
	}
	return c.Jutsu[i], true
}

// SummonOption looks up a summon tribe by slug.
func (c *Catalog) SummonOption(slug string) (SummonOption, bool) {
	i, ok := c.summonBySlug[slug]
	if !ok {
		return SummonOption{}, false
	}
	return c.Summons[i], true
}

// SealsFor returns the seals that apply to kind, ordered by rank then name.
func (c *Catalog) SealsFor(kind string) []Seal {
	var out []Seal
	for _, s := range c.Seals {
		if s.AppliesTo == kind {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := rankIndex(out[i].Rank), rankIndex(out[j].Rank); ri != rj {
			return ri < rj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// LoadCatalog reads the bases, seals, storable jutsu and summon tribes from
// rules.db, overlaying locale's translations.
func LoadCatalog(rulesDB *sql.DB, locale string) (*Catalog, error) {
	c := &Catalog{
		baseBySlug: map[string]int{}, sealBySlug: map[string]int{},
		jutsuBySlug: map[string]int{}, summonBySlug: map[string]int{},
	}
	if err := c.loadEquipment(rulesDB, locale); err != nil {
		return nil, err
	}
	if err := c.loadJutsu(rulesDB, locale); err != nil {
		return nil, err
	}
	if err := c.loadSummons(rulesDB, locale); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Catalog) loadEquipment(rulesDB *sql.DB, locale string) error {
	rows, err := rulesDB.Query(`
		SELECT e.slug, e.name, e.kind, COALESCE(e.cost_ryo, 0), COALESCE(e.description, ''),
		       COALESCE(e.damage_dice, ''), COALESCE(e.damage_type, ''), COALESCE(e.properties, ''),
		       COALESCE(e.ac_bonus, 0), COALESCE(e.armor_category, ''),
		       COALESCE(e.seal_rank, ''), COALESCE(e.seal_applies_to, ''),
		       EXISTS (SELECT 1 FROM equipment_armor_properties p
		               WHERE p.equipment_slug = e.slug AND p.property_slug = 'armor-property/high-quality')
		FROM equipment e
		WHERE e.kind IN ('weapon', 'armor', 'enhancement_seal')
		ORDER BY e.name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var (
			slug, name, kind, description, dice, damageType, props, category, rank, appliesTo string
			cost                                                                           float64
			ac                                                                             int
			highQuality                                                                    bool
		)
		if err := rows.Scan(&slug, &name, &kind, &cost, &description, &dice, &damageType, &props,
			&ac, &category, &rank, &appliesTo, &highQuality); err != nil {
			return err
		}
		slugs = append(slugs, slug)
		if kind == "enhancement_seal" {
			s := Seal{
				Slug: slug, Name: name, AppliesTo: appliesTo, Rank: rank, CostRyo: cost,
				Description: description, Family: slug[strings.LastIndex(slug, "/")+1:],
			}
			if rule, ok := sealChoices[slug]; ok {
				s.Choice = &rule
			}
			c.sealBySlug[slug] = len(c.Seals)
			c.Seals = append(c.Seals, s)
			continue
		}
		c.baseBySlug[slug] = len(c.Bases)
		c.Bases = append(c.Bases, Base{
			Slug: slug, Name: name, Kind: kind, CostRyo: cost, Description: description,
			DamageDice: dice, DamageType: damageType, Properties: props,
			ACBonus: ac, ArmorCategory: category, HighQuality: highQuality,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tr, err := i18n.Batch(rulesDB, locale, "equipment", slugs)
	if err != nil {
		return err
	}
	for i := range c.Bases {
		overlay(tr[c.Bases[i].Slug], &c.Bases[i].Name, &c.Bases[i].Description)
	}
	for i := range c.Seals {
		overlay(tr[c.Seals[i].Slug], &c.Seals[i].Name, &c.Seals[i].Description)
	}
	sort.SliceStable(c.Bases, func(i, j int) bool { return c.Bases[i].Name < c.Bases[j].Name })
	for i, b := range c.Bases {
		c.baseBySlug[b.Slug] = i
	}
	return nil
}

func (c *Catalog) loadJutsu(rulesDB *sql.DB, locale string) error {
	rows, err := rulesDB.Query(`
		SELECT slug, name, rank, classification FROM jutsu
		WHERE rank IN ('E', 'D', 'C', 'B')
		ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var j JutsuOption
		if err := rows.Scan(&j.Slug, &j.Name, &j.Rank, &j.Classification); err != nil {
			return err
		}
		if !storable(j.Classification) {
			continue
		}
		slugs = append(slugs, j.Slug)
		c.Jutsu = append(c.Jutsu, j)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	tr, err := i18n.Batch(rulesDB, locale, "jutsu", slugs)
	if err != nil {
		return err
	}
	for i := range c.Jutsu {
		overlay(tr[c.Jutsu[i].Slug], &c.Jutsu[i].Name, nil)
	}
	sort.SliceStable(c.Jutsu, func(i, j int) bool { return c.Jutsu[i].Name < c.Jutsu[j].Name })
	for i, j := range c.Jutsu {
		c.jutsuBySlug[j.Slug] = i
	}
	return nil
}

func (c *Catalog) loadSummons(rulesDB *sql.DB, locale string) error {
	rows, err := rulesDB.Query(`SELECT slug, name, summon_type FROM summon_tribes ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var s SummonOption
		if err := rows.Scan(&s.Slug, &s.Name, &s.SummonType); err != nil {
			return err
		}
		slugs = append(slugs, s.Slug)
		c.Summons = append(c.Summons, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	tr, err := i18n.Batch(rulesDB, locale, "summon_tribes", slugs)
	if err != nil {
		return err
	}
	for i := range c.Summons {
		overlay(tr[c.Summons[i].Slug], &c.Summons[i].Name, nil)
		c.summonBySlug[c.Summons[i].Slug] = i
	}
	return nil
}

// storable reports whether a jutsu classification ("Hijutsu, Bukijutsu"
// included) names one a Storing Seal accepts.
func storable(classification string) bool {
	for _, part := range strings.Split(classification, ",") {
		for _, ok := range storableClassifications {
			if strings.TrimSpace(part) == ok {
				return true
			}
		}
	}
	return false
}

func overlay(tr map[string]string, name, description *string) {
	if v, ok := tr["name"]; ok && v != "" {
		*name = v
	}
	if description != nil {
		if v, ok := tr["description"]; ok && v != "" {
			*description = v
		}
	}
}

// SummonsFor returns the summon tribes rule allows.
func (c *Catalog) SummonsFor(rule ChoiceRule) []SummonOption {
	var out []SummonOption
	for _, s := range c.Summons {
		for _, t := range rule.SummonTypes {
			if s.SummonType == t {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// JutsuFor returns the jutsu rule allows (at or below its rank cap).
func (c *Catalog) JutsuFor(rule ChoiceRule) []JutsuOption {
	var out []JutsuOption
	for _, j := range c.Jutsu {
		if rankIndex(j.Rank) <= rankIndex(rule.MaxJutsuRank) {
			out = append(out, j)
		}
	}
	return out
}
