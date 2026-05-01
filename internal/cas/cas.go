// Package cas implements a content-addressable storage for git content.
//
// Blobs are copied from cloned repositories to a local store, along with trees.
// When the same content is requested again, the content is read from the local store,
// avoiding the need to clone the repository or read from the network.
package cas

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DefaultCASCloneDepth is the default shallow clone depth for CAS (git clone --depth).
const DefaultCASCloneDepth = 1

// Option configures the behavior of CAS.
type Option func(*CAS)

// CloneOptions configures the behavior of a specific clone operation.
type CloneOptions struct {
	// Dir specifies the target directory for the clone.
	// If empty, uses the repository name.
	Dir string

	// Branch specifies which branch to clone.
	// If empty, uses HEAD.
	Branch string

	// IncludedGitFiles specifies the files to preserve from the .git directory.
	// If empty, does not preserve any files.
	IncludedGitFiles []string

	// Depth limits the clone history to the given number of commits passed to git clone --depth.
	// If zero, CAS falls back to its configured clone depth (default shallow depth 1).
	// Set to -1 for full history (Terragrunt omits --depth; git rejects --depth 0).
	Depth int

	// Mutable, when true, copies blobs into the target directory instead of
	// hardlinking them from the CAS store. The destination tree becomes safe
	// to mutate without corrupting the shared store.
	Mutable bool
}

// CAS clones a git repository using content-addressable storage.
type CAS struct {
	fs         vfs.FS
	blobStore  *Store
	treeStore  *Store
	synthStore *Store
	gitStore   *GitStore
	git        *git.GitRunner
	storePath  string
	cloneDepth int
}

// WithStorePath specifies a custom path for the content store.
// If not set, defaults to <user cache dir>/terragrunt/cas/store,
// where the user cache dir honors XDG_CACHE_HOME on Linux and the
// platform equivalents on macOS and Windows (see os.UserCacheDir).
func WithStorePath(path string) Option {
	return func(c *CAS) {
		c.storePath = path
	}
}

// WithCloneDepth sets git clone --depth for CAS (positive shallow clone; negative,
// e.g. -1, means full history with no --depth). Terragrunt validates user-supplied
// values with ValidateCASCloneDepth (zero is invalid for git). Omit this option to
// keep cloneDepth unset so per-operation CloneOptions.Depth can fall back to DefaultCASCloneDepth.
func WithCloneDepth(depth int) Option {
	return func(c *CAS) {
		c.cloneDepth = depth
	}
}

// WithFS specifies the filesystem for file operations.
// If not set, defaults to the real OS filesystem.
func WithFS(fs vfs.FS) Option {
	return func(c *CAS) {
		c.fs = fs
	}
}

// New creates a new CAS instance with the given options.
func New(opts ...Option) (*CAS, error) {
	c := &CAS{}

	for _, opt := range opts {
		opt(c)
	}

	if c.fs == nil {
		c.fs = vfs.NewOSFS()
	}

	// CAS shells out to git, which only sees the real disk. Validate
	// here so a non-OS backing fails at the constructor instead of
	// from a deeper store-init step.
	if !vfs.IsOSFS(c.fs) {
		return nil, ErrGitStoreFSNotOS
	}

	if c.storePath == "" {
		cacheDir, err := util.EnsureCacheDir()
		if err != nil {
			return nil, err
		}

		c.storePath = filepath.Join(cacheDir, "cas", "store")
	}

	if err := c.fs.MkdirAll(c.storePath, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("failed to create CAS store path: %w", err)
	}

	c.blobStore = NewStore(filepath.Join(c.storePath, "blobs")).WithFS(c.fs)
	c.treeStore = NewStore(filepath.Join(c.storePath, "trees")).WithFS(c.fs)
	c.synthStore = NewStore(filepath.Join(c.storePath, "synth", "trees")).WithFS(c.fs)

	for _, s := range []*Store{c.blobStore, c.treeStore, c.synthStore} {
		if err := c.fs.MkdirAll(s.Path(), DefaultDirPerms); err != nil {
			return nil, fmt.Errorf("failed to create CAS store subdirectory %s: %w", s.Path(), err)
		}
	}

	g, err := git.NewGitRunner(vexec.NewOSExec())
	if err != nil {
		return nil, err
	}

	c.git = g

	gs, err := NewGitStore(c.fs, g, filepath.Join(c.storePath, "git"))
	if err != nil {
		return nil, err
	}

	c.gitStore = gs

	return c, nil
}

// FS returns the configured filesystem.
func (c *CAS) FS() vfs.FS {
	return c.fs
}

// BlobStore returns the store for blob content.
func (c *CAS) BlobStore() *Store { return c.blobStore }

// TreeStore returns the store for git-derived tree content.
func (c *CAS) TreeStore() *Store { return c.treeStore }

// SynthStore returns the store for synthetic tree content.
func (c *CAS) SynthStore() *Store { return c.synthStore }

// Clone performs the clone operation
//
// TODO: Make options optional
func (c *CAS) Clone(ctx context.Context, l log.Logger, opts *CloneOptions, url string) error {
	if err := c.ensureCloneStores(); err != nil {
		return err
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "cas_clone", map[string]any{
		"url":    url,
		"branch": opts.Branch,
	}, func(childCtx context.Context) error {
		ref, err := c.resolveReference(childCtx, url, opts.Branch)
		if err != nil {
			return err
		}

		targetDir := c.prepareTargetDirectory(opts.Dir, url)

		canonicalHash, err := c.populateTreeFromRef(childCtx, l, opts, ref)
		if err != nil {
			return err
		}

		treeContent := NewContent(c.treeStore)

		treeData, err := treeContent.Read(canonicalHash)
		if err != nil {
			return err
		}

		tree, err := git.ParseTree(treeData, targetDir)
		if err != nil {
			return err
		}

		var linkOpts []LinkTreeOption
		if opts.Mutable {
			linkOpts = append(linkOpts, WithForceCopy())
		}

		return LinkTree(childCtx, c.blobStore, c.treeStore, tree, targetDir, linkOpts...)
	})
}

// ensureCloneStores creates the blob and tree store directories that
// [CAS.Clone] writes to. Defensive: [New] already creates them, but a
// long-lived [CAS] instance could see them removed between calls.
func (c *CAS) ensureCloneStores() error {
	for _, s := range []*Store{c.blobStore, c.treeStore} {
		if err := c.fs.MkdirAll(s.Path(), DefaultDirPerms); err != nil {
			return fmt.Errorf("create CAS store path %s: %w", s.Path(), err)
		}
	}

	return nil
}

// populateTreeFromRef dispatches by ref kind, short-circuiting on a
// cached [symbolicRef] and otherwise calling the kind-specific
// populate. Returns the canonical commit hash.
func (c *CAS) populateTreeFromRef(
	ctx context.Context,
	l log.Logger,
	opts *CloneOptions,
	ref resolvedRef,
) (string, error) {
	switch ref := ref.(type) {
	case *symbolicRef:
		if !c.treeStore.NeedsWrite(ref.Hash) {
			return ref.Hash, nil
		}

		if err := c.populateTreeFromSymbolicRef(ctx, l, opts, ref); err != nil {
			return "", err
		}

		return ref.Hash, nil

	case *commitRef:
		if ref.Hash != "" && !c.treeStore.NeedsWrite(ref.Hash) {
			return ref.Hash, nil
		}

		return c.populateTreeFromCommitRef(ctx, l, opts, ref)

	default:
		return "", fmt.Errorf("unsupported resolved ref type %T", ref)
	}
}

// populateTreeFromSymbolicRef stores the tree and reachable blobs
// for ref.Hash in the CAS. Tries the central [GitStore] first; on
// any error from it, logs a warning and falls back to a bare clone
// in a temporary directory.
func (c *CAS) populateTreeFromSymbolicRef(
	ctx context.Context,
	l log.Logger,
	opts *CloneOptions,
	ref *symbolicRef,
) error {
	depth := resolveCloneDepth(opts.Depth, c.cloneDepth)

	repo, err := c.gitStore.EnsureRef(ctx, l, c.fs, ref.URL, ref.Branch, ref.Hash, depth)
	if err == nil {
		defer repo.Release(l)

		runner := c.git.WithWorkDir(repo.Path)

		return c.storeRootTreeFrom(ctx, l, runner, ref.Hash, opts)
	}

	l.Warnf("central git store unavailable for %s, falling back to temporary clone: %v", ref.URL, err)

	tempDir, cleanup, err := c.makeFallbackCloneDir(l)
	if err != nil {
		return err
	}

	defer cleanup()

	runner := c.git.WithWorkDir(tempDir)

	if err := runner.Clone(ctx, ref.URL, true, depth, ref.Branch); err != nil {
		return err
	}

	return c.storeRootTreeFrom(ctx, l, runner, ref.Hash, opts)
}

// populateTreeFromCommitRef resolves ref via [GitStore.EnsureCommit]
// (full-depth fetch on a cache miss) and stores its tree in the CAS.
// Returns the canonical commit hash. Falls back to a temporary bare
// clone if the central [GitStore] is unavailable.
func (c *CAS) populateTreeFromCommitRef(
	ctx context.Context,
	l log.Logger,
	opts *CloneOptions,
	ref *commitRef,
) (string, error) {
	repo, err := c.gitStore.EnsureCommit(ctx, l, c.fs, ref.URL, ref.RawRef, ref.Hash)
	if err == nil {
		defer repo.Release(l)

		if !c.treeStore.NeedsWrite(repo.Hash) {
			return repo.Hash, nil
		}

		runner := c.git.WithWorkDir(repo.Path)

		if err := c.storeRootTreeFrom(ctx, l, runner, repo.Hash, opts); err != nil {
			return "", err
		}

		return repo.Hash, nil
	}

	if errors.Is(err, git.ErrNoMatchingReference) {
		return "", err
	}

	l.Warnf("central git store unavailable for %s, falling back to temporary clone: %v", ref.URL, err)

	tempDir, cleanup, err := c.makeFallbackCloneDir(l)
	if err != nil {
		return "", err
	}

	defer cleanup()

	runner := c.git.WithWorkDir(tempDir)

	if err := runner.Clone(ctx, ref.URL, true, 0, ""); err != nil {
		return "", err
	}

	canonicalHash, err := runner.RevParseCommit(ctx, ref.RawRef)
	if err != nil {
		if errors.Is(err, git.ErrUnknownRevision) {
			return "", &git.WrappedError{
				Op:      "git_clone_resolve",
				Context: fmt.Sprintf("%q in %s", ref.RawRef, ref.URL),
				Err:     git.ErrNoMatchingReference,
			}
		}

		return "", err
	}

	if !c.treeStore.NeedsWrite(canonicalHash) {
		return canonicalHash, nil
	}

	if err := c.storeRootTreeFrom(ctx, l, runner, canonicalHash, opts); err != nil {
		return "", err
	}

	return canonicalHash, nil
}

// makeFallbackCloneDir creates a temporary directory for a bare clone
// fallback and returns a cleanup function that removes it.
func (c *CAS) makeFallbackCloneDir(l log.Logger) (string, func(), error) {
	tempDir, err := vfs.MkdirTemp(c.fs, "", "terragrunt-cas-fallback-*")
	if err != nil {
		return "", nil, fmt.Errorf("create fallback clone dir: %w", errors.Join(ErrFallbackCloneDir, err))
	}

	cleanup := func() {
		if rmErr := c.fs.RemoveAll(tempDir); rmErr != nil {
			l.Warnf("cleanup error: %v", rmErr)
		}
	}

	return tempDir, cleanup, nil
}

func resolveCloneDepth(optDepth, casDepth int) int {
	depth := optDepth
	if depth == 0 {
		depth = casDepth
	}

	if depth == 0 {
		depth = DefaultCASCloneDepth
	}

	if depth < 0 {
		depth = 0
	}

	return depth
}

func (c *CAS) prepareTargetDirectory(dir, url string) string {
	targetDir := dir
	if targetDir == "" {
		targetDir = git.ExtractRepoName(url)
	}

	return filepath.Clean(targetDir)
}

// resolvedRef is what [CAS.resolveReference] returns: a [symbolicRef]
// when ls-remote resolved the input to a branch, tag, or HEAD; a
// [commitRef] when it did not. Sealed by package visibility.
type resolvedRef interface {
	// CommitHash returns the canonical commit hash for [symbolicRef]
	// and the raw user-supplied ref for [commitRef] (no
	// canonicalization: an abbreviated SHA is returned as-is, not
	// expanded to a full hash).
	CommitHash() string
}

// symbolicRef carries an ls-remote-resolved branch, tag, or HEAD.
type symbolicRef struct {
	URL    string
	Branch string // ref name, used for the per-ref fetch
	Hash   string // canonical commit hash
}

// CommitHash returns the canonical commit hash ls-remote resolved.
func (r *symbolicRef) CommitHash() string { return r.Hash }

// commitRef carries a ref ls-remote did not canonicalize to a
// commit on the remote. The typical case is a SHA the server does
// not publish as a branch tip, but any user-supplied name
// ls-remote returned no match for funnels here too. Resolution
// against the central git store happens later via rev-parse, with
// a full-history fetch on a cache miss.
type commitRef struct {
	// URL is the remote repository URL.
	URL string

	// RawRef is the user-supplied ref. SHAs (full SHA-1, full
	// SHA-256, or abbreviated prefixes) are the common case because
	// ls-remote does not surface commit hashes, but any name
	// ls-remote did not match also lands here. The central git
	// store canonicalizes via rev-parse, so any form git accepts
	// works.
	RawRef string

	// Hash is the canonical full SHA pre-resolved by
	// [GitStore.ProbeCachedCommit] before reaching ls-remote;
	// empty when the commitRef arose from an ls-remote miss. When
	// set, downstream code keys the tree-store short-circuit on it
	// and forwards it to [GitStore.EnsureCommit] to skip a
	// redundant rev-parse.
	Hash string
}

// CommitHash returns the user-supplied ref. Hash is intentionally
// not returned: stacks key the CAS on this value, and the key
// must not depend on whether the central git store happened to
// have the commit cached.
func (r *commitRef) CommitHash() string { return r.RawRef }

// resolveReference resolves branch into a [resolvedRef].
//
// Full-length SHA input (40 or 64 hex chars) is probed against the
// central git store first. A cached hit returns a [*commitRef]
// carrying the canonical hash without contacting the remote, so
// previously-cloned commits succeed offline even when ls-remote
// would fail to spawn. The pre-resolved hash also lets
// [CAS.populateTreeFromCommitRef] skip a redundant rev-parse
// inside [GitStore.EnsureCommit]. Abbreviated SHAs skip the probe
// because a hex-named branch could share the abbreviation as a
// prefix of its tip, which would freeze that branch at the
// first-fetched tip; see [looksLikeFullSHA].
//
// Otherwise ls-remote is authoritative: a result returns a
// [*symbolicRef] with the canonical hash; an empty result or
// [git.ErrNoMatchingReference] returns a [*commitRef] with an
// empty Hash so the central store resolves the input via rev-parse
// and a full-history fetch.
func (c *CAS) resolveReference(ctx context.Context, url, branch string) (resolvedRef, error) {
	if looksLikeFullSHA(branch) {
		if hash, ok := c.gitStore.ProbeCachedCommit(ctx, c.fs, url, branch); ok {
			return &commitRef{URL: url, RawRef: branch, Hash: hash}, nil
		}
	}

	results, err := c.git.LsRemote(ctx, url, branch)
	if err != nil {
		if errors.Is(err, git.ErrNoMatchingReference) {
			return &commitRef{URL: url, RawRef: branch}, nil
		}

		return nil, err
	}

	if len(results) == 0 {
		return &commitRef{URL: url, RawRef: branch}, nil
	}

	return &symbolicRef{URL: url, Branch: branch, Hash: results[0].Hash}, nil
}

// looksLikeFullSHA reports whether s is exactly 40 or 64 hex
// characters, the canonical full lengths for SHA-1 and SHA-256
// commit hashes.
//
// Abbreviations are rejected so the probe in [CAS.resolveReference]
// cannot mistake a hex-named branch (e.g. branch "a1b2" whose tip
// happens to start with "a1b2...") for a cached commit prefix and
// freeze the branch at its first-fetched tip. Abbreviated SHAs
// still resolve correctly via the [GitStore.EnsureCommit] fallback
// once ls-remote returns no match.
func looksLikeFullSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}

	_, err := hex.DecodeString(s)

	return err == nil
}

// storeRootTreeFrom reads the recursive tree at hash from the supplied
// runner's working repository and stores its tree and reachable blobs in
// the CAS. The runner must already have its WorkDir pointed at a bare repo
// (or worktree) that contains the requested object.
func (c *CAS) storeRootTreeFrom(
	ctx context.Context,
	l log.Logger,
	runner *git.GitRunner,
	hash string,
	opts *CloneOptions,
) error {
	tree, err := runner.LsTreeRecursive(ctx, hash)
	if err != nil {
		return err
	}

	if err = c.storeTreeRecursive(ctx, l, runner, hash, tree); err != nil {
		return err
	}

	if len(opts.IncludedGitFiles) == 0 {
		return nil
	}

	treeContent := NewContent(c.treeStore)

	data, err := treeContent.Read(hash)
	if err != nil {
		return err
	}

	for _, file := range opts.IncludedGitFiles {
		stat, err := c.fs.Stat(filepath.Join(runner.WorkDir, file))
		if err != nil {
			return err
		}

		if stat.IsDir() {
			continue
		}

		workDirPath := filepath.Join(runner.WorkDir, file)

		includedHash, err := hashFile(c.fs, workDirPath)
		if err != nil {
			return err
		}

		blobContent := NewContent(c.blobStore)

		if err := blobContent.EnsureCopy(l, includedHash, workDirPath); err != nil {
			return err
		}

		path := filepath.Join(".git", file)

		data = append(data, fmt.Appendf(nil, "%06o blob %s\t%s\n", stat.Mode().Perm(), includedHash, path)...)
	}

	// Overwrite the root tree with the new data
	return treeContent.Store(l, hash, data)
}

// storeTreeRecursive stores a tree fetched from git ls-tree -r
func (c *CAS) storeTreeRecursive(
	ctx context.Context,
	l log.Logger,
	runner *git.GitRunner,
	hash string,
	tree *git.Tree,
) error {
	if !c.treeStore.NeedsWrite(hash) {
		return nil
	}

	if err := c.storeBlobs(ctx, runner, tree.Entries()); err != nil {
		return err
	}

	// Store the tree object itself
	treeContent := NewContent(c.treeStore)
	if err := treeContent.EnsureWithWait(l, hash, tree.Data()); err != nil {
		return err
	}

	return nil
}

// storeBlobs stores blobs in the CAS
func (c *CAS) storeBlobs(ctx context.Context, runner *git.GitRunner, entries []git.TreeEntry) error {
	for _, entry := range entries {
		if !c.blobStore.NeedsWrite(entry.Hash) {
			continue
		}

		if err := c.ensureBlob(ctx, runner, entry.Hash, gitFilePerm(entry.Mode)); err != nil {
			return err
		}
	}

	return nil
}

// ensureBlob ensures that a blob exists in the CAS.
// It doesn't use the standard content.Store method because
// we want to take advantage of the ability to write to the
// entry using `git cat-file`. gitPerm is the git tree mode for
// this blob; the stored blob is chmodded to gitPerm with the
// write bits cleared so the default-link path can hardlink the
// blob directly without altering its executable-ness.
func (c *CAS) ensureBlob(ctx context.Context, runner *git.GitRunner, hash string, gitPerm os.FileMode) error {
	needsWrite, lock, err := c.blobStore.EnsureWithWait(hash)
	if err != nil {
		return err
	}

	// If content already exists or was written by another process, we're done
	if !needsWrite {
		return nil
	}

	// We have the lock and need to write the content
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			err = errors.Join(err, unlockErr)
		}
	}()

	content := NewContent(c.blobStore)

	tmpHandle, err := content.GetTmpHandle(hash)
	if err != nil {
		return err
	}

	tmpPath := tmpHandle.Name()

	// We want to make sure we remove the temporary file
	// if we encounter an error
	defer func() {
		if _, statErr := c.fs.Stat(tmpPath); statErr == nil {
			err = errors.Join(err, c.fs.Remove(tmpPath))
		}
	}()

	err = runner.CatFile(ctx, hash, tmpHandle)
	if err != nil {
		return err
	}

	// For Windows, ensure data is synchronized to disk
	if runtime.GOOS == "windows" {
		if err = tmpHandle.Sync(); err != nil {
			return err
		}
	}

	if err = tmpHandle.Close(); err != nil {
		return err
	}

	if err = c.fs.Rename(tmpPath, content.getPath(hash)); err != nil {
		return err
	}

	if err = c.fs.Chmod(content.getPath(hash), gitPerm.Perm()&^WriteBitMask); err != nil {
		return err
	}

	return nil
}

func hashFile(fs vfs.FS, path string) (string, error) {
	file, err := fs.Open(path)
	if err != nil {
		return "", err
	}

	h := sha1.New()

	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	hash := hex.EncodeToString(h.Sum(nil))

	if err := file.Close(); err != nil {
		return hash, fmt.Errorf(
			"hash of %s successfully computed as "+
				"%s, but closing the file failed: %w",
			path,
			hash,
			err,
		)
	}

	return hash, nil
}
