// Core book: Weapon and Armor Enhancement Seals (Chapter 7 downtime
// crafting). Template, identical for both sections:
//
//	WEAPON ENHANCEMENT SEALS      (or ARMOR ENHANCEMENT SEALS)
//	  D-RANK SEALS                 rank sub-heading, purely organizational
//	    <NAME> (<TIER>)             caps line; tier is one of five fixed words
//	    Ryo Cost: <N>
//	      prose description
//	    ...
//	  C-RANK SEALS
//	  ...
//
// The tier word on each seal ("MINOR"/"REFINED"/"GREATER"/"SUPERIOR"/
// "MASTERCRAFT") is the reliable rank signal — MASTERCRAFT entries print
// under the "A-RANK SEALS" heading (the book has no S-RANK heading) but are
// genuinely a step above SUPERIOR, so rank is derived from the tier word,
// not the section heading; a mismatch between the two is flagged.
package parse

import (
	"regexp"
	"strconv"
	"strings"
)

// EnhancementSeal is one seal entry.
type EnhancementSeal struct {
	Name        string
	Tier        string // "Minor", "Refined", "Greater", "Superior", "Mastercraft"
	Rank        string // "D", "C", "B", "A", "S" — derived from Tier
	AppliesTo   string // "weapon" or "armor"
	CostRyo     int
	Description string
	SourcePage  int
}

var (
	sealNameRe = regexp.MustCompile(`^(.+) \((MINOR|REFINED|GREATER|SUPERIOR|MASTERCRAFT)\)$`)
	rankHeadRe = regexp.MustCompile(`^([DCBA])-RANK SEALS$`)
)

// tierRank is the tier→rank mapping the book's own naming implies; used both
// to fill Rank and to cross-check against the section heading.
var tierRank = map[string]string{
	"MINOR": "D", "REFINED": "C", "GREATER": "B", "SUPERIOR": "A", "MASTERCRAFT": "S",
}

// ParseEnhancementSeals scans the whole core-book line stream for the two
// seal sections (they run back-to-back, ending at "LEARNING/ CREATING A
// JUTSU").
func ParseEnhancementSeals(lines []Line) ([]EnhancementSeal, []Anomaly) {
	var (
		seals     []EnhancementSeal
		anomalies []Anomaly
	)

	var ls []Line
	inSection := false
	for _, ln := range lines {
		switch ln.Text {
		case "WEAPON ENHANCEMENT SEALS":
			inSection = true
		case "LEARNING/ CREATING A JUTSU":
			inSection = false
		}
		if !inSection {
			continue
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		return nil, []Anomaly{{Subject: "Enhancement Seals", Problem: "sections not found"}}
	}

	appliesTo := ""   // set on the WEAPON/ARMOR section heading
	rankHeading := "" // current "D-RANK SEALS" etc., for the cross-check
	var cur *EnhancementSeal
	flush := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(cur.Description)
		if cur.Description == "" {
			anomalies = append(anomalies, Anomaly{Page: cur.SourcePage, Subject: cur.Name,
				Problem: "seal with empty description"})
		} else {
			seals = append(seals, *cur)
		}
		cur = nil
	}

	for _, ln := range ls {
		text := ln.Text
		switch text {
		case "WEAPON ENHANCEMENT SEALS":
			flush()
			appliesTo = "weapon"
			continue
		case "ARMOR ENHANCEMENT SEALS":
			flush()
			appliesTo = "armor"
			continue
		}
		if m := rankHeadRe.FindStringSubmatch(text); m != nil {
			flush()
			rankHeading = m[1]
			continue
		}
		if m := sealNameRe.FindStringSubmatch(text); m != nil && capsLineRe.MatchString(text) {
			flush()
			rank := tierRank[m[2]]
			if rankHeading != "" && rank != "S" && rank != rankHeading {
				anomalies = append(anomalies, Anomaly{Page: ln.Page, Subject: tidyName(m[1]),
					Problem: "tier " + m[2] + " (rank " + rank + ") printed under " + rankHeading + "-RANK SEALS heading"})
			}
			cur = &EnhancementSeal{
				Name: tidyName(m[1]), Tier: tidyName(m[2]), // tidyName expects ALL-CAPS input
				Rank: rank, AppliesTo: appliesTo, SourcePage: ln.Page,
			}
			continue
		}
		if cur == nil {
			continue // between-section prose, rank-heading intro text, etc.
		}
		if rest, ok := strings.CutPrefix(text, "Ryo Cost:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil {
				anomalies = append(anomalies, Anomaly{Page: ln.Page, Subject: cur.Name,
					Problem: "Ryo Cost not a plain number: " + rest})
			}
			cur.CostRyo = n
			continue
		}
		cur.Description += " " + text
	}
	flush()

	return seals, anomalies
}
