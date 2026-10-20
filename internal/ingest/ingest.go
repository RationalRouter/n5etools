// Package ingest is the reusable core of the maintainer CLI's parse-correct-
// load pipeline, extracted so both cmd/n5e-ingest (a human reviewing dry-run
// output before committing to a load) and the player-facing server's
// unattended startup auto-updater (internal/autoupdate) can drive the exact
// same parse/correct/store orchestration against a PDF, instead of
// duplicating it.
//
// Each exported function here always does a REAL ingest: parse the PDF,
// run the full text-correction sweep (LanguageTool included), and load the
// result into the given, already-open database, inside schema.Apply'd
// migrations. There is no dry-run mode in this package — cmd/n5e-ingest's
// dry-run display (no -db given) intentionally stays as its own simpler,
// LanguageTool-skipping code path in main.go, unchanged by this package.
package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/RationalRouter/n5e/internal/correct"
	"github.com/RationalRouter/n5e/internal/extract"
	"github.com/RationalRouter/n5e/internal/parse"
	"github.com/RationalRouter/n5e/internal/schema"
	"github.com/RationalRouter/n5e/internal/store"
)

// NamedLoad pairs one store.LoadReport with the label the CLI has always
// printed above it ("" for the book's primary batch, "summon tribes" for a
// secondary one, matching today's fmt.Println("\nsummon tribes:") headers).
type NamedLoad struct {
	Label  string
	Report *store.LoadReport
}

// Report is everything one ingest call produced, structured so a caller can
// either reproduce the CLI's exact stdout (cmd/n5e-ingest) or log a short
// summary (internal/autoupdate).
type Report struct {
	// Summary is the parsed-count breakdown + anomaly listing exactly as
	// the CLI has always printed it, e.g. "parsed 421 jutsu from 210
	// pages\n\nby rank:\n  ...\n\n12 anomalies:\n  ...".
	Summary string
	// Correction holds one report per batch parsed in this call (e.g.
	// jutsu, then summon tribes) — pass all of them to
	// printCorrectionReport at once, exactly as the CLI has always done,
	// to get the same summed totals.
	Correction []*correct.Report
	// Loads is one entry per store.LoadReport produced, in display order.
	Loads []NamedLoad
	// OCRAnomalies is every page where an independent OCR reading disagreed
	// with the text-layer parse enough to flag for review — see crosscheck.go.
	// Always empty when no CrossCheck is installed (the player app).
	// Advisory only; never affects what gets loaded.
	OCRAnomalies []OCRAnomaly
}

// FileSHA256 hashes a file's contents, recorded as provenance on every
// SourceBook row.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// correctOptions configures the automatic spelling/grammar pass for one real
// ingest call. LanguageTool always runs here (unlike the CLI's dry-run
// path) since this package only ever does real loads; dbPath keys the
// on-disk cache sidecar so re-ingesting unchanged text costs zero further
// LanguageTool requests.
func correctOptions(dbPath string) correct.Options {
	return correct.Options{Cache: correct.LoadCache(dbPath + ".lt-cache.json")}
}

func saveCorrectionCache(opts correct.Options) error {
	if opts.Cache != nil {
		return opts.Cache.Save()
	}
	return nil
}

// linesFrom extracts every page's reconstructed-reading-order lines from
// firstPage through the end of the document, tagged with their page number.
func linesFrom(doc *extract.Document, firstPage int) ([]parse.Line, error) {
	var lines []parse.Line
	for n := firstPage; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, parse.Line{Page: n, Text: ln})
		}
	}
	return lines, nil
}

func sourceBook(slug, title, version, pdfPath string) (store.SourceBook, error) {
	sha, err := FileSHA256(pdfPath)
	if err != nil {
		return store.SourceBook{}, err
	}
	return store.SourceBook{
		Slug:       slug,
		Title:      title,
		Version:    version,
		FileName:   filepath.Base(pdfPath),
		FileSHA256: sha,
	}, nil
}

// Jutsu parses the jutsu compendium and loads it (jutsu + summon tribes)
// into db.
func Jutsu(db *sql.DB, dbPath, pdfPath, version string) (*Report, error) {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	// Physical pages 1-3 are cover/credits/TOC; content starts at page 4.
	lines, err := linesFrom(doc, 4)
	if err != nil {
		return nil, err
	}
	ocrAnomalies := crossCheck(pdfPath, "book/jutsu-compendium", 4, doc.NumPages(), lines)

	jutsu, anomalies := parse.ParseJutsuBook(lines)
	tribes, tribeAnomalies := parse.ParseSummonTribes(lines)
	anomalies = append(anomalies, tribeAnomalies...)

	// jutsu.Name is deliberately excluded from correct.Sweep's proseFields
	// (see registry.go's comment: identifier-like fields shouldn't go
	// through spellcheck/grammar tools, which are more likely to mangle a
	// proper noun than fix a real mistake). A source-book data-entry typo
	// in the name itself still needs a durable fix that survives every
	// re-ingest, so it's applied here as a narrow, exact-match substitution
	// instead of widening that registry.
	for i := range jutsu {
		if fixed, ok := correct.CuratedJutsuNameFixes[jutsu[i].Name]; ok {
			jutsu[i].Name = fixed
		}
	}

	corrOpts := correctOptions(dbPath)
	jutsuCorr, err := correct.Sweep(context.Background(), &jutsu, corrOpts)
	if err != nil {
		return nil, err
	}
	tribeCorr, err := correct.Sweep(context.Background(), &tribes, corrOpts)
	if err != nil {
		return nil, err
	}
	if err := saveCorrectionCache(corrOpts); err != nil {
		return nil, err
	}

	summary := &strings.Builder{}
	byGroup := map[string]int{}
	byRank := map[string]int{}
	for _, j := range jutsu {
		byGroup[j.CategoryGroup]++
		byRank[j.Rank]++
	}
	fmt.Fprintf(summary, "parsed %d jutsu from %d pages\n\n", len(jutsu), doc.NumPages()-3)
	fmt.Fprintln(summary, "by rank:")
	for _, r := range []string{"E", "D", "C", "B", "A", "S", ""} {
		if byRank[r] > 0 {
			label := r
			if label == "" {
				label = "(none)"
			}
			fmt.Fprintf(summary, "  %-7s %4d\n", label, byRank[r])
		}
	}
	fmt.Fprintln(summary, "\nby section:")
	groups := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	for _, g := range groups {
		fmt.Fprintf(summary, "  %-40s %4d\n", g, byGroup[g])
	}
	totTribeRoles, totTribeFeatures := 0, 0
	for _, t := range tribes {
		totTribeRoles += len(t.Roles)
		totTribeFeatures += len(t.Features)
	}
	fmt.Fprintf(summary, "\nparsed %d summon tribes (%d roles, %d features)\n",
		len(tribes), totTribeRoles, totTribeFeatures)
	fmt.Fprintf(summary, "\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Fprintf(summary, "  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}
	fmt.Fprintf(summary, "\n%d OCR cross-check anomalies:\n", len(ocrAnomalies))
	for _, a := range ocrAnomalies {
		fmt.Fprintf(summary, "  p%-4d similarity %.2f\n", a.Page, a.Ratio)
	}

	if err := schema.Apply(db, schema.Rules); err != nil {
		return nil, fmt.Errorf("preparing rules database: %w", err)
	}
	book, err := sourceBook("book/jutsu-compendium", "Jiraiya's Jutsu Compendium", version, pdfPath)
	if err != nil {
		return nil, err
	}
	loadReport, err := store.LoadJutsu(db, book, jutsu, anomalies)
	if err != nil {
		return nil, err
	}
	tribeReport, err := store.LoadSummonTribes(db, book, tribes)
	if err != nil {
		return nil, err
	}
	corrections := append(jutsuCorr.Records, tribeCorr.Records...)
	if err := store.WriteTextCorrections(db, book, corrections); err != nil {
		return nil, err
	}
	if err := store.WriteOCRAnomalies(db, book, toStoreOCRAnomalies(ocrAnomalies)); err != nil {
		return nil, err
	}

	return &Report{
		Summary:    summary.String(),
		Correction: []*correct.Report{jutsuCorr, tribeCorr},
		Loads: []NamedLoad{
			{Label: "", Report: loadReport},
			{Label: "summon tribes", Report: tribeReport},
		},
		OCRAnomalies: ocrAnomalies,
	}, nil
}

// Clans parses the clan compendium (clans + bloodline latents) and loads it
// into db. verbose adds per-entry feature/trait/feat detail lines to
// Summary, matching the CLI's -v flag.
func Clans(db *sql.DB, dbPath, pdfPath, version string, verbose bool) (*Report, error) {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	// Physical pages 1-4 are cover/credits/TOC; clans start at page 5.
	lines, err := linesFrom(doc, 5)
	if err != nil {
		return nil, err
	}
	ocrAnomalies := crossCheck(pdfPath, "book/clan-compendium", 5, doc.NumPages(), lines)

	clans, anomalies := parse.ParseClanBook(lines)
	latentFeat, latents, latentAnomalies := parse.ParseBloodlineLatents(lines)
	anomalies = append(anomalies, latentAnomalies...)

	corrOpts := correctOptions(dbPath)
	clanCorr, err := correct.Sweep(context.Background(), &clans, corrOpts)
	if err != nil {
		return nil, err
	}
	latentCorr, err := correct.Sweep(context.Background(), &latents, corrOpts)
	if err != nil {
		return nil, err
	}
	// latentFeat is a single *Feat, not a slice — Sweep only walks slices,
	// so it's corrected via a one-element temporary and copied back.
	var latentFeatCorr *correct.Report
	if latentFeat != nil {
		wrapped := []parse.Feat{*latentFeat}
		latentFeatCorr, err = correct.Sweep(context.Background(), &wrapped, corrOpts)
		if err != nil {
			return nil, err
		}
		*latentFeat = wrapped[0]
	} else {
		latentFeatCorr = &correct.Report{}
	}
	if err := saveCorrectionCache(corrOpts); err != nil {
		return nil, err
	}

	summary := &strings.Builder{}
	totJutsu, totFeatures, totFeats := 0, 0, 0
	for _, c := range clans {
		totJutsu += len(c.Jutsu)
		totFeatures += len(c.Features)
		totFeats += len(c.Feats)
	}
	fmt.Fprintf(summary, "parsed %d clans (%d features, %d clan jutsu, %d clan feats), %d bloodline latents\n\n",
		len(clans), totFeatures, totJutsu, totFeats, len(latents))
	for _, c := range clans {
		speed := "?"
		if c.SpeedFeet != nil {
			speed = strconv.Itoa(*c.SpeedFeet)
		}
		fmt.Fprintf(summary, "  p%-4d %-24s %-28s speed %-3s asi %-28q feat %d jutsu %2d feats %d traits %d\n",
			c.SourcePage, c.Name, c.Epithet, speed, c.AbilityRaw,
			len(c.Features), len(c.Jutsu), len(c.Feats), len(c.Traits))
		if verbose {
			for _, f := range c.Features {
				lvl := "-"
				if f.Level != nil {
					lvl = strconv.Itoa(*f.Level)
				}
				fmt.Fprintf(summary, "         feature L%-2s %s\n", lvl, f.Name)
			}
			for _, tr := range c.Traits {
				fmt.Fprintf(summary, "         trait       %s\n", tr.Name)
			}
			for _, ft := range c.Feats {
				fmt.Fprintf(summary, "         feat        %s [%s]\n", ft.Name, ft.Prerequisites)
			}
		}
	}
	fmt.Fprintf(summary, "\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Fprintf(summary, "  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}
	fmt.Fprintf(summary, "\n%d OCR cross-check anomalies:\n", len(ocrAnomalies))
	for _, a := range ocrAnomalies {
		fmt.Fprintf(summary, "  p%-4d similarity %.2f\n", a.Page, a.Ratio)
	}

	if err := schema.Apply(db, schema.Rules); err != nil {
		return nil, fmt.Errorf("preparing rules database: %w", err)
	}
	book, err := sourceBook("book/clan-compendium", "Tsunade's Studies Compendium", version, pdfPath)
	if err != nil {
		return nil, err
	}
	loadReport, err := store.LoadClanBook(db, book, clans, anomalies)
	if err != nil {
		return nil, err
	}
	latentReport, err := store.LoadBloodlineLatents(db, book, latentFeat, latents)
	if err != nil {
		return nil, err
	}
	corrections := append(clanCorr.Records, latentCorr.Records...)
	corrections = append(corrections, latentFeatCorr.Records...)
	if err := store.WriteTextCorrections(db, book, corrections); err != nil {
		return nil, err
	}
	if err := store.WriteOCRAnomalies(db, book, toStoreOCRAnomalies(ocrAnomalies)); err != nil {
		return nil, err
	}

	return &Report{
		Summary:    summary.String(),
		Correction: []*correct.Report{clanCorr, latentCorr, latentFeatCorr},
		Loads: []NamedLoad{
			{Label: "", Report: loadReport},
			{Label: "bloodline latents", Report: latentReport},
		},
		OCRAnomalies: ocrAnomalies,
	}, nil
}

// Classes parses the class compendium (classes + subclasses + class feats)
// and loads it into db. verbose adds per-class proficiency/casting/feature
// detail lines to Summary, matching the CLI's -v flag.
func Classes(db *sql.DB, dbPath, pdfPath, version string, verbose bool) (*Report, error) {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	// Physical pages 1-5 are cover/credits/TOC; classes start at page 6.
	lines, err := linesFrom(doc, 6)
	if err != nil {
		return nil, err
	}
	ocrAnomalies := crossCheck(pdfPath, "book/class-compendium", 6, doc.NumPages(), lines)

	classes, anomalies := parse.ParseClassBook(lines)

	// Stage 2: subclass sections, anchored on the PDF bookmark outline.
	names := make([]string, len(classes))
	for i, c := range classes {
		names[i] = c.Name
	}
	sections := parse.ClassSections(doc.Outline(), names)
	for i := range classes {
		anomalies = append(anomalies, parse.ParseSubclasses(&classes[i], sections[classes[i].Name])...)
	}
	classFeats, featAnomalies := parse.ParseClassFeats(lines)
	anomalies = append(anomalies, featAnomalies...)

	corrOpts := correctOptions(dbPath)
	classCorr, err := correct.Sweep(context.Background(), &classes, corrOpts)
	if err != nil {
		return nil, err
	}
	classFeatCorr, err := correct.Sweep(context.Background(), &classFeats, corrOpts)
	if err != nil {
		return nil, err
	}
	if err := saveCorrectionCache(corrOpts); err != nil {
		return nil, err
	}

	summary := &strings.Builder{}
	fmt.Fprintf(summary, "parsed %d classes, %d class feats\n\n", len(classes), len(classFeats))
	for _, c := range classes {
		nSubclass, nFeat, nLists, nOpts := 0, 0, len(c.OptionLists), 0
		selLevels := ""
		if c.Group != nil {
			nSubclass = len(c.Group.Subclasses)
			for _, a := range c.Group.Subclasses {
				nFeat += len(a.Features)
			}
			selLevels = fmt.Sprint(c.Group.SelectionLevels)
		}
		for _, l := range c.OptionLists {
			nOpts += len(l.Options)
		}
		fmt.Fprintf(summary, "  p%-4d %-24s hit d%-2d chakra d%-2d profs %2d equip %d casting %d features %2d | %-20s arch %2d archfeat %3d lists %d opts %3d sel %s\n",
			c.SourcePage, c.Name, c.HitDie, c.ChakraDie,
			len(c.Proficiencies), len(c.Equipment), len(c.Casting), len(c.Features),
			c.GroupHeading, nSubclass, nFeat, nLists, nOpts, selLevels)
		if verbose {
			for _, p := range c.Proficiencies {
				fmt.Fprintf(summary, "         prof %-12s %-30s choose %d\n", p.Kind, p.Value, p.ChooseN)
			}
			for _, cast := range c.Casting {
				fmt.Fprintf(summary, "         cast %-10s %s\n", cast.Discipline, cast.Ability)
			}
			for _, f := range c.Features {
				lvl := "-"
				if f.Level != nil {
					lvl = strconv.Itoa(*f.Level)
				}
				fmt.Fprintf(summary, "         feature L%-2s %s\n", lvl, f.Name)
			}
		}
	}
	fmt.Fprintf(summary, "\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Fprintf(summary, "  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}
	fmt.Fprintf(summary, "\n%d OCR cross-check anomalies:\n", len(ocrAnomalies))
	for _, a := range ocrAnomalies {
		fmt.Fprintf(summary, "  p%-4d similarity %.2f\n", a.Page, a.Ratio)
	}

	if err := schema.Apply(db, schema.Rules); err != nil {
		return nil, fmt.Errorf("preparing rules database: %w", err)
	}
	book, err := sourceBook("book/class-compendium", "Orochimaru's Observation Compendium", version, pdfPath)
	if err != nil {
		return nil, err
	}
	loadReport, err := store.LoadClassBook(db, book, classes, anomalies)
	if err != nil {
		return nil, err
	}
	featReport, err := store.LoadClassFeats(db, book, classFeats)
	if err != nil {
		return nil, err
	}
	// Puppet Master's Puppet Upgrade tiers bundle several individually-named
	// upgrades into one class_options description field (see
	// internal/store/classoptionentries.go); scoped to this one class since
	// every other class's option rows are already one-row-per-choice.
	optionEntryReport, err := store.LoadClassOptionEntries(db, book, "class/puppet-master")
	if err != nil {
		return nil, err
	}
	// Science-Nin's own tiered gadget/upgrade catalogs (Scientific Ninja
	// Tools, S.N.B Upgrades, ...) bundle several individually-named entries
	// into one class_options description field the same way Puppet Master's
	// upgrade tiers do — split into class_option_entries so a picker can
	// address one named tool/upgrade at a time instead of the whole tier's
	// bundled text.
	scienceNinOptionEntryReport, err := store.LoadClassOptionEntries(db, book, "class/science-nin")
	if err != nil {
		return nil, err
	}
	// Black/Green/Red Technique's own "pick a base chassis/framework/role"
	// lists (Puppeteer Chassis, Puppet Frameworks, Puppet Roles) got
	// mis-bookmarked by the source PDF as one subclass feature instead of a
	// real option list — see subclassoptionlists.go's own doc. Weapon
	// Specialist's 8 Weapon Forms have the identical "[Form] Styles" shape.
	embeddedListReport, err := store.LoadEmbeddedSubclassOptionLists(db, book, "class/puppet-master")
	if err != nil {
		return nil, err
	}
	weaponFormStylesReport, err := store.LoadEmbeddedSubclassOptionLists(db, book, "class/weapon-specialist")
	if err != nil {
		return nil, err
	}
	corrections := append(classCorr.Records, classFeatCorr.Records...)
	if err := store.WriteTextCorrections(db, book, corrections); err != nil {
		return nil, err
	}
	if err := store.WriteOCRAnomalies(db, book, toStoreOCRAnomalies(ocrAnomalies)); err != nil {
		return nil, err
	}

	return &Report{
		Summary:    summary.String(),
		Correction: []*correct.Report{classCorr, classFeatCorr},
		Loads: []NamedLoad{
			{Label: "", Report: loadReport},
			{Label: "class feats", Report: featReport},
			{Label: "puppet upgrade entries", Report: optionEntryReport},
			{Label: "science-nin tool/upgrade entries", Report: scienceNinOptionEntryReport},
			{Label: "embedded subclass option lists", Report: embeddedListReport},
			{Label: "weapon form styles", Report: weaponFormStylesReport},
		},
		OCRAnomalies: ocrAnomalies,
	}, nil
}

// Core parses the core book's Chapter 13 slices (fighting stances, feats,
// backgrounds, enhancement seals, multiclass rules) and loads them into db.
func Core(db *sql.DB, dbPath, pdfPath, version string) (*Report, error) {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	// Physical page 1 is the cover; content and TOC follow.
	lines, err := linesFrom(doc, 2)
	if err != nil {
		return nil, err
	}
	ocrAnomalies := crossCheck(pdfPath, "book/core", 2, doc.NumPages(), lines)

	stances, anomalies := parse.ParseFightingStances(lines)
	feats, featAnomalies := parse.ParseCoreFeats(lines)
	anomalies = append(anomalies, featAnomalies...)
	backgrounds, bgAnomalies := parse.ParseBackgrounds(lines)
	anomalies = append(anomalies, bgAnomalies...)
	seals, sealAnomalies := parse.ParseEnhancementSeals(lines)
	anomalies = append(anomalies, sealAnomalies...)
	multiclass, mcAnomalies := parse.ParseMulticlassRules(lines)
	anomalies = append(anomalies, mcAnomalies...)

	// multiclass and its MulticlassRule struct have no prose fields (see
	// internal/correct/registry.go) — only the other three batches here
	// carry text worth checking.
	corrOpts := correctOptions(dbPath)
	stanceCorr, err := correct.Sweep(context.Background(), &stances, corrOpts)
	if err != nil {
		return nil, err
	}
	featCorr, err := correct.Sweep(context.Background(), &feats, corrOpts)
	if err != nil {
		return nil, err
	}
	bgCorr, err := correct.Sweep(context.Background(), &backgrounds, corrOpts)
	if err != nil {
		return nil, err
	}
	sealCorr, err := correct.Sweep(context.Background(), &seals, corrOpts)
	if err != nil {
		return nil, err
	}
	if err := saveCorrectionCache(corrOpts); err != nil {
		return nil, err
	}

	summary := &strings.Builder{}
	byKind := map[string]int{}
	for _, s := range stances {
		byKind[s.StanceType]++
	}
	byCat := map[string]int{}
	for _, f := range feats {
		byCat[f.Category]++
	}
	byApplies := map[string]int{}
	for _, s := range seals {
		byApplies[s.AppliesTo]++
	}
	fmt.Fprintf(summary, "parsed %d fighting stances (taijutsu %d, bukijutsu %d), %d feats, %d backgrounds, %d enhancement seals (weapon %d, armor %d), %d multiclass rules\n",
		len(stances), byKind["taijutsu"], byKind["bukijutsu"], len(feats), len(backgrounds),
		len(seals), byApplies["weapon"], byApplies["armor"], len(multiclass))
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		fmt.Fprintf(summary, "  feats %-24s %3d\n", c, byCat[c])
	}
	fmt.Fprintf(summary, "\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Fprintf(summary, "  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}
	fmt.Fprintf(summary, "\n%d OCR cross-check anomalies:\n", len(ocrAnomalies))
	for _, a := range ocrAnomalies {
		fmt.Fprintf(summary, "  p%-4d similarity %.2f\n", a.Page, a.Ratio)
	}

	if err := schema.Apply(db, schema.Rules); err != nil {
		return nil, fmt.Errorf("preparing rules database: %w", err)
	}
	book, err := sourceBook("book/core", "Naruto 5e Full Document", version, pdfPath)
	if err != nil {
		return nil, err
	}
	stanceReport, err := store.LoadFightingStances(db, book, stances)
	if err != nil {
		return nil, err
	}
	featReport, err := store.LoadCoreFeats(db, book, feats)
	if err != nil {
		return nil, err
	}
	bgReport, err := store.LoadBackgrounds(db, book, backgrounds)
	if err != nil {
		return nil, err
	}
	sealReport, err := store.LoadEnhancementSeals(db, book, seals)
	if err != nil {
		return nil, err
	}
	mcReport, err := store.LoadMulticlassRules(db, book, multiclass)
	if err != nil {
		return nil, err
	}

	corrections := append(stanceCorr.Records, featCorr.Records...)
	corrections = append(corrections, bgCorr.Records...)
	corrections = append(corrections, sealCorr.Records...)
	if err := store.WriteTextCorrections(db, book, corrections); err != nil {
		return nil, err
	}
	if err := store.WriteOCRAnomalies(db, book, toStoreOCRAnomalies(ocrAnomalies)); err != nil {
		return nil, err
	}

	return &Report{
		Summary:    summary.String(),
		Correction: []*correct.Report{stanceCorr, featCorr, bgCorr, sealCorr},
		Loads: []NamedLoad{
			{Label: "fighting stances", Report: stanceReport},
			{Label: "feats", Report: featReport},
			{Label: "backgrounds", Report: bgReport},
			{Label: "enhancement seals", Report: sealReport},
			{Label: "multiclass rules", Report: mcReport},
		},
		OCRAnomalies: ocrAnomalies,
	}, nil
}

// BingoBookPack1 parses Pack 1's adversary-building rules chapter (Ranks,
// Roles, Role Traits — see internal/parse/bingobook.go's package comment
// for the full scope) and loads them into db. This is the official chapter
// — the latest update layer of the Bingo Book, not the book itself; see
// BingoBook for the actual 70-page community bestiary/encounter-building
// document that slug now belongs to.
func BingoBookPack1(db *sql.DB, dbPath, pdfPath, version string) (*Report, error) {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	// No cover/TOC to skip — physical page 1 is the chapter's own first
	// content page.
	lines, err := linesFrom(doc, 1)
	if err != nil {
		return nil, err
	}
	ocrAnomalies := crossCheck(pdfPath, "book/bingo-book-pack-1", 1, doc.NumPages(), lines)

	roles, roleAnomalies := parse.ParseAdversaryRoles(lines)
	traits, traitAnomalies := parse.ParseAdversaryRoleTraits(lines)
	ranks, freeformSlots, rankAnomalies := parse.ParseAdversaryRanks(lines)
	strikeTable, strikeAnomalies := parse.ParseFreeformStrikeTable(lines)
	jutsuDamageTable, jutsuDamageAnomalies := parse.ParseFreeformJutsuDamageTable(lines)
	classMods, classModAnomalies := parse.ParseClassificationModifiers(lines)
	clarifications, clarificationAnomalies := parse.ParseAdversaryClarifications(lines)
	transformBuffs, transformBuffAnomalies := parse.ParseTransformationBuffs(lines)
	anomalies := append(roleAnomalies, traitAnomalies...)
	anomalies = append(anomalies, rankAnomalies...)
	anomalies = append(anomalies, strikeAnomalies...)
	anomalies = append(anomalies, jutsuDamageAnomalies...)
	anomalies = append(anomalies, classModAnomalies...)
	anomalies = append(anomalies, clarificationAnomalies...)
	anomalies = append(anomalies, transformBuffAnomalies...)

	corrOpts := correctOptions(dbPath)
	roleCorr, err := correct.Sweep(context.Background(), &roles, corrOpts)
	if err != nil {
		return nil, err
	}
	traitCorr, err := correct.Sweep(context.Background(), &traits, corrOpts)
	if err != nil {
		return nil, err
	}
	clarificationCorr, err := correct.Sweep(context.Background(), &clarifications, corrOpts)
	if err != nil {
		return nil, err
	}
	transformBuffCorr, err := correct.Sweep(context.Background(), &transformBuffs, corrOpts)
	if err != nil {
		return nil, err
	}
	if err := saveCorrectionCache(corrOpts); err != nil {
		return nil, err
	}

	summary := &strings.Builder{}
	byRole := map[string]int{}
	byRank := map[string]int{}
	for _, t := range traits {
		byRole[t.RoleName]++
		byRank[t.Rank]++
	}
	fmt.Fprintf(summary, "parsed %d adversary roles, %d role traits, %d ranks, %d freeform-slot rows\n",
		len(roles), len(traits), len(ranks), len(freeformSlots))
	fmt.Fprintf(summary, "parsed %d freeform strike rows, %d freeform jutsu damage rows, %d classification modifiers, %d clarifications, %d transformation buffs\n\n",
		len(strikeTable), len(jutsuDamageTable), len(classMods), len(clarifications), len(transformBuffs))
	fmt.Fprintln(summary, "role traits by role:")
	for _, r := range roles {
		fmt.Fprintf(summary, "  %-12s %3d\n", r.Name, byRole[r.Name])
	}
	fmt.Fprintln(summary, "role traits by rank:")
	for _, r := range []string{"D", "C", "B", "A", "S"} {
		fmt.Fprintf(summary, "  %-7s %3d\n", r+"-Rank", byRank[r])
	}
	fmt.Fprintf(summary, "\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Fprintf(summary, "  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}
	fmt.Fprintf(summary, "\n%d OCR cross-check anomalies:\n", len(ocrAnomalies))
	for _, a := range ocrAnomalies {
		fmt.Fprintf(summary, "  p%-4d similarity %.2f\n", a.Page, a.Ratio)
	}

	if err := schema.Apply(db, schema.Rules); err != nil {
		return nil, fmt.Errorf("preparing rules database: %w", err)
	}
	book, err := sourceBook("book/bingo-book-pack-1", "Bingo Book Pack 1 — Adversaries", version, pdfPath)
	if err != nil {
		return nil, err
	}
	roleReport, err := store.LoadAdversaryRoles(db, book, roles)
	if err != nil {
		return nil, err
	}
	traitReport, err := store.LoadAdversaryRoleTraits(db, book, traits)
	if err != nil {
		return nil, err
	}
	rankReport, err := store.LoadAdversaryRanks(db, book, ranks, freeformSlots)
	if err != nil {
		return nil, err
	}
	if err := store.LoadFreeformStrikeTable(db, book, strikeTable); err != nil {
		return nil, err
	}
	if err := store.LoadFreeformJutsuDamageTable(db, book, jutsuDamageTable); err != nil {
		return nil, err
	}
	if err := store.LoadClassificationModifiers(db, book, classMods); err != nil {
		return nil, err
	}
	clarificationReport, err := store.LoadAdversaryClarifications(db, book, clarifications)
	if err != nil {
		return nil, err
	}
	transformBuffReport, err := store.LoadTransformationBuffs(db, book, transformBuffs)
	if err != nil {
		return nil, err
	}

	corrections := append(roleCorr.Records, traitCorr.Records...)
	corrections = append(corrections, clarificationCorr.Records...)
	corrections = append(corrections, transformBuffCorr.Records...)
	if err := store.WriteTextCorrections(db, book, corrections); err != nil {
		return nil, err
	}
	if err := store.WriteOCRAnomalies(db, book, toStoreOCRAnomalies(ocrAnomalies)); err != nil {
		return nil, err
	}

	return &Report{
		Summary:    summary.String(),
		Correction: []*correct.Report{roleCorr, traitCorr, clarificationCorr, transformBuffCorr},
		Loads: []NamedLoad{
			{Label: "adversary roles", Report: roleReport},
			{Label: "adversary role traits", Report: traitReport},
			{Label: "adversary ranks", Report: rankReport},
			{Label: "adversary clarifications", Report: clarificationReport},
			{Label: "transformation buffs", Report: transformBuffReport},
		},
		OCRAnomalies: ocrAnomalies,
	}, nil
}
