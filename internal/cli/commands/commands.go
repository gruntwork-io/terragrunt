// Package commands represents CLI commands.
package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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
func New(l log.Logger, opts *options.TerragruntOptions, v venv.Venv) clihelper.Commands {
	mainCommands := clihelper.Commands{
		runcmd.NewCommand(l, opts, v),  // run
		stack.NewCommand(l, opts, v),   // stack
		execcmd.NewCommand(l, opts, v), // exec
		backend.NewCommand(l, opts, v), // backend
	}.SetCategory(
		&clihelper.Category{
			Name:  MainCommandsCategoryName,
			Order: 10, //nolint: mnd
		},
	)

	catalogCommands := clihelper.Commands{
		catalog.NewCommand(l, opts),     // catalog
		scaffold.NewCommand(l, opts, v), // scaffold
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
		hcl.NewCommand(l, opts, v),              // hcl
		info.NewCommand(l, opts, v),             // info
		dag.NewCommand(l, opts),                 // dag
		render.NewCommand(l, opts, v),           // render
		helpcmd.NewCommand(l, opts),             // help (hidden)
		versioncmd.NewCommand(),                 // version (hidden)
		awsproviderpatch.NewCommand(l, opts, v), // aws-provider-patch (hidden)
	}.SetCategory(
		&clihelper.Category{
			Name:  ConfigurationCommandsCategoryName,
			Order: 40, //nolint: mnd
		},
	)

	shortcutsCommands := NewShortcutsCommands(l, opts, v).SetCategory(
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

// WrapWithTelemetry wraps CLI command execution with setting of telemetry
// context and labels. If telemetry is disabled, just runs the command.
func WrapWithTelemetry(
	l log.Logger,
	opts *options.TerragruntOptions,
	v venv.Venv,
) func(ctx context.Context, cliCtx *clihelper.Context, action clihelper.ActionFunc) error {
	return func(
		ctx context.Context,
		cliCtx *clihelper.Context,
		action clihelper.ActionFunc,
	) error {
		cmdName := fmt.Sprintf(
			"%s %s", cliCtx.Command.Name, opts.TerraformCommand,
		)

		return telemetry.TelemeterFromContext(ctx).Collect(ctx, cmdName, map[string]any{
			"terraformCommand": opts.TerraformCommand,
			"args":             opts.TerraformCliArgs,
			"dir":              opts.WorkingDir,
		}, func(childCtx context.Context) error {
			if err := initialSetup(cliCtx, l, opts); err != nil {
				return err
			}

			if err := RunAction(childCtx, cliCtx, l, opts, v, action); err != nil {
				opts.Tips.Find(tips.DebuggingDocs).Evaluate(l)
				return err
			}

			return nil
		})
	}
}

// GiveWindowsSymlinksTip warns Windows users that OpenTofu/Terraform may not create symlinks
// for provider plugins installed in the local cache directory.
//
// We generally don't need to recommend this if:
//   - The user is not on Windows (this is generally a Windows-only problem)
//   - The user isn't using the provider cache directory or provider cache server
//   - We can successfully create a test symlink
//
// Note that OpenTofu doesn't want to emit a warning like this for OpenTofu users.
//
// See: https://github.com/opentofu/opentofu/issues/3972
//
// In the future, this may be less important, as the OpenTofu global provider cache
// dir might be the default, and no copying/symlinking will happen anyways. At that
// time, this check may need to have a version gate to avoid warning Windows users
// on a sufficiently new version of OpenTofu.
func GiveWindowsSymlinksTip(
	l log.Logger,
	fs vfs.FS,
	goos string,
	allTips tips.Tips,
	envs map[string]string,
	providerCacheEnabled bool,
	tfImpl tfimpl.Type,
	tfVersion *version.Version,
) {
	if goos != "windows" {
		return
	}

	if envs[tf.EnvNameTFPluginCacheDir] == "" && !providerCacheEnabled {
		return
	}

	tmp, err := vfs.MkdirTemp(fs, "", "terragrunt-test-symlink")
	if err != nil {
		l.Debugf("Failed to create temporary directory for testing symlink: %v", err)
		return
	}

	defer func() {
		if err := fs.RemoveAll(tmp); err != nil {
			l.Debugf("Failed to remove temporary directory for testing symlink: %v", err)
		}
	}()

	source := filepath.Join(tmp, "source")
	target := filepath.Join(tmp, "target")

	if err := fs.Mkdir(source, 0755); err != nil { //nolint:mnd
		l.Debugf("Failed to create source directory for testing symlink: %v", err)
		return
	}

	err = vfs.Symlink(fs, source, target)
	if err == nil {
		return
	}

	tip := allTips.Find(tips.WindowsSymlinkWarning)
	if tip == nil {
		return
	}

	if tfImpl == tfimpl.OpenTofu && tfVersion != nil {
		minVersion, verErr := version.NewVersion("1.12.0")
		if verErr == nil && !tfVersion.LessThan(minVersion) {
			tip.Message = tips.WindowsSymlinkWarningOpenTofuMessage
		}
	}

	tip.Evaluate(l)
}

// RunAction wires up cancellation, run-scoped caches, and (when enabled)
// the provider cache server, then invokes action with the resulting context.
func RunAction(
	ctx context.Context,
	cliCtx *clihelper.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	v venv.Venv,
	action clihelper.ActionFunc,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errGroup, ctx := errgroup.WithContext(ctx)

	// Set up automatic provider caching if enabled
	if !opts.NoAutoProviderCacheDir {
		if err := setupAutoProviderCacheDir(ctx, l, opts, v.Exec); err != nil {
			l.Debugf("Auto provider cache dir setup failed: %v", err)
		}
	}

	GiveWindowsSymlinksTip(
		l,
		v.FS,
		runtime.GOOS,
		opts.Tips,
		opts.Env,
		opts.ProviderCacheOptions.Enabled,
		opts.TofuImplementation,
		opts.TerraformVersion,
	)

	// Re-enable VT processing after subprocess execution may have reset console mode.
	// Defense-in-depth on top of RunCommandWithOutput's own save/restore cycle.
	if !exec.PrepareConsole(l) {
		l.Formatter().SetDisabledColors(true)
	}

	// Install run-scoped caches on actionCtx so memoized helpers like
	// [github.com/gruntwork-io/terragrunt/internal/shell.GitTopLevelDir] share
	// state across the whole action.
	actionCtx := cache.ContextWithCache(ctx)

	// Run provider cache server
	if opts.ProviderCacheOptions.Enabled {
		server, err := providercache.InitServer(l, &opts.ProviderCacheOptions, opts.RootWorkingDir)
		if err != nil {
			return err
		}

		ln, err := server.Listen(actionCtx)
		if err != nil {
			return err
		}
		defer ln.Close() //nolint:errcheck

		actionCtx = tf.ContextWithTerraformCommandHook(actionCtx, server.TerraformCommandHook)

		errGroup.Go(func() error {
			return server.Run(actionCtx, ln)
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
func setupAutoProviderCacheDir(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, exec vexec.Exec) error {
	// Set TF_PLUGIN_CACHE_DIR environment variable
	if opts.Env[tf.EnvNameTFPluginCacheDir] != "" {
		l.Debugf(
			"TF_PLUGIN_CACHE_DIR already set to %s, skipping auto provider cache dir",
			opts.Env[tf.EnvNameTFPluginCacheDir],
		)

		return nil
	}

	if opts.TerraformVersion == nil {
		_, ver, impl, err := run.PopulateTFVersion(
			ctx, l, exec, opts.WorkingDir,
			opts.VersionManagerFileName,
			configbridge.TFRunOptsFromOpts(opts),
		)
		if err != nil {
			return err
		}

		opts.TerraformVersion = ver
		opts.TofuImplementation = impl
	}

	terraformVersion := opts.TerraformVersion
	tfImplementation := opts.TofuImplementation

	// Check if OpenTofu is being used
	if tfImplementation != tfimpl.OpenTofu {
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
	providerCacheDir := opts.ProviderCacheOptions.Dir
	if providerCacheDir == "" {
		cacheDir, err := util.EnsureCacheDir()
		if err != nil {
			return errors.Errorf("failed to get cache directory: %w", err)
		}

		providerCacheDir = filepath.Join(cacheDir, "providers")
	}

	// Make sure the cache directory is absolute
	if !filepath.IsAbs(providerCacheDir) {
		providerCacheDir = filepath.Join(opts.RootWorkingDir, providerCacheDir)
	}

	providerCacheDir = filepath.Clean(providerCacheDir)

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
	// It is important to resolve the alias because `run --all` relies on
	// the OpenTofu/Terraform command to determine the order, and for
	// `destroy` the reverse order is used.
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
	} else if !filepath.IsAbs(opts.WorkingDir) {
		workingDir, err := filepath.Abs(opts.WorkingDir)
		if err != nil {
			return errors.New(err)
		}

		opts.WorkingDir = workingDir
	}

	opts.WorkingDir = filepath.Clean(opts.WorkingDir)

	l = l.WithField(placeholders.WorkDirKeyName, opts.WorkingDir)

	opts.RootWorkingDir = opts.WorkingDir

	if err := l.Formatter().SetBaseDir(opts.RootWorkingDir); err != nil {
		return err
	}

	if opts.Writers.LogShowAbsPaths {
		l.Formatter().DisableRelativePaths()
	}

	// --- Download Dir
	if opts.DownloadDir == "" {
		opts.DownloadDir = filepath.Join(opts.WorkingDir, util.TerragruntCacheDir)
	} else if !filepath.IsAbs(opts.DownloadDir) {
		opts.DownloadDir = filepath.Join(opts.RootWorkingDir, opts.DownloadDir)
	}

	opts.DownloadDir = filepath.Clean(opts.DownloadDir)

	// --- Terragrunt ConfigPath
	if opts.TerragruntConfigPath == "" {
		opts.TerragruntConfigPath = config.GetDefaultConfigPath(opts.WorkingDir)
	} else if !filepath.IsAbs(opts.TerragruntConfigPath) &&
		(cliCtx.Command.Name == runcmd.CommandName || slices.Contains(tf.CommandNames, cliCtx.Command.Name)) {
		opts.TerragruntConfigPath = filepath.Join(opts.WorkingDir, opts.TerragruntConfigPath)
	}

	opts.TerragruntConfigPath = filepath.Clean(opts.TerragruntConfigPath)

	if !filepath.IsAbs(opts.TFPath) && strings.Contains(opts.TFPath, string(filepath.Separator)) {
		opts.TFPath = filepath.Join(opts.WorkingDir, opts.TFPath)
	}

	var fileFilterStrings []string

	excludeFiltersFromFile, err := util.ExcludeFiltersFromFile(opts.WorkingDir, opts.ExcludesFile)
	if err != nil {
		return err
	}

	fileFilterStrings = append(fileFilterStrings, excludeFiltersFromFile...)

	// Process filters file if the filters file is not disabled
	if !opts.NoFiltersFile {
		filtersFromFile, filtersFromFileErr := util.GetFiltersFromFile(opts.WorkingDir, opts.FiltersFile)
		if filtersFromFileErr != nil {
			return filtersFromFileErr
		}

		fileFilterStrings = append(fileFilterStrings, filtersFromFile...)
	}

	if len(fileFilterStrings) > 0 {
		parsed, parseErr := filter.ParseFilterQueries(l, fileFilterStrings)
		if parseErr != nil {
			return parseErr
		}

		opts.Filters = append(opts.Filters, parsed...)
	}

	// Deduplicate filters by their string representation
	seen := make(map[string]struct{}, len(opts.Filters))
	deduped := make(filter.Filters, 0, len(opts.Filters))

	for _, f := range opts.Filters {
		key := f.String()
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		deduped = append(deduped, f)
	}

	opts.Filters = deduped

	// --- Terragrunt Version
	terragruntVersion, err := version.NewVersion(cliCtx.Version)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		if terragruntVersion, err = version.NewVersion("0.0"); err != nil {
			return errors.New(err)
		}
	}

	opts.TerragruntVersion = terragruntVersion
	// Log the terragrunt version in debug mode. This helps with
	// debugging issues and ensuring a specific version is used.
	l.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	// --- Others
	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		opts.Parallelism = 1
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.OriginalTerraformCommand = opts.TerraformCommand
	opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

	if !exec.PrepareConsole(l) {
		l.Debugf("Virtual terminal processing not available, disabling colors")
		l.Formatter().SetDisabledColors(true)
	}

	return nil
}
