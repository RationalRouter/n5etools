package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/RationalRouter/n5e/internal/autoupdate"
	"github.com/RationalRouter/n5e/internal/sources"
)

// sourcebookServer is testServer with a sourcebook library holding only the
// core book.
func sourcebookServer(t *testing.T) (*server, []byte) {
	t.Helper()
	s := testServer(t)
	s.sourcebookDir = t.TempDir()
	body := []byte("%PDF-1.4\n" + strings.Repeat("x", 4096))
	if err := os.WriteFile(autoupdate.LibraryPath(s.sourcebookDir, "book/core"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return s, body
}

func TestSourcebooksListStatuses(t *testing.T) {
	s, _ := sourcebookServer(t)

	s.sourcebookChecking = true
	page := getPage(t, s, "/sourcebooks")
	if !strings.Contains(page, `href="/sourcebooks/core"`) {
		t.Error("the downloaded core book has no Open link")
	}
	if strings.Contains(page, `href="/sourcebooks/kage-guide"`) {
		t.Error("a book with no library copy is linked")
	}
	if !strings.Contains(page, "Downloading in the background") {
		t.Error("missing books aren't shown as downloading while the check runs")
	}
	if got := strings.Count(page, `class="sourcebook-tag"`); got != 2 {
		t.Errorf("GM tags = %d, want 2 (Kage Guide and Bingo Book Pack 1)", got)
	}

	s.finishSourcebookCheck([]autoupdate.Result{{Slug: "book/kage-guide", Action: "error", Err: errors.New("checking for updates: offline")}})
	page = getPage(t, s, "/sourcebooks")
	if strings.Contains(page, "Downloading in the background") {
		t.Error("still shown as downloading after the check finished")
	}
	if !strings.Contains(page, "Not downloaded yet") || !strings.Contains(page, "checking for updates: offline") {
		t.Error("a failed book doesn't show as unavailable with its error")
	}
}

func TestSourcebookView(t *testing.T) {
	s, _ := sourcebookServer(t)
	page := getPage(t, s, "/sourcebooks/core?page=12")
	if !strings.Contains(page, `data-src="/static/vendor/pdfjs/web/viewer.html?file=%2Fsourcebooks%2Fcore%2Ffile#page=12"`) {
		t.Errorf("viewer URL not rendered as expected:\n%s", page)
	}

	for path, want := range map[string]int{
		"/sourcebooks/kage-guide":      http.StatusSeeOther, // not downloaded
		"/sourcebooks/nope":            http.StatusNotFound,
		"/sourcebooks/nope/file":       http.StatusNotFound,
		"/sourcebooks/..%2Fx/file":     http.StatusNotFound,
		"/sourcebooks/kage-guide/file": http.StatusNotFound,
	} {
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != want {
			t.Errorf("GET %s: status %d, want %d", path, w.Code, want)
		}
	}
}

func TestSourcebookFileRanges(t *testing.T) {
	s, body := sourcebookServer(t)

	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sourcebooks/core/file", nil))
	if w.Code != http.StatusOK || w.Body.String() != string(body) {
		t.Fatalf("full GET: status %d, %d bytes", w.Code, w.Body.Len())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	if w.Header().Get("Accept-Ranges") != "bytes" {
		t.Error("no Accept-Ranges: bytes, so PDF.js can't load progressively")
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "Shinobi Hand Book.pdf") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	req := httptest.NewRequest(http.MethodGet, "/sourcebooks/core/file", nil)
	req.Header.Set("Range", "bytes=0-7")
	w = httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	if w.Code != http.StatusPartialContent || w.Body.String() != "%PDF-1.4" {
		t.Errorf("range GET: status %d, body %q", w.Code, w.Body.String())
	}
}

// TestSourcebookViewerEmbedded: the viewer and the files it loads are in
// the embedded filesystem (static.go's go:embed lists directories
// explicitly), and the nav links to the page.
func TestSourcebookViewerEmbedded(t *testing.T) {
	s, _ := sourcebookServer(t)
	for _, path := range []string{
		"/static/vendor/pdfjs/web/viewer.html",
		"/static/vendor/pdfjs/web/viewer.mjs",
		"/static/vendor/pdfjs/build/pdf.mjs",
		"/static/vendor/pdfjs/build/pdf.worker.mjs",
		"/static/vendor/pdfjs/web/locale/locale.json",
		"/static/vendor/pdfjs/web/locale/pt-BR/viewer.ftl",
		"/static/js/sourcebook-view.js",
	} {
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: status %d", path, w.Code)
		}
		// Module scripts are refused unless served with a JavaScript type.
		if strings.HasSuffix(path, ".mjs") && !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
			t.Errorf("GET %s: Content-Type %q, want JavaScript", path, w.Header().Get("Content-Type"))
		}
	}
	if !strings.Contains(getPage(t, s, "/"), `href="/sourcebooks"`) {
		t.Error("nav has no Sourcebooks link")
	}
	for _, b := range sources.OfficialBooks {
		if _, ok := sourcebookByKey(sourcebookKey(b.Slug)); !ok {
			t.Errorf("%s doesn't resolve by its own key", b.Slug)
		}
	}
}
