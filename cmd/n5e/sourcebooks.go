package main

import (
	"errors"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/RationalRouter/n5e/internal/autoupdate"
	"github.com/RationalRouter/n5e/internal/i18n"
	"github.com/RationalRouter/n5e/internal/sources"
	"github.com/RationalRouter/n5e/internal/store"
)

// The in-app sourcebook reader. Each official book's PDF is the copy the
// startup auto-updater keeps in s.sourcebookDir (see internal/autoupdate's
// package doc), shown through the vendored PDF.js viewer under
// static/vendor/pdfjs.

// sourcebookStatus is one book's state on the list page.
type sourcebookStatus string

const (
	sourcebookAvailable   sourcebookStatus = "available"
	sourcebookDownloading sourcebookStatus = "downloading"
	sourcebookUnavailable sourcebookStatus = "unavailable"
)

// sourcebookRow is one card on the list page.
type sourcebookRow struct {
	Key, Title string
	GMFacing   bool
	Updated    string // Drive's last-modified date, "" when never recorded
	Status     sourcebookStatus
	Err        string // the last check's error, for an unavailable book
}

// sourcebookByKey resolves a URL key ("core") to its official book. Only
// keys derived from sources.OfficialBooks resolve, so a request can never
// name an arbitrary file.
func sourcebookByKey(key string) (sources.BookSource, bool) {
	for _, b := range sources.OfficialBooks {
		if sourcebookKey(b.Slug) == key {
			return b, true
		}
	}
	return sources.BookSource{}, false
}

// sourcebookKey is a book's URL key: the last segment of its slug, which is
// also its library file's base name.
func sourcebookKey(slug string) string {
	name := autoupdate.LibraryFileName(slug)
	return name[:len(name)-len(".pdf")]
}

// finishSourcebookCheck records the outcome of the startup auto-update run
// so the list page can tell "still downloading" from "failed".
func (s *server) finishSourcebookCheck(results []autoupdate.Result) {
	s.sourcebookMu.Lock()
	defer s.sourcebookMu.Unlock()
	s.sourcebookChecking = false
	s.sourcebookErrs = map[string]error{}
	for _, r := range results {
		if r.Err != nil {
			s.sourcebookErrs[r.Slug] = r.Err
		}
	}
}

func (s *server) sourcebookFileExists(slug string) bool {
	if s.sourcebookDir == "" {
		return false
	}
	_, err := os.Stat(autoupdate.LibraryPath(s.sourcebookDir, slug))
	return err == nil
}

// handleSourcebooks lists every official book with whether it can be opened.
func (s *server) handleSourcebooks(w http.ResponseWriter, r *http.Request) {
	s.sourcebookMu.Lock()
	checking := s.sourcebookChecking
	errs := s.sourcebookErrs
	s.sourcebookMu.Unlock()

	rows := make([]sourcebookRow, 0, len(sources.OfficialBooks))
	for _, b := range sources.OfficialBooks {
		row := sourcebookRow{Key: sourcebookKey(b.Slug), Title: b.Title, GMFacing: !b.Required}
		if t, found, err := store.GetSourceBookLastModified(s.rulesDB, b.Slug); err != nil {
			log.Println("sourcebook last-modified:", b.Slug, err)
		} else if found {
			row.Updated = t.Format("2006-01-02")
		}
		switch {
		case s.sourcebookFileExists(b.Slug):
			row.Status = sourcebookAvailable
		case checking:
			row.Status = sourcebookDownloading
		default:
			row.Status = sourcebookUnavailable
			if err := errs[b.Slug]; err != nil {
				row.Err = err.Error()
			}
		}
		rows = append(rows, row)
	}
	tr := i18n.Translator(localeFromContext(r.Context()))
	s.render(w, r, "sourcebooks.html", map[string]any{
		"Title": tr("sourcebooks.title"), "Books": rows,
	})
}

// pdfjsLocales maps the app's locales to the PDF.js viewer locales
// vendored under static/vendor/pdfjs/web/locale.
var pdfjsLocales = map[string]string{"en": "en-US", "es": "es-ES", "pt": "pt-BR"}

// handleSourcebookView shows one book in the PDF.js viewer. ?page=N opens it
// at that page.
func (s *server) handleSourcebookView(w http.ResponseWriter, r *http.Request) {
	book, ok := sourcebookByKey(r.PathValue("book"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !s.sourcebookFileExists(book.Slug) {
		http.Redirect(w, r, "/sourcebooks", http.StatusSeeOther)
		return
	}
	viewer := "/static/vendor/pdfjs/web/viewer.html?file=" +
		url.QueryEscape("/sourcebooks/"+sourcebookKey(book.Slug)+"/file")
	if page, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && page > 0 {
		viewer += "#page=" + strconv.Itoa(page)
	}
	pdfjsLocale, ok := pdfjsLocales[localeFromContext(r.Context())]
	if !ok {
		pdfjsLocale = "en-US"
	}
	s.render(w, r, "sourcebook_view.html", map[string]any{
		"Title": book.Title, "ViewerURL": viewer, "PDFJSLocale": pdfjsLocale,
	})
}

// handleSourcebookFile serves a book's PDF. http.ServeContent answers the
// Range requests PDF.js uses to load large books progressively.
func (s *server) handleSourcebookFile(w http.ResponseWriter, r *http.Request) {
	book, ok := sourcebookByKey(r.PathValue("book"))
	if !ok || s.sourcebookDir == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(autoupdate.LibraryPath(s.sourcebookDir, book.Slug))
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "couldn't open sourcebook", http.StatusInternalServerError)
		log.Println("open sourcebook:", err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "couldn't open sourcebook", http.StatusInternalServerError)
		log.Println("stat sourcebook:", err)
		return
	}
	// Replaces the global no-store: the file only changes when the
	// auto-updater replaces it, so revalidating against its modtime lets
	// the browser reuse a 40 MB book instead of re-reading it every visit.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "application/pdf")
	// Names the file the viewer's Download button saves.
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": book.Title + ".pdf"}))
	http.ServeContent(w, r, "", info.ModTime(), f)
}
