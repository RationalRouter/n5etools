package i18n

import (
	"path/filepath"
	"testing"
)

func TestPendingGlossaryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "glossary-pending.json")

	got, err := LoadPendingGlossary(path)
	if err != nil {
		t.Fatalf("LoadPendingGlossary on missing file: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map for a missing file, got %v", got)
	}

	entries := map[string]PendingGlossaryEntry{
		"jutsu\x00jutsu/fireball\x00description": {
			Table: "jutsu", Slug: "jutsu/fireball", Field: "description",
			EnHash: "abc123", Terms: []string{"Cinderfall Wisp"},
		},
	}
	if err := SavePendingGlossary(path, entries); err != nil {
		t.Fatalf("SavePendingGlossary: %v", err)
	}

	reread, err := LoadPendingGlossary(path)
	if err != nil {
		t.Fatalf("LoadPendingGlossary: %v", err)
	}
	entry, ok := reread["jutsu\x00jutsu/fireball\x00description"]
	if !ok {
		t.Fatalf("reread map missing expected key, got %v", reread)
	}
	if entry.EnHash != "abc123" || len(entry.Terms) != 1 || entry.Terms[0] != "Cinderfall Wisp" {
		t.Errorf("reread entry = %+v, want EnHash abc123, Terms [Cinderfall Wisp]", entry)
	}
}
