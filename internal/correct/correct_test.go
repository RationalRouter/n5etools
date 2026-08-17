package correct

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sergio/n5e/internal/parse"
)

func TestSweep_MisspellAndApostropheOnly(t *testing.T) {
	clans := []parse.Clan{
		{
			Name:     "Test Clan",
			Overview: "This clan has recieved many honors.",
			Traits: []parse.ClanTrait{
				{Name: "Weapon Proficiencies", Description: "You are proficient with Katana's, Broadswords and Odachi's."},
			},
			Feats: []parse.Feat{
				{Name: "Irrelevant Field", Prerequisites: "recieve this"}, // not in the registry — must stay untouched
			},
		},
	}

	rep, err := Sweep(context.Background(), &clans, Options{SkipLanguageTool: true})
	if err != nil {
		t.Fatal(err)
	}

	if clans[0].Overview != "This clan has received many honors." {
		t.Errorf("Overview = %q", clans[0].Overview)
	}
	if clans[0].Traits[0].Description != "You are proficient with Katanas, Broadswords and Odachis." {
		t.Errorf("Trait description = %q", clans[0].Traits[0].Description)
	}
	if clans[0].Feats[0].Prerequisites != "recieve this" {
		t.Errorf("Feats.Prerequisites should not be touched (not in registry): %q", clans[0].Feats[0].Prerequisites)
	}
	if rep.MisspellFixes != 1 {
		t.Errorf("MisspellFixes = %d, want 1", rep.MisspellFixes)
	}
	if rep.ApostropheFixes != 2 {
		t.Errorf("ApostropheFixes = %d, want 2", rep.ApostropheFixes)
	}

	var traitRecord *Record
	for i := range rep.Records {
		if rep.Records[i].Field == "Description" && rep.Records[i].EntityType == "ClanTrait" {
			traitRecord = &rep.Records[i]
			break
		}
	}
	if traitRecord == nil {
		t.Fatal("expected a Record for ClanTrait.Description")
	}
	if traitRecord.EntityPath != "test-clan/weapon-proficiencies" {
		t.Errorf("EntityPath = %q, want %q", traitRecord.EntityPath, "test-clan/weapon-proficiencies")
	}
}

func TestSweep_LanguageToolAppliesAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(ltResponse{Matches: []ltMatch{
			{
				Offset: 0, Length: 5,
				Replacements: []struct {
					Value string `json:"value"`
				}{{Value: "Howdy"}},
				Rule: struct {
					ID       string `json:"id"`
					Category struct {
						ID string `json:"id"`
					} `json:"category"`
				}{ID: "GREETING", Category: struct {
					ID string `json:"id"`
				}{ID: "GRAMMAR"}},
			},
		}})
	}))
	defer srv.Close()

	clans := []parse.Clan{{Name: "Test Clan", Overview: "Hello there, traveler."}}
	cache := LoadCache(t.TempDir() + "/cache.json")
	opts := Options{Cache: cache, BaseURL: srv.URL}

	rep, err := Sweep(context.Background(), &clans, opts)
	if err != nil {
		t.Fatal(err)
	}
	if clans[0].Overview != "Howdy there, traveler." {
		t.Errorf("Overview = %q", clans[0].Overview)
	}
	if rep.LanguageToolFixes != 1 {
		t.Errorf("LanguageToolFixes = %d, want 1", rep.LanguageToolFixes)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	// Second sweep of equivalent input text should hit the cache: zero
	// further network calls, same correction applied.
	clans2 := []parse.Clan{{Name: "Other Clan", Overview: "Hello there, traveler."}}
	rep2, err := Sweep(context.Background(), &clans2, opts)
	if err != nil {
		t.Fatal(err)
	}
	if clans2[0].Overview != "Howdy there, traveler." {
		t.Errorf("Overview = %q", clans2[0].Overview)
	}
	if calls != 1 {
		t.Errorf("calls = %d after cached sweep, want still 1", calls)
	}
	if rep2.LanguageToolFixes != 1 {
		t.Errorf("cached LanguageToolFixes = %d, want 1", rep2.LanguageToolFixes)
	}
}

func TestSweep_LanguageToolFailureDegradesGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	clans := []parse.Clan{{Name: "Test Clan", Overview: "This clan has recieved many honors."}}
	rep, err := Sweep(context.Background(), &clans, Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Sweep must not hard-fail on a LanguageTool outage: %v", err)
	}
	// misspell still ran even though LanguageTool was unreachable.
	if clans[0].Overview != "This clan has received many honors." {
		t.Errorf("Overview = %q", clans[0].Overview)
	}
	if len(rep.Warnings) == 0 {
		t.Error("expected a Warning about the LanguageTool outage")
	}
}

func TestSweep_RejectsNonPointerToSlice(t *testing.T) {
	if _, err := Sweep(context.Background(), parse.Clan{}, Options{}); err == nil {
		t.Fatal("want error for non pointer-to-slice input")
	}
}
