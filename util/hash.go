package util

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const (
	sha256InputSize = 32
)

// EncodeBase64Sha1 Returns the base 64 encoded sha1 hash of the given string
func EncodeBase64Sha1(str string) string {
	hash := sha1.Sum([]byte(str))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func GenerateRandomSha256() (string, error) {
	randomBytes := make([]byte, sha256InputSize)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", sha256.Sum256(randomBytes)), nil
}

func EncodeStringMap(stringMap *map[string]string, seed string) string {
	rollingHash := sha1.Sum([]byte(seed))

	for k, v := range *stringMap {
		rollingHash = sha1.Sum([]byte(fmt.Sprintf("%s:%s:%s", rollingHash, k, v)))
	}

	return string(rollingHash[:])
}
