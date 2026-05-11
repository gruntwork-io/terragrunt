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
	"github.com/gruntwork-io/terragrunt/internal/util"
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
	blobStore  *Store
	treeStore  *Store
	synthStore *Store
	gitStore   *GitStore
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

// WithCloneDepth sets git clone --depth for CAS. Positive values request a
// shallow clone; -1 means full history (no --depth). Zero is invalid for git
// and is rejected by [ValidateCASCloneDepth] before reaching this option.
func WithCloneDepth(depth int) Option {
	return func(c *CAS) {
		c.cloneDepth = depth
	}
}

// New creates a new CAS instance with the given options.
func New(opts ...Option) (*CAS, error) {
	c := &CAS{}

	for _, opt := range opts {
		opt(c)
	}

	if c.storePath == "" {
		cacheDir, err := util.EnsureCacheDir()
		if err != nil {
			return nil, err
		}

		c.storePath = filepath.Join(cacheDir, "cas", "store")
	}

	c.blobStore = NewStore(filepath.Join(c.storePath, "blobs"))
	c.treeStore = NewStore(filepath.Join(c.storePath, "trees"))
	c.synthStore = NewStore(filepath.Join(c.storePath, "synth", "trees"))
	c.gitStore = NewGitStore(filepath.Join(c.storePath, "git"))

	return c, nil
}

// BlobStore returns the store for blob content.
func (c *CAS) BlobStore() *Store { return c.blobStore }

// TreeStore returns the store for git-derived tree content.
func (c *CAS) TreeStore() *Store { return c.treeStore }

// SynthStore returns the store for synthetic tree content.
func (c *CAS) SynthStore() *Store { return c.synthStore }

// StorePath returns the root directory containing every CAS store.
func (c *CAS) StorePath() string { return c.storePath }

// ensureStorePaths creates the store directory hierarchy on v.FS. Callers
// invoke this from any top-level entry point that may write to a store, so
// the directories appear lazily on first use rather than at construction.
func (c *CAS) ensureStorePaths(v Venv) error {
	if !vfs.IsOSFS(v.FS) {
		return ErrGitStoreFSNotOS
	}

	if err := v.FS.MkdirAll(c.storePath, DefaultDirPerms); err != nil {
		return fmt.Errorf("create CAS store path: %w", err)
	}

	for _, s := range []*Store{c.blobStore, c.treeStore, c.synthStore} {
		if err := v.FS.MkdirAll(s.Path(), DefaultDirPerms); err != nil {
			return fmt.Errorf("create CAS store subdirectory %s: %w", s.Path(), err)
		}
	}

	return nil
}

// GitResolver is a [SourceResolver] for git URLs.
//
// Branch travels as a field rather than as a URL query parameter so
// SCP-form URLs (`git@host:path`) reach git intact: net/url.Parse
// rejects SCP form, so any encoding scheme that round-trips through
// it silently loses the branch.
type GitResolver struct {
	// Venv supplies the git runner Probe shells out through. Required.
	Venv Venv
	// Store enables an offline fast path: a full-length SHA in
	// [GitResolver.Branch] is checked against the local store before
	// reaching ls-remote. When nil, every Probe runs ls-remote.
	Store *GitStore
	// Branch is the ref to query. Empty means HEAD.
	Branch string
}

// Scheme returns "git".
func (r *GitResolver) Scheme() string { return "git" }

// Probe returns the commit SHA for r.Branch (HEAD when empty). The
// returned SHA is the cache key verbatim and doubles as the git
// object name the fetcher consumes.
//
// `git ls-remote` is authoritative; ls-remote misses (the caller
// supplied a commit-form ref directly) surface as
// [ErrNoVersionMetadata] so the fetcher canonicalizes via rev-parse.
// When [GitResolver.Store] is set, a full-length SHA in r.Branch
// short-circuits ls-remote on a local cache hit.
func (r *GitResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	if r.Store != nil && looksLikeFullSHA(r.Branch) {
		if hash, ok := r.Store.ProbeCachedCommit(ctx, r.Venv, rawURL, r.Branch); ok {
			return hash, nil
		}
	}

	results, err := r.Venv.Git.LsRemote(ctx, rawURL, r.Branch)
	if err != nil {
		if errors.Is(err, git.ErrNoMatchingReference) {
			return "", ErrNoVersionMetadata
		}

		return "", err
	}

	if len(results) == 0 {
		return "", ErrNoVersionMetadata
	}

	return results[0].Hash, nil
}

// Clone fetches url into opts.Dir through the CAS, using a [GitResolver]
// for the probe and ingesting via `git ls-tree -r` / `git cat-file` so the
// native git blob and tree formats reach the stores intact.
//
// TODO: Make options optional
func (c *CAS) Clone(ctx context.Context, l log.Logger, v Venv, opts *CloneOptions, url string) error {
	clonedOpts := *opts
	clonedOpts.Dir = c.prepareTargetDirectory(opts.Dir, url)

	return c.FetchSource(ctx, l, v, &clonedOpts, SourceRequest{
		Scheme:   "git",
		URL:      url,
		Resolver: &GitResolver{Venv: v, Store: c.gitStore, Branch: opts.Branch},
		Fetch:    c.gitFetcher(url, opts),
		Attrs:    map[string]any{"branch": opts.Branch},
	})
}

// gitFetcher returns a SourceFetcher that ingests through the git-native
// path (cat-file + ls-tree). A non-empty suggestedKey is the canonical
// commit SHA from ls-remote; empty means ls-remote produced no match and
// rev-parse against the central GitStore canonicalizes the user ref after
// fetching.
func (c *CAS) gitFetcher(url string, opts *CloneOptions) SourceFetcher {
	return func(ctx context.Context, l log.Logger, v Venv, suggestedKey string) (string, error) {
		var ref resolvedRef
		if suggestedKey != "" {
			ref = &symbolicRef{URL: url, Branch: opts.Branch, Hash: suggestedKey}
		} else {
			ref = &commitRef{URL: url, RawRef: opts.Branch}
		}

		return c.populateTreeFromRef(ctx, l, v, opts, ref)
	}
}

// populateTreeFromRef dispatches by ref kind, short-circuiting on a
// cached [symbolicRef] and otherwise calling the kind-specific
// populate. Returns the canonical commit hash.
func (c *CAS) populateTreeFromRef(
	ctx context.Context,
	l log.Logger,
	v Venv,
	opts *CloneOptions,
	ref resolvedRef,
) (string, error) {
	switch ref := ref.(type) {
	case *symbolicRef:
		if !c.treeStore.NeedsWrite(v, ref.Hash) {
			return ref.Hash, nil
		}

		if err := c.populateTreeFromSymbolicRef(ctx, l, v, opts, ref); err != nil {
			return "", err
		}

		return ref.Hash, nil

	case *commitRef:
		if ref.Hash != "" && !c.treeStore.NeedsWrite(v, ref.Hash) {
			return ref.Hash, nil
		}

		return c.populateTreeFromCommitRef(ctx, l, v, opts, ref)

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
	v Venv,
	opts *CloneOptions,
	ref *symbolicRef,
) error {
	depth := resolveCloneDepth(opts.Depth, c.cloneDepth)

	repo, err := c.gitStore.EnsureRef(ctx, l, v, ref.URL, ref.Branch, ref.Hash, depth)
	if err == nil {
		defer repo.Release(l)

		runner := v.Git.WithWorkDir(repo.Path)

		return c.storeRootTreeFrom(ctx, l, v, runner, ref.Hash, opts)
	}

	l.Warnf("central git store unavailable for %s, falling back to temporary clone: %v", ref.URL, err)

	tempDir, cleanup, err := c.makeFallbackCloneDir(l, v)
	if err != nil {
		return err
	}

	defer cleanup()

	runner := v.Git.WithWorkDir(tempDir)

	if err := runner.Clone(ctx, ref.URL, true, depth, ref.Branch); err != nil {
		return err
	}

	return c.storeRootTreeFrom(ctx, l, v, runner, ref.Hash, opts)
}

// populateTreeFromCommitRef resolves ref via [GitStore.EnsureCommit]
// (full-depth fetch on a cache miss) and stores its tree in the CAS.
// Returns the canonical commit hash. Falls back to a temporary bare
// clone if the central [GitStore] is unavailable.
func (c *CAS) populateTreeFromCommitRef(
	ctx context.Context,
	l log.Logger,
	v Venv,
	opts *CloneOptions,
	ref *commitRef,
) (string, error) {
	repo, err := c.gitStore.EnsureCommit(ctx, l, v, ref.URL, ref.RawRef, ref.Hash)
	if err == nil {
		defer repo.Release(l)

		if !c.treeStore.NeedsWrite(v, repo.Hash) {
			return repo.Hash, nil
		}

		runner := v.Git.WithWorkDir(repo.Path)

		if err := c.storeRootTreeFrom(ctx, l, v, runner, repo.Hash, opts); err != nil {
			return "", err
		}

		return repo.Hash, nil
	}

	if errors.Is(err, git.ErrNoMatchingReference) {
		return "", err
	}

	l.Warnf("central git store unavailable for %s, falling back to temporary clone: %v", ref.URL, err)

	tempDir, cleanup, err := c.makeFallbackCloneDir(l, v)
	if err != nil {
		return "", err
	}

	defer cleanup()

	runner := v.Git.WithWorkDir(tempDir)

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

	if !c.treeStore.NeedsWrite(v, canonicalHash) {
		return canonicalHash, nil
	}

	if err := c.storeRootTreeFrom(ctx, l, v, runner, canonicalHash, opts); err != nil {
		return "", err
	}

	return canonicalHash, nil
}

// makeFallbackCloneDir creates a temporary directory for a bare clone
// fallback and returns a cleanup function that removes it.
func (c *CAS) makeFallbackCloneDir(l log.Logger, v Venv) (string, func(), error) {
	tempDir, err := vfs.MkdirTemp(v.FS, "", "terragrunt-cas-fallback-*")
	if err != nil {
		return "", nil, fmt.Errorf("create fallback clone dir: %w", errors.Join(ErrFallbackCloneDir, err))
	}

	cleanup := func() {
		if rmErr := v.FS.RemoveAll(tempDir); rmErr != nil {
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

// resolvedRef is a sealed sum type returned by [CAS.resolveReference]:
// [symbolicRef] when ls-remote canonicalized the input, [commitRef]
// otherwise.
type resolvedRef interface {
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

// commitRef carries a ref ls-remote did not canonicalize. The central git
// store resolves it later via rev-parse and a full-history fetch on a
// cache miss.
type commitRef struct {
	// URL is the remote repository URL.
	URL string
	// RawRef is the user-supplied ref. Any form `git rev-parse` accepts
	// works (full SHA, abbreviated SHA, name ls-remote did not match).
	RawRef string
	// Hash is the canonical SHA when [GitStore.ProbeCachedCommit]
	// resolved RawRef locally before reaching ls-remote; empty
	// otherwise. Lets downstream code skip a redundant rev-parse.
	Hash string
}

// CommitHash returns the user-supplied ref, not r.Hash. Stacks key the
// CAS on this value, and the key must not depend on whether the central
// git store happened to have the commit cached.
func (r *commitRef) CommitHash() string { return r.RawRef }

// resolveReference resolves branch into a [resolvedRef] via [GitResolver].
//
// Full-length SHAs (40 or 64 hex chars) are checked against the local git
// store first so previously-cloned commits resolve offline; abbreviated
// SHAs skip the probe to avoid mistaking a hex-named branch tip for the
// SHA prefix and freezing the branch at its first-fetched tip (see
// [looksLikeFullSHA]).
func (c *CAS) resolveReference(ctx context.Context, v Venv, url, branch string) (resolvedRef, error) {
	if looksLikeFullSHA(branch) {
		if hash, ok := c.gitStore.ProbeCachedCommit(ctx, v, url, branch); ok {
			return &commitRef{URL: url, RawRef: branch, Hash: hash}, nil
		}
	}

	r := &GitResolver{Venv: v, Store: c.gitStore, Branch: branch}

	key, err := r.Probe(ctx, url)
	if err != nil {
		if errors.Is(err, ErrNoVersionMetadata) {
			return &commitRef{URL: url, RawRef: branch}, nil
		}

		return nil, err
	}

	return &symbolicRef{URL: url, Branch: branch, Hash: key}, nil
}

// looksLikeFullSHA reports whether s is exactly 40 or 64 hex characters,
// the canonical lengths for SHA-1 and SHA-256 commit hashes. Abbreviations
// are intentionally rejected; see the offline-probe rationale on
// [CAS.resolveReference].
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
	v Venv,
	runner *git.GitRunner,
	hash string,
	opts *CloneOptions,
) error {
	tree, err := runner.LsTreeRecursive(ctx, hash)
	if err != nil {
		return err
	}

	if err = c.storeTreeRecursive(ctx, l, v, runner, hash, tree); err != nil {
		return err
	}

	if len(opts.IncludedGitFiles) == 0 {
		return nil
	}

	treeContent := NewContent(c.treeStore)

	data, err := treeContent.Read(v, hash)
	if err != nil {
		return err
	}

	for _, file := range opts.IncludedGitFiles {
		stat, err := v.FS.Stat(filepath.Join(runner.WorkDir, file))
		if err != nil {
			return err
		}

		if stat.IsDir() {
			continue
		}

		workDirPath := filepath.Join(runner.WorkDir, file)

		includedHash, err := hashFile(v.FS, workDirPath)
		if err != nil {
			return err
		}

		blobContent := NewContent(c.blobStore)

		if err := blobContent.EnsureCopy(l, v, includedHash, workDirPath); err != nil {
			return err
		}

		path := filepath.Join(".git", file)

		data = append(data, fmt.Appendf(nil, "%06o blob %s\t%s\n", stat.Mode().Perm(), includedHash, path)...)
	}

	return treeContent.Store(l, v, hash, data)
}

// storeTreeRecursive stores a tree fetched from git ls-tree -r.
func (c *CAS) storeTreeRecursive(
	ctx context.Context,
	l log.Logger,
	v Venv,
	runner *git.GitRunner,
	hash string,
	tree *git.Tree,
) error {
	if !c.treeStore.NeedsWrite(v, hash) {
		return nil
	}

	if err := c.storeBlobs(ctx, v, runner, tree.Entries()); err != nil {
		return err
	}

	treeContent := NewContent(c.treeStore)
	if err := treeContent.EnsureWithWait(l, v, hash, tree.Data()); err != nil {
		return err
	}

	return nil
}

// storeBlobs stores blobs in the CAS.
func (c *CAS) storeBlobs(ctx context.Context, v Venv, runner *git.GitRunner, entries []git.TreeEntry) error {
	for _, entry := range entries {
		if !c.blobStore.NeedsWrite(v, entry.Hash) {
			continue
		}

		if err := c.ensureBlob(ctx, v, runner, entry.Hash, gitFilePerm(entry.Mode)); err != nil {
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
//
// err is a named return so the deferred unlock and tempfile cleanup
// can errors.Join their failures into what the caller actually sees;
// otherwise the assignments target a local variable that has no
// connection to the function's return slot.
func (c *CAS) ensureBlob(
	ctx context.Context,
	v Venv,
	runner *git.GitRunner,
	hash string,
	gitPerm os.FileMode,
) (err error) {
	needsWrite, lock, err := c.blobStore.EnsureWithWait(v, hash)
	if err != nil {
		return err
	}

	if !needsWrite {
		return nil
	}

	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			err = errors.Join(err, unlockErr)
		}
	}()

	content := NewContent(c.blobStore)

	tmpHandle, err := content.GetTmpHandle(v, hash)
	if err != nil {
		return err
	}

	tmpPath := tmpHandle.Name()

	defer func() {
		if _, statErr := v.FS.Stat(tmpPath); statErr == nil {
			err = errors.Join(err, v.FS.Remove(tmpPath))
		}
	}()

	err = runner.CatFile(ctx, hash, tmpHandle)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		if err = tmpHandle.Sync(); err != nil {
			return err
		}
	}

	if err = tmpHandle.Close(); err != nil {
		return err
	}

	if err = v.FS.Rename(tmpPath, content.getPath(hash)); err != nil {
		return err
	}

	// Symlink entries (git mode 120000) have no permission bits, but the blob
	// stores the link target string and must stay readable so linkTree can
	// resolve the symlink at materialization time.
	storedPerm := gitPerm.Perm() &^ WriteBitMask
	if storedPerm == 0 {
		storedPerm = StoredFilePerms
	}

	if err = v.FS.Chmod(content.getPath(hash), storedPerm); err != nil {
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
