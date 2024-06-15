package getproviders

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/rogpeppe/go-internal/dirhash"
)

// Hash is a specially-formatted string representing a checksum of a package or the contents of the package.
type Hash string

func (hash Hash) String() string {
	return string(hash)
}

// HashScheme is an enumeration of schemes.
type HashScheme string

const (
	// HashScheme1 is the scheme identifier for the first hash scheme.
	HashScheme1 HashScheme = HashScheme("h1:")

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

// PackageHashV1 computes a hash of the contents of the package at the given location using hash algorithm 1. The resulting Hash is guaranteed to have the scheme HashScheme1.
func PackageHashV1(path string) (Hash, error) {
	// We'll first dereference a possible symlink at our PackageDir location, as would be created if this package were linked in from another cache.
	packageDir, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}

	if fileInfo, err := os.Stat(packageDir); err != nil {
		return "", errors.WithStackTrace(err)
	} else if !fileInfo.IsDir() {
		return "", errors.Errorf("packageDir is not a directory %q", packageDir)
	}

	s, err := dirhash.HashDir(packageDir, "", dirhash.Hash1)
	return Hash(s), err
}

func DocumentHashes(doc []byte) []Hash {
	var hashes []Hash

	sc := bufio.NewScanner(bytes.NewReader(doc))
	for sc.Scan() {
		parts := bytes.Fields(sc.Bytes())
		columns := 2
		if len(parts) != columns {
			// Doesn't look like a valid sums file line, so we'll assume this whole thing isn't a checksums file.
			continue
		}

		// If this is a checksums file then the first part should be a hex-encoded SHA256 hash, so it should be 64 characters long and contain only hex digits.
		hashStr := parts[0]
		hashLen := 64
		if len(hashStr) != hashLen {
			return nil // doesn't look like a checksums file
		}

		var gotSHA256Sum [sha256.Size]byte
		if _, err := hex.Decode(gotSHA256Sum[:], hashStr); err != nil {
			return nil // doesn't look like a checksums file
		}

		hashes = append(hashes, HashLegacyZipSHAFromSHA(gotSHA256Sum))
	}

	return hashes
}
