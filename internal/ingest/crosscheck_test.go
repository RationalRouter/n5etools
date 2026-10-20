package ingest

import (
	"testing"

	"github.com/RationalRouter/n5e/internal/parse"
)

func TestCrossCheckUnsetSkips(t *testing.T) {
	old := CrossCheck
	CrossCheck = nil
	defer func() { CrossCheck = old }()

	if got := crossCheck("book.pdf", "book/core", 1, 10, nil); got != nil {
		t.Errorf("crossCheck with no CrossCheck installed = %v, want nil", got)
	}
}

func TestCrossCheckStampsBookSlug(t *testing.T) {
	old := CrossCheck
	defer func() { CrossCheck = old }()

	var gotFirst, gotLast int
	CrossCheck = func(pdfPath string, firstPage, lastPage int, lines []parse.Line) []OCRAnomaly {
		gotFirst, gotLast = firstPage, lastPage
		return []OCRAnomaly{{Page: 3}, {Page: 7}}
	}
	got := crossCheck("book.pdf", "book/core", 2, 9, nil)
	if gotFirst != 2 || gotLast != 9 {
		t.Errorf("CrossCheck called with pages %d-%d, want 2-9", gotFirst, gotLast)
	}
	if len(got) != 2 || got[0].BookSlug != "book/core" || got[1].BookSlug != "book/core" {
		t.Errorf("crossCheck = %+v, want both anomalies stamped with book/core", got)
	}
}
