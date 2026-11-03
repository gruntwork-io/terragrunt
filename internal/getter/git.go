package getter

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/detect"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	getter "github.com/hashicorp/go-getter/v2"
)

// ErrGitUnavailable is returned when the venv's exec cannot find a git binary.
var ErrGitUnavailable = errors.New("git must be available and on the PATH")

// ErrInvalidGitSource is returned when a git source's port, ref or sshkey
// cannot be used.
var ErrInvalidGitSource = errors.New("invalid git source")

// ErrGitTooOldForSSHKey is returned when a source sets the sshkey query
// parameter and git is older than [MinGitVersionForSSHKey].
var ErrGitTooOldForSSHKey = errors.New("using an ssh key needs a newer git")

// MinGitVersionForSSHKey is the first git version that reads GIT_SSH_COMMAND.
// The getter passes the sshkey to ssh through that variable.
var MinGitVersionForSSHKey = semver.MustParse("2.3")

const (
	// EnvGitSSHCommand is the variable git reads its ssh invocation from.
	EnvGitSSHCommand = "GIT_SSH_COMMAND"

	// sshKeyPerms keeps a decoded sshkey readable by its owner only.
	sshKeyPerms = 0o600
)

// defaultGitDetectors mirrors the v2 default upstream git getter detector chain.
func defaultGitDetectors() []Detector {
	return []Detector{
		new(GitHubDetector),
		new(GitDetector),
		new(detect.BitBucket),
		new(GitLabDetector),
	}
}

// GitGetter is Terragrunt's git-protocol getter.
//
// It downloads the way go-getter's git getter does. It honors the ref, depth
// and sshkey query parameters and refreshes a destination that already
// exists.
//
// It runs every git command through the venv's exec and reaches the
// filesystem through the venv's FS. A venv whose exec refuses subprocesses
// refuses git downloads too.
//
// Symlinks inside a cloned tree are kept whatever req.DisableSymlinks says.
type GitGetter struct {
	v        *venv.Venv
	detector *getter.GitGetter
}

// NewGitGetter returns a GitGetter that runs git through v.
func NewGitGetter(v *venv.Venv) *GitGetter {
	v.RequireExec()
	v.RequireFS()
	v.RequireEnv()
	v.RequireTempDir()

	return &GitGetter{
		v:        v,
		detector: &getter.GitGetter{Detectors: defaultGitDetectors()},
	}
}

// Mode reports that a git source is always a directory.
func (g *GitGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

// Detect reports whether req's source is a git address, using go-getter's
// git detectors.
func (g *GitGetter) Detect(req *getter.Request) (bool, error) {
	return g.detector.Detect(req)
}

// Get clones the repository at req's URL into req.Dst, or refreshes the
// checkout already there, then checks out the requested ref and the
// submodules.
func (g *GitGetter) Get(ctx context.Context, req *getter.Request) error {
	src, err := parseGitSource(req.URL())
	if err != nil {
		return err
	}

	return redactSourceError(g.get(ctx, src, req.Dst), src.url.String())
}

// GetFile downloads the repository that contains the file at req's URL, then
// copies that file to req.Dst.
func (g *GitGetter) GetFile(ctx context.Context, req *getter.Request) error {
	return getFileFromCheckout(ctx, g.v, req, g.Get)
}

func (g *GitGetter) get(ctx context.Context, src *gitSource, dst string) (err error) {
	v := g.v

	keyFile := ""

	if src.sshKey != "" {
		keyFile, err = writeSSHKey(v, src.sshKey)
		if err != nil {
			return err
		}

		defer func() {
			if rmErr := v.FS.Remove(keyFile); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				err = errors.Join(err, rmErr)
			}
		}()
	}

	v = v.WithEnv(gitSSHEnv(v.Env, keyFile))

	runner, err := git.NewGitRunner(v)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrGitUnavailable, err)
	}

	if keyFile != "" {
		if err := requireGitVersion(ctx, runner, MinGitVersionForSSHKey); err != nil {
			return err
		}
	}

	checkout := runner.WithWorkDir(dst)

	_, statErr := v.FS.Stat(dst)

	switch {
	case statErr == nil:
		err = updateGitCheckout(ctx, v.FS, checkout, src)
	case errors.Is(statErr, fs.ErrNotExist):
		err = cloneInto(ctx, v.FS, checkout, src)
	default:
		return statErr
	}

	if err != nil {
		return err
	}

	if src.ref != "" {
		if err := checkout.Checkout(ctx, src.ref, false); err != nil {
			return err
		}
	}

	return checkout.SubmoduleUpdate(ctx, src.depth)
}

// gitSource is a git source split into the address git fetches and the query
// parameters the getter consumes.
type gitSource struct {
	url    *url.URL
	ref    string
	sshKey string
	depth  int
}

// parseGitSource splits u into a [gitSource] without changing u. The ref,
// sshkey and depth query parameters are removed from the URL passed to git.
//
// A port above 65535 is refused. A ref that starts with a dash is refused
// because git would read it as an option.
func parseGitSource(u *url.URL) (*gitSource, error) {
	if port := u.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return nil, fmt.Errorf("%w: port %q is out of range", ErrInvalidGitSource, port)
		}
	}

	cleaned := *u
	q := cleaned.Query()

	src := &gitSource{
		url:    &cleaned,
		ref:    q.Get("ref"),
		sshKey: q.Get("sshkey"),
	}

	if n, err := strconv.Atoi(q.Get("depth")); err == nil {
		src.depth = n
	}

	if strings.HasPrefix(src.ref, "-") {
		return nil, fmt.Errorf("%w: ref %q starts with a dash", ErrInvalidGitSource, src.ref)
	}

	q.Del("ref")
	q.Del("sshkey")
	q.Del("depth")

	cleaned.RawQuery = q.Encode()

	return src, nil
}

// cloneInto creates runner's working dir and clones src into it. A failed
// clone removes the dir again, so the next download clones afresh rather than
// refreshing it.
func cloneInto(ctx context.Context, fsys vfs.FS, runner *git.GitRunner, src *gitSource) error {
	if err := fsys.MkdirAll(runner.WorkDir, checkoutDirPerms); err != nil {
		return err
	}

	err := runner.Clone(ctx, src.url.String(), false, src.depth, "")
	if err == nil {
		return nil
	}

	return errors.Join(err, fsys.RemoveAll(runner.WorkDir))
}

// updateGitCheckout refreshes the checkout at runner's working dir from src.
//
// It discards the old repository metadata and fetches into a fresh one, so a
// checkout made from another remote is replaced rather than merged. With no
// ref it checks out the branch that the remote's HEAD points to.
func updateGitCheckout(ctx context.Context, fsys vfs.FS, runner *git.GitRunner, src *gitSource) error {
	if err := removeGitDirs(fsys, runner.WorkDir); err != nil {
		return err
	}

	remote := src.url.String()

	ref := src.ref
	if ref == "" {
		branch, err := runner.LsRemoteDefaultBranch(ctx, remote)
		if err != nil {
			return err
		}

		ref = branch
	}

	if err := runner.Init(ctx); err != nil {
		return err
	}

	if err := runner.RemoteAdd(ctx, "origin", remote); err != nil {
		return err
	}

	if err := runner.FetchTags(ctx, "origin"); err != nil {
		return err
	}

	if err := runner.Fetch(ctx, "origin", ref, 0); err != nil {
		return err
	}

	if err := runner.ResetHard(ctx, "FETCH_HEAD"); err != nil {
		return err
	}

	if err := runner.Checkout(ctx, ref, false); err != nil {
		return err
	}

	return runner.PullFastForward(ctx, "origin", ref, src.depth)
}

// removeGitDirs removes every directory directly under dir whose name is .git
// in any letter case, since a case-sensitive filesystem can have more than one.
func removeGitDirs(fsys vfs.FS, dir string) error {
	entries, err := vfs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("reading %s to refresh its git checkout: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.EqualFold(entry.Name(), ".git") {
			continue
		}

		if err := fsys.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return fmt.Errorf("removing %s to refresh its git checkout: %w", entry.Name(), err)
		}
	}

	return nil
}

// requireGitVersion fails unless the git runner runs is at least minVersion.
func requireGitVersion(ctx context.Context, runner *git.GitRunner, minVersion *semver.Version) error {
	have, err := runner.Version(ctx)
	if err != nil {
		return err
	}

	if have.LessThan(minVersion) {
		return fmt.Errorf("%w: need git %s, have %s", ErrGitTooOldForSSHKey, minVersion, have)
	}

	return nil
}

// writeSSHKey decodes the base64 sshkey query value into a temporary file
// only its owner can read, and returns the file's path.
func writeSSHKey(v *venv.Venv, encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: sshkey is not base64", ErrInvalidGitSource)
	}

	f, err := vfs.CreateTemp(v.FS, v.Platform.TempDir(), "terragrunt-git-sshkey-")
	if err != nil {
		return "", err
	}

	path := f.Name()

	if err := v.FS.Chmod(path, sshKeyPerms); err != nil {
		return "", errors.Join(err, f.Close(), v.FS.Remove(path))
	}

	if _, err := f.Write(raw); err != nil {
		return "", errors.Join(err, f.Close(), v.FS.Remove(path))
	}

	if err := f.Close(); err != nil {
		return "", errors.Join(err, v.FS.Remove(path))
	}

	return path, nil
}

// gitSSHEnv returns a copy of env with GIT_SSH_COMMAND set to the ssh command
// already in env, or plain ssh, followed by `-i keyFile` when keyFile is not
// empty.
//
// The variable is set even without a key, so it overrides a core.sshCommand
// in git config.
func gitSSHEnv(env map[string]string, keyFile string) map[string]string {
	sshCmd := env[EnvGitSSHCommand]
	if sshCmd == "" {
		sshCmd = "ssh"
	}

	if keyFile != "" {
		sshCmd += " -i " + filepath.ToSlash(keyFile)
	}

	out := maps.Clone(env)
	out[EnvGitSSHCommand] = sshCmd

	return out
}
