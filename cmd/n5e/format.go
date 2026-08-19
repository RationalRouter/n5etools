package main

import (
	"html"
	"html/template"
	"regexp"
	"strconv"
	"strings"

	"github.com/sergio/n5e/internal/textentries"
)

// diceNotationPattern matches TTRPG dice notation like "3d6" or a bare
// "d8" (implicit count of 1) — word-bounded so it only fires on a real
// standalone NdM token, not inside an unrelated identifier or number.
var diceNotationPattern = regexp.MustCompile(`\b(\d*)d(\d+)\b`)

// diceAvg appends a roll's average in parentheses right after every dice
// notation token in s — "3d6" becomes "3d6 (10.5)", a bare "d8" becomes
// "d8 (4.5)" — surfaces the expected value at a glance anywhere the book
// (or this app's own derived text, e.g. Hit Die) states
// a die roll. Exposed as a template func (see templates.go) for the
// handful of fields that show dice notation outside of prose — Hit
// Die/Chakra Die, weapon damage dice, class-level dice-valued resources
// (e.g. Flurry Die, Chakra Scalpel damage) — and used internally by
// formatDescription so every prose field gets the same treatment for
// free. A no-op on text with no dice notation, so it's safe to apply
// unconditionally rather than needing a per-field "is this actually a
// dice field" check.
func diceAvg(s string) string {
	return diceNotationPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := diceNotationPattern.FindStringSubmatch(m)
		count := 1
		if sub[1] != "" {
			n, err := strconv.Atoi(sub[1])
			if err != nil {
				return m
			}
			count = n
		}
		sides, err := strconv.Atoi(sub[2])
		if err != nil || sides == 0 {
			return m
		}
		return m + " (" + diceAverageText(count, sides) + ")"
	})
}

// diceAverageText computes an NdM roll's average as exact halves-of-an-
// integer arithmetic (doubled := count*(sides+1), which is always exactly
// divisible by 2 either way) rather than floating point, so results like
// "10.5" or "7" are exact with no formatting/rounding surprises.
func diceAverageText(count, sides int) string {
	doubled := count * (sides + 1)
	whole := strconv.Itoa(doubled / 2)
	if doubled%2 == 0 {
		return whole
	}
	return whole + ".5"
}

// subBulletMarker finds the book's nested sub-bullet points, which the PDF
// text layer renders as a bare "o" — always immediately after a sentence
// boundary (". " or "; ") and immediately before a capitalized word, e.g.
// "...subsequent turns; o One damage immunity... or trait. o One damage
// resistance...". That anchoring is what keeps this from false-matching the
// letter "o" anywhere else in the text — "o" is never an English word on
// its own, so the punctuation-before/capital-after shape is what a real
// sub-bullet marker looks like and nothing else does.
//
// Go's RE2 engine has no lookahead, so the capital letter that starts the
// next word is captured by the match itself (not just asserted) — callers
// that use the match end as a slice boundary need to back up one byte to
// put that captured letter back into the sub-item text.
var subBulletMarker = regexp.MustCompile(`[.;] o [A-Z]`)

// entryMatch, entryKind, findEntries and titleCase are thin aliases over
// internal/textentries — the actual pattern-matching logic moved there so
// internal/store's ingest-time class_option_entries splitter (bundled
// Puppet Master upgrade tiers → individual rows) can reuse the exact same
// detection instead of forking it. Kept under these names here so every
// existing call site and test in this package needs no changes.
type entryMatch = textentries.EntryMatch
type entryKind = textentries.EntryKind

const (
	entryKindTitle = textentries.EntryKindTitle
	entryKindCaps  = textentries.EntryKindCaps
)

func findEntries(raw string) []entryMatch { return textentries.FindEntries(raw) }
func titleCase(s string) string           { return textentries.TitleCase(s) }

// formatDescription turns book text containing inline "•" bullet markers,
// or either unmarked named-entry list shape (see namedEntryPattern and
// capsEntryPattern), into real HTML structure. The sourcebooks print actual
// formatted lists, but PDF text extraction has no concept of line breaks or
// bold runs within a paragraph, so what lands in the database is one
// run-on sentence. Text matching none of these shapes is returned as a
// single paragraph, unchanged from how it always rendered.
//
// Named-entry detection only wins over plain bullet-list handling when the
// first named-entry boundary comes before the first "•" — some fields (e.g.
// a feat granting "• benefit one • benefit two • pick one of the following
// schools; o School A. ... Some Ability Name. flavor text ...") have a
// genuine top-level bullet list whose *nested* sub-items happen to contain
// something that looks like a named-entry boundary several levels deep;
// treating that as a top-level entry would be wrong. A field like Puppet
// Master's chassis list, by contrast, starts with its first named entry and
// only grows "•" bullets *inside* each entry's own body — see
// TestFormatDescriptionNamedEntriesWithNestedBullets and
// TestFormatDescriptionDoesNotPromoteDeeplyNestedNameToTopLevel.
func formatDescription(raw string) template.HTML {
	if raw == "" {
		return ""
	}
	entries := findEntries(raw)
	firstBullet := strings.Index(raw, "•")
	if len(entries) >= 2 && (firstBullet == -1 || entries[0].FullStart < firstBullet) {
		return formatNamedEntries(raw, entries)
	}
	if firstBullet != -1 {
		return formatBulletedDescription(raw)
	}
	return template.HTML("<p>" + html.EscapeString(diceAvg(raw)) + "</p>")
}

// formatNamedEntries renders the intro sentence (if any) as its own
// paragraph, then one entry per match from findEntries: "<strong>Name.</strong>"
// followed by its body, which itself gets real bullet-list treatment (see
// writeEntryBody) if the entry's own text contains "•" — e.g. Puppet
// Master's chassis options, which nest a bulleted benefits list inside each
// ALL-CAPS chassis entry.
func formatNamedEntries(raw string, entries []entryMatch) template.HTML {
	var b strings.Builder

	// Sliced at nameStart, not fullStart+1: fullStart only equals the
	// boundary punctuation's own position for entries that matched via the
	// "[.!?;] " alternative. The first entry can also match via the bare
	// "^" (start of string) alternative, where fullStart==nameStart==0 and
	// there is no punctuation to include — raw[:0+1] would wrongly grab the
	// entry name's own first character as a spurious one-letter intro
	// paragraph (caught by TestFormatDescriptionNamedEntriesNoIntroAtCapsStart).
	intro := strings.TrimSpace(raw[:entries[0].NameStart])
	if intro != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(diceAvg(intro)))
		b.WriteString("</p>")
	}

	b.WriteString(`<div class="named-entries">`)
	for i, e := range entries {
		name := raw[e.NameStart:e.NameEnd]
		bodyEnd := len(raw)
		if i+1 < len(entries) {
			bodyEnd = entries[i+1].FullStart + 1 // include the sentence-ending punctuation before the next name
		}
		body := strings.TrimSpace(raw[e.BodyStart:bodyEnd])
		writeEntryBody(&b, name, body, e.Kind)
	}
	b.WriteString("</div>")
	return template.HTML(b.String())
}

// statLineFieldPattern matches one "Cost: 32 Creation Points"-shaped field
// at the very start of a caps-entry's body — see splitLeadingStatLine. The
// unit words are an explicit enumeration (confirmed exhaustive by sweeping
// every "Cost:"/"Drain:" occurrence in the corpus), not "any capitalized
// word": the first word of the prose sentence that follows is capitalized
// too (it starts a sentence), and an early draft of this pattern that
// accepted any capitalized word greedily swallowed it into the value —
// "Cost: 16 Creation Points Drain: 15 CCD Chakra" even ate the SECOND
// field's own "Drain" keyword as if it were one of the first field's unit
// words. Enumerating the real unit words rules both mistakes out at once.
//
// The range separator accepts a hyphen, en dash, or em dash ([-–—], same
// class internal/parse's own heading/label patterns already use) — a few
// Grenadier B.I.M entries' own "Cost: X-Y Creation Points" range came
// through PDF extraction with an en dash instead of a hyphen ("Cost:
// 8–12 Creation Points"), which a hyphen-only pattern stops matching mid-
// field: the badge silently truncated to "Cost: 8" and the stray
// "–12 Creation Points" spilled into the entry's own prose body instead.
var statLineFieldPattern = regexp.MustCompile(`^(?:Cost|Drain): \d+(?:[-–—]\d+)?(?: (?:Creation|Points?|CCD|Chakra))*`)

// splitLeadingStatLine peels a leading run of "Cost: ..."/"Drain: ..."
// fields off the front of a caps-entry's body — e.g. Science-Nin's
// Mastercraft upgrades: "Cost: 32 Creation Points Drain: 30 CCD Chakra You
// gain a barrier..." The PDF text layer has no line break between the
// book's own printed stat-line and the prose that follows it, so without
// this the two run together as one sentence ("...CCD Chakra You gain...").
//
// Scoped to Cost/Drain only — the two overwhelmingly common fields in this
// position (474 of ~550 "known stat-line keyword" occurrences swept across
// the corpus) and the only two confirmed to share this exact
// number-then-short-capitalized-word-run value shape. Other stat-line
// keywords seen in the data (Prerequisite, Keyword, Bulk, ...) have looser,
// less consistent shapes and are deliberately left alone rather than
// guessed at — same "don't force an ill-fitting pattern" restraint
// namedEntryPattern's own doc already argues for.
//
// Returns ("", body) when body doesn't start with a recognized field — the
// overwhelming majority of entries, including every non-gadget one.
func splitLeadingStatLine(body string) (statLine, rest string) {
	rest = body
	var fields []string
	for {
		m := statLineFieldPattern.FindString(rest)
		if m == "" {
			break
		}
		fields = append(fields, m)
		rest = strings.TrimSpace(rest[len(m):])
	}
	if len(fields) == 0 {
		return "", body
	}
	return strings.Join(fields, " · "), rest
}

// writeEntryBody writes one named entry as "<p><strong>Name.</strong>
// lead text</p>", followed by a real "<ul>" — reusing the same
// per-item/sub-bullet logic bullet-only fields use (see writeBulletList) —
// if the entry's own body contains "•" (e.g. Puppet Master's chassis
// options, which nest a bulleted benefits list inside each entry).
//
// A caps-kind name (see entryKind) is displayed title-cased rather than
// shouting, and rendered in a separate CSS class that italicizes it —
// deliberately different from a title-kind name's plain bold, so the two
// unmarked list shapes this file detects don't look identical once
// rendered, matching how the book itself distinguishes this style. A
// caps-kind entry also gets a shot at splitLeadingStatLine — a title-kind
// entry never does, since that shape is unique to the ALL-CAPS
// gadget/upgrade lists the caps pattern alone matches.
func writeEntryBody(b *strings.Builder, name, body string, kind entryKind) {
	strongClass := ""
	statLine := ""
	if kind == entryKindCaps {
		name = titleCase(name)
		strongClass = ` class="caps-entry-name"`
		statLine, body = splitLeadingStatLine(body)
	}

	parts := strings.Split(body, "•")
	lead := strings.TrimSpace(parts[0])

	b.WriteString("<p><strong")
	b.WriteString(strongClass)
	b.WriteString(">")
	b.WriteString(html.EscapeString(name))
	b.WriteString(".</strong>")

	if statLine != "" {
		b.WriteString(` <span class="entry-stat-line">`)
		b.WriteString(html.EscapeString(statLine))
		b.WriteString("</span></p>")
		if lead != "" {
			b.WriteString("<p>")
			b.WriteString(html.EscapeString(diceAvg(lead)))
			b.WriteString("</p>")
		}
	} else {
		if lead != "" {
			b.WriteString(" ")
			b.WriteString(html.EscapeString(diceAvg(lead)))
		}
		b.WriteString("</p>")
	}

	if len(parts) > 1 {
		writeBulletList(b, parts[1:])
	}
}

// formatBulletedDescription is the pre-existing "•" bullet handling,
// unchanged, just split out of formatDescription so it reads as one branch
// among several list shapes rather than the only one.
func formatBulletedDescription(raw string) template.HTML {
	parts := strings.Split(raw, "•")
	intro := strings.TrimSpace(parts[0])

	var b strings.Builder
	if intro != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(diceAvg(intro)))
		b.WriteString("</p>")
	}
	writeBulletList(&b, parts[1:])
	return template.HTML(b.String())
}

// writeBulletList writes parts (text already split on "•", with any
// leading intro text already stripped by the caller) as a real "<ul>",
// shared by the top-level "a whole field is one bullet list" case
// (formatBulletedDescription) and named entries whose own body contains
// bullets (writeEntryBody).
func writeBulletList(b *strings.Builder, parts []string) {
	b.WriteString(`<ul class="prose-list">`)
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		b.WriteString("<li>")
		writeBulletItem(b, item)
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
}

// writeBulletItem writes one top-level bullet's contents, splitting out any
// nested "o" sub-bullets (see subBulletMarker) into their own indented list.
func writeBulletItem(b *strings.Builder, item string) {
	locs := subBulletMarker.FindAllStringIndex(item, -1)
	if locs == nil {
		b.WriteString(html.EscapeString(diceAvg(item)))
		return
	}

	lead := strings.TrimSpace(item[:locs[0][0]+1]) // up to and including the "." or ";"
	b.WriteString(html.EscapeString(diceAvg(lead)))

	b.WriteString(`<ul class="prose-list prose-sublist">`)
	for i, loc := range locs {
		start := loc[1] - 1 // back up into the captured capital letter (see subBulletMarker doc)
		end := len(item)
		if i+1 < len(locs) {
			end = locs[i+1][0] + 1 // include the sentence-ending punctuation before the next marker
		}
		sub := strings.TrimSpace(item[start:end])
		if sub == "" {
			continue
		}
		b.WriteString("<li>")
		b.WriteString(html.EscapeString(diceAvg(sub)))
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
}
