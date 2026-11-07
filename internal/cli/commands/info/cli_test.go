package info_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/strict"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subcommandsTemplate renders one subcommand name per line in place of the app's help template.
const subcommandsTemplate = "{{range .Command.Subcommands}}{{.Name}}\n{{end}}"

// TestNewCommandShowsHelpWithItsSubcommands pins that bare `info` prints help naming its subcommands.
func TestNewCommandShowsHelpWithItsSubcommands(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	cmd := info.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(v.Exec), v)
	cmd.CustomHelpTemplate = subcommandsTemplate

	out := new(bytes.Buffer)

	app := clihelper.NewApp(map[string]string{})
	app.Writer = out
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	require.NoError(t, cmd.Run(t.Context(), clihelper.NewAppContext(app, nil), nil))
	assert.Equal(t, []string{strict.CommandName, print.CommandName}, strings.Fields(out.String()))
}
