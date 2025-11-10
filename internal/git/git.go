// Package git provides support for Git operations needed throughout the Terragrunt codebase.
package git

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/go-git/go-git/v6/storage/memory"
)

const (
	minGitPartsLength = 2
)

// GitRunner handles git command execution
type GitRunner struct {
	storage *memory.Storage

	GitPath string
	WorkDir string
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
		storage: memory.NewStorage(),
	}, nil
}

// WithWorkDir returns a new GitRunner with the specified working directory
func (g *GitRunner) WithWorkDir(workDir string) *GitRunner {
	copy := *g
	copy.WorkDir = workDir

	return &copy
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
			Err:     ErrCommandSpawn,
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
			Err:     ErrGitClone,
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

	g.SetWorkDir(tempDir)

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

// GetRepoName extracts the repository name from a git URL
func GetRepoName(repo string) string {
	name := filepath.Base(repo)
	return strings.TrimSuffix(name, ".git")
}

// LsTreeRecursive runs git ls-tree -r and returns all blobs recursively
// This eliminates the need for multiple separate ls-tree calls on subtrees
func (g *GitRunner) LsTreeRecursive(ctx context.Context, ref, path string) (string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return "", err
	}

	// Use recursive ls-tree to get all blobs in a single command
	cmd := g.prepareCommand(ctx, "ls-tree", "-r", ref)
	cmd.Dir = g.WorkDir

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", &WrappedError{
			Op:      "git_ls_tree_recursive",
			Context: stderr.String(),
			Err:     ErrReadTree,
		}
	}

	return stdout.String(), nil
}

// ParseTree parses the complete output of git ls-tree
func ParseTree(output, path string) (*Tree, error) {
	// Pre-allocate capacity based on newline count
	capacity := strings.Count(output, "\n") + 1
	entries := make([]TreeEntry, 0, capacity)

	// Use a scanner for more efficient line reading
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		entry, err := ParseTreeEntry(line)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, &WrappedError{
			Op:      "parse_tree",
			Context: "failed to read tree output",
			Err:     err,
		}
	}

	return &Tree{
		entries: entries,
		path:    path,
		data:    []byte(output),
	}, nil
}

const (
	minTreePartsLength = 4
)

// ParseTreeEntry parses a single line from git ls-tree output
func ParseTreeEntry(line string) (TreeEntry, error) {
	// Format: <mode> <type> <hash> <path>
	parts := strings.Fields(line)
	if len(parts) < minTreePartsLength {
		return TreeEntry{}, &WrappedError{
			Op:      "parse_tree_entry",
			Context: "invalid tree entry format",
			Err:     ErrParseTree,
		}
	}

	return TreeEntry{
		Mode: parts[0],
		Type: parts[1],
		Hash: parts[2],
		Path: strings.Join(parts[3:], " "), // Handle paths with spaces
	}, nil
}

// GoLsTreeRecursive uses the `go-git` library to recursively list the contents of a git tree.
//
// In testing, this is significantly slower than LsTreeRecursive, so we don't use it right now.
// We'll keep it here and benchmark it again later if we can optimize it.
func (g *GitRunner) GoLsTreeRecursive(ref, path string) ([]TreeEntry, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	baseDir := g.WorkDir

	fs := osfs.New(baseDir)
	if _, err := fs.Stat(git.GitDirName); err == nil {
		fs, err = fs.Chroot(git.GitDirName)
		if err != nil {
			return nil, err
		}
	}

	s := filesystem.NewStorageWithOptions(fs, cache.NewObjectLRUDefault(), filesystem.Options{KeepDescriptors: true})
	repo, err := git.Open(s, fs)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, err
	}

	c, err := repo.CommitObject(*h)
	if err != nil {
		return nil, err
	}

	tree, err := c.Tree()
	if err != nil {
		return nil, err
	}

	entries, err := g.goLsTreeOnTree(tree, path)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// goLsTreeOnTree uses the `go-git` library to recursively list the contents of a git tree.
func (g *GitRunner) goLsTreeOnTree(tree *object.Tree, path string) ([]TreeEntry, error) {
	entries := make([]TreeEntry, 0, len(tree.Entries))

	for _, entry := range tree.Entries {
		if entry.Mode.IsFile() {
			entries = append(entries, TreeEntry{
				Mode: entry.Mode.String(),
				Type: "blob",
				Hash: entry.Hash.String(),
				Path: filepath.Join(path, entry.Name),
			})
		} else {
			mode, err := entry.Mode.ToOSFileMode()
			if err != nil {
				return nil, err
			}

			if mode.IsDir() {
				subTree, err := tree.Tree(entry.Name)
				if err != nil {
					return nil, err
				}

				subTreePath := filepath.Join(path, entry.Name)

				subTreeEntries, err := g.goLsTreeOnTree(subTree, subTreePath)
				if err != nil {
					return nil, err
				}

				entries = append(entries, subTreeEntries...)
			}
		}
	}

	return entries, nil
}

// CatFile writes the contents of a git object
// to a given writer.
func (g *GitRunner) CatFile(ctx context.Context, hash string, out io.Writer) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	var stderr bytes.Buffer

	cmd := g.prepareCommand(ctx, "cat-file", "-p", hash)
	cmd.Dir = g.WorkDir
	cmd.Stdout = out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		context := stderr.String()

		return &WrappedError{
			Op:      "git_cat_file",
			Context: context,
			Err:     ErrCommandSpawn,
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

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", dir, ref)
	cmd.Dir = g.WorkDir

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_create_detached_worktree",
			Context: stderr.String(),
			Err:     ErrCommandSpawn,
		}
	}

	return nil
}

// SetWorkDir sets the working directory for git commands
func (g *GitRunner) SetWorkDir(dir string) {
	g.WorkDir = dir
}

func (g *GitRunner) prepareCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, g.GitPath, append([]string{name}, args...)...)
}
