package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseConfigFile_ValidJSON tests parsing valid JSON configurations
func TestParseConfigFile_ValidJSON(t *testing.T) {
	tests := []struct {
		name           string
		fileContent    string
		expectedValues map[string]interface{}
	}{
		{
			name: "simple boolean flag",
			fileContent: `{
				"non-interactive": true
			}`,
			expectedValues: map[string]interface{}{
				"non-interactive": true,
			},
		},
		{
			name: "string flag",
			fileContent: `{
				"working-dir": "/path/to/dir"
			}`,
			expectedValues: map[string]interface{}{
				"working-dir": "/path/to/dir",
			},
		},
		{
			name: "number flag",
			fileContent: `{
				"parallelism": 10
			}`,
			expectedValues: map[string]interface{}{
				"parallelism": float64(10), // JSON numbers decode to float64
			},
		},
		{
			name: "array flag",
			fileContent: `{
				"include-dirs": ["dir1", "dir2", "dir3"]
			}`,
			expectedValues: map[string]interface{}{
				"include-dirs": []interface{}{"dir1", "dir2", "dir3"},
			},
		},
		{
			name: "multiple flags of different types",
			fileContent: `{
				"non-interactive": true,
				"working-dir": "/infrastructure",
				"parallelism": 5,
				"log-level": "debug"
			}`,
			expectedValues: map[string]interface{}{
				"non-interactive": true,
				"working-dir":     "/infrastructure",
				"parallelism":     float64(5),
				"log-level":       "debug",
			},
		},
		{
			name: "nested object values",
			fileContent: `{
				"some-config": {
					"nested": "value"
				}
			}`,
			expectedValues: map[string]interface{}{
				"some-config": map[string]interface{}{
					"nested": "value",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file with test content
			tmpfile := createTempConfigFile(t, tt.fileContent)
			defer os.Remove(tmpfile)

			// Parse config file
			config, err := ParseConfigFile(tmpfile)
			if err != nil {
				t.Fatalf("ParseConfigFile() unexpected error: %v", err)
			}

			// Verify source file path is absolute
			if !filepath.IsAbs(config.SourceFile) {
				t.Errorf("SourceFile should be absolute path, got: %s", config.SourceFile)
			}

			// Verify values
			if len(config.Values) != len(tt.expectedValues) {
				t.Errorf("expected %d values, got %d", len(tt.expectedValues), len(config.Values))
			}

			for key, expectedValue := range tt.expectedValues {
				actualValue, exists := config.Values[key]
				if !exists {
					t.Errorf("missing expected key: %s", key)
					continue
				}

				// Deep comparison for different types
				if !deepEqual(actualValue, expectedValue) {
					t.Errorf("key %s: expected %v (%T), got %v (%T)",
						key, expectedValue, expectedValue, actualValue, actualValue)
				}
			}
		})
	}
}

// TestParseConfigFile_EmptyFile tests that empty files are handled gracefully
func TestParseConfigFile_EmptyFile(t *testing.T) {
	// Create empty temp file
	tmpfile := createTempConfigFile(t, "")
	defer os.Remove(tmpfile)

	// Parse empty file
	config, err := ParseConfigFile(tmpfile)
	if err != nil {
		t.Fatalf("ParseConfigFile() should not error on empty file, got: %v", err)
	}

	// Verify empty config returned
	if config == nil {
		t.Fatal("ParseConfigFile() returned nil config for empty file")
	}

	if len(config.Values) != 0 {
		t.Errorf("expected empty Values map, got %d entries", len(config.Values))
	}

	// Verify source file is set
	if config.SourceFile == "" {
		t.Error("SourceFile should be set even for empty file")
	}
}

// TestParseConfigFile_EmptyJSON tests parsing empty JSON object
func TestParseConfigFile_EmptyJSON(t *testing.T) {
	tmpfile := createTempConfigFile(t, "{}")
	defer os.Remove(tmpfile)

	config, err := ParseConfigFile(tmpfile)
	if err != nil {
		t.Fatalf("ParseConfigFile() unexpected error: %v", err)
	}

	if len(config.Values) != 0 {
		t.Errorf("expected empty Values map, got %d entries", len(config.Values))
	}
}

// TestParseConfigFile_MalformedJSON tests error handling for invalid JSON
func TestParseConfigFile_MalformedJSON(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		errorContains string
	}{
		{
			name:          "missing comma",
			fileContent:   `{"non-interactive": true "working-dir": "/path"}`,
			errorContains: "syntax error",
		},
		{
			name:          "missing closing brace",
			fileContent:   `{"non-interactive": true`,
			errorContains: "unexpected",
		},
		{
			name:          "invalid key (unquoted)",
			fileContent:   `{non-interactive: true}`,
			errorContains: "syntax error",
		},
		{
			name:          "trailing comma",
			fileContent:   `{"non-interactive": true,}`,
			errorContains: "syntax error",
		},
		{
			name:          "single quote instead of double quote",
			fileContent:   `{'non-interactive': true}`,
			errorContains: "syntax error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpfile := createTempConfigFile(t, tt.fileContent)
			defer os.Remove(tmpfile)

			config, err := ParseConfigFile(tmpfile)
			if err == nil {
				t.Fatal("ParseConfigFile() should return error for malformed JSON")
			}

			if config != nil {
				t.Error("ParseConfigFile() should return nil config on error")
			}

			// Verify error is ConfigError
			configErr, ok := err.(*ConfigError)
			if !ok {
				t.Errorf("expected *ConfigError, got %T", err)
			}

			// Verify error contains expected substring
			errorMsg := err.Error()
			if !strings.Contains(strings.ToLower(errorMsg), strings.ToLower(tt.errorContains)) {
				t.Errorf("error message should contain %q, got: %s", tt.errorContains, errorMsg)
			}

			// Verify error includes file path
			if !strings.Contains(errorMsg, tmpfile) && !strings.Contains(errorMsg, "file=") {
				t.Errorf("error message should include file path, got: %s", errorMsg)
			}

			// Verify ConfigError has path set
			if configErr != nil && configErr.Path == "" {
				t.Error("ConfigError.Path should be set")
			}
		})
	}
}

// TestParseConfigFile_FileNotFound tests error handling when file doesn't exist
func TestParseConfigFile_FileNotFound(t *testing.T) {
	nonExistentPath := "/tmp/nonexistent-config-" + t.Name() + ".json"

	config, err := ParseConfigFile(nonExistentPath)
	if err == nil {
		t.Fatal("ParseConfigFile() should return error for non-existent file")
	}

	if config != nil {
		t.Error("ParseConfigFile() should return nil config on error")
	}

	// Verify error is ConfigError
	_, ok := err.(*ConfigError)
	if !ok {
		t.Errorf("expected *ConfigError, got %T", err)
	}

	// Verify error mentions the file
	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "open") && !strings.Contains(errorMsg, "failed") {
		t.Errorf("error should mention file open failure, got: %s", errorMsg)
	}
}

// TestParseConfigFile_TypePreservation tests that JSON types are preserved
func TestParseConfigFile_TypePreservation(t *testing.T) {
	fileContent := `{
		"bool-true": true,
		"bool-false": false,
		"string": "hello",
		"number-int": 42,
		"number-float": 3.14,
		"array": ["a", "b", "c"],
		"null": null
	}`

	tmpfile := createTempConfigFile(t, fileContent)
	defer os.Remove(tmpfile)

	config, err := ParseConfigFile(tmpfile)
	if err != nil {
		t.Fatalf("ParseConfigFile() unexpected error: %v", err)
	}

	// Verify types
	tests := []struct {
		key          string
		expectedType string
		expectedVal  interface{}
	}{
		{"bool-true", "bool", true},
		{"bool-false", "bool", false},
		{"string", "string", "hello"},
		{"number-int", "float64", float64(42)}, // JSON numbers are float64
		{"number-float", "float64", float64(3.14)},
		{"array", "[]interface {}", []interface{}{"a", "b", "c"}},
		{"null", "nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			val, exists := config.Values[tt.key]
			if !exists {
				t.Fatalf("key %s not found in config", tt.key)
			}

			// Check type
			actualType := getTypeName(val)
			if actualType != tt.expectedType {
				t.Errorf("key %s: expected type %s, got %s", tt.key, tt.expectedType, actualType)
			}

			// Check value
			if !deepEqual(val, tt.expectedVal) {
				t.Errorf("key %s: expected value %v, got %v", tt.key, tt.expectedVal, val)
			}
		})
	}
}

// TestValidateFlagNames_AllKnown tests validation when all flags are known
func TestValidateFlagNames_AllKnown(t *testing.T) {
	config := &TerragruntConfig{
		SourceFile: "/path/to/.terragruntrc.json",
		Values: map[string]interface{}{
			"non-interactive": true,
			"working-dir":     "/path",
			"log-level":       "debug",
		},
	}

	knownFlags := map[string]bool{
		"non-interactive": true,
		"working-dir":     true,
		"log-level":       true,
		"parallelism":     true, // Extra known flag not in config (OK)
	}

	warnings := ValidateFlagNames(config, knownFlags)

	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(warnings), warnings)
	}
}

// TestValidateFlagNames_UnknownFlags tests validation with unknown flags
func TestValidateFlagNames_UnknownFlags(t *testing.T) {
	config := &TerragruntConfig{
		SourceFile: "/path/to/.terragruntrc.json",
		Values: map[string]interface{}{
			"non-interactive": true,
			"unknown-flag-1":  "value",
			"working-dir":     "/path",
			"unknown-flag-2":  42,
		},
	}

	knownFlags := map[string]bool{
		"non-interactive": true,
		"working-dir":     true,
	}

	warnings := ValidateFlagNames(config, knownFlags)

	// Should have warnings for 2 unknown flags
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %v", len(warnings), warnings)
	}

	// Verify warnings mention the unknown flags
	warningsStr := strings.Join(warnings, " ")
	if !strings.Contains(warningsStr, "unknown-flag-1") {
		t.Error("warnings should mention unknown-flag-1")
	}
	if !strings.Contains(warningsStr, "unknown-flag-2") {
		t.Error("warnings should mention unknown-flag-2")
	}

	// Verify warnings mention forward compatibility
	if !strings.Contains(warningsStr, "forward compatibility") {
		t.Error("warnings should mention forward compatibility")
	}
}

// TestValidateFlagNames_EmptyConfig tests validation with empty config
func TestValidateFlagNames_EmptyConfig(t *testing.T) {
	config := &TerragruntConfig{
		SourceFile: "/path/to/.terragruntrc.json",
		Values:     map[string]interface{}{},
	}

	knownFlags := map[string]bool{
		"non-interactive": true,
	}

	warnings := ValidateFlagNames(config, knownFlags)

	if len(warnings) != 0 {
		t.Errorf("expected no warnings for empty config, got %d: %v", len(warnings), warnings)
	}
}

// TestValidateFlagNames_NoKnownFlags tests validation with no known flags
func TestValidateFlagNames_NoKnownFlags(t *testing.T) {
	config := &TerragruntConfig{
		SourceFile: "/path/to/.terragruntrc.json",
		Values: map[string]interface{}{
			"some-flag": true,
		},
	}

	knownFlags := map[string]bool{} // Empty known flags

	warnings := ValidateFlagNames(config, knownFlags)

	// Should have warning for the unknown flag
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}

	if !strings.Contains(warnings[0], "some-flag") {
		t.Error("warning should mention the unknown flag name")
	}
}

// Helper functions
// Note: createTempConfigFile is defined in config_test.go and is shared across all test files

// deepEqual compares two values deeply, handling different types
func deepEqual(a, b interface{}) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle slices
	aSlice, aIsSlice := a.([]interface{})
	bSlice, bIsSlice := b.([]interface{})
	if aIsSlice && bIsSlice {
		if len(aSlice) != len(bSlice) {
			return false
		}
		for i := range aSlice {
			if !deepEqual(aSlice[i], bSlice[i]) {
				return false
			}
		}
		return true
	}

	// Handle maps
	aMap, aIsMap := a.(map[string]interface{})
	bMap, bIsMap := b.(map[string]interface{})
	if aIsMap && bIsMap {
		if len(aMap) != len(bMap) {
			return false
		}
		for key, aVal := range aMap {
			bVal, exists := bMap[key]
			if !exists {
				return false
			}
			if !deepEqual(aVal, bVal) {
				return false
			}
		}
		return true
	}

	// For primitive types, use direct comparison
	return a == b
}

// getTypeName returns a string representation of the type
func getTypeName(v interface{}) string {
	if v == nil {
		return "nil"
	}

	switch v.(type) {
	case bool:
		return "bool"
	case string:
		return "string"
	case float64:
		return "float64"
	case int:
		return "int"
	case []interface{}:
		return "[]interface {}"
	case map[string]interface{}:
		return "map[string]interface {}"
	default:
		return "unknown"
	}
}
