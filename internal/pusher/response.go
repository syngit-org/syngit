package pusher

type GitPushResponse struct {
	Paths      []string // The git paths where the resource has been pushed
	CommitHash string   // The commit hash of the commit
	URL        string   // The url of the repository
}

func ResponseBuilder(paths []string, commitHash, url string) GitPushResponse {
	return GitPushResponse{
		Paths:      paths,
		CommitHash: commitHash,
		URL:        url,
	}
}
