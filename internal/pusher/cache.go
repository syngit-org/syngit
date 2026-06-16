package pusher

import (
	"sync"

	git "github.com/go-git/go-git/v5"
	"github.com/syngit-org/syngit/internal/cache"
)

// repoHandle holds a single cached go-git repository. go-git repositories are
// not safe for concurrent use, so each handle carries its own mutex and is
// leased to a single caller at a time via acquire/release; the lease is held
// for the whole pipeline (clone/refresh -> worktree -> commit -> push).
type repoHandle struct {
	repo *git.Repository // nil until the first clone populates it
	mu   sync.Mutex      // serializes pipeline use; held for the whole lease
}

// repoCache is the package-level repository cache, keyed by "<url>#<branch>".
// LFU ranking and eviction are delegated to the generic cache.LFU. A nil
// cache means caching is disabled (every request clones from scratch);
// cache.LFU methods are nil-safe.
var repoCache *cache.LFU[string, *repoHandle]

// InitRepoCache (re)configures the package-level repository cache. A maxRepos of
// zero or less disables caching entirely.
func InitRepoCache(maxRepos int) {
	repoCache = cache.NewLFU[string, *repoHandle](maxRepos)
}

// repoLease is a borrowed reference to a cached repository. The caller holds the
// handle's mutex until it calls release (or discard) and must not use the
// repository afterwards.
type repoLease struct {
	key    string
	handle *repoHandle
}

// acquire returns a lease for key, creating the handle on a miss. Concurrent
// callers for the same key share one handle (and thus one clone) and serialize
// on its mutex. A handle that is evicted while still leased is finished safely:
// each handle owns an independent in-memory repository, so the worst case is a
// redundant re-clone, never two callers touching the same go-git repository.
// The returned lease's repo() is nil on a cache miss (the caller must clone and
// call set).
func acquire(key string) *repoLease {
	handle, _ := repoCache.LoadOrStore(key, &repoHandle{})
	handle.mu.Lock()
	return &repoLease{key: key, handle: handle}
}

func (l *repoLease) repo() *git.Repository { return l.handle.repo }

func (l *repoLease) set(repo *git.Repository) { l.handle.repo = repo }

// release returns the leased repository to the cache by unlocking its handle.
func (l *repoLease) release() {
	l.handle.mu.Unlock()
}

// discard drops a never-populated handle from the cache (used when a cache-miss
// clone fails) and releases the lease, so a later request retries cleanly.
func (l *repoLease) discard() {
	if l.handle.repo == nil {
		repoCache.Delete(l.key)
	}
	l.handle.mu.Unlock()
}
