// Package envtest provides an in-process git server toolkit for integration
// tests that run against a controller-runtime envtest environment.
//
// A GitServer wraps github.com/sosedoff/gitkit and exposes both HTTP and
// HTTPS listeners simultaneously. All repositories, users, and permissions
// are shared between the two listeners. Tests pick the protocol they need:
// plain HTTP (URL, RepoURL, FQDN) for most tests, and HTTPS (TLSURL,
// TLSRepoURL, TLSFQDN, CACert) for tests that exercise CA bundle handling.
//
// Per-user, per-repository access levels (NoAccess, ReadOnly, ReadWrite)
// are enforced in the auth function by inspecting the git smart HTTP
// service in the URL: git-upload-pack is a read operation, git-receive-pack
// is a write operation.
//
// Push attempts are counted per repository (PushAttempts,
// ResetPushAttempts), including the ones the access levels reject, so tests
// can assert on how many times a client retried a failing push.
//
// Two GitServers are intended to be started once in BeforeSuite and
// shared across parallel tests. Some are testing the target against
// two different url. Test isolation comes from each test creating
// uniquely-named repositories; the users/permissions maps are
// protected by a sync.RWMutex for safe concurrent access.
package envtest
