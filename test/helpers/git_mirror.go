package helpers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/require"
)

const (
	// MirrorURLPlaceholder is replaced by the live HTTP mirror URL
	// during [TerragruntMirror.RenderFixture]. Fixtures that previously
	// held a `https://github.com/gruntwork-io/terragrunt.git` URL embed
	// this placeholder instead.
	MirrorURLPlaceholder = "__MIRROR_URL__"

	// MirrorSSHURLPlaceholder is replaced by the live SSH mirror URL
	// (of form `ssh://git@127.0.0.1:PORT/terragrunt.git`). SSH-specific
	// fixtures embed this placeholder instead of an SSH GitHub URL.
	MirrorSSHURLPlaceholder = "__MIRROR_SSH_URL__"

	// MirrorSHAPlaceholder is replaced by the mirror's HEAD commit hash.
	// Used by the one fixture that pins a commit SHA.
	MirrorSHAPlaceholder = "__MIRROR_SHA__"
)

// terragruntMirrorTags lists the git tag names integration test
// fixtures pin against. The mirror creates all of them at HEAD; tests
// don't depend on the historical content of any of these refs, only on
// the ability to clone the named ref.
var terragruntMirrorTags = []string{
	"v0.53.8",
	"v0.67.4",
	"v0.78.4",
	"v0.79.0",
	"v0.83.2",
	"v0.84.1",
	"v0.85.0",
	"v0.93.2",
	"v0.99.1",
}

// terragruntMirrorBranches lists non-default branches the mirror
// creates at HEAD. Tests that exercise URL-parser tolerance for
// slash-bearing ref names (e.g., source-map's slash-in-ref behavior)
// rely on these.
var terragruntMirrorBranches = []string{
	"fixture/test-fixtures",
}

// TerragruntMirror is an in-process git server pair (HTTP + optional
// SSH) that serves the current working tree's `test/fixtures/`
// directory as if it were the upstream `gruntwork-io/terragrunt`
// repository. Tests use it to avoid cloning the real repo from GitHub.
//
// SSHURL is empty when the local `ssh` client binary or
// `git-upload-pack` is unavailable. Callers that need SSH should
// invoke [TerragruntMirror.RequireSSH], which calls t.Skip when
// SSHURL is empty and otherwise wires `GIT_SSH_COMMAND` into the
// calling test's environment.
type TerragruntMirror struct {
	// URL is the HTTP endpoint of the mirror, of form
	// `http://127.0.0.1:PORT/terragrunt.git`. Substituted into
	// fixtures via [MirrorURLPlaceholder].
	URL string
	// SSHURL is the SSH endpoint of the mirror, or empty when the
	// SSH mirror is unavailable.
	SSHURL string
	// HeadSHA is the commit SHA of the mirror's HEAD on `main`.
	// Substituted into fixtures via [MirrorSHAPlaceholder].
	HeadSHA string
	// sshKeyPEM is the OpenSSH-format private key the SSH mirror
	// accepts. [TerragruntMirror.RequireSSH] writes it into the
	// calling test's t.TempDir() so each test gets a fresh on-disk
	// copy that's removed automatically when the test ends.
	sshKeyPEM []byte
}

// RequireSSH skips the calling test when the SSH mirror is unavailable
// (typically because `ssh` or `git-upload-pack` is not on PATH).
// Otherwise it writes the mirror's private key under t.TempDir() and
// points `GIT_SSH_COMMAND` at it via t.Setenv so `git` invocations
// during the test authenticate against the mirror. Both the key file
// and the env var are scoped to the test by the testing package — no
// leaked tmpfiles, no shared state. Because t.Setenv panics on
// parallel tests, callers must not invoke t.Parallel.
func (m *TerragruntMirror) RequireSSH(t *testing.T) {
	t.Helper()

	if m.SSHURL == "" {
		t.Skip("ssh mirror unavailable (ssh client or git-upload-pack missing)")
	}

	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(keyPath, m.sshKeyPEM, sshKeyFilePerm), "write ssh key")

	t.Setenv("GIT_SSH_COMMAND", fmt.Sprintf(
		"ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes",
		keyPath,
	))
}

var (
	mirrorOnce sync.Once
	mirrorRef  *TerragruntMirror
	mirrorErr  error
)

// StartTerragruntMirror returns the process-wide mirror, lazily
// initializing it on first call. The mirror is shared across tests
// because its content is read-only after startup.
func StartTerragruntMirror(t *testing.T) *TerragruntMirror {
	t.Helper()

	mirrorOnce.Do(func() {
		mirrorRef, mirrorErr = startTerragruntMirror()
	})

	require.NoError(t, mirrorErr, "start terragrunt mirror")

	return mirrorRef
}

// SourceURL returns a go-getter source string of the form
// `git::<mirror-url>//<subpath>?ref=<ref>`. Pass an empty `ref` to
// omit the query.
func (m *TerragruntMirror) SourceURL(subpath, ref string) string {
	src := "git::" + m.URL + "//" + strings.TrimPrefix(subpath, "/")
	if ref != "" {
		src += "?ref=" + ref
	}

	return src
}

// RenderFixture is a backwards-compatible wrapper around
// [CopyEnvironment]. The substitution now happens automatically inside
// [CopyEnvironment] whenever placeholders are present, so callers can
// drop the mirror-specific helper in favour of plain [CopyEnvironment].
// It is retained because several call sites read more clearly when they
// say "render this fixture against the mirror".
func (m *TerragruntMirror) RenderFixture(t *testing.T, fixturePath string, includeInCopy ...string) string {
	t.Helper()

	return CopyEnvironment(t, fixturePath, includeInCopy...)
}

// applyMirrorSubst replaces [MirrorURLPlaceholder],
// [MirrorSSHURLPlaceholder], and [MirrorSHAPlaceholder] in `*.hcl`,
// `*.tf`, and `*.tofu` files under root. A first pass scans for any
// placeholder so trees without one bail out before the mirror is
// started. Trees that need substitution lazy-start the mirror via
// [StartTerragruntMirror].
func applyMirrorSubst(t *testing.T, root string) {
	t.Helper()

	var found bool

	scanErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		switch filepath.Ext(d.Name()) {
		case ".hcl", ".tf", ".tofu":
		default:
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if bytes.Contains(data, []byte(MirrorURLPlaceholder)) ||
			bytes.Contains(data, []byte(MirrorSSHURLPlaceholder)) ||
			bytes.Contains(data, []byte(MirrorSHAPlaceholder)) {
			found = true

			return filepath.SkipAll
		}

		return nil
	})
	require.NoError(t, scanErr, "scan for mirror placeholders in %s", root)

	if !found {
		return
	}

	m := StartTerragruntMirror(t)

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		switch filepath.Ext(d.Name()) {
		case ".hcl", ".tf", ".tofu":
		default:
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if !bytes.Contains(contents, []byte(MirrorURLPlaceholder)) &&
			!bytes.Contains(contents, []byte(MirrorSSHURLPlaceholder)) &&
			!bytes.Contains(contents, []byte(MirrorSHAPlaceholder)) {
			return nil
		}

		contents = bytes.ReplaceAll(contents, []byte(MirrorURLPlaceholder), []byte(m.URL))
		contents = bytes.ReplaceAll(contents, []byte(MirrorSSHURLPlaceholder), []byte(m.SSHURL))
		contents = bytes.ReplaceAll(contents, []byte(MirrorSHAPlaceholder), []byte(m.HeadSHA))

		return os.WriteFile(path, contents, readWritePermissions)
	})
	require.NoError(t, walkErr, "render mirror placeholders in %s", root)
}

func startTerragruntMirror() (*TerragruntMirror, error) {
	srv, err := git.NewServer()
	if err != nil {
		return nil, fmt.Errorf("new server: %w", err)
	}

	// Bind and serve before committing so the mirror URL is known.
	// We need the URL to substitute __MIRROR_URL__ in fixture files
	// before they land in the mirror, otherwise modules cloned from
	// the mirror that reference other mirror fixtures (e.g.,
	// download/hello-world's inner module ref) would still hit
	// upstream github. Concurrent reads against an empty storage are
	// fine; sync.Once gates external callers until commits+tags are
	// in place.
	url, err := srv.Start(context.Background())
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	fixturesDir, err := locateFixturesDir()
	if err != nil {
		return nil, err
	}

	// Best-effort SSH mirror. If `ssh` or `git-upload-pack` is missing,
	// SSHURL stays empty and the few SSH-only tests skip via
	// RequireSSH. The SSH URL must be known *before* committing
	// fixtures so __MIRROR_SSH_URL__ can be substituted at bake time.
	sshURL, sshKeyPEM, sshErr := startSSHMirror(fixturesDir, url)
	if sshErr != nil {
		fmt.Fprintf(os.Stderr, "terragrunt SSH mirror unavailable: %v\n", sshErr)
	}

	if err := commitFixtureTree(srv, fixturesDir, url, sshURL); err != nil {
		return nil, fmt.Errorf("commit fixture tree: %w", err)
	}

	head, err := srv.Head()
	if err != nil {
		return nil, fmt.Errorf("resolve head: %w", err)
	}

	for _, tag := range terragruntMirrorTags {
		if err := srv.Tag(tag); err != nil {
			return nil, fmt.Errorf("tag %s: %w", tag, err)
		}
	}

	for _, branch := range terragruntMirrorBranches {
		if err := srv.Branch(branch); err != nil {
			return nil, fmt.Errorf("branch %s: %w", branch, err)
		}
	}

	return &TerragruntMirror{
		URL:       url,
		SSHURL:    sshURL,
		HeadSHA:   head,
		sshKeyPEM: sshKeyPEM,
	}, nil
}

func locateFixturesDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locate helper file via runtime.Caller")
	}

	return filepath.Join(filepath.Dir(thisFile), "..", "fixtures"), nil
}

// commitFixtureTree walks fixturesDir on disk, writes every file into
// srv's worktree at path `test/fixtures/<rel>`, then bulk-stages and
// creates a single commit. Skipped: `.terraform`, `.terragrunt-cache`,
// symlinks, `terraform.tfstate*`, `terragrunt-debug.tfvars.json`.
//
// For `*.hcl`, `*.tf`, and `*.tofu` files, [MirrorURLPlaceholder] is
// substituted with mirrorURL before commit so a clone of one fixture
// that references another fixture via __MIRROR_URL__ stays inside the
// mirror and never reaches upstream github.
//
// Bulk-staging via [AddOptions.All] is required because per-file
// `Add()` calls `Status()` on the entire worktree each time, which
// degrades to O(n²) on the ~3K fixture files.
func commitFixtureTree(srv *git.Server, fixturesDir, mirrorURL, mirrorSSHURL string) error {
	repo := srv.Repo()

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	walkErr := filepath.WalkDir(fixturesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == ".terraform" || d.Name() == ".terragrunt-cache" {
				return filepath.SkipDir
			}

			return nil
		}

		// Skip symlinks (e.g., regression benchmark fixtures point a
		// dependency-group dir at a template dir). They have no
		// bearing on the github-mirror tests and following them would
		// require duplicating the target into the mirror.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		name := d.Name()
		if name == "terragrunt-debug.tfvars.json" || strings.HasPrefix(name, "terraform.tfstate") {
			return nil
		}

		rel, err := filepath.Rel(fixturesDir, path)
		if err != nil {
			return err
		}

		repoPath := filepath.ToSlash(filepath.Join("test/fixtures", rel))

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		switch filepath.Ext(name) {
		case ".hcl", ".tf", ".tofu":
			data = bytes.ReplaceAll(data, []byte(MirrorURLPlaceholder), []byte(mirrorURL))
			data = bytes.ReplaceAll(data, []byte(MirrorSSHURLPlaceholder), []byte(mirrorSSHURL))
		}

		f, err := w.Filesystem.Create(repoPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", repoPath, err)
		}

		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("write %s: %w", repoPath, err)
		}

		if err := f.Close(); err != nil {
			return fmt.Errorf("close %s: %w", repoPath, err)
		}

		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	if err := w.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return fmt.Errorf("add all: %w", err)
	}

	sig := &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}

	if _, err := w.Commit("seed test/fixtures from working tree", &gogit.CommitOptions{
		Author:    sig,
		Committer: sig,
	}); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}
