package main

import (
	"database/sql"
	"flag"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/RationalRouter/n5e/internal/i18n"
)

// normalize-glossary repairs translated rows that DeepL rendered with a
// different term than glossary.md's approved one.
//
// This exists because a glossary entry only constrains translations made
// AFTER it was added. The five base elemental Releases (Fire/Water/Earth/
// Wind/Lightning) got their standalone glossary rows late -- see
// glossary.md's "'Elemento X' bug" section -- so the bulk of the content
// translated before that shipped with whatever term DeepL picked per table,
// and for Portuguese that turned into eleven competing renderings of
// "Lightning Release" alone ("Técnica do Relâmpago", "Técnica da Descarga
// de Relâmpagos", "Descarga de Relâmpago", ...). A glossary entry cannot
// retroactively fix that; this pass can, and is re-runnable after any
// future `translate -force` run re-introduces the same drift.
//
// It is deliberately narrow in two ways:
//
//   - It only touches rows `verify` already flags (the same
//     GlossaryChecker.MissingTerms call), and only for the specific terms
//     that row is flagged for. A row whose Portuguese already contains the
//     approved rendering is never rewritten, so re-running is a no-op.
//   - It only handles terms in elementReleaseTerms. Rewriting arbitrary
//     glossary terms in free prose is not safe to do mechanically -- many
//     are ordinary words pinned to a game-specific sense (see
//     MissingTerms' own "accepted noise source" note), and replacing those
//     would corrupt correct prose. The Release terms are unambiguous
//     multi-word proper mechanics names, which is what makes them
//     mechanically rewritable at all.
//
// Dry-run by default; -apply writes.

// elementReleaseTerms are the glossary source terms this pass will rewrite.
// Every one is a multi-word game-mechanic name with no ordinary-prose
// homograph, which is the property that makes a mechanical rewrite safe.
var elementReleaseTerms = map[string]bool{
	"Fire Release":      true,
	"Water Release":     true,
	"Earth Release":     true,
	"Wind Release":      true,
	"Lightning Release": true,
}

// wrongRenderingRe matches the drifted renderings of one element in target
// prose, built from the element's own head noun (e.g. "Fogo", "Água").
// The alternation covers every wrong family actually observed in the data:
// "Técnica do Fogo", "Liberação de Fogo", "Lançamento de Explosão"-style
// "Lançamento", the "Elemento Fogo" family, and the partially-corrected
// "Técnica da Descarga do Relâmpago" (canonical term buried inside a wrong
// wrapper). Matching the wrapper as well as the bare "Elemento X" form is
// what keeps "Técnica da Descarga de Relâmpagos" from being left as a
// double prefix once the inner term is already right.
// articlePattern matches a Romance contracted/plain article in either
// target language: Portuguese de/da/do/das/dos, Spanish de/del/de la/de los/
// de las. Kept as one shared alternation rather than per-locale so a mixed
// row (they exist -- a Spanish row that kept a Portuguese fragment, and
// vice versa) still normalizes.
const articlePattern = `(?:d[oae]s?|del|de la|de los|de las)`

// governedArticle is what replaces a consumed article once the term it
// governed becomes the approved rendering. Both languages' approved head
// noun is the feminine singular "descarga", so this is agreement with
// "descarga", not with the element noun.
var governedArticle = map[string]string{"pt": "da ", "es": "de la "}

func wrongRenderingRe(headNoun string) *regexp.Regexp {
	noun := regexp.QuoteMeta(headNoun)
	// Optional trailing "s" tolerates the observed plural drift
	// ("Relâmpagos", "Rayos").
	art := `(?:\s+` + articlePattern + `)?`
	wrapper := `(?:T[ée]cnica|Libera[çc][ãa]o|Liberaci[óo]n|Lan[çc]amento|Habilidade)` + art
	descarga := `(?:\s+Descarga` + art + `)?`
	elemento := `(?:\s+[Ee]lemento)?`
	// Group 1 is a leading article the wrong rendering was governed by
	// ("chakra do Elemento Fogo", "chakra del Elemento Fuego"). It has to
	// be consumed and re-agreed -- see governedArticle.
	return regexp.MustCompile(`(?i)(` + articlePattern + `\s+)?(?:` + wrapper + descarga + elemento + `|Descarga` + art + elemento + `|[Ee]lemento)` + art + `\s+` + noun + `s?`)
}

// applyRendering swaps every drifted rendering in s for the approved one,
// preserving two things the glossary value itself cannot carry: the
// capitalization of the position it lands in (glossary.md spells these
// lowercase for mid-sentence prose, but the overwhelmingly common site is
// a jutsu name's own leading "Descarga da Terra: X"), and agreement of a
// preceding article.
func applyRendering(re *regexp.Regexp, approved, article, s string) string {
	return re.ReplaceAllStringFunc(s, func(match string) string {
		out := approved
		if m := re.FindStringSubmatch(match); len(m) > 1 && m[1] != "" {
			out = article + out
		}
		if r := []rune(match); len(r) > 0 && r[0] == []rune(strings.ToUpper(string(r[0])))[0] &&
			r[0] != []rune(strings.ToLower(string(r[0])))[0] {
			o := []rune(out)
			o[0] = []rune(strings.ToUpper(string(o[0])))[0]
			out = string(o)
		}
		return out
	})
}

// elementHeadNoun is the element's own noun inside the approved rendering
// ("descarga da Terra" -> "Terra"), used to build wrongRenderingRe. Derived
// from the glossary value itself rather than hardcoded, so a future change
// to the approved wording only has to happen in glossary.md.
func elementHeadNoun(approved string) string {
	f := strings.Fields(approved)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

func cmdNormalizeGlossary(args []string) error {
	fs := flag.NewFlagSet("normalize-glossary", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to rules.db")
	locale := fs.String("locale", "", "locale code (es, pt)")
	table := fs.String("table", "", "restrict to one table (default: every translatable table)")
	glossaryPath := fs.String("glossary", "internal/i18n/glossary.md", "path to glossary.md")
	apply := fs.Bool("apply", false, "write the rewrites (default: dry run, print them)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *locale == "" {
		return fmt.Errorf("usage: n5e-i18n normalize-glossary -db <path> -locale <code> [-table <name>] [-apply]")
	}
	col, ok := glossaryColumn[*locale]
	if !ok {
		return fmt.Errorf("no glossary column for locale %q", *locale)
	}
	article, ok := governedArticle[*locale]
	if !ok {
		return fmt.Errorf("no governed-article rule for locale %q -- add one before normalizing it", *locale)
	}

	db, err := openRulesDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	terms, err := i18n.ParseGlossaryTables(*glossaryPath, col)
	if err != nil {
		return err
	}
	checker := i18n.NewGlossaryChecker(terms)

	// Precompile one rewrite per handled term.
	type rule struct {
		approved string // "descarga da Terra"
		wrong    *regexp.Regexp
	}
	rules := map[string]rule{}
	for source, approved := range terms {
		if !elementReleaseTerms[source] {
			continue
		}
		noun := elementHeadNoun(approved)
		if noun == "" {
			continue
		}
		rules[source] = rule{approved: approved, wrong: wrongRenderingRe(noun)}
	}
	if len(rules) != len(elementReleaseTerms) {
		return fmt.Errorf("glossary.md is missing a %s rendering for one of the element Release terms (found %d of %d) -- fix glossary.md before normalizing",
			col, len(rules), len(elementReleaseTerms))
	}

	tables := []string{*table}
	if *table == "" {
		tables = tables[:0]
		for t := range i18n.TranslatableFields {
			tables = append(tables, t)
		}
		sort.Strings(tables)
	}

	var rewritten, unfixable int
	for _, t := range tables {
		rowsData, err := loadEnglishValues(db, t, "")
		if err != nil {
			return err
		}
		translated, err := loadTranslatedValues(db, t, *locale)
		if err != nil {
			return err
		}
		for _, rv := range rowsData {
			for field, enText := range rv.Fields {
				target, ok := translated[rv.Key+"\x00"+field]
				if !ok {
					continue
				}
				missing := checker.MissingTerms(enText, target)
				if len(missing) == 0 {
					continue
				}
				updated := target
				for _, term := range missing {
					r, ok := rules[term]
					if !ok {
						continue // not a term this pass handles -- left for a human
					}
					updated = applyRendering(r.wrong, r.approved, article, updated)
				}
				if updated == target {
					continue
				}
				// Only accept a rewrite that actually clears the findings
				// it was made for. A row whose drift this pass cannot
				// recognize is left exactly as it was rather than
				// half-rewritten.
				still := checker.MissingTerms(enText, updated)
				stillHandled := false
				for _, term := range still {
					if rules[term].wrong != nil {
						stillHandled = true
					}
				}
				if stillHandled {
					unfixable++
					fmt.Printf("SKIP  %s/%s.%s: rewrite did not clear %v\n", t, rv.Key, field, still)
					continue
				}
				rewritten++
				fmt.Printf("FIX   %s/%s.%s\n  - %s\n  + %s\n", t, rv.Key, field, clip(target), clip(updated))
				if *apply {
					if err := updateTranslation(db, t, rv.Key, *locale, field, updated); err != nil {
						return err
					}
				}
			}
		}
	}

	mode := "dry run (re-run with -apply to write)"
	if *apply {
		mode = "applied"
	}
	fmt.Printf("\nn5e-i18n normalize-glossary: locale=%s rewritten=%d skipped=%d -- %s\n", *locale, rewritten, unfixable, mode)
	return nil
}

// updateTranslation writes one normalized value back. status is deliberately
// left alone: a machine row stays machine (this is a term substitution, not
// a human review), and a reviewed row stays reviewed.
func updateTranslation(db *sql.DB, table, slug, locale, field, value string) error {
	_, err := db.Exec(`UPDATE content_translations SET value = ?, updated_at = datetime('now')
	                   WHERE table_name = ? AND slug = ? AND locale = ? AND field = ?`,
		value, table, slug, locale, field)
	return err
}

func clip(s string) string {
	const max = 160
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
