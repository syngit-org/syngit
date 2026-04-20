package errors

import (
	"errors"
	"strings"
	"testing"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
)

// syngitErr is the shared surface of every concrete error type in this package.
type syngitErr interface {
	error
	ShouldContains(err error) bool
	Unwrap() error
}

func assertErrorContract(t *testing.T, got syngitErr, wantSubstr string, sentinel error) {
	t.Helper()
	if msg := got.Error(); !strings.Contains(msg, wantSubstr) {
		t.Errorf("Error()=%q, want substring %q", msg, wantSubstr)
	}
	if !got.ShouldContains(got) {
		t.Errorf("ShouldContains should return true for own Error()")
	}
	if got.ShouldContains(errors.New("completely unrelated error")) {
		t.Errorf("ShouldContains should return false for unrelated error")
	}
	if got.Unwrap() != sentinel {
		t.Errorf("Unwrap()=%v, want %v", got.Unwrap(), sentinel)
	}
	if !errors.Is(got, sentinel) {
		t.Errorf("errors.Is should recognize the wrapped sentinel")
	}
}

func TestNewResourceScopeForbidden(t *testing.T) {
	user := authv1.UserInfo{Username: "alice"}
	resources := []string{"secrets", "configmaps"}

	e := NewResourceScopeForbidden(user, resources)

	if e.User.Username != "alice" {
		t.Errorf("User.Username=%q, want %q", e.User.Username, "alice")
	}
	if len(e.ForbiddenResources) != 2 {
		t.Fatalf("ForbiddenResources length=%d, want 2", len(e.ForbiddenResources))
	}
	assertErrorContract(t, e, "resource scope forbidden", ErrResourceScopeForbidden)

	msg := e.Error()
	for _, r := range resources {
		if !strings.Contains(msg, r) {
			t.Errorf("Error() should contain resource %q, got %q", r, msg)
		}
	}
}

func TestNewRemoteUserDenied(t *testing.T) {
	user := authv1.UserInfo{Username: "bob"}
	ref := v1.ObjectReference{Name: "my-remote-user"}

	e := NewRemoteUserDenied(user, ref)

	if e.RemoteUserRef.Name != "my-remote-user" {
		t.Errorf("RemoteUserRef.Name=%q, want %q", e.RemoteUserRef.Name, "my-remote-user")
	}
	assertErrorContract(t, e, "get remote user denied", ErrRemoteUserDenied)
	if !strings.Contains(e.Error(), "my-remote-user") {
		t.Errorf("Error() should include ref name, got %q", e.Error())
	}
}

func TestNewRemoteUserBindingNotFound(t *testing.T) {
	e := NewRemoteUserBindingNotFound("charlie")

	if e.Username != "charlie" {
		t.Errorf("Username=%q, want %q", e.Username, "charlie")
	}
	assertErrorContract(t, e, "remote user binding not found", ErrRemoteUserBindingNotFound)
	if !strings.Contains(e.Error(), "charlie") {
		t.Errorf("Error() should include username, got %q", e.Error())
	}
}

func TestNewWrongRemoteTargetConfig(t *testing.T) {
	rs := syngit.RemoteSyncer{}
	rs.Name = "my-syncer" // nolint:goconst
	ru := syngit.RemoteUser{}
	ru.Name = "my-user"

	e := NewWrongRemoteTargetConfig(rs, ru)

	if e.RemoteSyncer.Name != "my-syncer" || e.RemoteUser.Name != "my-user" { // nolint:goconst
		t.Errorf("field mapping failed: syncer=%q user=%q", e.RemoteSyncer.Name, e.RemoteUser.Name)
	}
	assertErrorContract(t, e, "wrong remote target config", ErrWrongRemoteTargetConfig)
	if !strings.Contains(e.Error(), "my-syncer") || !strings.Contains(e.Error(), "my-user") { // nolint:goconst
		t.Errorf("Error() should include both names, got %q", e.Error())
	}
}

func TestNewWrongRemoteSyncerConfig(t *testing.T) {
	e := NewWrongRemoteSyncerConfig("missing field")

	if e.Message != "missing field" {
		t.Errorf("Message=%q, want %q", e.Message, "missing field")
	}
	assertErrorContract(t, e, "wrong remote syncer config", ErrWrongRemoteSyncerConfig)
	if want := "wrong remote syncer config: missing field"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewRemoteTargetNotFound(t *testing.T) {
	e := NewRemoteTargetNotFound("no match")

	if e.Details != "no match" {
		t.Errorf("Details=%q, want %q", e.Details, "no match")
	}
	assertErrorContract(t, e, "no remote target found", ErrRemoteTargetNotFound)
	if want := "no remote target found: no match"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewRemoteUserNotFound(t *testing.T) {
	e := NewRemoteUserNotFound("missing")

	if e.Details != "missing" {
		t.Errorf("Details=%q, want %q", e.Details, "missing")
	}
	assertErrorContract(t, e, "remote user not found", ErrRemoteUserNotFound)
	if want := "remote user not found: missing"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewCredentialsNotFound(t *testing.T) {
	e := NewCredentialsNotFound("secret missing", "git-creds")

	if e.Details != "secret missing" || e.SecretName != "git-creds" {
		t.Errorf("field mapping failed: details=%q secret=%q", e.Details, e.SecretName)
	}
	assertErrorContract(t, e, "credential not found", ErrCredentialsNotFound)
	msg := e.Error()
	if !strings.Contains(msg, "git-creds") || !strings.Contains(msg, "secret missing") {
		t.Errorf("Error() should include both secret name and details, got %q", msg)
	}
}

func TestNewTooMuchRemoteTarget(t *testing.T) {
	e := NewTooMuchRemoteTarget("details", 3)

	if e.Details != "details" || e.RemoteTargetsCount != 3 { // nolint:goconst
		t.Errorf("field mapping failed: details=%q count=%d", e.Details, e.RemoteTargetsCount)
	}
	assertErrorContract(t, e, "too much remote target found", ErrTooMuchRemoteTarget)
	if want := "too much remote target found (3): details"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewWrongLabelParsing(t *testing.T) {
	e := NewWrongLabelParsing("invalid")

	if e.Details != "invalid" {
		t.Errorf("Details=%q, want %q", e.Details, "invalid")
	}
	assertErrorContract(t, e, "wrong label parsing", ErrWrongLabelParsing)
	if want := "wrong label parsing: invalid"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewTooMuchRemoteUserBinding(t *testing.T) {
	e := NewTooMuchRemoteUserBinding("details", 5)

	if e.Details != "details" || e.RemoteUserBindingsCount != 5 {
		t.Errorf("field mapping failed: details=%q count=%d", e.Details, e.RemoteUserBindingsCount)
	}
	assertErrorContract(t, e, "too much remote user binding found", ErrTooMuchRemoteUserBinding)
	if want := "too much remote user binding found (5): details"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewTooMuchRemoteUser(t *testing.T) {
	e := NewTooMuchRemoteUser("details", 4)

	if e.Details != "details" || e.RemoteUsersCount != 4 {
		t.Errorf("field mapping failed: details=%q count=%d", e.Details, e.RemoteUsersCount)
	}
	assertErrorContract(t, e, "too much remote user found", ErrTooMuchRemoteUser)
	if want := "too much remote user found (4): details"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewTooMuchSubject(t *testing.T) {
	e := NewTooMuchSubject("non-unique")

	if e.Details != "non-unique" {
		t.Errorf("Details=%q, want %q", e.Details, "non-unique")
	}
	assertErrorContract(t, e, "too much subjects", ErrTooMuchSubject)
	if want := "too much subjects: non-unique"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewWrongYAMLFormat(t *testing.T) {
	e := NewWrongYAMLFormat("cannot parse")

	if e.Details != "cannot parse" {
		t.Errorf("Details=%q, want %q", e.Details, "cannot parse")
	}
	assertErrorContract(t, e, "wrong yaml format", ErrWrongYAMLFormat)
	if e.Error() != "wrong yaml format" {
		t.Errorf("Error()=%q, want %q", e.Error(), "wrong yaml format")
	}
}

func TestNewGitPipeline(t *testing.T) {
	e := NewGitPipeline("push failed")

	if e.Details != "push failed" {
		t.Errorf("Details=%q, want %q", e.Details, "push failed")
	}
	assertErrorContract(t, e, "git pipeline processing error", ErrGitPipeline)
	if want := "git pipeline processing error: push failed"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestNewInterceptorPipeline(t *testing.T) {
	e := NewInterceptorPipeline("broken")

	if e.Details != "broken" {
		t.Errorf("Details=%q, want %q", e.Details, "broken")
	}
	assertErrorContract(t, e, "interceptor pipeline processing error", ErrInterceptorPipeline)
	if want := "interceptor pipeline processing error: broken"; e.Error() != want {
		t.Errorf("Error()=%q, want %q", e.Error(), want)
	}
}

func TestBuildInterceptorPipelineErr(t *testing.T) {
	got := BuildInterceptorPipelineErr("broken")
	want := NewInterceptorPipeline("broken").Error()
	if got != want {
		t.Errorf("BuildInterceptorPipelineErr=%q, want %q", got, want)
	}
}
