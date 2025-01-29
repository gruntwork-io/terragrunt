package flags

import (
	"strings"
)

const (
	TgPrefix         = "TG"
	TerragruntPrefix = "TERRAGRUNT"
)

type Prefix []string

func (prefix Prefix) Prepend(val string) Prefix {
	return append([]string{val}, prefix...)
}

func (prefix Prefix) Append(val string) Prefix {
	return append(prefix, val)
}

func (prefix Prefix) EnvVar(name string) string {
	name = strings.Join(append(prefix, name), "_")

	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func (prefix Prefix) EnvVars(names ...string) []string {
	var envVars = make([]string, len(names))

	for i := range names {
		envVars[i] = prefix.EnvVar(names[i])
	}

	return envVars
}

func (prefix Prefix) FlagName(name string) string {
	name = strings.Join(append(prefix, name), "-")

	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func (prefix Prefix) FlagNames(names ...string) []string {
	var flagNames = make([]string, len(names))

	for i := range names {
		flagNames[i] = prefix.FlagName(names[i])
	}

	return flagNames
}
