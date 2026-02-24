package flags

import (
	"strings"
)

const (
	// TgPrefix is an environment variable prefix.
	TgPrefix = "TG"

	// TerragruntPrefix is an environment variable deprecated prefix.
	TerragruntPrefix = "TERRAGRUNT"
)

// Prefix helps to combine strings into flag names or environment variables in a convenient way.
// Can be passed to subcommands and contain the names of parent commands,
// thus creating env vars as a chain of "TG prefix, parent command names, command name, flag name". For example:
// `TG_HLC_FMT_FILE`, where `hcl` is the parent command, `fmt` is the command and `file` is a flag. Example of use:
//
//	func main () {
//		ParentCommand(Prefix{TgPrefix})
//	}
//
//	func ParentCommand(prefix Prefix) {
//		Command(prefix.Append("hcl"))
//	}
//
//	func Command(prefix Prefix) {
//		Flag(prefix.Append("fmt"))
//	}
//
//	func Flag(prefix Prefix) {
//		envName := prefix.EnvVar("file") // TG_HCL_FMT_FILE
//	}
type Prefix []string

// Prepend adds a value to the beginning of the slice.
func (prefix Prefix) Prepend(val string) Prefix {
	return append([]string{val}, prefix...)
}

// Append adds a value to the end of the slice.
func (prefix Prefix) Append(val string) Prefix {
	return append(prefix, val)
}

// EnvVar returns a string that is the concatenation of the slice values with the given `name`,
// using underscores as separators, replacing dashes with underscores, converting to uppercase.
func (prefix Prefix) EnvVar(name string) string {
	if name == "" {
		return ""
	}

	name = strings.Join(append(prefix, name), "_")

	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// EnvVars does the same `EnvVar`, except it takes and returns the slice.
func (prefix Prefix) EnvVars(names ...string) []string {
	var envVars = make([]string, len(names))

	for i := range names {
		envVars[i] = prefix.EnvVar(names[i])
	}

	return envVars
}

// FlagName returns a string that is the concatenation of the slice values with the given `name`,
// using dashes as separators, replacing dashes with underscores, converting to lowercase.
func (prefix Prefix) FlagName(name string) string {
	if name == "" {
		return ""
	}

	name = strings.Join(append(prefix, name), "-")

	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// FlagNames does the same `FlagName`, except it takes and returns the slice.
func (prefix Prefix) FlagNames(names ...string) []string {
	var flagNames = make([]string, len(names))

	for i := range names {
		flagNames[i] = prefix.FlagName(names[i])
	}

	return flagNames
}
