package correct

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func fastLanguageTool(baseURL string) *languageTool {
	lt := newLanguageTool(&http.Client{Timeout: 5 * time.Second}, baseURL)
	lt.lastCall = time.Now().Add(-time.Hour) // skip pacing in tests
	return lt
}

func TestLanguageTool_Check_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ltResponse{Matches: []ltMatch{
			{Offset: 5, Length: 3, Rule: struct {
				ID       string `json:"id"`
				Category struct {
					ID string `json:"id"`
				} `json:"category"`
			}{ID: "TEST_RULE", Category: struct {
				ID string `json:"id"`
			}{ID: "GRAMMAR"}}},
		}})
	}))
	defer srv.Close()

	lt := fastLanguageTool(srv.URL)
	matches, err := lt.check(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Offset != 5 {
		t.Errorf("matches = %+v", matches)
	}
}

func TestLanguageTool_Check_RetriesOn429ThenSucceeds(t *testing.T) {
	orig := minRequestInterval
	minRequestInterval = time.Millisecond
	defer func() { minRequestInterval = orig }()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(ltResponse{})
	}))
	defer srv.Close()

	lt := fastLanguageTool(srv.URL)
	_, err := lt.check(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestLanguageTool_Check_HardFailureReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	lt := fastLanguageTool(srv.URL)
	_, err := lt.check(context.Background(), "hello world")
	if err == nil {
		t.Fatal("want error on hard failure")
	}
}
