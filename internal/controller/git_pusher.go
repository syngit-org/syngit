package controller

import (
	"fmt"

	kgiov1 "dams.kgio/kgio/api/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type GitPusher struct {
	resourcesInterceptor kgiov1.ResourcesInterceptor
	interceptedYAML      string
	interceptedGVR       schema.GroupVersionResource
	interceptedName      string
}

type GitPushResponse struct {
	path       string // The git path were the resource has been pushed
	commitHash string // The commit hash of the commit
}

func (gp *GitPusher) Push() (GitPushResponse, error) {
	gpResponse := &GitPushResponse{path: "", commitHash: ""}

	gvr := gp.interceptedGVR
	gvrn := &kgiov1.GroupVersionResourceName{
		GroupVersionResource: &gvr,
	}

	tempPath := kgiov1.GetPathFromGVRN(gp.resourcesInterceptor.Spec.IncludedResources, *gvrn.DeepCopy())
	fmt.Println(tempPath)

	return *gpResponse, nil
}
