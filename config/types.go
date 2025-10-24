package config

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
)

// FlagType represents the type of a CLI flag for type conversion purposes
type FlagType string

const (
	// FlagTypeBool represents a boolean flag
	FlagTypeBool FlagType = "bool"
	// FlagTypeString represents a string flag
	FlagTypeString FlagType = "string"
	// FlagTypeInt represents an integer flag
	FlagTypeInt FlagType = "int"
	// FlagTypeInt64 represents a 64-bit integer flag
	FlagTypeInt64 FlagType = "int64"
	// FlagTypeUint represents an unsigned integer flag
	FlagTypeUint FlagType = "uint"
	// FlagTypeStringSlice represents a string slice flag
	FlagTypeStringSlice FlagType = "[]string"
	// FlagTypeUnknown represents an unknown or unsupported flag type
	FlagTypeUnknown FlagType = "unknown"
)

// FlagMapping represents the mapping between a flag name and its type information
type FlagMapping struct {
	// Name is the primary flag name
	Name string
	// Type is the detected flag type
	Type FlagType
	// Flag is the original cli.Flag interface
	Flag cli.Flag
}

// ConfigError represents a configuration-related error
type ConfigError struct {
	message string
}

func (e ConfigError) Error() string {
	return e.message
}

// NewConfigError creates a new ConfigError with a formatted message
func NewConfigError(format string, args ...interface{}) ConfigError {
	return ConfigError{message: fmt.Sprintf(format, args...)}
}

// BuildFlagRegistry introspects cli.Flags and builds type mappings
// It iterates through all flags, extracts their primary names, detects types,
// and returns a complete registry mapping flag names to their type information.
func BuildFlagRegistry(flagList cli.Flags) map[string]FlagMapping {
	registry := make(map[string]FlagMapping)

	for _, flag := range flagList {
		if flag == nil {
			continue
		}

		// Extract primary flag name (first in Names array)
		names := flag.Names()
		if len(names) == 0 {
			continue
		}
		primaryName := names[0]

		// Detect flag type
		flagType := InferFlagType(flag)

		// Build FlagMapping
		mapping := FlagMapping{
			Name: primaryName,
			Type: flagType,
			Flag: flag,
		}

		// Store in registry using primary name as key
		registry[primaryName] = mapping
	}

	return registry
}

// InferFlagType determines FlagType from cli.Flag using type assertions
// It handles Terragrunt's flags.Flag wrapper by unwrapping it first,
// then uses type switches to detect the underlying flag type.
func InferFlagType(flag cli.Flag) FlagType {
	if flag == nil {
		return FlagTypeUnknown
	}

	// Handle Terragrunt's flags.Flag wrapper - unwrap it first
	if wrapper, ok := flag.(*flags.Flag); ok {
		// Recursively infer type from the wrapped flag
		return InferFlagType(wrapper.Flag)
	}

	// Type switch on cli.Flag interface to detect concrete flag types
	switch flag.(type) {
	case *cli.BoolFlag:
		return FlagTypeBool

	case *cli.GenericFlag[string]:
		return FlagTypeString

	case *cli.GenericFlag[int]:
		return FlagTypeInt

	case *cli.GenericFlag[int64]:
		return FlagTypeInt64

	case *cli.GenericFlag[uint]:
		return FlagTypeUint

	case *cli.SliceFlag[string]:
		return FlagTypeStringSlice

	default:
		// For unknown types, try to infer from the flag's value using reflection
		if flag.Value() != nil {
			return inferTypeFromValue(flag.Value())
		}
		return FlagTypeUnknown
	}
}

// inferTypeFromValue attempts to infer the flag type from the flag's value using reflection
func inferTypeFromValue(flagValue cli.FlagValue) FlagType {
	if flagValue == nil {
		return FlagTypeUnknown
	}

	// Check if it's a bool flag using the interface method
	if flagValue.IsBoolFlag() {
		return FlagTypeBool
	}

	// Use reflection to inspect the actual value
	value := flagValue.Get()
	if value == nil {
		return FlagTypeUnknown
	}

	t := reflect.TypeOf(value)

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Map Go types to FlagType
	switch t.Kind() {
	case reflect.String:
		return FlagTypeString
	case reflect.Bool:
		return FlagTypeBool
	case reflect.Int:
		return FlagTypeInt
	case reflect.Int64:
		return FlagTypeInt64
	case reflect.Uint:
		return FlagTypeUint
	case reflect.Slice:
		// Check slice element type
		if t.Elem().Kind() == reflect.String {
			return FlagTypeStringSlice
		}
		return FlagTypeUnknown
	default:
		return FlagTypeUnknown
	}
}

// ConvertValue converts interface{} to the target type specified by targetType
// It handles all supported flag types with proper error handling and type conversion.
// Returns ConfigError for type mismatches or conversion failures.
func ConvertValue(rawValue interface{}, targetType FlagType) (interface{}, error) {
	// Handle nil values
	if rawValue == nil {
		return nil, nil
	}

	switch targetType {
	case FlagTypeBool:
		return convertToBool(rawValue)

	case FlagTypeString:
		return convertToString(rawValue)

	case FlagTypeInt:
		return convertToInt(rawValue)

	case FlagTypeInt64:
		return convertToInt64(rawValue)

	case FlagTypeUint:
		return convertToUint(rawValue)

	case FlagTypeStringSlice:
		return convertToStringSlice(rawValue)

	default:
		return nil, NewConfigError("unsupported target type: %s (source value type: %T)", targetType, rawValue)
	}
}

// convertToBool converts a value to bool
// Handles: bool directly, string "true"/"false" via strconv.ParseBool, int (0=false, non-zero=true)
func convertToBool(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return false, NewConfigError("cannot convert string %q to bool: %v", v, err)
		}
		return parsed, nil
	case int:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case uint:
		return v != 0, nil
	case float64:
		// Handle JSON numbers which are parsed as float64
		return v != 0, nil
	default:
		return false, NewConfigError("cannot convert %T to bool (expected bool, string, or number)", value)
	}
}

// convertToString converts a value to string
// Handles: string directly, any value via fmt.Sprint
func convertToString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case fmt.Stringer:
		return v.String(), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint:
		return strconv.FormatUint(uint64(v), 10), nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		// Handle JSON numbers
		return fmt.Sprintf("%v", v), nil
	default:
		// Fallback: convert any value to string
		return fmt.Sprintf("%v", v), nil
	}
}

// convertToInt converts a value to int
// Handles: int directly, JSON number, string via strconv.Atoi, bool (true=1, false=0)
func convertToInt(value interface{}) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		if v > math.MaxInt || v < math.MinInt {
			return 0, NewConfigError("int64 value %d overflows int", v)
		}
		return int(v), nil
	case uint:
		if v > math.MaxInt {
			return 0, NewConfigError("uint value %d overflows int", v)
		}
		return int(v), nil
	case float64:
		// Handle JSON numbers which are parsed as float64
		if v != float64(int(v)) {
			return 0, NewConfigError("float64 value %f is not an integer", v)
		}
		return int(v), nil
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, NewConfigError("cannot convert string %q to int: %v", v, err)
		}
		return parsed, nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, NewConfigError("cannot convert %T to int (expected int, number, string, or bool)", value)
	}
}

// convertToInt64 converts a value to int64
func convertToInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case uint:
		return int64(v), nil
	case float64:
		// Handle JSON numbers
		if v != float64(int64(v)) {
			return 0, NewConfigError("float64 value %f is not an integer", v)
		}
		return int64(v), nil
	case string:
		parsed, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			return 0, NewConfigError("cannot convert string %q to int64: %v", v, err)
		}
		return parsed, nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, NewConfigError("cannot convert %T to int64 (expected int64, number, string, or bool)", value)
	}
}

// convertToUint converts a value to uint
func convertToUint(value interface{}) (uint, error) {
	switch v := value.(type) {
	case uint:
		return v, nil
	case int:
		if v < 0 {
			return 0, NewConfigError("cannot convert negative int %d to uint", v)
		}
		return uint(v), nil
	case int64:
		if v < 0 {
			return 0, NewConfigError("cannot convert negative int64 %d to uint", v)
		}
		return uint(v), nil
	case float64:
		// Handle JSON numbers
		if v < 0 {
			return 0, NewConfigError("cannot convert negative float64 %f to uint", v)
		}
		if v != float64(uint(v)) {
			return 0, NewConfigError("float64 value %f is not an unsigned integer", v)
		}
		return uint(v), nil
	case string:
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, NewConfigError("cannot convert string %q to uint: %v", v, err)
		}
		return uint(parsed), nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, NewConfigError("cannot convert %T to uint (expected uint, non-negative number, string, or bool)", value)
	}
}

// convertToStringSlice converts a value to []string
// Handles: []string directly, []interface{} (convert each element), string (single element or comma-separated)
func convertToStringSlice(value interface{}) ([]string, error) {
	switch v := value.(type) {
	case []string:
		return v, nil

	case string:
		// Check if it's a comma-separated string
		if strings.Contains(v, ",") {
			// Split comma-separated string
			parts := strings.Split(v, ",")
			result := make([]string, len(parts))
			for i, part := range parts {
				result[i] = strings.TrimSpace(part)
			}
			return result, nil
		}
		// Single string becomes single-element slice
		return []string{v}, nil

	case []interface{}:
		// Convert each element to string
		result := make([]string, len(v))
		for i, item := range v {
			str, err := convertToString(item)
			if err != nil {
				return nil, NewConfigError("element %d: %v", i, err)
			}
			result[i] = str
		}
		return result, nil

	default:
		// Try to handle other slice types using reflection
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Slice {
			result := make([]string, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				elem := rv.Index(i).Interface()
				str, err := convertToString(elem)
				if err != nil {
					return nil, NewConfigError("element %d: %v", i, err)
				}
				result[i] = str
			}
			return result, nil
		}

		return nil, NewConfigError("cannot convert %T to []string (expected []string, []interface{}, or string)", value)
	}
}
