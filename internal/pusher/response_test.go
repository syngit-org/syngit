package pusher

import (
	"reflect"
	"testing"
)

func TestResponseBuilder(t *testing.T) {
	t.Run("maps all fields", func(t *testing.T) {
		got := ResponseBuilder([]string{"a.yaml", "b.yaml"}, "deadbeef", "https://example.com/repo.git")
		if !reflect.DeepEqual(got.Paths, []string{"a.yaml", "b.yaml"}) {
			t.Errorf("Paths=%v", got.Paths)
		}
		if got.CommitHash != "deadbeef" {
			t.Errorf("CommitHash=%q, want deadbeef", got.CommitHash)
		}
		if got.URL != "https://example.com/repo.git" {
			t.Errorf("URL=%q, want https://example.com/repo.git", got.URL)
		}
	})

	t.Run("empty values", func(t *testing.T) {
		got := ResponseBuilder(nil, "", "")
		if got.Paths != nil {
			t.Errorf("Paths=%v, want nil", got.Paths)
		}
		if got.CommitHash != "" || got.URL != "" {
			t.Errorf("expected empty strings, got %+v", got)
		}
	})
}
