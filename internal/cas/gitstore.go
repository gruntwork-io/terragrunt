package cas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
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

// NewGitStore returns a GitStore rooted at rootPath, creating the directory
// on fs if needed. The filesystem is not retained; callers pass one explicitly
// to EnsureRef.
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

// EnsureRef ensures the bare repository for url contains the object at hash,
// fetching ref at the requested depth if it does not.
//
// On success it returns the bare repository path (suitable for
// git.GitRunner.WithWorkDir) and an Unlocker the caller must release once
// done reading objects. Failing to release the lock blocks subsequent
// fetches against the same URL.
//
// On failure the lock is released before returning so callers can take a
// different code path without managing the lock themselves.
func (s *GitStore) EnsureRef(
	ctx context.Context,
	l log.Logger,
	fs vfs.FS,
	url, ref, hash string,
	depth int,
) (string, vfs.Unlocker, error) {
	if !vfs.IsOSFS(fs) {
		return "", nil, ErrGitStoreFSNotOS
	}

	dir, repoPath, lockPath := s.repoPaths(url)

	if err := fs.MkdirAll(dir, DefaultDirPerms); err != nil {
		return "", nil, fmt.Errorf("create git store entry %s: %w", dir, errors.Join(ErrGitStorePath, err))
	}

	lockCtx, cancel := context.WithTimeout(ctx, gitStoreLockTimeout)
	defer cancel()

	unlocker, err := vfs.LockContext(lockCtx, fs, lockPath)
	if err != nil {
		return "", nil, fmt.Errorf("lock git store for %s: %w", url, errors.Join(ErrGitStoreLock, err))
	}

	released := false

	defer func() {
		if released {
			return
		}

		if unlockErr := unlocker.Unlock(); unlockErr != nil {
			l.Warnf("git store: failed to release lock for %s: %v", url, unlockErr)
		}
	}()

	if err := fs.MkdirAll(repoPath, DefaultDirPerms); err != nil {
		return "", nil, fmt.Errorf("create bare repo dir %s: %w", repoPath, errors.Join(ErrGitStorePath, err))
	}

	runner := s.runner.WithWorkDir(repoPath)

	initialized, err := bareRepoInitialized(fs, repoPath)
	if err != nil {
		return "", nil, fmt.Errorf("inspect bare repo %s: %w", repoPath, errors.Join(ErrGitStorePath, err))
	}

	if !initialized {
		if err := runner.InitBare(ctx); err != nil {
			return "", nil, err
		}
	}

	has, err := runner.HasObject(ctx, hash)
	if err != nil {
		return "", nil, err
	}

	if !has {
		fetchRef := ref
		if fetchRef == "" {
			fetchRef = "HEAD"
		}

		if err := runner.Fetch(ctx, url, fetchRef, depth); err != nil {
			return "", nil, err
		}

		has, err = runner.HasObject(ctx, hash)
		if err != nil {
			return "", nil, err
		}

		if !has {
			return "", nil, &GitStoreObjectMissingError{Hash: hash, Ref: fetchRef, URL: url}
		}
	}

	released = true

	return repoPath, unlocker, nil
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
// part of repository setup, so its presence lets EnsureRef skip the
// per-call init spawn once a store entry exists.
func bareRepoInitialized(fs vfs.FS, repoPath string) (bool, error) {
	return vfs.FileExists(fs, filepath.Join(repoPath, "HEAD"))
}
