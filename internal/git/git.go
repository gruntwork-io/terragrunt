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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/storage/filesystem"
)

const (
	minGitPartsLength = 2
)

// GitRunner handles git command execution
type GitRunner struct {
	goRepo    *git.Repository
	goStorage *filesystem.Storage
	GitPath   string
	WorkDir   string
}

// NewGitRunner creates a new GitRunner instance
func NewGitRunner() (*GitRunner, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, &WrappedError{
			Op:      "git",
			Context: "git not found",
			Err:     ErrCommandSpawn,
		}
	}

	return &GitRunner{
		GitPath: gitPath,
	}, nil
}

// WithWorkDir returns a new GitRunner with the specified working directory
func (g *GitRunner) WithWorkDir(workDir string) *GitRunner {
	if g == nil {
		return &GitRunner{WorkDir: workDir}
	}

	newRunner := *g
	newRunner.WorkDir = workDir

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

// GetRepoRoot returns the root directory of the git repository.
func (g *GitRunner) GetRepoRoot(ctx context.Context) (string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return "", err
	}

	cmd := g.prepareCommand(ctx, "rev-parse", "--show-toplevel")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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
		args = append(args, "--depth", "1", "--single-branch")
	}

	if branch != "" {
		args = append(args, "--branch", branch)
	}

	args = append(args, repo, g.WorkDir)

	cmd := g.prepareCommand(ctx, "clone", args...)

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

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

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	cmd.Stdout = out
	cmd.Stderr = &stderr

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

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_init",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

func (g *GitRunner) prepareCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, g.GitPath, append([]string{name}, args...)...)

	if g.WorkDir != "" {
		cmd.Dir = g.WorkDir
	}

	return cmd
}
