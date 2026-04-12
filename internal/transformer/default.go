package transformer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
)

type DefaultTransformer struct{}

func (dt DefaultTransformer) Transform(params syngit.GitPipelineParams, worktree *git.Worktree) (*git.Worktree, syngit.ModifiedPaths, error) {
	modifiedPaths := syngit.NewModifiedPaths()

	path, err := dt.pathConstructor(params, worktree)
	if err != nil {
		return worktree, modifiedPaths, err
	}

	fullFilePath, err := dt.writeFile(params, path, worktree)
	if err != nil {
		return worktree, modifiedPaths, err
	}

	if params.InterceptedYAML == "" {
		modifiedPaths.AppendDeletedPath(fullFilePath)
	} else {
		modifiedPaths.AppendAddedPath(fullFilePath)
	}

	return worktree, modifiedPaths, nil
}

func (dt DefaultTransformer) pathConstructor(params syngit.GitPipelineParams, worktree *git.Worktree) (string, error) {
	gvr := params.InterceptedGVR

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

func (dt DefaultTransformer) validatePath(path string) (string, error) {
	// Validate and clean the path
	cleanPath := filepath.Clean(path)
	// !filepath.IsAbs(cleanPath) test absolute path ?
	if dt.containsInvalidCharacters(cleanPath) {
		return path, errors.New("the path is not valid")
	}

	return cleanPath, nil
}

func (dt DefaultTransformer) containsInvalidCharacters(path string) bool {
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

func (dt DefaultTransformer) getFileDirName(resourceName, path, filename string) (string, string) {
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

func (dt DefaultTransformer) writeFile(params syngit.GitPipelineParams, path string, w *git.Worktree) (string, error) {
	fullFilePath := path
	dir := ""

	fileInfo, err := w.Filesystem.Stat(fullFilePath)
	if err != nil {
		errMsg := "failed to stat file " + fullFilePath + " : " + err.Error()
		return fullFilePath, errors.New(errMsg)
	}

	fileName := ""
	if fileInfo.IsDir() {
		fileName = params.InterceptedName + ".yaml"
	}
	dir, fileName = dt.getFileDirName(params.InterceptedName, fullFilePath, fileName)
	fullFilePath = filepath.Join(dir, fileName)

	content := []byte(params.InterceptedYAML)

	if params.InterceptedYAML == "" { // The file has been deleted
		return fullFilePath, nil
	}

	file, err := w.Filesystem.Create(fullFilePath)
	if err != nil {
		errMsg := "failed to create file: " + err.Error()
		return fullFilePath, errors.New(errMsg)
	}

	_, err = file.Write(content)
	if err != nil {
		errMsg := "failed to write to file" + err.Error()
		return fullFilePath, errors.New(errMsg)
	}
	err = file.Close()

	return fullFilePath, err
}
