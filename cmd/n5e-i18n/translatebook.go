// translate-book and check-drive: the book-scoped half of the centralized
// translation pipeline (see internal/autoupdate/translations.go's package
// doc for the client half). A maintainer runs check-drive to see which
// books have changed since they were last translated, then translate-book
// on each one -- chaining export -> glossary-gate -> DeepL -> import for
// every translatable table in one pass, instead of doing it table by table
// by hand.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/RationalRouter/n5e/internal/fetch"
	"github.com/RationalRouter/n5e/internal/i18n"
	"github.com/RationalRouter/n5e/internal/sources"
)

// knownNounColumns lists, per table, the columns whose raw text names a
// real in-game entity (class/clan/jutsu/item/etc.) -- read once up front to
// build translate-book's "known term" set for FindUnglossaried, so an
// already-catalogued proper noun never gets flagged as unglossaried just
// because glossary.md itself hasn't been updated to include it.
var knownNounColumns = map[string][]string{
	"classes":              {"name"},
	"clans":                {"name"},
	"jutsu":                {"name"},
	"equipment":            {"name"},
	"feats":                {"name"},
	"subclasses":           {"name"},
	"class_options":        {"name"},
	"class_option_entries": {"name"},
	"summon_tribes":        {"name"},
	"adversary_roles":      {"name"},
	"puppet_armor_chassis": {"name"},
	"armor_properties":     {"name"},
	"weapon_properties":    {"name"},
	"fighting_stances":     {"name"},
	"class_features":       {"name"},
	"subclass_features":    {"name"},
	"backgrounds":          {"name"},
}

func loadKnownNouns(db *sql.DB) (map[string]struct{}, error) {
	known := map[string]struct{}{}
	for table, cols := range knownNounColumns {
		for _, col := range cols {
			rows, err := db.Query("SELECT " + quoteIdent(col) + " FROM " + quoteIdent(table))
			if err != nil {
				// Table may not exist in an older rules.db copy -- known
				// nouns is a best-effort aid to the glossary gate, not a
				// hard requirement, so skip rather than fail the run.
				continue
			}
			for rows.Next() {
				var v sql.NullString
				if err := rows.Scan(&v); err != nil {
					rows.Close()
					return nil, err
				}
				if v.Valid && v.String != "" {
					known[v.String] = struct{}{}
				}
			}
			rows.Close()
		}
	}
	return known, nil
}

// cmdTranslateBook chains export -> glossary-gate -> DeepL -> import for
// every translatable table, scoped to one book's rows (see
// loadEnglishValues' bookSlug parameter). Rows whose English text contains
// a term FindUnglossaried can't resolve are held back in a pending file
// instead of being sent to DeepL or blocking the rest of the book --
// exactly one row's worth of content, not the whole book, per the
// glossary-gate design decision.
func cmdTranslateBook(args []string) error {
	fs := flag.NewFlagSet("translate-book", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to read from and write into")
	locale := fs.String("locale", "", "target locale code, e.g. es")
	glossaryPath := fs.String("glossary", "internal/i18n/glossary.md", "glossary markdown file")
	pendingPath := fs.String("pending", "internal/i18n/glossary-pending.json", "file tracking rows held back for missing glossary terms")
	statusFlag := fs.String("status", "machine", `translation status to record ("machine" or "reviewed")`)
	force := fs.Bool("force", false, "re-translate every field, even ones already translated and fresh")
	pro := fs.Bool("pro", false, "use the DeepL Pro API host instead of the Free one")
	stateFile := fs.String("state", "internal/i18n/translation-state.json", "file tracking each book+locale's last-translated timestamp")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: n5e-i18n translate-book <slug> -db <path> -locale <code> [-glossary <file>] [-pending <file>] [-status machine|reviewed] [-force] [-pro]")
	}
	bookSlug := fs.Arg(0)
	if *dbPath == "" || *locale == "" {
		return fmt.Errorf("usage: n5e-i18n translate-book <slug> -db <path> -locale <code> [-glossary <file>] [-pending <file>] [-status machine|reviewed] [-force] [-pro]")
	}
	targetLang, ok := deeplTargetLang[*locale]
	if !ok {
		return fmt.Errorf("no DeepL target_lang mapping for locale %q -- add one to deeplTargetLang", *locale)
	}
	apiKey := os.Getenv("DEEPL_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("DEEPL_API_KEY is not set")
	}

	db, err := openRulesDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	glossaryTerms, err := i18n.ParseGlossaryTables(*glossaryPath, glossaryColumn[*locale])
	if err != nil {
		fmt.Fprintf(os.Stderr, "translate-book: warning: could not parse glossary %s: %v\n", *glossaryPath, err)
		glossaryTerms = map[string]string{}
	}
	known, err := loadKnownNouns(db)
	if err != nil {
		return fmt.Errorf("loading known-noun set: %w", err)
	}
	pending, err := i18n.LoadPendingGlossary(*pendingPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", *pendingPath, err)
	}

	var tables []string
	for t := range i18n.TranslatableFields {
		tables = append(tables, t)
	}
	sort.Strings(tables)

	var records []importRecord
	var enTexts []string
	skipped, resolved := 0, 0

	for _, table := range tables {
		rowsData, err := loadEnglishValues(db, table, bookSlug)
		if err != nil {
			return err
		}
		existing, err := existingTranslations(db, table, *locale)
		if err != nil {
			return err
		}
		for _, rv := range rowsData {
			for field, enText := range rv.Fields {
				enHash := hashText(enText)
				pendingKey := table + "\x00" + rv.Key + "\x00" + field
				if !*force {
					if prior, ok := existing[rv.Key+"\x00"+field]; ok && prior.EnHash == enHash {
						delete(pending, pendingKey) // English (and any glossary flag) is moot now
						continue
					}
				}

				if terms := i18n.FindUnglossaried(enText, glossaryTerms, known); len(terms) > 0 {
					pending[pendingKey] = i18n.PendingGlossaryEntry{
						Table: table, Slug: rv.Key, Field: field, EnHash: enHash, Terms: terms,
					}
					skipped++
					continue
				}
				if _, wasPending := pending[pendingKey]; wasPending {
					delete(pending, pendingKey)
					resolved++
				}

				records = append(records, importRecord{Table: table, Slug: rv.Key, Field: field, EnHash: enHash})
				enTexts = append(enTexts, enText)
			}
		}
	}

	if err := i18n.SavePendingGlossary(*pendingPath, pending); err != nil {
		return fmt.Errorf("writing %s: %w", *pendingPath, err)
	}

	if len(records) == 0 {
		fmt.Fprintf(os.Stderr, "translate-book %s/%s: nothing to translate (%d skipped for missing glossary terms, %d resolved)\n", bookSlug, *locale, skipped, resolved)
		return nil
	}

	ctx := context.Background()
	client := i18n.NewDeepLClient(apiKey, *pro)
	glossaryID, err := setUpGlossary(ctx, client, "translate-book", "n5e-content", *locale, *glossaryPath, targetLang)
	if err != nil {
		return err
	}

	protected := make([]string, len(enTexts))
	for i, t := range enTexts {
		protected[i] = i18n.ProtectPlaceholders(t)
	}
	translated, translateErr := client.Translate(ctx, protected, targetLang, glossaryID)
	for i := 0; i < len(translated); i++ {
		records[i].Value = i18n.RestorePlaceholders(translated[i])
	}
	records = records[:len(translated)] // only completed batches get applied -- see cmdTranslate's own comment on this

	applied, err := applyImportRecords(db, *locale, *statusFlag, records)
	if err != nil {
		return fmt.Errorf("applying translated records: %w", err)
	}
	if translateErr != nil {
		return fmt.Errorf("translating (%d/%d field(s) completed and applied before the error): %w", applied, len(enTexts), translateErr)
	}

	if err := touchTranslationState(*stateFile, bookSlug, *locale); err != nil {
		fmt.Fprintf(os.Stderr, "translate-book: warning: could not update %s: %v\n", *stateFile, err)
	}

	fmt.Fprintf(os.Stderr, "translate-book %s/%s: %d field(s) translated, %d skipped for missing glossary terms, %d resolved from a prior run\n",
		bookSlug, *locale, applied, skipped, resolved)
	return nil
}

// cmdCheckDrive is the maintainer-side staleness check: for every official
// sourcebook and supported non-English locale, compares the book's live
// Drive Last-Modified time against the last time translate-book actually
// ran for that book+locale (tracked in -state, a small maintainer-only
// side file -- deliberately not a rules.db table, since end users have no
// use for Sergio's own translation-workflow bookkeeping).
func cmdCheckDrive(args []string) error {
	fs := flag.NewFlagSet("check-drive", flag.ContinueOnError)
	stateFile := fs.String("state", "internal/i18n/translation-state.json", "file tracking each book+locale's last-translated timestamp")
	if err := fs.Parse(args); err != nil {
		return err
	}

	state, err := loadTranslationState(*stateFile)
	if err != nil {
		return fmt.Errorf("loading %s: %w", *stateFile, err)
	}

	ctx := context.Background()
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	anyStale := false
	for _, book := range sources.OfficialBooks {
		driveModified, err := fetch.HeadLastModified(ctx, nil, book.DriveURL)
		if err != nil {
			fmt.Fprintf(w, "ERROR %-28s checking Drive: %v\n", book.Slug, err)
			continue
		}
		for _, locale := range i18n.SupportedLocales {
			if locale == "en" {
				continue
			}
			lastTranslated, ok := state[stateKey(book.Slug, locale)]
			switch {
			case !ok:
				anyStale = true
				fmt.Fprintf(w, "NEEDS TRANSLATION  %-28s %s (never translated)\n", book.Slug, locale)
			case driveModified.After(lastTranslated):
				anyStale = true
				fmt.Fprintf(w, "NEEDS REFRESH      %-28s %s (drive modified %s, last translated %s)\n",
					book.Slug, locale, driveModified.Format(time.RFC3339), lastTranslated.Format(time.RFC3339))
			default:
				fmt.Fprintf(w, "up to date         %-28s %s\n", book.Slug, locale)
			}
		}
	}
	if !anyStale {
		fmt.Fprintln(w, "\nevery book+locale is current")
	}
	return nil
}
