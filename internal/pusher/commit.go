package pusher

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
)

func GetPathsFromModifiedPaths(modifiedPaths syngit.ModifiedPaths) []string {
	return append(modifiedPaths.Add, modifiedPaths.Delete...)
}

func Commit(params syngit.GitPipelineParams, worktree *git.Worktree, paths syngit.ModifiedPaths, targetRepository *git.Repository) (string, error) {
	for _, path := range paths.Add {
		_, err := worktree.Add(path)
		if err != nil {
			errMsg := "failed to add file to staging area: " + err.Error()
			return "", errors.New(errMsg)
		}
	}

	for _, path := range paths.Delete {
		_, err := worktree.Remove(path)
		if err != nil && !strings.Contains(err.Error(), "entry not found") {
			errMsg := "failed to delete file in staging area: " + err.Error()
			return "", errors.New(errMsg)
		}
	}

	commitMessage := buildCommitMessage(params, paths)

	// Commit the changes
	commit, err := worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  params.GitUserInfo.User,
			Email: params.GitUserInfo.Token,
			When:  time.Now(),
		},
	})
	if err != nil {
		if isErrorSkipable(err) {
			ref, refErr := targetRepository.Head()
			if refErr != nil {
				return "", refErr
			}
			commit, commitErr := targetRepository.CommitObject(ref.Hash())
			if commitErr != nil {
				return "", commitErr
			}
			return commit.Hash.String(), nil
		}
		return "", fmt.Errorf("failed to commit changes (%s/%s): %v", params.RemoteTarget.Spec.TargetRepository, params.RemoteTarget.Spec.TargetBranch, err)
	}

	return commit.String(), nil
}

func buildCommitMessage(params syngit.GitPipelineParams, paths syngit.ModifiedPaths) string {
	resourceMessage := fmt.Sprintf("%s.%s/%s: %s/%s",
		params.InterceptedGVR.Resource,
		params.InterceptedGVR.Group,
		params.InterceptedGVR.Version,
		params.RemoteSyncer.Namespace,
		params.InterceptedName,
	)

	additionMsg := ""
	deletionMsg := ""

	addPaths := len(paths.Add)
	if addPaths > 0 {
		additionMsg = fmt.Sprintf("%d+", addPaths)
	}
	deletePaths := len(paths.Delete)
	if deletePaths > 0 {
		deletionMsg = fmt.Sprintf("%d-", deletePaths)
	}

	return fmt.Sprintf("%s%s %s", additionMsg, deletionMsg, resourceMessage)
}

func isErrorSkipable(err error) bool {
	switch {
	case errors.Is(err, git.ErrEmptyCommit):
		return true
	case strings.Contains(err.Error(), "failed to delete file in staging area: entry not found"):
		return true
	default:
		return false
	}
}
