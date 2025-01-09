package flags

import "strings"

const EnvVarPrefix = "TG_"

// EnvVars calls `EnvVarsWithPrefix` with a default `EnvVarPrefix` prefix.
func EnvVars(names ...string) []string {
	return EnvVarsWithPrefix(EnvVarPrefix, names...)
}

// EnvVarsWithPrefix converts the given flag names into their corresponding environment variables, with a given `prefix` added.
func EnvVarsWithPrefix(prefix string, names ...string) []string {
	var envVars = make([]string, len(names))

	for i := range names {
		suffix := strings.ToUpper(strings.ReplaceAll(names[i], "-", "_"))
		envVars[i] = prefix + suffix
	}

	return envVars
}

// FlagNames returns the given names with the given prefix added.
func FlagNames(prefix string, names ...string) []string {
	var flagNames = make([]string, len(names))

	for i := range names {
		flagNames[i] = prefix + names[i]
	}

	return flagNames
}
