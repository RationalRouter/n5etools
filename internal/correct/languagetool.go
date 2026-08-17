package correct

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultLanguageToolURL = "https://api.languagetool.org/v2/check"

// maxBatchChars stays safely under LanguageTool's ~20k char/request cap,
// leaving room for form-encoding overhead.
const maxBatchChars = 15000

// minRequestInterval paces requests under the documented ~20/min public
// rate limit; padding below the exact 3s/request line avoids clock-skew
// 429s. A var, not a const, so tests can shrink it instead of sleeping
// several real seconds per retry test.
var minRequestInterval = 3500 * time.Millisecond

// allowedLTCategories is an allowlist, not a denylist — safer for
// zero-review auto-apply. Notably excludes LanguageTool's own dictionary
// spellchecker (rule MORFOLOGIK_RULE_EN_US, category TYPOS): verified
// live against this corpus that it doesn't know jargon like "Katana" or
// "Odachi" and proposes garbage ("Katanga", "Kartana") — misspell already
// covers genuine typos without that risk. STYLE/CASING/REDUNDANCY and
// anything unrecognized are excluded by default-deny.
var allowedLTCategories = map[string]bool{
	"GRAMMAR":        true,
	"TYPOGRAPHY":     true,
	"PUNCTUATION":    true,
	"CONFUSED_WORDS": true,
	"COMPOUNDING":    true,
}

type ltMatch struct {
	Offset       int `json:"offset"` // rune offset into the submitted text, not byte
	Length       int `json:"length"`
	Replacements []struct {
		Value string `json:"value"`
	} `json:"replacements"`
	Rule struct {
		ID       string `json:"id"`
		Category struct {
			ID string `json:"id"`
		} `json:"category"`
	} `json:"rule"`
}

type ltResponse struct {
	Matches []ltMatch `json:"matches"`
}

func acceptMatch(m ltMatch) bool {
	return len(m.Replacements) > 0 && allowedLTCategories[m.Rule.Category.ID]
}

type languageTool struct {
	client   *http.Client
	baseURL  string
	mu       sync.Mutex
	lastCall time.Time
}

func newLanguageTool(client *http.Client, baseURL string) *languageTool {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if baseURL == "" {
		baseURL = defaultLanguageToolURL
	}
	return &languageTool{client: client, baseURL: baseURL}
}

// check sends one already size-bounded batch of text and returns raw
// matches, handling pacing and 429 backoff. It returns an error only on a
// hard, unrecoverable failure — the caller should stop calling check for
// the rest of this run, not retry per-batch forever.
//
// retryAfter from doCheck is trusted directly as the wait: a 429 always
// carries either the server's real Retry-After value or a safe 10s
// default (see doCheck), so there is no separate exponential-backoff floor
// to layer on top — the 4-attempt cap already bounds the worst case.
func (lt *languageTool) check(ctx context.Context, text string) ([]ltMatch, error) {
	for attempt := 0; attempt < 4; attempt++ {
		lt.pace()

		matches, retryAfter, err := lt.doCheck(ctx, text)
		if err == nil {
			return matches, nil
		}
		if retryAfter <= 0 {
			return nil, err
		}
		select {
		case <-time.After(retryAfter):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("languagetool: exceeded retry attempts")
}

func (lt *languageTool) pace() {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if wait := minRequestInterval - time.Since(lt.lastCall); wait > 0 {
		time.Sleep(wait)
	}
	lt.lastCall = time.Now()
}

// doCheck makes one HTTP call. A non-zero retryAfter alongside a non-nil
// err means "wait this long and retry"; a zero retryAfter with a non-nil
// err means "give up".
func (lt *languageTool) doCheck(ctx context.Context, text string) ([]ltMatch, time.Duration, error) {
	form := url.Values{"text": {text}, "language": {"en-US"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, lt.baseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := lt.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		// retryAfter must stay strictly positive here: check() uses
		// retryAfter <= 0 as the "hard failure, stop retrying" signal, so a
		// zero or malformed header falls back to the same 10s default as a
		// missing one, rather than being mistaken for a non-429 failure.
		retryAfter := 10 * time.Second
		if h := resp.Header.Get("Retry-After"); h != "" {
			if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		return nil, retryAfter, fmt.Errorf("languagetool: rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, 0, fmt.Errorf("languagetool: status %d: %s", resp.StatusCode, body)
	}

	var parsed ltResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, 0, fmt.Errorf("languagetool: decoding response: %w", err)
	}
	return parsed.Matches, 0, nil
}
