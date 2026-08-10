// Package cache provides a small thread-safe in-memory cache with TTL,
// used to memoize adapter tool results so repeated/related queries reuse the
// same data instead of re-hitting the database (and re-burning tokens).
package cache

import (
	"sync"
	"time"
)

type entry struct {
	value  []byte
	expiry time.Time
}

// Cache is a TTL-keyed byte cache. Zero value is not usable; use New.
type Cache struct {
	mu   sync.Mutex
	ttl  time.Duration
	data map[string]entry
}

func New(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl, data: make(map[string]entry)}
}

// Get returns the cached value and whether it was a live hit. Expired entries
// are dropped on read.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiry) {
		delete(c.data, key)
		return nil, false
	}
	return e.value, true
}

func (c *Cache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = entry{value: value, expiry: time.Now().Add(c.ttl)}
}

func (c *Cache) TTL() time.Duration { return c.ttl }
