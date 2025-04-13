package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

func Sanitize(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])[:12]
}

func SoftSanitize(input string) string {
	const validPattern = `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	validRegex := regexp.MustCompile(validPattern)

	if validRegex.MatchString(input) {
		return input
	}

	allowedChars := regexp.MustCompile(`[^a-z0-9\.-]`)
	sanitized := allowedChars.ReplaceAllString(strings.ToLower(input), "-")
	return sanitized
}
