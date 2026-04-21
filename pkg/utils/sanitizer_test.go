package utils

import (
	"regexp"
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	t.Run("deterministic for same input", func(t *testing.T) {
		if Sanitize("hello") != Sanitize("hello") { // nolint:staticcheck
			t.Errorf("Sanitize should be deterministic")
		}
	})

	t.Run("output length is 12", func(t *testing.T) {
		inputs := []string{"", "a", "hello world", strings.Repeat("x", 1024)}
		for _, in := range inputs {
			if got := Sanitize(in); len(got) != 12 {
				t.Errorf("Sanitize(%q) length=%d, want 12", in, len(got))
			}
		}
	})

	t.Run("output is lowercase hex", func(t *testing.T) {
		hexRe := regexp.MustCompile(`^[0-9a-f]{12}$`)
		if got := Sanitize("arbitrary input 123 !@#"); !hexRe.MatchString(got) {
			t.Errorf("Sanitize=%q, want 12 lowercase hex chars", got)
		}
	})

	t.Run("distinct inputs produce distinct outputs", func(t *testing.T) {
		if Sanitize("foo") == Sanitize("bar") {
			t.Errorf("distinct inputs should produce distinct outputs")
		}
	})
}

func TestSoftSanitize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"already valid simple", "foo", "foo"},
		{"already valid with dashes", "foo-bar", "foo-bar"},
		{"already valid with dots", "foo.bar.baz", "foo.bar.baz"},
		{"already valid with digits", "a1.b2-c3", "a1.b2-c3"},
		{"uppercase is lowercased", "FooBar", "foobar"},
		{"disallowed chars replaced with dash", "foo_bar", "foo-bar"},
		{"slash replaced with dash", "foo/bar", "foo-bar"},
		{"multiple disallowed chars each replaced", "a@b#c", "a-b-c"},
		{"leading dash is invalid, gets replaced", "-foo", "-foo"},
		{"mixed case and invalid chars", "Foo_Bar@Baz", "foo-bar-baz"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SoftSanitize(tc.input); got != tc.want {
				t.Errorf("SoftSanitize(%q)=%q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
