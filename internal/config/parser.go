package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ParseConfigFile parses a .terragruntrc.json file and returns the configuration.
// It reads the file at the given path, parses the JSON content, and returns a
// TerragruntConfig with the absolute path and parsed values.
//
// The function handles several edge cases:
// - Empty files return an empty TerragruntConfig (not an error)
// - JSON syntax errors are wrapped in ConfigError with line/column information
// - Type information from JSON is preserved (bool, string, number, array)
//
// Parameters:
//   - path: absolute or relative path to the .terragruntrc.json file
//
// Returns:
//   - *TerragruntConfig: parsed configuration with source file path and values
//   - error: ConfigError for parse/read failures, nil on success
//
// Example usage:
//
//	config, err := ParseConfigFile("/path/to/.terragruntrc.json")
//	if err != nil {
//	    return fmt.Errorf("failed to load config: %w", err)
//	}
func ParseConfigFile(path string) (*TerragruntConfig, error) {
	// Convert to absolute path for consistent error reporting
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, NewConfigError(path, "", "failed to resolve absolute path", err)
	}

	// Open the config file
	file, err := os.Open(absPath)
	if err != nil {
		return nil, NewConfigError(absPath, "", "failed to open config file", err)
	}
	defer file.Close()

	// Check for empty file
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, NewConfigError(absPath, "", "failed to stat config file", err)
	}

	// Empty file is valid - return empty config
	if fileInfo.Size() == 0 {
		return &TerragruntConfig{
			SourceFile: absPath,
			Values:     make(map[string]interface{}),
		}, nil
	}

	// Use json.Decoder for file-based parsing (more efficient than reading entire file)
	var values map[string]interface{}
	decoder := json.NewDecoder(file)

	// Decode JSON into map
	if err := decoder.Decode(&values); err != nil {
		return nil, formatJSONError(err, absPath)
	}

	// Create and return TerragruntConfig
	config := &TerragruntConfig{
		SourceFile: absPath,
		Values:     values,
	}

	return config, nil
}

// ValidateFlagNames checks flag names in the configuration against known Terragrunt flags.
// It returns a list of warning messages for unknown flags but does NOT error.
// This approach supports forward compatibility - config files created for newer Terragrunt
// versions can work with older versions (unknown flags are simply ignored with warnings).
//
// Parameters:
//   - config: parsed TerragruntConfig to validate
//   - knownFlags: map of known flag names (flag name -> true)
//
// Returns:
//   - []string: list of warning messages for unknown flags, empty slice if all flags known
//
// Example usage:
//
//	knownFlags := map[string]bool{
//	    "non-interactive": true,
//	    "working-dir": true,
//	}
//	warnings := ValidateFlagNames(config, knownFlags)
//	for _, warning := range warnings {
//	    log.Warn(warning)
//	}
func ValidateFlagNames(config *TerragruntConfig, knownFlags map[string]bool) []string {
	var warnings []string

	// Check each config key against known flags
	for flagName := range config.Values {
		if !knownFlags[flagName] {
			warning := fmt.Sprintf(
				"unknown flag '%s' in config file %s (will be ignored for forward compatibility)",
				flagName,
				config.SourceFile,
			)
			warnings = append(warnings, warning)
		}
	}

	return warnings
}

// formatJSONError formats JSON parsing errors with helpful context including
// file path, line/column numbers, and clear error messages.
//
// It handles specific JSON error types:
// - json.SyntaxError: malformed JSON with byte offset
// - json.UnmarshalTypeError: type mismatch with field and type info
// - io.EOF: unexpected end of JSON
//
// Parameters:
//   - err: error from json.Decoder.Decode()
//   - filePath: absolute path to config file for error context
//
// Returns:
//   - error: ConfigError with formatted message and context
func formatJSONError(err error, filePath string) error {
	// Handle json.SyntaxError (malformed JSON)
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return NewConfigError(
			filePath,
			"",
			fmt.Sprintf("JSON syntax error at byte offset %d: %v", syntaxErr.Offset, syntaxErr.Error()),
			syntaxErr,
		)
	}

	// Handle json.UnmarshalTypeError (type mismatch)
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return NewConfigError(
			filePath,
			typeErr.Field,
			fmt.Sprintf("JSON type error at field '%s': expected %s but got %s", typeErr.Field, typeErr.Type, typeErr.Value),
			typeErr,
		)
	}

	// Handle io.EOF (unexpected end of JSON)
	if errors.Is(err, io.EOF) {
		return NewConfigError(
			filePath,
			"",
			"unexpected end of JSON input (file may be incomplete)",
			err,
		)
	}

	// Handle other errors
	return NewConfigError(
		filePath,
		"",
		fmt.Sprintf("failed to parse JSON: %v", err),
		err,
	)
}
