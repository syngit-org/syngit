package interceptor

import (
	"context"
	"net/url"
	"slices"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"github.com/syngit-org/syngit/pkg/utils"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GitUserInfo struct {
	user  string
	email string
	token string
}

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
) (map[GitUserInfo][]syngit.RemoteTarget, error) {
	// Set empty map of GitUserInfo/RemoteTargets
	userTargetsMap := map[GitUserInfo][]syngit.RemoteTarget{}

	remoteUserBinding, err := GetRemoteUserBindingByUsername(
		ctx,
		remoteSyncer,
		user.Username,
		remoteSyncerRemoteRepoUrl.Host,
	)
	if err != nil {
		return userTargetsMap, err
	}

	k8sClient := K8sClientFromContext(ctx)

	if remoteUserBinding != nil {
		// User-specific RemoteTargets are now pre-created by the UserSpecificPolicyReconciler.
		// The RUB already contains all the necessary RemoteTarget refs.

		// Search for RemoteTargets
		remoteTargetRefNames := make([]string, 0, len(remoteUserBinding.Spec.RemoteTargetRefs))
		for _, remoteTargetRef := range remoteUserBinding.Spec.RemoteTargetRefs {
			remoteTargetRefNames = append(remoteTargetRefNames, remoteTargetRef.Name)
		}
		var remoteTargetList = &syngit.RemoteTargetList{}
		listOps := &client.ListOptions{
			Namespace: remoteSyncer.Namespace,
		}
		if remoteSyncer.Spec.RemoteTargetSelector != nil {
			labelSelector, err := v1.LabelSelectorAsSelector(remoteSyncer.Spec.RemoteTargetSelector)
			if err != nil {
				parseLabelErr := utils.LabelSeletorParsingError{Kind: utils.RemoteTargetSelectorError, LabelError: err}
				return userTargetsMap, parseLabelErr
			}
			listOps.LabelSelector = labelSelector
		}
		err := k8sClient.List(ctx, remoteTargetList, listOps)
		if err != nil {
			return userTargetsMap, err
		}

		// Search for RemoteUsers
		remoteUserRefNames := make([]string, 0, len(remoteUserBinding.Spec.RemoteUserRefs))
		for _, remoteUserRef := range remoteUserBinding.Spec.RemoteUserRefs {
			remoteUserRefNames = append(remoteUserRefNames, remoteUserRef.Name)
		}

		listOps = &client.ListOptions{
			Namespace: remoteSyncer.Namespace,
		}
		var remoteUserList = &syngit.RemoteUserList{}
		err = k8sClient.List(ctx, remoteUserList, listOps)
		if err != nil {
			return userTargetsMap, err
		}

		// Associate RemoteUser with RemoteTarget
		rtUrl := &url.URL{}
		for _, remoteTarget := range remoteTargetList.Items {
			rtUrl, err = url.Parse(remoteTarget.Spec.TargetRepository)
			if err != nil {
				return userTargetsMap, err
			}
			if slices.Contains(remoteTargetRefNames, remoteTarget.Name) {
				if remoteTarget.Spec.UpstreamRepository == remoteSyncer.Spec.RemoteRepository && remoteTarget.Spec.UpstreamBranch == remoteSyncer.Spec.DefaultBranch {
					for _, remoteUser := range remoteUserList.Items {
						if slices.Contains(remoteUserRefNames, remoteUser.Name) {
							if rtUrl.Host == remoteUser.Spec.GitBaseDomainFQDN {
								gitUserInfo, err := GetGitUserInfoByRemoteUser(ctx, remoteUser, remoteSyncer.Namespace)
								if err != nil {
									return userTargetsMap, err
								}
								userTargetsMap[*gitUserInfo] = append(userTargetsMap[*gitUserInfo], remoteTarget)
							}
						}
					}
				}
			}
		}

		totalTargets := 0
		for _, targets := range userTargetsMap {
			totalTargets += len(targets)
		}
		if remoteSyncer.Spec.TargetStrategy == syngit.OneTarget && totalTargets > 1 {
			multipleTargetError := utils.MultipleTargetError{RemoteTargetsCount: totalTargets}
			return userTargetsMap, multipleTargetError
		}

		if len(userTargetsMap) == 0 {
			notFoundError := utils.RemoteTargetNotFoundError{}
			return userTargetsMap, notFoundError
		}

	} else {
		// No RUB with the right targets and associated to this user found.
		// Fallback to default user.
		// Check if there is a default user that we can use

		if remoteSyncer.Spec.DefaultUnauthorizedUserMode != syngit.UseDefaultUser || remoteSyncer.Spec.DefaultRemoteUserRef == nil || remoteSyncer.Spec.DefaultRemoteUserRef.Name == "" {
			notFoundError := utils.RemoteUserBindingNotFoundError{Username: user.Username}
			return userTargetsMap, notFoundError
		}

		// Search for the default RemoteUser object
		userNamespacedName := &types.NamespacedName{
			Namespace: remoteSyncer.Namespace,
			Name:      remoteSyncer.Spec.DefaultRemoteUserRef.Name,
		}
		remoteUser := &syngit.RemoteUser{}
		err := k8sClient.Get(ctx, *userNamespacedName, remoteUser)
		if err != nil {
			notFoundError := utils.DefaultRemoteUserNotFoundError{DefaultUserName: remoteSyncer.Spec.DefaultRemoteUserRef.Name}
			return userTargetsMap, notFoundError
		}

		if remoteUser.Spec.GitBaseDomainFQDN != remoteSyncerRemoteRepoUrl.Host {
			mismatchErr := utils.DefaultRemoteTargetMismatchError{RemoteSyncer: remoteSyncer, RemoteUser: *remoteUser}
			return userTargetsMap, mismatchErr
		}
		gitUserInfo, err := GetGitUserInfoByRemoteUser(ctx, *remoteUser, remoteSyncer.Namespace)
		if err != nil {
			return userTargetsMap, err
		}

		// Search for the default RemoteTarget
		targetNamespacedName := &types.NamespacedName{
			Namespace: remoteSyncer.Namespace,
			Name:      remoteSyncer.Spec.DefaultRemoteTargetRef.Name,
		}
		remoteTarget := &syngit.RemoteTarget{}
		err = k8sClient.Get(ctx, *targetNamespacedName, remoteTarget)
		if err != nil {
			notFoundError := utils.DefaultRemoteTargetNotFoundError{DefaultTargetName: remoteSyncer.Spec.DefaultRemoteTargetRef.Name}
			return userTargetsMap, notFoundError
		}

		if remoteTarget.Spec.UpstreamRepository != remoteSyncer.Spec.RemoteRepository || remoteTarget.Spec.UpstreamBranch != remoteSyncer.Spec.DefaultBranch {
			remoteTargetSearchError := utils.RemoteTargetSearchError{
				UpstreamRepository: remoteSyncer.Spec.RemoteRepository,
				UpstreamBranch:     remoteSyncer.Spec.DefaultBranch,
				TargetRepository:   remoteTarget.Spec.UpstreamRepository,
				TargetBranch:       remoteTarget.Spec.UpstreamBranch,
			}
			return userTargetsMap, remoteTargetSearchError
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
	k8sClient := K8sClientFromContext(ctx)

	var remoteUserBindings = &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: remoteSyncer.Namespace,
	}
	if remoteSyncer.Spec.RemoteUserBindingSelector != nil {
		labelSelector, labelErr := v1.LabelSelectorAsSelector(remoteSyncer.Spec.RemoteUserBindingSelector)
		if labelErr != nil {
			parseLabelError := utils.LabelSeletorParsingError{Kind: utils.RemoteUserBindingSelectorError, LabelError: labelErr}
			return nil, parseLabelError
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
		multipleRubError := utils.MultipleRemoteUserBindingError{RemoteUserBindingsCount: userCountLoop}
		return nil, multipleRubError
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
) (*GitUserInfo, error) {
	remoteUserCount := 0

	k8sClient := K8sClientFromContext(ctx)

	var gitUser *GitUserInfo

	namespace := remoteSyncer.Namespace
	for _, ref := range rub.Spec.RemoteUserRefs {
		namespacedName := &types.NamespacedName{
			Namespace: namespace,
			Name:      ref.Name,
		}
		remoteUser := &syngit.RemoteUser{}
		err := k8sClient.Get(ctx, *namespacedName, remoteUser)
		if err != nil {
			continue
		}

		if remoteUser.Spec.GitBaseDomainFQDN == fqdn {
			remoteUserCount++
			gitUser, err = GetGitUserInfoByRemoteUser(ctx, *remoteUser, namespace)
			if err != nil {
				return nil, err
			}
		}
	}

	if remoteUserCount == 0 {
		remoteUserSearchError := utils.RemoteUserSearchError{Reason: utils.RemoteUserNotFound, Fqdn: fqdn}
		return nil, remoteUserSearchError
	}
	if remoteUserCount > 1 {
		remoteUserSearchError := utils.RemoteUserSearchError{Reason: utils.MoreThanOneRemoteUserFound, Fqdn: fqdn}
		return nil, remoteUserSearchError
	}

	return gitUser, nil
}

func GetGitUserInfoByRemoteUser(
	ctx context.Context,
	remoteUser syngit.RemoteUser,
	namespace string,
) (*GitUserInfo, error) {
	k8sClient := K8sClientFromContext(ctx)

	secretNamespacedName := &types.NamespacedName{
		Namespace: namespace,
		Name:      remoteUser.Spec.SecretRef.Name,
	}
	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, *secretNamespacedName, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			searchError := utils.CrendentialSearchError{Reason: utils.SecretNotFound, RemoteUser: remoteUser}
			return nil, searchError
		}
		searchError := utils.CrendentialSearchError{Reason: utils.ConnectionError, RemoteUser: remoteUser}
		return nil, searchError
	}

	token := string(secret.Data["password"])

	gitUser := &GitUserInfo{
		user:  string(secret.Data["username"]),
		email: remoteUser.Spec.Email,
		token: token,
	}

	if token == "" {
		searchError := utils.CrendentialSearchError{Reason: utils.TokenNotFound, RemoteUser: remoteUser}
		return nil, searchError
	}

	return gitUser, nil
}
