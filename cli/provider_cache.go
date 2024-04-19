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
	"github.com/gruntwork-io/terragrunt/terraform/cache/controllers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/exp/maps"
)

const (
	// The paths to the automatically generated local CLI configs
	localCLIFilename = ".terraformrc"
)

var (
	// HTTPStatusCacheProviderReg is regular expression to determine the success result of the command `terraform lock providers -platform=cache provider`.
	// The reg matches if the text contains "423 Locked", for example:
	//
	// - registry.terraform.io/hashicorp/template: could not query provider registry for registry.terraform.io/hashicorp/template: 423 Locked.
	HTTPStatusCacheProviderReg = regexp.MustCompile(`(?mi)` + strconv.Itoa(controllers.HTTPStatusCacheProvider) + `[^a-z0-9]*` + http.StatusText(controllers.HTTPStatusCacheProvider))
)

type ProviderCache struct {
	*cache.Server
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

	if opts.ProviderCacheArchiveDir == "" {
		opts.ProviderCacheArchiveDir = filepath.Join(cacheDir, "archives")
	}

	if opts.ProviderCacheArchiveDir, err = filepath.Abs(opts.ProviderCacheArchiveDir); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if opts.ProviderCacheToken == "" {
		opts.ProviderCacheToken = uuid.New().String()
	}
	// Currently, the cache cache only supports the `x-api-key` token.
	if !strings.HasPrefix(strings.ToLower(opts.ProviderCacheToken), handlers.AuthorizationApiKeyHeaderName+":") {
		opts.ProviderCacheToken = fmt.Sprintf("%s:%s", handlers.AuthorizationApiKeyHeaderName, opts.ProviderCacheToken)
	}

	userProviderDir, err := cliconfig.UserProviderDir()
	if err != nil {
		return nil, err
	}

	cache := cache.NewServer(
		cache.WithHostname(opts.ProviderCacheHostname),
		cache.WithPort(opts.ProviderCachePort),
		cache.WithToken(opts.ProviderCacheToken),
		cache.WithUserProviderDir(userProviderDir),
		cache.WithProviderCacheDir(opts.ProviderCacheDir),
		cache.WithProviderArchiveDir(opts.ProviderCacheArchiveDir),
	)

	return &ProviderCache{Server: cache}, nil
}

func (cache *ProviderCache) TerraformCommandHook(ctx context.Context, opts *options.TerragruntOptions, args []string) (*shell.CmdOutput, error) {
	// To prevent a loop
	ctx = shell.ContextWithTerraformCommandHook(ctx, nil)

	// Use Hook only for the `terraform init` command, which can be run explicitly by the user or Terragrunt's `auto-init` feature.
	if util.FirstArg(args) != terraform.CommandNameInit {
		return shell.RunTerraformCommandWithOutput(ctx, opts, args...)
	}

	var (
		cliConfigFilename     = filepath.Join(opts.WorkingDir, localCLIFilename)
		terraformLockFilename = filepath.Join(opts.WorkingDir, terraform.TerraformLockFile)
		cacheRequestID        = uuid.New().String()
		env                   = providerCacheEnvironment(opts, cliConfigFilename)
	)

	// Create terraform cli config file that enables provider caching and does not use provider cache dir
	if err := cache.createLocalCLIConfig(opts, cliConfigFilename, cacheRequestID, false); err != nil {
		return nil, err
	}

	log.Infof("Caching terraform providers for %s", opts.WorkingDir)
	// Before each init, we warm up the global cache to ensure that all necessary providers are cached.
	// To do this we are using 'terraform providers lock' to force TF to request all the providers from our TG cache, and that's how we know what providers TF needs, and can load them into the cache.
	// It's low cost operation, because it does not cache the same provider twice, but only new previously non-existent providers.
	if err := runTerraformCommand(ctx, opts, args, env); err != nil {
		return nil, err
	}

	cache.Provider.WaitForCacheReady(cacheRequestID)

	// Create terraform cli config file that uses provider cache dir
	if err := cache.createLocalCLIConfig(opts, cliConfigFilename, "", true); err != nil {
		return nil, err
	}

	if opts.ProviderCacheDisablePartialLockFile && !util.FileExists(terraformLockFilename) {
		log.Infof("Getting terraform modules for %s", opts.WorkingDir)
		if err := runTerraformCommand(ctx, opts, []string{terraform.CommandNameGet}, env); err != nil {
			return nil, err
		}

		log.Infof("Generating Terraform lock file for %s", opts.WorkingDir)
		// Create complete terraform lock files. By default this feature is disabled, since it's not superfast.
		// Instead we use Terraform `TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE` feature, that creates hashes from the local cache.
		// And since the Terraform developers warn that this feature will be removed soon, it's good to have a workaround.
		if err := runTerraformCommand(ctx, opts, []string{terraform.CommandNameProviders, terraform.CommandNameLock}, env); err != nil {
			return nil, err
		}

	}

	cloneOpts := opts.Clone(opts.TerragruntConfigPath)
	cloneOpts.WorkingDir = opts.WorkingDir
	maps.Copy(cloneOpts.Env, env)

	return shell.RunTerraformCommandWithOutput(ctx, cloneOpts, args...)
}

// createLocalCLIConfig creates a local CLI configs that merges the default/user configuration with our Private Registry configuration.
// We don't want to use Terraform's `plugin_cache_dir` feature because the cache is populated by our Terragrunt Provider Cache cache, and to make sure that no Terraform process ever overwrites the global cache, we clear this value.
// In order to force Terraform to queries our cache cache instead of the original one, we use the section below.
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
// This func doesn't change the default CLI config file, only creates a new one at the given path `cliConfigFile`. Ultimately, we can assign this path to `TF_CLI_CONFIG_FILE`.
func (cache *ProviderCache) createLocalCLIConfig(opts *options.TerragruntOptions, filename string, cacheRequestID string, setProviderInstallation bool) error {
	cfg, err := cliconfig.LoadUserConfig()
	if err != nil {
		return err
	}
	cfg.PluginCacheDir = ""

	var providerInstallationIncludes []string

	for _, registryName := range opts.ProviderCacheRegistryNames {
		providerInstallationIncludes = append(providerInstallationIncludes, fmt.Sprintf("%s/*/*", registryName))

		cfg.AddHost(registryName, map[string]any{
			"providers.v1": fmt.Sprintf("%s/%s/%s/", cache.ProviderURL(), cacheRequestID, registryName),
			// Since Terragrunt Provider Cache only caches providers, we need to route module requests to the original registry.
			"modules.v1": fmt.Sprintf("https://%s/v1/modules", registryName),
		})
	}

	if setProviderInstallation {
		cfg.SetProviderInstallation(
			cliconfig.NewProviderInstallationFilesystemMirror(opts.ProviderCacheDir, providerInstallationIncludes, nil),
			cliconfig.NewProviderInstallationDirect(providerInstallationIncludes, nil),
		)
	} else {
		cfg.SetProviderInstallation(
			nil,
			cliconfig.NewProviderInstallationDirect(nil, nil),
		)
	}

	if cfgDir := filepath.Dir(filename); !util.FileExists(cfgDir) {
		if err := os.MkdirAll(cfgDir, os.ModePerm); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return cfg.Save(filename)
}

func runTerraformCommand(ctx context.Context, opts *options.TerragruntOptions, args []string, envs map[string]string) error {
	// We use custom writer in order to trap the log from `terraform providers lock -platform=provider-cache` command, which terraform considers an error, but to us a success.
	errWriter := util.NewTrapWriter(opts.ErrWriter, HTTPStatusCacheProviderReg)

	// add -no-color flag if the user specified it in the CLI
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
	if err := shell.RunTerraformCommand(ctx, cloneOpts, cloneOpts.TerraformCliArgs...); err != nil && len(errWriter.Msgs()) == 0 {
		return err
	}
	return nil
}

// providerCacheEnvironment returns TF_* name/value ENVs, which we use to force terraform processes to make requests through our cache cache (proxy) instead of making direct requests to the origin servers.
func providerCacheEnvironment(opts *options.TerragruntOptions, cliConfigFile string) map[string]string {
	envs := make(map[string]string)

	for _, registryName := range opts.ProviderCacheRegistryNames {
		envName := fmt.Sprintf(terraform.EnvNameTFTokenFmt, strings.ReplaceAll(registryName, ".", "_"))
		// We use `TF_TOKEN_*` for authentication with our private registry (cache cache).
		// https://developer.hashicorp.com/terraform/cli/config/config-file#environment-variable-credentials
		envs[envName] = opts.ProviderCacheToken
	}

	if !opts.ProviderCacheDisablePartialLockFile {
		// By using `TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE` we force terraform to generate `.terraform.lock.hcl` only based on cached files, otherwise it downloads three files (provider zip archive, SHA256SUMS, sig) from the original registry to calculate hashes.
		// https://developer.hashicorp.com/terraform/cli/config/config-file#allowing-the-provider-plugin-cache-to-break-the-dependency-lock-file
		envs[terraform.EnvNameTFPluginCacheMayBreakDependencyLockFile] = "1"
	}

	// By using `TF_CLI_CONFIG_FILE` we force terraform to use our auto-generated cli configuration file.
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_cli_config_file
	envs[terraform.EnvNameTFCLIConfigFile] = cliConfigFile
	// Clear this `TF_PLUGIN_CACHE_DIR` value since we are using our own caching mechanism.
	// https://developer.hashicorp.com/terraform/cli/config/environment-variables#tf_plugin_cache_dir
	envs[terraform.EnvNameTFPluginCacheDir] = ""

	return envs
}
