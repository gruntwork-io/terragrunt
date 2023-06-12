package cli

import (
	"io"

	"github.com/gruntwork-io/go-commons/version"

	"github.com/gruntwork-io/terragrunt/cli/commands"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	validateinputs "github.com/gruntwork-io/terragrunt/cli/commands/validate-inputs"

	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/env"
	"github.com/gruntwork-io/terragrunt/shell"
)

func init() {
	cli.AppHelpTemplate = appHelpTemplate
	cli.CommandHelpTemplate = commandHelpTemplate
}

// NewApp creates the Terragrunt CLI App.
func NewApp(writer io.Writer, errwriter io.Writer) *cli.App {
	opts := options.NewTerragruntOptions()
	// The env vars are renamed to "..._NO_AUTO_..." in the gobal flags`. These ones are left for backwards compatibility.
	opts.AutoInit = env.GetBoolEnv("TERRAGRUNT_AUTO_INIT", opts.AutoInit)
	opts.AutoRetry = env.GetBoolEnv("TERRAGRUNT_AUTO_RETRY", opts.AutoRetry)
	opts.RunAllAutoApprove = env.GetBoolEnv("TERRAGRUNT_AUTO_APPROVE", opts.RunAllAutoApprove)

	app := cli.NewApp()
	app.Name = "terragrunt"
	app.Usage = "Terragrunt is a thin wrapper for Terraform that provides extra tools for working with multiple\nTerraform modules, remote state, and locking. For documentation, see https://github.com/gruntwork-io/terragrunt/."
	app.UsageText = "terragrunt <command> [global options]"
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version.GetVersion()
	app.Writer = writer
	app.ErrWriter = errwriter
	app.AddFlags(commands.NewGlobalFlags(opts)...)
	app.AddCommands(append(
		commands.NewDeprecatedCommands(opts),
		runall.NewCommand(opts),            // run-all
		terragruntinfo.NewCommand(opts),    // terragrunt-info
		validateinputs.NewCommand(opts),    // validate-inputs
		graphdependencies.NewCommand(opts), // graph-dependencies
		hclfmt.NewCommand(opts),            // hclfmt
		renderjson.NewCommand(opts),        // render-json
		awsproviderpatch.NewCommand(opts),  // aws-provider-patch
		terraform.NewCommand(opts),         // * (to show in app help)
	)...)
	app.Before = func(ctx *cli.Context) error {
		if showHelp := ctx.Flags.Get(commands.FlagNameHelp).Value().IsSet(); showHelp {
			ctx.Command.Action = nil

			// if app command is specified show the command help.
			if !ctx.Command.IsRoot {
				return cli.ShowCommandHelp(ctx, ctx.Command.Name)
			}

			// if there is no args at all show the app help.
			if !ctx.Args().Present() || ctx.Args().First() == terraform.CommandName {
				return cli.ShowAppHelp(ctx)
			}

			// in other cases show the Terraform help.
			terraformHelpCmd := append([]string{ctx.Args().First(), "-help"}, ctx.Args().Tail()...)
			return shell.RunTerraformCommand(opts, terraformHelpCmd...)
		}

		return nil
	}
	app.Action = terraform.CommandAction(opts) // run when no terragrunt command is specified

	return app
}
