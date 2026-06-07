package mutator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/syngit-org/syngit/pkg/interceptor"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type DefaultWorktreeCustomizer struct{}

// Writes the artifact to the pre-determined path:
// ([RootPath/]<namespace>/<group>/<version>/<resource>/<name>.yaml)
// and returns the claimed paths.
func (dt DefaultWorktreeCustomizer) place(params interceptor.GitPipelineParams, artifacts ArtifactSet, worktree *git.Worktree) (interceptor.ClaimedPaths, error) {
	claimed := interceptor.NewClaimedPaths()

	for _, a := range artifacts.Items {
		path, err := dt.pathConstructor(params, a.GVR, worktree)
		if err != nil {
			return interceptor.NewClaimedPaths(), err
		}

		fullFilePath, err := dt.writeFile(params, a.Content, path, worktree)
		if err != nil {
			return interceptor.NewClaimedPaths(), err
		}

		if a.IsDeletion() {
			claimed.AppendDeletedPath(fullFilePath)
		} else {
			claimed.AppendAddedPath(fullFilePath)
		}
	}

	return claimed, nil
}

func (dt DefaultWorktreeCustomizer) pathConstructor(params interceptor.GitPipelineParams, gvr schema.GroupVersionResource, worktree *git.Worktree) (string, error) {
	tempPath := ""
	if params.RemoteSyncer.Spec.RootPath != "" {
		tempPath += params.RemoteSyncer.Spec.RootPath + "/"
	}
	tempPath += params.RemoteSyncer.Namespace + "/" + gvr.Group + "/" + gvr.Version + "/" + gvr.Resource + "/"

	path, err := dt.validatePath(tempPath)
	if err != nil {
		return tempPath, err
	}

	_, err = worktree.Filesystem.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// If the end of the path ends with .yaml or .yml
			pathDir, _ := dt.getFileDirName(params.InterceptedName, path, "")

			// Path does not exist, create the directory structure
			err = worktree.Filesystem.MkdirAll(pathDir, 0755)
			if err != nil {
				return pathDir, err
			}
		} else {
			return tempPath, err
		}
	}

	return path, nil
}

func (dt DefaultWorktreeCustomizer) validatePath(path string) (string, error) {
	// Validate and clean the path
	cleanPath := filepath.Clean(path)
	// !filepath.IsAbs(cleanPath) test absolute path ?
	if dt.containsInvalidCharacters(cleanPath) {
		return path, errors.New("the path is not valid")
	}

	return cleanPath, nil
}

func (dt DefaultWorktreeCustomizer) containsInvalidCharacters(path string) bool {
	invalidChars := []rune{':', '*', '?', '"', '<', '>', '|'}
	for _, char := range path {
		for _, invalidChar := range invalidChars {
			if char == invalidChar {
				return true
			}
		}
	}
	return false
}

func (dt DefaultWorktreeCustomizer) getFileDirName(resourceName, path, filename string) (string, string) {
	pathArr := strings.Split(path, "/")
	if filename == "" {
		return path + "/", resourceName + ".yaml"
	}
	if strings.Contains(pathArr[len(pathArr)-1], ".yaml") || strings.Contains(pathArr[len(pathArr)-1], ".yml") {
		filename := pathArr[len(pathArr)-1]
		pathArr := pathArr[:len(pathArr)-1]
		return strings.Join(pathArr, "/"), filename
	}
	return strings.Join(pathArr, "/"), resourceName + ".yaml"
}

func (dt DefaultWorktreeCustomizer) writeFile(params interceptor.GitPipelineParams, content []byte, path string, w *git.Worktree) (string, error) {
	fullFilePath := path
	dir := ""

	fileInfo, err := w.Filesystem.Stat(fullFilePath)
	if err != nil {
		return fullFilePath, fmt.Errorf("failed to stat file %s: %v", fullFilePath, err)
	}

	fileName := ""
	if fileInfo.IsDir() {
		fileName = params.InterceptedName + ".yaml"
	}
	dir, fileName = dt.getFileDirName(params.InterceptedName, fullFilePath, fileName)
	fullFilePath = filepath.Join(dir, fileName)

	if content == nil { // The file has been deleted
		return fullFilePath, nil
	}

	if err := writeWorktreeFile(w, fullFilePath, content); err != nil {
		return fullFilePath, fmt.Errorf("failed to write to file %s: %v", fullFilePath, err)
	}

	return fullFilePath, nil
}
