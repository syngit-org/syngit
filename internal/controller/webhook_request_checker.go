package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"

	kgiov1 "dams.kgio/kgio/api/v1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

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
	repoFQDN   string
	repoPath   string
	commitHash string
	gitToken   string
}

const (
	defaultFailureMessage = "The changes have not been pushed to the remote git repository:"
	defaultSuccessMessage = "The changes were correctly been pushed on the remote git repository:"
)

type WebhookRequestChecker struct {
	// The webhook admission review containing the request
	admReview admissionv1.AdmissionReview
	// The resources interceptor object
	resourcesInterceptor kgiov1.ResourcesInterceptor
	// The kubernetes client to make request to the api
	k8sClient client.Client
}

func (wrc *WebhookRequestChecker) ProcessSteps() admissionv1.AdmissionReview {

	// STEP 1 : Get the request details
	rDetails, err := wrc.retrieveRequestDetails()
	if err != nil {
		return wrc.responseConstructor(rDetails)
	}

	// STEP 2 : Check if the request can be proceded
	processAllowed, err := wrc.processAllowed(&rDetails)
	rDetails.processPass = processAllowed
	if err != nil {
		return wrc.responseConstructor(rDetails)
	}

	// STEP 3 : Convert the request to get the yaml of the object
	err = wrc.convertToYaml(&rDetails)
	if err != nil {
		return wrc.responseConstructor(rDetails)
	}

	// STEP 4 : Git push
	isPushed, err := wrc.gitPush(&rDetails)
	rDetails.processPass = isPushed
	if err != nil {
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

	return *details, nil
}

func (wrc *WebhookRequestChecker) processAllowed(details *wrcDetails) (bool, error) {

	// Check if the incoming object is part of the specified names in the included resources
	includedNames := kgiov1.GetNamesFromGVR(kgiov1.NSRPstoNSRs(wrc.resourcesInterceptor.Spec.IncludedResources), details.interceptedGVR)
	if len(includedNames) > 0 && !slices.Contains(includedNames, details.interceptedName) {
		const errMsg = "the name of the resource is not part of the IncludedResources"
		details.messageAddition = errMsg // Keep this ? Does the user really need to know that his resource is not scoped ?
		return false, errors.New(errMsg)
	}

	// Check if the incoming object is part of the specified names in the excluded resources
	excludedNames := kgiov1.GetNamesFromGVR(wrc.resourcesInterceptor.Spec.ExcludedResources, details.interceptedGVR)
	if len(excludedNames) > 0 && slices.Contains(excludedNames, details.interceptedName) {
		const errMsg = "the name of the resource is part of the ExcludedResources"
		details.messageAddition = errMsg // Keep this ? Does the user really need to know that his resource is not scoped ?
		return false, errors.New(errMsg)
	}

	// Check if the user can push (and so, create the resource)
	incomingUser := wrc.admReview.Request.UserInfo
	u, err := url.Parse(wrc.resourcesInterceptor.Spec.RemoteRepository)
	if err != nil {
		errMsg := "error parsing git repository URL: " + err.Error()
		details.messageAddition = errMsg
		return false, errors.New(errMsg)
	}

	fqdn := u.Host
	ctx := context.Background()

	userGitToken := ""
	userCountLoop := 0 // Prevent non-unique name attack
	for _, ref := range wrc.resourcesInterceptor.Spec.AuthorizedUsers {
		namespacedName := &types.NamespacedName{
			Namespace: wrc.resourcesInterceptor.Namespace,
			Name:      ref.Name,
		}
		gitUserBinding := &kgiov1.GitUserBinding{}
		err := wrc.k8sClient.Get(ctx, *namespacedName, gitUserBinding)
		if err != nil {
			continue
		}

		// The subject name can not be unique -> in specific conditions, a commit can be done as another user
		// Need to be studied
		if gitUserBinding.Spec.Subject.Name == incomingUser.Username {
			userGitToken, err = wrc.searchForGitToken(*gitUserBinding, fqdn)
			if err != nil {
				errMsg := err.Error()
				details.messageAddition = errMsg
				return false, err
			}
			userCountLoop++
		}
	}

	if userCountLoop == 0 || userGitToken == "" {
		errMsg := "no GitUserBinding found for the user " + incomingUser.Username
		details.messageAddition = errMsg
		return false, errors.New(errMsg)
	}
	if userCountLoop > 1 {
		const errMsg = "multiple GitUserBinding found OR the name of the user is not unique; this version of the operator work with the name as unique identifier for users"
		details.messageAddition = errMsg
		return false, errors.New(errMsg)
	}

	details.gitToken = userGitToken
	fmt.Println(userGitToken)

	return true, nil
}

func (wrc *WebhookRequestChecker) searchForGitToken(gub kgiov1.GitUserBinding, fqdn string) (string, error) {
	userGitToken := ""
	gitRemoteCount := 0
	secretCount := 0
	ctx := context.Background()

	namespace := wrc.resourcesInterceptor.Namespace
	for _, ref := range gub.Spec.RemoteRefs {
		namespacedName := &types.NamespacedName{
			Namespace: namespace,
			Name:      ref.Name,
		}
		gitRemote := &kgiov1.GitRemote{}
		err := wrc.k8sClient.Get(ctx, *namespacedName, gitRemote)
		if err != nil {
			continue
		}

		gitRemoteCount++
		if gitRemote.Spec.GitBaseDomainFQDN == fqdn {
			secretNamespacedName := &types.NamespacedName{
				Namespace: namespace,
				Name:      gitRemote.Spec.SecretRef.Name,
			}
			secret := &corev1.Secret{}
			err := wrc.k8sClient.Get(ctx, *secretNamespacedName, secret)
			if err != nil {
				continue
			}
			userGitToken = secret.StringData["password"]
			secretCount++
		}
	}

	if gitRemoteCount == 0 {
		return userGitToken, errors.New("no GitRemote found for the current user with this fqdn : " + fqdn)
	}
	if gitRemoteCount > 1 {
		return userGitToken, errors.New("more than one GitRemote found for the current user with this fqdn : " + fqdn)
	}
	if secretCount == 0 {
		return userGitToken, errors.New("no Secret found for the current user to log on the git repository with this fqdn : " + fqdn)
	}
	if gitRemoteCount > 1 {
		return userGitToken, errors.New("more than one Secret found for the current user to log on the git repository with this fqdn : " + fqdn)
	}

	return userGitToken, nil
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
	paths := wrc.resourcesInterceptor.Spec.ExcludedFields

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

func (wrc *WebhookRequestChecker) gitPush(details *wrcDetails) (bool, error) {
	gitPusher := &GitPusher{
		resourcesInterceptor: *wrc.resourcesInterceptor.DeepCopy(),
		interceptedYAML:      details.interceptedYAML,
		interceptedGVR:       details.interceptedGVR,
		interceptedName:      details.interceptedName,
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
	details.repoFQDN = wrc.resourcesInterceptor.Spec.RemoteRepository

	return true, nil
}

func (wrc *WebhookRequestChecker) postcheck(details *wrcDetails) bool {
	// Check the Commit Process mode
	if wrc.resourcesInterceptor.Spec.CommitProcess == kgiov1.CommitOnly {
		details.webhookPass = false
	}
	if wrc.resourcesInterceptor.Spec.CommitProcess == kgiov1.CommitApply {
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

	// Set the status and the message depending of the status of the webhook
	status := "Failure"
	message := defaultFailureMessage
	if details.webhookPass {
		status = "Success"
		message = defaultSuccessMessage
	}

	// Set the final message
	if details.messageAddition != "" {
		message += " " + details.messageAddition
	}

	// Annotation that will be stored in the outcoming object
	auditAnnotation := make(map[string]string)
	if details.repoFQDN != "" {
		auditAnnotation["kgio-git-repo-fqdn"] = details.repoFQDN
	}
	if details.repoPath != "" {
		auditAnnotation["kgio-git-repo-path"] = details.repoPath
	}
	if details.commitHash != "" {
		auditAnnotation["kgio-git-commit-hash"] = details.commitHash
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
