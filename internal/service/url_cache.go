package service

import (
	"sync"
	"time"
)

// URLCache stores presigned URLs keyed by an arbitrary string with a TTL.
// Implementations must be safe for concurrent use. Get returns false when the
// entry is missing or expired; the caller is expected to recompute and Set in
// that case.
type URLCache interface {
	Get(key string) (string, bool)
	Set(key, url string, ttl time.Duration)
}

type urlCacheEntry struct {
	url       string
	expiresAt time.Time
}

type inMemoryURLCache struct {
	entries sync.Map // map[string]urlCacheEntry
}

// NewInMemoryURLCache returns a process-local URLCache backed by sync.Map.
// Entries are evicted lazily on Get; there is no background sweeper because
// the working set tracks the live presigned URLs (one per object key) and
// stale entries are overwritten on the next Set.
func NewInMemoryURLCache() URLCache {
	return &inMemoryURLCache{}
}

func (c *inMemoryURLCache) Get(key string) (string, bool) {
	raw, ok := c.entries.Load(key)
	if !ok {
		return "", false
	}
	entry := raw.(urlCacheEntry)
	if time.Now().After(entry.expiresAt) {
		c.entries.Delete(key)
		return "", false
	}
	return entry.url, true
}

func (c *inMemoryURLCache) Set(key, url string, ttl time.Duration) {
	c.entries.Store(key, urlCacheEntry{
		url:       url,
		expiresAt: time.Now().Add(ttl),
	})
}
