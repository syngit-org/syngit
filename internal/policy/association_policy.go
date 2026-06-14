package policy

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const associationPolicyFinalizer = "syngit.io/association-policy"

// AssociationPolicy manages the RemoteUserBinding of a RemoteUser that carries
// the managed annotation. It implements policy.Policy[*syngit.RemoteUser] and is
// run by RemoteUserReconciler.
type AssociationPolicy struct {
	client.Client
}

func (p *AssociationPolicy) Name() string { return "association-policy" }

func (p *AssociationPolicy) Finalizer() string { return associationPolicyFinalizer }

func (p *AssociationPolicy) Applies(remoteUser *syngit.RemoteUser) bool {
	return remoteUser.Annotations[syngit.RubAnnotationKeyManaged] == "true" // nolint:goconst
}

func (p *AssociationPolicy) Reconcile(ctx context.Context, remoteUser *syngit.RemoteUser) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	rdm := time.Duration(rand.Intn(5)) * time.Second

	sanitizedUsername := remoteUser.Labels[syngit.K8sUserLabelKey]
	rawUsername := remoteUser.Annotations[syngit.K8sUserLabelKey]

	if sanitizedUsername == "" || rawUsername == "" {
		logger.Info("RemoteUser has managed annotation but no k8s-user label/annotation, waiting for mutating webhook to stamp it")
		return ctrl.Result{}, nil
	}

	// Find or create the managed RemoteUserBinding for this user
	rub, err := p.findOrCreateManagedRUB(ctx, remoteUser, sanitizedUsername, rawUsername)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the RemoteUser is in the RUB's remoteUserRefs
	if err := p.ensureRemoteUserRef(ctx, rub, remoteUser.Name); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
	}

	// Search for RemoteTargets with one-or-many-branches label and ensure they're in the RUB
	if err := p.associateExistingRemoteTargets(ctx, rub); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfter + rdm}, err
	}

	return ctrl.Result{}, nil
}

func (p *AssociationPolicy) Cleanup(ctx context.Context, remoteUser *syngit.RemoteUser) error {
	return p.cleanupAssociation(ctx, remoteUser, remoteUser.Labels[syngit.K8sUserLabelKey])
}

// findOrCreateManagedRUB finds the managed RemoteUserBinding for a user, or creates one.
func (p *AssociationPolicy) findOrCreateManagedRUB(ctx context.Context, remoteUser *syngit.RemoteUser, sanitizedUsername, rawUsername string) (*syngit.RemoteUserBinding, error) {
	rubList := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   sanitizedUsername,
		}),
		Namespace: remoteUser.Namespace,
	}
	if err := p.List(ctx, rubList, listOps); err != nil {
		return nil, err
	}

	if len(rubList.Items) > 0 {
		freshRub := &syngit.RemoteUserBinding{}
		if err := p.Get(ctx, types.NamespacedName{Name: rubList.Items[0].Name, Namespace: remoteUser.Namespace}, freshRub); err != nil {
			return nil, err
		}
		return freshRub, nil
	}

	// No managed RUB seen via the cached List. Try to claim a name, starting
	// from the deterministic base. On AlreadyExists, inspect the existing
	// object: if it is the managed RUB for this user, reuse it; otherwise
	// advance to the next suffix so a user-owned RUB at the base name
	// doesn't block us.
	baseName := syngit.RubNamePrefix + "-" + sanitizedUsername
	const maxAttempts = 100
	for i := 0; i < maxAttempts; i++ {
		name := baseName
		if i > 0 {
			name = fmt.Sprintf("%s-%d", baseName, i)
		}

		rub := &syngit.RemoteUserBinding{
			ObjectMeta: ctrl.ObjectMeta{
				Name:      name,
				Namespace: remoteUser.Namespace,
				Labels: map[string]string{
					syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
					syngit.K8sUserLabelKey:   sanitizedUsername,
				},
			},
			Spec: syngit.RemoteUserBindingSpec{
				Subject: rbacv1.Subject{
					Kind: "User",
					Name: rawUsername,
				},
				RemoteUserRefs: []corev1.ObjectReference{
					{Name: remoteUser.Name},
				},
			},
		}

		createErr := p.Create(ctx, rub)
		if createErr == nil {
			return rub, nil
		}
		if !apierrors.IsAlreadyExists(createErr) {
			return nil, createErr
		}

		existing := &syngit.RemoteUserBinding{}
		if getErr := p.Get(ctx, types.NamespacedName{Name: name, Namespace: remoteUser.Namespace}, existing); getErr != nil {
			if apierrors.IsNotFound(getErr) {
				continue
			}
			return nil, getErr
		}

		if existing.Labels[syngit.ManagedByLabelKey] == syngit.ManagedByLabelValue &&
			existing.Labels[syngit.K8sUserLabelKey] == sanitizedUsername {
			return existing, nil
		}
	}

	return nil, fmt.Errorf("could not allocate a name for managed RemoteUserBinding (base=%q)", baseName)
}

// ensureRemoteUserRef ensures the RemoteUser is in the RUB's remoteUserRefs.
func (p *AssociationPolicy) ensureRemoteUserRef(ctx context.Context, rub *syngit.RemoteUserBinding, remoteUserName string) error {
	return utils.MutateOrDeleteManagedRemoteUserBinding(ctx, p.Client,
		types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace},
		func(fresh *syngit.RemoteUserBinding) error {
			for _, ref := range fresh.Spec.RemoteUserRefs {
				if ref.Name == remoteUserName {
					return nil
				}
			}
			fresh.Spec.RemoteUserRefs = append(fresh.Spec.RemoteUserRefs, corev1.ObjectReference{Name: remoteUserName})
			return nil
		})
}

// associateExistingRemoteTargets finds all one-or-many-branches RemoteTargets and ensures they're in the RUB.
func (p *AssociationPolicy) associateExistingRemoteTargets(ctx context.Context, rub *syngit.RemoteUserBinding) error {
	listOps := &client.ListOptions{
		Namespace: rub.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.RtLabelKeyPolicy:  syngit.RtLabelValueOneOrManyBranches,
		}),
	}

	return utils.MutateOrDeleteManagedRemoteUserBinding(ctx, p.Client,
		types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace},
		func(fresh *syngit.RemoteUserBinding) error {
			rtList := &syngit.RemoteTargetList{}
			if err := p.List(ctx, rtList, listOps); err != nil {
				return err
			}
			for _, rt := range rtList.Items {
				utils.AddRemoteTargetRef(fresh, rt.Name)
			}
			return nil
		})
}

// cleanupAssociation removes the RemoteUser from its managed RUB and deletes the RUB if empty.
func (p *AssociationPolicy) cleanupAssociation(ctx context.Context, remoteUser *syngit.RemoteUser, sanitizedUsername string) error {
	if sanitizedUsername == "" {
		return nil
	}

	rubList := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   sanitizedUsername,
		}),
		Namespace: remoteUser.Namespace,
	}
	if err := p.List(ctx, rubList, listOps); err != nil {
		return err
	}

	for i := range rubList.Items {
		rub := &rubList.Items[i]
		if err := utils.MutateOrDeleteManagedRemoteUserBinding(ctx, p.Client,
			types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace},
			func(fresh *syngit.RemoteUserBinding) error {
				newRefs := make([]corev1.ObjectReference, 0, len(fresh.Spec.RemoteUserRefs))
				for _, ref := range fresh.Spec.RemoteUserRefs {
					if ref.Name != remoteUser.Name {
						newRefs = append(newRefs, ref)
					}
				}
				fresh.Spec.RemoteUserRefs = newRefs
				return nil
			}); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
	}

	return nil
}
