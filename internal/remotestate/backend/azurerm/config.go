package azurerm

import (
	"maps"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mitchellh/mapstructure"
)

// Config is the raw remote_state backend configuration for the azurerm backend.
type Config map[string]any

// GetTFInitArgs returns the config filtered (Terragrunt-only keys removed) and
// normalized for `tofu init -backend-config`.
func (cfg Config) GetTFInitArgs() Config {
	filtered := cfg.FilterOutTerragruntKeys()

	return Config(backend.NormalizeBoolValues(backend.Config(filtered), &ExtendedRemoteStateConfigAzurerm{}))
}

// FilterOutTerragruntKeys returns a copy of the config with the Terragrunt-only
// bootstrap keys removed, leaving only keys the azurerm backend understands.
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

// IsEqual returns true if the backend portion of cfg equals targetCfg.
func (cfg Config) IsEqual(targetCfg Config, logger log.Logger) bool {
	newConfig := backend.Config{}
	maps.Copy(newConfig, cfg.FilterOutTerragruntKeys())

	return newConfig.IsEqual(backend.Config(targetCfg), BackendName, logger)
}

// ParseExtendedAzurermConfig parses the raw map into an extended azurerm config
// (without validation).
func (cfg Config) ParseExtendedAzurermConfig() (*ExtendedRemoteStateConfigAzurerm, error) {
	var extendedConfig ExtendedRemoteStateConfigAzurerm

	// RemoteStateConfigAzurerm is squashed, so a single decode populates both the
	// extended fields and the nested backend fields.
	if err := mapstructure.WeakDecode(cfg, &extendedConfig); err != nil {
		return nil, err
	}

	return &extendedConfig, nil
}

// ExtendedAzurermConfig parses the raw map into an extended azurerm config and
// validates it.
func (cfg Config) ExtendedAzurermConfig() (*ExtendedRemoteStateConfigAzurerm, error) {
	extCfg, err := cfg.ParseExtendedAzurermConfig()
	if err != nil {
		return nil, err
	}

	return extCfg, extCfg.Validate()
}
