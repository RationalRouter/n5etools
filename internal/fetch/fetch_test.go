package fetch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDriveFileID(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{"drive_link query", "https://drive.google.com/file/d/1lSqTKAbzWptSUhov9iHsslxk8H-B3W5b/view?usp=drive_link", "1lSqTKAbzWptSUhov9iHsslxk8H-B3W5b", false},
		{"sharing query", "https://drive.google.com/file/d/1wOsPeLLN85kQ2knL9slxyoZCL6VepR22/view?usp=sharing", "1wOsPeLLN85kQ2knL9slxyoZCL6VepR22", false},
		{"no query", "https://drive.google.com/file/d/1gbs5MxYxfalxSNtr1fVGF3SD-S6-xT15/view", "1gbs5MxYxfalxSNtr1fVGF3SD-S6-xT15", false},
		{"not a drive URL", "https://example.com/whatever", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DriveFileID(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("DriveFileID(%q) = %q, nil; want error", tc.url, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DriveFileID(%q) unexpected error: %v", tc.url, err)
			}
			if got != tc.want {
				t.Errorf("DriveFileID(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestHeadLastModified(t *testing.T) {
	want := time.Date(2025, 5, 4, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("got method %s, want HEAD", r.Method)
		}
		w.Header().Set("Last-Modified", want.Format(http.TimeFormat))
	}))
	defer srv.Close()

	got, err := headLastModified(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("headLastModified: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("headLastModified = %v, want %v", got, want)
	}
}

func TestHeadLastModifiedMissingHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	if _, err := headLastModified(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected an error for a response with no Last-Modified header, got nil")
	}
}

func TestHeadLastModifiedBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := headLastModified(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected an error for a non-200 status, got nil")
	}
}

func TestDownloadTo(t *testing.T) {
	body := []byte("%PDF-1.4\nfake pdf content for a test\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	destPath := filepath.Join(t.TempDir(), "book.pdf")
	gotSHA, err := downloadTo(context.Background(), srv.Client(), srv.URL, destPath)
	if err != nil {
		t.Fatalf("downloadTo: %v", err)
	}

	wantSHA := fmt.Sprintf("%x", sha256.Sum256(body))
	if gotSHA != wantSHA {
		t.Errorf("sha256 = %s, want %s", gotSHA, wantSHA)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("downloaded content = %q, want %q", got, body)
	}
}

func TestDownloadToRejectsNonPDF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>Google Drive - Virus scan warning</body></html>"))
	}))
	defer srv.Close()

	destPath := filepath.Join(t.TempDir(), "book.pdf")
	if _, err := downloadTo(context.Background(), srv.Client(), srv.URL, destPath); err == nil {
		t.Fatal("expected an error for a non-PDF response, got nil")
	}
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("expected no file to be created for a rejected non-PDF response, but %s exists", destPath)
	}
}

func TestDownloadToBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	destPath := filepath.Join(t.TempDir(), "book.pdf")
	if _, err := downloadTo(context.Background(), srv.Client(), srv.URL, destPath); err == nil {
		t.Fatal("expected an error for a non-200 status, got nil")
	}
}
