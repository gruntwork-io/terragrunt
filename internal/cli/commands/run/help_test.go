package run_test

import (
	"bytes"
	"context"
	"slices"
	"testing"

	runcommand "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShowTFHelpIncludesTheBinaryHelp pins the OpenTofu/Terraform help `run <command> --help` prints.
func TestShowTFHelpIncludesTheBinaryHelp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		wantBinary string
		args       []string
		want       []string
		notWant    []string
		result     vexec.Result
	}{
		{
			name:       "help body without its usage line",
			result:     vexec.Result{Stdout: []byte("\nUsage: tofu [global options] plan [options]\n\n  Generates a plan.\n")},
			wantBinary: "tofu",
			want:       []string{"`tofu plan` help:", "  Generates a plan."},
			notWant:    []string{"Usage: tofu [global options]"},
		},
		{
			name:       "usage-only help is kept whole",
			result:     vexec.Result{Stdout: []byte("Usage: tofu plan\n")},
			wantBinary: "tofu",
			want:       []string{"Usage: tofu plan"},
		},
		{
			name:       "binary from the tf-path flag",
			args:       []string{"--tf-path", "/opt/bin/terraform"},
			result:     vexec.Result{Stdout: []byte("  Generates a plan.\n")},
			wantBinary: "/opt/bin/terraform",
			want:       []string{"`terraform plan` help:", "  Generates a plan."},
		},
		{
			name:       "binary that exits non-zero",
			result:     vexec.Result{ExitCode: 1},
			wantBinary: "tofu",
			want:       []string{"Failed to execute \"tofu -help plan\""},
		},
		{
			name:       "binary that cannot run",
			result:     vexec.Result{Err: vexec.ErrNoSpawn},
			wantBinary: "tofu",
			want:       []string{"Failed to execute \"tofu -help plan\""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var invoked []string

			v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				invoked = append([]string{inv.Name}, inv.Args...)

				return tc.result
			})

			opts := newOpts(t, v, "", "")

			out, err := showTFHelp(t, runcommand.ShowTFHelp(logger.CreateLogger(), opts, v), tc.args)
			require.NoError(t, err)
			assert.Equal(t, []string{tc.wantBinary, tf.FlagNameHelpLong, tf.CommandNamePlan}, invoked)

			for _, fragment := range tc.want {
				assert.Contains(t, out, fragment)
			}

			for _, fragment := range tc.notWant {
				assert.NotContains(t, out, fragment)
			}
		})
	}
}

// TestShowTFHelpRejectsAnIncompleteTFPathFlag pins that a --tf-path without a value fails before running anything.
func TestShowTFHelpRejectsAnIncompleteTFPathFlag(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := rec.venv(t, nil)

	_, err := showTFHelp(t, runcommand.ShowTFHelp(logger.CreateLogger(), newOpts(t, v, "", ""), v), []string{"--tf-path"})
	require.Error(t, err)
	assert.Empty(t, rec.invocations(), "the binary never ran")
}

// showTFHelp renders help for `run plan` with args through fn and returns what it printed.
func showTFHelp(t *testing.T, fn clihelper.HelpFunc, args []string) (string, error) {
	t.Helper()

	out := new(bytes.Buffer)

	app := clihelper.NewApp(map[string]string{})
	app.Writer = out

	runCmd := &clihelper.Command{Name: runcommand.CommandName}
	planCmd := &clihelper.Command{Name: tf.CommandNamePlan, Usage: "Show changes required by the current configuration."}

	cliCtx := clihelper.NewAppContext(app, nil).
		NewCommandContext(runCmd, nil).
		NewCommandContext(planCmd, slices.Clone(args))

	err := fn(t.Context(), cliCtx)

	return out.String(), err
}
