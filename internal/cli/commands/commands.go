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
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/internal/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/dag"
	execcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/exec"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl"
	helpcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/help"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/render"
	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/stack"
	versioncmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/version"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
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
func New(l log.Logger, opts *options.TerragruntOptions) clihelper.Commands {
	mainCommands := clihelper.Commands{
		runcmd.NewCommand(l, opts),  // run
		stack.NewCommand(l, opts),   // stack
		execcmd.NewCommand(l, opts), // exec
		backend.NewCommand(l, opts), // backend
	}.SetCategory(
		&clihelper.Category{
			Name:  MainCommandsCategoryName,
			Order: 10, //nolint: mnd
		},
	)

	catalogCommands := clihelper.Commands{
		catalog.NewCommand(l, opts),  // catalog
		scaffold.NewCommand(l, opts), // scaffold
	}.SetCategory(
		&clihelper.Category{
			Name:  CatalogCommandsCategoryName,
			Order: 20, //nolint: mnd
		},
	)

	discoveryCommands := clihelper.Commands{
		find.NewCommand(l, opts), // find
		list.NewCommand(l, opts), // list
	}.SetCategory(
		&clihelper.Category{
			Name:  DiscoveryCommandsCategoryName,
			Order: 30, //nolint: mnd
		},
	)

	configurationCommands := clihelper.Commands{
		hcl.NewCommand(l, opts),              // hcl
		info.NewCommand(l, opts),             // info
		dag.NewCommand(l, opts),              // dag
		render.NewCommand(l, opts),           // render
		helpcmd.NewCommand(l, opts),          // help (hidden)
		versioncmd.NewCommand(),              // version (hidden)
		awsproviderpatch.NewCommand(l, opts), // aws-provider-patch (hidden)
	}.SetCategory(
		&clihelper.Category{
			Name:  ConfigurationCommandsCategoryName,
			Order: 40, //nolint: mnd
		},
	)

	shortcutsCommands := NewShortcutsCommands(l, opts).SetCategory(
		&clihelper.Category{
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
func WrapWithTelemetry(l log.Logger, opts *options.TerragruntOptions) func(ctx context.Context, cliCtx *clihelper.Context, action clihelper.ActionFunc) error {
	return func(ctx context.Context, cliCtx *clihelper.Context, action clihelper.ActionFunc) error {
		return telemetry.TelemeterFromContext(ctx).Collect(ctx, fmt.Sprintf("%s %s", cliCtx.Command.Name, opts.TerraformCommand), map[string]any{
			"terraformCommand": opts.TerraformCommand,
			"args":             opts.TerraformCliArgs,
			"dir":              opts.WorkingDir,
		}, func(childCtx context.Context) error {
			if err := initialSetup(cliCtx, l, opts); err != nil {
				return err
			}

			if err := runAction(childCtx, cliCtx, l, opts, action); err != nil {
				opts.Tips.Find(tips.DebuggingDocs).Evaluate(l)
				return err
			}

			return nil
		})
	}
}

func runAction(ctx context.Context, cliCtx *clihelper.Context, l log.Logger, opts *options.TerragruntOptions, action clihelper.ActionFunc) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errGroup, ctx := errgroup.WithContext(ctx)

	// Set up automatic provider caching if enabled
	if !opts.NoAutoProviderCacheDir {
		if err := setupAutoProviderCacheDir(ctx, l, opts); err != nil {
			l.Debugf("Auto provider cache dir setup failed: %v", err)
		}
	}

	// actionCtx is the context passed to the action, which may be wrapped with hooks
	actionCtx := ctx

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

		actionCtx = tf.ContextWithTerraformCommandHook(ctx, server.TerraformCommandHook)

		errGroup.Go(func() error {
			return server.Run(ctx, ln)
		})
	}

	// Run command action
	errGroup.Go(func() error {
		defer cancel()

		if action != nil {
			return action(actionCtx, cliCtx)
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

	if opts.TerraformVersion == nil {
		_, err := run.PopulateTFVersion(ctx, l, opts)
		if err != nil {
			return err
		}
	}

	terraformVersion := opts.TerraformVersion
	tfImplementation := opts.TofuImplementation

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
func initialSetup(cliCtx *clihelper.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// convert the rest flags (intended for terraform) to one dash, e.g. `--input=true` to `-input=true`
	args := cliCtx.Args().WithoutBuiltinCmdSep().Normalize(clihelper.SingleDashFlag)
	cmdName := cliCtx.Command.Name

	if cmdName == runcmd.CommandName {
		cmdName = args.CommandName()
	} else {
		args = append([]string{cmdName}, args...)
	}

	// `terraform apply -destroy` is an alias for `terraform destroy`.
	// It is important to resolve the alias because the `run --all` relies on terraform command to determine the order, for `destroy` command is used the reverse order.
	if cmdName == tf.CommandNameApply && slices.Contains(args, tf.FlagNameDestroy) {
		cmdName = tf.CommandNameDestroy
		args = append([]string{tf.CommandNameDestroy}, args.Tail()...)
		args = slices.DeleteFunc(args, func(arg string) bool { return arg == tf.FlagNameDestroy })
	}

	// Since Terragrunt and Terraform have the same `-no-color` flag,
	// if a user specifies `-no-color` for Terragrunt, we should propagate it to Terraform as well.
	if l.Formatter().DisabledColors() {
		args = append(args, tf.FlagNameNoColor)
	}

	opts.TerraformCommand = cmdName
	opts.TerraformCliArgs = iacargs.New(args...)

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
		opts.DownloadDir = filepath.Join(opts.WorkingDir, util.TerragruntCacheDir)
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
		opts.TerragruntConfigPath = filepath.Join(opts.WorkingDir, opts.TerragruntConfigPath)
	}

	opts.TerragruntConfigPath, err = filepath.Abs(opts.TerragruntConfigPath)
	if err != nil {
		return errors.New(err)
	}

	opts.TFPath = filepath.ToSlash(opts.TFPath)

	excludeFiltersFromFile, err := util.ExcludeFiltersFromFile(opts.WorkingDir, opts.ExcludesFile)
	if err != nil {
		return err
	}

	opts.FilterQueries = append(opts.FilterQueries, excludeFiltersFromFile...)

	// Process filters file if the filter-flag experiment is enabled and the filters file is not disabled
	if !opts.NoFiltersFile {
		filtersFromFile, filtersFromFileErr := util.GetFiltersFromFile(opts.WorkingDir, opts.FiltersFile)
		if filtersFromFileErr != nil {
			return filtersFromFileErr
		}

		opts.FilterQueries = append(opts.FilterQueries, filtersFromFile...)
	}

	// Sort and compact opts.FilterQueries to make them unique
	slices.Sort(opts.FilterQueries)
	opts.FilterQueries = slices.Compact(opts.FilterQueries)

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

	exec.PrepareConsole(l)

	return nil
}
