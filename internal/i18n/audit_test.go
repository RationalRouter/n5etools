package i18n

import "testing"

// TestI18nAudit is the compiled coverage gate: it runs the same scanner as
// `n5e-i18n audit` against the repo root and fails if anything NOT already
// in audit_baseline.json turns up. This is what catches a hardcoded-string
// or missing-overlay gap the moment it's written in ordinary development,
// rather than only at the next translation run -- see this file's sibling
// audit.go and translation_methodology.md's "Automated coverage pipeline"
// section.
//
// A failure here means either: (a) a real new gap was introduced -- fix it,
// or run `go run ./cmd/n5e-i18n audit -fix` to auto-wire the mechanical
// subset, or (b) a genuine false positive / accepted exception -- mark it
// `// i18n:ignore <reason>` at the flagged line, or if it's a real
// deliberate exception at scale, regenerate the baseline via
// `go run ./cmd/n5e-i18n audit -seed-baseline` and note why in a commit
// message.
func TestI18nAudit(t *testing.T) {
	result, err := RunAudit("../..", false)
	if err != nil {
		t.Fatalf("RunAudit: %v", err)
	}
	if len(result.New) == 0 {
		return
	}
	t.Errorf("%d new i18n coverage violation(s) not in audit_baseline.json:", len(result.New))
	for _, v := range result.New {
		switch v.Kind {
		case AuditMissingOverlay:
			t.Errorf("  %s: [%s] table %q: %s", v.File, v.Kind, v.Table, v.Text)
		default:
			t.Errorf("  %s:%d: [%s] field %s: %q", v.File, v.Line, v.Kind, v.Field, v.Text)
		}
	}
}
