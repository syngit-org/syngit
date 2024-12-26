package utils

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TypeBasedConditionUpdater(conditions []metav1.Condition, condition metav1.Condition) []metav1.Condition {
	conditions = TypeBasedConditionRemover(conditions, condition.Type)
	conditions = append(conditions, condition)

	return conditions
}

func TypeBasedConditionRemover(conditions []metav1.Condition, typeKind string) []metav1.Condition {
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
