package cas

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// gitSymlinkMode is the git tree entry mode for a symbolic link.
const gitSymlinkMode = "120000"

// DefaultLocalHashAlgorithm is used for content-addressed hashing of local source
// trees. It is chosen independently of any git repository's object format because
// local sources have no repository to inherit a format from.
const DefaultLocalHashAlgorithm = HashSHA256

// StoreLocalDirectory persists all content from a local source directory into the CAS
// and then links the persisted files to the target directory.
func (c *CAS) StoreLocalDirectory(
	ctx context.Context,
	l log.Logger,
	v Venv,
	sourceDir, targetDir string,
	opts ...LinkTreeOption,
) error {
	hash, treeData, err := c.buildLocalTree(v, sourceDir, DefaultLocalHashAlgorithm)
	if err != nil {
		return fmt.Errorf("failed to hash local directory %s: %w", sourceDir, err)
	}

	if err = c.storeFetchedContent(l, v, sourceDir, hash, treeData, DefaultLocalHashAlgorithm); err != nil {
		return fmt.Errorf("failed to store local content: %w", err)
	}

	tree, err := git.ParseTree(treeData, targetDir)
	if err != nil {
		return fmt.Errorf("failed to parse local tree: %w", err)
	}

	return LinkTree(ctx, v, c.blobStore, c.treeStore, tree, targetDir, opts...)
}

// ComputeLocalRootHash walks dir in deterministic (lexical) order and produces a
// content-addressed hash over (relpath, mode, file-content-hash) triples. The
// returned hash plays the same role as a git ref hash does in the remote flow:
// it is the "root" for DeterministicTreeHash calls when rewriting nested sources.
// The same file-content hashes are used both inside the root-hash and as blob
// hashes in the synthetic tree, so blob lookups and tree lookups stay consistent.
func (c *CAS) ComputeLocalRootHash(v Venv, dir string, alg HashAlgorithm) (string, error) {
	hash, _, err := c.buildLocalTree(v, dir, alg)
	return hash, err
}

// buildLocalTree walks dir and returns (rootHash, treeData). The treeData has
// the same "<mode> blob <hash>\t<path>\n" format as a git tree, but with
// file-content hashes taken in the chosen algorithm.
//
// Symlinks are preserved as 120000 entries whose blob hash is the hash of the
// link target string, matching git's symlink representation. Targets that
// escape dir are rejected at ingest time so the CAS cannot store a tree that
// would resolve outside the destination at materialize time.
func (c *CAS) buildLocalTree(v Venv, dir string, alg HashAlgorithm) (string, []byte, error) {
	var (
		treeData []byte
		rootBuf  []byte
	)

	err := vfs.WalkDir(v.FS, dir, func(path string, d fs.DirEntry, walkErr error) error {
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

		relPath, err := localRelPath(dir, path)
		if err != nil {
			return err
		}

		mode, blobHash, err := hashLocalEntry(v.FS, dir, path, info, alg)
		if err != nil {
			return err
		}

		if mode == "" {
			// Non-regular, non-symlink (device, fifo, socket); skip.
			return nil
		}

		treeData = append(treeData, fmt.Appendf(nil, "%s blob %s\t%s\n", mode, blobHash, relPath)...)
		rootBuf = append(rootBuf, fmt.Appendf(nil, "%s %s %s\n", relPath, mode, blobHash)...)

		return nil
	})
	if err != nil {
		return "", nil, err
	}

	return alg.Sum(rootBuf), treeData, nil
}

// localRelPath returns the git-style (forward-slash) relative path of path
// inside dir.
func localRelPath(dir, path string) (string, error) {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(rel, string(filepath.Separator), "/"), nil
}

// hashLocalEntry returns the git-style mode and content hash of a single
// directory entry. For regular files it hashes the bytes through alg; for
// symlinks it hashes the link target after [vfs.ValidateSymlinkTarget] checks
// it stays inside dir. mode is empty for entries that should be skipped
// entirely (devices, FIFOs, sockets).
func hashLocalEntry(
	fsys vfs.FS,
	dir, path string,
	info os.FileInfo,
	alg HashAlgorithm,
) (mode, hash string, err error) {
	switch {
	case info.Mode().IsRegular():
		fileHash, err := hashFileAlg(fsys, path, alg)
		if err != nil {
			return "", "", fmt.Errorf("failed to hash file %s: %w", path, err)
		}

		return fmt.Sprintf("%06o", info.Mode().Perm()), fileHash, nil

	case info.Mode()&os.ModeSymlink != 0:
		target, err := vfs.Readlink(fsys, path)
		if err != nil {
			return "", "", fmt.Errorf("read symlink %s: %w", path, err)
		}

		if err := vfs.ValidateSymlinkTarget(dir, path, target); err != nil {
			return "", "", err
		}

		return gitSymlinkMode, alg.Sum([]byte(target)), nil

	default:
		return "", "", nil
	}
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
