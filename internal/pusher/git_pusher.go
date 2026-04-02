package pusher

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	admissionv1 "k8s.io/api/admission/v1"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	features "github.com/syngit-org/syngit/pkg/feature"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type GitPusher struct {
	RemoteSyncer    syngit.RemoteSyncer
	RemoteTarget    syngit.RemoteTarget
	InterceptedYAML string
	InterceptedGVR  schema.GroupVersionResource
	InterceptedName string
	GitUser         string
	GitEmail        string
	GitToken        string
	Operation       admissionv1.Operation
	CABundle        []byte
}

type GitPushResponse struct {
	Paths      []string // The git paths where the resource has been pushed
	CommitHash string   // The commit hash of the commit
	URL        string   // The url of the repository
}

var forcePush bool

func (gp *GitPusher) Push() (GitPushResponse, error) {
	gpResponse := &GitPushResponse{Paths: []string{}, CommitHash: "", URL: gp.RemoteTarget.Spec.TargetRepository}

	var w *git.Worktree

	repoRetriever := RepoRetriever{gitPusher: gp}

	// STEP 1 : Get the repos
	targetRepo, getRepoErr := repoRetriever.GetTargetRepository()
	if getRepoErr != nil {
		return *gpResponse, getRepoErr
	}
	// Set the upstream repo the same as the target one by default
	// Considering the target branch to be the same as the uypstream one
	upstreamRepo := targetRepo

	// If a merge strategy is set, then the target & upstream are different
	if gp.RemoteTarget.Spec.MergeStrategy != "" {
		// Different target and upstream
		upstreamRepo, getRepoErr = repoRetriever.GetUpstreamRepository()
		if getRepoErr != nil {
			return *gpResponse, getRepoErr
		}
	}

	// STEP 2 : Get the worktree
	wr := WorktreeRetriever{
		upstreamRepository: upstreamRepo,
		targetRepository:   targetRepo,
		strategy:           gp.RemoteTarget.Spec.MergeStrategy,
	}
	var err error
	w, forcePush, err = wr.GetWorkTree(*gp)
	if err != nil {
		errMsg := "failed to get worktree: " + err.Error()
		return *gpResponse, errors.New(errMsg)
	}

	// STEP 3 : Construct path
	var results ResourceFinderResults
	var pathsShouldExist = map[string]bool{}

	if features.LoadedFeatureGates.Enabled(features.ResourceFinder) &&
		gp.RemoteSyncer.Spec.ResourceFinder {
		resourceFinder := ResourceFinder{
			SearchedGVK:       gp.InterceptedGVR,
			SearchedName:      gp.InterceptedName,
			SearchedNamespace: gp.RemoteSyncer.Namespace,
			Content:           gp.InterceptedYAML,
		}
		results, err = resourceFinder.BuildWorktree(w)
		if err != nil {
			return *gpResponse, err
		}

		for _, path := range results.Paths {
			gpResponse.Paths = append(gpResponse.Paths, path)
			pathsShouldExist[path] = true
		}
	}

	if !features.LoadedFeatureGates.Enabled(features.ResourceFinder) ||
		!gp.RemoteSyncer.Spec.ResourceFinder ||
		!results.Found {
		path, err := gp.pathConstructor(w)
		if err != nil {
			return *gpResponse, err
		}

		fullFilePath, err := gp.writeFile(path, w)
		gpResponse.Paths = append(gpResponse.Paths, fullFilePath)
		if err != nil {
			return *gpResponse, err
		}

		if gp.InterceptedYAML == "" {
			pathsShouldExist[fullFilePath] = false
		} else {
			pathsShouldExist[fullFilePath] = true
		}
	}

	// STEP 4 : Commit the changes
	commitHash, err := gp.commitChanges(w, pathsShouldExist, targetRepo)
	gpResponse.CommitHash = commitHash
	if err != nil {
		return *gpResponse, err
	}

	// STEP 5 : Push the changes
	err = gp.pushChanges(targetRepo)
	if err != nil {
		return *gpResponse, err
	}

	return *gpResponse, nil
}

func (gp *GitPusher) pathConstructor(w *git.Worktree) (string, error) {
	gvr := gp.InterceptedGVR

	tempPath := ""
	if gp.RemoteSyncer.Spec.RootPath != "" {
		tempPath += gp.RemoteSyncer.Spec.RootPath + "/"
	}
	tempPath += gp.RemoteSyncer.Namespace + "/" + gvr.Group + "/" + gvr.Version + "/" + gvr.Resource + "/"

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
		return path + "/", gp.InterceptedName + ".yaml"
	}
	if strings.Contains(pathArr[len(pathArr)-1], ".yaml") || strings.Contains(pathArr[len(pathArr)-1], ".yml") {
		filename := pathArr[len(pathArr)-1]
		pathArr := pathArr[:len(pathArr)-1]
		return strings.Join(pathArr, "/"), filename
	}
	return strings.Join(pathArr, "/"), gp.InterceptedName + ".yaml"
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
		dir, fileName = gp.getFileDirName(fullFilePath, gp.InterceptedName+".yaml")
		fullFilePath = filepath.Join(dir, fileName)
	} else {
		dir, fileName = gp.getFileDirName(fullFilePath, "")
		fullFilePath = filepath.Join(dir, fileName)
	}
	content := []byte(gp.InterceptedYAML)

	if gp.InterceptedYAML == "" { // The file has been deleted
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

func (gp *GitPusher) commitMessageConstructor(current string, isAddition bool) (string, error) {
	commitMessage := ""
	resourceMessage := fmt.Sprintf("%s.%s/%s: %s/%s",
		gp.InterceptedGVR.Resource,
		gp.InterceptedGVR.Group,
		gp.InterceptedGVR.Version,
		gp.RemoteSyncer.Namespace,
		gp.InterceptedName,
	)
	const additionPrefix = "1+"
	const addition = "+"
	const deletionPrefix = "1-"
	const deletion = "-"

	const errorMessage = "error during commit message construction: "

	if current == "" {
		if !isAddition {
			commitMessage = deletionPrefix + resourceMessage
		} else {
			commitMessage = additionPrefix + resourceMessage
		}
	} else {
		if !isAddition {
			deletionSlice := strings.Split(current, deletion)
			lengthBefore := len(deletionSlice[0])
			number, err := strconv.Atoi(deletionSlice[0][lengthBefore-1:])
			if err != nil {
				return "", fmt.Errorf("%s %w", errorMessage, err)
			}
			number++
			if lengthBefore == 3 {
				commitMessage = deletionSlice[0][0:lengthBefore-1] + strconv.Itoa(number) + deletion + current[3:len(current)-1]
			} else {
				commitMessage = strconv.Itoa(number) + deletion + current[3:len(current)-1]
			}
		}
		if isAddition {
			additionSlice := strings.Split(current, addition)
			number, err := strconv.Atoi(additionSlice[0])
			if err != nil {
				return "", fmt.Errorf("%s %w", errorMessage, err)
			}
			number++
			commitMessage = strconv.Itoa(number) + addition + additionSlice[1]
		}
	}

	return commitMessage, nil
}

func (gp *GitPusher) commitChanges(w *git.Worktree, pathsShouldExist map[string]bool, targetRepo *git.Repository) (string, error) {
	commitMessage := ""

	for path, shouldExist := range pathsShouldExist {
		if shouldExist {
			_, err := w.Add(path)
			if err != nil {
				errMsg := "failed to add file to staging area: " + err.Error()
				return "", errors.New(errMsg)
			}
			commitMessage, err = gp.commitMessageConstructor(commitMessage, true)
			if err != nil {
				return "", err
			}
		} else {
			_, err := w.Remove(path)
			if err != nil && !strings.Contains(err.Error(), "entry not found") {
				errMsg := "failed to delete file in staging area: " + err.Error()
				return "", errors.New(errMsg)
			}
			commitMessage, err = gp.commitMessageConstructor(commitMessage, false)
			if err != nil {
				return "", err
			}
		}
	}

	// Commit the changes
	commit, err := w.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  gp.GitUser,
			Email: gp.GitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		if gp.isErrorSkipable(err) {
			ref, refErr := targetRepo.Head()
			if refErr != nil {
				return "", refErr
			}
			commit, commitErr := targetRepo.CommitObject(ref.Hash())
			if commitErr != nil {
				return "", commitErr
			}
			return commit.Hash.String(), nil
		}
		errMsg := fmt.Sprintf("failed to commit changes (%s/%s): %s", gp.RemoteTarget.Spec.TargetRepository, gp.RemoteTarget.Spec.TargetBranch, err.Error())
		return "", errors.New(errMsg)
	}

	return commit.String(), nil
}

func (gp *GitPusher) isErrorSkipable(err error) bool {
	s := err.Error()
	switch {
	case strings.Contains(s, "cannot create empty commit: clean working tree"):
		return true
	case strings.Contains(s, "failed to delete file in staging area: entry not found"):
		return true
	default:
		return false
	}
}

func (gp *GitPusher) pushChanges(repo *git.Repository) error {
	targetBranch := gp.RemoteTarget.Spec.TargetBranch

	variables := fmt.Sprintf("\nRepository: %s\nReference: %s\nUsername: %s\nEmail: %s\n",
		gp.RemoteSyncer.Spec.RemoteRepository,
		plumbing.ReferenceName(targetBranch),
		gp.GitUser,
		gp.GitEmail,
	)
	var verboseOutput bytes.Buffer
	pushOptions := &git.PushOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", targetBranch, targetBranch)),
		},
		Auth: &http.BasicAuth{
			Username: gp.GitUser,
			Password: gp.GitToken,
		},
		InsecureSkipTLS: gp.RemoteSyncer.Spec.InsecureSkipTlsVerify,
		Progress:        io.MultiWriter(&verboseOutput), // Capture verbose output
		Force:           forcePush,
	}
	if gp.CABundle != nil {
		pushOptions.CABundle = gp.CABundle
	}
	err := repo.Push(pushOptions)
	if err != nil {
		if strings.Contains(err.Error(), "already up-to-date") {
			return nil
		}
		errMsg := fmt.Sprintf("failed to push changes: %s\nVerbose output:%s\nVariables: %s\n", err.Error(), verboseOutput.String(), variables)
		return errors.New(errMsg)
	}

	return nil
}
