package envtest_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	httpgit "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/syngit-org/syngit/pkg/envtest"
)

// startServer returns a started GitServer and registers cleanup.
func startServer(t *testing.T) *envtest.GitServer {
	t.Helper()
	gs, err := envtest.NewGitServer()
	if err != nil {
		t.Fatalf("NewGitServer: %v", err)
	}
	t.Cleanup(gs.Stop)
	return gs
}

func basicAuth(user envtest.GitUser) *httpgit.BasicAuth {
	return &httpgit.BasicAuth{Username: user.Username, Password: user.Password}
}

func TestCreateRepoAndCloneWithReadWrite(t *testing.T) {
	gs := startServer(t)

	user := envtest.GitUser{Username: "alice", Password: "secret", Email: "alice@example.com"}
	repo := envtest.RepoRef{Owner: "syngituser", Name: "merry"}
	gs.AddUser(user)
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	gs.SetPermission(user.Username, repo, envtest.ReadWrite)

	r, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:             gs.RepoURL(repo),
		ReferenceName:   plumbing.NewBranchReferenceName("main"),
		SingleBranch:    true,
		Auth:            basicAuth(user),
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Create a commit and push it back
	wt, _ := r.Worktree()
	f, _ := wt.Filesystem.Create("hello.txt")
	_, _ = f.Write([]byte("hello"))
	_ = f.Close()
	if _, err := wt.Add("hello.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := wt.Commit("add hello", &git.CommitOptions{}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	err = r.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{"refs/heads/main:refs/heads/main"},
		Auth:       basicAuth(user),
	})
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	// Verify commit landed on the bare repo
	content, err := gs.ReadFile(repo, "main", "hello.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestReadOnlyUserCannotPush(t *testing.T) {
	gs := startServer(t)

	user := envtest.GitUser{Username: "bob", Password: "pwd"}
	repo := envtest.RepoRef{Owner: "syngituser", Name: "read-only-test"}
	gs.AddUser(user)
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	gs.SetPermission(user.Username, repo, envtest.ReadOnly)

	// Clone must succeed
	r, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:             gs.RepoURL(repo),
		ReferenceName:   plumbing.NewBranchReferenceName("main"),
		SingleBranch:    true,
		Auth:            basicAuth(user),
		InsecureSkipTLS: true,
	})
	if err != nil {
		t.Fatalf("clone should succeed: %v", err)
	}

	// Push must fail
	wt, _ := r.Worktree()
	f, _ := wt.Filesystem.Create("x.txt")
	_, _ = f.Write([]byte("x"))
	_ = f.Close()
	_, _ = wt.Add("x.txt")
	_, _ = wt.Commit("x", &git.CommitOptions{})
	err = r.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{"refs/heads/main:refs/heads/main"},
		Auth:       basicAuth(user),
	})
	if err == nil {
		t.Fatalf("push should fail for read-only user")
	}
}

func TestNoAccessUserCannotClone(t *testing.T) {
	gs := startServer(t)
	user := envtest.GitUser{Username: "eve", Password: "pwd"}
	repo := envtest.RepoRef{Owner: "syngituser", Name: "private"}
	gs.AddUser(user)
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	// permission left at NoAccess

	_, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:             gs.RepoURL(repo),
		ReferenceName:   plumbing.NewBranchReferenceName("main"),
		SingleBranch:    true,
		Auth:            basicAuth(user),
		InsecureSkipTLS: true,
	})
	if err == nil {
		t.Fatalf("clone should fail for no-access user")
	}
}

func TestWrongPasswordRejected(t *testing.T) {
	gs := startServer(t)
	user := envtest.GitUser{Username: "luffy", Password: "right"}
	repo := envtest.RepoRef{Owner: "syngituser", Name: "merry"}
	gs.AddUser(user)
	gs.SetPermission(user.Username, repo, envtest.ReadWrite)
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	_, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:             gs.RepoURL(repo),
		ReferenceName:   plumbing.NewBranchReferenceName("main"),
		SingleBranch:    true,
		Auth:            &httpgit.BasicAuth{Username: user.Username, Password: "wrong"},
		InsecureSkipTLS: true,
	})
	if err == nil {
		t.Fatalf("clone should fail with wrong password")
	}
}

func TestCommitFileAndReadFile(t *testing.T) {
	gs := startServer(t)
	repo := envtest.RepoRef{Owner: "syngituser", Name: "demo"}
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	if err := gs.CommitFile(repo, "main", "configs/app.yaml", []byte("key: value\n"), "add app"); err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	content, err := gs.ReadFile(repo, "main", "configs/app.yaml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "key: value\n" {
		t.Fatalf("unexpected content: %q", content)
	}

	ok, err := gs.FileExists(repo, "main", "configs/app.yaml")
	if err != nil || !ok {
		t.Fatalf("FileExists: ok=%v err=%v", ok, err)
	}
	ok, _ = gs.FileExists(repo, "main", "missing.yaml")
	if ok {
		t.Fatalf("FileExists(missing) should be false")
	}

	files, err := gs.ListFiles(repo, "main")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "configs/app.yaml" {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestCreateBranchAndMergeFastForward(t *testing.T) {
	gs := startServer(t)
	repo := envtest.RepoRef{Owner: "syngituser", Name: "branches"}
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	if err := gs.CreateBranch(repo, "feature", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := gs.CommitFile(repo, "feature", "feat.txt", []byte("f\n"), "feat"); err != nil {
		t.Fatalf("CommitFile feature: %v", err)
	}

	// main has no new commits, so merging feature into main is a fast-forward.
	if err := gs.MergeBranch(repo, "feature", "main"); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	ok, err := gs.FileExists(repo, "main", "feat.txt")
	if err != nil || !ok {
		t.Fatalf("expected feat.txt on main after FF merge: ok=%v err=%v", ok, err)
	}
}

func TestMergeNonFastForward(t *testing.T) {
	gs := startServer(t)
	repo := envtest.RepoRef{Owner: "syngituser", Name: "diverge"}
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	if err := gs.CreateBranch(repo, "feature", "main"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := gs.CommitFile(repo, "main", "m.txt", []byte("m\n"), "main"); err != nil {
		t.Fatalf("commit main: %v", err)
	}
	if err := gs.CommitFile(repo, "feature", "f.txt", []byte("f\n"), "feat"); err != nil {
		t.Fatalf("commit feature: %v", err)
	}

	// Divergent histories -> merge commit using source tree.
	if err := gs.MergeBranch(repo, "feature", "main"); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}
	ok, _ := gs.FileExists(repo, "main", "f.txt")
	if !ok {
		t.Fatalf("expected f.txt on main after merge")
	}
}

func TestTLSClone(t *testing.T) {
	gs := startServer(t)
	user := envtest.GitUser{Username: "tls-user", Password: "pwd"}
	repo := envtest.RepoRef{Owner: "syngituser", Name: "tls-repo"}
	gs.AddUser(user)
	gs.SetPermission(user.Username, repo, envtest.ReadWrite)
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	caPEM := gs.CACert()
	if caPEM == nil {
		t.Fatalf("CACert returned nil")
	}

	_, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:           gs.TLSRepoURL(repo),
		ReferenceName: plumbing.NewBranchReferenceName("main"),
		SingleBranch:  true,
		Auth:          basicAuth(user),
		CABundle:      caPEM,
	})
	if err != nil {
		t.Fatalf("TLS clone with CA bundle: %v", err)
	}

	// Sanity-check that the CA actually verifies via standard http.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	resp, err := client.Get(gs.TLSURL())
	if err != nil {
		t.Fatalf("HTTP GET over TLS: %v", err)
	}
	_ = resp.Body.Close()
}

func TestConcurrentRepoAccess(t *testing.T) {
	gs := startServer(t)
	user := envtest.GitUser{Username: "concurrent", Password: "pwd"}
	gs.AddUser(user)

	const n = 8
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			repo := envtest.RepoRef{Owner: "syngituser", Name: "concurrent-" + itoa(i)}
			if err := gs.CreateRepo(repo, "main"); err != nil {
				done <- err
				return
			}
			gs.SetPermission(user.Username, repo, envtest.ReadWrite)
			err := gs.CommitFile(repo, "main", "x.txt", []byte("x"), "x")
			done <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent op failed: %v", err)
		}
	}
}

func TestIsObjectInRepo(t *testing.T) {
	gs := startServer(t)
	repo := envtest.RepoRef{Owner: "syngituser", Name: "assertions"}
	if err := gs.CreateRepo(repo, "main"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	cm := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "test"},
		Data:       map[string]string{"key": "value"},
	}
	if err := gs.CommitObject(repo, "main", "test/configmaps/my-cm.yaml", cm, "add cm"); err != nil {
		t.Fatalf("CommitObject: %v", err)
	}

	ok, err := gs.IsObjectInRepo(repo, "main", cm)
	if err != nil || !ok {
		t.Fatalf("IsObjectInRepo: ok=%v err=%v", ok, err)
	}

	// Different name -> no match
	other := &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "test"},
	}
	ok, _ = gs.IsObjectInRepo(repo, "main", other)
	if ok {
		t.Fatalf("expected no match for %s", other.Name)
	}

	// Search with spec match
	found, err := gs.SearchForObjectInRepo(repo, "main", cm)
	if err != nil {
		t.Fatalf("SearchForObjectInRepo: %v", err)
	}
	if len(found) != 1 || !strings.Contains(found[0].Path, "my-cm.yaml") {
		t.Fatalf("unexpected search results: %+v", found)
	}
}

func TestHelpersSecrets(t *testing.T) {
	gs := startServer(t)
	user := envtest.GitUser{Username: "sanji", Password: "cook", Email: "sanji@syngit.io"}
	secret := envtest.NewBasicAuthSecret(user, "sanji-creds", "test")
	if secret.Type != corev1.SecretTypeBasicAuth {
		t.Fatalf("unexpected type: %v", secret.Type)
	}
	if string(secret.Data[corev1.BasicAuthUsernameKey]) != "sanji" {
		t.Fatalf("wrong username")
	}

	tlsSecret := gs.NewTLSSecret("ca", "test")
	if tlsSecret == nil {
		t.Fatalf("NewTLSSecret: nil")
	}
	if tlsSecret.Type != corev1.SecretTypeTLS {
		t.Fatalf("unexpected TLS secret type: %v", tlsSecret.Type)
	}
	if len(tlsSecret.Data[corev1.TLSCertKey]) == 0 {
		t.Fatalf("TLS secret missing ca cert")
	}
}

func itoa(i int) string {
	return string(rune('0' + i))
}
