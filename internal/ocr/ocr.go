// Package ocr digitizes the handful of sourcebook tables that only exist
// as rendered images in the PDF — no text layer at all — so they can never
// be picked up by internal/extract's text-based parsing. The two known
// cases today are the core book's Armor/Weapons price tables (Chapter 5)
// and the class compendium's per-class level-progression tables, both of
// which the Mastersheet-bootstrap plan (see project notes) always intended
// to eventually replace with a real, repeatable OCR pass instead of a
// third-party spreadsheet or one-off hand transcription.
//
// The pipeline is three shell-outs plus pure-Go reconstruction, matching
// this codebase's existing preference for external tools over cgo bindings
// (see internal/extract's use of pdfcpu/dslipak instead of a PDF-rendering
// C library):
//
//  1. RasterizePage shells out to poppler's pdftoppm to render one PDF page
//     to a high-DPI PNG (the resolution tesseract actually needs — the
//     screen-res renders elsewhere in this codebase are for human
//     reading, not OCR).
//  2. RunTesseract shells out to tesseract's TSV output mode, which reports
//     every recognized word with its pixel bounding box and a confidence
//     score — the bounding boxes are what make table reconstruction
//     possible at all; plain-text OCR output would just be word soup.
//  3. GroupRows/ExtractTable are pure Go: cluster words into text rows by
//     vertical position, then bucket each row's words into columns by
//     their horizontal position against caller-supplied column boundaries
//     (there's no reliable way to detect column boundaries automatically
//     from OCR output alone — these tables have colored zebra-striped
//     rows but no vertical rule lines — so the maintainer workflow is:
//     run WordsCommand once to see where words land, read off the column
//     start-x values by eye, then pass them to ExtractTable).
package ocr

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Word is one tesseract TSV row: a single recognized word and its pixel
// bounding box on the rasterized page image.
type Word struct {
	Left, Top, Width, Height int
	Conf                     float64 // -1 for non-text TSV rows (block/paragraph/line markers), 0-100 otherwise
	Text                     string
}

// Right and Bottom are convenience accessors — TSV only gives left/top/
// width/height, but row/column bucketing is easier to read in terms of
// the box's edges.
func (w Word) Right() int  { return w.Left + w.Width }
func (w Word) Bottom() int { return w.Top + w.Height }

// CenterY is used for row clustering: two words belong to the same text
// row if their vertical centers are close, which tolerates the small
// baseline jitter tesseract reports even along one printed table row.
func (w Word) CenterY() int { return w.Top + w.Height/2 }

// RasterizePage renders one page of a PDF to a PNG using poppler's
// pdftoppm (a system dependency, not a Go module — see the package doc).
// dpi should be high for OCR accuracy; 400 is what worked well against
// this book's page-image tables in testing. Returns the PNG's path.
func RasterizePage(pdfPath string, page, dpi int, outDir string) (string, error) {
	prefix := filepath.Join(outDir, fmt.Sprintf("ocr-page-%d", page))
	cmd := exec.Command("pdftoppm",
		"-png", "-r", strconv.Itoa(dpi),
		"-f", strconv.Itoa(page), "-l", strconv.Itoa(page),
		pdfPath, prefix,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pdftoppm: %w\n%s", err, out)
	}
	// pdftoppm appends a zero-padded page number to the prefix
	// (e.g. "ocr-page-28-028.png") rather than using the exact prefix —
	// the padding width depends on the document's total page count, so
	// glob for it instead of guessing the format.
	matches, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("pdftoppm: expected exactly 1 output file matching %s-*.png, found %d", prefix, len(matches))
	}
	return matches[0], nil
}

// RunTesseract shells out to tesseract's TSV output mode (`tesseract
// <image> stdout --psm 6 tsv`) and parses the result into Words. psm 6
// ("assume a single uniform block of text") is the mode that performed
// best against these tables in testing — the default psm (automatic page
// segmentation) tended to split a single table into multiple
// unpredictably-ordered blocks.
func RunTesseract(imagePath string) ([]Word, error) {
	cmd := exec.Command("tesseract", imagePath, "stdout", "--psm", "6", "tsv")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("tesseract: %w\n%s", err, ee.Stderr)
		}
		return nil, fmt.Errorf("tesseract: %w", err)
	}
	return ParseTSV(strings.NewReader(string(out)))
}

// ParseTSV parses tesseract's TSV output format — split out from
// RunTesseract so it's testable without the tesseract binary installed.
// The format is a header row followed by one row per detected layout
// element (page/block/paragraph/line/word, distinguished by the `level`
// column); only level 5 (word) rows carry real text and are kept.
//
// Columns, in order: level, page_num, block_num, par_num, line_num,
// word_num, left, top, width, height, conf, text.
func ParseTSV(r io.Reader) ([]Word, error) {
	scanner := bufio.NewScanner(bufio.NewReader(r))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var words []Word
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			continue // header row
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 12 {
			continue // malformed row, skip rather than fail the whole page
		}
		level := cols[0]
		if level != "5" {
			continue // not a word-level row
		}
		left, _ := strconv.Atoi(cols[6])
		top, _ := strconv.Atoi(cols[7])
		width, _ := strconv.Atoi(cols[8])
		height, _ := strconv.Atoi(cols[9])
		conf, _ := strconv.ParseFloat(cols[10], 64)
		text := cols[11]
		if strings.TrimSpace(text) == "" {
			continue
		}
		words = append(words, Word{Left: left, Top: top, Width: width, Height: height, Conf: conf, Text: text})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return words, nil
}

// GroupRows clusters words into text rows by vertical position. Words
// are first sorted by CenterY, then a new row starts whenever the next
// word's center is more than yTolerance pixels below the running row's
// average center — a simple single-pass clustering that works well for
// table rows, which don't overlap vertically. Each returned row is
// sorted left-to-right.
func GroupRows(words []Word, yTolerance int) [][]Word {
	if len(words) == 0 {
		return nil
	}
	sorted := make([]Word, len(words))
	copy(sorted, words)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CenterY() < sorted[j].CenterY() })

	var rows [][]Word
	var current []Word
	rowYSum, rowYCount := 0, 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		sort.Slice(current, func(i, j int) bool { return current[i].Left < current[j].Left })
		rows = append(rows, current)
		current = nil
		rowYSum, rowYCount = 0, 0
	}

	for _, w := range sorted {
		if len(current) > 0 {
			avgY := rowYSum / rowYCount
			if w.CenterY()-avgY > yTolerance {
				flush()
			}
		}
		current = append(current, w)
		rowYSum += w.CenterY()
		rowYCount++
	}
	flush()
	return rows
}

// Column names one output column and the pixel x-coordinate where it
// starts on the rasterized page — found by eye from a WordsCommand-style
// dump of the header row's word positions (see package doc).
type Column struct {
	Name   string
	StartX int
}

// ExtractTable buckets each row's words into columns by comparing each
// word's Left edge against the column start-x values (a word belongs to
// the last column whose StartX is <= the word's Left), then joins each
// column's words with spaces. columns must be sorted by StartX ascending.
// Returns one []string per row, positionally parallel to columns.
func ExtractTable(rows [][]Word, columns []Column) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		cells := make([]string, len(columns))
		for _, w := range row {
			col := columnFor(w.Left, columns)
			if cells[col] == "" {
				cells[col] = w.Text
			} else {
				cells[col] += " " + w.Text
			}
		}
		out[i] = cells
	}
	return out
}

func columnFor(x int, columns []Column) int {
	col := 0
	for i, c := range columns {
		if x >= c.StartX {
			col = i
		} else {
			break
		}
	}
	return col
}
