package utils

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTypeBasedConditionUpdater(t *testing.T) {
	t.Run("nil slice gets condition appended", func(t *testing.T) {
		c := metav1.Condition{Type: "Ready", Reason: "r1"}
		got := TypeBasedConditionUpdater(nil, c)
		if len(got) != 1 || got[0].Type != "Ready" || got[0].Reason != "r1" { // nolint:goconst
			t.Errorf("got %+v, want single Ready condition", got)
		}
	})

	t.Run("replaces existing condition of same type", func(t *testing.T) {
		existing := []metav1.Condition{
			{Type: "Ready", Reason: "old"},
			{Type: "Synced", Reason: "synced"},
		}
		got := TypeBasedConditionUpdater(existing, metav1.Condition{Type: "Ready", Reason: "new"})
		if len(got) != 2 {
			t.Fatalf("length=%d, want 2", len(got))
		}
		// Synced should still be present; Ready should have reason "new".
		var ready, synced *metav1.Condition
		for i := range got {
			switch got[i].Type {
			case "Ready": // nolint:goconst
				ready = &got[i]
			case "Synced": // nolint:goconst
				synced = &got[i]
			}
		}
		if ready == nil || ready.Reason != "new" {
			t.Errorf("Ready condition not replaced: %+v", ready)
		}
		if synced == nil || synced.Reason != "synced" {
			t.Errorf("Synced condition lost: %+v", synced)
		}
	})

	t.Run("appends when type is absent", func(t *testing.T) {
		existing := []metav1.Condition{{Type: "Ready", Reason: "r1"}}
		got := TypeBasedConditionUpdater(existing, metav1.Condition{Type: "Synced", Reason: "s1"})
		if len(got) != 2 {
			t.Fatalf("length=%d, want 2", len(got))
		}
		if got[1].Type != "Synced" {
			t.Errorf("expected Synced appended, got %s", got[1].Type)
		}
	})
}

func TestTypeBasedConditionRemover(t *testing.T) {
	t.Run("nil slice returns nil", func(t *testing.T) {
		got := TypeBasedConditionRemover(nil, "Ready")
		if len(got) != 0 {
			t.Errorf("got %+v, want empty", got)
		}
	})

	t.Run("removes matching type", func(t *testing.T) {
		in := []metav1.Condition{
			{Type: "Ready", Reason: "r"},
			{Type: "Synced", Reason: "s"},
		}
		got := TypeBasedConditionRemover(in, "Ready")
		if len(got) != 1 || got[0].Type != "Synced" {
			t.Errorf("got %+v, want single Synced", got)
		}
	})

	t.Run("type absent leaves list unchanged", func(t *testing.T) {
		in := []metav1.Condition{{Type: "Ready", Reason: "r"}}
		got := TypeBasedConditionRemover(in, "Missing")
		if len(got) != 1 || got[0].Type != "Ready" {
			t.Errorf("got %+v, want unchanged", got)
		}
	})
}
