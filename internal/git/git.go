// Package git provides support for Git operations needed throughout the Terragrunt codebase.
// All operations are backed by the git binary installed on the host system.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	semver "github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	minGitPartsLength = 2

	// catFileMissingExitCode is the exit code `git cat-file -e` returns when
	// the requested object is absent. Any other non-zero exit is an
	// execution failure (e.g. 128 from a fatal error).
	catFileMissingExitCode = 1
)

// GitRunner handles git command execution
type GitRunner struct {
	exec    vexec.Exec
	env     map[string]string
	GitPath string
	WorkDir string
}

// NewGitRunner creates a new GitRunner instance. It resolves the `git` binary
// through the venv's exec handle. Every git command it prepares carries the
// venv's environment, so a subprocess does not inherit the process
// environment.
func NewGitRunner(v *venv.Venv) (*GitRunner, error) {
	v.RequireExec()
	v.RequireEnv()

	gitPath, err := v.Exec.LookPath("git")
	if err != nil {
		return nil, &WrappedError{
			Op:      "git",
			Context: "git not found",
			Err:     ErrCommandSpawn,
		}
	}

	return &GitRunner{
		GitPath: gitPath,
		exec:    v.Exec,
		env:     v.Env,
	}, nil
}

// ExtractRepoName extracts the repository name from a git URL
func ExtractRepoName(repo string) string {
	name := filepath.Base(repo)
	return strings.TrimSuffix(name, ".git")
}

// WithWorkDir returns a new GitRunner with the specified working directory
func (g *GitRunner) WithWorkDir(workDir string) *GitRunner {
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

	args := []string{"--", repo, ref}

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

// FetchMatch returns the entry of results that `git fetch` selects for ref.
//
// [GitRunner.LsRemote] matches ref against the tail of each advertised name,
// so v1.2.3 also matches refs/heads/release/v1.2.3, which ls-remote lists
// before refs/tags/v1.2.3. `git fetch` expands ref through the rules in
// gitrevisions(7) instead and takes the first rule that names an advertised
// ref, so a tag wins over a branch of the same name. It also returns the rule
// that matched. It reports false when no entry is a name `git fetch` would
// accept for ref.
func FetchMatch(results []LsRemoteResult, ref string) (LsRemoteResult, FetchRule, bool) {
	for _, rule := range fetchRefRules {
		want := rule.Expand(ref)

		if i := slices.IndexFunc(results, func(res LsRemoteResult) bool { return res.Ref == want }); i >= 0 {
			return results[i], rule, true
		}
	}

	return LsRemoteResult{}, "", false
}

// FetchRule is one of the patterns `git fetch` expands a short ref name
// through, as gitrevisions(7) lists them.
type FetchRule string

// ParseFetchRule returns s as a [FetchRule], reporting false when s is not
// one of the rules `git fetch` tries.
func ParseFetchRule(s string) (FetchRule, bool) {
	rule := FetchRule(s)

	return rule, slices.Contains(fetchRefRules, rule)
}

// Expand returns the full ref name r makes of ref.
//
// Fetching the full name selects the ref [FetchMatch] matched. Fetching ref
// alone can miss it, because `git fetch` reads a 40-hex name as an object ID.
func (r FetchRule) Expand(ref string) string {
	return fmt.Sprintf(string(r), ref)
}

// fetchRefRules are the rules `git fetch` tries for a ref, most preferred
// first.
var fetchRefRules = []FetchRule{
	"%s",
	"refs/%s",
	"refs/tags/%s",
	"refs/heads/%s",
	"refs/remotes/%s",
	"refs/remotes/%s/HEAD",
}

const refsTags = "refs/tags/"

// LsRemoteTags lists the tags remote advertises, with `git ls-remote`. A
// remote with no tags returns no results and no error.
func (g *GitRunner) LsRemoteTags(ctx context.Context, remote string) ([]LsRemoteResult, error) {
	results, err := g.LsRemote(ctx, remote, refsTags+"*")
	if errors.Is(err, ErrNoMatchingReference) {
		return nil, nil
	}

	return results, err
}

// LocalTags lists the tags in the configured working-directory repository, in
// the shape [GitRunner.LsRemote] reports them, without contacting a remote.
func (g *GitRunner) LocalTags(ctx context.Context) ([]LsRemoteResult, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	cmd := g.prepareCommand(ctx, "for-each-ref", "--format=%(objectname) %(refname)", refsTags)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return nil, &WrappedError{
			Op:      "git_for_each_ref",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	var results []LsRemoteResult

	for line := range strings.Lines(stdout.String()) {
		parts := strings.Fields(line)
		if len(parts) < minGitPartsLength {
			continue
		}

		results = append(results, LsRemoteResult{Hash: parts[0], Ref: parts[1]})
	}

	return results, nil
}

// ReleaseTags returns the release tags among refs, highest version first. A
// release tag is a refs/tags/ name that parses as a version with no
// pre-release suffix. The peeled `^{}` entries ls-remote lists for annotated
// tags are left out.
func ReleaseTags(refs []LsRemoteResult) []LsRemoteResult {
	type release struct {
		version *semver.Version
		ref     LsRemoteResult
	}

	releases := make([]release, 0, len(refs))

	for _, ref := range refs {
		name, isTag := strings.CutPrefix(ref.Ref, refsTags)
		if !isTag || strings.HasSuffix(name, "^{}") {
			continue
		}

		version, err := semver.Parse(name)
		if err != nil || version.Prerelease() != "" {
			continue
		}

		releases = append(releases, release{version: version, ref: ref})
	}

	slices.SortStableFunc(releases, func(a, b release) int {
		return b.version.Compare(a.version)
	})

	out := make([]LsRemoteResult, 0, len(releases))
	for _, r := range releases {
		out = append(out, r.ref)
	}

	return out
}

// LatestReleaseTag returns the name of the highest release tag among refs, as
// [ReleaseTags] orders them, or "" when refs holds none.
func LatestReleaseTag(refs []LsRemoteResult) string {
	releases := ReleaseTags(refs)
	if len(releases) == 0 {
		return ""
	}

	return strings.TrimPrefix(releases[0].Ref, refsTags)
}

// Clone performs a git clone operation
func (g *GitRunner) Clone(
	ctx context.Context,
	repo string,
	bare bool,
	depth int,
	branch string,
) error {
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

	args = append(args, "--", repo, g.WorkDir)

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

// InitBare runs `git init --bare` in the configured working directory.
// `git init --bare` is itself idempotent (it reinitializes an existing bare
// repo as a no-op), so callers may invoke this freely.
func (g *GitRunner) InitBare(ctx context.Context) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	cmd := g.prepareCommand(ctx, "init", "--bare", g.WorkDir)

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_init_bare",
			Context: stderr.String(),
			Err:     errors.Join(ErrGitInitBare, err),
		}
	}

	return nil
}

// Fetch runs `git fetch` for a single ref against the given remote URL. A
// positive depth adds --depth and --no-tags. A zero or negative depth fetches
// full history.
func (g *GitRunner) Fetch(ctx context.Context, repo, ref string, depth int) error {
	args := []string{}

	if depth > 0 {
		args = append(args, "--depth", strconv.Itoa(depth), "--no-tags")
	}

	return g.fetch(ctx, repo, ref, args)
}

// FetchUnshallow runs `git fetch --unshallow` for a single ref against the
// given remote URL, bringing in the history a previous depth-limited fetch
// stopped at. Git rejects it on a repository that has no shallow boundary,
// so callers gate it on [GitRunner.IsShallow].
func (g *GitRunner) FetchUnshallow(ctx context.Context, repo, ref string) error {
	return g.fetch(ctx, repo, ref, []string{"--unshallow"})
}

// IsShallow reports whether the configured working-directory repository has
// a shallow boundary, which a fetch must unshallow to reach older history.
func (g *GitRunner) IsShallow(ctx context.Context) (bool, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return false, err
	}

	cmd := g.prepareCommand(ctx, "rev-parse", "--is-shallow-repository")

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return false, &WrappedError{
			Op:      "git_rev_parse",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return strings.TrimSpace(stdout.String()) == "true", nil
}

// RevParseCommit resolves ref to its canonical commit hash in the
// configured working-directory repository. ref may be a full SHA
// (SHA-1 or SHA-256) or an abbreviated SHA that disambiguates inside
// the repo. The `^{commit}` peeling suffix is what enforces that the
// object actually exists locally and is a commit; plain rev-parse
// (even with --verify) only checks revision syntax and would let a
// caller treat an empty bare repo as already containing the commit.
// A non-zero exit returns [ErrUnknownRevision] so callers can branch
// on the typed error without parsing stderr.
func (g *GitRunner) RevParseCommit(ctx context.Context, ref string) (string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return "", err
	}

	cmd := g.prepareCommand(ctx, "rev-parse", "--verify", ref+"^{commit}")

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		if vexec.ExitCode(err) > 0 {
			return "", &WrappedError{
				Op:      "git_rev_parse",
				Context: strings.TrimSpace(stderr.String()),
				Err:     ErrUnknownRevision,
			}
		}

		return "", &WrappedError{
			Op:      "git_rev_parse",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return strings.TrimSpace(stdout.String()), nil
}

// HasObject reports whether the given object exists in the configured
// working-directory repository. Exit code 1 from `git cat-file -e` means
// the object is absent. Other non-zero exits (e.g. 128 for a corrupted
// repo or unreadable .git) are returned as errors so callers do not loop
// into a refetch against a broken store.
func (g *GitRunner) HasObject(ctx context.Context, hash string) (bool, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return false, err
	}

	cmd := g.prepareCommand(ctx, "cat-file", "-e", hash)

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		if vexec.ExitCode(err) == catFileMissingExitCode {
			return false, nil
		}

		return false, &WrappedError{
			Op:      "git_cat_file_exists",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return true, nil
}

// CreateTempDir creates a new temporary directory for git operations
func (g *GitRunner) CreateTempDir(v *venv.Venv) (string, func() error, error) {
	v.RequireFS()
	v.RequireTempDir()

	prefix := "terragrunt-cas-"

	// Add a timestamp to the prefix to avoid conflicts
	prefix += strconv.FormatInt(time.Now().UnixNano(), 10)

	tempDir, err := vfs.MkdirTemp(v.FS, v.Platform.TempDir(), prefix)
	if err != nil {
		return "", nil, &WrappedError{
			Op:      "create_temp_dir",
			Context: err.Error(),
			Err:     ErrCreateTempDir,
		}
	}

	g.WorkDir = tempDir

	cleanup := func() error {
		if err := v.FS.RemoveAll(tempDir); err != nil {
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

// WorktreeCheckout selects whether [GitRunner.CreateDetachedWorktree] fills the
// worktree it creates.
type WorktreeCheckout int

const (
	// CheckoutFiles has git write the files of the reference into the worktree.
	CheckoutFiles WorktreeCheckout = iota
	// SkipCheckout leaves the worktree empty, for a caller that fills it
	// itself, such as with [GitRunner.CheckoutPaths].
	SkipCheckout
)

// TreePaths is the set of paths in a tree, in the slash-separated form git
// reports them in.
type TreePaths map[string]struct{}

// Has reports whether the tree contains path.
func (t TreePaths) Has(path string) bool {
	_, ok := t[path]

	return ok
}

// HasOrContains reports whether the tree holds path itself or anything under
// it, which is what a pathspec naming path selects.
func (t TreePaths) HasOrContains(path string) bool {
	if t.Has(path) {
		return true
	}

	prefix := path + "/"

	for treePath := range t {
		if strings.HasPrefix(treePath, prefix) {
			return true
		}
	}

	return false
}

// LsTreeNames returns every path in the tree at ref, recursively, which tells a
// caller what a reference contains without checking it out.
func (g *GitRunner) LsTreeNames(ctx context.Context, v *venv.Venv, ref string) (TreePaths, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	// Run from the repository root: git limits a listing to the directory it
	// runs in, and callers may be working from a subdirectory. -z keeps a path
	// with unusual characters intact, which git would otherwise quote.
	root, err := GoRepoRoot(ctx, v, g.WorkDir)
	if err != nil {
		return nil, err
	}

	cmd := g.prepareCommand(ctx, "ls-tree", "-r", "--name-only", "-z", ref)
	cmd.SetDir(root)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return nil, &WrappedError{
			Op:      "git_ls_tree_names",
			Context: stderr.String(),
			Err:     errors.Join(ErrReadTree, err),
		}
	}

	paths := make(TreePaths)

	for name := range strings.SplitSeq(strings.TrimSuffix(stdout.String(), "\x00"), "\x00") {
		if name == "" {
			continue
		}

		paths[name] = struct{}{}
	}

	return paths, nil
}

// CreateDetachedWorktree creates a new detached worktree for a given reference
// as a given directory
func (g *GitRunner) CreateDetachedWorktree(
	ctx context.Context,
	v *venv.Venv,
	dir, ref string,
	checkout WorktreeCheckout,
) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	args := []string{
		"-c", "checkout.workers=" + strconv.Itoa(vfs.FSWorkersFor(v.FS, dir)),
		"worktree", "add", "--detach",
	}
	if checkout == SkipCheckout {
		args = append(args, "--no-checkout")
	}

	args = append(args, dir, ref)

	cmd := g.prepareCommand(ctx, args[0], args[1:]...)

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

// CheckoutPaths writes the paths the working directory's HEAD holds into the
// working directory and stages them in its index. Given pathspecs, only the
// paths they name are written; without any, the whole tree is. The working
// directory is expected to be a worktree registered without a checkout, so
// the checkout fills it rather than switching to another reference. Relative
// references such as HEAD~1 name the worktree's own HEAD, so the checkout
// runs against HEAD rather than against the reference the caller holds.
func (g *GitRunner) CheckoutPaths(
	ctx context.Context,
	v *venv.Venv,
	pathspecs ...string,
) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	target := []string{"--force", "HEAD"}
	if len(pathspecs) > 0 {
		target = append([]string{"HEAD", "--"}, pathspecs...)
	}

	args := slices.Concat(
		[]string{
			"-c",
			"checkout.workers=" + strconv.Itoa(vfs.FSWorkersFor(v.FS, g.WorkDir)),
			"checkout",
		},
		target,
	)

	cmd := g.prepareCommand(ctx, args[0], args[1:]...)

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_checkout_paths",
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

		if symref, ok := strings.CutPrefix(line, "ref:"); ok {
			if fields := strings.Fields(symref); len(fields) > 0 {
				if after, ok := strings.CutPrefix(fields[0], "refs/heads/"); ok {
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
		return "sha1", nil //nolint:nilerr // older Git lacks --show-object-format, and only ever wrote sha1
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Add stages the given paths in the working directory.
func (g *GitRunner) Add(ctx context.Context, paths ...string) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	cmd := g.prepareCommand(ctx, "add", paths...)

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_add",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// Commit creates a commit with the given message. The working directory must
// have a user.email and user.name configured (use [GitRunner.ConfigSet]).
func (g *GitRunner) Commit(ctx context.Context, message string, flags ...string) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	commitArgs := []string{"-m", message}
	args := make([]string, 0, len(flags)+len(commitArgs))
	args = append(args, flags...)
	args = append(args, commitArgs...)

	cmd := g.prepareCommand(ctx, "commit", args...)

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_commit",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// Checkout checks out a branch. Pass create=true to create it first.
func (g *GitRunner) Checkout(ctx context.Context, branch string, create bool) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	var args []string
	if create {
		args = append(args, "-b")
	}

	args = append(args, branch)
	cmd := g.prepareCommand(ctx, "checkout", args...)

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_checkout",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// ConfigSet writes a git configuration value in the working directory.
func (g *GitRunner) ConfigSet(ctx context.Context, name, value string) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	cmd := g.prepareCommand(ctx, "config", name, value)

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_config_set",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return nil
}

// fetch runs `git fetch` with args placed ahead of the remote and refspec.
func (g *GitRunner) fetch(ctx context.Context, repo, ref string, args []string) error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	args = append(args, "--", repo, ref)

	cmd := g.prepareCommand(ctx, "fetch", args...)

	var stderr bytes.Buffer

	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return &WrappedError{
			Op:      "git_fetch",
			Context: stderr.String(),
			Err:     errors.Join(ErrGitFetch, err),
		}
	}

	return nil
}

func (g *GitRunner) prepareCommand(ctx context.Context, name string, args ...string) vexec.Cmd {
	ctx = vexec.WithTrustedCommand(ctx)

	cmd := g.exec.Command(ctx, g.GitPath, append([]string{name}, args...)...)
	cmd.SetEnv(venv.Environ(g.env))
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
