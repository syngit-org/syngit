package interceptor

import (
	"context"
	"fmt"
	"net/url"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"github.com/syngit-org/syngit/pkg/utils"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Find the RemoteTargets associated to the user.
// If no RemoteTargets found, then fallback to the
// RemoteSyncer's default RemoteTarget.
// Returns a map of the credentials to access to
// the target defined by the RemoteTarget
func GetUserInfoRemoteTargetsAssociation( // nolint: gocyclo
	ctx context.Context,
	user authenticationv1.UserInfo,
	remoteSyncerRemoteRepoUrl *url.URL,
	remoteSyncer syngit.RemoteSyncer,
) (map[interceptor.GitUserInfo][]syngit.RemoteTarget, error) {
	// Set empty map of GitUserInfo/RemoteTargets
	userTargetsMap := map[interceptor.GitUserInfo][]syngit.RemoteTarget{}

	remoteUserBinding, err := GetRemoteUserBindingByUsername(
		ctx,
		remoteSyncer,
		user.Username,
		remoteSyncerRemoteRepoUrl.Host,
	)
	if err != nil {
		return userTargetsMap, err
	}

	k8sClient := utils.K8sClientFromContext(ctx)

	if remoteUserBinding != nil {
		// User-specific RemoteTargets are now pre-created by the user-specific policy
		// (run inside RemoteSyncerReconciler). The RUB already contains all the
		// necessary RemoteTarget refs.
		//
		// Both ref lists are resolved one by one instead of being listed and
		// intersected by name: a reference may point outside of the binding's
		// namespace, which a namespaced List would never surface. A reference
		// that does not resolve to an existing object is skipped.

		// Search for RemoteTargets
		var labelSelector labels.Selector
		if remoteSyncer.Spec.RemoteTargetSelector != nil {
			selector, err := v1.LabelSelectorAsSelector(remoteSyncer.Spec.RemoteTargetSelector)
			if err != nil {
				return userTargetsMap, syngiterrors.NewWrongLabelParsing(fmt.Sprintf("error parsing the LabelSelector for the remoteTargetSelector: %v", err))
			}
			labelSelector = selector
		}

		remoteTargets, err := resolveRefs(ctx, remoteUserBinding.Spec.RemoteTargetRefs,
			remoteUserBinding.Namespace, "remoteTargetRefs",
			func() *syngit.RemoteTarget { return &syngit.RemoteTarget{} })
		if err != nil {
			return userTargetsMap, err
		}

		// Search for RemoteUsers
		remoteUsers, err := resolveRefs(ctx, remoteUserBinding.Spec.RemoteUserRefs,
			remoteUserBinding.Namespace, "remoteUserRefs",
			func() *syngit.RemoteUser { return &syngit.RemoteUser{} })
		if err != nil {
			return userTargetsMap, err
		}

		// Associate RemoteUser with RemoteTarget
		for _, remoteTarget := range remoteTargets {
			// The selector was applied by the API server before; now that the
			// targets are fetched by name it has to be applied here.
			if labelSelector != nil && !labelSelector.Matches(labels.Set(remoteTarget.Labels)) {
				continue
			}
			rtUrl, err := url.Parse(remoteTarget.Spec.TargetRepository)
			if err != nil {
				return userTargetsMap, err
			}
			if remoteTarget.Spec.UpstreamRepository == remoteSyncer.Spec.RemoteRepository && remoteTarget.Spec.UpstreamBranch == remoteSyncer.Spec.DefaultBranch {
				for _, remoteUser := range remoteUsers {
					if rtUrl.Host == remoteUser.Spec.GitBaseDomainFQDN {
						gitUserInfo, err := GetGitUserInfoByRemoteUser(ctx, *remoteUser)
						if err != nil {
							return userTargetsMap, err
						}
						userTargetsMap[*gitUserInfo] = append(userTargetsMap[*gitUserInfo], *remoteTarget)
					}
				}
			}
		}

		totalTargets := 0
		for _, targets := range userTargetsMap {
			totalTargets += len(targets)
		}
		if remoteSyncer.Spec.TargetStrategy == syngit.OneTarget && totalTargets > 1 {
			return userTargetsMap, syngiterrors.NewTooMuchRemoteTarget("multiple RemoteTargets found for OneTarget set as the TargetStrategy in the RemoteSyncer", totalTargets)
		}

		if len(userTargetsMap) == 0 {
			return userTargetsMap, syngiterrors.NewRemoteTargetNotFound("no matching remote target found")
		}

	} else {
		// No RUB with the right targets and associated to this user found.
		// Fallback to default user.
		// Check if there is a default user that we can use

		if remoteSyncer.Spec.DefaultUnauthorizedUserMode != syngit.UseDefaultUser || remoteSyncer.Spec.DefaultRemoteUserRef == nil || remoteSyncer.Spec.DefaultRemoteUserRef.Name == "" {
			return userTargetsMap, syngiterrors.NewRemoteUserBindingNotFound(user.Username)
		}

		// Search for the default RemoteUser object
		userNamespace, err := utils.ResolveNamespace(
			remoteSyncer.Spec.DefaultRemoteUserRef.Namespace,
			remoteSyncer.Namespace,
			field.NewPath("spec", "defaultRemoteUserRef"),
		)
		if err != nil {
			return userTargetsMap, err
		}
		userNamespacedName := &types.NamespacedName{
			Namespace: userNamespace,
			Name:      remoteSyncer.Spec.DefaultRemoteUserRef.Name,
		}
		remoteUser := &syngit.RemoteUser{}
		if err := k8sClient.Get(ctx, *userNamespacedName, remoteUser); err != nil {
			return userTargetsMap, syngiterrors.NewRemoteUserNotFound("the default RemoteUser is not found")
		}

		if remoteUser.Spec.GitBaseDomainFQDN != remoteSyncerRemoteRepoUrl.Host {
			return userTargetsMap, syngiterrors.NewWrongRemoteTargetConfig(remoteSyncer, *remoteUser)
		}
		gitUserInfo, err := GetGitUserInfoByRemoteUser(ctx, *remoteUser)
		if err != nil {
			return userTargetsMap, err
		}

		// Search for the default RemoteTarget
		if remoteSyncer.Spec.DefaultRemoteTargetRef == nil || remoteSyncer.Spec.DefaultRemoteTargetRef.Name == "" {
			return userTargetsMap, syngiterrors.NewRemoteTargetNotFound("no default remote target is set")
		}
		targetNamespace, err := utils.ResolveNamespace(
			remoteSyncer.Spec.DefaultRemoteTargetRef.Namespace,
			remoteSyncer.Namespace,
			field.NewPath("spec", "defaultRemoteTargetRef"),
		)
		if err != nil {
			return userTargetsMap, err
		}
		targetNamespacedName := &types.NamespacedName{
			Namespace: targetNamespace,
			Name:      remoteSyncer.Spec.DefaultRemoteTargetRef.Name,
		}
		remoteTarget := &syngit.RemoteTarget{}
		err = k8sClient.Get(ctx, *targetNamespacedName, remoteTarget)
		if err != nil {
			return userTargetsMap, syngiterrors.NewRemoteTargetNotFound("default remote target does not exist: " + remoteSyncer.Spec.DefaultRemoteTargetRef.Name)
		}

		if remoteTarget.Spec.UpstreamRepository != remoteSyncer.Spec.RemoteRepository || remoteTarget.Spec.UpstreamBranch != remoteSyncer.Spec.DefaultBranch {
			return userTargetsMap, syngiterrors.NewWrongRemoteSyncerConfig(fmt.Sprintf(
				"the RemoteSyncer's repository or branch does not match the upstream repository or branch of the default RemoteTarget. RemoteSyncer repo: %s; RemoteSyncer branch: %s; RemoteTarget upstream repo: %s; RemoteTarget upstream branch: %s", //nolint:lll
				remoteTarget.Spec.UpstreamRepository,
				remoteTarget.Spec.UpstreamBranch,
				remoteTarget.Spec.TargetRepository,
				remoteTarget.Spec.TargetBranch,
			))
		}

		userTargetsMap[*gitUserInfo] = append(userTargetsMap[*gitUserInfo], *remoteTarget)
	}

	return userTargetsMap, nil
}

// Find the RemoteUserBinding associated to the k8s username.
// The searching is also based on potential label selectors
// set in the RemoteSyncer.
func GetRemoteUserBindingByUsername(
	ctx context.Context,
	remoteSyncer syngit.RemoteSyncer,
	username, fqdn string,
) (*syngit.RemoteUserBinding, error) {
	k8sClient := utils.K8sClientFromContext(ctx)

	var remoteUserBindings = &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: remoteSyncer.Namespace,
	}
	if remoteSyncer.Spec.RemoteUserBindingSelector != nil {
		labelSelector, labelErr := v1.LabelSelectorAsSelector(remoteSyncer.Spec.RemoteUserBindingSelector)
		if labelErr != nil {
			return nil, syngiterrors.NewWrongLabelParsing(fmt.Sprintf("error parsing the LabelSelector for the remoteUserBindingSelector: %v", labelErr))
		}
		listOps.LabelSelector = labelSelector
	}
	err := k8sClient.List(ctx, remoteUserBindings, listOps)

	if err != nil {
		return nil, err
	}

	var rub syngit.RemoteUserBinding
	userCountLoop := 0 // Prevent non-unique name attack
	for _, rubItem := range remoteUserBindings.Items {
		// The subject name can not be unique -> in specific conditions, a commit can be done as another user
		// TODO: need to be studied
		if rubItem.Spec.Subject.Name == username {

			_, err = GetGitUserInfoByRemoteUserBinding(ctx, remoteSyncer, rubItem, fqdn)
			if err != nil {
				return nil, err
			}
			userCountLoop++

			rub = rubItem
		}
	}

	if userCountLoop > 1 {
		return nil, syngiterrors.NewTooMuchRemoteUserBinding(
			"multiple RemoteUserBinding found OR the name of the user is not unique; this version of the operator work with the name as unique identifier for users",
			userCountLoop,
		)
	}

	if userCountLoop == 0 {
		return nil, nil
	}

	remoteUserBinding := &syngit.RemoteUserBinding{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace}, remoteUserBinding)
	if err != nil {
		return nil, err
	}

	return remoteUserBinding, nil
}

func GetGitUserInfoByRemoteUserBinding(
	ctx context.Context,
	remoteSyncer syngit.RemoteSyncer,
	rub syngit.RemoteUserBinding,
	fqdn string,
) (*interceptor.GitUserInfo, error) {
	remoteUserCount := 0

	k8sClient := utils.K8sClientFromContext(ctx)

	var gitUser *interceptor.GitUserInfo

	// Each reference resolves against the binding that holds it, not against the
	// RemoteSyncer that led us here.
	for i, ref := range rub.Spec.RemoteUserRefs {
		namespace, err := utils.ResolveNamespace(
			ref.Namespace, rub.Namespace, field.NewPath("spec", "remoteUserRefs").Index(i),
		)
		if err != nil {
			return nil, err
		}
		namespacedName := &types.NamespacedName{
			Namespace: namespace,
			Name:      ref.Name,
		}
		remoteUser := &syngit.RemoteUser{}
		if err := k8sClient.Get(ctx, *namespacedName, remoteUser); err != nil {
			continue
		}

		if remoteUser.Spec.GitBaseDomainFQDN == fqdn {
			remoteUserCount++
			gitUser, err = GetGitUserInfoByRemoteUser(ctx, *remoteUser)
			if err != nil {
				return nil, err
			}
		}
	}

	if remoteUserCount == 0 {
		return nil, syngiterrors.NewRemoteUserNotFound("no RemoteUser found for the current user for " + fqdn)
	}
	if remoteUserCount > 1 {
		return nil, syngiterrors.NewTooMuchRemoteUser(
			"more than one RemoteUser found for the current user for"+fqdn,
			remoteUserCount,
		)
	}

	return gitUser, nil
}

// Reads the credentials of a RemoteUser.
func GetGitUserInfoByRemoteUser(
	ctx context.Context,
	remoteUser syngit.RemoteUser,
) (*interceptor.GitUserInfo, error) {
	k8sClient := utils.K8sClientFromContext(ctx)

	secretNamespace, err := utils.ResolveNamespace(
		remoteUser.Spec.SecretRef.Namespace,
		remoteUser.Namespace,
		field.NewPath("spec", "secretRef"),
	)
	if err != nil {
		return nil, err
	}

	secretNamespacedName := &types.NamespacedName{
		Namespace: secretNamespace,
		Name:      remoteUser.Spec.SecretRef.Name,
	}
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, *secretNamespacedName, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, syngiterrors.NewCredentialsNotFound("secret not found for remote user: "+remoteUser.Name, secretNamespacedName.Name)
		}
		return nil, syngiterrors.NewCredentialsNotFound("connection error", secretNamespacedName.Name)
	}

	token := string(secret.Data["password"])

	gitUser := &interceptor.GitUserInfo{
		User:  string(secret.Data["username"]),
		Email: remoteUser.Spec.Email,
		Token: token,
	}

	if token == "" {
		return nil, syngiterrors.NewCredentialsNotFound(
			"token not found; the token must be specified in the password field and the secret type must be kubernetes.io/basic-auth",
			secretNamespacedName.Name,
		)
	}

	return gitUser, nil
}
