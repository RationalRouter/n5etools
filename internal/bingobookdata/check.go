package bingobookdata

import (
	"fmt"
	"strconv"
	"strings"
)

// JutsuRankCosts is the chakra cost the book prints for each jutsu rank.
// No transcribed block lists an S-Rank jutsu, so S has no entry.
var JutsuRankCosts = map[string]int{"E": 2, "D": 5, "C": 9, "B": 14, "A": 20}

var statRanks = map[string]bool{"D": true, "C": true, "B": true, "A": true, "S": true}
var statClasses = map[string]bool{"minion": true, "standard": true, "elite": true, "solo": true}

// Check reports internal inconsistencies in the transcribed data — the
// arithmetic a stat block must satisfy (an ability modifier follows from its
// score; passive Perception is 10 + the Perception bonus), required fields,
// and slug uniqueness. A transcription slip almost always breaks one of
// these. Named checks a block lists in Waive are skipped for that block.
func Check(blocks []StatBlock, traits []GeneralTrait) []string {
	var problems []string
	seen := map[string]bool{}
	for _, b := range blocks {
		waived := map[string]bool{}
		for _, w := range b.Waive {
			waived[w] = true
		}
		bad := func(check, format string, args ...any) {
			if !waived[check] {
				problems = append(problems, fmt.Sprintf("p%d %s: %s: %s", b.Page, b.Name, check, fmt.Sprintf(format, args...)))
			}
		}
		if len(b.Waive) > 0 && b.Notes == "" {
			bad("notes", "waives checks without explaining why in notes")
		}
		if !strings.HasPrefix(b.Slug, "adversary-template/") {
			bad("slug", "%q lacks the adversary-template/ prefix", b.Slug)
		}
		if seen[b.Slug] {
			bad("slug", "duplicate slug %q", b.Slug)
		}
		seen[b.Slug] = true
		if b.Name == "" || b.Page == 0 {
			bad("required", "missing name or page")
		}
		if !statRanks[b.Rank] {
			bad("rank", "invalid rank %q", b.Rank)
		}
		if !statClasses[b.Class] {
			bad("class", "invalid class %q", b.Class)
		}
		if (b.TenacityDie == "") != (b.TenacityCount == 0) {
			bad("tenacity", "die %q with count %d", b.TenacityDie, b.TenacityCount)
		}
		if _, err := FirstHP(b.HP); err != nil {
			bad("hp", "%v", err)
		}
		for _, a := range b.Abilities() {
			if want := modFor(a.Score); a.Mod != want {
				bad("mod", "%s %d has mod %+d, want %+d", a.Key, a.Score, a.Mod, want)
			}
			if a.Save < a.Mod {
				bad("save", "%s save %+d below its mod %+d", a.Key, a.Save, a.Mod)
			}
		}
		perception := b.Wis.Mod
		for _, s := range b.Skills {
			if s.Name == "Perception" {
				perception = s.Mod
			}
			if s.Stars < 0 || s.Stars > 3 {
				bad("skill", "%s has %d stars", s.Name, s.Stars)
			}
		}
		if b.PassivePerception != 10+perception {
			bad("passive", "passive Perception %d, want %d", b.PassivePerception, 10+perception)
		}
		for _, sec := range b.Sections {
			if !SectionKinds[sec.Kind] {
				bad("section", "invalid section kind %q", sec.Kind)
			}
			for _, e := range sec.Entries {
				if strings.TrimSpace(e.Text) == "" {
					bad("section", "%s entry %q has no text", sec.Title, e.Name)
				}
			}
		}
		for _, d := range b.Jutsu {
			for _, r := range d.Ranks {
				if want, ok := JutsuRankCosts[r.Rank]; !ok {
					bad("jutsu", "%s: invalid rank %q", d.Discipline, r.Rank)
				} else if r.Cost != want {
					bad("jutsu", "%s %s-Rank cost %d, want %d", d.Discipline, r.Rank, r.Cost, want)
				}
				names := map[string]bool{}
				for _, n := range r.Names {
					names[n] = true
				}
				for _, n := range r.New {
					if !names[n] {
						bad("jutsu", "%s %s-Rank: [New] %q not in names", d.Discipline, r.Rank, n)
					}
				}
			}
		}
	}

	traitSeen := map[string]bool{}
	for _, t := range traits {
		if t.Name == "" || t.Description == "" {
			problems = append(problems, fmt.Sprintf("general trait %q: missing name or description", t.Name))
		}
		if t.Rank != "" && !statRanks[t.Rank] {
			problems = append(problems, fmt.Sprintf("general trait %q: invalid rank %q", t.Name, t.Rank))
		}
		if traitSeen[TraitSlug(t.Name)] {
			problems = append(problems, fmt.Sprintf("general trait %q: duplicate slug", t.Name))
		}
		traitSeen[TraitSlug(t.Name)] = true
	}
	return problems
}

func modFor(score int) int {
	d := score - 10
	if d < 0 {
		return (d - 1) / 2
	}
	return d / 2
}

// FirstHP returns the first (full-health) phase of a printed HP value:
// 42 for "42", 360 for "360/216/108".
func FirstHP(hp string) (int, error) {
	first, _, _ := strings.Cut(hp, "/")
	n, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil {
		return 0, fmt.Errorf("unreadable HP %q", hp)
	}
	return n, nil
}

// TraitSlug is the slug of a General Traits glossary row.
func TraitSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == '\'' || r == '’':
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return "adversary-beta-general-trait/" + strings.TrimSuffix(b.String(), "-")
}
