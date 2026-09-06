package i18n

import "testing"

func TestCatalogsLoad(t *testing.T) {
	for _, locale := range SupportedLocales {
		if _, ok := catalogs[locale]; !ok {
			t.Fatalf("catalog for locale %q did not load", locale)
		}
	}
}

// TestNoOrphanKeys ensures every non-English catalog only defines keys that
// also exist in English -- a key present only in a translated locale is
// almost certainly a typo, since it can never be reached by a caller that
// falls back through English.
func TestNoOrphanKeys(t *testing.T) {
	en := catalogs["en"]
	for _, locale := range SupportedLocales {
		if locale == "en" {
			continue
		}
		for key := range catalogs[locale] {
			if _, ok := en[key]; !ok {
				t.Errorf("locales/%s.json defines key %q, missing from locales/en.json", locale, key)
			}
		}
	}
}

func TestTranslatorFallsBackToEnglish(t *testing.T) {
	tr := Translator("es")
	if got := tr("locale.name.es"); got != "Español" {
		t.Errorf("expected Español, got %q", got)
	}
	// A key that doesn't exist in any catalog returns the key itself,
	// never a blank string or a panic.
	if got := tr("does.not.exist"); got != "does.not.exist" {
		t.Errorf("expected missing key echoed back, got %q", got)
	}
}
