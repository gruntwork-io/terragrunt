package cliconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl"
	hclast "github.com/hashicorp/hcl/hcl/ast"
	svchost "github.com/hashicorp/terraform-svchost"

	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	// envNamePluginCacheDir names the env var that overrides plugin_cache_dir from every config file.
	envNamePluginCacheDir = "TF_PLUGIN_CACHE_DIR"
	// userProviderDirName is the directory under the CLI config dir holding user-installed plugins.
	userProviderDirName = "plugins"
	// providerInstallationBlockName is the top-level block whose method blocks the AST walk collects.
	providerInstallationBlockName = "provider_installation"
	// devOverridesBlockName is the provider_installation method Terragrunt parses but does not model.
	devOverridesBlockName = "dev_overrides"
)

// userConfigFile mirrors the shape OpenTofu and Terraform decode the CLI config into.
type userConfigFile struct {
	Hosts                      map[string]*userConfigHost              `hcl:"host"`
	Credentials                map[string]map[string]any               `hcl:"credentials"`
	CredentialsHelpers         map[string]*userConfigCredentialsHelper `hcl:"credentials_helper"`
	PluginCacheDir             string                                  `hcl:"plugin_cache_dir"`
	DisableCheckpoint          bool                                    `hcl:"disable_checkpoint"`
	DisableCheckpointSignature bool                                    `hcl:"disable_checkpoint_signature"`
}

// userConfigHost is the "host" block, which overrides service discovery for one hostname.
type userConfigHost struct {
	Services map[string]any `hcl:"services"`
}

// userConfigCredentialsHelper is the "credentials_helper" block naming an external token provider.
type userConfigCredentialsHelper struct {
	Args []string `hcl:"args"`
}

// providerInstallationMethodAttributes are the attributes every provider_installation method may carry.
type providerInstallationMethodAttributes struct {
	Path    string   `hcl:"path"`
	URL     string   `hcl:"url"`
	Include []string `hcl:"include"`
	Exclude []string `hcl:"exclude"`
}

// LoadUserConfig loads the OpenTofu/Terraform CLI configuration from the
// filesystem and platform handles carried by v, reading the file locations
// impl's binary would read.
func LoadUserConfig(v *venv.Venv, impl tfimpl.Type, opts ...ConfigOption) (*Config, error) {
	v.RequireEnv()
	v.RequireFS()
	v.RequirePlatform()

	paths, err := userConfigPaths(v, impl)
	if err != nil {
		return nil, err
	}

	config := NewConfig(v.FS).WithProviderInstallation(&ProviderInstallation{})

	var helperSources []string

	for _, path := range paths {
		fileConfig, err := loadUserConfigFile(v, path)
		if err != nil {
			return nil, err
		}

		if fileConfig.CredentialsHelpers != nil {
			helperSources = append(helperSources, path)
		}

		mergeUserConfig(config, fileConfig)
	}

	// The env var overrides every file, and is deliberately not expanded.
	if pluginCacheDir := v.Env[envNamePluginCacheDir]; pluginCacheDir != "" {
		config.PluginCacheDir = pluginCacheDir
	}

	if err := validateUserConfig(config, helperSources); err != nil {
		return nil, err
	}

	return config.WithOptions(opts...), nil
}

// UserProviderDir returns the absolute directory where OpenTofu/Terraform discovers
// user-installed provider plugins, or "" when no CLI config directory resolves. It never
// returns a relative path, which a caller would otherwise resolve against its own
// working directory.
func UserProviderDir(v *venv.Venv, impl tfimpl.Type) (string, error) {
	configDir, err := UserConfigDir(v, impl)
	if err != nil {
		return "", err
	}

	if configDir == "" {
		return "", nil
	}

	return filepath.Join(configDir, userProviderDirName), nil
}

// userConfigPaths lists the CLI config files to load, most general first, honoring the env-var override.
func userConfigPaths(v *venv.Venv, impl tfimpl.Type) ([]string, error) {
	// An override names the file outright, so an unreadable one is the user's error and the config dir is skipped.
	if override, envName := UserConfigOverride(v); override != "" {
		exists, err := vfs.FileExists(v.FS, override)
		if err != nil {
			return nil, fmt.Errorf("%w: checking %s from %s: %w", ErrUserConfig, override, envName, err)
		}

		if exists {
			return []string{override}, nil
		}

		return nil, nil
	}

	var paths []string

	// A default candidate that cannot be stat'd reads as absent, the way OpenTofu treats one.
	for _, candidate := range UserConfigCandidates(v, impl) {
		if !vfs.Exists(v.FS, candidate) {
			continue
		}

		paths = append(paths, candidate)

		break
	}

	configDir, err := UserConfigDir(v, impl)
	if err != nil {
		return nil, err
	}

	fragments, err := userConfigFragments(v, configDir)
	if err != nil {
		return nil, err
	}

	return append(paths, fragments...), nil
}

// userConfigFragments lists the *.tfrc fragments configDir contributes, in name order.
func userConfigFragments(v *venv.Venv, configDir string) ([]string, error) {
	if configDir == "" || !vfs.IsDir(v.FS, configDir) {
		return nil, nil
	}

	entries, err := vfs.ReadDir(v.FS, configDir)
	if err != nil {
		return nil, fmt.Errorf("%w: reading directory %s: %w", ErrUserConfig, configDir, err)
	}

	fragments := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !isUserConfigFragment(entry.Name()) {
			continue
		}

		fragments = append(fragments, filepath.Join(configDir, entry.Name()))
	}

	return fragments, nil
}

// isUserConfigFragment reports whether name is a CLI config fragment OpenTofu would read.
func isUserConfigFragment(name string) bool {
	return strings.HasSuffix(name, ".tfrc") || strings.HasSuffix(name, ".tfrc.json")
}

// loadUserConfigFile parses one CLI config file, which is HCL1 in both its native and its JSON spelling.
func loadUserConfigFile(v *venv.Venv, path string) (*Config, error) {
	data, err := vfs.ReadFile(v.FS, path)
	if err != nil {
		return nil, fmt.Errorf("%w: reading %s: %w", ErrUserConfig, path, err)
	}

	// hcl.Parse detects the JSON spelling from the content, so the file name never has to.
	node, err := hcl.Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("%w: parsing %s: %w", ErrUserConfig, path, err)
	}

	var file userConfigFile
	if err := hcl.DecodeObject(&file, node); err != nil {
		return nil, fmt.Errorf("%w: decoding %s: %w", ErrUserConfig, path, err)
	}

	// Ranging a map to pick "the" helper would decide by iteration order, so reject the
	// ambiguity the way OpenTofu's Config.Validate does.
	if len(file.CredentialsHelpers) > 1 {
		return nil, fmt.Errorf(
			"%w: no more than one credentials_helper block may be specified, %s declares %d",
			ErrInvalidUserConfig, path, len(file.CredentialsHelpers),
		)
	}

	methods, err := decodeProviderInstallation(path, node)
	if err != nil {
		return nil, err
	}

	config := NewConfig(v.FS).
		WithPluginCacheDir(expandUserConfigEnv(file.PluginCacheDir, v.Env)).
		WithCredentials(userCredentials(file)).
		WithCredentialsHelpers(userCredentialsHelper(file)).
		WithHosts(userHosts(file)).
		WithProviderInstallation(&ProviderInstallation{Methods: methods})

	if file.DisableCheckpoint {
		config.WithDisableCheckpoint()
	}

	if file.DisableCheckpointSignature {
		config.WithDisableCheckpointSignature()
	}

	return config, nil
}

// decodeProviderInstallation walks the AST for provider_installation methods, whose ordered,
// repeated block structure the struct decoder cannot represent.
func decodeProviderInstallation(path string, node hclast.Node) (ProviderInstallationMethods, error) {
	file, ok := node.(*hclast.File)
	if !ok {
		return nil, nil
	}

	root, ok := file.Node.(*hclast.ObjectList)
	if !ok {
		return nil, nil
	}

	var methods ProviderInstallationMethods

	for _, block := range root.Items {
		if len(block.Keys) == 0 || block.Keys[0].Token.Value() != providerInstallationBlockName {
			continue
		}

		body, ok := block.Val.(*hclast.ObjectType)
		if !ok {
			return nil, fmt.Errorf(
				"%w: the provider_installation block at %s in %s must be a block, not an attribute",
				ErrInvalidUserConfig, block.Pos(), path,
			)
		}

		for _, methodBlock := range body.List.Items {
			method, err := decodeProviderInstallationMethod(path, methodBlock)
			if err != nil {
				return nil, err
			}

			if method != nil {
				methods = append(methods, method)
			}
		}
	}

	return methods, nil
}

// decodeProviderInstallationMethod decodes one direct, filesystem_mirror, or network_mirror block.
func decodeProviderInstallationMethod(path string, block *hclast.ObjectItem) (ProviderInstallationMethod, error) {
	if len(block.Keys) == 0 {
		return nil, nil
	}

	methodType, _ := block.Keys[0].Token.Value().(string)

	// dev_overrides bypasses version selection entirely, so it has no place in the generated config.
	if methodType == devOverridesBlockName {
		return nil, nil
	}

	var attrs providerInstallationMethodAttributes
	if err := hcl.DecodeObject(&attrs, block.Val); err != nil {
		return nil, fmt.Errorf(
			"%w: decoding the %s block at %s in %s: %w",
			ErrUserConfig, methodType, block.Pos(), path, err,
		)
	}

	switch methodType {
	case "direct":
		return NewProviderInstallationDirect(attrs.Include, attrs.Exclude), nil
	case "filesystem_mirror":
		return NewProviderInstallationFilesystemMirror(attrs.Path, attrs.Include, attrs.Exclude), nil
	case "network_mirror":
		return NewProviderInstallationNetworkMirror(attrs.URL, attrs.Include, attrs.Exclude), nil
	default:
		return nil, fmt.Errorf(
			"%w: unsupported provider installation method %q at %s in %s",
			ErrInvalidUserConfig, methodType, block.Pos(), path,
		)
	}
}

// expandUserConfigEnv expands $VAR and ${VAR} references in value against the injected environment.
func expandUserConfigEnv(value string, env map[string]string) string {
	if value == "" {
		return ""
	}

	return os.Expand(value, func(name string) string { return env[name] })
}

// validateUserConfig rejects the malformed blocks OpenTofu rejects, so a bad hostname surfaces
// here rather than as an unauthenticated registry request later. helperSources names every
// file that declared a credentials_helper, which upstream allows only once across them all.
func validateUserConfig(config *Config, helperSources []string) error {
	if len(helperSources) > 1 {
		return fmt.Errorf(
			"%w: no more than one credentials_helper block may be specified, found one in each of %s",
			ErrInvalidUserConfig, strings.Join(helperSources, ", "),
		)
	}

	for _, creds := range config.Credentials {
		if _, err := svchost.ForComparison(creds.Name); err != nil {
			return fmt.Errorf(
				"%w: the credentials %q block has an invalid hostname: %w",
				ErrInvalidUserConfig, creds.Name, err,
			)
		}
	}

	for _, host := range config.Hosts {
		if _, err := svchost.ForComparison(host.Name); err != nil {
			return fmt.Errorf(
				"%w: the host %q block has an invalid hostname: %w",
				ErrInvalidUserConfig, host.Name, err,
			)
		}
	}

	return nil
}

// mergeUserConfig folds with into config, matching how OpenTofu's Config.Merge resolves each field.
func mergeUserConfig(config, with *Config) {
	// plugin_cache_dir is first-wins upstream, unlike every other field here.
	if config.PluginCacheDir == "" {
		config.PluginCacheDir = with.PluginCacheDir
	}

	config.DisableCheckpoint = config.DisableCheckpoint || with.DisableCheckpoint
	config.DisableCheckpointSignature = config.DisableCheckpointSignature || with.DisableCheckpointSignature

	for _, credentials := range with.Credentials {
		setCredentials(config, credentials)
	}

	if with.CredentialsHelpers != nil {
		config.CredentialsHelpers = with.CredentialsHelpers
	}

	for _, host := range with.Hosts {
		setHost(config, host)
	}

	config.ProviderInstallation.Methods = append(
		config.ProviderInstallation.Methods,
		with.ProviderInstallation.Methods...,
	)
}

// userCredentials flattens the decoded credentials blocks, keeping only the token each one carries.
func userCredentials(file userConfigFile) []ConfigCredentials {
	credentials := make([]ConfigCredentials, 0, len(file.Credentials))

	for name, credential := range file.Credentials {
		token, _ := credential["token"].(string)
		credentials = append(credentials, ConfigCredentials{Name: name, Token: token})
	}

	// Map iteration is random, so order by hostname to keep a merged config reproducible.
	slices.SortFunc(credentials, func(a, b ConfigCredentials) int { return strings.Compare(a.Name, b.Name) })

	return credentials
}

// userCredentialsHelper returns the single credentials_helper block, or nil when the file
// declares none. More than one is rejected by the caller, so the map holds at most one entry.
func userCredentialsHelper(file userConfigFile) *ConfigCredentialsHelper {
	for name, helper := range file.CredentialsHelpers {
		var args []string
		if helper != nil {
			args = helper.Args
		}

		return &ConfigCredentialsHelper{Name: name, Args: args}
	}

	return nil
}

// userHosts flattens the decoded host blocks into their service-discovery overrides.
func userHosts(file userConfigFile) []ConfigHost {
	hosts := make([]ConfigHost, 0, len(file.Hosts))

	for name, host := range file.Hosts {
		services := make(map[string]string)

		if host != nil {
			for key, val := range host.Services {
				if val, ok := val.(string); ok {
					services[key] = val
				}
			}
		}

		hosts = append(hosts, ConfigHost{Name: name, Services: services})
	}

	slices.SortFunc(hosts, func(a, b ConfigHost) int { return strings.Compare(a.Name, b.Name) })

	return hosts
}

// setCredentials replaces the credentials already recorded for the same host, or appends them.
func setCredentials(config *Config, credentials ConfigCredentials) {
	for i := range config.Credentials {
		if config.Credentials[i].Name != credentials.Name {
			continue
		}

		config.Credentials[i] = credentials

		return
	}

	config.Credentials = append(config.Credentials, credentials)
}

// setHost replaces the host already recorded under the same name, or appends it.
func setHost(config *Config, host ConfigHost) {
	for i := range config.Hosts {
		if config.Hosts[i].Name != host.Name {
			continue
		}

		config.Hosts[i] = host

		return
	}

	config.Hosts = append(config.Hosts, host)
}
