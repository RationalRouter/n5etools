package i18n

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sprintfVerb matches a single Go fmt verb (%s, %d, %v, %.2f, ...) -- the
// catalog stores these inline (e.g. "Drag to reorder %s") for values the Go
// template layer fills in at render time. DeepL must never rephrase or
// reorder these, so ProtectPlaceholders swaps each one for a self-closing
// tag DeepL is told to ignore (tag_handling: html, ignore_tags: ["ph"] on
// Translate above) before the string is sent, and RestorePlaceholders swaps
// the original verb back in afterward.
//
// Two deliberate restrictions versus the naive "%[flags]*[width]*[verb
// letter]" pattern, both found live while building the verify QA pass
// (internal/i18n/verify.go's PlaceholdersMatch) against real translated
// game content -- content prose (unlike the curated UI catalog) routinely
// contains a bare "%" for an actual percentage ("Under 50% hit points"),
// which a too-permissive verb pattern matches as a bogus fmt verb and both
// mis-protects during translation (splitting "hit" into "<ph>% h</ph>it",
// handing DeepL a word interrupted mid-token) and mis-flags during verify
// (comparing a real English "%s"-shaped placeholder count against zero real
// placeholders that happen to share no verb letter):
//
//  1. No literal space in the flag class -- this project's own catalog
//     (grep confirmed: only %s/%d/%v ever appear, zero uses of Go's
//     sign-space flag) never needs it, and a raw "N% word" is exactly
//     "%" + space-as-flag + a verb-shaped first letter of the next word.
//  2. The verb letter is restricted to Go's actual fmt.Sprintf verb set,
//     not "any letter" -- narrows, though doesn't eliminate, the residual
//     risk of a percentage directly followed by no space (e.g. "50%increase").
var sprintfVerb = regexp.MustCompile(`%[-+0#]*[0-9]*\.?[0-9]*[vTtbcdoOqxXUeEfFgGsp%]`)

// PlaceholderTokens returns every fmt-verb-shaped token in s, in order --
// exported so internal/i18n/verify.go's language-agnostic placeholder-
// integrity check uses the exact same pattern ProtectPlaceholders protects,
// rather than a second, potentially-drifted copy of it.
func PlaceholderTokens(s string) []string {
	return sprintfVerb.FindAllString(s, -1)
}

// ProtectPlaceholders replaces every Sprintf verb in s with an
// ignore-tagged placeholder DeepL won't translate or move, returning the
// protected string. Pass the same string to RestorePlaceholders after
// translation to put the original verbs back.
//
// Translate sends text with tag_handling: html, so the plain text is
// HTML-escaped first: a literal "&" or "<" in the English would otherwise be
// parsed as markup. RestorePlaceholders decodes the entities again.
func ProtectPlaceholders(s string) string {
	return sprintfVerb.ReplaceAllStringFunc(html.EscapeString(s), func(verb string) string {
		return "<ph>" + verb + "</ph>"
	})
}

var phTag = regexp.MustCompile(`<ph>(.*?)</ph>`)

// RestorePlaceholders reverses ProtectPlaceholders on a translated string --
// DeepL passes ignored tags and their contents through untouched (that's
// the whole point of ignore_tags), so this strips the wrapper back off. It
// then decodes HTML entities: in html mode DeepL returns escaped text
// (e.g. "C&#x27;s"), which would otherwise be stored and shown verbatim.
func RestorePlaceholders(s string) string {
	return html.UnescapeString(phTag.ReplaceAllString(s, "$1"))
}

// DeepLClient is a small client for the parts of the DeepL API this project
// needs: translating batches of strings and managing a short-lived glossary
// that pins terms (proper nouns, established loanwords, approved renderings)
// so they aren't touched by machine translation. Modeled loosely on
// internal/correct's LanguageTool integration (retry on 429 using the
// server's own Retry-After, injectable *http.Client for the base URL).
type DeepLClient struct {
	client  *http.Client
	baseURL string // e.g. https://api-free.deepl.com/v2 (no trailing slash)
	apiKey  string
}

// NewDeepLClient builds a client. pro selects the Pro API host
// (api.deepl.com) instead of the Free one (api-free.deepl.com); the two
// plans use different hosts even though the request/response shapes are
// identical.
func NewDeepLClient(apiKey string, pro bool) *DeepLClient {
	host := "https://api-free.deepl.com/v2"
	if pro {
		host = "https://api.deepl.com/v2"
	}
	return &DeepLClient{
		client:  &http.Client{Timeout: 60 * time.Second},
		baseURL: host,
		apiKey:  apiKey,
	}
}

// maxTextsPerRequest is DeepL's own cap on how many strings one /translate
// call can carry.
const maxTextsPerRequest = 50

// Translate translates texts from English into targetLang (a DeepL language
// code, e.g. "ES"), optionally constrained by a glossary (glossaryID may be
// empty). Batches internally to stay under DeepL's 50-texts-per-request cap;
// the returned slice is the same length and order as texts on success.
//
// On error (e.g. quota exhausted partway through), the returned slice still
// holds every batch that completed successfully before the failure --
// DeepL bills each completed batch regardless of whether a later one in the
// same call fails, so a caller that discarded partial results on error
// would be paying for translations it never gets to keep. Check
// len(result) against len(texts) to see how far it got.
func (c *DeepLClient) Translate(ctx context.Context, texts []string, targetLang, glossaryID string) ([]string, error) {
	return c.TranslateFrom(ctx, texts, "EN", targetLang, glossaryID)
}

// TranslateFrom is Translate generalized to an arbitrary source language --
// used for the back-translation QA check (internal/i18n's verify machinery
// translates already-translated target-language text back to English to
// get a language-agnostic drift signal), where the source is whatever
// locale was just translated into, not always English.
func (c *DeepLClient) TranslateFrom(ctx context.Context, texts []string, sourceLang, targetLang, glossaryID string) ([]string, error) {
	out := make([]string, 0, len(texts))
	for start := 0; start < len(texts); start += maxTextsPerRequest {
		end := min(start+maxTextsPerRequest, len(texts))
		batch, err := c.translateBatch(ctx, texts[start:end], sourceLang, targetLang, glossaryID)
		if err != nil {
			return out, fmt.Errorf("deepl: translating batch %d-%d: %w", start, end, err)
		}
		out = append(out, batch...)
	}
	return out, nil
}

type deeplTranslateRequest struct {
	Text        []string `json:"text"`
	SourceLang  string   `json:"source_lang"`
	TargetLang  string   `json:"target_lang"`
	GlossaryID  string   `json:"glossary_id,omitempty"`
	TagHandling string   `json:"tag_handling,omitempty"`
	IgnoreTags  []string `json:"ignore_tags,omitempty"`
}

type deeplTranslateResponse struct {
	Translations []struct {
		Text string `json:"text"`
	} `json:"translations"`
}

func (c *DeepLClient) translateBatch(ctx context.Context, texts []string, sourceLang, targetLang, glossaryID string) ([]string, error) {
	body := deeplTranslateRequest{
		Text:        texts,
		SourceLang:  sourceLang,
		TargetLang:  targetLang,
		GlossaryID:  glossaryID,
		TagHandling: "html",
		IgnoreTags:  []string{"ph"},
	}
	var resp deeplTranslateResponse
	if err := c.do(ctx, http.MethodPost, "/translate", body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Translations) != len(texts) {
		return nil, fmt.Errorf("deepl: expected %d translations, got %d", len(texts), len(resp.Translations))
	}
	out := make([]string, len(resp.Translations))
	for i, t := range resp.Translations {
		out[i] = t.Text
	}
	return out, nil
}

type deeplGlossaryRequest struct {
	Name          string `json:"name"`
	SourceLang    string `json:"source_lang"`
	TargetLang    string `json:"target_lang"`
	Entries       string `json:"entries"`
	EntriesFormat string `json:"entries_format"`
}

type deeplGlossaryResponse struct {
	GlossaryID string `json:"glossary_id"`
}

// CreateGlossary uploads entries (English term -> target-language term; an
// identity pair where both sides are equal tells DeepL to leave that term
// untouched, e.g. for proper nouns and loanwords) as a new DeepL glossary
// and returns its id. Glossaries are immutable once created; callers should
// create one per run and delete it afterward via DeleteGlossary rather than
// trying to update or reuse one across runs.
func (c *DeepLClient) CreateGlossary(ctx context.Context, name, targetLang string, entries map[string]string) (string, error) {
	var tsv strings.Builder
	for source, target := range entries {
		fmt.Fprintf(&tsv, "%s\t%s\n", source, target)
	}
	body := deeplGlossaryRequest{
		Name:          name,
		SourceLang:    "EN",
		TargetLang:    targetLang,
		Entries:       tsv.String(),
		EntriesFormat: "tsv",
	}
	var resp deeplGlossaryResponse
	if err := c.do(ctx, http.MethodPost, "/glossaries", body, &resp); err != nil {
		return "", fmt.Errorf("deepl: creating glossary: %w", err)
	}
	return resp.GlossaryID, nil
}

// DeleteGlossary removes a glossary created by CreateGlossary. Safe to call
// even if the glossary is already gone -- a 404 here is not the caller's
// problem, it just means there's nothing left to clean up.
func (c *DeepLClient) DeleteGlossary(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/glossaries/"+id, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+c.apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("deepl: delete glossary: status %d: %s", resp.StatusCode, b)
	}
	return nil
}

// maxRetryAttempts is do's total retry budget. Unlike the old single
// fixed-5s-then-give-up attempt (which only ever retried a 429), this covers
// 429/5xx/network-level failures alike -- a translation run that was
// silently failing on the first transient blip is exactly the "not fire and
// forget" complaint this hardening addresses; see
// translation_methodology.md's "Automated coverage pipeline" section.
const maxRetryAttempts = 5

// baseRetryDelay is the starting point for do's exponential backoff
// (doubled each attempt, capped at maxRetryDelay, plus up to 50% jitter so
// concurrent callers don't all retry in lockstep).
const baseRetryDelay = 2 * time.Second
const maxRetryDelay = 30 * time.Second

// glossaryInfo is one entry from GET /v2/glossaries.
type glossaryInfo struct {
	GlossaryID string `json:"glossary_id"`
	Name       string `json:"name"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

type listGlossariesResponse struct {
	Glossaries []glossaryInfo `json:"glossaries"`
}

// ListGlossaries returns every glossary currently stored on this DeepL
// account.
func (c *DeepLClient) ListGlossaries(ctx context.Context) ([]glossaryInfo, error) {
	var resp listGlossariesResponse
	if err := c.doGet(ctx, "/glossaries", &resp); err != nil {
		return nil, fmt.Errorf("deepl: listing glossaries: %w", err)
	}
	return resp.Glossaries, nil
}

func (c *DeepLClient) doGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+c.apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// glossarySourceLang is the fixed source language every glossary in this
// project is built against (glossary.md's "English" column).
const glossarySourceLang = "EN"

// hashGlossaryEntries returns a short, stable content hash of entries,
// independent of map iteration order -- used to detect whether a persistent
// glossary's entries have changed since it was last created.
func hashGlossaryEntries(entries map[string]string) string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s\x00%s\x00", k, entries[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// GetOrCreatePersistentGlossary returns the id of a DeepL glossary named
// baseName for targetLang whose entries exactly match entries, creating one
// if needed and deleting any stale version left over from a previous run
// with different entries. DeepL glossaries are immutable once created (no
// update endpoint), so "persistent" here means "reused across runs when
// unchanged, replaced in place when glossary.md has changed" rather than
// the old per-run create-then-delete-unconditionally behavior -- this is
// the piece of DeepL's own glossary management (named, listable,
// account-level glossaries) the pipeline previously ignored in favor of
// reinventing a throwaway one every single invocation. See
// translation_methodology.md's "Automated coverage pipeline" section.
func (c *DeepLClient) GetOrCreatePersistentGlossary(ctx context.Context, baseName, targetLang string, entries map[string]string) (string, error) {
	if len(entries) == 0 {
		return "", nil
	}
	wantName := baseName + "-" + hashGlossaryEntries(entries)

	existing, err := c.ListGlossaries(ctx)
	if err != nil {
		return "", err
	}
	for _, g := range existing {
		if g.Name == wantName && g.SourceLang == glossarySourceLang && g.TargetLang == targetLang {
			return g.GlossaryID, nil // unchanged since last run -- reuse it
		}
	}
	// Clean up stale versions (same base name, different content hash) so
	// the account doesn't accumulate one abandoned glossary per edit to
	// glossary.md.
	for _, g := range existing {
		if strings.HasPrefix(g.Name, baseName+"-") && g.TargetLang == targetLang {
			_ = c.DeleteGlossary(ctx, g.GlossaryID)
		}
	}
	return c.CreateGlossary(ctx, wantName, targetLang, entries)
}

// glossaryLanguagePairsResponse is GET /v2/glossary-language-pairs' shape.
type glossaryLanguagePairsResponse struct {
	SupportedLanguages []struct {
		SourceLang string `json:"source_lang"`
		TargetLang string `json:"target_lang"`
	} `json:"supported_languages"`
}

// GlossarySupported reports whether DeepL's glossary feature supports the
// given source/target language pair at all -- not every target language
// among the 32 this project can translate into via -translate actually
// supports a glossary; translating to one that doesn't used to just log a
// swallowed warning (cmd/n5e-i18n/main.go's old "could not parse glossary"
// message covered a different failure, not this one) and silently proceed
// unconstrained. Callers should check this once per run and surface it in
// the run's report rather than finding out only by the glossary create call
// failing.
func (c *DeepLClient) GlossarySupported(ctx context.Context, sourceLang, targetLang string) (bool, error) {
	var resp glossaryLanguagePairsResponse
	if err := c.doGet(ctx, "/glossary-language-pairs", &resp); err != nil {
		return false, fmt.Errorf("deepl: checking glossary language pairs: %w", err)
	}
	for _, p := range resp.SupportedLanguages {
		if strings.EqualFold(p.SourceLang, sourceLang) && strings.EqualFold(p.TargetLang, targetLang) {
			return true, nil
		}
	}
	return false, nil
}

// do sends one JSON request and decodes a JSON response, retrying on a 429
// (honoring the server's Retry-After header when present), a 5xx, or a
// network-level error (timeout, connection reset, DNS failure) up to
// maxRetryAttempts times with exponential backoff. A 4xx other than 429 is
// never retried -- it means the request itself is wrong (bad glossary id,
// malformed body) and retrying would just waste the same attempt budget on
// a request that can't succeed.
func (c *DeepLClient) do(ctx context.Context, method, path string, body, out any) error {
	var lastErr error
	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if attempt > 0 {
			wait := retryDelay(attempt)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "DeepL-Auth-Key "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			// Network-level failure (timeout, connection reset, DNS): retry.
			lastErr = fmt.Errorf("deepl: request failed: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if h := resp.Header.Get("Retry-After"); h != "" {
				if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
					lastErr = fmt.Errorf("deepl: rate limited (429)")
					resp.Body.Close()
					select {
					case <-time.After(time.Duration(secs) * time.Second):
					case <-ctx.Done():
						return ctx.Err()
					}
					continue
				}
			}
			lastErr = fmt.Errorf("deepl: rate limited (429)")
			resp.Body.Close()
			continue
		}

		if resp.StatusCode >= 500 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			lastErr = fmt.Errorf("deepl: status %d: %s", resp.StatusCode, b)
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return fmt.Errorf("deepl: status %d: %s", resp.StatusCode, b)
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return fmt.Errorf("deepl: exceeded %d retry attempts, last error: %w", maxRetryAttempts, lastErr)
}

// retryDelay returns the exponential-backoff-with-jitter wait before the
// given (1-indexed) retry attempt.
func retryDelay(attempt int) time.Duration {
	d := baseRetryDelay * time.Duration(1<<uint(attempt-1))
	if d > maxRetryDelay {
		d = maxRetryDelay
	}
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	return d + jitter
}
