package env

import (
	"os"
	"strconv"
	"strings"
)

// GetIntEnv returns the environment value converted to boolean type, or returns the specified fallback value if the variable with the given key is not present.
func GetBoolEnv(key string, fallback bool) bool {
	if strVal, ok := LookupEnv(key); ok {
		if val, err := strconv.ParseBool(strVal); err == nil {
			return val
		}
	}

	return fallback
}

// GetIntEnv returns the environment value converted to intetger type, or returns the specified fallback value if the variable with the given key is not present.
func GetIntEnv(key string, fallback int) int {
	if strVal, ok := LookupEnv(key); ok {
		if val, err := strconv.Atoi(strVal); err == nil {
			return val
		}
	}

	return fallback
}

// GetStringEnv returns an environment variable by the given key, or returns the given fallback value if the env variable is not present.
func GetStringEnv(key string, fallback string) string {
	if val, ok := LookupEnv(key); ok {
		return val
	}

	return fallback
}

// LookupEnv behaves the same as `os.LookupEnv`, but additionally trims spaces in the value.
func LookupEnv(key string) (string, bool) {
	if key == "" {
		return "", false
	}

	val, ok := os.LookupEnv(key)
	val = strings.TrimSpace(val)

	isPresent := ok && val != ""
	return val, isPresent
}
