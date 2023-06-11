package cli

// RunFunc is the action to execute when no commands/subcommands are specified.
type RunFunc func(*Context) error

// SplitterFunc is used to parse flags containing multiple values.
type SplitterFunc func(s, sep string) []string
