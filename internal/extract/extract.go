// Package extract turns sourcebook PDF pages into linear, reading-ordered
// text lines the parsers consume.
//
// How it works — and why it is this simple:
//
// The sourcebooks are exported from Microsoft Word (Acrobat PDFMaker). Word
// writes the PDF content stream in DOCUMENT LOGICAL ORDER: on a two-column
// page the entire left column is emitted before the right column, exactly
// the order a human reads. So the extractor takes text fragments in stream
// order and only inserts line breaks where the baseline (Y coordinate)
// changes. No column detection, no coordinate sorting.
//
// Library note (hard-won, do not revisit casually):
//   - github.com/dslipak/pdf Content().Text in stream order decodes these
//     books perfectly (verified char-for-char against poppler pdftotext on
//     sample pages of all four books).
//   - github.com/ledongthuc/pdf's raw fragment API mis-orders glyphs around
//     punctuation on these fonts, and its per-fragment X/W coordinates are
//     stale/zero, which breaks any coordinate-based reconstruction.
//   - pdftotext's own -raw mode re-sorts headings out of order. Stream order
//     beats it here.
package extract

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/dslipak/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Document is an open sourcebook PDF.
type Document struct {
	reader *pdf.Reader // the reader the pages came from (normalized if needed)
	pages  []pdf.Page  // leaf pages in document order, from our own tree walk
	path   string
}

// OutlineNode is one entry of the PDF bookmark tree. Word-exported books
// build this tree from the document's heading styles, so it captures the
// author's real structure (class → archetype → feature) — the class book
// parser anchors on it.
type OutlineNode struct {
	Title    string
	Children []OutlineNode
}

// Outline returns the document's bookmark tree, or nil when the PDF has no
// bookmarks.
func (d *Document) Outline() []OutlineNode {
	return convertOutline(d.reader.Outline().Child)
}

func convertOutline(kids []pdf.Outline) []OutlineNode {
	var nodes []OutlineNode
	for _, k := range kids {
		nodes = append(nodes, OutlineNode{
			Title:    strings.TrimSpace(k.Title),
			Children: convertOutline(k.Child),
		})
	}
	return nodes
}

// Open opens a sourcebook PDF for extraction.
//
// Some of the books (the class compendium) store most of their objects in
// compressed object streams that the pdf library fails to resolve — the page
// tree silently comes up short and, with the library's own Page(n) walk,
// pages past the gap repeat earlier ones. Open detects the shortfall by
// comparing the pages actually found against the count the document declares
// and, when they disagree, rewrites the PDF in memory with pdfcpu (pure Go)
// into a classic uncompressed layout the library reads correctly.
func Open(path string) (*Document, error) {
	r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	declared := r.NumPage() // root /Count — trustworthy in all four books
	pages := flattenPageTree(r.Trailer().Key("Root").Key("Pages"), 0)
	if len(pages) < declared {
		r2, p2, err := normalizeAndFlatten(path, declared)
		if err != nil {
			return nil, fmt.Errorf("%s: found only %d of %d declared pages and normalizing failed: %w",
				path, len(pages), declared, err)
		}
		r, pages = r2, p2
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("opening %s: no pages found in page tree", path)
	}
	return &Document{reader: r, pages: pages, path: path}, nil
}

// normalizeAndFlatten rewrites the PDF without object streams or xref
// streams (in memory; the books are tens of MB) and re-walks its page tree.
// It returns the new reader too — the original one cannot resolve these
// pages, and later needs (the bookmark outline) go through the same reader.
func normalizeAndFlatten(path string, declared int) (*pdf.Reader, []pdf.Page, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	conf := model.NewDefaultConfiguration()
	conf.WriteObjectStream = false
	conf.WriteXRefStream = false
	var buf bytes.Buffer
	if err := api.Optimize(f, &buf, conf); err != nil {
		return nil, nil, fmt.Errorf("pdfcpu rewrite: %w", err)
	}
	r, err := pdf.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return nil, nil, fmt.Errorf("reopening rewritten PDF: %w", err)
	}
	pages := flattenPageTree(r.Trailer().Key("Root").Key("Pages"), 0)
	if len(pages) < declared {
		return nil, nil, fmt.Errorf("still only %d of %d pages after rewrite", len(pages), declared)
	}
	return r, pages, nil
}

// flattenPageTree collects the leaf Page dictionaries in document order by
// walking the Kids arrays literally, the way poppler does.
//
// We do NOT use pdf.Reader.Page(n): it navigates by trusting each interior
// node's /Count to skip subtrees, and the class compendium's page tree has a
// /Count inconsistency that makes that walk loop — every physical page past
// 150 silently repeated pages 126-150, hiding a third of the book. Walking
// the tree ourselves and ignoring /Count sidesteps the whole class of bug.
// maxPageTreeDepth only guards against a cyclic tree (never legal in a PDF).
const maxPageTreeDepth = 32

func flattenPageTree(node pdf.Value, depth int) []pdf.Page {
	if depth > maxPageTreeDepth {
		return nil
	}
	switch node.Key("Type").Name() {
	case "Page":
		return []pdf.Page{{V: node}}
	case "Pages":
		var pages []pdf.Page
		kids := node.Key("Kids")
		for i := 0; i < kids.Len(); i++ {
			pages = append(pages, flattenPageTree(kids.Index(i), depth+1)...)
		}
		return pages
	}
	return nil
}

func (d *Document) NumPages() int { return len(d.pages) }

// yLineBreak is how far (in PDF points) the baseline must move between
// consecutive fragments to count as a new line. Body text in these books has
// ~12pt leading; sub/superscripts move less than 3pt.
const yLineBreak = 3.0

// PageLines extracts one physical page (1-based) as trimmed text lines in
// reading order. Empty lines are dropped.
func (d *Document) PageLines(n int) ([]string, error) {
	if n < 1 || n > len(d.pages) {
		return nil, fmt.Errorf("%s page %d: out of range 1-%d", d.path, n, len(d.pages))
	}
	p := d.pages[n-1]
	if p.V.IsNull() {
		return nil, fmt.Errorf("%s page %d: null page", d.path, n)
	}
	content := p.Content()

	var lines []string
	var b strings.Builder
	prevY := math.Inf(1)
	first := true

	flush := func() {
		if s := strings.TrimSpace(b.String()); s != "" {
			lines = append(lines, s)
		}
		b.Reset()
	}

	for _, t := range content.Text {
		if !first && math.Abs(t.Y-prevY) > yLineBreak {
			flush()
		}
		b.WriteString(t.S)
		prevY = t.Y
		first = false
	}
	flush()
	return lines, nil
}
