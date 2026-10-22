package test_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/portaltest"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const (
	portalLoginCommand   = "terragrunt --experiment tg-login login"
	portalCatalogCommand = "terragrunt --experiment tg-login catalog --format jsonl --working-dir "

	portalOrgName      = "Acme"
	portalAccountEmail = "you@example.com"

	// portalFirstUserCode is the code the double gives its first login request.
	portalFirstUserCode = "TEST-0001"

	// portalDecisionDelay has the user answer between the first poll and the
	// second, so the first is answered authorization_pending.
	portalDecisionDelay = portaltest.DefaultInterval + time.Second
)

// TestPortalLoginThenCatalog walks the whole grant against the portal double:
// the device request, the user approving it on the verification page, the
// poll that collects the token, and a catalog run that sends it as a bearer and
// lists the repository the portal names.
func TestPortalLoginThenCatalog(t *testing.T) {
	t.Parallel()

	repoDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "repo")

	synctest.Test(t, func(t *testing.T) {
		p := portaltest.New(t, &portaltest.Config{
			OrgName:      portalOrgName,
			AccountEmail: portalAccountEmail,
			Repositories: []string{filepath.ToSlash(repoDir)},
		})
		srv := httptest.NewTestServer(t, p)

		var opened string

		out, v := newPortalVenv(t, srv.Client(), portalUser(t, srv.Client(), portaltest.Approve, &opened))

		writePortalRepo(t, v, repoDir)

		require.NoError(t, helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, portalLoginCommand))

		assert.Equal(t,
			portal.DefaultBaseURL+portaltest.VerificationPath+"?"+portaltest.UserCodeParam+"="+portalFirstUserCode,
			opened,
		)
		assert.Contains(t, out.String(), portalFirstUserCode)
		assert.Contains(t, out.String(), "Signed in as "+portalAccountEmail+" ("+portalOrgName+")")

		credentials, err := portal.LoadCredentials(logger.CreateLogger(), v, portal.DefaultBaseURL)
		require.NoError(t, err)
		require.Len(t, credentials.Valid, 1)

		out.Reset()

		require.NoError(t, helpers.RunTerragruntCommandWithVenv(
			t, t.Context(), v, portalCatalogCommand+helpers.TmpDirWOSymlinks(t),
		))

		assert.Equal(t, []string{"alpha", "bravo"}, portalComponentDirs(t, out.String()))

		stats := p.Stats()
		assert.Empty(t, stats.Violations)
		assert.Equal(t, 1, stats.Authorizations)
		assert.Equal(t, 1, stats.PendingPolls)
		assert.Zero(t, stats.SlowDowns)
		assert.Equal(t, 1, stats.TokensIssued)
		assert.Equal(t, 1, stats.CatalogServed)
		assert.Zero(t, stats.CatalogRejected)
	})
}

// TestPortalLoginKeepsThePaceThePortalSets pins that a login the portal
// throttles widens its poll interval by what each slow_down asks, and that
// none of its later polls arrive before the widened interval is up.
func TestPortalLoginKeepsThePaceThePortalSets(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const slowDowns = 2

		p := portaltest.New(t, &portaltest.Config{
			OrgName:      portalOrgName,
			AccountEmail: portalAccountEmail,
			SlowDowns:    slowDowns,
		})
		srv := httptest.NewTestServer(t, p)

		var opened string

		_, v := newPortalVenv(t, srv.Client(), portalUser(t, srv.Client(), portaltest.Approve, &opened))

		start := time.Now()

		require.NoError(t, helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, portalLoginCommand))

		assert.Equal(t, 5*time.Second+10*time.Second+15*time.Second, time.Since(start))

		stats := p.Stats()
		assert.Empty(t, stats.Violations)
		assert.Equal(t, slowDowns, stats.SlowDowns)
		assert.Zero(t, stats.TooFastPolls)
		assert.Equal(t, 1, stats.TokensIssued)
	})
}

// TestPortalLoginEndsWithoutACredential pins that a login the user refuses, or
// never answers, ends in the error that says which, files no credential, and
// leaves a later catalog run with nothing to send the portal.
func TestPortalLoginEndsWithoutACredential(t *testing.T) {
	t.Parallel()

	tc := []struct {
		wantErr  error
		name     string
		decision portaltest.Decision
	}{
		{
			name:     "the user denies the request",
			decision: portaltest.Deny,
			wantErr:  portal.ErrLoginDenied,
		},
		{
			name:    "the request expires unanswered",
			wantErr: portal.ErrLoginExpired,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				p := portaltest.New(t, &portaltest.Config{
					OrgName:      portalOrgName,
					AccountEmail: portalAccountEmail,
					ExpiresIn:    time.Minute,
				})
				srv := httptest.NewTestServer(t, p)

				var opened string

				browser := portalUser(t, srv.Client(), tt.decision, &opened)
				out, v := newPortalVenv(t, srv.Client(), browser)

				err := helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, portalLoginCommand)
				require.ErrorIs(t, err, tt.wantErr)

				credentials, err := portal.LoadCredentials(logger.CreateLogger(), v, portal.DefaultBaseURL)
				require.NoError(t, err)
				assert.Empty(t, credentials.Valid)

				out.Reset()

				require.NoError(t, helpers.RunTerragruntCommandWithVenv(
					t, t.Context(), v, portalCatalogCommand+helpers.TmpDirWOSymlinks(t),
				))
				assert.Empty(t, out.String())

				stats := p.Stats()
				assert.Empty(t, stats.Violations)
				assert.Zero(t, stats.TokensIssued)
				assert.Zero(t, stats.CatalogServed+stats.CatalogRejected, "the catalog had no credential to send")
			})
		})
	}
}

// TestPortalCatalogWithoutAWorkingCredential pins what a catalog run does once
// the credential a login filed stops working. One the CLI knows has expired is
// never sent. One the portal withdrew early is sent and refused, and the run
// still succeeds without the portal's repositories.
func TestPortalCatalogWithoutAWorkingCredential(t *testing.T) {
	t.Parallel()

	tc := []struct {
		invalidate   func(p *portaltest.Portal)
		name         string
		wantRejected int
	}{
		{
			name: "the credential expired",
			invalidate: func(*portaltest.Portal) {
				synctest.Sleep(portaltest.DefaultTokenLifetime)
			},
		},
		{
			name:         "the portal revoked the credential",
			invalidate:   (*portaltest.Portal).Revoke,
			wantRejected: 1,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "repo")

			synctest.Test(t, func(t *testing.T) {
				p := portaltest.New(t, &portaltest.Config{
					OrgName:      portalOrgName,
					AccountEmail: portalAccountEmail,
					Repositories: []string{filepath.ToSlash(repoDir)},
				})
				srv := httptest.NewTestServer(t, p)

				var opened string

				out, v := newPortalVenv(t, srv.Client(), portalUser(t, srv.Client(), portaltest.Approve, &opened))

				writePortalRepo(t, v, repoDir)

				require.NoError(t, helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, portalLoginCommand))

				tt.invalidate(p)
				out.Reset()

				require.NoError(t, helpers.RunTerragruntCommandWithVenv(
					t, t.Context(), v, portalCatalogCommand+helpers.TmpDirWOSymlinks(t),
				))
				assert.Empty(t, out.String())

				stats := p.Stats()
				assert.Empty(t, stats.Violations)
				assert.Zero(t, stats.CatalogServed)
				assert.Equal(t, tt.wantRejected, stats.CatalogRejected)
			})
		})
	}
}

// newPortalVenv returns a venv whose every request reaches the portal double
// through c, and whose credentials and clones land in directories of the
// test's own. The returned buffer collects standard output.
func newPortalVenv(t *testing.T, c vhttp.Client, browser vbrowser.Opener) (*strings.Builder, *venv.Venv) {
	t.Helper()

	configDir := t.TempDir()
	tempDir := t.TempDir()
	out := &strings.Builder{}

	v := venvtest.NewWithOSFS().
		WithHTTP(c).
		WithBrowser(browser).
		WithWriter(out).
		WithUserConfigDir(func() (string, error) { return configDir, nil }).
		WithTempDir(func() string { return tempDir })

	return out, v
}

// portalUser returns a browser that records the page the CLI opened in opened
// and, once the first poll is in, submits decision on it. An empty decision is
// a user who never answers.
func portalUser(t *testing.T, c vhttp.Client, decision portaltest.Decision, opened *string) vbrowser.Opener {
	t.Helper()

	ctx := t.Context()

	return vbrowser.NewMemOpener(func(_ context.Context, rawURL string) error {
		*opened = rawURL

		if decision == "" {
			return nil
		}

		go func() {
			time.Sleep(portalDecisionDelay)
			assert.NoError(t, portaltest.Decide(ctx, c, rawURL, decision))
		}()

		return nil
	})
}

// writePortalRepo writes a checked-out repository holding two components.
func writePortalRepo(t *testing.T, v *venv.Venv, repoDir string) {
	t.Helper()

	require.NoError(t, v.FS.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
	require.NoError(t, vfs.WriteFile(
		v.FS, filepath.Join(repoDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644,
	))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(repoDir, ".git", "config"), []byte(`[core]
	repositoryformatversion = 0
[remote "origin"]
	url = github.com/acme/repo
`), 0o644))

	for _, name := range []string{"alpha", "bravo"} {
		dir := filepath.Join(repoDir, name)

		require.NoError(t, v.FS.MkdirAll(dir, 0o755))
		require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(dir, "main.tf"), []byte("# "+name+"\n"), 0o644))
	}
}

// portalComponentDirs returns the directory of each component a jsonl catalog
// run listed, sorted, since repositories load concurrently.
func portalComponentDirs(t *testing.T, out string) []string {
	t.Helper()

	var dirs []string

	for line := range strings.Lines(out) {
		var entry format.Entry

		require.NoError(t, json.Unmarshal([]byte(line), &entry))

		dirs = append(dirs, entry.Dir)
	}

	slices.Sort(dirs)

	return dirs
}
