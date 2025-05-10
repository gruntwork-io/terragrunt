package flags

import (
	"strings"
)

// Name helps to combine strings into flag names or environment variables in a convenient way.
// Can be passed to subcommands and contain the names of parent commands,
// thus creating env vars as a chain of "TG prefix, parent command names, command name, flag name". For example:
// `TG_HLC_FMT_FILE`, where `hcl` is the parent command, `fmt` is the command and `file` is a flag. Example of use:
//
//	func main () {
//		ParentCommand(Name{TgPrefix})
//	}
//
//	func ParentCommand(prefix Name) {
//		Command(prefix.Append("hcl"))
//	}
//
//	func Command(prefix Name) {
//		Flag(prefix.Append("fmt"))
//	}
//
//	func Flag(prefix Name) {
//		envName := prefix.EnvVar("file") // TG_HCL_FMT_FILE
//	}
type Name []string

// Prepend adds a value to the beginning of the slice.
func (prefix Name) Prepend(val string) Name {
	return append([]string{val}, prefix...)
}

// Append adds a value to the end of the slice.
func (prefix Name) Append(val string) Name {
	return append(prefix, val)
}

// EnvVar returns a string that is the concatenation of the slice values with the given `name`,
// using underscores as separators, replacing dashes with underscores, converting to uppercase.
func (prefix Name) EnvVar(name string) string {
	if name == "" {
		return ""
	}

	name = strings.Join(append(prefix, name), "_")

	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// EnvVars does the same `EnvVar`, except it takes and returns the slice.
func (prefix Name) EnvVars(names ...string) []string {
	var envVars = make([]string, len(names))

	for i := range names {
		envVars[i] = prefix.EnvVar(names[i])
	}

	return envVars
}

// ConfigKey returns a string that is the concatenation of the slice values with the given `name`,
// using underscores as separators, replacing dashes with underscores, converting to lowercase.
func (prefix Name) ConfigKey(key string) string {
	if key == "" {
		return ""
	}

	key = strings.Join(append(prefix, key), "_")

	return strings.ToLower(strings.ReplaceAll(key, "-", "_"))
}

// FlagName returns a string that is the concatenation of the slice values with the given `name`,
// using dashes as separators, replacing dashes with underscores, converting to lowercase.
func (prefix Name) FlagName(name string) string {
	if name == "" {
		return ""
	}

	name = strings.Join(append(prefix, name), "-")

	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// FlagNames does the same `FlagName`, except it takes and returns the slice.
func (prefix Name) FlagNames(names ...string) []string {
	var flagNames = make([]string, len(names))

	for i := range names {
		flagNames[i] = prefix.FlagName(names[i])
	}

	return flagNames
}
