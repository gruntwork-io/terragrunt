package prepare

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	semver "github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const minTofuVersionForAutoProviderCacheDir = "1.10.0"

// PopulateTFImplementation resolves which OpenTofu/Terraform implementation opts.TFPath names, so
// callers like the provider cache server and the `tfr:///` registry resolver can follow that
// implementation's behavior. It reuses the run version cache and returns without probing when
// both the implementation and version are known.
func PopulateTFImplementation(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) error {
	if opts.TofuImplementation != "" && opts.TofuImplementation != tfimpl.Unknown &&
		opts.TerraformVersion != nil {
		return nil
	}

	_, ver, impl, err := run.PopulateTFVersion(ctx, l, v, run.PopulateTFVersionInput{
		TFOpts:       configbridge.TFRunOptsFromOpts(v.Env, opts),
		WorkingDir:   opts.WorkingDir,
		VersionFiles: opts.VersionManagerFileName,
	})
	if err != nil {
		return err
	}

	opts.TerraformVersion = ver
	opts.TofuImplementation = impl

	return nil
}

// SetupAutoProviderCacheDir configures native provider caching by setting TF_PLUGIN_CACHE_DIR.
//
// Only works with OpenTofu version >= 1.10. Returns error if conditions aren't met.
// Run paths call it before spawning the binary; commands that never install
// providers simply don't call it.
//
// Panics if v.Env is nil, as it mutates it.
func SetupAutoProviderCacheDir(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) error {
	if v.Env == nil {
		panic("SetupAutoProviderCacheDir: venv environment map is nil")
	}

	//nolint:staticcheck // The deprecated TerragruntOptions field keeps working for Go API callers.
	if opts.ProviderCacheOptions.NoAutoCacheDir || opts.NoAutoProviderCacheDir {
		return nil
	}

	// Set TF_PLUGIN_CACHE_DIR environment variable
	if v.Env[tf.EnvNameTFPluginCacheDir] != "" {
		l.Debugf(
			"TF_PLUGIN_CACHE_DIR already set to %s, skipping auto provider cache dir",
			v.Env[tf.EnvNameTFPluginCacheDir],
		)

		return nil
	}

	if err := PopulateTFImplementation(ctx, l, v, opts); err != nil {
		return err
	}

	terraformVersion := opts.TerraformVersion
	tfImplementation := opts.TofuImplementation

	// Check if OpenTofu is being used
	if tfImplementation != tfimpl.OpenTofu {
		return fmt.Errorf(
			"auto provider cache dir requires OpenTofu, but detected %s",
			tfImplementation,
		)
	}

	// Check OpenTofu version > 1.10
	if terraformVersion == nil {
		return errors.New("cannot determine OpenTofu version")
	}

	requiredVersion, err := semver.Parse(minTofuVersionForAutoProviderCacheDir)
	if err != nil {
		return fmt.Errorf("failed to parse required version: %w", err)
	}

	if terraformVersion.LessThan(requiredVersion) {
		return fmt.Errorf(
			"auto provider cache dir requires OpenTofu version >= 1.10, but found %s",
			terraformVersion,
		)
	}

	// Set up the provider cache directory
	providerCacheDir := opts.ProviderCacheOptions.Dir
	if providerCacheDir == "" {
		cacheDir, err := util.EnsureCacheDir(v)
		if err != nil {
			return fmt.Errorf("failed to get cache directory: %w", err)
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
		return fmt.Errorf("failed to create provider cache directory: %w", err)
	}

	v.Env[tf.EnvNameTFPluginCacheDir] = providerCacheDir

	l.Debugf("Auto provider cache dir enabled: TF_PLUGIN_CACHE_DIR=%s", providerCacheDir)

	v.RequireGOOS()

	tips.GiveWindowsSymlinksTip(
		l,
		v.FS,
		v.Platform.GOOS,
		opts.Tips,
		v.Env,
		opts.ProviderCacheOptions.Enabled,
		tfImplementation,
		terraformVersion,
	)

	return nil
}
