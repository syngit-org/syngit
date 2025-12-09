package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func Merge(repo Repo, sourceBranch string, targetBranch string) error {
	return merge(repo, sourceBranch, targetBranch)
}

func GetGiteaURL(namespace string) (string, error) {
	// Run kubectl to get the NodePort of the gitea service in the given namespace
	port, err := exec.Command("kubectl", "get", "svc", "gitea-http", "-n", namespace, "-o", "jsonpath={.spec.ports[0].nodePort}").Output() //nolint:lll
	if err != nil {
		return "", err
	}
	ip, err := exec.Command("kubectl", "get", "node", "-o", "jsonpath={.items[0].status.addresses[0].address}").Output()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s:%s", ip, port)
	return url, nil
}

func AreObjectsUploaded(repo Repo, objects []runtime.Object) bool {
	for _, object := range objects {
		isObjInRepo, err := IsObjectInRepo(repo, object)
		if err != nil || !isObjInRepo {
			return false
		}
	}
	return true
}

func GetObjectInRepo(repo Repo, tree []Tree, obj runtime.Object) ([]File, error) {
	return searchForObjectInAllManifests(repo, tree, obj, false)
}

func IsObjectInRepo(repo Repo, obj runtime.Object) (bool, error) {
	tree, err := GetRepoTree(repo)
	if err != nil {
		return false, err
	}
	files, err := searchForObjectInAllManifests(repo, tree, obj, false)
	return len(files) > 0, err
}

func SearchForObjectInRepo(repo Repo, obj runtime.Object) ([]File, error) {
	tree, err := GetRepoTree(repo)
	if err != nil {
		return nil, err
	}
	return searchForObjectInAllManifests(repo, tree, obj, true)
}

func IsFieldDefined(repo Repo, obj runtime.Object, yamlPath string) (bool, error) {
	found := false

	tree, err := GetRepoTree(repo)
	if err != nil {
		return false, err
	}
	files, err := searchForObjectInAllManifests(repo, tree, obj, false)
	if err != nil {
		return false, err
	}

	for _, file := range files {
		var parsed map[string]interface{}
		err = yaml.Unmarshal(file.Content, &parsed)
		if err != nil {
			return false, err
		}
		_, found = isFieldDefinedInYaml(parsed, yamlPath)
	}

	return found, nil
}

// GetLatestCommit fetches metadata of the latest commit from the specified repository.
func GetLatestCommit(repoUrl string, repoOwner string, repoName string) (*Commit, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/commits?limit=1", repoUrl, repoOwner, repoName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add basic auth header
	token, err := getAdminToken(repoUrl)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "token "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest commit: %v", err)
	}
	defer resp.Body.Close() // nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get latest commit: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var commits []Commit
	if err := json.Unmarshal(body, &commits); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found in the repository")
	}

	return &commits[0], nil
}

func GetRepoTree(repo Repo) ([]Tree, error) {
	branch := "main"
	if repo.Branch != "" {
		branch = repo.Branch
	}
	return getTree(repo.Fqdn, repo.Owner, repo.Name, branch)
}

func CommitObjectOnSpecifiedPath(repo Repo, obj runtime.Object, path string) error {
	content, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}
	return commitYamlOnSpecifiedPath(repo, content, path)
}

func CommitYamlOnSpecifiedPath(repo Repo, content []byte, path string) error {
	return commitYamlOnSpecifiedPath(repo, content, path)
}
