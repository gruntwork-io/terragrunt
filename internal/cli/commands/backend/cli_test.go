package backend_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/bootstrap"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/delete"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend/migrate"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subcommandsTemplate renders one subcommand name per line in place of the app's help template.
const subcommandsTemplate = "{{range .Command.Subcommands}}{{.Name}}\n{{end}}"

// TestNewCommandShowsHelpWithItsSubcommands pins that bare `backend` prints help naming its subcommands.
func TestNewCommandShowsHelpWithItsSubcommands(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	cmd := backend.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(v.Exec), v)
	cmd.CustomHelpTemplate = subcommandsTemplate

	out, err := runCommand(t, cmd)
	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{bootstrap.CommandName, delete.CommandName, migrate.CommandName},
		strings.Fields(out),
	)
}

// TestNewCommandRunsTheNamedSubcommand pins that `backend bootstrap` reaches the bootstrap action.
func TestNewCommandRunsTheNamedSubcommand(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/backend")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{
		"terragrunt.hcl": "this is ) not ( valid hcl",
	}))

	opts, err := options.NewTerragruntOptionsWithConfigPath(v.Exec, filepath.Join(root, "terragrunt.hcl"))
	require.NoError(t, err)

	_, err = runCommand(t, backend.NewCommand(logger.CreateLogger(), opts, v), bootstrap.CommandName)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags, "bootstrap parsed the unit config")
}

// runCommand runs cmd with args the way the app does and returns what it printed.
func runCommand(t *testing.T, cmd *clihelper.Command, args ...string) (string, error) {
	t.Helper()

	out := new(bytes.Buffer)

	app := clihelper.NewApp(map[string]string{})
	app.Writer = out
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	err := cmd.Run(config.WithCaches(t.Context()), clihelper.NewAppContext(app, args), args)

	return out.String(), err
}
