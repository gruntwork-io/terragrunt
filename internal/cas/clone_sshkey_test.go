package cas_test

import (
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/redact"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const sshKeyFixture = "test/fixtures/download/hello-world"

// startSSHKeyServer starts an SSH git server serving [sshKeyFixture]. The
// returned venv's ssh trusts that server but holds no key it accepts, so only
// an sshkey parameter can authenticate. The second value is that parameter's
// escaped value.
func startSSHKeyServer(t *testing.T) (*helpers.GitServer, string, *venv.Venv) {
	t.Helper()

	srv := helpers.NewGitServer(t)
	srv.AddFixtures(sshKeyFixture)
	srv.RequireSSH()

	v := venvtest.NewOSWithEmptyEnv().WithEnv(map[string]string{
		"PATH": os.Getenv("PATH"),
		git.EnvGitSSHCommand: "ssh -o BatchMode=yes -o IdentitiesOnly=yes " +
			"-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
	})

	return srv, url.QueryEscape(base64.StdEncoding.EncodeToString(srv.SSHPrivateKey())), v
}

// casGet downloads src into dst through a [getter.CASGetter] over c.
func casGet(t *testing.T, c *cas.CAS, v *venv.Venv, src, dst string) error {
	t.Helper()

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &cas.CloneOptions{})
	client := &getter.Client{Getters: []getter.Getter{g}}

	_, err := client.Get(t.Context(), &getter.Request{Src: src, Dst: dst, GetMode: getter.ModeDir})

	return err
}

func TestCASClone_E2E_ConnectsWithSSHKeyParam(t *testing.T) {
	srv, sshKey, v := startSSHKeyServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	keyDir := filepath.Join(tempDir, "tmp")
	require.NoError(t, os.Mkdir(keyDir, 0o700))
	t.Setenv("TMPDIR", keyDir)

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	require.Error(t, casGet(t, c, v, "git::"+srv.SSHURL+"?ref=main", filepath.Join(tempDir, "keyless")),
		"the server must refuse a clone without the key, or this test proves nothing")

	dst := filepath.Join(tempDir, "dst")
	require.NoError(t, casGet(t, c, v, "git::"+srv.SSHURL+"?ref=main&sshkey="+sshKey, dst))

	assert.FileExists(t, filepath.Join(dst, sshKeyFixture, "main.tf"))

	leftover, err := filepath.Glob(filepath.Join(keyDir, "terragrunt-git-sshkey-*"))
	require.NoError(t, err)
	assert.Empty(t, leftover, "the key file must be removed after the clone")
}

//nolint:paralleltest // RequireSSH changes the process environment.
func TestProcessStackComponent_ConnectsWithSSHKeyParam(t *testing.T) {
	srv, sshKey, v := startSSHKeyServer(t)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	source := "git::" + srv.SSHURL + "//" + sshKeyFixture + "?ref=main&sshkey=" + sshKey

	result, err := c.ProcessStackComponent(t.Context(), logger.CreateLogger(), v, source, "unit")
	require.NoError(t, err)

	defer result.Cleanup()

	assert.FileExists(t, filepath.Join(result.ContentDir, "main.tf"))
}

// TestCASClone_E2E_SSHKeyCallersAvoidKeylessFlightsWithRacing pins that a
// caller with a key is never left waiting on a flight that a caller without
// one started, which would fail it with the other caller's authentication
// error. Callers without a key may fail or may read what a keyed caller stored,
// so only the keyed outcomes are asserted.
//
//nolint:paralleltest // RequireSSH changes the process environment.
func TestCASClone_E2E_SSHKeyCallersAvoidKeylessFlightsWithRacing(t *testing.T) {
	const (
		rounds  = 5
		callers = 8
	)

	srv, sshKey, v := startSSHKeyServer(t)

	for range rounds {
		tempDir := helpers.TmpDirWOSymlinks(t)

		c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(filepath.Join(tempDir, "store")))
		require.NoError(t, err)

		var g errgroup.Group

		for i := range callers {
			dst := filepath.Join(tempDir, "dst", string(rune('a'+i)))

			if i%2 == 0 {
				g.Go(func() error {
					return casGet(t, c, v, "git::"+srv.SSHURL+"?ref=main&sshkey="+sshKey, dst)
				})

				continue
			}

			g.Go(func() error {
				if err := casGet(t, c, v, "git::"+srv.SSHURL+"?ref=main", dst); err != nil {
					t.Logf("keyless caller failed as expected: %v", err)
				}

				return nil
			})
		}

		require.NoError(t, g.Wait())
	}
}

func TestCASClone_E2E_RefusesSSHKeyThatIsNotBase64(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	err = c.Clone(t.Context(), logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(), redact.NewURL(repoURL),
		cas.WithDir(filepath.Join(tempDir, "dst")),
		cas.WithSSHKey("not base64!"))
	require.ErrorIs(t, err, git.ErrInvalidSSHKey)
}
