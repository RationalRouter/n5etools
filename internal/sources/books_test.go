package sources

import "testing"

// TestOfficialBooks catches the kind of typo that's easy to introduce in a
// hand-written literal: a duplicate or empty slug/URL, or a slug that no
// longer matches what cmd/n5e-ingest's loaders actually use — this list is
// the input a future auto-fetcher hands to those same loaders.
func TestOfficialBooks(t *testing.T) {
	wantSlugs := map[string]bool{
		"book/core": true, "book/class-compendium": true, "book/clan-compendium": true,
		"book/jutsu-compendium": true, "book/kage-guide": true, "book/bingo-book": true,
	}
	seen := map[string]bool{}
	requiredCount := 0
	for _, b := range OfficialBooks {
		if b.Slug == "" || b.Title == "" || b.DriveURL == "" {
			t.Errorf("incomplete entry: %+v", b)
		}
		if seen[b.Slug] {
			t.Errorf("duplicate slug: %s", b.Slug)
		}
		seen[b.Slug] = true
		if !wantSlugs[b.Slug] {
			t.Errorf("unexpected slug: %s", b.Slug)
		}
		if b.Required {
			requiredCount++
		}
	}
	if len(OfficialBooks) != 6 {
		t.Errorf("got %d books, want 6", len(OfficialBooks))
	}
	if requiredCount != 4 {
		t.Errorf("required books = %d, want 4 (Kage Guide and the Bingo Book are the optional, GM-facing ones)", requiredCount)
	}
}
