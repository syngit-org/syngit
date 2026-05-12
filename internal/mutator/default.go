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

func (dt DefaultWorktreeCustomizer) Customize(params interceptor.GitPipelineParams, mutations Mutations, customWorktree *CustomWorktree) error {
	for gvr, content := range mutations {
		path, err := dt.pathConstructor(params, gvr, customWorktree.Worktree)
		if err != nil {
			return err
		}

		fullFilePath, err := dt.writeFile(params, content, path, customWorktree.Worktree)
		if err != nil {
			return err
		}

		if params.InterceptedYAML == "" {
			customWorktree.ModifiedPaths.AppendDeletedPath(fullFilePath)
		} else {
			customWorktree.ModifiedPaths.AppendAddedPath(fullFilePath)
		}
	}

	return nil
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

	file, err := w.Filesystem.Create(fullFilePath)
	if err != nil {
		return fullFilePath, fmt.Errorf("failed to create file %s: %v", fullFilePath, err)
	}

	_, err = file.Write(content)
	if err != nil {
		return fullFilePath, fmt.Errorf("failed to write to file %s: %v", fullFilePath, err)
	}
	err = file.Close()

	return fullFilePath, err
}
