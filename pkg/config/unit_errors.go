package config

import (
	"fmt"
	"strings"
)

// UnparsedIncludeError is an assembly reaching an included file that the parse it builds on stopped before.
type UnparsedIncludeError struct {
	// ConfigPath is the file that includes the unparsed file.
	ConfigPath string
	// IncludePath is the unparsed file.
	IncludePath string
}

func (e UnparsedIncludeError) Error() string {
	return fmt.Sprintf("%s: the parse stopped before reaching %s", e.ConfigPath, e.IncludePath)
}

// PartialParseReadsDependencyError is a partial parse reaching parts it was asked for that read dependency outputs,
// which it does not fetch.
type PartialParseReadsDependencyError struct {
	// ConfigPath is the file that holds the parts.
	ConfigPath string
	// Parts names the parts that read dependency outputs.
	Parts []string
}

func (e PartialParseReadsDependencyError) Error() string {
	return fmt.Sprintf(
		"%s: %s read dependency outputs, which a partial parse does not fetch",
		e.ConfigPath,
		strings.Join(e.Parts, " and "),
	)
}
