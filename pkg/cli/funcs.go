package cli

// CompleteFunc is an action to execute when the shell completion flag is set
type CompleteFunc func(ctx *Context) error

// ActionFunc is the action to execute when no commands/subcommands are specified.
type ActionFunc func(ctx *Context) error

// SplitterFunc is used to parse flags containing multiple values.
type SplitterFunc func(s, sep string) []string
