package pusher

import (
	"sync"

	git "github.com/go-git/go-git/v5"
)

// useCountCap is the aging threshold for the LFU counters. When any entry's
// useCount reaches it, every counter is halved so the values never overflow and
// once-popular-but-now-cold repositories fade and become evictable again.
const useCountCap = 1 << 16

// repositoryCache is a count-bounded, in-memory LFU cache of go-git
// repositories keyed by "<url>#<branch>". go-git repositories are not safe for
// concurrent use, so each entry carries its own mutex and is leased to a single
// caller at a time via acquire/release; the lease is held for the whole pipeline
// (clone/refresh -> worktree -> commit -> push).
//
// Eviction is least-frequently-used: every access increments the entry's
// useCount and the entry with the smallest useCount is evicted first. Whether an
// entry is currently in use does not influence the ranking; it only prevents
// eviction (an entry whose mutex is held is skipped).
type repositoryCache struct {
	mu       sync.Mutex
	maxRepos int
	entries  map[string]*cacheEntry
}

type cacheEntry struct {
	key      string
	repo     *git.Repository // nil until the first clone populates it
	useCount uint32          // LFU rank: ++ on every access, halved on aging
	mu       sync.Mutex      // serializes pipeline use; held for the whole lease
}

// repoCache is the package-level singleton consulted by getRepository. A nil
// value means caching is disabled and every request clones from scratch.
var repoCache *repositoryCache

// InitRepoCache (re)configures the package-level repository cache. A maxRepos of
// zero or less disables caching entirely.
func InitRepoCache(maxRepos int) {
	if maxRepos <= 0 {
		repoCache = nil
		return
	}
	repoCache = &repositoryCache{
		maxRepos: maxRepos,
		entries:  make(map[string]*cacheEntry, maxRepos),
	}
}

// repoLease is a borrowed reference to a cached repository. The caller holds the
// entry's mutex until it calls release (or discard) and must not use the
// repository afterwards.
type repoLease struct {
	cache *repositoryCache
	entry *cacheEntry
}

// acquire returns a lease for key, creating the entry if it does not exist. It
// blocks until any in-flight lease on the same key is released. The returned
// lease's repo() is nil on a cache miss (the caller must clone and call set).
func (c *repositoryCache) acquire(key string) *repoLease {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		entry = &cacheEntry{key: key}
		c.entries[key] = entry
	}
	c.touchLocked(entry)
	c.evictLocked(key)
	c.mu.Unlock()

	// Lock the entry only after releasing c.mu so the cache lock is never held
	// while waiting on an entry lock.
	entry.mu.Lock()
	return &repoLease{cache: c, entry: entry}
}

// touchLocked bumps the LFU counter for entry and ages all counters if the cap
// is reached. Must be called with c.mu held.
func (c *repositoryCache) touchLocked(entry *cacheEntry) {
	entry.useCount++
	if entry.useCount >= useCountCap {
		for _, e := range c.entries {
			e.useCount >>= 1
		}
	}
}

// evictLocked removes least-frequently-used entries until the cache is back to
// maxRepos. Must be called with c.mu held.
func (c *repositoryCache) evictLocked(exceptKey string) {
	for len(c.entries) > c.maxRepos {
		if !c.evictOneIdleLocked(exceptKey) {
			// Every candidate is currently in use; the cap is exceeded until one
			// of them is released.
			return
		}
	}
}

// evictOneIdleLocked evicts the least-frequently-used entry that is neither
// exceptKey (the entry being acquired) nor currently in use. An entry is "in
// use" when its mutex is held by a lease, detected with TryLock so eviction
// never blocks and never removes a repository out from under a running pipeline.
// It returns false when no evictable entry exists. Must be called with c.mu held.
func (c *repositoryCache) evictOneIdleLocked(exceptKey string) bool {
	var victim *cacheEntry
	for _, e := range c.entries {
		if e.key == exceptKey {
			continue
		}
		if !e.mu.TryLock() {
			continue // in use; cannot evict
		}
		if victim == nil || e.useCount < victim.useCount {
			if victim != nil {
				victim.mu.Unlock()
			}
			victim = e
		} else {
			e.mu.Unlock()
		}
	}
	if victim == nil {
		return false
	}
	delete(c.entries, victim.key)
	victim.mu.Unlock()
	return true
}

func (l *repoLease) repo() *git.Repository { return l.entry.repo }

func (l *repoLease) set(repo *git.Repository) { l.entry.repo = repo }

// release returns the leased repository to the cache. An in-use entry cannot
// have been evicted, so it is still in the map and is simply unlocked.
func (l *repoLease) release() {
	l.entry.mu.Unlock()
}

// discard drops a never-populated entry from the cache (used when a cache-miss
// clone fails) and releases the lease, so a later request retries cleanly.
func (l *repoLease) discard() {
	l.cache.mu.Lock()
	if l.entry.repo == nil {
		delete(l.cache.entries, l.entry.key)
	}
	l.cache.mu.Unlock()
	l.entry.mu.Unlock()
}
