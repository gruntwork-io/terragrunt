package discovery_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// boundaryFixture is an in-memory monorepo whose graph crosses out of the
// working directory (environments/staging) in both directions:
//
//	environments/staging/vpc          (dependent-direction target)
//	environments/staging/app          depends on ../vpc
//	environments/staging/edge         depends on ../../production/external
//	environments/production/consumer  depends on ../../staging/vpc
//	environments/production/external  (dependency-direction target's external dep)
type boundaryFixture struct {
	repoRoot    string
	stagingDir  string
	vpcDir      string
	appDir      string
	edgeDir     string
	consumerDir string
	externalDir string
}

func newBoundaryFixture(t *testing.T) (boundaryFixture, *venv.Venv) {
	t.Helper()

	repoRoot := string(filepath.Separator) + "repo"

	// The venv answers the git top-level probe with repoRoot, so traversal
	// bounds to the repository root when no discovery boundary is configured.
	v := memGitTopLevelVenv(t, repoRoot)

	f := boundaryFixture{
		repoRoot:    repoRoot,
		stagingDir:  filepath.Join(repoRoot, "environments", "staging"),
		vpcDir:      filepath.Join(repoRoot, "environments", "staging", "vpc"),
		appDir:      filepath.Join(repoRoot, "environments", "staging", "app"),
		edgeDir:     filepath.Join(repoRoot, "environments", "staging", "edge"),
		consumerDir: filepath.Join(repoRoot, "environments", "production", "consumer"),
		externalDir: filepath.Join(repoRoot, "environments", "production", "external"),
	}

	writeUnits(t, v.FS, map[string]string{
		f.vpcDir:      ``,
		f.externalDir: ``,
		f.appDir: `
dependency "vpc" {
  config_path = "../vpc"
}
`,
		f.consumerDir: `
dependency "vpc" {
  config_path = "../../staging/vpc"
}
`,
		f.edgeDir: `
dependency "external" {
  config_path = "../../production/external"
}
`,
	})

	return f, v
}

func (f *boundaryFixture) discover(t *testing.T, v *venv.Venv, query, boundary string) (component.Components, error) {
	t.Helper()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = f.stagingDir
	opts.RootWorkingDir = f.stagingDir

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{query})
	require.NoError(t, err)

	d := discovery.NewDiscovery(f.stagingDir).WithFilters(filters)

	if boundary != "" {
		d = d.WithDiscoveryBoundary(boundary)
	}

	return d.Discover(t.Context(), logger.CreateLogger(), v, opts)
}

// Test that a dependent found by the upstream walk is returned when the
// boundary encloses it. The walk searches directories above the working
// directory, and the components it turns up are restamped as graph-discovered,
// which is the same mark that decides what the boundary withholds. A boundary
// wide enough to contain a dependent has to return it regardless.
func TestDiscoveryBoundary_ReturnsDependentsFoundUpstream(t *testing.T) {
	t.Parallel()

	f, v := newBoundaryFixture(t)

	// environments/ contains both staging (the working directory) and the
	// production environment holding vpc's other dependent.
	configs, err := f.discover(t, v, "...{"+f.vpcDir+"}", "..")
	require.NoError(t, err)

	assert.ElementsMatch(
		t,
		[]string{f.vpcDir, f.appDir, f.consumerDir},
		configs.Filter(component.UnitKind).Paths(),
	)
}

// Test that a discovery boundary encloses graph discovery in both directions,
// while the default (git root) crosses out of the working directory. The
// dependent direction (`...{vpc}`) reaches a dependent in a sibling
// environment; the dependency direction (`{edge}...`) reaches an external
// dependency in a sibling environment. Both are confined by the boundary.
func TestDiscoveryBoundary_EnclosesGraphDiscovery(t *testing.T) {
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

			f, v := newBoundaryFixture(t)

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
			configs, err := f.discover(t, v, query, "")
			require.NoError(t, err)
			assert.ElementsMatch(t, resolve(tc.unbounded), configs.Filter(component.UnitKind).Paths())

			// "." resolves against the working directory (staging).
			configs, err = f.discover(t, v, query, ".")
			require.NoError(t, err)
			assert.ElementsMatch(t, resolve(tc.bounded), configs.Filter(component.UnitKind).Paths())
		})
	}
}

// Test that an invalid boundary supplied directly to Discover is rejected
// with a typed error, covering callers that bypass the command constructors
// (like the runner pool).
func TestDiscoveryBoundary_DiscoverValidatesBoundary(t *testing.T) {
	t.Parallel()

	f, v := newBoundaryFixture(t)

	testCases := []struct {
		errAs    any
		name     string
		boundary string
	}{
		{
			name:     "nonexistent boundary",
			boundary: filepath.Join(f.repoRoot, "does-not-exist"),
			errAs:    &discovery.DiscoveryBoundaryDirError{},
		},
		{
			name:     "boundary is a file",
			boundary: filepath.Join(f.vpcDir, "terragrunt.hcl"),
			errAs:    &discovery.DiscoveryBoundaryDirError{},
		},
		{
			name:     "boundary does not contain working directory",
			boundary: f.consumerDir,
			errAs:    &discovery.DiscoveryBoundaryScopeError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := f.discover(t, v, "...{"+f.vpcDir+"}", tc.boundary)
			require.ErrorAs(t, err, tc.errAs)
		})
	}
}

// Test that NewForDiscoveryCommand validates the boundary so commands that
// swallow Discover errors (like find and list) still surface invalid input.
func TestNewForDiscoveryCommand_DiscoveryBoundaryValidation(t *testing.T) {
	t.Parallel()

	f, v := newBoundaryFixture(t)

	newForDiscoveryCommand := func(t *testing.T, query, boundary string) (*discovery.Discovery, error) {
		t.Helper()

		filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{query})
		require.NoError(t, err)

		return discovery.NewForDiscoveryCommand(logger.CreateLogger(), v.FS, &discovery.DiscoveryCommandOptions{
			WorkingDir:        f.stagingDir,
			DiscoveryBoundary: boundary,
			Filters:           filters,
		})
	}

	testCases := []struct {
		errAs    any
		name     string
		query    string
		boundary string
	}{
		{
			name:     "nonexistent boundary",
			query:    "...{" + f.vpcDir + "}",
			boundary: filepath.Join(f.repoRoot, "does-not-exist"),
			errAs:    &discovery.DiscoveryBoundaryDirError{},
		},
		{
			name:     "dependent direction cannot start outside the boundary",
			query:    "...{" + f.vpcDir + "}",
			boundary: f.consumerDir,
			errAs:    &discovery.DiscoveryBoundaryScopeError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newForDiscoveryCommand(t, tc.query, tc.boundary)
			require.ErrorAs(t, err, tc.errAs)
		})
	}

	t.Run("valid boundary", func(t *testing.T) {
		t.Parallel()

		d, err := newForDiscoveryCommand(t, "...{"+f.vpcDir+"}", "..")
		require.NoError(t, err)
		require.NotNil(t, d)
	})

	t.Run("dependency direction accepts a boundary outside the working directory", func(t *testing.T) {
		t.Parallel()

		d, err := newForDiscoveryCommand(t, "{"+f.edgeDir+"}...", f.consumerDir)
		require.NoError(t, err)
		require.NotNil(t, d)
	})
}

// Test that the boundary survives filter evaluation when relationships are
// discovered, as the runner pool does for `run --all`. Components kept by the
// graph phase still carry edges pointing across the boundary, so evaluation
// has to refuse to follow them rather than growing the set back.
func TestDiscoveryBoundary_SurvivesFilterEvaluationWithRelationships(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		boundary string
		expected []string
	}{
		{
			name:     "unbounded traversal reaches the sibling environment",
			boundary: "",
			expected: []string{"edge", "external"},
		},
		{
			name:     "bounded traversal stops at the boundary",
			boundary: ".",
			expected: []string{"edge"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, v := newBoundaryFixture(t)

			byName := map[string]string{"edge": f.edgeDir, "external": f.externalDir}
			paths := make([]string, len(tc.expected))

			for i, n := range tc.expected {
				paths[i] = byName[n]
			}

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.WorkingDir = f.stagingDir
			opts.RootWorkingDir = f.stagingDir

			filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"{" + f.edgeDir + "}..."})
			require.NoError(t, err)

			d := discovery.NewDiscovery(f.stagingDir).WithFilters(filters).WithRelationships()

			if tc.boundary != "" {
				d = d.WithDiscoveryBoundary(tc.boundary)
			}

			configs, err := d.Discover(t.Context(), logger.CreateLogger(), v, opts)
			require.NoError(t, err)
			assert.ElementsMatch(t, paths, configs.Filter(component.UnitKind).Paths())
		})
	}
}

// Test what the boundary withholds under filter shapes that do not bound their
// own traversal. A filter that only excludes starts from every component
// discovered, so the boundary is the only thing standing between a run and a
// component traversal reached across it. An inline "(dir)" operand is the one
// shape that overrides the flag, and it does so for the whole run.
func TestDiscoveryBoundary_WithholdingAcrossFilterShapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		queriesFor func(f boundaryFixture) []string
		name       string
		expected   []string
	}{
		{
			name:       "a filter that only excludes still withholds what traversal reached",
			queriesFor: func(_ boundaryFixture) []string { return []string{"!name=app"} },
			expected:   []string{"vpc", "edge"},
		},
		{
			name: "an inline operand on any filter stands the flag down",
			queriesFor: func(f boundaryFixture) []string {
				return []string{"{" + f.edgeDir + "}...(..)", "{" + f.vpcDir + "}"}
			},
			expected: []string{"vpc", "edge", "external"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, v := newBoundaryFixture(t)

			byName := map[string]string{
				"vpc":      f.vpcDir,
				"app":      f.appDir,
				"edge":     f.edgeDir,
				"external": f.externalDir,
			}

			paths := make([]string, len(tc.expected))
			for i, n := range tc.expected {
				paths[i] = byName[n]
			}

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.WorkingDir = f.stagingDir
			opts.RootWorkingDir = f.stagingDir

			filters, err := filter.ParseFilterQueries(logger.CreateLogger(), tc.queriesFor(f))
			require.NoError(t, err)

			configs, err := discovery.NewDiscovery(f.stagingDir).
				WithFilters(filters).
				WithRelationships().
				WithDiscoveryBoundary(".").
				Discover(t.Context(), logger.CreateLogger(), v, opts)
			require.NoError(t, err)

			assert.ElementsMatch(t, paths, configs.Filter(component.UnitKind).Paths())
		})
	}
}

// Test that a boundary the working directory sits outside of is accepted and
// honored for dependency-direction filters, matching what the equivalent
// inline "(dir)" operand already does.
func TestDiscoveryBoundary_DependencyDirectionOutsideWorkingDir(t *testing.T) {
	t.Parallel()

	f, v := newBoundaryFixture(t)

	// environments/production holds edge's dependency but not the working
	// directory (environments/staging).
	productionDir := filepath.Dir(f.externalDir)

	testCases := []struct {
		name     string
		boundary string
		expected []string
	}{
		{
			name:     "dependency inside the boundary is traversed",
			boundary: productionDir,
			expected: []string{"edge", "external"},
		},
		{
			name:     "dependency outside the boundary is pruned",
			boundary: f.consumerDir,
			expected: []string{"edge"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			byName := map[string]string{"edge": f.edgeDir, "external": f.externalDir}
			paths := make([]string, len(tc.expected))

			for i, n := range tc.expected {
				paths[i] = byName[n]
			}

			configs, err := f.discover(t, v, "{"+f.edgeDir+"}...", tc.boundary)
			require.NoError(t, err)
			assert.ElementsMatch(t, paths, configs.Filter(component.UnitKind).Paths())
		})
	}
}

// Test that a run carrying no filter keeps every component the filesystem walk
// found, boundary or not, and only withholds what traversal reached across the
// boundary. A component that stays in the run has to keep the edges that order
// it, so an edge may only be withheld along with the component it points at.
func TestDiscoveryBoundary_UnfilteredRunWithholdsOnlyWhatTraversalReached(t *testing.T) {
	t.Parallel()

	f, v := newBoundaryFixture(t)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = f.stagingDir
	opts.RootWorkingDir = f.stagingDir

	configs, err := discovery.NewDiscovery(f.stagingDir).
		WithRelationships().
		WithDiscoveryBoundary(".").
		Discover(t.Context(), logger.CreateLogger(), v, opts)
	require.NoError(t, err)

	// production/external is edge's dependency, reached across the boundary.
	assert.ElementsMatch(
		t,
		[]string{f.vpcDir, f.appDir, f.edgeDir},
		configs.Filter(component.UnitKind).Paths(),
	)

	var edge component.Component

	for _, c := range configs {
		if c.Path() == f.edgeDir {
			edge = c
		}
	}

	require.NotNil(t, edge)

	depPaths := make([]string, 0, len(edge.Dependencies()))
	for _, dep := range edge.Dependencies() {
		depPaths = append(depPaths, dep.Path())
	}

	assert.Equal(t, []string{f.externalDir}, depPaths, "the edge that orders edge against its dependency stands")
}

// Test that a dependency the boundary excludes is still read and still linked.
// Dropping the edge would leave a run unable to order itself against that
// dependency or fetch its outputs, so the boundary has to keep the edge while
// keeping the component itself out of what discovery returns.
func TestDiscoveryBoundary_ExcludedDependencyStaysLinked(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		query    string
		boundary string
		returned []string
	}{
		{
			name:     "flag boundary keeps the crossing edge out of the result",
			query:    "{%[1]s}...",
			boundary: ".",
			returned: []string{"edge"},
		},
		{
			// The inline operand overrides the flag and reaches wider, so the
			// dependency it reaches is returned as well as linked.
			name:     "wider inline operand returns what it reaches",
			query:    "{%[1]s}...(..)",
			boundary: ".",
			returned: []string{"edge", "external"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, v := newBoundaryFixture(t)

			byName := map[string]string{"edge": f.edgeDir, "external": f.externalDir}
			expected := make([]string, len(tc.returned))

			for i, n := range tc.returned {
				expected[i] = byName[n]
			}

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.WorkingDir = f.stagingDir
			opts.RootWorkingDir = f.stagingDir

			query := fmt.Sprintf(tc.query, f.edgeDir)

			filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{query})
			require.NoError(t, err)

			configs, err := discovery.NewDiscovery(f.stagingDir).
				WithFilters(filters).
				WithRelationships().
				WithDiscoveryBoundary(tc.boundary).
				Discover(t.Context(), logger.CreateLogger(), v, opts)
			require.NoError(t, err)

			assert.ElementsMatch(t, expected, configs.Filter(component.UnitKind).Paths())

			var edge component.Component

			for _, c := range configs {
				if c.Path() == f.edgeDir {
					edge = c
				}
			}

			require.NotNil(t, edge, "edge should always be discovered; it is the filter target")

			depPaths := make([]string, 0, len(edge.Dependencies()))
			for _, dep := range edge.Dependencies() {
				depPaths = append(depPaths, dep.Path())
			}

			assert.Equal(
				t,
				[]string{f.externalDir},
				depPaths,
				"the dependency stays linked whether or not the boundary returns it",
			)
		})
	}
}
