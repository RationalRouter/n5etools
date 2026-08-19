// Splitting bundled class_options tier fields into individually addressable
// upgrade rows.
//
// Puppet Master's "Puppet Upgrades" mechanic (both the class-wide "Puppet
// Master Upgrades" list and each subclass's own exclusive list, e.g. "Black
// Iron Upgrades") prints one row per TIER ("Wood Tier", "Bronze Tier", ...)
// in class_options, but each tier's own description bundles several
// individually-named upgrades into one prose field — the same PDF
// bold/line-break-loss shape internal/textentries already detects and
// cmd/n5e/format.go already renders to HTML for read-only display. This
// file reuses that exact same detection at ingest time to produce real,
// separately addressable rows instead — needed so a player can pick a
// SPECIFIC upgrade (see internal/charstore/companion_upgrades.go), not just
// see the whole tier's bundled text.
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sergio/n5e/internal/textentries"
)

// knownEntrySplitFixes hand-corrects a small, closed list of real
// class_option_entries splits where the true name got truncated because a
// word INSIDE it (the real name has its own internal colon, like a jutsu
// name "Sealing Art: X") was itself shaped exactly like
// textentries.FindEntries' own "Keyword:" anchor and got mistaken for one.
// "POISON MIST HELL: CONTINUOUS FIRING Techniques: Black, Perfect..." split
// with name="Poison Mist" (capsEntryPattern's greedy caps-run backed off to
// stop right before "HELL:", the first word satisfying the keyword-capture
// shape) and description starting "HELL: CONTINUOUS FIRING Techniques:
// ...". Purely syntactic — the regex has no way to know "HELL" isn't a real
// keyword like "Techniques"/"Cost"/"ASI" without a closed vocabulary, which
// would risk missing a genuinely new keyword in a future sourcebook update
// (see capsEntryPattern's own doc on why that trade was rejected) — a
// targeted, hand-verified correction is safer than widening the pattern.
// Keyed by (class_options slug, WRONG name) so a coincidentally-matching
// name elsewhere is never silently "fixed" into something wrong; extend one
// confirmed instance at a time.
var knownEntrySplitFixes = map[[2]string]string{
	{"class/puppet-master/option/black-iron-upgrades/silver-tier", "Poison Mist"}: "HELL: CONTINUOUS FIRING ",
}

// fixKnownEntrySplit applies knownEntrySplitFixes when this exact
// (classOptionSlug, name) pair is a known-broken split and the description
// still starts with the exact prefix that belongs back on the name —
// re-verifying the prefix rather than trusting the map blindly means a
// future re-ingest that fixes the root cause (or a rules update that
// reshuffles this entry) fails loud (the description simply won't start
// with the expected prefix any more) instead of silently mis-editing
// unrelated text.
func fixKnownEntrySplit(classOptionSlug, name, description string) (string, string) {
	prefix, ok := knownEntrySplitFixes[[2]string{classOptionSlug, name}]
	if !ok || !strings.HasPrefix(description, prefix) {
		return name, description
	}
	return name + " " + textentries.TitleCase(strings.TrimSpace(prefix)),
		strings.TrimSpace(strings.TrimPrefix(description, prefix))
}

// knownMissingPeriodFixes hand-corrects a small, closed list of real PDF-
// extraction drops: the sentence immediately before a bundled sub-entry's
// own bold header lost its terminal period entirely, so
// textentries.capsEntryPattern's own anchor — every documented widening of
// which still requires SOME punctuation or string-start immediately before
// the caps run — never fires there at all, and the whole entry silently
// vanishes into the previous one's body (e.g. "...concentrating on a
// B-Rank jutsu BOB AND WEAVE Techniques: ..." swallowed Bob and Weave's
// entire entry into Antagonistic Connection's). Fixed as a targeted text
// correction, applied before FindEntries ever sees the description, rather
// than loosening capsEntryPattern's own anchor requirement to fire with no
// punctuation at all: that pattern is also used at RENDER time by
// cmd/n5e/format.go for every other class's description text, and dropping
// the punctuation requirement there risks new false-positive splits across
// the whole corpus, not just these 4 known spots. Keyed by the exact
// original substring so a coincidentally-matching phrase elsewhere is never
// silently "fixed" into something wrong; extend one confirmed instance at
// a time, same discipline as knownEntrySplitFixes above.
var knownMissingPeriodFixes = strings.NewReplacer(
	"concentrating on a B-Rank jutsu BOB AND WEAVE", "concentrating on a B-Rank jutsu. BOB AND WEAVE",
	"if the creature acquires them while you are connected DEFENSIVE MANIPULATION", "if the creature acquires them while you are connected. DEFENSIVE MANIPULATION",
	"to maintain concentration of this Upgrade RIBBON DANCE", "to maintain concentration of this Upgrade. RIBBON DANCE",
	"Shinobi Cross TRACING ATTACK", "Shinobi Cross. TRACING ATTACK",
	// Science-Nin's Scientific Ninja Tools Minor tier repeats the identical
	// "You regain all expended uses when you complete a short or long rest"
	// closing sentence for 5 of its 6 elemental amplifiers, every time with
	// no terminal period before the next tool's own ALL-CAPS name — the same
	// systemic gap as the 4 fixes above, just recurring across one field
	// instead of one-off. Confirmed by sweeping every Scientific Ninja Tools
	// tier's own raw text for a caps-run+"Keyword:" header shape with no
	// sentence-ending punctuation immediately before it.
	"or long rest GEO AMPLIFIER", "or long rest. GEO AMPLIFIER",
	"or long rest CRYO AMPLIFIER", "or long rest. CRYO AMPLIFIER",
	"or long rest ELECTRO AMPLIFIER", "or long rest. ELECTRO AMPLIFIER",
	"or long rest EXPLOSIVE AMPLIFIER", "or long rest. EXPLOSIVE AMPLIFIER",
	"or long rest SCUBA EQUIPMENT", "or long rest. SCUBA EQUIPMENT",
	"any physical deformities ENHANCED NINJA TABI", "any physical deformities. ENHANCED NINJA TABI",
	"instead of 10 HOLOGRAPHIC CLOAK", "instead of 10. HOLOGRAPHIC CLOAK",
	"besides take the move action VOICE CHANGER AND AMPLIFIER", "besides take the move action. VOICE CHANGER AND AMPLIFIER",
	// Supreme tier's own Neuroadaptable Headgear entry loses its terminal
	// period the same way, swallowing Mass Chakra Absorption into it.
	"do You cast Geas as an Action MASS CHAKRA ABSORPTION", "do You cast Geas as an Action. MASS CHAKRA ABSORPTION",
	// Refined tier's "A.N. Ts" (a stylized rendering of "A.N.Ts", i.e.
	// "ants" — small sentry turrets) carries a stray extraction space that
	// makes "Ts" itself look like a one-word Title-Case entry name sitting
	// right before its own following sentence ("A.N. Ts. As an action...") —
	// namedEntryPattern's real, working shape, just tripped by punctuation
	// that was never meant to end a sentence there. Closing the space
	// removes the false sentence boundary without changing what the text
	// says.
	"A.N. Ts", "A.N.Ts",
)

// LoadClassOptionEntries splits every class_options row under classSlug
// whose description bundles 2+ named upgrades (see textentries.FindEntries)
// into class_option_entries rows. A description with fewer than 2 matches
// is left alone — the same threshold cmd/n5e/format.go's formatDescription
// already uses to avoid false-positive splits, so e.g. Blue Technique's
// "Puppet Weapon Types" list (already one class_options row per option:
// Drone/Ogre/Sentinel) produces zero entries per row, not a spurious split.
//
// Scoped to one class at a time (called with "class/puppet-master" and
// "class/science-nin" — every other class's option rows are already
// one-row-per-choice with no bundling problem observed, so generalizing
// this loader to every class remains a separate future change, not
// attempted here.
func LoadClassOptionEntries(db *sql.DB, book SourceBook, classSlug string) (*LoadReport, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	rows, err := tx.Query(`
		SELECT slug, name, description, source_page FROM class_options WHERE class_slug = ? ORDER BY slug`,
		classSlug)
	if err != nil {
		return nil, err
	}
	type optionRow struct {
		slug, name, description string
		sourcePage              sql.NullInt64
	}
	var options []optionRow
	for rows.Next() {
		var o optionRow
		if err := rows.Scan(&o.slug, &o.name, &o.description, &o.sourcePage); err != nil {
			rows.Close()
			return nil, err
		}
		o.description = knownMissingPeriodFixes.Replace(o.description)
		options = append(options, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	report := &LoadReport{}
	seen := map[string]string{} // slug → name, for duplicate/vanished detection
	remember := func(slug, name string) string {
		if _, dup := seen[slug]; !dup {
			seen[slug] = name
			return slug
		}
		base := slug
		for n := 2; ; n++ {
			slug = fmt.Sprintf("%s-%d", base, n)
			if _, dup := seen[slug]; !dup {
				break
			}
		}
		report.Duplicates = append(report.Duplicates,
			fmt.Sprintf("%s: reused name %q stored as %s", base, name, slug))
		seen[slug] = name
		return slug
	}

	for _, o := range options {
		entries := textentries.FindEntries(o.description)
		if len(entries) < 2 {
			continue
		}
		for i, e := range entries {
			name := o.description[e.NameStart:e.NameEnd]
			if e.Kind == textentries.EntryKindCaps {
				name = textentries.TitleCase(name)
			}
			bodyEnd := len(o.description)
			if i+1 < len(entries) {
				bodyEnd = entries[i+1].FullStart + 1
			}
			description := strings.TrimSpace(o.description[e.BodyStart:bodyEnd])
			name, description = fixKnownEntrySplit(o.slug, name, description)

			var sourcePage any
			if o.sourcePage.Valid {
				sourcePage = o.sourcePage.Int64
			}
			slug := remember(o.slug+"/entry/"+Slugify(name), name)
			outcome, err := upsertClassOptionEntry(tx, book, slug, o.slug, name, description, i, sourcePage)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", slug, err)
			}
			report.count(outcome, slug)
		}
	}

	if err := findVanished(tx, "class_option_entries",
		"class_option_slug LIKE '"+classSlug+"/%'", book.Slug, seen, report); err != nil {
		return nil, err
	}

	return report, tx.Commit()
}

func upsertClassOptionEntry(tx *sql.Tx, book SourceBook, slug, classOptionSlug, name, description string, order int, sourcePage any) (rowOutcome, error) {
	var old struct {
		name, description sql.NullString
		status            string
	}
	err := tx.QueryRow(`
		SELECT name, description, detection_status
		FROM class_option_entries WHERE slug = ?`, slug).Scan(&old.name, &old.description, &old.status)

	if err == sql.ErrNoRows {
		_, err := tx.Exec(`
			INSERT INTO class_option_entries (slug, class_option_slug, name, description,
			                                   sort_order, source_book, source_version,
			                                   source_page, detection_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'auto')`,
			slug, classOptionSlug, name, description, order, book.Slug, book.Version, sourcePage)
		return rowCreated, err
	}
	if err != nil {
		return 0, err
	}

	changed := old.name.String != name || old.description.String != description
	newStatus, outcome := decideStatus(old.status, "auto", changed)
	if outcome == rowUnchanged {
		return rowUnchanged, nil
	}
	_, err = tx.Exec(`
		UPDATE class_option_entries
		SET name = ?, description = ?, sort_order = ?, class_option_slug = ?,
		    source_book = ?, source_version = ?, source_page = ?, detection_status = ?
		WHERE slug = ?`,
		name, description, order, classOptionSlug, book.Slug, book.Version, sourcePage, newStatus, slug)
	return outcome, err
}
