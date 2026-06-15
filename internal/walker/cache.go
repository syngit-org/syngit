package walker

import "github.com/syngit-org/syngit/internal/cache"

// docCacheKey identifies a resolved lookup: a selector within a given scope. The
// scope is an opaque, caller-supplied identifier for "which worktree's content"
// (in practice the target repository URL + branch), so the same resource in
// different repositories never collides. ObjectSelector is comparable, so the
// whole key can be a map key.
type docCacheKey struct {
	Scope string
	Sel   ObjectSelector
}

// docCacheValue is what we remember about a key: the worktree-relative path that
// held the matching document the last time we walked. NoCache is set once a key
// has matched more than one file, in which case we never trust a single cached
// path and always fall back to a full walk so every copy keeps being rewritten.
type docCacheValue struct {
	Path    string
	NoCache bool
}

// docCache maps a (scope, selector) to the file path that satisfies it, letting
// ReplaceObject jump straight to the file instead of walking the whole worktree.
// A nil docCache disables caching; all cache.LFU methods are nil-safe.
var docCache *cache.LFU[docCacheKey, docCacheValue]

// InitDocumentCache (re)configures the package-level document path cache. A
// maxEntries of zero or less disables caching entirely.
func InitDocumentCache(maxEntries int) {
	docCache = cache.NewLFU[docCacheKey, docCacheValue](maxEntries)
}

// recordMatches updates the cache after a full walk for (scope, sel): exactly one
// match is remembered as the fast-path location, two-or-more matches mark the key
// as never-cache so it always full-walks, and zero matches drop any stale entry.
func recordMatches(scope string, sel ObjectSelector, matched []string) {
	key := docCacheKey{Scope: scope, Sel: sel}
	switch len(matched) {
	case 0:
		docCache.Delete(key)
	case 1:
		docCache.Set(key, docCacheValue{Path: matched[0]})
	default:
		docCache.Set(key, docCacheValue{NoCache: true})
	}
}
