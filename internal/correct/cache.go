package correct

import (
	"encoding/json"
	"os"
	"sync"
)

// cachedRecord is a Record stripped of entity identity (Tool/RuleID are
// meaningful regardless of which entity the text came from; EntityType/
// EntityPath/Field get re-stamped with the current unit's identity on a
// cache hit — two different entities can share identical prose, e.g. the
// same jutsu's description reached via two different clans).
type cachedRecord struct {
	Tool, RuleID, Original, Corrected string
}

type cachedResult struct {
	CorrectedText string         `json:"corrected_text"`
	Records       []cachedRecord `json:"records"`
}

// Cache is a content-addressed, on-disk cache of LanguageTool results,
// keyed by sha256 of the (misspell+apostrophe-corrected) input text. Its
// purpose is purely performance: re-ingesting an unchanged PDF should cost
// zero LanguageTool requests on the second and later runs — only genuinely
// new or changed prose text triggers a network call.
type Cache struct {
	path  string
	mu    sync.Mutex
	data  map[string]cachedResult
	dirty bool
}

// LoadCache opens (or initializes) the cache file at path. A missing or
// corrupt file is not an error — the cache just starts empty and the
// caller pays for more LanguageTool traffic than usual, not a failed
// ingest.
func LoadCache(path string) *Cache {
	c := &Cache{path: path, data: map[string]cachedResult{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, &c.data) // corrupt cache -> start empty, same as missing
	return c
}

func (c *Cache) Get(text string) (cachedResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.data[contentHash(text)]
	return r, ok
}

func (c *Cache) Put(text string, result cachedResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[contentHash(text)] = result
	c.dirty = true
}

// Save writes the cache to disk if anything changed, via a temp-file-then-
// rename so a crash mid-write can't corrupt it.
func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return nil
	}
	b, err := json.Marshal(c.data)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return err
	}
	c.dirty = false
	return nil
}
