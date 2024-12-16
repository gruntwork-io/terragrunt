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
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/terraform/cache"
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	"github.com/gruntwork-io/terragrunt/terraform/getproviders"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	// The paths to the automatically generated local CLI configs
	localCLIFilename = ".terraformrc"

	// The status returned when making a request to the caching provider.
	// It is needed to prevent further loading of providers by terraform, and at the same time make sure that the request was processed successfully.
	CacheProviderHTTPStatusCode = http.StatusLocked

	// Authentication type on the Terragrunt Provider Cache server.
	APIKeyAuth = "x-api-key"
)

var (
	// httpStatusCacheProviderReg is a regular expression to determine the success result of the command `terraform init`.
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
	httpStatusCacheProviderReg = regexp.MustCompile(`(?smi)` + strconv.Itoa(CacheProviderHTTPStatusCode) + `.*` + http.StatusText(CacheProviderHTTPStatusCode))
)

type ProviderCache struct {
	*cache.Server
	cliCfg          *cliconfig.Config
	providerService *services.ProviderService
}

func InitProviderCacheServer(opts *options.TerragruntOptions) (*ProviderCache, error) {
	// ProviderCacheDir has the same file structure as terraform plugin_cache_dir.
	// https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache
	if opts.ProviderCacheDir == "" {
		cacheDir, err := util.GetCacheDir()
		if err != nil {
			return nil, err
		}

		opts.ProviderCacheDir = filepath.Join(cacheDir, "providers")
	}

	var err error
	if opts.ProviderCacheDir, err = filepath.Abs(opts.ProviderCacheDir); err != nil {
		return nil, errors.New(err)
	}

	if opts.ProviderCacheToken == "" {
		opts.ProviderCacheToken = uuid.New().String()
	}
	// Currently, the cache server only supports the `x-api-key` token.
	if !strings.HasPrefix(strings.ToLower(opts.ProviderCacheToken), APIKeyAuth+":") {
		opts.ProviderCacheToken = fmt.Sprintf("%s:%s", APIKeyAuth, opts.ProviderCacheToken)
	}

	cliCfg, err := cliconfig.LoadUserConfig()
	if err != nil {
		return nil, err
	}

	userProviderDir, err := cliconfig.UserProviderDir()
	if err != nil {
		return nil, err
	}

	providerService := services.NewProviderService(opts.ProviderCacheDir, userProviderDir, cliCfg.CredentialsSource(), opts.Logger)

	var (
		providerHandlers = make([]handlers.ProviderHandler, 0, len(cliCfg.ProviderInstallation.Methods))
		excludeAddrs     = make([]string, 0, len(cliCfg.ProviderInstallation.Methods))
		directIsdefined  bool
	)

	for _, registryName := range opts.ProviderCacheRegistryNames {
		excludeAddrs = append(excludeAddrs, registryName+"/*/*")
	}

	for _, method := range cliCfg.ProviderInstallation.Methods {
		switch method := method.(type) {
		case *cliconfig.ProviderInstallationFilesystemMirror:
			providerHandlers = append(providerHandlers, handlers.NewProviderFilesystemMirrorHandler(providerService, CacheProviderHTTPStatusCode, method))
		case *cliconfig.ProviderInstallationNetworkMirror:
			networkMirrorHandler, err := handlers.NewProviderNetworkMirrorHandler(providerService, CacheProviderHTTPStatusCode, method, cliCfg.CredentialsSource())
			if err != nil {
				return nil, err
			}

			providerHandlers = append(providerHandlers, networkMirrorHandler)
		case *cliconfig.ProviderInstallationDirect:
			providerHandlers = append(providerHandlers, handlers.NewProviderDirectHandler(providerService, CacheProviderHTTPStatusCode, method, cliCfg.CredentialsSource()))
			directIsdefined = true
		}

		method.AppendExclude(excludeAddrs)
	}

	if !directIsdefined {
		// In a case if none of direct provider installation methods `cliCfg.ProviderInstallation.Methods` are specified.
		providerHandlers = append(providerHandlers, handlers.NewProviderDirectHandler(providerService, CacheProviderHTTPStatusCode, new(cliconfig.ProviderInstallationDirect), cliCfg.CredentialsSource()))
	}

	cache := cache.NewServer(
		cache.WithHostname(opts.ProviderCacheHostname),
		cache.WithPort(opts.ProviderCachePort),
		cache.WithToken(opts.ProviderCacheToken),
		cache.WithServices(providerService),
		cache.WithProviderHandlers(providerHandlers...),
		cache.WithLogger(opts.Logger),
	)

	return &ProviderCache{
		Server:          cache,
		cliCfg:          cliCfg,
		providerService: providerService,
	}, nil
}

// TerraformCommandHook warms up the providers cache, creates `.terraform.lock.hcl` and runs the `tofu/terraform init`
// command with using this cache. Used as a hook function that is called after running the target tofu/terraform command.
// For example, if the target command is `tofu plan`, it will be intercepted before it is run in the `/shell` package,
// then control will be passed to this function to init the working directory using cached providers.
func (cache *ProviderCache) TerraformCommandHook(
	ctx context.Context,
	opts *options.TerragruntOptions,
	args cli.Args,
) (*util.CmdOutput, error) {
	// To prevent a loop
	ctx = shell.ContextWithTerraformCommandHook(ctx, nil)

	cliConfigFilename := filepath.Join(opts.WorkingDir, localCLIFilename)

	if !filepath.IsAbs(cliConfigFilename) {
		absPath, err := filepath.Abs(cliConfigFilename)
		if err != nil {
			return nil, errors.New(err)
		}

		cliConfigFilename = absPath
	}

	var (
		env                  = providerCacheEnvironment(opts, cliConfigFilename)
		skipRunTargetCommand bool
	)

	// Use Hook only for the `terraform init` command, which can be run explicitly by the user or Terragrunt's `auto-init` feature.
	switch {
	case args.CommandName() == terraform.CommandNameInit:
		// Provider caching for `terraform init` command.
	case args.CommandName() == terraform.CommandNameProviders && args.SubCommandName() == terraform.CommandNameLock:
		// Provider caching for `terraform providers lock` command.
		// Since the Terragrunt provider cache server creates the cache and generates the lock file, we don't need to run the `terraform providers lock ...` command at all.
		skipRunTargetCommand = true
	default:
		// skip cache creation for all other commands
		return shell.RunTerraformCommandWithOutput(ctx, opts, args...)
	}

	if output, err := cache.warmUpCache(ctx, opts, cliConfigFilename, args, env); err != nil {
		return output, err
	}

	if skipRunTargetCommand {
		return &util.CmdOutput{}, nil
	}

	return cache.runTerraformWithCache(ctx, opts, cliConfigFilename, args, env)
}

func (cache *ProviderCache) warmUpCache(
	ctx context.Context,
	opts *options.TerragruntOptions,
	cliConfigFilename string,
	args cli.Args,
	env map[string]string,
) (*util.CmdOutput, error) {
	var (
		cacheRequestID = uuid.New().String()
		commandsArgs   = convertToMultipleCommandsByPlatforms(args)
	)

	// Create terraform cli config file that enables provider caching and does not use provider cache dir
	if err := cache.createLocalCLIConfig(ctx, opts, cliConfigFilename, cacheRequestID); err != nil {
		return nil, err
	}

	opts.Logger.Infof("Caching terraform providers for %s", opts.WorkingDir)
	// Before each init, we warm up the global cache to ensure that all necessary providers are cached.
	// To do this we are using 'terraform providers lock' to force TF to request all the providers from our TG cache, and that's how we know what providers TF needs, and can load them into the cache.
	// It's low cost operation, because it does not cache the same provider twice, but only new previously non-existent providers.

	for _, args := range commandsArgs {
		if output, err := runTerraformCommand(ctx, opts, args, env); err != nil {
			return output, err
		}
	}

	caches, err := cache.providerService.WaitForCacheReady(cacheRequestID)
	if err != nil {
		return nil, err
	}

	err = getproviders.UpdateLockfile(ctx, opts.WorkingDir, caches)

	return nil, err
}

func (cache *ProviderCache) runTerraformWithCache(
	ctx context.Context,
	opts *options.TerragruntOptions,
	cliConfigFilename string,
	args cli.Args,
	env map[string]string,
) (*util.CmdOutput, error) {
	// Create terraform cli config file that uses provider cache dir
	if err := cache.createLocalCLIConfig(ctx, opts, cliConfigFilename, ""); err != nil {
		return nil, err
	}

	cloneOpts, err := opts.Clone(opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	cloneOpts.WorkingDir = opts.WorkingDir
	cloneOpts.Env = env

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
func (cache *ProviderCache) createLocalCLIConfig(ctx context.Context, opts *options.TerragruntOptions, filename string, cacheRequestID string) error {
	cfg := cache.cliCfg.Clone()
	cfg.PluginCacheDir = ""

	var providerInstallationIncludes = make([]string, 0, len(opts.ProviderCacheRegistryNames))

	for _, registryName := range opts.ProviderCacheRegistryNames {
		providerInstallationIncludes = append(providerInstallationIncludes, registryName+"/*/*")

		apiURLs, err := cache.Server.DiscoveryURL(ctx, registryName)
		if err != nil {
			return err
		}

		cfg.AddHost(registryName, map[string]string{
			"providers.v1": fmt.Sprintf("%s/%s/%s/", cache.ProviderController.URL(), cacheRequestID, registryName),
			// Since Terragrunt Provider Cache only caches providers, we need to route module requests to the original registry.
			"modules.v1": fmt.Sprintf("https://%s%s", registryName, apiURLs.ModulesV1),
		})
	}

	if cacheRequestID == "" {
		cfg.AddProviderInstallationMethods(
			cliconfig.NewProviderInstallationFilesystemMirror(opts.ProviderCacheDir, providerInstallationIncludes, nil),
		)
	} else {
		cfg.ProviderInstallation = nil
	}

	cfg.AddProviderInstallationMethods(
		cliconfig.NewProviderInstallationDirect(nil, nil),
	)

	if cfgDir := filepath.Dir(filename); !util.FileExists(cfgDir) {
		if err := os.MkdirAll(cfgDir, os.ModePerm); err != nil {
			return errors.New(err)
		}
	}

	return cfg.Save(filename)
}

func runTerraformCommand(ctx context.Context, opts *options.TerragruntOptions, args []string, envs map[string]string) (*util.CmdOutput, error) {
	// We use custom writer in order to trap the log from `terraform providers lock -platform=provider-cache` command, which terraform considers an error, but to us a success.
	errWriter := util.NewTrapWriter(opts.ErrWriter)

	// add -no-color flag to args if it was set in Terragrunt arguments
	if util.ListContainsElement(opts.TerraformCliArgs, terraform.FlagNameNoColor) &&
		!util.ListContainsElement(args, terraform.FlagNameNoColor) {
		args = append(args, terraform.FlagNameNoColor)
	}

	cloneOpts, err := opts.Clone(opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	cloneOpts.Writer = io.Discard
	cloneOpts.ErrWriter = errWriter
	cloneOpts.WorkingDir = opts.WorkingDir
	cloneOpts.TerraformCliArgs = args
	cloneOpts.Env = envs

	output, err := shell.RunTerraformCommandWithOutput(ctx, cloneOpts, cloneOpts.TerraformCliArgs...)
	// If the Terraform error matches `httpStatusCacheProviderReg` we ignore it and hide the log from users, otherwise we process the error as is.
	if err != nil && httpStatusCacheProviderReg.Match(output.Stderr.Bytes()) {
		return new(util.CmdOutput), nil
	}

	if err := errWriter.Flush(); err != nil {
		return nil, err
	}

	return output, err
}

// providerCacheEnvironment returns TF_* name/value ENVs, which we use to force terraform processes to make requests through our cache server (proxy) instead of making direct requests to the origin servers.
func providerCacheEnvironment(opts *options.TerragruntOptions, cliConfigFile string) map[string]string {
	envs := opts.Env

	for _, registryName := range opts.ProviderCacheRegistryNames {
		envName := fmt.Sprintf(terraform.EnvNameTFTokenFmt, strings.ReplaceAll(registryName, ".", "_"))

		// delete existing key case insensitive
		for key := range envs {
			if strings.EqualFold(key, envName) {
				delete(envs, key)
			}
		}

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

// convertToMultipleCommandsByPlatforms converts `providers lock -platform=.. -platform=..` command into multiple commands that include only one platform.
// for example:
// `providers lock -platform=linux_amd64 -platform=darwin_arm64 -platform=freebsd_amd64`
// to
// `providers lock -platform=linux_amd64`,
// `providers lock -platform=darwin_arm64`,
// `providers lock -platform=freebsd_amd64`
func convertToMultipleCommandsByPlatforms(args []string) [][]string {
	var (
		filteredArgs = make([]string, 0, len(args))
		platformArgs = make([]string, 0, len(args))
	)

	for _, arg := range args {
		if strings.HasPrefix(arg, terraform.FlagNamePlatform) {
			platformArgs = append(platformArgs, arg)
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	if len(platformArgs) == 0 {
		return [][]string{args}
	}

	var commandsArgs = make([][]string, 0, len(platformArgs))

	for _, platformArg := range platformArgs {
		var commandArgs = make([]string, len(filteredArgs), len(filteredArgs)+1)

		copy(commandArgs, filteredArgs)
		commandsArgs = append(commandsArgs, append(commandArgs, platformArg))
	}

	return commandsArgs
}
