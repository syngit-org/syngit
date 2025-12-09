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

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	httpgit "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const (
	gitAdminUsername = "syngituser"
	gitAdminPassword = "syngit_password"
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
		username = gitAdminUsername
		password = gitAdminPassword
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
func searchForObjectInAllManifests(
	repo Repo,
	tree []Tree,
	obj runtime.Object,
	shouldContentMatch bool) ([]File, error) {
	files := []File{}
	for _, entry := range tree {
		if entry.Type == "blob" { // Only process files
			url := strings.Replace(entry.URL, "git.example.com", repo.Fqdn, 1)
			content, err := fetchFileContent(url)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch content for %s: %w", entry.Path, err)
			}

			if containsYamlMetadataName(content, obj) {
				if !shouldContentMatch || (shouldContentMatch && containsYamlObjectSpec(content, obj)) {
					files = append(files, File{Path: entry.Path, Content: content})
				}
			}
		} else {
			file, err := searchForObjectInAllManifests(repo, entry.Entries, obj, shouldContentMatch)
			if err != nil {
				continue
			}
			if file != nil {
				return file, nil
			}
			files = append(files, file...)
		}
	}

	if len(files) == 0 {
		return []File{}, errors.New("object not found in all of the manifests of the repository")
	}

	return files, nil
}

// containsYAMLMetadataName parses the content of a YAML file
// and checks if `.metadata.name` matches the given value.
// It handles both single and multi-document YAML files (separated by ---).
func containsYamlMetadataName(content []byte, obj runtime.Object) bool {
	metadata, err := getObjectMetadata(obj)
	if err != nil {
		return false
	}

	// Split content by document separator to handle multi-document YAML
	documents := splitYamlDocuments(content)

	// Check each document
	for _, doc := range documents {
		if checkDocumentMetadataMatch(doc, obj, metadata) {
			return true
		}
	}

	return false
}

// splitYamlDocuments splits multi-document YAML content by the --- separator
func splitYamlDocuments(content []byte) [][]byte {
	contentStr := string(content)
	// Split by YAML document separator
	parts := strings.Split(contentStr, "\n---\n")

	var documents [][]byte
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		// Skip empty documents
		if trimmed != "" && trimmed != "---" {
			documents = append(documents, []byte(trimmed))
		}
	}

	// If no documents found after splitting, return the original content
	if len(documents) == 0 {
		documents = append(documents, content)
	}

	return documents
}

// checkDocumentMetadataMatch checks if a single YAML document matches the object's metadata
func checkDocumentMetadataMatch(content []byte, obj runtime.Object, metadata metav1.Object) bool {
	var parsed map[string]interface{}
	err := yaml.Unmarshal(content, &parsed)
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

// containsYamlObjectSpec parses the content of a YAML file and
// checks if the spec and/or data fields match the given object.
// It handles both single and multi-document YAML files (separated by ---).
func containsYamlObjectSpec(content []byte, obj runtime.Object) bool {
	// Split content by document separator to handle multi-document YAML
	documents := splitYamlDocuments(content)

	// Marshal the object to YAML to extract its spec/data
	objYaml, err := yaml.Marshal(obj)
	if err != nil {
		return false
	}

	var objParsed map[string]interface{}
	err = yaml.Unmarshal(objYaml, &objParsed)
	if err != nil {
		return false
	}

	// Check each document
	for _, doc := range documents {
		if checkDocumentSpecMatch(doc, objParsed) {
			return true
		}
	}

	return false
}

// checkDocumentSpecMatch checks if a single YAML document's spec/data matches the object
func checkDocumentSpecMatch(content []byte, objParsed map[string]interface{}) bool {
	var parsed map[string]interface{}
	err := yaml.Unmarshal(content, &parsed)
	if err != nil {
		return false
	}

	// Check if spec field exists and matches
	yamlSpec, yamlHasSpec := isFieldDefinedInYaml(parsed, "spec")
	objSpec, objHasSpec := isFieldDefinedInYaml(objParsed, "spec")

	if yamlHasSpec && objHasSpec {
		yamlSpecJSON, err := json.Marshal(yamlSpec)
		if err != nil {
			return false
		}

		objSpecJSON, err := json.Marshal(objSpec)
		if err != nil {
			return false
		}

		if string(yamlSpecJSON) != string(objSpecJSON) {
			return false
		}
	} else if yamlHasSpec != objHasSpec {
		// One has spec and the other doesn't
		return false
	}

	// Check if data field exists and matches (for ConfigMaps/Secrets)
	yamlData, yamlHasData := isFieldDefinedInYaml(parsed, "data")
	objData, objHasData := isFieldDefinedInYaml(objParsed, "data")

	if yamlHasData && objHasData {
		yamlDataJSON, err := json.Marshal(yamlData)
		if err != nil {
			return false
		}

		objDataJSON, err := json.Marshal(objData)
		if err != nil {
			return false
		}

		if string(yamlDataJSON) != string(objDataJSON) {
			return false
		}
	} else if yamlHasData != objHasData {
		// One has data and the other doesn't
		return false
	}

	// If neither spec nor data exist in both, return false
	if !yamlHasSpec && !yamlHasData {
		return false
	}

	return true
}

func isFieldDefinedInYaml(parsed map[string]interface{}, path string) (interface{}, bool) {
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
			next, ok := value.(map[string]interface{})
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

func commitYamlOnSpecifiedPath(repository Repo, content []byte, path string) error {
	repoUrl := fmt.Sprintf("https://%s/%s/%s.git", repository.Fqdn, repository.Owner, repository.Name)

	repo, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL:             repoUrl,
		ReferenceName:   plumbing.NewBranchReferenceName(repository.Branch),
		SingleBranch:    true,
		Depth:           1,
		InsecureSkipTLS: true,
		Auth: &httpgit.BasicAuth{
			Username: gitAdminUsername,
			Password: gitAdminPassword,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to clone repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	fs := wt.Filesystem
	f, err := fs.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	if _, err := io.Copy(f, bytes.NewReader(content)); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	err = f.Close()
	if err != nil {
		return fmt.Errorf("failed to close the %s file in the worktree: %w", path, err)
	}

	if _, err := wt.Add(path); err != nil {
		return fmt.Errorf("failed to add file: %w", err)
	}

	_, err = wt.Commit("Update "+path, &git.CommitOptions{
		Author: &object.Signature{
			Name: string(Luffy),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	ref := plumbing.NewBranchReferenceName(repository.Branch)
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec(ref + ":" + ref),
		},
		InsecureSkipTLS: true,
		Auth: &httpgit.BasicAuth{
			Username: gitAdminUsername,
			Password: gitAdminPassword,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}
