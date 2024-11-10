package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"sync"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	syngit "syngit.io/syngit/api/v1beta1"
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
	repoUrl               string
	repoPath              string
	repoHost              string
	commitHash            string
	gitUser               gitUser
	insecureSkipTlsVerify bool
	caBundle              string
	pushDetails           string

	// Error
	errorDuringProcess bool
}

const (
	defaultFailureMessage = "The changes have not been pushed to the remote git repository:"
	defaultSuccessMessage = "The changes were correctly been pushed on the remote git repository."
)

type WebhookRequestChecker struct {
	// The webhook admission review containing the request
	admReview admissionv1.AdmissionReview
	// The resources interceptor object
	remoteSyncer syngit.RemoteSyncer
	// The kubernetes client to make request to the api
	k8sClient client.Client
	// Logger
	log *logr.Logger
	// Status and condition mutex
	sync.RWMutex
}

func (wrc *WebhookRequestChecker) ProcessSteps() admissionv1.AdmissionReview {

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
	isPushed, err := wrc.gitPush(&rDetails)
	wrc.gitPushPostChecker(isPushed, err, &rDetails)
	if err != nil {
		rDetails.errorDuringProcess = true
		return wrc.responseConstructor(rDetails)
	}

	// STEP 5 : Post checking
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

	wrc.updateStatus("LastObservedObjectState", *details)

	return *details, nil
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
	details.repoHost = fqdn
	ctx := context.Background()
	gitUser := &gitUser{
		gitUser:  "",
		gitEmail: "",
		gitToken: "",
	}

	var remoteUserBindings = &syngit.RemoteUserBindingList{}
	err = wrc.k8sClient.List(ctx, remoteUserBindings, client.InNamespace(wrc.remoteSyncer.Namespace))
	if err != nil {
		errMsg := err.Error()
		details.messageAddition = errMsg
		return false, errors.New(errMsg)
	}

	userCountLoop := 0 // Prevent non-unique name attack
	for _, remoteUserBinding := range remoteUserBindings.Items {

		// The subject name can not be unique -> in specific conditions, a commit can be done as another user
		// Need to be studied
		if remoteUserBinding.Spec.Subject.Name == incomingUser.Username {
			gitUser, err = wrc.searchForGitTokenFromRemoteUserBinding(remoteUserBinding, fqdn)
			if err != nil {
				errMsg := err.Error()
				details.messageAddition = errMsg
				return false, err
			}
			userCountLoop++
		}
	}

	if userCountLoop == 0 {

		// Check if there is a default user that we can use
		if wrc.remoteSyncer.Spec.DefaultUnauthorizedUserMode != syngit.UseDefaultUser || wrc.remoteSyncer.Spec.DefaultRemoteUserRef == nil || wrc.remoteSyncer.Spec.DefaultRemoteUserRef.Name == "" {
			errMsg := "no RemoteUserBinding found for the user " + incomingUser.Username
			details.messageAddition = errMsg
			return false, errors.New(errMsg)
		}

		// Search for the default RemoteUser object
		namespacedName := &types.NamespacedName{
			Namespace: wrc.remoteSyncer.Namespace,
			Name:      wrc.remoteSyncer.Spec.DefaultRemoteUserRef.Name,
		}
		remoteUser := &syngit.RemoteUser{}
		err := wrc.k8sClient.Get(ctx, *namespacedName, remoteUser)
		if err != nil {
			errMsg := "the default user is not found : " + wrc.remoteSyncer.Spec.DefaultRemoteUserRef.Name
			details.messageAddition = errMsg
			return false, err
		}

		if remoteUser.Spec.GitBaseDomainFQDN != fqdn {
			errMsg := "the fqdn of the default user does not match the associated RemoteSyncer (" + wrc.remoteSyncer.Name + ") fqdn (" + remoteUser.Spec.GitBaseDomainFQDN + ")"
			details.messageAddition = errMsg
			return false, err
		}
		gitUser, err = wrc.searchForGitToken(*remoteUser)
		if err != nil {
			errMsg := err.Error()
			details.messageAddition = errMsg
			return false, err
		}
	}

	if userCountLoop > 1 {
		const errMsg = "multiple RemoteUserBinding found OR the name of the user is not unique; this version of the operator work with the name as unique identifier for users"
		details.messageAddition = errMsg
		return false, errors.New(errMsg)
	}

	details.gitUser = *gitUser

	return true, nil
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
	for _, ref := range rub.Spec.RemoteRefs {
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

	wrc.updateStatus("LastBypassedObjectState", *details)

	return wrc.responseConstructor(*details)
}

func (wrc *WebhookRequestChecker) convertToYaml(details *wrcDetails) error {
	// Convert the json string object to a yaml string
	// We have no other choice than extracting the json into a map
	//  and then convert the map into a yaml string
	// Because the 'map' object is, by definition, not ordered
	//  we cannot reorder fields

	var data map[string]interface{}
	err := json.Unmarshal(wrc.admReview.Request.Object.Raw, &data)
	if err != nil {
		errMsg := err.Error()
		details.messageAddition = errMsg
		return errors.New(errMsg)
	}

	// Paths to remove
	paths := wrc.remoteSyncer.Spec.ExcludedFields

	// Check if the excludedFields ConfigMap exists
	if wrc.remoteSyncer.Spec.ExcludedFieldsConfigMapRef != nil && wrc.remoteSyncer.Spec.ExcludedFieldsConfigMapRef.Name != "" {
		ctx := context.Background()
		secretNamespacedName := &types.NamespacedName{
			Namespace: wrc.remoteSyncer.Namespace,
			Name:      wrc.remoteSyncer.Spec.ExcludedFieldsConfigMapRef.Name,
		}
		excludedFieldsConfig := &corev1.ConfigMap{}
		err := wrc.k8sClient.Get(ctx, *secretNamespacedName, excludedFieldsConfig)
		if err != nil {
			errMsg := err.Error()
			details.messageAddition = errMsg
			return errors.New(errMsg)
		}
		yamlString := excludedFieldsConfig.Data["excludedFields"]
		var excludedFields []string

		// Unmarshal the YAML string into the Go array
		err = yaml.Unmarshal([]byte(yamlString), &excludedFields)
		if err != nil {
			errMsg := "failed to convert the excludedFields from the ConfigMap (wrong yaml format)"
			details.messageAddition = errMsg
			return errors.New(errMsg)
		}

		paths = append(paths, excludedFields...)
	}

	// Remove unwanted fields
	for _, path := range paths {
		ExcludedFieldsFromJson(data, path)
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
	insecureSkipTlsVerify := false
	caBundle := ""

	ctx := context.Background()
	errMsg := "the CA bundle secret must be of type \"kubernetes.io/ts\""

	// Step 1: Search for the global CA Bundle of the server located in the syngit namespace
	globalNamespacedName := types.NamespacedName{Namespace: os.Getenv("MANAGER_NAMESPACE"), Name: details.repoHost + "-ca-bundle"}
	caBundleSecret := &corev1.Secret{}
	err := wrc.k8sClient.Get(ctx, globalNamespacedName, caBundleSecret)
	if err == nil {
		if caBundleSecret.Type != "kubernetes.io/tls" {
			details.messageAddition = errMsg
			return errors.New(errMsg)
		}
		caBundle = caBundleSecret.StringData["tls.crt"]
	}

	// Step 2: Search for a specific CA Bundle located in the current namespace
	caBundleSecretRef := wrc.remoteSyncer.Spec.CABundleSecretRef
	namespacedName := types.NamespacedName{Namespace: caBundleSecretRef.Namespace, Name: caBundleSecretRef.Name}
	err = wrc.k8sClient.Get(ctx, namespacedName, caBundleSecret)
	if err == nil {
		if caBundleSecret.Type != "kubernetes.io/tls" {
			details.messageAddition = errMsg
			return errors.New(errMsg)
		}
		caBundle = caBundleSecret.StringData["tls.crt"]
	}

	details.insecureSkipTlsVerify = insecureSkipTlsVerify
	details.caBundle = caBundle

	return nil
}

func (wrc *WebhookRequestChecker) gitPush(details *wrcDetails) (bool, error) {
	gitPusher := &GitPusher{
		remoteSyncer:          *wrc.remoteSyncer.DeepCopy(),
		interceptedYAML:       details.interceptedYAML,
		interceptedGVR:        details.interceptedGVR,
		interceptedName:       details.interceptedName,
		gitUser:               details.gitUser.gitUser,
		gitEmail:              details.gitUser.gitEmail,
		gitToken:              details.gitUser.gitToken,
		operation:             wrc.admReview.Request.Operation,
		insecureSkipTlsVerify: details.insecureSkipTlsVerify,
		caBundle:              details.caBundle,
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

	details.repoPath = res.path
	details.commitHash = res.commitHash
	details.repoUrl = wrc.remoteSyncer.Spec.RemoteRepository

	return true, nil
}

func (wrc *WebhookRequestChecker) gitPushPostChecker(isPushed bool, err error, details *wrcDetails) {
	details.processPass = isPushed
	if isPushed {
		details.pushDetails = "Resource successfully pushed"
	} else {
		details.pushDetails = err.Error()
	}

	wrc.updateStatus("LastPushedObjectState", *details)
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
	if wrc.remoteSyncer.Spec.ProcessMode == syngit.CommitOnly {
		details.webhookPass = false
	}
	if wrc.remoteSyncer.Spec.ProcessMode == syngit.CommitApply {
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
			Type:               "NotSynced",
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

	// Annotation that will be stored in the outcoming object
	auditAnnotation := make(map[string]string)
	if details.repoUrl != "" {
		auditAnnotation["syngit-git-repo-fqdn"] = details.repoUrl
	}
	if details.repoPath != "" {
		auditAnnotation["syngit-git-repo-path"] = details.repoPath
	}
	if details.commitHash != "" {
		auditAnnotation["syngit-git-commit-hash"] = details.commitHash
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
			AuditAnnotations: auditAnnotation,
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

	added := false
	conditions := make([]v1.Condition, 0)
	for _, cond := range wrc.remoteSyncer.Status.Conditions {
		if cond.Type == condition.Type {
			conditions = append(conditions, condition)
			added = true
		} else {
			conditions = append(conditions, cond)
		}
	}
	if !added {
		conditions = append(conditions, condition)
	}
	wrc.remoteSyncer.Status.Conditions = conditions

	ctx := context.Background()
	if err := wrc.k8sClient.Status().Update(ctx, &wrc.remoteSyncer); err != nil {
		wrc.log.Error(err, "can't update the conditions of the remote syncer "+wrc.remoteSyncer.Namespace+"/"+wrc.remoteSyncer.Name)
	}
}

func (wrc *WebhookRequestChecker) updateStatus(kind string, details wrcDetails) {
	wrc.Lock()
	defer wrc.Unlock()

	gvrn := &syngit.JsonGVRN{
		Group:    details.interceptedGVR.Group,
		Version:  details.interceptedGVR.Version,
		Resource: details.interceptedGVR.Resource,
		Name:     details.interceptedName,
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
			LastObservedObjectUserInfo: wrc.admReview.Request.UserInfo,
			LastObservedObject:         *gvrn,
		}
		wrc.remoteSyncer.Status.LastObservedObjectState = *lastObservedObjectState
	case "LastPushedObjectState":
		lastPushedObjectState := &syngit.LastPushedObjectState{
			LastPushedObjectTime:          v1.Now(),
			LastPushedObject:              *gvrn,
			LastPushedObjectGitPath:       details.repoPath,
			LastPushedObjectGitRepo:       details.repoUrl,
			LastPushedObjectGitCommitHash: details.commitHash,
			LastPushedGitUser:             details.gitUser.gitUser,
			LastPushedObjectStatus:        details.pushDetails,
		}
		wrc.remoteSyncer.Status.LastPushedObjectState = *lastPushedObjectState
	}

	ctx := context.Background()
	if err := wrc.k8sClient.Status().Update(ctx, &wrc.remoteSyncer); err != nil {
		wrc.log.Error(err, "can't update the status of the remote syncer "+wrc.remoteSyncer.Namespace+"/"+wrc.remoteSyncer.Name)
	}

}
