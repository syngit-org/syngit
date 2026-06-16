package pusher

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
)

// touch acquires key, populates it on a miss, and releases the lease.
func touch(key string) {
	l := acquire(key)
	if l.repo() == nil {
		l.set(&git.Repository{})
	}
	l.release()
}

func TestRepositoryCacheAcquireSerializes(t *testing.T) {
	InitRepoCache(2)
	defer InitRepoCache(0)

	l1 := acquire("a")
	l1.set(&git.Repository{})

	done := make(chan struct{})
	go func() {
		l2 := acquire("a")
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
	InitRepoCache(2)
	defer InitRepoCache(0)

	// A miss whose clone fails is discarded and must not linger.
	l := acquire("a")
	if l.repo() != nil {
		t.Fatalf("expected a cache miss for a fresh key")
	}
	l.discard()

	if _, ok := repoCache.Get("a"); ok {
		t.Errorf("discarded entry %q must be removed from the cache", "a")
	}

	// The next acquisition is a clean miss again.
	l2 := acquire("a")
	if l2.repo() != nil {
		t.Errorf("expected a cache miss after discard")
	}
	l2.release()
}

func TestRepositoryCacheReuse(t *testing.T) {
	InitRepoCache(2)
	defer InitRepoCache(0)

	touch("a") // populate on a miss

	// A second acquisition of the same key reuses the cached repository.
	l := acquire("a")
	if l.repo() == nil {
		t.Errorf("expected a cache hit for a populated key")
	}
	l.release()
}

func TestInitRepoCache(t *testing.T) {
	defer InitRepoCache(0) // restore disabled state for other tests

	InitRepoCache(0)
	if repoCache != nil {
		t.Errorf("InitRepoCache(0) should disable caching (nil cache)")
	}

	InitRepoCache(-3)
	if repoCache != nil {
		t.Errorf("InitRepoCache(<0) should disable caching (nil cache)")
	}

	InitRepoCache(7)
	if repoCache == nil {
		t.Fatalf("InitRepoCache(7) should enable caching")
	}
}
