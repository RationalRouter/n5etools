// Bloodline Latents parser (clan compendium, after the clan sections).
//
// The section opens with the BLOODLINE, LATENT feat (Category: Clan, Rare —
// grants 10 Bloodline Points), then one pricing table per clan:
//
//	LATENT ABURAME
//	Bloodline Ability Name | Bloodline Point Cost | Ability description
//	Latent Bug Host I        2   Beginning at 1st level, twice per long rest…
//	D-Rank Hijutsu           2   You gain the ability to learn D-Rank …
//
// The tables are extracted as text, but messily: ability names wrap across
// lines mid-name, and ordinal suffixes are superscript glyphs that arrive as
// their own lines ("…at 1" / "st" / "level, twice…"). Everything from
// "LEGACY CONTENT" on is skipped (legacy rules, out of scope for v1 data).
package parse

import (
	"regexp"
	"strconv"
	"strings"
)

// BloodlineLatent is one purchasable ability row from a clan's latent table.
type BloodlineLatent struct {
	ClanName      string // "Aburame" — from the LATENT <NAME> heading
	Name          string // "Latent Bug Host I"
	PointCost     int
	Prerequisites string // extracted "(You must have …)" prefix, if any
	Description   string
	SourcePage    int
}

var (
	latentHeadingRe = regexp.MustCompile(`^LATENT (.+)$`)
	prereqPrefixRe  = regexp.MustCompile(`^\((You must have [^)]+)\)[.,]?\s*`)
	superscriptRe   = regexp.MustCompile(`^(st|nd|rd|th)$`)
	// Point costs are a single digit — and 0 is real ("Latent Branch Family 0",
	// drawback abilities like Hozuki's "Lightning Release Inability 0").
	singleDigitRe = regexp.MustCompile(`^[0-9]$`)
	// Genwa names its abilities "Latent 1s and 0s I/II/III" — the only place
	// a name word starts with a digit.
	digitWordRe = regexp.MustCompile(`^\d+s$`)
)

// latentNameWord reports whether one token can be part of an ability name:
// a capitalized word ("Latent", "D-Rank", "Hijutsu"), a roman numeral, or a
// lowercase connector.
func latentNameWord(w string) bool {
	switch w {
	case "of", "the", "and", "with": // "Latent One with Earth" (Shoton)
		return true
	}
	if digitWordRe.MatchString(w) {
		return true
	}
	r := []rune(w)
	if len(r) == 0 || !(r[0] >= 'A' && r[0] <= 'Z' || r[0] >= 'À' && r[0] <= 'Ž') {
		return false
	}
	for _, c := range r {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 'À' && c <= 'ž' ||
			c == '’' || c == '\'' || c == '-' || c == '/') {
			return false
		}
	}
	return true
}

// latentHeaderLine reports whether a line is (part of) the wrapped column
// header ("Bloodline Ability Name", "Point Cost Ability description", …).
func latentHeaderLine(text string) bool {
	for _, w := range strings.Fields(strings.ToLower(text)) {
		switch w {
		case "bloodline", "ability", "name", "point", "cost", "description":
		default:
			return false
		}
	}
	return true
}

// ParseBloodlineLatents scans the whole clan-book line stream for the
// Bloodline Latents section. Returns the BLOODLINE, LATENT feat, the table
// rows, and anomalies.
func ParseBloodlineLatents(lines []Line) (*Feat, []BloodlineLatent, []Anomaly) {
	var (
		feat      *Feat
		latents   []BloodlineLatent
		anomalies []Anomaly
	)

	// Locate the section, apply the standard clan-book line filter.
	var ls []Line
	inSection := false
	for idx := 0; idx < len(lines); idx++ {
		ln := lines[idx]
		if !inSection {
			inSection = ln.Text == "BLOODLINE LATENTS"
			continue
		}
		if ln.Text == "LEGACY CONTENT" {
			break
		}
		if pageNumberRe.MatchString(ln.Text) || punctOnlyRe.MatchString(ln.Text) {
			continue
		}
		if ln.Text == "ART CREDIT" {
			idx++
			continue
		}
		ls = append(ls, ln)
	}
	if len(ls) == 0 {
		anomalies = append(anomalies, Anomaly{Subject: "Bloodline Latents",
			Problem: "section not found"})
		return nil, nil, anomalies
	}

	var (
		clanName    string
		cur         *BloodlineLatent // row being collected
		pendingName []string         // name fragments before the cost digit
	)
	finishRow := func() {
		if cur == nil {
			return
		}
		cur.Description = strings.TrimSpace(cur.Description)
		if m := prereqPrefixRe.FindStringSubmatch(cur.Description); m != nil {
			cur.Prerequisites = m[1]
			cur.Description = strings.TrimSpace(cur.Description[len(m[0]):])
		}
		latents = append(latents, *cur)
		cur = nil
	}

	for i := 0; i < len(ls); i++ {
		text := ls[i].Text

		if m := latentHeadingRe.FindStringSubmatch(text); m != nil && capsLineRe.MatchString(text) {
			finishRow()
			if len(pendingName) > 0 {
				anomalies = append(anomalies, Anomaly{Page: ls[i].Page, Subject: "LATENT " + m[1],
					Problem: "orphaned name fragments before heading: " + strings.Join(pendingName, " ")})
				pendingName = nil
			}
			clanName = tidyName(m[1])
			continue
		}

		// The BLOODLINE, LATENT feat entry at the section top.
		if feat == nil && startsFeat(ls, i) {
			f, next := parseFeat(ls, i)
			feat = &f
			i = next - 1
			continue
		}

		if clanName == "" || latentHeaderLine(text) {
			continue
		}

		// Superscript ordinal suffix on its own line: glue to what precedes.
		if superscriptRe.MatchString(text) {
			if cur != nil {
				cur.Description = strings.TrimRight(cur.Description, " ") + text
			}
			continue
		}

		fields := strings.Fields(text)

		// "<cost> <description…>" completing a wrapped name.
		if len(pendingName) > 0 && len(fields) > 0 && singleDigitRe.MatchString(fields[0]) {
			finishRow()
			n, _ := strconv.Atoi(fields[0])
			cur = &BloodlineLatent{
				ClanName: clanName, Name: strings.Join(pendingName, " "),
				PointCost: n, Description: strings.Join(fields[1:], " "),
				SourcePage: ls[i].Page,
			}
			pendingName = nil
			continue
		}

		// "<name…> <cost> <description…>" on one line: find the cost digit
		// with only name-like words before it. The description part may be
		// empty — some tables put the cost at the end of the name's last
		// wrapped line ("Chakra I 2") with the description on the next line.
		costIdx := costSplit(fields)
		if costIdx > 0 {
			finishRow()
			name := strings.Join(append(append([]string{}, pendingName...), fields[:costIdx]...), " ")
			pendingName = nil
			n, _ := strconv.Atoi(fields[costIdx])
			cur = &BloodlineLatent{
				ClanName: clanName, Name: name, PointCost: n,
				Description: strings.Join(fields[costIdx+1:], " "),
				SourcePage:  ls[i].Page,
			}
			continue
		}

		// A short all-name-like line may be a wrapped name fragment — real
		// only if a cost digit follows within the next few lines, either
		// leading its own line ("2 Beginning at…") or ending a further name
		// line ("Chakra I 2"). A description wrap never leads to either.
		if len(fields) <= 4 && allWordsNameLike(fields) {
			frag := append([]string{}, fields...)
			j := i + 1
			// Window of 4: names wrap across up to four lines ("Latent" /
			// "Corrupted" / "Chakra Mode" / "II" / "3 (You must…").
			for j < len(ls) && j <= i+4 {
				nf := strings.Fields(ls[j].Text)
				if len(nf) > 0 && (singleDigitRe.MatchString(nf[0]) || costSplit(nf) > 0) {
					pendingName = append(pendingName, frag...)
					break
				}
				if len(nf) <= 4 && allWordsNameLike(nf) {
					frag = append(frag, nf...)
					j++
					continue
				}
				break
			}
			if len(pendingName) > 0 {
				i = j - 1
				continue
			}
			// Not a name — fall through as description text.
		}

		// Description continuation.
		if cur != nil {
			cur.Description += " " + text
			continue
		}
		anomalies = append(anomalies, Anomaly{Page: ls[i].Page, Subject: "LATENT " + clanName,
			Problem: "text outside any table row: " + snippet(text)})
	}
	finishRow()
	if len(pendingName) > 0 {
		anomalies = append(anomalies, Anomaly{Subject: "Bloodline Latents",
			Problem: "orphaned name fragments at section end: " + strings.Join(pendingName, " ")})
	}
	if feat == nil {
		anomalies = append(anomalies, Anomaly{Subject: "Bloodline Latents",
			Problem: "BLOODLINE, LATENT feat entry not found"})
	}
	return feat, latents, anomalies
}

// costSplit finds the index of a lone cost digit preceded only by name-like
// words ("Latent Bug Host I 2 Beginning…" → 4). Returns -1 when the line has
// no such split, meaning it is not a "name then cost" table row.
func costSplit(fields []string) int {
	for k := 1; k < len(fields) && k <= 6; k++ {
		if singleDigitRe.MatchString(fields[k]) && allWordsNameLike(fields[:k]) {
			return k
		}
	}
	return -1
}

func allWordsNameLike(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, w := range fields {
		if !latentNameWord(w) {
			return false
		}
	}
	return true
}
