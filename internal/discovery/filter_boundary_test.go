package discovery_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// boundaryFixture is a git repository mimicking a monorepo with sibling
// environments. The working directory is environments/staging, and the graph
// crosses out of it in both directions:
//
//	environments/staging/vpc          (dependent-direction target)
//	environments/staging/app          depends on ../vpc
//	environments/staging/edge         depends on ../../production/external
//	environments/production/consumer  depends on ../../staging/vpc
//	environments/production/external  (dependency-direction target's external dep)
type boundaryFixture struct {
	tmpDir      string
	stagingDir  string
	vpcDir      string
	appDir      string
	edgeDir     string
	consumerDir string
	externalDir string
}

func newBoundaryFixture(t *testing.T) boundaryFixture {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Initialize a git repository so graph traversal bounds to the repo root
	// when no filter boundary is configured.
	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run())

	f := boundaryFixture{
		tmpDir:      tmpDir,
		stagingDir:  filepath.Join(tmpDir, "environments", "staging"),
		vpcDir:      filepath.Join(tmpDir, "environments", "staging", "vpc"),
		appDir:      filepath.Join(tmpDir, "environments", "staging", "app"),
		edgeDir:     filepath.Join(tmpDir, "environments", "staging", "edge"),
		consumerDir: filepath.Join(tmpDir, "environments", "production", "consumer"),
		externalDir: filepath.Join(tmpDir, "environments", "production", "external"),
	}

	for _, dir := range []string{f.vpcDir, f.appDir, f.edgeDir, f.consumerDir, f.externalDir} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(f.vpcDir, "terragrunt.hcl"), []byte(``), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(f.externalDir, "terragrunt.hcl"), []byte(``), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(f.appDir, "terragrunt.hcl"), []byte(`
dependency "vpc" {
  config_path = "../vpc"
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(f.consumerDir, "terragrunt.hcl"), []byte(`
dependency "vpc" {
  config_path = "../../staging/vpc"
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(f.edgeDir, "terragrunt.hcl"), []byte(`
dependency "external" {
  config_path = "../../production/external"
}
`), 0o644))

	return f
}

func (f *boundaryFixture) discover(t *testing.T, query, boundary string) (component.Components, error) {
	t.Helper()

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = f.stagingDir
	opts.RootWorkingDir = f.stagingDir

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{query})
	require.NoError(t, err)

	d := discovery.NewDiscovery(f.stagingDir).WithFilters(filters)

	if boundary != "" {
		d = d.WithFilterBoundary(boundary)
	}

	return d.Discover(t.Context(), logger.CreateLogger(), opts)
}

// Test that a filter boundary encloses graph discovery in both directions,
// while the default (git root) crosses out of the working directory. The
// dependent direction (`...{vpc}`) reaches a dependent in a sibling
// environment; the dependency direction (`{edge}...`) reaches an external
// dependency in a sibling environment. Both are confined by the boundary.
func TestDiscoveryFilterBoundary_EnclosesGraphDiscovery(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		// queryFor builds the filter query for the given fixture; the target is
		// referenced by absolute path so discovery can resolve it directly.
		queryFor  func(f boundaryFixture) string
		name      string
		unbounded []string
		bounded   []string
	}{
		{
			name:     "dependent direction",
			queryFor: func(f boundaryFixture) string { return "...{" + f.vpcDir + "}" },
			// vpc's dependents: app (staging) and consumer (production).
			unbounded: []string{"vpc", "app", "consumer"},
			bounded:   []string{"vpc", "app"},
		},
		{
			name:     "dependency direction",
			queryFor: func(f boundaryFixture) string { return "{" + f.edgeDir + "}..." },
			// edge's dependency closure: external (production).
			unbounded: []string{"edge", "external"},
			bounded:   []string{"edge"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newBoundaryFixture(t)

			byName := map[string]string{
				"vpc": f.vpcDir, "app": f.appDir, "edge": f.edgeDir,
				"consumer": f.consumerDir, "external": f.externalDir,
			}
			resolve := func(names []string) []string {
				out := make([]string, len(names))

				for i, n := range names {
					out[i] = byName[n]
				}

				return out
			}

			query := tc.queryFor(f)

			// Without a boundary, traversal reaches the git root and crosses
			// into the sibling production environment. This pins the fixture as
			// actually exercising cross-boundary traversal, so the bounded
			// assertion below is meaningful.
			configs, err := f.discover(t, query, "")
			require.NoError(t, err)
			assert.ElementsMatch(t, resolve(tc.unbounded), configs.Filter(component.UnitKind).Paths())

			// "." resolves against the working directory (staging).
			configs, err = f.discover(t, query, ".")
			require.NoError(t, err)
			assert.ElementsMatch(t, resolve(tc.bounded), configs.Filter(component.UnitKind).Paths())
		})
	}
}

// Test that an invalid boundary supplied directly to Discover is rejected
// with a typed error, covering callers that bypass the command constructors
// (like the runner pool).
func TestDiscoveryFilterBoundary_DiscoverValidatesBoundary(t *testing.T) {
	t.Parallel()

	f := newBoundaryFixture(t)

	testCases := []struct {
		errAs    any
		name     string
		boundary string
	}{
		{
			name:     "nonexistent boundary",
			boundary: filepath.Join(f.tmpDir, "does-not-exist"),
			errAs:    &discovery.FilterBoundaryDirError{},
		},
		{
			name:     "boundary is a file",
			boundary: filepath.Join(f.vpcDir, "terragrunt.hcl"),
			errAs:    &discovery.FilterBoundaryDirError{},
		},
		{
			name:     "boundary does not contain working directory",
			boundary: f.consumerDir,
			errAs:    &discovery.FilterBoundaryScopeError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := f.discover(t, "...{"+f.vpcDir+"}", tc.boundary)
			require.ErrorAs(t, err, tc.errAs)
		})
	}
}

// Test that NewForDiscoveryCommand validates the boundary so commands that
// swallow Discover errors (like find and list) still surface invalid input.
func TestNewForDiscoveryCommand_FilterBoundaryValidation(t *testing.T) {
	t.Parallel()

	f := newBoundaryFixture(t)

	testCases := []struct {
		errAs    any
		name     string
		boundary string
	}{
		{
			name:     "nonexistent boundary",
			boundary: filepath.Join(f.tmpDir, "does-not-exist"),
			errAs:    &discovery.FilterBoundaryDirError{},
		},
		{
			name:     "boundary does not contain working directory",
			boundary: f.consumerDir,
			errAs:    &discovery.FilterBoundaryScopeError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := discovery.NewForDiscoveryCommand(logger.CreateLogger(), &discovery.DiscoveryCommandOptions{
				WorkingDir:     f.stagingDir,
				FilterBoundary: tc.boundary,
			})
			require.ErrorAs(t, err, tc.errAs)
		})
	}

	t.Run("valid boundary", func(t *testing.T) {
		t.Parallel()

		d, err := discovery.NewForDiscoveryCommand(logger.CreateLogger(), &discovery.DiscoveryCommandOptions{
			WorkingDir:     f.stagingDir,
			FilterBoundary: "..",
		})
		require.NoError(t, err)
		require.NotNil(t, d)
	})
}
