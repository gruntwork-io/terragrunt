package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	validateinputs "github.com/gruntwork-io/terragrunt/cli/commands/validate-inputs"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
	hashicorpversion "github.com/hashicorp/go-version"

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
		newDeprecatedCommands(opts),
		newCommands(opts)...,
	)...)
	app.Before = func(ctx *cli.Context) error {
		if showHelp := ctx.Flags.Get(commands.FlagNameHelp).Value().IsSet(); showHelp {
			ctx.Command.Action = nil

			// if app command is specified show the command help.
			if !ctx.Command.IsRoot && ctx.Command.Name != terraform.CommandName {
				return cli.ShowCommandHelp(ctx, ctx.Command.Name)
			}

			// if there is no args at all show the app help.
			if !ctx.Args().Present() {
				return cli.ShowAppHelp(ctx)
			}

			// in other cases show the Terraform help.
			terraformHelpCmd := append([]string{ctx.Args().First(), "-help"}, ctx.Args().Tail()...)
			return shell.RunTerraformCommand(opts, terraformHelpCmd...)
		}

		if err := initialSetup(ctx, opts); err != nil {
			return err
		}

		return nil
	}
	app.Action = terraform.CommandAction(opts) // run when no terragrunt command is specified

	return app
}

// also using in unit test
func newCommands(opts *options.TerragruntOptions) cli.Commands {
	return cli.Commands{
		runall.NewCommand(opts),            // run-all
		terragruntinfo.NewCommand(opts),    // terragrunt-info
		validateinputs.NewCommand(opts),    // validate-inputs
		graphdependencies.NewCommand(opts), // graph-dependencies
		hclfmt.NewCommand(opts),            // hclfmt
		renderjson.NewCommand(opts),        // render-json
		awsproviderpatch.NewCommand(opts),  // aws-provider-patch
		terraform.NewCommand(opts),         // * (to show in app help)
	}
}

func initialSetup(ctx *cli.Context, opts *options.TerragruntOptions) error {
	// Log the terragrunt version in debug mode. This helps with debugging issues and ensuring a specific version of  terragrunt used.
	defer opts.Logger.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	// convert the rest flags (intended for terraform) to one prefix, e.g. `--input=true` to `-input=true`
	args := ctx.Args().Normalize(cli.OnePrefixFlag)

	opts.TerraformCommand = args.First()
	opts.TerraformCliArgs = args.Slice()

	opts.LogLevel = util.ParseLogLevel(opts.LogLevelStr)
	opts.Logger = util.CreateLogEntry("", opts.LogLevel)
	opts.Logger.Logger.SetOutput(ctx.App.ErrWriter)

	opts.Writer = ctx.App.Writer
	opts.ErrWriter = ctx.App.ErrWriter
	opts.Env = env.ParseEnvs(os.Environ())

	if opts.WorkingDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return errors.WithStackTrace(err)
		}
		opts.WorkingDir = currentDir
	} else {
		path, err := filepath.Abs(opts.WorkingDir)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		opts.WorkingDir = path
	}
	opts.WorkingDir = filepath.ToSlash(opts.WorkingDir)

	if opts.DownloadDir == "" {
		opts.DownloadDir = util.JoinPath(opts.WorkingDir, options.TerragruntCacheDir)
	} else {
		path, err := filepath.Abs(opts.DownloadDir)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		opts.DownloadDir = path
	}
	opts.DownloadDir = filepath.ToSlash(opts.DownloadDir)

	if opts.TerragruntConfigPath == "" {
		opts.TerragruntConfigPath = config.GetDefaultConfigPath(opts.WorkingDir)
	}
	opts.TerraformPath = filepath.ToSlash(opts.TerraformPath)

	terragruntVersion, err := hashicorpversion.NewVersion(ctx.App.Version)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		if terragruntVersion, err = hashicorpversion.NewVersion("0.0"); err != nil {
			return errors.WithStackTrace(err)
		}
	}
	opts.TerragruntVersion = terragruntVersion

	jsonOutput := false
	for _, arg := range opts.TerraformCliArgs {
		if strings.EqualFold(arg, "-json") {
			jsonOutput = true
			break
		}
	}
	if opts.IncludeModulePrefix && !jsonOutput {
		opts.OutputPrefix = fmt.Sprintf("[%s] ", opts.WorkingDir)
	} else {
		opts.IncludeModulePrefix = false
	}

	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		opts.Parallelism = 1
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.OriginalTerraformCommand = opts.TerraformCommand
	opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

	return nil
}
