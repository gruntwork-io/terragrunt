package cas

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// StoreLocalDirectory persists all content from a local source directory into the CAS
// and then links the persisted files to the target directory
func (c *CAS) StoreLocalDirectory(ctx context.Context, l log.Logger, sourceDir, targetDir string) error {
	// Generate a synthetic hash for the local directory based on its contents
	hash, treeData, err := c.hashDirectory(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to hash local directory %s: %w", sourceDir, err)
	}

	// Store all files from the directory into the CAS
	if err = c.storeLocalContent(l, sourceDir, hash, treeData); err != nil {
		return fmt.Errorf("failed to store local content: %w", err)
	}

	// Parse the tree data and link to target directory
	tree, err := git.ParseTree(treeData, targetDir)
	if err != nil {
		return fmt.Errorf("failed to parse local tree: %w", err)
	}

	return LinkTree(ctx, c.store, tree, targetDir)
}

// hashDirectory creates a synthetic hash and tree structure for a local directory
func (c *CAS) hashDirectory(sourceDir string) (string, []byte, error) {
	var treeData []byte

	var allHashes []string

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Implicitly handled by tracking the file hashes.
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Convert to forward slashes for consistency (git-style paths)
		relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		fileHash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("failed to hash file %s: %w", path, err)
		}

		// Artificially create a tree entry for the file.
		mode := fmt.Sprintf("%06o", info.Mode().Perm())
		treeLine := fmt.Sprintf("%s blob %s\t%s\n", mode, fileHash, relPath)
		treeData = append(treeData, []byte(treeLine)...)

		// Collect all hashes for directory hash calculation
		allHashes = append(allHashes, fileHash)

		return nil
	})
	if err != nil {
		return "", nil, err
	}

	// Create a synthetic hash for the entire directory based on all file hashes
	// This ensures the same directory contents always get the same hash
	dirHash := hashString(strings.Join(allHashes, ""))

	return dirHash, treeData, nil
}

// storeLocalContent stores all files from a local directory into the CAS
func (c *CAS) storeLocalContent(l log.Logger, sourceDir, dirHash string, treeData []byte) error {
	// First store the tree object itself
	content := NewContent(c.store)
	if err := content.Ensure(l, dirHash, treeData); err != nil {
		return fmt.Errorf("failed to store tree data: %w", err)
	}

	// Walk the directory and store all files
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and the root directory itself
		if info.IsDir() {
			return nil
		}

		// Hash the file to get its content hash
		fileHash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("failed to hash file %s: %w", path, err)
		}

		if err := content.EnsureCopy(l, fileHash, path); err != nil {
			return fmt.Errorf("failed to store file %s: %w", path, err)
		}

		return nil
	})
}

func hashString(s string) string {
	h := sha1.New()
	h.Write([]byte(s))

	return hex.EncodeToString(h.Sum(nil))
}
