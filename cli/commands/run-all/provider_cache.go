package runall

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gruntwork-io/go-commons/errors"
	terraformcmd "github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/terraform/registry"
	"github.com/gruntwork-io/terragrunt/terraform/registry/controllers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/services"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/sync/errgroup"
)

const (
	// The default path to the automatically generated local CLI configuration, relative to the `.terragrunt-cache` folder.
	defaultLocalTerraformCLIFilename = ".terraformrc"
)

// HTTPStatusCacheProviderReg is regular expression to determine the success result of the command `terraform lock providers -platform=cache provider`.
var HTTPStatusCacheProviderReg = regexp.MustCompile(`(?mi)` + strconv.Itoa(controllers.HTTPStatusCacheProvider) + `[^a-z0-9]*` + http.StatusText(controllers.HTTPStatusCacheProvider))

func RunWithProviderCache(ctx context.Context, opts *options.TerragruntOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	util.RegisterInterruptHandler(func() {
		cancel()
	})

	if opts.RegistryToken == "" {
		opts.RegistryToken = fmt.Sprintf("%s:%s", handlers.AuthorizationApiKeyHeaderName, uuid.New().String())
	}

	if opts.RegistryPort == 0 {
		port, err := util.GetFreePort(opts.RegistryHostname)
		if err != nil {
			return err
		}
		opts.RegistryPort = port
	}

	provider := services.NewProviderService(opts.ProviderCompleteLock)
	registryServer := registry.NewServer(provider,
		registry.WithHostname(opts.RegistryHostname),
		registry.WithPort(opts.RegistryPort),
		registry.WithToken(opts.RegistryToken),
	)

	if err := PrepareProviderCacheEnvironment(opts, registryServer.ProviderURL()); err != nil {
		return err
	}

	log.Debugf("Provider cache dir %q", opts.ProviderCacheDir)
	provider.SetCacheDir(opts.ProviderCacheDir)

	ctx = terraformcmd.ContextWithRetry(ctx, &terraformcmd.Retry{
		HookFunc: func(ctx context.Context, opts *options.TerragruntOptions, callback terraformcmd.RetryCallback) (*shell.CmdOutput, error) {
			// Use Hook only for the `terraform init` command, which can be run explicitly by the user or Terragrunt's `auto-init` feature.
			if util.FirstArg(opts.TerraformCliArgs) == terraform.CommandNameInit {
				log.Debugf("Caching providers for %q", opts.WorkingDir)

				// Before each init, we warm up the global cache to ensure that all necessary providers are cached.
				// It's low cost operation, because it does not cache the same provider twice, but only new previously non-existent providers.
				if err := RunTerraformProvidersLockCommand(opts, "-platform="+controllers.PlatformNameCacheProvider); err != nil {
					return nil, err
				}
				provider.WaitForCacheReady()

				// Create complete terraform lock files. By default this feature is disabled, since it's not superfast.
				// Instead we use Terraform `TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE` feature, that creates hashes from the local cache.
				// And since the Terraform developers warn that this feature will be removed soon, it's good to have a workaround.
				if opts.ProviderCompleteLock && !util.FileExists(filepath.Join(opts.WorkingDir, terraform.TerraformLockFile)) {
					log.Debugf("Generating Terraform lock file for %q", opts.WorkingDir)

					if err := RunTerraformProvidersLockCommand(opts); err != nil {
						return nil, err
					}
				}
			}

			return callback(ctx, opts)
		},
	})

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return registryServer.Run(ctx)
	})
	errGroup.Go(func() error {
		defer cancel()
		return Run(ctx, opts)
	})

	return errGroup.Wait()
}

// RunTerraformProvidersLockCommand runs `terraform providers lock` for two purposes:
// 1. First, warm up the global cache
// 2. To create complete terraform lock files, if `opts.ProviderCompleteLock` is true
func RunTerraformProvidersLockCommand(opts *options.TerragruntOptions, flags ...string) error {
	// We use custom writter in order to trap the log from `terraform providers lock -platform=provider-cache` command, which terraform considers an error, but to us a success.
	errWritter := util.NewTrapWriter(opts.ErrWriter, HTTPStatusCacheProviderReg)

	lockOpts := opts.Clone(opts.TerragruntConfigPath)
	lockOpts.ErrWriter = errWritter
	lockOpts.WorkingDir = opts.WorkingDir
	lockOpts.TerraformCliArgs = []string{terraform.CommandNameProviders, terraform.CommandNameLock}
	lockOpts.TerraformCliArgs = append(lockOpts.TerraformCliArgs, flags...)

	// If the Terraform error matches `HTTPStatusCacheProviderReg` we ignore it and hide the log from users, otherwise we process everything as is.
	if err := shell.RunTerraformCommand(lockOpts, lockOpts.TerraformCliArgs...); err != nil && len(errWritter.Msgs()) == 0 {
		return err
	}
	return nil
}

// PrepareProviderCacheEnvironment creates a local CLI config and defines Terraform envs.
func PrepareProviderCacheEnvironment(opts *options.TerragruntOptions, registryProviderURL *url.URL) error {
	cliConfigFile := filepath.Join(opts.DownloadDir, defaultLocalTerraformCLIFilename)

	if err := CreateLocalCLIConfig(opts, cliConfigFile, registryProviderURL); err != nil {
		return err
	}

	for _, registryName := range opts.RegistryNames {
		envName := fmt.Sprintf(terraform.EnvNameTFTokenFmt, strings.ReplaceAll(registryName, ".", "_"))
		opts.Env[envName] = opts.RegistryToken
	}

	if !opts.ProviderCompleteLock {
		opts.Env[terraform.EnvNameTFPluginCacheMayBreakDependencyLockFile] = "1"
	}

	opts.Env[terraform.EnvNameTFCLIConfigFile] = cliConfigFile
	opts.Env[terraform.EnvNameTFPluginCacheDir] = ""

	return nil
}

// CreateLocalCLIConfig creates a local CLI configuration that merges the default/user configuration with our Private Registry configuration.
// If opts.ProviderCacheDir is not specified and CLI config value PluginCacheDir is defined, we assign this path to opts.ProviderCacheDir
// We also don't want to use Terraform's `plugin_cache_dir` feature because the cache is populated by our built-in private registry, and to make sure that no Terraform process ever overwrites the global cache, we clear this value.
// In order to force Terraform to queries our registry instead of the original one, we use the section:
//
//	host "registry.terraform.io" {
//		services = {
//			"providers.v1" = "http://localhost:5758/v1/providers/registry.terraform.io/",
//		}
//	}
//
// In order to force Terraform to create symlinks from the provider cache instead of downloading large binary files, we use the section:
//
//	provider_installation {
//		filesystem_mirror {
//			path    = "/path/to/the/provider/cache"
//			include = ["example.com/*/*"]
//		}
//		direct {
//			exclude = ["example.com/*/*"]
//		}
//	}
func CreateLocalCLIConfig(opts *options.TerragruntOptions, cliConfigFile string, registryProviderURL *url.URL) error {
	cfg, err := terraform.LoadConfig()
	if err != nil {
		return err
	}

	if opts.ProviderCacheDir == "" {
		if cfg.PluginCacheDir != "" {
			opts.ProviderCacheDir = cfg.PluginCacheDir
		} else {
			cacheDir, err := util.GetCacheDir()
			if err != nil {
				return err
			}
			opts.ProviderCacheDir = cacheDir
		}
	}
	cfg.PluginCacheDir = ""

	providerInstallationIncludes := make([]string, len(opts.RegistryNames))

	for i, registryName := range opts.RegistryNames {
		cfg.AddHost(registryName, map[string]any{
			"providers.v1": fmt.Sprintf("%s/%s/", registryProviderURL, registryName),
		})

		providerInstallationIncludes[i] = fmt.Sprintf("%s/*/*", registryName)
	}

	cfg.AddProviderInstallation(
		terraform.NewProviderInstallationFilesystemMirror(opts.ProviderCacheDir, providerInstallationIncludes, nil),
		terraform.NewProviderInstallationDirect(providerInstallationIncludes, nil),
	)

	if cfgDir := filepath.Dir(cliConfigFile); !util.FileExists(cfgDir) {
		if err := os.MkdirAll(cfgDir, os.ModePerm); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	if err := cfg.Save(cliConfigFile); err != nil {
		return err
	}
	return nil
}
