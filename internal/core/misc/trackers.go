package misc

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// DefaultTrackersURL is the upstream tracker list source.
const DefaultTrackersURL = "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_best.txt"

// TrackersCache is a small in-process cache for the fetched tracker list. A
// fresh copy is fetched when the cache is missing or stale, or when a refresh
// is explicitly requested.
type TrackersCache struct {
	mu      sync.Mutex
	content string
	time    time.Time
	ttl     time.Duration
	fetch   func() (string, error)
}

// NewTrackersCache creates a cache backed by the given fetch function.
func NewTrackersCache(ttl time.Duration, fetch func() (string, error)) *TrackersCache {
	return &TrackersCache{ttl: ttl, fetch: fetch}
}

// DefaultTrackers returns a cache backed by DefaultTrackersURL with a 30
// minute TTL.
func DefaultTrackers() *TrackersCache {
	return NewTrackersCache(30*time.Minute, func() (string, error) {
		return fetchURL(DefaultTrackersURL)
	})
}

// Get returns the tracker list. If refresh is true or the cached value is
// missing or older than the TTL, it fetches a fresh copy and updates the cache.
// The cache is left untouched if the fetch fails.
func (c *TrackersCache) Get(refresh bool) (string, error) {
	c.mu.Lock()
	if !refresh && c.content != "" && time.Since(c.time) <= c.ttl {
		cached := c.content
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	content, err := c.fetch()
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.content = content
	c.time = time.Now()
	c.mu.Unlock()
	return content, nil
}

func fetchURL(url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	return string(body), nil
}
