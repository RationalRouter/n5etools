// Jutsu compendium's Summoning chapter: 20 summon tribe stat blocks (Bear,
// Boar, Dog/Wolf, ... Weasel). A new entity type — creature templates a
// summoner fills in at cast time, not characters or jutsu.
//
// Per-tribe template, always in this order:
//
//	<NAME>                          caps heading (may contain "/", wraps as
//	                                 "HARE/ RABBIT" — extra space tidied)
//	  flavor prose
//	Summon Type: …
//	Toughness: …
//	Defensive Ability Score: …
//	Saving Throws: …
//	Creature Skills: …
//	Creature Senses: …               (2 of 20 tribes print "Senses:" instead)
//	ROLES
//	  intro prose
//	  • RoleName: description        one bullet per selectable role; any
//	                                  unbulleted line that follows is a
//	                                  continuation of the role above it
//	NATURAL/WEAPONS
//	  AttackName. description        e.g. "Claws. Melee Weapon Attack: …"
//	SAVE DC'S & ATTACK BONUSES:
//	  All Jutsu Save DC's: …
//	  All Jutsu Attack bonus: …
//	SPECIAL FEATURES:
//	  D-RANK / C-RANK / B-RANK / A-RANK / S-RANK   rank sub-headings
//	    FeatureName: description     one or more per rank
//	<NAME> JUTSU SPECIALTY
//	  keyword access list            kept as one raw block
//	<NAME>                            heading repeats, opens the stat table
//	  Rank Level Size STR DEX CON INT WIS CHA Jutsu Slots Jutsu Speed
//	  D-Rank 4th M 16 10 14 10 12 10 4 2 D-Rank 30ft      (ordinal suffix
//	  ...                                                  wraps onto its
//	                                                        own line, as in
//	                                                        the latent
//	                                                        tables)
package parse

import (
	"regexp"
	"strconv"
	"strings"
)

type SummonRole struct {
	Name        string
	Description string
}

type SummonAttack struct {
	Name        string
	Description string
}

type SummonFeature struct {
	Name        string
	Rank        string
	Description string
	SourcePage  int
}

type SummonProgressionRow struct {
	Rank      string
	Level     int
	SizeText  string
	StatsText string
}

type SummonTribe struct {
	Name             string
	SummonType       string
	Description      string
	Toughness        int
	DefensiveAbility string
	SavingThrows     string
	Skills           string
	Senses           string
	JutsuSaveDCText  string
	JutsuAttackText  string
	JutsuSpecialty   string
	Roles            []SummonRole
	Attacks          []SummonAttack
	Features         []SummonFeature
	Progression      []SummonProgressionRow
	SourcePage       int
}

var (
	summonRankHeadingRe = regexp.MustCompile(`^([DCBAS])-RANK$`)
	summonAttackNameRe  = regexp.MustCompile(`^([A-Z][a-zA-Z/ ]{1,24})\.\s+(.*)$`)
	// Most features are "Name: description". A few print "Name. description"
	// instead (confirmed against real entries — "Tokage’s Martial Skill.
	// This summon can instead be summoned..."), but period alone is too
	// loose to trust anywhere: a wrapped mid-sentence line can coincidentally
	// look identical ("...immune to\nPoison and Acid Damage. If it would...").
	// The period form is only ever used as the FIRST feature under a rank
	// heading in this book, so it's accepted only when no feature is
	// currently being built (see modeFeatures below) — never as a possible
	// interruption of one already in progress. ’ is the curly apostrophe.
	summonFeatureColonRe  = regexp.MustCompile(`^([A-Z][\w'’ /-]{1,40}):\s+(.*)$`)
	summonFeaturePeriodRe = regexp.MustCompile(`^([A-Z][\w'’ /-]{1,40})\.\s+(.*)$`)
	summonProgOrdinalRe   = regexp.MustCompile(`^(st|nd|rd|th)$`)
	// "D-Rank 4th M 16 10 14 10 12 10 4 2 D-Rank 30ft" or, at higher ranks,
	// "C-Rank 8th M-L +6 Ability Score Increases up to 20. 6 2 D-Rank, 2 C-Rank 30ft"
	summonProgRowRe = regexp.MustCompile(`^([DCBAS])-Rank (\d+)(?:st|nd|rd|th) (\S+) (.*)$`)
)

// startsSummonTribe reports whether the caps line at i opens a tribe entry
// — a "Summon Type:" label appears before the next caps heading. Flavor
// paragraphs vary widely in length (some tribes run 10+ wrapped lines), so
// the scan runs until the next heading rather than a fixed line count.
func startsSummonTribe(ls []Line, i int) bool {
	if !capsLineRe.MatchString(ls[i].Text) {
		return false
	}
	for j := i + 1; j < len(ls); j++ {
		if strings.HasPrefix(ls[j].Text, "Summon Type:") {
			return true
		}
		if capsLineRe.MatchString(ls[j].Text) {
			return false
		}
	}
	return false
}

// ParseSummonTribes scans the jutsu compendium's whole line stream for the
// Summoning chapter (it ends where the Genjutsu chapter begins).
func ParseSummonTribes(lines []Line) ([]SummonTribe, []Anomaly) {
	var (
		tribes    []SummonTribe
		anomalies []Anomaly
	)

	var ls []Line
	inSection := false
	for _, ln := range lines {
		if ln.Text == "SUMMONING JUTSU" {
			inSection = true
		}
		if ln.Text == "GENJUTSU" && inSection {
			break
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
		return nil, []Anomaly{{Subject: "Summon Tribes", Problem: "chapter not found"}}
	}

	var starts []int
	for i := range ls {
		if startsSummonTribe(ls, i) {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		return nil, []Anomaly{{Subject: "Summon Tribes", Problem: "no tribe entries found"}}
	}

	for k, start := range starts {
		end := len(ls)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		tribe, anoms := parseSummonTribe(ls[start:end])
		tribes = append(tribes, tribe)
		anomalies = append(anomalies, anoms...)
	}
	return tribes, anomalies
}

func parseSummonTribe(ls []Line) (SummonTribe, []Anomaly) {
	var anomalies []Anomaly
	name := tidyName(strings.Join(strings.Fields(strings.ReplaceAll(ls[0].Text, "/", "/ ")), " "))
	name = strings.ReplaceAll(name, "/ ", "/") // undo the padding, now evenly spaced either way
	t := SummonTribe{Name: name, SourcePage: ls[0].Page}
	flag := func(page int, problem string) {
		anomalies = append(anomalies, Anomaly{Page: page, Subject: t.Name, Problem: problem})
	}

	const (
		modeFlavor = iota
		modeHeader
		modeRoles
		modeAttacks
		modeSaveDC
		modeFeatures
		modeSpecialty
		modeProgHeader // between the repeated name heading and the table header row
		modeProgTable
	)
	mode := modeFlavor
	var desc []string
	rank := "" // current SPECIAL FEATURES rank sub-heading
	var curRole *SummonRole
	var curAttack *SummonAttack
	var curFeature *SummonFeature
	var specialty []string

	flushRole := func() {
		if curRole != nil {
			curRole.Description = strings.TrimSpace(curRole.Description)
			t.Roles = append(t.Roles, *curRole)
			curRole = nil
		}
	}
	flushAttack := func() {
		if curAttack != nil {
			curAttack.Description = strings.TrimSpace(curAttack.Description)
			t.Attacks = append(t.Attacks, *curAttack)
			curAttack = nil
		}
	}
	flushFeature := func() {
		if curFeature != nil {
			curFeature.Description = strings.TrimSpace(curFeature.Description)
			t.Features = append(t.Features, *curFeature)
			curFeature = nil
		}
	}

	for i := 1; i < len(ls); i++ {
		text := ls[i].Text
		page := ls[i].Page

		if capsLineRe.MatchString(text) {
			switch {
			case strings.HasPrefix(text, "ROLES"):
				mode = modeRoles
				continue
			case text == "NATURAL/WEAPONS":
				flushRole()
				mode = modeAttacks
				continue
			case strings.HasPrefix(text, "SAVE DC"):
				flushAttack()
				mode = modeSaveDC
				continue
			case text == "SPECIAL FEATURES:":
				mode = modeFeatures
				continue
			case summonRankHeadingRe.MatchString(text):
				flushFeature()
				rank = summonRankHeadingRe.FindStringSubmatch(text)[1]
				continue
			case strings.HasSuffix(text, "JUTSU SPECIALTY"):
				flushFeature()
				mode = modeSpecialty
				continue
			case strings.HasSuffix(text, "JUTSU") && i+1 < len(ls) && ls[i+1].Text == "SPECIALTY":
				// Long tribe names push the heading to wrap: "HAWK/PREDATOR
				// BIRDS JUTSU" / "SPECIALTY".
				flushFeature()
				mode = modeSpecialty
				i++ // consume the wrapped "SPECIALTY" line too
				continue
			case mode == modeSpecialty:
				// The tribe name repeats to open the stat table, but not
				// reliably: sometimes with the book's own stray slash-
				// spacing ("HARE/ RABBIT"), sometimes abbreviated to just
				// one word ("SHARK" for "Shark/Predator Fish"). Rather than
				// match that text, treat ANY caps heading reached while
				// still collecting specialty text as this noise and start
				// waiting for the table's first "D-Rank" row instead.
				mode = modeProgHeader
				continue
			}
		}

		switch mode {
		case modeFlavor:
			if lbl, val, ok := splitSummonHeaderLabel(text); ok {
				applySummonHeaderLabel(&t, lbl, val, page, flag)
				mode = modeHeader
				continue
			}
			desc = append(desc, text)
		case modeHeader:
			if lbl, val, ok := splitSummonHeaderLabel(text); ok {
				applySummonHeaderLabel(&t, lbl, val, page, flag)
				continue
			}
			// Fell through to ROLES/etc without matching a caps heading
			// case above (shouldn't normally happen); ignore stray text.
		case modeRoles:
			if strings.HasPrefix(text, "•") {
				flushRole()
				rest := strings.TrimSpace(strings.TrimPrefix(text, "•"))
				nm, val, ok := splitOnColon(rest)
				if !ok {
					flag(page, "role bullet without a 'Name: description' colon: "+snippet(rest))
					continue
				}
				curRole = &SummonRole{Name: tidyName(nm), Description: val}
				continue
			}
			if curRole != nil {
				curRole.Description += " " + text
			}
		case modeAttacks:
			if m := summonAttackNameRe.FindStringSubmatch(text); m != nil {
				flushAttack()
				curAttack = &SummonAttack{Name: tidyName(m[1]), Description: m[2]}
				continue
			}
			if curAttack != nil {
				curAttack.Description += " " + text
			}
		case modeSaveDC:
			switch {
			case strings.HasPrefix(text, "All Jutsu Save DC"):
				t.JutsuSaveDCText = trimAfterColon(text)
			case strings.HasPrefix(text, "All Jutsu Attack bonus"):
				t.JutsuAttackText = trimAfterColon(text)
			default:
				// Wrapped continuation of whichever line was last set.
				if t.JutsuAttackText != "" {
					t.JutsuAttackText += " " + text
				} else if t.JutsuSaveDCText != "" {
					t.JutsuSaveDCText += " " + text
				}
			}
		case modeFeatures:
			m := summonFeatureColonRe.FindStringSubmatch(text)
			if m == nil && curFeature == nil {
				m = summonFeaturePeriodRe.FindStringSubmatch(text)
			}
			if m != nil {
				flushFeature()
				if rank == "" {
					flag(page, "feature printed before any rank sub-heading: "+m[1])
				}
				curFeature = &SummonFeature{Name: tidyName(m[1]), Rank: rank,
					Description: m[2], SourcePage: page}
				continue
			}
			if curFeature != nil {
				curFeature.Description += " " + text
			}
		case modeSpecialty:
			specialty = append(specialty, text)
		case modeProgHeader:
			// The "Rank Level Size STR DEX CON INT WIS CHA Jutsu Slots
			// Jutsu Speed" header line itself carries no data — skip it
			// and anything else until the first data row.
			if strings.HasPrefix(text, "D-Rank") {
				mode = modeProgTable
				i-- // reprocess this line as the first data row
			}
		case modeProgTable:
			if len(t.Progression) >= 5 {
				// The table's 5 ranks are complete. Sometimes a Jutsu
				// Specialty bullet gets displaced after the stat block by
				// the source PDF's page layout (e.g. Deer's third bullet
				// prints after its table, right before the next tribe
				// starts) — route any leftover text there instead of
				// flagging it as a broken row.
				mode = modeSpecialty
				specialty = append(specialty, text)
				continue
			}
			row, consumed, ok := parseSummonProgressionRow(ls, i)
			if !ok {
				flag(page, "unreadable progression row: "+snippet(text))
				continue
			}
			t.Progression = append(t.Progression, row)
			i += consumed - 1
		}
	}
	flushRole()
	flushAttack()
	flushFeature()

	t.Description = strings.TrimSpace(strings.Join(desc, " "))
	t.JutsuSpecialty = strings.TrimSpace(strings.Join(specialty, " "))

	if t.SummonType == "" {
		flag(t.SourcePage, "Summon Type not found")
	}
	if len(t.Roles) == 0 {
		flag(t.SourcePage, "no roles parsed")
	}
	if len(t.Features) == 0 {
		flag(t.SourcePage, "no special features parsed")
	}
	if len(t.Progression) != 5 {
		flag(t.SourcePage, "progression table has "+strconv.Itoa(len(t.Progression))+" rows, want 5")
	}
	return t, anomalies
}

// splitSummonHeaderLabel recognizes one of the tribe header block's fixed
// labels ("Summon Type:", "Creature Senses:"/"Senses:", ...).
func splitSummonHeaderLabel(text string) (label, value string, ok bool) {
	for _, l := range []string{"Summon Type:", "Toughness:", "Defensive Ability Score:",
		"Saving Throws:", "Creature Skills:", "Creature Senses:", "Senses:"} {
		if strings.HasPrefix(text, l) {
			return l, strings.TrimSpace(strings.TrimPrefix(text, l)), true
		}
	}
	return "", "", false
}

func applySummonHeaderLabel(t *SummonTribe, label, value string, page int, flag func(int, string)) {
	switch label {
	case "Summon Type:":
		t.SummonType = value
	case "Toughness:":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			flag(page, "Toughness not a plain number: "+value)
		}
		t.Toughness = n
	case "Defensive Ability Score:":
		t.DefensiveAbility = value
	case "Saving Throws:":
		t.SavingThrows = value
	case "Creature Skills:":
		t.Skills = value
	case "Creature Senses:", "Senses:":
		t.Senses = value
	}
}

// splitOnColon splits "Name: rest of the sentence" once at the first colon.
func splitOnColon(s string) (name, rest string, ok bool) {
	i := strings.Index(s, ":")
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
}

func trimAfterColon(s string) string {
	_, rest, ok := splitOnColon(s)
	if !ok {
		return s
	}
	return rest
}

// parseSummonProgressionRow reads one stat-table row starting at ls[i]. The
// ordinal suffix on the level ("4th") sometimes wraps onto its own line, as
// in the Bloodline Latents tables — consumed is how many raw lines were
// used (2 when the suffix wrapped, 1 otherwise).
func parseSummonProgressionRow(ls []Line, i int) (SummonProgressionRow, int, bool) {
	text := ls[i].Text
	consumed := 1
	if m := summonProgRowRe.FindStringSubmatch(text); m != nil {
		lvl, _ := strconv.Atoi(m[2])
		fields := strings.Fields(m[4])
		if len(fields) == 0 {
			return SummonProgressionRow{}, 0, false
		}
		return SummonProgressionRow{
			Rank: m[1], Level: lvl, SizeText: fields[0],
			StatsText: strings.TrimSpace(m[3] + " " + strings.Join(fields[1:], " ")),
		}, consumed, true
	}
	// Wrapped ordinal: "D-Rank 4" / "th" / "M 16 10 14 10 12 10 4 2 D-Rank 30ft"
	fields := strings.Fields(text)
	if len(fields) < 2 || !strings.HasSuffix(fields[0], "-Rank") {
		return SummonProgressionRow{}, 0, false
	}
	if i+2 >= len(ls) || !summonProgOrdinalRe.MatchString(ls[i+1].Text) {
		return SummonProgressionRow{}, 0, false
	}
	rank := strings.TrimSuffix(fields[0], "-Rank")
	lvl, err := strconv.Atoi(fields[1])
	if err != nil {
		return SummonProgressionRow{}, 0, false
	}
	rest := strings.Fields(ls[i+2].Text)
	if len(rest) == 0 {
		return SummonProgressionRow{}, 0, false
	}
	return SummonProgressionRow{
		Rank: rank, Level: lvl, SizeText: rest[0],
		StatsText: strings.Join(rest[1:], " "),
	}, 3, true
}
