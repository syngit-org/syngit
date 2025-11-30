package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func getAPIVersions() ([]string, error) {
	// Get the absolute path to the api directory
	apiDir := filepath.Join("pkg", "api")
	var versions []string

	entries, err := os.ReadDir(apiDir)
	if err != nil {
		return versions, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}

	if len(versions) == 0 {
		return versions, fmt.Errorf("no API versions found in %s", apiDir)
	}

	// Sort versions - this works because of the version format (v1alphaN < v1betaN)
	sort.Strings(versions)

	return versions, nil
}

// GetLatestAPIVersion returns the latest API version by scanning the api directory
// Returns the latest version (e.g. "v1beta3") or an error if unable to read versions
func GetLatestAPIVersion() (string, error) {
	versions, err := getAPIVersions()
	return versions[len(versions)-1], err
}

// GetPrevLatestAPIVersion returns the API version before the latest one by scanning the api directory
// Returns the version before the latest one (e.g. "v1beta2") or an error if unable to read versions
func GetPrevLatestAPIVersion() (string, error) {
	versions, err := getAPIVersions()
	return versions[len(versions)-2], err
}
