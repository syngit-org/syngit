package pusher

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
)

func newTestCache(maxRepos int) *repositoryCache {
	return &repositoryCache{
		maxRepos: maxRepos,
		entries:  make(map[string]*cacheEntry),
	}
}

// touch acquires key, populates it on a miss, and releases the lease.
func touch(c *repositoryCache, key string) {
	l := c.acquire(key)
	if l.repo() == nil {
		l.set(&git.Repository{})
	}
	l.release()
}

func TestRepositoryCacheLFUEviction(t *testing.T) {
	c := newTestCache(2)

	touch(c, "a")
	touch(c, "a") // a accessed more often than b
	touch(c, "b")

	// Adding a third repo must evict the least-frequently-used idle one (b).
	touch(c, "c")

	if _, ok := c.entries["b"]; ok {
		t.Errorf("expected least-used entry %q to be evicted", "b")
	}
	if _, ok := c.entries["a"]; !ok {
		t.Errorf("expected frequently-used entry %q to survive", "a")
	}
	if _, ok := c.entries["c"]; !ok {
		t.Errorf("expected newly-added entry %q to be present", "c")
	}
	if len(c.entries) != 2 {
		t.Errorf("cache size = %d, want 2", len(c.entries))
	}
}

func TestRepositoryCacheAging(t *testing.T) {
	c := newTestCache(5)

	touch(c, "a")
	touch(c, "b")

	c.entries["a"].useCount = useCountCap - 1
	c.entries["b"].useCount = 100

	// One more access pushes "a" to the cap and ages every counter by halving.
	touch(c, "a")

	wantA := uint32(useCountCap) / 2
	if got := c.entries["a"].useCount; got != wantA {
		t.Errorf("a.useCount = %d, want %d (halved on aging)", got, wantA)
	}
	if got := c.entries["b"].useCount; got != 50 {
		t.Errorf("b.useCount = %d, want 50 (halved on aging)", got)
	}
	// Relative ordering is preserved.
	if c.entries["a"].useCount <= c.entries["b"].useCount {
		t.Errorf("aging must preserve ordering: a=%d b=%d", c.entries["a"].useCount, c.entries["b"].useCount)
	}
}

func TestRepositoryCacheEvictionSkipsInUse(t *testing.T) {
	c := newTestCache(1)

	// Hold a lease on "a" so it is in use.
	la := c.acquire("a")
	la.set(&git.Repository{})

	// Acquiring "b" cannot evict the in-use "a"; the cap is exceeded instead.
	lb := c.acquire("b")
	lb.set(&git.Repository{})

	if _, ok := c.entries["a"]; !ok {
		t.Errorf("in-use entry %q must not be evicted", "a")
	}
	if _, ok := c.entries["b"]; !ok {
		t.Errorf("entry %q should be present", "b")
	}

	la.release()
	lb.release()

	// Once "a" is idle again, a new acquisition can evict it.
	touch(c, "c")
	if len(c.entries) != 1 {
		t.Errorf("cache size = %d, want 1 after idle eviction", len(c.entries))
	}
}

func TestRepositoryCacheAcquireSerializes(t *testing.T) {
	c := newTestCache(2)

	l1 := c.acquire("a")
	l1.set(&git.Repository{})

	done := make(chan struct{})
	go func() {
		l2 := c.acquire("a")
		close(done)
		l2.release()
	}()

	select {
	case <-done:
		t.Fatal("second acquire returned while the first lease was still held")
	case <-time.After(50 * time.Millisecond):
	}

	l1.release()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("second acquire did not proceed after the first lease was released")
	}
}

func TestRepositoryCacheDiscard(t *testing.T) {
	c := newTestCache(2)

	// A miss whose clone fails is discarded and must not linger.
	l := c.acquire("a")
	if l.repo() != nil {
		t.Fatalf("expected a cache miss for a fresh key")
	}
	l.discard()

	if _, ok := c.entries["a"]; ok {
		t.Errorf("discarded entry %q must be removed from the cache", "a")
	}

	// The next acquisition is a clean miss again.
	l2 := c.acquire("a")
	if l2.repo() != nil {
		t.Errorf("expected a cache miss after discard")
	}
	l2.release()
}

func TestInitRepoCache(t *testing.T) {
	defer InitRepoCache(0) // restore disabled state for other tests

	InitRepoCache(0)
	if repoCache != nil {
		t.Errorf("InitRepoCache(0) should disable caching (nil singleton)")
	}

	InitRepoCache(-3)
	if repoCache != nil {
		t.Errorf("InitRepoCache(<0) should disable caching (nil singleton)")
	}

	InitRepoCache(7)
	if repoCache == nil {
		t.Fatalf("InitRepoCache(7) should enable caching")
	}
	if repoCache.maxRepos != 7 {
		t.Errorf("maxRepos = %d, want 7", repoCache.maxRepos)
	}
}
