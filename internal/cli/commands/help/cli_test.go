package help_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/help"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCommandShowsHelpForTheNamedCommand pins which help `terragrunt help <args>` prints.
func TestNewCommandShowsHelpForTheNamedCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want string
		args []string
	}{
		{name: "no command shows the app help", want: "app help"},
		{name: "unknown command shows the app help", args: []string{"nope"}, want: "app help"},
		{name: "known command shows its help", args: []string{"outer"}, want: "outer help"},
		{name: "nested command shows its help", args: []string{"outer", "inner"}, want: "inner help"},
		{
			name: "unknown nested command shows the parent help",
			args: []string{"outer", "nope"},
			want: "outer help",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := runHelp(t, log.New(log.WithLevel(log.InfoLevel)), tc.args...)
			require.NoError(t, err)
			assert.Equal(t, tc.want, out)
		})
	}
}

// TestNewCommandEnablesTemplateErrorsUnderDebugLogging pins that debug logging surfaces help template errors.
func TestNewCommandEnablesTemplateErrorsUnderDebugLogging(t *testing.T) {
	t.Parallel()

	out, err := runHelp(t, logger.CreateLogger(), "outer")
	require.NoError(t, err)
	assert.Equal(t, "outer help", out)
	assert.Equal(t, "1", venv.OSVenv().Env[templateErrorDebugEnv], "urfave/cli reads this to print template errors")
}

func TestNewCommandSetsTemplateErrorDebugUnderDebugLogging(t *testing.T) {
	t.Setenv(templateErrorDebugEnv, "")

	out, err := runHelp(t, logger.CreateLogger(), "outer")
	require.NoError(t, err)
	assert.Equal(t, "outer help", out)
	assert.Equal(t, "1", venv.OSVenv().Env[templateErrorDebugEnv], "debug logging should set the variable")
}

// runHelp runs `help args` in an app holding an outer command with one inner subcommand.
func runHelp(t *testing.T, l log.Logger, args ...string) (string, error) {
	t.Helper()

	out := new(bytes.Buffer)

	cmd := help.NewCommand(l, options.NewTerragruntOptions())

	app := clihelper.NewApp(map[string]string{})
	app.Writer = out
	app.CustomAppHelpTemplate = "app help"
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }
	app.Commands = clihelper.Commands{
		&clihelper.Command{
			Name:               "outer",
			CustomHelpTemplate: "outer help",
			Subcommands: clihelper.Commands{
				&clihelper.Command{Name: "inner", CustomHelpTemplate: "inner help"},
			},
		},
		cmd,
	}

	err := cmd.Run(t.Context(), clihelper.NewAppContext(app, args), args)

	return out.String(), err
}

const templateErrorDebugEnv = "CLI_TEMPLATE_ERROR_DEBUG"
