package cliconfig

import "errors"

var (
	// ErrUserConfig is the root of every failure to locate, read, or decode the user's CLI config.
	ErrUserConfig = errors.New("cli config")
	// ErrInvalidUserConfig reports a CLI config that parsed but is not valid.
	ErrInvalidUserConfig = errors.New("invalid cli config")
)
