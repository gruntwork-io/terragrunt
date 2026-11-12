package discovery_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// pipelineRun is one discovery over a fixture, through the pipeline or the phases.
type pipelineRun struct {
	// configure applies the options under test to the discovery.
	configure func(d *discovery.Discovery) *discovery.Discovery
	// files maps each path below the corpus repo root to its contents.
	files map[string]string
	// queries are the filter queries the discovery runs with.
	queries []string
	// usePipeline selects the pipeline through [discovery.EnvPipeline].
	usePipeline bool
}

// discover runs the discovery with ctx and returns what it found.
func (r pipelineRun) discover(ctx context.Context, t *testing.T) (component.Components, error) {
	t.Helper()

	v := memRepoRootVenv(t, corpusRepoRoot)
	writeFixture(t, v, prefixPaths(r.files))

	if r.usePipeline {
		v.Env[discovery.EnvPipeline] = "true"
	}

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = corpusRepoRoot
	opts.RootWorkingDir = corpusRepoRoot

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, r.queries)
	require.NoError(t, err)

	d := discovery.NewDiscovery(corpusRepoRoot).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: corpusRepoRoot}).
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

func TestDiscoveryPipelineRelatesDependencyCyclesOutsideTheFilters(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"apps/api/terragrunt.hcl":      dependenciesOn("../../infra/vpc"),
			"infra/vpc/terragrunt.hcl":     dependenciesOn("../network"),
			"infra/network/terragrunt.hcl": dependenciesOn("../vpc"),
		},
		queries: []string{"./apps/**"},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithRelationships()
		},
		usePipeline: true,
	}

	got, err := run.discover(t.Context(), t)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Dependencies(), 1)

	vpc := got[0].Dependencies()[0]

	require.Len(t, vpc.Dependencies(), 1)
	assert.Equal(t, component.Components{vpc}, vpc.Dependencies()[0].Dependencies())
}

func TestDiscoveryPipelineReportsDependenciesWithoutAConfig(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"apps/api/terragrunt.hcl":  dependenciesOn("../../infra/vpc"),
			"infra/vpc/terragrunt.hcl": dependenciesOn("../missing"),
		},
		queries: []string{"./apps/**"},
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
	require.Len(t, got, 1)

	assert.Equal(
		t,
		[]string{
			filepath.Join(corpusRepoRoot, "apps", "api"),
			filepath.Join(corpusRepoRoot, "infra", "vpc"),
			filepath.Join(corpusRepoRoot, "infra", "missing"),
		},
		dependencyChain(got[0]),
	)
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

func TestDiscoveryPipelineKeepsOneUnitPerDirectory(t *testing.T) {
	t.Parallel()

	run := pipelineRun{
		files: map[string]string{
			"unit/a.hcl": `locals { name = "a" }`,
			"unit/b.hcl": "locals {\n",
		},
		configure: func(d *discovery.Discovery) *discovery.Discovery {
			return d.WithConfigFilenames([]string{"a.hcl", "b.hcl"}).WithParseExclude()
		},
	}

	want, err := run.discover(t.Context(), t)
	require.NoError(t, err)
	require.Len(t, want, 1)

	run.usePipeline = true

	got, err := run.discover(t.Context(), t)
	require.NoError(t, err)

	assert.Equal(t, componentViews(want), componentViews(got))
}

func TestDiscoveryPipelineReportsParseErrors(t *testing.T) {
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

func TestDiscoveryPipelineReportsCoexistence(t *testing.T) {
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

func TestDiscoveryPipelineStopsOnCancellation(t *testing.T) {
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

func TestDiscoveryPipelineRunsOnlyWhenSelected(t *testing.T) {
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
