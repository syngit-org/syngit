package interceptor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	patterns "github.com/syngit-org/syngit/internal/patterns/v1beta3"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"github.com/syngit-org/syngit/pkg/utils"
	"gopkg.in/yaml.v3"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type gitUser struct {
	gitUser  string
	gitEmail string
	gitToken string
}

type wrcDetails struct {
	// Incoming object information
	interceptedGVR  schema.GroupVersionResource
	interceptedName string
	interceptedYAML string

	// Process checking information
	processPass bool

	// Front webhook information
	requestUID      types.UID
	webhookPass     bool
	messageAddition string

	// GitPusher information
	serverHost             string
	caBundle               []byte
	gitUser                gitUser
	targetsPushInformation []pushInformation
	pushDetails            string

	// Error
	errorDuringProcess bool
}

type pushInformation struct {
	repoUrl    string
	repoPath   string
	commitHash string
}

const (
	defaultFailureMessage   = "The changes have not been pushed to the remote git repository:"
	defaultSuccessMessage   = "The changes were correctly been pushed on the remote git repository."
	statusUpdateRetryNumber = 5
)

type WebhookRequestChecker struct {
	// The webhook admission review containing the request
	admReview admissionv1.AdmissionReview
	// The resources interceptor object
	remoteSyncer syngit.RemoteSyncer
	// Targets
	remoteTargets []syngit.RemoteTarget
	// The kubernetes client to make request to the api
	k8sClient client.Client
	// The manager where syngit is installed
	managerNamespace string
	// Status and condition mutex
	sync.RWMutex
}

func (wrc *WebhookRequestChecker) ProcessSteps() admissionv1.AdmissionReview {

	wrc.remoteTargets = []syngit.RemoteTarget{}

	// STEP 1 : Get the request details
	rDetails, err := wrc.retrieveRequestDetails()
	if err != nil {
		rDetails.errorDuringProcess = true
		return wrc.responseConstructor(rDetails)
	}

	// STEP 2 : Check if is bypass user (SA of argo, flux, etc..)
	isBypassUser, err := wrc.isBypassSubject(&rDetails)
	if err != nil {
		rDetails.errorDuringProcess = true
		return wrc.responseConstructor(rDetails)
	}
	if isBypassUser {
		return wrc.letPassRequest(&rDetails)
	}

	// STEP 3 : Check the user's rights
	processAllowed, err := wrc.userAllowed(&rDetails)
	rDetails.processPass = processAllowed
	if err != nil {
		rDetails.errorDuringProcess = true
		return wrc.responseConstructor(rDetails)
	}

	// STEP 4 : Convert the request to get the yaml of the object
	if wrc.admReview.Request.Operation != admissionv1.Delete {
		err = wrc.convertToYaml(&rDetails)
		if err != nil {
			rDetails.errorDuringProcess = true
			return wrc.responseConstructor(rDetails)
		}
	} else {
		rDetails.interceptedYAML = ""
	}

	// Step 5 : TLS constructor
	err = wrc.tlsContructor(&rDetails)
	if err != nil {
		rDetails.errorDuringProcess = true
		return wrc.responseConstructor(rDetails)
	}

	// STEP 6 : Git push
	areTheyPushed, err := wrc.gitPush(&rDetails)
	wrc.gitPushPostChecker(areTheyPushed, err, &rDetails)
	if err != nil {
		rDetails.errorDuringProcess = true
		return wrc.responseConstructor(rDetails)
	}

	// STEP 7 : Post checking
	wrc.postcheck(&rDetails)

	return wrc.responseConstructor(rDetails)
}

func (wrc *WebhookRequestChecker) retrieveRequestDetails() (wrcDetails, error) {
	details := &wrcDetails{
		processPass: false,
		webhookPass: false,
		requestUID:  wrc.admReview.DeepCopy().Request.UID,
	}

	if wrc.admReview.Request == nil {
		const errMsg = "the request is empty and it should not be"
		details.messageAddition = errMsg
		return *details, errors.New(errMsg)
	}

	interceptedGVR := (*schema.GroupVersionResource)(wrc.admReview.Request.RequestResource.DeepCopy())
	details.interceptedGVR = *interceptedGVR

	details.interceptedName = wrc.admReview.Request.Name

	wrc.updateStatusState("LastObservedObjectState", *details)

	details.targetsPushInformation = []pushInformation{}

	return *details, nil
}

func (wrc *WebhookRequestChecker) getRemoteUserBinding(username string, fqdn string, details *wrcDetails) (*syngit.RemoteUserBinding, *gitUser, error) {
	ctx := context.Background()
	gitUser := &gitUser{
		gitUser:  "",
		gitEmail: "",
		gitToken: "",
	}

	var remoteUserBindings = &syngit.RemoteUserBindingList{}
	listOps := &client.ListOptions{
		Namespace: wrc.remoteSyncer.Namespace,
	}
	if wrc.remoteSyncer.Spec.RemoteUserBindingSelector != nil {
		labelSelector, labelErr := v1.LabelSelectorAsSelector(wrc.remoteSyncer.Spec.RemoteUserBindingSelector)
		if labelErr != nil {
			errMsg := "error parsing the LabelSelector for the remoteUserBindingSelector: " + labelErr.Error()
			details.messageAddition = errMsg
			return nil, nil, errors.New(errMsg)
		}
		listOps.LabelSelector = labelSelector
	}
	err := wrc.k8sClient.List(ctx, remoteUserBindings, listOps)

	if err != nil {
		errMsg := err.Error()
		details.messageAddition = errMsg
		return nil, nil, errors.New(errMsg)
	}

	var rub = syngit.RemoteUserBinding{}
	userCountLoop := 0 // Prevent non-unique name attack
	for _, remoteUserBinding := range remoteUserBindings.Items {

		// The subject name can not be unique -> in specific conditions, a commit can be done as another user
		// Need to be studied
		if remoteUserBinding.Spec.Subject.Name == username {

			gitUser, err = wrc.searchForGitTokenFromRemoteUserBinding(remoteUserBinding, fqdn)
			if err != nil {
				errMsg := err.Error()
				details.messageAddition = errMsg
				return nil, nil, err
			}
			userCountLoop++

			rub = remoteUserBinding

		}
	}

	if userCountLoop > 1 {
		const errMsg = "multiple RemoteUserBinding found OR the name of the user is not unique; this version of the operator work with the name as unique identifier for users"
		details.messageAddition = errMsg
		return nil, nil, errors.New(errMsg)
	}

	if userCountLoop == 0 {
		return nil, gitUser, nil
	}

	remoteUserBinding := &syngit.RemoteUserBinding{}
	getErr := wrc.k8sClient.Get(ctx, types.NamespacedName{Name: rub.Name, Namespace: rub.Namespace}, remoteUserBinding)
	if getErr != nil {
		details.messageAddition = getErr.Error()
		return nil, nil, getErr
	}

	return remoteUserBinding, gitUser, nil
}

func (wrc *WebhookRequestChecker) userAllowed(details *wrcDetails) (bool, error) {
	// Check if the user can push (and so, create the resource)
	incomingUser := wrc.admReview.Request.UserInfo
	u, err := url.Parse(wrc.remoteSyncer.Spec.RemoteRepository)
	if err != nil {
		errMsg := "error parsing git repository URL: " + err.Error()
		details.messageAddition = errMsg
		return false, errors.New(errMsg)
	}

	fqdn := u.Host
	details.serverHost = fqdn
	ctx := context.Background()

	var gitUser *gitUser
	remoteUserBinding, gitUser, rubErr := wrc.getRemoteUserBinding(incomingUser.Username, fqdn, details)
	if rubErr != nil {
		return false, rubErr
	}

	if remoteUserBinding != nil {

		// Check for annotation
		if wrc.remoteSyncer.Annotations[syngit.RtAnnotationUserSpecificKey] != "" {
			// Create the user specific remote target -> will create with the one-user-one-branch pattern by default.
			// The external providers need to overwrite the target-repo & target-branch if the pattern is set to one-user-one-fork.
			remoteTargetPattern := &patterns.UserSpecificPattern{
				PatternSpecification: patterns.PatternSpecification{
					Client:         wrc.k8sClient,
					NamespacedName: types.NamespacedName{Name: wrc.remoteSyncer.Name, Namespace: wrc.remoteSyncer.Namespace},
				},
				Username:     incomingUser.Username,
				RemoteSyncer: wrc.remoteSyncer,
			}
			err := patterns.Trigger(remoteTargetPattern, ctx)
			if err != nil {
				return false, err
			}

			// The user specific pattern add a new association to the RemoteUserBinding.
			// Therefore, we must either get again the new RemoteUserBinding OR
			// add the user specific RemoteTarget to the current object.
			userSpecificRemoteTarget := remoteTargetPattern.GetRemoteTarget()
			remoteUserBinding.Spec.RemoteTargetRefs = append(remoteUserBinding.Spec.RemoteTargetRefs, corev1.ObjectReference{
				Name: userSpecificRemoteTarget.Name,
			})
		}

		remoteTargetRefNames := []string{}
		for _, remoteTargetRef := range remoteUserBinding.Spec.RemoteTargetRefs {
			remoteTargetRefNames = append(remoteTargetRefNames, remoteTargetRef.Name)
		}

		// Search for RemoteTargets
		var remoteTargets = &syngit.RemoteTargetList{}
		listOps := &client.ListOptions{
			Namespace: wrc.remoteSyncer.Namespace,
		}
		if wrc.remoteSyncer.Spec.RemoteTargetSelector != nil {
			labelSelector, labelErr := v1.LabelSelectorAsSelector(wrc.remoteSyncer.Spec.RemoteTargetSelector)
			if labelErr != nil {
				errMsg := "error parsing the LabelSelector for the remoteTargetSelector: " + labelErr.Error()
				details.messageAddition = errMsg
				return false, errors.New(errMsg)
			}
			listOps.LabelSelector = labelSelector
		}
		listErr := wrc.k8sClient.List(ctx, remoteTargets, listOps)

		if listErr != nil {
			details.messageAddition = listErr.Error()
			return false, listErr
		}
		for _, remoteTarget := range remoteTargets.Items {
			if slices.Contains(remoteTargetRefNames, remoteTarget.Name) {
				if remoteTarget.Spec.UpstreamRepository == wrc.remoteSyncer.Spec.RemoteRepository && remoteTarget.Spec.UpstreamBranch == wrc.remoteSyncer.Spec.DefaultBranch {
					wrc.remoteTargets = append(wrc.remoteTargets, remoteTarget)
				}
			}
		}

		if wrc.remoteSyncer.Spec.TargetStrategy == syngit.OneTarget && len(wrc.remoteTargets) > 1 {
			errMsg := "multiple RemoteTargets found for OneTarget set as the TargetStrategy in the RemoteSyncer"
			details.messageAddition = errMsg
			return false, errors.New(errMsg)
		}

		if len(wrc.remoteTargets) == 0 {
			errMsg := "no RemoteTarget found"
			details.messageAddition = errMsg
			return false, errors.New(errMsg)
		}
	}

	if remoteUserBinding == nil {

		// Check if there is a default user that we can use
		if wrc.remoteSyncer.Spec.DefaultUnauthorizedUserMode != syngit.UseDefaultUser || wrc.remoteSyncer.Spec.DefaultRemoteUserRef == nil || wrc.remoteSyncer.Spec.DefaultRemoteUserRef.Name == "" {
			errMsg := "no RemoteUserBinding found for the user " + incomingUser.Username
			details.messageAddition = errMsg
			return false, errors.New(errMsg)
		}

		// Search for the default RemoteUser object
		userNamespacedName := &types.NamespacedName{
			Namespace: wrc.remoteSyncer.Namespace,
			Name:      wrc.remoteSyncer.Spec.DefaultRemoteUserRef.Name,
		}
		remoteUser := &syngit.RemoteUser{}
		err := wrc.k8sClient.Get(ctx, *userNamespacedName, remoteUser)
		if err != nil {
			errMsg := "the default RemoteUser is not found : " + wrc.remoteSyncer.Spec.DefaultRemoteUserRef.Name
			details.messageAddition = errMsg
			return false, err
		}

		if remoteUser.Spec.GitBaseDomainFQDN != fqdn {
			errMsg := "the fqdn of the default RemoteUser does not match the associated RemoteSyncer (" + wrc.remoteSyncer.Name + ") fqdn (" + remoteUser.Spec.GitBaseDomainFQDN + ")"
			details.messageAddition = errMsg
			return false, err
		}
		gitUser, err = wrc.searchForGitToken(*remoteUser)
		if err != nil {
			errMsg := err.Error()
			details.messageAddition = errMsg
			return false, err
		}

		// Search for the default RemoteTarget
		targetNamespacedName := &types.NamespacedName{
			Namespace: wrc.remoteSyncer.Namespace,
			Name:      wrc.remoteSyncer.Spec.DefaultRemoteTargetRef.Name,
		}
		remoteTarget := &syngit.RemoteTarget{}
		err = wrc.k8sClient.Get(ctx, *targetNamespacedName, remoteTarget)
		if err != nil {
			errMsg := "the default RemoteTarget is not found : " + wrc.remoteSyncer.Spec.DefaultRemoteTargetRef.Name
			details.messageAddition = errMsg
			return false, err
		}

		if remoteTarget.Spec.UpstreamRepository != wrc.remoteSyncer.Spec.RemoteRepository || remoteTarget.Spec.UpstreamBranch != wrc.remoteSyncer.Spec.DefaultBranch {
			errMsg := fmt.Sprintf("the RemoteSyncer's repository or branch does not match the upstream repository or branch of the default RemoteTarget. RemoteSyncer repo: %s; RemoteSyncer branch: %s; RemoteTarget upstream repo: %s; RemoteTarget upstream branch: %s", wrc.remoteSyncer.Spec.RemoteRepository, wrc.remoteSyncer.Spec.DefaultBranch, remoteTarget.Spec.UpstreamRepository, remoteTarget.Spec.UpstreamBranch)
			details.messageAddition = errMsg
			return false, err
		}

		wrc.remoteTargets = []syngit.RemoteTarget{*remoteTarget}
	}

	details.gitUser = *gitUser

	return true, nil
}

func (wrc *WebhookRequestChecker) updateRemoteUserBinding(ctx context.Context, remoteUserBinding syngit.RemoteUserBinding, retryNumber int) error {
	var rub syngit.RemoteUserBinding
	if err := wrc.k8sClient.Get(ctx, types.NamespacedName{Name: remoteUserBinding.Name, Namespace: remoteUserBinding.Namespace}, &rub); err != nil {
		return err
	}

	rub.Spec.RemoteTargetRefs = remoteUserBinding.Spec.RemoteTargetRefs
	if err := wrc.k8sClient.Update(ctx, &rub); err != nil {
		if retryNumber > 0 {
			time.Sleep(2 * time.Second)
			return wrc.updateRemoteUserBinding(ctx, remoteUserBinding, retryNumber-1)
		}
		return err
	}
	return nil
}

func (wrc *WebhookRequestChecker) searchForGitToken(remoteUser syngit.RemoteUser) (*gitUser, error) {
	userGitName := ""
	userGitEmail := ""
	userGitToken := ""
	namespace := wrc.remoteSyncer.Namespace

	secretCount := 0
	ctx := context.Background()

	secretNamespacedName := &types.NamespacedName{
		Namespace: namespace,
		Name:      remoteUser.Spec.SecretRef.Name,
	}
	secret := &corev1.Secret{}
	err := wrc.k8sClient.Get(ctx, *secretNamespacedName, secret)
	if err == nil {
		userGitName = string(secret.Data["username"])
		userGitToken = string(secret.Data["password"])
		secretCount++

		userGitEmail = remoteUser.Spec.Email
	}

	gitUser := &gitUser{
		gitUser:  userGitName,
		gitEmail: userGitEmail,
		gitToken: userGitToken,
	}

	if secretCount == 0 {
		return gitUser, errors.New("no Secret found for the current user to log on the git repository with the RemoteUser : " + remoteUser.Name)
	}
	if userGitToken == "" {
		return gitUser, errors.New("no token found in the secret; the token must be specified in the password field and the secret type must be kubernetes.io/basic-auth")
	}

	return gitUser, nil
}

func (wrc *WebhookRequestChecker) searchForGitTokenFromRemoteUserBinding(rub syngit.RemoteUserBinding, fqdn string) (*gitUser, error) {
	remoteUserCount := 0
	ctx := context.Background()

	var gitUser *gitUser

	namespace := wrc.remoteSyncer.Namespace
	for _, ref := range rub.Spec.RemoteUserRefs {
		namespacedName := &types.NamespacedName{
			Namespace: namespace,
			Name:      ref.Name,
		}
		remoteUser := &syngit.RemoteUser{}
		err := wrc.k8sClient.Get(ctx, *namespacedName, remoteUser)
		if err != nil {
			continue
		}

		if remoteUser.Spec.GitBaseDomainFQDN == fqdn {
			remoteUserCount++
			gitUser, err = wrc.searchForGitToken(*remoteUser)
			if err != nil {
				return gitUser, err
			}
		}
	}

	if remoteUserCount == 0 {
		return gitUser, errors.New("no RemoteUser found for the current user with this fqdn : " + fqdn)
	}
	if remoteUserCount > 1 {
		return gitUser, errors.New("more than one RemoteUser found for the current user with this fqdn : " + fqdn)
	}
	if remoteUserCount > 1 {
		return gitUser, errors.New("more than one Secret found for the current user to log on the git repository with this fqdn : " + fqdn)
	}

	return gitUser, nil
}

func (wrc *WebhookRequestChecker) isBypassSubject(details *wrcDetails) (bool, error) {
	incomingUser := wrc.admReview.Request.UserInfo
	isBypassSubject := false

	userCountLoop := 0 // Prevent non-unique name attack
	for _, subject := range wrc.remoteSyncer.Spec.BypassInterceptionSubjects {
		// The subject name can not be unique -> in specific conditions, a commit can be done as another user
		// Need to be studied
		if subject.Name == incomingUser.Username {
			isBypassSubject = true
			details.messageAddition = "this user bypass the process"
			userCountLoop++
		}
	}

	if userCountLoop > 1 {
		const errMsg = "the name of the user is not unique; this version of the operator work with the name as unique identifier for users"
		details.messageAddition = errMsg
		return isBypassSubject, errors.New(errMsg)
	}

	return isBypassSubject, nil
}

func (wrc *WebhookRequestChecker) letPassRequest(details *wrcDetails) admissionv1.AdmissionReview {
	details.webhookPass = true

	wrc.updateStatusState("LastBypassedObjectState", *details)

	return wrc.responseConstructor(*details)
}

func getPathsFromConfigMap(ctx context.Context, client client.Client, configMapNN types.NamespacedName) ([]string, error) {
	excludedFieldsConfig := &corev1.ConfigMap{}
	err := client.Get(ctx, configMapNN, excludedFieldsConfig)
	if err != nil {
		return nil, err
	}
	yamlString := excludedFieldsConfig.Data["excludedFields"]
	var excludedFields []string

	// Unmarshal the YAML string into the Go array
	err = yaml.Unmarshal([]byte(yamlString), &excludedFields)
	if err != nil {
		errMsg := "failed to convert the excludedFields from the ConfigMap (wrong yaml format)"
		return nil, errors.New(errMsg)
	}

	return excludedFields, nil
}

func (wrc *WebhookRequestChecker) convertToYaml(details *wrcDetails) error {
	// Convert the json string object to a yaml string
	// We have no other choice than extracting the json into a map
	//  and then convert the map into a yaml string
	// Because the 'map' object is, by definition, not ordered
	//  we cannot reorder fields
	ctx := context.Background()

	var data map[string]interface{}
	err := json.Unmarshal(wrc.admReview.Request.Object.Raw, &data)
	if err != nil {
		errMsg := err.Error()
		details.messageAddition = errMsg
		return errors.New(errMsg)
	}

	// Excluded fields paths to remove
	paths := []string{}

	// Search for cluster default excluded fields
	defaultExcludedFieldsCms := corev1.ConfigMapList{}
	listOps := &client.ListOptions{
		Namespace: wrc.managerNamespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"syngit.io/cluster-default-excluded-fields": "true",
		}),
	}
	err = wrc.k8sClient.List(ctx, &defaultExcludedFieldsCms, listOps)
	if err != nil {
		errMsg := err.Error()
		details.messageAddition = errMsg
		return errors.New(errMsg)
	}
	for _, defaultExcludedFieldsCm := range defaultExcludedFieldsCms.Items {
		configMapNN := types.NamespacedName{
			Name:      defaultExcludedFieldsCm.Name,
			Namespace: wrc.managerNamespace,
		}
		excludedFieldsFromCm, exfiErr := getPathsFromConfigMap(ctx, wrc.k8sClient, configMapNN)
		if exfiErr != nil {
			details.messageAddition = exfiErr.Error()
			return exfiErr
		}
		paths = append(paths, excludedFieldsFromCm...)
	}

	// excludedFields hardcoded in RemoteSyncer
	excludedFieldsFromRsy := wrc.remoteSyncer.Spec.ExcludedFields
	paths = append(paths, excludedFieldsFromRsy...)

	// Check if the excludedFields ConfigMap exists
	if wrc.remoteSyncer.Spec.ExcludedFieldsConfigMapRef != nil && wrc.remoteSyncer.Spec.ExcludedFieldsConfigMapRef.Name != "" {
		configMapNN := types.NamespacedName{
			Name:      wrc.remoteSyncer.Spec.ExcludedFieldsConfigMapRef.Name,
			Namespace: wrc.remoteSyncer.Namespace,
		}
		excludedFieldsFromCm, exfiErr := getPathsFromConfigMap(ctx, wrc.k8sClient, configMapNN)
		if exfiErr != nil {
			details.messageAddition = exfiErr.Error()
			return exfiErr
		}
		paths = append(paths, excludedFieldsFromCm...)
	}

	// Remove unwanted fields
	for _, path := range paths {
		utils.ExcludedFieldsFromJson(data, path)
	}

	// Marshal back to YAML
	updatedYAML, err := yaml.Marshal(data)
	if err != nil {
		errMsg := err.Error()
		details.messageAddition = errMsg
		return errors.New(errMsg)
	}

	details.interceptedYAML = string(updatedYAML)

	return nil
}

func (wrc *WebhookRequestChecker) tlsContructor(details *wrcDetails) error {
	// Step 1: Search for the global CA Bundle of the server located in the syngit namespace
	caBundle, caErr := utils.FindGlobalCABundle(wrc.k8sClient, strings.Split(details.serverHost, ":")[0])
	if caErr != nil && strings.Contains(caErr.Error(), utils.CaSecretWrongTypeErrorMessage) {
		details.messageAddition = caErr.Error()
		return caErr
	}

	// Step 2: Search for a specific CA Bundle located in the current namespace
	caBundleSecretRef := wrc.remoteSyncer.Spec.CABundleSecretRef
	ns := caBundleSecretRef.Namespace
	if ns == "" {
		ns = wrc.remoteSyncer.Namespace
	}
	caBundleRsy, caErr := utils.FindCABundle(wrc.k8sClient, ns, caBundleSecretRef.Name)
	if caErr != nil {
		details.messageAddition = caErr.Error()
		return caErr
	}
	if caBundleRsy != nil {
		caBundle = caBundleRsy
	}

	details.caBundle = caBundle

	return nil
}

func (wrc *WebhookRequestChecker) gitPush(details *wrcDetails) (bool, error) {

	for _, remoteTarget := range wrc.remoteTargets {
		gitPusher := &GitPusher{
			remoteSyncer:    *wrc.remoteSyncer.DeepCopy(),
			remoteTarget:    *remoteTarget.DeepCopy(),
			interceptedYAML: details.interceptedYAML,
			interceptedGVR:  details.interceptedGVR,
			interceptedName: details.interceptedName,
			gitUser:         details.gitUser.gitUser,
			gitEmail:        details.gitUser.gitEmail,
			gitToken:        details.gitUser.gitToken,
			operation:       wrc.admReview.Request.Operation,
			caBundle:        details.caBundle,
		}
		res, err := gitPusher.Push()
		if err != nil {
			errMsg := err.Error()
			details.messageAddition = errMsg
			return false, errors.New(errMsg)
		}

		if res.commitHash == "" {
			return false, nil
		}

		details.targetsPushInformation = append(details.targetsPushInformation, pushInformation{
			repoPath:   res.path,
			commitHash: res.commitHash,
			repoUrl:    res.url,
		})
	}

	return true, nil
}

func (wrc *WebhookRequestChecker) gitPushPostChecker(areTheyPushed bool, err error, details *wrcDetails) {
	details.processPass = areTheyPushed
	if areTheyPushed {
		details.pushDetails = "Resource successfully pushed"
	} else {
		details.pushDetails = err.Error()
	}

	wrc.updateStatusState("LastPushedObjectState", *details)
	condition := &v1.Condition{
		LastTransitionTime: v1.Now(),
		Type:               "Synced",
		Reason:             "Pushed",
		Status:             "True",
		Message:            details.pushDetails,
	}
	wrc.updateConditions(*condition)
}

func (wrc *WebhookRequestChecker) postcheck(details *wrcDetails) bool {
	// Check the Commit Process mode
	if wrc.remoteSyncer.Spec.Strategy == syngit.CommitOnly {
		details.webhookPass = false
	}
	if wrc.remoteSyncer.Spec.Strategy == syngit.CommitApply {
		details.webhookPass = true
	}

	// If for some reasons, the process is not going right
	if !details.processPass {
		details.webhookPass = false
		details.messageAddition = "The process has unexpectly interupted. The resource has not been applied to the cluster."
		return false
	}

	return true
}

func (wrc *WebhookRequestChecker) responseConstructor(details wrcDetails) admissionv1.AdmissionReview {

	successMessage := defaultSuccessMessage
	if wrc.remoteSyncer.Spec.DefaultBlockAppliedMessage != "" {
		successMessage = wrc.remoteSyncer.Spec.DefaultBlockAppliedMessage
	}

	// Set the status and the message depending of the status of the webhook
	status := "Failure"
	message := defaultFailureMessage
	if !details.errorDuringProcess {
		status = "Success"
		message = successMessage
	} else {
		condition := &v1.Condition{
			LastTransitionTime: v1.Now(),
			Type:               "Synced",
			Reason:             "WebhookHandlerError",
			Status:             "False",
			Message:            details.messageAddition,
		}
		wrc.updateConditions(*condition)
	}

	// Set the final message
	if details.messageAddition != "" {
		message += " " + details.messageAddition
	}

	// Construct the admisson review request
	admissionReviewResp := admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{
			UID:     details.requestUID,
			Allowed: details.webhookPass,
			Result: &v1.Status{
				Status:  status,
				Message: message,
			},
		},
	}
	admissionReviewResp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "admission.k8s.io",
		Version: "v1",
		Kind:    "AdmissionReview",
	})

	return admissionReviewResp
}

func (wrc *WebhookRequestChecker) updateConditions(condition v1.Condition) {
	wrc.Lock()
	defer wrc.Unlock()

	conditions := utils.TypeBasedConditionUpdater(wrc.remoteSyncer.Status.DeepCopy().Conditions, condition)
	wrc.remoteSyncer.Status.Conditions = conditions

	ctx := context.Background()
	wrc.updateStatus(ctx, statusUpdateRetryNumber)
}

func (wrc *WebhookRequestChecker) updateStatus(ctx context.Context, retryNumber int) {
	_ = log.FromContext(ctx)

	namespacedName := types.NamespacedName{
		Namespace: wrc.remoteSyncer.Namespace,
		Name:      wrc.remoteSyncer.Name,
	}
	var remoteSyncer syngit.RemoteSyncer
	if err := wrc.k8sClient.Get(ctx, namespacedName, &remoteSyncer); err != nil {
		log.Log.Error(err, "can't get the remote syncer "+wrc.remoteSyncer.Namespace+"/"+wrc.remoteSyncer.Name)
	}

	remoteSyncer.Status = *wrc.remoteSyncer.Status.DeepCopy()

	err := wrc.k8sClient.Status().Update(ctx, &remoteSyncer)
	if err != nil {
		if retryNumber > 0 {
			wrc.updateStatus(ctx, retryNumber-1)
		} else {
			log.Log.Error(err, "can't update the conditions of the remote syncer "+wrc.remoteSyncer.Namespace+"/"+wrc.remoteSyncer.Name)
		}
	}

}

func (wrc *WebhookRequestChecker) updateStatusState(kind string, details wrcDetails) {
	wrc.Lock()
	defer wrc.Unlock()

	gvrn := &syngit.JsonGVRN{
		Group:    details.interceptedGVR.Group,
		Version:  details.interceptedGVR.Version,
		Resource: details.interceptedGVR.Resource,
		Name:     details.interceptedName,
	}

	repos := []string{}
	for _, info := range details.targetsPushInformation {
		repos = append(repos, info.repoUrl)
	}
	commitHashes := []string{}
	for _, info := range details.targetsPushInformation {
		commitHashes = append(commitHashes, info.commitHash)
	}

	repoPath := ""
	if len(details.targetsPushInformation) > 0 {
		repoPath = details.targetsPushInformation[0].repoPath
	}

	switch kind {
	case "LastBypassedObjectState":
		lastBypassedObjectState := &syngit.LastBypassedObjectState{
			LastBypassedObjectTime:     v1.Now(),
			LastBypassedObjectUserInfo: wrc.admReview.Request.UserInfo,
			LastBypassedObject:         *gvrn,
		}
		wrc.remoteSyncer.Status.LastBypassedObjectState = *lastBypassedObjectState
	case "LastObservedObjectState":
		lastObservedObjectState := &syngit.LastObservedObjectState{
			LastObservedObjectTime:     v1.Now(),
			LastObservedObjectUsername: wrc.admReview.Request.UserInfo.Username,
			LastObservedObject:         *gvrn,
		}
		wrc.remoteSyncer.Status.LastObservedObjectState = *lastObservedObjectState
	case "LastPushedObjectState":
		lastPushedObjectState := &syngit.LastPushedObjectState{
			LastPushedObjectTime:            v1.Now(),
			LastPushedObject:                *gvrn,
			LastPushedObjectGitPath:         repoPath,
			LastPushedObjectGitRepos:        repos,
			LastPushedObjectGitCommitHashes: commitHashes,
			LastPushedGitUser:               details.gitUser.gitUser,
			LastPushedObjectStatus:          details.pushDetails,
		}
		wrc.remoteSyncer.Status.LastPushedObjectState = *lastPushedObjectState
	}

	ctx := context.Background()
	wrc.updateStatus(ctx, statusUpdateRetryNumber)

}
