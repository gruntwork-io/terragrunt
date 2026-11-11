package cas

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-getter/v2"

	"github.com/gruntwork-io/terragrunt/internal/detect"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/redact"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RemoteSourceDetectors is the go-getter detector chain CAS applies to
// shorthand remote sources (e.g. "github.com/org/repo") so they can be
// rewritten into URLs git understands. Used by ProcessStackComponent to
// canonicalize sources before resolving refs and dispatching to the
// central git store.
var RemoteSourceDetectors = []getter.Detector{
	new(getter.GitHubDetector),
	new(getter.GitDetector),
	new(detect.BitBucket),
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

// maxRewriteDepth bounds how deep update_source_with_cas sources may nest before the rewrite stops.
const maxRewriteDepth = 64

// StackCASResult holds the results of CAS processing for a stack component.
type StackCASResult struct {
	// Cleanup removes the temporary directory when called.
	Cleanup func()
	// ContentDir is the path to the directory containing rewritten content to copy.
	ContentDir string
	// SourcesInsideStack lists the rewritten blocks whose source lies inside the directory of
	// their stack file.
	SourcesInsideStack []SourceInsideStack
}

// SourceInsideStack is a unit or stack block with update_source_with_cas whose source lies
// inside the directory of the stack file that declares it.
//
// Stack generation copies that directory whole, so the generated stack contains the source
// twice: once as the cas:: reference, and once as a copy that `run --all` discovers as its own
// unit or stack.
type SourceInsideStack struct {
	// StackFile is the path of the stack file that declares the block, relative to the root of
	// the component source.
	StackFile string
	// BlockType is "unit" or "stack".
	BlockType string
	// Name is the label of the block.
	Name string
	// Source is the source of the block, as written.
	Source string
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
//
// Requires v.FS unconditionally. Remote sources additionally require
// v.Exec for the git runner; the assertion fires once dispatch picks
// the remote branch.
//
// Returns the [NewGitStoreVenv] error for a remote source when v cannot
// back the git store.
// Extra cloneOpts, such as [WithMutable], apply to the remote clone.
func (c *CAS) ProcessStackComponent(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	source, kind string,
	cloneOpts ...CloneOption,
) (*StackCASResult, error) {
	v.RequireFS()

	repoURL, subdir := getter.SourceDirSubdir(source)

	if isLocalPath(v.FS, repoURL) {
		return c.processLocalStackComponent(ctx, l, v, repoURL, subdir)
	}

	v.RequireExec()

	detectedURL, err := DetectRemoteSource(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to detect source URL %q: %w", redact.NewURL(repoURL), err)
	}

	parsedURL, err := url.Parse(detectedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source URL %q: %w", redact.NewURL(detectedURL), err)
	}

	strippedURL, params := StripGitURLParams(parsedURL)
	ref := params.Ref

	cleanURL := redact.NewURL(strings.TrimPrefix(strippedURL.String(), "git::"))

	gv, err := NewGitStoreVenv(v)
	if err != nil {
		return nil, err
	}

	// Stack processing derives deterministic CAS keys from refHash, so
	// abbreviated SHAs would produce keys that depend on the input
	// shape. Full SHAs are the supported form for commit refs in
	// stacks; CommitHash returns the user input as-is for the
	// commit-ref path, and the canonical hash for the symbolic-ref
	// path.
	resolved, err := c.resolveReference(ctx, l, gv, cleanURL, ref, params.SSHKey)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve reference %q: %w", ref, err)
	}

	refHash := resolved.CommitHash()

	tempDir, err := vfs.MkdirTemp(v.FS, "", "terragrunt-cas-stack-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	cleanup := func() {
		if rmErr := v.FS.RemoveAll(tempDir); rmErr != nil {
			l.Warnf("cleanup error for %s: %v", tempDir, rmErr)
		}
	}

	cloneDir := filepath.Join(tempDir, "repo")

	cloneOpts = append([]CloneOption{
		WithDir(cloneDir),
		WithBranch(ref),
		WithDepth(c.cloneDepth),
		WithSSHKey(params.SSHKey),
	}, cloneOpts...)

	if err := c.Clone(ctx, l, v, cleanURL, cloneOpts...); err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to CAS clone %q: %w", cleanURL, err)
	}

	hashAlg, err := detectRepoHashAlgorithm(ctx, v, cloneDir)
	if err != nil {
		l.Debugf("Failed to detect object format, defaulting to SHA-1: %v", err)

		hashAlg = HashSHA1
	}

	contentDir := cloneDir

	if subdir != "" {
		if filepath.IsAbs(subdir) {
			cleanup()

			return nil, fmt.Errorf("%w: %q", ErrAbsoluteSource, subdir)
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

	inside, err := c.processDirectory(ctx, l, v, cloneDir, contentDir, refHash, hashAlg, 0)
	if err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to process directory for CAS: %w", err)
	}

	return &StackCASResult{
		ContentDir:         contentDir,
		Cleanup:            cleanup,
		SourcesInsideStack: inside,
	}, nil
}

// ProcessStoredStackComponent rewrites, in place, the component at contentDir of root, a tree
// stored under treeKey, and returns the rewritten blocks whose source lies inside the directory of
// their stack file.
func (c *CAS) ProcessStoredStackComponent(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	root, contentDir, treeKey string,
) ([]SourceInsideStack, error) {
	v.RequireFS()

	inside, err := c.processDirectory(ctx, l, v, root, contentDir, treeKey, DefaultLocalHashAlgorithm, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to process stored tree %s for CAS: %w", treeKey, err)
	}

	return inside, nil
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
// through a runner derived from v.Exec, so callers control the vexec binding.
func detectRepoHashAlgorithm(
	ctx context.Context,
	v *venv.Venv,
	repoDir string,
) (HashAlgorithm, error) {
	runner, err := git.NewGitRunner(v)
	if err != nil {
		return "", err
	}

	format, err := runner.WithWorkDir(repoDir).ObjectFormat(ctx)
	if err != nil {
		return "", err
	}

	return HashAlgorithm(format), nil
}

// processDirectory recursively processes a stack or unit directory, rewriting
// sources and creating synthetic CAS entries. It returns the rewritten blocks,
// at any depth, whose source lies inside the directory of their stack file.
func (c *CAS) processDirectory(
	ctx context.Context, l log.Logger, v *venv.Venv,
	repoRoot, dirPath, refHash string, hashAlg HashAlgorithm, depth int,
) ([]SourceInsideStack, error) {
	if depth > maxRewriteDepth {
		return nil, fmt.Errorf("%w (%d levels): %s", ErrRewriteTooDeep, maxRewriteDepth, dirPath)
	}

	stackFile := filepath.Join(dirPath, "terragrunt.stack.hcl")
	unitFile := filepath.Join(dirPath, "terragrunt.hcl")

	if _, err := v.FS.Stat(stackFile); err == nil {
		return c.processStackFile(ctx, l, v, repoRoot, dirPath, stackFile, refHash, hashAlg, depth)
	}

	if _, err := v.FS.Stat(unitFile); err == nil {
		return nil, c.processUnitFile(l, v, repoRoot, dirPath, unitFile, refHash, hashAlg)
	}

	return nil, nil
}

// processStackFile processes a terragrunt.stack.hcl file, rewriting sources
// for blocks that have update_source_with_cas = true, including the blocks a
// stack block's autoinclude injects into its nested stack. It returns the
// rewritten blocks, here and in nested stacks, whose source lies inside the
// directory of their stack file.
func (c *CAS) processStackFile(
	ctx context.Context, l log.Logger, v *venv.Venv,
	repoRoot, dirPath, stackFile, refHash string, hashAlg HashAlgorithm, depth int,
) ([]SourceInsideStack, error) {
	content, err := vfs.ReadFile(v.FS, stackFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read stack file %s: %w", stackFile, err)
	}

	f, blocks, err := ReadStackBlocks(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stack file %s: %w", stackFile, err)
	}

	stackFileInRepo, err := filepath.Rel(repoRoot, stackFile)
	if err != nil {
		return nil, fmt.Errorf("failed to compute path of stack file %s in its source: %w", stackFile, err)
	}

	var inside []SourceInsideStack

	rewritten := false

	for _, block := range blocks {
		if !block.UpdateSourceWithCAS {
			continue
		}

		if strings.HasPrefix(block.Source, CASProtocolPrefix) {
			continue
		}

		l.Debugf(
			"Processing CAS source rewrite for %s %q with source %q",
			block.BlockType,
			block.Name,
			block.Source,
		)

		targetDir, err := ResolveInRepoSource(repoRoot, dirPath, block.Source)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: failed to resolve source for %s %q: %w",
				ErrRewriteIncomplete,
				block.BlockType,
				block.Name,
				err,
			)
		}

		nested, err := c.processDirectory(ctx, l, v, repoRoot, targetDir, refHash, hashAlg, depth+1)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: failed to process %s %q source: %w",
				ErrRewriteIncomplete,
				block.BlockType,
				block.Name,
				err,
			)
		}

		inside = append(inside, nested...)

		synthHash, err := c.buildSyntheticTree(l, v, targetDir, refHash, repoRoot, hashAlg)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: failed to build synthetic tree for %s %q: %w",
				ErrRewriteIncomplete,
				block.BlockType,
				block.Name,
				err,
			)
		}

		if util.HasPathPrefix(targetDir, dirPath) {
			inside = append(inside, SourceInsideStack{
				StackFile: filepath.ToSlash(stackFileInRepo),
				BlockType: block.BlockType,
				Name:      block.Name,
				Source:    block.Source,
			})
		}

		newSource := FormatCASRef(synthHash)

		block.SetSource(newSource)

		rewritten = true

		l.Debugf("Rewrote %s %q source to %s", block.BlockType, block.Name, newSource)
	}

	if err := replaceFile(v.FS, stackFile, f.Bytes()); err != nil {
		if rewritten {
			return nil, fmt.Errorf("%w: failed to rewrite stack file %s: %w", ErrRewriteIncomplete, stackFile, err)
		}

		return nil, fmt.Errorf("failed to rewrite stack file %s: %w", stackFile, err)
	}

	return inside, nil
}

// processUnitFile processes a terragrunt.hcl file, rewriting the
// terraform.source if update_source_with_cas is set.
//
// When the source uses the "//" subdir convention (e.g. "../..//modules/foo"),
// the synthetic tree is built from the resolved base path (everything before
// "//") and the rewritten CAS reference preserves the "//subdir" tail. This
// keeps sibling files reachable from the materialized working directory, so
// OpenTofu/Terraform modules that reference each other via relative paths
// (e.g. source = "../bar") resolve correctly. Sources without "//" produce a
// shallow tree of just the leaf module and a CAS reference with no subdir.
func (c *CAS) processUnitFile(
	l log.Logger,
	v *venv.Venv,
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

	if strings.HasPrefix(source, CASProtocolPrefix) {
		return nil
	}

	l.Debugf("Processing CAS source rewrite for terraform source %q in %s", source, unitFile)

	basePath, subdir := SplitSourceDoubleSlash(source)

	treeDir, err := ResolveInRepoSource(repoRoot, dirPath, basePath)
	if err != nil {
		return fmt.Errorf("%w: failed to resolve terraform source %q: %w", ErrRewriteIncomplete, source, err)
	}

	if subdir != "" {
		moduleDir := filepath.Clean(filepath.Join(treeDir, subdir))

		rel, relErr := filepath.Rel(treeDir, moduleDir)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: %w: %q", ErrRewriteIncomplete, ErrSourceEscapesRepo, source)
		}

		if _, err := v.FS.Stat(moduleDir); err != nil {
			return fmt.Errorf("%w: subdir %q not found under %s: %w", ErrRewriteIncomplete, subdir, treeDir, err)
		}
	}

	synthHash, err := c.buildSyntheticTree(l, v, treeDir, refHash, repoRoot, hashAlg)
	if err != nil {
		return fmt.Errorf("%w: failed to build synthetic tree for terraform source %q: %w", ErrRewriteIncomplete, source, err)
	}

	newSource := FormatCASRef(synthHash)
	if subdir != "" {
		newSource = FormatCASRefWithSubdir(synthHash, subdir)
	}

	content, err = RewriteTerraformSource(content, newSource)
	if err != nil {
		return fmt.Errorf("%w: failed to rewrite terraform source: %w", ErrRewriteIncomplete, err)
	}

	l.Debugf("Rewrote terraform source to %s", newSource)

	if err := replaceFile(v.FS, unitFile, content); err != nil {
		return fmt.Errorf("%w: failed to rewrite unit file %s: %w", ErrRewriteIncomplete, unitFile, err)
	}

	return nil
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
	l log.Logger, v *venv.Venv, dirPath, refHash, repoRoot string, hashAlg HashAlgorithm,
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
			if err := blobContent.Ensure(l, v, blobHash, []byte(target), StoredFilePerms); err != nil {
				return fmt.Errorf("failed to store symlink blob %s: %w", path, err)
			}

			treeData = append(
				treeData,
				fmt.Appendf(nil, "%s blob %s\t%s\n", gitSymlinkMode, blobHash, relPath)...)

		case info.Mode().IsRegular():
			fileHash, err := hashFileAlg(v.FS, path, hashAlg)
			if err != nil {
				return fmt.Errorf("failed to hash file %s: %w", path, err)
			}

			if err := blobContent.EnsureCopy(l, v, fileHash, path); err != nil {
				return fmt.Errorf("failed to store blob %s: %w", path, err)
			}

			mode := gitTreeMode(info.Mode())
			treeData = append(
				treeData,
				fmt.Appendf(nil, "%s blob %s\t%s\n", mode, fileHash, relPath)...)
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
	if err := synthContent.Ensure(l, v, treeHash, treeData, StoredFilePerms); err != nil {
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
func isLocalPath(fsys vfs.FS, source string) bool {
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

	info, err := fsys.Stat(source)
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
	ctx context.Context, l log.Logger, v *venv.Venv, sourceDir, subdir string,
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

	tempDir, err := vfs.MkdirTemp(v.FS, "", "terragrunt-cas-stack-local-")
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

			return nil, fmt.Errorf("%w: %q", ErrAbsoluteSource, subdir)
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

	inside, err := c.processDirectory(
		ctx,
		l,
		v,
		repoRoot,
		contentDir,
		rootHash,
		DefaultLocalHashAlgorithm,
		0,
	)
	if err != nil {
		cleanup()

		return nil, fmt.Errorf("failed to process local source for CAS: %w", err)
	}

	return &StackCASResult{
		ContentDir:         contentDir,
		Cleanup:            cleanup,
		SourcesInsideStack: inside,
	}, nil
}

// copyTree copies the directory tree rooted at src into dst using v.FS for all
// reads and writes, preserving file permissions. Regular files, directories,
// and symlinks are copied; other special files and [ignoredSourceDirs]
// are skipped.
func (c *CAS) copyTree(v *venv.Venv, src, dst string) error {
	return vfs.WalkDir(v.FS, src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if ignoredSourceEntry(src, path, d) {
			return dropIgnoredSourceEntry(d)
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
func (c *CAS) copyFileInFS(v *venv.Venv, srcPath, dstPath string, perm fs.FileMode) error {
	if err := v.FS.MkdirAll(filepath.Dir(dstPath), DefaultDirPerms); err != nil {
		return err
	}

	in, err := v.FS.Open(srcPath)
	if err != nil {
		return err
	}

	out, err := v.FS.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return errors.Join(err, in.Close())
	}

	if _, err := io.Copy(out, in); err != nil {
		return errors.Join(err, in.Close(), out.Close())
	}

	if err := in.Close(); err != nil {
		return errors.Join(err, out.Close())
	}

	return out.Close()
}

// replaceFile unlinks path and writes content with the mode of the replaced file, or [RegularFilePerms] for a new one.
func replaceFile(fsys vfs.FS, path string, content []byte) error {
	perm := RegularFilePerms

	info, err := fsys.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err == nil {
		perm = info.Mode().Perm()
	}

	if err := fsys.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return vfs.WriteFile(fsys, path, content, perm)
}
