// Package git provides support for Git operations needed throughout the Terragrunt codebase.
//
// The package primarily uses the `git` binary installed on the host system, but experimentally supports
// the `go-git` library for some operations. As of yet, the performance of the `go-git` library is not
// as good as the `git` binary, so we don't use it by default. If we can optimize usage of the `go-git` library
// so that the performance difference is negligible, we can choose to use it instead of the `git` binary for certain
// operations.
//
// Even assuming the performance differences are negligible, we'll still prefer to use the `git` binary for certain
// operations. For example, operations related to remotes are likely easier to support with the `git` binary, as
// users might have git configurations for authentication that would be inconvenient to port over to configuration
// of the `go-git` library. This might change in the future.
//
// We'll prefix usage of the `go-git` library with "Go" to make it clear when we're using it.
package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-version"
)

const (
	minGitPartsLength = 2
)

// GitRunner handles git command execution
type GitRunner struct {
	goRepo         *git.Repository
	goStorage      *filesystem.Storage
	exec           vexec.Exec
	repoRootMu     *sync.Mutex
	GitPath        string
	WorkDir        string
	repoRoot       string
	repoRootCached bool
}

// NewGitRunner creates a new GitRunner instance. The provided vexec.Exec is
// used to resolve the `git` binary on PATH.
func NewGitRunner(e vexec.Exec) (*GitRunner, error) {
	gitPath, err := e.LookPath("git")
	if err != nil {
		return nil, &WrappedError{
			Op:      "git",
			Context: "git not found",
			Err:     ErrCommandSpawn,
		}
	}

	return &GitRunner{
		GitPath:    gitPath,
		exec:       e,
		repoRootMu: &sync.Mutex{},
	}, nil
}

// WithWorkDir returns a new GitRunner with the specified working directory
func (g *GitRunner) WithWorkDir(workDir string) *GitRunner {
	if g == nil {
		return &GitRunner{WorkDir: workDir, exec: vexec.NewOSExec(), repoRootMu: &sync.Mutex{}}
	}

	newRunner := *g
	newRunner.WorkDir = workDir
	// A different WorkDir may resolve to a different root, so reset the memo.
	newRunner.repoRootMu = &sync.Mutex{}
	newRunner.repoRoot = ""
	newRunner.repoRootCached = false

	return &newRunner
}

// RequiresWorkDir returns an error if no working directory is set
func (g *GitRunner) RequiresWorkDir() error {
	if g.WorkDir == "" {
		return &WrappedError{
			Op:      "git",
			Context: "no working directory set",
			Err:     ErrNoWorkDir,
		}
	}

	return nil
}

// RequiresGoRepo returns an error if no go repository is set
func (g *GitRunner) RequiresGoRepo() error {
	if g.goRepo == nil {
		return &WrappedError{
			Op:      "git",
			Context: "no go repository set",
			Err:     ErrNoGoRepo,
		}
	}

	return nil
}

// GetRepoRoot returns the root directory of the git repository. The
// successful result is memoized per-runner so subsequent calls skip the
// `git rev-parse` fork; failures are not cached so callers can retry.
// WithWorkDir clears the memo so a derived runner resolves its own root.
func (g *GitRunner) GetRepoRoot(ctx context.Context) (string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return "", err
	}

	g.repoRootMu.Lock()
	defer g.repoRootMu.Unlock()

	if g.repoRootCached {
		return g.repoRoot, nil
	}

	root, err := g.runRepoRoot(ctx)
	if err != nil {
		return "", err
	}

	g.repoRoot = root
	g.repoRootCached = true

	return root, nil
}

// runRepoRoot performs the uncached `git rev-parse --show-toplevel`. Use
// GetRepoRoot for the memoized entry point.
func (g *GitRunner) runRepoRoot(ctx context.Context) (string, error) {
	cmd := g.prepareCommand(ctx, "rev-parse", "--show-toplevel")

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return "", &WrappedError{
			Op:      "git_rev_parse",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

// LsRemoteResult represents the output of git ls-remote
type LsRemoteResult struct {
	Hash string
	Ref  string
}

// LsRemote runs git ls-remote for a specific reference.
// If ref is empty, we check HEAD instead.
func (g *GitRunner) LsRemote(ctx context.Context, repo, ref string) ([]LsRemoteResult, error) {
	if ref == "" {
		ref = "HEAD"
	}

	args := []string{repo, ref}

	cmd := g.prepareCommand(ctx, "ls-remote", args...)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return nil, &WrappedError{
			Op:      "git_ls_remote",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	var results []LsRemoteResult

	lines := strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n")

	for line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= minGitPartsLength {
			results = append(results, LsRemoteResult{
				Hash: parts[0],
				Ref:  parts[1],
			})
		}
	}

	if len(results) == 0 {
		return nil, &WrappedError{
			Op:      "git_ls_remote",
			Context: "no matching references",
			Err:     ErrNoMatchingReference,
		}
	}

	return results, nil
}

const refsTags = "refs/tags/"

// LatestReleaseTag returns the highest semver release tag from the given remote.
// It uses `git ls-remote --tags` against the named remote (e.g. "origin") and
// returns the tag with the greatest semantic version, or "" if none exist.
func (g *GitRunner) LatestReleaseTag(ctx context.Context, remote string) (string, error) {
	results, err := g.LsRemote(ctx, remote, "refs/tags/*")
	if err != nil {
		// No tags is not an error — just means no release tags exist.
		if errors.Is(err, ErrNoMatchingReference) {
			return "", nil
		}

		return "", err
	}

	var best *version.Version

	for _, r := range results {
		name := strings.TrimPrefix(r.Ref, refsTags)
		// Skip dereferenced tag objects (e.g. refs/tags/v1.0.0^{})
		if strings.HasSuffix(name, "^{}") {
			continue
		}

		v, err := version.NewVersion(name)
		if err != nil {
			continue
		}

		if v.Prerelease() != "" {
			continue
		}

		if best == nil || v.GreaterThan(best) {
			best = v
		}
	}

	if best == nil {
		return "", nil
	}

	return best.Original(), nil
}

// Clone performs a git clone operation
func (g *GitRunner) Clone(ctx context.Context, repo string, bare bool, depth int, branch string) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	args := []string{}

	if bare {
		args = append(args, "--bare")
	}

	if depth > 0 {
		args = append(args, "--depth", strconv.Itoa(depth), "--single-branch")
	}

	if branch != "" {
		args = append(args, "--branch", branch)
	}

	args = append(args, repo, g.WorkDir)

	cmd := g.prepareCommand(ctx, "clone", args...)

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_clone",
			Context: stderr.String(),
			Err:     errors.Join(ErrGitClone, err),
		}
	}

	return nil
}

// CreateTempDir creates a new temporary directory for git operations
func (g *GitRunner) CreateTempDir() (string, func() error, error) {
	prefix := "terragrunt-cas-"

	// Add a timestamp to the prefix to avoid conflicts
	prefix += strconv.FormatInt(time.Now().UnixNano(), 10)

	tempDir, err := os.MkdirTemp("", prefix+"*")
	if err != nil {
		return "", nil, &WrappedError{
			Op:      "create_temp_dir",
			Context: err.Error(),
			Err:     ErrCreateTempDir,
		}
	}

	g.WorkDir = tempDir

	cleanup := func() error {
		if err := os.RemoveAll(tempDir); err != nil {
			return &WrappedError{
				Op:      "cleanup_temp_dir",
				Context: err.Error(),
				Err:     ErrCleanupTempDir,
			}
		}

		return nil
	}

	return tempDir, cleanup, nil
}

// ExtractRepoName extracts the repository name from a git URL
func ExtractRepoName(repo string) string {
	name := filepath.Base(repo)
	return strings.TrimSuffix(name, ".git")
}

// LsTreeRecursive runs git ls-tree -r and returns all blobs recursively
// This eliminates the need for multiple separate ls-tree calls on subtrees
func (g *GitRunner) LsTreeRecursive(ctx context.Context, ref string) (*Tree, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	// Use recursive ls-tree to get all blobs in a single command
	cmd := g.prepareCommand(ctx, "ls-tree", "-r", ref)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return nil, &WrappedError{
			Op:      "git_ls_tree_recursive",
			Context: stderr.String(),
			Err:     errors.Join(ErrReadTree, err),
		}
	}

	tree, err := ParseTree(stdout.Bytes(), ".")
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// CatFile writes the contents of a git object
// to a given writer.
func (g *GitRunner) CatFile(ctx context.Context, hash string, out io.Writer) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	var stderr bytes.Buffer

	cmd := g.prepareCommand(ctx, "cat-file", "-p", hash)

	cmd.SetStdout(out)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_cat_file",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// CreateDetachedWorktree creates a new detached worktree for a given reference
// as a given directory
func (g *GitRunner) CreateDetachedWorktree(ctx context.Context, dir, ref string) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	cmd := g.prepareCommand(ctx, "worktree", "add", "--detach", dir, ref)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_create_detached_worktree",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// RemoveWorktree removes a Git worktree for a given path
func (g *GitRunner) RemoveWorktree(ctx context.Context, path string) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	cmd := g.prepareCommand(ctx, "worktree", "remove", "--force", path)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_remove_worktree",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// Diff determines the diff between two Git references.
func (g *GitRunner) Diff(ctx context.Context, fromRef, toRef string) (*Diffs, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	cmd := g.prepareCommand(ctx, "diff", "--name-status", "--no-renames", fromRef, toRef)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return nil, &WrappedError{
			Op:      "git_diff",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return ParseDiff(stdout.Bytes())
}

// Init initializes a Git repository
func (g *GitRunner) Init(ctx context.Context) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	cmd := g.prepareCommand(ctx, "init")

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_init",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// HasUncommittedChanges checks if there are uncommitted changes in the working directory.
// Returns true if there are uncommitted changes, false otherwise (including if git command fails or not in a git repo).
func (g *GitRunner) HasUncommittedChanges(ctx context.Context) bool {
	cmd := g.prepareCommand(ctx, "status", "--porcelain")

	var stdout bytes.Buffer

	cmd.SetStdout(&stdout)

	// If git command fails (e.g., not in a git repo), return false
	if err := cmd.Run(); err != nil {
		return false
	}

	// Check if there are uncommitted changes (non-empty output)
	return strings.TrimSpace(stdout.String()) != ""
}

// Config gets the configuration of the Git repository
func (g *GitRunner) Config(ctx context.Context, name string) (string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return "", err
	}

	cmd := g.prepareCommand(ctx, "config", name)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return "", &WrappedError{
			Op:      "git_config",
			Context: stderr.String(),
			Err:     ErrCommandSpawn,
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GetRemoteURL returns the origin remote URL, or empty string on error.
func (g *GitRunner) GetRemoteURL(ctx context.Context) string {
	remote, _ := g.Config(ctx, "remote.origin.url")
	return remote
}

// GetCurrentBranch returns the current branch name, or empty string on error.
func (g *GitRunner) GetCurrentBranch(ctx context.Context) string {
	if err := g.RequiresWorkDir(); err != nil {
		return ""
	}

	cmd := g.prepareCommand(ctx, "rev-parse", "--abbrev-ref", "HEAD")

	var stdout bytes.Buffer

	cmd.SetStdout(&stdout)

	if err := cmd.Run(); err != nil {
		return ""
	}

	return strings.TrimSpace(stdout.String())
}

// GetHeadCommit returns the current HEAD commit hash, or empty string on error.
func (g *GitRunner) GetHeadCommit(ctx context.Context) string {
	if err := g.RequiresWorkDir(); err != nil {
		return ""
	}

	cmd := g.prepareCommand(ctx, "rev-parse", "HEAD")

	var stdout bytes.Buffer

	cmd.SetStdout(&stdout)

	if err := cmd.Run(); err != nil {
		return ""
	}

	return strings.TrimSpace(stdout.String())
}

// GetDefaultBranch implements the hybrid approach to detect the default branch:
// 1. Tries to determine the default branch of the remote repository using the fast local method first
// 2. Falls back to the network method if the local method fails
// 3. Attempts to update local cache for future use
// Returns the branch name (e.g., "main") or an error if both methods fail.
func (g *GitRunner) GetDefaultBranch(ctx context.Context, l log.Logger) string {
	branch, err := g.GetDefaultBranchLocal(ctx)
	if err == nil && branch != "" {
		return branch
	}

	branch, err = g.GetDefaultBranchRemote(ctx)
	if err == nil && branch != "" {
		err = g.SetRemoteHeadAuto(ctx)
		if err != nil {
			l.Warnf("Failed to update local cache for default branch: %v", err)
		}

		return branch
	}

	l.Debugf("Failed to determine default branch of remote repository," +
		" attempting to get default branch of local repository")

	if b, err := g.Config(ctx, "init.defaultBranch"); err == nil && b != "" {
		return b
	}

	l.Debugf("Failed to determine default branch of local repository, using 'main' as fallback")

	return "main"
}

// GetDefaultBranchLocal attempts to get the default branch using the local cached remote HEAD.
// Returns the branch name (e.g., "main") if successful, or an error if the local ref is not set.
// This is fast and works offline, but requires that `git remote set-head origin --auto` has been run.
func (g *GitRunner) GetDefaultBranchLocal(ctx context.Context) (string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return "", err
	}

	cmd := g.prepareCommand(ctx, "rev-parse", "--abbrev-ref", "origin/HEAD")

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return "", &WrappedError{
			Op:      "git_rev_parse_origin_head",
			Context: stderr.String(),
			Err:     ErrCommandSpawn,
		}
	}

	result := strings.TrimSpace(stdout.String())

	// If the result is just "origin/HEAD", the local ref is not properly set
	if result == "origin/HEAD" {
		return "", &WrappedError{
			Op:      "git_rev_parse_origin_head",
			Context: "local origin/HEAD ref not set",
			Err:     ErrNoMatchingReference,
		}
	}

	if after, ok := strings.CutPrefix(result, "origin/"); ok {
		return after, nil
	}

	return result, nil
}

// GetDefaultBranchRemote queries the remote repository to determine the default branch.
// This is the most accurate method but requires network access.
// Returns the branch name (e.g., "main") if successful.
func (g *GitRunner) GetDefaultBranchRemote(ctx context.Context) (string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return "", err
	}

	cmd := g.prepareCommand(ctx, "ls-remote", "--symref", "origin", "HEAD")

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return "", &WrappedError{
			Op:      "git_ls_remote_symref",
			Context: stderr.String(),
			Err:     ErrCommandSpawn,
		}
	}

	// Parse output: "ref: refs/heads/main    HEAD"
	output := stdout.String()
	lines := strings.SplitSeq(strings.TrimSpace(output), "\n")

	for line := range lines {
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "ref:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 { //nolint:mnd
				ref := parts[1]

				if after, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
					return after, nil
				}
			}
		}
	}

	return "", &WrappedError{
		Op:      "git_ls_remote_symref",
		Context: "could not parse default branch from ls-remote output",
		Err:     ErrNoMatchingReference,
	}
}

// SetRemoteHeadAuto runs `git remote set-head origin --auto` to update the local cached remote HEAD.
// This makes future calls to GetDefaultBranchLocal faster.
func (g *GitRunner) SetRemoteHeadAuto(ctx context.Context) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	cmd := g.prepareCommand(ctx, "remote", "set-head", "origin", "--auto")

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_remote_set_head",
			Context: stderr.String(),
			Err:     ErrCommandSpawn,
		}
	}

	return nil
}

// ObjectFormat returns the object format (hash algorithm) used by the repository in the
// working directory. Returns "sha1" or "sha256". Requires a working directory with a
// git repository (bare or non-bare).
func (g *GitRunner) ObjectFormat(ctx context.Context) (string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return "", err
	}

	cmd := g.prepareCommand(ctx, "rev-parse", "--show-object-format")

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		// Older Git versions don't support --show-object-format; default to sha1.
		return "sha1", nil //nolint:nilerr
	}

	return strings.TrimSpace(stdout.String()), nil
}

func (g *GitRunner) prepareCommand(ctx context.Context, name string, args ...string) vexec.Cmd {
	cmd := g.exec.Command(ctx, g.GitPath, append([]string{name}, args...)...)
	cmd.SetCancel(func() error {
		sig := signal.SignalFromContext(ctx)
		if sig == nil {
			sig = os.Kill
		}

		if err := cmd.Signal(sig); err != nil && !errors.Is(err, vexec.ErrProcessNotStarted) {
			return err
		}

		return nil
	})

	if g.WorkDir != "" {
		cmd.SetDir(g.WorkDir)
	}

	return cmd
}
