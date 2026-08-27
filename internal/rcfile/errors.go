package rcfile

import "errors"

var (
	// ErrMissingFlagName is returned when an rc file declares a flag without a name.
	ErrMissingFlagName = errors.New("flag declaration is missing a name")

	// ErrMissingCommandName is returned when an rc file declares a command without a name.
	ErrMissingCommandName = errors.New("command declaration is missing a name")

	// ErrMissingFlagDefault is returned when a declared flag has no default value.
	ErrMissingFlagDefault = errors.New("flag declaration is missing a default value")

	// ErrUnsupportedFlagDefault is returned when a declared default is neither a scalar
	// nor a list of scalars.
	ErrUnsupportedFlagDefault = errors.New("unsupported default value type")
)
