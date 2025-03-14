package gcs

import (
	"reflect"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mitchellh/mapstructure"
	"golang.org/x/exp/slices"

	"github.com/gruntwork-io/terragrunt/util"
)

const (
	lockTableKey                 = "lock_table"
	dynamoDBTableKey             = "dynamodb_table"
	encryptKey                   = "encrypt"
	assumeRoleKey                = "assume_role"
	accessloggingTargetPrefixKey = "accesslogging_target_prefix"

	DefaultS3BucketAccessLoggingTargetPrefix = "TFStateLogs/"

	lockTableDeprecationMessage = "Remote state configuration 'lock_table' attribute is deprecated; use 'dynamodb_table' instead."
)

type Config map[string]any

func (cfg Config) GetTerraformInitArgs() Config {
	var filtered = make(Config)

	for key, val := range cfg {
		if slices.Contains(terragruntOnlyConfigs, key) {
			continue
		}

		filtered[key] = val
	}

	return filtered
}

func (cfg Config) IsEqual(comparableCfg Config, logger log.Logger) bool {
	// If other keys in config are bools, DeepEqual also will consider the maps to be different.
	for key, value := range comparableCfg {
		if util.KindOf(comparableCfg[key]) == reflect.String && util.KindOf(cfg[key]) == reflect.Bool {
			if convertedValue, err := strconv.ParseBool(value.(string)); err == nil {
				comparableCfg[key] = convertedValue
			}
		}
	}

	// Construct a new map excluding custom GCS labels that are only used in Terragrunt config and not in Terraform's backend
	newConfig := backend.Config{}

	for key, val := range cfg {
		if !slices.Contains(terragruntOnlyConfigs, key) {
			newConfig[key] = val
		}
	}

	return newConfig.IsEqual(backend.Config(comparableCfg), BackendName, logger)
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
