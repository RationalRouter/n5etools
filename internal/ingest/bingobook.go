package ingest

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/RationalRouter/n5e/internal/bingobookdata"
	"github.com/RationalRouter/n5e/internal/schema"
	"github.com/RationalRouter/n5e/internal/store"
)

// BingoBook loads the Bingo Book (book/bingo-book, the 70-page community
// bestiary/encounter-building document) — its NPC template stat blocks,
// General Traits glossary and Specialist Jutsu Lists — into db.
//
// Unlike every other book, nothing is parsed from the PDF: its text layer is
// an OCR pass over page images that silently corrupts the numbers a stat
// block is made of, so the content comes from internal/bingobookdata's hand
// transcription. pdfPath is only hashed. When it differs from the file the
// transcription was made from, the book has been revised: the existing
// transcription is still loaded (it remains the best available data) and the
// summary says the transcription needs updating against the new revision.
// pdfPath may be empty, in which case the transcription's own source hash is
// recorded.
func BingoBook(db *sql.DB, dbPath, pdfPath, version string) (*Report, error) {
	blocks, err := bingobookdata.StatBlocks()
	if err != nil {
		return nil, err
	}
	traits, err := bingobookdata.GeneralTraits()
	if err != nil {
		return nil, err
	}
	lists, err := bingobookdata.SpecialistLists()
	if err != nil {
		return nil, err
	}
	if problems := bingobookdata.Check(blocks, traits); len(problems) > 0 {
		return nil, fmt.Errorf("bingo book transcription fails its consistency checks:\n  %s",
			strings.Join(problems, "\n  "))
	}

	book := newSourceBook("book/bingo-book", "Bingo Book", bingobookdata.SourceVersion, bingobookdata.SourceSHA256)
	summary := &strings.Builder{}
	if pdfPath != "" {
		sha, err := FileSHA256(pdfPath)
		if err != nil {
			return nil, err
		}
		book.FileName = filepath.Base(pdfPath)
		book.FileSHA256 = sha
		if sha != bingobookdata.SourceSHA256 {
			fmt.Fprintf(summary, "WARNING: %s (sha256 %s, version %q) is not the revision this transcription was made from (sha256 %s, version %s). The transcription was loaded unchanged and needs updating against the new revision.\n\n",
				book.FileName, sha, version, bingobookdata.SourceSHA256, bingobookdata.SourceVersion)
		}
	}

	if err := schema.Apply(db, schema.Rules); err != nil {
		return nil, fmt.Errorf("preparing rules database: %w", err)
	}
	n, err := store.LoadBingoBookTemplates(db, book, blocks, traits, lists)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(summary, "loaded %d NPC template stat blocks (%d skills, %d entries, %d jutsu), %d general traits, %d specialist-list jutsu\n",
		n.StatBlocks, n.Skills, n.Entries, n.Jutsu, n.GeneralTraits, n.SpecialistJutsu)
	return &Report{Summary: summary.String()}, nil
}

// newSourceBook is sourceBook for a book whose hash is already known rather
// than read from a file on disk.
func newSourceBook(slug, title, version, sha string) store.SourceBook {
	return store.SourceBook{Slug: slug, Title: title, Version: version, FileSHA256: sha}
}
