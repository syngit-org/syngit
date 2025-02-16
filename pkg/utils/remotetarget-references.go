package utils

import (
	"fmt"
	"net/url"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

func RemoteTargetNameConstructor(upstreamRepo string, upstreamBranch string, targetRepo string, targetBranch string) (string, error) {

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
		targetRepoName = strings.ReplaceAll(strings.ReplaceAll(targetU.Path, "/", "-"), ".git", "")
	}

	upstreamRepoName := strings.ReplaceAll(strings.ReplaceAll(upstreamU.Path, "/", "-"), ".git", "")
	name := fmt.Sprintf("%s%s-%s%s-%s", syngit.RtManagedNamePrefix, upstreamRepoName, upstreamBranch, targetRepoName, targetBranch)

	return name, nil
}

func GetBranchesFromAnnotation(in string) []string {
	out := strings.Split(strings.ReplaceAll(in, " ", ""), ",")
	if len(out) == 1 && out[0] == "" {
		return []string{}
	}
	return out
}
