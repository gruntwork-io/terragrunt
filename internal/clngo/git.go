package clngo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitRunner handles git command execution
type GitRunner struct {
	workDir string
}

// NewGitRunner creates a new GitRunner instance
func NewGitRunner() *GitRunner {
	return &GitRunner{}
}

// WithWorkDir returns a new GitRunner with the specified working directory
func (g *GitRunner) WithWorkDir(workDir string) *GitRunner {
	return &GitRunner{workDir: workDir}
}

// RequiresWorkDir returns an error if no working directory is set
func (g *GitRunner) RequiresWorkDir() error {
	if g.workDir == "" {
		return &WrappedError{
			Op:      "check_work_dir",
			Context: "working directory not set",
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

// LsRemote runs git ls-remote for a specific reference
func (g *GitRunner) LsRemote(repo, reference string) ([]LsRemoteResult, error) {
	cmd := exec.Command("git", "ls-remote", repo, reference)
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

	output := strings.TrimSpace(stdout.String())
	lines := strings.Split(output, "\n")
	results := make([]LsRemoteResult, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
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

	args := []string{"clone"}

	if bare {
		args = append(args, "--bare")
	}

	if depth > 0 {
		args = append(args, "--depth", "1", "--single-branch")
	}

	if branch != "" {
		args = append(args, "--branch", branch)
	}

	args = append(args, repo, g.workDir)

	cmd := exec.Command("git", args...)

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
	tempDir, err := os.MkdirTemp("", "clngo-*")
	if err != nil {
		return "", nil, &WrappedError{
			Op:  "create_temp_dir",
			Err: ErrTempDir,
		}
	}

	g.SetWorkDir(tempDir)

	cleanup := func() error {
		if err := os.RemoveAll(tempDir); err != nil {
			return &WrappedError{
				Op:   "cleanup_temp_dir",
				Path: tempDir,
				Err:  ErrTempDir,
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

	cmd := exec.Command("git", "ls-tree", reference)
	cmd.Dir = g.workDir
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

// CatFile retrieves the content of a git object
func (g *GitRunner) CatFile(hash string) ([]byte, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "cat-file", "-p", hash)
	cmd.Dir = g.workDir

	output, err := cmd.Output()
	if err != nil {
		return nil, &WrappedError{
			Op:      "git_cat_file",
			Context: err.Error(),
			Err:     ErrCommandSpawn,
		}
	}

	return output, nil
}

// SetWorkDir sets the working directory for git commands
func (g *GitRunner) SetWorkDir(dir string) {
	g.workDir = dir
}
