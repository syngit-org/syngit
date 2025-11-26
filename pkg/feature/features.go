// pkg/features/features.go
package features

import (
	"fmt"
	"strconv"
	"strings"
)

type FeatureGates map[Feature]bool

type Feature string

const (
	ResourceFinder Feature = "ResourceFinder"
)

var (
	LoadedFeatureGates = FeatureGates{
		ResourceFinder: false, // Alpha: default off
	}
)

// Enabled checks if a feature is enabled
func (f FeatureGates) Enabled(feature Feature) bool {
	enabled, exists := f[feature]
	if !exists {
		return false
	}
	return enabled
}

// Set parses a feature gate string like "Feature1=true,Feature2=false"
func (f FeatureGates) Set(value string) error {
	if value == "" {
		return nil
	}

	for _, s := range strings.Split(value, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid feature gate format: %s", s)
		}

		feature := Feature(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		if _, exists := f[feature]; !exists {
			return fmt.Errorf("unknown feature gate: %s", feature)
		}

		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for %s: %s (must be true or false)", feature, value)
		}

		f[feature] = boolValue
	}

	return nil
}

// String returns a string representation of loaded features
func (f FeatureGates) String() string {
	var loaded = []string{}
	for feature, boolValue := range f {
		value := strconv.FormatBool(boolValue)
		loaded = append(loaded, fmt.Sprintf("%s=%s", feature, value))
	}
	return strings.Join(loaded, ",")
}
