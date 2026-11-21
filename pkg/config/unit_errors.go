package config

import "fmt"

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
