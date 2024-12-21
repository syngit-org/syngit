package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	admissionv1 "k8s.io/api/admission/v1"

	syngit "github.com/syngit-org/syngit/api/v1beta2"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type GitPusher struct {
	remoteSyncer          syngit.RemoteSyncer
	interceptedYAML       string
	interceptedGVR        schema.GroupVersionResource
	interceptedName       string
	branch                string
	gitUser               string
	gitEmail              string
	gitToken              string
	operation             admissionv1.Operation
	insecureSkipTlsVerify bool
	caBundle              string
}

type GitPushResponse struct {
	path       string // The git path were the resource has been pushed
	commitHash string // The commit hash of the commit
}

func (gp *GitPusher) Push() (GitPushResponse, error) {
	gpResponse := &GitPushResponse{path: "", commitHash: ""}
	gp.branch = gp.remoteSyncer.Spec.DefaultBranch

	// Clone the repository into memory
	var verboseOutput bytes.Buffer
	cloneOption := &git.CloneOptions{
		URL:           gp.remoteSyncer.Spec.RemoteRepository,
		ReferenceName: plumbing.ReferenceName(gp.branch),
		Auth: &http.BasicAuth{
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
		SingleBranch:    true,
		InsecureSkipTLS: gp.insecureSkipTlsVerify,
		Progress:        io.MultiWriter(os.Stdout, &verboseOutput),
	}
	if gp.caBundle != "" {
		cloneOption.CABundle = []byte(gp.caBundle)
	}
	repo, err := git.Clone(memory.NewStorage(), memfs.New(), cloneOption)
	if err != nil {
		variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
			gp.remoteSyncer.Spec.RemoteRepository,
			plumbing.ReferenceName(gp.branch),
			gp.gitUser,
			gp.gitEmail,
		)
		errMsg := fmt.Sprintf("failed to clone repository: %s\nVerbose output: %s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return *gpResponse, errors.New(errMsg)
	}

	// Get the working directory for the repository
	w, err := repo.Worktree()
	if err != nil {
		errMsg := "failed to get worktree: " + err.Error()
		return *gpResponse, errors.New(errMsg)
	}

	// STEP 1 : Set the path
	path, err := gp.pathConstructor(w)
	if err != nil {
		return *gpResponse, err
	}

	// STEP 2 : Write the file
	fullFilePath, err := gp.writeFile(path, w)
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
	err = gp.pushChanges(repo)
	if err != nil {
		return *gpResponse, err
	}

	return *gpResponse, nil
}

func (gp *GitPusher) pathConstructor(w *git.Worktree) (string, error) {
	gvr := gp.interceptedGVR

	tempPath := ""
	if gp.remoteSyncer.Spec.RootPath != "" {
		tempPath += gp.remoteSyncer.Spec.RootPath + "/"
	}
	tempPath += gp.remoteSyncer.Namespace + "/" + gvr.Group + "/" + gvr.Version + "/" + gvr.Resource + "/"

	path, err := gp.validatePath(tempPath)
	if err != nil {
		return tempPath, err
	}

	_, err = w.Filesystem.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// If the end of the path ends with .yaml or .yml
			pathDir, _ := gp.getFileDirName(path, "")

			// Path does not exist, create the directory structure
			err = w.Filesystem.MkdirAll(pathDir, 0755)
			if err != nil {
				return pathDir, err
			}
		} else {
			return tempPath, err
		}
	}

	return path, nil
}

func (gp *GitPusher) validatePath(path string) (string, error) {
	// Validate and clean the path
	cleanPath := filepath.Clean(path)
	// !filepath.IsAbs(cleanPath) test absolute path ?
	if gp.containsInvalidCharacters(cleanPath) {
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

func (gp *GitPusher) getFileDirName(path string, filename string) (string, string) {
	pathArr := strings.Split(path, "/")
	if filename == "" {
		return path + "/", gp.interceptedName + ".yaml"
	}
	if strings.Contains(pathArr[len(pathArr)-1], ".yaml") || strings.Contains(pathArr[len(pathArr)-1], ".yml") {
		filename := pathArr[len(pathArr)-1]
		pathArr := pathArr[:len(pathArr)-1]
		return strings.Join(pathArr, "/"), filename
	}
	return strings.Join(pathArr, "/"), gp.interceptedName + ".yaml"
}

func (gp *GitPusher) writeFile(path string, w *git.Worktree) (string, error) {
	fullFilePath := path
	dir := ""
	fileName := ""

	fileInfo, err := w.Filesystem.Stat(fullFilePath)
	if err != nil {
		errMsg := "failed to stat file " + fullFilePath + " : " + err.Error()
		return fullFilePath, errors.New(errMsg)
	}
	if fileInfo.IsDir() {
		dir, fileName = gp.getFileDirName(fullFilePath, gp.interceptedName+".yaml")
		fullFilePath = filepath.Join(dir, fileName)
	} else {
		dir, fileName = gp.getFileDirName(fullFilePath, "")
		fullFilePath = filepath.Join(dir, fileName)
	}
	content := []byte(gp.interceptedYAML)

	if gp.interceptedYAML == "" { // The file has been deleted
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

func (gp *GitPusher) commitChanges(w *git.Worktree, pathToAdd string) (string, error) {
	commitMessage := ""

	if gp.interceptedYAML == "" { // The file has been deleted
		_, err := w.Remove(pathToAdd)
		if err != nil {
			errMsg := "failed to delete file in staging area: " + err.Error()
			return "", errors.New(errMsg)
		}
		commitMessage = "Delete "
	} else { // Add the file to the staging area
		_, err := w.Add(pathToAdd)
		if err != nil {
			errMsg := "failed to add file to staging area: " + err.Error()
			return "", errors.New(errMsg)
		}
		commitMessage = "Add or modify "
	}

	// Commit the changes
	commit, err := w.Commit(commitMessage+gp.interceptedGVR.Resource+" "+gp.interceptedName, &git.CommitOptions{
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

func (gp *GitPusher) pushChanges(repo *git.Repository) error {
	variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
		gp.remoteSyncer.Spec.RemoteRepository,
		plumbing.ReferenceName(gp.branch),
		gp.gitUser,
		gp.gitEmail,
	)
	var verboseOutput bytes.Buffer
	pushOptions := &git.PushOptions{
		Auth: &http.BasicAuth{
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
		InsecureSkipTLS: gp.insecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput), // Capture verbose output
	}
	if gp.caBundle != "" {
		pushOptions.CABundle = []byte(gp.caBundle)
	}
	err := repo.Push(pushOptions)
	if err != nil {
		errMsg := fmt.Sprintf("failed to push changes: %s\nVerbose output:%s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return errors.New(errMsg)
	}

	return nil
}
