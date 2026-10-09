// Parser for the community "N5e Adversary Builder" Google Sheet, distributed
// as an xlsx export (community filename "BingoBook.xlsx" — an unrelated,
// non-official spreadsheet that happens to share a name with the real Bingo
// Book PDF chapter already covered by bingobook.go). Its TraitsMain sheet is
// a flat catalog of clan-, affiliation-, class-mod-, and role-flavored
// traits a GM can hand a generated adversary, plus a small "Iconic" named-
// ability catalog. Unlike the official Bingo Book PDF, this is not a page of
// prose to be line-parsed — it's already a spreadsheet, so this parser reads
// cells directly via excelize rather than reusing bingobook.go's regex/line
// state machine.
package parse

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// AdversaryBuilderTrait is one row of the sheet's TraitsMain catalog.
type AdversaryBuilderTrait struct {
	Category    string // general, role, clan, affiliation, class_mod, passive, iconic
	CategoryTag string // the sheet's own tag column, e.g. "Aburame", "Cursed Seal", "Controller", "Iconic"
	Name        string
	Rank        string // "", D, C, B, A, S
	Description string
	Keywords    string
	RowNumber   int // 1-based sheet row, for stable ordering and diagnostics
}

const adversaryBuilderSheetName = "TraitsMain"

// adversaryBuilderFirstRow/LastRow bound the sheet's real content — row 1-2
// are title/header, and rows 949-951 are the sheet author's own scratch
// entries ("Test"/"Fight" joke rows with no real trait data), confirmed by
// direct inspection, not assumed from a blank-row heuristic.
const (
	adversaryBuilderFirstRow = 3
	adversaryBuilderLastRow  = 948
)

// traitTypeToCategory maps the sheet's own "Trait Type" column (F) to this
// package's category enum. A row whose category_tag (column A) is exactly
// "Iconic" is reclassified to the iconic category regardless of its nominal
// Trait Type ("Role") — see the Iconic-catalog doc comment below.
var traitTypeToCategory = map[string]string{
	"General":     "general",
	"Role":        "role",
	"Clan":        "clan",
	"Affiliation": "affiliation",
	"Class Mod":   "class_mod",
	"Passives":    "passive",
}

// adversaryBuilderRankNormalize maps the sheet's raw Rank column (C) values
// to this project's single-letter convention. Everything not listed here
// (e.g. "No rank", "No Role Rank", "-", "None", or a truly blank cell)
// normalizes to "" — legitimately unranked/passive content, not an error.
// "A-Ran" is a single confirmed sheet typo (row 319, "Elemental Savagry").
var adversaryBuilderRankNormalize = map[string]string{
	"D-Rank": "D",
	"C-Rank": "C",
	"B-Rank": "B",
	"A-Rank": "A",
	"S-Rank": "S",
	"A-Ran":  "A",
}

// adversaryBuilderKnownUnranked lists the sheet's own confirmed spellings
// for "no rank applies here" — normalizes silently to "", no anomaly. Any
// OTHER unrecognized raw value still raises an anomaly, since it's more
// likely a new typo than a new legitimate unranked spelling.
var adversaryBuilderKnownUnranked = map[string]bool{
	"No rank":                     true,
	"No Role Rank":                true,
	"No Affiliation Passive Rank": true,
	"-":                           true,
	"None":                        true,
}

// AdversaryBuilderSheet parses the TraitsMain sheet of the community
// Adversary Builder xlsx export into a clean, filtered trait catalog.
//
// A row is kept only if its Name (col B) and Description (col D) are both
// non-empty AND its Rank (col C) is not the literal string "Rank" — that
// last check catches repeated-header rows scattered mid-sheet (605, 673,
// 796, 867, 902) whose Description cell held placeholder text ("Description",
// "Passive/Ability Description Description") rather than being empty, which
// a plain empty-Description filter alone would miss. Columns I-M are
// unrelated dropdown-validation source lists reused across unrelated rows,
// not per-row data, and are deliberately never read here.
func AdversaryBuilderSheet(path string) ([]AdversaryBuilderTrait, []Anomaly, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var traits []AdversaryBuilderTrait
	var anomalies []Anomaly

	for row := adversaryBuilderFirstRow; row <= adversaryBuilderLastRow; row++ {
		categoryTag := strings.TrimSpace(cellAt(f, adversaryBuilderSheetName, "A", row))
		name := strings.TrimSpace(cellAt(f, adversaryBuilderSheetName, "B", row))
		rawRank := strings.TrimSpace(cellAt(f, adversaryBuilderSheetName, "C", row))
		description := strings.TrimSpace(cellAt(f, adversaryBuilderSheetName, "D", row))
		keywords := strings.TrimSpace(cellAt(f, adversaryBuilderSheetName, "E", row))
		traitType := strings.TrimSpace(cellAt(f, adversaryBuilderSheetName, "F", row))

		if name == "" || description == "" || rawRank == "Rank" {
			continue
		}

		category, ok := traitTypeToCategory[traitType]
		if !ok {
			anomalies = append(anomalies, Anomaly{
				Page:    row,
				Subject: name,
				Problem: fmt.Sprintf("unrecognized Trait Type %q", traitType),
			})
			category = "general"
		}
		if categoryTag == "Iconic" {
			category = "iconic"
		}

		rank := ""
		if rawRank != "" {
			normalized, ok := adversaryBuilderRankNormalize[rawRank]
			if !ok && !adversaryBuilderKnownUnranked[rawRank] {
				anomalies = append(anomalies, Anomaly{
					Page:    row,
					Subject: name,
					Problem: fmt.Sprintf("unrecognized Rank value %q, treated as unranked", rawRank),
				})
			}
			rank = normalized
		}

		traits = append(traits, AdversaryBuilderTrait{
			Category:    category,
			CategoryTag: categoryTag,
			Name:        name,
			Rank:        rank,
			Description: description,
			Keywords:    keywords,
			RowNumber:   row,
		})
	}

	return traits, anomalies, nil
}

func cellAt(f *excelize.File, sheet, col string, row int) string {
	v, err := f.GetCellValue(sheet, fmt.Sprintf("%s%d", col, row))
	if err != nil {
		return ""
	}
	return v
}
