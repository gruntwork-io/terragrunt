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

	"maps"

	"github.com/gruntwork-io/terragrunt/util"
)

const (
	configLockTableKey                 = "lock_table"
	configDynamoDBTableKey             = "dynamodb_table"
	configEncryptKey                   = "encrypt"
	configKeyKey                       = "key"
	configAssumeRoleKey                = "assume_role"
	configAssumeRoleWithWebIdentityKey = "assume_role_with_web_identity"
	configAccessloggingTargetPrefixKey = "accesslogging_target_prefix"

	DefaultS3BucketAccessLoggingTargetPrefix = "TFStateLogs/"

	lockTableDeprecationMessage = "Remote state configuration 'lock_table' attribute is deprecated; use 'dynamodb_table' instead."
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

func (cfg Config) GetTFInitArgs() Config {
	var filtered = make(Config)

	for key, val := range cfg.FilterOutTerragruntKeys() {
		// Remove the deprecated "lock_table" attribute so that it
		// will not be passed either when generating a backend block
		// or as a command-line argument.
		if key == configLockTableKey {
			filtered[configDynamoDBTableKey] = val
			continue
		}

		if key == configAssumeRoleKey {
			if mapVal, ok := val.(map[string]any); ok {
				filtered[key] = hclhelper.WrapMapToSingleLineHcl(mapVal)

				continue
			}
		}

		if key == configAssumeRoleWithWebIdentityKey {
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

	maps.Copy(normalized, cfg)

	// Nowadays it only makes sense to set the "dynamodb_table" attribute as it has
	// been supported in Terraform since the release of version 0.10. The deprecated
	// "lock_table" attribute is either set to NULL in the state file or missing
	// from it altogether. Display a deprecation warning when the "lock_table"
	// attribute is being used.
	if util.KindOf(normalized[configLockTableKey]) == reflect.String && normalized[configLockTableKey] != "" {
		logger.Warnf("%s\n", lockTableDeprecationMessage)

		normalized[configDynamoDBTableKey] = normalized[configLockTableKey]
		delete(normalized, configLockTableKey)
	}

	return normalized
}
func (cfg Config) IsEqual(targetCfg Config, logger log.Logger) bool {
	// Terraform's `backend` configuration uses a boolean for the `encrypt` parameter. However, perhaps for backwards compatibility reasons,
	// Terraform stores that parameter as a string in the `terraform.tfstate` file. Therefore, we have to convert it accordingly, or `DeepEqual`
	// will fail.
	if util.KindOf(targetCfg[configEncryptKey]) == reflect.String && util.KindOf(cfg[configEncryptKey]) == reflect.Bool {
		// If encrypt in remoteState is a bool and a string in existingBackend, DeepEqual will consider the maps to be different.
		// So we convert the value from string to bool to make them equivalent.
		if value, err := strconv.ParseBool(targetCfg[configEncryptKey].(string)); err == nil {
			targetCfg[configEncryptKey] = value
		} else {
			logger.Warnf("Remote state configuration encrypt contains invalid value %v, should be boolean.", targetCfg["encrypt"])
		}
	}

	// If other keys in config are bools, DeepEqual also will consider the maps to be different.
	for key, value := range targetCfg {
		if util.KindOf(targetCfg[key]) == reflect.String && util.KindOf(cfg[key]) == reflect.Bool {
			if convertedValue, err := strconv.ParseBool(value.(string)); err == nil {
				targetCfg[key] = convertedValue
			}
		}
	}

	// We now construct a version of the config that matches what we expect in the backend by stripping out terragrunt
	// related configs.
	newConfig := backend.Config{}

	maps.Copy(newConfig, cfg.FilterOutTerragruntKeys())

	return newConfig.IsEqual(backend.Config(targetCfg), BackendName, logger)
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

	_, targetPrefixExists := cfg[configAccessloggingTargetPrefixKey]
	if !targetPrefixExists {
		extendedConfig.AccessLoggingTargetPrefix = DefaultS3BucketAccessLoggingTargetPrefix
	}

	extendedConfig.RemoteStateConfigS3 = s3Config

	return &extendedConfig, nil
}

// ExtendedS3Config parses the given map into an extended S3 config and validates this config.
func (cfg Config) ExtendedS3Config(logger log.Logger) (*ExtendedRemoteStateConfigS3, error) {
	extS3Cfg, err := cfg.Normalize(logger).ParseExtendedS3Config()
	if err != nil {
		return nil, err
	}

	return extS3Cfg, extS3Cfg.Validate()
}
