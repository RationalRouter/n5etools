// Recovering option lists that the class-book outline mis-bookmarked as a
// single subclass feature instead of a genuine pickable list.
//
// Puppet Master's Blue Technique bookmarks "Puppet Weapon Types" as its own
// TOP-LEVEL section (a sibling of "Blue Technique ~ Warmaster" itself), so
// ParseSubclasses's normal outline-driven logic already produces one
// class_options row per weapon type (Drone/Ogre/Sentinel). Black, Green, and
// Red Technique's own equivalent lists (Puppeteer Chassis, Puppet
// Frameworks, Puppet Roles) are NOT bookmarked that way — the book nests
// them as a plain child of their own subclass node, with the same title
// repeated (Black/Green) or standing alone (Red) and no bookmarked
// grandchildren at all for the individual options (Specialized/Quadrupedal/
// Warforged/Winged, etc.). ParseSubclasses' isSubclass test classifies the
// WHOLE subclass node by its children's overall shape, so these lists get
// swallowed as one bloated ClassFeature (confirmed: shipped as
// .../puppeteer-chassis-2, .../puppet-frameworks-2, and a single
// .../puppet-roles row, 3-5KB of unsplit prose each) rather than becoming
// their own OptionList — same root shape as class_options' own bundled-tier
// text (see classoptionentries.go), just landing in subclass_features
// instead. The prose itself extracted correctly; only the STRUCTURE is
// wrong, so this reuses textentries.FindEntries (the exact same splitter
// class_options tiers already use) against the already-correct text rather
// than re-parsing anything from the PDF.
package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sergio/n5e/internal/parse"
	"github.com/sergio/n5e/internal/textentries"
)

// embeddedSubclassOptionList describes one subclass_features row that is
// really a bundled option list: where to read the bundled prose from, and
// where the split-out options should land as real class_options rows.
type embeddedSubclassOptionList struct {
	SourceSlug   string // subclass_features.slug holding the bundled prose
	SubclassSlug string // subclasses.slug the new class_options rows belong to
	ListName     string // class_options.list_name / OptionList heading
	SlugPrefix   string // 'class/puppet-master/option/<prefix>/<option-slug>'
}

// embeddedSubclassOptionLists is a small, explicit, hand-verified list — see
// this file's own doc for why these rows need it. Extend one confirmed
// instance at a time, the same discipline classoptionentries.go's own doc
// follows for its bundled tiers.
//
// All 8 Weapon Specialist Weapon Forms have the identical shape as Puppet
// Master's own three: a 3rd-level "[Form] Styles" subclass feature bundles
// 4-6 individually-named Styles into one prose field, with a level-scaled
// "Styles Known" cap (class_level_resources) gating how many a player can
// pick — same "book bookmarked a real option list as one plain feature"
// root cause as Puppeteer Chassis/Puppet Frameworks/Puppet Roles.
var embeddedSubclassOptionLists = []embeddedSubclassOptionList{
	{
		SourceSlug:   "class/puppet-master/group/puppet-techniques/black-technique-puppeteer/feature/puppeteer-chassis-2",
		SubclassSlug: "class/puppet-master/group/puppet-techniques/black-technique-puppeteer",
		ListName:     "Puppeteer Chassis",
		SlugPrefix:   "puppeteer-chassis",
	},
	{
		SourceSlug:   "class/puppet-master/group/puppet-techniques/green-technique-marionettist/feature/puppet-frameworks-2",
		SubclassSlug: "class/puppet-master/group/puppet-techniques/green-technique-marionettist",
		ListName:     "Puppet Frameworks",
		SlugPrefix:   "puppet-frameworks",
	},
	{
		SourceSlug:   "class/puppet-master/group/puppet-techniques/red-technique-performer/feature/puppet-roles",
		SubclassSlug: "class/puppet-master/group/puppet-techniques/red-technique-performer",
		ListName:     "Puppet Roles",
		SlugPrefix:   "puppet-roles",
	},
	{
		SourceSlug:   "class/weapon-specialist/group/weapon-forms/battle-dancer-form/feature/battle-dancer-styles",
		SubclassSlug: "class/weapon-specialist/group/weapon-forms/battle-dancer-form",
		ListName:     "Battle Dancer Styles",
		SlugPrefix:   "battle-dancer-styles",
	},
	{
		SourceSlug:   "class/weapon-specialist/group/weapon-forms/gungnir-piercer-form/feature/gungnir-piercer-styles",
		SubclassSlug: "class/weapon-specialist/group/weapon-forms/gungnir-piercer-form",
		ListName:     "Gungnir Piercer Styles",
		SlugPrefix:   "gungnir-piercer-styles",
	},
	{
		SourceSlug:   "class/weapon-specialist/group/weapon-forms/obsidian-hammer-form/feature/obsidian-hammer-styles",
		SubclassSlug: "class/weapon-specialist/group/weapon-forms/obsidian-hammer-form",
		ListName:     "Obsidian Hammer Styles",
		SlugPrefix:   "obsidian-hammer-styles",
	},
	{
		SourceSlug:   "class/weapon-specialist/group/weapon-forms/phantom-blade-form/feature/phantom-blade-styles",
		SubclassSlug: "class/weapon-specialist/group/weapon-forms/phantom-blade-form",
		ListName:     "Phantom Blade Styles",
		SlugPrefix:   "phantom-blade-styles",
	},
	{
		SourceSlug:   "class/weapon-specialist/group/weapon-forms/primal-weapon-form/feature/primal-weapon-styles",
		SubclassSlug: "class/weapon-specialist/group/weapon-forms/primal-weapon-form",
		ListName:     "Primal Weapon Styles",
		SlugPrefix:   "primal-weapon-styles",
	},
	{
		SourceSlug:   "class/weapon-specialist/group/weapon-forms/ranger-form/feature/ranger-styles",
		SubclassSlug: "class/weapon-specialist/group/weapon-forms/ranger-form",
		ListName:     "Ranger Styles",
		SlugPrefix:   "ranger-styles",
	},
	{
		SourceSlug:   "class/weapon-specialist/group/weapon-forms/samurai-form/feature/samurai-styles",
		SubclassSlug: "class/weapon-specialist/group/weapon-forms/samurai-form",
		ListName:     "Samurai Styles",
		SlugPrefix:   "samurai-styles",
	},
	{
		SourceSlug:   "class/weapon-specialist/group/weapon-forms/slayer-form/feature/slayer-styles",
		SubclassSlug: "class/weapon-specialist/group/weapon-forms/slayer-form",
		ListName:     "Slayer Styles",
		SlugPrefix:   "slayer-styles",
	},
}

// embeddedListPunctuationFixes hand-corrects a small, closed list of real
// PDF-extraction punctuation drops in the Weapon Form Styles lists — the
// same "targeted text fix before FindEntries ever sees it" discipline
// classoptionentries.go's own knownMissingPeriodFixes follows, not a
// pattern-widening in textentries itself:
//   - Obsidian Hammer Styles' "Unleashed! . As" and Primal Weapon Styles'
//     "Primal Edge!. As" both print the entry name's own flavor "!" where
//     namedEntryPattern requires a literal ". " immediately after the name
//     (letters/apostrophes/hyphens only) — the exclamation point breaks the
//     anchor and the whole entry silently vanishes into the previous one's
//     body. Normalized to a plain period; the body text is unaffected, only
//     the auto-detected header loses the exclamation mark.
//   - Slayer Styles is missing the period that should separate "...if you
//     pass or fail" from "Studied Tracking" entirely (no sentence break at
//     all between the two), so the anchor never fires there either and
//     "Studied Tracking" swallows into "Blood Reader"'s body.
var embeddedListPunctuationFixes = strings.NewReplacer(
	"Unleashed! . As", "Unleashed. As",
	"Primal Edge!. As", "Primal Edge. As",
	"if you pass or fail Studied Tracking.", "if you pass or fail. Studied Tracking.",
)

// classSlugFromSubclassSlug extracts the "class/<slug>" prefix from a
// subclass slug ("class/weapon-specialist/group/weapon-forms/samurai-form"
// -> "class/weapon-specialist"), so LoadEmbeddedSubclassOptionLists can tell
// which of the shared embeddedSubclassOptionLists specs belong to the class
// it was called for, without a redundant explicit field on every spec.
func classSlugFromSubclassSlug(subclassSlug string) string {
	parts := strings.SplitN(subclassSlug, "/", 3)
	if len(parts) < 2 {
		return subclassSlug
	}
	return parts[0] + "/" + parts[1]
}

// LoadEmbeddedSubclassOptionLists splits each configured bundled
// subclass_features row belonging to classSlug into real class_options rows
// (one per named sub-entry, via textentries.FindEntries — the identical
// splitter LoadClassOptionEntries already uses for class_options' own
// bundled tiers), then deletes the now-redundant source subclass_features
// row so the sheet's Features & Traits list doesn't also show the same
// content as one giant unformatted blob alongside the new picker.
func LoadEmbeddedSubclassOptionLists(db *sql.DB, book SourceBook, classSlug string) (*LoadReport, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := upsertSourceBook(tx, book); err != nil {
		return nil, err
	}

	report := &LoadReport{}
	for _, spec := range embeddedSubclassOptionLists {
		if classSlugFromSubclassSlug(spec.SubclassSlug) != classSlug {
			continue
		}
		var desc string
		var sourcePage sql.NullInt64
		err := tx.QueryRow(`SELECT description, source_page FROM subclass_features WHERE slug = ?`,
			spec.SourceSlug).Scan(&desc, &sourcePage)
		if err == sql.ErrNoRows {
			continue // already split and cleaned up on a prior run
		}
		if err != nil {
			return nil, err
		}
		desc = embeddedListPunctuationFixes.Replace(desc)

		entries := textentries.FindEntries(desc)
		if len(entries) < 2 {
			return nil, fmt.Errorf("%s: expected a bundled option list, found %d entries", spec.SourceSlug, len(entries))
		}

		seen := map[string]string{}
		for i, e := range entries {
			name := desc[e.NameStart:e.NameEnd]
			if e.Kind == textentries.EntryKindCaps {
				name = textentries.TitleCase(name)
			}
			bodyEnd := len(desc)
			if i+1 < len(entries) {
				bodyEnd = entries[i+1].FullStart + 1
			}
			description := strings.TrimSpace(desc[e.BodyStart:bodyEnd])

			slug := spec.SlugPrefix + "/" + Slugify(name)
			if _, dup := seen[slug]; dup {
				return nil, fmt.Errorf("%s: duplicate option name %q", spec.SourceSlug, name)
			}
			seen[slug] = name

			fullSlug := "class/" + classSlugSuffix(classSlug) + "/option/" + slug
			outcome, err := upsertClassOption(tx, book, fullSlug, classSlug, spec.SubclassSlug,
				spec.ListName, parse.ClassOption{Name: name, Description: description, SourcePage: int(sourcePage.Int64)}, i)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fullSlug, err)
			}
			report.count(outcome, fullSlug)
		}

		if _, err := tx.Exec(`DELETE FROM subclass_features WHERE slug = ?`, spec.SourceSlug); err != nil {
			return nil, err
		}
	}

	return report, tx.Commit()
}

// classSlugSuffix strips the "class/" prefix a class slug always carries,
// so callers can rebuild "class/<suffix>/option/..." without hardcoding it.
func classSlugSuffix(classSlug string) string {
	return strings.TrimPrefix(classSlug, "class/")
}
