package cli_test

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandHelpTemplate(t *testing.T) {
	t.Parallel()

	// Set environment variable format based on OS
	envVarChar := "$"
	closeEnvVarChar := ""

	if runtime.GOOS == "windows" {
		envVarChar = "%"
		closeEnvVarChar = "%"
	}

	tgPrefix := flags.Prefix{flags.TgPrefix}

	app := cli.NewApp()
	app.Flags = cli.Flags{
		&cli.GenericFlag[string]{
			Name:    "working-dir",
			EnvVars: tgPrefix.EnvVars("working-dir"),
			Usage:   "The path to the directory of Terragrunt configurations. Default is current directory.",
		},
		&cli.BoolFlag{
			Name:    "log-disable",
			EnvVars: tgPrefix.EnvVars("log-disable"),
			Usage:   "Disable logging.",
		},
	}.Sort()

	cmd := &cli.Command{
		Name:        "run",
		Usage:       "Run an OpenTofu/Terraform command.",
		UsageText:   "terragrunt run [options] -- <tofu/terraform command>",
		Description: "Run a command, passing arguments to an orchestrated tofu/terraform binary.\n\nThis is the explicit, and most flexible form of running an IaC command with Terragrunt. Shortcuts can be found in \"terragrunt --help\" for common use-cases.",
		Examples: []string{
			"# Run a plan\nterragrunt run -- plan\n# Shortcut:\n# terragrunt plan",
			"# Run output with -json flag\nterragrunt run -- output -json\n# Shortcut:\n# terragrunt output -json",
			"# Run a plan against a Stack of configurations in the current directory\nterragrunt run --all -- plan",
		},
		Subcommands: cli.Commands{
			&cli.Command{
				Name:  "fmt",
				Usage: "Recursively find hcl files and rewrite them into a canonical format.",
			},
			&cli.Command{
				Name:  "validate",
				Usage: "Find all hcl files from the config stack and validate them.",
			},
		},
		Flags: cli.Flags{
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				EnvVars: tgPrefix.EnvVars("all"),
				Usage:   `Run the specified OpenTofu/Terraform command on the "Stack" of Units in the current directory.`,
			},
			&cli.BoolFlag{
				Name:    "graph",
				EnvVars: tgPrefix.EnvVars("graph"),
				Usage:   "Run the specified OpenTofu/Terraform command following the Directed Acyclic Graph (DAG) of dependencies.",
			},
		},
	}

	var out bytes.Buffer

	app.Writer = &out

	ctx := cli.NewAppContext(t.Context(), app, nil).NewCommandContext(cmd, nil)
	require.Error(t, cli.ShowCommandHelp(ctx))

	expectedOutput := fmt.Sprintf(`Usage: terragrunt run [options] -- <tofu/terraform command>

   Run a command, passing arguments to an orchestrated tofu/terraform binary.

   This is the explicit, and most flexible form of running an IaC command with Terragrunt. Shortcuts can be found in "terragrunt --help" for common use-cases.

Examples:
   # Run a plan
   terragrunt run -- plan
   # Shortcut:
   # terragrunt plan

   # Run output with -json flag
   terragrunt run -- output -json
   # Shortcut:
   # terragrunt output -json

   # Run a plan against a Stack of configurations in the current directory
   terragrunt run --all -- plan

Commands:
   fmt        Recursively find hcl files and rewrite them into a canonical format.
   validate   Find all hcl files from the config stack and validate them.

Options:
   --all, -a  Run the specified OpenTofu/Terraform command on the "Stack" of Units in the current directory. [%sTG_ALL%s]
   --graph    Run the specified OpenTofu/Terraform command following the Directed Acyclic Graph (DAG) of dependencies. [%sTG_GRAPH%s]

Global Options:
   --log-disable        Disable logging. [%sTG_LOG_DISABLE%s]
   --working-dir value  The path to the directory of Terragrunt configurations. Default is current directory. [%sTG_WORKING_DIR%s]

`, envVarChar, closeEnvVarChar, envVarChar, closeEnvVarChar, envVarChar, closeEnvVarChar, envVarChar, closeEnvVarChar)

	assert.Equal(t, expectedOutput, out.String())
}
