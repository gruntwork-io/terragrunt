package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

			l := logger.CreateLogger()
			l.SetOptions(log.WithOutput(io.Discard))

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.NoAutoProviderCacheDir = true

			testV := venvtest.New()
			testV.Env["TG_TELEMETRY_TRACE_EXPORTER"] = "console"

			terragruntCommands := commands.New(l, opts, testV)
			setCommandAction(
				func(context.Context, *clihelper.Context) error { return nil },
				terragruntCommands...)

			var stdout bytes.Buffer

			app := clihelper.NewApp(testV.Env)
			app.Writer = &stdout
			app.ErrWriter = io.Discard

			app.Flags = append(global.NewFlags(l, opts, nil), run.NewFlags(l, opts, testV, nil)...)
			app.Commands = terragruntCommands.WrapAction(commands.WrapWithTelemetry(l, opts, testV))

			args := append([]string{"terragrunt", "--working-dir", workDir}, tc.args...)
			require.NoError(t, app.RunContext(config.WithCaches(t.Context()), args))

			var roots []exportedSpan

			dec := json.NewDecoder(&stdout)
			for dec.More() {
				var span exportedSpan
				require.NoError(t, dec.Decode(&span))

				if span.Parent.SpanID == "0000000000000000" {
					roots = append(roots, span)
				}
			}

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

// exportedSpan holds the fields of a console-exported span that
// TestWrapWithTelemetryCommandSpan reads.
type exportedSpan struct {
	Name   string `json:"Name"`
	Parent struct {
		SpanID string `json:"SpanID"`
	} `json:"Parent"`
	Attributes []struct {
		Value struct {
			Value any `json:"Value"`
		} `json:"Value"`
		Key string `json:"Key"`
	} `json:"Attributes"`
}
