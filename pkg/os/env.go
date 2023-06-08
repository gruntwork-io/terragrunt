package os

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
)

// GetIntEnv returns the environment value converted to boolean type, or returns the specified fallback value if the variable with the given key is not present.
func GetBoolEnv(key string, fallback bool) (bool, error) {
	if strVal, ok := LookupEnv(key); ok {
		val, err := strconv.ParseBool(strVal)
		if err != nil {
			err = errors.WithStackTrace(err)
		}
		return val, err
	}

	return fallback, nil
}

// GetIntEnv returns the environment value converted to intetger type, or returns the specified fallback value if the variable with the given key is not present.
func GetIntEnv(key string, fallback int) (int, error) {
	if strVal, ok := LookupEnv(key); ok {
		val, err := strconv.Atoi(strVal)
		if err != nil {
			err = errors.WithStackTrace(err)
		}
		return val, err
	}

	return fallback, nil
}

// GetStringEnv returns an environment variable by the given key, or returns the given fallback value if the env variable is not present.
func GetStringEnv(key string, fallback string) string {
	if val, ok := LookupEnv(key); ok {
		return val
	}

	return fallback
}

// GetStringSliceEnv returns the environment value converted to []string type separated by the given `sep` value,
// or returns the specified fallback value if the variable with the given key is not present.
func GetStringSliceEnv(key, sep string, splitter func(s, sep string) []string, fallback []string) []string {
	if strVal, ok := LookupEnv(key); ok {
		vals := splitter(strVal, sep)

		for i := range vals {
			vals[i] = strings.TrimSpace(vals[i])
		}
		return vals
	}

	return fallback
}

// GetStringMapEnv returns the environment value converted to map[string]string type separated by the given `sliceSep` and `mapSep` values,
// or returns the specified fallback value if the variable with the given key is not present.
func GetStringMapEnv(key, sliceSep, mapSep string, splitter func(s, sep string) []string, fallback map[string]string) (map[string]string, error) {
	if strVal, ok := LookupEnv(key); ok {
		fmt.Println("------------")
		vals := splitter(strVal, sliceSep)
		keyVals := make(map[string]string)

		for i := range vals {
			str := strings.TrimSpace(vals[i])

			parts := splitter(str, mapSep)
			if len(parts) != 2 {
				err := fmt.Errorf("valid format: key%svalue", mapSep)
				return nil, errors.WithStackTrace(err)
			}

			keyVals[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}

		return keyVals, nil
	}

	return fallback, nil
}

// LookupEnv retrieves the value of the environment variable named by the key. If the variable is present in the environment and the value is not empty the
// value is returned and the boolean is true. Otherwise the returned value will be empty and the boolean will be false.
func LookupEnv(key string) (string, bool) {
	if key == "" {
		return "", false
	}

	val, ok := os.LookupEnv(key)
	val = strings.TrimSpace(val)

	isPresent := ok && val != ""
	return val, isPresent
}
