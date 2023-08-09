package env

import (
	"strconv"
	"strings"
)

// GetBoolEnv converts the given value to the bool type and returns that value, or returns the specified fallback value if the value is empty.
func GetBoolEnv(value string, fallback bool) bool {
	if strVal, ok := nonEmptyValue(value); ok {
		if val, err := strconv.ParseBool(strVal); err == nil {
			return val
		}
	}

	return fallback
}

// GetNegativeBoolEnv converts the given value to the bool type and returns the inverted value, or returns the specified fallback value if the value is empty.
func GetNegativeBoolEnv(value string, fallback bool) bool {
	if strVal, ok := nonEmptyValue(value); ok {
		if val, err := strconv.ParseBool(strVal); err == nil {
			return !val
		}
	}

	return fallback
}

// GetIntEnv converts the given value to the integer type and returns that value, or returns the specified fallback value if the value is empty.
func GetIntEnv(value string, fallback int) int {
	if strVal, ok := nonEmptyValue(value); ok {
		if val, err := strconv.Atoi(strVal); err == nil {
			return val
		}
	}

	return fallback
}

// GetStringEnv returns the same string value, or returns the given fallback value if the value is empty.
func GetStringEnv(value string, fallback string) string {
	if val, ok := nonEmptyValue(value); ok {
		return val
	}

	return fallback
}

// nonEmptyValue trims spaces in the value and returns this trimmed value and true if the value is not empty, otherwise false.
func nonEmptyValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	isPresent := value != ""

	return value, isPresent
}

func ParseEnvs(envs []string) map[string]string {
	envMap := make(map[string]string)

	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)

		if len(parts) == 2 {
			envMap[strings.TrimSpace(parts[0])] = parts[1]
		}
	}

	return envMap
}
