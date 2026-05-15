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
	"github.com/gruntwork-io/terragrunt/internal/vexec"
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
func (c *CAS) ProcessStackComponent(ctx context.Context, l log.Logger, source, kind string) (*StackCASResult, error) {
	repoURL, subdir := getter.SourceDirSubdir(source)

	if isLocalPath(repoURL) {
		return c.processLocalStackComponent(ctx, l, repoURL, subdir)
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

	// Remove ref from query so we can clone
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
	resolved, err := c.resolveReference(ctx, cleanURL, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve reference %q: %w", ref, err)
	}

	refHash := resolved.CommitHash()

	// Create temp dir for the clone
	tempDir, err := os.MkdirTemp("", "terragrunt-cas-stack-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	cloneDir := filepath.Join(tempDir, "repo")

	// Clone the repo via CAS
	if err := c.Clone(ctx, l, &CloneOptions{
		Dir:    cloneDir,
		Branch: ref,
		Depth:  c.cloneDepth,
	}, cleanURL); err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to CAS clone %q: %w", cleanURL, err)
	}

	// Detect the repository's hash algorithm from the cloned content.
	hashAlg, err := detectRepoHashAlgorithm(ctx, cloneDir)
	if err != nil {
		l.Debugf("Failed to detect object format, defaulting to SHA-1: %v", err)

		hashAlg = HashSHA1
	}

	// Navigate to the subdir within the cloned repo
	contentDir := cloneDir
	if subdir != "" {
		contentDir = filepath.Join(cloneDir, subdir)
	}

	if _, err := os.Stat(contentDir); err != nil {
		cleanup()

		return nil, fmt.Errorf("subdir %q not found in cloned repo: %w", subdir, err)
	}

	// Process the directory: rewrite sources, create synthetic CAS entries
	if err := c.processDirectory(ctx, l, cloneDir, contentDir, refHash, hashAlg); err != nil {
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

// detectRepoHashAlgorithm queries the git object format of a cloned repository.
func detectRepoHashAlgorithm(ctx context.Context, repoDir string) (HashAlgorithm, error) {
	g, err := git.NewGitRunner(vexec.NewOSExec())
	if err != nil {
		return "", fmt.Errorf("failed to create git runner: %w", err)
	}

	g.WorkDir = repoDir

	format, err := g.ObjectFormat(ctx)
	if err != nil {
		return "", err
	}

	return HashAlgorithm(format), nil
}

// processDirectory recursively processes a stack or unit directory, rewriting
// sources and creating synthetic CAS entries.
func (c *CAS) processDirectory(
	ctx context.Context, l log.Logger,
	repoRoot, dirPath, refHash string, hashAlg HashAlgorithm,
) error {
	stackFile := filepath.Join(dirPath, "terragrunt.stack.hcl")
	unitFile := filepath.Join(dirPath, "terragrunt.hcl")

	if _, err := os.Stat(stackFile); err == nil {
		return c.processStackFile(ctx, l, repoRoot, dirPath, stackFile, refHash, hashAlg)
	}

	if _, err := os.Stat(unitFile); err == nil {
		return c.processUnitFile(l, repoRoot, dirPath, unitFile, refHash, hashAlg)
	}

	return nil
}

// processStackFile processes a terragrunt.stack.hcl file, rewriting sources
// for blocks that have update_source_with_cas = true.
func (c *CAS) processStackFile(
	ctx context.Context, l log.Logger,
	repoRoot, dirPath, stackFile, refHash string, hashAlg HashAlgorithm,
) error {
	content, err := os.ReadFile(stackFile)
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

		if err := c.processDirectory(ctx, l, repoRoot, targetDir, refHash, hashAlg); err != nil {
			return fmt.Errorf("failed to process %s %q source: %w", block.BlockType, block.Name, err)
		}

		// Build a synthetic tree for the target directory
		synthHash, err := c.buildSyntheticTree(l, targetDir, refHash, repoRoot, hashAlg)
		if err != nil {
			return fmt.Errorf("failed to build synthetic tree for %s %q: %w", block.BlockType, block.Name, err)
		}

		// Rewrite the source in the stack file
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
	if err := os.Remove(stackFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove stack file before rewrite %s: %w", stackFile, err)
	}

	return os.WriteFile(stackFile, content, RegularFilePerms)
}

// processUnitFile processes a terragrunt.hcl file, rewriting the
// terraform.source if update_source_with_cas is set.
func (c *CAS) processUnitFile(l log.Logger, repoRoot, dirPath, unitFile, refHash string, hashAlg HashAlgorithm) error {
	content, err := os.ReadFile(unitFile)
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

	synthHash, err := c.buildSyntheticTree(l, moduleDir, refHash, repoRoot, hashAlg)
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
	if err := os.Remove(unitFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unit file before rewrite %s: %w", unitFile, err)
	}

	return os.WriteFile(unitFile, content, RegularFilePerms)
}

// buildSyntheticTree creates a synthetic CAS tree entry for a directory. It
// hashes every file, stores the blobs, and writes a tree object into the synth
// store. The resulting tree hash is deterministic: hashAlg(refHash + relPathInRepo).
func (c *CAS) buildSyntheticTree(
	l log.Logger, dirPath, refHash, repoRoot string, hashAlg HashAlgorithm,
) (string, error) {
	var treeData []byte

	blobContent := NewContent(c.blobStore)

	err := vfs.WalkDir(c.fs, dirPath, func(path string, d fs.DirEntry, walkErr error) error {
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

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		// Convert to forward slashes for consistency (git-style paths)
		relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		fileHash, err := hashFileAlg(c.fs, path, hashAlg)
		if err != nil {
			return fmt.Errorf("failed to hash file %s: %w", path, err)
		}

		if err := blobContent.EnsureCopy(l, fileHash, path); err != nil {
			return fmt.Errorf("failed to store blob %s: %w", path, err)
		}

		mode := gitTreeMode(info.Mode())
		treeLine := fmt.Sprintf("%s blob %s\t%s\n", mode, fileHash, relPath)
		treeData = append(treeData, []byte(treeLine)...)

		return nil
	})
	if err != nil {
		return "", err
	}

	// Compute deterministic hash: hashAlg(refHash + relPathInRepo)
	relPathInRepo, err := filepath.Rel(repoRoot, dirPath)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path for deterministic hash: %w", err)
	}

	relPathInRepo = strings.ReplaceAll(relPathInRepo, string(filepath.Separator), "/")

	treeHash := hashAlg.Sum([]byte(refHash + relPathInRepo))

	// Store in synth tree store
	synthContent := NewContent(c.synthStore)
	if err := synthContent.Ensure(l, treeHash, treeData); err != nil {
		return "", fmt.Errorf("failed to store synthetic tree: %w", err)
	}

	return treeHash, nil
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

// isLocalPath reports whether source refers to an existing directory on the
// local filesystem. Remote URLs, go-getter forcers (git::), SSH shorthand
// (git@host:…), and non-directory paths all return false and fall through to
// the remote processing flow.
func isLocalPath(source string) bool {
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

	// SSH shorthand like git@github.com:owner/repo.git — no scheme but not local.
	if strings.Contains(source, "@") && strings.Contains(source, ":") {
		return false
	}

	if u, err := url.Parse(source); err == nil && u.Scheme != "" {
		return false
	}

	info, err := os.Stat(source)
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
	ctx context.Context, l log.Logger, sourceDir, subdir string,
) (*StackCASResult, error) {
	absSource, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local source %q: %w", sourceDir, err)
	}

	info, err := os.Stat(absSource)
	if err != nil {
		return nil, fmt.Errorf("failed to stat local source %q: %w", absSource, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrNotADirectory, absSource)
	}

	tempDir, err := os.MkdirTemp("", "terragrunt-cas-stack-local-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	repoRoot := filepath.Join(tempDir, "repo")

	if err := c.copyTree(absSource, repoRoot); err != nil {
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

	if _, err := os.Stat(contentDir); err != nil {
		cleanup()

		return nil, fmt.Errorf("subdir %q not found in local source: %w", subdir, err)
	}

	rootHash, err := c.ComputeLocalRootHash(repoRoot, DefaultLocalHashAlgorithm)
	if err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to compute local root hash: %w", err)
	}

	if err := c.processDirectory(ctx, l, repoRoot, contentDir, rootHash, DefaultLocalHashAlgorithm); err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to process local source for CAS: %w", err)
	}

	return &StackCASResult{
		ContentDir: contentDir,
		Cleanup:    cleanup,
	}, nil
}

// copyTree copies the directory tree rooted at src into dst using c.fs for all
// reads and writes, preserving file permissions. Regular files, directories,
// and symlinks are copied; other special files are skipped.
func (c *CAS) copyTree(src, dst string) error {
	return vfs.WalkDir(c.fs, src, func(path string, d fs.DirEntry, walkErr error) error {
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
			return c.fs.MkdirAll(target, DefaultDirPerms)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := vfs.Readlink(c.fs, path)
			if err != nil {
				return err
			}

			resolved := linkTarget
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			}

			resolved = filepath.Clean(resolved)

			rel, relErr := filepath.Rel(filepath.Clean(src), resolved)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("%w: symlink %q -> %q", ErrSourceEscapesRepo, path, linkTarget)
			}

			if err := c.fs.MkdirAll(filepath.Dir(target), DefaultDirPerms); err != nil {
				return err
			}

			return vfs.Symlink(c.fs, linkTarget, target)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		return c.copyFileInFS(path, target, info.Mode().Perm())
	})
}

// copyFileInFS copies a single regular file from srcPath to dstPath through
// c.fs, creating any missing parent directories with DefaultDirPerms.
func (c *CAS) copyFileInFS(srcPath, dstPath string, perm fs.FileMode) error {
	if err := c.fs.MkdirAll(filepath.Dir(dstPath), DefaultDirPerms); err != nil {
		return err
	}

	in, err := c.fs.Open(srcPath)
	if err != nil {
		return err
	}

	out, err := c.fs.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
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
