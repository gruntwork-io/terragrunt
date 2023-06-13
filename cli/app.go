package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
	hashicorpversion "github.com/hashicorp/go-version"

	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/cli/flags"
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
func NewApp(writer io.Writer, errWriter io.Writer) *cli.App {
	opts := options.NewTerragruntOptions()
	opts.Writer = writer
	opts.ErrWriter = errWriter

	app := cli.NewApp()
	app.Name = "terragrunt"
	app.Usage = "Terragrunt is a thin wrapper for Terraform that provides extra tools for working with multiple\nTerraform modules, remote state, and locking. For documentation, see https://github.com/gruntwork-io/terragrunt/."
	app.UsageText = "terragrunt <command> [global options]"
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version.GetVersion()
	app.Writer = writer
	app.ErrWriter = errWriter
	app.Flags = cli.Flags{flags.NewHelpFlag()}
	app.Commands = append(
		commands.NewDeprecatedCommands(opts),
		commands.NewCommands(opts)...)
	app.Before = beforeRunningCommand(opts)
	app.DefaultCommand = terraform.CommandName
	app.OsExiter = osExiter

	return app
}

func beforeRunningCommand(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		if flagHelp := ctx.Flags.Get(flags.FlagNameHelp); flagHelp.Value().IsSet() {
			ctx.Command.Action = nil

			if err := showHelp(ctx, opts); err != nil {
				return err
			}
		}

		if err := initialSetup(ctx, opts); err != nil {
			return err
		}

		return nil
	}
}

func showHelp(ctx *cli.Context, opts *options.TerragruntOptions) error {
	// if app command is specified show the command help.
	if !ctx.Command.IsRoot && ctx.Command.Name != terraform.CommandName {
		return cli.ShowCommandHelp(ctx, ctx.Command.Name)
	}

	// if there is no args at all show the app help.
	if !ctx.Args().Present() {
		ctx.App.Flags = ctx.App.Commands.Get(terraform.CommandName).Flags
		return cli.ShowAppHelp(ctx)
	}

	// in other cases show the Terraform help.
	terraformHelpCmd := append([]string{ctx.Args().First(), "-help"}, ctx.Args().Tail()...)
	return shell.RunTerraformCommand(opts, terraformHelpCmd...)
}

func initialSetup(ctx *cli.Context, opts *options.TerragruntOptions) error {
	// The env vars are renamed to "..._NO_AUTO_..." in the gobal flags`. These ones are left for backwards compatibility.
	opts.AutoInit = env.GetBoolEnv("TERRAGRUNT_AUTO_INIT", opts.AutoInit)
	opts.AutoRetry = env.GetBoolEnv("TERRAGRUNT_AUTO_RETRY", opts.AutoRetry)
	opts.RunAllAutoApprove = env.GetBoolEnv("TERRAGRUNT_AUTO_APPROVE", opts.RunAllAutoApprove)

	// convert the rest flags (intended for terraform) to one dash, e.g. `--input=true` to `-input=true`
	args := ctx.Args().Normalize(cli.OneDashFlag)

	opts.TerraformCommand = args.First()
	opts.TerraformCliArgs = args.Slice()

	opts.LogLevel = util.ParseLogLevel(opts.LogLevelStr)
	opts.Logger = util.CreateLogEntry("", opts.LogLevel)
	opts.Logger.Logger.SetOutput(ctx.App.ErrWriter)

	opts.Env = env.ParseEnvs(os.Environ())

	// --- WorkingDir
	if opts.WorkingDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return errors.WithStackTrace(err)
		}
		opts.WorkingDir = currentDir
	}
	opts.WorkingDir = filepath.ToSlash(opts.WorkingDir)

	// --- DownloadDir
	if opts.DownloadDir == "" {
		opts.DownloadDir = util.JoinPath(opts.WorkingDir, options.TerragruntCacheDir)
	}

	downloadDir, err := filepath.Abs(opts.DownloadDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	opts.DownloadDir = filepath.ToSlash(downloadDir)

	// --- TerragruntConfigPath
	if opts.TerragruntConfigPath == "" {
		opts.TerragruntConfigPath = config.GetDefaultConfigPath(opts.WorkingDir)
	}
	opts.TerraformPath = filepath.ToSlash(opts.TerraformPath)

	// --- terragruntVersion
	terragruntVersion, err := hashicorpversion.NewVersion(ctx.App.Version)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		if terragruntVersion, err = hashicorpversion.NewVersion("0.0"); err != nil {
			return errors.WithStackTrace(err)
		}
	}
	opts.TerragruntVersion = terragruntVersion
	// Log the terragrunt version in debug mode. This helps with debugging issues and ensuring a specific version of terragrunt used.
	opts.Logger.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	// --- IncludeModulePrefix
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

	// --- others
	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		opts.Parallelism = 1
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.OriginalTerraformCommand = opts.TerraformCommand
	opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

	return nil
}

func osExiter(exitCode int) {
	// Do nothing. We just need to override this function, as the default value calls os.Exit, which
	// kills the app (or any automated test) dead in its tracks.
}
