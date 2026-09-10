package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	// defaultMaxWalkDepth caps the upward walk so a path whose Dir() never
	// reaches a fixed point cannot hang the process.
	defaultMaxWalkDepth = 1024

	opRepoRoot = "go_repo_root"

	// EnvNameGitCeilingDirectories lists directories the walk will not ascend
	// into, the same way git's own discovery treats it.
	EnvNameGitCeilingDirectories = "GIT_CEILING_DIRECTORIES"
)

// Repository-root discovery errors.
var (
	// ErrNoRepoRoot reports that no directory at or above the starting path
	// holds a `.git` entry. It is the walk's equivalent of git exiting with
	// "not a git repository".
	ErrNoRepoRoot = errors.New("no enclosing git repository")

	// ErrRepoRootWalkDepth reports a walk aborted at its depth bound.
	ErrRepoRootWalkDepth = errors.New("repository-root walk exceeded max depth")

	// ErrRepoPathNotAbsolute reports a starting path that is empty or relative.
	ErrRepoPathNotAbsolute = errors.New("repository-root lookup needs an absolute path")
)

// RepoRootOption configures a [GoRepoRoot] call.
type RepoRootOption func(*repoRootOpts)

type repoRootOpts struct {
	maxDepth int
}

// WithMaxWalkDepth sets how many directories the walk examines before giving
// up, in place of [defaultMaxWalkDepth].
func WithMaxWalkDepth(depth int) RepoRootOption {
	return func(o *repoRootOpts) { o.maxDepth = depth }
}

// maxWalkDepth returns the bound to enforce, falling back to
// [defaultMaxWalkDepth] when the caller leaves it unset.
func (o *repoRootOpts) maxWalkDepth() int {
	if o.maxDepth > 0 {
		return o.maxDepth
	}

	return defaultMaxWalkDepth
}

// GoRepoRoot returns the git repository root that contains path, which must be
// absolute. It walks up from path until a `.git` entry appears rather than
// running `git rev-parse --show-toplevel`, so resolving a root needs no git
// binary. A `.git` file counts the same as a directory, so a linked worktree
// and a submodule resolve the way git resolves them.
//
// Answers are memoized for the run, so a repository created or removed while a
// run is in flight is not observed. A unit inside a nested repository resolves
// to the nested root whichever unit the run resolved first.
//
// The walk reads GIT_CEILING_DIRECTORIES and stops where git's discovery would.
// It ignores GIT_DIR, GIT_WORK_TREE and core.worktree, and does not apply the
// safe.directory ownership check. A bare repository holds no `.git` entry, so
// the walk passes through one to whatever encloses it rather than stopping
// there as git does.
func GoRepoRoot(
	ctx context.Context,
	v *venv.Venv,
	path string,
	opts ...RepoRootOption,
) (string, error) {
	o := &repoRootOpts{}
	for _, opt := range opts {
		opt(o)
	}

	maxDepth := o.maxWalkDepth()

	repoRoots := cache.ContextRepoRootCache(ctx, cache.RepoRootCacheContextKey)

	current, err := startDir(v.FS, path)
	if err != nil {
		return "", err
	}

	ceiling := walkCeiling(v.FS, v.Env, current)

	// Directories the walk proved hold no `.git`, so whichever root it lands on
	// answers for all of them.
	var walked []string

	for range maxDepth {
		// A memo entry at or above the ceiling belongs to a walk that started
		// there, where the ceiling did not apply. Reading the memo first would
		// hand this walk an answer git refuses.
		if current == ceiling {
			return "", &WrappedError{Op: opRepoRoot, Path: path, Err: ErrNoRepoRoot}
		}

		if root, ok := repoRoots.Lookup(ctx, current); ok {
			repoRoots.Add(ctx, root, walked...)

			return root, nil
		}

		_, statErr := v.FS.Stat(filepath.Join(current, ".git"))
		if statErr == nil {
			repoRoots.Add(ctx, current, walked...)

			return current, nil
		}

		if !errors.Is(statErr, os.ErrNotExist) {
			return "", &WrappedError{Op: opRepoRoot, Path: current, Err: statErr}
		}

		walked = append(walked, current)

		parent := filepath.Dir(current)
		if parent == current {
			return "", &WrappedError{Op: opRepoRoot, Path: path, Err: ErrNoRepoRoot}
		}

		current = parent
	}

	return "", &WrappedError{
		Op:      opRepoRoot,
		Path:    path,
		Context: "depth " + strconv.Itoa(maxDepth),
		Err:     ErrRepoRootWalkDepth,
	}
}

// walkCeiling returns the deepest GIT_CEILING_DIRECTORIES entry the walk from
// start must not ascend into, or "" when none applies. Git compares resolved
// entries against a resolved starting directory, which is what [startDir]
// produces. It drops entries that are not absolute, and reads a leading empty
// entry as opting the rest out of symlink resolution.
func walkCeiling(fsys vfs.FS, env map[string]string, start string) string {
	entries := filepath.SplitList(env[EnvNameGitCeilingDirectories])

	resolve := true
	if len(entries) > 0 && entries[0] == "" {
		resolve = false
		entries = entries[1:]
	}

	var deepest string

	for _, entry := range entries {
		if !filepath.IsAbs(entry) {
			continue
		}

		ceiling := filepath.Clean(entry)
		if resolve {
			ceiling = vfs.ResolveForCompare(fsys, ceiling)
		}

		// Git tests ancestry, and no directory is its own ancestor, so an entry
		// naming the starting directory does not block the walk beginning there.
		if !isStrictAncestor(ceiling, start) {
			continue
		}

		if len(ceiling) > len(deepest) {
			deepest = ceiling
		}
	}

	return deepest
}

// isStrictAncestor reports whether path sits strictly below ancestor.
func isStrictAncestor(ancestor, path string) bool {
	// [filepath.Rel] keeps the comparison on component boundaries, so `/repo`
	// is not read as an ancestor of `/repobar`.
	rel, err := filepath.Rel(ancestor, path)
	if err != nil || rel == "." {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// startDir canonicalizes path into the form the walk and the memo compare on,
// and refuses a path that is not absolute. Two spellings of one directory have
// to reduce to one string, and git resolves the working directory physically,
// so a path reached through a symlink walks its real parents rather than its
// lexical ones.
func startDir(fsys vfs.FS, path string) (string, error) {
	// Resolving a relative path would read the process working directory,
	// which the CLI absolutizes once while parsing flags. One arriving here is
	// a caller that skipped that.
	if !filepath.IsAbs(path) {
		return "", &WrappedError{Op: opRepoRoot, Path: path, Err: ErrRepoPathNotAbsolute}
	}

	return vfs.ResolveForCompare(fsys, filepath.Clean(path)), nil
}
