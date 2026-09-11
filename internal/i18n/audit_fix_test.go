package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunAuditDetectsAndFixesHardcodedLiteral is a self-contained regression
// test (isolated temp-dir "repo", not the real one) proving the gate
// actually gates: a fresh hardcoded-literal violation is reported as new,
// -fix rewrites the call site and registers a catalog key, and a second run
// against the fixed source reports nothing new. This is the "deliberately
// reintroduce one known-shape gap" verification step from the translation
// engine plan, kept as a permanent test rather than a one-off manual check.
func TestRunAuditDetectsAndFixesHardcodedLiteral(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "cmd/n5e"))
	mustMkdirAll(t, filepath.Join(root, "internal/i18n/locales"))
	mustWriteFile(t, filepath.Join(root, "internal/i18n/locales/en.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "internal/i18n/audit_baseline.json"), "[]\n")

	src := `package n5e

type featureChoiceOption struct {
	Value       string
	Label       string
	Description string
}

func loadOptions(tr func(key string, args ...any) string) []featureChoiceOption {
	return []featureChoiceOption{
		{Value: "int", Label: "Test Option", Description: "A description."},
	}
}
`
	target := filepath.Join(root, "cmd/n5e/scratch_test_file.go")
	mustWriteFile(t, target, src)

	result, err := RunAudit(root, false)
	if err != nil {
		t.Fatalf("RunAudit (report-only): %v", err)
	}
	found := false
	for _, v := range result.New {
		if v.Kind == AuditHardcodedLiteral && v.Field == "Label" && v.Text == "Test Option" {
			found = true
			if !v.Fixable {
				t.Errorf("expected Label violation to be Fixable (tr param is in scope), got Fixable=false")
			}
		}
	}
	if !found {
		t.Fatalf("expected a hardcoded-literal violation for Label %q, got %+v", "Test Option", result.New)
	}

	fixResult, err := RunAudit(root, true)
	if err != nil {
		t.Fatalf("RunAudit (fix): %v", err)
	}
	if len(fixResult.Fixed) == 0 {
		t.Fatalf("expected at least one violation fixed, got none (New: %+v)", fixResult.New)
	}

	fixedSrc, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fixedSrc), `"Test Option"`) {
		t.Errorf("expected the literal to be rewritten out of the source, still present:\n%s", fixedSrc)
	}
	if !strings.Contains(string(fixedSrc), `tr("audit.scratch_test_file.label.`) {
		t.Errorf("expected a tr(\"audit.scratch_test_file.label.<hash>\") call site, got:\n%s", fixedSrc)
	}

	catalog, err := readCatalogFile(filepath.Join(root, "internal/i18n/locales/en.json"))
	if err != nil {
		t.Fatal(err)
	}
	haveText := false
	for _, v := range catalog {
		if v == "Test Option" {
			haveText = true
		}
	}
	if !haveText {
		t.Errorf("expected catalog to contain the English text %q, got %+v", "Test Option", catalog)
	}

	rerun, err := RunAudit(root, false)
	if err != nil {
		t.Fatalf("RunAudit (rerun after fix): %v", err)
	}
	for _, v := range rerun.New {
		if v.Field == "Label" {
			t.Errorf("expected no remaining Label violation after fix, still got: %+v", v)
		}
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
