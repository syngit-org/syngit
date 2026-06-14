package policy

import (
	"context"
	"errors"
	"time"

	"github.com/syngit-org/syngit/pkg/utils"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const requeueAfter = time.Second

// Policy is a self-gating unit of reconcile logic attached to a primary
// resource of type T. The host controller runs every registered policy on each
// reconcile; each policy decides via Applies whether it should act.
type Policy[T client.Object] interface {
	// Name identifies the policy in logs and events.
	Name() string
	// Finalizer is the finalizer this policy owns so that Cleanup is guaranteed
	// to run before the object is deleted. Return "" for a policy that needs no
	// cleanup; no finalizer is then managed on its behalf.
	Finalizer() string
	// Applies reports whether the policy's condition is met for obj (typically
	// an annotation or spec field). When false the host runs Cleanup instead of
	// Reconcile.
	Applies(obj T) bool
	// Reconcile performs the policy's work. It is only called when Applies is
	// true and the finalizer (if any) is already present.
	Reconcile(ctx context.Context, obj T) (ctrl.Result, error)
	// Cleanup reverses the policy's work. It is called when the object is being
	// deleted or when Applies became false, before the finalizer is removed.
	Cleanup(ctx context.Context, obj T) error
}

// RunPolicies drives the finalizer-gated lifecycle of every policy against obj.
// Call it from a per-CRD reconciler after that reconciler's own core logic, so
// a single controller owns the object and same-key reconciles stay serialized.
func RunPolicies[T client.Object](ctx context.Context, c client.Client, obj T, policies []Policy[T]) (ctrl.Result, error) {
	// Deletion: run Cleanup for every policy that still holds its finalizer,
	// then drop the finalizer so the object can be garbage collected.
	if !obj.GetDeletionTimestamp().IsZero() {
		for _, p := range policies {
			fin := p.Finalizer()
			if fin == "" || !controllerutil.ContainsFinalizer(obj, fin) {
				continue
			}
			if err := p.Cleanup(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
			if err := removeFinalizer(ctx, c, obj, fin); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure the finalizer of every applicable policy is persisted before any of
	// them create external state, so Cleanup is guaranteed to run on delete. The
	// Update re-triggers this controller through its own watch, so we just
	// return and let the next reconcile do the work.
	if added, err := ensureFinalizers(ctx, c, obj, policies); err != nil {
		return ctrl.Result{}, err
	} else if added {
		return ctrl.Result{}, nil
	}

	// Run each applicable policy, or clean up one whose condition no longer holds.
	var result ctrl.Result
	var errs []error
	for _, p := range policies {
		if p.Applies(obj) {
			res, err := p.Reconcile(ctx, obj)
			if err != nil {
				errs = append(errs, err)
			}
			result = utils.MergeResults(result, res)
			continue
		}

		// The condition no longer holds: clean up. Cleanup must
		// be idempotent. Drop the finalizer afterwards if it is
		// still there.
		if err := p.Cleanup(ctx, obj); err != nil {
			errs = append(errs, err)
			continue
		}
		if fin := p.Finalizer(); fin != "" && controllerutil.ContainsFinalizer(obj, fin) {
			if err := removeFinalizer(ctx, c, obj, fin); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return result, errors.Join(errs...)
}

// ensureFinalizers adds, in a single update, the finalizer of every policy that
// currently applies to obj. It reports whether anything was added so the caller
// can return early. obj is refreshed in place to the latest version.
func ensureFinalizers[T client.Object](ctx context.Context, c client.Client, obj T, policies []Policy[T]) (bool, error) {
	needed := make([]string, 0, len(policies))
	for _, p := range policies {
		if fin := p.Finalizer(); fin != "" && p.Applies(obj) {
			needed = append(needed, fin)
		}
	}
	if len(needed) == 0 {
		return false, nil
	}

	added := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return err
		}
		changed := false
		for _, fin := range needed {
			if controllerutil.AddFinalizer(obj, fin) {
				changed = true
			}
		}
		if !changed {
			added = false
			return nil
		}
		if err := c.Update(ctx, obj); err != nil {
			return err
		}
		added = true
		return nil
	})
	return added, err
}

// removeFinalizer drops fin from obj, tolerating a concurrent deletion.
func removeFinalizer[T client.Object](ctx context.Context, c client.Client, obj T, fin string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return client.IgnoreNotFound(err)
		}
		if !controllerutil.RemoveFinalizer(obj, fin) {
			return nil
		}
		return c.Update(ctx, obj)
	})
}
