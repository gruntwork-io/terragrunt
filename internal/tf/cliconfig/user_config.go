package cliconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const envNamePluginCacheDir = "TF_PLUGIN_CACHE_DIR"

var userConfigBlocks = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
	{Type: "credentials", LabelNames: []string{"hostname"}},
	{Type: "credentials_helper", LabelNames: []string{"name"}},
	{Type: "host", LabelNames: []string{"hostname"}},
	{Type: "provider_installation"},
}}

var providerInstallationBlocks = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
	{Type: "direct"},
	{Type: "filesystem_mirror"},
	{Type: "network_mirror"},
}}

type userConfigAttributes struct {
	Remain                     hcl.Body `hcl:",remain"`
	PluginCacheDir             string   `hcl:"plugin_cache_dir,optional"`
	DisableCheckpoint          bool     `hcl:"disable_checkpoint,optional"`
	DisableCheckpointSignature bool     `hcl:"disable_checkpoint_signature,optional"`
}

type credentialsAttributes struct {
	Remain hcl.Body `hcl:",remain"`
	Token  string   `hcl:"token,optional"`
}

type credentialsHelperAttributes struct {
	Remain hcl.Body `hcl:",remain"`
	Args   []string `hcl:"args,optional"`
}

type hostAttributes struct {
	Remain   hcl.Body          `hcl:",remain"`
	Services map[string]string `hcl:"services,optional"`
}

type directAttributes struct {
	Include []string `hcl:"include,optional"`
	Exclude []string `hcl:"exclude,optional"`
}

type filesystemMirrorAttributes struct {
	Path    string   `hcl:"path,attr"`
	Include []string `hcl:"include,optional"`
	Exclude []string `hcl:"exclude,optional"`
}

type networkMirrorAttributes struct {
	URL     string   `hcl:"url,attr"`
	Include []string `hcl:"include,optional"`
	Exclude []string `hcl:"exclude,optional"`
}

// LoadUserConfig loads the OpenTofu/Terraform CLI configuration from the
// filesystem and platform handles carried by v.
func LoadUserConfig(v *venv.Venv, opts ...ConfigOption) (*Config, error) {
	v.RequireEnv()
	v.RequireFS()
	v.RequirePlatform()

	paths, err := userConfigPaths(v)
	if err != nil {
		return nil, err
	}

	config := NewConfig(v.FS).WithProviderInstallation(&ProviderInstallation{})

	for _, path := range paths {
		fileConfig, err := loadUserConfigFile(v, path)
		if err != nil {
			return nil, err
		}

		mergeUserConfig(config, fileConfig)
	}

	if pluginCacheDir := v.Env[envNamePluginCacheDir]; pluginCacheDir != "" {
		config.PluginCacheDir = pluginCacheDir
	}

	return config.WithOptions(opts...), nil
}

// UserProviderDir returns the directory where OpenTofu/Terraform discovers
// user-installed provider plugins.
func UserProviderDir(v *venv.Venv) (string, error) {
	configDir, err := UserConfigDir(v)
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "plugins"), nil
}

// userConfigPaths lists the CLI config files to load, most general first, honoring the env-var override.
func userConfigPaths(v *venv.Venv) ([]string, error) {
	if override, _ := UserConfigOverride(v); override != "" {
		exists, err := vfs.FileExists(v.FS, override)
		if err != nil {
			return nil, fmt.Errorf("checking CLI config override %s: %w", override, err)
		}

		if exists {
			return []string{override}, nil
		}

		return nil, nil
	}

	var paths []string

	for _, candidate := range UserConfigCandidates(v) {
		exists, err := vfs.FileExists(v.FS, candidate)
		if err != nil {
			return nil, fmt.Errorf("checking CLI config %s: %w", candidate, err)
		}

		if !exists {
			continue
		}

		paths = append(paths, candidate)

		break
	}

	configDir, err := UserConfigDir(v)
	if err != nil {
		return nil, err
	}

	fragments, err := userConfigFragments(v, configDir)
	if err != nil {
		return nil, err
	}

	return append(paths, fragments...), nil
}

// userConfigFragments lists the *.tfrc fragments configDir contributes, in directory order.
func userConfigFragments(v *venv.Venv, configDir string) ([]string, error) {
	if configDir == "" {
		return nil, nil
	}

	info, err := v.FS.Stat(configDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("reading CLI config directory %s: %w", configDir, err)
	}

	if !info.IsDir() {
		return nil, nil
	}

	entries, err := vfs.ReadDir(v.FS, configDir)
	if err != nil {
		return nil, fmt.Errorf("reading CLI config directory %s: %w", configDir, err)
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

// loadUserConfigFile parses one CLI config file, in native HCL or its JSON variant.
func loadUserConfigFile(v *venv.Venv, path string) (*Config, error) {
	data, err := vfs.ReadFile(v.FS, path)
	if err != nil {
		return nil, fmt.Errorf("reading CLI config %s: %w", path, err)
	}

	parser := hclparse.NewParser()

	var (
		file  *hcl.File
		diags hcl.Diagnostics
	)

	if strings.HasSuffix(path, ".json") {
		file, diags = parser.ParseJSON(data, path)
	}

	if file == nil {
		file, diags = parser.ParseHCL(data, path)
	}

	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing CLI config %s: %w", path, errors.New(diags.Error()))
	}

	var attrs userConfigAttributes
	if diags := gohcl.DecodeBody(file.Body, nil, &attrs); diags.HasErrors() {
		return nil, fmt.Errorf("decoding CLI config %s: %w", path, errors.New(diags.Error()))
	}

	config := NewConfig(v.FS).
		WithPluginCacheDir(expandUserConfigEnv(attrs.PluginCacheDir, v.Env)).
		WithProviderInstallation(&ProviderInstallation{})

	if attrs.DisableCheckpoint {
		config.WithDisableCheckpoint()
	}

	if attrs.DisableCheckpointSignature {
		config.WithDisableCheckpointSignature()
	}

	content, _, diags := attrs.Remain.PartialContent(userConfigBlocks)
	if diags.HasErrors() {
		return nil, fmt.Errorf("decoding CLI config blocks in %s: %w", path, errors.New(diags.Error()))
	}

	for _, block := range content.Blocks {
		if err := decodeUserConfigBlock(config, block); err != nil {
			return nil, fmt.Errorf("decoding CLI config %s: %w", path, err)
		}
	}

	return config, nil
}

// expandUserConfigEnv expands $VAR references in value against the injected environment.
func expandUserConfigEnv(value string, env map[string]string) string {
	return os.Expand(value, func(name string) string { return env[name] })
}

// decodeUserConfigBlock decodes one top-level CLI config block into config.
func decodeUserConfigBlock(config *Config, block *hcl.Block) error {
	switch block.Type {
	case "credentials":
		var attrs credentialsAttributes
		if diags := gohcl.DecodeBody(block.Body, nil, &attrs); diags.HasErrors() {
			return errors.New(diags.Error())
		}

		setCredentials(config, ConfigCredentials{Name: block.Labels[0], Token: attrs.Token})
	case "credentials_helper":
		var attrs credentialsHelperAttributes
		if diags := gohcl.DecodeBody(block.Body, nil, &attrs); diags.HasErrors() {
			return errors.New(diags.Error())
		}

		config.CredentialsHelpers = &ConfigCredentialsHelper{Name: block.Labels[0], Args: attrs.Args}
	case "host":
		var attrs hostAttributes
		if diags := gohcl.DecodeBody(block.Body, nil, &attrs); diags.HasErrors() {
			return errors.New(diags.Error())
		}

		setHost(config, ConfigHost{Name: block.Labels[0], Services: attrs.Services})
	case "provider_installation":
		return decodeProviderInstallation(config, block.Body)
	}

	return nil
}

// decodeProviderInstallation appends the installation methods a provider_installation block declares.
func decodeProviderInstallation(config *Config, body hcl.Body) error {
	content, _, diags := body.PartialContent(providerInstallationBlocks)
	if diags.HasErrors() {
		return errors.New(diags.Error())
	}

	for _, block := range content.Blocks {
		method, err := decodeProviderInstallationMethod(block)
		if err != nil {
			return err
		}

		config.ProviderInstallation.Methods = append(config.ProviderInstallation.Methods, method)
	}

	return nil
}

// decodeProviderInstallationMethod decodes one direct, filesystem_mirror, or network_mirror block.
func decodeProviderInstallationMethod(block *hcl.Block) (ProviderInstallationMethod, error) {
	switch block.Type {
	case "direct":
		var attrs directAttributes
		if diags := gohcl.DecodeBody(block.Body, nil, &attrs); diags.HasErrors() {
			return nil, errors.New(diags.Error())
		}

		return NewProviderInstallationDirect(attrs.Include, attrs.Exclude), nil
	case "filesystem_mirror":
		var attrs filesystemMirrorAttributes
		if diags := gohcl.DecodeBody(block.Body, nil, &attrs); diags.HasErrors() {
			return nil, errors.New(diags.Error())
		}

		return NewProviderInstallationFilesystemMirror(attrs.Path, attrs.Include, attrs.Exclude), nil
	case "network_mirror":
		var attrs networkMirrorAttributes
		if diags := gohcl.DecodeBody(block.Body, nil, &attrs); diags.HasErrors() {
			return nil, errors.New(diags.Error())
		}

		return NewProviderInstallationNetworkMirror(attrs.URL, attrs.Include, attrs.Exclude), nil
	default:
		return nil, fmt.Errorf("unsupported provider installation method %q", block.Type)
	}
}

// mergeUserConfig folds with into config, letting the later file win the way OpenTofu's Config.Merge does.
func mergeUserConfig(config, with *Config) {
	if with.PluginCacheDir != "" {
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
