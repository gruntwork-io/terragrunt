package util

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand"
)

// Returns the base 64 encoded sha1 hash of the given string
func EncodeBase64Sha1(str string) string {
	hash := sha1.Sum([]byte(str))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func GenerateRandomSha256() (string, error) {
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", sha256.Sum256(randomBytes)), nil
}
