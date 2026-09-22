package login_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/login"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommandIsNamedLogin(t *testing.T) {
	t.Parallel()

	cmd := login.NewCommand(
		logger.CreateLogger(),
		options.NewTerragruntOptions(vexec.NewOSExec()),
		venvtest.New(),
	)

	require.NotNil(t, cmd)
	assert.Equal(t, login.CommandName, cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Flags)
}

// TestLoginIsReachableFromTheCommandLine confirms that Terragrunt registers the
// login command.
func TestLoginIsReachableFromTheCommandLine(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions(vexec.NewOSExec())

	err := cli.NewApp(l, opts, v).Run(l, v, []string{"terragrunt", login.CommandName})

	require.ErrorIs(t, err, login.ErrExperimentRequired)
}

// TestExperimentGateRefusesTheRun confirms that the tg-login experiment is
// required to use the login command.
func TestExperimentGateRefusesTheRun(t *testing.T) {
	t.Parallel()

	tc := []struct {
		wantErr        error
		name           string
		withExperiment bool
	}{
		{
			name:           "experiment off",
			withExperiment: false,
			wantErr:        login.ErrExperimentRequired,
		},
		{
			name:           "experiment on",
			withExperiment: true,
			wantErr:        nil,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			if tt.withExperiment {
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.TGLogin))
			}

			cmd := login.NewCommand(logger.CreateLogger(), opts, venvtest.New())

			err := cmd.Before(t.Context(), &clihelper.Context{})

			if tt.wantErr == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, tt.wantErr)

			var exitErr clihelper.ExitCoder

			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, int(clihelper.ExitCodeGeneralError), exitErr.ExitCode())
		})
	}
}

// TestNewCommandStaysVisibleWithoutTheExperiment pins that a disabled
// experiment still leaves the command in `--help`.
func TestNewCommandStaysVisibleWithoutTheExperiment(t *testing.T) {
	t.Parallel()

	cmd := login.NewCommand(
		logger.CreateLogger(),
		options.NewTerragruntOptions(vexec.NewOSExec()),
		venvtest.New(),
	)

	assert.False(t, cmd.Hidden)
}

// TestExperimentIsRegistered confirms the experiment name the gate reads is one
// a user can actually enable. EnableExperiment rejects an unregistered name.
func TestExperimentIsRegistered(t *testing.T) {
	t.Parallel()

	exps := experiment.NewExperiments()
	assert.False(t, exps.Evaluate(experiment.TGLogin))

	require.NoError(t, exps.EnableExperiment(experiment.TGLogin))
	assert.True(t, exps.Evaluate(experiment.TGLogin))
}
