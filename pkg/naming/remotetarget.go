package naming

import (
	"fmt"
	"net/url"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta5"
)

// RemoteTargetName derives the name of the RemoteTarget that syngit manages for
// one (upstream repo, upstream branch, target repo, target branch) tuple. An
// empty targetRepo means the target is a fork of the upstream one.
func RemoteTargetName(
	upstreamRepo string,
	upstreamBranch string,
	targetRepo string,
	targetBranch string,
) (string, error) {

	upstreamU, err := url.Parse(upstreamRepo)
	if err != nil {
		return "", err
	}

	targetRepoName := syngit.RtManagedDefaultForkNamePrefix
	if targetRepo != "" {
		targetU, err := url.Parse(targetRepo)
		if err != nil {
			return "", err
		}
		targetRepoName = strings.ToLower(strings.ReplaceAll(SoftSanitize(targetU.Path), ".git", ""))
	}

	upstreamRepoName := strings.ToLower(strings.ReplaceAll(SoftSanitize(upstreamU.Path), ".git", ""))
	if len(upstreamRepoName) >= 1 {
		upstreamRepoName = upstreamRepoName[1:]
	}
	name := fmt.Sprintf("%s-%s%s-%s",
		upstreamRepoName,
		strings.ToLower(upstreamBranch),
		targetRepoName,
		strings.ToLower(targetBranch),
	)

	return name, nil
}
