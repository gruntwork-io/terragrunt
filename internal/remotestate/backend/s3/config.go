package s3

import (
	"reflect"
	"slices"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/hclhelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mitchellh/mapstructure"

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
		// Remove attributes that are specific to Terragrunt as
		// Terraform would fail with an error while trying to
		// consume these attributes.
		if slices.Contains(terragruntOnlyConfigs, key) {
			continue
		}

		// Remove the deprecated "lock_table" attribute so that it
		// will not be passed either when generating a backend block
		// or as a command-line argument.
		if key == lockTableKey {
			filtered[dynamoDBTableKey] = val
			continue
		}

		if key == assumeRoleKey {
			if mapVal, ok := val.(map[string]any); ok {
				filtered[key] = hclhelper.WrapMapToSingleLineHcl(mapVal)

				continue
			}
		}

		filtered[key] = val
	}

	return filtered
}

func (cfg Config) Normalize(logger log.Logger) Config {
	var normalized = make(Config)

	for key, val := range cfg {
		normalized[key] = val
	}

	// Nowadays it only makes sense to set the "dynamodb_table" attribute as it has
	// been supported in Terraform since the release of version 0.10. The deprecated
	// "lock_table" attribute is either set to NULL in the state file or missing
	// from it altogether. Display a deprecation warning when the "lock_table"
	// attribute is being used.
	if util.KindOf(normalized[lockTableKey]) == reflect.String && normalized[lockTableKey] != "" {
		logger.Warnf("%s\n", lockTableDeprecationMessage)

		normalized[dynamoDBTableKey] = normalized[lockTableKey]
		delete(normalized, lockTableKey)
	}

	return normalized
}
func (cfg Config) IsEqual(comparableCfg Config, logger log.Logger) bool {
	// Terraform's `backend` configuration uses a boolean for the `encrypt` parameter. However, perhaps for backwards compatibility reasons,
	// Terraform stores that parameter as a string in the `terraform.tfstate` file. Therefore, we have to convert it accordingly, or `DeepEqual`
	// will fail.
	if util.KindOf(comparableCfg[encryptKey]) == reflect.String && util.KindOf(cfg[encryptKey]) == reflect.Bool {
		// If encrypt in remoteState is a bool and a string in existingBackend, DeepEqual will consider the maps to be different.
		// So we convert the value from string to bool to make them equivalent.
		if value, err := strconv.ParseBool(comparableCfg[encryptKey].(string)); err == nil {
			comparableCfg[encryptKey] = value
		} else {
			logger.Warnf("Remote state configuration encrypt contains invalid value %v, should be boolean.", comparableCfg["encrypt"])
		}
	}

	// If other keys in config are bools, DeepEqual also will consider the maps to be different.
	for key, value := range comparableCfg {
		if util.KindOf(comparableCfg[key]) == reflect.String && util.KindOf(cfg[key]) == reflect.Bool {
			if convertedValue, err := strconv.ParseBool(value.(string)); err == nil {
				comparableCfg[key] = convertedValue
			}
		}
	}

	// We now construct a version of the config that matches what we expect in the backend by stripping out terragrunt
	// related configs.
	newConfig := backend.Config{}

	for key, val := range cfg {
		if !slices.Contains(terragruntOnlyConfigs, key) {
			newConfig[key] = val
		}
	}

	return newConfig.IsEqual(backend.Config(comparableCfg), BackendName, logger)
}

// ParseExtendedS3Config parses the given map into an extended S3 config.
func (cfg Config) ParseExtendedS3Config() (*ExtendedRemoteStateConfigS3, error) {
	var (
		s3Config       RemoteStateConfigS3
		extendedConfig ExtendedRemoteStateConfigS3
	)

	if err := mapstructure.Decode(cfg, &s3Config); err != nil {
		return nil, errors.New(err)
	}

	if err := mapstructure.Decode(cfg, &extendedConfig); err != nil {
		return nil, errors.New(err)
	}

	_, targetPrefixExists := cfg[accessloggingTargetPrefixKey]
	if !targetPrefixExists {
		extendedConfig.AccessLoggingTargetPrefix = DefaultS3BucketAccessLoggingTargetPrefix
	}

	extendedConfig.RemoteStateConfigS3 = s3Config

	return &extendedConfig, nil
}
