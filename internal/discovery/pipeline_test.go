package discovery_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// pipelineRun is one discovery over a fixture, through the pipeline or the phases.
type pipelineRun struct {
	// configure applies the options under test to the discovery.
	configure func(d *discovery.Discovery) *discovery.Discovery
	// exec runs the commands the configs call. When nil, the venv spawns nothing and every command fails.
	exec vexec.Exec
	// files maps each path below the corpus repo root to its contents.
	files map[string]string
	// links maps each symlink below the corpus repo root to the path below the root it points at.
	links map[string]string
	// workingDir is the directory below the corpus repo root that discovery runs from, the root itself when empty.
	workingDir string
	// queries are the filter queries the discovery runs with.
	queries []string
	// usePipeline selects the pipeline through [discovery.EnvPipeline].
	usePipeline bool
}

// discover runs the discovery with ctx and returns what it found.
func (r *pipelineRun) discover(ctx context.Context, t *testing.T) (component.Components, error) {
	t.Helper()

	v := memRepoRootVenv(t, corpusRepoRoot)
	writeFixture(t, v, prefixPaths(r.files))

	for link, target := range r.links {
		require.NoError(t, vfs.Symlink(v.FS, repoPath(target), repoPath(link)))
	}

	if r.exec != nil {
		v = v.WithExec(r.exec)
	}

	if r.usePipeline {
		v.Env[discovery.EnvPipeline] = "true"
	}

	workingDir := repoPath(r.workingDir)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, r.queries)
	require.NoError(t, err)

	d := discovery.NewDiscovery(workingDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
		WithGitRoot(corpusRepoRoot).
		WithFilters(filters)

	if r.configure != nil {
		d = r.configure(d)
	}

	return d.Discover(config.WithCaches(ctx), l, v, new(rewrite.Store), opts)
}

func TestPipelineEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		env  map[string]string
		name string
		want bool
	}{
		{name: "unset", env: map[string]string{}, want: false},
		{name: "true", env: map[string]string{discovery.EnvPipeline: "true"}, want: true},
		{name: "false", env: map[string]string{discovery.EnvPipeline: "false"}, want: false},
		{name: "not a boolean", env: map[string]string{discovery.EnvPipeline: "pipeline"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, discovery.PipelineEnabled(tt.env))
		})
	}
}

func TestDiscoveryPipelineMatchesPhasesWithRacing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		configure func(d *discovery.Discovery) *discovery.Discovery
		name      string
		queries   []string
	}{
		{name: "walk only"},
		{name: "path filter", queries: []string{"./apps/**"}},
		{name: "negated path filter", queries: []string{"!./apps/web"}},
		{name: "kind filter", queries: []string{"type=stack"}},
		{
			name:    "filter that reads the config",
			queries: []string{"reading=common.yaml"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
		{
			name: "read files",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithReadFiles().WithSuppressParseErrors()
			},
		},
		{
			name: "exclude blocks",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithParseExclude().WithSuppressParseErrors()
			},
		},
		{
			name:    "relationships with every parse option",
			queries: []string{"./apps/**", "./infra/**"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.
					WithRequiresParse().
					WithRelationships().
					WithParseExclude().
					WithParseIncludes().
					WithTrackReads().
					WithSuppressParseErrors()
			},
		},
		{
			name:    "dependents of a unit",
			queries: []string{"...db"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
		{
			name:    "dependencies of a unit",
			queries: []string{"web..."},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
		{
			name:    "dependencies of a unit one deep",
			queries: []string{"web...1"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
		{
			name:    "dependencies of a unit without the unit",
			queries: []string{"^web..."},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
		{
			name:    "dependencies of a unit within a boundary",
			queries: []string{"web..."},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithDiscoveryBoundary("apps").WithSuppressParseErrors()
			},
		},
		{
			name:    "dependents of the dependents of a unit",
			queries: []string{"...vpc"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
		{
			name:    "dependents and dependencies of a unit",
			queries: []string{"...db..."},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
		{
			name:    "dependents of a unit within a boundary",
			queries: []string{"...db"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithDiscoveryBoundary("apps").WithSuppressParseErrors()
			},
		},
		{
			name:    "relationships of the dependents of a unit",
			queries: []string{"...db"},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithRelationships().WithSuppressParseErrors()
			},
		},
		{
			name:    "relationships of the dependencies of a unit",
			queries: []string{"web..."},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithRelationships().WithSuppressParseErrors()
			},
		},
		{
			name:    "relationships of a path filter beside a graph filter",
			queries: []string{"./infra/**", "worker..."},
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithRelationships().WithSuppressParseErrors()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := pipelineRun{files: rewriteCorpus, queries: tt.queries, configure: tt.configure}

			want, err := run.discover(t.Context(), t)
			require.NoError(t, err)
			require.NotEmpty(t, want)

			run.usePipeline = true

			got, err := run.discover(t.Context(), t)
			require.NoError(t, err)

			assert.Equal(t, componentViews(want), componentViews(got))
		})
	}
}

func TestDiscoveryPipelineRelatesDependenciesOutsideTheFiltersWithRacing(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"apps/api/terragrunt.hcl":      dependenciesOn("../web", "../../infra/vpc"),
			"apps/web/terragrunt.hcl":      dependenciesOn("../../infra/vpc"),
			"infra/vpc/terragrunt.hcl":     dependenciesOn("../network"),
			"infra/network/terragrunt.hcl": dependenciesOn("../account"),
			"infra/account/terragrunt.hcl": "",
		},
		queries: []string{"./apps/**"},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithRelationships()
		},
	}

	want, err := run.discover(t.Context(), t)
	require.NoError(t, err)

	run.usePipeline = true

	got, err := run.discover(t.Context(), t)
	require.NoError(t, err)

	assert.Equal(t, componentViews(want), componentViews(got))

	byPath := make(map[string]component.Component, len(got))
	for _, c := range got {
		byPath[c.Path()] = c
	}

	api := byPath[filepath.Join(corpusRepoRoot, "apps", "api")]
	web := byPath[filepath.Join(corpusRepoRoot, "apps", "web")]

	require.NotNil(t, api)
	require.NotNil(t, web)
	require.Len(t, web.Dependencies(), 1)

	vpc := web.Dependencies()[0]

	assert.Contains(t, api.Dependencies(), web)
	assert.Contains(t, api.Dependencies(), vpc)

	assert.Equal(
		t,
		[]string{
			filepath.Join(corpusRepoRoot, "infra", "vpc"),
			filepath.Join(corpusRepoRoot, "infra", "network"),
			filepath.Join(corpusRepoRoot, "infra", "account"),
		},
		dependencyChain(vpc),
	)
}

func TestDiscoveryPipelineRelatesUnitsJoinedOutsideTheFiltersWithRacing(t *testing.T) {
	t.Parallel()

	middles := []struct {
		name     string
		filename string
		contents string
	}{
		{name: "hcl", filename: "terragrunt.hcl", contents: dependenciesOn("../c")},
		{name: "json", filename: "terragrunt.hcl.json", contents: `{"dependencies": {"paths": ["../c"]}}`},
	}

	for _, middle := range middles {
		for _, usePipeline := range []bool{false, true} {
			t.Run(middle.name+"/pipeline="+strconv.FormatBool(usePipeline), func(t *testing.T) {
				t.Parallel()

				run := pipelineRun{
					files: map[string]string{
						"live/a/terragrunt.hcl":     dependenciesOn("../b"),
						"live/b/" + middle.filename: middle.contents,
						"live/c/terragrunt.hcl":     "",
					},
					queries: []string{"./live/**", "!./live/b"},
					configure: func(d *discovery.Discovery) *discovery.Discovery {
						return d.WithRelationships()
					},
					usePipeline: usePipeline,
				}

				got, err := run.discover(t.Context(), t)
				require.NoError(t, err)

				byPath := make(map[string]component.Component, len(got))
				for _, c := range got {
					byPath[c.Path()] = c
				}

				a := byPath[filepath.Join(corpusRepoRoot, "live", "a")]
				c := byPath[filepath.Join(corpusRepoRoot, "live", "c")]

				require.NotNil(t, a)
				require.NotNil(t, c)
				require.Len(t, got, 2)

				assert.Equal(
					t,
					[]string{
						filepath.Join(corpusRepoRoot, "live", "a"),
						filepath.Join(corpusRepoRoot, "live", "b"),
						filepath.Join(corpusRepoRoot, "live", "c"),
					},
					dependencyChain(a),
				)
				assert.Same(t, c, a.Dependencies()[0].Dependencies()[0])
			})
		}
	}
}

func TestDiscoveryPipelineLeavesDependenciesOfALoneUnitUnreadWithRacing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		configure func(d *discovery.Discovery) *discovery.Discovery
		name      string
	}{
		{
			name: "run",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithRelationships()
			},
		},
		{
			name: "find and list",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithRelationships().WithSuppressParseErrors().WithBreakCycles()
			},
		},
	}

	for _, tt := range tests {
		for _, usePipeline := range []bool{false, true} {
			t.Run(tt.name+"/pipeline="+strconv.FormatBool(usePipeline), func(t *testing.T) {
				t.Parallel()

				marks := &markRecorder{}

				run := pipelineRun{
					files:       outsideTheFiltersFixture(),
					queries:     []string{"./live/**"},
					configure:   tt.configure,
					exec:        marks.exec(),
					usePipeline: usePipeline,
				}

				got, err := run.discover(t.Context(), t)
				require.NoError(t, err)
				require.Len(t, got, 1)

				a, ok := got[0].(*component.Unit)
				require.True(t, ok)
				require.NotNil(t, a.Config())
				require.Len(t, a.Dependencies(), 1)

				e := a.Dependencies()[0]

				assert.Equal(t, filepath.Join(corpusRepoRoot, "ext", "e"), e.Path())
				assert.Empty(t, e.Dependencies())
				assert.Empty(t, marks.marked())
			})
		}
	}
}

func TestDiscoveryPipelineReadsDependenciesOutsideTheFiltersToRelateUnitsWithRacing(t *testing.T) {
	t.Parallel()

	for _, usePipeline := range []bool{false, true} {
		t.Run("pipeline="+strconv.FormatBool(usePipeline), func(t *testing.T) {
			t.Parallel()

			marks := &markRecorder{}

			files := outsideTheFiltersFixture()
			files["live/b/terragrunt.hcl"] = ""

			run := pipelineRun{
				files:   files,
				queries: []string{"./live/**"},
				configure: func(d *discovery.Discovery) *discovery.Discovery {
					return d.WithRelationships()
				},
				exec:        marks.exec(),
				usePipeline: usePipeline,
			}

			_, err := run.discover(t.Context(), t)
			require.ErrorAs(t, err, new(hcl.Diagnostics))
			assert.Equal(t, []string{"e"}, marks.marked())
		})
	}
}

func TestDiscoveryPipelineStopsWalkingOnceEveryUnitIsReachedWithRacing(t *testing.T) {
	t.Parallel()

	for _, usePipeline := range []bool{false, true} {
		t.Run("pipeline="+strconv.FormatBool(usePipeline), func(t *testing.T) {
			t.Parallel()

			run := pipelineRun{
				files: map[string]string{
					"live/a/terragrunt.hcl": dependenciesOn("../../ext/x"),
					"live/c/terragrunt.hcl": "",
					"ext/x/terragrunt.hcl":  dependenciesOn("../../live/c", "../y"),
					"ext/y/terragrunt.hcl":  "locals {\n",
				},
				queries: []string{"./live/**"},
				configure: func(d *discovery.Discovery) *discovery.Discovery {
					return d.WithRelationships()
				},
				usePipeline: usePipeline,
			}

			got, err := run.discover(t.Context(), t)
			require.NoError(t, err)

			a := got.FilterByPath(filepath.Join(corpusRepoRoot, "live", "a"))
			require.Len(t, a, 1)
			require.Len(t, a[0].Dependencies(), 1)

			x := a[0].Dependencies()[0]

			assert.ElementsMatch(
				t,
				[]string{filepath.Join(corpusRepoRoot, "live", "c"), filepath.Join(corpusRepoRoot, "ext", "y")},
				x.Dependencies().Paths(),
			)
		})
	}
}

func TestDiscoveryPipelineWalksPastADiscoveredStackWithRacing(t *testing.T) {
	t.Parallel()

	for _, usePipeline := range []bool{false, true} {
		t.Run("pipeline="+strconv.FormatBool(usePipeline), func(t *testing.T) {
			t.Parallel()

			run := pipelineRun{
				files: map[string]string{
					"live/a/terragrunt.hcl":       dependenciesOn("../s", "../../ext/x"),
					"live/c/terragrunt.hcl":       "",
					"live/s/terragrunt.stack.hcl": "",
					"ext/x/terragrunt.hcl":        dependenciesOn("../../live/c"),
				},
				queries: []string{"./live/**"},
				configure: func(d *discovery.Discovery) *discovery.Discovery {
					return d.WithRelationships()
				},
				usePipeline: usePipeline,
			}

			got, err := run.discover(t.Context(), t)
			require.NoError(t, err)

			a := got.FilterByPath(repoPath("live", "a"))
			require.Len(t, a, 1)

			c := got.FilterByPath(repoPath("live", "c"))
			require.Len(t, c, 1)

			x := a[0].Dependencies().FilterByPath(repoPath("ext", "x"))
			require.Len(t, x, 1)

			assert.Equal(t, component.Components{c[0]}, x[0].Dependencies())
		})
	}
}

func TestDiscoveryPipelineRelatesUnitsUnderASymlinkWithRacing(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"real/live/a/terragrunt.hcl": dependenciesOn("../c"),
			"real/live/c/terragrunt.hcl": "",
		},
		links:      map[string]string{"link": "real"},
		workingDir: filepath.Join("link", "live"),
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithRelationships()
		},
		usePipeline: true,
	}

	got, err := run.discover(t.Context(), t)
	require.NoError(t, err)

	a := got.FilterByPath(repoPath("link", "live", "a"))
	require.Len(t, a, 1)

	c := got.FilterByPath(repoPath("link", "live", "c"))
	require.Len(t, c, 1)

	assert.Equal(t, component.Components{c[0]}, a[0].Dependencies())
}

func TestDiscoveryPipelineRelatesDependencyCyclesOutsideTheFiltersWithRacing(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"live/a/terragrunt.hcl": dependenciesOn("../../ext/x"),
			"live/b/terragrunt.hcl": "",
			"ext/x/terragrunt.hcl":  dependenciesOn("../y"),
			"ext/y/terragrunt.hcl":  dependenciesOn("../x"),
		},
		queries: []string{"./live/**"},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithRelationships()
		},
		usePipeline: true,
	}

	got, err := run.discover(t.Context(), t)
	require.NoError(t, err)

	a := got.FilterByPath(filepath.Join(corpusRepoRoot, "live", "a"))
	require.Len(t, a, 1)
	require.Len(t, a[0].Dependencies(), 1)

	x := a[0].Dependencies()[0]

	require.Len(t, x.Dependencies(), 1)
	assert.Equal(t, component.Components{x}, x.Dependencies()[0].Dependencies())
}

func TestDiscoveryPipelineReportsDependenciesWithoutAConfigWithRacing(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"live/a/terragrunt.hcl": dependenciesOn("../../ext/x"),
			"live/b/terragrunt.hcl": "",
			"ext/x/terragrunt.hcl":  dependenciesOn("../missing"),
		},
		queries: []string{"./live/**"},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithRelationships()
		},
		usePipeline: true,
	}

	_, err := run.discover(t.Context(), t)
	require.ErrorAs(t, err, new(config.TerragruntConfigNotFoundError))

	run.configure = func(d *discovery.Discovery) *discovery.Discovery {
		return d.WithRelationships().WithSuppressParseErrors()
	}

	got, err := run.discover(t.Context(), t)
	require.NoError(t, err)

	a := got.FilterByPath(filepath.Join(corpusRepoRoot, "live", "a"))
	require.Len(t, a, 1)

	assert.Equal(
		t,
		[]string{
			filepath.Join(corpusRepoRoot, "live", "a"),
			filepath.Join(corpusRepoRoot, "ext", "x"),
			filepath.Join(corpusRepoRoot, "ext", "missing"),
		},
		dependencyChain(a[0]),
	)
}

func TestDiscoveryPipelineReadsDependenciesAGraphFilterTraversesWithRacing(t *testing.T) {
	t.Parallel()

	marks := &markRecorder{}

	run := pipelineRun{
		files:   outsideTheFiltersFixture(),
		queries: []string{"{./live/a}..."},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithRelationships()
		},
		exec:        marks.exec(),
		usePipeline: true,
	}

	_, err := run.discover(t.Context(), t)
	require.ErrorAs(t, err, new(hcl.Diagnostics))
	assert.Contains(t, marks.marked(), "e")

	run.files["ext/f/terragrunt.hcl"] = ""

	got, err := run.discover(t.Context(), t)
	require.NoError(t, err)

	assert.ElementsMatch(
		t,
		[]string{
			filepath.Join(corpusRepoRoot, "live", "a"),
			filepath.Join(corpusRepoRoot, "ext", "e"),
			filepath.Join(corpusRepoRoot, "ext", "f"),
		},
		got.Paths(),
	)
}

// outsideTheFiltersFixture returns a unit under live that depends on a unit under ext, which calls run_cmd("mark",
// "e") and depends on a unit whose config does not parse.
func outsideTheFiltersFixture() map[string]string {
	return map[string]string{
		"live/a/terragrunt.hcl": dependenciesOn("../../ext/e"),
		"ext/e/terragrunt.hcl":  "locals {\n  mark = run_cmd(\"mark\", \"e\")\n}\n" + dependenciesOn("../f"),
		"ext/f/terragrunt.hcl":  "locals {\n",
	}
}

// markRecorder records the arguments of every command the configs of a discovery run.
type markRecorder struct {
	args []string
	mu   sync.Mutex
}

// exec returns an executor that records the arguments of each command it runs.
func (m *markRecorder) exec() vexec.Exec {
	return vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		m.mu.Lock()
		defer m.mu.Unlock()

		m.args = append(m.args, inv.Args...)

		return vexec.Result{}
	})
}

// marked returns the arguments recorded so far.
func (m *markRecorder) marked() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.args)
}

func TestDiscoveryPipelineSkipsUnitsOutsideTheDependentBoundaryWithRacing(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"live/account/terragrunt.hcl":   "",
			"live/roles/terragrunt.hcl":     dependenciesOn("../account"),
			"catalog/roles/terragrunt.hcl":  "locals {\n",
			"catalog/users/terragrunt.hcl":  dependenciesOn("../../live/account"),
			"live/unrelated/terragrunt.hcl": "",
		},
		queries: []string{"...{./live/account}"},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithDiscoveryBoundary("live")
		},
		usePipeline: true,
	}

	got, err := run.discover(t.Context(), t)
	require.NoError(t, err)

	assert.ElementsMatch(
		t,
		[]string{
			filepath.Join(corpusRepoRoot, "live", "account"),
			filepath.Join(corpusRepoRoot, "live", "roles"),
		},
		got.Paths(),
	)

	run.configure = nil

	_, err = run.discover(t.Context(), t)
	require.ErrorAs(t, err, new(hcl.Diagnostics))
}

func TestDiscoveryPipelineTraversesASharedDependencyToTheGreatestDepthWithRacing(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"apps/web/terragrunt.hcl":    dependenciesOn("../db"),
			"apps/worker/terragrunt.hcl": dependenciesOn("../db"),
			"apps/db/terragrunt.hcl":     dependenciesOn("../../infra/vpc"),
			"infra/vpc/terragrunt.hcl":   "",
		},
		usePipeline: true,
	}

	want := []string{
		filepath.Join(corpusRepoRoot, "apps", "db"),
		filepath.Join(corpusRepoRoot, "apps", "web"),
		filepath.Join(corpusRepoRoot, "apps", "worker"),
		filepath.Join(corpusRepoRoot, "infra", "vpc"),
	}

	for _, queries := range [][]string{{"web...1", "worker..."}, {"worker...", "web...1"}} {
		run.queries = queries

		got, err := run.discover(t.Context(), t)
		require.NoError(t, err)

		assert.ElementsMatch(t, want, got.Paths(), "queries %q", queries)
	}
}

func TestDiscoveryPipelineTraversesEveryGraphExpressionOfAFilterWithRacing(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"apps/web/terragrunt.hcl":    dependenciesOn("../../infra/db"),
			"apps/worker/terragrunt.hcl": dependenciesOn("../../infra/vpc"),
			"infra/db/terragrunt.hcl":    dependenciesOn("../vpc"),
			"infra/vpc/terragrunt.hcl":   "",
		},
		usePipeline: true,
	}

	want := []string{
		filepath.Join(corpusRepoRoot, "apps", "web"),
		filepath.Join(corpusRepoRoot, "infra", "db"),
		filepath.Join(corpusRepoRoot, "infra", "vpc"),
	}

	for _, queries := range [][]string{{"web... | worker...1", "web..."}, {"worker...1 | web...", "web..."}} {
		run.queries = queries

		got, err := run.discover(t.Context(), t)
		require.NoError(t, err)

		assert.ElementsMatch(t, want, got.Paths(), "queries %q", queries)
	}
}

func TestDiscoveryTraversesDependentsOfAUnitAnotherFilterReachedWithRacing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		files   map[string]string
		name    string
		queries []string
		want    []string
	}{
		{
			name: "dependent of a target in the working directory",
			files: map[string]string{
				"apps/worker/terragrunt.hcl": dependenciesOn("../../infra/web"),
				"infra/web/terragrunt.hcl":   dependenciesOn("../db"),
				"infra/db/terragrunt.hcl":    "",
			},
			queries: []string{"...db", "web...1"},
			want: []string{
				filepath.Join(corpusRepoRoot, "infra", "db"),
				filepath.Join(corpusRepoRoot, "infra", "web"),
				filepath.Join(corpusRepoRoot, "apps", "worker"),
			},
		},
		{
			name: "dependent of a dependent outside the working directory",
			files: map[string]string{
				"apps/worker/terragrunt.hcl": dependenciesOn("../web"),
				"apps/web/terragrunt.hcl":    dependenciesOn("../../infra/db"),
				"infra/app/terragrunt.hcl":   dependenciesOn("../../apps/web"),
				"infra/db/terragrunt.hcl":    "",
			},
			queries: []string{"...db", "app...1"},
			want: []string{
				filepath.Join(corpusRepoRoot, "infra", "app"),
				filepath.Join(corpusRepoRoot, "infra", "db"),
				filepath.Join(corpusRepoRoot, "apps", "web"),
				filepath.Join(corpusRepoRoot, "apps", "worker"),
			},
		},
	}

	for _, tt := range tests {
		for _, usePipeline := range []bool{false, true} {
			t.Run(tt.name+" pipeline "+strconv.FormatBool(usePipeline), func(t *testing.T) {
				t.Parallel()

				run := pipelineRun{
					files:       tt.files,
					workingDir:  "infra",
					usePipeline: usePipeline,
				}

				for _, queries := range [][]string{tt.queries, {tt.queries[1], tt.queries[0]}} {
					run.queries = queries

					got, err := run.discover(t.Context(), t)
					require.NoError(t, err)

					assert.ElementsMatch(t, tt.want, got.Paths(), "queries %q", queries)
				}
			})
		}
	}
}

func TestDiscoveryPipelineFindsDependentsAboveTheWorkingDirectory(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"live/prod/vpc/terragrunt.hcl":      "",
		"live/prod/app/terragrunt.hcl":      dependenciesOn("../vpc"),
		"live/dev/app/terragrunt.hcl":       dependenciesOn("../../prod/vpc"),
		"tools/audit/report/terragrunt.hcl": dependenciesOn("../../../live/dev/app"),
		"tools/unrelated/terragrunt.hcl":    "",
	}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "every dependent",
			query: "...vpc",
			want:  []string{"live/prod/vpc", "live/prod/app", "live/dev/app", "tools/audit/report"},
		},
		{
			name:  "depth counts dependencies, not directories",
			query: "1...vpc",
			want:  []string{"live/prod/vpc", "live/prod/app", "live/dev/app"},
		},
		{
			name:  "boundary",
			query: "(../../live)...vpc",
			want:  []string{"live/prod/vpc", "live/prod/app", "live/dev/app"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workingDir := filepath.Join(corpusRepoRoot, "live", "prod")

			v := memRepoRootVenv(t, corpusRepoRoot)
			writeFixture(t, v, prefixPaths(files))

			v.Env[discovery.EnvPipeline] = "true"

			opts := options.NewTerragruntOptions()
			opts.WorkingDir = workingDir
			opts.RootWorkingDir = workingDir

			l := logger.CreateLogger()

			filters, err := filter.ParseFilterQueries(l, []string{tt.query})
			require.NoError(t, err)

			got, err := discovery.NewDiscovery(workingDir).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir}).
				WithGitRoot(corpusRepoRoot).
				WithFilters(filters).
				Discover(config.WithCaches(t.Context()), l, v, new(rewrite.Store), opts)
			require.NoError(t, err)

			want := make([]string, 0, len(tt.want))
			for _, path := range tt.want {
				want = append(want, filepath.Join(corpusRepoRoot, filepath.FromSlash(path)))
			}

			assert.ElementsMatch(t, want, got.Paths())
		})
	}
}

// dependenciesOn returns a unit config that depends on the units at paths.
func dependenciesOn(paths ...string) string {
	quoted := make([]string, 0, len(paths))
	for _, path := range paths {
		quoted = append(quoted, strconv.Quote(path))
	}

	return "dependencies {\n  paths = [" + strings.Join(quoted, ", ") + "]\n}\n"
}

// dependencyChain returns the path of c and of each component after it on a chain of single dependencies.
func dependencyChain(c component.Component) []string {
	var chain []string

	for range 16 {
		chain = append(chain, c.Path())

		if len(c.Dependencies()) != 1 {
			break
		}

		c = c.Dependencies()[0]
	}

	return chain
}

func TestDiscoveryPipelineReportsParseErrorsWithRacing(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"ok/terragrunt.hcl":     "",
			"broken/terragrunt.hcl": "locals {\n",
		},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithParseExclude()
		},
	}

	_, phasesErr := run.discover(t.Context(), t)
	require.Error(t, phasesErr)

	run.usePipeline = true

	_, pipelineErr := run.discover(t.Context(), t)
	require.Error(t, pipelineErr)
}

func TestDiscoveryPipelineReportsCoexistenceWithRacing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		configure func(d *discovery.Discovery) *discovery.Discovery
		name      string
	}{
		{name: "parse errors reported"},
		{
			name: "parse errors suppressed",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithSuppressParseErrors()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := pipelineRun{
				files: map[string]string{
					"both/terragrunt.hcl":       "",
					"both/terragrunt.stack.hcl": "",
				},
				configure:   tt.configure,
				usePipeline: true,
			}

			_, err := run.discover(t.Context(), t)
			require.ErrorAs(t, err, new(discovery.CoexistenceError))
		})
	}
}

func TestDiscoveryReportsConfigConflictsBeforeParsingWithRacing(t *testing.T) {
	t.Parallel()

	conflicts := []struct {
		requireErr func(t *testing.T, err error)
		files      map[string]string
		name       string
	}{
		{
			name: "unit and stack",
			files: map[string]string{
				"both/terragrunt.hcl":       `locals { mark = run_cmd("mark", "unit") }`,
				"both/terragrunt.stack.hcl": `locals { mark = run_cmd("mark", "stack") }`,
			},
			requireErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorAs(t, err, new(discovery.CoexistenceError))
			},
		},
		{
			name: "both default unit configs",
			files: map[string]string{
				"both/terragrunt.hcl":      `locals { mark = run_cmd("mark", "hcl") }`,
				"both/terragrunt.hcl.json": `{"locals": {"mark": "${run_cmd(\"mark\", \"json\")}"}}`,
			},
			requireErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorAs(t, err, new(config.AmbiguousConfigError))
			},
		},
	}

	flows := []struct {
		configure   func(d *discovery.Discovery) *discovery.Discovery
		name        string
		usePipeline bool
	}{
		{
			name: "phases",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithParseExclude()
			},
		},
		{
			name: "phases with parse errors suppressed",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithParseExclude().WithSuppressParseErrors()
			},
		},
		{
			name: "pipeline",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithParseExclude()
			},
			usePipeline: true,
		},
		{
			name: "pipeline with parse errors suppressed",
			configure: func(d *discovery.Discovery) *discovery.Discovery {
				return d.WithParseExclude().WithSuppressParseErrors()
			},
			usePipeline: true,
		},
	}

	for _, conflict := range conflicts {
		for _, flow := range flows {
			t.Run(conflict.name+"/"+flow.name, func(t *testing.T) {
				t.Parallel()

				var (
					mu     sync.Mutex
					marked []string
				)

				run := pipelineRun{
					files:     conflict.files,
					configure: flow.configure,
					exec: vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
						mu.Lock()
						defer mu.Unlock()

						marked = append(marked, inv.Args...)

						return vexec.Result{}
					}),
					usePipeline: flow.usePipeline,
				}

				_, err := run.discover(t.Context(), t)
				conflict.requireErr(t, err)

				assert.Empty(t, marked)
			})
		}
	}
}

func TestDiscoveryFindsJSONUnitsWithRacing(t *testing.T) {
	t.Parallel()

	flows := []struct {
		name        string
		usePipeline bool
	}{
		{name: "phases"},
		{name: "pipeline", usePipeline: true},
	}

	for _, flow := range flows {
		t.Run(flow.name, func(t *testing.T) {
			t.Parallel()

			run := pipelineRun{
				files: map[string]string{
					"hcl-unit/terragrunt.hcl":       "",
					"json-unit/terragrunt.hcl.json": "{}",
				},
				configure: func(d *discovery.Discovery) *discovery.Discovery {
					return d.WithParseExclude()
				},
				usePipeline: flow.usePipeline,
			}

			got, err := run.discover(t.Context(), t)
			require.NoError(t, err)

			assert.ElementsMatch(t, []string{repoPath("hcl-unit"), repoPath("json-unit")}, got.Paths())
		})
	}
}

func TestDiscoveryPipelineStopsOnCancellationWithRacing(t *testing.T) {
	t.Parallel()

	files := make(map[string]string)
	for i := range 256 {
		files[fmt.Sprintf("units/unit-%d/terragrunt.hcl", i)] = ""
	}

	run := pipelineRun{
		files: files,
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithParseExclude()
		},
		usePipeline: true,
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := run.discover(ctx, t)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDiscoveryPipelineReportsCancellationDuringParsingWithRacing(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	run := pipelineRun{
		files: map[string]string{"unit/terragrunt.hcl": `locals { mark = run_cmd("cancel") }`},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithParseExclude()
		},
		exec: vexec.NewMemExec(func(context.Context, vexec.Invocation) vexec.Result {
			cancel()

			return vexec.Result{}
		}),
		usePipeline: true,
	}

	_, err := run.discover(ctx, t)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDiscoveryReportsConfigConflictsBeforeReadingDependenciesWithRacing(t *testing.T) {
	t.Parallel()

	for _, usePipeline := range []bool{false, true} {
		t.Run("pipeline="+strconv.FormatBool(usePipeline), func(t *testing.T) {
			t.Parallel()

			marks := &markRecorder{}

			run := pipelineRun{
				files: map[string]string{
					"a/terragrunt.hcl":       `locals { mark = run_cmd("mark", "a") }`,
					"z/terragrunt.hcl":       "",
					"z/terragrunt.stack.hcl": "",
				},
				configure: func(d *discovery.Discovery) *discovery.Discovery {
					return d.WithRelationships()
				},
				exec:        marks.exec(),
				usePipeline: usePipeline,
			}

			_, err := run.discover(t.Context(), t)
			require.ErrorAs(t, err, new(discovery.CoexistenceError))
			assert.Empty(t, marks.marked())
		})
	}
}

func TestDiscoveryPipelineRunsOnlyWhenSelectedWithRacing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantSpan    string
		usePipeline bool
	}{
		{name: "selected", usePipeline: true, wantSpan: "discovery_pipeline"},
		{name: "not selected", usePipeline: false, wantSpan: "discovery_phase_filesystem"},
	}

	spans := []string{"discovery_pipeline", "discovery_phase_filesystem"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := new(bytes.Buffer)

			tlm, err := telemetry.NewTelemeter(
				t.Context(),
				logger.CreateLogger(),
				"terragrunt",
				"v0.0.0-test",
				out,
				&telemetry.Options{TraceExporter: "console"},
				false,
			)
			require.NoError(t, err)

			run := pipelineRun{
				files:       map[string]string{"unit/terragrunt.hcl": ""},
				usePipeline: tt.usePipeline,
			}

			_, err = run.discover(telemetry.ContextWithTelemeter(t.Context(), tlm), t)
			require.NoError(t, err)
			require.NoError(t, tlm.Shutdown(t.Context()))

			for _, span := range spans {
				assert.Equal(t, span == tt.wantSpan, bytes.Contains(out.Bytes(), []byte(`"`+span+`"`)), span)
			}
		})
	}
}
