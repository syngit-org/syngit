package utils

import (
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestMergeResults(t *testing.T) {
	tests := []struct {
		name string
		a    ctrl.Result
		b    ctrl.Result
		want ctrl.Result
	}{
		{
			name: "both zero stays zero",
			a:    ctrl.Result{},
			b:    ctrl.Result{},
			want: ctrl.Result{},
		},
		{
			name: "b fills in when a is zero",
			a:    ctrl.Result{},
			b:    ctrl.Result{RequeueAfter: 5 * time.Second},
			want: ctrl.Result{RequeueAfter: 5 * time.Second},
		},
		{
			name: "a kept when b is zero",
			a:    ctrl.Result{RequeueAfter: 5 * time.Second},
			b:    ctrl.Result{},
			want: ctrl.Result{RequeueAfter: 5 * time.Second},
		},
		{
			name: "b wins when sooner",
			a:    ctrl.Result{RequeueAfter: 10 * time.Second},
			b:    ctrl.Result{RequeueAfter: 3 * time.Second},
			want: ctrl.Result{RequeueAfter: 3 * time.Second},
		},
		{
			name: "a kept when sooner",
			a:    ctrl.Result{RequeueAfter: 3 * time.Second},
			b:    ctrl.Result{RequeueAfter: 10 * time.Second},
			want: ctrl.Result{RequeueAfter: 3 * time.Second},
		},
		{
			name: "equal durations keep a",
			a:    ctrl.Result{RequeueAfter: 7 * time.Second},
			b:    ctrl.Result{RequeueAfter: 7 * time.Second},
			want: ctrl.Result{RequeueAfter: 7 * time.Second},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MergeResults(tc.a, tc.b); got != tc.want {
				t.Errorf("MergeResults(%+v, %+v) = %+v, want %+v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
