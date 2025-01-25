package v1beta3

import (
	"context"
	"net/http"
	"slices"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"github.com/syngit-org/syngit/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type RemoteSyncerTargetPatternWebhookHandler struct {
	Client  client.Client
	Decoder *admission.Decoder
}

// +kubebuilder:webhook:path=/syngit-v1beta3-remotesyncer-target-pattern,mutating=false,failurePolicy=fail,sideEffects=None,groups=syngit.io,resources=remotesyncers,verbs=create;update;delete,versions=v1beta3,admissionReviewVersions=v1,name=vremotesyncers-target-pattern.v1beta3.syngit.io

func (rsyt *RemoteSyncerTargetPatternWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	oldRemoteSyncer := &syngit.RemoteSyncer{}
	if string(req.Operation) == "DELETE" || string(req.Operation) == "UPDATE" { //nolint:goconst
		err := rsyt.Decoder.DecodeRaw(req.OldObject, oldRemoteSyncer)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	// DELETE
	if string(req.Operation) == "DELETE" { //nolint:goconst
		err := rsyt.deleteRemoteTargetsIfNotReferenced(ctx, *oldRemoteSyncer)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		return admission.Allowed("All the RemoteTargets dependencies are deleted")
	}

	// CREATE OR UPDATE
	remoteSyncer := &syngit.RemoteSyncer{}
	err := rsyt.Decoder.Decode(req, remoteSyncer)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if rsyt.isThereDiff(*oldRemoteSyncer, *remoteSyncer) {
		err := rsyt.deleteRemoteTargetsIfNotReferenced(ctx, *oldRemoteSyncer)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if remoteSyncer.Annotations[syngit.RtAnnotationEnabled] == "true" {
			err = rsyt.createRemoteTargets(ctx, *remoteSyncer)
			if err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
			return admission.Allowed("RemoteTargets created or updated")
		}
	}

	return admission.Allowed("No differences concerning RemoteTargets")
}

func (rsyt *RemoteSyncerTargetPatternWebhookHandler) createRemoteTargets(ctx context.Context, remoteSyncer syngit.RemoteSyncer) error {
	rsyAnnotations := remoteSyncer.Annotations

	if rsyAnnotations[syngit.RtAnnotationBranches] == "" {
		// We take the RemoteSyncer's default branch by default
		branch := remoteSyncer.Spec.DefaultBranch
		name, nameErr := utils.RemoteTargetNameConstructor(remoteSyncer, branch)
		if nameErr != nil {
			return nameErr
		}
		rt := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: remoteSyncer.Namespace,
				Labels: map[string]string{
					syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
					syngit.RtLabelBranchKey:  branch,
				},
			},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: remoteSyncer.Spec.RemoteRepository,
				UpstreamBranch:     remoteSyncer.Spec.DefaultBranch,
				TargetRepository:   remoteSyncer.Spec.RemoteRepository,
				TargetBranch:       branch,
			},
		}
		createOrUpdateErr := rsyt.createOrUpdateRemoteTarget(ctx, rt)
		if createOrUpdateErr != nil {
			return createOrUpdateErr
		}

	} else {

		branches := strings.Split(strings.ReplaceAll(rsyAnnotations[syngit.RtAnnotationBranches], " ", ""), ",")

		for _, branch := range branches {
			name, nameErr := utils.RemoteTargetNameConstructor(remoteSyncer, branch)
			if nameErr != nil {
				return nameErr
			}
			mergeStrategy := syngit.TryPullOrDie
			if remoteSyncer.Spec.DefaultBranch == branch {
				mergeStrategy = ""
			}
			rt := &syngit.RemoteTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: remoteSyncer.Namespace,
					Labels: map[string]string{
						syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
						syngit.RtLabelBranchKey:  branch,
					},
				},
				Spec: syngit.RemoteTargetSpec{
					UpstreamRepository: remoteSyncer.Spec.RemoteRepository,
					UpstreamBranch:     remoteSyncer.Spec.DefaultBranch,
					TargetRepository:   remoteSyncer.Spec.RemoteRepository,
					TargetBranch:       branch,
					MergeStrategy:      mergeStrategy,
				},
			}
			createOrUpdateErr := rsyt.createOrUpdateRemoteTarget(ctx, rt)
			if createOrUpdateErr != nil {
				return createOrUpdateErr
			}
		}
	}

	return nil
}

func (rsyt *RemoteSyncerTargetPatternWebhookHandler) createOrUpdateRemoteTarget(ctx context.Context, rt *syngit.RemoteTarget) error {
	if createErr := rsyt.Client.Create(ctx, rt); createErr != nil {
		if !strings.Contains(createErr.Error(), "already exists") {
			return createErr
		}
	}
	rubs := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: rt.Namespace,
	}
	listErr := rsyt.Client.List(ctx, rubs, listOps)
	if listErr != nil {
		return listErr
	}

	for _, rub := range rubs.Items {
		newRtRefs := append(rub.Spec.DeepCopy().RemoteTargetRefs, v1.ObjectReference{
			Name: rt.Name,
		})

		rub.Spec.RemoteTargetRefs = newRtRefs
		updateErr := rsyt.updateRemoteUserBinding(ctx, rub, 2)
		if updateErr != nil {
			return updateErr
		}
	}

	return nil
}

func (rsyt *RemoteSyncerTargetPatternWebhookHandler) isThereDiff(old syngit.RemoteSyncer, new syngit.RemoteSyncer) bool {

	oldRepo := old.Spec.RemoteRepository
	oldBranch := old.Spec.DefaultBranch
	oldAnnotations := old.Annotations
	newRepo := new.Spec.RemoteRepository
	newBranch := new.Spec.DefaultBranch
	newAnnotations := new.Annotations

	if oldAnnotations[syngit.RtAnnotationEnabled] != newAnnotations[syngit.RtAnnotationEnabled] {
		return true
	}
	if oldAnnotations[syngit.RtAnnotationBranches] != newAnnotations[syngit.RtAnnotationBranches] {
		return true
	}
	if oldRepo != newRepo {
		return true
	}
	if oldBranch != newBranch {
		return true
	}

	return false
}

// Search for automatically created RemoteTargets that are used by the RemoteSyncer
func (rsyt *RemoteSyncerTargetPatternWebhookHandler) searchForRemoteTargetsDependencies(ctx context.Context, remoteSyncer syngit.RemoteSyncer) ([]syngit.RemoteTarget, error) {
	rts := &syngit.RemoteTargetList{}
	listOps := &client.ListOptions{
		Namespace: remoteSyncer.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
		}),
	}
	listErr := rsyt.Client.List(ctx, rts, listOps)
	if listErr != nil {
		return nil, listErr
	}

	referencedRemoteTargets := []syngit.RemoteTarget{}
	rsyRepo := remoteSyncer.Spec.RemoteRepository
	rsyBranch := remoteSyncer.Spec.DefaultBranch
	rsyAnnotations := remoteSyncer.Annotations

	branchesAnnotation := rsyAnnotations[syngit.RtAnnotationBranches]
	branches := []string{rsyBranch}
	if branchesAnnotation != "" {
		branches = strings.Split(strings.ReplaceAll(branchesAnnotation, " ", ""), ",")
	}
	for _, rt := range rts.Items {
		if rt.Spec.UpstreamRepository == rsyRepo && rt.Spec.UpstreamBranch == rsyBranch && rt.Spec.TargetRepository == rsyRepo && slices.Contains(branches, rt.Spec.TargetBranch) {
			referencedRemoteTargets = append(referencedRemoteTargets, rt)
		}

		if rsyAnnotations[syngit.RtAnnotationUserSpecific] == string(syngit.RtAnnotationOneUserOneForkValue) {
			if rt.Spec.UpstreamRepository == rsyRepo && rt.Spec.UpstreamBranch == rsyBranch {
				referencedRemoteTargets = append(referencedRemoteTargets, rt)
			}
		}
		if rsyAnnotations[syngit.RtAnnotationUserSpecific] == string(syngit.RtAnnotationOneUserOneBranchValue) {
			username := rt.Labels[syngit.K8sUserLabelKey]
			if username != "" {
				if rt.Spec.UpstreamRepository == rsyRepo && rt.Spec.UpstreamBranch == rsyBranch && rt.Spec.TargetRepository == rsyRepo && rt.Spec.TargetBranch == username {
					referencedRemoteTargets = append(referencedRemoteTargets, rt)
				}
			}
		}
	}

	return referencedRemoteTargets, nil
}

func (rsyt *RemoteSyncerTargetPatternWebhookHandler) convertRemoteTargetsToNamespacedName(remoteTargets []syngit.RemoteTarget) []types.NamespacedName {
	namespacedNames := []types.NamespacedName{}
	for _, rt := range remoteTargets {
		nn := types.NamespacedName{
			Name:      rt.Name,
			Namespace: rt.Namespace,
		}
		namespacedNames = append(namespacedNames, nn)
	}
	return namespacedNames
}

// Can be referenced by another RemoteSyncer that have the same upstream repo & branch
func (rsyt *RemoteSyncerTargetPatternWebhookHandler) deleteRemoteTargetsIfNotReferenced(ctx context.Context, oldRemoteSyncer syngit.RemoteSyncer) error {
	rsys := &syngit.RemoteSyncerList{}
	listOps := &client.ListOptions{
		Namespace: oldRemoteSyncer.Namespace,
	}
	listErr := rsyt.Client.List(ctx, rsys, listOps)
	if listErr != nil {
		return listErr
	}

	// List RemoteTargets that scopes the OLD RemoteSyncer's upstream repo & branch
	referencedRemoteTargets, refErr := rsyt.searchForRemoteTargetsDependencies(ctx, oldRemoteSyncer)
	if refErr != nil {
		return refErr
	}
	referencedRemoteTargetsNN := rsyt.convertRemoteTargetsToNamespacedName(referencedRemoteTargets)

	dontTouchToThese := []types.NamespacedName{}
	// Loop over each RemoteSyncer of the namespace
	for _, rsy := range rsys.Items {

		if rsy.Name != oldRemoteSyncer.Name && rsy.Namespace != oldRemoteSyncer.Namespace {
			// List RemoteTargets that scopes the NEW RemoteSyncer's upstream repo & branch
			specificRsyReferences, refErr := rsyt.searchForRemoteTargetsDependencies(ctx, rsy)
			if refErr != nil {
				return refErr
			}
			specificRsyReferencesNN := rsyt.convertRemoteTargetsToNamespacedName(specificRsyReferences)

			// If a RemoteTarget that scopes the OLD INCOMING RemoteSyncer's specs is part of
			// the slice of the CURRENT loop RemoteSyncer, then do not touch to it.
			// If the RemoteTargets that could be deleted are referenced by the CURRENT loop
			// RemoteSyncer, then it is a dependency and we must not delete it.
			for _, specificRsy := range specificRsyReferencesNN {
				if slices.Contains(referencedRemoteTargetsNN, specificRsy) {
					dontTouchToThese = append(dontTouchToThese, specificRsy)
				}
			}
		}

	}

	for _, rt := range referencedRemoteTargets {
		nn := types.NamespacedName{
			Name:      rt.Name,
			Namespace: rt.Namespace,
		}
		if !slices.Contains(dontTouchToThese, nn) {
			delErr := rsyt.Client.Delete(ctx, &rt)
			if delErr != nil {
				return delErr
			}
			updateErr := rsyt.removeFromRemoteUserBinding(ctx, oldRemoteSyncer, rt)
			if updateErr != nil {
				return updateErr
			}
		}
	}

	return nil
}

func (rsyt RemoteSyncerTargetPatternWebhookHandler) removeFromRemoteUserBinding(ctx context.Context, remoteSyncer syngit.RemoteSyncer, remoteTarget syngit.RemoteTarget) error {
	rubs := &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: remoteSyncer.Namespace,
	}
	listErr := rsyt.Client.List(ctx, rubs, listOps)
	if listErr != nil {
		return listErr
	}

	for _, rub := range rubs.Items {
		newRtRefs := []v1.ObjectReference{}
		for _, rtRef := range rub.Spec.RemoteTargetRefs {
			if rtRef.Name != remoteTarget.Name {
				newRtRefs = append(newRtRefs, rtRef)
			}
		}

		rub.Spec.RemoteTargetRefs = newRtRefs
		updateErr := rsyt.updateRemoteUserBinding(ctx, rub, 2)
		if updateErr != nil {
			return updateErr
		}
	}

	return nil
}

func (rsyt RemoteSyncerTargetPatternWebhookHandler) updateRemoteUserBinding(ctx context.Context, remoteUserBinding syngit.RemoteUserBinding, retryNumber int) error {
	var rub syngit.RemoteUserBinding
	if err := rsyt.Client.Get(ctx, types.NamespacedName{Name: remoteUserBinding.Name, Namespace: remoteUserBinding.Namespace}, &rub); err != nil {
		return err
	}

	rub.Spec.RemoteTargetRefs = remoteUserBinding.Spec.RemoteTargetRefs
	if err := rsyt.Client.Update(ctx, &rub); err != nil {
		if retryNumber > 0 {
			return rsyt.updateRemoteUserBinding(ctx, remoteUserBinding, retryNumber-1)
		}
		return err
	}
	return nil
}
