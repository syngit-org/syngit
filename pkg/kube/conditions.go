package kube

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetCondition replaces the condition carrying the same type as the given one,
// or appends it when no condition of that type is present. The replacement is
// unconditional: the given condition is stored as-is, timestamps included.
func SetCondition(conditions []metav1.Condition, condition metav1.Condition) []metav1.Condition {
	conditions = RemoveCondition(conditions, condition.Type)
	conditions = append(conditions, condition)

	return conditions
}

// RemoveCondition drops the condition carrying the given type, if any.
func RemoveCondition(conditions []metav1.Condition, typeKind string) []metav1.Condition {
	removeIndex := -1
	for i, statusCondition := range conditions {
		if typeKind == statusCondition.Type {
			removeIndex = i
		}
	}
	if removeIndex != -1 {
		conditions = slices.Delete(conditions, removeIndex, removeIndex+1)
	}

	return conditions
}
