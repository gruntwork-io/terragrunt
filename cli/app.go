package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/terragrunt/cli/commands/graph"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"

	"github.com/gruntwork-io/terragrunt/shell"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/util"
	hashicorpversion "github.com/hashicorp/go-version"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	outputmodulegroups "github.com/gruntwork-io/terragrunt/cli/commands/output-module-groups"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	validateinputs "github.com/gruntwork-io/terragrunt/cli/commands/validate-inputs"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

func init() {
	cli.AppVersionTemplate = AppVersionTemplate
	cli.AppHelpTemplate = AppHelpTemplate
	cli.CommandHelpTemplate = CommandHelpTemplate
}

// NewApp creates the Terragrunt CLI App.
func NewApp(writer io.Writer, errWriter io.Writer) *cli.App {
	opts := options.NewTerragruntOptions()
	opts.Writer = writer
	opts.ErrWriter = errWriter

	app := cli.NewApp()
	app.Name = "terragrunt"
	app.Usage = "Terragrunt is a thin wrapper for Terraform that provides extra tools for working with multiple\nTerraform modules, remote state, and locking. For documentation, see https://github.com/gruntwork-io/terragrunt/."
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version.GetVersion()
	app.Writer = writer
	app.ErrWriter = errWriter
	app.Flags = append(
		commands.NewGlobalFlags(opts),
		commands.NewHelpVersionFlags(opts)...)
	app.Commands = append(
		deprecatedCommands(opts),
		terragruntCommands(opts)...)
	app.Before = beforeAction(opts)
	app.CommonBefore = initialSetup(opts)                                   // all commands run this function before running their own `Action` function
	app.DefaultCommand = telemetryCommand(opts, terraform.NewCommand(opts)) // by default, if no terragrunt command is specified, run the Terraform command
	app.OsExiter = osExiter

	return app
}

// This set of commands is also used in unit tests
func terragruntCommands(opts *options.TerragruntOptions) cli.Commands {
	cmds := cli.Commands{
		telemetryCommand(opts, runall.NewCommand(opts)),             // run-all
		telemetryCommand(opts, terragruntinfo.NewCommand(opts)),     // terragrunt-info
		telemetryCommand(opts, validateinputs.NewCommand(opts)),     // validate-inputs
		telemetryCommand(opts, graphdependencies.NewCommand(opts)),  // graph-dependencies
		telemetryCommand(opts, hclfmt.NewCommand(opts)),             // hclfmt
		telemetryCommand(opts, renderjson.NewCommand(opts)),         // render-json
		telemetryCommand(opts, awsproviderpatch.NewCommand(opts)),   // aws-provider-patch
		telemetryCommand(opts, outputmodulegroups.NewCommand(opts)), // output-module-groups
		telemetryCommand(opts, catalog.NewCommand(opts)),            // catalog
		telemetryCommand(opts, scaffold.NewCommand(opts)),           // scaffold
		telemetryCommand(opts, graph.NewCommand(opts)),              // graph
	}

	sort.Sort(cmds)

	// add terraform command `*` after sorting to put the command at the end of the list in the help.
	cmds.Add(telemetryCommand(opts, terraform.NewCommand(opts)))

	return cmds
}

func telemetryCommand(opts *options.TerragruntOptions, cmd *cli.Command) *cli.Command {
	fn := cmd.Action
	cmd.Action = func(ctx *cli.Context) error {
		return telemetry.Trace(opts, fmt.Sprintf("%s %s", ctx.Command.Name, opts.TerraformCommand), map[string]interface{}{
			"terraformCommand": opts.TerraformCommand,
			"args":             opts.TerraformCliArgs,
			"dir":              opts.WorkingDir,
		}, func(childCtx context.Context) error {
			ctx.Context = childCtx
			opts.Ctx = childCtx
			return fn(ctx)
		})
	}

	return cmd
}

func beforeAction(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		// setting current context to the options
		opts.Ctx = ctx
		// show help if the args are not specified.
		if !ctx.Args().Present() {
			err := cli.ShowAppHelp(ctx)
			// exit the app
			return cli.NewExitError(err, 0)
		}

		return nil
	}
}

// mostly preparing terragrunt options
func initialSetup(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		// The env vars are renamed to "..._NO_AUTO_..." in the gobal flags`. These ones are left for backwards compatibility.
		opts.AutoInit = env.GetBool(os.Getenv("TERRAGRUNT_AUTO_INIT"), opts.AutoInit)
		opts.AutoRetry = env.GetBool(os.Getenv("TERRAGRUNT_AUTO_RETRY"), opts.AutoRetry)
		opts.RunAllAutoApprove = env.GetBool(os.Getenv("TERRAGRUNT_AUTO_APPROVE"), opts.RunAllAutoApprove)

		// `TF_INPUT` is the old env var for`--terragrunt-non-interactive` flag, now is replaced with `TERRAGRUNT_NON_INTERACTIVE` but kept for backwards compatibility.
		// If `TF_INPUT` is false then `opts.NonInteractive` is true.
		opts.NonInteractive = env.GetNegativeBool(os.Getenv("TF_INPUT"), opts.NonInteractive)

		// --- Args
		// convert the rest flags (intended for terraform) to one dash, e.g. `--input=true` to `-input=true`
		args := ctx.Args().Normalize(cli.SingleDashFlag).Slice()
		cmdName := ctx.Command.Name

		switch cmdName {
		case terraform.CommandName, runall.CommandName, graph.CommandName:
			cmdName = ctx.Args().CommandName()
		default:
			args = append([]string{ctx.Command.Name}, args...)
		}

		opts.TerraformCommand = cmdName
		opts.TerraformCliArgs = args

		opts.Env = env.Parse(os.Environ())

		// --- Logger
		if opts.DisableLogColors {
			util.DisableLogColors()
		}

		if opts.JsonLogFormat {
			util.JsonFormat()
		}

		opts.LogLevel = util.ParseLogLevel(opts.LogLevelStr)
		opts.Logger = util.CreateLogEntry("", opts.LogLevel)
		opts.Logger.Logger.SetOutput(ctx.App.ErrWriter)

		// --- Working Dir
		if opts.WorkingDir == "" {
			currentDir, err := os.Getwd()
			if err != nil {
				return errors.WithStackTrace(err)
			}
			opts.WorkingDir = currentDir
		}
		opts.WorkingDir = filepath.ToSlash(opts.WorkingDir)

		// --- Download Dir
		if opts.DownloadDir == "" {
			opts.DownloadDir = util.JoinPath(opts.WorkingDir, util.TerragruntCacheDir)
		}

		downloadDir, err := filepath.Abs(opts.DownloadDir)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		opts.DownloadDir = filepath.ToSlash(downloadDir)

		// --- Terragrunt ConfigPath
		if opts.TerragruntConfigPath == "" {
			opts.TerragruntConfigPath = config.GetDefaultConfigPath(opts.WorkingDir)
		} else if !filepath.IsAbs(opts.TerragruntConfigPath) && ctx.Command.Name == terraform.CommandName {
			opts.TerragruntConfigPath = util.JoinPath(opts.WorkingDir, opts.TerragruntConfigPath)
		}

		opts.TerraformPath = filepath.ToSlash(opts.TerraformPath)

		opts.ExcludeDirs, err = util.GlobCanonicalPath(opts.WorkingDir, opts.ExcludeDirs...)
		if err != nil {
			return err
		}

		opts.IncludeDirs, err = util.GlobCanonicalPath(opts.WorkingDir, opts.IncludeDirs...)
		if err != nil {
			return err
		}

		// --- Terragrunt Version
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

		// --- Others
		if !opts.RunAllAutoApprove {
			// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
			opts.Parallelism = 1
		}

		opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
		opts.OriginalTerraformCommand = opts.TerraformCommand
		opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

		opts.RunTerragrunt = terraform.Run

		shell.PrepareConsole(opts)

		return nil
	}
}

func osExiter(exitCode int) {
	// Do nothing. We just need to override this function, as the default value calls os.Exit, which
	// kills the app (or any automated test) dead in its tracks.
}
