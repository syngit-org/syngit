package pusher

import (
	"bytes"
	"fmt"
	"io"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
)

type GetRepositoryParams struct {
	GitUserInfo  syngit.GitUserInfo
	RemoteSyncer syngit.RemoteSyncer
	CABundle     []byte
	Repository   string
	Branch       string
}

func getRepository(params GetRepositoryParams) (*git.Repository, error) {
	// Clone the repository into memory
	var verboseOutput bytes.Buffer
	cloneOptions := &git.CloneOptions{
		URL:           params.Repository,
		ReferenceName: plumbing.ReferenceName(params.Branch),
		Auth: &http.BasicAuth{
			Username: params.GitUserInfo.User,
			Password: params.GitUserInfo.Token,
		},
		SingleBranch:    true,
		InsecureSkipTLS: params.RemoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput),
	}
	if params.CABundle != nil {
		cloneOptions.CABundle = params.CABundle
	}
	repository, err := git.Clone(memory.NewStorage(), memfs.New(), cloneOptions)
	if err != nil {
		variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			params.Repository,
			plumbing.ReferenceName(params.Branch),
			params.GitUserInfo.User,
			params.GitUserInfo.Token,
		)
		return nil, fmt.Errorf(
			"failed to clone repository: %v\nVerbose output: %s\nVariables: %s",
			err, verboseOutput.String(), variables,
		)
	}

	return repository, nil
}

func GetUpstreamRepository(params syngit.GitPipelineParams) (*git.Repository, error) {
	return getRepository(GetRepositoryParams{
		RemoteSyncer: params.RemoteSyncer,
		CABundle:     params.CABundle,
		GitUserInfo:  params.GitUserInfo,
		Repository:   params.RemoteTarget.Spec.UpstreamRepository,
		Branch:       params.RemoteTarget.Spec.UpstreamBranch,
	})
}

func GetTargetRepository(params syngit.GitPipelineParams) (*git.Repository, error) {
	return getRepository(GetRepositoryParams{
		RemoteSyncer: params.RemoteSyncer,
		CABundle:     params.CABundle,
		GitUserInfo:  params.GitUserInfo,
		Repository:   params.RemoteTarget.Spec.TargetRepository,
		Branch:       params.RemoteTarget.Spec.UpstreamBranch,
	})
}
