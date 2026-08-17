package main

import (
	"database/sql"
	"testing"

	"github.com/sergio/n5e/internal/charsheet"
	"github.com/sergio/n5e/internal/charstore"
)

func nullInt(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}

func TestPuppetSwarmStatsAggregatesAcrossPuppets(t *testing.T) {
	companions := []charstore.Companion{
		{Kind: "puppet", AC: nullInt(15), HPMax: nullInt(40), Str: nullInt(16), Dex: nullInt(12), Con: nullInt(14), Int: nullInt(6), Wis: nullInt(6), Cha: nullInt(6)},
		{Kind: "puppet", AC: nullInt(18), HPMax: nullInt(60), Str: nullInt(12), Dex: nullInt(18), Con: nullInt(10), Int: nullInt(6), Wis: nullInt(10), Cha: nullInt(6)},
		// A non-puppet companion (a Summoning Technique summon) must not
		// contribute to the swarm's own stats at all.
		{Kind: "summon", AC: nullInt(99), HPMax: nullInt(999), Str: nullInt(20)},
	}

	got := puppetSwarmStats(companions, 10)

	if got.AC != 20 { // highest AC (18) + 2
		t.Errorf("AC = %d, want 20", got.AC)
	}
	if got.HP != 150 { // (40+60) * 1.5
		t.Errorf("HP = %d, want 150", got.HP)
	}
	if got.HP100 != 200 { // (40+60) * 2, even though not yet active at L10
		t.Errorf("HP100 = %d, want 200", got.HP100)
	}
	if got.Speed != 60 {
		t.Errorf("Speed = %d, want 60", got.Speed)
	}
	if got.Str != 16 || got.Dex != 18 || got.Con != 14 || got.Wis != 10 {
		t.Errorf("ability scores = Str %d Dex %d Con %d Wis %d, want 16/18/14/10", got.Str, got.Dex, got.Con, got.Wis)
	}
	if got.DexMod != 4 { // (18-10)/2
		t.Errorf("DexMod = %d, want 4", got.DexMod)
	}
	if got.Has100 {
		t.Error("Has100 = true at level 10, want false")
	}
	if got.Commands != 5 || got.Commands100 != 0 {
		t.Errorf("Commands = %d/%d, want 5/0 below L20", got.Commands, got.Commands100)
	}
}

func TestPuppetSwarmStatsPerformanceOf100(t *testing.T) {
	companions := []charstore.Companion{
		{Kind: "puppet", AC: nullInt(15), HPMax: nullInt(40)},
	}
	got := puppetSwarmStats(companions, 20)
	if !got.Has100 {
		t.Error("Has100 = false at level 20, want true")
	}
	if got.Commands100 != 10 {
		t.Errorf("Commands100 = %d, want 10", got.Commands100)
	}
}

func TestPuppetSwarmStatsNoPuppets(t *testing.T) {
	got := puppetSwarmStats(nil, 10)
	if got.AC != 0 || got.HP != 0 {
		t.Errorf("with no puppet companions, got AC=%d HP=%d, want 0/0", got.AC, got.HP)
	}
}

func TestLoadPuppetSwarmReferenceTextMissingRow(t *testing.T) {
	s := testServer(t)
	got, err := s.loadPuppetSwarmReferenceText()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("got %q with no seeded row, want empty", got)
	}
}

func TestLoadPuppetSwarmReferenceTextFound(t *testing.T) {
	s := testServer(t)
	if _, err := s.rulesDB.Exec(
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 10)`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.rulesDB.Exec(
		`INSERT INTO puppet_swarm_stat_block (class_slug, raw_text) VALUES ('class/puppet-master', 'Huge Swarm of Puppets...')`,
	); err != nil {
		t.Fatal(err)
	}
	got, err := s.loadPuppetSwarmReferenceText()
	if err != nil {
		t.Fatal(err)
	}
	if got != "Huge Swarm of Puppets..." {
		t.Errorf("got %q, want the seeded raw_text", got)
	}
}

// seedRedTechniquePuppetMaster seeds a minimal Puppet Master class plus its
// Red Technique ~ Performer subclass — same shape as companions_test.go's
// own seedPuppetMasterRules, just for Red instead of Blue, since that's the
// only subclass loadPuppetsTabData's own Puppet Swarm gating cares about.
func seedRedTechniquePuppetMaster(t *testing.T, s *server) {
	t.Helper()
	stmts := []string{
		`INSERT INTO classes (slug, name, hit_die, chakra_die) VALUES ('class/puppet-master', 'Puppet Master', 8, 8)`,
		`INSERT INTO subclass_groups (slug, class_slug, display_name) VALUES
			('class/puppet-master/group/puppet-techniques', 'class/puppet-master', 'Puppet Techniques')`,
		`INSERT INTO subclasses (slug, group_slug, name) VALUES
			('class/puppet-master/group/puppet-techniques/red-technique-performer',
			 'class/puppet-master/group/puppet-techniques', 'Red Technique ~ Performer')`,
		`INSERT INTO subclasses (slug, group_slug, name) VALUES
			('class/puppet-master/group/puppet-techniques/blue-technique-warmaster',
			 'class/puppet-master/group/puppet-techniques', 'Blue Technique ~ Warmaster')`,
	}
	for _, stmt := range stmts {
		if _, err := s.rulesDB.Exec(stmt); err != nil {
			t.Fatalf("seed rules: %v\n%s", err, stmt)
		}
	}
}

// TestLoadPuppetsTabDataPuppetSwarmGating confirms the Puppet Swarm card is
// only populated for Red Technique at 10th Puppet Master level or above —
// every other technique, and Red Technique below 10th level, must leave it
// nil so the template's own {{if .PuppetsTab.PuppetSwarm}} guard hides the
// panel entirely.
func TestLoadPuppetsTabDataPuppetSwarmGating(t *testing.T) {
	cases := []struct {
		name         string
		subclassSlug string
		level        int
		wantPresent  bool
	}{
		{"red at 10", "class/puppet-master/group/puppet-techniques/red-technique-performer", 10, true},
		{"red at 20", "class/puppet-master/group/puppet-techniques/red-technique-performer", 20, true},
		{"red at 9", "class/puppet-master/group/puppet-techniques/red-technique-performer", 9, false},
		{"blue at 10", "class/puppet-master/group/puppet-techniques/blue-technique-warmaster", 10, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			seedRedTechniquePuppetMaster(t, s)
			if _, err := s.charDB.Exec(`INSERT INTO characters (id, name) VALUES (1, 'Sasori')`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.charDB.Exec(`
				INSERT INTO character_classes (character_id, class_slug, levels, order_index)
				VALUES (1, 'class/puppet-master', ?, 0)`, c.level); err != nil {
				t.Fatal(err)
			}
			if _, err := s.charDB.Exec(`
				INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
				VALUES (1, ?, 2)`, c.subclassSlug); err != nil {
				t.Fatal(err)
			}

			sheet, err := charsheet.Compute(s.rulesDB, s.charDB, 1)
			if err != nil {
				t.Fatal(err)
			}
			data, err := s.loadPuppetsTabData(1, sheet)
			if err != nil {
				t.Fatal(err)
			}
			if (data.PuppetSwarm != nil) != c.wantPresent {
				t.Errorf("PuppetSwarm present = %v, want %v", data.PuppetSwarm != nil, c.wantPresent)
			}
		})
	}
}
