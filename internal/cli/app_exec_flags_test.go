package cli_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/exec"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecAcceptsSourceMapAndNoAutoInit drives the real app so that a missing flag registration
// fails the same way it does for a user, with an undefined-flag error from the `exec` command.
func TestExecAcceptsSourceMapAndNoAutoInit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		env               map[string]string
		expectedSourceMap map[string]string
		args              []string
		expectedAutoInit  bool
	}{
		{
			name:              "no flags",
			args:              []string{exec.CommandName, "--", "echo", "hi"},
			expectedSourceMap: map[string]string{},
			expectedAutoInit:  true,
		},
		{
			name: "source map flag",
			args: []string{
				exec.CommandName,
				doubleDashed(run.SourceMapFlagName),
				"git::ssh://git@github.com/acme/vpc.git=/local/vpc",
				"--",
				"echo",
				"hi",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/local/vpc",
			},
			expectedAutoInit: true,
		},
		{
			name:              "no auto init flag",
			args:              []string{exec.CommandName, doubleDashed(run.NoAutoInitFlagName), "--", "echo", "hi"},
			expectedSourceMap: map[string]string{},
		},
		{
			name: "both flags together",
			args: []string{
				exec.CommandName,
				doubleDashed(run.SourceMapFlagName),
				"git::ssh://git@github.com/acme/vpc.git=/local/vpc",
				doubleDashed(run.NoAutoInitFlagName),
				"--",
				"echo",
				"hi",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/local/vpc",
			},
		},
		{
			name: "deprecated source map env var",
			args: []string{exec.CommandName, "--", "echo", "hi"},
			env: map[string]string{
				"TERRAGRUNT_SOURCE_MAP": "git::ssh://git@github.com/acme/vpc.git=/local/vpc",
			},
			expectedSourceMap: map[string]string{
				"git::ssh://git@github.com/acme/vpc.git": "/local/vpc",
			},
			expectedAutoInit: true,
		},
		{
			name:              "deprecated auto init env var",
			args:              []string{exec.CommandName, "--", "echo", "hi"},
			env:               map[string]string{"TERRAGRUNT_AUTO_INIT": "false"},
			expectedSourceMap: map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			require.NoError(t, runExecAppTest(t, opts, tc.env, tc.args))
			assert.Equal(t, tc.expectedSourceMap, opts.SourceMap)
			assert.Equal(t, tc.expectedAutoInit, opts.AutoInit)
		})
	}
}

func runExecAppTest(
	t *testing.T,
	opts *options.TerragruntOptions,
	env map[string]string,
	args []string,
) error {
	t.Helper()

	if env == nil {
		env = map[string]string{}
	}

	l := logger.CreateLogger()
	v := venvtest.New().WithEnv(env)

	app := cli.NewApp(l, opts, v)
	setCommandAction(func(context.Context, *clihelper.Context) error { return nil }, app.Commands...)

	return app.Run(l, v, append([]string{"terragrunt"}, args...))
}
