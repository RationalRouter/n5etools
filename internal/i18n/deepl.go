package i18n

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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
var sprintfVerb = regexp.MustCompile(`%[-+ 0#]*[0-9]*\.?[0-9]*[a-zA-Z%]`)

// ProtectPlaceholders replaces every Sprintf verb in s with an
// ignore-tagged placeholder DeepL won't translate or move, returning the
// protected string. Pass the same string to RestorePlaceholders after
// translation to put the original verbs back.
func ProtectPlaceholders(s string) string {
	return sprintfVerb.ReplaceAllStringFunc(s, func(verb string) string {
		return "<ph>" + verb + "</ph>"
	})
}

var phTag = regexp.MustCompile(`<ph>(.*?)</ph>`)

// RestorePlaceholders reverses ProtectPlaceholders on a translated string --
// DeepL passes ignored tags and their contents through untouched (that's
// the whole point of ignore_tags), so this just strips the wrapper back off.
func RestorePlaceholders(s string) string {
	return phTag.ReplaceAllString(s, "$1")
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
	out := make([]string, 0, len(texts))
	for start := 0; start < len(texts); start += maxTextsPerRequest {
		end := min(start+maxTextsPerRequest, len(texts))
		batch, err := c.translateBatch(ctx, texts[start:end], targetLang, glossaryID)
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

func (c *DeepLClient) translateBatch(ctx context.Context, texts []string, targetLang, glossaryID string) ([]string, error) {
	body := deeplTranslateRequest{
		Text:        texts,
		SourceLang:  "EN",
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

// do sends one JSON request and decodes a JSON response, retrying once on a
// 429 using the server's Retry-After header (defaulting to 5s if absent or
// malformed) -- DeepL's own rate limits are generous, so a single retry is
// enough for the batch sizes this project ever sends.
func (c *DeepLClient) do(ctx context.Context, method, path string, body, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
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
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := 5 * time.Second
			if h := resp.Header.Get("Retry-After"); h != "" {
				if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
					wait = time.Duration(secs) * time.Second
				}
			}
			resp.Body.Close()
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			return fmt.Errorf("deepl: status %d: %s", resp.StatusCode, b)
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return fmt.Errorf("deepl: exceeded retry attempts")
}
