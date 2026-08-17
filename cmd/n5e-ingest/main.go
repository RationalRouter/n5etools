// n5e-ingest is the maintainer CLI: it reads the sourcebook PDFs, populates
// the rules database, and ends every run with a validation report.
//
// Subcommands grow as the pipeline does. Current:
//
//	n5e-ingest dump <book.pdf> <page> [endpage]
//	    Print a page's text in reconstructed reading order. Used to eyeball
//	    extraction quality and to cut golden test fixtures.
//
//	n5e-ingest ocr words <book.pdf> <page> [-dpi N]
//	    Rasterize a page and OCR it (see internal/ocr), printing every
//	    recognized word with its pixel bounding box. Page-image tables
//	    (no text layer at all — e.g. the core book's Armor/Weapons price
//	    tables) can't be picked up by `dump`; this is the first step in
//	    digitizing one: run it once to read off each column's start-x by
//	    eye, then feed those into `ocr table`.
//
//	n5e-ingest ocr table <book.pdf> <page> -columns "Name:0,Cost:200,..." [-dpi N] [-y-tolerance N]
//	    Reconstruct a table from the page using the given column
//	    boundaries (name:startX pairs, sorted left to right) and print it
//	    as tab-separated rows — a preview to eyeball before writing a real
//	    migration/loader from the values, same spirit as `dump`.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/sergio/n5e/internal/correct"
	"github.com/sergio/n5e/internal/export"
	"github.com/sergio/n5e/internal/extract"
	"github.com/sergio/n5e/internal/ingest"
	"github.com/sergio/n5e/internal/ocr"
	"github.com/sergio/n5e/internal/parse"
	"github.com/sergio/n5e/internal/schema"
	"github.com/sergio/n5e/internal/sheet"
	"github.com/sergio/n5e/internal/sources"
	"github.com/sergio/n5e/internal/store"
	"github.com/sergio/n5e/internal/validate"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: n5e-ingest dump <book.pdf> <page> [endpage]")
	}
	switch args[0] {
	case "dump":
		return cmdDump(args[1:])
	case "jutsu":
		return cmdJutsu(args[1:])
	case "clans":
		return cmdClans(args[1:])
	case "classes":
		return cmdClasses(args[1:])
	case "core":
		return cmdCore(args[1:])
	case "bingobook":
		return cmdBingoBook(args[1:])
	case "sheet":
		return cmdSheet(args[1:])
	case "sources":
		return cmdSources()
	case "validate":
		return cmdValidate(args[1:])
	case "export":
		return cmdExport(args[1:])
	case "ocr":
		return cmdOCR(args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

// cmdJutsu parses the jutsu compendium and prints a summary + anomaly list.
// Without -db it is a dry run; with -db it loads the parsed jutsu into the
// rules database (creating it and applying migrations if needed) and prints
// the load report.
func cmdJutsu(args []string) error {
	fs := flag.NewFlagSet("jutsu", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to load into (omit for a dry run)")
	version := fs.String("version", "", "book version as printed, e.g. 3.1 (recorded in provenance)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: n5e-ingest jutsu [-db rules.db] [-version 3.1] <jutsu-compendium.pdf>")
	}
	pdfPath := fs.Arg(0)

	if *dbPath == "" {
		return dryRunJutsu(pdfPath)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	report, err := ingest.Jutsu(db, *dbPath, pdfPath, *version)
	if err != nil {
		return err
	}
	printReport(*dbPath, report)
	return nil
}

// dryRunJutsu parses and text-corrects (misspell/apostrophe only —
// LanguageTool is skipped, see correctOptions) the jutsu compendium and
// prints the same summary a real ingest would, without touching any
// database.
func dryRunJutsu(pdfPath string) error {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return err
	}

	var lines []parse.Line
	// Physical pages 1-3 are cover/credits/TOC; content starts at page 4.
	for n := 4; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			return fmt.Errorf("page %d: %w", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, parse.Line{Page: n, Text: ln})
		}
	}

	jutsu, anomalies := parse.ParseJutsuBook(lines)
	tribes, tribeAnomalies := parse.ParseSummonTribes(lines)
	anomalies = append(anomalies, tribeAnomalies...)

	corrOpts := correctOptions("")
	jutsuCorr, err := correct.Sweep(context.Background(), &jutsu, corrOpts)
	if err != nil {
		return err
	}
	tribeCorr, err := correct.Sweep(context.Background(), &tribes, corrOpts)
	if err != nil {
		return err
	}
	printCorrectionReport(jutsuCorr, tribeCorr)

	byGroup := map[string]int{}
	byRank := map[string]int{}
	for _, j := range jutsu {
		byGroup[j.CategoryGroup]++
		byRank[j.Rank]++
	}
	fmt.Printf("parsed %d jutsu from %d pages\n\n", len(jutsu), doc.NumPages()-3)
	fmt.Println("by rank:")
	for _, r := range []string{"E", "D", "C", "B", "A", "S", ""} {
		if byRank[r] > 0 {
			label := r
			if label == "" {
				label = "(none)"
			}
			fmt.Printf("  %-7s %4d\n", label, byRank[r])
		}
	}
	fmt.Println("\nby section:")
	groups := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	for _, g := range groups {
		fmt.Printf("  %-40s %4d\n", g, byGroup[g])
	}

	totTribeRoles, totTribeFeatures := 0, 0
	for _, t := range tribes {
		totTribeRoles += len(t.Roles)
		totTribeFeatures += len(t.Features)
	}
	fmt.Printf("\nparsed %d summon tribes (%d roles, %d features)\n",
		len(tribes), totTribeRoles, totTribeFeatures)

	fmt.Printf("\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Printf("  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}

	fmt.Println("\ndry run — pass -db rules.db to load")
	return nil
}

// cmdClans parses the clan compendium and prints per-clan summaries + the
// anomaly list. With -db it loads the batch into the rules database.
func cmdClans(args []string) error {
	fs := flag.NewFlagSet("clans", flag.ContinueOnError)
	verbose := fs.Bool("v", false, "print per-clan details (traits, features, feats)")
	dbPath := fs.String("db", "", "rules database to load into (omit for a dry run)")
	version := fs.String("version", "", "book version as printed, e.g. 3.11")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: n5e-ingest clans [-v] [-db rules.db] [-version 3.11] <clan-compendium.pdf>")
	}
	pdfPath := fs.Arg(0)

	if *dbPath == "" {
		return dryRunClans(pdfPath, *verbose)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	report, err := ingest.Clans(db, *dbPath, pdfPath, *version, *verbose)
	if err != nil {
		return err
	}
	printReport(*dbPath, report)
	return nil
}

// dryRunClans parses and text-corrects (LanguageTool skipped) the clan
// compendium and prints the same summary a real ingest would, without
// touching any database.
func dryRunClans(pdfPath string, verbose bool) error {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return err
	}

	var lines []parse.Line
	// Physical pages 1-4 are cover/credits/TOC; clans start at page 5.
	for n := 5; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			return fmt.Errorf("page %d: %w", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, parse.Line{Page: n, Text: ln})
		}
	}

	clans, anomalies := parse.ParseClanBook(lines)
	latentFeat, latents, latentAnomalies := parse.ParseBloodlineLatents(lines)
	anomalies = append(anomalies, latentAnomalies...)

	corrOpts := correctOptions("")
	clanCorr, err := correct.Sweep(context.Background(), &clans, corrOpts)
	if err != nil {
		return err
	}
	latentCorr, err := correct.Sweep(context.Background(), &latents, corrOpts)
	if err != nil {
		return err
	}
	// latentFeat is a single *Feat, not a slice — Sweep only walks slices,
	// so it's corrected via a one-element temporary and copied back.
	var latentFeatCorr *correct.Report
	if latentFeat != nil {
		wrapped := []parse.Feat{*latentFeat}
		latentFeatCorr, err = correct.Sweep(context.Background(), &wrapped, corrOpts)
		if err != nil {
			return err
		}
		*latentFeat = wrapped[0]
	} else {
		latentFeatCorr = &correct.Report{}
	}
	printCorrectionReport(clanCorr, latentCorr, latentFeatCorr)

	totJutsu, totFeatures, totFeats := 0, 0, 0
	for _, c := range clans {
		totJutsu += len(c.Jutsu)
		totFeatures += len(c.Features)
		totFeats += len(c.Feats)
	}
	fmt.Printf("parsed %d clans (%d features, %d clan jutsu, %d clan feats), %d bloodline latents\n\n",
		len(clans), totFeatures, totJutsu, totFeats, len(latents))
	for _, c := range clans {
		speed := "?"
		if c.SpeedFeet != nil {
			speed = strconv.Itoa(*c.SpeedFeet)
		}
		fmt.Printf("  p%-4d %-24s %-28s speed %-3s asi %-28q feat %d jutsu %2d feats %d traits %d\n",
			c.SourcePage, c.Name, c.Epithet, speed, c.AbilityRaw,
			len(c.Features), len(c.Jutsu), len(c.Feats), len(c.Traits))
		if verbose {
			for _, f := range c.Features {
				lvl := "-"
				if f.Level != nil {
					lvl = strconv.Itoa(*f.Level)
				}
				fmt.Printf("         feature L%-2s %s\n", lvl, f.Name)
			}
			for _, tr := range c.Traits {
				fmt.Printf("         trait       %s\n", tr.Name)
			}
			for _, ft := range c.Feats {
				fmt.Printf("         feat        %s [%s]\n", ft.Name, ft.Prerequisites)
			}
		}
	}

	fmt.Printf("\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Printf("  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}

	fmt.Println("\ndry run — pass -db rules.db to load")
	return nil
}

// cmdClasses parses the class compendium and prints per-class summaries +
// the anomaly list. Without -db it is a dry run; with -db it loads the
// parsed classes and class feats into the rules database.
func cmdClasses(args []string) error {
	fs := flag.NewFlagSet("classes", flag.ContinueOnError)
	verbose := fs.Bool("v", false, "print per-class details (proficiencies, features)")
	dbPath := fs.String("db", "", "rules database to load into (omit for a dry run)")
	version := fs.String("version", "", "book version as printed, e.g. 3.12")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: n5e-ingest classes [-v] [-db rules.db] [-version 3.12] <class-compendium.pdf>")
	}
	pdfPath := fs.Arg(0)

	if *dbPath == "" {
		return dryRunClasses(pdfPath, *verbose)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	report, err := ingest.Classes(db, *dbPath, pdfPath, *version, *verbose)
	if err != nil {
		return err
	}
	printReport(*dbPath, report)
	return nil
}

// dryRunClasses parses and text-corrects (LanguageTool skipped) the class
// compendium and prints the same summary a real ingest would, without
// touching any database.
func dryRunClasses(pdfPath string, verbose bool) error {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return err
	}

	var lines []parse.Line
	// Physical pages 1-5 are cover/credits/TOC; classes start at page 6.
	for n := 6; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			return fmt.Errorf("page %d: %w", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, parse.Line{Page: n, Text: ln})
		}
	}

	classes, anomalies := parse.ParseClassBook(lines)

	// Subclass sections, anchored on the PDF bookmark outline.
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

	corrOpts := correctOptions("")
	classCorr, err := correct.Sweep(context.Background(), &classes, corrOpts)
	if err != nil {
		return err
	}
	classFeatCorr, err := correct.Sweep(context.Background(), &classFeats, corrOpts)
	if err != nil {
		return err
	}
	printCorrectionReport(classCorr, classFeatCorr)

	fmt.Printf("parsed %d classes, %d class feats\n\n", len(classes), len(classFeats))
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
		fmt.Printf("  p%-4d %-24s hit d%-2d chakra d%-2d profs %2d equip %d casting %d features %2d | %-20s arch %2d archfeat %3d lists %d opts %3d sel %s\n",
			c.SourcePage, c.Name, c.HitDie, c.ChakraDie,
			len(c.Proficiencies), len(c.Equipment), len(c.Casting), len(c.Features),
			c.GroupHeading, nSubclass, nFeat, nLists, nOpts, selLevels)
		if verbose {
			for _, p := range c.Proficiencies {
				fmt.Printf("         prof %-12s %-30s choose %d\n", p.Kind, p.Value, p.ChooseN)
			}
			for _, cast := range c.Casting {
				fmt.Printf("         cast %-10s %s\n", cast.Discipline, cast.Ability)
			}
			for _, f := range c.Features {
				lvl := "-"
				if f.Level != nil {
					lvl = strconv.Itoa(*f.Level)
				}
				fmt.Printf("         feature L%-2s %s\n", lvl, f.Name)
			}
		}
	}
	fmt.Printf("\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Printf("  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}

	fmt.Println("\ndry run — pass -db rules.db to load")
	return nil
}

// cmdCore parses the core book's Chapter 13 slices (fighting stances,
// feats, backgrounds, enhancement seals, multiclass rules) and prints
// summaries. Without -db it is a dry run; with -db it loads them.
func cmdCore(args []string) error {
	fs := flag.NewFlagSet("core", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to load into (omit for a dry run)")
	version := fs.String("version", "", "book version as printed, e.g. 3.11")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: n5e-ingest core [-db rules.db] [-version 3.11] <core-book.pdf>")
	}
	pdfPath := fs.Arg(0)

	if *dbPath == "" {
		return dryRunCore(pdfPath)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	report, err := ingest.Core(db, *dbPath, pdfPath, *version)
	if err != nil {
		return err
	}
	printReport(*dbPath, report)
	return nil
}

// dryRunCore parses and text-corrects (LanguageTool skipped) the core
// book's Chapter 13 slices and prints the same summary a real ingest would,
// without touching any database.
func dryRunCore(pdfPath string) error {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return err
	}
	var lines []parse.Line
	// Physical page 1 is the cover; content and TOC follow.
	for n := 2; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			return fmt.Errorf("page %d: %w", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, parse.Line{Page: n, Text: ln})
		}
	}

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
	corrOpts := correctOptions("")
	stanceCorr, err := correct.Sweep(context.Background(), &stances, corrOpts)
	if err != nil {
		return err
	}
	featCorr, err := correct.Sweep(context.Background(), &feats, corrOpts)
	if err != nil {
		return err
	}
	bgCorr, err := correct.Sweep(context.Background(), &backgrounds, corrOpts)
	if err != nil {
		return err
	}
	sealCorr, err := correct.Sweep(context.Background(), &seals, corrOpts)
	if err != nil {
		return err
	}
	printCorrectionReport(stanceCorr, featCorr, bgCorr, sealCorr)

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
	fmt.Printf("parsed %d fighting stances (taijutsu %d, bukijutsu %d), %d feats, %d backgrounds, %d enhancement seals (weapon %d, armor %d), %d multiclass rules\n",
		len(stances), byKind["taijutsu"], byKind["bukijutsu"], len(feats), len(backgrounds),
		len(seals), byApplies["weapon"], byApplies["armor"], len(multiclass))
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		fmt.Printf("  feats %-24s %3d\n", c, byCat[c])
	}
	fmt.Printf("\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Printf("  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}

	fmt.Println("\ndry run — pass -db rules.db to load")
	return nil
}

// cmdBingoBook parses the Bingo Book Pack 1 adversary-building rules
// chapter (Ranks, Roles, Role Traits — see internal/parse/bingobook.go's
// package comment for the full scope) and prints a summary + anomaly list.
// Without -db it is a dry run; with -db it loads the parsed content into
// the given rules database. Only reachable manually here for the same
// eyeball-before-trust workflow every other book's subcommand gets — the
// unattended startup auto-updater (internal/autoupdate) calls
// ingest.BingoBook directly, not through this CLI.
func cmdBingoBook(args []string) error {
	fs := flag.NewFlagSet("bingobook", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to load into (omit for a dry run)")
	version := fs.String("version", "", "book version/date, e.g. 2026-07-27 (auto, drive)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: n5e-ingest bingobook [-db rules.db] [-version ...] <bingo-book.pdf>")
	}
	pdfPath := fs.Arg(0)

	if *dbPath == "" {
		return dryRunBingoBook(pdfPath)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	report, err := ingest.BingoBook(db, *dbPath, pdfPath, *version)
	if err != nil {
		return err
	}
	printReport(*dbPath, report)
	return nil
}

// dryRunBingoBook parses and text-corrects (LanguageTool skipped) the
// Bingo Book and prints the same summary a real ingest would, without
// touching any database.
func dryRunBingoBook(pdfPath string) error {
	doc, err := extract.Open(pdfPath)
	if err != nil {
		return err
	}
	var lines []parse.Line
	// No cover/TOC to skip — physical page 1 is the chapter's own first
	// content page.
	for n := 1; n <= doc.NumPages(); n++ {
		pageLines, err := doc.PageLines(n)
		if err != nil {
			return fmt.Errorf("page %d: %w", n, err)
		}
		for _, ln := range pageLines {
			lines = append(lines, parse.Line{Page: n, Text: ln})
		}
	}

	roles, roleAnomalies := parse.ParseAdversaryRoles(lines)
	traits, traitAnomalies := parse.ParseAdversaryRoleTraits(lines)
	ranks, freeformSlots, rankAnomalies := parse.ParseAdversaryRanks(lines)
	anomalies := append(roleAnomalies, traitAnomalies...)
	anomalies = append(anomalies, rankAnomalies...)

	corrOpts := correctOptions("")
	roleCorr, err := correct.Sweep(context.Background(), &roles, corrOpts)
	if err != nil {
		return err
	}
	traitCorr, err := correct.Sweep(context.Background(), &traits, corrOpts)
	if err != nil {
		return err
	}
	printCorrectionReport(roleCorr, traitCorr)

	byRole := map[string]int{}
	byRank := map[string]int{}
	for _, t := range traits {
		byRole[t.RoleName]++
		byRank[t.Rank]++
	}
	fmt.Printf("parsed %d adversary roles, %d role traits, %d ranks, %d freeform-slot rows\n\n",
		len(roles), len(traits), len(ranks), len(freeformSlots))
	fmt.Println("role traits by role:")
	for _, r := range roles {
		fmt.Printf("  %-12s %3d\n", r.Name, byRole[r.Name])
	}
	fmt.Println("role traits by rank:")
	for _, r := range []string{"D", "C", "B", "A", "S"} {
		fmt.Printf("  %-7s %3d\n", r+"-Rank", byRank[r])
	}
	fmt.Printf("\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Printf("  p%-4d %-40s %s\n", a.Page, a.Subject, a.Problem)
	}

	fmt.Println("\ndry run — pass -db rules.db to load")
	return nil
}

// cmdSheet reads the community Mastersheet xlsx — the agreed authority for
// the tabular data that exists only as images in the PDFs. Currently: the
// ClassChartMaster class progression charts.
func cmdSheet(args []string) error {
	fs := flag.NewFlagSet("sheet", flag.ContinueOnError)
	dbPath := fs.String("db", "", "rules database to load into (omit for a dry run)")
	version := fs.String("version", "", "sheet version as in the filename, e.g. 3.1")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: n5e-ingest sheet [-db rules.db] [-version 3.1] <mastersheet.xlsx>")
	}
	path := fs.Arg(0)

	charts, anomalies, err := sheet.ParseClassCharts(path)
	if err != nil {
		return err
	}
	fmt.Printf("parsed %d class charts\n", len(charts))
	for _, c := range charts {
		resources := map[string]bool{}
		jutsuKnown := 0
		for _, lr := range c.Levels {
			for _, r := range lr.Resources {
				resources[r.Name] = true
			}
			if lr.JutsuKnown != nil {
				jutsuKnown++
			}
		}
		names := make([]string, 0, len(resources))
		for n := range resources {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Printf("  %-24s hit d%-2d chakra d%-2d levels %d jutsu-known rows %2d resources %v\n",
			c.Name, c.HitDie, c.ChakraDie, len(c.Levels), jutsuKnown, names)
	}
	armors, weapons, waAnomalies, err := sheet.ParseWeapArmor(path)
	if err != nil {
		return err
	}
	anomalies = append(anomalies, waAnomalies...)
	fmt.Printf("\nparsed %d armor rows, %d weapon rows\n", len(armors), len(weapons))

	fmt.Printf("\n%d anomalies:\n", len(anomalies))
	for _, a := range anomalies {
		fmt.Printf("  %-30s %s\n", a.Subject, a.Problem)
	}

	if *dbPath == "" {
		fmt.Println("\ndry run — pass -db rules.db to load")
		return nil
	}
	sha, err := ingest.FileSHA256(path)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := schema.Apply(db, schema.Rules); err != nil {
		return fmt.Errorf("preparing rules database: %w", err)
	}
	book := store.SourceBook{
		Slug:       "sheet/mastersheet",
		Title:      "N5E Community Mastersheet",
		Version:    *version,
		FileName:   filepath.Base(path),
		FileSHA256: sha,
	}
	report, err := store.LoadClassLevels(db, book, charts)
	if err != nil {
		return err
	}
	printLoadReport(*dbPath, report)

	waReport, err := store.LoadWeapArmor(db, book, armors, weapons)
	if err != nil {
		return err
	}
	fmt.Println("\nweapons & armor:")
	printLoadReport(*dbPath, waReport)
	return nil
}

// cmdValidate opens an existing rules DB and prints the aggregate
// needs_review summary across every content table, plus (when -sheet is
// given) the Sheet1 name cross-check.
func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	dbPath := fs.String("db", "out/rules.db", "rules database to check")
	sheetPath := fs.String("sheet", "", "Mastersheet xlsx for the Sheet1 name cross-check (omit to skip)")
	outPath := fs.String("out", "", "also write the report to this file (omit for stdout only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	report, err := validate.Run(db, *sheetPath)
	if err != nil {
		return err
	}

	w := io.Writer(os.Stdout)
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		w = io.MultiWriter(os.Stdout, f)
	}
	return report.Write(w)
}

// cmdExport writes spreadsheet-friendly output derived from the rules DB —
// currently just the ClassChartMaster CSV, the friend's explicitly
// requested automation.
func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	dbPath := fs.String("db", "out/rules.db", "rules database to export from")
	outPath := fs.String("out", "out/classchart.csv", "output CSV path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || fs.Arg(0) != "classchart" {
		return fmt.Errorf("usage: n5e-ingest export classchart [-db rules.db] [-out classchart.csv]")
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	f, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := export.WriteClassChart(db, f); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", *outPath)
	return nil
}

// cmdSources prints the canonical, creator-maintained Google Drive location
// of every official sourcebook — a quick reference for the maintainer, and
// the data set a future "point the app at a Drive link" auto-fetcher will
// read from (see internal/sources).
func cmdSources() error {
	fmt.Println("Official N5E sourcebooks (canonical, immutable links per the creator):")
	for _, b := range sources.OfficialBooks {
		req := "required to play"
		if !b.Required {
			req = "optional, DM-facing"
		}
		fmt.Printf("  %-24s %-40s (%s)\n    %s\n", b.Slug, b.Title, req, b.DriveURL)
	}
	return nil
}

// correctOptions configures the automatic spelling/grammar pass (see
// internal/correct) for one subcommand invocation. LanguageTool (network,
// rate-limited) only runs when actually loading into a DB — a dry run stays
// fast, fully offline, and off the shared public API quota. misspell and
// the apostrophe heuristic (both local, instant) always run regardless,
// via correct.Sweep itself.
func correctOptions(dbPath string) correct.Options {
	opts := correct.Options{SkipLanguageTool: dbPath == ""}
	if dbPath != "" {
		opts.Cache = correct.LoadCache(dbPath + ".lt-cache.json")
	}
	return opts
}

// saveCorrectionCache persists any LanguageTool results fetched during this
// run, so re-ingesting unchanged text costs zero further requests.
func saveCorrectionCache(opts correct.Options) error {
	if opts.Cache != nil {
		return opts.Cache.Save()
	}
	return nil
}

func printCorrectionReport(reports ...*correct.Report) {
	var scanned, misspell, apostrophe, lt, ltSkipped int
	var warnings []string
	for _, r := range reports {
		scanned += r.FieldsScanned
		misspell += r.MisspellFixes
		apostrophe += r.ApostropheFixes
		lt += r.LanguageToolFixes
		ltSkipped += r.LanguageToolSkipped
		warnings = append(warnings, r.Warnings...)
	}
	fmt.Printf("\ntext correction: %d fields scanned, %d misspelling(s), %d apostrophe fix(es), %d languagetool fix(es)",
		scanned, misspell, apostrophe, lt)
	if ltSkipped > 0 {
		fmt.Printf(" (%d languagetool match(es) skipped)", ltSkipped)
	}
	fmt.Println()
	for _, w := range warnings {
		fmt.Println("  WARNING:", w)
	}
}

// printReport reproduces a real ingest's stdout exactly as the old
// monolithic cmdX functions printed it: correction report, then the
// parsed-count/anomaly summary, then one load report per batch (with a
// "\nLabel:\n" header for every batch after the first).
func printReport(dbPath string, report *ingest.Report) {
	printCorrectionReport(report.Correction...)
	fmt.Print(report.Summary)
	for _, l := range report.Loads {
		if l.Label != "" {
			fmt.Printf("\n%s:\n", l.Label)
		}
		printLoadReport(dbPath, l.Report)
	}
}

func printLoadReport(dbPath string, report *store.LoadReport) {
	fmt.Printf("\nloaded into %s:\n", dbPath)
	fmt.Printf("  created   %4d\n", report.Created)
	fmt.Printf("  updated   %4d\n", report.Updated)
	fmt.Printf("  unchanged %4d\n", report.Unchanged)
	fmt.Printf("  needs_review rows in DB: %d\n", report.NeedsReview)
	for _, s := range report.Demoted {
		fmt.Printf("  DEMOTED (was verified, content changed): %s\n", s)
	}
	for _, s := range report.Duplicates {
		fmt.Printf("  DUPLICATE SLUG: %s\n", s)
	}
	for _, s := range report.Skipped {
		fmt.Printf("  SKIPPED: %s\n", s)
	}
	for _, s := range report.Vanished {
		fmt.Printf("  VANISHED (in DB, not in this book): %s\n", s)
	}
}

func cmdDump(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: n5e-ingest dump <book.pdf> <page> [endpage]")
	}
	first, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("bad page number %q", args[1])
	}
	last := first
	if len(args) > 2 {
		if last, err = strconv.Atoi(args[2]); err != nil {
			return fmt.Errorf("bad end page %q", args[2])
		}
	}

	doc, err := extract.Open(args[0])
	if err != nil {
		return err
	}

	for n := first; n <= last; n++ {
		lines, err := doc.PageLines(n)
		if err != nil {
			return err
		}
		if last > first {
			fmt.Printf("===== page %d =====\n", n)
		}
		for _, ln := range lines {
			fmt.Println(ln)
		}
	}
	return nil
}

// cmdOCR dispatches to its two sub-modes (see the package-doc usage
// comment above main): "words" for the raw column-finding dump, "table"
// for the reconstructed preview once column boundaries are known.
func cmdOCR(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: n5e-ingest ocr words|table <book.pdf> <page> [flags]")
	}
	switch args[0] {
	case "words":
		return cmdOCRWords(args[1:])
	case "table":
		return cmdOCRTable(args[1:])
	default:
		return fmt.Errorf("unknown ocr mode %q (want words or table)", args[0])
	}
}

// ocrImage rasterizes the given page (into the OS temp dir, left in place
// for inspection rather than cleaned up — these runs are infrequent
// maintainer-only digitization passes, not something that needs tidy
// disk hygiene) and OCRs it, returning the recognized words.
func ocrImage(pdfPath string, page, dpi int) ([]ocr.Word, error) {
	imagePath, err := ocr.RasterizePage(pdfPath, page, dpi, os.TempDir())
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "rasterized to %s\n", imagePath)
	return ocr.RunTesseract(imagePath)
}

func cmdOCRWords(args []string) error {
	fs := flag.NewFlagSet("ocr words", flag.ContinueOnError)
	dpi := fs.Int("dpi", 400, "rasterization DPI")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: n5e-ingest ocr words <book.pdf> <page> [-dpi N]")
	}
	page, err := strconv.Atoi(fs.Arg(1))
	if err != nil {
		return fmt.Errorf("bad page number %q", fs.Arg(1))
	}

	words, err := ocrImage(fs.Arg(0), page, *dpi)
	if err != nil {
		return err
	}
	fmt.Printf("%d words\n", len(words))
	fmt.Println("left\ttop\twidth\theight\tconf\ttext")
	for _, w := range words {
		fmt.Printf("%d\t%d\t%d\t%d\t%.1f\t%s\n", w.Left, w.Top, w.Width, w.Height, w.Conf, w.Text)
	}
	return nil
}

// parseColumns parses "-columns" values like "Name:0,Cost:200,Bulk:400"
// into ocr.Columns sorted by StartX — ExtractTable requires that order,
// and sorting here means the flag can be typed in any order.
func parseColumns(spec string) ([]ocr.Column, error) {
	parts := strings.Split(spec, ",")
	columns := make([]ocr.Column, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		nameX := strings.SplitN(p, ":", 2)
		if len(nameX) != 2 {
			return nil, fmt.Errorf("bad -columns entry %q, want Name:StartX", p)
		}
		x, err := strconv.Atoi(strings.TrimSpace(nameX[1]))
		if err != nil {
			return nil, fmt.Errorf("bad -columns entry %q: %w", p, err)
		}
		columns = append(columns, ocr.Column{Name: strings.TrimSpace(nameX[0]), StartX: x})
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("-columns must name at least one column")
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].StartX < columns[j].StartX })
	return columns, nil
}

func cmdOCRTable(args []string) error {
	fs := flag.NewFlagSet("ocr table", flag.ContinueOnError)
	dpi := fs.Int("dpi", 400, "rasterization DPI")
	yTolerance := fs.Int("y-tolerance", 15, "pixel tolerance for clustering words into the same row")
	columnsFlag := fs.String("columns", "", `column boundaries, e.g. "Name:0,Cost:200,Bulk:400" (required)`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 || *columnsFlag == "" {
		return fmt.Errorf(`usage: n5e-ingest ocr table <book.pdf> <page> -columns "Name:0,Cost:200,..." [-dpi N] [-y-tolerance N]`)
	}
	page, err := strconv.Atoi(fs.Arg(1))
	if err != nil {
		return fmt.Errorf("bad page number %q", fs.Arg(1))
	}
	columns, err := parseColumns(*columnsFlag)
	if err != nil {
		return err
	}

	words, err := ocrImage(fs.Arg(0), page, *dpi)
	if err != nil {
		return err
	}
	rows := ocr.GroupRows(words, *yTolerance)
	table := ocr.ExtractTable(rows, columns)

	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.Name
	}
	fmt.Println(strings.Join(names, "\t"))
	for _, row := range table {
		fmt.Println(strings.Join(row, "\t"))
	}
	return nil
}
