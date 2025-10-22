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
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:golint,revive
	syngit_utils "github.com/syngit-org/syngit/pkg/utils"
)

func warnError(err error) {
	fmt.Fprintf(GinkgoWriter, "warning: %v\n", err) //nolint
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is err="secrets \"cert-manager-webhook-ca\" already exists"
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.Split(wd, "/test/e2e")[0]
	return wd, nil
}

func SanitizeUsername(username string) string {
	return syngit_utils.Sanitize(username)
}
