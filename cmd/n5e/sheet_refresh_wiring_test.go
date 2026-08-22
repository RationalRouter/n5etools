package main

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// The data-also-refresh chain fails SILENTLY by design (sheet-refresh.js:
// "a cosmetic refresh must never be able to break the action that
// triggered it") — which means a wiring mistake ships with zero visible
// error and surfaces only as "this number stopped updating until I reload
// the page." That exact failure shipped twice before this test existed:
// #sheet-vitals carried no data-fragment attribute at all, so every
// level/subclass/ASI edit left Max HP, Max Chakra, and every Special
// Resource pool frozen at their pre-edit values (rests never noticed —
// they target sheet-vitals as their PRIMARY swap, which doesn't read the
// attribute); and the Pending Choices banner didn't exist in the DOM until
// the first full page load after a choice unlocked, so the ASI/Feat picker
// never appeared in place on the level-up that unlocked it.
//
// Invariants enforced, one per failure mode of window.n5eRefreshBlocks:
//  1. Every element id named in any literal data-also-refresh list has a
//     defining element in the templates, and that element carries a
//     data-fragment attribute in the SAME tag.
//  2. Every data-fragment template name (on any element, not just
//     also-refresh targets) is whitelisted in sheetLiveFragments — the
//     fragment endpoint 404s on anything not in that map, which
//     n5eRefreshBlocks reduces to a console.warn.
//
// Ids assembled through template variables ({{$alsoRefresh}},
// {{.AlsoRefresh}}) can't be extracted statically here; the literal ids
// their call sites pass ("sheet-elemental-affinities",
// "sheet-other-rollables", ...) still appear as literals at those call
// sites or in this package's Go string constants, and invariant 2 covers
// every data-fragment-carrying element regardless of how it gets refreshed.
//
// (?s): a doc comment can span many lines. Non-greedy so back-to-back
// comments on one joined string don't swallow the markup between them.
var templateCommentRe = regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`)

func TestSheetAlsoRefreshTargetsAreRefreshable(t *testing.T) {
	var pages []string
	err := fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := fs.ReadFile(templatesFS, path)
		if err != nil {
			return err
		}
		pages = append(pages, string(b))
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded templates: %v", err)
	}
	all := strings.Join(pages, "\n")
	// Doc comments quote example attribute values (`data-also-refresh="...
	// sheet-skills ..."`) that must not be mistaken for real wiring.
	all = templateCommentRe.ReplaceAllString(all, "")

	alsoRefreshRe := regexp.MustCompile(`data-also-refresh="([^"]*)"`)
	ids := map[string]bool{}
	for _, m := range alsoRefreshRe.FindAllStringSubmatch(all, -1) {
		for _, id := range strings.Fields(m[1]) {
			// Template-expression values can't be resolved statically —
			// invariant 2 still covers whatever they name at runtime.
			if strings.Contains(id, "{{") {
				continue
			}
			ids[id] = true
		}
	}
	if len(ids) == 0 {
		t.Fatal("found no literal data-also-refresh ids at all — the extraction regex is broken")
	}

	for id := range ids {
		// The defining element: id="X" preceded by a tag open on the same
		// tag (no > between < and the id attribute). A quoted-attribute
		// substring like data-box-id="sheet-squares" must NOT count as the
		// element — require id= at a word boundary that isn't part of a
		// longer attribute name (i.e. preceded by whitespace).
		defRe := regexp.MustCompile(`<[^>]*\sid="` + regexp.QuoteMeta(id) + `"[^>]*>`)
		tags := defRe.FindAllString(all, -1)
		if len(tags) == 0 {
			t.Errorf("data-also-refresh names %q but no element with that id exists in any template", id)
			continue
		}
		found := false
		for _, tag := range tags {
			if strings.Contains(tag, "data-fragment=") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("data-also-refresh names %q but its element carries no data-fragment attribute — n5eRefreshBlocks will silently skip it and the block will stay stale until a full page reload", id)
		}
	}

	fragmentRe := regexp.MustCompile(`data-fragment="([^"]+)"`)
	for _, m := range fragmentRe.FindAllStringSubmatch(all, -1) {
		name := m[1]
		if strings.Contains(name, "{{") {
			continue
		}
		if !sheetLiveFragments[name] {
			t.Errorf("template element declares data-fragment=%q but that name is not in sheetLiveFragments — the fragment endpoint will 404 and n5eRefreshBlocks will only console.warn", name)
		}
	}
}

// The reverse direction of the wiring test above: every fragment name the
// fragment endpoint accepts must actually render. A name present in
// sheetLiveFragments but missing from renderSheetFragment's switch (or the
// template define it dispatches to) would answer the fragment request with
// an executable-but-wrong result or a template error — the same silent
// console.warn failure shape. Rendering is exercised by the handler tests
// elsewhere; here it's enough that the two Go-side lists agree with the
// template-side data-fragment declarations, which the test above already
// ties to sheetLiveFragments — so this direction only needs to confirm
// every whitelisted name is declared by some template element (nothing in
// the whitelist is dead weight pointing at a define that no longer exists).
func TestSheetLiveFragmentsAllDeclaredInTemplates(t *testing.T) {
	var sb strings.Builder
	err := fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := fs.ReadFile(templatesFS, path)
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteString("\n")
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded templates: %v", err)
	}
	all := sb.String()

	for name := range sheetLiveFragments {
		// Either a data-fragment declaration (refreshable block) or a
		// {{define}} of the same name (primary-swap-only fragments like
		// sheet_ryo that nothing also-refreshes) counts as live.
		if !strings.Contains(all, fmt.Sprintf("data-fragment=%q", name)) &&
			!strings.Contains(all, fmt.Sprintf("{{define %q}}", name)) {
			t.Errorf("sheetLiveFragments whitelists %q but no template declares data-fragment=%q or defines that template — stale whitelist entry or missing wiring", name, name)
		}
	}
}
