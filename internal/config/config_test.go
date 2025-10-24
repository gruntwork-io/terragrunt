package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test Helper Functions for Terragrunt Configuration Testing
//
// This file provides a comprehensive test framework for testing Terragrunt configuration
// loading and processing. It follows Go testing best practices for 2025, including:
// - Automatic cleanup with t.Cleanup()
// - Table-driven test patterns
// - Realistic mock data
// - Clear error messages

// createTempConfigFile creates a temporary .terragruntrc.json file for testing.
// The file is automatically cleaned up when the test completes.
//
// Parameters:
//   - t: The testing.T instance
//   - content: The JSON content to write to the config file
//
// Returns:
//   - The absolute path to the created config file
//
// Example usage:
//
//	configPath := createTempConfigFile(t, `{"log-level": "debug"}`)
//	// Use configPath in tests
//	// File is automatically deleted when test completes
func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()

	// Create a temporary directory for this test
	tmpDir := t.TempDir()

	// Create the config file path
	configPath := filepath.Join(tmpDir, ".terragruntrc.json")

	// Write the content to the file
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err, "Failed to create temp config file")

	return configPath
}

// createTempConfigFileInDir creates a .terragruntrc.json file in a specific directory.
// This is useful for testing configuration file discovery and precedence.
//
// Parameters:
//   - t: The testing.T instance
//   - dir: The directory where the config file should be created
//   - content: The JSON content to write to the config file
//
// Returns:
//   - The absolute path to the created config file
//
// Example usage:
//
//	tmpDir := t.TempDir()
//	configPath := createTempConfigFileInDir(t, tmpDir, `{"auto-approve": true}`)
func createTempConfigFileInDir(t *testing.T, dir string, content string) string {
	t.Helper()

	configPath := filepath.Join(dir, ".terragruntrc.json")
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err, "Failed to create temp config file in directory")

	return configPath
}

// createTestFlags returns a slice of mock CLI flags for testing.
// These flags mirror the types used in real Terragrunt CLI code.
//
// The returned flags include examples of:
//   - BoolFlag: Boolean flags (e.g., --auto-approve)
//   - GenericFlag[string]: String flags (e.g., --log-level)
//   - GenericFlag[int]: Integer flags (e.g., --parallelism)
//   - SliceFlag: Slice flags for multiple values (e.g., --var-file)
//
// Returns:
//   - A slice of cli.Flag instances representing common Terragrunt flags
//
// Example usage:
//
//	flags := createTestFlags()
//	// Use flags for testing configuration loading and flag mapping
func createTestFlags() []cli.Flag {
	// Create destination variables for flags (required for testing)
	var (
		autoApprove       bool
		logLevel          string
		parallelism       int
		terraformPath     string
		workingDir        string
		downloadDir       string
		debug             bool
		jsonLog           bool
		varFiles          []string
		terragruntConfig  string
		ignoreExternal    bool
		includeDirs       []string
		maxParallelism    int
	)

	return []cli.Flag{
		// Boolean flags
		&cli.BoolFlag{
			Name:        "auto-approve",
			Destination: &autoApprove,
			Usage:       "Automatically approve all actions",
			EnvVars:     []string{"TERRAGRUNT_AUTO_APPROVE"},
		},
		&cli.BoolFlag{
			Name:        "debug",
			Destination: &debug,
			Usage:       "Enable debug logging",
			EnvVars:     []string{"TERRAGRUNT_DEBUG"},
		},
		&cli.BoolFlag{
			Name:        "json-log",
			Destination: &jsonLog,
			Usage:       "Output logs in JSON format",
			EnvVars:     []string{"TERRAGRUNT_JSON_LOG"},
		},
		&cli.BoolFlag{
			Name:        "ignore-external-dependencies",
			Destination: &ignoreExternal,
			Usage:       "Ignore external dependencies",
			EnvVars:     []string{"TERRAGRUNT_IGNORE_EXTERNAL_DEPENDENCIES"},
		},

		// String flags (using GenericFlag[string])
		&cli.GenericFlag[string]{
			Name:        "log-level",
			Destination: &logLevel,
			Usage:       "Set the logging level (trace, debug, info, warn, error)",
			EnvVars:     []string{"TERRAGRUNT_LOG_LEVEL"},
		},
		&cli.GenericFlag[string]{
			Name:        "terraform-path",
			Destination: &terraformPath,
			Usage:       "Path to the Terraform binary",
			EnvVars:     []string{"TERRAGRUNT_TFPATH"},
		},
		&cli.GenericFlag[string]{
			Name:        "working-dir",
			Destination: &workingDir,
			Usage:       "The working directory for Terragrunt",
			EnvVars:     []string{"TERRAGRUNT_WORKING_DIR"},
		},
		&cli.GenericFlag[string]{
			Name:        "download-dir",
			Destination: &downloadDir,
			Usage:       "The directory to download Terraform code",
			EnvVars:     []string{"TERRAGRUNT_DOWNLOAD"},
		},
		&cli.GenericFlag[string]{
			Name:        "terragrunt-config",
			Destination: &terragruntConfig,
			Usage:       "Path to the Terragrunt config file",
			EnvVars:     []string{"TERRAGRUNT_CONFIG"},
		},

		// Integer flags (using GenericFlag[int])
		&cli.GenericFlag[int]{
			Name:        "parallelism",
			Destination: &parallelism,
			Usage:       "Number of concurrent operations",
			EnvVars:     []string{"TERRAGRUNT_PARALLELISM"},
		},
		&cli.GenericFlag[int]{
			Name:        "max-parallelism",
			Destination: &maxParallelism,
			Usage:       "Maximum number of parallel operations",
			EnvVars:     []string{"TERRAGRUNT_MAX_PARALLELISM"},
		},

		// Slice flags (for multiple values)
		&cli.SliceFlag[string]{
			Name:        "var-file",
			Destination: &varFiles,
			Usage:       "Terraform var file paths",
			EnvVars:     []string{"TERRAGRUNT_VAR_FILE"},
		},
		&cli.SliceFlag[string]{
			Name:        "terragrunt-include-dir",
			Destination: &includeDirs,
			Usage:       "Include specific directories",
			EnvVars:     []string{"TERRAGRUNT_INCLUDE_DIR"},
		},
	}
}

// assertConfigEqual performs a deep equality check between two TerragruntConfig instances.
// It provides clear, detailed error messages when configs don't match.
//
// This helper checks:
//   - SourceFile paths match
//   - Values maps have the same keys
//   - Values for each key match (with type-aware comparison)
//
// Parameters:
//   - t: The testing.T instance
//   - expected: The expected TerragruntConfig
//   - actual: The actual TerragruntConfig to compare
//
// Example usage:
//
//	expected := &TerragruntConfig{
//	    SourceFile: "/path/to/config.json",
//	    Values: map[string]interface{}{"log-level": "debug"},
//	}
//	assertConfigEqual(t, expected, actualConfig)
func assertConfigEqual(t *testing.T, expected, actual *TerragruntConfig) {
	t.Helper()

	// Check for nil cases
	if expected == nil && actual == nil {
		return
	}
	if expected == nil {
		t.Error("expected config is nil but actual is not")
		return
	}
	if actual == nil {
		t.Error("actual config is nil but expected is not")
		return
	}

	// Compare SourceFile
	assert.Equal(t, expected.SourceFile, actual.SourceFile,
		"SourceFile mismatch: expected %s, got %s", expected.SourceFile, actual.SourceFile)

	// Compare Values map
	if expected.Values == nil && actual.Values == nil {
		return
	}
	if expected.Values == nil {
		t.Error("expected Values map is nil but actual is not")
		return
	}
	if actual.Values == nil {
		t.Error("actual Values map is nil but expected is not")
		return
	}

	// Check if both maps have the same keys
	if len(expected.Values) != len(actual.Values) {
		t.Errorf("Values map length mismatch: expected %d keys, got %d keys",
			len(expected.Values), len(actual.Values))
	}

	// Compare each key-value pair
	for key, expectedValue := range expected.Values {
		actualValue, exists := actual.Values[key]
		if !exists {
			t.Errorf("Key %q missing in actual Values map", key)
			continue
		}

		if !reflect.DeepEqual(expectedValue, actualValue) {
			t.Errorf("Value mismatch for key %q: expected %v (%T), got %v (%T)",
				key, expectedValue, expectedValue, actualValue, actualValue)
		}
	}

	// Check for extra keys in actual
	for key := range actual.Values {
		if _, exists := expected.Values[key]; !exists {
			t.Errorf("Unexpected key %q in actual Values map", key)
		}
	}
}

// assertConfigContains checks if a config contains specific key-value pairs.
// This is useful for testing partial config matches without requiring exact equality.
//
// Parameters:
//   - t: The testing.T instance
//   - config: The TerragruntConfig to check
//   - expectedValues: Map of key-value pairs that should be present
//
// Example usage:
//
//	assertConfigContains(t, config, map[string]interface{}{
//	    "log-level": "debug",
//	    "auto-approve": true,
//	})
func assertConfigContains(t *testing.T, config *TerragruntConfig, expectedValues map[string]interface{}) {
	t.Helper()

	require.NotNil(t, config, "config should not be nil")
	require.NotNil(t, config.Values, "config.Values should not be nil")

	for key, expectedValue := range expectedValues {
		actualValue, exists := config.Values[key]
		assert.True(t, exists, "Key %q should exist in config", key)
		assert.Equal(t, expectedValue, actualValue,
			"Value mismatch for key %q: expected %v, got %v", key, expectedValue, actualValue)
	}
}

// Table-Driven Test Template and Examples
//
// This section demonstrates how to write table-driven tests using the helper functions above.
// Table-driven tests are the idiomatic way to test multiple scenarios in Go.

// TestTemplate_ConfigLoading demonstrates the table-driven test pattern for configuration testing.
// This is a template that shows best practices for structuring tests.
//
// To use this template:
// 1. Copy the test structure
// 2. Replace "TestTemplate_ConfigLoading" with your actual test name
// 3. Fill in test cases with your scenarios
// 4. Update the test logic to match what you're testing
//
// NOTE: This test is prefixed with "TestTemplate_" so it won't be executed by default.
// Remove the "Template_" prefix when you create real tests based on this pattern.
func TestTemplate_ConfigLoading(t *testing.T) {
	t.Skip("This is a template test - copy and adapt for actual tests")
	// Define test cases using a struct with named fields
	tests := []struct {
		name        string      // Descriptive name for the test case
		configJSON  string      // JSON content for the config file
		setupFunc   func(t *testing.T) string // Optional setup function
		want        *TerragruntConfig // Expected result
		wantErr     bool        // Whether an error is expected
		errContains string      // Expected error message substring
	}{
		{
			name: "valid config with string value",
			configJSON: `{
				"log-level": "debug"
			}`,
			want: &TerragruntConfig{
				Values: map[string]interface{}{
					"log-level": "debug",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with boolean value",
			configJSON: `{
				"auto-approve": true
			}`,
			want: &TerragruntConfig{
				Values: map[string]interface{}{
					"auto-approve": true,
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with multiple values",
			configJSON: `{
				"log-level": "info",
				"auto-approve": false,
				"parallelism": 10
			}`,
			want: &TerragruntConfig{
				Values: map[string]interface{}{
					"log-level":    "info",
					"auto-approve": false,
					"parallelism":  float64(10), // JSON numbers are float64
				},
			},
			wantErr: false,
		},
		{
			name:        "invalid JSON",
			configJSON:  `{"log-level": invalid}`,
			want:        nil,
			wantErr:     true,
			errContains: "invalid",
		},
		{
			name:       "empty config",
			configJSON: `{}`,
			want: &TerragruntConfig{
				Values: map[string]interface{}{},
			},
			wantErr: false,
		},
	}

	// Run test cases using subtests
	for _, tt := range tests {
		// t.Run creates a subtest with the given name
		// This makes failures easy to identify and allows running specific tests
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Create test config file
			var configPath string
			if tt.setupFunc != nil {
				configPath = tt.setupFunc(t)
			} else {
				configPath = createTempConfigFile(t, tt.configJSON)
			}

			// Execute: Load the config (this is where you'd call your actual function)
			// For this example, we'll create a config manually
			// In real tests, you'd call your config loading function here
			// Example: got, err := LoadConfig(configPath)

			// This is just a placeholder - replace with actual function call
			var got *TerragruntConfig
			var err error

			// Validate error expectation
			if tt.wantErr {
				require.Error(t, err, "Expected an error but got none")
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains,
						"Error message should contain %q", tt.errContains)
				}
				return
			}

			// Validate no error occurred
			require.NoError(t, err, "Unexpected error: %v", err)

			// Validate the result
			if tt.want != nil {
				require.NotNil(t, got, "Result should not be nil")
				// Update SourceFile in expected config to match actual
				tt.want.SourceFile = configPath
				assertConfigEqual(t, tt.want, got)
			}
		})
	}
}

// Template_ExampleBenchmark demonstrates how to write benchmark tests for configuration operations.
// Benchmarks help measure performance and identify optimization opportunities.
//
// Run benchmarks with: go test -bench=. -benchmem
func Template_ExampleBenchmark(b *testing.B) {
	// Setup: Create test data (not timed)
	configJSON := `{
		"log-level": "debug",
		"auto-approve": true,
		"parallelism": 10,
		"terraform-path": "/usr/bin/terraform"
	}`

	// For benchmarks, create a temp file outside the loop
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, ".terragruntrc.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	// Reset timer to exclude setup time
	b.ResetTimer()

	// Benchmark loop
	for i := 0; i < b.N; i++ {
		// Code to benchmark goes here
		// Example: config, err := LoadConfig(configPath)
		// Prevent compiler optimization by assigning to a variable
		// _ = config
		// _ = err
	}
}

// Template_ExampleParallelTest demonstrates how to run tests in parallel.
// Parallel tests can significantly speed up test execution.
func Template_ExampleParallelTest(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
	}{
		{"config1", `{"log-level": "debug"}`},
		{"config2", `{"auto-approve": true}`},
		{"config3", `{"parallelism": 5}`},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable for parallel test
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // Mark test as parallelizable

			configPath := createTempConfigFile(t, tt.configJSON)
			// Run test logic
			_ = configPath
		})
	}
}
