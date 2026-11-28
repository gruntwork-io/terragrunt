// Package providercaching sets up provider caching around the action of a command that can run
// `tofu init`: directly, through a hook or a command the user names, or by reading the outputs of
// a dependency.
//
// The setup probes the version of the OpenTofu/Terraform binary, so a command that never runs it
// leaves its action unwrapped.
package providercaching

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	semver "github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Wrap returns action with [Run] around it.
func Wrap(
	l log.Logger,
	opts *options.TerragruntOptions,
	v *venv.Venv,
	action clihelper.ActionFunc,
) clihelper.ActionFunc {
	return func(ctx context.Context, cliCtx *clihelper.Context) error {
		return Run(ctx, cliCtx, l, opts, v, action)
	}
}

// Run invokes action with provider caching set up: TF_PLUGIN_CACHE_DIR pointed at a shared
// provider cache dir when the OpenTofu version supports one, and the provider cache server
// running when it is enabled.
//
// It requires the run-scoped caches of [github.com/gruntwork-io/terragrunt/internal/cache.ContextWithCache] on ctx.
func Run(
	ctx context.Context,
	cliCtx *clihelper.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	v *venv.Venv,
	action clihelper.ActionFunc,
) error {
	probe := tfimpl.Unprobed()

	if !opts.NoAutoProviderCacheDir {
		var err error

		if probe, err = setupAutoProviderCacheDir(ctx, l, opts, v); err != nil {
			l.Debugf("Auto provider cache dir setup failed: %v", err)
		}
	}

	// The implementation decides which CLI config files the cache server reads, so resolve it before the tips and the server start; a run the probe leaves mismatched is warned about, and bypasses the cache, in the command hook.
	if opts.ProviderCacheOptions.Enabled {
		var err error

		if _, probe, err = configbridge.ProbeTF(ctx, l, v, opts); err != nil {
			l.Debugf(
				"Failed to detect the OpenTofu/Terraform implementation; the provider cache server falls back to OpenTofu's CLI config file locations: %v",
				err,
			)
		}
	}

	v.RequireGOOS()

	GiveWindowsSymlinksTip(
		l,
		v.FS,
		v.Platform.GOOS,
		opts.Tips,
		v.Env,
		opts.ProviderCacheOptions.Enabled,
		probe.Type,
		probe.Version,
	)

	// Re-enable VT processing after subprocess execution may have reset console mode.
	// Defense-in-depth on top of RunCommandWithOutput's own save/restore cycle.
	if !exec.PrepareConsole(l) {
		l.Formatter().SetDisabledColors(true)
	}

	if !opts.ProviderCacheOptions.Enabled {
		return action(ctx, cliCtx)
	}

	server, err := providercache.InitServer(
		l,
		v,
		probe.Type,
		&opts.ProviderCacheOptions,
		opts.RootWorkingDir,
	)
	if err != nil {
		return err
	}

	ln, err := server.Listen(ctx, v)
	if err != nil {
		return err
	}

	defer func() {
		if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			l.Debugf("Failed to close the provider cache listener: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errGroup, ctx := errgroup.WithContext(ctx)
	ctx = tf.ContextWithTerraformCommandHook(ctx, server.TerraformCommandHook)

	errGroup.Go(func() error {
		return server.Run(ctx, ln)
	})

	errGroup.Go(func() error {
		defer cancel()

		return action(ctx, cliCtx)
	})

	return errGroup.Wait()
}

const minTofuVersionForAutoProviderCacheDir = "1.10.0"

// setupAutoProviderCacheDir configures native provider caching by setting TF_PLUGIN_CACHE_DIR.
//
// Only works with OpenTofu version >= 1.10. Returns error if conditions aren't met.
//
// The returned probe is the one it ran on the binary opts selects, and
// [tfimpl.Unprobed] when TF_PLUGIN_CACHE_DIR was already set or the probe failed.
//
// Panics if v.Env is nil, as it mutates it.
func setupAutoProviderCacheDir(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	v *venv.Venv,
) (tfimpl.Probe, error) {
	if v.Env == nil {
		panic("setupAutoProviderCacheDir: venv environment map is nil")
	}

	// Set TF_PLUGIN_CACHE_DIR environment variable
	if v.Env[tf.EnvNameTFPluginCacheDir] != "" {
		l.Debugf(
			"TF_PLUGIN_CACHE_DIR already set to %s, skipping auto provider cache dir",
			v.Env[tf.EnvNameTFPluginCacheDir],
		)

		return tfimpl.Unprobed(), nil
	}

	_, probe, err := configbridge.ProbeTF(ctx, l, v, opts)
	if err != nil {
		return probe, err
	}

	terraformVersion := probe.Version
	tfImplementation := probe.Type

	// Check if OpenTofu is being used
	if tfImplementation != tfimpl.OpenTofu {
		return probe, fmt.Errorf(
			"auto provider cache dir requires OpenTofu, but detected %s",
			tfImplementation,
		)
	}

	// Check OpenTofu version > 1.10
	if terraformVersion == nil {
		return probe, errors.New("cannot determine OpenTofu version")
	}

	requiredVersion, err := semver.Parse(minTofuVersionForAutoProviderCacheDir)
	if err != nil {
		return probe, fmt.Errorf("failed to parse required version: %w", err)
	}

	if terraformVersion.LessThan(requiredVersion) {
		return probe, fmt.Errorf(
			"auto provider cache dir requires OpenTofu version >= 1.10, but found %s",
			terraformVersion,
		)
	}

	// Set up the provider cache directory
	providerCacheDir := opts.ProviderCacheOptions.Dir
	if providerCacheDir == "" {
		cacheDir, err := util.EnsureCacheDir(v)
		if err != nil {
			return probe, fmt.Errorf("failed to get cache directory: %w", err)
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
	if err := v.FS.MkdirAll(providerCacheDir, cacheDirMode); err != nil {
		return probe, fmt.Errorf("failed to create provider cache directory: %w", err)
	}

	v.Env[tf.EnvNameTFPluginCacheDir] = providerCacheDir

	l.Debugf("Auto provider cache dir enabled: TF_PLUGIN_CACHE_DIR=%s", providerCacheDir)

	return probe, nil
}

// probeDirPerms is the mode of the throwaway directory the symlink probe creates.
const probeDirPerms = 0o755

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
	fsys vfs.FS,
	goos string,
	allTips tips.Tips,
	envs map[string]string,
	providerCacheEnabled bool,
	tfImpl tfimpl.Type,
	tfVersion *semver.Version,
) {
	if goos != "windows" {
		return
	}

	if envs[tf.EnvNameTFPluginCacheDir] == "" && !providerCacheEnabled {
		return
	}

	tmp, err := vfs.MkdirTemp(fsys, "", "terragrunt-test-symlink")
	if err != nil {
		l.Debugf("Failed to create temporary directory for testing symlink: %v", err)
		return
	}

	defer func() {
		if err := fsys.RemoveAll(tmp); err != nil {
			l.Debugf("Failed to remove temporary directory for testing symlink: %v", err)
		}
	}()

	source := filepath.Join(tmp, "source")
	target := filepath.Join(tmp, "target")

	if err := fsys.Mkdir(source, probeDirPerms); err != nil {
		l.Debugf("Failed to create source directory for testing symlink: %v", err)
		return
	}

	err = vfs.Symlink(fsys, source, target)
	if err == nil {
		return
	}

	tip := allTips.Find(tips.WindowsSymlinkWarning)
	if tip == nil {
		return
	}

	if tfImpl == tfimpl.OpenTofu && tfVersion != nil {
		minVersion, verErr := semver.Parse("1.12.0")
		if verErr == nil && !tfVersion.LessThan(minVersion) {
			tip.Message = tips.WindowsSymlinkWarningOpenTofuMessage
		}
	}

	tip.Evaluate(l)
}
