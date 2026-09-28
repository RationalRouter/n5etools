// n5e-i18n is the maintainer CLI for translating both the UI string catalog
// and game content into another locale via the DeepL API, per
// internal/i18n/translation_methodology.md:
//
//	n5e-i18n export -db <rules.db> -locale es -table <table> [-out <file.jsonl>] [-force]
//	    Dump every translatable field (internal/i18n.TranslatableFields)
//	    that has no fresh translation yet for the given locale/table to a
//	    JSONL file, one field per line: {"table","slug","field","en_hash",
//	    "en_text"}. "Fresh" means a content_translations row already exists
//	    whose en_hash matches the current English text -- a field whose
//	    English changed since it was translated is re-exported
//	    automatically, no separate staleness flag needed. -force re-exports
//	    every translatable field regardless of existing translations, for
//	    redoing a table from scratch. Opening -db applies the rules schema
//	    migrations first, so this works against out/rules.db (the canonical
//	    maintainer copy, which the app itself never opens) as well as a
//	    live dist/rules.db copy.
//
//	n5e-i18n translate -in <export.jsonl> -out <translated.jsonl> -locale es [-pro]
//	    Translates every record's en_text via the DeepL API, constrained by
//	    a glossary built from internal/i18n/glossary.md, and writes an
//	    import-ready JSONL file (en_text replaced by value). The
//	    content-table analogue of catalog-translate below -- same DeepL
//	    client, glossary, and placeholder-protection logic, applied to
//	    export's JSONL shape instead of the flat catalog JSON. Requires
//	    DEEPL_API_KEY in the environment.
//
//	n5e-i18n import -db <rules.db> -locale es -in <file.jsonl> [-status machine|reviewed]
//	    Load a translated JSONL file (same shape as export's output, but
//	    with "en_text" replaced by "value" holding the translated text)
//	    into content_translations. Upserts by (table, slug, locale, field);
//	    en_hash is taken from the file itself (what the translation was
//	    actually made against), not recomputed from the live English
//	    column -- so a translation imported against since-edited English
//	    correctly reports as stale on the next report/export, rather than
//	    silently looking fresh.
//
//	n5e-i18n report -db <rules.db> -locale es [-table <table>]
//	    Per-table field-instance counts: total translatable, translated,
//	    stale (en_hash no longer matches current English), reviewed.
//	    Omit -table for every table in TranslatableFields.
//
//	n5e-i18n catalog-translate -in locales/en.json -out locales/es.json -locale es [-force] [-pro]
//	    Translates the UI string catalog (internal/i18n/locales/en.json)
//	    into another locale via the DeepL API, constrained by a glossary
//	    built from internal/i18n/glossary.md so proper nouns and approved
//	    term renderings aren't touched. Requires DEEPL_API_KEY in the
//	    environment. Skips keys already present in -out unless -force.
//
//	n5e-i18n audit [-root .] [-fix] [-seed-baseline]
//	    Compiled coverage sweep for player-facing text gaps (see
//	    internal/i18n/audit.go): hardcoded literals in suspect struct fields
//	    that never touch TranslatableFields/the UI catalog, and files that
//	    read a TranslatableFields table but never overlay it via
//	    i18n.Row/Batch. Reports every finding not already in
//	    internal/i18n/audit_baseline.json; exits non-zero if any remain.
//	    -fix additionally auto-wires the safe subset (a known display-only
//	    field with a translator already in scope) into the UI catalog and
//	    rewrites the call site, so those gaps get translated on the same
//	    run instead of blocking it. -seed-baseline overwrites the baseline
//	    with every current finding instead of reporting/fixing anything --
//	    used once to bootstrap the gate, or deliberately after a batch of
//	    findings has been reviewed and accepted rather than fixed.
//
//	n5e-i18n check-drive [-state <file>]
//	    Compares every official sourcebook's live Drive Last-Modified time
//	    against the last time translate-book actually ran for that
//	    book+locale (tracked in -state, a small maintainer-only JSON file,
//	    default internal/i18n/translation-state.json). Prints which
//	    book+locale pairs need a translation refresh; makes no changes.
//
//	n5e-i18n translate-book <slug> -db <rules.db> -locale es [-glossary <file>] [-pending <file>] [-status machine|reviewed] [-force] [-pro]
//	    Chains export -> glossary-gate -> DeepL -> import for every
//	    translatable table (internal/i18n.TranslatableFields), scoped to one
//	    sourcebook's rows (see internal/sources.OfficialBooks for slugs).
//	    Before sending a field to DeepL, scans its English text for terms
//	    FindUnglossaried can't resolve against glossary.md or the DB's own
//	    known names -- an unresolved row is held back in -pending (default
//	    internal/i18n/glossary-pending.json) instead of being translated or
//	    blocking the rest of the book, and is re-checked (and cleared) on
//	    every subsequent run once glossary.md covers its terms. Updates
//	    -state on success, so a later check-drive run sees this book+locale
//	    as current. Requires DEEPL_API_KEY in the environment.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/RationalRouter/n5e/internal/i18n"
	"github.com/RationalRouter/n5e/internal/schema"
	"github.com/RationalRouter/n5e/internal/statlines"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: n5e-i18n export|translate|import|report|catalog-translate|audit|verify|normalize-glossary|check-drive|translate-book ...")
	}
	switch args[0] {
	case "export":
		return cmdExport(args[1:])
	case "translate":
		return cmdTranslate(args[1:])
	case "import":
		return cmdImport(args[1:])
	case "report":
		return cmdReport(args[1:])
	case "catalog-translate":
		return cmdCatalogTranslate(args[1:])
	case "audit":
		return cmdAudit(args[1:])
	case "verify":
		return cmdVerify(args[1:])
	case "translate-book":
		return cmdTranslateBook(args[1:])
	case "check-drive":
		return cmdCheckDrive(args[1:])
	case "normalize-glossary":
		return cmdNormalizeGlossary(args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func cmdAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	root := fs.String("root", ".", "repo root to sweep")
	fix := fs.Bool("fix", false, "auto-wire the safe subset of findings into the UI catalog")
	seed := fs.Bool("seed-baseline", false, "overwrite audit_baseline.json with every current finding")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *seed {
		n, err := i18n.WriteAuditBaseline(*root)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "audit: baseline written with %d finding(s)\n", n)
		return nil
	}

	result, err := i18n.RunAudit(*root, *fix)
	if err != nil {
		return err
	}

	for _, v := range result.Fixed {
		fmt.Fprintf(os.Stderr, "audit: fixed %s:%d field %s -> catalog key %q\n", v.File, v.Line, v.Field, v.Key)
	}
	if len(result.New) == 0 {
		fmt.Fprintln(os.Stderr, "audit: no new coverage gaps")
		return nil
	}
	fmt.Fprintf(os.Stderr, "audit: %d new coverage gap(s) not in audit_baseline.json:\n", len(result.New))
	for _, v := range result.New {
		switch v.Kind {
		case i18n.AuditMissingOverlay:
			fmt.Fprintf(os.Stderr, "ERROR: %s: [%s] table %q: %s\n", v.File, v.Kind, v.Table, v.Text)
		default:
			fmt.Fprintf(os.Stderr, "ERROR: %s:%d: [%s] field %s: %q\n", v.File, v.Line, v.Kind, v.Field, v.Text)
		}
	}
	return fmt.Errorf("%d new i18n coverage gap(s) -- see errors above; fix them, mark them // i18n:ignore, or run -seed-baseline if deliberately accepted", len(result.New))
}

// openRulesDB opens a rules.db copy and brings it up to date with the rules
// migration series -- content_translations (migration 0069) only exists on
// a copy that's actually been opened by the running app (which applies
// migrations on every startup, see cmd/n5e/rulesdb.go) or by n5e-ingest.
// The canonical maintainer copy (out/rules.db) is built by n5e-ingest and
// never opened by the app itself, so without this it would be missing the
// table entirely. Safe to call on any rules.db copy: migrations are
// idempotent and skip anything already applied.
func openRulesDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := schema.Apply(db, schema.Rules); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying rules schema migrations to %s: %w", path, err)
	}
	return db, nil
}

// deeplTargetLang and glossaryColumn map this project's own locale codes
// (matching internal/i18n.SupportedLocales) to what DeepL and glossary.md
// respectively call that language. Extend both together when a new locale
// is added -- glossary.md needs a matching "Xxx" column in its term tables
// before that locale's glossary carries any real entries.
var deeplTargetLang = map[string]string{
	"es": "ES",
	"pt": "PT-BR",
}

var glossaryColumn = map[string]string{
	"es": "Spanish",
	"pt": "Portuguese",
}

// glossaryTargetLang strips a regional variant (e.g. "PT-BR") down to the
// base language code DeepL's glossary endpoint requires -- it rejects
// variant codes outright even though the translate endpoint requires one.
// A glossary built against the base code still applies when translating
// with the regional variant, since both share the same base language.
func glossaryTargetLang(targetLang string) string {
	base, _, _ := strings.Cut(targetLang, "-")
	return base
}

func cmdCatalogTranslate(args []string) error {
	fs := flag.NewFlagSet("catalog-translate", flag.ContinueOnError)
	in := fs.String("in", "", "source catalog JSON, e.g. internal/i18n/locales/en.json")
	out := fs.String("out", "", "output catalog JSON, e.g. internal/i18n/locales/es.json")
	locale := fs.String("locale", "", "target locale code, e.g. es")
	glossaryPath := fs.String("glossary", "internal/i18n/glossary.md", "glossary markdown file")
	force := fs.Bool("force", false, "retranslate every key, even ones already present in -out")
	pro := fs.Bool("pro", false, "use the DeepL Pro API host instead of the Free one")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" || *locale == "" {
		return fmt.Errorf("usage: n5e-i18n catalog-translate -in <en.json> -out <locale.json> -locale <code> [-force] [-pro]")
	}
	targetLang, ok := deeplTargetLang[*locale]
	if !ok {
		return fmt.Errorf("no DeepL target_lang mapping for locale %q -- add one to deeplTargetLang", *locale)
	}
	apiKey := os.Getenv("DEEPL_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("DEEPL_API_KEY is not set")
	}

	src, err := readCatalog(*in)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *in, err)
	}
	existing := map[string]string{}
	if !*force {
		existing, err = readCatalog(*out)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", *out, err)
		}
		if existing == nil {
			existing = map[string]string{}
		}
	}

	var pendingKeys []string
	for key := range src {
		if _, done := existing[key]; !done {
			pendingKeys = append(pendingKeys, key)
		}
	}
	sort.Strings(pendingKeys)
	if len(pendingKeys) == 0 {
		fmt.Fprintln(os.Stderr, "catalog-translate: nothing to do, every key already present in", *out)
		return nil
	}

	ctx := context.Background()
	client := i18n.NewDeepLClient(apiKey, *pro)

	glossaryID, err := setUpGlossary(ctx, client, "catalog-translate", "n5e-catalog", *locale, *glossaryPath, targetLang)
	if err != nil {
		return err
	}

	protected := make([]string, len(pendingKeys))
	for i, key := range pendingKeys {
		protected[i] = i18n.ProtectPlaceholders(src[key])
	}
	translated, translateErr := client.Translate(ctx, protected, targetLang, glossaryID)
	// Persist whatever batches did complete even on error -- DeepL bills
	// each completed batch regardless of whether a later one in this call
	// fails, so writing only the partial result still saves the spent
	// quota's work instead of discarding it.
	for i := 0; i < len(translated); i++ {
		existing[pendingKeys[i]] = i18n.RestorePlaceholders(translated[i])
	}
	if err := writeCatalog(*out, existing); err != nil {
		return fmt.Errorf("writing %s: %w", *out, err)
	}
	if translateErr != nil {
		return fmt.Errorf("translating (%d/%d key(s) completed and saved to %s before the error): %w", len(translated), len(pendingKeys), *out, translateErr)
	}
	fmt.Fprintf(os.Stderr, "catalog-translate: %d key(s) translated, %d total written to %s\n", len(pendingKeys), len(existing), *out)
	return nil
}

func readCatalog(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cat map[string]string
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cat, nil
}

func writeCatalog(path string, cat map[string]string) error {
	keys := make([]string, 0, len(cat))
	for k := range cat {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("{\n")
	for i, k := range keys {
		kb, _ := json.Marshal(k)
		vb, _ := json.Marshal(cat[k])
		b.WriteString("  ")
		b.Write(kb)
		b.WriteString(": ")
		b.Write(vb)
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// setUpGlossary parses glossary.md for locale, checks DeepL actually
// supports a glossary for this language pair (logging and continuing
// unconstrained if not, rather than the old silent-on-parse-error-only
// warning), and returns the id of a persistent glossary
// (DeepLClient.GetOrCreatePersistentGlossary) reused across runs when
// glossary.md hasn't changed for this name/target combination -- shared by
// cmdCatalogTranslate and cmdTranslate so both use identical glossary
// handling. cmdLabel is just the calling subcommand's name, for log
// prefixing.
func setUpGlossary(ctx context.Context, client *i18n.DeepLClient, cmdLabel, namePrefix, locale, glossaryPath, targetLang string) (string, error) {
	terms, err := i18n.ParseGlossaryTables(glossaryPath, glossaryColumn[locale])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: warning: could not parse glossary %s: %v\n", cmdLabel, glossaryPath, err)
		return "", nil
	}
	if len(terms) == 0 {
		return "", nil
	}
	glossaryTarget := glossaryTargetLang(targetLang)
	supported, err := client.GlossarySupported(ctx, "EN", glossaryTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: warning: could not check glossary language-pair support: %v\n", cmdLabel, err)
	} else if !supported {
		fmt.Fprintf(os.Stderr, "ERROR: %s: DeepL glossaries do not support EN->%s -- translating this locale unconstrained by the glossary (proper nouns/approved terms may drift)\n", cmdLabel, glossaryTarget)
		return "", nil
	}
	glossaryID, err := client.GetOrCreatePersistentGlossary(ctx, namePrefix+"-"+locale, glossaryTarget, terms)
	if err != nil {
		return "", fmt.Errorf("setting up glossary: %w", err)
	}
	fmt.Fprintf(os.Stderr, "%s: glossary with %d term(s) ready (id %s)\n", cmdLabel, len(terms), glossaryID)
	return glossaryID, nil
}

// cmdVerify runs the language-agnostic post-translation QA checks
// (internal/i18n/verify.go) against every translated field for a locale and
// writes a consolidated, human-readable report -- see this file's package
// doc comment and translation_methodology.md's "Automated coverage
// pipeline" section. Unlike `report -strict` (a hard gate on missing/stale
// coverage), verify's findings are review signals, not failures, with one
// exception: a placeholder-mismatch is a real correctness bug (it can crash
// a Sprintf call at render time), so that class alone makes verify exit
// non-zero.
func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to read from")
	locale := fs.String("locale", "", "target locale, e.g. es")
	table := fs.String("table", "", "limit to one table (default: every table in TranslatableFields)")
	glossaryPath := fs.String("glossary", "internal/i18n/glossary.md", "glossary markdown file")
	out := fs.String("out", "", "report file to write (default: stdout)")
	backtranslate := fs.Bool("backtranslate", true, "translate target text back to English via DeepL and flag rows whose meaning drifted")
	pro := fs.Bool("pro", false, "use the DeepL Pro API host instead of the Free one")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *locale == "" {
		return fmt.Errorf("usage: n5e-i18n verify -db <path> -locale <code> [-table <name>] [-out <file>] [-backtranslate=false]")
	}

	db, err := openRulesDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	tables := []string{*table}
	if *table == "" {
		tables = tables[:0]
		for t := range i18n.TranslatableFields {
			tables = append(tables, t)
		}
		sort.Strings(tables)
	}

	glossaryTerms, _ := i18n.ParseGlossaryTables(*glossaryPath, glossaryColumn[*locale])
	glossaryChecker := i18n.NewGlossaryChecker(glossaryTerms)

	type rowRef struct {
		table, slug, field, en, target string
	}
	var refs []rowRef
	var findings []i18n.VerifyFinding

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
					continue // not yet translated -- report -strict already covers missing coverage
				}
				refs = append(refs, rowRef{t, rv.Key, field, enText, target})

				if ok, _, _ := i18n.PlaceholdersMatch(enText, target); !ok {
					findings = append(findings, i18n.VerifyFinding{Table: t, Slug: rv.Key, Field: field, Kind: "placeholder-mismatch", Detail: "placeholder tokens differ between English and translation"})
				}
				if i18n.SuspiciouslyIdentical(enText, target) {
					findings = append(findings, i18n.VerifyFinding{Table: t, Slug: rv.Key, Field: field, Kind: "suspiciously-identical", Detail: "translated value is byte-identical to English"})
				}
				if ratio, outlier := i18n.LengthRatioOutlier(enText, target); outlier {
					findings = append(findings, i18n.VerifyFinding{Table: t, Slug: rv.Key, Field: field, Kind: "length-ratio-outlier", Detail: fmt.Sprintf("length ratio %.2f (target/English)", ratio)})
				}
				if missing := glossaryChecker.MissingTerms(enText, target); len(missing) > 0 {
					findings = append(findings, i18n.VerifyFinding{Table: t, Slug: rv.Key, Field: field, Kind: "missing-glossary-term", Detail: "expected glossary rendering not found for: " + strings.Join(missing, ", ")})
				}
			}
		}
	}

	if *backtranslate && len(refs) > 0 {
		apiKey := os.Getenv("DEEPL_API_KEY")
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "verify: warning: DEEPL_API_KEY not set, skipping back-translation check")
		} else {
			client := i18n.NewDeepLClient(apiKey, *pro)
			ctx := context.Background()
			sourceLang := glossaryTargetLang(deeplTargetLang[*locale])
			texts := make([]string, len(refs))
			for i, r := range refs {
				texts[i] = i18n.ProtectPlaceholders(r.target)
			}
			backTranslated, btErr := client.TranslateFrom(ctx, texts, sourceLang, "EN", "")
			for i := 0; i < len(backTranslated); i++ {
				sim := i18n.TextSimilarity(refs[i].en, i18n.RestorePlaceholders(backTranslated[i]))
				if sim < i18n.BackTranslationDriftThreshold {
					findings = append(findings, i18n.VerifyFinding{
						Table: refs[i].table, Slug: refs[i].slug, Field: refs[i].field,
						Kind:   "backtranslation-drift",
						Detail: fmt.Sprintf("round-trip similarity %.2f (back-translated: %q)", sim, i18n.RestorePlaceholders(backTranslated[i])),
					})
				}
			}
			if btErr != nil {
				fmt.Fprintf(os.Stderr, "verify: warning: back-translation incomplete (%d/%d checked): %v\n", len(backTranslated), len(refs), btErr)
			}
		}
	}

	w := io.Writer(os.Stdout)
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	hasHardFailure := false
	fmt.Fprintf(w, "n5e-i18n verify: locale=%s tables=%d rows-checked=%d findings=%d\n\n", *locale, len(tables), len(refs), len(findings))
	if len(findings) == 0 {
		fmt.Fprintln(w, "no findings")
	}
	for _, f := range findings {
		prefix := "WARN"
		if f.Kind == "placeholder-mismatch" {
			prefix = "ERROR"
			hasHardFailure = true
		}
		fmt.Fprintf(w, "%s: %s/%s.%s [%s]: %s\n", prefix, f.Table, f.Slug, f.Field, f.Kind, f.Detail)
	}

	if hasHardFailure {
		return fmt.Errorf("verify: %d placeholder-mismatch finding(s) -- see report", countKind(findings, "placeholder-mismatch"))
	}
	return nil
}

func countKind(findings []i18n.VerifyFinding, kind string) int {
	n := 0
	for _, f := range findings {
		if f.Kind == kind {
			n++
		}
	}
	return n
}

// loadTranslatedValues returns the current content_translations values for
// one table/locale, keyed by "slug\x00field" -> translated text. Unlike
// existingTranslations (which only reads en_hash/status for staleness
// bookkeeping), this reads the actual translated value -- what verify
// checks against.
func loadTranslatedValues(db *sql.DB, table, locale string) (map[string]string, error) {
	rows, err := db.Query(
		`SELECT slug, field, value FROM content_translations WHERE table_name = ? AND locale = ?`,
		table, locale,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var slug, field, value string
		if err := rows.Scan(&slug, &field, &value); err != nil {
			return nil, err
		}
		out[slug+"\x00"+field] = value
	}
	return out, rows.Err()
}

// exportRecord is one translatable field's current state, as written by
// export and read back (with EnText replaced by a translated Value) by
// import.
type exportRecord struct {
	Table  string `json:"table"`
	Slug   string `json:"slug"`
	Field  string `json:"field"`
	EnHash string `json:"en_hash"`
	EnText string `json:"en_text"`
}

type importRecord struct {
	Table  string `json:"table"`
	Slug   string `json:"slug"`
	Field  string `json:"field"`
	EnHash string `json:"en_hash"`
	Value  string `json:"value"`
}

// cmdTranslate reads an export-produced JSONL file and translates every
// record's EnText via the DeepL API, writing an import-ready JSONL file
// (same records, EnText replaced by Value) -- the content-table analogue of
// cmdCatalogTranslate, sharing the same glossary/placeholder-protection
// machinery so game content and UI strings are translated identically.
func cmdTranslate(args []string) error {
	fs := flag.NewFlagSet("translate", flag.ContinueOnError)
	in := fs.String("in", "", "exported JSONL file (from n5e-i18n export)")
	out := fs.String("out", "", "translated JSONL file to write (import-ready)")
	locale := fs.String("locale", "", "target locale code, e.g. es")
	glossaryPath := fs.String("glossary", "internal/i18n/glossary.md", "glossary markdown file")
	pro := fs.Bool("pro", false, "use the DeepL Pro API host instead of the Free one")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" || *locale == "" {
		return fmt.Errorf("usage: n5e-i18n translate -in <export.jsonl> -out <translated.jsonl> -locale <code> [-pro]")
	}
	targetLang, ok := deeplTargetLang[*locale]
	if !ok {
		return fmt.Errorf("no DeepL target_lang mapping for locale %q -- add one to deeplTargetLang", *locale)
	}
	apiKey := os.Getenv("DEEPL_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("DEEPL_API_KEY is not set")
	}

	records, err := readExportRecords(*in)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *in, err)
	}
	if len(records) == 0 {
		fmt.Fprintln(os.Stderr, "translate: nothing to do,", *in, "is empty")
		return nil
	}

	ctx := context.Background()
	client := i18n.NewDeepLClient(apiKey, *pro)

	glossaryID, err := setUpGlossary(ctx, client, "translate", "n5e-content", *locale, *glossaryPath, targetLang)
	if err != nil {
		return err
	}

	protected := make([]string, len(records))
	for i, rec := range records {
		protected[i] = i18n.ProtectPlaceholders(rec.EnText)
	}
	translated, translateErr := client.Translate(ctx, protected, targetLang, glossaryID)
	// Write whatever batches did complete even on error -- DeepL bills
	// each completed batch regardless of whether a later one in this call
	// fails, so a partial output file still saves the spent quota's work
	// (and can be re-imported directly) instead of discarding it.
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	enc := json.NewEncoder(bw)
	for i := 0; i < len(translated); i++ {
		rec := records[i]
		if err := enc.Encode(importRecord{
			Table:  rec.Table,
			Slug:   rec.Slug,
			Field:  rec.Field,
			EnHash: rec.EnHash,
			Value:  i18n.RestorePlaceholders(translated[i]),
		}); err != nil {
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if translateErr != nil {
		return fmt.Errorf("translating (%d/%d record(s) completed and saved to %s before the error): %w", len(translated), len(records), *out, translateErr)
	}
	fmt.Fprintf(os.Stderr, "translate: %d record(s) translated, written to %s\n", len(records), *out)
	return nil
}

func readExportRecords(path string) ([]exportRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var out []exportRecord
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec exportRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("parse line %d: %w", len(out)+1, err)
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// rowValues is one content-table row's key (already joined per
// i18n.KeyColumnsFor) plus its translatable field values (English column
// text, may be absent/empty for a field that's genuinely blank in that row).
type rowValues struct {
	Key    string
	Fields map[string]string
}

// loadEnglishValues reads every row of table, projecting the key column(s)
// (per i18n.KeyColumnsFor) and every field in i18n.TranslatableFields[table].
// Shared by export (to know what needs translating) and report (to detect
// staleness against the live English text). If bookSlug is non-empty and
// the table has its own source_book column, rows are scoped to that book
// (see translate-book); tables with no source_book column of their own
// (mostly child tables of a book-scoped parent -- class_option_entries,
// clan_traits, and similar) fall back to every row, since resolving them
// would need a per-table join map this command doesn't otherwise need.
func loadEnglishValues(db *sql.DB, table, bookSlug string) ([]rowValues, error) {
	fields, ok := i18n.TranslatableFields[table]
	if !ok || len(fields) == 0 {
		return nil, fmt.Errorf("table %q has no translatable fields registered in internal/i18n.TranslatableFields", table)
	}
	keyCols := i18n.KeyColumnsFor(table)

	cols := append(append([]string{}, keyCols...), fields...)
	query := "SELECT " + strings.Join(quoteIdents(cols), ", ") + " FROM " + quoteIdent(table)
	var args []any
	if bookSlug != "" && tableHasColumn(db, table, "source_book") {
		query += " WHERE source_book = ?"
		args = append(args, bookSlug)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", table, err)
	}
	defer rows.Close()

	var out []rowValues
	for rows.Next() {
		scanTargets := make([]any, len(cols))
		vals := make([]sql.NullString, len(cols))
		for i := range vals {
			scanTargets[i] = &vals[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, fmt.Errorf("scan %s row: %w", table, err)
		}
		keyParts := make([]string, len(keyCols))
		for i := range keyCols {
			keyParts[i] = vals[i].String
		}
		rv := rowValues{Key: strings.Join(keyParts, i18n.KeySeparator), Fields: map[string]string{}}
		for i, f := range fields {
			v := vals[len(keyCols)+i]
			if v.Valid && strings.TrimSpace(v.String) != "" {
				rv.Fields[f] = normalizeForTranslation(table, f, v.String)
			}
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

// normalizeForTranslation strips a machine-parsed stat line (Cost/Drain/
// Keyword/Prerequisite) off a class_option_entries/class_options
// description BEFORE it is ever exported for translation, so DeepL never
// sees it at all and no runtime code ever has to guess where a translated
// stat line ends -- see internal/statlines' header doc. This is the single
// choke point both cmdExport (what gets hashed and sent to DeepL) and
// cmdReport (what staleness is compared against) read through, so both
// always agree on what "the English text" is for these two fields. Every
// other table/field is returned unchanged -- the stat-line shape only ever
// appears in these two tables, and the vast majority of their own rows
// carry no stat line either (statlines.StripForTranslation no-ops for
// ordinary prose).
func normalizeForTranslation(table, field, text string) string {
	if field != "description" {
		return text
	}
	if table != "class_option_entries" && table != "class_options" {
		return text
	}
	return statlines.StripForTranslation(text)
}

// tableHasColumn reports whether table has a column named col, via
// PRAGMA table_info -- used to decide whether a book-slug filter can be
// applied directly, since not every content table carries its own
// source_book column.
func tableHasColumn(db *sql.DB, table, col string) bool {
	rows, err := db.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return false
	}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return false
		}
		for i, c := range cols {
			if c == "name" {
				if name, ok := vals[i].(string); ok && name == col {
					return true
				}
			}
		}
	}
	return false
}

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func quoteIdents(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = quoteIdent(s)
	}
	return out
}

// existingTranslations returns the current content_translations rows for
// one table/locale, keyed by "slug\x00field" -> {value, status, en_hash}.
func existingTranslations(db *sql.DB, table, locale string) (map[string]struct{ EnHash, Status string }, error) {
	rows, err := db.Query(
		`SELECT slug, field, en_hash, status FROM content_translations WHERE table_name = ? AND locale = ?`,
		table, locale,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{ EnHash, Status string }{}
	for rows.Next() {
		var slug, field, enHash, status string
		if err := rows.Scan(&slug, &field, &enHash, &status); err != nil {
			return nil, err
		}
		out[slug+"\x00"+field] = struct{ EnHash, Status string }{enHash, status}
	}
	return out, rows.Err()
}

func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to read from")
	locale := fs.String("locale", "", "target locale, e.g. es")
	table := fs.String("table", "", "content table to export (see internal/i18n.TranslatableFields)")
	book := fs.String("book", "", "limit to rows whose source_book matches this slug (default: every book)")
	out := fs.String("out", "", "output JSONL path (default: stdout)")
	force := fs.Bool("force", false, "re-export every translatable field, even ones already translated and fresh")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *locale == "" || *table == "" {
		return fmt.Errorf("usage: n5e-i18n export -db <path> -locale <code> -table <name> [-book <slug>] [-out <file>] [-force]")
	}

	db, err := openRulesDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	rowsData, err := loadEnglishValues(db, *table, *book)
	if err != nil {
		return err
	}
	existing, err := existingTranslations(db, *table, *locale)
	if err != nil {
		return err
	}

	w := io.Writer(os.Stdout)
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	enc := json.NewEncoder(bw)
	written := 0
	for _, rv := range rowsData {
		fields := make([]string, 0, len(rv.Fields))
		for f := range rv.Fields {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		for _, field := range fields {
			enText := rv.Fields[field]
			enHash := hashText(enText)
			if !*force {
				if prior, ok := existing[rv.Key+"\x00"+field]; ok && prior.EnHash == enHash {
					continue // already translated and still matches current English
				}
			}
			if err := enc.Encode(exportRecord{
				Table: *table, Slug: rv.Key, Field: field, EnHash: enHash, EnText: enText,
			}); err != nil {
				return err
			}
			written++
		}
	}
	fmt.Fprintf(os.Stderr, "export %s/%s: %d field(s) written\n", *table, *locale, written)
	return nil
}

func cmdImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to write into")
	locale := fs.String("locale", "", "target locale, e.g. es")
	in := fs.String("in", "", "input JSONL path (translated records)")
	status := fs.String("status", "machine", `translation status to record ("machine" or "reviewed")`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *locale == "" || *in == "" {
		return fmt.Errorf("usage: n5e-i18n import -db <path> -locale <code> -in <file> [-status machine|reviewed]")
	}
	if *status != "machine" && *status != "reviewed" {
		return fmt.Errorf("status must be %q or %q, got %q", "machine", "reviewed", *status)
	}

	db, err := openRulesDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	f, err := os.Open(*in)
	if err != nil {
		return err
	}
	defer f.Close()

	var records []importRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for lineNo := 1; sc.Scan(); lineNo++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec importRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return fmt.Errorf("parse line %d: %w", lineNo, err)
		}
		if !i18n.IsTranslatable(rec.Table, rec.Field) {
			return fmt.Errorf("line %d: %s.%s is not a registered translatable field", lineNo, rec.Table, rec.Field)
		}
		if strings.TrimSpace(rec.Value) == "" {
			return fmt.Errorf("line %d: empty translated value for %s/%s/%s", lineNo, rec.Table, rec.Slug, rec.Field)
		}
		records = append(records, rec)
	}
	if err := sc.Err(); err != nil {
		return err
	}

	count, err := applyImportRecords(db, *locale, *status, records)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "import %s: %d row(s) upserted\n", *locale, count)
	return nil
}

// applyImportRecords upserts a batch of already-translated records into
// content_translations for one locale, in a single transaction -- the
// write core shared by cmdImport (reading a JSONL file written by a
// separate `translate` run) and translate-book (writing DeepL's output
// directly, with no JSONL round-trip).
func applyImportRecords(db *sql.DB, locale, status string, records []importRecord) (int, error) {
	if status != "machine" && status != "reviewed" {
		return 0, fmt.Errorf("status must be %q or %q, got %q", "machine", "reviewed", status)
	}
	if len(records) == 0 {
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO content_translations (table_name, slug, locale, field, value, status, en_hash, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(table_name, slug, locale, field) DO UPDATE SET
			value = excluded.value, status = excluded.status,
			en_hash = excluded.en_hash, updated_at = excluded.updated_at`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()

	for i, rec := range records {
		if strings.TrimSpace(rec.Value) == "" {
			tx.Rollback()
			return 0, fmt.Errorf("record %d: empty translated value for %s/%s/%s", i, rec.Table, rec.Slug, rec.Field)
		}
		if _, err := stmt.Exec(rec.Table, rec.Slug, locale, rec.Field, rec.Value, status, rec.EnHash); err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("record %d: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(records), nil
}

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to read from")
	locale := fs.String("locale", "", "target locale, e.g. es")
	table := fs.String("table", "", "limit to one table (default: every table in TranslatableFields)")
	strict := fs.Bool("strict", false, "exit non-zero if any table has missing or stale field(s) for this locale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *locale == "" {
		return fmt.Errorf("usage: n5e-i18n report -db <path> -locale <code> [-table <name>]")
	}

	db, err := openRulesDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	tables := []string{*table}
	if *table == "" {
		tables = tables[:0]
		for t := range i18n.TranslatableFields {
			tables = append(tables, t)
		}
		sort.Strings(tables)
	}

	fmt.Printf("%-28s %8s %8s %8s %8s %8s\n", "table", "total", "done", "stale", "missing", "reviewed")
	var gTotal, gDone, gStale, gReviewed int
	for _, t := range tables {
		rowsData, err := loadEnglishValues(db, t, "")
		if err != nil {
			return err
		}
		existing, err := existingTranslations(db, t, *locale)
		if err != nil {
			return err
		}
		var total, done, stale, reviewed int
		for _, rv := range rowsData {
			for field, enText := range rv.Fields {
				total++
				prior, ok := existing[rv.Key+"\x00"+field]
				if !ok {
					continue
				}
				done++
				if prior.EnHash != hashText(enText) {
					stale++
				}
				if prior.Status == "reviewed" {
					reviewed++
				}
			}
		}
		fmt.Printf("%-28s %8d %8d %8d %8d %8d\n", t, total, done, stale, total-done, reviewed)
		gTotal += total
		gDone += done
		gStale += stale
		gReviewed += reviewed
	}
	if len(tables) > 1 {
		fmt.Printf("%-28s %8d %8d %8d %8d %8d\n", "TOTAL", gTotal, gDone, gStale, gTotal-gDone, gReviewed)
	}
	if *strict && (gTotal-gDone > 0 || gStale > 0) {
		return fmt.Errorf("ERROR: report -strict: locale %s has %d missing and %d stale field(s) -- translate/import did not complete cleanly", *locale, gTotal-gDone, gStale)
	}
	return nil
}
