package cli

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
	"time"

	"github.com/google/uuid"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/terraform/registry"
	"github.com/gruntwork-io/terragrunt/terraform/registry/controllers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/handlers"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	// The default path to the automatically generated local CLI configuration, relative to the `.terragrunt-cache` folder.
	defaultLocalTerraformCLIFilename = ".terraformrc"
)

// HTTPStatusCacheProviderReg is regular expression to determine the success result of the command `terraform lock providers -platform=cache provider`.
var HTTPStatusCacheProviderReg = regexp.MustCompile(`(?mi)` + strconv.Itoa(controllers.HTTPStatusCacheProvider) + `[^a-z0-9]*` + http.StatusText(controllers.HTTPStatusCacheProvider))

func initProviderCache(ctx context.Context, opts *options.TerragruntOptions) (context.Context, *registry.Server, error) {
	if opts.ProviderCacheDir == "" {
		cacheDir, err := util.GetCacheDir()
		if err != nil {
			return nil, nil, err
		}
		opts.ProviderCacheDir = filepath.Join(cacheDir, "providers")
	}

	if opts.RegistryToken == "" {
		opts.RegistryToken = fmt.Sprintf("%s:%s", handlers.AuthorizationApiKeyHeaderName, uuid.New().String())
	}

	registryServer := registry.NewServer(
		registry.WithHostname(opts.RegistryHostname),
		registry.WithPort(opts.RegistryPort),
		registry.WithToken(opts.RegistryToken),
		registry.WithProviderCacheDir(opts.ProviderCacheDir),
		registry.WithProviderCompleteLock(opts.ProviderCompleteLock),
	)

	testCtx, testCancel := context.WithCancel(ctx)
	defer testCancel()

	go func() {
		select {
		case <-testCtx.Done():
		case <-time.After(time.Second * 60):
			fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! failed to listen")
			os.Exit(1)
		}
	}()

	if err := registryServer.Listen(); err != nil {
		return nil, nil, err
	}
	testCancel()

	if err := prepareProviderCacheEnvironment(opts, registryServer.ProviderURL()); err != nil {
		return nil, nil, err
	}

	ctx = shell.ContextWithTerraformCommandHook(ctx,
		func(ctx context.Context, opts *options.TerragruntOptions, args []string) error {
			// Use Hook only for the `terraform init` command, which can be run explicitly by the user or Terragrunt's `auto-init` feature.
			if util.FirstArg(opts.TerraformCliArgs) != terraform.CommandNameInit {
				return nil
			}

			log.Debugf("Getting terraform modules for %q", opts.WorkingDir)
			if err := runTerraformCommand(ctx, opts, terraform.CommandNameGet); err != nil {
				return err
			}

			log.Debugf("Caching terraform providers for %q", opts.WorkingDir)
			// Before each init, we warm up the global cache to ensure that all necessary providers are cached.
			// It's low cost operation, because it does not cache the same provider twice, but only new previously non-existent providers.
			if err := runTerraformCommand(ctx, opts, terraform.CommandNameProviders, terraform.CommandNameLock, "-platform="+controllers.PlatformNameCacheProvider); err != nil {
				return err
			}
			registryServer.Provider.WaitForCacheReady()

			if opts.ProviderCompleteLock && !util.FileExists(filepath.Join(opts.WorkingDir, terraform.TerraformLockFile)) {
				log.Debugf("Generating Terraform lock file for %q", opts.WorkingDir)
				// Create complete terraform lock files. By default this feature is disabled, since it's not superfast.
				// Instead we use Terraform `TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE` feature, that creates hashes from the local cache.
				// And since the Terraform developers warn that this feature will be removed soon, it's good to have a workaround.
				if err := runTerraformCommand(ctx, opts, terraform.CommandNameProviders, terraform.CommandNameLock); err != nil {
					return err
				}
			}

			return nil
		},
	)

	return ctx, registryServer, nil
}

func runTerraformCommand(ctx context.Context, opts *options.TerragruntOptions, args ...string) error {
	// We use custom writter in order to trap the log from `terraform providers lock -platform=provider-cache` command, which terraform considers an error, but to us a success.
	errWritter := util.NewTrapWriter(opts.ErrWriter, HTTPStatusCacheProviderReg)

	if util.ListContainsElement(opts.TerraformCliArgs, terraform.FlagNameNoColor) {
		args = append(args, terraform.FlagNameNoColor)
	}

	cloneOpts := opts.Clone(opts.TerragruntConfigPath)
	cloneOpts.ErrWriter = errWritter
	cloneOpts.WorkingDir = opts.WorkingDir
	cloneOpts.TerraformCliArgs = args

	// If the Terraform error matches `HTTPStatusCacheProviderReg` we ignore it and hide the log from users, otherwise we process everything as is.
	if err := shell.RunTerraformCommand(ctx, cloneOpts, cloneOpts.TerraformCliArgs...); err != nil && len(errWritter.Msgs()) == 0 {
		return err
	}
	return nil
}

// prepareProviderCacheEnvironment creates a local CLI config and defines Terraform envs.
func prepareProviderCacheEnvironment(opts *options.TerragruntOptions, registryProviderURL *url.URL) error {
	cliConfigFile := filepath.Join(opts.DownloadDir, defaultLocalTerraformCLIFilename)

	if err := createLocalCLIConfig(opts, cliConfigFile, registryProviderURL); err != nil {
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

// createLocalCLIConfig creates a local CLI configuration that merges the default/user configuration with our Private Registry configuration.
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
func createLocalCLIConfig(opts *options.TerragruntOptions, cliConfigFile string, registryProviderURL *url.URL) error {
	cfg, err := terraform.LoadConfig()
	if err != nil {
		return err
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
