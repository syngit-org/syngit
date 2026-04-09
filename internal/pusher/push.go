package pusher

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
)

func Push(params syngit.GitPipelineParams, targetRepository *git.Repository, needForcePush bool) error {
	targetBranch := params.RemoteTarget.Spec.TargetBranch

	variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
		params.RemoteSyncer.Spec.RemoteRepository,
		plumbing.ReferenceName(targetBranch),
		params.GitUserInfo.User,
		params.GitUserInfo.Token,
	)
	var verboseOutput bytes.Buffer
	pushOptions := &git.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", targetBranch, targetBranch)),
		},
		Auth: &http.BasicAuth{
			Username: params.GitUserInfo.User,
			Password: params.GitUserInfo.Token,
		},
		InsecureSkipTLS: params.RemoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput), // Capture verbose output
		Force:           needForcePush,
	}
	if params.CABundle != nil {
		pushOptions.CABundle = params.CABundle
	}
	err := targetRepository.Push(pushOptions)
	if err != nil {
		if strings.Contains(err.Error(), "already up-to-date") {
			return nil
		}
		return fmt.Errorf("failed to push changes: %v\nVerbose output:%s\nVariables: %s", err, verboseOutput.String(), variables)
	}

	return nil
}
