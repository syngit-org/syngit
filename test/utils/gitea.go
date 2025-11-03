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
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type Repo struct {
	Fqdn   string
	Owner  string
	Name   string
	Branch string
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

type File struct {
	Content []byte `json:"content"`
	Path    string `json:"path"`
}

var Repos = map[string]Repo{}

func getAdminToken(baseFqdn string) (string, error) {
	const (
		username = "syngituser"
		password = "syngit_password"
	)
	url := fmt.Sprintf("https://%s/api/v1/users/%s/tokens", baseFqdn, username)

	// Prepare the request payload
	tokenName := fmt.Sprintf("%s%c", "admin-e2e-token", rand.IntN(1000))
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

	// Skip Tls verify
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Skip TLS verification
		},
	}

	// Send the request
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close() //nolint:errcheck

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
	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/git/trees/%s", repoFqdn, repoOwner, repoName, sha)
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

	// Skip Tls verify
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Skip TLS verification
		},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

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
	var allEntries []Tree //nolint:prealloc
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

// searchForObjectInAllManifests checks if a YAML file in the tree
// contains the specified `.metadata.name` with the given value.
func searchForObjectInAllManifests(repo Repo, tree []Tree, obj runtime.Object) (*File, error) {
	for _, entry := range tree {
		if entry.Type == "blob" { // Only process files
			url := strings.Replace(entry.URL, "git.example.com", repo.Fqdn, 1)
			content, err := fetchFileContent(url)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch content for %s: %w", entry.Path, err)
			}

			if containsYamlMetadataName(content, obj) {
				return &File{Path: entry.Path, Content: content}, nil
			}
		} else {
			file, err := searchForObjectInAllManifests(repo, entry.Entries, obj)
			if err != nil {
				continue
			}
			if file != nil {
				return file, nil
			}
		}
	}
	return nil, errors.New("object not found in all of the manifests of the repository")
}

// containsYAMLMetadataName parses the content of a YAML file and checks if `.metadata.name` matches the given value.
func containsYamlMetadataName(content []byte, obj runtime.Object) bool {
	metadata, err := getObjectMetadata(obj)
	if err != nil {
		return false
	}

	var parsed map[interface{}]interface{}
	err = yaml.Unmarshal(content, &parsed)
	if err != nil {
		return false
	}

	apiVersion := ""
	if apiVersionValue, ok := isFieldDefinedInYaml(parsed, "apiVersion"); ok {
		apiVersion = apiVersionValue.(string)
	}
	kind := ""
	if kindValue, ok := isFieldDefinedInYaml(parsed, "kind"); ok {
		kind = kindValue.(string)
	}
	name := ""
	if nameValue, ok := isFieldDefinedInYaml(parsed, "metadata.name"); ok {
		name = nameValue.(string)
	}
	namespace := ""
	if namespaceValue, ok := isFieldDefinedInYaml(parsed, "metadata.namespace"); ok {
		namespace = namespaceValue.(string)
	}

	constructedApiVersion := obj.GetObjectKind().GroupVersionKind().Group + "/" + obj.GetObjectKind().GroupVersionKind().Version //nolint:lll

	return name == metadata.GetName() &&
		namespace == metadata.GetNamespace() &&
		(apiVersion == constructedApiVersion || (strings.HasPrefix(constructedApiVersion, "/") &&
			!strings.Contains(apiVersion, "/"))) &&
		kind == obj.GetObjectKind().GroupVersionKind().Kind
}

func isFieldDefinedInYaml(parsed map[interface{}]interface{}, path string) (interface{}, bool) {
	// Split the path by dots to traverse the map
	keys := strings.Split(path, ".")
	current := parsed

	for i, key := range keys {
		if value, exists := current[key]; exists {
			// Check if we are at the last key in the path
			if i == len(keys)-1 {
				return value, true
			}

			// Check if the value is a nested map
			next, ok := value.(map[interface{}]interface{})
			if ok {
				return isFieldDefinedInYaml(next, strings.Join(keys[1:], "."))
			} else {
				return nil, false // Not a nested map, but there are more keys in the path
			}
		} else {
			return nil, false // Key does not exist
		}
	}
	return nil, false
}

// responseStruct represents the structure of the JSON response from Gitea.
type responseStruct struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// fetchFileContent fetches the content of a file from the given URL.
func fetchFileContent(url string) ([]byte, error) {
	// Skip Tls verify
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Skip TLS verification
		},
	}

	// Create an HTTP client
	client := &http.Client{Transport: tr}

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
	defer resp.Body.Close() //nolint:errcheck

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

func getObjectMetadata(obj runtime.Object) (metav1.Object, error) {
	// Use meta.Accessor to get the metadata
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to access metadata: %w", err)
	}
	return metadata, nil
}

func merge(repo Repo, sourceBranch string, targetBranch string) error {
	prUrl := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls", repo.Fqdn, repo.Owner, repo.Name)

	// CREATE PULL REQUEST
	data := map[string]interface{}{
		"head":  sourceBranch,
		"base":  targetBranch,
		"title": fmt.Sprintf("Merge %s into %s", sourceBranch, targetBranch),
		"body":  "This pull request was created programmatically.",
	}

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", prUrl, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	apiToken, err := getAdminToken(repo.Fqdn)
	if err != nil {
		return fmt.Errorf("failed to get the apiToken")
	}
	req.Header.Set("Authorization", "token "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	// Skip Tls verify
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Skip TLS verification
		},
	}

	// Send the request
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create pull request: %s", string(bodyBytes))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	pullUrl := strings.Split(response["html_url"].(string), "/")
	pullID, _ := strconv.Atoi(pullUrl[len(pullUrl)-1])

	// MERGE PULL REQUEST
	mergeUrl := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls/%d/merge", repo.Fqdn, repo.Owner, repo.Name, pullID)

	data = map[string]interface{}{
		"do":            "merge",
		"merge_title":   "Merging branches programmatically",
		"merge_message": "Merge completed using Gitea API",
	}

	body, err = json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err = http.NewRequest("POST", mergeUrl, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client = &http.Client{Transport: tr}
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to merge pull request: %s", string(bodyBytes))
	}

	return nil
}
