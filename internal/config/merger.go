// Package config provides configuration merging functionality for Terragrunt.
package config

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// MergeConfigurations merges configuration from file and environment variables with precedence.
// Precedence order: environment variables > config file
//
// The merge process:
// 1. Start with config file values as base (if configFile is not nil)
// 2. Apply environment variable overrides on top (overwrites config file values)
// 3. Return merged map with all configuration values
//
// Parameters:
//   - configFile: TerragruntConfig parsed from configuration file (can be nil)
//   - envOverrides: Map of configuration values from environment variables
//
// Returns:
//   - Merged map[string]interface{} with all configuration values
//
// Note: This function never returns an error as inputs are assumed to be validated.
func MergeConfigurations(configFile *TerragruntConfig, envOverrides map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Start with config file values as base (if present)
	if configFile != nil && configFile.Values != nil {
		for key, value := range configFile.Values {
			merged[key] = value
			log.Debugf("Config file value: %s=%v", key, value)
		}
		log.Infof("Loaded %d values from config file: %s", len(configFile.Values), configFile.SourceFile)
	}

	// Apply environment variable overrides (overwrites config file values)
	// Environment variables have higher precedence than config file
	for key, value := range envOverrides {
		if _, exists := merged[key]; exists {
			log.Debugf("Environment variable overrides config file value for: %s", key)
		}
		merged[key] = value
	}

	log.Infof("Merged configuration contains %d values total", len(merged))
	return merged
}
