/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
)

type Repo struct {
	Fqdn  string
	Owner string
	Name  string
}

type Tree struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Sha     string `json:"sha"`
	URL     string `json:"url"`
	Entries []Tree `json:"tree,omitempty"`
}

type Commit struct {
	ID        string    `json:"id"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	URL       string    `json:"url"`
	Author    struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"author"`
}

var Repos = map[string]Repo{}

var GitP1Fqdn string
var GitP2Fqdn string

func RetrieveRepos() {
	Repos[os.Getenv("PLATFORM1")+"-"+os.Getenv("REPO1")] = Repo{
		Fqdn:  GitP1Fqdn,
		Owner: os.Getenv("ADMIN_USERNAME"),
		Name:  os.Getenv("REPO1"),
	}
	Repos[os.Getenv("PLATFORM1")+"-"+os.Getenv("REPO2")] = Repo{
		Fqdn:  GitP1Fqdn,
		Owner: os.Getenv("ADMIN_USERNAME"),
		Name:  os.Getenv("REPO2"),
	}
	Repos[os.Getenv("PLATFORM2")+"-"+os.Getenv("REPO1")] = Repo{
		Fqdn:  GitP2Fqdn,
		Owner: os.Getenv("ADMIN_USERNAME"),
		Name:  os.Getenv("REPO1"),
	}
	Repos[os.Getenv("PLATFORM2")+"-"+os.Getenv("REPO2")] = Repo{
		Fqdn:  GitP2Fqdn,
		Owner: os.Getenv("ADMIN_USERNAME"),
		Name:  os.Getenv("REPO2"),
	}
}

func GetRepoTree(repo Repo) ([]Tree, error) {
	return getTree(repo.Fqdn, repo.Owner, repo.Name, "main")
}

func getAdminToken(baseFqdn string) (string, error) {
	const (
		username = "syngituser"
		password = "syngit_password"
	)
	url := fmt.Sprintf("http://%s/api/v1/users/%s/tokens", baseFqdn, username)

	// Prepare the request payload
	tokenName := "admin-e2e-token"
	payload := map[string]interface{}{
		"name":   tokenName,
		"scopes": []string{"all"},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}

	// Add basic auth and headers
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	// Log the response body
	defer resp.Body.Close()
	// body, _ := io.ReadAll(resp.Body)

	// Check HTTP status code
	if resp.StatusCode != http.StatusCreated {
		return "", err
	}

	// Parse the response
	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return "", err
	}

	token, ok := response["sha1"].(string)
	if !ok {
		return "", errors.New("response does not contain 'sha1' field")
	}

	return token, nil
}

// GetRepoTree fetches the full tree of the specified repository.
func getTree(repoFqdn string, repoOwner string, repoName string, sha string) ([]Tree, error) {
	url := fmt.Sprintf("http://%s/api/v1/repos/%s/%s/git/trees/%s", repoFqdn, repoOwner, repoName, sha)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add basic auth header
	token, err := getAdminToken(repoFqdn)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "token "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to get repo tree: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get repo tree: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var tree Tree
	if err := json.Unmarshal(body, &tree); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// If there are directories, recurse into them
	var allEntries []Tree
	for _, entry := range tree.Entries {
		if entry.Type == "tree" { // Directory
			// Recursively call GetRepoTree to get entries in the directory
			subEntries, err := getTree(repoFqdn, repoOwner, repoName, entry.Sha)
			if err != nil {
				return nil, err
			}
			entry.Entries = subEntries
		}
		allEntries = append(allEntries, entry)
	}

	return allEntries, nil
}

func IsObjectInRepo(repo Repo, obj runtime.Object) (bool, error) {
	tree, err := GetRepoTree(repo)
	if err != nil {
		return false, err
	}
	return isObjectInYAML(repo, tree, obj)
}

// IsValueInYAML checks if a YAML file in the tree contains the specified `.metadata.name` with the given value.
func isObjectInYAML(repo Repo, tree []Tree, obj runtime.Object) (bool, error) {
	for _, entry := range tree {
		if entry.Type == "blob" { // Only process files
			url := strings.Replace(entry.URL, "git.example.com", repo.Fqdn, 1)
			content, err := fetchFileContent(url)
			if err != nil {
				return false, fmt.Errorf("failed to fetch content for %s: %w", entry.Path, err)
			}

			if containsYAMLMetadataName(content, obj) {
				return true, nil
			}
		} else {
			exists, err := isObjectInYAML(repo, entry.Entries, obj)
			if err != nil {
				return false, err
			}
			if exists {
				return true, nil
			}
		}
	}
	return false, nil
}

// containsYAMLMetadataName parses the content of a YAML file and checks if `.metadata.name` matches the given value.
func containsYAMLMetadataName(content []byte, obj runtime.Object) bool {
	meta, err := getObjectMetadata(obj)
	if err != nil {
		return false
	}

	var parsed map[string]interface{}
	err = yaml.Unmarshal(content, &parsed)
	if err != nil {
		return false
	}

	// Traverse the YAML structure to check `.metadata.name`
	metadata := parsed["metadata"].(map[interface{}]interface{})
	apiVersion := parsed["apiVersion"].(string)
	kind := parsed["kind"].(string)
	name := metadata["name"].(string)
	namespace := metadata["namespace"].(string)

	constructedApiVersion := obj.GetObjectKind().GroupVersionKind().Group + "/" + obj.GetObjectKind().GroupVersionKind().Version
	return name == meta.GetName() && namespace == meta.GetNamespace() && (apiVersion == constructedApiVersion || (strings.HasPrefix(constructedApiVersion, "/") && !strings.Contains(apiVersion, "/"))) && kind == obj.GetObjectKind().GroupVersionKind().Kind
}

// responseStruct represents the structure of the JSON response from Gitea.
type responseStruct struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// fetchFileContent fetches the content of a file from the given URL.
func fetchFileContent(url string) ([]byte, error) {
	// Create an HTTP client
	client := &http.Client{}

	// Create a new HTTP GET request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch file content: %w", err)
	}
	defer resp.Body.Close()

	// Check for a successful response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch file content, status: %s", resp.Status)
	}

	// Parse the JSON response
	var responseData responseStruct
	err = json.NewDecoder(resp.Body).Decode(&responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	// Check if the content is Base64 encoded
	if responseData.Encoding != "base64" {
		return nil, fmt.Errorf("unsupported encoding: %s", responseData.Encoding)
	}

	// Decode the Base64 content
	decodedContent, err := base64.StdEncoding.DecodeString(responseData.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %w", err)
	}

	return decodedContent, nil
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
		return nil, err
	}
	defer resp.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to get latest commit: %v", err)
	}
	defer resp.Body.Close()

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

func GetGiteaURL(namespace string) (string, error) {
	// Run kubectl to get the NodePort of the gitea service in the given namespace
	port, err := exec.Command("kubectl", "get", "svc", "gitea-http", "-n", namespace, "-o", "jsonpath={.spec.ports[0].nodePort}").Output()
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
