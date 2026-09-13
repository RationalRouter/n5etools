// OCR cross-validation: an independent second opinion on every page a real
// re-ingest already parses. internal/extract's text-layer reading is and
// remains the sole write path — nothing here is ever loaded into the
// database — but a page where an independent OCR reading disagrees sharply
// with the text-layer reading is worth a maintainer's attention, since it can
// mean the text-layer parser silently misread something the PDF's own
// content stream actually says correctly (or vice versa).
//
// This only ever runs from inside an ingest.X function, which itself only
// runs when internal/autoupdate has already decided a book is stale — so it
// never adds OCR's tesseract/pdftoppm shell-out cost to an ordinary,
// unchanged-content startup.
package ingest

import (
	"fmt"
	"os"
	"strings"

	"github.com/sergio/n5e/internal/ocr"
	"github.com/sergio/n5e/internal/parse"
	"github.com/sergio/n5e/internal/store"
)

// OCRAnomaly is one page where an independent OCR reading disagreed with
// internal/extract's text-layer reading by more than normal noise (line-break
// placement, OCR misreads, whitespace) accounts for.
type OCRAnomaly struct {
	Page     int
	BookSlug string
	TextLine string // extract.Document's reading, condensed
	OCRLine  string // ocr.PageText's reading, condensed
	Ratio    float64
}

// ocrSimilarityThreshold is deliberately conservative: this is a coarse
// "does this page look right at all" smoke check, not a per-field diff, so
// it should only fire on a real disagreement, not on the line-boundary and
// whitespace differences that are expected even between two correct
// readings of the same page.
const ocrSimilarityThreshold = 0.85

// ocrDPI/ocrYTolerance match the values internal/ocr's own doc comment
// records as having worked well in testing against this book's page-image
// tables — reused here rather than re-tuned, since cross-validation is
// reading the same kind of rendered page.
const (
	ocrDPI        = 400
	ocrYTolerance = 6
)

// crossValidatePage rasterizes and OCRs one page of pdfPath, compares it
// against the text-layer's own reading of that page (textPageLines, already
// extracted by the caller), and returns a non-nil anomaly only when the two
// readings disagree by more than ocrSimilarityThreshold tolerates. A
// rasterize/OCR failure is reported as an error, not silently swallowed —
// the caller decides whether to treat that as fatal or just skip the page
// (see crossValidatePages below, which skips and keeps going: OCR trouble
// on one page must never block a real ingest of the book's actual content).
func crossValidatePage(pdfPath string, page int, textPageLines []string) (*OCRAnomaly, error) {
	tmpDir, err := os.MkdirTemp("", "n5e-ocr-crosscheck-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	imgPath, err := ocr.RasterizePage(pdfPath, page, ocrDPI, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("rasterizing page %d: %w", page, err)
	}
	words, err := ocr.RunTesseract(imgPath)
	if err != nil {
		return nil, fmt.Errorf("OCRing page %d: %w", page, err)
	}

	textLine := normalizeForCompare(strings.Join(textPageLines, "\n"))
	ocrLine := normalizeForCompare(ocr.PageText(words, ocrYTolerance))
	ratio := tokenSimilarity(textLine, ocrLine)
	if ratio >= ocrSimilarityThreshold {
		return nil, nil
	}
	return &OCRAnomaly{
		Page:     page,
		TextLine: excerpt(textLine, 200),
		OCRLine:  excerpt(ocrLine, 200),
		Ratio:    ratio,
	}, nil
}

// crossValidatePages runs crossValidatePage across every page in [firstPage,
// lastPage], keyed by the same lines the caller already extracted via
// linesFrom, grouped back out per page, and stamps bookSlug onto each
// resulting anomaly. A page that fails to rasterize/OCR at all (a missing
// pdftoppm/tesseract binary, a malformed page image) is skipped with no
// anomaly recorded — cross-validation is advisory, and must never turn an
// environment gap into a failed real ingest.
func crossValidatePages(pdfPath, bookSlug string, firstPage, lastPage int, lines []parse.Line) []OCRAnomaly {
	byPage := map[int][]string{}
	for _, l := range lines {
		byPage[l.Page] = append(byPage[l.Page], l.Text)
	}

	var anomalies []OCRAnomaly
	for page := firstPage; page <= lastPage; page++ {
		a, err := crossValidatePage(pdfPath, page, byPage[page])
		if err != nil {
			continue
		}
		if a != nil {
			a.BookSlug = bookSlug
			anomalies = append(anomalies, *a)
		}
	}
	return anomalies
}

// normalizeForCompare strips the formatting noise that differs between the
// text layer's and OCR's readings of an identical, correct page —
// whitespace runs, case, and punctuation — so the similarity check reflects
// actual content disagreement, not incidental rendering differences.
func normalizeForCompare(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r == ' ' || r == '\n' || r == '\t' || r == '\r':
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
		case strings.ContainsRune(`.,;:!?'"()[]{}-–—`, r):
			// dropped entirely, not replaced with a space
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// tokenSimilarity is a token-set Jaccard ratio: |intersection| / |union| of
// each string's distinct whitespace-separated tokens. Deliberately
// order-insensitive — extract.Document's line-break placement and
// ocr.GroupRows' row clustering won't agree on line boundaries even on a
// perfect page, so comparing token sets rather than aligned lines avoids
// false positives from that alone.
func tokenSimilarity(a, b string) float64 {
	setA := tokenSet(a)
	setB := tokenSet(b)
	if len(setA) == 0 && len(setB) == 0 {
		return 1
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	inter := 0
	for t := range setA {
		if setB[t] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	return float64(inter) / float64(union)
}

func tokenSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, t := range strings.Fields(s) {
		set[t] = true
	}
	return set
}

// toStoreOCRAnomalies drops BookSlug (store.WriteOCRAnomalies already takes
// the book separately, same as WriteTextCorrections) and converts to
// store's own mirror type, which exists only to avoid an ingest<->store
// import cycle (see store/ocranomalies.go).
func toStoreOCRAnomalies(anomalies []OCRAnomaly) []store.OCRAnomaly {
	out := make([]store.OCRAnomaly, len(anomalies))
	for i, a := range anomalies {
		out[i] = store.OCRAnomaly{Page: a.Page, TextLine: a.TextLine, OCRLine: a.OCRLine, Ratio: a.Ratio}
	}
	return out
}

func excerpt(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
