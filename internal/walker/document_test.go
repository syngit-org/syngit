package walker

import (
	"strings"
	"testing"
)

func TestTransformChangedDocs_KeepsVerbatimDocuments(t *testing.T) {
	kept := "kind: Service\nmetadata:\n  name: kept\nENC"
	existing := kept + "\n---\nkind: Service\nmetadata:\n  name: changed\nold: true\n"

	var previous string
	transform := DocTransform(func(_ string, existing, content []byte) ([]byte, error) {
		previous = string(existing)
		return append(content, []byte("\nENC")...), nil
	})
	content := kept + "\n---\nkind: Service\nmetadata:\n  name: changed\nold: false\n"

	got, err := TransformChangedDocs("file.yaml", []byte(existing), []byte(content), transform)
	if err != nil {
		t.Fatalf("TransformChangedDocs: %v", err)
	}
	want := kept + "\n---\nkind: Service\nmetadata:\n  name: changed\nold: false\nENC\n"
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(previous, "old: true") {
		t.Fatalf("expected the previous version of the changed document, got %q", previous)
	}
}
