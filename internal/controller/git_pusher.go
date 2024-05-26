package controller

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"

	kgiov1 "dams.kgio/kgio/api/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type GitPusher struct {
	resourcesInterceptor kgiov1.ResourcesInterceptor
	interceptedYAML      string
	interceptedGVR       schema.GroupVersionResource
	interceptedName      string
	gitUser              string
	gitEmail             string
	gitToken             string
}

type GitPushResponse struct {
	path       string // The git path were the resource has been pushed
	commitHash string // The commit hash of the commit
}

func (gp *GitPusher) Push() (GitPushResponse, error) {
	gpResponse := &GitPushResponse{path: "", commitHash: ""}

	// Clone the repository into memory
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: gp.resourcesInterceptor.Spec.RemoteRepository,
		Auth: &http.BasicAuth{
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
	})
	if err != nil {
		errMsg := "failed to clone repository: " + err.Error()
		return *gpResponse, errors.New(errMsg)
	}

	// Get the working directory for the repository
	w, err := repo.Worktree()
	if err != nil {
		errMsg := "failed to get worktree: " + err.Error()
		return *gpResponse, errors.New(errMsg)
	}

	// STEP 1 : Set the path
	path, fileInfo, err := gp.pathConstructor()
	if err != nil {
		return *gpResponse, err
	}

	// STEP 2 : Write the file
	fullFilePath, err := gp.writeFile(path, fileInfo)
	gpResponse.path = fullFilePath
	if err != nil {
		return *gpResponse, err
	}

	// STEP 3 : Commit the changes
	commitHash, err := gp.commitChanges(w, fullFilePath)
	gpResponse.commitHash = commitHash
	if err != nil {
		return *gpResponse, err
	}

	// STEP 4 : Push the changes

	return *gpResponse, nil
}

func (gp *GitPusher) pathConstructor() (string, fs.FileInfo, error) {
	gvr := gp.interceptedGVR
	gvrn := &kgiov1.GroupVersionResourceName{
		GroupVersionResource: &gvr,
	}

	tempPath := kgiov1.GetPathFromGVRN(gp.resourcesInterceptor.Spec.IncludedResources, *gvrn.DeepCopy())

	if tempPath == "" {
		tempPath = gvr.Group + "/" + gvr.Version + "/" + gvr.Resource + "/" + gp.interceptedName + ".yaml"
	}

	path, err := gp.validatePath(tempPath)
	if err != nil {
		return tempPath, nil, err
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			pathDir := path

			// If the end of the path ends with .yaml or .yml
			pathDir, _ = gp.getFileDirName(path)

			// Path does not exist, create the directory structure
			err = os.MkdirAll(pathDir, 0755)
			if err != nil {
				return pathDir, nil, err
			}
		} else {
			return tempPath, nil, err
		}
	}

	return path, fileInfo, nil
}

func (gp *GitPusher) validatePath(path string) (string, error) {
	// Validate and clean the path
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(path) || !gp.containsInvalidCharacters(path) {
		return path, errors.New("the path is not valid")
	}

	return cleanPath, nil
}

func (gp *GitPusher) containsInvalidCharacters(path string) bool {
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

func (gp *GitPusher) getFileDirName(path string) (string, string) {
	pathArr := strings.Split(path, "/")
	if strings.Contains(pathArr[len(pathArr)-1], ".yaml") || strings.Contains(pathArr[len(pathArr)-1], ".yml") {
		filename := pathArr[len(pathArr)-1]
		pathArr := pathArr[:len(pathArr)-1]
		return strings.Join(pathArr, "/"), filename
	}
	return strings.Join(pathArr, "/"), gp.resourcesInterceptor.Name + ".yaml"
}

func (gp *GitPusher) writeFile(path string, fileInfo fs.FileInfo) (string, error) {
	fullFilePath := path

	if fileInfo.IsDir() {
		fullFilePath = path + gp.interceptedName + ".yaml"
	} else {
		d, f := gp.getFileDirName(path)
		fullFilePath = filepath.Join(d, f)
	}

	dir, fileName := gp.getFileDirName(fullFilePath)
	content := []byte(gp.interceptedYAML)
	err := os.WriteFile(fullFilePath, content, 0644)
	if err != nil {
		errMsg := "failed to create " + fileName + " in the directory " + dir + "; " + err.Error()
		return fullFilePath, errors.New(errMsg)
	}

	return fullFilePath, nil
}

func (gp *GitPusher) commitChanges(w *git.Worktree, pathToAdd string) (string, error) {
	// Add the file to the staging area
	_, err := w.Add(pathToAdd)
	if err != nil {
		errMsg := "failed to add file to staging area: " + err.Error()
		return "", errors.New(errMsg)
	}

	// Commit the changes
	commit, err := w.Commit("Add or modify file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  gp.gitUser,
			Email: gp.gitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		errMsg := "failed to commit changes: " + err.Error()
		return "", errors.New(errMsg)
	}

	return commit.String(), nil
}
