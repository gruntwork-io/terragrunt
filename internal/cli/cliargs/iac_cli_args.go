// Package cliargs provides utilities for parsing and manipulating IaC CLI arguments.
package cliargs

import (
	"slices"
	"strings"
)

// flagsWithSpaceValue contains flags that take space-separated values (not using = format).
var flagsWithSpaceValue = map[string]bool{
	"-var":      true,
	"-var-file": true,
	"-target":   true,
	"-replace":  true,
}

// IacCliArgs represents parsed IaC (terraform/tofu) CLI arguments
// following standard CLI terminology: command, flags, arguments.
type IacCliArgs struct {
	Command   string   // e.g., "apply", "plan", "destroy"
	Flags     []string // e.g., "-input=false", "-auto-approve"
	Arguments []string // e.g., plan files, resource addresses
}

// Parse creates IacCliArgs from a slice of strings.
// It separates the command, flags, and arguments based on the following rules:
// - First non-flag argument is the command
// - Arguments starting with "-" are flags
// - All other arguments after the command are positional arguments
// - Known flags like -var, -var-file, -target, -replace that take space-separated values are handled specially.
func Parse(args []string) *IacCliArgs {
	result := &IacCliArgs{
		Flags:     make([]string, 0),
		Arguments: make([]string, 0),
	}

	skipNext := false

	for i, arg := range args {
		if skipNext {
			skipNext = false

			continue
		}

		if strings.HasPrefix(arg, "-") {
			// Check if this flag takes a space-separated value
			flagName := arg
			if idx := strings.Index(arg, "="); idx > 0 {
				flagName = arg[:idx]
			}

			if flagsWithSpaceValue[flagName] && !strings.Contains(arg, "=") {
				// Flag expects value in next arg
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					result.Flags = append(result.Flags, arg, args[i+1])
					skipNext = true
				} else {
					result.Flags = append(result.Flags, arg)
				}
			} else {
				result.Flags = append(result.Flags, arg)
			}
		} else if result.Command == "" {
			result.Command = arg
		} else {
			result.Arguments = append(result.Arguments, arg)
		}
	}

	return result
}

// ToArgs serializes back to []string with correct ordering:
// [command] [flags...] [arguments...]
func (a *IacCliArgs) ToArgs() []string {
	result := make([]string, 0, 1+len(a.Flags)+len(a.Arguments))

	if a.Command != "" {
		result = append(result, a.Command)
	}

	result = append(result, a.Flags...)
	result = append(result, a.Arguments...)

	return result
}

// AddFlag adds a flag if not already present.
func (a *IacCliArgs) AddFlag(flag string) {
	if !slices.Contains(a.Flags, flag) {
		a.Flags = append(a.Flags, flag)
	}
}

// HasFlag checks if flag exists (by prefix for -flag=value).
func (a *IacCliArgs) HasFlag(name string) bool {
	for _, f := range a.Flags {
		if f == name || strings.HasPrefix(f, name+"=") {
			return true
		}
	}

	return false
}

// RemoveFlag removes a flag by name (handles both -flag and -flag=value).
func (a *IacCliArgs) RemoveFlag(name string) {
	a.Flags = slices.DeleteFunc(a.Flags, func(f string) bool {
		return f == name || strings.HasPrefix(f, name+"=")
	})
}

// AddArgument adds an argument if not already present.
func (a *IacCliArgs) AddArgument(arg string) {
	if !slices.Contains(a.Arguments, arg) {
		a.Arguments = append(a.Arguments, arg)
	}
}
