// Package commands represents CLI commands.
package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend"
	"github.com/gruntwork-io/terragrunt/cli/commands/dag"
	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/cli/commands/graph"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl"
	"github.com/gruntwork-io/terragrunt/cli/commands/info"
	"github.com/gruntwork-io/terragrunt/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/cli/commands/render"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/cli/commands/stack"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/sync/errgroup"

	helpCmd "github.com/gruntwork-io/terragrunt/cli/commands/help"
	versionCmd "github.com/gruntwork-io/terragrunt/cli/commands/version"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog"
	execCmd "github.com/gruntwork-io/terragrunt/cli/commands/exec"
	outputmodulegroups "github.com/gruntwork-io/terragrunt/cli/commands/output-module-groups"
	runCmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	hashicorpversion "github.com/hashicorp/go-version"
)

// Command category names.
const (
	// MainCommandsCategoryName represents primary Terragrunt operations like run, exec.
	MainCommandsCategoryName = "Main commands"
	// CatalogCommandsCategoryName represents commands for managing Terragrunt catalogs.
	CatalogCommandsCategoryName = "Catalog commands"
	// DiscoveryCommandsCategoryName represents commands for discovering Terragrunt configurations.
	DiscoveryCommandsCategoryName = "Discovery commands"
	// ConfigurationCommandsCategoryName represents commands for managing Terragrunt configurations.
	ConfigurationCommandsCategoryName = "Configuration commands"
	// ShortcutsCommandsCategoryName represents OpenTofu-specific shortcut commands.
	ShortcutsCommandsCategoryName = "OpenTofu shortcuts"
)

// New returns the set of Terragrunt commands, grouped into categories.
// Categories are ordered in increments of 10 for easy insertion of new categories.
func New(opts *options.TerragruntOptions) cli.Commands {
	mainCommands := cli.Commands{
		runCmd.NewCommand(opts),  // run
		runall.NewCommand(opts),  // run-all
		stack.NewCommand(opts),   // stack
		graph.NewCommand(opts),   // graph
		execCmd.NewCommand(opts), // exec
		backend.NewCommand(opts), // backend
	}.SetCategory(
		&cli.Category{
			Name:  MainCommandsCategoryName,
			Order: 10, //nolint: mnd
		},
	)

	catalogCommands := cli.Commands{
		catalog.NewCommand(opts),  // catalog
		scaffold.NewCommand(opts), // scaffold
	}.SetCategory(
		&cli.Category{
			Name:  CatalogCommandsCategoryName,
			Order: 20, //nolint: mnd
		},
	)

	discoveryCommands := cli.Commands{
		find.NewCommand(opts), // find
		list.NewCommand(opts), // list
	}.SetCategory(
		&cli.Category{
			Name:  DiscoveryCommandsCategoryName,
			Order: 30, //nolint: mnd
		},
	)

	configurationCommands := cli.Commands{
		graphdependencies.NewCommand(opts),  // graph-dependencies
		outputmodulegroups.NewCommand(opts), // output-module-groups
		hcl.NewCommand(opts),                // hcl
		info.NewCommand(opts),               // info
		dag.NewCommand(opts),                // dag
		terragruntinfo.NewCommand(opts),     // terragrunt-info
		render.NewCommand(opts),             // render
		helpCmd.NewCommand(opts),            // help (hidden)
		versionCmd.NewCommand(opts),         // version (hidden)
		awsproviderpatch.NewCommand(opts),   // aws-provider-patch (hidden)
	}.SetCategory(
		&cli.Category{
			Name:  ConfigurationCommandsCategoryName,
			Order: 40, //nolint: mnd
		},
	)

	shortcutsCommands := NewShortcutsCommands(opts).SetCategory(
		&cli.Category{
			Name:  ShortcutsCommandsCategoryName,
			Order: 50, //nolint: mnd
		},
	)

	allCommands := NewDeprecatedCommands(opts).
		Merge(mainCommands...).
		Merge(catalogCommands...).
		Merge(discoveryCommands...).
		Merge(configurationCommands...).
		Merge(shortcutsCommands...)

	return allCommands
}

// WrapWithTelemetry wraps CLI command execution with setting of telemetry context and labels, if telemetry is disabled, just runAction the command.
func WrapWithTelemetry(opts *options.TerragruntOptions) func(ctx *cli.Context, action cli.ActionFunc) error {
	return func(ctx *cli.Context, action cli.ActionFunc) error {
		return telemetry.TelemeterFromContext(ctx).Collect(ctx.Context, fmt.Sprintf("%s %s", ctx.Command.Name, opts.TerraformCommand), map[string]any{
			"terraformCommand": opts.TerraformCommand,
			"args":             opts.TerraformCliArgs,
			"dir":              opts.WorkingDir,
		}, func(childCtx context.Context) error {
			ctx.Context = childCtx //nolint:fatcontext
			if err := initialSetup(ctx, opts); err != nil {
				return err
			}

			// TODO: See if this lint should be ignored
			return runAction(ctx, opts, action) //nolint:contextcheck
		})
	}
}

func runAction(cliCtx *cli.Context, opts *options.TerragruntOptions, action cli.ActionFunc) error {
	ctx, cancel := context.WithCancel(cliCtx.Context)
	defer cancel()

	errGroup, ctx := errgroup.WithContext(ctx)

	// Run provider cache server
	if opts.ProviderCache {
		server, err := providercache.InitServer(opts)
		if err != nil {
			return err
		}

		ln, err := server.Listen()
		if err != nil {
			return err
		}
		defer ln.Close() //nolint:errcheck

		cliCtx.Context = tf.ContextWithTerraformCommandHook(ctx, server.TerraformCommandHook)

		errGroup.Go(func() error {
			return server.Run(ctx, ln)
		})
	}

	// Run command action
	errGroup.Go(func() error {
		defer cancel()

		if action != nil {
			return action(cliCtx)
		}

		return nil
	})

	return errGroup.Wait()
}

// mostly preparing terragrunt options
func initialSetup(cliCtx *cli.Context, opts *options.TerragruntOptions) error {
	// convert the rest flags (intended for terraform) to one dash, e.g. `--input=true` to `-input=true`
	args := cliCtx.Args().WithoutBuiltinCmdSep().Normalize(cli.SingleDashFlag)
	cmdName := cliCtx.Command.Name

	switch {
	case cmdName == runCmd.CommandName:
		fallthrough
	case cmdName == runall.CommandName:
		fallthrough
	case cmdName == graph.CommandName && cliCtx.Parent().Command.IsRoot:
		cmdName = args.CommandName()
	default:
		args = append([]string{cmdName}, args...)
	}

	// `terraform apply -destroy` is an alias for `terraform destroy`.
	// It is important to resolve the alias because the `run-all` relies on terraform command to determine the order, for `destroy` command is used the reverse order.
	if cmdName == tf.CommandNameApply && util.ListContainsElement(args, tf.FlagNameDestroy) {
		cmdName = tf.CommandNameDestroy
		args = append([]string{tf.CommandNameDestroy}, args.Tail()...)
		args = util.RemoveElementFromList(args, tf.FlagNameDestroy)
	}

	// Since Terragrunt and Terraform have the same `-no-color` flag,
	// if a user specifies `-no-color` for Terragrunt, we should propagate it to Terraform as well.
	if opts.Logger.Formatter().DisabledColors() {
		args = append(args, tf.FlagNameNoColor)
	}

	opts.TerraformCommand = cmdName
	opts.TerraformCliArgs = args

	opts.Env = env.Parse(os.Environ())

	// --- Working Dir
	if opts.WorkingDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return errors.New(err)
		}

		opts.WorkingDir = currentDir
	}

	opts.WorkingDir = filepath.ToSlash(opts.WorkingDir)

	workingDir, err := filepath.Abs(opts.WorkingDir)
	if err != nil {
		return errors.New(err)
	}

	opts.Logger = opts.Logger.WithField(placeholders.WorkDirKeyName, workingDir)

	opts.RootWorkingDir = filepath.ToSlash(workingDir)

	if err := opts.Logger.Formatter().SetBaseDir(opts.RootWorkingDir); err != nil {
		return err
	}

	if opts.LogShowAbsPaths {
		opts.Logger.Formatter().DisableRelativePaths()
	}

	// --- Download Dir
	if opts.DownloadDir == "" {
		opts.DownloadDir = util.JoinPath(opts.WorkingDir, util.TerragruntCacheDir)
	}

	downloadDir, err := filepath.Abs(opts.DownloadDir)
	if err != nil {
		return errors.New(err)
	}

	opts.DownloadDir = filepath.ToSlash(downloadDir)

	// --- Terragrunt ConfigPath
	if opts.TerragruntConfigPath == "" {
		opts.TerragruntConfigPath = config.GetDefaultConfigPath(opts.WorkingDir)
	} else if !filepath.IsAbs(opts.TerragruntConfigPath) &&
		(cliCtx.Command.Name == runCmd.CommandName || slices.Contains(tf.CommandNames, cliCtx.Command.Name)) {
		opts.TerragruntConfigPath = util.JoinPath(opts.WorkingDir, opts.TerragruntConfigPath)
	}

	opts.TerragruntConfigPath, err = filepath.Abs(opts.TerragruntConfigPath)
	if err != nil {
		return errors.New(err)
	}

	opts.TerraformPath = filepath.ToSlash(opts.TerraformPath)

	opts.ExcludeDirs, err = util.GlobCanonicalPath(opts.WorkingDir, opts.ExcludeDirs...)
	if err != nil {
		return err
	}

	if len(opts.IncludeDirs) > 0 {
		opts.Logger.Debugf("Included directories set. Excluding by default.")
		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && len(opts.ModulesThatInclude) > 0 {
		opts.Logger.Debugf("Modules that include set. Excluding by default.")
		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && len(opts.UnitsReading) > 0 {
		opts.Logger.Debugf("Units that read set. Excluding by default.")
		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && opts.StrictInclude {
		opts.Logger.Debugf("Strict include set. Excluding by default.")
		opts.ExcludeByDefault = true
	}

	opts.IncludeDirs, err = util.GlobCanonicalPath(opts.WorkingDir, opts.IncludeDirs...)
	if err != nil {
		return err
	}

	excludeDirs, err := util.GetExcludeDirsFromFile(opts.WorkingDir, opts.ExcludesFile)
	if err != nil {
		return err
	}

	opts.ExcludeDirs = append(opts.ExcludeDirs, excludeDirs...)

	// --- Terragrunt Version
	terragruntVersion, err := hashicorpversion.NewVersion(cliCtx.App.Version)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		if terragruntVersion, err = hashicorpversion.NewVersion("0.0"); err != nil {
			return errors.New(err)
		}
	}

	opts.TerragruntVersion = terragruntVersion
	// Log the terragrunt version in debug mode. This helps with debugging issues and ensuring a specific version of terragrunt used.
	opts.Logger.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	// --- Others
	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		opts.Parallelism = 1
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.OriginalTerraformCommand = opts.TerraformCommand
	opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

	opts.RunTerragrunt = runCmd.Run

	exec.PrepareConsole(opts.Logger)

	return nil
}
