package utils

import (
	"fmt"
	"net/url"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
)

func RemoteTargetNameConstructor(remoteSyncer syngit.RemoteSyncer, branch string) (string, error) {

	u, err := url.Parse(remoteSyncer.Spec.RemoteRepository)
	if err != nil {
		return "", err
	}

	repoName := strings.ReplaceAll(strings.ReplaceAll(u.Path, "/", "-"), ".git", "")
	name := fmt.Sprintf("%s%s-%s%s-%s", syngit.RtPrefix, repoName, remoteSyncer.Spec.DefaultBranch, repoName, branch)

	return name, nil
}
