// Package commands represents CLI commands.
package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/backend"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/cli/commands/dag"
	execcmd "github.com/gruntwork-io/terragrunt/cli/commands/exec"
	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl"
	helpcmd "github.com/gruntwork-io/terragrunt/cli/commands/help"
	"github.com/gruntwork-io/terragrunt/cli/commands/info"
	"github.com/gruntwork-io/terragrunt/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/cli/commands/render"
	runcmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/cli/commands/stack"
	versioncmd "github.com/gruntwork-io/terragrunt/cli/commands/version"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/hashicorp/go-version"
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
func New(l log.Logger, opts *options.TerragruntOptions) cli.Commands {
	mainCommands := cli.Commands{
		runcmd.NewCommand(l, opts),  // run
		stack.NewCommand(l, opts),   // stack
		execcmd.NewCommand(l, opts), // exec
		backend.NewCommand(l, opts), // backend
	}.SetCategory(
		&cli.Category{
			Name:  MainCommandsCategoryName,
			Order: 10, //nolint: mnd
		},
	)

	catalogCommands := cli.Commands{
		catalog.NewCommand(l, opts),  // catalog
		scaffold.NewCommand(l, opts), // scaffold
	}.SetCategory(
		&cli.Category{
			Name:  CatalogCommandsCategoryName,
			Order: 20, //nolint: mnd
		},
	)

	discoveryCommands := cli.Commands{
		find.NewCommand(l, opts), // find
		list.NewCommand(l, opts), // list
	}.SetCategory(
		&cli.Category{
			Name:  DiscoveryCommandsCategoryName,
			Order: 30, //nolint: mnd
		},
	)

	configurationCommands := cli.Commands{
		hcl.NewCommand(l, opts),              // hcl
		info.NewCommand(l, opts),             // info
		dag.NewCommand(l, opts),              // dag
		render.NewCommand(l, opts),           // render
		helpcmd.NewCommand(l, opts),          // help (hidden)
		versioncmd.NewCommand(opts),          // version (hidden)
		awsproviderpatch.NewCommand(l, opts), // aws-provider-patch (hidden)
	}.SetCategory(
		&cli.Category{
			Name:  ConfigurationCommandsCategoryName,
			Order: 40, //nolint: mnd
		},
	)

	shortcutsCommands := NewShortcutsCommands(l, opts).SetCategory(
		&cli.Category{
			Name:  ShortcutsCommandsCategoryName,
			Order: 50, //nolint: mnd
		},
	)

	allCommands := mainCommands.
		Merge(catalogCommands...).
		Merge(discoveryCommands...).
		Merge(configurationCommands...).
		Merge(shortcutsCommands...)

	return allCommands
}

// WrapWithTelemetry wraps CLI command execution with setting of telemetry context and labels, if telemetry is disabled, just runAction the command.
func WrapWithTelemetry(l log.Logger, opts *options.TerragruntOptions) func(ctx *cli.Context, action cli.ActionFunc) error {
	return func(ctx *cli.Context, action cli.ActionFunc) error {
		return telemetry.TelemeterFromContext(ctx).Collect(ctx.Context, fmt.Sprintf("%s %s", ctx.Command.Name, opts.TerraformCommand), map[string]any{
			"terraformCommand": opts.TerraformCommand,
			"args":             opts.TerraformCliArgs,
			"dir":              opts.WorkingDir,
		}, func(childCtx context.Context) error {
			ctx.Context = childCtx //nolint:fatcontext
			if err := initialSetup(ctx, l, opts); err != nil {
				return err
			}

			// TODO: See if this lint should be ignored
			return runAction(ctx, l, opts, action) //nolint:contextcheck
		})
	}
}

func runAction(cliCtx *cli.Context, l log.Logger, opts *options.TerragruntOptions, action cli.ActionFunc) error {
	ctx, cancel := context.WithCancel(cliCtx.Context)
	defer cancel()

	errGroup, ctx := errgroup.WithContext(ctx)

	// Set up automatic provider caching if enabled
	if !opts.NoAutoProviderCacheDir {
		if err := setupAutoProviderCacheDir(ctx, l, opts); err != nil {
			l.Debugf("Auto provider cache dir setup failed: %v", err)
		}
	}

	// Run provider cache server
	if opts.ProviderCache {
		server, err := providercache.InitServer(l, opts)
		if err != nil {
			return err
		}

		ln, err := server.Listen(ctx)
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

const minTofuVersionForAutoProviderCacheDir = "1.10.0"

// setupAutoProviderCacheDir configures native provider caching by setting TF_PLUGIN_CACHE_DIR.
//
// Only works with OpenTofu version >= 1.10. Returns error if conditions aren't met.
func setupAutoProviderCacheDir(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// Set TF_PLUGIN_CACHE_DIR environment variable
	if opts.Env[tf.EnvNameTFPluginCacheDir] != "" {
		l.Debugf(
			"TF_PLUGIN_CACHE_DIR already set to %s, skipping auto provider cache dir",
			opts.Env[tf.EnvNameTFPluginCacheDir],
		)

		return nil
	}

	var err error

	l, terraformVersion, tfImplementation, err := run.GetTFVersion(ctx, l, opts)
	if err != nil {
		return err
	}

	// Check if OpenTofu is being used
	if tfImplementation != options.OpenTofuImpl {
		return errors.Errorf("auto provider cache dir requires OpenTofu, but detected %s", tfImplementation)
	}

	// Check OpenTofu version > 1.10
	if terraformVersion == nil {
		return errors.New("cannot determine OpenTofu version")
	}

	requiredVersion, err := version.NewVersion(minTofuVersionForAutoProviderCacheDir)
	if err != nil {
		return errors.Errorf("failed to parse required version: %w", err)
	}

	if terraformVersion.LessThan(requiredVersion) {
		return errors.Errorf("auto provider cache dir requires OpenTofu version >= 1.10, but found %s", terraformVersion)
	}

	// Set up the provider cache directory
	providerCacheDir := opts.ProviderCacheDir
	if providerCacheDir == "" {
		cacheDir, err := util.GetCacheDir()
		if err != nil {
			return errors.Errorf("failed to get cache directory: %w", err)
		}

		providerCacheDir = filepath.Join(cacheDir, "providers")
	}

	// Make sure the cache directory is absolute
	if !filepath.IsAbs(providerCacheDir) {
		absPath, err := filepath.Abs(providerCacheDir)
		if err != nil {
			return errors.Errorf("failed to get absolute path for provider cache directory: %w", err)
		}

		providerCacheDir = absPath
	}

	const cacheDirMode = 0755

	// Create the cache directory if it doesn't exist
	if err := os.MkdirAll(providerCacheDir, cacheDirMode); err != nil {
		return errors.Errorf("failed to create provider cache directory: %w", err)
	}

	// Initialize environment variables map if it's nil
	if opts.Env == nil {
		opts.Env = make(map[string]string)
	}

	opts.Env[tf.EnvNameTFPluginCacheDir] = providerCacheDir

	l.Debugf("Auto provider cache dir enabled: TF_PLUGIN_CACHE_DIR=%s", providerCacheDir)

	return nil
}

// mostly preparing terragrunt options
func initialSetup(cliCtx *cli.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// convert the rest flags (intended for terraform) to one dash, e.g. `--input=true` to `-input=true`
	args := cliCtx.Args().WithoutBuiltinCmdSep().Normalize(cli.SingleDashFlag)
	cmdName := cliCtx.Command.Name

	if cmdName == runcmd.CommandName {
		cmdName = args.CommandName()
	} else {
		args = append([]string{cmdName}, args...)
	}

	// `terraform apply -destroy` is an alias for `terraform destroy`.
	// It is important to resolve the alias because the `run --all` relies on terraform command to determine the order, for `destroy` command is used the reverse order.
	if cmdName == tf.CommandNameApply && util.ListContainsElement(args, tf.FlagNameDestroy) {
		cmdName = tf.CommandNameDestroy
		args = append([]string{tf.CommandNameDestroy}, args.Tail()...)
		args = util.RemoveElementFromList(args, tf.FlagNameDestroy)
	}

	// Since Terragrunt and Terraform have the same `-no-color` flag,
	// if a user specifies `-no-color` for Terragrunt, we should propagate it to Terraform as well.
	if l.Formatter().DisabledColors() {
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

	l = l.WithField(placeholders.WorkDirKeyName, workingDir)

	opts.RootWorkingDir = filepath.ToSlash(workingDir)

	if err := l.Formatter().SetBaseDir(opts.RootWorkingDir); err != nil {
		return err
	}

	if opts.LogShowAbsPaths {
		l.Formatter().DisableRelativePaths()
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
		(cliCtx.Command.Name == runcmd.CommandName || slices.Contains(tf.CommandNames, cliCtx.Command.Name)) {
		opts.TerragruntConfigPath = util.JoinPath(opts.WorkingDir, opts.TerragruntConfigPath)
	}

	opts.TerragruntConfigPath, err = filepath.Abs(opts.TerragruntConfigPath)
	if err != nil {
		return errors.New(err)
	}

	opts.TFPath = filepath.ToSlash(opts.TFPath)

	if len(opts.IncludeDirs) > 0 {
		l.Debugf("Included directories set. Excluding by default.")

		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && len(opts.ModulesThatInclude) > 0 {
		l.Debugf("Modules that include set. Excluding by default.")

		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && len(opts.UnitsReading) > 0 {
		l.Debugf("Units that read set. Excluding by default.")

		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && opts.StrictInclude {
		l.Debugf("Strict include set. Excluding by default.")

		opts.ExcludeByDefault = true
	}

	doubleStarEnabled := opts.StrictControls.FilterByNames("double-star").SuppressWarning().Evaluate(cliCtx.Context) != nil

	// Sort and compact opts.IncludeDirs to make them unique
	slices.Sort(opts.IncludeDirs)
	opts.IncludeDirs = slices.Compact(opts.IncludeDirs)

	if !doubleStarEnabled {
		opts.IncludeDirs, err = util.GlobCanonicalPath(l, opts.WorkingDir, opts.IncludeDirs...)
		if err != nil {
			return errors.Errorf("invalid include dirs: %w", err)
		}
	}

	excludeDirsFromFile, err := util.GetExcludeDirsFromFile(opts.WorkingDir, opts.ExcludesFile)
	if err != nil {
		return err
	}

	opts.ExcludeDirs = append(opts.ExcludeDirs, excludeDirsFromFile...)
	// Sort and compact opts.ExcludeDirs to make them unique
	slices.Sort(opts.ExcludeDirs)
	opts.ExcludeDirs = slices.Compact(opts.ExcludeDirs)

	if !doubleStarEnabled {
		opts.ExcludeDirs, err = util.GlobCanonicalPath(l, opts.WorkingDir, opts.ExcludeDirs...)
		if err != nil {
			return errors.Errorf("invalid exclude dirs: %w", err)
		}
	}

	// --- Terragrunt Version
	terragruntVersion, err := version.NewVersion(cliCtx.Version)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		if terragruntVersion, err = version.NewVersion("0.0"); err != nil {
			return errors.New(err)
		}
	}

	opts.TerragruntVersion = terragruntVersion
	// Log the terragrunt version in debug mode. This helps with debugging issues and ensuring a specific version of terragrunt used.
	l.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	// --- Others
	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		opts.Parallelism = 1
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.OriginalTerraformCommand = opts.TerraformCommand
	opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

	opts.RunTerragrunt = run.Run

	exec.PrepareConsole(l)

	return nil
}
