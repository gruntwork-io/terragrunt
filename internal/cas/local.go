package cas

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DefaultLocalHashAlgorithm is used for content-addressed hashing of local source
// trees. It is chosen independently of any git repository's object format because
// local sources have no repository to inherit a format from.
const DefaultLocalHashAlgorithm = HashSHA256

// StoreLocalDirectory persists all content from a local source directory into the CAS
// and then links the persisted files to the target directory.
func (c *CAS) StoreLocalDirectory(ctx context.Context, l log.Logger, sourceDir, targetDir string) error {
	hash, treeData, err := c.buildLocalTree(sourceDir, DefaultLocalHashAlgorithm)
	if err != nil {
		return fmt.Errorf("failed to hash local directory %s: %w", sourceDir, err)
	}

	if err = c.storeLocalContent(l, sourceDir, hash, treeData, DefaultLocalHashAlgorithm); err != nil {
		return fmt.Errorf("failed to store local content: %w", err)
	}

	tree, err := git.ParseTree(treeData, targetDir)
	if err != nil {
		return fmt.Errorf("failed to parse local tree: %w", err)
	}

	return LinkTree(ctx, c.blobStore, c.treeStore, tree, targetDir)
}

// ComputeLocalRootHash walks dir in deterministic (lexical) order and produces a
// content-addressed hash over (relpath, mode, file-content-hash) triples. The
// returned hash plays the same role as a git ref hash does in the remote flow —
// it is the "root" for DeterministicTreeHash calls when rewriting nested sources.
// The same file-content hashes are used both inside the root-hash and as blob
// hashes in the synthetic tree, so blob lookups and tree lookups stay consistent.
func (c *CAS) ComputeLocalRootHash(dir string, alg HashAlgorithm) (string, error) {
	hash, _, err := c.buildLocalTree(dir, alg)
	return hash, err
}

// buildLocalTree walks dir and returns (rootHash, treeData). The treeData has
// the same "<mode> blob <hash>\t<path>\n" format as a git tree, but with
// file-content hashes taken in the chosen algorithm.
func (c *CAS) buildLocalTree(dir string, alg HashAlgorithm) (string, []byte, error) {
	var (
		treeData []byte
		rootBuf  []byte
	)

	err := vfs.WalkDir(c.fs, dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to stat file %s: %w", path, err)
		}

		// Skip symlinks and other non-regular entries to keep the synthetic
		// tree consistent with copyTree, which only copies regular files.
		if !info.Mode().IsRegular() {
			return nil
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Git-style forward slashes in tree entries, regardless of host OS.
		relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		fileHash, err := hashFileAlg(c.fs, path, alg)
		if err != nil {
			return fmt.Errorf("failed to hash file %s: %w", path, err)
		}

		mode := fmt.Sprintf("%06o", info.Mode().Perm())
		treeData = append(treeData, fmt.Appendf(nil, "%s blob %s\t%s\n", mode, fileHash, relPath)...)

		// Root hash input includes path, mode, and content hash so that two
		// trees with identical files at different relative paths (or different
		// permissions) get distinct root hashes.
		rootBuf = append(rootBuf, fmt.Appendf(nil, "%s %s %s\n", relPath, mode, fileHash)...)

		return nil
	})
	if err != nil {
		return "", nil, err
	}

	return alg.Sum(rootBuf), treeData, nil
}

// storeLocalContent stores the tree object and every blob referenced by it.
func (c *CAS) storeLocalContent(l log.Logger, sourceDir, dirHash string, treeData []byte, alg HashAlgorithm) error {
	treeContent := NewContent(c.treeStore)
	if err := treeContent.Ensure(l, dirHash, treeData); err != nil {
		return fmt.Errorf("failed to store tree data: %w", err)
	}

	blobContent := NewContent(c.blobStore)

	return vfs.WalkDir(c.fs, sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to stat file %s: %w", path, err)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		fileHash, err := hashFileAlg(c.fs, path, alg)
		if err != nil {
			return fmt.Errorf("failed to hash file %s: %w", path, err)
		}

		if err := blobContent.EnsureCopy(l, fileHash, path); err != nil {
			return fmt.Errorf("failed to store file %s: %w", path, err)
		}

		return nil
	})
}

// hashFileAlg hashes a file's contents using the given algorithm and returns
// the hex-encoded digest. It exists alongside hashFile (which is hard-coded
// to SHA-1 for the git remote flow) so the local flow can use SHA-256.
func hashFileAlg(fsys vfs.FS, path string, alg HashAlgorithm) (string, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return "", err
	}

	h := alg.NewHash()

	if _, err := io.Copy(h, file); err != nil {
		_ = file.Close()

		return "", err
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("closing %s after hashing failed: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
