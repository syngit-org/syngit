package controller

import (
	kgiov1 "dams.kgio/kgio/api/v1"
)

type GitPusher struct {
	resourcesInterceptor kgiov1.ResourcesInterceptor
	interceptedYAML      string
}

type GitPushResponse struct {
	path       string // The git path were the resource has been pushed
	commitHash string // The commit hash of the commit
}

func (gp *GitPusher) push() (GitPushResponse, error) {
	gpResponse := &GitPushResponse{path: "", commitHash: ""}

	return *gpResponse, nil
}
