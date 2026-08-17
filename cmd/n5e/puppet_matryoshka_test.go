package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/sergio/n5e/internal/charstore"
)

// puppetMatryoshkaTestSetup seeds a minimal Puppet Master character with one
// Puppet Tool companion — the split/merge handlers don't themselves care
// about class/subclass rows (charstore.SplitCompanionIntoBodies/
// MergeCompanionBodies take no rules-db dependency at all), but respondSheet
// re-renders the full sheet_puppet_tab fragment, which does.
func puppetMatryoshkaTestSetup(t *testing.T) (s *server, companionID int64) {
	t.Helper()
	s = testServer(t)
	seedPuppetMasterRules(t, s)
	if _, err := s.charDB.Exec(`
		INSERT INTO characters (name, base_str, base_dex, base_con, base_int, base_wis, base_cha)
		VALUES ('Kankuro', 10, 10, 10, 10, 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_classes (character_id, class_slug, levels, order_index)
		VALUES (1, 'class/puppet-master', 14, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.charDB.Exec(`
		INSERT INTO character_subclasses (character_id, subclass_slug, chosen_at_level)
		VALUES (1, 'class/puppet-master/group/puppet-techniques/blue-technique-warmaster', 2)`); err != nil {
		t.Fatal(err)
	}
	companionID, err := charstore.AddCompanion(s.charDB, 1, "puppet", "Sandman")
	if err != nil {
		t.Fatal(err)
	}
	if err := charstore.SetCompanionIntField(s.charDB, 1, companionID, "hp_max",
		nullInt(31)); err != nil {
		t.Fatal(err)
	}
	return s, companionID
}

func TestHandlePuppetMatryoshkaSplit(t *testing.T) {
	s, companionID := puppetMatryoshkaTestSetup(t)
	cid := strconv.FormatInt(companionID, 10)

	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/matryoshka-split",
		strings.NewReader(url.Values{"count": {"3"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", cid)
	w := httptest.NewRecorder()
	s.handlePuppetMatryoshkaSplit(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("split: status %d, body %s", w.Code, w.Body.String())
	}

	all, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("len(all) = %d, want 3", len(all))
	}
}

func TestHandlePuppetMatryoshkaSplitRejectsBadCount(t *testing.T) {
	s, companionID := puppetMatryoshkaTestSetup(t)
	cid := strconv.FormatInt(companionID, 10)

	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/matryoshka-split",
		strings.NewReader(url.Values{"count": {"5"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", cid)
	w := httptest.NewRecorder()
	s.handlePuppetMatryoshkaSplit(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("split with count=5: status %d, want 400", w.Code)
	}

	all, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("len(all) = %d, want 1 (rejected split should not touch anything)", len(all))
	}
}

func TestHandlePuppetMatryoshkaSplitRejectsNonPuppet(t *testing.T) {
	s, _ := puppetMatryoshkaTestSetup(t)
	summonID, err := charstore.AddCompanion(s.charDB, 1, "summon", "Bear Summon")
	if err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(summonID, 10)

	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/matryoshka-split",
		strings.NewReader(url.Values{"count": {"2"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", cid)
	w := httptest.NewRecorder()
	s.handlePuppetMatryoshkaSplit(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("split a summon: status %d, want 400", w.Code)
	}
}

func TestHandlePuppetMatryoshkaMerge(t *testing.T) {
	s, companionID := puppetMatryoshkaTestSetup(t)
	if err := charstore.SplitCompanionIntoBodies(s.charDB, 1, companionID, 2); err != nil {
		t.Fatal(err)
	}
	cid := strconv.FormatInt(companionID, 10)

	req := httptest.NewRequest(http.MethodPost, "/characters/1/companions/"+cid+"/matryoshka-merge", nil)
	req.Header.Set("X-Requested-With", "fetch")
	req.SetPathValue("id", "1")
	req.SetPathValue("cid", cid)
	w := httptest.NewRecorder()
	s.handlePuppetMatryoshkaMerge(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("merge: status %d, body %s", w.Code, w.Body.String())
	}

	all, err := charstore.ListCompanions(s.charDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("len(all) = %d, want 1", len(all))
	}
}
