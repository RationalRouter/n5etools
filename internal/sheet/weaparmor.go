// WeapArmor tab: armor and weapon stat tables, images in the core book's
// Chapter 5. Two independent flat lists in one sheet (see package doc in
// classcharts.go for the Mastersheet's general shape):
//
//	row 6:  armor header  (Armor Name | Calculation Type | Ability Mod x2 |
//	                        Max Mod | Armor Bonus | Bulk)
//	row 7…: armor rows, until a "[[...]]" divider marks the friend's own
//	        homebrew custom-armor section (out of scope — official content
//	        only)
//	row 58: weapon header (Attack/Weapon Name | Damage Dice | Dmg. Type |
//	                        Misc. Atk./Dmg. Bonus | Qualities)
//	row 60…: weapon rows, same divider convention ("[[Custom Weapons]]")
//
// Neither table carries cost or weight — those come from the core book's
// own Chapter 5 prose (a separate parser) and will fill in via the same
// slug on a later load; this loader only ever sets the columns it owns.
package sheet

import (
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Armor is one row of the armor table.
type Armor struct {
	Name       string
	CalcType   string // "Light Armor", "Medium Armor", "Heavy Armor", "Custom", "Ability Mod"
	Ability1   string // "DEX", "INT", "NONE", ...
	Ability2   string // second modifier, usually "NONE" or "PROF"
	MaxMod     *float64
	ArmorBonus float64
	Bulk       float64
}

// Weapon is one row of the weapon table.
type Weapon struct {
	Name       string
	DamageDice string
	DamageType string
	Qualities  string // printed properties, verbatim
}

// isDivider reports whether a row marks the end of official content
// ("[[Custom Armor Below Here]]", "[[Custom Weapons]]").
func isDivider(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "[[")
}

func parseFloatCell(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

// ParseWeapArmor reads the WeapArmor tab's official (non-homebrew) armor
// and weapon rows.
func ParseWeapArmor(path string) ([]Armor, []Weapon, []Anomaly, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, nil, nil, err
	}
	defer f.Close()

	rows, err := f.GetRows("WeapArmor")
	if err != nil {
		return nil, nil, nil, err
	}
	cell := func(r, c int) string { // 0-based
		if r < len(rows) && c < len(rows[r]) {
			return strings.TrimSpace(rows[r][c])
		}
		return ""
	}

	var anomalies []Anomaly
	flag := func(subject, problem string) {
		anomalies = append(anomalies, Anomaly{Subject: subject, Problem: problem})
	}

	var armors []Armor
	// Armor rows start right after the header at row 6 (index 5).
	for r := 6; r < len(rows); r++ {
		name := cell(r, 0)
		if name == "" {
			continue
		}
		if isDivider(name) {
			break
		}
		a := Armor{Name: name, CalcType: cell(r, 6), Ability1: cell(r, 12), Ability2: cell(r, 16)}
		if v, ok := parseFloatCell(cell(r, 20)); ok {
			a.MaxMod = &v
		}
		if v, ok := parseFloatCell(cell(r, 24)); ok {
			a.ArmorBonus = v
		} else {
			flag(name, "armor bonus not readable: "+cell(r, 24))
		}
		if v, ok := parseFloatCell(cell(r, 32)); ok {
			a.Bulk = v
		} else {
			flag(name, "bulk not readable: "+cell(r, 32))
		}
		armors = append(armors, a)
	}

	var weapons []Weapon
	// Weapon header is row 58 (index 57); rows start at 60 (index 59).
	for r := 59; r < len(rows); r++ {
		name := cell(r, 0)
		if name == "" {
			continue
		}
		if isDivider(name) {
			break
		}
		weapons = append(weapons, Weapon{
			Name: name, DamageDice: cell(r, 7), DamageType: cell(r, 24), Qualities: cell(r, 34),
		})
	}

	if len(armors) == 0 {
		flag("WeapArmor", "no armor rows found")
	}
	if len(weapons) == 0 {
		flag("WeapArmor", "no weapon rows found")
	}
	return armors, weapons, anomalies, nil
}
