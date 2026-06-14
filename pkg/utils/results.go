package utils

import (
	ctrl "sigs.k8s.io/controller-runtime"
)

// MergeResults keeps the soonest non-zero RequeueAfter across the core and
// policy reconcile results.
func MergeResults(a, b ctrl.Result) ctrl.Result {
	if b.RequeueAfter > 0 && (a.RequeueAfter == 0 || b.RequeueAfter < a.RequeueAfter) {
		a.RequeueAfter = b.RequeueAfter
	}
	return a
}
