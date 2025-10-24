package config

import (
	"reflect"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInferFlagType tests type detection for all flag types
func TestInferFlagType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		flag     cli.Flag
		expected FlagType
	}{
		{
			name: "BoolFlag",
			flag: &cli.BoolFlag{
				Name: "test-bool",
			},
			expected: FlagTypeBool,
		},
		{
			name: "GenericFlag[string]",
			flag: &cli.GenericFlag[string]{
				Name: "test-string",
			},
			expected: FlagTypeString,
		},
		{
			name: "GenericFlag[int]",
			flag: &cli.GenericFlag[int]{
				Name: "test-int",
			},
			expected: FlagTypeInt,
		},
		{
			name: "GenericFlag[int64]",
			flag: &cli.GenericFlag[int64]{
				Name: "test-int64",
			},
			expected: FlagTypeInt64,
		},
		{
			name: "GenericFlag[uint]",
			flag: &cli.GenericFlag[uint]{
				Name: "test-uint",
			},
			expected: FlagTypeUint,
		},
		{
			name: "SliceFlag[string]",
			flag: &cli.SliceFlag[string]{
				Name: "test-slice",
			},
			expected: FlagTypeStringSlice,
		},
		{
			name:     "nil flag",
			flag:     nil,
			expected: FlagTypeUnknown,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := InferFlagType(tt.flag)
			assert.Equal(t, tt.expected, got, "InferFlagType() returned unexpected type")
		})
	}
}

// TestInferFlagType_WithWrapper tests unwrapping of Terragrunt's flags.Flag wrapper
func TestInferFlagType_WithWrapper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wrappedFlag  cli.Flag
		expectedType FlagType
	}{
		{
			name: "wrapped BoolFlag",
			wrappedFlag: flags.NewFlag(&cli.BoolFlag{
				Name: "wrapped-bool",
			}),
			expectedType: FlagTypeBool,
		},
		{
			name: "wrapped GenericFlag[string]",
			wrappedFlag: flags.NewFlag(&cli.GenericFlag[string]{
				Name: "wrapped-string",
			}),
			expectedType: FlagTypeString,
		},
		{
			name: "wrapped GenericFlag[int]",
			wrappedFlag: flags.NewFlag(&cli.GenericFlag[int]{
				Name: "wrapped-int",
			}),
			expectedType: FlagTypeInt,
		},
		{
			name: "wrapped SliceFlag[string]",
			wrappedFlag: flags.NewFlag(&cli.SliceFlag[string]{
				Name: "wrapped-slice",
			}),
			expectedType: FlagTypeStringSlice,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := InferFlagType(tt.wrappedFlag)
			assert.Equal(t, tt.expectedType, got, "InferFlagType() failed to unwrap and detect type")
		})
	}
}

// TestConvertValue_Bool tests bool conversion from various types
func TestConvertValue_Bool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     interface{}
		want      bool
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "bool true",
			input:   true,
			want:    true,
			wantErr: false,
		},
		{
			name:    "bool false",
			input:   false,
			want:    false,
			wantErr: false,
		},
		{
			name:    "string 'true'",
			input:   "true",
			want:    true,
			wantErr: false,
		},
		{
			name:    "string 'false'",
			input:   "false",
			want:    false,
			wantErr: false,
		},
		{
			name:    "string '1'",
			input:   "1",
			want:    true,
			wantErr: false,
		},
		{
			name:    "string '0'",
			input:   "0",
			want:    false,
			wantErr: false,
		},
		{
			name:    "int non-zero",
			input:   42,
			want:    true,
			wantErr: false,
		},
		{
			name:    "int zero",
			input:   0,
			want:    false,
			wantErr: false,
		},
		{
			name:    "float64 non-zero (JSON number)",
			input:   1.0,
			want:    true,
			wantErr: false,
		},
		{
			name:    "float64 zero (JSON number)",
			input:   0.0,
			want:    false,
			wantErr: false,
		},
		{
			name:      "invalid string",
			input:     "not-a-bool",
			wantErr:   true,
			errSubstr: "cannot convert string",
		},
		{
			name:      "unsupported type",
			input:     []string{"test"},
			wantErr:   true,
			errSubstr: "cannot convert",
		},
		{
			name:    "nil value",
			input:   nil,
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertValue(tt.input, FlagTypeBool)

			if tt.wantErr {
				require.Error(t, err, "ConvertValue() should return error")
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr, "Error message should contain expected substring")
				}
				return
			}

			require.NoError(t, err, "ConvertValue() should not return error")
			if tt.input != nil {
				assert.Equal(t, tt.want, got, "ConvertValue() returned unexpected value")
			}
		})
	}
}

// TestConvertValue_String tests string conversion from various types
func TestConvertValue_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		want    string
		wantErr bool
	}{
		{
			name:    "string value",
			input:   "test",
			want:    "test",
			wantErr: false,
		},
		{
			name:    "int value",
			input:   42,
			want:    "42",
			wantErr: false,
		},
		{
			name:    "int64 value",
			input:   int64(123),
			want:    "123",
			wantErr: false,
		},
		{
			name:    "uint value",
			input:   uint(99),
			want:    "99",
			wantErr: false,
		},
		{
			name:    "bool true",
			input:   true,
			want:    "true",
			wantErr: false,
		},
		{
			name:    "bool false",
			input:   false,
			want:    "false",
			wantErr: false,
		},
		{
			name:    "float64 (JSON number)",
			input:   3.14,
			want:    "3.14",
			wantErr: false,
		},
		{
			name:    "nil value",
			input:   nil,
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertValue(tt.input, FlagTypeString)

			if tt.wantErr {
				require.Error(t, err, "ConvertValue() should return error")
				return
			}

			require.NoError(t, err, "ConvertValue() should not return error")
			if tt.input != nil {
				assert.Equal(t, tt.want, got, "ConvertValue() returned unexpected value")
			}
		})
	}
}

// TestConvertValue_Int tests int conversion from various types
func TestConvertValue_Int(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     interface{}
		want      int
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "int value",
			input:   42,
			want:    42,
			wantErr: false,
		},
		{
			name:    "int64 value",
			input:   int64(123),
			want:    123,
			wantErr: false,
		},
		{
			name:    "uint value",
			input:   uint(99),
			want:    99,
			wantErr: false,
		},
		{
			name:    "float64 integer value (JSON)",
			input:   42.0,
			want:    42,
			wantErr: false,
		},
		{
			name:    "string '123'",
			input:   "123",
			want:    123,
			wantErr: false,
		},
		{
			name:    "string '-456'",
			input:   "-456",
			want:    -456,
			wantErr: false,
		},
		{
			name:    "bool true",
			input:   true,
			want:    1,
			wantErr: false,
		},
		{
			name:    "bool false",
			input:   false,
			want:    0,
			wantErr: false,
		},
		{
			name:      "float64 non-integer",
			input:     3.14,
			wantErr:   true,
			errSubstr: "is not an integer",
		},
		{
			name:      "invalid string",
			input:     "not-a-number",
			wantErr:   true,
			errSubstr: "cannot convert string",
		},
		{
			name:      "unsupported type",
			input:     []string{"test"},
			wantErr:   true,
			errSubstr: "cannot convert",
		},
		{
			name:    "nil value",
			input:   nil,
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertValue(tt.input, FlagTypeInt)

			if tt.wantErr {
				require.Error(t, err, "ConvertValue() should return error")
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr, "Error message should contain expected substring")
				}
				return
			}

			require.NoError(t, err, "ConvertValue() should not return error")
			if tt.input != nil {
				assert.Equal(t, tt.want, got, "ConvertValue() returned unexpected value")
			}
		})
	}
}

// TestConvertValue_Int64 tests int64 conversion
func TestConvertValue_Int64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     interface{}
		want      int64
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "int64 value",
			input:   int64(9223372036854775807),
			want:    9223372036854775807,
			wantErr: false,
		},
		{
			name:    "int value",
			input:   42,
			want:    42,
			wantErr: false,
		},
		{
			name:    "string value",
			input:   "123456789",
			want:    123456789,
			wantErr: false,
		},
		{
			name:      "invalid string",
			input:     "not-a-number",
			wantErr:   true,
			errSubstr: "cannot convert string",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertValue(tt.input, FlagTypeInt64)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
				return
			}

			require.NoError(t, err)
			if tt.input != nil {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestConvertValue_Uint tests uint conversion
func TestConvertValue_Uint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     interface{}
		want      uint
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "uint value",
			input:   uint(42),
			want:    42,
			wantErr: false,
		},
		{
			name:    "positive int",
			input:   42,
			want:    42,
			wantErr: false,
		},
		{
			name:      "negative int",
			input:     -42,
			wantErr:   true,
			errSubstr: "cannot convert negative",
		},
		{
			name:    "string value",
			input:   "123",
			want:    123,
			wantErr: false,
		},
		{
			name:      "negative float64",
			input:     -3.14,
			wantErr:   true,
			errSubstr: "cannot convert negative",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertValue(tt.input, FlagTypeUint)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
				return
			}

			require.NoError(t, err)
			if tt.input != nil {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestConvertValue_StringSlice tests []string conversion from various types
func TestConvertValue_StringSlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     interface{}
		want      []string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "[]string value",
			input:   []string{"a", "b", "c"},
			want:    []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "single string",
			input:   "test",
			want:    []string{"test"},
			wantErr: false,
		},
		{
			name:    "comma-separated string",
			input:   "a,b,c",
			want:    []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "comma-separated with spaces",
			input:   "a, b, c",
			want:    []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "[]interface{} with strings",
			input:   []interface{}{"a", "b", "c"},
			want:    []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "[]interface{} with mixed types",
			input:   []interface{}{"a", 42, true},
			want:    []string{"a", "42", "true"},
			wantErr: false,
		},
		{
			name:    "empty slice",
			input:   []string{},
			want:    []string{},
			wantErr: false,
		},
		{
			name:      "unsupported type",
			input:     42,
			wantErr:   true,
			errSubstr: "cannot convert",
		},
		{
			name:    "nil value",
			input:   nil,
			want:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertValue(tt.input, FlagTypeStringSlice)

			if tt.wantErr {
				require.Error(t, err, "ConvertValue() should return error")
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr, "Error message should contain expected substring")
				}
				return
			}

			require.NoError(t, err, "ConvertValue() should not return error")
			if tt.input != nil {
				assert.Equal(t, tt.want, got, "ConvertValue() returned unexpected value")
			}
		})
	}
}

// TestConvertValue_UnsupportedType tests conversion to unsupported types
func TestConvertValue_UnsupportedType(t *testing.T) {
	t.Parallel()

	_, err := ConvertValue("test", FlagTypeUnknown)
	require.Error(t, err, "ConvertValue() should return error for unsupported type")
	assert.Contains(t, err.Error(), "unsupported target type", "Error should mention unsupported type")
}

// TestBuildFlagRegistry tests flag registry construction
func TestBuildFlagRegistry(t *testing.T) {
	t.Parallel()

	// Create a set of test flags
	testFlags := cli.Flags{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
		},
		&cli.GenericFlag[string]{
			Name:    "output",
			Aliases: []string{"o"},
		},
		&cli.GenericFlag[int]{
			Name: "count",
		},
		&cli.SliceFlag[string]{
			Name: "tags",
		},
	}

	registry := BuildFlagRegistry(testFlags)

	// Verify registry size
	assert.Len(t, registry, 4, "Registry should contain 4 flags")

	// Verify each flag mapping
	t.Run("verbose flag", func(t *testing.T) {
		mapping, exists := registry["verbose"]
		require.True(t, exists, "Registry should contain 'verbose' flag")
		assert.Equal(t, "verbose", mapping.Name)
		assert.Equal(t, FlagTypeBool, mapping.Type)
		assert.NotNil(t, mapping.Flag)
	})

	t.Run("output flag", func(t *testing.T) {
		mapping, exists := registry["output"]
		require.True(t, exists, "Registry should contain 'output' flag")
		assert.Equal(t, "output", mapping.Name)
		assert.Equal(t, FlagTypeString, mapping.Type)
	})

	t.Run("count flag", func(t *testing.T) {
		mapping, exists := registry["count"]
		require.True(t, exists, "Registry should contain 'count' flag")
		assert.Equal(t, "count", mapping.Name)
		assert.Equal(t, FlagTypeInt, mapping.Type)
	})

	t.Run("tags flag", func(t *testing.T) {
		mapping, exists := registry["tags"]
		require.True(t, exists, "Registry should contain 'tags' flag")
		assert.Equal(t, "tags", mapping.Name)
		assert.Equal(t, FlagTypeStringSlice, mapping.Type)
	})
}

// TestBuildFlagRegistry_WithWrapper tests registry with wrapped flags
func TestBuildFlagRegistry_WithWrapper(t *testing.T) {
	t.Parallel()

	testFlags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name: "debug",
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name: "config",
		}),
	}

	registry := BuildFlagRegistry(testFlags)

	assert.Len(t, registry, 2, "Registry should contain 2 flags")

	t.Run("wrapped debug flag", func(t *testing.T) {
		mapping, exists := registry["debug"]
		require.True(t, exists, "Registry should contain 'debug' flag")
		assert.Equal(t, FlagTypeBool, mapping.Type, "Should detect bool type through wrapper")
	})

	t.Run("wrapped config flag", func(t *testing.T) {
		mapping, exists := registry["config"]
		require.True(t, exists, "Registry should contain 'config' flag")
		assert.Equal(t, FlagTypeString, mapping.Type, "Should detect string type through wrapper")
	})
}

// TestBuildFlagRegistry_EmptyFlags tests registry with empty flag list
func TestBuildFlagRegistry_EmptyFlags(t *testing.T) {
	t.Parallel()

	registry := BuildFlagRegistry(cli.Flags{})
	assert.Empty(t, registry, "Registry should be empty for empty flag list")
}

// TestBuildFlagRegistry_NilFlag tests registry with nil flag
func TestBuildFlagRegistry_NilFlag(t *testing.T) {
	t.Parallel()

	testFlags := cli.Flags{
		nil,
		&cli.BoolFlag{Name: "test"},
	}

	registry := BuildFlagRegistry(testFlags)
	assert.Len(t, registry, 1, "Registry should skip nil flags")
	_, exists := registry["test"]
	assert.True(t, exists, "Registry should contain non-nil flag")
}

// TestConvertValue_AllTypeCombinations tests all type conversion combinations
func TestConvertValue_AllTypeCombinations(t *testing.T) {
	t.Parallel()

	// This is a comprehensive test to ensure all type combinations are handled
	types := []FlagType{
		FlagTypeBool,
		FlagTypeString,
		FlagTypeInt,
		FlagTypeInt64,
		FlagTypeUint,
		FlagTypeStringSlice,
	}

	inputs := []interface{}{
		true,
		"test",
		42,
		int64(123),
		uint(99),
		[]string{"a", "b"},
	}

	for _, targetType := range types {
		for _, input := range inputs {
			name := string(targetType) + "_from_" + reflect.TypeOf(input).String()
			t.Run(name, func(t *testing.T) {
				// Just ensure it doesn't panic and returns a result or error
				_, err := ConvertValue(input, targetType)
				// We accept both success and error - just checking for panics
				_ = err
			})
		}
	}
}
