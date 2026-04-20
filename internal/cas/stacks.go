package cas

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter/v2"
)

// StackCASResult holds the results of CAS processing for a stack component.
type StackCASResult struct {
	// Cleanup removes the temporary directory when called.
	Cleanup func()
	// ContentDir is the path to the directory containing rewritten content to copy.
	ContentDir string
}

// ProcessStackComponent clones a remote source via CAS, resolves update_source_with_cas
// references, rewrites sources to CAS references, and creates synthetic CAS entries.
//
// The source should be a remote URL with an optional //subdir and ?ref= query.
// The kind should be "unit" or "stack".
func (c *CAS) ProcessStackComponent(ctx context.Context, l log.Logger, source, kind string) (*StackCASResult, error) {
	repoURL, subdir := getter.SourceDirSubdir(source)

	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source URL %q: %w", repoURL, err)
	}

	ref := parsedURL.Query().Get("ref")

	// Remove ref from query so we can clone
	q := parsedURL.Query()
	q.Del("ref")
	parsedURL.RawQuery = q.Encode()

	cleanURL := strings.TrimPrefix(parsedURL.String(), "git::")

	// Resolve ref to commit hash
	refHash, err := c.resolveReference(ctx, cleanURL, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve reference %q: %w", ref, err)
	}

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

// detectRepoHashAlgorithm queries the git object format of a cloned repository.
func detectRepoHashAlgorithm(ctx context.Context, repoDir string) (HashAlgorithm, error) {
	g, err := git.NewGitRunner()
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

// processDirectory recursively processes a stack or unit directory,
// rewriting sources and creating synthetic CAS entries.
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

// processStackFile processes a terragrunt.stack.hcl file, rewriting sources for blocks
// that have update_source_with_cas = true.
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

// processUnitFile processes a terragrunt.hcl file, rewriting the terraform.source
// if update_source_with_cas is set.
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

// buildSyntheticTree creates a synthetic CAS tree entry for a directory.
// It hashes all files, stores blobs, and creates a tree entry in the synth store.
// The tree hash is deterministic: SHA1(refHash + relPathInRepo).
func (c *CAS) buildSyntheticTree(
	l log.Logger, dirPath, refHash, repoRoot string, hashAlg HashAlgorithm,
) (string, error) {
	var treeData []byte

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		// Convert to forward slashes for consistency (git-style paths)
		relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		fileHash, err := hashFile(c.fs, path)
		if err != nil {
			return fmt.Errorf("failed to hash file %s: %w", path, err)
		}

		// Store the blob
		blobContent := NewContent(c.blobStore)
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

	// Compute deterministic hash: SHA1(refHash + relPathInRepo)
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

// DeterministicTreeHash generates a deterministic hash for a synthetic tree entry
// by combining the resolved git ref hash with the path within the repository.
// The hash algorithm is detected from the refHash length to match the repository's object format.
func DeterministicTreeHash(refHash, pathInRepo string) string {
	alg := DetectHashAlgorithm(refHash)

	return alg.Sum([]byte(refHash + pathInRepo))
}

// SplitSourceDoubleSlash splits a source string at the // separator.
// Returns the base path and the subdirectory (if any).
// Example: "../..//modules/ec2" -> ("../../", "modules/ec2")
// Example: "../../modules/ec2"  -> ("../../modules/ec2", "")
func SplitSourceDoubleSlash(source string) (basePath, subdir string) {
	idx := strings.Index(source, "//")
	if idx == -1 {
		return source, ""
	}

	return source[:idx], source[idx+2:]
}
