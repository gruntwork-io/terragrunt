package gcs

import (
	"reflect"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/exp/slices"

	"maps"

	"github.com/gruntwork-io/terragrunt/util"
)

type Config map[string]any

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

	if err := mapstructure.Decode(cfg, &gcsConfig); err != nil {
		return nil, errors.New(err)
	}

	if err := mapstructure.Decode(cfg, &extendedConfig); err != nil {
		return nil, errors.New(err)
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
