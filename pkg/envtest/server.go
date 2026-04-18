package envtest

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/sosedoff/gitkit"
)

// AccessLevel defines the git access for a user on a repository.
type AccessLevel int

const (
	// NoAccess denies all git operations (clone and push).
	NoAccess AccessLevel = iota
	// ReadOnly allows clone/fetch but denies push.
	ReadOnly
	// ReadWrite allows clone/fetch and push.
	ReadWrite
)

// GitUser represents a git user with basic-auth credentials.
type GitUser struct {
	Username string
	Password string
	Email    string
}

// RepoRef identifies a repository by owner and name.
type RepoRef struct {
	Owner string
	Name  string
}

// String returns "owner/name".
func (r RepoRef) String() string {
	return r.Owner + "/" + r.Name
}

// GitServer is an in-process git server for integration tests. It starts
// both an HTTP and an HTTPS listener backed by a single gitkit handler,
// sharing the same repositories, users, and permissions between the two.
type GitServer struct {
	dir string

	handler    *gitkit.Server
	httpServer *httptest.Server
	tlsServer  *httptest.Server

	mu          sync.RWMutex
	users       map[string]GitUser
	permissions map[string]AccessLevel // key: "username|owner/name"
}

// NewGitServer creates, starts, and returns a GitServer ready to serve git
// operations over both HTTP and HTTPS. Callers must invoke Stop to release
// resources.
func NewGitServer() (*GitServer, error) {
	dir, err := os.MkdirTemp("", "syngit-envtest-gitserver-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	gs := &GitServer{
		dir:         dir,
		users:       make(map[string]GitUser),
		permissions: make(map[string]AccessLevel),
	}

	cfg := gitkit.Config{
		Dir:        dir,
		AutoCreate: false,
		AutoHooks:  false,
		Auth:       true,
	}
	gs.handler = gitkit.New(cfg)
	gs.handler.AuthFunc = gs.authFunc
	if err := gs.handler.Setup(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("failed to setup gitkit: %w", err)
	}

	gs.httpServer = httptest.NewServer(gs.handler)
	gs.tlsServer = httptest.NewTLSServer(gs.handler)

	return gs, nil
}

// Stop shuts down both HTTP servers and removes the on-disk repository
// directory. Safe to call multiple times.
func (gs *GitServer) Stop() {
	if gs.httpServer != nil {
		gs.httpServer.Close()
		gs.httpServer = nil
	}
	if gs.tlsServer != nil {
		gs.tlsServer.Close()
		gs.tlsServer = nil
	}
	if gs.dir != "" {
		_ = os.RemoveAll(gs.dir)
		gs.dir = ""
	}
}

// URL returns the plain-HTTP base URL of the server, e.g. "http://127.0.0.1:PORT".
func (gs *GitServer) URL() string {
	return gs.httpServer.URL
}

// RepoURL returns the plain-HTTP clone URL for a repository, e.g.
// "http://127.0.0.1:PORT/{owner}/{name}.git".
func (gs *GitServer) RepoURL(repo RepoRef) string {
	return fmt.Sprintf("%s/%s/%s.git", gs.URL(), repo.Owner, repo.Name)
}

// FQDN returns the host:port of the HTTP listener, suitable for
// RemoteUser.Spec.GitBaseDomainFQDN.
func (gs *GitServer) FQDN() string {
	return hostPort(gs.URL())
}

// TLSURL returns the HTTPS base URL of the server, e.g.
// "https://127.0.0.1:PORT".
func (gs *GitServer) TLSURL() string {
	return gs.tlsServer.URL
}

// TLSRepoURL returns the HTTPS clone URL for a repository.
func (gs *GitServer) TLSRepoURL(repo RepoRef) string {
	return fmt.Sprintf("%s/%s/%s.git", gs.TLSURL(), repo.Owner, repo.Name)
}

// TLSFQDN returns the host:port of the HTTPS listener.
func (gs *GitServer) TLSFQDN() string {
	return hostPort(gs.TLSURL())
}

// CACert returns the PEM-encoded CA certificate that signed the TLS
// listener's server certificate. Use for CABundle in TLS tests.
func (gs *GitServer) CACert() []byte {
	cert := gs.tlsServer.Certificate()
	if cert == nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

// CACertX509 returns the server certificate as an *x509.Certificate.
func (gs *GitServer) CACertX509() *x509.Certificate {
	return gs.tlsServer.Certificate()
}

// AddUser registers a git user. The user starts with no access to any
// repository; call SetPermission to grant access.
func (gs *GitServer) AddUser(user GitUser) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.users[user.Username] = user
}

// GetUser returns the GitUser registered for username.
func (gs *GitServer) GetUser(username string) (GitUser, bool) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	u, ok := gs.users[username]
	return u, ok
}

// SetPermission sets the access level for a user on a specific repository.
func (gs *GitServer) SetPermission(username string, repo RepoRef, level AccessLevel) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.permissions[permKey(username, repo)] = level
}

// GetPermission returns the access level for a user on a repository.
// Returns NoAccess if no explicit permission was set.
func (gs *GitServer) GetPermission(username string, repo RepoRef) AccessLevel {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	level, ok := gs.permissions[permKey(username, repo)]
	if !ok {
		return NoAccess
	}
	return level
}

// authFunc is the gitkit AuthFunc. It validates credentials, looks up the
// permission for the requested repo, and enforces read-vs-write access.
func (gs *GitServer) authFunc(cred gitkit.Credential, req *gitkit.Request) (bool, error) {
	gs.mu.RLock()
	user, userExists := gs.users[cred.Username]
	gs.mu.RUnlock()

	if !userExists || user.Password != cred.Password {
		return false, nil
	}

	repo, err := parseRepoName(req.RepoName)
	if err != nil {
		return false, nil
	}

	level := gs.GetPermission(cred.Username, repo)
	if level == NoAccess {
		return false, nil
	}

	if level == ReadOnly && isWriteOperation(req.Request) {
		return false, nil
	}

	return true, nil
}

// isWriteOperation reports whether the request is a git push (write).
// git smart HTTP uses git-receive-pack for pushes and git-upload-pack for
// clones/fetches.
func isWriteOperation(r *http.Request) bool {
	if strings.Contains(r.URL.Path, "git-receive-pack") {
		return true
	}
	return r.URL.Query().Get("service") == "git-receive-pack"
}

// parseRepoName converts a gitkit RepoName ("owner/name.git") into RepoRef.
func parseRepoName(name string) (RepoRef, error) {
	name = strings.TrimSuffix(name, ".git")
	idx := strings.LastIndex(name, "/")
	if idx <= 0 || idx == len(name)-1 {
		return RepoRef{}, fmt.Errorf("invalid repo name %q", name)
	}
	return RepoRef{Owner: name[:idx], Name: name[idx+1:]}, nil
}

func permKey(username string, repo RepoRef) string {
	return username + "|" + repo.String()
}

func hostPort(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
