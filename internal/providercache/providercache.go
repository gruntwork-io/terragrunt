// Package providercache provides initialization of the Terragrunt provider caching server for caching OpenTofu providers.
package providercache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"maps"

	"github.com/google/uuid"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/tf/getproviders"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// The paths to the automatically generated local CLI configs
	localCLIFilename = ".terraformrc"

	// The status returned when making a request to the caching provider.
	// It is needed to prevent further loading of providers by terraform, and at the same time make sure that the request was processed successfully.
	CacheProviderHTTPStatusCode = http.StatusLocked

	// Authentication type on the Terragrunt Provider Cache server.
	APIKeyAuth = "x-api-key"

	// Retry configuration for registry operations during cache warm-up
	registryRetryMaxAttempts   = 3
	registryRetrySleepInterval = 5 * time.Second
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

	// registryTimeoutPatterns matches transient network errors that should trigger retries
	registryTimeoutPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?s).*Client\.Timeout exceeded while awaiting headers.*`),
		regexp.MustCompile(`(?s).*TLS handshake timeout.*`),
		regexp.MustCompile(`(?s).*context deadline exceeded.*`),
		regexp.MustCompile(`(?s).*connection reset by peer.*`),
		regexp.MustCompile(`(?s).*tcp.*timeout.*`),
	}
)

type ProviderCache struct {
	*cache.Server
	cliCfg          *cliconfig.Config
	providerService *services.ProviderService
	fs              vfs.FS
}

// NewProviderCache creates a new ProviderCache with sensible defaults.
// Use builder methods like WithFS() to customize the configuration.
func NewProviderCache() *ProviderCache {
	return &ProviderCache{
		fs: vfs.NewOSFS(),
	}
}

// WithFS sets the filesystem for file operations and returns the ProviderCache
// for method chaining. If not called, defaults to the real OS filesystem.
func (pc *ProviderCache) WithFS(fs vfs.FS) *ProviderCache {
	pc.fs = fs
	return pc
}

// FS returns the configured filesystem.
func (pc *ProviderCache) FS() vfs.FS {
	return pc.fs
}

// Init initializes the ProviderCache with the given logger and options.
// Call this after configuring the ProviderCache with builder methods.
func (pc *ProviderCache) Init(l log.Logger, opts *options.TerragruntOptions) error {
	// ProviderCacheDir has the same file structure as terraform plugin_cache_dir.
	// https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache
	if opts.ProviderCacheDir == "" {
		cacheDir, err := util.GetCacheDir()
		if err != nil {
			return fmt.Errorf("failed to get cache directory: %w", err)
		}

		opts.ProviderCacheDir = filepath.Join(cacheDir, "providers")
	}

	var err error
	if opts.ProviderCacheDir, err = filepath.Abs(opts.ProviderCacheDir); err != nil {
		return errors.New(err)
	}

	if opts.ProviderCacheToken == "" {
		opts.ProviderCacheToken = uuid.New().String()
	}
	// Currently, the cache server only supports the `x-api-key` token.
	if !strings.HasPrefix(strings.ToLower(opts.ProviderCacheToken), APIKeyAuth+":") {
		opts.ProviderCacheToken = fmt.Sprintf("%s:%s", APIKeyAuth, opts.ProviderCacheToken)
	}

	// Pass filesystem to LoadUserConfig
	cliCfg, err := cliconfig.LoadUserConfig(cliconfig.WithFS(pc.FS()))
	if err != nil {
		return err
	}

	userProviderDir, err := cliconfig.UserProviderDir()
	if err != nil {
		return err
	}

	providerService := services.NewProviderService(opts.ProviderCacheDir, userProviderDir, cliCfg.CredentialsSource(), l, services.WithFS(pc.FS()))
	proxyProviderHandler := handlers.NewProxyProviderHandler(l, cliCfg.CredentialsSource())

	providerHandlers, err := handlers.NewProviderHandlers(cliCfg, l, opts.ProviderCacheRegistryNames)
	if err != nil {
		return errors.Errorf("creating provider handlers failed: %w", err)
	}

	cacheServer := cache.NewServer(
		cache.WithHostname(opts.ProviderCacheHostname),
		cache.WithPort(opts.ProviderCachePort),
		cache.WithToken(opts.ProviderCacheToken),
		cache.WithProviderService(providerService),
		cache.WithProviderHandlers(providerHandlers...),
		cache.WithProxyProviderHandler(proxyProviderHandler),
		cache.WithCacheProviderHTTPStatusCode(CacheProviderHTTPStatusCode),
		cache.WithLogger(l),
	)

	pc.Server = cacheServer
	pc.cliCfg = cliCfg
	pc.providerService = providerService

	return nil
}

// InitServer creates and initializes a new ProviderCache with the given logger and options.
// This is a convenience function that combines NewProviderCache() and Init().
func InitServer(l log.Logger, opts *options.TerragruntOptions) (*ProviderCache, error) {
	pc := NewProviderCache()
	if err := pc.Init(l, opts); err != nil {
		return nil, err
	}

	return pc, nil
}

// TerraformCommandHook warms up the providers cache, creates `.terraform.lock.hcl` and runs the `tofu/terraform init`
// command with using this cache. Used as a hook function that is called after running the target tofu/terraform command.
// For example, if the target command is `tofu plan`, it will be intercepted before it is run in the `/shell` package,
// then control will be passed to this function to init the working directory using cached providers.
func (pc *ProviderCache) TerraformCommandHook(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	args clihelper.Args,
) (*util.CmdOutput, error) {
	// To prevent a loop
	ctx = tf.ContextWithTerraformCommandHook(ctx, nil)

	cliConfigFilename := filepath.Join(opts.WorkingDir, localCLIFilename)

	if !filepath.IsAbs(cliConfigFilename) {
		absPath, err := filepath.Abs(cliConfigFilename)
		if err != nil {
			return nil, errors.New(err)
		}

		cliConfigFilename = absPath
	}

	var skipRunTargetCommand bool

	// Use Hook only for the `terraform init` command, which can be run explicitly by the user or Terragrunt's `auto-init` feature.
	switch {
	case args.CommandName() == tf.CommandNameInit:
		// Provider caching for `terraform init` command.
	case args.CommandName() == tf.CommandNameProviders && args.SubCommandName() == tf.CommandNameLock:
		// Provider caching for `terraform providers lock` command.
		// Since the Terragrunt provider cache server creates the cache and generates the lock file, we don't need to run the `terraform providers lock ...` command at all.
		skipRunTargetCommand = true
	default:
		// skip cache creation for all other commands
		return tf.RunCommandWithOutput(ctx, l, opts, args...)
	}

	env := providerCacheEnvironment(opts, cliConfigFilename)

	if output, err := pc.warmUpCache(ctx, l, opts, cliConfigFilename, args, env); err != nil {
		return output, err
	}

	if skipRunTargetCommand {
		return &util.CmdOutput{}, nil
	}

	return pc.runTerraformWithCache(ctx, l, opts, cliConfigFilename, args, env)
}

func (pc *ProviderCache) warmUpCache(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	cliConfigFilename string,
	args clihelper.Args,
	env map[string]string,
) (*util.CmdOutput, error) {
	var (
		cacheRequestID = uuid.New().String()
		commandsArgs   = convertToMultipleCommandsByPlatforms(args)
	)

	// Create terraform cli config file that enables provider caching and does not use provider cache dir
	if err := pc.createLocalCLIConfig(ctx, opts, cliConfigFilename, cacheRequestID); err != nil {
		return nil, err
	}

	l.Infof("Caching terraform providers for %s", opts.WorkingDir)
	// Before each init, we warm up the global cache to ensure that all necessary providers are cached.
	// To do this we are using 'terraform providers lock' to force TF to request all the providers from our TG cache, and that's how we know what providers TF needs, and can load them into the cache.
	// It's low cost operation, because it does not cache the same provider twice, but only new previously non-existent providers.

	for _, args := range commandsArgs {
		if output, err := runTerraformCommand(ctx, l, opts, args, env); err != nil {
			return output, err
		}
	}

	caches, err := pc.providerService.WaitForCacheReady(cacheRequestID)
	if err != nil {
		return nil, err
	}

	providerConstraints, err := getproviders.ParseProviderConstraints(opts, filepath.Dir(opts.TerragruntConfigPath))
	if err != nil {
		l.Debugf("Failed to parse provider constraints from %s: %v", filepath.Dir(opts.TerragruntConfigPath), err)

		providerConstraints = make(getproviders.ProviderConstraints)
	}

	for _, provider := range caches {
		if providerCache, ok := provider.(*services.ProviderCache); ok {
			providerAddr := provider.Address()
			if constraint, exists := providerConstraints[providerAddr]; exists {
				providerCache.Provider.OriginalConstraints = constraint
				l.Debugf("Applied constraint %s to provider %s", constraint, providerAddr)
			} else {
				l.Debugf("No constraint found for provider %s", providerAddr)
			}
		}
	}

	err = getproviders.UpdateLockfile(ctx, opts.WorkingDir, caches)
	if err != nil {
		return nil, err
	}

	// For upgrade scenarios where no providers were newly cached, we still need to update
	// the lock file if module constraints have changed. This only happens during upgrades.
	// Check if user passed -upgrade flag to terraform init
	isUpgrade := opts.TerraformCliArgs.Contains("-upgrade")

	if len(caches) == 0 && len(providerConstraints) > 0 && isUpgrade {
		l.Debugf("No new providers cached, but constraints exist. Updating lock file constraints for upgrade scenario.")

		err = getproviders.UpdateLockfileConstraints(ctx, opts.WorkingDir, providerConstraints)
	}

	return nil, err
}

func (pc *ProviderCache) runTerraformWithCache(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	cliConfigFilename string,
	args clihelper.Args,
	env map[string]string,
) (*util.CmdOutput, error) {
	// Create terraform cli config file that uses provider cache dir
	if err := pc.createLocalCLIConfig(ctx, opts, cliConfigFilename, ""); err != nil {
		return nil, err
	}

	l, cloneOpts, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	cloneOpts.WorkingDir = opts.WorkingDir
	cloneOpts.Env = env

	return tf.RunCommandWithOutput(ctx, l, cloneOpts, args...)
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
func (pc *ProviderCache) createLocalCLIConfig(ctx context.Context, opts *options.TerragruntOptions, filename string, cacheRequestID string) error {
	cfg := pc.cliCfg.Clone()
	cfg.PluginCacheDir = ""

	// Filter registries based on OpenTofu or Terraform implementation to avoid contacting unnecessary registries
	filteredRegistryNames := filterRegistriesByImplementation(
		opts.ProviderCacheRegistryNames,
		opts.TofuImplementation,
	)

	var providerInstallationIncludes = make([]string, 0, len(filteredRegistryNames))

	for _, registryName := range filteredRegistryNames {
		providerInstallationIncludes = append(providerInstallationIncludes, registryName+"/*/*")

		apiURLs, err := pc.DiscoveryURL(ctx, registryName)
		if err != nil {
			return err
		}

		cfg.AddHost(registryName, map[string]string{
			"providers.v1": fmt.Sprintf("%s/%s/%s/", pc.ProviderController.URL(), cacheRequestID, registryName),
			// Since Terragrunt Provider Cache only caches providers, we need to route module requests to the original registry.
			"modules.v1": ResolveModulesURL(registryName, apiURLs.ModulesV1),
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

	// Use VFS for directory operations
	fs := pc.FS()
	cfgDir := filepath.Dir(filename)

	cfgDirExists, err := vfs.FileExists(fs, cfgDir)
	if err != nil {
		return errors.New(err)
	}

	if !cfgDirExists {
		if err := fs.MkdirAll(cfgDir, os.ModePerm); err != nil {
			return errors.New(err)
		}
	}

	return cfg.Save(filename)
}

// isRegistryTimeoutError checks if the error output matches known transient registry timeout patterns
func isRegistryTimeoutError(output []byte) bool {
	return slices.ContainsFunc(registryTimeoutPatterns, func(pattern *regexp.Regexp) bool {
		return pattern.Match(output)
	})
}

func runTerraformCommand(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, args []string, envs map[string]string) (*util.CmdOutput, error) {
	// add -no-color flag to args if it was set in Terragrunt arguments
	if opts.TerraformCliArgs.Contains(tf.FlagNameNoColor) &&
		!slices.Contains(args, tf.FlagNameNoColor) {
		args = append(args, tf.FlagNameNoColor)
	}

	l, cloneOpts, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	cloneOpts.Writer = io.Discard
	cloneOpts.WorkingDir = opts.WorkingDir
	cloneOpts.TerraformCliArgs = iacargs.New(args...)
	cloneOpts.Env = envs

	var finalOutput *util.CmdOutput

	err = util.DoWithRetry(
		ctx,
		"Running terraform providers lock",
		registryRetryMaxAttempts,
		registryRetrySleepInterval,
		l,
		log.DebugLevel,
		func(ctx context.Context) error {
			errWriter := util.NewTrapWriter(opts.ErrWriter)
			cloneOpts.ErrWriter = errWriter

			output, cmdErr := tf.RunCommandWithOutput(ctx, l, cloneOpts, cloneOpts.TerraformCliArgs.Slice()...)
			finalOutput = output

			// If the OpenTofu/Terraform error matches `httpStatusCacheProviderReg` (423 Locked),
			// it means success - the cache recorded the request
			if cmdErr != nil && httpStatusCacheProviderReg.Match(output.Stderr.Bytes()) {
				return nil
			}

			if cmdErr != nil {
				if isRegistryTimeoutError(output.Stderr.Bytes()) {
					return cmdErr
				}

				err = errWriter.Flush()
				if err != nil {
					l.Warnf("Failed to flush stderr: %v", err)
				}

				return util.FatalError{Underlying: cmdErr}
			}

			if flushErr := errWriter.Flush(); flushErr != nil {
				return util.FatalError{Underlying: flushErr}
			}

			return nil
		},
	)
	if err != nil {
		// Unwrap FatalError to return the original error
		var fatalErr util.FatalError
		if errors.As(err, &fatalErr) {
			return finalOutput, fatalErr.Underlying
		}

		return finalOutput, err
	}

	return finalOutput, nil
}

// providerCacheEnvironment returns TF_* name/value ENVs, which we use to force terraform processes to make requests through our cache server (proxy) instead of making direct requests to the origin servers.
func providerCacheEnvironment(opts *options.TerragruntOptions, cliConfigFile string) map[string]string {
	// make copy + ensure non-nil
	envs := make(map[string]string, len(opts.Env))
	maps.Copy(envs, opts.Env)

	// Filter registries based on OpenTofu or Terraform implementation to avoid setting env vars for unnecessary registries
	filteredRegistryNames := filterRegistriesByImplementation(
		opts.ProviderCacheRegistryNames,
		opts.TofuImplementation,
	)

	for _, registryName := range filteredRegistryNames {
		envName := fmt.Sprintf(tf.EnvNameTFTokenFmt, strings.ReplaceAll(registryName, ".", "_"))

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
	envs[tf.EnvNameTFCLIConfigFile] = cliConfigFile
	// Clear this `TF_PLUGIN_CACHE_DIR` value since we are using our own caching mechanism.
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_plugin_cache_dir
	envs[tf.EnvNameTFPluginCacheDir] = ""

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
		if strings.HasPrefix(arg, tf.FlagNamePlatform) {
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

// filterRegistriesByImplementation filters registry names based on the Terraform implementation being used.
// If the registry names match the default registries (both registry.terraform.io and registry.opentofu.org),
// it filters them based on the implementation:
//   - OpenTofuImpl: returns only registry.opentofu.org
//   - TerraformImpl: returns only registry.terraform.io
//   - UnknownImpl: returns both (backward compatibility)
//
// If the user has explicitly set registry names (don't match defaults), returns them as-is.
func filterRegistriesByImplementation(registryNames []string, implementation options.TerraformImplementationType) []string {
	// Default registries in the same order as defined in options/options.go
	defaultRegistries := []string{
		"registry.terraform.io",
		"registry.opentofu.org",
	}

	// Check if registry names match defaults exactly (order-independent)
	if len(registryNames) == len(defaultRegistries) {
		matchesDefault := true

		for _, defaultReg := range defaultRegistries {
			if !slices.Contains(registryNames, defaultReg) {
				matchesDefault = false
				break
			}
		}

		// If matches defaults, filter based on implementation
		if matchesDefault {
			switch implementation {
			case options.OpenTofuImpl:
				return []string{"registry.opentofu.org"}
			case options.TerraformImpl:
				return []string{"registry.terraform.io"}
			case options.UnknownImpl:
				// Backward compatibility: use both registries if implementation is unknown
				return registryNames
			default:
				// Unknown implementation type, return as-is
				return registryNames
			}
		}
	}

	// User explicitly set registry names, return as-is
	return registryNames
}

// ResolveModulesURL resolves the modules.v1 URL from registry discovery.
// If the URL is already absolute (contains "://"), it is returned as-is.
// Otherwise, it is treated as a relative path and combined with the registry name.
func ResolveModulesURL(registryName, modulesV1 string) string {
	if strings.Contains(modulesV1, "://") {
		return modulesV1
	}

	return fmt.Sprintf("https://%s%s", registryName, modulesV1)
}
