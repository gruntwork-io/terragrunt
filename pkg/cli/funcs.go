package cli

// ActionFunc is the action to execute when no subcommands are specified.
type ActionFunc func(*Context) error

// SplitterFunc is used to parse flags containing multiple values.
type SplitterFunc func(s, sep string) []string
