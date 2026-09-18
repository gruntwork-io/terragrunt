package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const (
	portalBaseURL  = "https://portal.example.com"
	catalogPath    = "/api/v1/catalog"
	organizationID = "org_fake"
)

// errPortalReached is returned by a portal no test staged an answer for.
var errPortalReached = errors.New("the catalog reached the portal")

// TestRunLoadsComponentsFromAPortalRepository pins that a repository the portal
// names is browsed exactly as one the catalog block names, down to the source
// string a scaffold is generated from.
func TestRunLoadsComponentsFromAPortalRepository(t *testing.T) {
	t.Parallel()

	// The clone reads a repository on the real filesystem.
	rootDir := t.TempDir()
	repoDir := filepath.Join(rootDir, "repo")

	// Both producers name the repository with the spelling an HCL string can hold.
	repoURL := filepath.ToSlash(repoDir)

	fromPortal := renderCatalog(t, rootDir, repoDir, func(t *testing.T, v *venv.Venv) *venv.Venv {
		t.Helper()

		return withPortalCatalog(t, v, repoURL)
	})

	fromCatalogBlock := renderCatalog(t, rootDir, repoDir, func(t *testing.T, v *venv.Venv) *venv.Venv {
		t.Helper()

		writeCatalogBlock(t, v, rootDir, repoURL)

		return v
	})

	assert.Equal(t, []string{"alpha", "bravo"}, sortedDirs(t, fromPortal))
	assert.Equal(t, componentSources(t, fromCatalogBlock), componentSources(t, fromPortal),
		"a portal repository must scaffold from the same source string as a configured one")
}

// TestRunLoadsARepoNamedByThePortalAndTheConfigOnceWithRacing pins that the
// portal feeds the same channel the local discoverers do, so a repository two
// of them name at the same moment is not cloned twice. Neither producer knows
// how the other spells a repository, so the spellings below have to reach the
// same one.
func TestRunLoadsARepoNamedByThePortalAndTheConfigOnceWithRacing(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name         string
		portalSuffix string
	}{
		{name: "the same spelling", portalSuffix: ""},
		{name: "a trailing separator", portalSuffix: "/"},
		{name: "a git suffix", portalSuffix: ".git"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rootDir := t.TempDir()

			// A local path that does not exist fails in the getter without reaching the network.
			repoURL := filepath.ToSlash(filepath.Join(rootDir, "missing-repo"))

			v := withPortalCatalog(t, newPortalVenv(t), repoURL+tt.portalSuffix)

			writeCatalogBlock(t, v, rootDir, repoURL)

			err := catalog.Run(
				t.Context(), logger.CreateLogger(), v,
				newPortalOptions(t, rootDir, catalog.FormatJSONL), "",
			)

			var loadErr *tui.SourceLoadError

			require.ErrorAs(t, err, &loadErr)
			assert.Len(t, loadErr.Failures, 1)
			assert.Equal(t, 1, loadErr.Attempted)
		})
	}
}

// TestRunWithoutTheExperimentLeavesThePortalAlone pins that switching the
// experiment off stops the portal request, which is the only way a user who has
// signed in can stop sending a credential that is good for another 30 days.
func TestRunWithoutTheExperimentLeavesThePortalAlone(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	v := newPortalVenv(t).
		WithWriter(&buf).
		WithHTTP(vhttp.NewMemClient(
			func(context.Context, *http.Request) (*http.Response, error) {
				assert.Fail(t, "the catalog reached the portal with the experiment switched off")

				return nil, errPortalReached
			},
		))

	saveCredential(t, v, organizationID)

	opts := newOptions(t, t.TempDir(), catalog.FormatJSONL)
	opts.PortalBaseURL = portalBaseURL

	require.NoError(t, catalog.Run(t.Context(), logger.CreateLogger(), v, opts, ""))
	assert.Empty(t, buf.String())
}

// TestRunFetchesEveryOrganizationsCatalogWithRacing pins that a user signed in
// to more than one organization gets the repositories of all of them, and that
// the fetches those take share a channel and a logger without racing.
func TestRunFetchesEveryOrganizationsCatalogWithRacing(t *testing.T) {
	t.Parallel()

	// The clones read repositories on the real filesystem.
	rootDir := t.TempDir()
	first := filepath.Join(rootDir, "first")
	second := filepath.Join(rootDir, "second")

	var buf strings.Builder

	v := newPortalVenv(t).
		WithWriter(&buf).
		WithHTTP(portalAnsweringInTurn(t, catalogBody(t, first), catalogBody(t, second)))

	saveCredential(t, v, "org_first")
	saveCredential(t, v, "org_second")

	writeLocalRepo(t, v, first)
	writeLocalRepo(t, v, second)

	require.NoError(t, catalog.Run(
		t.Context(), logger.CreateLogger(), v,
		newPortalOptions(t, rootDir, catalog.FormatJSONL), "",
	))

	assert.Equal(t, []string{"alpha", "alpha", "bravo", "bravo"}, sortedDirs(t, buf.String()))
}

// TestRunWithoutACredentialLeavesThePortalAlone pins that a user who never
// signed in makes no portal request at all, and the run ends as it did before
// the portal was wired in.
func TestRunWithoutACredentialLeavesThePortalAlone(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	v := newPortalVenv(t).
		WithWriter(&buf).
		WithHTTP(vhttp.NewMemClient(
			func(context.Context, *http.Request) (*http.Response, error) {
				assert.Fail(t, "the catalog reached the portal without a credential")

				return nil, errPortalReached
			},
		))

	err := catalog.Run(
		t.Context(), logger.CreateLogger(), v,
		newPortalOptions(t, t.TempDir(), catalog.FormatJSONL), "",
	)

	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

// TestRunSurvivesAPortalThatServesNoCatalog pins that a portal refusing the
// fetch leaves the locally configured sources loading. A credential the portal
// no longer accepts, and an organization it holds no catalog for, must cost the
// user nothing beyond the repositories the portal would have added.
func TestRunSurvivesAPortalThatServesNoCatalog(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name   string
		body   string
		status int
	}{
		{
			name:   "no catalog for this organization",
			status: http.StatusNotFound,
			body:   `{"error":"not_found"}`,
		},
		{
			name:   "the credential is no longer accepted",
			status: http.StatusUnauthorized,
			body:   `{"error":"invalid_token"}`,
		},
		{
			name:   "the feature is switched off",
			status: http.StatusForbidden,
			body:   `{"error":"feature_not_enabled","message":"Not enabled for this organization."}`,
		},
		{
			name:   "the portal is broken",
			status: http.StatusInternalServerError,
			body:   `{"error":"server_error"}`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The clone reads a repository on the real filesystem.
			rootDir := t.TempDir()
			repoDir := filepath.Join(rootDir, "repo")

			var buf strings.Builder

			v := newPortalVenv(t).
				WithWriter(&buf).
				WithHTTP(portalAnswering(t, tt.status, tt.body))

			saveCredential(t, v, organizationID)
			writeLocalRepo(t, v, repoDir)
			writeCatalogBlock(t, v, rootDir, filepath.ToSlash(repoDir))

			err := catalog.Run(
				t.Context(), logger.CreateLogger(), v,
				newPortalOptions(t, rootDir, catalog.FormatJSONL), "",
			)

			require.NoError(t, err)
			assert.Equal(t, []string{"alpha", "bravo"}, sortedDirs(t, buf.String()))
		})
	}
}

// TestRunEndsWhenTheRunIsAbandonedMidFetchWithRacing pins the shutdown a third
// producer could have broken. The portal discoverer holds a network call the
// local two do not, and the loader loop cannot end until every producer has
// finished with the channel they share.
func TestRunEndsWhenTheRunIsAbandonedMidFetchWithRacing(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	v := newPortalVenv(t).WithHTTP(vhttp.NewMemClient(
		func(reqCtx context.Context, _ *http.Request) (*http.Response, error) {
			// The user quits while the fetch is still in flight, which is the
			// only moment the portal can hold the run open.
			cancel()
			<-reqCtx.Done()

			return nil, reqCtx.Err()
		},
	))

	saveCredential(t, v, organizationID)

	err := catalog.Run(
		ctx, logger.CreateLogger(), v,
		newPortalOptions(t, t.TempDir(), catalog.FormatJSONL), "",
	)

	require.NoError(t, err, "a run the user abandoned is not a failure")
	require.ErrorIs(t, ctx.Err(), context.Canceled, "the portal was never asked for the catalog")
}

// TestRunNamesTheRepositoriesItCouldNotClone pins what a user of a plain format
// is left with when a source fails. Nothing reaches standard output, so the
// error is the whole report.
func TestRunNamesTheRepositoriesItCouldNotClone(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	// A local path that does not exist fails in the getter without reaching the network.
	repoURL := filepath.Join(rootDir, "missing-repo")

	v := withPortalCatalog(t, newPortalVenv(t), repoURL)

	err := catalog.Run(
		t.Context(), logger.CreateLogger(), v,
		newPortalOptions(t, rootDir, catalog.FormatJSONL), "",
	)

	require.ErrorAs(t, err, new(*tui.SourceLoadError))
	assert.Contains(t, err.Error(), repoURL)
	assert.Contains(t, err.Error(), tui.SourceAccessHint)
}

// renderCatalog runs the catalog over a repository at repoDir and returns what
// the plain format wrote, with configure deciding how the run is told about it.
func renderCatalog(
	t *testing.T,
	rootDir, repoDir string,
	configure func(t *testing.T, v *venv.Venv) *venv.Venv,
) string {
	t.Helper()

	var buf strings.Builder

	v := configure(t, newPortalVenv(t).WithWriter(&buf))

	writeLocalRepo(t, v, repoDir)

	require.NoError(t, catalog.Run(
		t.Context(), logger.CreateLogger(), v,
		newPortalOptions(t, rootDir, catalog.FormatJSONL), "",
	))

	return buf.String()
}

// newPortalVenv returns a venv on the real filesystem. Clones and portal
// credentials land in directories of the test's own, so no other test and no
// developer's home directory sees them.
func newPortalVenv(t *testing.T) *venv.Venv {
	t.Helper()

	configDir := t.TempDir()
	tempDir := t.TempDir()

	return venvtest.NewWithOSFS().
		WithUserConfigDir(func() (string, error) { return configDir, nil }).
		WithTempDir(func() string { return tempDir })
}

// newPortalOptions returns catalog options addressed at the test portal, with
// the experiment the portal half of the catalog is gated behind switched on.
func newPortalOptions(t *testing.T, workDir, outputFormat string) *catalog.Options {
	t.Helper()

	opts := newOptions(t, workDir, outputFormat)
	opts.PortalBaseURL = portalBaseURL

	require.NoError(t, opts.Experiments.EnableExperiment(experiment.TGLogin))

	return opts
}

// withPortalCatalog signs the venv in and stages a portal serving repoURL.
func withPortalCatalog(t *testing.T, v *venv.Venv, repoURL string) *venv.Venv {
	t.Helper()

	v = v.WithHTTP(portalAnswering(t, http.StatusOK, catalogBody(t, repoURL)))

	saveCredential(t, v, organizationID)

	return v
}

// catalogBody is what a portal serving repoURL and nothing else answers with.
func catalogBody(t *testing.T, repoURL string) string {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"repositories": []map[string]string{{"url": repoURL}},
	})
	require.NoError(t, err)

	return string(body)
}

// saveCredential files the credential an earlier `terragrunt login` to orgID
// would have left behind.
func saveCredential(t *testing.T, v *venv.Venv, orgID string) {
	t.Helper()

	require.NoError(t, portal.SaveToken(logger.CreateLogger(), v, portalBaseURL, &portal.Token{
		AccessToken: portal.Secret("fake-access-token-" + orgID),
		TokenType:   "Bearer",
		Scope:       portal.ScopeCatalogRead,
		ExpiresIn:   time.Hour,
		Org:         portal.Org{ID: orgID, Name: "Acme"},
	}))
}

// portalAnswering builds a client answering the catalog endpoint, and only that
// endpoint, with the staged response. A run that never asked for the catalog
// fails the test, so a portal case cannot pass by never reaching the portal.
func portalAnswering(t *testing.T, status int, body string) vhttp.Client {
	t.Helper()

	var asked atomic.Bool

	t.Cleanup(func() {
		assert.True(t, asked.Load(), "the run never asked the portal for the catalog")
	})

	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		if req.URL.Path != catalogPath {
			assert.Failf(t, "the catalog reached an endpoint no answer was staged for", "path: %s", req.URL.Path)

			return nil, errPortalReached
		}

		asked.Store(true)

		return vhttp.Respond(status, []byte(body), http.Header{"Content-Type": {"application/json"}}), nil
	})
}

// portalAnsweringInTurn builds a client answering the catalog endpoint with one
// staged body per request, so a run signed in to several organizations can be
// told from one that asked only once. Fetches run concurrently, so which
// organization gets which body is not fixed.
func portalAnsweringInTurn(t *testing.T, bodies ...string) vhttp.Client {
	t.Helper()

	var asked atomic.Int64

	t.Cleanup(func() {
		assert.Equal(t, int64(len(bodies)), asked.Load(),
			"every organization the user is signed in to must be asked for its catalog")
	})

	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		if req.URL.Path != catalogPath {
			assert.Failf(t, "the catalog reached an endpoint no answer was staged for", "path: %s", req.URL.Path)

			return nil, errPortalReached
		}

		i := asked.Add(1) - 1
		if i >= int64(len(bodies)) {
			assert.Failf(t, "the portal was asked more times than answers were staged",
				"asked: %d, staged: %d", i+1, len(bodies))

			return nil, errPortalReached
		}

		return vhttp.Respond(
			http.StatusOK, []byte(bodies[i]), http.Header{"Content-Type": {"application/json"}},
		), nil
	})
}

// writeCatalogBlock writes a root config naming repoURL in its catalog block.
func writeCatalogBlock(t *testing.T, v *venv.Venv, rootDir, repoURL string) {
	t.Helper()

	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(rootDir, "root.hcl"),
		[]byte("catalog {\n  urls = [\""+repoURL+"\"]\n}\n"),
		0o644,
	))
}

// componentSources returns the source strings a scaffold would be generated
// from, one per rendered component, in the order they were written.
func componentSources(t *testing.T, out string) string {
	t.Helper()

	trimmed := strings.TrimSuffix(out, "\n")
	require.NotEmpty(t, trimmed)

	lines := strings.Split(trimmed, "\n")
	sources := make([]string, 0, len(lines))

	for _, line := range lines {
		var entry format.Entry

		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		require.NotEmpty(t, entry.ComponentSource)

		sources = append(sources, entry.ComponentSource)
	}

	slices.Sort(sources)

	return strings.Join(sources, "\n")
}
