package azurerm

import (
	"maps"
	"reflect"
	"slices"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mitchellh/mapstructure"
)

// Config is the raw map[string]any view of an azurerm backend block, as
// produced by HCL parsing before any typing.
type Config map[string]any

// GetTFInitArgs returns the config filtered (terragrunt-only keys
// stripped) and bool-normalized so it can be passed to terraform init.
func (cfg Config) GetTFInitArgs() Config {
	filtered := cfg.FilterOutTerragruntKeys()

	return Config(backend.NormalizeBoolValues(backend.Config(filtered), &ExtendedRemoteStateConfigAzureRM{}))
}

// FilterOutTerragruntKeys removes terragrunt-only keys (location,
// skip_*, soft-delete tunables, tags, …) so terraform never sees them.
func (cfg Config) FilterOutTerragruntKeys() Config {
	filtered := make(Config, len(cfg))

	for key, val := range cfg {
		if slices.Contains(terragruntOnlyConfigs, key) {
			continue
		}

		filtered[key] = val
	}

	return filtered
}

// IsEqual reports whether cfg and targetCfg describe the same backend
// after normalizing string-encoded booleans and stripping terragrunt-only
// keys, mirroring the gcs and s3 backends.
func (cfg Config) IsEqual(targetCfg Config, logger log.Logger) bool {
	for key, value := range targetCfg {
		if util.KindOf(targetCfg[key]) == reflect.String && util.KindOf(cfg[key]) == reflect.Bool {
			if convertedValue, err := strconv.ParseBool(value.(string)); err == nil {
				targetCfg[key] = convertedValue
			}
		}
	}

	newConfig := backend.Config{}
	maps.Copy(newConfig, cfg.FilterOutTerragruntKeys())

	return newConfig.IsEqual(backend.Config(targetCfg), BackendName, logger)
}

// ParseExtendedAzureRMConfig decodes the raw map into the strongly-typed
// extended config without running validation.
func (cfg Config) ParseExtendedAzureRMConfig() (*ExtendedRemoteStateConfigAzureRM, error) {
	var (
		azureCfg    RemoteStateConfigAzureRM
		extendedCfg ExtendedRemoteStateConfigAzureRM
	)

	if err := mapstructure.WeakDecode(cfg, &azureCfg); err != nil {
		return nil, errors.New(err)
	}

	if err := mapstructure.WeakDecode(cfg, &extendedCfg); err != nil {
		return nil, errors.New(err)
	}

	extendedCfg.RemoteStateConfigAzureRM = azureCfg

	return &extendedCfg, nil
}

// ExtendedAzureRMConfig parses and validates the config in one shot.
func (cfg Config) ExtendedAzureRMConfig() (*ExtendedRemoteStateConfigAzureRM, error) {
	extCfg, err := cfg.ParseExtendedAzureRMConfig()
	if err != nil {
		return nil, err
	}

	return extCfg, extCfg.Validate()
}
