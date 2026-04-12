package v1beta4

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	admissionv1 "k8s.io/api/admission/v1"
)

type GitUserInfo struct {
	User  string
	Email string
	Token string
}

type GitPipelineParams struct {
	RemoteSyncer    RemoteSyncer
	RemoteTarget    RemoteTarget
	InterceptedYAML string
	InterceptedGVR  schema.GroupVersionResource
	InterceptedName string
	GitUserInfo     GitUserInfo
	Operation       admissionv1.Operation
	CABundle        []byte
}

type ModifiedPaths struct {
	Add    []string
	Delete []string
}

func NewModifiedPaths() ModifiedPaths {
	return ModifiedPaths{
		Add:    []string{},
		Delete: []string{},
	}
}

func (mp ModifiedPaths) IsModified() bool {
	return len(mp.Add) > 0 || len(mp.Delete) > 0
}

func (mp *ModifiedPaths) AppendAddedPath(path string) {
	mp.Add = append(mp.Add, path)
}

func (mp *ModifiedPaths) AppendDeletedPath(path string) {
	mp.Delete = append(mp.Delete, path)
}

func (mp *ModifiedPaths) AppendModifiedPaths(paths ModifiedPaths) {
	mp.Delete = append(mp.Delete, paths.Delete...)
	mp.Add = append(mp.Add, paths.Add...)
}
