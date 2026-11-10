package gcs

import (
	"maps"
	"reflect"
	"slices"
	"strconv"

	"github.com/go-viper/mapstructure/v2"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Config is the config map of a gcs remote_state block, including the keys only Terragrunt reads.
type Config map[string]any

// GetTFInitArgs returns the config filtered and normalized for terraform init.
func (cfg Config) GetTFInitArgs() Config {
	filtered := cfg.FilterOutTerragruntKeys()

	return Config(
		backend.NormalizeBoolValues(backend.Config(filtered), &ExtendedRemoteStateConfigGCS{}),
	)
}

// FilterOutTerragruntKeys returns a copy of cfg without the keys that only Terragrunt reads, such as project,
// location, and gcs_bucket_labels.
func (cfg Config) FilterOutTerragruntKeys() Config {
	var filtered = make(Config)

	for key, val := range cfg {
		if slices.Contains(terragruntOnlyConfigs, key) {
			continue
		}

		filtered[key] = val
	}

	return filtered
}

// IsEqual reports whether targetCfg matches cfg once the Terragrunt-only keys are removed from cfg.
//
// Modifies targetCfg: a string value that parses as a bool, under a key whose value in cfg is a bool, is
// replaced with the parsed bool.
func (cfg Config) IsEqual(targetCfg Config, logger log.Logger) bool {
	// If other keys in config are bools, DeepEqual also will consider the maps to be different.
	for key, value := range targetCfg {
		if util.KindOf(targetCfg[key]) == reflect.String && util.KindOf(cfg[key]) == reflect.Bool {
			if convertedValue, err := strconv.ParseBool(value.(string)); err == nil {
				targetCfg[key] = convertedValue
			}
		}
	}

	// Construct a new map excluding custom GCS labels that are only used in Terragrunt config and not in Terraform's backend
	newConfig := backend.Config{}

	maps.Copy(newConfig, cfg.FilterOutTerragruntKeys())

	return newConfig.IsEqual(backend.Config(targetCfg), BackendName, logger)
}

// ParseExtendedGCSConfig parses the given map into a GCS config.
func (cfg Config) ParseExtendedGCSConfig() (*ExtendedRemoteStateConfigGCS, error) {
	var (
		gcsConfig      RemoteStateConfigGCS
		extendedConfig ExtendedRemoteStateConfigGCS
	)

	if err := mapstructure.WeakDecode(cfg, &gcsConfig); err != nil {
		return nil, err
	}

	if err := mapstructure.WeakDecode(cfg, &extendedConfig); err != nil {
		return nil, err
	}

	extendedConfig.RemoteStateConfigGCS = gcsConfig

	return &extendedConfig, nil
}

// ExtendedGCSConfig parses the given map into an extended GCS config and validates this config.
func (cfg Config) ExtendedGCSConfig() (*ExtendedRemoteStateConfigGCS, error) {
	extGCSCfg, err := cfg.ParseExtendedGCSConfig()
	if err != nil {
		return nil, err
	}

	return extGCSCfg, extGCSCfg.Validate()
}
