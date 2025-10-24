package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// LoadEnvVarOverrides reads TERRAGRUNT_* environment variables and converts them
// to their appropriate types based on the flag mappings.
//
// This function follows a best-effort approach:
// - It never returns an error, only logs warnings for invalid values
// - Type conversion errors are logged but don't stop processing
// - Uses os.LookupEnv to distinguish between unset and empty values
// - Returns a map of successfully converted environment variable values
//
// Parameters:
//   - flagMappings: map of flag names to their type information
//
// Returns:
//   - map[string]interface{}: successfully converted env var overrides
//   - error: always nil (best-effort processing)
func LoadEnvVarOverrides(flagMappings map[string]FlagMapping) (map[string]interface{}, error) {
	overrides := make(map[string]interface{})

	for flagName, mapping := range flagMappings {
		envVarName := mapping.EnvVarName

		// Use LookupEnv to distinguish between unset and empty values
		value, exists := os.LookupEnv(envVarName)
		if !exists {
			continue
		}

		// Handle empty string values - for boolean flags, empty means true
		if value == "" && mapping.Type == FlagTypeBool {
			overrides[flagName] = true
			log.Debugf("Env var %s (empty) interpreted as boolean true", envVarName)
			continue
		}

		// Skip empty values for non-boolean types
		if value == "" {
			log.Debugf("Skipping empty env var %s for non-boolean flag", envVarName)
			continue
		}

		// Convert string value to appropriate type
		converted, err := ConvertValue(value, mapping.Type)
		if err != nil {
			// Log warning but continue processing other env vars
			log.Warnf("Failed to convert environment variable %s=%q: %v", envVarName, value, err)
			continue
		}

		log.Debugf("Loaded env var override: %s=%v (type: %s)", flagName, converted, mapping.Type)
		overrides[flagName] = converted
	}

	log.Infof("Loaded %d environment variable overrides", len(overrides))
	// Never returns error as per specification
	return overrides, nil
}

// ConvertValue converts a string value to the specified flag type.
// This is the core type conversion function used for both env vars and config file values.
//
// Supported conversions:
//   - FlagTypeBool: parses "true"/"false" or "1"/"0" via strconv.ParseBool
//   - FlagTypeString: returns string as-is
//   - FlagTypeInt: parses integer via strconv.Atoi
//   - FlagTypeStringSlice: splits comma-separated values, trims whitespace
//
// Parameters:
//   - value: string value to convert (from env var or config file)
//   - flagType: target type for conversion
//
// Returns:
//   - interface{}: converted value in appropriate Go type
//   - error: conversion error with context, or nil on success
func ConvertValue(value interface{}, flagType FlagType) (interface{}, error) {
	// If value is already the correct type, return it
	switch v := value.(type) {
	case string:
		// String value needs conversion
		return convertStringValue(v, flagType)
	case bool:
		if flagType == FlagTypeBool {
			return v, nil
		}
		return convertStringValue(fmt.Sprint(v), flagType)
	case int, int64, float64:
		if flagType == FlagTypeInt {
			// Handle JSON numbers
			switch num := v.(type) {
			case int:
				return num, nil
			case int64:
				return int(num), nil
			case float64:
				return int(num), nil
			}
		}
		return convertStringValue(fmt.Sprint(v), flagType)
	case []interface{}:
		// JSON array for string slice
		if flagType == FlagTypeStringSlice {
			result := make([]string, 0, len(v))
			for _, item := range v {
				result = append(result, fmt.Sprint(item))
			}
			return result, nil
		}
		return nil, fmt.Errorf("cannot convert array to %s", flagType)
	default:
		// Try string conversion as fallback
		return convertStringValue(fmt.Sprint(v), flagType)
	}
}

// convertStringValue converts a string to the target flag type.
func convertStringValue(value string, flagType FlagType) (interface{}, error) {
	switch flagType {
	case FlagTypeString:
		return value, nil

	case FlagTypeBool:
		val, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("invalid boolean value %q: %w", value, err)
		}
		return val, nil

	case FlagTypeInt:
		val, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		return val, nil

	case FlagTypeStringSlice:
		// Split by comma and trim spaces
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result, nil

	default:
		return nil, fmt.Errorf("unsupported flag type: %s", flagType)
	}
}
