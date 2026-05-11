package cas

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter/v2"
)

// RemoteSourceDetectors is the go-getter detector chain CAS applies to
// shorthand remote sources (e.g. "github.com/org/repo") so they can be
// rewritten into URLs git understands. Used by ProcessStackComponent to
// canonicalize sources before resolving refs and dispatching to the
// central git store.
var RemoteSourceDetectors = []getter.Detector{
	new(getter.GitHubDetector),
	new(getter.GitDetector),
	new(getter.BitBucketDetector),
	new(getter.GitLabDetector),
}

// DetectRemoteSource runs the shorthand-rewriting detectors against src and
// returns the rewritten URL. Sources already prefixed with "git::" are
// returned unchanged. If no detector recognizes src, it is returned as-is so
// the caller can decide whether to treat that as an error.
func DetectRemoteSource(src string) (string, error) {
	if strings.HasPrefix(src, "git::") {
		return src, nil
	}

	for _, d := range RemoteSourceDetectors {
		out, ok, err := d.Detect(src, "")
		if err != nil {
			return "", err
		}

		if ok {
			return out, nil
		}
	}

	return src, nil
}

// StackCASResult holds the results of CAS processing for a stack component.
type StackCASResult struct {
	// Cleanup removes the temporary directory when called.
	Cleanup func()
	// ContentDir is the path to the directory containing rewritten content to copy.
	ContentDir string
}

// ProcessStackComponent resolves update_source_with_cas references for a stack
// component, rewriting nested sources to cas:: references and populating the
// CAS store with the referenced blobs and synthetic trees.
//
// The source may be a remote URL with an optional //subdir and ?ref= query, or
// a local filesystem path (optionally with a //subdir). Remote sources are
// cloned into a temp directory; local sources are copied into a temp directory
// so rewrites do not mutate the caller's working tree. The kind should be
// "unit" or "stack".
func (c *CAS) ProcessStackComponent(
	ctx context.Context,
	l log.Logger,
	v Venv,
	source, kind string,
) (*StackCASResult, error) {
	repoURL, subdir := getter.SourceDirSubdir(source)

	if isLocalPath(v.FS, repoURL) {
		return c.processLocalStackComponent(ctx, l, v, repoURL, subdir)
	}

	detectedURL, err := DetectRemoteSource(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to detect source URL %q: %w", repoURL, err)
	}

	parsedURL, err := url.Parse(detectedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source URL %q: %w", detectedURL, err)
	}

	ref := parsedURL.Query().Get("ref")

	q := parsedURL.Query()
	q.Del("ref")
	parsedURL.RawQuery = q.Encode()

	cleanURL := strings.TrimPrefix(parsedURL.String(), "git::")

	// Stack processing derives deterministic CAS keys from refHash, so
	// abbreviated SHAs would produce keys that depend on the input
	// shape. Full SHAs are the supported form for commit refs in
	// stacks; CommitHash returns the user input as-is for the
	// commit-ref path, and the canonical hash for the symbolic-ref
	// path.
	resolved, err := c.resolveReference(ctx, v, cleanURL, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve reference %q: %w", ref, err)
	}

	refHash := resolved.CommitHash()

	tempDir, err := vfs.MkdirTemp(v.FS, "", "terragrunt-cas-stack-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	cleanup := func() {
		if rmErr := v.FS.RemoveAll(tempDir); rmErr != nil {
			l.Warnf("cleanup error for %s: %v", tempDir, rmErr)
		}
	}

	cloneDir := filepath.Join(tempDir, "repo")

	if err := c.Clone(ctx, l, v, &CloneOptions{
		Dir:    cloneDir,
		Branch: ref,
		Depth:  c.cloneDepth,
	}, cleanURL); err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to CAS clone %q: %w", cleanURL, err)
	}

	hashAlg, err := detectRepoHashAlgorithm(ctx, v.Git, cloneDir)
	if err != nil {
		l.Debugf("Failed to detect object format, defaulting to SHA-1: %v", err)

		hashAlg = HashSHA1
	}

	contentDir := cloneDir

	if subdir != "" {
		if filepath.IsAbs(subdir) {
			cleanup()

			return nil, fmt.Errorf("%w: %q", ErrSourceEscapesRepo, subdir)
		}

		contentDir = filepath.Clean(filepath.Join(cloneDir, subdir))

		rel, err := filepath.Rel(cloneDir, contentDir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			cleanup()

			return nil, fmt.Errorf("%w: %q", ErrSourceEscapesRepo, subdir)
		}
	}

	if _, err := v.FS.Stat(contentDir); err != nil {
		cleanup()

		return nil, fmt.Errorf("subdir %q not found in cloned repo: %w", subdir, err)
	}

	if err := c.processDirectory(ctx, l, v, cloneDir, contentDir, refHash, hashAlg); err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to process directory for CAS: %w", err)
	}

	return &StackCASResult{
		ContentDir: contentDir,
		Cleanup:    cleanup,
	}, nil
}

// DeterministicTreeHash returns a deterministic tree hash derived from the
// given root hash and a path within the root. The root hash is a git commit
// hash for remote sources, or a content-addressed hash of the local tree for
// local sources; the algorithm is inferred from the root hash's length.
func DeterministicTreeHash(refHash, pathInRepo string) string {
	alg := DetectHashAlgorithm(refHash)

	return alg.Sum([]byte(refHash + pathInRepo))
}

// SplitSourceDoubleSlash splits a source string at the "//" separator and
// returns the base path and the subdirectory, if any.
//
// Examples:
//
//	"../..//modules/ec2" -> ("../..", "modules/ec2")
//	"../../modules/ec2"  -> ("../../modules/ec2", "")
func SplitSourceDoubleSlash(source string) (basePath, subdir string) {
	before, after, found := strings.Cut(source, "//")
	if !found {
		return source, ""
	}

	return before, after
}

// ResolveInRepoSource resolves an update_source_with_cas source string relative to
// dirPath and returns the cleaned absolute path. Absolute sources and sources
// whose resolved path escapes repoRoot via ".." segments are rejected so CAS
// materialization stays scoped to the cloned repository.
func ResolveInRepoSource(repoRoot, dirPath, source string) (string, error) {
	sourcePath, sourceSubdir := SplitSourceDoubleSlash(source)
	if filepath.IsAbs(sourcePath) {
		return "", fmt.Errorf("%w: %q", ErrAbsoluteSource, source)
	}

	resolved := filepath.Clean(filepath.Join(dirPath, sourcePath))
	if sourceSubdir != "" {
		resolved = filepath.Join(resolved, sourceSubdir)
	}

	rel, err := filepath.Rel(filepath.Clean(repoRoot), resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrSourceEscapesRepo, source)
	}

	return resolved, nil
}

// detectRepoHashAlgorithm queries the git object format of a cloned repository
// using the supplied runner so callers control the git/vexec binding.
func detectRepoHashAlgorithm(ctx context.Context, runner *git.GitRunner, repoDir string) (HashAlgorithm, error) {
	format, err := runner.WithWorkDir(repoDir).ObjectFormat(ctx)
	if err != nil {
		return "", err
	}

	return HashAlgorithm(format), nil
}

// processDirectory recursively processes a stack or unit directory, rewriting
// sources and creating synthetic CAS entries.
func (c *CAS) processDirectory(
	ctx context.Context, l log.Logger, v Venv,
	repoRoot, dirPath, refHash string, hashAlg HashAlgorithm,
) error {
	stackFile := filepath.Join(dirPath, "terragrunt.stack.hcl")
	unitFile := filepath.Join(dirPath, "terragrunt.hcl")

	if _, err := v.FS.Stat(stackFile); err == nil {
		return c.processStackFile(ctx, l, v, repoRoot, dirPath, stackFile, refHash, hashAlg)
	}

	if _, err := v.FS.Stat(unitFile); err == nil {
		return c.processUnitFile(l, v, repoRoot, dirPath, unitFile, refHash, hashAlg)
	}

	return nil
}

// processStackFile processes a terragrunt.stack.hcl file, rewriting sources
// for blocks that have update_source_with_cas = true.
func (c *CAS) processStackFile(
	ctx context.Context, l log.Logger, v Venv,
	repoRoot, dirPath, stackFile, refHash string, hashAlg HashAlgorithm,
) error {
	content, err := vfs.ReadFile(v.FS, stackFile)
	if err != nil {
		return fmt.Errorf("failed to read stack file %s: %w", stackFile, err)
	}

	blocks, err := ReadStackBlocks(content)
	if err != nil {
		return fmt.Errorf("failed to parse stack file %s: %w", stackFile, err)
	}

	for _, block := range blocks {
		if !block.UpdateSourceWithCAS {
			continue
		}

		l.Debugf("Processing CAS source rewrite for %s %q with source %q", block.BlockType, block.Name, block.Source)

		targetDir, err := ResolveInRepoSource(repoRoot, dirPath, block.Source)
		if err != nil {
			return fmt.Errorf("failed to resolve source for %s %q: %w", block.BlockType, block.Name, err)
		}

		if err := c.processDirectory(ctx, l, v, repoRoot, targetDir, refHash, hashAlg); err != nil {
			return fmt.Errorf("failed to process %s %q source: %w", block.BlockType, block.Name, err)
		}

		synthHash, err := c.buildSyntheticTree(l, v, targetDir, refHash, repoRoot, hashAlg)
		if err != nil {
			return fmt.Errorf("failed to build synthetic tree for %s %q: %w", block.BlockType, block.Name, err)
		}

		newSource := FormatCASRef(synthHash)

		content, err = RewriteStackBlockSource(content, block.BlockType, block.Name, newSource)
		if err != nil {
			return fmt.Errorf("failed to rewrite source for %s %q: %w", block.BlockType, block.Name, err)
		}

		l.Debugf("Rewrote %s %q source to %s", block.BlockType, block.Name, newSource)
	}

	// The file may be a read-only hard link from the CAS store, so remove it
	// before writing the rewritten content to avoid permission errors and to
	// avoid mutating the stored blob.
	if err := v.FS.Remove(stackFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove stack file before rewrite %s: %w", stackFile, err)
	}

	return vfs.WriteFile(v.FS, stackFile, content, RegularFilePerms)
}

// processUnitFile processes a terragrunt.hcl file, rewriting the
// terraform.source if update_source_with_cas is set.
func (c *CAS) processUnitFile(
	l log.Logger,
	v Venv,
	repoRoot, dirPath, unitFile, refHash string,
	hashAlg HashAlgorithm,
) error {
	content, err := vfs.ReadFile(v.FS, unitFile)
	if err != nil {
		return fmt.Errorf("failed to read unit file %s: %w", unitFile, err)
	}

	source, updateWithCAS, err := ReadTerraformSourceInfo(content)
	if err != nil {
		return fmt.Errorf("failed to parse unit file %s: %w", unitFile, err)
	}

	if !updateWithCAS || source == "" {
		return nil
	}

	l.Debugf("Processing CAS source rewrite for terraform source %q in %s", source, unitFile)

	moduleDir, err := ResolveInRepoSource(repoRoot, dirPath, source)
	if err != nil {
		return fmt.Errorf("failed to resolve terraform source %q: %w", source, err)
	}

	synthHash, err := c.buildSyntheticTree(l, v, moduleDir, refHash, repoRoot, hashAlg)
	if err != nil {
		return fmt.Errorf("failed to build synthetic tree for terraform source %q: %w", source, err)
	}

	newSource := FormatCASRef(synthHash)

	content, err = RewriteTerraformSource(content, newSource)
	if err != nil {
		return fmt.Errorf("failed to rewrite terraform source: %w", err)
	}

	l.Debugf("Rewrote terraform source to %s", newSource)

	// The file may be a read-only hard link from the CAS store, so remove it
	// before writing the rewritten content to avoid permission errors and to
	// avoid mutating the stored blob.
	if err := v.FS.Remove(unitFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unit file before rewrite %s: %w", unitFile, err)
	}

	return vfs.WriteFile(v.FS, unitFile, content, RegularFilePerms)
}

// buildSyntheticTree creates a synthetic CAS tree entry for a directory. It
// hashes every file, stores the blobs, and writes a tree object into the synth
// store. The resulting tree hash is deterministic: hashAlg(refHash + relPathInRepo).
//
// Symlinks are stored as 120000 entries whose blob is the link target string.
// [vfs.ValidateSymlinkTarget] rejects targets that escape dirPath, since the
// CAS protocol getter materializes synthetic trees into a self-contained
// destination directory and any escape would dangle.
func (c *CAS) buildSyntheticTree(
	l log.Logger, v Venv, dirPath, refHash, repoRoot string, hashAlg HashAlgorithm,
) (string, error) {
	var treeData []byte

	blobContent := NewContent(c.blobStore)

	err := vfs.WalkDir(v.FS, dirPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		relPath, err := localRelPath(dirPath, path)
		if err != nil {
			return err
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := vfs.Readlink(v.FS, path)
			if err != nil {
				return fmt.Errorf("read symlink %s: %w", path, err)
			}

			if err := vfs.ValidateSymlinkTarget(dirPath, path, target); err != nil {
				return err
			}

			blobHash := hashAlg.Sum([]byte(target))
			if err := blobContent.Ensure(l, v, blobHash, []byte(target)); err != nil {
				return fmt.Errorf("failed to store symlink blob %s: %w", path, err)
			}

			treeData = append(treeData, fmt.Appendf(nil, "%s blob %s\t%s\n", gitSymlinkMode, blobHash, relPath)...)

		case info.Mode().IsRegular():
			fileHash, err := hashFileAlg(v.FS, path, hashAlg)
			if err != nil {
				return fmt.Errorf("failed to hash file %s: %w", path, err)
			}

			if err := blobContent.EnsureCopy(l, v, fileHash, path); err != nil {
				return fmt.Errorf("failed to store blob %s: %w", path, err)
			}

			mode := gitTreeMode(info.Mode())
			treeData = append(treeData, fmt.Appendf(nil, "%s blob %s\t%s\n", mode, fileHash, relPath)...)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	relPathInRepo, err := filepath.Rel(repoRoot, dirPath)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path for deterministic hash: %w", err)
	}

	relPathInRepo = strings.ReplaceAll(relPathInRepo, string(filepath.Separator), "/")

	treeHash := hashAlg.Sum([]byte(refHash + relPathInRepo))

	synthContent := NewContent(c.synthStore)
	if err := synthContent.Ensure(l, v, treeHash, treeData); err != nil {
		return "", fmt.Errorf("failed to store synthetic tree: %w", err)
	}

	return treeHash, nil
}

// gitTreeMode returns the git tree-entry mode string for a file with the given
// filesystem mode. Directories are handled by the caller, so only the regular
// file, executable, and symlink cases are covered here.
func gitTreeMode(mode os.FileMode) string {
	switch {
	case mode&os.ModeSymlink != 0:
		return "120000"
	case mode&0o111 != 0:
		return "100755"
	default:
		return "100644"
	}
}

// isLocalPath reports whether source refers to an existing directory on fs.
// Remote URLs, go-getter forcers (git::), SSH shorthand (git@host:…), and
// non-directory paths all return false and fall through to the remote
// processing flow.
func isLocalPath(fs vfs.FS, source string) bool {
	if source == "" {
		return false
	}

	if strings.HasPrefix(source, "git::") {
		return false
	}

	// Filesystem absolute paths must be classified as local before any URL
	// parsing. On Windows, "C:\..." would otherwise be read as a URL with
	// scheme "C" and routed to the remote flow.
	if filepath.IsAbs(source) {
		return true
	}

	// SSH shorthand like git@github.com:owner/repo.git has no scheme but is not local.
	if strings.Contains(source, "@") && strings.Contains(source, ":") {
		return false
	}

	if u, err := url.Parse(source); err == nil && u.Scheme != "" {
		return false
	}

	info, err := fs.Stat(source)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// processLocalStackComponent is the local-path analogue of the remote clone
// flow. It copies the source tree into a temp directory so rewrites do not
// mutate the caller's working tree, computes a content-addressed root hash,
// and dispatches through the same processDirectory pipeline as the remote case.
func (c *CAS) processLocalStackComponent(
	ctx context.Context, l log.Logger, v Venv, sourceDir, subdir string,
) (*StackCASResult, error) {
	absSource, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local source %q: %w", sourceDir, err)
	}

	info, err := v.FS.Stat(absSource)
	if err != nil {
		return nil, fmt.Errorf("failed to stat local source %q: %w", absSource, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrNotADirectory, absSource)
	}

	tempDir, err := vfs.MkdirTemp(v.FS, "", "terragrunt-cas-stack-local-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	cleanup := func() {
		if rmErr := v.FS.RemoveAll(tempDir); rmErr != nil {
			l.Warnf("cleanup error for %s: %v", tempDir, rmErr)
		}
	}

	repoRoot := filepath.Join(tempDir, "repo")

	if err := c.copyTree(v, absSource, repoRoot); err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to copy local source into temp dir: %w", err)
	}

	contentDir := repoRoot

	if subdir != "" {
		if filepath.IsAbs(subdir) {
			cleanup()

			return nil, fmt.Errorf("%w: %q", ErrSourceEscapesRepo, subdir)
		}

		contentDir = filepath.Clean(filepath.Join(repoRoot, subdir))

		rel, err := filepath.Rel(repoRoot, contentDir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			cleanup()

			return nil, fmt.Errorf("%w: %q", ErrSourceEscapesRepo, subdir)
		}
	}

	if _, err := v.FS.Stat(contentDir); err != nil {
		cleanup()

		return nil, fmt.Errorf("subdir %q not found in local source: %w", subdir, err)
	}

	rootHash, err := c.ComputeLocalRootHash(v, repoRoot, DefaultLocalHashAlgorithm)
	if err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to compute local root hash: %w", err)
	}

	if err := c.processDirectory(ctx, l, v, repoRoot, contentDir, rootHash, DefaultLocalHashAlgorithm); err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to process local source for CAS: %w", err)
	}

	return &StackCASResult{
		ContentDir: contentDir,
		Cleanup:    cleanup,
	}, nil
}

// copyTree copies the directory tree rooted at src into dst using v.FS for all
// reads and writes, preserving file permissions. Regular files, directories,
// and symlinks are copied; other special files are skipped.
func (c *CAS) copyTree(v Venv, src, dst string) error {
	return vfs.WalkDir(v.FS, src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			return v.FS.MkdirAll(target, DefaultDirPerms)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := vfs.Readlink(v.FS, path)
			if err != nil {
				return err
			}

			if err := vfs.ValidateSymlinkTarget(src, path, linkTarget); err != nil {
				return fmt.Errorf("%w: %w", ErrSourceEscapesRepo, err)
			}

			if err := v.FS.MkdirAll(filepath.Dir(target), DefaultDirPerms); err != nil {
				return err
			}

			return vfs.Symlink(v.FS, linkTarget, target)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		return c.copyFileInFS(v, path, target, info.Mode().Perm())
	})
}

// copyFileInFS copies a single regular file from srcPath to dstPath through
// v.FS, creating any missing parent directories with DefaultDirPerms.
func (c *CAS) copyFileInFS(v Venv, srcPath, dstPath string, perm fs.FileMode) error {
	if err := v.FS.MkdirAll(filepath.Dir(dstPath), DefaultDirPerms); err != nil {
		return err
	}

	in, err := v.FS.Open(srcPath)
	if err != nil {
		return err
	}

	out, err := v.FS.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		_ = in.Close()

		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = in.Close()
		_ = out.Close()

		return err
	}

	if err := in.Close(); err != nil {
		_ = out.Close()

		return err
	}

	return out.Close()
}
