package interceptor

import (
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	"k8s.io/apimachinery/pkg/runtime/schema"

	admissionv1 "k8s.io/api/admission/v1"
)

type GitUserInfo struct {
	User  string
	Email string
	Token string
}

type GitPipelineParams struct {
	RemoteSyncer    syngit.RemoteSyncer
	RemoteTarget    syngit.RemoteTarget
	InterceptedYAML string
	InterceptedGVR  schema.GroupVersionResource
	InterceptedName string
	GitUserInfo     GitUserInfo
	Operation       admissionv1.Operation
	CABundle        []byte
}

type ClaimedPaths struct {
	Add    []string
	Delete []string
}

func NewClaimedPaths() ClaimedPaths {
	return ClaimedPaths{
		Add:    []string{},
		Delete: []string{},
	}
}

func (mp ClaimedPaths) ClaimExists() bool {
	return len(mp.Add) > 0 || len(mp.Delete) > 0
}

func (mp *ClaimedPaths) AppendAddedPath(path string) {
	mp.Add = append(mp.Add, path)
}

func (mp *ClaimedPaths) AppendDeletedPath(path string) {
	mp.Delete = append(mp.Delete, path)
}

func (mp *ClaimedPaths) AppendClaimedPaths(paths ClaimedPaths) {
	mp.Delete = append(mp.Delete, paths.Delete...)
	mp.Add = append(mp.Add, paths.Add...)
}

type GitPushResponse struct {
	Paths      []string // The git paths where the resource has been pushed
	CommitHash string   // The commit hash of the commit
	URL        string   // The url of the repository
}
