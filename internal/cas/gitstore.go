package cas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	gitStoreURLHashLen = 16
	// gitStoreLockTimeout bounds how long EnsureRef waits for the per-URL
	// lock before giving up and letting the caller fall back to a temporary
	// clone. Generous enough to outlast a typical fetch, short enough that a
	// hung holder does not stall every concurrent unit indefinitely.
	gitStoreLockTimeout = 5 * time.Minute
)

// GitStore keeps one bare git repository per remote URL on disk so CAS cache
// misses can issue an incremental git fetch instead of a full shallow clone.
// Each per-URL repository is gated by an exclusive flock because pack-file
// writes are not safe to interleave with concurrent reads of the same repo.
// EnsureRef waits up to gitStoreLockTimeout for the lock; on context
// cancellation or timeout the caller can fall back to a temporary clone
// rather than block indefinitely on a hung holder. After acquiring the
// lock, EnsureRef re-checks for the requested object so a unit that simply
// waited out a peer's fetch can proceed without re-doing the work.
// The flock is held from EnsureRef return until the caller releases it.
type GitStore struct {
	runner   *git.GitRunner
	rootPath string
}

// GitStoreRepo is a locked handle to a per-URL bare repository. The
// caller has exclusive access to the underlying repo until [GitStoreRepo.Unlock]
// is called; failing to release the lock blocks subsequent fetches
// against the same URL.
type GitStoreRepo struct {
	unlocker vfs.Unlocker

	// url records the URL acquire was called with so Release can
	// include it in unlock-failure log messages without callers
	// re-threading it.
	url string

	// Path is the bare repository path, suitable for
	// [git.GitRunner.WithWorkDir].
	Path string

	// Hash is the canonical commit hash resolved by [GitStore.EnsureCommit].
	// Empty for repos returned by [GitStore.EnsureRef], where the caller
	// already knows the hash.
	Hash string
}

// Unlock releases the per-URL flock held by this repo handle and
// returns any unlock error to the caller.
func (r *GitStoreRepo) Unlock() error {
	return r.unlocker.Unlock()
}

// Release unlocks the repo handle, logging unlock failures against the
// originating URL. Intended for `defer repo.Release(l)`; callers that
// need the unlock error directly should call [GitStoreRepo.Unlock] instead.
func (r *GitStoreRepo) Release(l log.Logger) {
	if err := r.unlocker.Unlock(); err != nil {
		l.Warnf("git store: failed to release lock for %s: %v", r.url, err)
	}
}

// NewGitStore returns a [GitStore] rooted at rootPath, creating the directory
// on fs if needed. The filesystem is not retained; callers pass one explicitly
// to [GitStore.EnsureRef] and [GitStore.EnsureCommit].
//
// The git store shells out to `git`, which only sees the real disk. Callers
// must pass an OS-backed [vfs.FS] from [vfs.NewOSFS]; an in-memory backing
// returns [ErrGitStoreFSNotOS].
func NewGitStore(fs vfs.FS, runner *git.GitRunner, rootPath string) (*GitStore, error) {
	if !vfs.IsOSFS(fs) {
		return nil, ErrGitStoreFSNotOS
	}

	if err := fs.MkdirAll(rootPath, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("create git store at %s: %w", rootPath, errors.Join(ErrGitStorePath, err))
	}

	return &GitStore{
		runner:   runner,
		rootPath: rootPath,
	}, nil
}

// EnsureRef ensures the bare repository for url contains the object at
// hash, fetching ref at the requested depth if it does not.
//
// On success the returned repo's Path is suitable for
// [git.GitRunner.WithWorkDir], and the caller must release the embedded
// flock with [GitStoreRepo.Unlock] (or [GitStoreRepo.Release]) once done
// reading objects.
//
// On failure the lock is released before returning so callers can take
// a different code path without managing the lock themselves.
func (s *GitStore) EnsureRef(
	ctx context.Context,
	l log.Logger,
	fs vfs.FS,
	url, ref, hash string,
	depth int,
) (*GitStoreRepo, error) {
	session, err := s.acquire(ctx, fs, l, url)
	if err != nil {
		return nil, err
	}

	defer session.cleanup()

	has, err := session.runner.HasObject(ctx, hash)
	if err != nil {
		return nil, err
	}

	if !has {
		fetchRef := ref
		if fetchRef == "" {
			fetchRef = "HEAD"
		}

		if err := session.runner.Fetch(ctx, url, fetchRef, depth); err != nil {
			return nil, err
		}

		has, err = session.runner.HasObject(ctx, hash)
		if err != nil {
			return nil, err
		}

		if !has {
			return nil, &GitStoreObjectMissingError{Hash: hash, Ref: fetchRef, URL: url}
		}
	}

	return session.keep(), nil
}

// EnsureCommit ensures the bare repository for url contains a commit
// reachable from rawRef and returns its canonical full hash via
// [GitStoreRepo.Hash].
//
// rawRef may be a full SHA-1 (40 hex chars), full SHA-256 (64 hex
// chars), or an abbreviated SHA that disambiguates inside the repo.
// Resolution runs `git rev-parse <ref>^{commit}` against the per-URL
// bare repository, so any form git accepts works.
//
// Behavior:
//
//  1. If the commit is already cached in the bare repo, no network
//     call is made.
//  2. Otherwise the bare repo is updated with a full-history fetch of
//     every branch (no `--depth`). Fetching by raw SHA is avoided
//     because it requires `uploadpack.allowAnySHA1InWant`, which is
//     not universally enabled on git servers.
//  3. If rev-parse still cannot resolve rawRef after the fetch, a
//     [git.WrappedError] wrapping [git.ErrNoMatchingReference] is
//     returned so callers can use [errors.Is] for the same condition
//     `git ls-remote` surfaces for symbolic refs.
//
// On success the caller must release the lock via [GitStoreRepo.Unlock]
// or [GitStoreRepo.Release], matching the contract of [GitStore.EnsureRef].
func (s *GitStore) EnsureCommit(
	ctx context.Context,
	l log.Logger,
	fs vfs.FS,
	url, rawRef string,
) (*GitStoreRepo, error) {
	session, err := s.acquire(ctx, fs, l, url)
	if err != nil {
		return nil, err
	}

	defer session.cleanup()

	hash, err := session.runner.RevParseCommit(ctx, rawRef)
	if err == nil {
		session.repo.Hash = hash
		return session.keep(), nil
	}

	if !errors.Is(err, git.ErrUnknownRevision) {
		return nil, err
	}

	if err := session.runner.Fetch(ctx, url, "+refs/heads/*:refs/heads/*", 0); err != nil {
		return nil, err
	}

	hash, err = session.runner.RevParseCommit(ctx, rawRef)
	if err != nil {
		if errors.Is(err, git.ErrUnknownRevision) {
			return nil, &git.WrappedError{
				Op:      "git_store_resolve",
				Context: fmt.Sprintf("%q in %s", rawRef, url),
				Err:     git.ErrNoMatchingReference,
			}
		}

		return nil, err
	}

	session.repo.Hash = hash

	return session.keep(), nil
}

// ProbeCachedCommit returns the canonical commit hash if rawRef
// resolves to a commit already stored in the per-URL bare repository
// for url and rawRef is a prefix of that hash. Returns ok=false in
// any other case (no bare repo yet, unresolvable ref, or a name that
// resolved through ref lookup such as a hex-named branch whose tip
// is a different commit).
//
// Panics if fs is not OS-backed. git only sees the real disk, so a
// non-OS backing cannot satisfy the probe.
//
// The probe is lock-free on purpose. The per-URL flock serializes
// fetches so concurrent pack-file writes do not interleave, but
// rev-parse only reads pack indices and refs, both of which git
// updates atomically. Acquiring the flock for a read would queue the
// probe behind any in-flight fetch and erase the offline win.
func (s *GitStore) ProbeCachedCommit(ctx context.Context, fs vfs.FS, url, rawRef string) (string, bool) {
	if !vfs.IsOSFS(fs) {
		panic(ErrGitStoreFSNotOS)
	}

	_, repoPath, _ := s.repoPaths(url)

	initialized, err := bareRepoInitialized(fs, repoPath)
	if err != nil || !initialized {
		return "", false
	}

	hash, err := s.runner.WithWorkDir(repoPath).RevParseCommit(ctx, rawRef)
	if err != nil {
		return "", false
	}

	// rev-parse falls back to ref lookup when the input is not an
	// object hash, so a hex-named branch resolves to its tip. Without
	// this prefix check the probe would mistake those branches for
	// cached commits and serve stale data when the branch has moved.
	if !strings.HasPrefix(strings.ToLower(hash), strings.ToLower(rawRef)) {
		return "", false
	}

	return hash, true
}

// RootPath returns the directory that contains all per-URL bare repositories.
func (s *GitStore) RootPath() string {
	return s.rootPath
}

// EntryPathForURL returns the directory used for the bare repository
// belonging to url. Exported so tests can place blocking files at the path.
func EntryPathForURL(rootPath, url string) string {
	sum := sha256.Sum256([]byte(url))
	name := hex.EncodeToString(sum[:])[:gitStoreURLHashLen]

	return filepath.Join(rootPath, name)
}

func (s *GitStore) repoPaths(url string) (dir, repo, lockPath string) {
	dir = EntryPathForURL(s.rootPath, url)
	repo = filepath.Join(dir, "repo")
	lockPath = filepath.Join(dir, "lock")

	return dir, repo, lockPath
}

// bareRepoInitialized reports whether repoPath already holds a bare git
// repository. Checking for HEAD is enough: `git init --bare` writes it as
// part of repository setup, so its presence lets acquire skip the
// per-call init spawn once a store entry exists.
func bareRepoInitialized(fs vfs.FS, repoPath string) (bool, error) {
	return vfs.FileExists(fs, filepath.Join(repoPath, "HEAD"))
}

// repoSession bundles everything a [GitStore.acquire] caller needs
// to operate on a per-URL bare repository: the [GitStoreRepo] handle
// (the locked thing), a runner pointed at it, and a deferred-cleanup
// helper. Callers defer cleanup(); keep() promotes the handle so the
// lock survives until the caller releases it explicitly.
type repoSession struct {
	l      log.Logger
	repo   *GitStoreRepo
	runner *git.GitRunner
	kept   bool
}

// keep promotes the session's repo handle to the caller. After keep
// the deferred cleanup is a no-op and the caller owns the lock until
// it invokes [GitStoreRepo.Unlock] or [GitStoreRepo.Release].
func (s *repoSession) keep() *GitStoreRepo {
	s.kept = true
	return s.repo
}

// cleanup releases the lock unless keep was called. Intended for
// `defer session.cleanup()`.
func (s *repoSession) cleanup() {
	if s.kept {
		return
	}

	s.repo.Release(s.l)
}

// acquire claims the per-URL flock for url, prepares the bare-repo
// directory, and returns a [repoSession] carrying the locked handle
// and a runner pointed at it. The caller defers session.cleanup();
// session.keep() promotes the handle so the lock survives.
//
// `git init --bare` is invoked only on first use of a store entry;
// subsequent calls detect the existing HEAD and skip the spawn.
func (s *GitStore) acquire(ctx context.Context, fs vfs.FS, l log.Logger, url string) (*repoSession, error) {
	if !vfs.IsOSFS(fs) {
		return nil, ErrGitStoreFSNotOS
	}

	dir, repoPath, lockPath := s.repoPaths(url)

	if err := fs.MkdirAll(dir, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("create git store entry %s: %w", dir, errors.Join(ErrGitStorePath, err))
	}

	lockCtx, cancel := context.WithTimeout(ctx, gitStoreLockTimeout)
	defer cancel()

	unlocker, err := vfs.LockContext(lockCtx, fs, lockPath)
	if err != nil {
		return nil, fmt.Errorf("lock git store for %s: %w", url, errors.Join(ErrGitStoreLock, err))
	}

	session := &repoSession{
		l:      l,
		repo:   &GitStoreRepo{unlocker: unlocker, url: url, Path: repoPath},
		runner: s.runner.WithWorkDir(repoPath),
	}

	if err := fs.MkdirAll(repoPath, DefaultDirPerms); err != nil {
		session.cleanup()
		return nil, fmt.Errorf("create bare repo dir %s: %w", repoPath, errors.Join(ErrGitStorePath, err))
	}

	initialized, err := bareRepoInitialized(fs, repoPath)
	if err != nil {
		session.cleanup()
		return nil, fmt.Errorf("inspect bare repo %s: %w", repoPath, errors.Join(ErrGitStorePath, err))
	}

	if !initialized {
		if err := session.runner.InitBare(ctx); err != nil {
			session.cleanup()
			return nil, err
		}
	}

	return session, nil
}
