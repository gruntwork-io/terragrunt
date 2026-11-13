package dag_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/dag"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/dag/graph"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subcommandsTemplate renders one subcommand name per line in place of the app's help template.
const subcommandsTemplate = "{{range .Command.Subcommands}}{{.Name}}\n{{end}}"

// TestNewCommandShowsHelpWithItsSubcommands pins that bare `dag` prints help naming its subcommands.
func TestNewCommandShowsHelpWithItsSubcommands(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()
	cmd := dag.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(), v, new(rewrite.Store))
	cmd.CustomHelpTemplate = subcommandsTemplate

	out := new(bytes.Buffer)

	require.NoError(t, runCommand(t, cmd, out))
	assert.Equal(t, []string{graph.CommandName}, strings.Fields(out.String()))
}

// TestNewCommandRunsTheGraphSubcommand pins that `dag graph` renders the stack as DOT.
func TestNewCommandRunsTheGraphSubcommand(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/dag")
	out := new(bytes.Buffer)
	v := venv.MemVenv().WithWriter(out).WithFS(venvtest.NewFS(t, root, map[string]string{
		"app/terragrunt.hcl": "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\n",
		"vpc/terragrunt.hcl": "",
	}))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = root
	opts.RootWorkingDir = root

	require.NoError(
		t,
		runCommand(
			t,
			dag.NewCommand(logger.CreateLogger(), opts, v, new(rewrite.Store)),
			new(bytes.Buffer),
			graph.CommandName,
		),
	)
	assert.Equal(t, "digraph {\n\t\"app\" ;\n\t\"app\" -> \"vpc\";\n\t\"vpc\" ;\n}\n", out.String())
}

// runCommand runs cmd with args the way the app does, printing help into out.
func runCommand(t *testing.T, cmd *clihelper.Command, out *bytes.Buffer, args ...string) error {
	t.Helper()

	app := clihelper.NewApp(map[string]string{})
	app.Writer = out
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	return cmd.Run(config.WithCaches(t.Context()), clihelper.NewAppContext(app, args), args)
}
