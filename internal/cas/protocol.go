package cas

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// CASProtocolPrefix is the go-getter prefix for CAS protocol references.
const CASProtocolPrefix = "cas::"

// HashAlgorithm identifies the hash algorithm used in a CAS reference.
type HashAlgorithm string

const (
	// HashSHA1 is the SHA-1 hash algorithm (40 hex characters).
	HashSHA1 HashAlgorithm = "sha1"
	// HashSHA256 is the SHA-256 hash algorithm (64 hex characters).
	HashSHA256 HashAlgorithm = "sha256"
)

// validAlgorithms are the hash algorithm prefixes accepted in CAS references.
var validAlgorithms = []HashAlgorithm{HashSHA1, HashSHA256}

// DetectHashAlgorithm detects the hash algorithm from a hex-encoded hash string.
func DetectHashAlgorithm(hexHash string) HashAlgorithm {
	const sha256HexLen = 64

	if len(hexHash) >= sha256HexLen {
		return HashSHA256
	}

	return HashSHA1
}

// NewHash returns a new hash.Hash for the algorithm.
func (a HashAlgorithm) NewHash() hash.Hash {
	if a == HashSHA256 {
		return sha256.New()
	}

	return sha1.New()
}

// Sum hashes data and returns the hex-encoded digest.
func (a HashAlgorithm) Sum(data []byte) string {
	h := a.NewHash()
	h.Write(data)

	return hex.EncodeToString(h.Sum(nil))
}

// ParseCASRef extracts the hash and algorithm from a CAS reference string.
// Expected format: "<algorithm>:<hash>" (after cas:: prefix has been stripped by Detect).
// Accepted algorithms: "sha1", "sha256".
func ParseCASRef(ref string) (string, error) {
	for _, alg := range validAlgorithms {
		prefix := string(alg) + ":"

		after, ok := strings.CutPrefix(ref, prefix)
		if !ok {
			continue
		}

		if after == "" {
			return "", &WrappedError{
				Op:      "parse_cas_ref",
				Context: ref,
				Err:     ErrCASRefEmptyHash,
			}
		}

		return after, nil
	}

	return "", &WrappedError{
		Op:      "parse_cas_ref",
		Context: ref,
		Err:     ErrCASRefMissingPrefix,
	}
}

// FormatCASRef formats a hash into a full CAS reference string, e.g. "cas::sha1:<hash>".
// The algorithm is detected from the hash length.
func FormatCASRef(hash string) string {
	alg := DetectHashAlgorithm(hash)

	return CASProtocolPrefix + string(alg) + ":" + hash
}

// FormatCASRefWithSubdir formats a hash and subdirectory into a CAS reference,
// e.g. "cas::sha1:<hash>//subdir". The algorithm is detected from the hash length.
func FormatCASRefWithSubdir(hash, subdir string) string {
	return FormatCASRef(hash) + "//" + subdir
}

// MaterializeTree reads a tree from the CAS store and links its contents to the destination directory.
// It tries the synth store first, then falls back to the git tree store.
func (c *CAS) MaterializeTree(
	ctx context.Context,
	l log.Logger,
	hash string,
	dest string,
	opts ...LinkTreeOption,
) error {
	var treeData []byte

	var treeStoreUsed *Store

	// Try synth store first (synthetic trees from stack CAS processing).
	synthContent := NewContent(c.synthStore)

	data, err := synthContent.Read(hash)
	if err == nil {
		treeData = data
		treeStoreUsed = c.synthStore
	}

	// Fall back to main tree store (git-derived trees).
	if treeData == nil {
		treeContent := NewContent(c.treeStore)

		data, err = treeContent.Read(hash)
		if err != nil {
			return &WrappedError{
				Op:   "materialize_tree",
				Path: hash,
				Err:  ErrTreeNotFound,
			}
		}

		treeData = data
		treeStoreUsed = c.treeStore
	}

	tree, err := git.ParseTree(treeData, dest)
	if err != nil {
		return fmt.Errorf("failed to parse CAS tree %s: %w", hash, err)
	}

	return LinkTree(ctx, c.blobStore, treeStoreUsed, tree, dest, opts...)
}
