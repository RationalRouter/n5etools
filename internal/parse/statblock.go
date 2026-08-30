package parse

import (
	"regexp"
	"sort"
	"strings"
)

// abilityScoreRowRe matches the six-ability-score line every 5e-style
// companion/summon stat card in this corpus prints, with or without a
// preceding "STR DEX CON INT WIS CHA" header (some sources squish it into
// "STRDEX..." or drop it altogether) — six "score (+/-modifier)" pairs back
// to back. This shape does not occur anywhere else in ordinary sourcebook
// prose, which makes it the load-bearing anchor for detecting a stat block
// glued into a feature's description by the flat PDF text extractor (see
// internal/extract's own doc comment — no column/font signal survives
// extraction, so only a text-pattern anchor like this one is available).
var abilityScoreRowRe = regexp.MustCompile(
	`(\d{1,2})\s*\([+-]\d{1,2}\)\s*` +
		`(\d{1,2})\s*\([+-]\d{1,2}\)\s*` +
		`(\d{1,2})\s*\([+-]\d{1,2}\)\s*` +
		`(\d{1,2})\s*\([+-]\d{1,2}\)\s*` +
		`(\d{1,2})\s*\([+-]\d{1,2}\)\s*` +
		`(\d{1,2})\s*\([+-]\d{1,2}\)`)

// sizeCreatureTypeRe matches a stat block's own opening "<Size> <Type>"
// clause (e.g. "Medium Construct", "Small Beast") — confirmed present
// immediately before the stat block proper in every known instance in this
// corpus (Puppet Master's Puppet Tool, Science-Nin's Titan base card, the
// Draconic Gauntlet's Whelp, the S.N.B Specialist's base creature),
// including Titan's own broken "X Construct" (the source PDF itself never
// fills in Titan's size word — a bug in the sourcebook, not the extractor).
// Used as the boundary between a feature's real prose and a glued-on stat
// block: the LAST occurrence of this pattern before the ability-score row
// is where the stat block begins. Requiring this in addition to the
// ability-score row (see SplitStatBlock) keeps false positives at zero
// against every known example — six-numbers-in-a-row alone could
// coincidentally appear elsewhere, and "Medium Construct" alone is just two
// ordinary words, but the combination has never misfired in this corpus.
var sizeCreatureTypeRe = regexp.MustCompile(
	`\b(?:Tiny|Small|Medium|Large|Huge|Gargantuan|X)\s+(?:Beast|Construct|Humanoid|Fiend|Elemental|Ooze|Aberration|Dragon|Fey|Monstrosity|Plant|Undead)\b`)

// leadingIntRe (jutsu.go) is reused below for ACFormulaText's leading digits.
var (
	speedInStatBlockRe  = regexp.MustCompile(`Speed\D{0,10}?(\d{1,3})\s*ft`)
	passivePerceptionRe = regexp.MustCompile(`(?i)passive perception\D{0,10}?\d{1,2}`)
)

// statBlockSectionKeywords are the section headings a stat block in this
// corpus may print, scanned for their actual position in the text rather
// than assumed to appear in a fixed order — confirmed examples disagree on
// whether Damage Resistance, Damage Immunity, and Condition Immunities are
// even all present, let alone in the same order.
var statBlockSectionKeywords = []string{
	"Armor Class", "Hit Points", "Speed", "Saving Throws",
	"Damage Resistance", "Damage Immunity", "Damage Immunities",
	"Condition Immunities", "Senses",
}

// StatBlockFields is a best-effort structured read of a split-out stat
// block. A zero/empty value means "not stated in the extracted text", not
// "value is zero" — some real stat blocks in this corpus (Titan, the Whelp)
// never print a recoverable AC at all, confirmed against dist/rules.db, not
// merely unparsed. This exists as reference data for a maintainer
// hand-wiring a new companion (see cmd/n5e/wow_whelp.go's own established
// pattern) — deliberately not itemized further than this (named traits and
// attacks stay one raw block, TraitsAndAttacksText) since the bold-run
// information needed to reliably split them out does not survive the flat
// PDF text extractor.
type StatBlockFields struct {
	CreatureType string
	// PreambleText is whatever text (if any) sits between the creature-type
	// clause and the first recognized section keyword — e.g. Puppet Tool's
	// "Proficiency = Puppet Master's Proficiency".
	PreambleText string

	ACFormulaText string
	AC            *int
	HPFormulaText string
	Speed         int
	Str, Dex, Con int
	Int, Wis, Cha int

	SavingThrowsText    string
	Resistances         string
	Immunities          string
	ConditionImmunities string
	Senses              string

	// TraitsAndAttacksText is everything after Senses's own "Passive
	// Perception N" clause, to the end of the raw stat block — narrated
	// traits followed by named attacks. Left empty if no Senses/Passive
	// Perception anchor was found (RawStatBlock still has the full text
	// verbatim regardless — this field is only a best-effort convenience).
	TraitsAndAttacksText string
}

// StatBlockMatch is the result of SplitStatBlock.
type StatBlockMatch struct {
	Found bool
	// Prose is the feature's real description text with the stat block
	// removed — callers should store this in place of the untouched text.
	Prose string
	// RawStatBlock is the stat block's own text, verbatim, from its opening
	// "<Size> <Type>" clause to the end of the input — always preserved in
	// full, regardless of how much of StatBlockFields could be populated.
	RawStatBlock string
	Fields       StatBlockFields
}

// SplitStatBlock detects a 5e-style companion/summon stat card glued into a
// feature's text by the flat PDF extractor (internal/extract has no
// column/font-size signal to detect this at the layout level) and splits it
// out. Requires BOTH the six-ability-score row and a preceding "<Size>
// <Type>" clause before treating anything as a stat block — see
// sizeCreatureTypeRe's own doc for why the combination, not either alone.
// Generalizes the two previous one-off fixes for this bug (Puppet Master's
// Puppet Tool, Science-Nin's Titan base card), plus catches instances no one
// ever wrote a special case for (the S.N.B Specialist's base creature,
// documented in CLASS_AUDIT.md but never fixed).
func SplitStatBlock(text string) StatBlockMatch {
	abilityLoc := abilityScoreRowRe.FindStringSubmatchIndex(text)
	if abilityLoc == nil {
		return StatBlockMatch{Prose: text}
	}
	typeMatches := sizeCreatureTypeRe.FindAllStringIndex(text[:abilityLoc[0]], -1)
	if len(typeMatches) == 0 {
		return StatBlockMatch{Prose: text}
	}
	lastType := typeMatches[len(typeMatches)-1]
	start := lastType[0]

	prose := strings.TrimSpace(text[:start])
	raw := strings.TrimSpace(text[start:])
	creatureTypeEndInRaw := lastType[1] - start

	fields := StatBlockFields{
		CreatureType: strings.TrimSpace(text[lastType[0]:lastType[1]]),
	}

	m := abilityScoreRowRe.FindStringSubmatch(text)
	fields.Str, fields.Dex, fields.Con = atoiSafe(m[1]), atoiSafe(m[2]), atoiSafe(m[3])
	fields.Int, fields.Wis, fields.Cha = atoiSafe(m[4]), atoiSafe(m[5]), atoiSafe(m[6])

	if sm := speedInStatBlockRe.FindStringSubmatch(raw); sm != nil {
		fields.Speed = atoiSafe(sm[1])
	}

	type hit struct {
		pos int
		kw  string
	}
	var hits []hit
	seen := map[string]bool{}
	for _, kw := range statBlockSectionKeywords {
		if seen[kw] {
			continue
		}
		if idx := strings.Index(raw, kw); idx >= 0 {
			hits = append(hits, hit{idx, kw})
			seen[kw] = true
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })

	if len(hits) > 0 {
		fields.PreambleText = strings.Trim(raw[creatureTypeEndInRaw:hits[0].pos], " ,")
	}

	for i, h := range hits {
		end := len(raw)
		if i+1 < len(hits) {
			end = hits[i+1].pos
		}
		section := strings.TrimSpace(raw[h.pos+len(h.kw) : end])
		switch h.kw {
		case "Armor Class":
			fields.ACFormulaText = section
			if lm := leadingIntRe.FindStringSubmatch(section); lm != nil {
				n := atoiSafe(lm[1])
				fields.AC = &n
			}
		case "Hit Points":
			fields.HPFormulaText = section
		case "Saving Throws":
			fields.SavingThrowsText = section
		case "Damage Resistance":
			fields.Resistances = section
		case "Damage Immunity", "Damage Immunities":
			fields.Immunities = section
		case "Condition Immunities":
			fields.ConditionImmunities = section
		case "Senses":
			// Senses text often runs straight into unlabeled trait prose
			// with no further separator ("Senses Passive Perception 10
			// Elemental Existence. ...") — keep only Senses's own "Passive
			// Perception N" clause and treat everything after it as
			// TraitsAndAttacksText, confirmed the right split point against
			// every known example in this corpus.
			if pm := passivePerceptionRe.FindStringIndex(section); pm != nil {
				fields.Senses = strings.TrimSpace(section[:pm[1]])
				fields.TraitsAndAttacksText = strings.TrimSpace(section[pm[1]:])
			} else {
				fields.Senses = section
			}
		}
	}

	return StatBlockMatch{
		Found:        true,
		Prose:        prose,
		RawStatBlock: raw,
		Fields:       fields,
	}
}
