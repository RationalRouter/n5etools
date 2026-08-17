// Core book parser (Naruto 5e - Full Document). Parsed in slices, one
// section family per function:
//
//   - ParseFightingStances: Chapter 13's FIGHTING STANCES section — 21
//     stances under TAIJUTSU STANCES / BUKIJUTSU STANCES kind headings.
//   - ParseCoreFeats: Chapter 13's FEATS section — seven Category sections
//     of ordinary feat blocks, same shape as the clan and class books.
//
// Remaining core-book slices (backgrounds, equipment, enhancement seals,
// jutsu-creation costs, multiclassing) land in later functions.
package parse

import "strings"

// FightingStance is one stance option from Chapter 13.
type FightingStance struct {
	Name          string
	StanceType    string // "taijutsu" or "bukijutsu", from the kind heading
	Prerequisites string
	Description   string
	SourcePage    int
}

// ParseFightingStances scans the core-book line stream for the FIGHTING
// STANCES section (it ends where the FEATS section begins).
func ParseFightingStances(lines []Line) ([]FightingStance, []Anomaly) {
	var (
		stances   []FightingStance
		anomalies []Anomaly
	)

	var ls []Line
	inSection := false
	for _, ln := range lines {
		if !inSection {
			inSection = ln.Text == "FIGHTING STANCES"
			continue
		}
		if ln.Text == "FEATS" {
			break
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		anomalies = append(anomalies, Anomaly{Subject: "Fighting Stances",
			Problem: "section not found"})
		return nil, anomalies
	}

	kind := ""
	var cur *FightingStance
	flush := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(cur.Description)
		if cur.Description == "" {
			anomalies = append(anomalies, Anomaly{Page: cur.SourcePage, Subject: cur.Name,
				Problem: "stance with empty description"})
		} else {
			stances = append(stances, *cur)
		}
		cur = nil
	}
	for _, ln := range ls {
		text := ln.Text
		if capsLineRe.MatchString(text) {
			switch text {
			case "TAIJUTSU STANCES":
				flush()
				kind = "taijutsu"
				continue
			case "BUKIJUTSU STANCES":
				flush()
				kind = "bukijutsu"
				continue
			}
			if kind == "" {
				// Still in the section intro; a caps line here is decorative.
				continue
			}
			flush()
			cur = &FightingStance{Name: tidyName(text), StanceType: kind, SourcePage: ln.Page}
			continue
		}
		if cur == nil {
			continue // section intro prose (the shared stance rules)
		}
		if rest, ok := strings.CutPrefix(text, "Prerequisite:"); ok {
			cur.Prerequisites = strings.TrimSpace(rest)
			continue
		}
		cur.Description += " " + text
	}
	flush()
	return stances, anomalies
}

// coreFeatSections are the navigational headings of the core FEATS chapter.
var coreFeatSections = map[string]bool{
	"FEATS":          true,
	"GENERAL FEATS":  true,
	"SKILL FEATS":    true,
	"CHAKRA FEATS":   true,
	"NINJUTSU FEATS": true,
	"TAIJUTSU FEATS": true,
	"GENJUTSU FEATS": true,
	"CRITICAL FEATS": true,
}

// ParseCoreFeats scans the core-book line stream for Chapter 13's FEATS
// section (GENERAL FEATS up to Chapter 14).
func ParseCoreFeats(lines []Line) ([]Feat, []Anomaly) {
	var ls []Line
	inSection := false
	for _, ln := range lines {
		if !inSection {
			inSection = ln.Text == "GENERAL FEATS"
			if !inSection {
				continue
			}
		}
		if strings.HasPrefix(ln.Text, "CHAPTER 14") {
			break
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		return nil, []Anomaly{{Subject: "Core Feats", Problem: "FEATS section not found"}}
	}
	return parseFeatRun(ls, func(text string) bool {
		return coreFeatSections[text]
	}, "Core Feats")
}
