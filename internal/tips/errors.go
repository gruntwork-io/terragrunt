package tips

import (
	"strings"
)

// InvalidTipNameError is an error that is returned when an invalid tip name is requested.
type InvalidTipNameError struct {
	requestedName string
	allowedNames  []string
}

// NewInvalidTipNameError returns an [InvalidTipNameError] for the tip requestedName, listing allowedNames.
func NewInvalidTipNameError(requestedName string, allowedNames []string) *InvalidTipNameError {
	return &InvalidTipNameError{
		requestedName: requestedName,
		allowedNames:  allowedNames,
	}
}

func (err InvalidTipNameError) Error() string {
	return "invalid tip suppression requested for `--no-tip`: '" +
		err.requestedName +
		"'; valid tip(s) for suppression: " +
		strings.Join(err.allowedNames, ", ")
}

// Is reports whether target is an *InvalidTipNameError, whatever names it holds.
func (err InvalidTipNameError) Is(target error) bool {
	_, ok := target.(*InvalidTipNameError)
	return ok
}
