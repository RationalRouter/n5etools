package main

import (
	"database/sql"
	"log"
	"net/http"
	"sort"

	"github.com/sergio/n5e/internal/charsheet"
)

// referenceFeatureEntry is one row in the Class Reference popup's single,
// combined, level-sorted feature list — merging every class's general
// features with every chosen subclass's own features into ONE reading
// order, the same shape the Core tab's own Features & Traits list
// (mergeFeatFeatures) already uses for the character's currently-granted
// features. SourceLabel disambiguates which class/subclass a given entry
// came from (needed once entries interleave — a bare level number alone
// no longer tells them apart on a multiclass character), and IsSubclass
// drives the gold/blue split (see app.css's .subclass-scope) since that
// distinction can no longer be made by which section an entry sits in.
type referenceFeatureEntry struct {
	Name        string
	SourceLabel string
	IsSubclass  bool
	Level       int
	Locked      bool
	Description string
	// ScalesThrough is the highest ordinal level mentioned within
	// Description itself (see scalinglevel.go), if higher than Level — many
	// class/subclass features are gained at one level but keep escalating
	// in their own text at later ones. 0 if it doesn't scale past its own
	// gate level.
	ScalesThrough int
}

// subclassBaseOptions is one chosen subclass's own "pick a base template"
// option list, if it has one — kept separate from the merged Features list
// below since these aren't leveled features at all (an untiered
// class_options row scoped to a subclass with no class_option_entries of
// its own; used to be read only for Puppet Master's own Puppeteer Chassis/
// Puppet Weapon Types/Puppet Frameworks/Puppet Roles, but the query itself
// was never actually Puppet-Master-specific).
type subclassBaseOptions struct {
	ClassName    string
	SubclassName string
	ListName     string
	Options      []companionFeatureRef
}

// characterReference is the "Class Reference" popup's own data: every
// class the character has levels in, plus every subclass chosen for any of
// them, merged into one combined, level-sorted feature list — including
// locked, not-yet-reached features — a rules reference, unlike the Core
// tab's Features & Traits list, which only ever shows what the character
// actually has right now.
type characterReference struct {
	CharacterName string
	Features      []referenceFeatureEntry
	BaseOptions   []subclassBaseOptions
}

// loadCharacterReference builds the Class Reference popup's data. onlyClassSlug,
// if non-empty, restricts the result to that one class (and whichever
// subclass the character chose for it) instead of merging every class the
// character has — see handleCharacterReference for when that filter is used
// (a multiclass character picking one class from characterClassOptions).
func (s *server) loadCharacterReference(characterID int64, onlyClassSlug string) (characterReference, error) {
	var ref characterReference

	classRows, err := s.charDB.Query(
		`SELECT class_slug, levels FROM character_classes WHERE character_id = ? ORDER BY order_index`, characterID)
	if err != nil {
		return ref, err
	}
	type classLevelPair struct {
		Slug   string
		Levels int
	}
	var classes []classLevelPair
	for classRows.Next() {
		var c classLevelPair
		if err := classRows.Scan(&c.Slug, &c.Levels); err != nil {
			classRows.Close()
			return ref, err
		}
		classes = append(classes, c)
	}
	classRows.Close()
	if err := classRows.Err(); err != nil {
		return ref, err
	}
	if onlyClassSlug != "" {
		filtered := classes[:0]
		for _, c := range classes {
			if c.Slug == onlyClassSlug {
				filtered = append(filtered, c)
			}
		}
		classes = filtered
	}

	classLevels := map[string]int{}
	classNames := map[string]string{}
	for _, c := range classes {
		classLevels[c.Slug] = c.Levels
		var name string
		if err := s.rulesDB.QueryRow(`SELECT name FROM classes WHERE slug = ?`, c.Slug).Scan(&name); err != nil {
			name = c.Slug
		}
		classNames[c.Slug] = name

		rows, err := s.rulesDB.Query(`
			SELECT name, level, description FROM v_class_features
			WHERE class_slug = ? ORDER BY sort_order, level`, c.Slug)
		if err != nil {
			return ref, err
		}
		for rows.Next() {
			var fname, description string
			var level sql.NullInt64
			if err := rows.Scan(&fname, &level, &description); err != nil {
				rows.Close()
				return ref, err
			}
			entry := referenceFeatureEntry{
				Name: fname, SourceLabel: name, Level: int(level.Int64), Description: description,
				Locked: level.Valid && int(level.Int64) > c.Levels,
			}
			if scales := highestScalingLevel(description); scales > entry.Level {
				entry.ScalesThrough = scales
			}
			ref.Features = append(ref.Features, entry)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return ref, err
		}

		// Class-wide option catalogs (subclass_slug IS NULL — e.g. Taijutsu
		// Specialist's 20-entry Martial Techniques list) never surfaced here
		// before: the loop below only ever queries subclass-scoped options.
		// A large class-wide catalog like this one already has its own
		// dedicated picker box on the sheet (Available/Known lists with
		// their own tooltips), but that box only shows the Available side
		// in full — this gives the Class Reference popup a complete,
		// always-available fallback listing every entry, matching how a
		// subclass's own options list is already treated below.
		optRows, err := s.rulesDB.Query(`
			SELECT list_name, name, description FROM class_options
			WHERE class_slug = ? AND subclass_slug IS NULL
			ORDER BY list_name, sort_order`, c.Slug)
		if err != nil {
			return ref, err
		}
		classBaseOpts := map[string]*subclassBaseOptions{}
		var classBaseOptsOrder []string
		for optRows.Next() {
			var listName, oname, odescription string
			if err := optRows.Scan(&listName, &oname, &odescription); err != nil {
				optRows.Close()
				return ref, err
			}
			b, ok := classBaseOpts[listName]
			if !ok {
				b = &subclassBaseOptions{ClassName: name, ListName: listName}
				classBaseOpts[listName] = b
				classBaseOptsOrder = append(classBaseOptsOrder, listName)
			}
			b.Options = append(b.Options, companionFeatureRef{Name: oname, Description: odescription})
		}
		optRows.Close()
		if err := optRows.Err(); err != nil {
			return ref, err
		}
		for _, listName := range classBaseOptsOrder {
			ref.BaseOptions = append(ref.BaseOptions, *classBaseOpts[listName])
		}
	}

	subRows, err := s.charDB.Query(
		`SELECT subclass_slug FROM character_subclasses WHERE character_id = ?`, characterID)
	if err != nil {
		return ref, err
	}
	var subclassSlugs []string
	for subRows.Next() {
		var slug string
		if err := subRows.Scan(&slug); err != nil {
			subRows.Close()
			return ref, err
		}
		subclassSlugs = append(subclassSlugs, slug)
	}
	subRows.Close()
	if err := subRows.Err(); err != nil {
		return ref, err
	}

	for _, slug := range subclassSlugs {
		var subclassName, classSlug string
		if err := s.rulesDB.QueryRow(`
			SELECT sc.name, g.class_slug FROM subclasses sc
			JOIN subclass_groups g ON g.slug = sc.group_slug
			WHERE sc.slug = ?`, slug,
		).Scan(&subclassName, &classSlug); err != nil {
			continue // a stale/removed subclass slug just contributes nothing
		}
		gateLevel, ok := classLevels[classSlug]
		if !ok {
			continue // subclass chosen for a class the character no longer has
		}
		sourceLabel := classNames[classSlug] + " — " + subclassName

		rows, err := s.rulesDB.Query(`
			SELECT name, level, description FROM v_subclass_features
			WHERE subclass_slug = ? ORDER BY sort_order, level`, slug)
		if err != nil {
			return ref, err
		}
		for rows.Next() {
			var fname, description string
			var level sql.NullInt64
			if err := rows.Scan(&fname, &level, &description); err != nil {
				rows.Close()
				return ref, err
			}
			entry := referenceFeatureEntry{
				Name: fname, SourceLabel: sourceLabel, IsSubclass: true,
				Level: int(level.Int64), Description: description,
				Locked: level.Valid && int(level.Int64) > gateLevel,
			}
			if scales := highestScalingLevel(description); scales > entry.Level {
				entry.ScalesThrough = scales
			}
			ref.Features = append(ref.Features, entry)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return ref, err
		}

		optRows, err := s.rulesDB.Query(`
			SELECT co.list_name, co.name, co.description FROM class_options co
			WHERE co.subclass_slug = ?
			  AND NOT EXISTS (SELECT 1 FROM class_option_entries e WHERE e.class_option_slug = co.slug)
			ORDER BY co.list_name, co.sort_order`, slug)
		if err != nil {
			return ref, err
		}
		baseOpts := subclassBaseOptions{ClassName: classNames[classSlug], SubclassName: subclassName}
		for optRows.Next() {
			var listName, name, description string
			if err := optRows.Scan(&listName, &name, &description); err != nil {
				optRows.Close()
				return ref, err
			}
			baseOpts.ListName = listName
			baseOpts.Options = append(baseOpts.Options, companionFeatureRef{Name: name, Description: description})
		}
		optRows.Close()
		if err := optRows.Err(); err != nil {
			return ref, err
		}
		if len(baseOpts.Options) > 0 {
			ref.BaseOptions = append(ref.BaseOptions, baseOpts)
		}
	}

	// One combined reading order across every class AND subclass — stable
	// by Level only (not the book's raw ingestion sort_order, which the
	// per-source queries above still order by first as a same-level tie-
	// break), matching mergeFeatFeatures' own technique for the Core tab's
	// Features & Traits list. This is what actually interleaves class and
	// subclass entries instead of leaving them as two separate stacked
	// blocks — sorting each source's own slice before concatenating (the
	// previous approach) never achieves that, no matter how each half is
	// individually ordered.
	sort.SliceStable(ref.Features, func(i, j int) bool { return ref.Features[i].Level < ref.Features[j].Level })

	return ref, nil
}

// classReferenceChoice is one entry in the multiclass Class Reference
// picker (handleCharacterReference) — which class to view, plus the level
// taken in it so the picker reads like the sheet's own class/level line.
type classReferenceChoice struct {
	Slug   string
	Name   string
	Levels int
}

// handleCharacterReference serves the "Class Reference" popup — the
// character's own full class+subclass feature catalog (general class
// features gold, subclass ones blue — see app.css's .subclass-scope),
// including features not yet reached (dimmed). Opened via
// reference-popup.js's window.open, the same real-separate-browser-window
// mechanism companion-popup.js already uses for a companion's own sheet, so
// it stays a movable window rather than a panel fixed in one spot.
//
// A single-class character goes straight to their one class's reference,
// same as before. A multiclass character instead sees a picker first (this
// same URL, no "class" query param) — merging every class into one list
// got hard to read once the same page also had to interleave two or more
// classes' worth of features, so the player picks one class at a time via
// ?class=<slug> instead, navigating within the same popup window
// (reference-popup.js only ever opens this one URL; every link after that
// is a normal same-window navigation, no extra JS needed).
func (s *server) handleCharacterReference(w http.ResponseWriter, r *http.Request) {
	id, err := parseCharacterID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sheet, err := charsheet.Compute(s.rulesDB, s.charDB, id)
	if err != nil {
		var exists int
		if s.charDB.QueryRow(`SELECT COUNT(*) FROM characters WHERE id = ?`, id).Scan(&exists) == nil && exists == 0 {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("compute sheet for reference popup:", err)
		return
	}

	classes, err := s.loadCharacterClassLevels(id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load character classes for reference popup:", err)
		return
	}
	isMulticlass := len(classes) > 1

	classSlug := r.URL.Query().Get("class")
	if classSlug == "" {
		if isMulticlass {
			choices := make([]classReferenceChoice, 0, len(classes))
			for _, c := range classes {
				var name string
				if err := s.rulesDB.QueryRow(`SELECT name FROM classes WHERE slug = ?`, c.Slug).Scan(&name); err != nil {
					name = c.Slug
				}
				choices = append(choices, classReferenceChoice{Slug: c.Slug, Name: name, Levels: c.Levels})
			}
			s.render(w, "character_reference_picker.html", map[string]any{
				"Title":         sheet.Name + " — Class Reference",
				"CharacterID":   id,
				"CharacterName": sheet.Name,
				"Classes":       choices,
			})
			return
		}
		if len(classes) == 1 {
			classSlug = classes[0].Slug
		}
	}

	ref, err := s.loadCharacterReference(id, classSlug)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		log.Println("load character reference:", err)
		return
	}
	ref.CharacterName = sheet.Name

	s.render(w, "character_reference.html", map[string]any{
		"Title":         sheet.Name + " — Class Reference",
		"CharacterID":   id,
		"CharacterName": sheet.Name,
		"Reference":     ref,
		"Multiclass":    isMulticlass,
	})
}
