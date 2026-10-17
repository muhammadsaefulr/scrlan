package scanner

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type hostnameCache struct {
	mu        sync.RWMutex
	entries   map[string]hostnameCacheEntry
	ttl       time.Duration
	resolving singleflight.Group
}

type hostnameCacheEntry struct {
	hostname  string
	expiresAt time.Time
}

func newHostnameCache(ttl time.Duration) *hostnameCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &hostnameCache{
		entries: make(map[string]hostnameCacheEntry),
		ttl:     ttl,
	}
}

func (c *hostnameCache) resolve(key string, lookup func() string) string {
	if hostname, found := c.get(key); found {
		return hostname
	}

	value, _, _ := c.resolving.Do(key, func() (any, error) {
		if hostname, found := c.get(key); found {
			return hostname, nil
		}
		hostname := lookup()
		if hostname != "" {
			c.set(key, hostname)
		}
		return hostname, nil
	})
	return value.(string)
}

func (c *hostnameCache) get(key string) (string, bool) {
	c.mu.RLock()
	entry, found := c.entries[key]
	c.mu.RUnlock()
	if !found || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.hostname, true
}

func (c *hostnameCache) set(key string, hostname string) {
	c.mu.Lock()
	c.entries[key] = hostnameCacheEntry{
		hostname:  hostname,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}
