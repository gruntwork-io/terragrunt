package util

import (
	"crypto/sha1"
	"encoding/base64"
)

// Returns the base 64 encoded sha1 hash of the given string
func EncodeBase64Sha1(str string) string {
	hash := sha1.Sum([]byte(str))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
