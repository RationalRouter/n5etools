// Cross-validation hook: an optional, independent second opinion on every
// page a real re-ingest parses. internal/extract's text-layer reading is and
// remains the sole write path. A cross-check never changes what is loaded;
// it only flags pages where its own reading disagrees sharply with the text
// layer, which can mean the parser silently misread something.
//
// The player app leaves CrossCheck nil, so its startup auto-update never
// runs one. cmd/n5e-ingest installs an OCR-based check (see its
// ocrcheck.go), which needs pdftoppm and tesseract on the maintainer's
// machine.
package ingest

import (
	"github.com/RationalRouter/n5e/internal/parse"
	"github.com/RationalRouter/n5e/internal/store"
)

// OCRAnomaly is one page where an independent OCR reading disagreed with
// internal/extract's text-layer reading by more than normal noise (line-break
// placement, OCR misreads, whitespace) accounts for.
type OCRAnomaly struct {
	Page     int
	BookSlug string
	TextLine string // extract.Document's reading, condensed
	OCRLine  string // the OCR reading, condensed
	Ratio    float64
}

// CrossCheck, when non-nil, cross-validates pages [firstPage, lastPage] of
// pdfPath against lines, the text layer the ingest already extracted, and
// returns the pages that disagree. It must never fail an ingest: a page it
// can't read is skipped, not reported.
var CrossCheck func(pdfPath string, firstPage, lastPage int, lines []parse.Line) []OCRAnomaly

// crossCheck runs CrossCheck when one is installed and stamps bookSlug onto
// each anomaly.
func crossCheck(pdfPath, bookSlug string, firstPage, lastPage int, lines []parse.Line) []OCRAnomaly {
	if CrossCheck == nil {
		return nil
	}
	anomalies := CrossCheck(pdfPath, firstPage, lastPage, lines)
	for i := range anomalies {
		anomalies[i].BookSlug = bookSlug
	}
	return anomalies
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
