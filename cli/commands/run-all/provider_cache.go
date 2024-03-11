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
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/sync/errgroup"
)

const (
	defaultLocalTerraformCLIFilename = ".terraformrc"
	defaultProviderCacheDir          = "provider-cache"
)

var httpStatusCacheProcessingReg = regexp.MustCompile(`(?mi)` + strconv.Itoa(controllers.HTTPStatusCacheProcessing) + `[^a-z0-9]*` + http.StatusText(controllers.HTTPStatusCacheProcessing))

func RunWithProviderCache(ctx context.Context, opts *options.TerragruntOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	util.RegisterInterruptHandler(func() {
		cancel()
	})

	registryConfig := &registry.Config{
		Hostname: opts.RegistryHostname,
		Port:     opts.RegistryPort,
		Token:    opts.RegistryToken,
	}
	registryServer := registry.NewServer(registryConfig)

	if err := prepareProviderCacheEnvironment(opts, registryServer.ProviderURLPrefix()); err != nil {
		return err
	}
	registryServer.SetProviderCacheDir(opts.ProviderCacheDir)

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return registryServer.Run(ctx)
	})

	ctx = terraformcmd.ContextWithRetry(ctx, &terraformcmd.Retry{
		HookFunc: func(ctx context.Context, opts *options.TerragruntOptions, callback terraformcmd.RetryCallback) (*shell.CmdOutput, error) {
			if util.FirstArg(opts.TerraformCliArgs) == terraform.CommandNameInit {
				log.Debugf("Caching providers for %q", opts.WorkingDir)

				platformName := controllers.PlatformNameCacheProvider
				if opts.ProviderCompleteLock {
					platformName = controllers.PlatformNameCacheProviderAndArchive
				}

				if err := runTerraformProvidersLock(ctx, opts, "-platform="+platformName); err != nil {
					return nil, err
				}
				registryServer.WaitForCacheReady(ctx)

				if opts.ProviderCompleteLock && !util.FileExists(filepath.Join(opts.WorkingDir, terraform.TerraformLockFile)) {
					log.Debugf("Generating Terraform lock file for %q", opts.WorkingDir)

					if err := runTerraformProvidersLock(ctx, opts); err != nil {
						return nil, err
					}
				}
			}

			return callback(ctx, opts)
		},
	})
	if err := Run(ctx, opts); err != nil {
		return err
	}
	cancel()

	return errGroup.Wait()
}

func runTerraformProvidersLock(ctx context.Context, opts *options.TerragruntOptions, flags ...string) error {
	errWritter := util.NewTrapWriter(opts.ErrWriter, httpStatusCacheProcessingReg)

	lockOpts := opts.Clone(opts.TerragruntConfigPath)
	lockOpts.ErrWriter = errWritter
	lockOpts.TerraformCliArgs = []string{terraform.CommandNameProviders, terraform.CommandNameLock}
	lockOpts.TerraformCliArgs = append(lockOpts.TerraformCliArgs, flags...)

	if err := shell.RunTerraformCommand(lockOpts, lockOpts.TerraformCliArgs...); err != nil && len(errWritter.Msgs()) == 0 {
		return err
	}
	return nil
}

func prepareProviderCacheEnvironment(opts *options.TerragruntOptions, registryProviderURL *url.URL) error {
	cliConfigFile := filepath.Join(opts.DownloadDir, defaultLocalTerraformCLIFilename)

	if err := createLocalCLIConfig(opts, cliConfigFile, registryProviderURL); err != nil {
		return err
	}

	if opts.RegistryToken == "" {
		opts.RegistryToken = fmt.Sprintf("x-api-key:%s", uuid.New().String())
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

func createLocalCLIConfig(opts *options.TerragruntOptions, cliConfigFile string, registryProviderURL *url.URL) error {
	cfg, err := terraform.LoadConfig()
	if err != nil {
		return err
	}

	if opts.ProviderCacheDir == "" {
		if cfg.PluginCacheDir != "" {
			opts.ProviderCacheDir = cfg.PluginCacheDir
		} else {
			opts.ProviderCacheDir = filepath.Join(opts.DownloadDir, defaultProviderCacheDir)
		}
	}
	cfg.PluginCacheDir = ""

	providerInstallationIncludes := make([]string, len(opts.RegistryNames))

	for i, registryName := range opts.RegistryNames {
		host := terraform.NewConfigHost(map[string]any{
			"providers.v1": fmt.Sprintf("%s/%s/", registryProviderURL, registryName),
		})
		cfg.AddHost(registryName, host)

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

	if err := cfg.SaveConfig(cliConfigFile); err != nil {
		return err
	}
	return nil
}
