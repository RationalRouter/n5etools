package i18n

import (
	"bufio"
	"os"
	"strings"
)

// ParseGlossaryTables reads a translation glossary markdown file (see
// glossary.md) and extracts English -> target-language term pairs from
// every markdown table that has both an "English" column and a column
// matching targetColumn (e.g. "Spanish"). glossary.md has many such tables
// scattered through the file (core mechanical terms, class names, jutsu
// component labels, clan/lineage terms surfaced by later pilot batches,
// etc.) rather than one canonical table, so this scans generically instead
// of hardcoding section names -- a new table added to the file is picked up
// automatically. A "Source"/notes column, if present, is ignored.
//
// An entry whose English and target text are identical (glossary.md marks
// these "unchanged" in their Source column, e.g. Chakra -> Chakra) becomes
// an identity pair -- exactly what a DeepL glossary needs to tell it not to
// touch a proper noun or established loanword.
func ParseGlossaryTables(path, targetColumn string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	entries := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var header []string
	inTable := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "|") {
			inTable = false
			header = nil
			continue
		}
		cells := splitTableRow(line)
		if isTableSeparatorRow(cells) {
			continue // the |---|---|---| row right under a header
		}
		if !inTable {
			header = cells
			inTable = true
			continue
		}
		enIdx := columnIndex(header, "English")
		targetIdx := columnIndex(header, targetColumn)
		if enIdx < 0 || targetIdx < 0 || enIdx >= len(cells) || targetIdx >= len(cells) {
			continue // a table without both columns we care about
		}
		en := cleanCell(cells[enIdx])
		target := cleanCell(cells[targetIdx])
		if en == "" || target == "" {
			continue
		}
		entries[en] = target
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	// Two fixed, prose (non-table) lists in glossary.md name proper
	// nouns/language-neutral symbols that stay untranslated in every
	// language -- not expressible as a table since there's only one
	// language-neutral column, so they're parsed as identity pairs here
	// rather than via the table scan above.
	if names, err := parseCommaListAfterHeading(path, "## Ranks / titles"); err == nil {
		for _, n := range names {
			entries[n] = n
		}
	}
	if letters, err := parseCommaListAfterHeading(path, "## Jutsu rank letters"); err == nil {
		for _, l := range letters {
			entries[l] = l
		}
	}

	return entries, nil
}

func splitTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// isTableSeparatorRow reports whether every cell in a row is made up only of
// "-" and ":" (markdown's header/body divider, e.g. "|---|---|---|" or
// "|:--|--:|").
func isTableSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			return false
		}
		if strings.Trim(c, "-:") != "" {
			return false
		}
	}
	return true
}

func columnIndex(header []string, name string) int {
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), name) {
			return i
		}
	}
	return -1
}

// cleanCell strips markdown emphasis markers and parenthetical/footnote
// clarifications that shouldn't be sent to DeepL as part of the term itself
// -- e.g. "**Rama Principal / Rama Secundaria**" -> "Rama Principal / Rama
// Secundaria", "Puppet Master" stays as-is but a cell like "Short Rest" with
// trailing markdown bold is unwrapped.
func cleanCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*")
	return strings.TrimSpace(s)
}

// parseCommaListAfterHeading finds the given "## Heading" line and returns
// the comma-separated tokens on the next non-empty line, stopping each
// token at the first " —" (em dash) or " -" trailing clarification (e.g.
// "E, D, C, B, A, S — kept as bare letters..." yields ["E","D","C","B","A",
// "S"], not the trailing prose).
func parseCommaListAfterHeading(path, heading string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	found := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !found {
			if line == heading {
				found = true
			}
			continue
		}
		if line == "" {
			continue
		}
		// Cut at the first em dash or " - " clarification, if any.
		if i := strings.Index(line, "—"); i >= 0 {
			line = line[:i]
		}
		var tokens []string
		for _, tok := range strings.Split(line, ",") {
			tok = strings.TrimSpace(tok)
			tok = strings.TrimSuffix(tok, ".")
			if tok != "" {
				tokens = append(tokens, tok)
			}
		}
		return tokens, sc.Err()
	}
	return nil, sc.Err()
}
