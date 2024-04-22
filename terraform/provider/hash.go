package provider

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
)

// Hash is a specially-formatted string representing a checksum of a package or the contents of the package.
type Hash string

func (hash Hash) String() string {
	return string(hash)
}

// HashScheme is an enumeration of schemes.
type HashScheme string

const (
	// HashSchemeZip is the scheme identifier for the legacy hash scheme that applies to distribution archives (.zip files) rather than package contents.
	HashSchemeZip HashScheme = HashScheme("zh:")
)

// New creates a new Hash value with the receiver as its scheme and the given raw string as its value.
func (scheme HashScheme) New(value string) Hash {
	return Hash(string(scheme) + value)
}

// PackageHashLegacyZipSHA implements the old provider package hashing scheme of taking a SHA256 hash of the containing .zip archive itself, rather than of the contents of the archive.
func PackageHashLegacyZipSHA(path string) (Hash, error) {
	archivePath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", errors.WithStackTrace(err)
	}

	gotHash := hash.Sum(nil)
	return HashSchemeZip.New(fmt.Sprintf("%x", gotHash)), nil
}

// HashLegacyZipSHAFromSHA is a convenience method to produce the schemed-string hash format from an already-calculated hash of a provider .zip archive.
func HashLegacyZipSHAFromSHA(sum [sha256.Size]byte) Hash {
	return HashSchemeZip.New(fmt.Sprintf("%x", sum[:]))
}
