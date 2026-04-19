package pusher

import "github.com/syngit-org/syngit/pkg/interceptor"

func ResponseBuilder(paths []string, commitHash, url string) interceptor.GitPushResponse {
	return interceptor.GitPushResponse{
		Paths:      paths,
		CommitHash: commitHash,
		URL:        url,
	}
}
