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

// graphBoundaryFixture is a git repository mimicking a monorepo with sibling
// environments. The working directory is environments/staging, and the graph
// crosses out of it in both directions:
//
//	environments/staging/vpc          (dependent-direction target)
//	environments/staging/app          depends on ../vpc
//	environments/staging/edge         depends on ../../production/external
//	environments/production/consumer  depends on ../../staging/vpc
//	environments/production/external  (dependency-direction target's external dep)
type graphBoundaryFixture struct {
	stagingDir  string
	vpcDir      string
	appDir      string
	edgeDir     string
	consumerDir string
	externalDir string
}

func newGraphBoundaryFixture(t *testing.T) graphBoundaryFixture {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	cmd := exec.CommandContext(t.Context(), "git", "init")
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	require.NoError(t, cmd.Run())

	f := graphBoundaryFixture{
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

func (f *graphBoundaryFixture) discover(t *testing.T, query string) component.Components {
	t.Helper()

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = f.stagingDir
	opts.RootWorkingDir = f.stagingDir

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{query})
	require.NoError(t, err)

	configs, err := discovery.NewDiscovery(f.stagingDir).
		WithFilters(filters).
		Discover(t.Context(), logger.CreateLogger(), opts)
	require.NoError(t, err)

	return configs
}

// Test that an inline "(dir)" boundary encloses graph discovery in both
// directions, while the default (git root) crosses out of the working
// directory. The dependent direction reaches a dependent in a sibling
// environment; the dependency direction reaches an external dependency in a
// sibling environment. Both are confined by the inline boundary.
func TestDiscoveryGraphBoundary_EnclosesGraphDiscovery(t *testing.T) {
	t.Parallel()

	t.Run("dependent direction", func(t *testing.T) {
		t.Parallel()

		f := newGraphBoundaryFixture(t)

		unbounded := f.discover(t, "...{"+f.vpcDir+"}")
		assert.ElementsMatch(t,
			[]string{f.vpcDir, f.appDir, f.consumerDir},
			unbounded.Filter(component.UnitKind).Paths(),
		)

		bounded := f.discover(t, "("+f.stagingDir+")...{"+f.vpcDir+"}")
		assert.ElementsMatch(t,
			[]string{f.vpcDir, f.appDir},
			bounded.Filter(component.UnitKind).Paths(),
		)
	})

	t.Run("dependency direction", func(t *testing.T) {
		t.Parallel()

		f := newGraphBoundaryFixture(t)

		unbounded := f.discover(t, "{"+f.edgeDir+"}...")
		assert.ElementsMatch(t,
			[]string{f.edgeDir, f.externalDir},
			unbounded.Filter(component.UnitKind).Paths(),
		)

		bounded := f.discover(t, "{"+f.edgeDir+"}...("+f.stagingDir+")")
		assert.ElementsMatch(t,
			[]string{f.edgeDir},
			bounded.Filter(component.UnitKind).Paths(),
		)
	})
}

// Test that an invalid inline boundary surfaces a typed error from Discover.
func TestDiscoveryGraphBoundary_ValidatesBoundary(t *testing.T) {
	t.Parallel()

	f := newGraphBoundaryFixture(t)

	testCases := []struct {
		errAs any
		name  string
		query string
	}{
		{
			name:  "nonexistent boundary",
			query: "(" + filepath.Join(f.stagingDir, "does-not-exist") + ")...{" + f.vpcDir + "}",
			errAs: &discovery.FilterBoundaryDirError{},
		},
		{
			name:  "boundary does not contain working directory",
			query: "(" + f.consumerDir + ")...{" + f.vpcDir + "}",
			errAs: &discovery.FilterBoundaryScopeError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			opts.WorkingDir = f.stagingDir
			opts.RootWorkingDir = f.stagingDir

			filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{tc.query})
			require.NoError(t, err)

			_, err = discovery.NewDiscovery(f.stagingDir).
				WithFilters(filters).
				Discover(t.Context(), logger.CreateLogger(), opts)
			require.ErrorAs(t, err, tc.errAs)
		})
	}
}
