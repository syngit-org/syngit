package interceptor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	admissionv1 "k8s.io/api/admission/v1"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type GitPusher struct {
	remoteSyncer    syngit.RemoteSyncer
	remoteTarget    syngit.RemoteTarget
	interceptedYAML string
	interceptedGVR  schema.GroupVersionResource
	interceptedName string
	gitUser         string
	gitEmail        string
	gitToken        string
	operation       admissionv1.Operation
	caBundle        []byte
}

type GitPushResponse struct {
	path       string // The git path were the resource has been pushed
	commitHash string // The commit hash of the commit
	url        string // The url of the repository
}

var forcePush bool

func (gp *GitPusher) Push() (GitPushResponse, error) {
	gpResponse := &GitPushResponse{path: "", commitHash: "", url: gp.remoteTarget.Spec.TargetRepository}

	var w *git.Worktree

	repoRetriever := RepoRetriever{gitPusher: gp}

	// PRE-STEP 1 : Get the repos
	targetRepo, getRepoErr := repoRetriever.GetTargetRepository()
	if getRepoErr != nil {
		return *gpResponse, getRepoErr
	}
	// Set the upstream repo the same as the target one by default
	// Considering the target branch to be the same as the uypstream one
	upstreamRepo := targetRepo

	// If a merge strategy is set, then the target & upstream are different
	if gp.remoteTarget.Spec.MergeStrategy != "" {
		// Different target and upstream
		upstreamRepo, getRepoErr = repoRetriever.GetUpstreamRepository()
		if getRepoErr != nil {
			return *gpResponse, getRepoErr
		}
	}

	// PRE-STEP 2 : Get the worktree
	wr := WorktreeRetriever{
		upstreamRepository: upstreamRepo,
		targetRepository:   targetRepo,
		strategy:           gp.remoteTarget.Spec.MergeStrategy,
	}
	var err error
	w, forcePush, err = wr.GetWorkTree(*gp)
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
	err = gp.pushChanges(targetRepo)
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
		errMsg := fmt.Sprintf("failed to commit changes (%s - %s): %s", gp.remoteTarget.Spec.TargetRepository, gp.remoteTarget.Spec.TargetBranch, err.Error())
		return "", errors.New(errMsg)
	}

	return commit.String(), nil
}

func (gp *GitPusher) pushChanges(repo *git.Repository) error {
	targetBranch := gp.remoteTarget.Spec.TargetBranch

	variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
		gp.remoteSyncer.Spec.RemoteRepository,
		plumbing.ReferenceName(targetBranch),
		gp.gitUser,
		gp.gitEmail,
	)
	var verboseOutput bytes.Buffer
	pushOptions := &git.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", targetBranch, targetBranch)),
		},
		Auth: &http.BasicAuth{
			Username: gp.gitUser,
			Password: gp.gitToken,
		},
		InsecureSkipTLS: gp.remoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput), // Capture verbose output
		Force:           forcePush,
	}
	if gp.caBundle != nil {
		pushOptions.CABundle = gp.caBundle
	}
	err := repo.Push(pushOptions)
	if err != nil {
		errMsg := fmt.Sprintf("failed to push changes: %s\nVerbose output:%s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return errors.New(errMsg)
	}

	return nil
}
