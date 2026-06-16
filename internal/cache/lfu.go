// Package cache provides small, count-bounded in-memory caches shared across
// syngit. LFU is a generic least-frequently-used cache used as the storage layer
// for higher-level caches (e.g. the walker's resource->path cache).
package cache

import "sync"

// useCountCap is the aging threshold for the LFU counters. When any entry's
// useCount reaches it, every counter is halved so the values never overflow and
// once-popular-but-now-cold entries fade and become evictable again.
const useCountCap = 1 << 16

// LFU is a count-bounded, in-memory least-frequently-used cache. Every access
// increments the touched entry's useCount and the entry with the smallest
// useCount is evicted first once the cache exceeds its capacity.
//
// LFU is safe for concurrent use. A nil *LFU is a valid, permanently-disabled
// cache: all methods are no-ops (Get always misses), so callers can treat
// "caching disabled" as a nil cache without guarding every call site.
type LFU[K comparable, V any] struct {
	mu      sync.Mutex
	max     int
	entries map[K]*entry[V]
}

type entry[V any] struct {
	value    V
	useCount uint32 // LFU rank: ++ on every access, halved on aging
}

// NewLFU returns a cache holding at most max entries. A max of zero or less
// returns nil, which disables caching (every method becomes a no-op).
func NewLFU[K comparable, V any](max int) *LFU[K, V] {
	if max <= 0 {
		return nil
	}
	return &LFU[K, V]{
		max:     max,
		entries: make(map[K]*entry[V], max),
	}
}

// Get returns the value stored for key and whether it was present. A hit bumps
// the entry's LFU counter.
func (c *LFU[K, V]) Get(key K) (V, bool) {
	if c == nil {
		var zero V
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.touchLocked(e)
	return e.value, true
}

// Set stores value for key, replacing any existing value, and evicts the
// least-frequently-used entries until the cache is back within capacity.
func (c *LFU[K, V]) Set(key K, value V) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		e.value = value
		c.touchLocked(e)
		return
	}
	e := &entry[V]{value: value}
	c.entries[key] = e
	c.touchLocked(e)
	c.evictLocked(key)
}

// LoadOrStore returns the existing value for key (a load, which bumps its LFU
// counter) if present. Otherwise it stores value, evicts the least-frequently-
// used entries until the cache is back within capacity, and returns value. The
// bool reports whether the value was already present. On a nil cache it stores
// nothing and returns (value, false).
func (c *LFU[K, V]) LoadOrStore(key K, value V) (V, bool) {
	if c == nil {
		return value, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		c.touchLocked(e)
		return e.value, true
	}
	e := &entry[V]{value: value}
	c.entries[key] = e
	c.touchLocked(e)
	c.evictLocked(key)
	return value, false
}

// Delete removes key from the cache. It is a no-op when key is absent.
func (c *LFU[K, V]) Delete(key K) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// touchLocked bumps the LFU counter for e and ages all counters if the cap is
// reached. Must be called with c.mu held.
func (c *LFU[K, V]) touchLocked(e *entry[V]) {
	e.useCount++
	if e.useCount >= useCountCap {
		for _, other := range c.entries {
			other.useCount >>= 1
		}
	}
}

// evictLocked removes least-frequently-used entries until the cache is back to
// its capacity. exceptKey (the entry just inserted) is never chosen as a victim.
// Must be called with c.mu held.
func (c *LFU[K, V]) evictLocked(exceptKey K) {
	for len(c.entries) > c.max {
		var (
			victimKey K
			victim    *entry[V]
		)
		for k, e := range c.entries {
			if k == exceptKey {
				continue
			}
			if victim == nil || e.useCount < victim.useCount {
				victimKey, victim = k, e
			}
		}
		if victim == nil {
			return
		}
		delete(c.entries, victimKey)
	}
}
