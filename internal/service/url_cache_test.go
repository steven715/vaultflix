package service

import (
	"sync"
	"testing"
	"time"
)

func TestURLCache_SetThenGet(t *testing.T) {
	c := NewInMemoryURLCache()
	c.Set("thumb:abc", "https://example.com/thumb", time.Minute)

	got, ok := c.Get("thumb:abc")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "https://example.com/thumb" {
		t.Errorf("expected stored URL, got %q", got)
	}
}

func TestURLCache_GetMiss(t *testing.T) {
	c := NewInMemoryURLCache()
	if _, ok := c.Get("never-set"); ok {
		t.Error("expected miss on absent key")
	}
}

func TestURLCache_Expired(t *testing.T) {
	c := NewInMemoryURLCache()
	c.Set("preview:expired", "https://example.com/preview", 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)

	if _, ok := c.Get("preview:expired"); ok {
		t.Error("expected miss after TTL")
	}
}

func TestURLCache_OverwriteExtendsTTL(t *testing.T) {
	c := NewInMemoryURLCache()
	c.Set("k", "v1", 10*time.Millisecond)
	time.Sleep(15 * time.Millisecond)

	// Overwrite resets the expiry; next Get should hit.
	c.Set("k", "v2", time.Minute)

	got, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit after overwrite")
	}
	if got != "v2" {
		t.Errorf("expected v2, got %q", got)
	}
}

func TestURLCache_Concurrent(t *testing.T) {
	c := NewInMemoryURLCache()
	const writers = 20
	const readers = 20
	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for i := 0; i < writers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				c.Set("k", "writer-"+string(rune('A'+id)), time.Minute)
			}
		}(i)
	}

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				_, _ = c.Get("k")
			}
		}()
	}

	wg.Wait()

	// After the storm a value must be present (TTL is 1 minute, well in the future).
	if _, ok := c.Get("k"); !ok {
		t.Error("expected cache to hold a value after concurrent storm")
	}
}
