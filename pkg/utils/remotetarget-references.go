package utils

import (
	"fmt"
	"net/url"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

func RemoteTargetNameConstructor(upstreamRepo string, upstreamBranch string, branch string) (string, error) {

	u, err := url.Parse(upstreamRepo)
	if err != nil {
		return "", err
	}

	repoName := strings.ReplaceAll(strings.ReplaceAll(u.Path, "/", "-"), ".git", "")
	name := fmt.Sprintf("%s%s-%s%s-%s", syngit.RtPrefix, repoName, upstreamBranch, repoName, branch)

	return name, nil
}

func GetBranchesFromAnnotation(in string) []string {
	out := strings.Split(strings.ReplaceAll(in, " ", ""), ",")
	if len(out) == 1 && out[0] == "" {
		return []string{}
	}
	return out
}
