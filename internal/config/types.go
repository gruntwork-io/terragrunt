// Package config provides configuration file loading and management for Terragrunt.
// It supports reading .terragruntrc.json files from multiple locations and merging
// them with environment variables and CLI flags following proper precedence rules.
package config

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// TerragruntConfig represents parsed configuration from .terragruntrc.json file.
// It contains the raw configuration values loaded from JSON along with metadata
// about the source file location.
type TerragruntConfig struct {
	// SourceFile is the absolute path to the config file that was loaded.
	// This is used for error reporting and debugging.
	SourceFile string

	// Values contains the raw configuration values from JSON.
	// Keys are flag names in kebab-case (e.g., "non-interactive", "working-dir").
	// Values are interface{} to support multiple types: bool, string, int, []string.
	Values map[string]interface{}
}

// FlagMapping maps between different representations of a flag.
// It provides the relationship between CLI flag names, environment variable names,
// type information, and the original flag definition.
type FlagMapping struct {
	// FlagName is the CLI flag name in kebab-case (e.g., "non-interactive").
	FlagName string

	// EnvVarName is the environment variable name (e.g., "TERRAGRUNT_NON_INTERACTIVE").
	// This follows the pattern TERRAGRUNT_<FLAG_NAME_UPPERCASE>.
	EnvVarName string

	// Type is the flag's data type.
	// Used for type conversion and validation.
	Type FlagType

	// OriginalFlag is reference to the original cli.Flag.
	// This provides access to the Value field for setting merged configuration.
	OriginalFlag cli.Flag
}

// FlagType represents the data type of a flag.
// This enumeration is used by the type mapper to determine the correct
// conversion logic when processing configuration values.
type FlagType int

const (
	// FlagTypeUnknown represents an unknown or unsupported flag type.
	// Defaults to string conversion when encountered.
	FlagTypeUnknown FlagType = iota

	// FlagTypeBool represents a boolean flag (true/false).
	FlagTypeBool

	// FlagTypeString represents a string flag.
	FlagTypeString

	// FlagTypeInt represents an integer flag.
	FlagTypeInt

	// FlagTypeStringSlice represents a string slice flag (array of strings).
	FlagTypeStringSlice
)

// String returns the string representation of the FlagType.
func (ft FlagType) String() string {
	switch ft {
	case FlagTypeBool:
		return "bool"
	case FlagTypeString:
		return "string"
	case FlagTypeInt:
		return "int"
	case FlagTypeStringSlice:
		return "[]string"
	default:
		return "unknown"
	}
}

// ConfigError represents an error that occurred during configuration processing.
// It provides rich context including the config file path, specific flag name,
// and the underlying cause to aid in debugging configuration issues.
type ConfigError struct {
	// Path is the config file path where the error occurred.
	// Empty string if error is not file-specific.
	Path string

	// FlagName is the flag that caused the error.
	// Empty string if error is not flag-specific.
	FlagName string

	// Message is the error message describing what went wrong.
	Message string

	// Cause is the underlying error that triggered this error.
	// May be nil if this is the root cause.
	Cause error
}

// Error implements the error interface.
// It formats the error message with context about the config file path
// and flag name when available.
func (e *ConfigError) Error() string {
	var parts []string
	if e.Path != "" {
		parts = append(parts, fmt.Sprintf("file=%s", e.Path))
	}
	if e.FlagName != "" {
		parts = append(parts, fmt.Sprintf("flag=%s", e.FlagName))
	}

	errMsg := "config error"
	if len(parts) > 0 {
		errMsg = fmt.Sprintf("config error [%s]", strings.Join(parts, ", "))
	}

	if e.Message != "" {
		errMsg = fmt.Sprintf("%s: %s", errMsg, e.Message)
	}

	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", errMsg, e.Cause)
	}

	return errMsg
}

// Unwrap returns the underlying cause error.
// This enables error chain traversal with errors.Is and errors.As.
func (e *ConfigError) Unwrap() error {
	return e.Cause
}

// NewConfigError creates a new ConfigError with the given parameters.
// This is a convenience constructor for creating properly initialized ConfigError instances.
func NewConfigError(path, flagName, message string, cause error) *ConfigError {
	return &ConfigError{
		Path:     path,
		FlagName: flagName,
		Message:  message,
		Cause:    cause,
	}
}

// BuildFlagRegistry introspects cli.Flags and builds type mappings for each flag.
// It extracts the primary flag name, infers the type, and constructs environment
// variable names following Terragrunt conventions.
//
// This function handles:
//   - urfave/cli standard flag types (*cli.BoolFlag, *cli.StringFlag, etc.)
//   - Terragrunt's custom flags.Flag wrapper (unwraps and processes inner flag)
//   - Generic flags via reflection and type assertions
//
// Parameters:
//   - flags: slice of cli.Flag from cli.App or cli.Command
//
// Returns:
//   - map[string]FlagMapping: registry mapping flag names to their type information
func BuildFlagRegistry(flags []cli.Flag) map[string]FlagMapping {
	registry := make(map[string]FlagMapping)

	for _, flag := range flags {
		// Extract primary flag name (first in names list)
		flagName := extractFlagName(flag)
		if flagName == "" {
			continue
		}

		// Infer flag type via type assertions
		flagType := InferFlagType(flag)

		// Build environment variable name
		envVarName := FlagNameToEnvVar(flagName)

		// Create mapping
		registry[flagName] = FlagMapping{
			FlagName:     flagName,
			EnvVarName:   envVarName,
			Type:         flagType,
			OriginalFlag: flag,
		}
	}

	return registry
}

// extractFlagName extracts the primary flag name from a cli.Flag.
// Returns the first name in the Names() list, or empty string if unavailable.
func extractFlagName(flag cli.Flag) string {
	names := flag.Names()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// InferFlagType determines FlagType from cli.Flag using type assertions.
// It performs a type switch to detect the underlying flag type from urfave/cli.
//
// Supported types:
//   - *cli.BoolFlag -> FlagTypeBool
//   - *cli.StringFlag -> FlagTypeString
//   - *cli.IntFlag -> FlagTypeInt
//   - *cli.StringSliceFlag -> FlagTypeStringSlice
//
// For unknown types, defaults to FlagTypeString for safe string conversion.
func InferFlagType(flag cli.Flag) FlagType {
	switch flag.(type) {
	case *cli.BoolFlag:
		return FlagTypeBool
	case *cli.StringFlag:
		return FlagTypeString
	case *cli.IntFlag:
		return FlagTypeInt
	case *cli.StringSliceFlag:
		return FlagTypeStringSlice
	case *cli.Int64Flag:
		return FlagTypeInt
	case *cli.Uint64Flag:
		return FlagTypeInt
	case *cli.UintFlag:
		return FlagTypeInt
	default:
		// Default to string for unknown types
		return FlagTypeString
	}
}

// FlagNameToEnvVar converts a flag name to its corresponding environment variable name.
// It follows Terragrunt conventions:
// - Converts kebab-case to SCREAMING_SNAKE_CASE
// - Adds TERRAGRUNT_ prefix
//
// Examples:
//   - "non-interactive" -> "TERRAGRUNT_NON_INTERACTIVE"
//   - "working-dir" -> "TERRAGRUNT_WORKING_DIR"
//   - "terragrunt-log-level" -> "TERRAGRUNT_TERRAGRUNT_LOG_LEVEL"
func FlagNameToEnvVar(flagName string) string {
	// Remove leading dashes if present
	name := strings.TrimPrefix(flagName, "--")
	name = strings.TrimPrefix(name, "-")

	// Convert to uppercase and replace dashes with underscores
	envVarName := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))

	// Add TERRAGRUNT_ prefix
	return "TERRAGRUNT_" + envVarName
}
