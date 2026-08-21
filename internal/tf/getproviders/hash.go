package getproviders

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"golang.org/x/mod/sumdb/dirhash"
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
func PackageHashLegacyZipSHA(fsys vfs.FS, path string) (Hash, error) {
	archivePath, err := vfs.EvalSymlinks(fsys, path)
	if err != nil {
		return "", err
	}

	gotHash, err := vfs.FileSHA256(fsys, archivePath)
	if err != nil {
		return "", err
	}

	return HashSchemeZip.New(hex.EncodeToString(gotHash)), nil
}

// HashLegacyZipSHAFromSHA is a convenience method to produce the schemed-string hash format from an already-calculated hash of a provider .zip archive.
func HashLegacyZipSHAFromSHA(sum [sha256.Size]byte) Hash {
	return HashSchemeZip.New(hex.EncodeToString(sum[:]))
}

// PackageHashV1 computes a hash of the contents of the package at the given location using hash algorithm 1. The resulting Hash is guaranteed to have the scheme HashScheme1.
func PackageHashV1(fsys vfs.FS, path string) (Hash, error) {
	// We'll first dereference a possible symlink at our PackageDir location, as would be created if this package were linked in from another cache.
	packageDir, err := vfs.EvalSymlinks(fsys, path)
	if err != nil {
		return "", err
	}

	if fileInfo, err := fsys.Stat(packageDir); err != nil {
		return "", err
	} else if !fileInfo.IsDir() {
		return "", fmt.Errorf("packageDir is not a directory %q", packageDir)
	}

	s, err := hashDir(fsys, packageDir)

	return Hash(s), err
}

// hashDir reproduces [dirhash.HashDir] over fsys. The hash itself still comes
// from [dirhash.Hash1], which is pure once it is handed the file list and a way
// to open each entry, so only the directory walk and the opens change.
func hashDir(fsys vfs.FS, dir string) (string, error) {
	dir = filepath.Clean(dir)

	var files []string

	err := vfs.WalkDir(fsys, dir, func(entry string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if entry == dir {
			return fmt.Errorf("%s is not a directory", dir)
		}

		rel := entry
		if dir != "." {
			rel = entry[len(dir)+1:]
		}

		files = append(files, filepath.ToSlash(rel))

		return nil
	})
	if err != nil {
		return "", err
	}

	return dirhash.Hash1(files, func(name string) (io.ReadCloser, error) {
		return fsys.Open(filepath.Join(dir, name))
	})
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
