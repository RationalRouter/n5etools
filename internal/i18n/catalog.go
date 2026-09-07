package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed locales/*.json
var localeFS embed.FS

// Catalog maps a UI string key to its translated text for one locale.
// English ("en") is the source of truth: every key used anywhere in the
// app must exist in locales/en.json, enforced by catalog_test.go.
type Catalog map[string]string

var catalogs map[string]Catalog

func init() {
	catalogs = make(map[string]Catalog, len(SupportedLocales))
	for _, locale := range SupportedLocales {
		cat, err := loadCatalog(locale)
		if err != nil {
			panic(fmt.Sprintf("i18n: loading locale %q: %v", locale, err))
		}
		catalogs[locale] = cat
	}
}

// SupportedLocales is the ordered, known-good set of locale codes the app
// accepts -- both for the locale-switcher UI (English first, then in this
// order) and for validating the n5e_locale cookie. Extend this slice (and
// add the matching locales/<code>.json file) to ship a new language; no
// other code changes are required for the UI-catalog side of a new locale.
var SupportedLocales = []string{"en", "es", "pt"}

// IsSupported reports whether locale is one of SupportedLocales.
func IsSupported(locale string) bool {
	for _, l := range SupportedLocales {
		if l == locale {
			return true
		}
	}
	return false
}

func loadCatalog(locale string) (Catalog, error) {
	data, err := localeFS.ReadFile("locales/" + locale + ".json")
	if err != nil {
		return nil, err
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("parsing locales/%s.json: %w", locale, err)
	}
	return cat, nil
}

// TranslateFunc looks up key in the active locale's catalog, falling back
// to English when the key is missing from that locale (never a raw key or
// a blank string), then formats it with fmt.Sprintf if args are given.
type TranslateFunc func(key string, args ...any) string

// Translator returns a TranslateFunc bound to locale. Safe to call per
// request; catalogs are loaded once at startup and read-only after that.
func Translator(locale string) TranslateFunc {
	cat, ok := catalogs[locale]
	if !ok {
		cat = catalogs["en"]
	}
	return func(key string, args ...any) string {
		s, ok := cat[key]
		if !ok {
			s, ok = catalogs["en"][key]
			if !ok {
				// Missing from English too is a catalog bug, but must never
				// surface as a broken page -- show the key itself so it's
				// at least obvious in testing, rather than an empty string.
				return key
			}
		}
		if len(args) > 0 {
			return fmt.Sprintf(s, args...)
		}
		return s
	}
}
