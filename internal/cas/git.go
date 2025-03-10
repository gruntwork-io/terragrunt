package cas

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	minGitPartsLength = 2
)

// GitRunner handles git command execution
type GitRunner struct {
	WorkDir string
}

// NewGitRunner creates a new GitRunner instance
func NewGitRunner() *GitRunner {
	return &GitRunner{}
}

// WithWorkDir returns a new GitRunner with the specified working directory
func (g *GitRunner) WithWorkDir(workDir string) *GitRunner {
	// Create new instance instead of modifying existing one
	return &GitRunner{
		WorkDir: workDir,
	}
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
func (g *GitRunner) LsRemote(repo, ref string) ([]LsRemoteResult, error) {
	if ref == "" {
		ref = "HEAD"
	}

	args := []string{repo, ref}

	cmd := g.prepareCommand("ls-remote", args...)

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

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	for _, line := range lines {
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
func (g *GitRunner) Clone(repo string, bare bool, depth int, branch string) error {
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

	cmd := g.prepareCommand("clone", args...)

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
	tempDir, err := os.MkdirTemp("", "terragrunt-cas-*")
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

// LsTree runs git ls-tree and returns the parsed tree
func (g *GitRunner) LsTree(reference, path string) (*Tree, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	cmd := g.prepareCommand("ls-tree", reference)
	cmd.Dir = g.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &WrappedError{
			Op:      "git_ls_tree",
			Context: stderr.String(),
			Err:     ErrReadTree,
		}
	}

	return ParseTree(stdout.String(), path)
}

// CatFile writes the contents of a git object
// to a given writer.
func (g *GitRunner) CatFile(hash string, out io.Writer) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	var stderr bytes.Buffer

	cmd := g.prepareCommand("cat-file", "-p", hash)
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

// SetWorkDir sets the working directory for git commands
func (g *GitRunner) SetWorkDir(dir string) {
	g.WorkDir = dir
}

func (g *GitRunner) prepareCommand(name string, args ...string) *exec.Cmd {
	return exec.Command("git", append([]string{name}, args...)...)
}
