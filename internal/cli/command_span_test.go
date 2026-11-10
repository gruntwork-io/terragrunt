package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errGetwd = errors.New("fake getwd failure")

func TestWrapWithTelemetryCommandSpan(t *testing.T) {
	t.Parallel()

	workDir := venvtest.Root("/repo/live")

	testCases := []struct {
		wantAttrs map[string]any
		name      string
		wantName  string
		args      []string
	}{
		{
			name:     "run",
			args:     []string{"run", "--", "plan", "-input=false"},
			wantName: "plan",
			wantAttrs: map[string]any{
				"dir":          workDir,
				"tofu_command": "plan",
				"args":         []any{"plan", "-input=false", "-no-color"},
			},
		},
		{
			name:     "run all",
			args:     []string{"run", "--all", "--", "plan"},
			wantName: "plan",
			wantAttrs: map[string]any{
				"dir":          workDir,
				"tofu_command": "plan",
				"args":         []any{"plan", "-no-color"},
			},
		},
		{
			name:     "shortcut",
			args:     []string{"plan"},
			wantName: "plan",
			wantAttrs: map[string]any{
				"dir":          workDir,
				"tofu_command": "plan",
				"args":         []any{"plan", "-no-color"},
			},
		},
		{
			name:      "run without a tofu command",
			args:      []string{"run"},
			wantName:  "run",
			wantAttrs: map[string]any{"dir": workDir},
		},
		{
			name:      "command that does not run tofu",
			args:      []string{"find"},
			wantName:  "find",
			wantAttrs: map[string]any{"dir": workDir},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testV := venv.MemVenv()
			testV.Env["TG_TELEMETRY_TRACE_EXPORTER"] = "console"

			stdout := runWithTelemetry(t, testV, append([]string{"--working-dir", workDir}, tc.args...), nil)

			roots := rootSpans(t, stdout)
			require.Len(t, roots, 1, "expected one command span")
			assert.Equal(t, tc.wantName, roots[0].Name)

			gotAttrs := make(map[string]any, len(roots[0].Attributes))
			for _, attr := range roots[0].Attributes {
				gotAttrs[attr.Key] = attr.Value.Value
			}

			assert.Equal(t, tc.wantAttrs, gotAttrs)
		})
	}
}

func TestWrapWithTelemetryCommandMetricDir(t *testing.T) {
	t.Parallel()

	workDir := venvtest.Root("/repo/live")

	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "working dir flag",
			args: []string{"--working-dir", workDir, "find"},
		},
		{
			name: "current directory",
			args: []string{"find"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testV := venv.MemVenv()
			testV.Env["TG_TELEMETRY_METRIC_EXPORTER"] = "console"
			testV.Platform.Getwd = func() (string, error) { return workDir, nil }

			stdout := runWithTelemetry(t, testV, tc.args, nil)

			got := metricDataPoints(t, stdout, "find_duration")
			require.NotEmpty(t, got, "expected a find_duration data point")

			for _, attrs := range got {
				assert.Equal(t, map[string]any{"dir": workDir}, attrs)
			}
		})
	}
}

func TestWrapWithTelemetryWorkingDirErrorMarksSpanFailed(t *testing.T) {
	t.Parallel()

	testV := venv.MemVenv()
	testV.Env["TG_TELEMETRY_TRACE_EXPORTER"] = "console"
	testV.Platform.Getwd = func() (string, error) { return "", errGetwd }

	roots := rootSpans(t, runWithTelemetry(t, testV, []string{"find"}, errGetwd))

	require.Len(t, roots, 1, "expected one command span")
	assert.Equal(t, "find", roots[0].Name)
	assert.Equal(t, "Error", roots[0].Status.Code)
}

func TestWrapWithTelemetryWorkingDirErrorCountsFailure(t *testing.T) {
	t.Parallel()

	testV := venv.MemVenv()
	testV.Env["TG_TELEMETRY_METRIC_EXPORTER"] = "console"
	testV.Platform.Getwd = func() (string, error) { return "", errGetwd }

	stdout := runWithTelemetry(t, testV, []string{"find"}, errGetwd)

	assert.NotEmpty(t, metricDataPoints(t, stdout, "find_errors_count"))
}

// runWithTelemetry runs args through the CLI with a no-op action on every
// command, requires the run to fail with wantErr, or to succeed when wantErr is
// nil, and returns what the console exporters wrote.
func runWithTelemetry(t *testing.T, testV *venv.Venv, args []string, wantErr error) *bytes.Buffer {
	t.Helper()

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	terragruntCommands := commands.New(l, opts, testV, new(rewrite.Store))
	setCommandAction(
		func(context.Context, *clihelper.Context) error { return nil },
		terragruntCommands...)

	var stdout bytes.Buffer

	app := clihelper.NewApp(testV.Env)
	app.Writer = &stdout
	app.ErrWriter = io.Discard

	app.Flags = append(global.NewFlags(l, opts, nil), run.NewFlags(l, opts, testV, nil)...)
	app.Commands = terragruntCommands.WrapAction(commands.WrapWithTelemetry(l, opts, testV))

	require.ErrorIs(t, app.RunContext(config.WithCaches(t.Context()), append([]string{"terragrunt"}, args...)), wantErr)

	return &stdout
}

// rootSpans decodes the console-exported spans in r and returns those with no parent.
func rootSpans(t *testing.T, r io.Reader) []exportedSpan {
	t.Helper()

	var roots []exportedSpan

	dec := json.NewDecoder(r)
	for dec.More() {
		var span exportedSpan
		require.NoError(t, dec.Decode(&span))

		if span.Parent.SpanID == "0000000000000000" {
			roots = append(roots, span)
		}
	}

	return roots
}

// metricDataPoints decodes the console-exported metrics in r and returns the
// attributes of each data point of the named metric.
func metricDataPoints(t *testing.T, r io.Reader, name string) []map[string]any {
	t.Helper()

	var points []map[string]any

	dec := json.NewDecoder(r)
	for dec.More() {
		var export exportedMetrics
		require.NoError(t, dec.Decode(&export))

		points = append(points, export.dataPointAttrs(name)...)
	}

	return points
}

// exportedSpan holds the fields of a console-exported span that the tests in
// this file read.
type exportedSpan struct {
	Name   string `json:"Name"`
	Parent struct {
		SpanID string `json:"SpanID"`
	} `json:"Parent"`
	Status struct {
		Code string `json:"Code"`
	} `json:"Status"`
	Attributes []struct {
		Value struct {
			Value any `json:"Value"`
		} `json:"Value"`
		Key string `json:"Key"`
	} `json:"Attributes"`
}

// exportedMetrics holds the fields of a console-exported metrics batch that the
// tests in this file read.
type exportedMetrics struct {
	ScopeMetrics []struct {
		Metrics []struct {
			Name string `json:"Name"`
			Data struct {
				DataPoints []struct {
					Attributes []struct {
						Value struct {
							Value any `json:"Value"`
						} `json:"Value"`
						Key string `json:"Key"`
					} `json:"Attributes"`
				} `json:"DataPoints"`
			} `json:"Data"`
		} `json:"Metrics"`
	} `json:"ScopeMetrics"`
}

// dataPointAttrs returns the attributes of each data point of the named metric.
func (e exportedMetrics) dataPointAttrs(name string) []map[string]any {
	var points []map[string]any

	for _, scope := range e.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}

			for _, point := range metric.Data.DataPoints {
				attrs := make(map[string]any, len(point.Attributes))
				for _, attr := range point.Attributes {
					attrs[attr.Key] = attr.Value.Value
				}

				points = append(points, attrs)
			}
		}
	}

	return points
}
