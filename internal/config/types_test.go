package config_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlagTypeString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		flagType config.FlagType
		expected string
	}{
		{
			name:     "bool type returns 'bool'",
			flagType: config.FlagTypeBool,
			expected: "bool",
		},
		{
			name:     "string type returns 'string'",
			flagType: config.FlagTypeString,
			expected: "string",
		},
		{
			name:     "int type returns 'int'",
			flagType: config.FlagTypeInt,
			expected: "int",
		},
		{
			name:     "string slice type returns '[]string'",
			flagType: config.FlagTypeStringSlice,
			expected: "[]string",
		},
		{
			name:     "unknown type returns 'unknown'",
			flagType: config.FlagTypeUnknown,
			expected: "unknown",
		},
		{
			name:     "invalid type value returns 'unknown'",
			flagType: config.FlagType(999),
			expected: "unknown",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := tc.flagType.String()
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestConfigErrorError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		configError   *config.ConfigError
		expectedSubstr []string
	}{
		{
			name: "error with all fields populated",
			configError: &config.ConfigError{
				Path:     "/home/user/.terragruntrc.json",
				FlagName: "working-dir",
				Message:  "invalid path",
				Cause:    errors.New("path does not exist"),
			},
			expectedSubstr: []string{
				"config error",
				"/home/user/.terragruntrc.json",
				"working-dir",
				"invalid path",
				"path does not exist",
			},
		},
		{
			name: "error with path and message only",
			configError: &config.ConfigError{
				Path:    "/home/user/.terragruntrc.json",
				Message: "malformed JSON",
			},
			expectedSubstr: []string{
				"config error",
				"/home/user/.terragruntrc.json",
				"malformed JSON",
			},
		},
		{
			name: "error with flag name and message only",
			configError: &config.ConfigError{
				FlagName: "parallelism",
				Message:  "value must be positive integer",
			},
			expectedSubstr: []string{
				"config error",
				"parallelism",
				"value must be positive integer",
			},
		},
		{
			name: "error with message only",
			configError: &config.ConfigError{
				Message: "configuration file not found",
			},
			expectedSubstr: []string{
				"config error",
				"configuration file not found",
			},
		},
		{
			name: "error with cause only",
			configError: &config.ConfigError{
				Cause: errors.New("underlying error"),
			},
			expectedSubstr: []string{
				"config error",
				"underlying error",
			},
		},
		{
			name:        "empty error",
			configError: &config.ConfigError{},
			expectedSubstr: []string{
				"config error",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := tc.configError.Error()

			for _, substr := range tc.expectedSubstr {
				assert.Contains(t, actual, substr,
					"error message should contain '%s', got: %s", substr, actual)
			}
		})
	}
}

func TestConfigErrorUnwrap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		configError *config.ConfigError
		expected    error
	}{
		{
			name: "unwrap returns cause when present",
			configError: &config.ConfigError{
				Message: "outer error",
				Cause:   errors.New("inner error"),
			},
			expected: errors.New("inner error"),
		},
		{
			name: "unwrap returns nil when no cause",
			configError: &config.ConfigError{
				Message: "error without cause",
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := tc.configError.Unwrap()

			if tc.expected == nil {
				assert.Nil(t, actual)
			} else {
				require.NotNil(t, actual)
				assert.Equal(t, tc.expected.Error(), actual.Error())
			}
		})
	}
}

func TestNewConfigError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		path         string
		flagName     string
		message      string
		cause        error
		validateFunc func(t *testing.T, err *config.ConfigError)
	}{
		{
			name:     "creates error with all parameters",
			path:     "/path/to/config.json",
			flagName: "test-flag",
			message:  "test message",
			cause:    errors.New("test cause"),
			validateFunc: func(t *testing.T, err *config.ConfigError) {
				assert.Equal(t, "/path/to/config.json", err.Path)
				assert.Equal(t, "test-flag", err.FlagName)
				assert.Equal(t, "test message", err.Message)
				assert.NotNil(t, err.Cause)
				assert.Equal(t, "test cause", err.Cause.Error())
			},
		},
		{
			name:     "creates error with empty strings",
			path:     "",
			flagName: "",
			message:  "test message",
			cause:    nil,
			validateFunc: func(t *testing.T, err *config.ConfigError) {
				assert.Empty(t, err.Path)
				assert.Empty(t, err.FlagName)
				assert.Equal(t, "test message", err.Message)
				assert.Nil(t, err.Cause)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := config.NewConfigError(tc.path, tc.flagName, tc.message, tc.cause)

			require.NotNil(t, actual)
			tc.validateFunc(t, actual)
		})
	}
}

func TestTerragruntConfigStructure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		createFunc func() *config.TerragruntConfig
		validate   func(t *testing.T, cfg *config.TerragruntConfig)
	}{
		{
			name: "empty config initializes correctly",
			createFunc: func() *config.TerragruntConfig {
				return &config.TerragruntConfig{
					SourceFile: "",
					Values:     make(map[string]interface{}),
				}
			},
			validate: func(t *testing.T, cfg *config.TerragruntConfig) {
				assert.Empty(t, cfg.SourceFile)
				assert.NotNil(t, cfg.Values)
				assert.Empty(t, cfg.Values)
			},
		},
		{
			name: "config with values",
			createFunc: func() *config.TerragruntConfig {
				return &config.TerragruntConfig{
					SourceFile: "/home/user/.terragruntrc.json",
					Values: map[string]interface{}{
						"non-interactive": true,
						"working-dir":     "/path/to/dir",
						"parallelism":     10,
					},
				}
			},
			validate: func(t *testing.T, cfg *config.TerragruntConfig) {
				assert.Equal(t, "/home/user/.terragruntrc.json", cfg.SourceFile)
				assert.Len(t, cfg.Values, 3)
				assert.Equal(t, true, cfg.Values["non-interactive"])
				assert.Equal(t, "/path/to/dir", cfg.Values["working-dir"])
				assert.Equal(t, 10, cfg.Values["parallelism"])
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := tc.createFunc()
			tc.validate(t, cfg)
		})
	}
}

func TestFlagMappingStructure(t *testing.T) {
	t.Parallel()

	// Test that FlagMapping can be created and fields are accessible
	mapping := &config.FlagMapping{
		FlagName:   "test-flag",
		EnvVarName: "TERRAGRUNT_TEST_FLAG",
		Type:       config.FlagTypeString,
		// OriginalFlag would be set to actual cli.Flag in real usage
		OriginalFlag: nil,
	}

	assert.Equal(t, "test-flag", mapping.FlagName)
	assert.Equal(t, "TERRAGRUNT_TEST_FLAG", mapping.EnvVarName)
	assert.Equal(t, config.FlagTypeString, mapping.Type)
	assert.Nil(t, mapping.OriginalFlag)
}
