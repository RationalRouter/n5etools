// Package sheet reads the community Mastersheet xlsx — the friend's Google
// Sheet that the N5E community actually plays from.
//
// Source-of-truth split (agreed with the maintainer): the PDFs stay
// authoritative for prose and relational structure; the Mastersheet is
// authoritative for the TABULAR data that only exists as images in the PDFs
// — class progression charts, subclass level charts, and armor/weapon
// stats. This package replaces the OCR tier the plan originally called for.
//
// ClassChartMaster layout, one block per class:
//
//	row r   col A: "Genjutsu-Specialist"        ← block marker
//	row r+1 cols B-E: name, hit die, chakra die, (internal helper)
//	row r+2 cols B-…: header — Level | Proficiency Bonus | Features |
//	                  <zero or more class resources> | [Jutsu Known]
//	row r+3…r+22: the twenty level rows
//
// Cell formats are inconsistent ("+3" vs 3.0, "-" for none, dice strings
// like "3d8"); everything is normalized here so the loader sees clean data.
package sheet

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Anomaly mirrors parse.Anomaly (this package stays independent of parse).
type Anomaly struct {
	Subject string
	Problem string
}

// Resource is one class-specific per-level column ("Malleable Mirages" = 2).
// Values stay strings — some are dice ("3d8"), some counts, some bonuses.
type Resource struct {
	Name  string
	Value string
}

// LevelRow is one row of a class's progression chart.
type LevelRow struct {
	Level        int
	ProfBonus    int
	FeaturesText string // verbatim; used by the validation cross-check
	JutsuKnown   *int   // nil when the class has no Jutsu Known column
	Resources    []Resource
}

// ClassChart is one class block from ClassChartMaster.
type ClassChart struct {
	Name      string // as printed in the sheet: "Genjutsu-Specialist"
	HitDie    int
	ChakraDie int
	Levels    []LevelRow
}

var profRe = regexp.MustCompile(`^\+?(\d+)$`)

// cellInt reads "+3", "3", or 3.0 as an int; ok=false for "-" and blanks.
func cellInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if m := profRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f), true
	}
	return 0, false
}

// cleanCell trims and collapses the sheet's stray spacing
// ("Puppet Technique ( 6 )" → "Puppet Technique (6)").
func cleanCell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "( ", "(")
	s = strings.ReplaceAll(s, " )", ")")
	s = strings.ReplaceAll(s, " ,", ",")
	return s
}

// knownResourceCellFixes hand-corrects a small, closed list of Mastersheet
// resource-column cells that read wrong even after cleanCell's generic
// whitespace normalization — a stray artifact baked into the source
// spreadsheet cell itself, not a formatting shape cleanCell already
// handles. Confirmed directly against the shipped rules.db: Science-Nin's
// own Creation Points column reads "18 ." at 9th level (every neighboring
// level in the same column reads a clean integer — "16" at 8th, "20" at
// 10th, each a flat +2 step) — strconv.Atoi silently fails on the stray
// " ." suffix, which cmd/n5e's classLevelResourceInt treats as "no value"
// (returns 0, nil, not an error) rather than surfacing the problem, so this
// one level's own Creation Points budget would silently read 0 instead of
// 18 without this fix. Keyed by (class name as printed, level, resource
// name) so a coincidentally-matching value elsewhere in the sheet is never
// silently rewritten; extend one confirmed instance at a time, same
// discipline as internal/store/classoptionentries.go's own known-fix maps.
var knownResourceCellFixes = map[[3]string]string{
	{"Science-Nin", "9", "Creation Points"}: "18",
}

// ParseClassCharts reads the ClassChartMaster tab.
func ParseClassCharts(path string) ([]ClassChart, []Anomaly, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	rows, err := f.GetRows("ClassChartMaster")
	if err != nil {
		return nil, nil, fmt.Errorf("reading ClassChartMaster: %w", err)
	}

	cell := func(r, c int) string { // 0-based, safe on ragged rows
		if r < len(rows) && c < len(rows[r]) {
			return strings.TrimSpace(rows[r][c])
		}
		return ""
	}

	var charts []ClassChart
	var anomalies []Anomaly
	flag := func(subject, problem string) {
		anomalies = append(anomalies, Anomaly{Subject: subject, Problem: problem})
	}

	for r := 0; r < len(rows); r++ {
		name := cell(r, 0)
		if name == "" {
			continue
		}
		chart := ClassChart{Name: name}

		// Meta row: name again, hit die, chakra die.
		if got := cell(r+1, 1); got != name {
			flag(name, fmt.Sprintf("meta row name %q does not match block marker", got))
		}
		if n, ok := cellInt(cell(r+1, 2)); ok {
			chart.HitDie = n
		} else {
			flag(name, "hit die not readable: "+cell(r+1, 2))
		}
		if n, ok := cellInt(cell(r+1, 3)); ok {
			chart.ChakraDie = n
		} else {
			flag(name, "chakra die not readable: "+cell(r+1, 3))
		}

		// Header row: fixed Level/Proficiency Bonus/Features, then class
		// resources, with Jutsu Known last when present.
		var headers []string
		for c := 1; ; c++ {
			h := cleanCell(cell(r+2, c))
			if h == "" {
				break
			}
			headers = append(headers, h)
		}
		if len(headers) < 3 || headers[0] != "Level" || headers[2] != "Features" {
			flag(name, fmt.Sprintf("unexpected header row: %v", headers))
			continue
		}
		jutsuKnownCol := -1
		var resourceCols []int // header indexes that are class resources
		for i, h := range headers[3:] {
			if h == "Jutsu Known" {
				jutsuKnownCol = 3 + i
			} else {
				resourceCols = append(resourceCols, 3+i)
			}
		}

		for lvl := 1; lvl <= 20; lvl++ {
			row := r + 2 + lvl
			lr := LevelRow{Level: lvl}
			wantLevel := fmt.Sprintf("%d", lvl)
			if got := cell(row, 1); !strings.HasPrefix(got, wantLevel) {
				flag(name, fmt.Sprintf("level %d row reads %q", lvl, got))
			}
			if n, ok := cellInt(cell(row, 2)); ok {
				lr.ProfBonus = n
			} else {
				flag(name, fmt.Sprintf("level %d: proficiency bonus not readable: %q", lvl, cell(row, 2)))
			}
			lr.FeaturesText = cleanCell(cell(row, 3))
			if jutsuKnownCol >= 0 {
				if n, ok := cellInt(cell(row, 1+jutsuKnownCol)); ok {
					lr.JutsuKnown = &n
				}
			}
			for _, rc := range resourceCols {
				v := cleanCell(cell(row, 1+rc))
				if fixed, ok := knownResourceCellFixes[[3]string{name, wantLevel, headers[rc]}]; ok {
					v = fixed
				}
				if v == "" || v == "-" {
					continue
				}
				lr.Resources = append(lr.Resources, Resource{Name: headers[rc], Value: v})
			}
			chart.Levels = append(chart.Levels, lr)
		}
		charts = append(charts, chart)
		r += 22 // skip past this block
	}

	if len(charts) == 0 {
		flag("ClassChartMaster", "no class blocks found")
	}
	return charts, anomalies, nil
}
