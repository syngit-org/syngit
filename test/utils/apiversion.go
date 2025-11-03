package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// GetLatestAPIVersion returns the latest API version by scanning the api directory
// Returns the latest version (e.g. "v1beta3") or an error if unable to read versions
func GetLatestAPIVersion() (string, error) {
	// Get the absolute path to the api directory
	apiDir := filepath.Join("pkg", "api")

	entries, err := os.ReadDir(apiDir)
	if err != nil {
		return "", err
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no API versions found in %s", apiDir)
	}

	// Sort versions - this works because of the version format (v1alphaN < v1betaN)
	sort.Strings(versions)

	return versions[len(versions)-1], nil
}
