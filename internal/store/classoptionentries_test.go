package store

import (
	"database/sql"
	"testing"
)

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func seedPuppetMasterOptionFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `INSERT INTO classes (slug, name) VALUES ('class/puppet-master', 'Puppet Master')`)
	mustExec(t, db, `INSERT INTO class_options (slug, class_slug, list_name, name, description, sort_order)
		VALUES ('class/puppet-master/option/black-iron-upgrades/wood-tier', 'class/puppet-master',
		        'Black Iron Upgrades', 'Wood Tier',
		        'CHAKRA DISRUPTION BLADE Techniques: Black, Perfect You fit your Puppet Tool with multiple small compartments of Black Iron Sand. HIDDEN BLADES Techniques: Black, Perfect You install blades within your Puppet.',
		        0)`)
	// Already one-row-per-option (Blue Technique's untiered Puppet Weapon
	// Types list) — must NOT get split, since its own description has no
	// second bundled name.
	mustExec(t, db, `INSERT INTO class_options (slug, class_slug, list_name, name, description, sort_order)
		VALUES ('class/puppet-master/option/puppet-weapon-types/drone-weapon', 'class/puppet-master',
		        'Puppet Weapon Types', 'Drone Weapon',
		        'Your Puppet Tool gains a ranged natural weapon that deals 1d8 piercing damage.', 0)`)
}

func TestLoadClassOptionEntriesSplitsBundledTier(t *testing.T) {
	db := testDB(t)
	seedPuppetMasterOptionFixture(t, db)

	report, err := LoadClassOptionEntries(db, SourceBook{Slug: "book/class-compendium", Version: "3.12"}, "class/puppet-master")
	if err != nil {
		t.Fatal(err)
	}
	if report.Created != 2 {
		t.Errorf("created = %d, want 2 (only the bundled tier's two names)", report.Created)
	}

	rows, err := db.Query(`SELECT name, description FROM class_option_entries WHERE class_option_slug = ? ORDER BY sort_order`,
		"class/puppet-master/option/black-iron-upgrades/wood-tier")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name, desc string
		if err := rows.Scan(&name, &desc); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
		if desc == "" {
			t.Errorf("entry %q has empty description", name)
		}
	}
	want := []string{"Chakra Disruption Blade", "Hidden Blades"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestLoadClassOptionEntriesLeavesUntieredSingleOptionRowsAlone(t *testing.T) {
	db := testDB(t)
	seedPuppetMasterOptionFixture(t, db)

	if _, err := LoadClassOptionEntries(db, SourceBook{Slug: "book/class-compendium", Version: "3.12"}, "class/puppet-master"); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM class_option_entries WHERE class_option_slug = ?`,
		"class/puppet-master/option/puppet-weapon-types/drone-weapon").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("got %d entries for the untiered single-option row, want 0", n)
	}
}

// TestLoadClassOptionEntriesFixesKnownGaps covers the two real-corpus
// anchor-gap shapes found 2026-08-05 (a full-database Puppet Master audit):
// a missing terminal period before a bundled sub-entry's own header (fixed
// via knownMissingPeriodFixes), and a trailing parenthetical annotation
// between the header and its "Keyword:" line (fixed via capsEntryPattern's
// own widened regex) — both previously swallowed the second entry entirely
// into the first one's body, with no row of its own.
func TestLoadClassOptionEntriesFixesKnownGaps(t *testing.T) {
	db := testDB(t)
	mustExec(t, db, `INSERT INTO classes (slug, name) VALUES ('class/puppet-master', 'Puppet Master')`)
	mustExec(t, db, `INSERT INTO class_options (slug, class_slug, list_name, name, description, sort_order)
		VALUES ('class/puppet-master/option/interwoven-upgrades/bronze-tier', 'class/puppet-master',
		        'Interwoven Upgrades', 'Bronze Tier',
		        'ANTAGONISTIC CONNECTION Techniques: White You can attempt to connect to unwilling creatures, while using this upgrade, you are considered to be concentrating on a B-Rank jutsu BOB AND WEAVE Techniques: White When you take the help action on a creature connected to your strings, they get advantage.',
		        0)`)
	mustExec(t, db, `INSERT INTO class_options (slug, class_slug, list_name, name, description, sort_order)
		VALUES ('class/puppet-master/option/upgrades-of-war/gold-tier', 'class/puppet-master',
		        'Upgrades of War', 'Gold Tier',
		        'KILL COMMAND Techniques: Blue You take an Overdrive mechanism used by other Puppet Masters. Once this Upgrade ends, your Puppet Tool gains 4 ranks of weakened. IMPROVED ARCHITECTURE II (BLUE) Techniques: Blue Prerequisite: Improved Architecture I (Blue) You improve upon the design of your Puppet Tool.',
		        0)`)

	if _, err := LoadClassOptionEntries(db, SourceBook{Slug: "book/class-compendium", Version: "3.12"}, "class/puppet-master"); err != nil {
		t.Fatal(err)
	}

	assertEntryNames := func(classOptionSlug string, want []string) {
		t.Helper()
		rows, err := db.Query(`SELECT name FROM class_option_entries WHERE class_option_slug = ? ORDER BY sort_order`, classOptionSlug)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var got []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			got = append(got, name)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: names = %v, want %v", classOptionSlug, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: names[%d] = %q, want %q", classOptionSlug, i, got[i], want[i])
			}
		}
	}

	assertEntryNames("class/puppet-master/option/interwoven-upgrades/bronze-tier",
		[]string{"Antagonistic Connection", "Bob and Weave"})
	assertEntryNames("class/puppet-master/option/upgrades-of-war/gold-tier",
		[]string{"Kill Command", "Improved Architecture II (Blue)"})
}

func TestLoadClassOptionEntriesRerunIsStable(t *testing.T) {
	db := testDB(t)
	seedPuppetMasterOptionFixture(t, db)
	book := SourceBook{Slug: "book/class-compendium", Version: "3.12"}

	if _, err := LoadClassOptionEntries(db, book, "class/puppet-master"); err != nil {
		t.Fatal(err)
	}
	report, err := LoadClassOptionEntries(db, book, "class/puppet-master")
	if err != nil {
		t.Fatal(err)
	}
	if report.Created != 0 || report.Updated != 0 || report.Unchanged != 2 {
		t.Errorf("second run = %+v, want 0 created/updated, 2 unchanged", report)
	}
}
