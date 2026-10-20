package crafting

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/RationalRouter/n5e/internal/schema"
)

// openBuiltRulesDB copies the repo's built out/rules.db to a temp dir and
// brings it up to date with every rules migration. Skips when no built
// database is present.
func openBuiltRulesDB(t *testing.T) *sql.DB {
	t.Helper()
	src, err := os.Open(filepath.Join("..", "..", "out", "rules.db"))
	if err != nil {
		t.Skipf("no built rules.db: %v", err)
	}
	defer src.Close()
	path := filepath.Join(t.TempDir(), "rules.db")
	dst, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	dst.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := schema.Apply(db, schema.Rules); err != nil {
		t.Fatalf("applying rules migrations: %v", err)
	}
	return db
}

var asksForChoice = regexp.MustCompile(`(?i)\b(select|choose)\b`)

// TestSealChoicesCoverage fails when a seal's text asks the crafter to pick
// something and sealChoices has no rule for it, or when a rule names a seal
// that doesn't exist.
func TestSealChoicesCoverage(t *testing.T) {
	db := openBuiltRulesDB(t)
	c, err := LoadCatalog(db, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Seals) != 149 {
		t.Errorf("seals = %d, want 149", len(c.Seals))
	}
	for _, s := range c.Seals {
		if asksForChoice.MatchString(s.Description) && s.Choice == nil {
			t.Errorf("%s asks for a choice but has no sealChoices rule", s.Slug)
		}
		if _, ok := RankRuleFor(s.Rank); !ok {
			t.Errorf("%s has unknown rank %q", s.Slug, s.Rank)
		}
		if s.AppliesTo != "weapon" && s.AppliesTo != "armor" {
			t.Errorf("%s applies to %q", s.Slug, s.AppliesTo)
		}
	}
	for slug, rule := range sealChoices {
		if _, ok := c.Seal(slug); !ok {
			t.Errorf("sealChoices names missing seal %s", slug)
		}
		switch rule.Kind {
		case ChoiceSummonTribe:
			if len(c.SummonsFor(rule)) == 0 {
				t.Errorf("%s: no summon tribes of types %v", slug, rule.SummonTypes)
			}
		case ChoiceJutsu:
			if len(c.JutsuFor(rule)) == 0 {
				t.Errorf("%s: no storable jutsu up to rank %s", slug, rule.MaxJutsuRank)
			}
		}
	}
	for fam := range dieDamageFamilies {
		found := false
		for _, s := range c.Seals {
			if s.Family == fam {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("dieDamageFamilies names missing family %s", fam)
		}
	}
}

// TestBookExampleAgainstCatalog prices the real Chunin Jacket the way the
// book's crafting example does.
func TestBookExampleAgainstCatalog(t *testing.T) {
	db := openBuiltRulesDB(t)
	c, err := LoadCatalog(db, "en")
	if err != nil {
		t.Fatal(err)
	}
	r := recipe("armor/chunin-jacket", "standard")
	if res := Calculate(c, r); res.BaseValue != 750 || res.BaseDowntime != 8 {
		t.Errorf("solo: value %v downtime %d, want 750 / 8", res.BaseValue, res.BaseDowntime)
	}
	r.Crafting.Participants = 3
	if res := Calculate(c, r); res.BaseDowntime != 3 {
		t.Errorf("three crafters: downtime %d, want 3", res.BaseDowntime)
	}

	refined, ok := c.Seal("seal/weapon/refined/hunter-seal")
	if !ok || refined.Rank != "C" || refined.CostRyo != 650 {
		t.Errorf("refined hunter seal = %+v", refined)
	}
	superior, ok := c.Seal("seal/weapon/superior/hunter-seal")
	if !ok || superior.Rank != "A" || superior.CostRyo != 1800 {
		t.Errorf("superior hunter seal = %+v", superior)
	}
}
