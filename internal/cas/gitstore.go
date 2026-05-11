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
// misses can issue an incremental fetch instead of a fresh shallow clone.
//
// Each per-URL repository is gated by an exclusive flock because pack-file
// writes are not safe to interleave with concurrent reads. EnsureRef waits up
// to [gitStoreLockTimeout] before giving up so the caller can fall back to a
// temporary clone rather than block indefinitely on a hung holder.
type GitStore struct {
	rootPath string
}

// GitStoreRepo is a locked handle to a per-URL bare repository. The caller
// has exclusive access until [GitStoreRepo.Unlock] returns; failing to
// release the lock blocks every subsequent fetch against the same URL.
type GitStoreRepo struct {
	unlocker vfs.Unlocker

	// url is the source URL, kept so Release can name it in
	// unlock-failure logs without callers re-threading it.
	url string

	// Path is the bare repository path, suitable for
	// [git.GitRunner.WithWorkDir].
	Path string

	// Hash is the canonical commit hash resolved by [GitStore.EnsureCommit].
	// Empty for repos returned by [GitStore.EnsureRef], where the caller
	// already knows the hash.
	Hash string
}

// Unlock releases the per-URL flock and returns any unlock error.
func (r *GitStoreRepo) Unlock() error {
	return r.unlocker.Unlock()
}

// Release unlocks and logs any unlock error against the originating URL.
// Intended for `defer repo.Release(l)`; callers that need the error
// directly should use [GitStoreRepo.Unlock].
func (r *GitStoreRepo) Release(l log.Logger) {
	if err := r.unlocker.Unlock(); err != nil {
		l.Warnf("git store: failed to release lock for %s: %v", r.url, err)
	}
}

// NewGitStore returns a [GitStore] rooted at rootPath. The directory is
// created lazily on first write.
func NewGitStore(rootPath string) *GitStore {
	return &GitStore{rootPath: rootPath}
}

// EnsureRef ensures the bare repository for url contains the object at
// hash, fetching ref at the requested depth on a cache miss. The returned
// handle holds the per-URL flock; the caller must release it via
// [GitStoreRepo.Unlock] or [GitStoreRepo.Release]. On error the lock is
// released before returning.
func (s *GitStore) EnsureRef(
	ctx context.Context,
	l log.Logger,
	v Venv,
	url, ref, hash string,
	depth int,
) (*GitStoreRepo, error) {
	session, err := s.acquire(ctx, v, l, url)
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
// [GitStoreRepo.Hash]. Any rawRef `git rev-parse` accepts works.
//
// If knownHash is non-empty (typically from [GitStore.ProbeCachedCommit])
// the cache-hit path verifies it with [git.GitRunner.HasObject] and skips
// rev-parse. On a cache miss the bare repo fetches every branch with no
// --depth; fetching by raw SHA would require `uploadpack.allowAnySHA1InWant`,
// which is not universally enabled on git servers. An unresolvable rawRef
// after the fetch surfaces as [git.WrappedError] wrapping
// [git.ErrNoMatchingReference] so callers can match it with [errors.Is].
//
// Lock contract matches [GitStore.EnsureRef]: callers must release on
// success, the lock is released for them on error.
func (s *GitStore) EnsureCommit(
	ctx context.Context,
	l log.Logger,
	v Venv,
	url, rawRef, knownHash string,
) (*GitStoreRepo, error) {
	session, err := s.acquire(ctx, v, l, url)
	if err != nil {
		return nil, err
	}

	defer session.cleanup()

	if knownHash != "" {
		return s.ensureKnownCommit(ctx, session, url, rawRef, knownHash)
	}

	hash, err := session.runner.RevParseCommit(ctx, rawRef)
	if err == nil {
		session.repo.Hash = hash
		return session.keep(), nil
	}

	if !errors.Is(err, git.ErrUnknownRevision) {
		return nil, err
	}

	if err := session.runner.Fetch(ctx, url, "+refs/*:refs/*", 0); err != nil {
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

// ensureKnownCommit handles the [GitStore.EnsureCommit] path where the
// caller has already canonicalized rawRef. A locked miss (a peer ran
// git-gc between the lock-free probe and this verify) triggers a
// full-history fetch and a recheck.
func (s *GitStore) ensureKnownCommit(
	ctx context.Context,
	session *repoSession,
	url, rawRef, knownHash string,
) (*GitStoreRepo, error) {
	has, err := session.runner.HasObject(ctx, knownHash)
	if err != nil {
		return nil, err
	}

	if has {
		session.repo.Hash = knownHash
		return session.keep(), nil
	}

	if err := session.runner.Fetch(ctx, url, "+refs/*:refs/*", 0); err != nil {
		return nil, err
	}

	has, err = session.runner.HasObject(ctx, knownHash)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, &git.WrappedError{
			Op:      "git_store_resolve",
			Context: fmt.Sprintf("%q in %s", rawRef, url),
			Err:     git.ErrNoMatchingReference,
		}
	}

	session.repo.Hash = knownHash

	return session.keep(), nil
}

// ProbeCachedCommit returns the canonical commit hash when rawRef is a
// prefix of a commit already stored in the per-URL bare repository, and
// ok=false otherwise (no bare repo, unresolvable ref, or a name that
// happened to resolve through ref lookup, such as a hex-named branch).
//
// The probe is lock-free: rev-parse only reads pack indices and refs,
// both updated atomically by git. Acquiring the per-URL flock here would
// queue every probe behind any in-flight fetch and erase the offline
// win.
//
// Panics when v.FS is not OS-backed; git only sees the real disk.
func (s *GitStore) ProbeCachedCommit(ctx context.Context, v Venv, url, rawRef string) (string, bool) {
	if !vfs.IsOSFS(v.FS) {
		panic(ErrGitStoreFSNotOS)
	}

	_, repoPath, _ := s.repoPaths(url)

	initialized, err := bareRepoInitialized(v.FS, repoPath)
	if err != nil || !initialized {
		return "", false
	}

	hash, err := v.Git.WithWorkDir(repoPath).RevParseCommit(ctx, rawRef)
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

// repoSession bundles the locked repo handle, the runner pointed at it,
// and a deferred-cleanup helper. Callers `defer session.cleanup()` to
// release the lock on error and call `session.keep()` to promote the
// handle on success.
type repoSession struct {
	l      log.Logger
	repo   *GitStoreRepo
	runner *git.GitRunner
	kept   bool
}

// keep promotes the handle so cleanup is a no-op and the caller owns
// the lock until [GitStoreRepo.Unlock] or [GitStoreRepo.Release].
func (s *repoSession) keep() *GitStoreRepo {
	s.kept = true
	return s.repo
}

// cleanup releases the lock unless keep was called.
func (s *repoSession) cleanup() {
	if s.kept {
		return
	}

	s.repo.Release(s.l)
}

// acquire claims the per-URL flock and returns a [repoSession] carrying
// the locked handle. `git init --bare` runs only on first use of a store
// entry; subsequent calls detect HEAD and skip the spawn.
func (s *GitStore) acquire(ctx context.Context, v Venv, l log.Logger, url string) (*repoSession, error) {
	if !vfs.IsOSFS(v.FS) {
		return nil, ErrGitStoreFSNotOS
	}

	if err := v.FS.MkdirAll(s.rootPath, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("create git store at %s: %w", s.rootPath, errors.Join(ErrGitStorePath, err))
	}

	dir, repoPath, lockPath := s.repoPaths(url)

	if err := v.FS.MkdirAll(dir, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("create git store entry %s: %w", dir, errors.Join(ErrGitStorePath, err))
	}

	lockCtx, cancel := context.WithTimeout(ctx, gitStoreLockTimeout)
	defer cancel()

	unlocker, err := vfs.LockContext(lockCtx, v.FS, lockPath)
	if err != nil {
		return nil, fmt.Errorf("lock git store for %s: %w", url, errors.Join(ErrGitStoreLock, err))
	}

	session := &repoSession{
		l:      l,
		repo:   &GitStoreRepo{unlocker: unlocker, url: url, Path: repoPath},
		runner: v.Git.WithWorkDir(repoPath),
	}

	if err := v.FS.MkdirAll(repoPath, DefaultDirPerms); err != nil {
		session.cleanup()
		return nil, fmt.Errorf("create bare repo dir %s: %w", repoPath, errors.Join(ErrGitStorePath, err))
	}

	initialized, err := bareRepoInitialized(v.FS, repoPath)
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
