package i18n

import (
	"encoding/json"
	"os"
	"sort"
)

// PendingGlossaryEntry is one field's translation held back because its
// English text contains a term FindUnglossaried couldn't resolve against
// glossary.md or the known-terms set. Rows here are re-checked on every
// translate-book run and cleared once glossary.md covers every term they
// flagged -- see cmd/n5e-i18n's translate-book command.
type PendingGlossaryEntry struct {
	Table  string   `json:"table"`
	Slug   string   `json:"slug"`
	Field  string   `json:"field"`
	EnHash string   `json:"en_hash"`
	Terms  []string `json:"terms"`
}

// LoadPendingGlossary reads a pending-glossary file, returning an empty map
// (not an error) if it doesn't exist yet -- the normal state before any run
// has ever skipped a row. Keyed by "table\x00slug\x00field", matching
// KeySeparator's own convention elsewhere in this package.
func LoadPendingGlossary(path string) (map[string]PendingGlossaryEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]PendingGlossaryEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []PendingGlossaryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	out := make(map[string]PendingGlossaryEntry, len(entries))
	for _, e := range entries {
		out[e.Table+"\x00"+e.Slug+"\x00"+e.Field] = e
	}
	return out, nil
}

// SavePendingGlossary writes entries back out, sorted for a stable diff
// across runs.
func SavePendingGlossary(path string, entries map[string]PendingGlossaryEntry) error {
	list := make([]PendingGlossaryEntry, 0, len(entries))
	for _, e := range entries {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Table != list[j].Table {
			return list[i].Table < list[j].Table
		}
		if list[i].Slug != list[j].Slug {
			return list[i].Slug < list[j].Slug
		}
		return list[i].Field < list[j].Field
	})
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
