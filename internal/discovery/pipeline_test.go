package discovery_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
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
	// exec runs the commands the configs call. When nil, the venv spawns nothing and every command fails.
	exec vexec.Exec
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

	if r.exec != nil {
		v = v.WithExec(r.exec)
	}

	if r.usePipeline {
		v.Env[discovery.EnvPipeline] = "true"
	}

	opts := options.NewTerragruntOptions()
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

func TestDiscoveryPipelineReportsCoexistenceBeforeParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu     sync.Mutex
				marked []string
			)

			run := pipelineRun{
				files: map[string]string{
					"both/terragrunt.hcl":       `locals { mark = run_cmd("mark", "unit") }`,
					"both/terragrunt.stack.hcl": `locals { mark = run_cmd("mark", "stack") }`,
				},
				configure: tt.configure,
				exec: vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
					mu.Lock()
					defer mu.Unlock()

					marked = append(marked, inv.Args...)

					return vexec.Result{}
				}),
				usePipeline: tt.usePipeline,
			}

			_, err := run.discover(t.Context(), t)
			require.ErrorAs(t, err, new(discovery.CoexistenceError))

			assert.Empty(t, marked)
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
