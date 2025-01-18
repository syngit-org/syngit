package v1beta3

import (
	"context"
	"fmt"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"github.com/syngit-org/syngit/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UserSpecificPattern struct {
	PatternSpecification
	Username                 string
	RemoteSyncer             syngit.RemoteSyncer
	associatedRemoteTarget   syngit.RemoteTarget
	remoteTargetToBeSetuped  *syngit.RemoteTarget
	remoteTargetsToBeRemoved []syngit.RemoteTarget
	RemoteUserBinding        *syngit.RemoteUserBinding
}

func (usp *UserSpecificPattern) GetRemoteTarget() syngit.RemoteTarget {
	return usp.associatedRemoteTarget
}

func (usp *UserSpecificPattern) Setup(ctx context.Context) *ErrorPattern {
	if usp.remoteTargetToBeSetuped != nil {
		createErr := usp.Client.Create(ctx, usp.remoteTargetToBeSetuped)
		if createErr != nil {
			return &ErrorPattern{Message: createErr.Error(), Reason: Errored}
		}

		spec := usp.RemoteUserBinding.Spec.DeepCopy()
		spec.RemoteTargetRefs = append(spec.RemoteTargetRefs, corev1.ObjectReference{
			Name: usp.remoteTargetToBeSetuped.Name,
		})
		updateErr := updateOrDeleteRemoteUserBinding(ctx, usp.Client, *spec, *usp.RemoteUserBinding, 2)
		if updateErr != nil {
			return &ErrorPattern{Message: updateErr.Error(), Reason: Errored}
		}
	}

	return nil
}

func (usp *UserSpecificPattern) Remove(ctx context.Context) *ErrorPattern {
	if len(usp.remoteTargetsToBeRemoved) > 0 {
		remoteTargetsRef := []corev1.ObjectReference{}

		for _, rt := range usp.remoteTargetsToBeRemoved {
			// Delete RemoteTarget
			remoteTarget := &syngit.RemoteTarget{}
			namespacedName := types.NamespacedName{Name: rt.Name, Namespace: rt.Namespace}
			getErr := usp.Client.Get(ctx, namespacedName, remoteTarget)
			if getErr != nil {
				return &ErrorPattern{Message: getErr.Error(), Reason: Errored}
			}
			delErr := usp.Client.Delete(ctx, remoteTarget)
			if delErr != nil {
				return &ErrorPattern{Message: delErr.Error(), Reason: Errored}
			}

			// Unreference it from the associated RemoteUserBinding
			if usp.RemoteUserBinding == nil {
				return &ErrorPattern{Message: "Server error: no associated RemoteUserBinding found", Reason: Errored}
			}
			for _, remoteTargetRef := range usp.RemoteUserBinding.Spec.RemoteTargetRefs {
				if remoteTargetRef.Name != rt.Name {
					remoteTargetsRef = append(remoteTargetsRef, remoteTargetRef)
				}
			}
		}

		// Apply the unreferencement
		spec := usp.RemoteUserBinding.Spec.DeepCopy()
		spec.RemoteTargetRefs = remoteTargetsRef
		updateErr := updateOrDeleteRemoteUserBinding(ctx, usp.Client, *spec, *usp.RemoteUserBinding, 2)
		if updateErr != nil {
			return &ErrorPattern{Message: updateErr.Error(), Reason: Errored}
		}
	}

	return nil
}

func (usp *UserSpecificPattern) Diff(ctx context.Context) *ErrorPattern {
	usp.remoteTargetsToBeRemoved = []syngit.RemoteTarget{}

	// Get associated RemoteUserBinding
	remoteUserBindingList := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   usp.Username,
		}),
		Namespace: usp.RemoteSyncer.Namespace,
	}
	listErr := usp.Client.List(ctx, remoteUserBindingList, listOps)
	if listErr != nil {
		return &ErrorPattern{Message: listErr.Error(), Reason: Errored}
	}
	if len(remoteUserBindingList.Items) > 1 {
		return &ErrorPattern{Message: fmt.Sprintf("only one RemoteUserBinding for the user %s should be managed by Syngit", usp.Username), Reason: Denied}
	}
	if len(remoteUserBindingList.Items) == 0 {
		return nil
	}

	if usp.RemoteUserBinding == nil {
		remoteUserBinding := &syngit.RemoteUserBinding{}
		namespacedName := types.NamespacedName{Name: remoteUserBindingList.Items[0].Name, Namespace: remoteUserBindingList.Items[0].Namespace}
		getErr := usp.Client.Get(ctx, namespacedName, remoteUserBinding)
		if getErr != nil {
			return &ErrorPattern{Message: getErr.Error(), Reason: Errored}
		}
		usp.RemoteUserBinding = remoteUserBinding
	}

	// Get RemoteTargets that are already bound to this user
	// Scope only the RemoteTargets with the same upstream repo & branch as the RemoteSyncer
	boundRemoteTargets, listErr := usp.getExistingRemoteTarget(ctx)
	if listErr != nil {
		return &ErrorPattern{Message: listErr.Error(), Reason: Errored}
	}

	// If some are bound and there is no user specific annotation anymore
	userSpecificAnnotation := usp.RemoteSyncer.Annotations[syngit.RtAnnotationUserSpecificKey]
	if userSpecificAnnotation == "" {
		if len(boundRemoteTargets) > 0 {
			usp.remoteTargetsToBeRemoved = boundRemoteTargets
		}
		return nil
	}

	alreadyExists := false
	for _, rt := range boundRemoteTargets {
		if userSpecificAnnotation == string(syngit.RtAnnotationOneUserOneBranchValue) {
			// If the upstream repo & branch are the same (already filtered), the target repo is the same as the upstream and the branch is the username.
			// An user specific target could be different branch on the same repo (target-branch != upstream-branch)
			if rt.Spec.UpstreamBranch == usp.RemoteSyncer.Spec.DefaultBranch && rt.Spec.TargetRepository == usp.RemoteSyncer.Spec.RemoteRepository && rt.Spec.TargetBranch == usp.Username {
				usp.associatedRemoteTarget = rt
				alreadyExists = true
				break
			}
		}

		if userSpecificAnnotation == string(syngit.RtAnnotationOneUserOneForkValue) {
			// If the upstream repo & branch are the same (already filtered), then it is considered as found.
			// To allow permissive extension for external providers, we consider that the scope is the most open as possible.
			// An user specific target could be a fork (target-repo != upstream-repo)
			if rt.Spec.UpstreamBranch == usp.RemoteSyncer.Spec.DefaultBranch {
				usp.associatedRemoteTarget = rt
				alreadyExists = true
				break
			}
		}
	}

	if alreadyExists {
		return nil
	}

	// If the remoteTarget does not exists yet AND has to be created
	targetRepo := usp.RemoteSyncer.Spec.RemoteRepository
	if userSpecificAnnotation == string(syngit.RtAnnotationOneUserOneForkValue) {
		// Set it to empty because we do not know in advance the name of the fork
		// It will later be fill by the provider
		targetRepo = ""
	}
	builtRemoteTarget, buildErr := usp.buildRemoteTarget(targetRepo)
	if buildErr != nil {
		return &ErrorPattern{Message: buildErr.Error(), Reason: Errored}
	}

	usp.remoteTargetToBeSetuped = builtRemoteTarget
	usp.associatedRemoteTarget = *usp.remoteTargetToBeSetuped

	return nil
}

func (usp *UserSpecificPattern) getExistingRemoteTarget(ctx context.Context) ([]syngit.RemoteTarget, error) {
	var remoteTargetList = &syngit.RemoteTargetList{}
	listOps := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
			syngit.K8sUserLabelKey:   usp.Username,
		}),
		Namespace: usp.RemoteSyncer.Namespace,
	}
	err := usp.Client.List(ctx, remoteTargetList, listOps)
	if err != nil {
		return nil, err
	}

	remoteTargets := []syngit.RemoteTarget{}
	for _, rt := range remoteTargetList.Items {
		if rt.Spec.UpstreamRepository == usp.RemoteSyncer.Spec.RemoteRepository && rt.Spec.UpstreamBranch == usp.RemoteSyncer.Spec.DefaultBranch {
			remoteTargets = append(remoteTargets, rt)
		}
	}

	return remoteTargets, nil
}

func (usp *UserSpecificPattern) buildRemoteTarget(targetRepo string) (*syngit.RemoteTarget, error) {

	rtName, nameErr := utils.RemoteTargetNameConstructor(usp.RemoteSyncer.Spec.RemoteRepository, usp.RemoteSyncer.Spec.DefaultBranch, targetRepo, usp.Username)
	if nameErr != nil {
		return nil, nameErr
	}

	remoteTarget := &syngit.RemoteTarget{
		ObjectMeta: v1.ObjectMeta{
			Name:      rtName,
			Namespace: usp.RemoteSyncer.Namespace,
			Labels: map[string]string{
				syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
				syngit.K8sUserLabelKey:   usp.Username,
			},
			Annotations: map[string]string{
				syngit.RtAllowInjection: "true",
			},
		},
		Spec: syngit.RemoteTargetSpec{
			UpstreamRepository: usp.RemoteSyncer.Spec.RemoteRepository,
			UpstreamBranch:     usp.RemoteSyncer.Spec.DefaultBranch,
			TargetRepository:   targetRepo,
			TargetBranch:       usp.Username,
			MergeStrategy:      syngit.TryFastForwardOrHardReset,
		},
	}

	return remoteTarget, nil
}
