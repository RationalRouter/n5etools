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
// this file's own doc for why these three specific rows need it. Extend one
// confirmed instance at a time, the same discipline classoptionentries.go's
// own doc follows for its bundled tiers.
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
}

// LoadEmbeddedSubclassOptionLists splits each configured bundled
// subclass_features row into real class_options rows (one per named
// sub-entry, via textentries.FindEntries — the identical splitter
// LoadClassOptionEntries already uses for class_options' own bundled
// tiers), then deletes the now-redundant source subclass_features row so
// the sheet's Features & Traits list doesn't also show the same content as
// one giant unformatted blob alongside the new picker.
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
