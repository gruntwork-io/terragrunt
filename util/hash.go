package util

import (
	"crypto/sha1"
	"encoding/base64"
	"regexp"
)

// Returns the base 64 encoded sha1 hash of the given string
func EncodeBase64Sha1(str string) string {
	hash := sha1.Sum([]byte(str))
	return base64.URLEncoding.EncodeToString(hash[:])
}

var removeNonAlphaNumericCharsRegex = regexp.MustCompile("[^a-zA-Z0-9]+")

// Returns the base 64 encoded sha1 hash of the given string keeping only alphanumeric characters
func EncodeBase64Sha1AlphaNum(str string) string {
	return removeNonAlphaNumericCharsRegex.ReplaceAllString(EncodeBase64Sha1(str), "")
}
