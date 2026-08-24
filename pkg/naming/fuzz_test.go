package naming

import (
	"regexp"
	"testing"
)

// hexRe and allowedRe describe the shape each helper promises to produce,
// independently of what was fed to it.
var (
	hexRe     = regexp.MustCompile(`^[0-9a-f]{12}$`)
	allowedRe = regexp.MustCompile(`^[a-z0-9.-]*$`)
)

// FuzzSanitize checks that the hashed form is always a 12-character lowercase
// hex digest and never depends on anything but the input.
func FuzzSanitize(f *testing.F) {
	for _, seed := range []string{"", "a", "hello world", "ABC/def.git", "üñïçødé", "\x00\xff"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		got := Sanitize(input)
		if !hexRe.MatchString(got) {
			t.Fatalf("Sanitize(%q) = %q, want 12 lowercase hex chars", input, got)
		}
		if again := Sanitize(input); again != got {
			t.Fatalf("Sanitize(%q) is not deterministic: %q then %q", input, got, again)
		}
	})
}

// FuzzSoftSanitize checks the two properties the callers rely on: the result
// only ever contains characters that are legal in a Kubernetes object name, and
// running it again over its own output changes nothing.
func FuzzSoftSanitize(f *testing.F) {
	for _, seed := range []string{"", "a", "my-repo.git", "My Repo", "ÜPPER/case", "..", "-lead", "trail-", "\x00"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		got := SoftSanitize(input)
		if !allowedRe.MatchString(got) {
			t.Fatalf("SoftSanitize(%q) = %q, want only [a-z0-9.-]", input, got)
		}
		if again := SoftSanitize(got); again != got {
			t.Fatalf("SoftSanitize is not idempotent: %q -> %q -> %q", input, got, again)
		}
	})
}

// FuzzRemoteTargetName checks that deriving a name from arbitrary repository
// URLs and branch names never panics and stays deterministic. The branches are
// not sanitized by design, so the result is not constrained further here.
func FuzzRemoteTargetName(f *testing.F) {
	f.Add("https://github.com/syngit-org/syngit.git", "main", "", "dev")
	f.Add("https://github.com/syngit-org/syngit.git", "main", "https://github.com/damsien/syngit.git", "dev")
	f.Add("", "", "", "")
	f.Add("://", "ü", "%zz", "\x00")

	f.Fuzz(func(t *testing.T, upstreamRepo, upstreamBranch, targetRepo, targetBranch string) {
		got, err := RemoteTargetName(upstreamRepo, upstreamBranch, targetRepo, targetBranch)
		if err != nil {
			return
		}
		again, err := RemoteTargetName(upstreamRepo, upstreamBranch, targetRepo, targetBranch)
		if err != nil {
			t.Fatalf("RemoteTargetName errored on the second call only: %v", err)
		}
		if again != got {
			t.Fatalf("RemoteTargetName is not deterministic: %q then %q", got, again)
		}
	})
}
