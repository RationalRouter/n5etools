package main

import (
	"encoding/json"
	"os"
	"time"
)

// translationState tracks, per "bookSlug|locale" key, the last time
// translate-book actually applied a translation for that book+locale --
// check-drive's own basis for comparison against a book's live Drive
// Last-Modified time. Deliberately a small maintainer-only JSON file next
// to glossary.md, not a rules.db table: end users' installs have no use
// for Sergio's own translation-workflow bookkeeping (see
// internal/autoupdate/translations.go's translation_bundles table for the
// unrelated, legitimately client-side state this is not).
type translationState map[string]time.Time

func stateKey(bookSlug, locale string) string { return bookSlug + "|" + locale }

func loadTranslationState(path string) (translationState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return translationState{}, nil
	}
	if err != nil {
		return nil, err
	}
	st := translationState{}
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return st, nil
}

func saveTranslationState(path string, st translationState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// touchTranslationState records "now" as the last-translated time for one
// book+locale, immediately after translate-book successfully applies at
// least one translation for it.
func touchTranslationState(path, bookSlug, locale string) error {
	st, err := loadTranslationState(path)
	if err != nil {
		return err
	}
	st[stateKey(bookSlug, locale)] = time.Now().UTC()
	return saveTranslationState(path, st)
}
