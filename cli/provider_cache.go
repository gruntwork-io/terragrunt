package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/terraform/cache"
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	"github.com/gruntwork-io/terragrunt/terraform/getproviders"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/exp/maps"
)

const (
	// The paths to the automatically generated local CLI configs
	localCLIFilename = ".terraformrc"

	// The status returned when making a request to the caching provider.
	// It is needed to prevent further loading of providers by terraform, and at the same time make sure that the request was processed successfully.
	cacheProviderHTTPStatusCode = http.StatusLocked

	// Authentication type on the Terragrunt Provider Cache server.
	apiKeyAuth = "x-api-key"
)

var (
	// HTTPStatusCacheProviderReg is regular expression to determine the success result of the command `terraform init`.
	// The reg matches if the text contains "423 Locked", for example:
	//
	// - registry.terraform.io/hashicorp/template: could not query provider registry for registry.terraform.io/hashicorp/template: 423 Locked.
	//
	// It also will match cases where terminal window is small enough so that terraform splits output in multiple lines, like following:
	//
	//    ╷
	//    │ Error: Failed to install provider
	//    │
	//    │ Error while installing snowflake-labs/snowflake v0.89.0: could not query
	//    │ provider registry for registry.terraform.io/snowflake-labs/snowflake: 423
	//    │ Locked
	//    ╵
	HTTPStatusCacheProviderReg = regexp.MustCompile(`(?smi)` + strconv.Itoa(cacheProviderHTTPStatusCode) + `.*` + http.StatusText(cacheProviderHTTPStatusCode))
)

type ProviderCache struct {
	*cache.Server
	cliCfg          *cliconfig.Config
	providerService *services.ProviderService
}

func InitProviderCacheServer(opts *options.TerragruntOptions) (*ProviderCache, error) {
	cacheDir, err := util.GetCacheDir()
	if err != nil {
		return nil, err
	}

	// ProviderCacheDir has the same file structure as terraform plugin_cache_dir.
	// https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache
	if opts.ProviderCacheDir == "" {
		opts.ProviderCacheDir = filepath.Join(cacheDir, "providers")
	}

	if opts.ProviderCacheDir, err = filepath.Abs(opts.ProviderCacheDir); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if opts.ProviderCacheToken == "" {
		opts.ProviderCacheToken = uuid.New().String()
	}
	// Currently, the cache server only supports the `x-api-key` token.
	if !strings.HasPrefix(strings.ToLower(opts.ProviderCacheToken), apiKeyAuth+":") {
		opts.ProviderCacheToken = fmt.Sprintf("%s:%s", apiKeyAuth, opts.ProviderCacheToken)
	}

	userProviderDir, err := cliconfig.UserProviderDir()
	if err != nil {
		return nil, err
	}
	providerService := services.NewProviderService(opts.ProviderCacheDir, userProviderDir)

	cliCfg, err := cliconfig.LoadUserConfig()
	if err != nil {
		return nil, err
	}

	var (
		providerHandlers []handlers.ProviderHandler
		filteredMethods  []cliconfig.ProviderInstallationMethod
	)

	for _, method := range cliCfg.ProviderInstallation.Methods {
		switch method := method.(type) {
		case *cliconfig.ProviderInstallationNetworkMirror:
			providerHandlers = append(providerHandlers, handlers.NewProviderNetworkMirrorHandler(providerService, cacheProviderHTTPStatusCode, method))
		case *cliconfig.ProviderInstallationDirect:
		default:
			filteredMethods = append(filteredMethods, method)
		}
	}
	providerHandlers = append(providerHandlers, handlers.NewProviderRegistryHandler(providerService, cacheProviderHTTPStatusCode))
	cliCfg.ProviderInstallation.Methods = filteredMethods

	cache := cache.NewServer(
		cache.WithHostname(opts.ProviderCacheHostname),
		cache.WithPort(opts.ProviderCachePort),
		cache.WithToken(opts.ProviderCacheToken),
		cache.WithServices(providerService),
		cache.WithProviderHandlers(providerHandlers...),
	)

	return &ProviderCache{
		Server:          cache,
		cliCfg:          cliCfg,
		providerService: providerService,
	}, nil
}

func (cache *ProviderCache) TerraformCommandHook(ctx context.Context, opts *options.TerragruntOptions, args []string) (*shell.CmdOutput, error) {
	// To prevent a loop
	ctx = shell.ContextWithTerraformCommandHook(ctx, nil)

	// Use Hook only for the `terraform init` command, which can be run explicitly by the user or Terragrunt's `auto-init` feature.
	if util.FirstArg(args) != terraform.CommandNameInit {
		return shell.RunTerraformCommandWithOutput(ctx, opts, args...)
	}

	var (
		cliConfigFilename = filepath.Join(opts.WorkingDir, localCLIFilename)
		cacheRequestID    = uuid.New().String()
		env               = providerCacheEnvironment(opts, cliConfigFilename)
	)

	// Create terraform cli config file that enables provider caching and does not use provider cache dir
	if err := cache.createLocalCLIConfig(opts, cliConfigFilename, cacheRequestID); err != nil {
		return nil, err
	}

	log.Infof("Caching terraform providers for %s", opts.WorkingDir)
	// Before each init, we warm up the global cache to ensure that all necessary providers are cached.
	// To do this we are using 'terraform providers lock' to force TF to request all the providers from our TG cache, and that's how we know what providers TF needs, and can load them into the cache.
	// It's low cost operation, because it does not cache the same provider twice, but only new previously non-existent providers.
	args = append(args, "-sdfsdfsd")
	if output, err := runTerraformCommand(ctx, opts, args, env); err != nil {
		return output, err
	}

	caches := cache.providerService.WaitForCacheReady(cacheRequestID)
	if err := getproviders.UpdateLockfile(ctx, opts.WorkingDir, caches); err != nil {
		return nil, err
	}

	// Create terraform cli config file that uses provider cache dir
	if err := cache.createLocalCLIConfig(opts, cliConfigFilename, ""); err != nil {
		return nil, err
	}

	cloneOpts := opts.Clone(opts.TerragruntConfigPath)
	cloneOpts.WorkingDir = opts.WorkingDir
	maps.Copy(cloneOpts.Env, env)

	return shell.RunTerraformCommandWithOutput(ctx, cloneOpts, args...)
}

// createLocalCLIConfig creates a local CLI config that merges the default/user configuration with our Provider Cache configuration.
// We don't want to use Terraform's `plugin_cache_dir` feature because the cache is populated by our Terragrunt Provider cache server, and to make sure that no Terraform process ever overwrites the global cache, we clear this value.
// In order to force Terraform to queries our cache server instead of the original one, we use the section below.
// https://github.com/hashicorp/terraform/issues/28309 (officially undocumented)
//
//	host "registry.terraform.io" {
//		services = {
//			"providers.v1" = "http://localhost:5758/v1/providers/registry.terraform.io/",
//		}
//	}
//
// In order to force Terraform to create symlinks from the provider cache instead of downloading large binary files, we use the section below.
// https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation
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
//
// This func doesn't change the default CLI config file, only creates a new one at the given path `filename`. Ultimately, we can assign this path to `TF_CLI_CONFIG_FILE`.
//
// It creates two types of configuration depending on the `cacheRequestID` variable set.
// 1. If `cacheRequestID` is set, `terraform init` does _not_ use the provider cache directory, the cache server creates a cache for requested providers and returns HTTP status 423. Since for each module we create the CLI config, using `cacheRequestID` we have the opportunity later retrieve from the cache server exactly those cached providers that were requested by `terraform init` using this configuration.
// 2. If `cacheRequestID` is empty, 'terraform init` uses provider cache directory, the cache server acts as a proxy.
func (cache *ProviderCache) createLocalCLIConfig(opts *options.TerragruntOptions, filename string, cacheRequestID string) error {
	cfg := cache.cliCfg.Clone()
	cfg.PluginCacheDir = ""

	var providerInstallationIncludes []string

	for _, registryName := range opts.ProviderCacheRegistryNames {
		providerInstallationIncludes = append(providerInstallationIncludes, fmt.Sprintf("%s/*/*", registryName))

		cfg.AddHost(registryName, map[string]string{
			"providers.v1": fmt.Sprintf("%s/%s/%s/", cache.ProviderController.URL(), cacheRequestID, registryName),
			// Since Terragrunt Provider Cache only caches providers, we need to route module requests to the original registry.
			"modules.v1": fmt.Sprintf("https://%s/v1/modules", registryName),
		})
	}

	if cacheRequestID == "" {
		cfg.AddProviderInstallationMethods(
			cliconfig.NewProviderInstallationFilesystemMirror(opts.ProviderCacheDir, providerInstallationIncludes, nil),
		)
	}

	cfg.AddProviderInstallationMethods(
		cliconfig.NewProviderInstallationDirect(nil, nil),
	)

	if cfgDir := filepath.Dir(filename); !util.FileExists(cfgDir) {
		if err := os.MkdirAll(cfgDir, os.ModePerm); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return cfg.Save(filename)
}

func runTerraformCommand(ctx context.Context, opts *options.TerragruntOptions, args []string, envs map[string]string) (*shell.CmdOutput, error) {
	// We use custom writer in order to trap the log from `terraform providers lock -platform=provider-cache` command, which terraform considers an error, but to us a success.
	errWriter := util.NewTrapWriter(opts.ErrWriter, HTTPStatusCacheProviderReg)

	// add -no-color flag to args if it was set in Terragrunt arguments
	if util.ListContainsElement(opts.TerraformCliArgs, terraform.FlagNameNoColor) {
		args = append(args, terraform.FlagNameNoColor)
	}

	cloneOpts := opts.Clone(opts.TerragruntConfigPath)
	cloneOpts.Writer = io.Discard
	cloneOpts.ErrWriter = errWriter
	cloneOpts.WorkingDir = opts.WorkingDir
	cloneOpts.TerraformCliArgs = args

	if envs != nil {
		maps.Copy(cloneOpts.Env, envs)
	}

	// If the Terraform error matches `HTTPStatusCacheProviderReg` we ignore it and hide the log from users, otherwise we process the error as is.
	if output, err := shell.RunTerraformCommandWithOutput(ctx, cloneOpts, cloneOpts.TerraformCliArgs...); err != nil && len(errWriter.Msgs()) == 0 {
		return output, err
	}

	return nil, nil
}

// providerCacheEnvironment returns TF_* name/value ENVs, which we use to force terraform processes to make requests through our cache server (proxy) instead of making direct requests to the origin servers.
func providerCacheEnvironment(opts *options.TerragruntOptions, cliConfigFile string) map[string]string {
	envs := make(map[string]string)

	for _, registryName := range opts.ProviderCacheRegistryNames {
		envName := fmt.Sprintf(terraform.EnvNameTFTokenFmt, strings.ReplaceAll(registryName, ".", "_"))
		// We use `TF_TOKEN_*` for authentication with our private registry (cache server).
		// https://developer.hashicorp.com/terraform/cli/config/config-file#environment-variable-credentials
		envs[envName] = opts.ProviderCacheToken
	}

	// By using `TF_CLI_CONFIG_FILE` we force terraform to use our auto-generated cli configuration file.
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_cli_config_file
	envs[terraform.EnvNameTFCLIConfigFile] = cliConfigFile
	// Clear this `TF_PLUGIN_CACHE_DIR` value since we are using our own caching mechanism.
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_plugin_cache_dir
	envs[terraform.EnvNameTFPluginCacheDir] = ""

	return envs
}
