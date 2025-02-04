package v1beta3

// type RemoteUserBindingOneOrManyBranchPattern struct {
// 	PatternSpecification
// 	RemoteUserBinding *syngit.RemoteUserBinding
// }

// func (rubomp *RemoteUserBindingOneOrManyBranchPattern) Trigger(ctx context.Context) *errorPattern {

// 	removeErr := rubomp.RemoveExistingOnes(ctx)
// 	if removeErr != nil {
// 		return &errorPattern{Message: removeErr.Error(), Reason: Errored}
// 	}

// 	spec, err := rubomp.specAdder(ctx)
// 	if err != nil {
// 		return &errorPattern{Message: err.Error(), Reason: Errored}
// 	}

// 	updateErr := updateOrDeleteRemoteUserBinding(ctx, rubomp.Client, spec, *rubomp.RemoteUserBinding, 2)
// 	if updateErr != nil {
// 		return &errorPattern{Message: updateErr.Error(), Reason: Errored}
// 	}

// 	return nil
// }

// func (rubomp *RemoteUserBindingOneOrManyBranchPattern) RemoveExistingOnes(ctx context.Context) error {

// 	spec, err := rubomp.specRemover(ctx)
// 	if err != nil {
// 		return err
// 	}

// 	updateErr := updateOrDeleteRemoteUserBinding(ctx, rubomp.Client, spec, *rubomp.RemoteUserBinding, 2)
// 	if updateErr != nil {
// 		return updateErr
// 	}

// 	return nil
// }

// func (rubomp *RemoteUserBindingOneOrManyBranchPattern) SpecConstructor(ctx context.Context) (syngit.RemoteUserBindingSpec, error) {

// 	spec, err := rubomp.specRemover(ctx)
// 	if err != nil {
// 		return syngit.RemoteUserBindingSpec{}, err
// 	}

// 	rubomp.RemoteUserBinding.Spec = spec
// 	spec, err = rubomp.specAdder(ctx)
// 	if err != nil {
// 		return syngit.RemoteUserBindingSpec{}, err
// 	}

// 	return spec, nil
// }

// func (rubomp *RemoteUserBindingOneOrManyBranchPattern) specAdder(ctx context.Context) (syngit.RemoteUserBindingSpec, error) {
// 	// Search for all RemoteTargets that have a "one or many branches" pattern
// 	remoteTargets := &syngit.RemoteTargetList{}
// 	listOps := &client.ListOptions{
// 		Namespace: rubomp.NamespacedName.Namespace,
// 		LabelSelector: labels.SelectorFromSet(labels.Set{
// 			syngit.ManagedByLabelKey: syngit.ManagedByLabelValue,
// 			syngit.RtLabelPatternKey: syngit.RtLabelOneOrManyBranchesValue,
// 		}),
// 	}
// 	listErr := rubomp.Client.List(ctx, remoteTargets, listOps)
// 	if listErr != nil {
// 		return syngit.RemoteUserBindingSpec{}, nil
// 	}

// 	newRemoteTargetRefs := []v1.ObjectReference{}
// 	for _, remoteTarget := range remoteTargets.Items {
// 		newRemoteTargetRefs = append(
// 			newRemoteTargetRefs,
// 			v1.ObjectReference{Name: remoteTarget.Name},
// 		)
// 	}

// 	spec := rubomp.RemoteUserBinding.Spec
// 	spec.RemoteTargetRefs = append(spec.RemoteTargetRefs, newRemoteTargetRefs...)

// 	return spec, nil
// }

// func (rubomp *RemoteUserBindingOneOrManyBranchPattern) specRemover(ctx context.Context) (syngit.RemoteUserBindingSpec, error) {
// 	newRemoteTargetRefs := []v1.ObjectReference{}
// 	for _, remoteTargetRef := range rubomp.RemoteUserBinding.Spec.RemoteTargetRefs {
// 		remoteTarget := &syngit.RemoteTarget{}
// 		getErr := rubomp.Client.Get(
// 			ctx,
// 			types.NamespacedName{Name: remoteTarget.Name, Namespace: rubomp.NamespacedName.Namespace},
// 			remoteTarget,
// 		)
// 		if getErr != nil {
// 			return syngit.RemoteUserBindingSpec{}, getErr
// 		}

// 		label := remoteTarget.Labels[syngit.RtLabelPatternKey]

// 		if label != syngit.RtLabelOneOrManyBranchesValue {
// 			newRemoteTargetRefs = append(newRemoteTargetRefs, remoteTargetRef)
// 		}
// 	}

// 	spec := rubomp.RemoteUserBinding.Spec
// 	spec.RemoteTargetRefs = newRemoteTargetRefs

// 	return spec, nil
// }
